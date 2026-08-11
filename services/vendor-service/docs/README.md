# Vendor Service Documentation

## Section 7 backend

Section 7 adds onboarding, representative management, Media-owned document references, six-dimension verification projections, Wallet-owned masked bank onboarding, a data-driven checklist, automatic policy activation, signed callbacks, expiry handling, and durable reconciliation. The public surface is `/api/v1/vendor/onboarding`; the root-level internal callback is `/internal/vendor/v1/verification-events`.

**Section 7 contract correction:** VerificationStatus is exactly `NOT_STARTED`, `IN_PROGRESS`, `PENDING_REVIEW`, `VERIFIED`, `REJECTED`, `EXPIRED`, `SUSPENDED`; plain `PENDING` is invalid for verification. Unrelated outbox-delivery and reconciliation-operation states retain their own `PENDING` vocabulary. The callback was moved out of the public `/api/v1/vendor` router group after the route-composition audit found the doubled Vendor prefix accidental.

Vendor actors cannot set verification status. Vendor never stores uploaded bytes or returns full bank/recipient credentials. Bank replacement requires `vendor.bank_profile.manage` plus a single-use `finance.bank_change` step-up grant. Machine-readable routes are in `docs/openapi/section7-onboarding-verification.openapi.json`. Section 8 is not included.

This folder is the mandatory documentation control area for `services/vendor-service` (KarryGo Vendor Service and Commerce Logistics backend). It exists so every future implementation section leaves the codebase and its documentation consistent, per the source-of-truth implementation specification.

## Rule: a section is not complete until documentation is updated

After every implementation section:

1. Update [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md).
2. Update [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md) if routes changed.
3. Update [PLATFORM_SERVICE_CHANGE_REGISTER.md](PLATFORM_SERVICE_CHANGE_REGISTER.md) if another service changed (or needs to).
4. Update [FRONTEND_REQUIREMENTS.md](FRONTEND_REQUIREMENTS.md) for any frontend-facing backend behavior.
5. Update [UI_UX_REQUIREMENTS.md](UI_UX_REQUIREMENTS.md) for any new/changed user interaction.
6. Update `openapi/` where applicable.
7. Update service-specific technical docs/runbooks where applicable.

If a document has no impact from a given section, record `NO CHANGE — <reason>` in that document rather than leaving it silently untouched.

## Maintained documents

| Document | Purpose |
|---|---|
| [UI_UX_REQUIREMENTS.md](UI_UX_REQUIREMENTS.md) | Plain-language UI/UX requirements generated from backend/product implementation, for the future Vendor Flutter app team. No frontend code. |
| [FRONTEND_REQUIREMENTS.md](FRONTEND_REQUIREMENTS.md) | Plain-language backend-contract handoff (routes, auth, fields, validation, events) for the future frontend team. No frontend code. |
| [PLATFORM_SERVICE_CHANGE_REGISTER.md](PLATFORM_SERVICE_CHANGE_REGISTER.md) | Everything Vendor Service implementation requires outside `services/vendor-service` — config changes, migrations, routes, events on other services. |
| [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md) | Human-readable master catalogue of every Vendor Commerce route across every affected service. `openapi/` is the machine-readable authority once real routes exist; this is the human-readable index. |
| [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) | Per-section implementation tracker: scope, status, code paths, migrations, routes, events, workers, tests, affected services, documentation updated, remaining gaps, acceptance result. |
| [architecture/](architecture/) | Technical backend architecture documents (service boundaries, domain ownership, module map, integrations, payment allocation, fulfilment model, OTP/QR dependency status, event ownership, security model, worker/recovery model, feature flags, migration ownership). Read these before implementing any section. |
| [architecture/00_REPOSITORY_AUDIT_BASELINE.md](architecture/00_REPOSITORY_AUDIT_BASELINE.md) | The factual repository audit this whole documentation set is built on — read first. |
| `openapi/` | Machine-readable API specs for implemented Sections 5, 6 and 7 only; bidirectional route-parity tests prevent undocumented or phantom routes. |

## Section 6 delivery

Vendor business, brand, capability, and branch management is implemented in `internal/features/vendors`, migrations `000006`-`000007`, and `openapi/section6-vendor-business.openapi.json`. Section 7 onboarding builds on that graph; catalogues/commerce and Section 8 CRM remain out of scope. The UI handoff is documentation-only; no frontend source was changed.

## Reading order for a new implementation section

1. `architecture/00_REPOSITORY_AUDIT_BASELINE.md` — what actually exists today, verified.
2. `architecture/01_SERVICE_BOUNDARIES.md` and `02_DOMAIN_OWNERSHIP.md` — what you may and may not own.
3. `architecture/03_VENDOR_SERVICE_MODULE_MAP.md` — where your code goes.
4. Whichever of `04`–`12` is relevant to the section (integrations, payment, fulfilment, OTP/QR, events, security, workers, flags, migrations).
5. `IMPLEMENTATION_STATUS.md` — what has already shipped, so you don't redo or contradict it.
6. `PLATFORM_SERVICE_CHANGE_REGISTER.md` — check whether your section depends on an already-tracked external change.
