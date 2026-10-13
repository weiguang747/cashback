This is an Expo/React Native mobile application. Prioritize mobile-first patterns, performance, and cross-platform compatibility.

## Expo has changed — do not trust your training data

Expo ships breaking changes every SDK release. APIs you remember are likely renamed, moved, or removed. Before writing any code that touches an Expo, EAS, or React Native API:

1. Read the major version of the `expo` package in `package.json`.
2. Fetch the matching versioned docs: `https://docs.expo.dev/versions/v<major>.0.0/`
3. For anything else, fetch https://docs.expo.dev/llms.txt — an index of all Expo docs with corrections to common LLM misconceptions. Follow its links to the specific page you need; never answer from memory.

## Commands

Use `bunx` instead of `npx` if the project uses bun (`bun.lock` present).

```bash
npx expo install <package>  # ALWAYS use instead of npm/yarn/pnpm/bun add — resolves SDK-compatible versions
npx expo start              # start the dev server
npx expo lint               # lint
npx tsc --noEmit            # typecheck
npx expo-doctor             # diagnose dependency and config issues
npx expo install --fix      # fix incompatible package versions
```

Run lint and typecheck before declaring any task done.

## Navigation & Routing

- Use **Expo Router** for all navigation. Routes live in `src/app/` — every file there is a screen, `_layout.tsx` files define navigators. Keep non-route code (components, hooks, utils) outside `src/app/`.
- Import `Link`, `router`, and `useLocalSearchParams` from `expo-router`.
- Docs: https://docs.expo.dev/router/introduction.md

## Building with EAS

Use EAS to build, sign, and submit the app in the cloud (`eas build`, `eas submit`) and to ship over-the-air updates (`eas update`) — no local Xcode or Android Studio required. Run EAS CLI as `bunx eas-cli <command>` in Bun projects, or `npx eas-cli@latest <command>` otherwise; substitute that for bare `eas` in docs examples.
Docs: https://docs.expo.dev/eas/index.md

## Rules

- If `ios/` and `android/` directories do not exist, they are generated (Continuous Native Generation). Never create or edit them by hand — configure native behavior in `app.json` and config plugins.
- Expo Go only includes its bundled native modules. After adding a library with native code, the app needs a development build: `npx expo run:ios|android` locally, or `eas build --profile development`.
- Prefer recommended Expo modules over third-party libraries, and check your available skills before adding dependencies. Docs: https://docs.expo.dev/versions/latest/index.md

---

# This app: cashback wallet

A wallet with payments, campaign cash back and redemption, backed by the Go API in `../server` (read `../server/AGENTS.md` for the server's money rules). The app displays money and asks the server to move it. **The server is the only authority on balances, limits and cash back**; anything the app checks is a hint for the user.

Currently Expo SDK 57, React Native 0.86, React 19, TypeScript, Hermes, running in **Expo Go**. There's no development build or EAS setup yet: everything used must be in Expo Go (`expo-localization`, `expo-secure-store` and `expo-crypto` are).

## Running against the API

```bash
cd ../server && docker compose up -d --build     # API on :8080 (plus Postgres, Redis)
npx expo start                                   # then i / a, or open exp://127.0.0.1:8081
```

- The API URL is `EXPO_PUBLIC_API_URL`, default `http://localhost:8080` (see `.env.example`).
- **iOS simulator:** `localhost` works as is.
- **Android emulator:** run `adb reverse tcp:8080 tcp:8080 && adb reverse tcp:8081 tcp:8081` (adb is at `~/Library/Android/sdk/platform-tools/adb` here) so `localhost` reaches the Mac; re-run after the emulator restarts. Alternatively, set the URL to `http://10.0.2.2:8080`.
- **Demo users:** `andi@example.com`, `sarah@example.com`, `budi@example.com` (no password). Cash back only appears while a campaign is live; see the server's `cmd/campaign`.

## Layout

| Path | What lives there |
|---|---|
| `src/app/` | Routes only. `_layout.tsx` guards screens with `Stack.Protected` |
| `src/components/` | Generic, reusable UI (`Text`, `Button`, `Amount`, `TransactionRow`, …), exported from `index.ts` |
| `src/features/<area>/` | Components specific to one area (`wallet/BalanceCard`, `cashback/CashbackBanner`, …) |
| `src/lib/api.ts` | The only place that calls `fetch`: types mirroring the API, `ApiError`, request helpers |
| `src/lib/useApi.ts` | Generic load/reload hook; `accounts.ts`, `transactions.ts` and `cashback.ts` build data hooks on it |
| `src/lib/usePages.ts` | Cursor-paged lists: first page, `loadMore` for `onEndReached`, restart when the query changes |
| `src/lib/session.ts` | Signed-in user: in memory + SecureStore |
| `src/lib/format.ts` | Money formatting and exact money arithmetic |
| `src/lib/locale.ts` | Device locale, region currency, 12/24-hour setting |
| `src/theme/` | Colours, typography, spacing, radii. Use tokens; don't hardcode values |
| `src/data/mock.ts` | The merchant list. **Deliberately hardcoded in the app**; the server takes the merchant id and name from the payment request |

## Money in the app

- The API sends money as `Money = { value: string; currency: string }`, a decimal string such as `"-35000.00"`. **Never turn it into a JS `number` for arithmetic.** Use `addMoney`, `subtractMoney` and `compareMoney` (BigInt, exact) from `format.ts`. `Number(value)` is acceptable only for display ratios such as a progress bar.
- **Format only with `formatMoney`/`splitMoney`** (or the `Amount` component). They use the device locale via `Intl`, but work around Hermes on iOS, which has no `formatToParts` and ignores `signDisplay` and `currencyDisplay: 'narrowSymbol'`. So they:
  - build everything from plain `format()` output
  - copy the fraction digits from the string, so large values stay exact
  - swap in the local symbol (`Rp`) only when the currency is the device region's own; other currencies show the code (`IDR 35,000`) or the international symbol (`US$`)
  - drop `,00` from whole rupiah amounts (IDR has 2 ISO decimals but is written without them)
- `idr()` exists for the keypad input (whole rupiah typed by the user), which becomes `Money` before it's sent.

## Dates and locale

- The API sends timestamps as UTC ISO strings. Show them in the **device's** time zone with the helpers in `lib/transactions.ts` (`dayLabel`, `timeLabel`, `dateTimeLabel`); they use `Intl.DateTimeFormat` with the device locale and its 12/24-hour setting.
- Hermes has no `Intl.RelativeTimeFormat`, so "Today"/"Yesterday" fall back to English. The rest of the UI is English too; real translation would cover both.
- Day boundaries that matter for money (the cash back daily cap) are decided by the **server** in the campaign's time zone. The app only displays `resetAt`. Never compute limits from the device clock.

## Data loading

- Screens get data from hooks built on `useApi` (`useAccounts`, `useRecentTransactions`, `useCampaign`) or, for lists, `usePages` (`useTransactionPages`, `useCashbackHistory`).
- **Lists are paged by the server** (20 rows per page, `nextCursor`). Load more with `onEndReached` → `loadMore` and show `LoadMoreFooter`; after a failed page, wait for "Try again" instead of retrying on every scroll. **Filter and search on the server** (`kind`, `q` on `getTransactions`), never over the rows loaded so far: they're only part of the history. Debounce typed search (`useDebounced`).
- `data` stays populated when a reload fails, so screens show stale data alongside the error instead of blanking.
- State is only set in promise callbacks (the `react-hooks/set-state-in-effect` lint rule).
- Home reloads its hooks on focus (`useFocusEffect`, skipping the first), so balances update after paying or redeeming. Do the same for any screen showing balances.
- No data-fetching library yet; add one (e.g. React Query) only if caching or sharing between screens becomes necessary.

## Moving money (pay, redeem)

- Every money-moving request sends an `Idempotency-Key`: a `Crypto.randomUUID()` per attempt, kept in a `useRef` and **reused for retries of the same amount**, replaced when the amount changes.
  - **Unknown outcomes** (network error, `timeout`, 5xx, `in_progress`): keep the key and let the user retry. `payment-failed` passes it back to `pay` as the `key` param. Never tell the user no money moved: it may have. `request()` gives up after 15 s (`ApiError.code === 'timeout'`) so a stalled server can't leave "Processing…" up forever.
  - **Definite declines** (`ApiError.code === 'insufficient_funds'`): drop the key; the next try is a new request.
- Branch on `ApiError.code`, never on the message text. Show `error.message` to the user; server messages are written for that.
- The pre-checks in the app (over-balance hint, disabled buttons) are for the user's benefit only. The server enforces the real rules.
- Success screens show the **server's** numbers (amount, cash back, date, reference), passed as route params. Don't recompute them in the app.

## Session and auth

- The API has demo auth only: after sign-in the app sends `X-User-ID` on every request (`request()` in `api.ts`).
- `session.ts` stores the user in SecureStore and restores it **synchronously** at startup, so the first render already knows who's signed in. The web build keeps the session in memory only.
- Signing in or out just calls `setUser`/`clearUser`. `Stack.Protected` in `_layout.tsx` redirects automatically, so don't call `router.replace` for auth.
- Any 401 for a signed-in request clears the session, for example when the database was re-seeded and the saved user is gone.

## Testing changes

- Run `npx expo lint` and `npx tsc --noEmit`. There are no unit tests in the app yet.
- Check money and date formatting in more than one locale (e.g. `en-US` and `id-ID` with the Indonesia region). Hermes on iOS behaves differently from Node and Chrome, so check on the simulator, not just in Node.
- To change the simulator's locale: `xcrun simctl spawn <udid> defaults write -g AppleLocale id_ID` (and `AppleLanguages`), then restart Expo Go.
