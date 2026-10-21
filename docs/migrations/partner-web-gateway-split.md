# Partner Web / Public Gateway service split — deployment hold

Status: SOURCE MIGRATION / NO LIVE CUTOVER. Stacked on Secure Admin PR #12.
Old source: services/api-gateway (partner backend + public forwarding).
New owners:
- services/partner-web-service: backend for web/partner-web, legacy partner DB
  migrations 001-010, registration/login/OTP, API keys, quotes/deliveries,
  tracking, webhooks, billing, refunds and private dispatch/wallet callbacks.
- services/api-gateway: stateless public app traffic forwarding, correlation,
  timeouts, circuit breakers and composition-only customer overview.
- services/admin-backoffice-service: independent private HQ bridge, unchanged.

## Hard compatibility contract

Public URL api-gateway.staging.karryam.com stays unchanged. Partner API paths
/api/v1/partner/v1/* retain their methods, token/session/key semantics, original
request/response bodies, status codes, webhook signatures and idempotency.
Internal /api/v1/internal/callbacks/* and /api/v1/internal/partner/* belong
directly to partner-web-service on the private service network, NOT to public
api-gateway and NOT to Nginx. Never weaken the public internal/Admin denylist.
All other app paths stay owned by the same backend owners as before.

Existing partner PostgreSQL schema, migration history, key hashes, sessions,
billing records, replay records and Redis configuration are reused in place.
Do not rebaseline, truncate, copy unencrypted, rotate signing identity or
create new database records to make migration pass. The new service retains
legacy API_GATEWAY_* settings and outbound wallet caller api-gateway until
a separate signed wallet identity transition is approved.

## Source and CI checks before authorizing cutover

1. Verify both modules independently compile, vet and test, including with
   race instrumentation, real PostgreSQL, Redis and the old partner migrations.
2. Public gateway never owns DB, Redis, OAuth/JWT signing, partner API keys,
   pricing rules, wallets, invoices, callbacks or workers.
3. Partner-web service never mounts the platform reverse proxy or an Admin route.
4. Prove partner signup/OTP/session refresh, API key issuance/scopes/revocation,
   delivery quotes/create/list/track/cancel, webhook signatures and retry
   behavior, wallet HMAC identity and invoices against original contracts.
5. Prove public gateway forwards raw request bodies and legitimate auth headers,
   retains CORS/cookie boundaries, preserves existing mobile/frontend URL,
   passes websocket upgrades, hides privileged headers, and rejects Admin,
   /internal, double-encoded private URLs and partner internal callbacks.
6. Inventory all internal dispatch/wallet clients and any hardcoded
   api-gateway container alias; repoint callback traffic to the private
   partner-web-service:8080 origin before replacing the original gateway.
   Record real sender/receiver tests; absent owner evidence blocks release.
7. Validate Nginx/Compose and independent image/restart/rollback smoke tests.
8. Confirm no new service is exposed on a public host port. Only the
   api-gateway continues listening on host-loopback 127.0.0.1:4100.

## Staged operational cutover (manual approval required)

Both affected services are excluded from auto/all deployments. Before touching
live VPS, inspect current running compose/services, addresses, firewalls,
Docker aliases, nginx and monitored partner activity; obtain a quiet window,
DB backup/restore proof, immutable signed-off image SHAs, operator approval
and safe rollback instructions. Preserve existing gateway.env (mode 600):
create /opt/karrygo/partner-web-service/partner.env from its secrets using an
approved operator channel, never a repository commit. Provision
/opt/karrygo/api-gateway/public-gateway.env with only upstream URLs, CORS,
transport and timeout config (no database/partner credentials).

Ensure the old gateway's webhook, invoice/charge and Redis subscriber workers
have stopped before launching partner-web-service; do not run both active
workers against the same database. Partner-web-service must be the unique
worker owner. Provision the new service with no public ports; run migrations
in status mode first on the existing partner DB. Existing migration records
must be preserved. Test partner business routes and private internal callbacks
directly. Configure all public gateway canonical owners and the mandatory
private partner_web upstream. During a controlled cutover replace gateway
image on 127.0.0.1:4100 only after partner readiness and callback tests pass.

Keep Nginx site/admin deny blocks. Test customer, dispatch, taxi, hauling,
vendor, fleet, partner login/API-key/delivery/webhook/billing, websocket and
cookie/CORS end-to-end on the real staging network. Confirm callback producers
are sending directly to the partner service and that gateway public routes
never accept /internal. Validate payment amounts, no repeated charges and no
duplicate invoices or webhook events. Verify the public Nginx listener and
partner endpoints still work on the existing externally documented URLs.

Rollback: first stop the new partner workers before reactivating the original
all-in-one gateway container, so billing/webhook consumers do not run twice.
Restore the previously certified api-gateway image and original gateway.env
and network aliases, then revert any callback URL changes to the previous
internal recipient. Keep the DB additive schema/evidence intact; do not drop
migration records to force rollback. Verify platform and partner health.

## Blocked until separately certified

CI and source review do not prove the actual VPS network topology, partner
callbacks, data migration invariants, image routing or live integration.
No automatic migration, deployment or production switch is authorized.
This is a staged change intended for peer review and controlled cutover.
