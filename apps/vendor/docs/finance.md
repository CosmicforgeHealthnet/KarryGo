# Vendor Finance

Finance is business-wide and uses the same business wallet as Home. Changing the selected branch does not change the wallet. Mock remains the default; Live uses public Vendor APIs with no fixture fallback.

## Screens and interactions

- Finance: available balance, money awaiting settlement, withdrawal action, payout accounts and recent wallet activity. Pull to refresh updates these reads independently.
- Wallet activity: latest 100 records; tap a record for a receipt sheet with its actual reference, type, direction, amount, currency, memo and recorded timestamp. Copy reference is local clipboard functionality.
- Payout accounts: sheet showing the returned masked account, resolved name, bank code and account status. Manage bank details opens existing Business setup and verification; Finance does not create or delete accounts.
- Withdrawal request: full screen for amount and active-account selection, review sheet, then session-bound code verification. The primary action stays visible in a safe-area footer. Code resend has its own loading state. Money uses exact integer kobo, not floating-point rounding. Mock prefills the amount and code; Live code entry is empty.
- Withdrawal history: full screen with local All, In progress, Paid and Needs attention filters over the returned records. These are view filters, not invented backend query parameters.
- Withdrawal detail: full screen fetched by reference, with actual amount, currency, account reference and status. An owned bank account is labelled from the account read when available. No invented withdrawal date, fee, delivery estimate or status timeline.
- Success and refusal use explicit feedback dialogs. An accepted request is described as requested, not as paid. Navigation and verification transitions respect reduced motion.

## Public endpoint coverage

| Endpoint | Frontend use |
| --- | --- |
| `GET /api/v1/vendor/finance/overview` | Available and pending-settlement balances |
| `GET /api/v1/vendor/finance/transactions?limit=100` | Activity and receipt sheets |
| `GET /api/v1/vendor/finance/bank-accounts` | Payout accounts and withdrawal destination |
| `GET /api/v1/vendor/finance/withdrawals` | Latest withdrawal history |
| `GET /api/v1/vendor/finance/withdrawals/{reference}` | Authoritative transfer details and refresh |
| `POST /api/v1/vendor/finance/withdrawals` | Verified withdrawal request and original-command replay |

Read permission is `vendor.finance.read`; requests require `vendor.finance.withdraw`. A refused read does not blank other permitted resources. Refused resource data is cleared rather than shown after access is withdrawn. Permission-sensitive writes stay subject to the server's response.

Transactions accept only `limit`, default 50 and maximum 100. Withdrawal history has no public pagination parameters and currently returns the upstream latest 50. There is no transaction-detail endpoint: a receipt displays the canonical list record. No fabricated Load more or total counts.

## Withdrawal sequence

1. Refresh wallet and payout-account reads. Require available funds and a returned bank account whose status is exactly `active`.
2. Enter amount and select its `external_ref` as `bank_account_id`. Confirm amount, currency and masked destination in the review sheet.
3. Start auth step-up with purpose exactly `finance.withdrawal`. Starting step-up refreshes the signed-in session before creating the challenge.
4. Verify `challenge_id`, purpose and code. The server issues a short-lived session/staff/vendor-bound grant. The code stays in memory and is cleared; it is never journalled.
5. Before sending, save the command identity and exact body to the attempt journal. Live uses secure storage scoped by API origin and vendor; Mock uses isolated memory storage. Failed journal writes prevent the HTTP request.
6. POST with `Idempotency-Key` and only `bank_account_id`, integer `amount_kobo`, wallet `currency` and `step_up_grant_id`. Financial POSTs never automatically refresh/replay after 401, because refreshing would rotate the grant's session.
7. Use the returned record and reference, then refresh reads. An accepted write remains accepted even if a subsequent read or clearing the saved record fails. Storage cleanup can be retried for a known accepted record without sending another financial command.

Server-configured withdrawal minimum/maximum, ownership, currency, account activation, transfer-provider coverage, funds reservation and grant consumption remain backend checks. Their limits and transfer fees are not exposed by these APIs, so the UI does not invent them.

`pending_kobo` represents held commerce allocations awaiting settlement. It is not the total of requested withdrawals. Mock requests reduce available funds, preserve pending settlement and add one `withdrawal_reserved` debit and withdrawal record. Mock status remains processing unless the verification-required scenario is selected; reading does not silently mark a transfer paid.

## Status and recovery

The backend's statuses are `pending`, `processing`, `requires_otp`, `paid`, `failed` and `reversed`. Paid/active use teal, processing blue, pending/verification amber, failed/reversed red. Unknown server statuses keep their label and neutral styling.

Timeout, network loss, 5xx or an idempotency conflict can leave an unknown outcome. The journal retains the original key and exact body. New withdrawals are disabled. Retry original request asks for confirmation and replays that command; it does not request a new grant, key or payout. The app does not infer acceptance from another withdrawal with a similar amount or account. A later rejection of an originally uncertain command does not prove the original payout was absent, so it remains unresolved.

There are two backend gaps:

- `requires_otp` is a transfer-provider requirement. No public Vendor endpoint completes that transfer OTP. Auth verification is separate and must not be presented as transfer-OTP completion. The detail screen explains the limitation and permits a status refresh.
- No public endpoint resolves a withdrawal by client idempotency key when its reference was lost. If the original command cannot replay because its grant was consumed and no accepted response was cached, these APIs cannot safely prove its outcome. New requests remain paused. A backend reconciliation endpoint is needed to clear that case safely.

There are no public withdrawal cancel, delete or payment retry endpoints. No such actions are shown. Backend enablement and wallet/provider deployment configuration still determine Live availability.

## Verification

`test/finance_test.dart` checks exact kobo conversion, session/purpose-bound verification, single reservation, original-command replay after response loss and controller restoration, changed-body rejection, no automatic financial refresh, storage failures, independent permissions, transfer verification status and narrow-phone layouts.

No real payout, real OTP delivery or deployed bank transfer was executed during these tests.
