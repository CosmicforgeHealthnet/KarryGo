# Final production GO certification

Certification date: 2026-09-11. Environments available to this audit: local isolated PostgreSQL/Redis and read-only GitHub repository metadata. GitHub identifies a VPS-over-SSH deployment convention and the names of its three generic VPS secrets, but no VPS shell, Vendor staging environment file, public hostname, PostgreSQL provider, monitoring control plane, or downstream staging credentials were available.

SECTION 46: **COMPLETE**  
SECTION 47: **PARTIAL**  
SECTION 48: **PARTIAL**  
REPOSITORY IMPLEMENTATION: **COMPLETE**  
STAGING CERTIFICATION: **BLOCKED**  
PRODUCTION GO: **BLOCKED**

## Evidence summary

The actual backend deployment convention is GitHub Actions building immutable GHCR images and deploying over SSH to Docker Compose under `/opt/karrygo` on a VPS. The partner-auth API Gateway is not the Vendor ingress. Repository deployment now selects direct Vendor ingress through the VPS's existing reverse proxy/load balancer, binds the API container only to `127.0.0.1:8112`, and publishes no Worker port. The VPS proxy/DNS/TLS configuration is still inaccessible, so public staging ingress and network-level internal-route rejection cannot be certified.

Repository-controlled deployment work is complete: `services/vendor-service/Dockerfile` packages separate service and migration binaries plus the migrations; `deploy/vendor-service/docker-compose.staging.yml` defines distinct API and Worker units; and `deploy-vendor-service.yml` enforces migration -> API readiness -> Worker rollout. The packaged migration job acquired its advisory lock, ran idempotently against isolated PostgreSQL, and verified all 116 migration records. The image and Compose configuration both built/validated locally. These are deployment definitions, not proof that the uncommitted workflow ran on the staging VPS.

Local process probes used an isolated database at migration tip 116. API mode returned 200 from `/health`, `/ready`, and `/api/v1/vendor/meta`, exposed no worker leases, and rejected an unsigned internal request with 401. Worker mode initially exposed a nil-server panic; that defect was fixed in `shared/go/serviceapp`. The corrected worker stayed running, opened no listener, configured a real retention worker, and stopped on interrupt. A bad-database worker probe then found that startup only logged the failure; worker startup now performs a mandatory PostgreSQL ping and exits nonzero when it fails. `all` mode returned 200 for health and readiness. The probe database contained no queued work, so actual backlog claim/restart convergence remains covered by isolated repository tests rather than claimed as live staging evidence.

An empty-database application migration attempt timed out at migration 78 under the fixed 30-second application context. The purpose-built isolated runner still replayed all 116 migrations and passed 67/67 packages. Production deployment must use the migration job/runner rather than rely on API startup to bootstrap an empty database until that bounded migration policy is deliberately reviewed.

No staging endpoint, HMAC credential, caller allowlist, backup provider, monitoring deployment, alert deployment, or load-test identity was available. Therefore no live Wallet, Dispatch, Hauling, Verification, Media, Notification, Support, OTP/QR, failure-injection, restore, or staging SLO result is claimed.

## Final production GO blocker register

| Blocker | Owner | Exact action | Environment | Dependency | Validation | Pass condition | Risk if ignored | Blocking |
|---|---|---|---|---|---|---|---|---|
| Vendor public ingress | Platform/DevOps | On the VPS run the three ingress-discovery commands in `deploy/vendor-service/README.md` section 2; update the discovered existing proxy to forward only `/api/v1/vendor/` and `/api/v1/commerce/` to loopback port 8112; run the documented curl and `openssl s_client` matrix | staging then production | VPS/DNS/TLS access | retained public-route headers, three ingress-level private-route rejections, valid TLS chain | public routes reach Vendor; internal prefixes never reach Vendor; required headers/status preserved | outage or private-route exposure | YES |
| Separate deployables | Platform/DevOps | Install `/opt/karrygo/vendor-service/vendor.env` mode 600, review/merge these files, then run `gh workflow run deploy-vendor-service.yml --ref staging` and the documented `docker inspect`/`docker port` checks | staging | VPS shell and reviewed commit | workflow run plus VPS container inspection | migration succeeds; API healthy; Worker running with zero published ports | duplicate work or unavailable recovery | YES |
| Backups | Database/DevOps | Run the non-secret `psql`/Docker provider-discovery commands in runbook section 3; enable that identified provider's native encrypted backup, retention, access policy and failure notification; record approved RPO/RTO and latest success | staging/production | database provider access | provider policy/job metadata | successful encrypted backup inside approved RPO with tested access | unrecoverable authoritative data | YES |
| Restore drill | Database/DevOps + Vendor | Restore the newest backup only to `<source>_restore_drill_<UTC_DATE>`; keep writes/workers off; run the documented migration/outbox/inbox/job queries and representative domain integrity queries | isolated staging | valid backup and isolated target | retained restore log and read-only integrity output | tip 116 and representative authoritative/durable state preserved | recovery plan unproven | YES |
| Post-restore reconciliation | Vendor + Wallet/logistics/Verification owners | With writes still off, read back stored Wallet operation IDs and logistics mission IDs, refresh Verification, rebuild Search beside live, resume consumers then safe workers, and record backlog convergence/readiness before enabling writes | restored staging | live authorities | authority read-backs and backlog/readiness evidence | projections match authorities before writes resume | duplicate money/missions or stale verification | YES |
| Monitoring and alerts | SRE | Run runbook section 4 discovery commands on the VPS; connect API/Worker to the discovered stack; install the mandatory dashboard/rules; trigger and clear PostgreSQL, dead-letter, blocked-worker and dependency-outage examples with runbook links | staging | monitoring control plane | dashboard export/screenshots and rule evaluations | all signals visible; representative alerts fire, link, and clear | silent operational failure | YES |
| Credentials/allowlists | Service owners | Populate `vendor.env` through the approved secret manager; record URL/secret presence, HMAC v2 version, caller ID and remote allowlist for each named service; run authorized and wrong-caller probes before enabling each flag | staging | downstream service owners | redacted presence matrix and two-sided probes | intended caller succeeds; wrong caller fails | integration outage or unauthorized access | YES |
| Live multi-service black-box | QA + service owners | After environment proof, run the prompt's Wallet, Dispatch, Hauling, Verification, Media, Notification, Support and OTP matrices with test funds/missions; retain redacted operation IDs, replay results, authority read-backs and state transitions | staging | healthy isolated services and test accounts | final black-box matrix | intended flows pass; wrong callers fail; replay produces no duplicate money/mission; no insecure fallback | economic/custody corruption | YES |
| Controlled failure injection | QA + service owners | Prove every target is staging, then inject the seven bounded Wallet/Dispatch/Hauling/Verification/Notification/OTP dependency failures; retain original operation IDs, UNKNOWN/read-back results, durable recovery and recovered state | staging | approved fault controls | final fault-injection report | UNKNOWN stays UNKNOWN until read-back; no duplicate effect or invalid transition; secure completion stays blocked | duplicate effects or insecure completion | YES |
| Bounded load and SLOs | Performance/SRE | Attach proof every endpoint/DB/account is non-production, seed bounded fixtures, then run the nine scenarios for 15 minutes after warm-up at at most 25 clients and export all required latency/resource/backlog fields | isolated staging | representative deployment | signed staging measurement report | mandatory targets met or approved exception recorded | unknown capacity/saturation | YES |

## Required 68-point report

1. Vendor ingress architecture: direct Vendor Service ingress through the existing VPS proxy is selected; live proxy authority is unverified.
2. API Gateway: no Vendor mapping; future scaffold.
3. Public ingress: BLOCKED—no deployed endpoint evidence.
4. Internal isolation: local unsigned route returned 401; public ingress isolation unverified.
5. API mode: local PASS—listener/readiness/public metadata, zero worker leases; Compose definition binds loopback only.
6. Worker mode: local PASS after two fixes—no listener and PostgreSQL fail-closed startup; Compose publishes no port.
7. All mode: local PASS—health/readiness 200.
8. PostgreSQL readiness: local PASS; worker startup now enforces ping.
9. Backup system: BLOCKED—provider-discovery commands supplied; provider identity still unavailable.
10. Backup encryption: BLOCKED—none evidenced.
11. RPO: BLOCKED—not configured/evidenced.
12. RTO: BLOCKED—not configured/evidenced.
13. Latest backup: BLOCKED—no timestamp available.
14. Restore drill: NOT EXECUTED.
15. Restored migration state: NOT EXECUTED; fresh replay independently reached 116.
16. Restored outbox/inbox/jobs: NOT EXECUTED.
17. Wallet post-restore reconciliation: NOT EXECUTED.
18. Logistics post-restore reconciliation: NOT EXECUTED.
19. Verification post-restore reconciliation: NOT EXECUTED.
20. Search rebuild after restore: NOT EXECUTED.
21. Monitoring deployment: BLOCKED.
22. Alert deployment: BLOCKED.
23. Alert synthetic test: NOT EXECUTED.
24. Staging credentials: only generic GitHub VPS SSH secret names exist; Vendor/downstream values remain absent and were not read.
25. Service allowlists: unavailable for verification.
26. Wallet black-box: NOT EXECUTED.
27. Dispatch black-box: NOT EXECUTED.
28. Hauling black-box: NOT EXECUTED.
29. Taxi Goods: DEPENDENCY_GATED; staging enablement unavailable.
30. Verification black-box: NOT EXECUTED.
31. Media black-box: NOT EXECUTED.
32. Notification black-box: NOT EXECUTED.
33. Support black-box: NOT EXECUTED.
34. OTP/QR: repository contracts PASS; live staging NOT EXECUTED.
35. Failure injection: repository tests PASS; live staging NOT EXECUTED.
36. Duplicate-money protection: repository tests PASS; live Wallet proof missing.
37. Duplicate-mission protection: repository tests PASS; live logistics proof missing.
38. UNKNOWN read-back: repository tests PASS; live authority proof missing.
39. Provider eligibility: repository contract evidence retained; live marketplace run missing.
40. Staging load scenarios: 0/9 executed.
41. Discovery p50/p95/p99: NOT MEASURED.
42. Checkout quote p50/p95/p99: NOT MEASURED.
43. Order create p50/p95/p99: NOT MEASURED.
44. Payment orchestration p50/p95/p99: NOT MEASURED.
45. Fulfilment quote p50/p95/p99: NOT MEASURED.
46. Throughput/error rates: NOT MEASURED.
47. Worker backlog catch-up: repository tests PASS; staging NOT MEASURED.
48. SLO evaluation: BLOCKED by absent measurements.
49. Security scan: PASS locally.
50. Secret scan: PASS locally; no values printed.
51. IDOR/tenant tests: PASS in isolated Vendor suite.
52. Section 43: PASS—857 routes, 867 declarations, 857 unique operations, 10 approved duplicates, zero gaps.
53. Section 44: PASS—39 canonical events, zero parity gap.
54. Section 45: PASS—27 requirements; catalogue valid.
55. Section 46: PASS—56 controls/equivalents, modes coherent, guarantee disabled.
56. Vendor isolated regression: PASS—67/67 on fresh databases.
57. Cross-service regression: repository-side contracts PASS; deploy bundle PASS locally; live staging BLOCKED.
58. `go test`: PASS.
59. `go vet`: PASS.
60. `go build`: PASS.
61. Changed-file `gofmt`: PASS.
62. `git diff --check`: PASS.
63. Routes changed in this final phase: 0.
64. Migrations added: 0.
65. Historical migrations changed: 0.
66. Frontend changed: NO.
67. Remaining blockers: all ten rows in the blocker register require VPS/provider/staging operator access; their repository definitions and exact commands now exist.
68. Final decision: **PRODUCTION GO: BLOCKED**.

## Authority statement

Payment Wallet authority is preserved; refund and settlement authority remain Payment Wallet. Media File, Customer, Verification, Notification delivery, Dispatch, Hauling, Taxi Goods, VendorMission execution, and OTP/QR authorities are preserved. Search and analytics/intelligence are projections only. Vendor PostgreSQL is authoritative. Vendor Redis is non-authoritative and rebuildable. Pharmacy/medicine is out of scope. Order Guarantee is `FUTURE_DISABLED`.
