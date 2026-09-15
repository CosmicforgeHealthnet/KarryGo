# FINAL_DEPLOYED_STAGING_EVIDENCE

Phase baseline `9ebad225ea9ec25397778a4dc8d3ff842f2938a3`.
No secret value, hostname, key or credential appears in this document.

## 0. The short version

**No deployed staging certification was possible, and the reason is specific
and verifiable rather than a matter of opinion.** This document records what
the deployment system actually is, what it actually does today, and exactly
which operator actions are missing. Every claim below is drawn from the
repository or from GitHub Actions run history, not from assumption.

## 1. The real deployment system

Discovered by reading `.github/workflows/` and `deploy/`, not assumed:

```
GitHub Actions  →  build image  →  ghcr.io  →  SSH to a VPS  →  docker compose
```

- Secrets `VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY` are GitHub Actions repository
  secrets. They are not available to this session and must not be.
- Compose state lives on the VPS under `/opt/karrygo/<service>`.
- `deploy/vendor-service/docker-compose.staging.yml` is the only compose file
  in the repository. The api-gateway and media-file-service compose files exist
  only on the VPS.

**`infra/docker-compose.yml` defines nothing.** It is 382 lines, of which
**0 are active** — every line is commented out. It is a dead file, not a local
stack. (This corrects the note in `CLAUDE.md` claiming it does not exist: it
exists, and is inert. Either way it cannot be used to stand a stack up.)

## 2. Which services can deploy at all

| Service | Deploy workflow | Can reach staging today |
|---|---|---|
| api-gateway | `deploy-api-gateway.yml` | yes |
| media-file-service | `deploy-media-file-service.yml` | yes |
| vendor-service | `deploy-vendor-service.yml` | **blocked — see §3** |
| fleet-partner-service | **none** | no |
| payment-wallet-service | **none** | no |
| driver-taxi-service | **none** | no |
| driver-dispatch-delivery-service | **none** | no |
| driver-hauling-service | **none** | no |
| notification-service | **none** | no |
| verification-compliance-service | **none** | no |
| support-dispute-service | **none** | no |
| customer-service | **none** | no |

**Three of twelve services have a deployment path, and not one of them is on
the corporate money path.** Section 23's cross-service journeys need Fleet,
Wallet and at least one execution service. None of those can be deployed by
any mechanism this repository defines.

This is the finding that decides the phase. It is not "we ran out of time";
there is no workflow to run.

## 3. Vendor deploy: failing, and why

`Deploy vendor-service` has failed on every recent commit, including
`3007d6d4` and `9ebad225`. The job builds and pushes the image successfully,
SSHes to the VPS successfully, and then stops at its own precondition:

```
out: ERROR: /opt/karrygo/vendor-service/vendor.env is missing
Process exited with status 1
```

So: **the pipeline works and the host is reachable; the environment file has
never been placed on it.** `vendor.env` holds staging database URLs, Redis,
signing secrets and the internal caller map. It must be created by an operator,
mode 600, through the approved secret manager. It cannot be created from here,
and inventing its contents would be worse than reporting the gap.

`deploy/vendor-service/README.md` already documents this as a remaining
operator action, and says plainly that the repository and workstation "do not
identify or provide access to the VPS reverse proxy, PostgreSQL provider,
monitoring system, DNS provider, or TLS control plane." That statement is still
accurate.

## 3b. Confirmed again at this baseline

Re-observed on `4d7aebdf86c59af3d78d58de4d4af2cc1c2eebe5` after this phase's
changes, so the blocker is known to be unchanged rather than assumed to be:

| Workflow | Result at `4d7aebd` |
|---|---|
| Section 31 mandatory acceptance | **success** (it failed at `9ebad225`; that regression is fixed) |
| Deploy api-gateway | success — now stamping the commit into the binary |
| Deploy media-file-service | success — now stamping the commit into the binary |
| Vendor Sections 46-48 gates | success |
| Deploy vendor-service | **failure — `ERROR: /opt/karrygo/vendor-service/vendor.env is missing`** |

The Vendor image still builds and pushes with the new build args and still
reaches the SSH rollout before stopping, so the added stamping did not break
the build; the blocker is exactly where it was.

## 4. Why api-gateway and media-file-service "succeeding" is not evidence

Both green deploy jobs end with:

```
cd /opt/karrygo/<service>
docker-compose pull
docker-compose up -d
```

There is no health gate, no readiness wait and no version check. A green run
means *the SSH command returned 0*, not that the service is healthy, not that
it is serving, and not that it is running the commit that was just built. The
Vendor workflow is the only one that actually waits for health — and it is the
one that cannot run.

So even for the two deployable services, a green workflow is not OPS-002
evidence. After this change it could become evidence, because the commit is
now stamped into the binary and readable from `/health` (§5).

## 5. FIX-001 / OPS-002 — closed in code, verified on a running binary

Build metadata is now stamped by the linker and surfaced on the existing
`serviceapp` surfaces — no second metadata system was created.

Verified by building `driver-hauling-service` from `9ebad225` with the same
`-ldflags` the Dockerfiles now pass and booting it:

```
startup log : {"build_time":"2026-09-15T05:29:41Z",
               "commit_sha":"9ebad225ea9ec25397778a4dc8d3ff842f2938a3",
               "environment":"staging","event":"service_build",
               "service":"hauling-service","stamped":true,"version":"staging-verify"}

GET /health : {"commit_sha":"9ebad225ea9ec25397778a4dc8d3ff842f2938a3",
               "service":"hauling-service","status":"ok","success":true}

GET /api/v1/hauling/meta :
              {"api_base":"/api/v1/hauling","build_time":"2026-09-15T05:29:41Z",
               "commit_sha":"9ebad225ea9ec25397778a4dc8d3ff842f2938a3",
               "environment":"staging","service":"hauling-service",
               "version":"staging-verify", ...}
```

Three properties that matter:

1. **The SHA comes from the build, never the tree.** `commitSHA` is a linker
   variable; the Dockerfiles take it as `BUILD_COMMIT_SHA`, and the workflows
   pass `${{ github.sha }}`. A commit written into source would describe the
   commit someone typed, which is not the question OPS-002 asks.
2. **An unstamped binary says `unknown`, not empty.** A missing key and an
   empty value are indistinguishable to a script, so "the pipeline did not
   stamp this" is reported explicitly and `Stamped()` separates *wrong commit*
   from *cannot tell me which commit*. Those have different fixes.
3. **The commit survives log redaction.** The platform logger redacts by
   keyword, and a field name that tripped one would have turned the deployed-
   version log line into `[REDACTED]`. There is a test pinning that.

`/health` carries the commit as well as `/meta` because a health probe is the
one endpoint a deployment check is certain to reach, and "is it up" and "is it
the build I shipped" are asked at the same moment. A public commit on a public
branch is not a secret.

## 6. OPS-001 … OPS-014

No gate below is marked PASS. Not one of them can be evidenced without the
environment described in §2 and §3.

| ID | Gate | Status | Why |
|---|---|---|---|
| OPS-001 | Ingress / TLS | **NOT RUN — NO ENVIRONMENT** | No hostname is known to this session; it is a repository secret. Vendor's compose binds `127.0.0.1:8112` only, so even with the host there is no public listener to test. The reverse proxy is not in the repository. |
| OPS-002 | Deployed version | **CODE COMPLETE — NOT RUN** | The mechanism is built and verified on a running binary (§5). What remains is deploying and querying it. |
| OPS-003 | Database backups | **NOT RUN — NO ENVIRONMENT** | The PostgreSQL provider is not identified anywhere in the repository. Operational state cannot be inspected from here, and inspecting IaC would not satisfy the gate anyway — there is no IaC either. |
| OPS-004 | Restore drill | **NOT RUN — NO ENVIRONMENT** | Depends on OPS-003. |
| OPS-005 | Log visibility | **NOT RUN — NO ENVIRONMENT** | Correlation propagation is certified in-process; proving it across deployed services needs those services deployed. No log aggregation system is identified. |
| OPS-006 | Metrics visibility | **NOT RUN — NO ENVIRONMENT** | `/metrics` exists and was verified live locally, which the section explicitly says is *not* sufficient. No monitoring backend is identified. |
| OPS-007 | Alert delivery | **NOT RUN — NO ENVIRONMENT** | No alerting destination is identified. |
| OPS-008 | Service credentials | **NOT RUN — NO ENVIRONMENT** | Caller isolation is certified locally (401/403 matrix, previous phase). Deployed credentials live in `vendor.env` and its siblings, which do not exist yet. |
| OPS-009 | Supporting service reachability | **NOT RUN — NO ENVIRONMENT** | Notification, Media, Verification and Support have no deploy workflow except Media. |
| OPS-010 | Wallet / payment dependencies | **NOT RUN — NO ENVIRONMENT** | Wallet has no deploy workflow. No sandbox Paystack credentials are available, and none may be invented. |
| OPS-011 | Deployed black-box suite | **NOT RUN — NO ENVIRONMENT** | Requires staging URLs. Running against localhost would be exactly the mislabelling the section forbids. |
| OPS-012 | Failure injection | **NOT RUN — NO ENVIRONMENT** | Deliberately not simulated locally and reported as deployed evidence. |
| OPS-013 | Bounded load | **NOT RUN — NO ENVIRONMENT** (one path measured locally) | `LOAD-005` is measured against a real database at the usecase boundary and reported in the certification matrix. It is not HTTP-level staging load and is not labelled as such. |
| OPS-014 | Runbook | **UPDATED — partially unverified** | `FINAL_DEPLOYMENT_READINESS_CHECKLIST.md` now records the real pipeline, the real migration runners and the real blocker. Commands that were not executed are marked as such. |

## 7. What an operator must do to unblock this

In order. None of it is a code change; all of it needs VPS or provider access.

1. Create `/opt/karrygo/vendor-service/vendor.env`, mode 600, populated from
   the approved secret manager. This alone unblocks the one service whose
   deploy workflow is health-gated.
2. Add deploy workflows (or compose definitions on the VPS) for
   fleet-partner-service, payment-wallet-service, driver-taxi-service,
   driver-dispatch-delivery-service, driver-hauling-service,
   notification-service, verification-compliance-service and
   support-dispute-service. Without these, **no cross-service journey in
   Section 23 can ever be certified**, whatever else is done.
3. Provision staging PostgreSQL and Redis per service, and record the backup
   configuration so OPS-003 and OPS-004 become answerable.
4. Publish the reverse-proxy hostname and TLS configuration so OPS-001 becomes
   answerable. Forward only `/api/v1/...`; never `/internal/`.
5. Identify the monitoring and alerting destinations so OPS-006 and OPS-007
   become answerable.
6. Add `support-dispute-service` to Fleet's `PROVIDER_MEMBERSHIP_ALLOWED_CALLERS`
   deployment configuration and issue it a Fleet HMAC secret, so the new
   organization snapshot on safety cases resolves. The code default already
   lists it; a deployment that overrides the variable must include it.
7. Then re-run this phase's Part C, E, F and G against the deployed
   environment.

## 8. Claims this document does not make

- It does not claim any service is deployed.
- It does not claim TLS, backups, alerting or monitoring exist.
- It does not treat a green `docker-compose up -d` as a health check.
- It does not present local evidence as deployed evidence anywhere.
