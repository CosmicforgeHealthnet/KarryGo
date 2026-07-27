# Audit — Marketplace Acceptance, Assignments and Trip Integration (Sections 11–12)

Performed before any Sections 11–12 code was written.

Date: 2026-07-25
Branch: `staging` · Commit: `7320aaaeb3bb7427b1ffb15c003bc6389d549c1f`
Starting `git status --short`: `M go.work`, `?? services/fleet-partner-service/`
(Sections 3–10, uncommitted). `git diff --stat`: `go.work | 1 +`.

---

## 1. Headline finding — the three "authoritative execution services" are not equally real

Sections 11–12 require **real, authenticated reservation and execution
contracts implemented inside all three execution services**, and explicitly
forbid claiming completion while an authoritative service "still lacks the
required real contract".

Measured directly:

| Service | Go files | Migrations | HTTP handlers | State |
| --- | --- | --- | --- | --- |
| `driver-dispatch-delivery-service` | 138 | 27 | 11 | **Production-grade** |
| `driver-hauling-service` | 45 | 5 (unpaired `00N_*.sql`) | 16 | **Partial** |
| `driver-taxi-service` | **2** | **0** | **0** | **Scaffold only** |

`driver-taxi-service` contains `cmd/main.go` and `internal/config/config.go`.
Every feature directory (`auth`, `matching`, `providers`, `rides`, `trips`,
`vehicles`) holds only `README.md` and `.gitkeep` files. There is no database
package, no migration, no provider model, no ride model, no trip model and no
authentication code.

`driver-hauling-service` has `booking` (15 files), `provider_auth` (9),
`provider_profile` (7), `provider_availability` (4), `identity` (2),
`notifications` (2). Its `matching`, `haulage_requests`, `cargo_workflow`,
`providers` and `trucks` features are **empty** — there is no marketplace
offer/reservation layer to reserve against.

### 1.1 Correction to the Sections 8–10 audit

That audit stated all three provider services were already passwordless with
equivalent `/api/v1/auth/*` routes. That was **verified for dispatch and
hauling only**; for taxi it was inferred from directory names. It is wrong:
**`driver-taxi-service` has no authentication implementation at all.** The
Sections 8–10 conclusion "no provider-service change is required" still holds
for dispatch and hauling, but taxi has nothing to be compatible with.

---

## 2. Confirmed repository facts

### 2.1 fleet-partner-service (Sections 3–10)

Migrations `000001`–`000008` applied. Existing infrastructure this work
package builds on, all already tested: transactional outbox with lease-based
publisher, consumer inbox with payload-hash dedupe, durable jobs with
leases/heartbeats/tenant revalidation, idempotency store, append-only audit,
optimistic concurrency helpers, tenant context and tenant-scoped repositories,
deny-by-default authorizer with role-version cache keying, opaque IDs, Money,
cursor pagination, filter allowlisting, error envelopes.

Managed provider memberships exist (`fleet_partner_organization_provider_memberships`)
with the state machine `INVITED → PENDING_PROVIDER_ACCEPTANCE → PENDING_SETUP
→ ACTIVE`, plus policies carrying the DB-enforced invariant
`provider_decline_required = true`. Organization settings exist with
`assignment_timeout_seconds` (30–3600) — the natural home for assignment
response deadlines. Operating mode (`CENTRALIZED`/`DELEGATED`/`HYBRID`) does
**not** exist yet; `provider_visibility` is the only related setting.

Permission catalogue is seeded in `000005`; `marketplace.*`, `dispatch.assign`,
`assignments.*`, `trips.read` and `reconciliation.*` do **not** exist yet.
`orders.read/manage` and `assignments.manage` were seeded inactive.

Corporate token claims: `{sub, role=organization_user, service=fleet-partner-service,
session_id, typ, iat, exp}` (shared HS256). Sessions carry `role_version`.

### 2.2 driver-dispatch-delivery-service — the one service that can support this

- **Marketplace exists and already has the exact atomic reservation primitive
  required.** `request_broadcasts (booking_id UNIQUE, status IN
  ('broadcasting','accepted','expired','cancelled','no_provider_found'),
  accepted_by_provider_id, expires_at, booking_payload)`, and
  `MarkBroadcastAccepted` performs single-winner selection in one transaction:
  `UPDATE ... WHERE id=$1 AND booking_id=$2 AND status='broadcasting' AND
  expires_at > $4`, returning `409 Conflict` when `RowsAffected != 1`.
  `provider_request_inbox` is the per-provider offer fan-out.
- `trips (booking_id UNIQUE, provider_id, customer_id, status, fare_amount,
  ...)` with a `chk_trip_status` state machine (`assigned`, `en_route_pickup`,
  `arrived_pickup`, `in_progress`, …) and `trip_state_log`.
- `providers`, `provider_availability`, `availability_sessions`,
  `dispatch_rider_sessions`, verification tables, `delivery_proofs`.
- **No organization awareness anywhere**: no `organization_id`,
  `settlement_organization_id` or `organization_membership_id` column exists in
  any dispatch migration.
- **No transactional outbox and no event publication.** Grep for outbox across
  the service returns nothing.

### 2.3 driver-hauling-service

`haulage_bookings`, `booking_events`, `truck_providers`, `trucks`,
`provider_sessions`. Provider auth implemented and passwordless (uses shared
`HashOTP`). No marketplace broadcast/offer table, no matching feature, no
organization columns, no outbox. Migrations are single unpaired files
(`001_hauling_core.sql` …) with no down files and no tracking table.

### 2.4 Cross-cutting

- **No event broker exists** anywhere in the repository (unchanged since the
  Sections 3–7 audit). fleet-partner's outbox still runs on `DisabledPublisher`.
- **No execution service has an outbox**, so durable event delivery would have
  to be built into each one.
- Internal service auth: shared `serviceauth` HMAC (`X-Service-Name`,
  `-Timestamp`, `-Signature`) is available and already used.
- Media, verification and wallet contracts remain unconfigured adapters from
  Section 10 (documented deferrals).

---

## 3. What Sections 11–12 actually require, against that baseline

| Requirement | Dispatch | Hauling | Taxi |
| --- | --- | --- | --- |
| Reserve request for organization | Feasible — extend `request_broadcasts` accept with org columns + internal endpoint | Needs a marketplace layer built first | **Needs the whole service built first** |
| Confirm managed provider / create canonical execution | Feasible — `trips` exists | Partially — `haulage_bookings` exists, no assignment concept | **Nothing exists** |
| Read-back by idempotency key | Needs new command-operation storage | Needs same | **Nothing exists** |
| Durable event delivery to fleet | Needs an outbox built | Needs an outbox built | **Nothing exists** |
| Provider auth for Accept/Decline | Exists | Exists | **Nothing exists** |

Building the taxi side means implementing provider identity, provider
authentication, ride requests, matching, trips, availability, vehicles,
migrations and a database from zero — several work packages in its own right,
and outside "Sections 11–12: Request Acceptance, Assignment, Provider
Acceptance and Decline".

Building the hauling side means implementing a marketplace offer/reservation
layer that does not exist today.

---

## 4. Security and correctness notes carried into design

- **Provider authentication boundary (spec §9).** fleet-partner cannot
  securely validate dispatch/hauling provider sessions today: those services
  sign provider tokens with their own secrets and fleet-partner has no
  issuer/audience/revocation view. **Option B (owning-service proxy) is the
  correct choice** — provider-facing Accept/Decline must be authenticated by
  the owning service, which then calls fleet-partner internally with a signed
  provider assertion. Option A would require sharing provider signing secrets
  with fleet-partner, which weakens the boundary.
- **Universal Accept/Decline** is a server-authoritative invariant, not a UI
  concern. `allowedActions` must be computed from assignment state, and the
  existing `provider_decline_required = true` CHECK already prevents an
  organization policy from removing decline.
- **Single winner** must remain owned by the source service; dispatch already
  enforces it atomically, so fleet-partner must never record a local
  acceptance before source confirmation.
- **Provider data minimization**: dispatch `trips.fare_amount` and
  `request_broadcasts.booking_payload` carry customer pricing. Provider-facing
  DTOs must be built from explicit allowlists, never by stripping fields from
  a dispatcher/internal struct.

---

## 5. Missing contracts (blocking)

1. Taxi service does not exist (identity, auth, requests, trips, DB).
2. Hauling has no marketplace/offer/reservation layer.
3. No execution service has an outbox or event delivery.
4. No execution service has organization/settlement metadata on requests or
   trips.
5. No read-back-by-idempotency-key storage in any execution service.

## 6. Decisions taken

1. Provider Accept/Decline uses **Option B (owning-service proxy)** with signed
   provider assertions — never a corporate session, never a client-supplied
   provider ID.
2. Local acceptance is never recorded before confirmed source reservation;
   unknown outcomes go to reconciliation with read-back, never a blind retry.
3. Provider DTOs are built from explicit allowlists.
4. fleet-partner migrations continue additively from `000009`.
5. Operating mode is added to organization settings (additive), and can never
   disable decline.

## 7. Open scope question (raised with the requester)

Sections 11–12 cannot be completed to their stated completion gates
("real reservation contracts in all three execution services", "multi-service
end-to-end tests pass for dispatch, taxi and hauling", "remaining blockers
must be zero") because one of the three services does not exist and a second
lacks a marketplace layer. The scope decision is recorded in the final report.
