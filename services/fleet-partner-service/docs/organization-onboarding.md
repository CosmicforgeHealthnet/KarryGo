# Organization Onboarding

Onboarding is resumable: every stage is persisted, and
`GET /organizations/{id}/onboarding-status` returns the full picture —
organization status and version, each stage with its status, missing fields
and blocking issues, verification state, documents, wallet provisioning
state, allowed next actions and a timestamp. It returns organization-safe
data only: no reviewer-only notes.

## Stages

`OWNER_CONTACT`, `BUSINESS_PROFILE`, `AUTHORIZED_REPRESENTATIVE`,
`OPERATING_CAPABILITIES`, `HEADQUARTERS`, `BUSINESS_DOCUMENTS`,
`VERIFICATION`, `ORGANIZATION_SETTINGS`, `OPERATING_TERRITORIES`,
`WALLET_PROVISIONING`, `ACTIVATION` — each `NOT_STARTED | IN_PROGRESS |
COMPLETE | BLOCKED | CHANGES_REQUIRED`.

Stages are tracked separately from the organization lifecycle: a stage
describes progress, the lifecycle describes standing.

## Draft creation

`POST /organizations` requires `Idempotency-Key`. It atomically creates the
organization (status `DRAFT`), the owner membership with the system owner
role bound, default settings and all stage rows. It never activates.

## Documents

`POST /organizations/{id}/documents` attaches an existing
`media-file-service` file ID. Before recording it, the service confirms the
file exists, is owned by this organization, and is upload-complete and
scan-clean. Binary content and permanent public URLs are never stored.
Document requirements are configurable by country, organization type,
capabilities and platform policy — no single country's list is hard-coded as
the global standard.

## Submission

`POST /organizations/{id}/submit` requires `Idempotency-Key` and:

1. refuses if already submitted, under review or active;
2. refuses unless every required stage is `COMPLETE`;
3. refuses unless at least one document is `READY`;
4. writes an **immutable submission snapshot** with a monotonic
   `submission_version` (unique per organization);
5. moves the organization to `BUSINESS_SUBMITTED` **only after** the
   submission is durably recorded;
6. dispatches to `verification-compliance-service` outside the transaction,
   so an external call never holds a database lock;
7. moves to `UNDER_REVIEW` only on confirmed acceptance or a consumed
   verification event.

A timeout leaves `UNKNOWN_OUTCOME` for the reconciler to resolve by
read-back. Verification is **never** marked approved locally.

## Changes required

Review findings are appended (append-only, trigger-enforced) with issue
codes, field/document references and safe remediation text. Resubmission
creates a new submission version; earlier submissions and decisions are never
overwritten.

## Wallet provisioning

States: `NOT_REQUESTED → REQUESTED → PROVISIONING → ACTIVE`, plus
`FAILED_RETRYABLE`, `FAILED_PERMANENT`, `RECONCILIATION_REQUIRED`. The call is
idempotent on a stable key; the account ID is stored only from a confirmed
response or reconciliation; no balance is ever stored or computed. A wallet
outage blocks activation safely — there is no placeholder wallet ID.

## Activation

`TryActivate` is event- and worker-driven; there is **no owner-callable
unrestricted activate endpoint**. Every gate must pass:

- organization is under review;
- all required stages complete;
- verification approved by the authoritative service;
- organization wallet confirmed `ACTIVE` with an account ID;
- at least one active default branch;
- settings valid.

Any failure increments `fleet_partner_organization_activation_blocked_total`
and leaves the organization where it was.
