Cashback wallet: a demo payment app with campaign-based cash back, for Indonesian rupiah users first. Two projects share this repo:

| Folder | What | Read first |
|---|---|---|
| `app/` | Expo / React Native mobile app | `app/AGENTS.md` |
| `server/` | Go API, PostgreSQL, Redis | `server/AGENTS.md` |

Work inside the folder you're changing and follow its `AGENTS.md`. This file covers only what spans both.

## Running everything

```bash
cd server && docker compose up -d --build          # API :8080, Postgres :5432, Redis :6379
DATABASE_URL='postgres://cashback:cashback@localhost:5432/cashback?sslmode=disable' go run ./cmd/seed
cd ../app && npx expo start                        # Metro :8081, open in Expo Go
```

Launch a cash back campaign with `server/cmd/campaign` (see `server/AGENTS.md`); without one, payments earn nothing.

## The contract between app and server

Change both sides together when you change any of these, and keep `app/src/lib/api.ts` matching the server's JSON:

- **Base path `/v1`.** It may change freely while the app is unreleased; after release, breaking changes need `/v2`.
- **Money is `{"value": "<decimal string>", "currency": "<ISO 4217>"}`**, never a JSON number. The server stores integers of the minor unit; the app does exact arithmetic on the string (BigInt) and formats with the device locale.
- **Times are RFC 3339 in UTC.** The server decides anything per-day (in the campaign's time zone); the app only displays.
- **Errors are `{"error": "...", "code": "..."}`.** The app branches on `code` and shows `error` to the user.
- **Auth is demo-only:** `X-User-ID` header after sign-in. Real auth will replace `requireUser` on the server and `request()` in the app.
- **Money-moving requests** (`POST /v1/payments`, `POST /v1/cashback/redeem`) require an `Idempotency-Key`. The app reuses it for retries; the server guarantees at most one effect per key (Redis, plus a unique key in Postgres).
- **The server is the authority** on balances, limits and cash back. App-side checks are user hints only.

## Repo conventions

- Commit messages: a short imperative subject, then a body explaining what changed and why, grouped by `Server:` / `App:` when a change spans both.
- Never commit `.env` files (only `.env.example`).
- Before calling a change done, run each touched project's checks:
  - `server/`: `gofmt`, `go vet`, `sqlc diff`, and tests with `TEST_DATABASE_URL` / `TEST_REDIS_URL` set
  - `app/`: `npx expo lint`, `npx tsc --noEmit`
