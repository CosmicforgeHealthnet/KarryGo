# Eligibility and approval (WP9)

This document covers what was implemented: the durable eligibility decision
layer, the extended approval framework, the private vehicle and provider
authority contract, and the routes, policies, permissions, events and runbook
that go with them.

## 1. The idea

Two questions guard every sensitive organization operation.

**Is this subject eligible?** Answered by a *decision*: a durable record naming
the subject, the operation, the policy version that decided it, the
authoritative sources consulted and the version each answered at, when it was
taken and when it stops being usable. Only a decision that is **current** (not
superseded), **unexpired** and **ELIGIBLE** authorizes anything.

**Did the right people approve this exact thing?** Answered by an *approval
request* bound to a payload fingerprint and a subject version, spent exactly
once against a unique execution reference.

Neither question has a "skip" answer. An unreachable authority, data older than
policy allows, or sources that disagree produce `UNKNOWN`, `STALE` or
`RECONCILING`, and every gate treats those exactly as it treats `INELIGIBLE`.

## 2. Decision statuses

| Status | Meaning | Authorizes? |
| --- | --- | --- |
| `ELIGIBLE` | Every required check passed against fresh authoritative data | Yes, while current and unexpired |
| `INELIGIBLE` | A definitive no from data that was available | No |
| `UNKNOWN` | An authoritative source could not be consulted | No |
| `STALE` | The data consulted was older than policy allows | No |
| `RECONCILING` | Sources disagreed and the disagreement is being resolved | No |

`UNKNOWN` outranks `RECONCILING`, which outranks `STALE`, which outranks
`INELIGIBLE`. The strongest true statement wins: if an authority could not be
reached at all, reporting "ineligible because a document is missing" would
imply the rest of the picture was checked when it was not.

## 3. Schema

Migration `000019_eligibility_decisions`:

| Table | Purpose |
| --- | --- |
| `fleet_partner_eligibility_reason_codes` | The closed reason vocabulary. Decisions foreign-key into it, so a typo fails the write instead of producing an unqueryable reason. |
| `fleet_partner_eligibility_policies` | Immutable policy versions. Publishing archives the outgoing version; a trigger refuses any other edit. |
| `fleet_partner_eligibility_decisions` | The decisions. A trigger refuses every edit except superseding. |
| `fleet_partner_eligibility_decision_reasons` | Controlled reasons per decision, append-only. |
| `fleet_partner_eligibility_decision_sources` | Authority, kind, opaque reference, source version and freshness per decision, append-only. |
| `fleet_partner_eligibility_decision_history` | Append-only history: `EVALUATED`, `SUPERSEDED`, `EXPIRED`, `INVALIDATED`, `CONSUMED`. |

`uq_fleet_partner_eligibility_current` is a partial unique index on
`(organization_id, subject_type, subject_id, operation_type) WHERE
superseded_at IS NULL`. Two concurrent evaluators therefore produce one winner
and one conflict, never two live decisions a caller could choose between.

Subject identifiers are opaque text with no foreign key — a vehicle is owned by
another service, and a cross-service foreign key is the coupling this
architecture exists to avoid.

Migration `000020_approval_execution_binding` **extends** the framework from
`000006`. It adds `INVALIDATED` to the status set, `expected_subject_version`,
`payload_version`, `policy_version`, the copied approval controls
(`minimum_approvals`, `eligible_role_ids`, `self_approval_allowed`,
`separation_of_duties`), `eligibility_decision_id`, `execution_reference`,
`invalidated_at` / `invalidation_reason`, `branch_id`, `idempotency_key`, plus
`request_version` and `approved_fingerprint` on decisions, and the new
append-only `fleet_partner_approval_executions` table.

Two schema-level guarantees worth naming:

* `uq_fleet_partner_approval_execution_reference` is the exactly-once rule. A
  retried execute that reuses the reference collides instead of spending a
  second approval on the same operation.
* `chk_fleet_partner_approval_execution_evidence` refuses an `EXECUTED` row
  with no reference and no timestamp — an assertion nobody can check.

## 4. Policies

Policy versions are rows, never edits. A published version is immutable apart
from archival, so every decision that cited version *N* keeps citing exactly
what version *N* said. A tenant's own `ACTIVE` version wins over the platform
default; the lookup is ordered rather than merged, because a policy that is
half tenant and half platform is a rule nobody wrote.

Platform defaults seeded by migration 19:

| Policy key | Approval | Min | Subscription | Notable checks |
| --- | --- | --- | --- | --- |
| `COMPANY_ACTIVATION` | yes | 1 | no | lifecycle, verification, approval, restrictions, documents, branch, freshness |
| `PROVIDER_LINKING` | yes | 1 | yes | provider existence, membership, verification, service type, consent, freshness |
| `VEHICLE_LINKING` | yes | 1 | no | vehicle existence, ownership, provider association, service type, verification, restrictions, documents, source version |
| `ASSIGNMENT` | no | – | yes | full provider set incl. availability, documents, assignment conflict; vehicle set incl. ownership |
| `EXECUTION` | no | – | no | provider existence/verification/availability/documents; vehicle existence/ownership/verification |
| `PAID_FEATURE` | no | – | yes | lifecycle, restrictions, subscription |
| `FINANCIAL_RELEASE` | yes | 2 | yes | lifecycle, restrictions, subscription, freshness |
| `FINANCIAL_REVERSAL` | yes | 2 | no | lifecycle, restrictions, freshness |
| `DOCUMENT_ACTION` | no | – | no | lifecycle, restrictions, documents |

Every default sets `self_approval_allowed = false` and
`separation_of_duties = true`. A tenant publishing its own version gets
separation of duties **on** when the field is omitted: an author who forgets it
gets the safer rule.

## 5. The vehicle and provider authority contract

Fleet holds no vehicle table and no provider platform state. Both are read over
a private contract mounted by `driver-dispatch-delivery-service` at
`/api/v1/internal/authority`:

* `GET /vehicles/:vehicleId?providerId=&serviceType=`
* `GET /providers/:providerId?serviceType=`

The vehicle answer carries: vehicle and provider identifiers, control state,
`providerMatch`, service type and `serviceTypeMatch`, verification state,
availability, restriction state, per-document present/expired status, active
assignment conflict with its **basis**, source version and observation time.
The provider answer carries account state, verification, service type,
availability, `shiftOpen`, restriction state, documents and conflict.

It carries no plate number, chassis number, brand, model, colour, document URL,
name, phone or email. A fleet operator asking "may this vehicle work?" needs
verdicts, not a registration record.

`assignmentConflictBasis` is `PROVIDER_LIVE_TRIP`. Dispatch trips do not name a
vehicle, so the honest granularity is the owning provider's live work; stating
the basis stops a caller reading more precision into the answer than exists.

Security on every route, in order and none optional:

1. `serviceauth` HMAC over method, request URI, timestamp and body hash, inside
   a bounded freshness window;
2. a caller allowlist — holding a valid signature for some other integration
   must not grant vehicle reads;
3. a single-use nonce (`X-Service-Nonce`) claimed in
   `dispatch_service_request_nonces`, so a captured request cannot be replayed
   while its signature is still fresh.

A missing nonce store answers `503` rather than passing: a missing control must
never read as a passed one. Unknown vehicles and malformed identifiers return
the identical `404`, so the endpoint is not an existence oracle.

## 6. Routes

All under `/fleet-partner/v1`, tenant-scoped, branch-filtered, deny by default.

| Method | Path | Permission |
| --- | --- | --- |
| POST | `/eligibility/evaluations` | `eligibility.decisions.refresh` |
| GET | `/eligibility/decisions` | `eligibility.decisions.read` |
| GET | `/eligibility/decisions/:decisionId` | `eligibility.decisions.read` |
| GET | `/eligibility/decisions/:decisionId/history` | `eligibility.decisions.read` |
| GET | `/eligibility/policies` | `eligibility.policies.read` |
| POST | `/eligibility/policies` | `eligibility.policies.manage` |
| POST | `/approvals/requests` | `approvals.requests.create` |
| GET | `/approvals/requests` | `approvals.requests.read` |
| GET | `/approvals/requests/:approvalId` | `approvals.requests.read` |
| GET | `/approvals/requests/:approvalId/decisions` | `approvals.requests.read` |
| GET | `/approvals/requests/:approvalId/history` | `approvals.requests.read` |
| POST | `/approvals/requests/:approvalId/approve` | `approvals.requests.decide` |
| POST | `/approvals/requests/:approvalId/reject` | `approvals.requests.decide` |
| POST | `/approvals/requests/:approvalId/cancel` | `approvals.requests.cancel` |
| POST | `/approvals/requests/:approvalId/execute` | `approvals.requests.execute` |

Write routes require an `Idempotency-Key`. Out-of-scope identifiers answer
`404`, never `403`: a distinguishable refusal would confirm the identifier is
real.

## 7. Permissions

`eligibility.decisions.read`, `eligibility.decisions.refresh`,
`eligibility.policies.read`, `eligibility.policies.manage`,
`approvals.requests.read`, `approvals.requests.create`,
`approvals.requests.decide`, `approvals.requests.cancel`,
`approvals.requests.execute`.

Reading an answer, forcing a re-evaluation and rewriting the policy that
decides the answer are three separate powers, because whoever can rewrite the
policy can make anything eligible. Compliance and finance may decide but not
request; operations may request and execute but not decide.

## 8. Events

| Event | Emitted when |
| --- | --- |
| `organization.eligibility.decided.v1` | Any decision is recorded |
| `organization.approval.requested.v1` | An approval is raised |
| `organization.approval.decided.v1` | An approver approves or rejects |
| `organization.approval.invalidated.v1` | An approval stops describing the operation |
| `organization.approval.executed.v1` | An approval is spent |

Every event is written to the outbox in the same transaction as the state
change, alongside the history entry and the audit record. Payloads are
allowlisted field by field; the approval payload snapshot is deliberately never
published.

## 9. Enforcement

The gate is a port (`eligibility/enforcement.Gate`) that domain use cases call
through `enforcement.Require`. A **nil** gate is a refusal, not permission: a
service wired without one has no way to establish that an operation is
permitted, and a control that disappears when its dependency is missing is not
a control.

Gated today, in service logic rather than at the HTTP edge:

* company activation (`TryActivate`);
* provider linking (`CreateProvider`);
* assignment and reassignment (`Assign`, `Reassign`) — provider and vehicle;
* acceptance and the departure path (`AcceptAssignment`) under `EXECUTION`;
* settlement, refund and reversal requests, plus the wallet orchestration
  surface, which maps hold release and earning allocation onto
  `FINANCIAL_RELEASE` and refund/reversal onto `FINANCIAL_REVERSAL`, with
  everything else on `PAID_FEATURE`.

## 10. Runbook

**A gate is blocking and nobody knows why.** Read the decision:
`GET /fleet-partner/v1/eligibility/decisions?subjectId=…&currentOnly=true`.
The `reasons` array is a closed vocabulary and `sources` names each authority
with the version it answered at and its freshness. `status = UNKNOWN` with
`authority_unavailable` means an integration was down, not that the subject is
bad.

**Everything is UNKNOWN across the tenant.** Check the dispatch client
configuration. An unconfigured authority yields `authority_not_configured` and
blocks every vehicle and provider check by design.

**An approval cannot be executed.** `approval_quorum_not_met` means it has not
been approved yet. `approval_invalidated` with reason `payload_changed` or
`subject_version_changed` means it must be raised again — waiting will not
help. `approval_expired` means it can be re-requested. Every attempt, including
refused ones, is recorded in `fleet_partner_approval_executions` with its
outcome and safe reason code.

**A policy change needs rolling back.** Publish a new version with the previous
content. Historical decisions are untouched: they keep citing the version they
were decided under.

**Nonce table growth.** `dispatch_service_request_nonces` rows carry
`expires_at` ten minutes out. A periodic `DELETE FROM
dispatch_service_request_nonces WHERE expires_at < now()` is safe at any time;
the rows only need to outlive the signature freshness window.
