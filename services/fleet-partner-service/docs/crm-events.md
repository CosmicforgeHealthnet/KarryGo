# CRM events (Section 14)

Every CRM event goes through the **existing Fleet transactional outbox**. There
is no second CRM outbox: the domain row, its history, the audit record and the
outbox row are written in one transaction, so an event announcing a change that
was rolled back cannot exist.

## Catalogue

| Event | Aggregate |
| --- | --- |
| `organization.crm_customer.created.v1` | `crm_customer` |
| `organization.crm_customer.updated.v1` | `crm_customer` |
| `organization.crm_customer.archived.v1` | `crm_customer` |
| `organization.crm_customer.status_changed.v1` | `crm_customer` |
| `organization.crm_customer.risk_changed.v1` | `crm_customer` |
| `organization.crm_contact.created.v1` | `crm_contact` |
| `organization.crm_contact.updated.v1` | `crm_contact` |
| `organization.crm_contact.archived.v1` | `crm_contact` |
| `organization.crm_location.created.v1` | `crm_location` |
| `organization.crm_location.updated.v1` | `crm_location` |
| `organization.crm_location.archived.v1` | `crm_location` |
| `organization.sales_opportunity.created.v1` | `sales_opportunity` |
| `organization.sales_opportunity.updated.v1` | `sales_opportunity` |
| `organization.sales_opportunity.stage_changed.v1` | `sales_opportunity` |
| `organization.quotation.created.v1` | `quotation` |
| `organization.quotation.submitted.v1` | `quotation` |
| `organization.quotation.approved.v1` | `quotation` |
| `organization.quotation.issued.v1` | `quotation` |
| `organization.quotation.viewed.v1` | `quotation` |
| `organization.quotation.accepted.v1` | `quotation` |
| `organization.quotation.rejected.v1` | `quotation` |
| `organization.quotation.expired.v1` | `quotation` |
| `organization.quotation.cancelled.v1` | `quotation` |
| `organization.quotation.superseded.v1` | `quotation` |
| `organization.contract.created.v1` | `contract` |
| `organization.contract.approved.v1` | `contract` |
| `organization.contract.activated.v1` | `contract` |
| `organization.contract.suspended.v1` | `contract` |
| `organization.contract.terminated.v1` | `contract` |
| `organization.contract.renewal_pending.v1` | `contract` |
| `organization.contract.expired.v1` | `contract` |
| `organization.contract.superseded.v1` | `contract` |
| `organization.sla.created.v1` | `service_level_agreement` |
| `organization.sla.activated.v1` | `service_level_agreement` |
| `organization.sla.suspended.v1` | `service_level_agreement` |
| `organization.sla.outcome_recorded.v1` | `service_level_agreement` |
| `organization.sla.escalation_requested.v1` | `service_level_agreement` |
| `organization.customer_credit.updated.v1` | `customer_credit_profile` |
| `organization.customer_credit.hold_changed.v1` | `customer_credit_profile` |

Every payload carries `organizationId`, the aggregate ID, a version and
`occurredAt`, plus the correlation ID on the envelope.

## Payloads are built, never serialized

Each payload builder in `usecases/events.go` is written **field by field**. None
of them serializes a domain model.

That direction matters. A model that grows a field starts publishing it the
moment it is serialized wholesale — and the fields most likely to be added to
these models are exactly the ones that must never leave.

## What CRM events never contain

- full tax or registration identifiers
- raw phone numbers or email addresses
- customer legal names on contact and location events
- exact coordinates, street addresses, access instructions, safety instructions
- raw customer access tokens, or hashes of them
- IP addresses or user-agent strings
- internal sales notes, risk investigation notes, credit hold private notes
- credit limits
- full quotation line items
- contract commercial snapshots
- document bytes, signed URLs or storage paths
- wallet balances, provider information, commission or margin
- secrets or HMAC signatures

Consumers get an **opaque identifier and a status projection**. A consumer that
needs more asks through an authorized API, where the caller's permissions still
apply.

Specific decisions worth stating:

- **Customer risk changes** carry the previous and new risk status but **not the
  reason**. It is investigation detail, and a consumer that only needs to know a
  customer is restricted does not need to know why somebody thought so.
- **Customer updates** carry `changedFields` — field *names*, never values. A
  history or event that copies a tax reference turns an audit trail into a
  second, less guarded, copy of the PII it exists to account for.
- **Quotations** carry the total and currency, because downstream reporting
  needs a magnitude, but not the line items.
- **Opportunities** carry the stage and probability but not the expected value.
- **Credit** carries the hold status and a `blocking` flag but not the limit.
- **Views** carry `viewedAt` and nothing about the viewer.

## `organization.quotation.accepted.v1`

```json
{
  "quotationId": "qot_...",
  "organizationId": "org_...",
  "customerId": "cst_...",
  "quotationNumber": "QTN-2026-000001",
  "status": "ACCEPTED",
  "currency": "NGN",
  "totalAmountMinor": 1075000,
  "acceptedVersionId": "qvr_...",
  "acceptanceMethod": "SECURE_CUSTOMER_TOKEN",
  "actorType": "CUSTOMER",
  "acceptedAt": "2026-07-29T10:00:00Z",
  "conversionIntentId": "cvi_...",
  "conversionType": "FUTURE_PRIVATE_ORDER",
  "conversionStatus": "PENDING_SECTION_15"
}
```

The conversion intent ID is present so a future Section 15 consumer can find the
record it will act on. The payload says `PENDING_SECTION_15` and carries **no
order ID**: nothing has been ordered, and an event that implied otherwise would
invite a consumer to behave as though something had.

## `organization.sla.outcome_recorded.v1`

The outcome is reported exactly as stored. `UNKNOWN` and `INSUFFICIENT_DATA`
travel as themselves and are never normalized into `MET` on the way out: a
consumer building a service report must be able to tell "we met it" from "we
could not tell".

## `organization.sla.escalation_requested.v1`

Carries `financialEffect: "NONE"`. An escalation is a request for attention. It
carries no penalty, no credit, no deduction and no wallet reference, because none
was created.

## Metric labels

No organization ID, customer ID, quotation ID or contract ID is ever a metric
label. Those are unbounded cardinality and, worse, a way to enumerate one
tenant's records from another tenant's dashboard. Labels are statuses, stages,
outcomes and worker names only.
