# Secure quotation customer access (Section 14)

## The only unauthenticated CRM routes

A customer reaches an issued quotation through a **secure link**, not a session.
The token in the request is the entire authorization decision, and everything
else about the request is untrusted input.

```
GET  /fleet-partner/public/v1/quotations/{quotationId}/view?token=...
POST /fleet-partner/public/v1/quotations/{quotationId}/accept
POST /fleet-partner/public/v1/quotations/{quotationId}/reject
```

An `Authorization: Bearer <token>` header is preferred over the query parameter
because a URL ends up in browser history, proxy logs and referrer headers. The
query form is accepted because an emailed link cannot set a header; the token's
short life and single-quotation scope are what make that acceptable.

## The raw token exists once

`POST /fleet-partner/v1/quotations/{id}/issue` returns the raw token in its
response and **nowhere else**. What the database holds is an HMAC-SHA256 digest
keyed with the service session secret.

A database copy therefore cannot be replayed as the customer, and a query log or
slow-query trace cannot leak a working link. There is no endpoint that can
produce the raw value again — a caller that loses it must supersede the
quotation and issue a new one.

`TestCRMQuotationIssueAcceptanceAndConversionIntent` asserts the raw token is
absent from the database.

## Scope

A token is bound to:

- one organization
- one quotation
- one **issued version**
- an expiry, clamped to the quotation's own `valid_until` — a link that outlives
  the offer it points at is a link that invites an acceptance nobody meant to
  allow
- a scope: `VIEW_ONLY` or `VIEW_AND_RESPOND`
- optionally single-use

## Lookup is by digest alone

The token is found by its hash. The quotation ID in the request is compared
against what the token says **afterwards** and is never used to locate the row.

A token that names a different quotation must fail, and it cannot fail if the
query trusted the request to find it in the first place.

## Every rejection looks the same

"Wrong quotation", "no such token" and "revoked" all return
`quotation_token_invalid` with the same message. A caller must not be able to
tell them apart, because that difference is an oracle for probing which
quotations exist.

Only expiry is distinguishable, as `quotation_token_expired`, because a customer
with a genuinely stale link needs to be told to ask for a new one.

## Rate limiting

Token validation is rate limited to 20 attempts per minute **per client
address**. Without it a secure link is a 256-bit secret guarded only by
arithmetic.

The key is the address and not the token: keying by token would let an attacker
sidestep the limit by guessing a different token each time, which is exactly the
attack the limit exists to stop.

If the limiter is unavailable the route **refuses** with `503`. An unlimited
validation endpoint is a brute-force oracle, so refusing is the safe failure. If
no limiter is configured at all, the routes are not mounted.

## What the customer view contains

`CustomerQuotationView` is built field by field. It has **no field** for any of:

- internal notes
- approval records or approval history
- customer risk status
- credit limit or hold
- organization wallet information
- provider information
- platform commission or company margin
- staff identities
- Media object IDs or storage metadata
- token hashes, IP addresses or user agents

It cannot leak them however the underlying models change, because the type has
nowhere to put them. `TestCRMQuotationIssueAcceptanceAndConversionIntent`
asserts this against the serialized bytes.

The view renders the **immutable issued version**, not the live quotation: what
the customer sees is what they were offered, even if a draft revision has moved
on since.

## View transition

`ISSUED → VIEWED` happens on first view and is idempotent — a customer
refreshing the page does not produce a second transition, a second history row
or a second event.

The view history row records the **token record ID** as the actor. Never the raw
token, and never an IP address. `organization.quotation.viewed.v1` carries no IP
and no user agent either: a view event is "the customer opened it", not a session
record.

## Responding after responding

Accepting or rejecting revokes the link. A customer whose response was lost in
transit can still retry: the accept and reject paths resolve a revoked token
when the quotation is already terminal, which reaches the idempotent replay and
tells them what happened.

That is safe because it only ever reaches a replay — the quotation is terminal,
so every write path downstream refuses, and a retry carrying different content
still conflicts. The **view** path keeps the strict rule, because it has no
replay to reach.

## Acceptance evidence

An acceptance stores the client address and user agent as **keyed digests
only**, and neither is returned by any API or included in any event. An
acceptance record is evidence that somebody accepted, not a log of where they
were sitting.

The acceptance actor reference is the token *record* ID, not the raw token: an
evidence record containing a working credential is a liability.
