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

A follow-up commit containing only formatting fixes and these reports redeploys the same code through the
same workflow; its result is recorded in 09.

## Behaviour change on staging

None yet. Every new capability is behind a switch that defaults off (see 03). Until the owner turns the
switches on in the staging settings file, the new routes answer 503 `service_unavailable` and the new
workers do not run.

## Not done

- No switch was turned on in staging settings: those files live on the server, are owner-controlled,
  and changing them is an owner action, not a code change.
- No live end-to-end exercise with real staging users.
