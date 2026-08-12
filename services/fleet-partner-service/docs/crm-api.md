# CRM API reference (Section 14)

This documents **only routes that are actually mounted**. Nothing below is
aspirational.

## Conventions

Everything in [api-conventions.md](api-conventions.md) applies. Restated for
this surface:

- `Content-Type: application/json`
- Success: `{ "data": {}, "meta": {} }`
- Error: `{ "error": { "code", "message", "details", "requestId" } }`
- Money: `{ "amountMinor": 700000, "currency": "NGN" }` — integer minor units,
  ISO 4217, **no float**
- Quantity: a decimal **string**, e.g. `"2.5"`
- Probability: **basis points**, 0–10000
- Timestamps: UTC RFC 3339
- Identifiers: opaque and prefixed (`cst_`, `cnt_`, `cln_`, `opp_`, `qot_`,
  `qvr_`, `qac_`, `cvi_`, `ctr_`, `cvr_`, `sla_`, `slo_`, `crp_`). No sequential
  database ID is ever exposed.

### Pagination

Cursor only, stable ordering, organization scoped. `?cursor=<opaque>&limit=<n>`,
limit clamped to 1–100 (default 25). The next cursor is in `meta.nextCursor`, and
is empty on the last page.

### Filters

Allowlisted per endpoint. **An unknown query parameter is refused with `400`**,
never ignored: silently dropping a filter is the failure mode where a caller
believes they narrowed a list and got everything instead.

### Concurrency and idempotency

- Mutations require `expectedVersion` in the body, or `If-Match: "<version>"`.
- Commands require an `Idempotency-Key` header.
- Identical key + identical fingerprint replays the original result.
- Same key + changed fingerprint returns a conflict.

### Feature flags

With a capability's flag off, its routes answer `404`. They are still mounted, so
a disabled capability is observable rather than indistinguishable from a
deployment that never had it.

With `FLEET_CRM_KILL_SWITCH` engaged, every mutation answers `503`.

## Authentication

| Route class | Prefix | Auth |
| --- | --- | --- |
| Corporate | `/fleet-partner/v1` | Bearer token + resolved organization scope |
| Customer | `/fleet-partner/public/v1` | secure quotation token, rate limited |
| Internal | `/internal/fleet-partner/v1` | HMAC v2 + caller allowlist |

## Customers

| Method | Path | Permission | Idem | Ver |
| --- | --- | --- | --- | --- |
| GET | `/customers` | `crm.customers.read` | | |
| POST | `/customers` | `crm.customers.manage` | ✓ | |
| GET | `/customers/{customerId}` | `crm.customers.read` | | |
| PATCH | `/customers/{customerId}` | `crm.customers.manage` | | ✓ |
| POST | `/customers/{customerId}/archive` | `crm.customers.manage` | ✓ | ✓ |
| POST | `/customers/{customerId}/status` | `crm.customers.manage` | ✓ | ✓ |
| POST | `/customers/{customerId}/risk-status` | `crm.customers.manage` | ✓ | ✓ |
| GET | `/customers/{customerId}/history` | `crm.customers.read` | | |

Filters: `status`, `riskStatus`, `customerType`, `branchId`,
`accountOwnerUserId`, `createdAfter`, `createdBefore`, `search`.

`crm.sensitive.read` upgrades the response to include the unmasked
`registrationNumber` and `taxReference`.

## Contacts and locations

| Method | Path | Permission | Idem | Ver |
| --- | --- | --- | --- | --- |
| GET | `/customers/{id}/contacts` | `crm.contacts.read` | | |
| POST | `/customers/{id}/contacts` | `crm.contacts.manage` | ✓ | |
| GET | `/customers/{id}/contacts/{contactId}` | `crm.contacts.read` | | |
| PATCH | `/customers/{id}/contacts/{contactId}` | `crm.contacts.manage` | | ✓ |
| POST | `/customers/{id}/contacts/{contactId}/archive` | `crm.contacts.manage` | ✓ | ✓ |
| POST | `/customers/{id}/contacts/{contactId}/make-primary` | `crm.contacts.manage` | ✓ | ✓ |
| GET | `/customers/{id}/locations` | `crm.locations.read` | | |
| POST | `/customers/{id}/locations` | `crm.locations.manage` | ✓ | |
| GET | `/customers/{id}/locations/{locationId}` | `crm.locations.read` | | |
| PATCH | `/customers/{id}/locations/{locationId}` | `crm.locations.manage` | | ✓ |
| POST | `/customers/{id}/locations/{locationId}/archive` | `crm.locations.manage` | ✓ | ✓ |

Contact filters: `status`, `contactPurpose`. Location filters: `locationType`,
`status`.

Without `crm.sensitive.read`, contact email and phone are masked, and a
location's street address, exact coordinates, access instructions and safety
instructions are **absent from the response type entirely**.

## Sales opportunities

| Method | Path | Permission | Idem | Ver |
| --- | --- | --- | --- | --- |
| GET | `/sales/opportunities` | `crm.opportunities.read` | | |
| POST | `/sales/opportunities` | `crm.opportunities.manage` | ✓ | |
| GET | `/sales/opportunities/{id}` | `crm.opportunities.read` | | |
| PATCH | `/sales/opportunities/{id}` | `crm.opportunities.manage` | | ✓ |
| POST | `/sales/opportunities/{id}/mark-won` | `crm.opportunities.manage` | ✓ | ✓ |
| POST | `/sales/opportunities/{id}/mark-lost` | `crm.opportunities.manage` | ✓ | ✓ |
| POST | `/sales/opportunities/{id}/hold` | `crm.opportunities.manage` | ✓ | ✓ |
| POST | `/sales/opportunities/{id}/resume` | `crm.opportunities.manage` | ✓ | ✓ |
| GET | `/sales/opportunities/{id}/history` | `crm.opportunities.read` | | |

Filters: `stage`, `customerId`, `branchId`, `accountOwnerUserId`,
`createdAfter`, `createdBefore`.

`mark-lost` and `hold` require `reasonCode`. `resume` requires an explicit
`stage`, which is verified against the immutable history.

## Quotations

| Method | Path | Permission | Idem | Ver |
| --- | --- | --- | --- | --- |
| GET | `/quotations` | `crm.quotations.read` | | |
| POST | `/quotations` | `crm.quotations.manage` | ✓ | |
| GET | `/quotations/{id}` | `crm.quotations.read` | | |
| PATCH | `/quotations/{id}` | `crm.quotations.manage` | | ✓ |
| GET | `/quotations/{id}/history` | `crm.quotations.read` | | |
| GET | `/quotations/{id}/versions` | `crm.quotations.read` | | |
| GET | `/quotations/{id}/versions/{versionId}` | `crm.quotations.read` | | |
| GET | `/quotations/{id}/documents` | `crm.quotations.read` | | |
| POST | `/quotations/{id}/documents` | `crm.quotations.manage` | | |
| POST | `/quotations/{id}/submit` | `crm.quotations.submit` | ✓ | ✓ |
| POST | `/quotations/{id}/issue` | `crm.quotations.issue` | ✓ | ✓ |
| POST | `/quotations/{id}/accept` | `crm.quotations.accept` | ✓ | ✓ |
| POST | `/quotations/{id}/reject` | `crm.quotations.accept` | ✓ | ✓ |
| POST | `/quotations/{id}/cancel` | `crm.quotations.manage` | ✓ | ✓ |
| POST | `/quotations/{id}/supersede` | `crm.quotations.manage` | ✓ | ✓ |

Filters: `status`, `customerId`, `opportunityId`, `branchId`, `createdAfter`,
`createdBefore`.

### Issue response

`customerAccessToken` is present **only when `mintCustomerToken` was true**, and
only in this response. It is never returned again and only a keyed digest is
stored.

### `internalNotes`

Absent from the standard quotation view; present only with
`crm.sensitive.read`.

## Contracts

| Method | Path | Permission | Idem | Ver |
| --- | --- | --- | --- | --- |
| GET | `/contracts` | `crm.contracts.read` | | |
| POST | `/contracts` | `crm.contracts.manage` | ✓ | |
| GET | `/contracts/{id}` | `crm.contracts.read` | | |
| PATCH | `/contracts/{id}` | `crm.contracts.manage` | | ✓ |
| GET | `/contracts/{id}/history` | `crm.contracts.read` | | |
| GET | `/contracts/{id}/versions` | `crm.contracts.read` | | |
| GET | `/contracts/{id}/versions/{versionId}` | `crm.contracts.read` | | |
| GET | `/contracts/{id}/documents` | `crm.contracts.read` | | |
| POST | `/contracts/{id}/documents` | `crm.contracts.manage` | | |
| POST | `/contracts/{id}/documents/{documentId}/replace` | `crm.contracts.manage` | | |
| POST | `/contracts/{id}/documents/{documentId}/remove` | `crm.contracts.manage` | | |
| POST | `/contracts/{id}/submit` | `crm.contracts.manage` | ✓ | ✓ |
| POST | `/contracts/{id}/activate` | `crm.contracts.activate` | ✓ | ✓ |
| POST | `/contracts/{id}/suspend` | `crm.contracts.manage` | ✓ | ✓ |
| POST | `/contracts/{id}/resume` | `crm.contracts.activate` | ✓ | ✓ |
| POST | `/contracts/{id}/terminate` | `crm.contracts.terminate` | ✓ | ✓ |
| POST | `/contracts/{id}/renew` | `crm.contracts.manage` | ✓ | ✓ |

Filters: `status`, `customerId`, `branchId`, `createdAfter`, `createdBefore`.

`commercialSnapshot` is absent from the standard view and present only with
`crm.sensitive.read`.

`remove` detaches Fleet's pointer only; the response says
`mediaObjectRetained: true`.

## Service level agreements

| Method | Path | Permission | Idem | Ver |
| --- | --- | --- | --- | --- |
| GET | `/service-level-agreements` | `crm.sla.read` | | |
| POST | `/service-level-agreements` | `crm.sla.manage` | ✓ | |
| GET | `/service-level-agreements/{slaId}` | `crm.sla.read` | | |
| PATCH | `/service-level-agreements/{slaId}` | `crm.sla.manage` | | ✓ |
| POST | `/service-level-agreements/{slaId}/activate` | `crm.sla.activate` | ✓ | ✓ |
| POST | `/service-level-agreements/{slaId}/suspend` | `crm.sla.activate` | ✓ | ✓ |
| POST | `/service-level-agreements/{slaId}/terminate` | `crm.sla.manage` | ✓ | ✓ |
| GET | `/service-level-agreements/{slaId}/observations` | `crm.sla.read` | | |
| GET | `/service-level-agreements/{slaId}/history` | `crm.sla.read` | | |

Filters: `status`, `customerId`, `contractId`, `metricCode`; observations accept
`outcome`.

## Credit policy

| Method | Path | Permission | Idem | Ver |
| --- | --- | --- | --- | --- |
| GET | `/customers/{id}/credit-profile` | `crm.credit.read` | | |
| PUT | `/customers/{id}/credit-profile` | `crm.credit.manage` | ✓ | ✓* |
| POST | `/customers/{id}/credit-profile/hold` | `crm.credit.manage` | ✓ | ✓ |
| POST | `/customers/{id}/credit-profile/release` | `crm.credit.manage` | ✓ | ✓ |
| GET | `/customers/{id}/credit-profile/history` | `crm.credit.read` | | |

\* `expectedVersion` is required when updating an existing profile, and omitted
when creating the first one.

## Notification operations

| Method | Path | Permission |
| --- | --- | --- |
| GET | `/crm/notification-operations` | `crm.quotations.read` |
| POST | `/crm/notification-operations/{operationId}/replay` | `crm.quotations.manage` |

Filters: `status`, `subjectId`.

## Customer secure link

| Method | Path | Auth |
| --- | --- | --- |
| GET | `/fleet-partner/public/v1/quotations/{id}/view` | token |
| POST | `/fleet-partner/public/v1/quotations/{id}/accept` | token, `VIEW_AND_RESPOND` |
| POST | `/fleet-partner/public/v1/quotations/{id}/reject` | token, `VIEW_AND_RESPOND` |

Token via `Authorization: Bearer <token>` or `?token=`. Rate limited to 20
attempts per minute per client address; `429` when exceeded, `503` when the
limiter is unavailable.

## Internal

| Method | Path | Auth |
| --- | --- | --- |
| POST | `/internal/fleet-partner/v1/crm/sla-observations` | HMAC v2 + caller allowlist |

`201` for a new observation, `200` with `duplicate: true` for a replay.

## Stable error codes

`customer_not_found`, `customer_already_exists`, `customer_on_hold`,
`customer_risk_restricted`, `customer_archived`, `customer_invalid_state`,
`customer_contact_not_found`, `customer_contact_conflict`,
`customer_location_not_found`, `customer_location_invalid_timezone`,
`customer_location_invalid_coordinates`, `opportunity_not_found`,
`opportunity_invalid_transition`, `quotation_not_found`,
`quotation_invalid_state`, `quotation_expired`, `quotation_already_resolved`,
`quotation_approval_required`, `quotation_version_conflict`,
`quotation_document_not_ready`, `quotation_acceptance_conflict`,
`quotation_token_invalid`, `quotation_token_expired`, `contract_not_found`,
`contract_invalid_state`, `contract_not_active`, `contract_activation_blocked`,
`contract_approval_required`, `contract_document_not_ready`,
`contract_version_conflict`, `sla_not_found`, `sla_invalid_state`,
`sla_observation_conflict`, `sla_source_unknown`, `credit_profile_not_found`,
`customer_credit_hold`, `credit_change_approval_required`.

No response ever exposes a SQL error, a table name, a constraint name, an HMAC
failure detail, a Media or Notification provider error, or a stack trace.

## Machine-readable contract

The machine-readable OpenAPI 3.1 contract for these Section 14 routes is
[openapi/section14-crm.openapi.json](openapi/section14-crm.openapi.json).
Section 13 is documented separately and Section 15 has a separate contract at
`openapi/section15-private-operations.openapi.json`; this file does not claim to
cover the entire Fleet service.

Run `go test -count=1 -v
./services/fleet-partner-service/internal/features/crm/http -run OpenAPI` from
the repository root. The tests parse the JSON, resolve every local reference,
validate operation IDs and path parameters, mount the production Gin
registrars, and check route-to-spec and spec-to-route parity plus security,
permission, feature-flag, idempotency, expected-version, and data-minimization rules.

For a local rendered view without committing third-party assets, load the JSON
in an OpenAPI 3.1-compatible editor or run `npx @redocly/cli preview-docs
services/fleet-partner-service/docs/openapi/section14-crm.openapi.json`. No
runtime Swagger route is exposed by Fleet.
