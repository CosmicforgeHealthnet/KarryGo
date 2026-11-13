# Fleet spec v2: brief for other developers

Purpose: one place for the work the backend build does NOT do itself, mainly
the mobile/provider app frontends, plus a running log of the backend work as
it lands. The Fleet web app (`web/fleet`) is built alongside the backend and is
not listed here.

Status of the whole specification: `fleet-spec-v2-status.md`. Evidence:
`spec-v2-audit/`.

## 1. Provider apps: work for the app developers

These apply to `apps/dispatch_provider`, `apps/truck_provider` and
`apps/taxi_provider` unless stated. The provider services already expose (or
will expose, as noted in the log below) the routes. Providers never call the
Fleet service directly; they use their own provider service.

### 1.1 Company work: Accept / Decline (spec 2.0, 2.6, 11, 40, 58.12)

Every organization offer and every company assignment must show **both Accept
and Decline**. The company cannot accept on the provider's behalf.

- **List and detail** of company assignments and organization offers, with:
  - the company name;
  - pickup and drop-off;
  - the response deadline as a countdown;
  - any safety or vehicle notes.
- **Decline needs a reason** chosen from the list the API returns. The
  `decline_reason_required` error must be handled. An `unsafe_vehicle` decline
  should explain that the vehicle will be reviewed for safety, not that the
  provider is being punished.
- **Error handling** for these responses, with a refresh of the item:
  - `assignment_response_expired`
  - `assignment_already_resolved`
  - `assignment_not_owned_by_provider`
  - `provider_membership_not_active`
  - `409` for a reassigned or expired assignment
- **Never show** customer fare, commission, settlement amounts or contract
  prices (spec 2.9). Add a test that provider responses rendered in the app
  carry no fare or commission fields.
- **Current state per app:**
  - `dispatch_provider` already models `organization-offers` and
    `company-assignments` (`lib/features/requests/data/request_model.dart`).
    Add Accept/Decline with reasons, the countdown and the error handling.
  - `truck_provider` has no screens. Hauling exposes `/provider/company-assignments`
    and `/provider/private-assignments/:id/{accept,decline}`.
  - `taxi_provider` has no screens. Taxi exposes
    `/provider/organization-offers[/:id/accept|decline]` and
    `/provider/company-assignments[/:id/accept|decline]` (flag
    `TAXI_FLEET_PROVIDER_WORK_ENABLED`).

### 1.2 Company membership (spec 10, 39)

- **Accept a company relationship:** a screen shown when a company adds the
  provider (association invitation). Accepting is what activates the
  membership. The backend route is added in the log below.
- **Sponsored providers** (created by a company) must skip the independent
  self-onboarding and verification screens (spec 2.5). They still complete
  first-login security.
- **Membership indicator:** active or suspended, with the company name. On
  `provider_membership_not_active`, show the reason and stop company work.
- **History:** their own company assignments, accepted/declined responses and
  membership changes, read through the provider service.

### 1.3 Provider sub-wallet (spec 13, 42)

- **Sub-wallet balance, allocations and statements.** Managed providers are
  paid by company allocations, not trip settlement (spec 2.8).
- **Withdrawal requests** where company policy allows them, using the existing
  wallet PIN flow.

### 1.4 Notifications

Handle push and in-app notices for:
- new company assignment;
- assignment expiring;
- assignment reassigned or cancelled;
- membership suspended.

Until the notification intents exist (log below), poll the assignment list.

### 1.5 Safety and privacy

- **Safety report:** let a provider file one against a trip or vehicle
  (spec 49).
- **Privacy:** export and delete request entry points, once the provider
  services support them (spec 56).

### 1.6 General

- **429:** wait for `Retry-After`.
- **Unknown-outcome and 202 operations:** poll until resolved.
- **Request id:** show `requestId` on error screens so support can trace it.

## 2. Customer app (`apps/customer`)

No change is required by the spec: customer booking stays exactly as it is
(spec 2.2). If privacy requests are added for customers, an entry point will
be listed here.

## 3. Admin console (`apps/admin`): work for the admin developers

The admin web app is a scaffold today. The backend for these screens exists.

### 3.1 Company review (ADR-006, spec 70 journey 1 step 12)

Operators decide companies here; nothing else approves a company.

- **Review queue:** company submissions read through the admin bridge, resource
  kind `organization-submissions` (owner `verification-compliance-service`).
  - Show the company name and country from the submission snapshot, the
    status, when it was submitted, and the submitted documents (file ids in
    the snapshot; preview through media).
- **Actions,** sent as owner commands in the `verification` domain, with
  `expectedVersion` = the submission's `source_version`:

  | Action | Parameters | Effect |
  |---|---|---|
  | `START_ORGANIZATION_REVIEW` | none | Marks the submission in review. |
  | `REQUEST_ORGANIZATION_CHANGES` | `issue` (reason code), `remediation` (`RESUBMIT_DOCUMENT` / `PROVIDE_ADDITIONAL_DOCUMENT` / `CORRECT_DETAILS`), optional `document` (one of the four company document types) | The company sees the request on its company page and can resubmit. |
  | `APPROVE_ORGANIZATION` | none; needs a second approver | Fleet activates the company once its other checks pass. |
  | `REJECT_ORGANIZATION` | `reason`; needs a second approver | |
  | `SUSPEND_ORGANIZATION` | `reason`; needs a second approver | |

- **Results:** the bridge returns `ErrConflict` (409) when the submission
  changed since it was loaded. Reload and retry.
- **Permissions:**
  - Reviewing needs Verification's review grant.
  - Deciding needs the decide grant (`verification.organization_submissions.read`
    to list).

## 4. Platform / infrastructure work (needs server access or a product owner)

- Point-in-time recovery and encrypted database backups (RPO target 5 minutes;
  today nightly dumps).
- Prometheus scraping of Fleet `/metrics`, alert rules and dashboards for the
  Section 65 families.
- A KMS for envelope encryption keys (spec 64).
- Production settings: `FLEET_PARTNER_SERVICE_CREDENTIAL_SCOPING_ENABLED=true`,
  `FLEET_PARTNER_REDIS_REQUIRED=true`, metrics token and audit chain secret
  set.
- A quarterly disaster-recovery exercise.
- **Staging secrets** so Fleet can reach the services it depends on:
  - Fleet ↔ Wallet: `FLEET_PARTNER_PAYMENT_WALLET_SERVICE_SECRET`, plus the
    matching trust entry in Wallet.
  - Fleet ↔ Dispatch: `FLEET_PARTNER_DISPATCH_SERVICE_SECRET`.
  - Fleet ↔ Verification: `FLEET_PARTNER_VERIFICATION_COMPLIANCE_SERVICE_*`
    (base URL and secret).

## 5. Decisions needed from the product owner

All decided on 2026-11-13; see `fleet-spec-v2-status.md` Section 4.

App impact of the decisions:
- **Error codes (ADR-004):** Fleet moves to the Section 63 codes. Provider
  services relay them unchanged, so provider apps should key behaviour on the
  Section 63 codes (`assignment_response_expired`,
  `assignment_already_resolved`, `decline_reason_required`, ...).
- **API path (ADR-003):** unchanged; no app impact.

## 6. Work log (backend and cross-service)

Newest first. Each entry lists anything that creates new app-frontend work.

| Date | Area | What changed | App frontend impact |
|---|---|---|---|
| 2026-11-13 | Company activation (ADR-006) | Fleet applies Verification decisions (push + one-minute read-back), maps VERIFIED to APPROVED, activates approved companies; activation policy v2; Verification accepts Fleet's text ids, has company review admin commands and delivers decisions to Fleet; staging pairing workflow connects them | Admin console company review screens (3.1). Fleet web company page shows review progress (done). |
| 2026-11-13 | Audit | Full audit of Sections 1-72 and Appendices A-D (`spec-v2-audit/`) | None |
| 2026-11-13 | Fleet | Staff management, HQ branch, workforce/payroll fixes, trips pages, staging modules switched on (`fleet-end-to-end-activation-report.md`) | None |
