# Approval System

A generic framework for organization-controlled sensitive actions:
`approval_policies`, `approval_requests`, `approval_steps`,
`approval_decisions`, `approval_delegations`, `approval_state_history`.

## Policies

A policy matches a resource type + action within an organization or branch
scope, and defines the required step sequence, quorum, eligible roles,
whether the requester may approve, expiry, step-up requirement and escalation.

Money thresholds are stored as `threshold_amount_minor` (`bigint`) plus an
explicit ISO currency. **This is a threshold reference, not a pricing
formula** — no fare, commission or subscription arithmetic exists anywhere in
this service.

## Request fingerprinting

Each request stores a `request_fingerprint` over the normalized controlled
action plus an immutable `payload_snapshot`. Execution re-checks the
fingerprint, so an approved payload cannot be swapped afterwards
(`approval_fingerprint_mismatch`). A partial unique index allows only one
open request per `(resource, action)`, so a second request cannot race the
first to execution.

## Decisions

Decisions are immutable (append-only trigger). Database constraints enforce
the rules directly rather than relying on application checks alone:

- `uq_fleet_partner_decision_per_step` — one approver counts at most once per
  step;
- `uq_fleet_partner_decision_client_id` — a repeated `clientDecisionId` on the
  same request is idempotent, not a second vote.

Approver eligibility is evaluated at decision time; self-approval is refused
when the policy forbids it (`approval_self_decision_forbidden`); revoked or
expired delegations grant nothing. Concurrent final decisions resolve to one
outcome. Expiry is worker-enforced. Rejected actions never execute; approved
actions execute exactly the fingerprinted command, once.

## Initial high-risk actions

Organization closure, ownership transfer, custom-role permission escalation,
sensitive role changes, provider removal where policy requires, and — by
contract, for later work packages — wallet allocation, sensitive exports,
return-to-service and organization policy changes.

The framework is deliberately generic so those later modules integrate
without changing the approval domain.
