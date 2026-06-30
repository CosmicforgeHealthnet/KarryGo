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
