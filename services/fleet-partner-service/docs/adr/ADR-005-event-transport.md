# ADR-005: Deliver events over signed HTTP with Section 61 names

- Status: ACCEPTED (product owner, 2026-11-13)
- Affected specification: Sections 26, 60, 61, 62

## Decision

Fleet delivers its events from the transactional outbox to each consuming
service over HMAC-signed HTTP (the pattern Dispatch, Hauling and Wallet
already use), with retry, dead-lettering and replay. Event names and the
envelope follow Section 61 (including causationId, actor, requestId,
traceparent and metadata). Events Fleet consumes keep arriving at
`POST /internal/fleet-partner/v1/events`; producers in other services are
changed to the Section 61 names, and Fleet accepts the previous names during
the transition.

## Effects

- Events: renamed per Section 61; the Redis stream remains only as an
  internal fan-out.
- Other services: each consumer adds a signed ingest endpoint; each producer
  of an event Fleet needs adds a delivery target. Listed in the developer
  brief work log as they land.
