# Sections 2–5 audit (2026-10-18)

> **Historical (superseded 26 October 2026).** The baseline gap matrix below reflects 18 October. Current gaps and blockers: `sections-16-38-final-report.md` §E and `a-z-final-capability-traceability.json` (`not_shipped`).

Fetched `origin/staging` before editing. HEAD and previously audited commit are
both `1f4ff09380f6e9bf74a9f7cc6dbfaab9e85efe95`; the intervening diff is empty.
Working tree was clean. Implementation branch: `codex/secure-admin-sections-2-5`.
Only the pasted implementation brief was supplied; the referenced PDF was not
present. PDF-specific parity remains an external verification dependency.

## Source-backed baseline gap matrix

| Requirement | Baseline | Evidence / action |
| --- | --- | --- |
| Existing Admin service and feature organization | EXISTS | `services/admin-backoffice-service/internal/features/{operations,admin_users,dashboards,moderation,user_actions}` are documentation/scaffolds; reuse operations responsibility |
| Initialized private application | MISSING | `cmd/main.go` calls shared bootstrap with public `/api/v1/admin`, advertised unimplemented capabilities; shared bootstrap installs browser CORS |
| HMAC v2 | EXISTS | `shared/go/serviceauth/serviceauth.go`, compatibility and tamper tests; preserve bytes and caller identity |
| Durable receiver replay | PARTIAL | HMAC verifier checks timestamp/signature, not nonce storage. Domain-specific `shared/go/custodyops/repository.go` uses PostgreSQL unique nonce claims; reuse that atomic pattern locally |
| Public Admin denial | MISSING | `routing/models/routes.go` DefaultRoutes includes Admin; `proxy.go` accepts configured Admin origin; Nginx lacks explicit Admin denial |
| Other internal denial/public routes | EXISTS | Gateway edge tests and `IsInternalPath`, longest-prefix public routing; extend without replacing |
| Private deployment | MISSING | Admin absent from `deploy/platform/services.tsv`; shared non-root Dockerfile, capability-drop/log rotation and rollback scripts exist |
| Workload mTLS, remote identity/revocation | MISSING | No remote authentication in Admin scaffold |
| Staff assertion, scoped policy, approvals | MISSING | No executable features in Admin |
| Bridge-owned persistence and audit | MISSING | No Admin migrations; use dedicated PostgreSQL, no domain tables |
| Actual owner contracts | EXISTS | Analytics `/api/v1/analytics/network/mission-success` requires HMAC Admin caller (`networkanalytics/http/handler.go`). Verification `assetcapability`, Support admin routes already own decisions. Never duplicate these decisions |
| Existing typed advisory client | EXISTS | `shared/go/networkanalytics/client.go` implements readiness, provider-quality, supply and forecast advisory reads. It has no mission-success dashboard method; the Admin client adds only that distinct fixed contract and reuses shared authority validation and HMAC signing |
| Scoped owner adapters | PARTIAL | Analytics aggregate does not accept market/organization authorization filters. Adapter foundation can be tested, remote analytics must remain blocked pending owner scope contract |
| Canonical v1/OpenAPI/capability discovery | MISSING | No Admin API specification or real discovery handlers |
| HQ/VPS topology, approved identities/policy | BLOCKED | Requires operator-provided keys, private addresses, certificates, independent approval policy and HQ repository; no live changes authorized |

## Baseline test and CI evidence

`go test ./services/admin-backoffice-service/... ./shared/go/serviceauth/... ./services/api-gateway/internal/features/routing/...` passed.
Go: 1.26.3 windows/amd64. Docker executable unavailable; PostgreSQL 18 binaries
are installed and may support isolated database testing.

Remote staging CI at audited SHA: Deploy platform success (39108184211),
Section 31 mandatory failure (39108184167), Vendor Sections 46–48 failure
(39108184172), Customer V4 certification failure (39108184166).
Section 31 pre-existing failures include build/vet, Dispatch provider Flutter,
Media and Dispatch Go suites; Admin, Gateway, shared HMAC module jobs succeeded.
These failures must not be bypassed or described as passing.

## Implementation decisions

Close edge exposure first. Add a separately initialized TLS-only listener
instead of changing the shared customer/provider bootstrap. Reuse HMAC v2
unchanged. Use pinned Ed25519 staff keys, mTLS URI SAN identity plus local
certificate allowlist, PostgreSQL atomic nonce/JTI claims, scoped audit and
operation storage. Ship no executable domain mutations without a real scoped
owner adapter and approved high-risk policy. Publish disabled route contracts
truthfully, with a functional security/capability/operation foundation.

Network ingress stays loopback-only by default; automatic platform deployments
must exclude this uncertified service. Live transport/HQ tests remain mandatory.
