# Cashback wallet

A demo payment wallet with campaign-based cash back, built for Indonesian rupiah (IDR) users first.

| Folder | What |
|---|---|
| [`server/`](server) | Go API (Gin), PostgreSQL 18, Redis 8 |
| [`app/`](app) | Expo / React Native mobile app (runs in Expo Go) |

Contributor and agent guides with more detail: [`AGENTS.md`](AGENTS.md), [`server/AGENTS.md`](server/AGENTS.md), [`app/AGENTS.md`](app/AGENTS.md).

## Quick start

```bash
cd server && docker compose up -d --build          # API :8080, Postgres :5432, Redis :6379
DATABASE_URL='postgres://cashback:cashback@localhost:5432/cashback?sslmode=disable' go run ./cmd/seed
go run ./cmd/campaign create --name "Launch 5%" --currency IDR --rate-bps 500 \
  --min 20000 --daily-cap 50000 --budget 10000000 --round-to 1 --starts-at now
cd ../app && npx expo start                        # Metro :8081, open in Expo Go
```

Demo users: `andi@example.com`, `sarah@example.com`, `budi@example.com` (no password). Without a live campaign, payments earn no cash back.

---

## API

All app endpoints are under `/v1`. Request and response bodies are JSON.

### Conventions

- **Money** is always an object with a decimal string, never a JSON number:
  ```json
  { "value": "-35000.00", "currency": "IDR" }
  ```
  The value has exactly as many decimals as the currency's ISO 4217 minor unit (IDR 2, JPY 0). Negative means money out. Request amounts are always positive; the server applies the sign.
- **Times** are RFC 3339 in UTC, e.g. `"2026-10-13T07:42:00Z"`.
- **Errors** are `{"error": "message for the user", "code": "machine_code"}`. `code` is optional; clients branch on `code` and display `error`.
- **Auth (demo only):** after sign-in, send `X-User-ID: <user uuid>` on every request. There are no passwords or tokens yet.
- **Lists** are newest first. An optional `limit` query parameter must be a positive integer.
- **Request ids:** every response carries `X-Request-ID` (the client's, if well-formed, or a generated one) for finding the request in the logs.

### Endpoints

| Method | Path | Auth | Idempotency-Key | Purpose |
|---|---|---|---|---|
| `GET` | `/health` | – | – | Liveness check (unversioned, plain `ok`) |
| `POST` | `/v1/auth/sign-in` | – | – | Look up a user by email |
| `GET` | `/v1/accounts` | ✓ | – | The user's main and cash back balances |
| `GET` | `/v1/transactions` | ✓ | – | Main account activity |
| `POST` | `/v1/payments` | ✓ | **required** | Pay a merchant; may earn cash back |
| `GET` | `/v1/cashback/campaign` | ✓ | – | Live (or last) campaign and the user's progress today |
| `GET` | `/v1/cashback/history` | ✓ | – | Cash back balance and its activity |
| `POST` | `/v1/cashback/redeem` | ✓ | **required** | Move cash back into the main balance |

#### `POST /v1/auth/sign-in`

```json
// request
{ "email": "andi@example.com" }
// 200
{ "user": { "id": "7c0e…", "email": "andi@example.com", "name": "Andi" } }
```

`400` invalid email · `401` `no account for this email`.

#### `GET /v1/accounts`

```json
{
  "accounts": [
    { "id": "…", "kind": "main",     "balance": { "value": "12450000.00", "currency": "IDR" } },
    { "id": "…", "kind": "cashback", "balance": { "value": "3250.00",     "currency": "IDR" } }
  ]
}
```

System accounts (campaign budgets, funding) are never returned.

#### `GET /v1/transactions?limit=20`

Main account activity. `kind` is `in`, `out` or `cashback` (a redemption credit). A payment that earned cash back includes `cashback`.

```json
{
  "transactions": [
    {
      "id": "…", "name": "Kopi Kenangan", "description": "Payment", "kind": "out",
      "amount": { "value": "-35000.00", "currency": "IDR" },
      "cashback": { "value": "1750.00", "currency": "IDR" },
      "createdAt": "2026-10-13T01:42:00Z"
    }
  ]
}
```

#### `POST /v1/payments`

Headers: `X-User-ID`, `Idempotency-Key`.

```json
// request
{ "merchantId": "kopi-kenangan", "merchantName": "Kopi Kenangan",
  "amount": { "value": "35000", "currency": "IDR" } }
// 201
{
  "payment": {
    "transaction": { "id": "…", "name": "Kopi Kenangan", "kind": "out",
                     "amount": { "value": "-35000.00", "currency": "IDR" },
                     "cashback": { "value": "1750.00", "currency": "IDR" }, "…": "…" },
    "balance":  { "value": "12415000.00", "currency": "IDR" },
    "cashback": { "amount": { "value": "1750.00", "currency": "IDR" }, "reason": "awarded" }
  }
}
```

`cashback` is `null` when no campaign is live. Otherwise `reason` is one of `awarded`, `below_minimum`, `daily_cap_reached`, `budget_exhausted` (see [How a campaign works](#how-a-cash-back-campaign-works)). Merchants are not stored server-side; the id and name come from the request.

| Status | `code` | When |
|---|---|---|
| 400 | `invalid_idempotency_key` | Header missing or not 8–128 of `[A-Za-z0-9_-]` |
| 400 | `invalid_amount` | Not a positive decimal, too many decimals, or overflow |
| 400 | – | Missing merchant, malformed body |
| 409 | `in_progress` | Same key is still being processed |
| 422 | `insufficient_funds` | Balance too low |
| 422 | `unsupported_currency` | User has no account in that currency |
| 422 | `idempotency_key_reused` | Same key, different request |
| 503 | `unavailable` | Idempotency store (Redis) unreachable; nothing was done |

A replayed response carries the header `Idempotent-Replayed: true`.

#### `GET /v1/cashback/campaign?currency=IDR`

`currency` defaults to `IDR`. Returns the live campaign, otherwise the most recently ended one, otherwise `{"campaign": null}`.

```json
{
  "campaign": {
    "id": "…", "name": "Launch 5%", "status": "active",
    "rateBps": 500,
    "minSpend":        { "value": "20000.00",    "currency": "IDR" },
    "dailyCap":        { "value": "50000.00",    "currency": "IDR" },
    "earnedToday":     { "value": "1750.00",     "currency": "IDR" },
    "budgetRemaining": { "value": "9998250.00",  "currency": "IDR" },
    "resetAt": "2026-10-13T17:00:00Z"
  }
}
```

`resetAt` is the next midnight in the campaign's time zone (Asia/Jakarta), expressed in UTC.

#### `GET /v1/cashback/history?currency=IDR&limit=20`

```json
{
  "balance": { "value": "3250.00", "currency": "IDR" },
  "transactions": [
    { "id": "…", "name": "Cash back", "description": "From Kopi Kenangan", "kind": "cashback",
      "amount": { "value": "1750.00", "currency": "IDR" }, "createdAt": "…" },
    { "id": "…", "name": "Cash back redeemed", "description": "To available balance", "kind": "out",
      "amount": { "value": "-5000.00", "currency": "IDR" }, "createdAt": "…" }
  ]
}
```

`404` `no_cashback_account` if the user has no cash back account in that currency.

#### `POST /v1/cashback/redeem`

Headers: `X-User-ID`, `Idempotency-Key`. The app sends the full balance it displayed.

```json
// request
{ "amount": { "value": "3250.00", "currency": "IDR" } }
// 201
{
  "redemption": {
    "transaction":     { "kind": "cashback", "amount": { "value": "3250.00", "currency": "IDR" }, "…": "…" },
    "balance":         { "value": "12418250.00", "currency": "IDR" },
    "cashbackBalance": { "value": "0.00",        "currency": "IDR" }
  }
}
```

Errors mirror payments (`insufficient_funds` here means more than the cash back balance).

---

## Database design

PostgreSQL, with every schema change a [goose](https://github.com/pressly/goose) migration in [`server/migrations/`](server/migrations), applied on startup. SQL is hand-written in [`server/queries/`](server/queries) and compiled to typed Go by sqlc.

```mermaid
erDiagram
    currencies ||--o{ accounts : "denominates"
    users ||--o{ accounts : "owns (main, cashback)"
    accounts ||--o{ transactions : "ledger of"
    accounts ||--|| campaigns : "holds budget of"
    campaigns ||--o{ cashback_awards : "pays"
    users ||--o{ cashback_awards : "earns"
    transactions ||--o| cashback_awards : "payment / debit / credit"

    currencies {
        char3 code PK
        smallint minor_unit
    }
    users {
        uuid id PK
        text email UK
        text name
    }
    accounts {
        uuid id PK
        uuid user_id FK "NULL for system"
        text kind
        char3 currency
        bigint balance_minor
    }
    transactions {
        uuid id PK
        uuid account_id FK
        text kind
        bigint amount_minor
        char3 currency
        text idempotency_key
        timestamptz created_at
    }
    campaigns {
        uuid id PK
        uuid account_id FK
        char3 currency
        int rate_bps
        bigint min_spend_minor
        bigint daily_cap_minor
        bigint award_increment_minor
        bigint budget_minor
        text timezone
        timestamptz starts_at
        timestamptz ends_at
    }
    cashback_awards {
        uuid id PK
        uuid campaign_id FK
        uuid user_id FK
        uuid payment_transaction_id UK
        uuid debit_transaction_id UK
        uuid credit_transaction_id UK
        bigint amount_minor
        date campaign_day
    }
```

### Tables

| Table | Role |
|---|---|
| `currencies` | ISO 4217 code and `minor_unit` (decimal places). Reference data, seeded by migration. |
| `users` | Email is unique and must be lower-case. |
| `accounts` | A balance in one currency. `kind` is `main` / `cashback` (one each per user and currency) or the system kinds `campaign` (one per campaign, holds its budget) and `funding` (one per currency, where campaign budgets come from). |
| `transactions` | The ledger. Every change to a balance is a row here. `kind` is `in`, `out` or `cashback`. |
| `campaigns` | Cash back rules, the campaign's budget account, its time zone and active window. |
| `cashback_awards` | Links one payment to the two transactions that paid its cash back, with the campaign day for the daily cap. |

### Design decisions

- **Integer minor units.** All amounts are `BIGINT` columns named `*_minor` (Rp 35.000 is `3500000` sen). No floats, no `NUMERIC`, no Postgres `MONEY` (it depends on locale). Conversion to and from decimal strings happens only at the API edge ([`internal/money`](server/internal/money/money.go)).
- **Stored balances, kept equal to the ledger.** `accounts.balance_minor` must always equal `SUM(transactions.amount_minor)` for that account. Every write updates the balance and inserts the transaction in the **same DB transaction**; reconciliation queries and integration tests check it.
- **Signs are enforced.** `amount_minor <> 0`, and a CHECK ties `kind = 'out'` to a negative amount.
- **No overdraft, without races.** Debits are one conditional statement: `UPDATE accounts SET balance_minor = balance_minor - $amt WHERE id = $id AND balance_minor >= $amt RETURNING …`. No row returned means insufficient funds. A `CHECK (kind = 'funding' OR balance_minor >= 0)` backs it up.
- **Double-entry between accounts.** Campaign funding, cash back awards and redemptions are always a pair of rows that net to zero. Only `funding` may go negative: it represents money the business put in.
- **Currencies can't mix.** `accounts` has `UNIQUE (id, currency)` and `transactions` and `campaigns` reference it with a composite foreign key, so a row can't land on an account in another currency.
- **Owner rules in the schema.** `CHECK ((kind IN ('main','cashback')) = (user_id IS NOT NULL))`; a partial unique index allows one `funding` account per currency.
- **One live campaign per currency**, enforced by an exclusion constraint over `tstzrange(starts_at, COALESCE(ends_at, 'infinity'))` (`btree_gist`). Future campaigns can be scheduled back to back.
- **One award per payment.** `cashback_awards.payment_transaction_id` and both leg ids are `UNIQUE`. An index on `(campaign_id, user_id, campaign_day)` makes the daily-cap sum cheap.
- **Idempotency backstop.** `UNIQUE (account_id, idempotency_key)` on `transactions` (see [Idempotency](#idempotency)).
- **Lock order** in every money operation: user's main account → campaign (and its account) → user's cash back account. This keeps payments and redemptions from deadlocking each other.

### How each operation writes

| Operation | Rows written (one DB transaction) |
|---|---|
| Launch campaign | `funding` −budget, `campaign` +budget; `campaigns` row |
| Payment | main account `out` −amount (with idempotency key) |
| …with cash back | `campaign` `out` −award, user `cashback` +award, `cashback_awards` row; if the budget hits 0, `campaigns.ends_at` is set |
| Redemption | user `cashback` `out` −amount, main `cashback` +amount (both with the idempotency key) |
| End campaign | leftover `campaign` → `funding`; `ends_at` set |

Seeded opening balances and payments to merchants are single-entry, because merchants aren't accounts (yet).

---

## Server deep dives

### Rounding for cash back

Cash back maths lives in one pure function, [`cashback.Award`](server/internal/cashback/cashback.go), working only on integer minor units. Rounding is **always down**, so the business never pays more than the advertised rate.

1. **Apply the rate, floored.** `rate_bps` is basis points (500 = 5%). The award is `floor(amount × bps / 10000)`, computed as `amount/10000*bps + amount%10000*bps/10000` so it can't overflow `int64` even for huge amounts.
2. **Round down to the campaign's increment.** `award_increment_minor` sets the smallest unit an award may have. IDR has two ISO decimals (sen), but nobody uses sen, so IDR campaigns use `100` (whole rupiah). `cmd/campaign --round-to 1` means "1 rupiah" and is stored as `100`.
3. **If that leaves zero**, nothing is awarded and the reason is `below_minimum`.
4. **Clamp to the daily cap**, then **to the remaining budget**, rounding each clamped value down to the increment again, so the user's balance never holds a fraction of the increment.

Worked example, 5% campaign, whole-rupiah increment:

| Payment | 5% (exact) | Floored to sen | Rounded to Rp | Awarded |
|---|---|---|---|---|
| Rp 21.234,56 (`2123456`) | 106 172,8 sen | `106172` | `106100` | **Rp 1.061** |
| Rp 35.000 (`3500000`) | 175 000 sen | `175000` | `175000` | **Rp 1.750** |

Edge cases:

- **Partial awards at a limit** count as `awarded`. The reasons `daily_cap_reached` and `budget_exhausted` are only reported when the limit leaves the user with nothing.
- **Budget edge:** if the award is larger than the budget left, the user gets the rest (rounded down) and, if that brings the budget to exactly zero, the campaign ends in the same transaction. No overspend, and no stranded remainder unless it's smaller than one increment.
- Unit tests cover the arithmetic, including a property check that an award is never negative, never above the cap or budget, and always a multiple of the increment.

### Idempotency

`POST /v1/payments` and `POST /v1/cashback/redeem` require an `Idempotency-Key` header (8–128 of `[A-Za-z0-9_-]`). The app sends a UUID per attempt and reuses it when retrying. The server guarantees **at most one effect per key**, using two layers:

```mermaid
sequenceDiagram
    participant App
    participant API
    participant Redis
    participant Postgres
    App->>API: POST /v1/payments (Idempotency-Key: K)
    API->>Redis: SET NX idem:{user}:{K} {pending, fingerprint} EX 60s
    alt key new
        API->>Postgres: BEGIN; debit; INSERT … idempotency_key=K; award; COMMIT
        API->>Redis: SET idem:{user}:{K} {done, status, body} EX 24h
        API-->>App: 201
    else done, same fingerprint
        API-->>App: cached status + body, Idempotent-Replayed: true
    else pending
        API-->>App: 409 in_progress
    else different fingerprint
        API-->>App: 422 idempotency_key_reused
    end
```

**Layer 1: Redis** ([`internal/idempotency`](server/internal/idempotency/idempotency.go), wired by `runIdempotent` in [`idempotent.go`](server/idempotent.go)).

- Keys are scoped per user (`idem:{userID}:{key}`), so two users can't collide.
- The stored **fingerprint** is a SHA-256 of the operation name plus the request fields (`"payment"`, merchant, amount, currency). Prefixing the operation means a payment key can't be replayed as a redemption.
- **Definite outcomes are cached for 24 hours**, including business refusals such as `insufficient_funds`. A retry gets the same answer instead of re-running.
- **Unexpected errors release the key**, so the client can retry safely.
- The pending lock expires after 60 seconds in case the server dies mid-request.
- Once processing starts, the outcome is recorded even if the client disconnects (`context.WithoutCancel`).
- **Fail closed:** if Redis is unreachable the endpoint returns `503 unavailable` and moves no money.

**Layer 2: Postgres backstop.** The key is stored on the transaction rows, with `UNIQUE (account_id, idempotency_key)`. If Redis lost the key (flush, eviction, a crash between commit and cache write), the second insert hits the constraint, the attempt rolls back (undoing its debit), and the server returns the **original** transaction with `Idempotent-Replayed: true`, logged at `warn`. If the stored row doesn't match the new request, it's `422 idempotency_key_reused`. This layer is what guarantees "never twice"; Redis makes replays fast and blocks concurrent duplicates.

One limit of the backstop: the reason a payment earned *nothing* isn't stored, so a database replay reports `cashback` only for real awards.

**On the app side** ([`pay.tsx`](app/src/app/pay.tsx)), the key lives in a `useRef` tied to the amount:

- Unknown outcome (network error, 5xx, `in_progress`): keep the key; "Try again" reuses it.
- Definite decline (`insufficient_funds`): drop the key; the next attempt is a new request.
- Amount changed: new key.

### How a cash back campaign works

**Rules** (per campaign, all in minor units):

| Field | Meaning | Example (launch campaign) |
|---|---|---|
| `rate_bps` | Cash back rate in basis points | `500` = 5% |
| `min_spend_minor` | Minimum payment to earn anything | Rp 20.000 |
| `daily_cap_minor` | Most a user can earn per campaign day | Rp 50.000 |
| `award_increment_minor` | Rounding unit | `100` = whole rupiah |
| `budget_minor` | Total the campaign can pay out | Rp 10.000.000 |
| `timezone` | Defines "a day" | `Asia/Jakarta` |
| `starts_at` / `ends_at` | Active window; `ends_at` NULL while open | |

**Lifecycle** (managed with [`server/cmd/campaign`](server/cmd/campaign/main.go); there's no admin API yet):

1. **Launch** (`campaign create`): creates a `campaign` account and moves the budget into it from the currency's `funding` account, double-entry. The exclusion constraint rejects an overlapping campaign in the same currency.
2. **Award** (inside every payment, in the payment's DB transaction):
   1. Lock the campaign live at the payment time (`SELECT … FOR UPDATE`), and compute the payment's **campaign day**: `(now AT TIME ZONE timezone)::date`, in Postgres, from the server clock.
   2. Sum the user's awards for that campaign day, and read the budget (the campaign account's balance).
   3. Run `cashback.Award` (rate → rounding → min spend → daily cap → budget).
   4. If non-zero: conditionally debit the campaign account, credit the user's `cashback` account, and insert a `cashback_awards` row linking the payment to both legs.
   5. If the budget is now zero, set `ends_at` in the same transaction.
3. **End** (`campaign end <id>`, or automatically on budget exhaustion): leftover budget goes back to `funding`.

**Why per-day is server-side:** the daily cap resets at midnight in the campaign's time zone, computed by Postgres from the server timestamp. Changing the phone's clock or travelling can't game it. The app only displays `resetAt`.

**What the user sees:** `GET /v1/cashback/campaign` returns the rate, minimum, cap, `earnedToday`, budget remaining and `resetAt`. Each payment response tells them what they earned and, if nothing, why. Earned cash back accumulates in the cash back account until they redeem it into their main balance.

**Known limit:** every awarding payment locks the single campaign row. That's fine for hundreds of awards per second; beyond that, shorten the lock or shard the budget across several accounts.

---

## App: localisation

The app formats everything with the **device's** locale and settings, read through [`expo-localization`](app/src/lib/locale.ts) (works in Expo Go). The server sends locale-neutral data (decimal strings, ISO currency codes, UTC timestamps); the app decides presentation.

### Device locale

[`app/src/lib/locale.ts`](app/src/lib/locale.ts) reads, once at startup and again each time the app returns to the foreground (Android can change language without a restart):

- `languageTag`: the preferred BCP 47 locale, e.g. `id-ID` (default `en-US`)
- `uses24hourClock`: the user's 12/24-hour setting
- `currencyCode` / `currencySymbol`: the device region's currency, e.g. `IDR` / `Rp`

### Money formatting

[`app/src/lib/format.ts`](app/src/lib/format.ts) (`formatMoney`, `splitMoney`, and the `Amount` component) formats with `Intl.NumberFormat` in the device locale:

| Amount | `en-US` | `id-ID` |
|---|---|---|
| `{"value":"35000.00","currency":"IDR"}` | `IDR 35,000` | `Rp 35.000` |
| `{"value":"11.99","currency":"USD"}` | `$11.99` | `US$ 11,99` |

How it works:

- **Exact for any size.** Only the integer part goes through a JS number for grouping; the fraction digits are copied straight from the server's string. Arithmetic (`addMoney`, `subtractMoney`, `compareMoney`) uses BigInt on the decimal strings, never floats.
- **Local symbol only for the local currency.** If the currency matches the device region's, the region's symbol is used (`Rp`). Other currencies keep the unambiguous form Intl gives for the locale (`IDR`, `US$`).
- **Whole rupiah.** IDR has two ISO decimals, but prices are written without them, so `,00` is dropped for whole IDR amounts (fractions are still shown if present).
- **Signs follow the locale.** For `+` on incoming money, the amount is formatted as negative and the locale's minus is swapped for `+`, so the sign sits where the locale puts it (before or after the symbol).
- **Hermes workarounds.** Hermes on iOS has no `formatToParts` and ignores `signDisplay` and `currencyDisplay: 'narrowSymbol'`, so everything is built from plain `format()` output. Bidi marks in RTL locales are stripped when finding the symbol, and locales that put the symbol last (`35.000,00 €`) are handled by `symbolFirst`.
- Formatters are cached per locale, currency and digits.

### Dates and times

[`app/src/lib/transactions.ts`](app/src/lib/transactions.ts) converts UTC timestamps to the device's time zone:

- `dayLabel`: "Today" / "Yesterday" via `Intl.RelativeTimeFormat` where available ("Hari ini" / "Kemarin" in Indonesian), otherwise `13 Okt` or `13 Okt 2025`.
- `timeLabel`, `dateTimeLabel`: follow the device's 12/24-hour setting (`8:42 AM` vs `08.42`).
- Transaction lists are grouped into day sections in local time.

**Money-relevant day boundaries are not decided by the app.** The cash back daily cap resets at midnight Asia/Jakarta, computed by the server; the app only shows `resetAt` converted to local time.

### Not yet localised

UI strings are English. Hermes lacks `Intl.RelativeTimeFormat`, so "Today" / "Yesterday" fall back to English there. Real translation (a string catalogue) would cover both.

### Checking it

Test formatting in more than one locale, on the iOS simulator rather than only Node or Chrome, since Hermes behaves differently:

```bash
xcrun simctl spawn <udid> defaults write -g AppleLocale id_ID
```

Then restart Expo Go.

---

## Checks

```bash
# server/
gofmt -l . && go vet ./... && go tool sqlc diff
TEST_DATABASE_URL='postgres://cashback:cashback@localhost:5432/cashback_test?sslmode=disable' \
TEST_REDIS_URL=redis://localhost:6379/15 go test ./...

# app/
npx expo lint && npx tsc --noEmit
```

## Not built yet

Real authentication, an admin API for campaigns, refunds and cash back clawback, double-entry seed balances, CI, and translated UI strings.
