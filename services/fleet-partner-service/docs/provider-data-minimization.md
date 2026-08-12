# Provider Data Minimization

Providers see what they need to do the job safely and well. They do not see
the organization's commercial position. This document defines the rule, the
allowlists that implement it, and how the boundary is tested.

## 1. The rule

Provider-facing surfaces — API responses, notification payloads, event
payloads, webhooks, logs and exports — must **omit** commercial fields
entirely. Omission means the key is absent from the payload. Sending the key
with `0`, `null`, `""`, or `"REDACTED"` is a violation, because it still
discloses the shape of the organization's commercial model and invites
inference.

### 1.1 Forbidden on provider surfaces

`customerFare`, `customerPrice`, `quotedPrice`, `companySettlement`,
`organizationSettlement`, `platformCommission`, `commissionPercentage`,
`contractRate`, `contractTerms`, `profit`, `profitability`, `margin`,
`customerWallet`, `organizationWallet`, `invoiceAmount`,
`organizationRevenue`, `subscriptionPrice`

This list is a floor, not a ceiling. Any field that reveals what the customer
paid, what the organization keeps, or what the platform takes belongs here.

### 1.2 What providers do see

Operationally necessary detail only: the assignment and its deadline, the
service type, pickup and delivery locations and contacts, timing windows,
cargo or vehicle requirements, special instructions, safety-relevant notes,
and their own earnings as calculated and owned by their own provider service.

Provider earnings are **not** computed here. This service holds no ledger,
performs no settlement, and calculates no commission or price. Earnings shown
to a provider come from the provider's owning service.

## 2. How it is implemented

### 2.1 Allowlists, never stripping

Provider DTOs are built field-by-field from an explicit allowlist:

```go
view := ProviderAssignmentView{
    AssignmentID:     assignment.ID,
    Status:           assignment.Status,
    AllowedActions:   assignment.AllowedActions(now),
    ResponseDeadline: assignment.ResponseDeadline,
    ServiceType:      projection.ServiceType,
    // ... every field named deliberately
}
```

Denylist filtering — serializing the internal model then deleting keys — is
prohibited. A denylist fails open: a new commercial field added upstream leaks
until someone remembers to add it to the filter. An allowlist fails closed: a
new field simply does not appear until someone deliberately adds it.

For the same reason, provider views never embed the internal model, never use
`map[string]any` passthrough of source payloads, and never marshal a struct
that has commercial fields with `json:"-"` tags — a tag is one keystroke away
from becoming a leak.

### 2.2 Separate view types

There is no shared "assignment response" type with conditional fields. The
service defines distinct types:

| View | Audience |
| --- | --- |
| `ProviderAssignmentView` | The assigned provider |
| `ProviderMarketplaceOfferView` | A provider holding an offer recipient row |
| `DispatcherAssignmentView` | Organization staff with the relevant permission |
| `DispatcherMarketplaceRequestView` | Organization staff |
| `InternalMarketplaceRequestView` | Service-to-service callers |

A conditional field on a shared struct is one boolean bug away from a leak.
Separate types make the audience a compile-time fact.

### 2.3 Events and notifications

Provider-addressed events and notification payloads are constructed from the
same allowlists as the API views. A provider notification never carries a
price, and never carries a free-text body assembled from source data that
might contain one.

### 2.4 Logs

Commercial values are not logged on provider-facing code paths. Structured
log fields on those paths are limited to identifiers, statuses and timings.

## 3. Settlement identity is separate from executing identity

Execution requests to a source service carry two deliberately distinct fields:

- `SettlementOrganizationID` — who the platform settles with
- `ExecutingProviderID` — who performs the work

Collapsing these into one field would either bill the provider or leak the
organization's settlement relationship into provider-facing data. They are
never merged, and `SettlementOrganizationID` never appears in a provider view.

## 4. Testing

- **Leakage tests** serialize every provider-facing view, event and
  notification payload produced by the test suite and assert that no forbidden
  key appears anywhere in the JSON — including nested objects and arrays.
- The assertion is on **key absence**, so a zeroed or nulled field fails.
- **Architecture tests** fail the build if a pricing or settlement token
  appears in a provider-facing package.
- Tests run against realistic payloads with commercial fields populated
  upstream, so a passing result means the boundary actually held rather than
  the data happening to be empty.

## 5. Reviewer checklist

- [ ] New provider field added to an explicit allowlist, not inherited
- [ ] No provider DTO built by deleting keys from a larger payload
- [ ] No `map[string]any` passthrough of source or internal payloads
- [ ] No shared struct serving both provider and dispatcher audiences
- [ ] Provider notifications and events use the same allowlists as the API
- [ ] No commercial value logged on a provider path
- [ ] Leakage test updated to cover the new payload

See also [`universal-accept-decline.md`](universal-accept-decline.md) and
[`platform-boundaries.md`](platform-boundaries.md).
