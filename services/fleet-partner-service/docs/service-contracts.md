# Internal Service Contracts

Every cross-service interaction uses an authenticated internal API or a
durable event. No contract here reads another service's database, and none
invents a live endpoint path.

## Contract status (from the audit)

| Capability | Owning service | Exists today? | Adapter wired |
| --- | --- | --- | --- |
| Send OTP / invitation email | `notification-service` | **Yes** — `POST /api/v1/notifications/send` via `shared/go/notifications` | Real (`NotificationSender`) |
| Resolve provider by email **and** phone | dispatch / taxi / hauling | No | `UnconfiguredProvider` |
| Provision organization-sponsored provider | dispatch / taxi / hauling | No | `UnconfiguredProvider` |
| Create / read organization association | dispatch / taxi / hauling | No | `UnconfiguredProvider` |
| Read provider platform status | dispatch / taxi / hauling | No | `UnconfiguredProvider` |
| Revoke organization eligibility | dispatch / taxi / hauling | No | `UnconfiguredProvider` |
| Ensure organization wallet | `payment-wallet-service` | No | `UnconfiguredWallet` |
| Ensure provider sub-wallet | `payment-wallet-service` | No | `UnconfiguredWallet` |
| Read wallet account / disable on closure | `payment-wallet-service` | No | `UnconfiguredWallet` |
| Submit organization verification / read status | `verification-compliance-service` | No | `UnconfiguredVerification` |
| File metadata, ownership, readiness | `media-file-service` | Partial (`/files/:id`) | `UnconfiguredMedia` |

An unconfigured adapter returns an explicit error. It never returns a
fabricated success, and the caller never falls back to another service's
database. Enabling a capability later means implementing one adapter and
supplying configuration — no domain rewrite.

## Failure semantics

Outcomes are classified `success | validation | unauthenticated | forbidden |
not_found | conflict | rate_limited | transient | permanent |
unknown_outcome`. Only `transient` retries automatically.

**A timed-out state-changing call is never blindly repeated.** It records
`UNKNOWN_OUTCOME` and the reconciliation worker resolves it by *read-back*:

- provider provisioning → re-`Resolve` the contact pair; if the identity now
  exists, the earlier write landed;
- wallet / sub-wallet → `ReadAccount` on the stored account ID; only when no
  account was ever recorded is the (idempotent) ensure call repeated;
- verification → `ReadOrganizationStatus` for the submission.

Every downstream write carries a stable idempotency key derived from the
local aggregate ID, so a safe repeat cannot duplicate.

## Notification contract

`shared/go/notifications.Request` with `channels: ["email"]`, a template key,
an idempotency key and request/correlation IDs. Corporate recipients use a new
`corporate_user` recipient type supplied in this service's payload; the shared
client itself needed no change.

**Never in a notification payload or a domain event:** OTP codes, invitation
tokens, access or refresh tokens, identity documents, full financial account
details. Invitation emails carry a link and `sign_in_method:
email_and_phone_otp` — never a credential.

## Media contract

Before a business document is attached, `GetFile` must confirm the file
exists, is owned by **this** organization, and is upload-complete and
scan-clean. A file owned by another organization returns `404` (never a
disclosure that it exists elsewhere). If media is unreachable the attachment
is refused rather than accepted unverified.

## Wallet contract

This service stores only `wallet_account_id` plus status and source metadata.
It never stores a balance, never computes a settlement, and never credits
customer trip settlement to a provider sub-wallet. The account ID is written
only from a confirmed response or reconciliation — never optimistically.
