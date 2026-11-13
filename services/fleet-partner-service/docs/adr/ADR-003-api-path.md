# ADR-003: Keep the /fleet-partner/v1 API paths

- Status: ACCEPTED (product owner, 2026-11-13)
- Affected specification: Sections 4.2, 35, 36, 59, 67

## Decision

The service keeps its public base path `/fleet-partner/v1` and internal base
path `/internal/fleet-partner/v1`. The Section 36/59 base path
`/api/v1/corporate-logistics` is not adopted. Resource paths below the base
follow Section 59 where a new endpoint is added; existing endpoints keep their
paths. Section 59 endpoints that exist under a different path are recorded in
`docs/implementation/spec-v2-audit/section59-endpoints.tsv`.

## Rationale

Part 1 (Section 4.2) already uses `/fleet-partner/v1`. The web app, the API
gateway, the dispatch, taxi and hauling services and shared Go clients
hard-code the current paths. A rename would be a coordinated breaking change
with no functional gain.

## Effects

- API: no rename. New endpoints use Section 59 resource names under the
  current base.
- Clients: unchanged.
- Rollback: not applicable.
