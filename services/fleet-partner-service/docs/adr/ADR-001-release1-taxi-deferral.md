# ADR-001: Release 1 Taxi production-integration deferral

- Status: RELEASE-1-APPROVED-SCOPE-EXCEPTION
- Affected specification: Sections 35, 39, 41, 50, 59–61, 67–71
- Decision owner/approval: Product and Architecture approval required before future activation; no named approver is asserted here.

## Decision and rationale

Release 1 enables the real Dispatch and Hauling provider/execution integrations. Taxi production integration remains fail-closed because no approved, production-grade Taxi source-service contract is present. Fleet must not simulate Taxi provisioning, reservation, trip creation, lifecycle writes, or success responses.

No Taxi success is simulated.

Generic forward-compatible Taxi enum/capability values and data scaffolding may remain, but they confer no execution authority. This preserves schema and client compatibility without claiming an integration that does not exist.

## Release 1 effects

- API: no Taxi production provider or execution endpoints are exposed as successful integrations; configured Taxi execution flags are rejected or disabled.
- Data/schema: existing generic specialized-logistics records may retain a Taxi capability value; no Taxi source-of-truth or execution tables are fabricated.
- Events: no Taxi execution event is emitted as if a Taxi source service accepted it.
- Security: fail-closed capability and authority checks prevent a generic type value from becoming execution permission.
- Backward compatibility: Dispatch, Hauling, existing customer, and independent-provider behavior is unchanged.
- Tests: tests assert fail-closed configuration and never claim Taxi E2E success.
- Rollout: Taxi remains disabled at every Release 1 stage.
- Rollback: no Taxi state exists to migrate or unwind; keep the flag disabled.

## Future activation conditions

Activation requires the real Taxi source-service contract, authenticated provider resolution/provisioning, reservation and execution commands, event compatibility, unknown-outcome reconciliation, security review, migrations if genuinely required, and full provider/execution compatibility and end-to-end tests. Human product and architecture approval is mandatory.
