# Support & Dispute Service

Owns complaints, evidence collection, disputes, support chat, FAQs, and emergency
SOS for the **customer app** and the three **provider apps** (taxi / dispatch /
hauling), plus an **admin / back-office** surface for triage and resolution.

- Port: `8107`
- API base: `/api/v1/support-disputes`
- Datastore: Postgres only (no Redis)
- Bootstrap: `bash scripts/bootstrap_services/support-dispute-local-bootstrap.sh`
- Postman: `docs/support-disputes.postman_collection.json` (+ environment)

---

## Running locally

```bash
# 1. Provision Postgres (port 5439), create the DB, apply migrations 001–005, seed FAQs:
bash scripts/bootstrap_services/support-dispute-local-bootstrap.sh

# 2. Start the service:
cd services/support-dispute-service && go run ./cmd
#    (or let the service run migrations itself: MIGRATION=true go run ./cmd)
```

Optional integrations are **off by default** and become active only when their env
vars are set (see `.env.example`):

| Feature | Env (bare origin) | Effect when unset |
|---|---|---|
| Notifications + live chat | `SUPPORT_DISPUTE_NOTIFICATION_URL` / `_SECRET` | status/chat/SOS pushes + realtime routes are no-ops; apps poll |
| Dispute refunds | `SUPPORT_DISPUTE_PAYMENT_URL` / `_SECRET` | resolve never issues a refund |
| Identity enrichment | `SUPPORT_DISPUTE_CUSTOMER_SERVICE_URL`, `SUPPORT_DISPUTE_HAULING_SERVICE_URL` (+ secrets) | complainant name/phone stay empty (the action still succeeds) |

---

## Auth model

| Caller | Mechanism | Claim check |
|---|---|---|
| Public | none | — |
| Customer | bearer | `role=customer`, `service=customer` |
| Hauling provider (`/provider`) | bearer | `role=truck_provider`, `service=hauling` |
| Taxi provider (`/taxi-provider`) | bearer | `role=taxi_provider`, `service=taxi` |
| Dispatch provider (`/dispatch-provider`) | bearer | `role=dispatch_provider`, `service=dispatch` |
| Admin / internal (`/admin`) | HMAC (`shared/go/serviceauth`) | `service_name` ∈ `SUPPORT_DISPUTE_SERVICE_SECRETS` |

Pass the bearer as `Authorization: Bearer <token>`. The complainant identity
(`complainant_type` + `complainant_id`) is **always derived from the token**, never
the request body. A caller can only read/modify **their own** complaints; admins
bypass ownership. Taxi/dispatch groups are wired and ready, but their backend
services do not issue tokens yet, so those routes are not yet exercisable.

All responses use the platform envelope:

```json
{ "success": true, "data": { } }
{ "success": false, "error": { "code": "validation_failed", "message": "…", "request_id": "…", "fields": [] } }
```

---

## Endpoints

### Public — no auth

| Method & path | What it does |
|---|---|
| `GET /support/categories` | List complaint category codes for a dropdown: `incorrect_delivery`, `delayed_arrival`, `payment`, `damaged_goods`, `provider_misconduct`, `fraud`, `other`. |
| `GET /support/faqs?audience=` | Published help articles for self-service. `audience` = `all` \| `customer` \| `provider` (omit ⇒ `all`). |

### Customer (bearer, at root)

| Method & path | What it does |
|---|---|
| `POST /complaints` | File a complaint. Body: `service_type`* (`taxi\|dispatch\|hauling\|wallet\|platform`), `subject`*, `description`*, optional `category` (validated when present), `booking_reference`. |
| `GET /complaints?limit=&offset=` | List my complaints, newest first; each carries `unread_count`. |
| `GET /complaints/:id` | View my complaint (+ live unread count). `403` if not mine. |
| `POST /complaints/:id/evidence` | Attach evidence. Body: `media_url` and/or `media_asset_id`, optional `note`. Blocked on resolved/closed complaints. |
| `GET /complaints/:id/evidence` | List evidence on my complaint. |
| `POST /support-chat/start` | Open/resume a platform support chat; returns the backing complaint. |
| `POST /complaints/:id/messages` | Send a chat message. Body: `content`*. |
| `GET /complaints/:id/messages?limit=&offset=` | Read the chat thread (oldest first). |
| `POST /complaints/:id/messages/read` | Mark the other party's messages read (clears the badge). |
| `POST /sos` | Raise an emergency. Body (optional): `description`, `lat`, `lng`. Creates a `priority=emergency` complaint + admin-queue surfacing. |
| `POST /complaints/:id/dispute` | Escalate to a dispute. Body: `respondent_type`* (`customer\|taxi_provider\|dispatch_provider\|hauling_provider`), `respondent_id`*. |
| `GET /complaints/:id/dispute` | View the dispute on my complaint. |

\* required.

### Providers (bearer)

Same set as Customer **minus** the two `…/dispute` routes (escalation is
customer-initiated), under a per-app prefix:

```
/provider/...          (hauling / truck)
/taxi-provider/...     (taxi)
/dispatch-provider/... (dispatch)
```

e.g. `POST /provider/complaints`, `GET /provider/complaints`, `POST /provider/sos`,
`POST /provider/complaints/:id/messages/read`, …

### Admin / internal (HMAC, `/admin`)

| Method & path | What it does |
|---|---|
| `GET /admin/complaints?status=&priority=&service_type=&complainant_type=` | Triage queue (emergencies sort first). All filters optional. |
| `GET /admin/complaints/:id` | Review a complaint (with identity snapshot). |
| `GET /admin/complaints/:id/evidence` | Review its evidence. |
| `GET /admin/complaints/:id/dispute` | View the linked dispute. |
| `GET /admin/complaints/:id/events` | Audit trail (`complaint_created`, `status_changed`, `evidence_added`, `escalated`, `dispute_resolved`, `sos_raised`). |
| `POST /admin/complaints/:id/refresh-identity` | Re-pull the complainant name/phone from the owning service. |
| `PUT /admin/complaints/:id/status` | Change status + notify. Body: `status`*, optional `resolution_note`. Header `X-Admin-ID` records the actor. |
| `GET /admin/disputes?outcome=&service_type=` | List/filter disputes. |
| `POST /admin/disputes/:id/resolve` | Resolve. Body: `outcome`* (`favour_complainant\|favour_respondent\|split\|dismissed`), optional `note`, and optional refund (`refund_amount_kobo` + `refund_source_reference`, honoured only when the outcome favours the complainant/split **and** payment-wallet is configured). |
| `POST /admin/complaints/:id/messages` | Reply as admin (notifies the complainant). |
| `GET /admin/complaints/:id/messages` | Read the thread. |

### Realtime proxy (bearer — only when notification-service is configured)

Brokers a live websocket channel for support chat so apps never hold the HMAC
secret. Customer group + a per-provider group:

| Method & path | What it does |
|---|---|
| `GET /customer/notifications?limit=` | Recent notification feed. |
| `POST /customer/notifications/realtime-token` | Mint a short-lived websocket token. |
| `POST /customer/notifications/devices` | Register a push device token. Body: `token`*, `platform`, `app`. |

Providers use `/provider/notifications`, `/taxi-provider/notifications`,
`/dispatch-provider/notifications`.

---

## Internal lookup endpoints (added on owning services)

For identity enrichment this service calls, with HMAC service-auth signed as
`support-dispute-service`:

- `GET /api/v1/customer/internal/customers/:id` → `{ id, name, phone, email, status }`
- `GET /api/v1/hauling/internal/providers/:id` → `{ id, name, phone, status }`

Each owning service must trust `support-dispute-service=<shared-secret>` in its
`*_SERVICE_SECRETS`.

---

## Data model

| Table | Purpose |
|---|---|
| `complaints` | Primary ticket. Carries `category`, `priority`, optional `incident_lat/lng`, and a denormalised `complainant_name/phone` identity snapshot. |
| `complaint_evidence` | Files/notes attached to a complaint. |
| `disputes` | Formal escalation of a complaint against a respondent (+ respondent snapshot, outcome, adjudicator). |
| `complaint_events` | Append-only audit trail. |
| `support_chat_messages` | Chat thread; `is_read` drives unread counts. |
| `help_articles` | Published FAQ/help-center content. |

Migrations live in `migrations/` (`001`–`005`) and are applied in sorted order by
the bootstrap script or by `MIGRATION=true`. FAQ seed: `seeds/help_articles.sql`.

---

## Tests

```bash
cd services/support-dispute-service
go build ./... && go vet ./... && go test ./...
```

Usecase tests cover ownership denial (IDOR), enum validation, the `wallet`
service type, SOS emergency priority, and unread counts.

## V4 Section 17 evidence closure (local)

Support remains the case and resolution authority. Ride, Send, Truck and
order-scoped Food/Shop paths derive owner/source context server-side and reject
cross-customer or unrelated child references. Evidence now accepts MediaRef,
not arbitrary URLs: Support obtains a signed Media verdict for the exact owner,
purpose and case/order binding before attachment. If Media is unavailable or
the verdict is unusable, evidence attachment fails closed. Deterministic
Commerce refunds continue through Wallet/domain policy independently of a
case. Targeted HTTP, PostgreSQL, IDOR, Media-verdict and commerce-case race
tests pass locally; staging service HMAC configuration remains to certify.

## Secure Admin (Section 16)

Support is an enrolled Secure Admin source owner (`shared/go/adminowner.SupportProfile`,
`shared/go/admincommand.SupportSpec`). Migration `009_secure_admin_boundary.sql`
(reversal in `migrations/down/`) adds the owner tables, native versions and the
event outbox. **Everything is default-off and nothing is seeded** — no staff
grant, policy, secret or approval.

**Switches (prefix `SUPPORT_DISPUTE`)**

| Variable | Effect |
|---|---|
| `_ADMIN_RESOURCE_READ_ENABLED` + `_ADMIN_RESOURCE_SECRET_FILE`, `_ADMIN_ACCESS_POLICY_FILE`, `_ADMIN_ACCESS_POLICY_KEYS_FILE`, `_ADMIN_CONTEXT_KEYS_FILE` | Minimized resource reads |
| `_ADMIN_CONTEXT_PUBLISH_ENABLED` + `_ADMIN_CONTEXT_PUBLISH_SECRET_FILE` | Signed owner-context import |
| `_ADMIN_COMMAND_WRITES_ENABLED` / `_ADMIN_COMMAND_READBACK_ENABLED` + `_ADMIN_COMMAND_WRITE_SECRET_FILE` / `_ADMIN_COMMAND_READBACK_SECRET_FILE` | Command execution / receipt readback (writes require readback) |
| `_ADMIN_EVENTS_ENABLED` + `_ADMIN_EVENTS_BRIDGE_ORIGIN`, `_ADMIN_EVENTS_SECRET_FILE` | Outbox relay to the Bridge |
| `_ADMIN_EVENTS_SNAPSHOT_ENABLED` + `_ADMIN_EVENTS_SNAPSHOT_SECRET_FILE` | Backfill snapshot route |

Secret files hold base64url (raw) keys of at least 32 bytes; each key must
differ from every other Admin key.

**Routes** (root, never under the public API base, never via the gateway)

- `GET /internal/support/v1/admin/resources/{kind}[/{id}]` — kinds `cases`, `sos-cases`
  (priority emergency), `disputes`, `case-messages`, `case-evidence`.
- `POST /internal/support/v1/admin/authorization-contexts`, `GET .../authorization-contexts/bindings/{kind}/{id}`.
- `POST /internal/support/v1/admin/commands`, `GET .../commands/{kind}/{id}[/{key}]`.
- `GET /internal/support/v1/admin-events/snapshot?cursor=&limit=` (limit ≤ 200) —
  HMAC v2, caller exactly `admin-backoffice-service`, durable nonce replay guard.

Projections never select chat bodies, descriptions, notes, names, phones,
incident coordinates or media URLs. Labels (case subject, message preview) are
credential/contact-redacted and bounded to 240 runes; evidence exposes only the
media object reference. Every read and command also needs a native grant in
`support_admin_staff_grants` (`support.cases.read` … per kind;
`support.case.manage`, `support.sos.manage`, `support.dispute.decide`,
`support.evidence.attach` for commands).

**Command state machine** (version fence on `admin_version`, one immutable decision per command)

- `TRANSITION_CASE`: OPEN ↔ UNDER_REVIEW ↔ AWAITING_EVIDENCE, ESCALATED → UNDER_REVIEW.
- `ESCALATE_CASE` / `ESCALATE_SOS` (emergency only) → ESCALATED + `escalation_level`.
- `CLOSE_CASE`: RESOLVED → `resolved`; NO_FURTHER_ACTION / DUPLICATE → `closed`.
- `ASSIGN_CASE`, `REPLY_TO_CASE` (idempotent by `client_message_id`; complainant
  notification is a separate retried delivery after commit), `ATTACH_EVIDENCE`
  (Media verdict required).
- Resolved/closed cases accept no action. `RESOLVE_DISPUTE` only from PENDING;
  `RECORD_DISPUTE_REMEDY` only for FAVOUR_COMPLAINANT/SPLIT, once. Both need an
  independent HQ approval.

**Events**: every committed case change (including new messages, evidence, read
state and disputes) emits exactly one `support.case.changed.v1` per version to
`admin_event_outbox` in the same transaction.

**Deliberately absent**: an SLA policy (events report `POLICY_UNAVAILABLE`), a
locale column (events carry `null`), and any Wallet execution — Support records
a remedy only as a link to a Wallet operation the Bridge already confirmed.
