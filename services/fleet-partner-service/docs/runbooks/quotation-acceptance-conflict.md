# Runbook: quotation acceptance conflict

## Symptom

A customer or a staff member reports that accepting a quotation failed with one
of:

- `quotation_acceptance_conflict`
- `quotation_version_conflict`
- `quotation_already_resolved`
- `quotation_expired`
- `quotation_token_invalid`

## These are working as designed

Every one of these is a refusal that protects a commitment already made. None
should be "fixed" by forcing the acceptance through.

## Diagnosis

| Error | Cause | Correct response |
| --- | --- | --- |
| `quotation_version_conflict` | the accepted version is not the issued one | the caller re-reads the quotation and accepts the current issued version |
| `quotation_already_resolved` | already accepted, rejected, cancelled, expired or superseded | read the quotation and tell the customer what happened |
| `quotation_expired` | `valid_until` has passed by **server** time | issue a new quotation; do not extend the old one |
| `quotation_acceptance_conflict` | the same idempotency key was reused with different content | the client is reusing a key across distinct requests |
| `quotation_token_invalid` | wrong quotation, unknown token, or revoked | see below |

## Idempotency key reuse

A retry with the **same** key and the **same** content replays and returns the
original acceptance with `replayed: true`. That is not an error.

A retry with the same key and **different** content is refused. This is almost
always a client generating one key per user session rather than one per request.
Fix the client; do not work around it by creating acceptances manually.

## A "double acceptance"

There cannot be one. `idx_fleet_crm_acceptance_quotation` is unique on
`quotation_id`, and the quotation transition guards on both the expected version
and the expected status, so two concurrent acceptances resolve to exactly one
winner and one conflict.

If you believe two exist, check — it is far more likely one request replayed:

```sql
SELECT id, accepted_version_id, acceptance_method, accepted_at
FROM fleet_partner_quotation_acceptances
WHERE quotation_id = '<quotationId>';
```

## A "missing" order

Exactly one conversion intent exists per accepted version, guaranteed by
`idx_fleet_crm_conversion_intent_version`.

Quotation acceptance itself does not create an order. It creates a durable
conversion intent, and the Section 15 `orders/from-quotation` command consumes
that intent exactly once through the normal order use case. If no order exists,
inspect the intent, the referenced customer/credit state, and the order
conversion response before retrying with the same request.

## Token problems

Every token rejection returns the same generic message on purpose, so the
distinction is only visible in the database:

```sql
SELECT status, expires_at, revoked_reason, view_count
FROM fleet_partner_quotation_access_tokens
WHERE quotation_id = '<quotationId>';
```

- `status = REVOKED` with a `revoked_reason` of `accepted`, `rejected`,
  `CANCELLED` or `SUPERSEDED` — the quotation was resolved. A retry of the same
  response replays; a genuinely new action is correctly refused.
- `status = CONSUMED` — a single-use link that has been used.
- `expires_at` in the past — issue a replacement quotation.

The raw token cannot be recovered. To restore customer access, supersede and
re-issue.

## Rate limiting

`429` on the public routes means more than 20 attempts per minute came from one
address. If a legitimate customer is hitting it they are probably behind a shared
NAT; confirm with the correlation IDs before changing anything. The limit is what
stops a secure link being brute-forced.
