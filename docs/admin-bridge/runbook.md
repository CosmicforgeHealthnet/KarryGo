# Private Admin bridge operations

Repository foundation only: no live HQ/VPS certification. Domain writes are
disabled in code. Never put this bridge behind the public API Gateway.

## Network setup and revocation

Before a live change, inventory existing Nginx sites, listeners, Docker NAT,
firewall rules, management access and customer/provider health. Obtain
authorization for the concrete network change. No firewall/tunnel change is
performed by this implementation.

Use a dedicated WireGuard peer or Tailscale tagged workload identity for Karryam
HQ. Do not reuse HealthNet, another product or workstation identities. HQ route
allowlist contains only bridge VPN /32, never a KarryGo/Docker subnet. Transport
ACL permits only the exact HQ identity to TCP 8110; deny DB, Redis, Docker, SSH
and owner services. Preserve a separately verified management path.

Compose attaches only Admin and its DB to `admin-private` (internal); neither is
on `karrygo-net`. PostgreSQL has no published port. API 8110 defaults to loopback.
Readiness 8116 is container-loopback only and is never published.

`deploy/platform/network/admin_policy.py` renders a candidate using supplied
`--vpn-interface`, `--hq-ip`, `--bridge-ip`; it never applies rules. It permits
the exact HQ peer/port, drops other bridge-port access, then drops other traffic
from that HQ VPN peer. Review existing firewall/NAT priorities, run
`nft --check --file <candidate>`, preserve a rollback, and obtain authorization
before applying. Never flush existing rules. The candidate supplements the
existing default-deny host firewall. Preflight rejects public/wildcard/link-local
or unsupported IPv6 binds. Only operator-approved private IPv4 or default
127.0.0.1 is accepted.

Private transport remains replaceable: mTLS, HMAC and staff proof are independent.
VPN/HQ outage affects only HQ access. Customer/provider services have no dependency
on this bridge, its DB or HQ authentication infrastructure.

## First deployment and secrets

1. Build the reviewed immutable image with existing `services/Dockerfile`,
   `SERVICE_DIR=admin-backoffice-service`. Pass the new Admin workflow and all
   existing required gates. Resolve failures, never bypass them.
2. Provision mode-600 `/opt/karrygo/admin-backoffice-service/admin.env`,
   `admin.migrate.env`, `db.env` from templates. `db.env` defines dedicated
   `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`; generate credentials outside
   Git. No domain database access. Provision separate migration-owner and runtime
   roles. Runtime must not be superuser, DB/schema owner or migration-role member.
3. Populate `secrets/server.crt`, `server.key`, `hq-client-ca.pem`, `trust.json`.
   App mounts only those files read-only. Inspect image numeric `cosmicforge` UID;
   set secret ownership to that UID, mode 600, directory searchable by that UID.
   Compose bind mounts preserve ownership; do not rely on secret UID remapping.
   No Docker socket or privileged host mounts. HQ must validate the bridge
   serverAuth certificate chain and expected DNS/IP SAN.
4. Fill the intentionally invalid blank trust template with approved issuer,
   audience, URI, policy version, ACRs, assertion/session limits, quota, key IDs
   and expirations. No production addresses, keys or financial thresholds are
   supplied. Policy reload requires restart. Test invalid configuration first.
5. Run tools-profile `migrate` before first app start, using only its migration
   env file; it mounts no TLS or HMAC keys. Migration uses one transaction and
   advisory lock. Apply `admin-db-grants.sql` with psql variable `runtime_role`.
   Remove schema CREATE and unwanted PUBLIC/default grants. Runtime gets no
   approval writes, principal/revocation administration, audit read/update/delete/
   truncate, DDL or domain privileges. Audit readers are separate approved roles.
   Role creation/grants are an operator bootstrap, not runtime permissions.
6. Start with `ADMIN_REMOTE_ENABLED=false`. Verify readiness and local negative
   tests before any remote enrollment. Keep the DB and all private ports closed.

`deploy-service.sh` requires a mode-600 `bridge.certification` listing the exact
reviewed immutable image before migration/start. Provision only after acceptance;
retain previously certified references for rollback. Automatic pushes and `all`
selectors exclude Admin. Select its exact key explicitly when certified.
Independent health wait, controlled restart, log rotation and rollback reuse the
existing framework. First deployment must bootstrap migration/runtime grants
before invoking the normal deploy cycle.

`ADMIN_PRIVATE_BIND_IP` may replace loopback only after approved VPN/firewall
acceptance. Other variables: `ADMIN_BACKOFFICE_DATABASE_URL`, `ADMIN_TLS_ADDR`,
`ADMIN_TLS_CERT_FILE`, `ADMIN_TLS_KEY_FILE`, `ADMIN_TLS_CLIENT_CA_FILE`,
`ADMIN_TRUST_FILE`, `ADMIN_REMOTE_ENABLED`. Never print values in logs/issues.

## Principal, key and staff lifecycle

Validate certificate issuance, clientAuth, exclusive workload custody and exact
URI SAN. Using an operator DB identity, enroll DER SHA-256 in
`admin_remote_principals` with fixed principal `cosmicforge-hq-karryam-admin`,
`karryam`, exact URI/audience, expiry no later than certificate expiry. There is
no remote enrollment API. Audit enrollment through the infrastructure change
process. Test wrong chain/URI/fingerprint/product before enabling reads.

Revoke by setting enrollment `revoked_at`, remove tunnel identity/ACL and rotate
compromised signing credentials. Enrollment is checked every request, including
existing TLS connections. CA/key rotation uses controlled deployment and bounded
overlap of independently verified identities. Follow the signing profile for
overlapping staff/workload key IDs. Expired keys fail per request.

Startup logs only key IDs and expiry timestamps. Configure existing monitoring
to alert before key/certificate expiry, failed readiness, denied requests and
`admin_audit_unavailable`. Live monitoring/alert delivery is an operator dependency,
not claimed by source-level tests.

HQ owns SSO/MFA/workforce roles. Bridge checks exact signed permission and scope.
Revoke opaque subjects/sessions in `admin_revocations(kind='staff'|'session')`.
Unavailable revocation storage fails closed. Never trust caller-supplied actors,
roles or performed_by fields. Metadata permissions are
`bridge.capabilities.read`, `bridge.health.read`, `bridge.operations.read`,
`bridge.operations.submit`, with signed market/organization/purpose. Operation
reads are original-staff only; discovery filters verified permissions.

Approval primitives reject self-approval, wrong action/digest/scope/policy,
expiration/revocation and R4 without incident reference. R3/R4 policies require
approved strong ACRs. `admin_approvals` stores immutable independent evidence;
there is no remote approval authoring or high-risk execution API. Per-action
approved policy and a certified owner adapter are prerequisites for future
writes. HQ approval cannot bypass an owner's state/version/business checks.
Evidence/media, exports and private support content remain blocked.

## Replay, unknown outcomes and suspicious requests

Nonce and issuer/JTI are consumed atomically; concurrency admits exactly one.
Replays yield 409 before operation storage/owner effects. Preserve correlation
and audit records, revoke affected subject/session/workload, investigate HQ
signing/network access. Never delete active claims to make a request pass.

Idempotency is durable. Changed bytes/staff/command ID with a key, or a reused
command ID with another key, yields 409. Unsupported commands are REJECTED and
return 503 `capability_blocked`, `retryable:false`, with operation ID. No owner
action occurred. Read that scoped record if necessary. Requested version is
separate from nullable observed source version.

Future owner timeout must persist UNKNOWN_OUTCOME before reconciliation. Never
automatically repeat an uncertain irreversible request. Use the owner's real
authoritative read-back contract and preserve decision evidence before a safe
transition. No automatic retry worker is installed here.

Every authorized/denied request attempts an append-only minimized audit record.
No tokens, bodies, free-text reasons, keys, SQL errors or private evidence are
logged. Use opaque purpose/case references. Audit failure suppresses data responses
and emits sanitized host fallback logs with correlation. Forward host logs to
independently controlled storage: a failed database cannot durably audit its own
outage, and the application does not claim otherwise.

## Retention, rollback, kill switch and outages

Organization-approved operation/audit/approval retention is still required; no
period is invented. Runtime cannot edit/delete evidence. Use an independent
custodian for archive and backup/restore verification. Triggers reject routine
UPDATE/DELETE, but superusers can disable them; independent storage/operator
separation remains necessary. Do not truncate evidence. Down migration explicitly
fails rather than destroying audit/approval data.

An operator job may remove expired `admin_security_claims` and old rate windows
after verified clock synchronization. Claims last 11 minutes, beyond all accepted
HMAC/JWT windows. Never delete active claims or durable idempotency records during
cleanup. Alert on growth; no unapproved deletion job is automatically installed.

Rollback with `rollback.sh admin-backoffice-service <certified-image>`, retaining
additive schema and evidence. Restore approved trust if needed, leave remote
ingress disabled until revalidated. Back up the dedicated DB and test restore
before live acceptance.

Writes have no enable flag. For immediate remote isolation stop only the Admin
app/revoke its ACL. Persistent kill switch: `ADMIN_REMOTE_ENABLED=false`, recreate
only that app. Never stop shared networks or customer/provider containers to
disable HQ. On HQ outage leave product services running; restore transport and
identity/audit stores, verify key/session validity, then resume reads with fresh
nonce/JTI. Do not replay a backlog of irreversible commands.

## Live acceptance still required

Record real inter-VPS evidence: public Admin encoding/method/upgrade variants
denied; legitimate product routes healthy; no HQ access to public 8110, DB,
Redis, Docker, SSH or owner APIs; only the exact workload completes mTLS; wrong
product/scope/permission/MFA/replay fails before effects; revocation/rotation work;
HQ outage leaves product traffic healthy; monitoring, independent audit,
backup/restore and rollback are verified. Review the missing reference PDF, real
HQ assertion producer and approved policy, and pin HQ to the reviewed contract
SHA. Repository tests are not production or inter-VPS certification.
