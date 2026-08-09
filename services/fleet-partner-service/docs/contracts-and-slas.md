# Contracts and service level agreements (Section 14)

## Contract state machine

```
DRAFT             → AWAITING_APPROVAL, APPROVED, TERMINATED
AWAITING_APPROVAL → APPROVED, DRAFT (changes required), TERMINATED
APPROVED          → ACTIVE, TERMINATED
ACTIVE            → SUSPENDED, RENEWAL_PENDING, EXPIRED, TERMINATED, SUPERSEDED
SUSPENDED         → ACTIVE, TERMINATED, EXPIRED, RENEWAL_PENDING
RENEWAL_PENDING   → ACTIVE, EXPIRED, TERMINATED, SUPERSEDED
EXPIRED · TERMINATED · SUPERSEDED → (terminal)
```

Only a `DRAFT` is editable, enforced in SQL as well as in the use case. An
approved or activated contract is never hard-deleted.

## Commercial snapshot

Where a contract references a quotation, that quotation must be one the customer
**accepted**, and the commercial snapshot is taken from the accepted immutable
version rather than recomputed:

```json
{
  "source": "ACCEPTED_QUOTATION_VERSION",
  "quotationId": "...",
  "acceptedVersionId": "...",
  "currency": "NGN",
  "totalAmountMinor": 1075000,
  "quotationContentDigest": "..."
}
```

The customer agreed to those numbers. Recalculating them would be a different
agreement wearing the same reference. There is no marketplace pricing, no rate
card and no tax calculation anywhere in the contract flow.

## Activation revalidates everything

`POST /fleet-partner/v1/contracts/{id}/activate` requires
`crm.contracts.activate`, an `Idempotency-Key` and an `expectedVersion`.

Every dependency is revalidated **at activation**, not trusted from approval
time:

- contract status is `APPROVED`
- customer status is `ACTIVE`
- customer risk is not `RESTRICTED`
- no blocking credit hold
- every mandatory document is confirmed `READY` and safe, re-read from Media
  immediately beforehand
- the approval is current and its **exact approved payload** still matches

An approval is permission to activate a specific agreement, not a promise that
the world stood still. The customer may have been suspended, their risk may have
been escalated, a credit hold may have been applied and a signed document may
have been revoked, all since the reviewer said yes.
`TestCRMContractActivationBlockedByRestrictedRisk` asserts the restriction case.

Activation writes an immutable `fleet_partner_contract_versions` row, freezes the
document set onto it, moves the contract to `ACTIVE` and records history, audit
and outbox — all in one transaction. The notification is a durable intent
recorded in that transaction and delivered outside it, so a notification outage
cannot roll back an activation.

## Immutable activated versions

The version table carries a `BEFORE UPDATE OR DELETE` trigger that refuses both.
An activated version rejects an update, a delete, a document replacement, a
commercial change, a date change and a signatory replacement.

A change requires a new controlled version, a renewal, or a replacement
contract. The version in force is never silently mutated.

## Suspension, termination, renewal

**Suspension** preserves the active version untouched. It calculates no penalty,
creates no refund and makes no provider deduction: pausing an agreement is an
operational decision, and turning it into a financial one would be this service
inventing money movements it has no authority over.

**Termination** preserves every version and every document. It calculates no
termination fee, creates no refund and posts nothing to a wallet: what a party
owes on early termination is a commercial question settled elsewhere, and a
number invented here is a number somebody would act on. Termination routes
through the approval framework where policy requires it.

**Renewal** creates a *new draft contract* that supersedes the old one, and moves
the original to `RENEWAL_PENDING` while leaving its activated version exactly as
it is. A renewal never activates itself: a renewal is a new commitment, and new
commitments go through approval.

## Contract documents

Fleet stores an opaque Media object ID, a purpose, a readiness projection and a
retention reference. It stores **no bytes, no URL, no storage path, no upload
token, no checksum and no raw content**.

`POST /contracts/{id}/documents/{documentId}/remove` removes **Fleet's pointer
only**. It never asks Media to delete anything: physical deletion belongs to
Media's retention and legal-hold workflow, which is the only place that knows
whether a file may lawfully be destroyed. The response says so explicitly
(`mediaObjectRetained: true`).

A document bound to an activated contract version cannot be replaced or removed:
it is part of an immutable record.

## Service level agreements

### State machine

```
DRAFT     → ACTIVE, TERMINATED
ACTIVE    → SUSPENDED, EXPIRED, TERMINATED, SUPERSEDED
SUSPENDED → ACTIVE, EXPIRED, TERMINATED, SUPERSEDED
EXPIRED · TERMINATED · SUPERSEDED → (terminal)
```

Only a `DRAFT` is editable. An active SLA is a promise a customer is already
relying on; changing an active commitment goes through supersession.

Activating an SLA attached to a contract requires that **contract to be
`ACTIVE`**: a service commitment under an agreement that is not in force is a
commitment to nothing.

### Supported metrics

The metric list is **closed**, and each metric admits only the units it can
actually be measured in:

| Metric | Units | Direction |
| --- | --- | --- |
| `PICKUP_ON_TIME` | `PERCENT`, `BASIS_POINTS` | higher is better |
| `DELIVERY_ON_TIME` | `PERCENT`, `BASIS_POINTS` | higher is better |
| `ORDER_COMPLETION_TIME` | `MINUTES`, `HOURS` | lower is better |
| `PACKAGE_DAMAGE_RATE` | `PERCENT`, `BASIS_POINTS` | lower is better |
| `PROOF_COMPLETION_TIME` | `MINUTES`, `HOURS` | lower is better |
| `FAILED_ATTEMPT_RATE` | `PERCENT`, `BASIS_POINTS` | lower is better |

Windows: `DAILY`, `WEEKLY`, `MONTHLY`, `QUARTERLY`, `PER_ORDER`.

An SLA with an unrecognised metric or a unit that does not fit it would produce
`UNKNOWN` forever — a promise the operator cannot keep and the customer cannot
check — so both are refused at creation.

`future_service_catalog_item_id` is an **opaque forward reference**. Section 15
owns the service catalogue; nothing here resolves it.

### Observation ingestion

```
POST /internal/fleet-partner/v1/crm/sla-observations
```

The route sits on the internal route class, so it inherits HMAC v2 verification,
timestamp freshness and nonce replay protection from the shared middleware.

What this handler adds is the question a signature does not answer: **whether
this caller is entitled to assert an SLA outcome**. The approved source list is
closed — `driver-dispatch-delivery-service`, `driver-hauling-service`,
`support-dispute-service` — because an SLA outcome is a claim about service
quality, and a caller that does not perform the work has no standing to make it,
however well signed its request is.

The organization ID in the body is **checked against the SLA's actual owner**,
never trusted. A valid signature proves who is calling and proves nothing about
whose data they may write. A cross-tenant SLA ID is answered `sla_not_found`.

Observations are **append only**, enforced by trigger.

Identity is `(sla_id, source_type, source_reference, source_version)` with a
payload hash:

| Situation | Result |
| --- | --- |
| same version, same payload hash | duplicate — the stored row is returned |
| same version, different payload hash | `sla_observation_conflict` |
| older version than stored | recorded as `NOT_EVALUATED`, not promoted |
| version gap | recorded as `UNKNOWN` |

A source that changes its mind about a window it already reported must say so
with a **new version**.

### UNKNOWN is never MET

```
MET · MISSED · NOT_EVALUATED · INSUFFICIENT_DATA · UNKNOWN
```

An absent measurement is `INSUFFICIENT_DATA`. An uninterpretable metric or unit,
or a gap in the version sequence, is `UNKNOWN`. Neither is ever collapsed into
`MET`.

An SLA that could not be measured has not been met; it has not been measured.
Reporting the two as the same thing would let a monitoring outage read as
perfect service. The outcome travels as itself through the API and through
`organization.sla.outcome_recorded.v1`.

### Escalation creates no money

An escalation produces a notification intent, and a support-case intent where a
real contract exists. It creates **no penalty, no customer credit, no provider
deduction, no refund and no wallet journal** — the event says
`financialEffect: "NONE"`.

Whether a missed SLA costs anybody money is a commercial question this service
has no authority to answer, and answering it automatically would move money
nobody agreed to move.

## Routes

| Method | Path | Permission |
| --- | --- | --- |
| GET/POST | `/contracts` | `crm.contracts.read` / `.manage` |
| GET/PATCH | `/contracts/{id}` | `crm.contracts.read` / `.manage` |
| GET | `/contracts/{id}/history` | `crm.contracts.read` |
| GET | `/contracts/{id}/versions` | `crm.contracts.read` |
| GET | `/contracts/{id}/versions/{versionId}` | `crm.contracts.read` |
| GET/POST | `/contracts/{id}/documents` | `crm.contracts.read` / `.manage` |
| POST | `/contracts/{id}/documents/{documentId}/replace` | `crm.contracts.manage` |
| POST | `/contracts/{id}/documents/{documentId}/remove` | `crm.contracts.manage` |
| POST | `/contracts/{id}/submit` | `crm.contracts.manage` |
| POST | `/contracts/{id}/activate` | `crm.contracts.activate` |
| POST | `/contracts/{id}/suspend` | `crm.contracts.manage` |
| POST | `/contracts/{id}/resume` | `crm.contracts.activate` |
| POST | `/contracts/{id}/terminate` | `crm.contracts.terminate` |
| POST | `/contracts/{id}/renew` | `crm.contracts.manage` |
| GET/POST | `/service-level-agreements` | `crm.sla.read` / `.manage` |
| GET/PATCH | `/service-level-agreements/{slaId}` | `crm.sla.read` / `.manage` |
| POST | `/service-level-agreements/{slaId}/activate` | `crm.sla.activate` |
| POST | `/service-level-agreements/{slaId}/suspend` | `crm.sla.activate` |
| POST | `/service-level-agreements/{slaId}/terminate` | `crm.sla.manage` |
| GET | `/service-level-agreements/{slaId}/observations` | `crm.sla.read` |
| GET | `/service-level-agreements/{slaId}/history` | `crm.sla.read` |

The contract's `commercialSnapshot` is absent from `ContractView` and appears
only in `SensitiveContractView`, behind `crm.sensitive.read`: it is the
negotiated substance of the agreement and the most sensitive thing in the
aggregate.
