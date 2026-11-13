# ADR-006: Company approval through the admin console

- Status: ACCEPTED (product owner, 2026-11-13)
- Affected specification: Sections 8, 37, 58.1, 60, 70 (journey 1)

## Decision

An operator approves, rejects, requests changes to or restricts a company in
admin-backoffice. Admin-backoffice records the decision in
verification-compliance-service, which delivers
`organization.verification.status_changed.v1` to Fleet. Fleet maps the
verification status to its own (`VERIFIED` -> `APPROVED`, `PENDING` ->
`UNDER_REVIEW`, `REJECTED` -> `REJECTED`, `SUSPENDED` -> restricted), reads
the decision back when an event is missed, and activates the company when
every activation gate passes.

The wallet gate stays mandatory. On staging, where Fleet has no
payment-wallet credentials yet, it can be waived with
`FLEET_ACTIVATION_WALLET_GATE_WAIVED=true`; configuration validation refuses
that setting in production.

## Effects

- Companies stop being stuck UNDER_REVIEW once an operator decides.
- Rollback: turn the consumer off; companies keep their state.
