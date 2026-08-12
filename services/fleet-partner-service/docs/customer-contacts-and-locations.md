# Customer contacts and locations (Section 14)

## Contacts are not identities

A contact is a named person at a customer. It holds **no credential**, grants no
access, and is never promoted into an account. It exists so a quotation can be
sent to somebody and an escalation can reach somebody.

### Normalization

Email and phone are normalized once, at write time, through the platform
`contact` package. A customer entered as `Ops@Example.COM ` and one entered as
`ops@example.com` are the same contact for deduplication, masking and delivery.

Each half is normalized **independently and only when supplied**. The platform's
`NormalizePair` requires both, because an authentication identity is an email
*and* a phone; a CRM contact is a person at a customer, and many of them are
reachable by exactly one of the two.

A contact with neither is refused unless its preferred channel is `NONE`.

### Channels

Preferences: `EMAIL`, `PHONE`, `SMS`, `WHATSAPP`, `IN_APP`, `NONE`.

Only `EMAIL` and `IN_APP` are reported as **deliverable**. SMS and WhatsApp are
accepted and stored as a stated preference and are never claimed as operational:
promising a delivery route that does not exist is worse than recording the
preference and routing to something that does. `ContactView.channelDeliverable`
tells a caller which it is.

### Consent

`UNKNOWN`, `GRANTED`, `WITHDRAWN`, `NOT_REQUIRED`, with an optional source and a
timestamp stamped when the state changes.

Consent is checked **before** capability at delivery time: an operator who has
not recorded consent has not earned the right to send, regardless of which
channels work. A `WITHDRAWN` contact is never contacted.

The notification worker resolves the address from the **live** contact record,
not from the queued payload, so a customer who withdraws consent between the
intent being recorded and the worker running is not contacted.

### One primary per purpose

The partial unique index on

```
(organization_id, customer_id, contact_purpose) WHERE is_primary AND archived_at IS NULL
```

is what makes two concurrent "make this primary" requests resolve to one winner
instead of two primaries. The demote-then-promote pair runs inside one
transaction; the index decides the race.
`TestCRMContactNormalizationPrimaryAndPrivacy` runs both promotions
concurrently and asserts exactly one survives.

Archiving a contact **clears the primary flag in the same statement**. Leaving an
archived contact holding the slot would make the index refuse every future
primary for that purpose, and the customer would silently lose the ability to
have one.

### Masking

`ContactView` masks the email to its first character plus domain, and the phone
to its last two digits. `SensitiveContactView` carries the real values and the
consent source, and is only ever built for a caller holding
`crm.sensitive.read`.

They are different Go types, so a handler that forgets the check cannot produce
the unmasked shape by accident.

## Locations

### Validation

- **Timezone** must be a loadable IANA zone. `UTC+1` and `WAT` are refused: they
  are offsets, and an offset cannot represent a DST boundary.
- **Country** must be ISO 3166-1 alpha-2.
- **Coordinates** are a pair or nothing. Half a coordinate is worse than none —
  a latitude with no longitude routes a vehicle to the prime meridian rather
  than failing loudly. Latitude is bounded to ±90 and longitude to ±180, in the
  domain and in a database CHECK.
- A **contact reference** must belong to the same customer. Allowing a location
  to point at another customer's contact would leak one customer's staff into
  another's operational instructions.

### Sensitive fields

`LocationView` carries the city, region, country and timezone. The **street
address, exact coordinates, access instructions and safety instructions are
absent — not masked, absent**. There is no field for them on that type.

`hasCoordinates`, `hasAccessInstructions` and `hasSafetyInstructions` tell a
caller whether asking with `crm.sensitive.read` would return anything.

Access instructions are gate codes and named contacts. Safety instructions are
hazards. Both are exactly the kind of operational detail that should not travel
in a list response somebody left open on a shared screen.

### Archiving

Archived locations are never deleted. Issued quotation versions and activated
contract versions snapshot what they needed at the time, and a historical record
pointing at a deleted row is a record with a hole in it.

An archived contact or location refuses further edits.

## Events

`organization.crm_contact.*` carries the contact ID, purpose, primary flag,
status and consent state — **no name, no email, no phone**.

`organization.crm_location.*` carries the location ID, type, country and
timezone — **no street address, no coordinates, no instructions**. The country
and timezone are enough for a routing-adjacent consumer to plan; anything finer
is operational detail behind a permission.

## Routes

| Method | Path | Permission |
| --- | --- | --- |
| GET | `/customers/{id}/contacts` | `crm.contacts.read` |
| POST | `/customers/{id}/contacts` | `crm.contacts.manage` |
| GET | `/customers/{id}/contacts/{contactId}` | `crm.contacts.read` |
| PATCH | `/customers/{id}/contacts/{contactId}` | `crm.contacts.manage` |
| POST | `/customers/{id}/contacts/{contactId}/archive` | `crm.contacts.manage` |
| POST | `/customers/{id}/contacts/{contactId}/make-primary` | `crm.contacts.manage` |
| GET | `/customers/{id}/locations` | `crm.locations.read` |
| POST | `/customers/{id}/locations` | `crm.locations.manage` |
| GET | `/customers/{id}/locations/{locationId}` | `crm.locations.read` |
| PATCH | `/customers/{id}/locations/{locationId}` | `crm.locations.manage` |
| POST | `/customers/{id}/locations/{locationId}/archive` | `crm.locations.manage` |

Section 15 may later snapshot locations onto orders. **No such snapshot is
created now.**
