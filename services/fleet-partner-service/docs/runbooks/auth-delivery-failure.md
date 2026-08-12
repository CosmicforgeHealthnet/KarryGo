# Runbook — OTP / invitation delivery failure

## Symptom

Users report they never receive a sign-in code, or
`fleet_partner_otp_delivery_failures_total` is climbing. Callers see HTTP 503
`otp_delivery_unavailable`.

**The API is behaving correctly.** It deliberately refuses to report success
for an OTP it could not send. Nobody is locked out silently.

## Triage

1. Confirm the metric and its rate:
   `fleet_partner_otp_delivery_attempts_total` vs
   `fleet_partner_otp_delivery_failures_total`.
2. Check the affected challenges:

```sql
SELECT delivery_status, count(*)
FROM fleet_partner_auth_otp_challenges
WHERE created_at > now() - interval '30 minutes'
GROUP BY delivery_status;
```

`UNAVAILABLE` means the send was attempted and refused or unreachable.

3. Check the security events (hashed contacts only, no raw addresses):

```sql
SELECT count(*) FROM fleet_partner_auth_security_events
WHERE event_type = 'otp_delivery_failed' AND occurred_at > now() - interval '30 minutes';
```

## Likely causes

| Cause | Check | Fix |
| --- | --- | --- |
| `notification-service` down | its `/health` | restore that service |
| Base URL or secret missing | `FLEET_PARTNER_NOTIFICATION_SERVICE_BASE_URL` / `_SECRET` set? | supply configuration and restart |
| Service HMAC secret rotated on one side only | signature rejections in notification-service logs | re-align the shared secret |
| Template missing | notification-service template registry | register `corporate_auth_otp` / `corporate_invitation` |

## Recovery

No data repair is needed. Challenges that were never delivered simply expire
and are swept by the maintenance worker. Once delivery is restored, users
retry `POST /auth/start` and get a fresh code.

For invitations, use `POST /staff/invitations/{id}/resend` (or the provider
equivalent) after service restoration — it rotates the token and re-sends.

## Do not

- Do not read an OTP out of the database to give to a user: only the keyed
  digest is stored, and it is not reversible by design.
- Do not add a logging or console OTP sender "temporarily" — that path does
  not exist and must not be introduced (build-enforced).
- Do not disable the 503 and return 200; a user must never be told a code was
  sent when it was not.
