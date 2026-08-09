# Company CRM (Section 14)

## What a CRM customer is, and is not

A CRM customer is an **organization-owned commercial relationship record**. It
is not a marketplace identity.

| Concern | Owner |
| --- | --- |
| Marketplace customer accounts and credentials | customer-service |
| CRM customer records, contacts, locations | fleet-partner-service |
| Quotation and contract document bytes | media-file-service |
| Notification delivery | notification-service |
| Compliance decisions | verification-compliance-service |
| Disputes | support-dispute-service |
| Money, balances, ledgers | payment-wallet-service |

`marketplace_customer_id` is an **opaque pointer**. Fleet never resolves it by
reaching into another service's database, never creates a marketplace identity,
and never automatically merges the two. A CRM customer holds no credential and
grants no access to anything.

Fleet owns **no accounts receivable**. There is no debt calculation, no invoice
ageing and no derived available credit anywhere in Section 14. See
[customer-credit-policy.md](customer-credit-policy.md).

## Customer state machine

```
PROSPECT  → ACTIVE, INACTIVE, ARCHIVED
ACTIVE    → ON_HOLD, SUSPENDED, INACTIVE, ARCHIVED
ON_HOLD   → ACTIVE, SUSPENDED, INACTIVE, ARCHIVED
SUSPENDED → ACTIVE, INACTIVE, ARCHIVED
INACTIVE  → ACTIVE, ARCHIVED
ARCHIVED  → (terminal)
```

`ARCHIVED` has no outgoing edges: an archived customer is a closed record, not a
paused one. Archive is used instead of deletion because quotations, contracts
and their immutable versions reference the row, and a historical record pointing
at a deleted customer is a record with a hole in it.

There is no code path that assigns an arbitrary status. `models.CanTransitionCustomer`
is the whole matrix, and every write goes through it.

## Risk status

```
NOT_ASSESSED · LOW · MEDIUM · HIGH · RESTRICTED
```

A risk change **requires a reason code**: a risk decision nobody has to justify
is a risk decision nobody can review.

Escalating to `RESTRICTED` forces the customer's credit profile to `RESTRICTED`
**in the same transaction**. Leaving the two to disagree would mean a risk
decision that never reached the gate that actually blocks contract activation.

De-escalating *away from* `RESTRICTED` is routed through the WP9 approval
framework where the policy requires it — that is the change that removes a block
somebody deliberately applied.

A restricted customer's credit hold cannot be released while the restriction
stands; that would use the credit surface to undo a risk decision that has its
own permission.

## What blocks a new commitment

`Customer.BlocksNewCommitment` is the single source of truth:

| State | Effect | Error code |
| --- | --- | --- |
| `ARCHIVED` | blocks everything | `customer_archived` |
| risk `RESTRICTED` | blocks everything | `customer_risk_restricted` |
| `ON_HOLD` / `SUSPENDED` | blocks new commitments | `customer_on_hold` |
| credit hold `ON_HOLD` / `RESTRICTED` | blocks new commitments | `customer_credit_hold` |

Existing commitments are left intact in every case. Suspending a relationship
stops new offers; it does not retroactively void an issued quotation or an
active contract.

## Duplicate detection

Uniqueness is **organization scoped**. The same legal entity may legitimately be
a customer of two different fleet operators, and treating that as a conflict
would make one tenant's data a constraint on another's.

The duplicate key is:

1. `reg:<registration number>` where one is supplied, else
2. `tax:<tax reference>` where one is supplied, else
3. `name:<customer type>:<normalized name>`

A registration or tax reference is a genuine identity claim, so where one exists
it alone decides the question. The name fallback deliberately keeps the customer
type in the key and does **not** do fuzzy matching: rejecting two legitimately
separate customers because their names are similar is the more expensive
mistake.

## Sensitive data

`crm.customers.read` lets a caller see that a customer exists. It does **not**
carry their identity documents. The registration number and tax reference are
masked to their last four characters unless the caller also holds
`crm.sensitive.read`.

The masked and unmasked shapes are **different Go types** (`CustomerView` and
`SensitiveCustomerView`), each built field by field from the model. A handler
that forgets a permission check cannot accidentally produce the unmasked shape,
because building it requires deliberately calling the sensitive constructor.

## Routes

All routes are under `/fleet-partner/v1` and require a corporate bearer token
plus resolved organization scope.

| Method | Path | Permission |
| --- | --- | --- |
| GET | `/customers` | `crm.customers.read` |
| POST | `/customers` | `crm.customers.manage` |
| GET | `/customers/{customerId}` | `crm.customers.read` |
| PATCH | `/customers/{customerId}` | `crm.customers.manage` |
| POST | `/customers/{customerId}/archive` | `crm.customers.manage` |
| POST | `/customers/{customerId}/status` | `crm.customers.manage` |
| POST | `/customers/{customerId}/risk-status` | `crm.customers.manage` |
| GET | `/customers/{customerId}/history` | `crm.customers.read` |

List filters are allowlisted: `status`, `riskStatus`, `customerType`,
`branchId`, `accountOwnerUserId`, `createdAfter`, `createdBefore`, `search`,
plus `cursor` and `limit`. **An unknown query parameter is refused**, not
ignored — silently ignoring a filter is the failure mode where a caller believes
they narrowed a list and got everything instead.

`search` is parameterized and organization scoped, and LIKE metacharacters in
the caller's text are escaped, so a search for `100%` is a search rather than a
wildcard.

Every mutation requires `expectedVersion` (or `If-Match`); creates, archives and
status changes also require an `Idempotency-Key`.

## Tenant isolation

Every repository query carries `organization_id` in SQL. Nested resources carry
the parent ID as well as their own, so naming another customer's contact yields
the same not-found a nonexistent contact does.

A cross-tenant identifier and a genuinely missing one return **the same
response**. Any distinguishable answer is an existence oracle that lets one
organization enumerate another's customers.

## Atomicity

Every mutation writes the domain row, its immutable history, the audit record
and the outbox event inside **one transaction**. An event announcing a change
that was rolled back is worse than no event.

## Section 15

Section 15 is implemented in separate feature packages and migrations. Section
14 acceptance still creates only a durable conversion intent; the Section 15
order use case consumes that intent exactly once. CRM itself does not create a
shipment, package, manifest, recurrence, or bulk import.

## Machine-readable API contract

[openapi/section14-crm.openapi.json](openapi/section14-crm.openapi.json) is the
OpenAPI 3.1 contract for the mounted Section 14 CRM API only. Section 13 is
documented separately and Section 15 has its own OpenAPI contract. The shared declarative
route catalogue drives the production registrars, while the OpenAPI tests also
inspect Gin's registered route list and check both parity directions.

Validate it from the repository root with `go test -count=1 -v
./services/fleet-partner-service/internal/features/crm/http -run OpenAPI`.
Render it without committing viewer assets by opening the JSON in a compatible
OpenAPI editor or using `npx @redocly/cli preview-docs
services/fleet-partner-service/docs/openapi/section14-crm.openapi.json`. Fleet
does not expose a public runtime documentation route.
