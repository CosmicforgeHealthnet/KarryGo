# Admin bridge exposure prevention (Sections 20 and 29)

The private Admin bridge (`admin-backoffice-service`, mTLS on 8110) and every
service-to-service route must never be reachable from the internet. Four
independent layers enforce that; each is validated in CI where it can be, and
the rest has an operator procedure below. Nothing in this directory applies a
firewall rule, restarts a service or deploys anything by itself.

## Topology

```
internet ──443──> nginx (public vhost) ──127.0.0.1:4100──> api-gateway ──> public owners
                    │ denies /api/v1/admin, /internal,           │ no admin route, no admin upstream,
                    │ */internal/*, ../ before any proxy         │ multi-layer decoded internal deny,
                    │                                            │ websocket upgrades only on an allowlist
HQ VPN peer ──wg──> ADMIN_PRIVATE_BIND_IP:8110 ──> admin app ──admin-private (internal)──> admin DB
                    (default 127.0.0.1)            │
                                                   └─ admin-ingress: non-internal bridge used only to
                                                      publish 8110; no NAT egress, no ICC, private
                                                      default binding
```

* **nginx** (`deploy/platform/nginx/api-gateway.staging.karryam.com.conf`, and the
  include template `deploy/customer-public-ingress.locations.conf`): explicit,
  case-insensitive `return 404` for `^/api/v1/admin(/|$)`, `^/internal(/|$)` and
  `(^|/)(internal|..)(/|$)`, written before every `proxy_pass` location. No
  `^~` prefix locations, so the regex denies always win.
* **api-gateway** (`services/api-gateway/internal/features/routing`): the admin
  upstream is never registered, `IsInternalPath` checks every decoding layer
  (case, `%2F`/`%252F`, `%2e%2e`, backslashes, dot segments, double slashes),
  forged platform headers are stripped, the client's Host is never relayed as
  `X-Forwarded-Host`, and a protocol switch (`Upgrade:` of any kind, or a
  `Connection: upgrade` token) is carried only as `GET` + `Upgrade: websocket`
  to exactly these endpoints, owned by the matching upstream:
  * `/api/v1/notifications/ws` (notification-service)
  * `/api/v1/customer/dispatch/bookings/{id}/track` (dispatch)
  * `/api/v1/hauling/customer/bookings/{id}/track` (hauling)

  Anything else answers 404 before any upstream (or breaker) is touched.
  Note: the staging nginx only sets the upgrade headers for the notifications
  socket; the two `/track` sockets are allowlisted at the gateway but nginx
  does not yet carry their upgrade (pre-existing, unchanged here).
* **Compose** (`deploy/platform/compose/admin-backoffice-service.yml`): Docker
  cannot publish a port of a container attached only to an `internal: true`
  network, so the app joins a dedicated `admin-ingress` bridge
  (masquerade off, ICC off, `host_binding_ipv4` = the private bind IP) plus the
  internal `admin-private` network shared with PostgreSQL and the migration
  tool. The DB is never published and never on `admin-ingress`. Every service
  runs read-only, `cap_drop: [ALL]`, `no-new-privileges`, AppArmor
  `docker-default`, Docker's default seccomp profile (never `unconfined`), and
  pids/memory/CPU limits; the app and migrator run as the image's non-root
  `cosmicforge` uid/gid `100:101`; the DB keeps only CHOWN, DAC_OVERRIDE,
  FOWNER, SETGID, SETUID. Secret files must be owned by uid 100.
  If `172.31.110.0/28` collides on the host, set `ADMIN_INGRESS_SUBNET`.
* **Host firewall**: `deploy/platform/bin/close-public-ports.sh` drops,
  in Docker's `DOCKER-USER` chain and only for packets arriving on the public
  interface in the ORIGINAL conntrack direction, the host ports 4100, 4101,
  4103, 8080, 8110, 8101-8116, 5432-5451 and 6379-6391. 22/80/443 can never be
  listed (the script refuses) and outbound traffic is never matched. It is a
  **dry run by default**; `--apply` inserts the rules idempotently and installs a
  boot unit. `deploy/platform/network/admin_policy.py --public-interface <if>`
  renders the same list as an nftables candidate (forward + input hooks,
  policy accept, no output chain); `--vpn-interface/--hq-ip/--bridge-ip`
  renders the existing HQ-peer ACL candidate.

> The VPS deployment guide's step that runs `close-public-ports.sh` with no
> arguments now only prints the plan; use `--apply` to enforce it.

## What CI validates

| Workflow | Check |
| --- | --- |
| partner-gateway-split, admin-bridge | `go vet` + `go test -race` of api-gateway: `TestUpgradeIsOnlyCarriedToAllowlistedWebsocketEndpoints`, `TestWebsocketAllowlistMatchesOnlyExactShapes`, `TestAdminAndInternalBypassMatrix` (uppercase, `%2F`/`%252F`, `%2e%2e`, trailing/double slashes, GET/HEAD/OPTIONS/TRACE/CONNECT/..., websocket/Host/X-Forwarded-Host/X-Original-URL headers), `TestAdminPrefixConfusion`, `TestAbsoluteFormAndHostCannotSelectAnUpstream`, plus the existing `TestPublicAdminIsolation` |
| deployment-safety | `scripts/release/test_admin_exposure.py`: nginx denies precede every proxy, admin compose hardening + topology, no compose publishes an internal port on a non-private address, close-public-ports covers 8110 and never 22/80/443, dry-run default touches no firewall tool, nft candidates never drop 22/80/443 or outbound (and `nft --check` when run as root) |
| admin-bridge | `validate.py` (compose topology/hardening, nginx order), `test_network.py` (bind + both nft candidates), `docker compose ... config --quiet`, `nginx -t` with disposable certificates |

## What needs real infrastructure (operators; not executed from the repo)

Run only with authorization for the concrete host, keep a rollback and a
separately verified management (SSH) path.

```bash
# 1. Evidence before any change (on the host, read-only):
sudo bash scripts/admin-bridge/exposure/capture_host_evidence.sh --out-dir /var/tmp/karrygo-evidence

# 2. Review, then enforce the DOCKER-USER block:
sudo bash /opt/karrygo/platform/bin/close-public-ports.sh            # dry run
sudo bash /opt/karrygo/platform/bin/close-public-ports.sh --apply
sudo bash /opt/karrygo/platform/bin/close-public-ports.sh --check

# 3. Optional nftables candidate (review, check, never flush existing rules):
python3 deploy/platform/network/admin_policy.py --public-interface eth0 > /tmp/exposure.nft
sudo nft --check --file /tmp/exposure.nft

# 4. Recreate the admin bridge with the hardened compose (needs the certified IMAGE):
IMAGE=<certified-image> docker compose -f deploy/platform/compose/admin-backoffice-service.yml up -d

# 5. From OUTSIDE the server, against the authorized staging target:
bash scripts/admin-bridge/exposure/external_scan.sh --i-am-authorized \
  --base-url https://api-gateway.staging.karryam.com --host api-gateway.staging.karryam.com \
  --out exposure-scan.json

# 6. Evidence after the change; diff with step 1.
sudo bash scripts/admin-bridge/exposure/capture_host_evidence.sh --out-dir /var/tmp/karrygo-evidence
```

`external_scan.sh` exits non-zero if any admin/internal probe answers anything
but 404 (TRACE/CONNECT may be refused at the protocol level with 400/405/501),
if any protected TCP port accepts a connection, or if the target is unreachable.
