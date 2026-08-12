# Dispatch private execution authority

Fleet private orders use the internal contract documented at
`docs/openapi/private-execution.openapi.json`. The surface is mounted below
`/api/v1/internal/private-executions` and requires allowlisted HMAC v2 with a
fresh timestamp and single-use persisted nonce.

Create is bound to the stable Fleet operation id and `Idempotency-Key`. An
identical fingerprint replays the original canonical request/trip result;
changed reuse is a conflict. Timeout or ambiguous delivery is resolved through
the operation read-back route before another mutation. Cancellation is
idempotent and reports authoritative confirmation separately from command
acceptance.

Dispatch uses its existing reservation, eligibility and canonical trip use
cases. It stores Fleet order, shipment and assignment ids only as opaque
references, publishes versioned lifecycle events through its transactional
outbox, and excludes fares, margins, commissions, wallet data and customer
contact data. Provider apps act through the established Dispatch provider
bearer routes, which relay private assignment ids to Fleet with HMAC v2. Taxi is
outside this contract and remains disabled.
