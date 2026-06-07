# KarryGo driver-dispatch-delivery-service — Full API, Event, Redis, Config, Migration, and Database Structure Report

## Audit Scope

- Scope scanned: `services/driver-dispatch-delivery-service/`
- Runtime wiring source: `cmd/main.go` and `internal/router/router.go`
- Route sources: registered production handlers only; test-only routes are excluded.
- Database source: all SQL files under `migrations/` plus production repository queries.
- Status language:
  - **Implemented**: registered and backed by production handler/service/repository code.
  - **Reserved/placeholder**: named in code but not active end-to-end.
  - **Not found**: no implementation or configuration was found in this service.

## 1. Service Overview

| Property | Confirmed implementation |
| --- | --- |
| Service name | `driver-dispatch-delivery-service` |
| Go module | `karrygo/services/driver-dispatch-delivery-service` |
| Local/container port | `8103`; default `HTTP_ADDR=:8103` |
| Base API path | `/api/v1` |
| Framework/runtime | Go 1.26.3, Gin, pgx/pgxpool, go-redis, Asynq, Gorilla WebSocket |
| Database | PostgreSQL database `dispatch_delivery_service`; local host port `5435`, container port `5432` |
| Redis | Redis 7 Alpine; local host port `6382`, container port `6379` |
| Docker image | Multi-stage Go build; Alpine 3.22 runtime; non-root `karrygo` user |
| Readiness | `/ready` pings PostgreSQL and Redis |
| Current Compose storage | VPS/local mounted volume `../tmp/karrygo-uploads:/app/uploads`; `VERIFICATION_STORAGE_MODE=local_private` |
| Stored file references | Default `local-private://...` references; raw filesystem paths are not returned by local uploaders |
| Firebase note | Verification and trip proof storage recognize `firebase` mode but return “not configured”; a working Firebase adapter and Firebase credential env keys were **not found**. Vehicle storage does not support Firebase mode. |
| AWS note | No AWS SDK, S3, SNS, or SQS integration was found. |
| Internal auth | `DISPATCH_DELIVERY_INTERNAL_SERVICE_KEY`; header is exactly `X-Internal-Service-Key` |
| Background work | Asynq server with concurrency `10`; queue weights `critical:6`, `default:3`, `low:1` |

Docker Compose starts the service with `driver-dispatch-delivery-postgres` and `driver-dispatch-delivery-redis`. PostgreSQL forward migrations are mounted into `/docker-entrypoint-initdb.d/` for first-time initialization.

## 2. Full Route/Endpoint List

All 47 registered HTTP endpoints are implemented. No registered HTTP endpoint is a stub. The non-route limitations and placeholders are listed after the route tables.

### Auth

| Method | Path | Auth | Request body/query | Response and purpose | Status |
| --- | --- | --- | --- | --- | --- |
| POST | `/api/v1/auth/start` | Public | JSON: `phone_number` | `200`; creates hashed OTP, enforces Redis rate limit, publishes OTP request, returns message and expiry only | Implemented |
| POST | `/api/v1/auth/verify` | Public | JSON: `phone_number`, `otp_code`, optional `device_id`, `device_type` | `200`; verifies OTP, upserts identity, creates session, returns access/refresh token result | Implemented |
| POST | `/api/v1/auth/refresh` | Public | JSON: `refresh_token` | `200`; validates session, rotates refresh token, returns new tokens | Implemented |
| POST | `/api/v1/auth/logout` | Bearer JWT | Optional JSON: `refresh_token` | `200`; revokes current session and publishes logout event | Implemented |

### Provider Profile

Protected provider routes require a valid access JWT with role `dispatch_provider`.

| Method | Path | Auth | Request body/query | Response and purpose | Status |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/provider/:id/public` | Public; per-IP rate limit | Path: provider UUID | `200`; public-only profile fields. Limited to 60 requests/minute/IP | Implemented |
| POST | `/api/v1/provider/onboarding` | JWT provider | JSON: `full_name`, optional `email`, `state`, `city`, `operation_type` | `200`; records one-time onboarding data and evaluates completion | Implemented |
| GET | `/api/v1/provider/me` | JWT provider | None | `200`; authenticated provider profile and onboarding flags | Implemented |
| PATCH | `/api/v1/provider/me` | JWT provider | JSON subset: `full_name`, `email`, `state`, `city`, `profile_photo_url` | `200`; updates allowed fields and publishes changed fields | Implemented |
| POST | `/api/v1/provider/emergency-contact` | JWT provider | JSON: `full_name`, `phone`, `relationship` | `200`; upserts emergency contact | Implemented |
| GET | `/api/v1/provider/emergency-contact` | JWT provider | None | `200`; returns provider-owned emergency contact | Implemented |
| POST | `/api/v1/provider/guarantor` | JWT provider | JSON: `full_name`, `phone` | `200`; upserts guarantor | Implemented |
| GET | `/api/v1/provider/guarantor` | JWT provider | None | `200`; returns provider-owned guarantor | Implemented |
| GET | `/api/v1/provider/stats` | JWT provider | None | `200`; returns trips, rating, completion rate, activity, verification status | Implemented |

### Verification

| Method | Path | Auth | Request body/query | Response and purpose | Status |
| --- | --- | --- | --- | --- | --- |
| POST | `/api/v1/provider/verification/identity` | JWT provider | Multipart: `govt_id_type`, `govt_id_number`, `govt_id_file`, `profile_photo` | `200`; stores identity documents, submits identity step, updates profile photo | Implemented |
| POST | `/api/v1/provider/verification/licence` | JWT provider | Multipart: `licence_number`, `expiry_year`, `expiry_month`, `licence_file` | `200`; stores licence and submits licence step | Implemented |
| POST | `/api/v1/provider/verification/face` | JWT provider | Multipart: `selfie` | `200`; stores selfie, creates face check, runs configured matcher | Implemented; real Smile Identity network call is not implemented |
| GET | `/api/v1/provider/verification/status` | JWT provider | None | `200`; overall status, completion percentage, step summaries | Implemented |
| GET | `/api/v1/provider/verification/status/:step` | JWT provider | Path: verification step | `200`; detailed step, documents, and latest face check | Implemented |
| PATCH | `/api/v1/admin/verification/:provider_id/review` | JWT `platform_admin` | JSON: `step`, `action` (`approve`/`reject`), `reason` | `200`; reviews submitted step and publishes resulting events | Implemented |

### Vehicle

| Method | Path | Auth | Request body/query | Response and purpose | Status |
| --- | --- | --- | --- | --- | --- |
| POST | `/api/v1/provider/vehicle` | JWT provider | JSON: `bike_type`, `brand`, `model`, `year`, `color`, `plate_number`, optional `engine_cc`, `chassis_number` | `201`; registers bike and publishes `vehicle.registered` | Implemented |
| GET | `/api/v1/provider/vehicle` | JWT provider | None | `200`; lists provider-owned bikes | Implemented |
| GET | `/api/v1/provider/vehicle/:id` | JWT provider | Path: bike UUID | `200`; provider-owned bike with documents | Implemented |
| PATCH | `/api/v1/provider/vehicle/:id` | JWT provider | JSON subset: `brand`, `model`, `year`, `color`, `engine_cc`, `chassis_number` | `200`; updates mutable bike fields | Implemented |
| POST | `/api/v1/provider/vehicle/:id/documents` | JWT provider | Multipart: `document_type`, optional/conditional `expiry_date`, `document_file` | `201`; validates, stores, and records registration/insurance document | Implemented |
| GET | `/api/v1/provider/vehicle/:id/documents` | JWT provider | Path: bike UUID | `200`; lists provider-owned bike documents | Implemented |
| PATCH | `/api/v1/admin/vehicle/:id/review` | JWT `platform_admin` | JSON: `action` (`approved`/`rejected`/`suspended`), `reason` | `200`; reviews bike, writes audit, publishes vehicle status event | Implemented |

### Availability

| Method | Path | Auth | Request body/query | Response and purpose | Status |
| --- | --- | --- | --- | --- | --- |
| PATCH | `/api/v1/provider/availability` | JWT provider | JSON: `status` (`online` or `offline`) | `200`; changes availability after verification/vehicle gates | Implemented |
| GET | `/api/v1/provider/availability` | JWT provider | None | `200`; returns live/persisted availability and daily statistics | Implemented |
| GET | `/api/v1/provider/availability/session/current` | JWT provider | None | `200`; returns current open availability session | Implemented |
| POST | `/api/v1/provider/location` | JWT provider | JSON: `lat`, `lng`, `heading`, `speed`, `accuracy` | `200`; caches location, updates GEO, publishes WebSocket/domain updates | Implemented |
| GET | `/api/v1/provider/location` | JWT provider | None | `200`; returns provider’s cached live location; `404` if absent/expired | Implemented |

### Request/Broadcast

| Method | Path | Auth | Request body/query | Response and purpose | Status |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/provider/requests` | JWT provider | Query: optional `status`, `limit` default 20, `page` default 1 | `200`; provider-scoped pending/history inbox list | Implemented |
| GET | `/api/v1/provider/requests/:id` | JWT provider | Path: inbox UUID | `200`; provider-scoped booking/request detail | Implemented |
| POST | `/api/v1/provider/requests/:id/accept` | JWT provider | No body | `200`; first-writer-wins Redis lock plus DB transaction; publishes accepted event | Implemented |
| POST | `/api/v1/provider/requests/:id/reject` | JWT provider | Optional JSON: `reason` (`too_far`, `busy`, `other`) | `200`; rejects request and publishes rejected event | Implemented |

### Trip

| Method | Path | Auth | Request body/query | Response and purpose | Status |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/provider/trips` | JWT provider | Query: optional `status`, `page` default 1, `limit` default 20/max 50 | `200`; provider-scoped paginated trip list | Implemented |
| GET | `/api/v1/provider/trips/active` | JWT provider | None | `200`; active provider trip | Implemented |
| GET | `/api/v1/provider/trips/:id` | JWT provider | Path: trip UUID | `200`; provider-scoped detail with state log and proof | Implemented |
| POST | `/api/v1/provider/trips/:id/arrived` | JWT provider | No body | `200`; transitions assigned/en-route trip to `arrived_pickup` | Implemented |
| POST | `/api/v1/provider/trips/:id/start` | JWT provider | No body | `200`; transitions arrived trip to `in_progress` | Implemented |
| POST | `/api/v1/provider/trips/:id/proof` | JWT provider | Multipart: `delivery_photo`, `signature`, `receiver_name`, `receiver_phone` | `201`; validates/stores permanent proof and transitions to `proof_submitted` | Implemented |
| GET | `/api/v1/provider/trips/:id/proof` | JWT provider | Path: trip UUID | `200`; returns provider-owned proof references and verification state | Implemented |
| POST | `/api/v1/provider/trips/:id/complete` | JWT provider | No body | `200`; verifies proof exists, marks proof verified, completes trip | Implemented |
| POST | `/api/v1/provider/trips/:id/cancel` | JWT provider | JSON: `reason_code`, optional `reason_text` | `200`; cancels allowed states and applies penalty/investigation rules | Implemented |

### Internal Route

| Method | Path | Auth | Request body/query | Response and purpose | Status |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/internal/nearby` | `X-Internal-Service-Key` | Required `lat`/`lng` or `latitude`/`longitude`; optional `radius`/`radius_km`, `limit` | `200`; returns online providers from Redis GEO; called by request broadcast client | Implemented |

### Health/Ready

| Method | Path | Auth | Request body/query | Response and purpose | Status |
| --- | --- | --- | --- | --- | --- |
| GET | `/health` | Public | None | `200`; service liveness payload | Implemented |
| GET | `/ready` | Public | None | `200` only when PostgreSQL and Redis ping successfully | Implemented |

### Registered Route Stub Check

- Registered HTTP route stubs/placeholders: **none found**.
- `trip.Service.FoundationOperation` returns `501`, but its handler is not registered as a route.
- Verification’s production router uses the environment-backed Smile Identity client. The client supports fake modes; the real external request is **not implemented** and returns unavailable.

## 3. WebSocket Routes

| Path | Auth | Query/path params | Messages sent | Redis channel | Purpose |
| --- | --- | --- | --- | --- | --- |
| `/ws/provider/:id/location` | JWT in `?token=`; token subject must match `:id`, unless role is `platform_admin` or `internal_service` | Path `id`; query `token` | `location_update`, `location_unavailable`, `provider_offline` | `avail:loc:chan:{provider_id}` | Sends current location immediately, then forwards live Redis Pub/Sub location/offline messages |

Additional confirmed behavior:

- Missing, invalid, or expired query token is rejected before upgrade.
- A provider JWT cannot stream another provider’s location.
- WebSocket origin validation currently returns `true` for every origin; JWT authorization remains required.

## 4. Internal Service Routes

| Path | Required header | Calling feature | Purpose |
| --- | --- | --- | --- |
| `GET /api/v1/internal/nearby` | `X-Internal-Service-Key: <DISPATCH_DELIVERY_INTERNAL_SERVICE_KEY>` | Request/Broadcast `HTTPNearbyClient` | Finds online nearby providers using Redis GEO for dispatch broadcasting |

The service-key middleware rejects Bearer authorization on this route, trims both values, and compares the key with constant-time comparison.

## 5. Redis Keys

Exactly 11 explicit Redis key patterns are defined by production code. Asynq also uses Redis internally, but its library-managed keys are not declared by this service and are therefore not counted.

| Key pattern | TTL | Feature | Set/write behavior | Read/delete behavior and purpose |
| --- | ---: | --- | --- | --- |
| `dispatch_rider_auth:otp_rate:{phone_number}` | Configurable; default 10 min | Auth | `INCR`, then `EXPIRE` on first OTP request | Read through increment count; auto-expires; limits OTP requests |
| `dispatch_rider_auth:session:{session_id}` | Refresh/session TTL; default 30 days | Auth | `SET` JSON session on session creation | Deleted on refresh rotation; no production `GET` found; DB remains authoritative |
| `avail:status:{provider_id}` | 90 sec | Availability | Set online/offline/busy; refreshed on accepted GPS ping | Read for location/nearby status; auto-expires |
| `avail:location:{provider_id}` | 30 sec | Availability | Set canonical location JSON | Read by location and WebSocket initial state; deleted when forced/offline |
| `avail:geo:online` | None | Availability | `GEOADD` discoverable online providers | `GEOSEARCH`; `ZREM` when offline/busy/stale |
| `avail:ratelimit:location:{provider_id}` | 60 sec | Availability | `INCR`, `EXPIRE` on first ping | Limits to 30 location attempts per minute/provider |
| `request:lock:{booking_id}` | 10 sec | Request | `SETNX` provider ID during accept | Released only by lock owner; first accept wins |
| `request:accepted:{booking_id}` | 24 hours | Request | Set after successful DB accept transaction | Existence checked to reject later accepts |
| `request:broadcasting:{booking_id}` | Broadcast expiry plus 5 sec | Request | Set to broadcast ID for live window | Checked on accept; deleted on accept/cancel/expiry |
| `request:ratelimit:accept:{provider_id}` | 10 sec | Request | `INCR`, `EXPIRE` | Limits accept attempts to 5/window; fails open on Redis errors |
| `request:ratelimit:reject:{provider_id}` | 60 sec | Request | `INCR`, `EXPIRE` | Limits reject attempts to 10/window; fails open on Redis errors |

Redis Pub/Sub location channel pattern, counted as a channel rather than a key:

- `avail:loc:chan:{provider_id}`

## 6. Redis Pub/Sub / Event Topics

There are 34 unique topic names in production source: 32 active publish/subscribe contracts and 2 reserved/placeholder topics.

### Published by This Service

| Topic | Producer | In-service consumer(s) | Payload fields | Purpose |
| --- | --- | --- | --- | --- |
| `provider.auth.otp_requested` | Auth | None | `event`, `correlation_id`, `phone_number`, `otp_code`, `purpose`, `expires_in_seconds`, `created_at` | Notification delivery contract; OTP must not be logged/returned |
| `provider.auth.session_created` | Auth | Profile | `event`, `correlation_id`, `provider_id`, `phone_number`, `role`, `session_id`, `created_at` | Creates sparse provider profile after first auth session |
| `provider.auth.logged_out` | Auth | None | `event`, `correlation_id`, `provider_id`, `session_id`, `created_at` | Announces session revocation |
| `provider.profile.updated` | Profile | None | `event`, `correlation_id`, `provider_id`, `changed_fields`, `created_at` | Announces allowed profile changes |
| `provider.onboarding.completed` | Profile | Verification | `event`, `correlation_id`, `provider_id`, `phone`, `operation_type`, `created_at` | Initializes six verification steps |
| `verification.step.submitted` | Verification | None | `event`, `correlation_id`, `provider_id`, `step`, `status`, `created_at` | Announces submitted verification step |
| `verification.face.failed` | Verification | None | `event`, `correlation_id`, `provider_id`, `step`, `result`, `match_score`, `created_at` | Announces failed face match |
| `verification.status.updated` | Verification | Profile | `event`, `correlation_id`, `provider_id`, optional `step`, `status`, `verification_status`, `created_at` | Mirrors verification status to provider profile |
| `verification.fully_approved` | Verification | Availability | `event`, `correlation_id`, `provider_id`, `approved_at`, `created_at` | Unlocks provider eligibility to go online |
| `verification.rejected` | Verification | None | `event`, `correlation_id`, `provider_id`, `step`, `reason`, `created_at` | Announces rejected step |
| `vehicle.registered` | Vehicle | None | `event`, `correlation_id`, `provider_id`, `bike_id`, `created_at` | Announces new bike |
| `vehicle.documents.submitted` | Vehicle | None | `event`, `correlation_id`, `provider_id`, `bike_id`, `document_type`, `created_at` | Announces vehicle document submission |
| `vehicle.verified` | Vehicle | Verification, Availability | `event`, `correlation_id`, `provider_id`, `bike_id`, `verified_at`, `created_at` | Approves verification vehicle step and ensures availability row |
| `vehicle.rejected` | Vehicle | Verification, Availability | `event`, `correlation_id`, `provider_id`, `bike_id`, `reason`, `created_at` | Rejects vehicle step and may force provider offline |
| `vehicle.suspended` | Vehicle | Availability | `event`, `correlation_id`, `provider_id`, `bike_id`, `reason`, `created_at` | May force provider offline if no verified active bike remains |
| `provider.went_online` | Availability | None | `event`, `provider_id`, `status`, `session_start`, `occurred_at` | Announces provider online session |
| `provider.went_offline` | Availability | None | `event`, `provider_id`, `status`, `went_offline_at`, `forced_offline`, `occurred_at` | Announces provider offline transition |
| `provider.location_updated` | Availability | Trip | `event`, `provider_id`, `lat`, `lng`, `heading`, `speed`, `accuracy`, `updated_at`, `occurred_at` | Automatically transitions an assigned trip to `en_route_pickup` |
| `request.accepted` | Request | Trip | Booking, inbox/broadcast/provider IDs, route/fare/receiver/package fields, timestamps | Creates/ensures assigned trip |
| `request.rejected` | Request | None | `event`, correlation/booking/broadcast/inbox/provider IDs, `reason`, timestamps | Announces provider rejection |
| `request.no_provider_found` | Request worker | None | `event`, `correlation_id`, `booking_id`, `broadcast_id`, `attempts`, `occurred_at` | Announces exhausted broadcast attempts |
| `trip.created` | Trip | None | `event`, correlation/trip/booking/provider/customer IDs, `status`, timestamps | Announces trip creation |
| `trip.provider_arrived` | Trip | None | IDs, pickup address/coordinates, `arrived_at`, `occurred_at` | Announces provider arrival at pickup |
| `trip.started` | Trip | Availability | IDs, pickup/drop-off coordinates/address, `started_at`, `occurred_at` | Marks provider busy and removes from discoverable GEO |
| `trip.proof_submitted` | Trip | None | IDs, `photo_ref`, `signature_ref`, receiver fields, timestamps | Announces permanent proof submission |
| `trip.completed` | Trip | Profile, Availability | IDs, fare/currency, `completed_at`, `occurred_at` | Increments profile trips and returns provider online |
| `trip.cancelled` | Trip | Availability | IDs, cancelled-by/reason/penalty/investigation fields, timestamps | Returns provider online and informs downstream consumers |
| `verification.suspension_flag` | Trip | None | `event`, `provider_id`, `reason`, `count_30_days`, `occurred_at` | Flags 3+ provider cancellations in 30 days |

### Subscribed/Expected External Topics

| Topic | Expected producer | Consumer(s) in this service | Payload fields | Purpose/status |
| --- | --- | --- | --- | --- |
| `booking.dispatch.created` | Booking/customer domain | Request | booking/customer IDs, pickup/drop-off, fare/payment/currency, receiver/package fields, payload, `occurred_at` | Starts broadcast |
| `booking.dispatch.cancelled` | Booking/customer domain | Request, Trip | `event`, `correlation_id`, `booking_id`, `reason`, `cancelled_at`, `occurred_at` | Cancels live broadcast and customer-cancellable trip states |
| `customer.rating.submitted` | Customer domain | Profile | `event`, provider/booking/customer IDs, score/comment, `created_at` | Inserts rating and recalculates average |
| `provider.verification.suspended` | Expected verification/admin producer; publisher not found here | Vehicle, Availability | `event`, `correlation_id`, `provider_id`, `reason`, `created_at` | Suspends all bikes and forces provider offline |

### Reserved/Placeholder Topics

| Topic | Status | Details |
| --- | --- | --- |
| `provider.profile.suspended` | Placeholder | Constant and empty subscriber function exist; payload handling and identity status update are TODO |
| `verification.suspended` | Reserved | Payload type exists for future admin suspension flow; publisher is not implemented |

## 7. Environment / Config Keys

| Environment/config key | Default / requirement | Feature | Purpose |
| --- | --- | --- | --- |
| `APP_ENV` | `development` | Runtime/storage | Production mode behavior and storage requirement |
| `SERVICE_NAME` | `driver-dispatch-delivery-service` | Runtime/health | Service label |
| `GIN_MODE` | Gin-managed; Compose sets `debug` | HTTP runtime | Gin mode; service also forces release mode when `APP_ENV=production` |
| `HTTP_ADDR` | `:8103` | HTTP runtime | Listen address |
| `DISPATCH_DELIVERY_DATABASE_URL` | Local PostgreSQL URL on port `5435` | Database | pgxpool connection |
| `DISPATCH_DELIVERY_REDIS_ADDR` | `localhost:6382` | Redis/Asynq | Redis address |
| `DISPATCH_DELIVERY_REDIS_PASSWORD` | Empty | Redis/Asynq | Redis password |
| `DISPATCH_DELIVERY_REDIS_DB` | `0` | Redis/Asynq | Redis database number |
| `DISPATCH_DELIVERY_INTERNAL_SERVICE_KEY` | `development-internal-service-key` | Internal API | Protects `/api/v1/internal/nearby` |
| `AVAILABILITY_SERVICE_URL` | `http://localhost:8103` | Request | Base URL used by internal nearby client |
| `BROADCAST_INITIAL_RADIUS_KM` | `5` | Request | Initial nearby search radius |
| `BROADCAST_RADIUS_INCREMENT_KM` | `3` | Request | Radius increase per rebroadcast |
| `BROADCAST_MAX_ATTEMPTS` | `3` | Request | Maximum broadcast attempts |
| `BROADCAST_WINDOW_SECONDS` | `30` | Request | Broadcast window duration |
| `DISPATCH_RIDER_ACCESS_TOKEN_SECRET` | **Required** | Auth | HS256 access token secret |
| `DISPATCH_RIDER_REFRESH_TOKEN_SECRET` | **Required** | Auth | Required config value; opaque refresh tokens are DB-hashed |
| `DISPATCH_RIDER_OTP_SECRET` | **Required** | Auth | HMAC-SHA256 OTP hashing secret |
| `DISPATCH_RIDER_JWT_ACCESS_TTL_MINUTES` | `15` | Auth | Access token TTL |
| `DISPATCH_RIDER_JWT_REFRESH_TTL_DAYS` | `30` | Auth | Refresh/session TTL |
| `DISPATCH_RIDER_OTP_TTL_MINUTES` | `10` | Auth | OTP expiry |
| `DISPATCH_RIDER_OTP_RATE_LIMIT_WINDOW_MINUTES` | `10` | Auth | OTP request rate window |
| `DISPATCH_RIDER_OTP_RATE_LIMIT_MAX` | `3` | Auth | OTP requests per window |
| `DISPATCH_RIDER_OTP_MAX_ATTEMPTS` | `3` | Auth | OTP verification attempts |
| `DISPATCH_RIDER_OTP_LOCKOUT_MINUTES` | `30` | Auth | Lockout duration |
| `DISPATCH_RIDER_DEBUG_OTP` | `false`; Compose `true` | Auth | Allows development OTP logging |
| `VERIFICATION_STORAGE_MODE` | Development falls back to `local_private`; production requires explicit mode | Verification/Vehicle; trip fallback | Storage selection |
| `VERIFICATION_UPLOAD_ROOT` | Feature-specific OS temp fallback; Compose `/app/uploads` | Verification/Vehicle; trip fallback | Local upload root |
| `VERIFICATION_STORAGE_BASE_URL` | `local-private://` | Verification/Vehicle; trip fallback | Returned private reference prefix |
| `SMILE_IDENTITY_FAKE_MODE` | Empty; Compose `pass`; disabled in production | Verification | `pass`, `fail`, or `unavailable` fake face matcher |
| `SMILE_IDENTITY_API_KEY` | Empty | Verification | Required for intended real provider mode |
| `SMILE_IDENTITY_PARTNER_ID` | Empty | Verification | Required for intended real provider mode |
| `SMILE_IDENTITY_BASE_URL` | Empty | Verification | Required for intended real provider mode |
| `SMILE_IDENTITY_MATCH_THRESHOLD` | `70.00` | Verification | Face match threshold |
| `TRIP_PROOF_STORAGE_MODE` | Falls back to `VERIFICATION_STORAGE_MODE` | Trip | Trip proof storage selection |
| `TRIP_PROOF_UPLOAD_ROOT` | Falls back to `VERIFICATION_UPLOAD_ROOT` then OS temp | Trip | Trip proof local root |
| `TRIP_PROOF_STORAGE_BASE_URL` | Falls back to `VERIFICATION_STORAGE_BASE_URL`, then `local-private://` | Trip | Trip proof returned reference prefix |

Asynq concurrency and queue weights are hardcoded in `cmd/main.go`; no Asynq-specific environment variables were found. Firebase credential/config environment keys were **not found**.

## 8. Database Migrations

There are 46 SQL migration files: 23 forward files and 23 rollback files. Five additional Go migration test files validate availability, request, trip, vehicle, and verification migrations.

| Phase/feature | Forward file | Rollback file | Creates/alters, indexes, constraints |
| --- | --- | --- | --- |
| Phase 1 Auth | `001_dispatch_rider_auth.sql` | `001_dispatch_rider_auth_down.sql` | Creates `dispatch_rider_otps`, `dispatch_rider_identities`, `dispatch_rider_sessions`; auth lookup/expiry/status indexes; identity status check; session FK |
| Phase 1 Auth | `002_dispatch_rider_auth_alter.sql` | `002_dispatch_rider_auth_alter_down.sql` | Alters auth phone/device/IP and timestamp column types |
| Phase 2 Profile | `000004_create_providers.up.sql` | `000004_create_providers.down.sql` | Creates `providers`; unique phone; operation/verification checks; status index |
| Phase 2 Profile | `000005_create_emergency_contacts.up.sql` | `000005_create_emergency_contacts.down.sql` | Creates one-per-provider `emergency_contacts`; provider FK/index |
| Phase 2 Profile | `000006_create_guarantors.up.sql` | `000006_create_guarantors.down.sql` | Creates one-per-provider `guarantors`; provider FK/index |
| Phase 2 Profile | `000007_create_ratings.up.sql` | `000007_create_ratings.down.sql` | Creates `ratings`; score check; provider index; unique booking index |
| Phase 3 Verification | `000008_create_verification_steps.up.sql` | `000008_create_verification_steps.down.sql` | Creates `verification_steps`; unique provider/step; step/status/method checks and indexes |
| Phase 3 Verification | `000009_create_verification_documents.up.sql` | `000009_create_verification_documents.down.sql` | Creates `verification_documents`; step/provider FKs, document-type check and indexes |
| Phase 3 Verification | `000010_create_face_checks.up.sql` | `000010_create_face_checks.down.sql` | Creates `face_checks`; provider/step FKs, result check, provider index |
| Phase 3 Verification | `000011_create_verification_audit.up.sql` | `000011_create_verification_audit.down.sql` | Creates `verification_audit`; action/step/status checks and provider index |
| Phase 3 Verification | `000012_expand_verification_audit_actions.up.sql` | `000012_expand_verification_audit_actions.down.sql` | Adds `submitted` and `face_failed` audit actions |
| Phase 3 Verification | `000013_add_fully_approved_audit.up.sql` | `000013_add_fully_approved_audit.down.sql` | Adds `fully_approved` action and `all` step |
| Phase 4 Vehicle | `000014_create_bikes.up.sql` | `000014_create_bikes.down.sql` | Creates `bikes`; unique plate; type/status checks; provider/status indexes |
| Phase 4 Vehicle | `000015_create_bike_documents.up.sql` | `000015_create_bike_documents.down.sql` | Creates `bike_documents`; bike/provider FKs; type check and indexes |
| Phase 4 Vehicle | `000016_create_bike_audit.up.sql` | `000016_create_bike_audit.down.sql` | Creates `bike_audit`; action/status checks and indexes |
| Phase 5 Availability | `000017_create_provider_availability.up.sql` | `000017_create_provider_availability.down.sql` | Creates `provider_availability`; unique provider; status check/index |
| Phase 5 Availability | `000018_create_availability_sessions.up.sql` | `000018_create_availability_sessions.down.sql` | Creates `availability_sessions`; nonnegative checks; provider/time indexes; one open session partial unique index |
| Phase 6 Request | `000019_create_request_broadcasts.up.sql` | `000019_create_request_broadcasts.down.sql` | Creates `request_broadcasts`; unique booking; status/radius/attempt/count checks and indexes |
| Phase 6 Request | `000020_create_provider_request_inbox.up.sql` | `000020_create_provider_request_inbox.down.sql` | Creates `provider_request_inbox`; unique provider/booking; status check and inbox indexes |
| Phase 7 Trip | `000021_create_trips.up.sql` | `000021_create_trips.down.sql` | Creates `trips`; unique booking; status check; provider/status/active/time indexes |
| Phase 7 Trip | `000022_create_trip_state_log.up.sql` | `000022_create_trip_state_log.down.sql` | Creates `trip_state_log`; state and actor checks; trip/time indexes |
| Phase 7 Trip | `000023_create_delivery_proofs.up.sql` | `000023_create_delivery_proofs.down.sql` | Creates one-per-trip `delivery_proofs`; unique trip index |
| Phase 7 Trip | `000024_create_cancellations.up.sql` | `000024_create_cancellations.down.sql` | Creates one-per-trip `cancellations`; cancelled-by check and indexes |

Migration test files:

- `availability_migrations_test.go`
- `request_migrations_test.go`
- `trip_migrations_test.go`
- `vehicle_migrations_test.go`
- `verification_migrations_test.go`

## 9. Database Tables

The service creates and uses 22 tables.

| Table | Feature | Primary key / important columns | Foreign keys, uniqueness, checks, indexes | Purpose |
| --- | --- | --- | --- | --- |
| `dispatch_rider_otps` | Auth | UUID `id`; phone, OTP hash, attempts, expiry, verified, lock time | Phone/expiry/verified/lock indexes | Persistent hashed OTP attempts and lockout |
| `dispatch_rider_identities` | Auth | UUID `id`; unique phone, status | Status check `active/suspended/deleted`; phone/status indexes | Auth identity |
| `dispatch_rider_sessions` | Auth | UUID `id`; rider ID, phone, refresh hash, metadata, expiry/revocation | FK to identity cascade; rider/phone/hash/expiry/revoked indexes | Refresh/session authority |
| `providers` | Profile | UUID `id`; unique phone, profile, verification/rating/trip/activity fields | Operation and verification checks; status index | Provider domain profile |
| `emergency_contacts` | Profile | UUID `id`; unique `provider_id` | FK to providers cascade; provider index | One emergency contact per provider |
| `guarantors` | Profile | UUID `id`; unique `provider_id` | FK to providers cascade; provider index | One guarantor per provider |
| `ratings` | Profile | UUID `id`; provider/booking/customer, score/comment | Provider FK cascade; score 1–5; unique booking; provider index | Customer ratings and provider average source |
| `verification_steps` | Verification | UUID `id`; provider, step, optional/auto flags, status/review fields | Provider FK cascade; unique provider/step; step/status/method checks and indexes | Verification workflow state |
| `verification_documents` | Verification | UUID `id`; step/provider, document type, file reference/metadata | Step/provider FKs cascade; type check; step/provider indexes | Verification file references |
| `face_checks` | Verification | UUID `id`; provider/step, selfie/ID refs, score/result/provider/error | Provider/step FKs cascade; pass/fail check; provider index | Face match results |
| `verification_audit` | Verification | UUID `id`; provider, step/action/from/to, performer/notes | Provider FK cascade; final action/step/status checks; provider index | Verification audit trail |
| `bikes` | Vehicle | UUID `id`; provider, type/details, unique plate, verification/active/primary | Provider FK cascade; type/status checks; provider/status indexes | Provider bikes |
| `bike_documents` | Vehicle | UUID `id`; bike/provider, type, file reference/metadata/expiry | Bike/provider FKs cascade; type check; bike/provider indexes | Registration/insurance files |
| `bike_audit` | Vehicle | UUID `id`; bike/provider, action/from/to, performer/notes | Bike/provider FKs cascade; action/status checks; bike/provider indexes | Vehicle audit trail |
| `provider_availability` | Availability | UUID `id`; provider, status, eligibility, session start/change time | Provider FK cascade; unique provider; status check/index | Current persisted availability |
| `availability_sessions` | Availability | UUID `id`; provider, online/offline/duration/trips/forced fields | Provider FK cascade; nonnegative checks; one-open-session partial unique index | Availability session history |
| `request_broadcasts` | Request | UUID `id`; unique booking, radius/attempt/count/status/expiry, accepted provider, JSON payload | Accepted provider FK; status/positive checks; booking/status/expiry/accepted-provider indexes | Dispatch broadcast authority |
| `provider_request_inbox` | Request | UUID `id`; broadcast/booking/provider/status/timestamps/FCM state | Broadcast/provider FKs cascade; unique provider/booking; status and listing indexes | Per-provider request inbox |
| `trips` | Trip | UUID `id`; unique booking, provider/customer, status, route/fare/receiver/package/timestamps | Provider FK; status check; booking/provider/status/active/time indexes | Delivery trip authority |
| `trip_state_log` | Trip | UUID `id`; trip/from/to/changed-by/time/notes | Trip FK cascade; state/actor checks; trip/time indexes | Permanent trip transition audit |
| `delivery_proofs` | Trip | UUID `id`; unique trip, photo/signature refs, receiver, submitted/verified times | Trip FK cascade; unique trip index | Permanent one-per-trip delivery proof |
| `cancellations` | Trip | UUID `id`; unique trip, actor/reason/penalty/time | Trip FK cascade; actor check; trip/actor/time indexes | Permanent one-per-trip cancellation record |

## 10. File Upload / Storage Paths

| Area | Accepted files and maximums | Object/reference format | Storage behavior |
| --- | --- | --- | --- |
| Verification government ID | JPEG, PNG, PDF; 5 MB | `verifications/{provider_id}/identity/{uuid}_{sanitized_filename}`; default `local-private://...` | Magic-byte MIME detection; stored in `verification_documents` |
| Verification profile photo | JPEG, PNG; 3 MB | Same identity path pattern | Uploaded during identity verification and copied to `providers.profile_photo_url` |
| Verification licence | JPEG, PNG, PDF; 5 MB | `verifications/{provider_id}/licence/{uuid}_{sanitized_filename}` | Magic-byte MIME detection |
| Verification selfie | JPEG, PNG; 3 MB | `verifications/{provider_id}/face/{uuid}_{sanitized_filename}` | Used for face check |
| Vehicle documents | JPEG, PNG, PDF; 5 MB | `vehicles/{provider_id}/{bike_id}/{document_type}/{uuid}_{sanitized_filename}`; default `local-private://...` | Checks declared MIME and magic bytes; insurance requires future expiry date |
| Profile update URL | No file upload on profile route; optional HTTPS URL only | HTTPS URL supplied by provider | Profile route validates HTTPS; verification flow can set stored private profile reference internally |
| Trip delivery photo | JPEG or PNG; 5 MB | `trips/{provider_id}/{trip_id}/proof/{uuid}_photo_{sanitized_filename}` | Header and JPEG/PNG magic-byte validation |
| Trip signature | JPEG or PNG; 3 MB at service layer | `trips/{provider_id}/{trip_id}/proof/{uuid}_signature_{sanitized_filename}` | Header and JPEG/PNG magic-byte validation |

Storage conclusions:

- Current Compose mode is `local_private`, rooted at `/app/uploads` on a mounted VPS/local volume.
- Returned default references begin with `local-private://`; local uploaders do not return raw filesystem paths.
- Verification and trip recognize a `firebase` mode but deliberately fail because the Firebase uploader is not configured.
- Vehicle storage supports only local/local-private mode.
- No functional Firebase adapter or Firebase credential keys were found.
- No S3 uploader, AWS SDK, SNS, or SQS integration was found.

## 11. Feature-by-Feature Summary

| Phase | Main endpoints | Main tables | Main events/Redis | Tests and guarantees | Current status |
| --- | --- | --- | --- | --- | --- |
| Phase 1 — Auth | Start, verify, refresh, logout | Auth OTP/identity/session tables | OTP rate/session keys; auth OTP/session/logout events | OTP hash/expiry/lockout/rate limits, token validation, refresh rotation, revocation | Complete; JWT `jti` blacklist deferred |
| Phase 2 — Profile | Public profile, onboarding, me, emergency contact, guarantor, stats | Providers, emergency contacts, guarantors, ratings | Profile/onboarding events; consumes session, verification, trip, rating events | Ownership, read-only fields, public-field filtering, rate limit, idempotent subscribers | Complete; profile suspension subscriber placeholder remains |
| Phase 3 — Verification | Identity/licence/face submit, statuses, admin review | Verification steps/documents/face checks/audit | Verification events; consumes onboarding and vehicle review events | MIME/magic checks, review role, audit, event/subscriber idempotency | Complete for fake matcher/local storage; real Smile/Firebase adapters not implemented |
| Phase 4 — Vehicle | Register/list/get/update, documents, admin review | Bikes, bike documents, bike audit | Vehicle events; consumes provider suspension | Ownership/IDOR, immutable fields, MIME/magic/size, audit, status conflicts | Complete for local-private storage |
| Phase 5 — Availability | Status/session/location, nearby internal route, location WebSocket | Provider availability, availability sessions | Live status/location/GEO/rate keys and location channel; availability events/subscribers | Online gates, service key, query-token WS ownership, GPS rate limit, trip busy/return flow | Complete |
| Phase 6 — Request/Broadcast | Inbox/list/detail/accept/reject | Request broadcasts, provider inbox | Five request keys, booking subscribers, request events, three Asynq task types | Atomic accept lock, IDOR, expiry/rebroadcast, cancel, rate limits, no-provider event | Complete; accepted event distance is currently emitted as `0` |
| Phase 7 — Trip Lifecycle | List/active/detail, arrived/start/proof/get proof/complete/cancel | Trips, state log, delivery proofs, cancellations | Trip events; consumes request accepted, booking cancelled, provider location | State guards, ownership, proof permanence, completion verification, cancellation penalties/customer cancel rules | Complete |

## 12. Security Rules Implemented

### Confirmed Controls

- Provider APIs require HS256 access JWTs and role `dispatch_provider`; admin review APIs require `platform_admin`.
- Provider IDs are taken from JWT context rather than trusted from request bodies.
- Repositories scope provider-owned vehicle, request, and trip reads/actions by provider ID; IDOR tests expect not-found/denied behavior.
- Internal nearby route requires `X-Internal-Service-Key`, rejects Bearer auth, and uses constant-time comparison.
- OTPs are stored as HMAC-SHA256 hashes; refresh tokens are stored as SHA-256 hashes.
- OTP request rate limit defaults to 3 per 10 minutes; OTP lockout defaults to 3 failed attempts for 30 minutes.
- Public profile is limited to 60 requests/minute/IP.
- Location updates are limited to 30 attempts/minute/provider.
- Request accepts use a 10-second atomic Redis `SETNX` lock plus a 24-hour accepted marker.
- Request accept/reject attempts have per-provider Redis rate limits.
- Verification and vehicle files use magic-byte MIME checks; trip proof uses explicit JPEG/PNG magic-byte checks.
- Storage paths are cleaned, traversal/absolute paths are rejected, and filenames are sanitized.
- Trip transitions use an explicit state machine. Provider cancellation is limited to assigned, en-route, arrived, and in-progress states.
- In-progress provider cancellation applies penalty and requires admin investigation; 3+ recent cancellations emit a suspension flag.
- Completion requires a proof row and atomically marks the proof verified with `verified_at`.
- Delivery proof is one-per-trip by unique constraint and duplicate submission guard.
- Local uploaders return private references, not raw filesystem paths.
- Production-source/test guards confirm no AWS SDK/S3/SNS/SQS integration.

### Confirmed Gaps / Limitations

- Access JWTs do not include `jti`; access-token blacklist support is TODO.
- Session Redis cache is written and deleted on refresh rotation, but no production cache read is present; logout revokes DB session but does not delete that cache key.
- WebSocket `CheckOrigin` currently allows every origin; token authorization still applies.
- `provider.profile.suspended` subscriber is a placeholder with no payload handling.
- `verification.suspended` publisher is reserved but not implemented.
- Real Smile Identity network matching is not implemented; configured non-fake mode returns unavailable.
- Firebase storage adapter and Firebase credential configuration are not implemented.
- Failed second trip-proof file storage has a TODO for best-effort cleanup of the first stored file.
- Duplicate `request.accepted` is DB-idempotent for trip creation but may republish `trip.created`; consumers are instructed to deduplicate by `trip_id`.

## 13. Testing Commands and Coverage

### Test Inventory

- Test files: **61**
- Named `Test...` functions: **768**
- Migration test files: **5**
- Main tested areas: auth token/OTP/session behavior, route protection, profile ownership/public filtering, verification workflow/audit/storage, vehicle IDOR/storage/review, availability live store/WebSocket/internal auth/rate limits, request locks/rebroadcast/IDOR, trip state/proof/completion/cancellation/customer events, and migration apply/validate/rollback.
- Numeric statement/branch coverage percentage: **Not confirmed in code**; no checked-in coverage report was found.

### Standard Commands

```powershell
go work sync
cd services/driver-dispatch-delivery-service
go mod tidy
gofmt -w .
go test ./...
go vet ./...
go build ./...
```

Docker fallback used by this workspace:

```powershell
docker run --rm -v "${PWD}:/workspace" -w /workspace/services/driver-dispatch-delivery-service golang:1.26-alpine go test ./...
docker run --rm -v "${PWD}:/workspace" -w /workspace/services/driver-dispatch-delivery-service golang:1.26-alpine go vet ./...
docker run --rm -v "${PWD}:/workspace" -w /workspace/services/driver-dispatch-delivery-service golang:1.26-alpine go build ./...
```

Feature-focused examples:

```powershell
go test ./internal/features/auth/...
go test ./internal/features/profile/...
go test ./internal/features/verification/...
go test ./internal/features/vehicle/...
go test ./internal/features/availability/...
go test ./internal/features/request/...
go test ./internal/features/trip/...
go test ./migrations
```

### Runtime/Validation Evidence

- `verify_integration.go` is a standalone local Phase 4 vehicle integration harness covering Docker HTTP, PostgreSQL, Redis events, route protection, storage references, and subscriber resilience. It is not a `go test` file.
- Phase-focused unit/integration tests exist through Phase 7, including `phase6cde/fgh/ijk` and `phase7cde/fgh/ijk/lmn`.
- Current audit attempt on **June 7, 2026**:
  - `docker compose -f infra/docker-compose.yml config --quiet`: **passed**
  - Docker `go test ./...`, `go vet ./...`, and `go build ./...`: **not rerun**, because the Docker Desktop Linux engine was not running.

## 14. Final API Inventory

| Inventory item | Total |
| --- | ---: |
| Registered HTTP endpoints | **47** |
| WebSocket routes | **1** |
| Internal service routes | **1** |
| Unique event topics found | **34** |
| Active event topics | **32** |
| Reserved/placeholder event topics | **2** |
| Explicit Redis key patterns | **11** |
| Redis location channel patterns | **1** |
| Database tables | **22** |
| SQL migration files | **46** |
| Forward migrations | **23** |
| Rollback migrations | **23** |
| Migration Go test files | **5** |
| All Go test files | **61** |

## 15. Output Format and Audit Notes

- This report is Markdown and is based on the current local code under `services/driver-dispatch-delivery-service/`.
- Unrelated frontend, mobile, admin, and other microservice implementation code was excluded.
- External producers/consumers are named only where the service’s code comments or subscriber wiring establish the contract.
- No route, configuration key, event topic, Redis key, table, or migration was invented.
- Anything not confirmed by production code is explicitly marked as not found, reserved, placeholder, or not confirmed.
