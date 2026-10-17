# Vendor staging deployment and certification

## Discovered deployment authority

The repository's deployed backend convention is GitHub Actions -> GHCR -> SSH ->
Docker Compose on the KarryGo VPS. `deploy-api-gateway.yml` and
`deploy-media-file-service.yml` both use the repository secrets `VPS_HOST`,
`VPS_USER`, and `VPS_SSH_KEY`, then run Compose below `/opt/karrygo`.

Vendor does not belong behind the current partner-auth API Gateway. The staged
topology therefore uses direct Vendor ingress through the VPS's existing public
reverse proxy/load balancer. The API container publishes only on
`127.0.0.1:8112`; the Worker publishes no port. The public proxy must forward
only `/api/v1/vendor/` and `/api/v1/commerce/`. It must not forward `/internal/`
or `/api/v1/internal/`.

The repository and this workstation do not identify or provide access to the
VPS reverse proxy, PostgreSQL provider, monitoring system, DNS provider, or TLS
control plane. The commands below are the remaining operator actions; none may
be recorded as passed until their output is retained in the release evidence.

## 1. Install the deployment prerequisites

As the VPS administrator, replace `<VPS_USER>` with the value represented by the
existing GitHub secret. Do not paste its value into a ticket or repository.

```sh
sudo install -d -m 0750 -o <VPS_USER> -g <VPS_USER> /opt/karrygo/vendor-service
scp deploy/vendor-service/vendor.env.example <VPS_USER>@<VPS_HOST>:/opt/karrygo/vendor-service/vendor.env
ssh <VPS_USER>@<VPS_HOST> 'chmod 600 /opt/karrygo/vendor-service/vendor.env'
```

Populate `vendor.env` through the approved secret manager or an SSH session.
At minimum provide Vendor PostgreSQL, Redis, access/refresh/OTP signing secrets,
and the approved internal caller map. Leave the global kill switch enabled and
all downstream flags disabled until each reciprocal credential/allowlist probe
passes. Never echo or print the file.

After this change is reviewed and merged by the operator, deploy the immutable
commit image:

```sh
gh workflow run deploy-vendor-service.yml --ref staging
gh run list --workflow deploy-vendor-service.yml --limit 1
```

The workflow must pass in this order: immutable image push, Compose validation,
dedicated migration job, API readiness, then Worker start. Verify on the VPS:

```sh
cd /opt/karrygo/vendor-service
export VENDOR_IMAGE="ghcr.io/cosmicforgehealthnet/vendor-service:<DEPLOYED_SHA>"
export VENDOR_ENV_FILE=/opt/karrygo/vendor-service/vendor.env
docker compose -f docker-compose.staging.yml ps
docker inspect -f '{{.State.Health.Status}}' karrygo-vendor-api-staging
docker inspect -f '{{.State.Running}}' karrygo-vendor-worker-staging
test -z "$(docker port karrygo-vendor-worker-staging)"
```

Pass condition: API is healthy, Worker is running with no published port, and
the migration job reports the current migration count without an error.

## 2. Identify and configure the existing public ingress

Run these read-only commands on the VPS to identify the real ingress product:

```sh
sudo ss -ltnp | grep -E ':(80|443)[[:space:]]'
docker ps --format '{{.Names}}\t{{.Image}}\t{{.Ports}}' | grep -Ei 'nginx|traefik|caddy|haproxy|gateway|proxy'
systemctl --type=service --state=running | grep -Ei 'nginx|traefik|caddy|haproxy'
```

Update that existing product—do not install a second proxy—to terminate TLS and
forward only the two public prefixes to `http://127.0.0.1:8112`. Preserve
`Authorization`, `Idempotency-Key`, `If-Match`, the platform correlation header,
content type, body, query string, and response status. Configure the proxy itself
to reject the two internal prefixes before proxying.

With the operator-provided staging hostname, retain the following output:

```sh
export VENDOR_STAGING_BASE_URL='https://<staging-vendor-host>'
curl -fsS -o /dev/null -D public-vendor.headers "$VENDOR_STAGING_BASE_URL/api/v1/vendor/meta"
curl -sS -o /dev/null -D public-commerce.headers "$VENDOR_STAGING_BASE_URL/api/v1/commerce/carts"
curl -sS -o /dev/null -w '%{http_code}\n' "$VENDOR_STAGING_BASE_URL/internal/staff/provision-owner"
curl -sS -o /dev/null -w '%{http_code}\n' "$VENDOR_STAGING_BASE_URL/internal/vendor/v1/probe"
curl -sS -o /dev/null -w '%{http_code}\n' "$VENDOR_STAGING_BASE_URL/api/v1/internal/probe"
openssl s_client -connect '<staging-vendor-host>:443' -servername '<staging-vendor-host>' -verify_return_error </dev/null
```

Pass condition: public routes reach Vendor or its expected auth challenge; all
internal probes are rejected by the ingress rather than returning a Vendor
authentication response; TLS hostname and chain verification succeeds.

## 3. PostgreSQL backup, restore, and reconciliation

First identify the real PostgreSQL owner without exposing its connection string:

```sh
cd /opt/karrygo/vendor-service
set -a; . ./vendor.env; set +a
psql "$VENDOR_DATABASE_URL" -XAtc "select version(), current_database(), inet_server_addr(), inet_server_port()"
docker ps --format '{{.Names}}\t{{.Image}}' | grep -i postgres || true
```

Record whether the endpoint is VPS-hosted PostgreSQL or a named managed provider.
The Database/DevOps owner must then enable that provider's native encrypted
backup facility, retention, failure notification, and access policy. Record the
provider name, backup policy ID, encryption status, latest successful timestamp,
and approved—not merely recommended—RPO/RTO. This repository cannot supply a
provider-specific command until the preceding identity is returned.

Restore the newest backup to a new database/instance whose name ends in
`_restore_drill_<UTC_DATE>`. Never target the primary. Keep commerce writes,
event consumers, and dangerous workers disabled. Using a read-only credential,
record counts and representative IDs for the domain tables listed in the final
certification, and verify:

```sql
SELECT count(*), max(applied_at) FROM vendor_service_schema_migrations;
SELECT status, count(*) FROM vendor_outbox GROUP BY status ORDER BY status;
SELECT source_service, count(*) FROM vendor_inbox_processed_events GROUP BY source_service ORDER BY source_service;
SELECT status, count(*) FROM vendor_job_controls GROUP BY status ORDER BY status;
```

If actual table names differ, resolve them with `information_schema.tables` and
record that mapping; do not fabricate a successful query. Reconcile Wallet and
logistics by their stored operation/mission IDs, refresh Verification projections,
rebuild Search beside live, resume consumers, then safe workers. Enable writes
only after backlog convergence and readiness pass.

## 4. Monitoring and alerts

Identify the existing VPS/organization monitoring product:

```sh
docker ps --format '{{.Names}}\t{{.Image}}' | grep -Ei 'prometheus|grafana|datadog|otel|newrelic|loki|vector|fluent|elastic'
systemctl --type=service --state=running | grep -Ei 'prometheus|grafana|datadog|otel|newrelic|loki|vector|fluent|elastic'
```

Connect Vendor API/Worker logs and metrics to that product. The dashboard must
show request latency and error rate; checkout quote; payment orchestration;
UNKNOWN payment/refund; vendor-order acceptance; stock-reservation conflicts;
fulfilment quote; provider match; delivery failure; settlement lag; outbox age;
worker backlog and dead letters; search projection lag/rebuild duration; and
PostgreSQL readiness.

Install alert rules for PostgreSQL unavailable, oldest outbox age, dead letters,
worker BLOCKED, Wallet UNKNOWN age, refund UNKNOWN age, settlement lag, stock
reservation leak, provider no-supply backlog age, OTP/QR dependency outage,
Verification expiry spike, and search projection lag. Link every rule to its
matching runbook. In staging, trigger and clear PostgreSQL unavailable,
dead-letter, blocked-worker, and dependency-outage examples. Retain screenshots
or exported rule evaluations proving signal, fire, runbook link, and clear.

## 5. Credentials, allowlists, and live matrices

For Wallet, Dispatch, Hauling, Verification, Media, Notification, Support,
Customer, and Taxi Goods if enabled, the service owners must supply the staging
URL, HMAC v2 secret reference, caller ID, and reciprocal allowlist entry. Record
only presence, version, and caller names—not values. For each integration run an
authorized request and a wrong-caller request; pass requires acceptance then
rejection.

After those probes pass, execute the black-box matrices from the final execution
brief using test money and staging missions. Retain operation IDs and status
transitions with OTP/QR values redacted. Only then enable the corresponding flag.

## 6. Failure injection and load test

Before either test, attach evidence that the hostname, database, Wallet account,
and every downstream endpoint are staging/non-production. If any identity is
unknown, stop. Run the seven bounded dependency failures, then the nine load
scenarios at no more than 25 clients for the declared 15-minute measurement
window. Record requests, concurrency, duration, throughput, p50/p95/p99, errors,
timeouts, DB pool, CPU, memory, backlog, and recovery time. Do not label the
result production capacity.

## Enabling the full Vendor-app feature set

`sudo bash deploy/vendor-service/enable-staging-app-features.sh staging` (or
`production`; the argument must equal `APP_ENV` in `vendor.env`) writes every
flag the Vendor app needs, turns `COMMERCE_GLOBAL_KILL_SWITCH` off, enables only
the logistics authorities whose URL/secret are configured, recreates API and
worker, and restores the backup automatically if `/ready` or the worker fails.
It refuses to change anything while Notification, Media (incl.
`VENDOR_MEDIA_PUBLIC_URL`), Wallet or Verification credentials are missing.

Ingress note: vendor document/asset upload grants point at
`<VENDOR_MEDIA_PUBLIC_URL>/api/v1/media-files/vendor-uploads/{ref}/content`,
which the public nginx must forward to media-file-service (today only
`customer-uploads/.../content` is forwarded and the rest of
`/api/v1/media-files/` returns 404).

## Manual vendor review (until an admin screen exists)

New businesses and branches stay in DRAFT until every verification check is
decided. Operations approves them by hand with `vendor-review`, which records
each decision through verification-compliance-service's signed decision API as
`admin-backoffice-service`, so history, idempotency and the callback that
activates the business in vendor-service all run normally. Do not edit the
verification or vendor database directly.

One-time server setup (already done on staging, 2026-10-17):

- `verification.env` lists `admin-backoffice-service` in both
  `VERIFICATION_COMPLIANCE_ALLOWED_CALLERS` and
  `VERIFICATION_COMPLIANCE_SERVICE_SECRETS`; its secret is also in
  `/opt/karrygo/verification-service/vendor-review.env` (mode 600). Apply a
  settings change with `bash /opt/karrygo/platform/bin/apply-env.sh verification-compliance-service`.
- The Linux build of `services/verification-compliance-service/cmd/vendor-review`
  is at `/opt/karrygo/verification-service/vendor-review.bin`. Rebuild with
  `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o vendor-review ./cmd/vendor-review`.

Daily use, on the server:

```sh
cd /opt/karrygo/verification-service
vr() { docker run --rm --network karrygo-net --env-file verification.env --env-file vendor-review.env \
  -e VENDOR_REVIEW_OPERATOR="$USER" -v "$PWD/vendor-review.bin:/vendor-review:ro" --entrypoint /vendor-review \
  "$(sed -n 's/^IMAGE=//p' release.env)" "$@"; }
vr list                                   # businesses waiting, oldest first
vr show    vb_...                         # every check for one business
vr approve vb_...                         # approve all its waiting checks
vr reject  vb_... "Upload a clear photo of your CAC certificate"
```

Check the documents a vendor uploaded before approving. Approval activates the
business and its branches within a minute; a rejection shows the vendor the
text you give, so write it as an instruction they can act on. Every command is
safe to repeat.
