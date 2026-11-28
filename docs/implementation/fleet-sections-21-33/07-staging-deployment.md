# Fleet Sections 21-33: staging deployment

## Mechanism

The existing approved staging path, unchanged: a push to `staging` runs
`.github/workflows/deploy-staging-auto.yml`. It works out which services changed, builds and pushes an
image tagged with the commit, takes a database backup on the server, deploys with
`deploy/platform/bin/deploy-service.sh` (which rolls back automatically if the new version is not
healthy), and checks that the fleet containers are running the expected revision. Production, `main`,
protected approvals and the strict `deploy-platform.yml` flow were not used or bypassed.

## Deployment of `ae9c641e` (2026-11-26)

Workflow run `45906119149`: **success**. Services selected: `fleet-partner-service` only.

| Evidence (from the run log) | Value |
|---|---|
| Pre-deploy database backup | `/opt/karrygo/backups/predeploy/fleet-partner-service-20261126T224101Z.sql.gz` |
| Database migrations | applied by the service at startup (`builtin` runner) |
| `karrygo-fleet-partner-api` image revision | `ae9c641e0bc33ad40f041354ceae3859af61260b` (sha256:19615661...) |
| `karrygo-fleet-partner-worker` image revision | same |
| API health | `healthy` |

The API container's health check calls `/ready`, which fails unless the database answers **and every
bundled migration is applied** (`database.MigrationsCurrent`). The container reported healthy, so staging
is at migration head `000104_privacy_requests`. This is inferred from the readiness gate; no direct SQL
query was run against the staging database (no direct database access is used from here).

## Redeployment of `dc1569ed` (formatting-only follow-up)

Workflow run `45908256922`: **success**. Backup
`/opt/karrygo/backups/predeploy/fleet-partner-service-20261126T231316Z.sql.gz`; api and worker both on revision
`dc1569ed1fe5e115045a931a868b07afdefc417e` (sha256:6522ab0b...); API `healthy`. This is the revision running on
staging at closure.

## Behaviour change on staging

None yet. Every new capability is behind a switch that defaults off (see 03). Until the owner turns the
switches on in the staging settings file, the new routes answer 503 `service_unavailable` and the new
workers do not run.

## Not done

- No switch was turned on in staging settings: those files live on the server, are owner-controlled,
  and changing them is an owner action, not a code change.
- No live end-to-end exercise with real staging users.

---

# Final closure: deployment, activation and runtime evidence (2026-11-27)

## Deployment of `d5409a21` (code) - run `45947752384`, success

Ten services rebuilt and deployed by `deploy-staging-auto.yml` (Fleet for its own changes; nine through
the owner-authorized shared-Dockerfile patch; media also for its OpenTelemetry bump). Each was backed up
first and reported healthy; no rollback happened.

| Service | Pre-deploy backup | Healthy on |
|---|---|---|
| media-file-service | `media-file-service-20261127T055940Z.sql.gz` | `d5409a21` |
| payment-wallet-service | `payment-wallet-service-20261127T060012Z.sql.gz` | `d5409a21` |
| verification-compliance-service | (no database container on the host) | `d5409a21` |
| fleet-partner-service | `fleet-partner-service-20261127T060129Z.sql.gz` | `d5409a21` |
| customer-service | `customer-service-20261127T060156Z.sql.gz` | `d5409a21` |
| driver-taxi-service | `driver-taxi-service-20261127T060220Z.sql.gz` | `d5409a21` |
| driver-hauling-service | `driver-hauling-service-20261127T060238Z.sql.gz` | `d5409a21` |
| support-dispute-service | `support-dispute-service-20261127T060300Z.sql.gz` | `d5409a21` |
| referral-service | `referral-service-20261127T060321Z.sql.gz` | `d5409a21` |
| pricing-service | `pricing-service-20261127T060339Z.sql.gz` | `d5409a21` |

Every one of the ten image builds printed `openssl-runtime libssl3 3.5.9-r2` and
`openssl-runtime libcrypto3 3.5.9-r2` from the build-time floor check.

Not redeployed by design: api-gateway, partner-web-service, admin-backoffice-service,
driver-dispatch-delivery-service and analytics-service are manual-deploy services. Dispatch's websocket fix
is therefore in CI and in the image it will next build, but not yet on its staging runtime.

The follow-up commit `befb4c33` (Hauling test-only) redeployed through run `45949687555`, success.

## Capability activation - run `45949687696`, success

Applied through `fleet-staging-modules.yml` (authorized initiator, settings backup, health wait, automatic
restore on failure). Fleet API and worker reported healthy with the new settings at 06:12:19 UTC.

| Switched on | Left off, and why |
|---|---|
| `OPERATIONAL_EXCEPTIONS_ENABLED`, `NOTIFICATION_PREFERENCES_ENABLED`, `ANALYTICS_REPORTS_ENABLED`, `FEATURE_AI_RECOMMENDATIONS`, `FEATURE_AUTOMATION`, `PRIVACY_REQUESTS_ENABLED`, `SECURITY_EVENTS_ENABLED`, `SECURITY_SESSIONS_ENABLED`, `SECTION21_WORKERS_ENABLED`, `SECTION24_WORKERS_ENABLED` (warehouse, fulfilment, yard and finance read flags were already on) | `SECTION22_WORKERS_ENABLED` (expense posting and posting reconciliation call the wallet); `SECTION23_WORKERS_ENABLED` (would deliver every notification intent queued since 2026-11-13 at once to real staging inboxes, and SLA escalation depends on it); `SUPPORT_PROJECTIONS_ENABLED` (no support-dispute credentials on staging); finance posting, wallet allocations and withdrawals (unchanged) |

The post-change report (names and booleans only) shows each switch on as listed and the left-off ones absent.

## Runtime evidence - read-only run `45950455856` (06:15 UTC)

| Item | Value |
|---|---|
| `karrygo-fleet-partner-api` | running, healthy, revision `d5409a21419f9c6be592f2154aaadd9651eea1b9`, started 06:12:13 (activation restart) |
| `karrygo-fleet-partner-worker` | running, revision `d5409a21...` (the container defines no health check) |
| Release image | `ghcr.io/cosmicforgehealthnet/fleet-partner-service:d5409a21...` |
| Migration head | `000105_security_session_management` (105 applied), read from `fleet_partner_schema_migrations` |
| Settings file | mode 600, owned by the deploy account (contents never read) |

## Live probe of the deployed API (unauthenticated, `https://fleet-api.staging.karryam.com`)

`/health` 200 and `/ready` 200. `GET /fleet-partner/v1/security/sessions`,
`POST .../security/sessions/{id}/revoke`, `GET .../audit-events`, `POST .../sessions/{id}/revoke`,
`GET .../report-runs` and `GET .../exceptions` all answer **401** to an anonymous caller, while an unknown
path answers 404: the new routes are mounted and refuse unauthenticated access.

## Not done on staging

Authenticated journeys (warehouse receive/put-away/pick, invoice and payroll, exception escalation,
automation approval by a second person, privacy requests, session revocation) were not run: they need
signed-in staff, which means OTP delivery to an inbox this session can read and, for separation of duties,
two distinct staff identities. Neither is available here. Worker activity after activation is evidenced
only by the worker running on the new revision; its logs were not read.

## Final closure, round 2 (2026-11-27)

| Step | Run | Result |
|---|---|---|
| Read-only evidence before changes | `45971479853` | Fleet `d5409a21`; Support/Dispute lists its callers explicitly (incl. Fleet); Dispatch still `8f992eee` (pre websocket fix); notification `ef9e5b39` (unpatched OpenSSL) |
| Backlog evidence (aggregates only) | `45973177043` | **No** notification intents, Section 22/23 jobs, posting operations or escalations exist on staging. The earlier statement that a backlog had been queued since 2026-11-13 was wrong. One Fleet organization, `UNDER_REVIEW`. Fleet templates absent from notification-service. |
| Deploy (automatic) `73ab7ff9` | `45973177045` | Fleet, Support/Dispute and notification healthy on `73ab7ff9`; every image `openssl-runtime libssl3/libcrypto3 3.5.9-r2` |
| Fleet settings: `PRIVACY_SETTINGS_ENABLED` | `45973176981` | success |
| Pairing Fleet -> Support/Dispute | `45981913711` | new shared secret generated on the server; Support/Dispute's six other callers kept; `SUPPORT_PROJECTIONS_ENABLED` and Fleet's Support/Dispute URL set; all touched services healthy |
| Fleet settings: `SECTION22_WORKERS_ENABLED`, `SECTION23_WORKERS_ENABLED` | `45983930377` | success, Fleet API and worker healthy 11:02:37 UTC |
| Deploy (automatic) `0cbe08e4` (formatting only) | `45985568753` | success |
| Read-only evidence after | `45988729997` | Fleet API and worker on `0cbe08e4`, migration head `000106` (106 applied); Support/Dispute and notification on `73ab7ff9` |

Live anonymous probe: `GET` and `PATCH /fleet-partner/v1/privacy-settings` and
`POST .../support/cases/{id}/messages` answer 401; an unknown path 404; `/health` and `/ready` 200.

Worker behaviour: the Section 22/23 schedulers enqueue work per **active** organization. Staging's only
organization is `UNDER_REVIEW`, so no Section 22/23 job has run yet (evidence `45988729997` shows none);
they will start once an organization is activated.

Not applied: notification-service starts with `MIGRATION=false` on staging, so its migration 012 (Fleet
templates) is in the image but has not run; until an approved migration run applies it, Fleet notices are
refused with "template not found" and dead-lettered after 8 attempts (no loop, no email).

## Email redesign and authenticated testing (2026-11-28)

| Step | Run | Result |
|---|---|---|
| Deploy (automatic) `0f1c4894` | `46145807265` | notification-service and Fleet API/worker healthy on `0f1c4894` (`NOTIFICATION_RUNTIME_EVIDENCE`, `FLEET_RUNTIME_EVIDENCE`); every mandatory workflow green |
| Earlier attempts `7c0016bd`, `8f837af0` | | Not deployed: a gofmt failure, then a skipped preview test (the Customer V4 gate counts a skip as a failure) |
| Deploy (automatic) `87aebf1d` | `46154680166` | Fleet API/worker healthy on `87aebf1d` (approval create response fix); all 11 workflows green |

Authenticated journeys run against this revision are recorded in 09, section E.
