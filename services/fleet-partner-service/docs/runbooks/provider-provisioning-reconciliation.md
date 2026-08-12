# Runbook — Provider provisioning reconciliation

## Symptom

Managed providers stay in `INVITED` or `PENDING_PROVIDER_ACCEPTANCE`,
`fleet_partner_provider_provisioning_total{outcome="resolve_unavailable"}` or
`{outcome="provision_unknown"}` is climbing, or a company reports a provider
they added never received an invitation.

## Triage

```sql
SELECT overall_status, provider_resolution_status, provider_identity_status,
       association_status, subwallet_status, notification_status, count(*)
FROM fleet_partner_provider_provisioning_sagas
GROUP BY 1,2,3,4,5,6
ORDER BY count DESC;
```

| Saga state | Meaning |
| --- | --- |
| `RECONCILIATION_REQUIRED` | an external call had an unknown or unavailable outcome; the worker retries with read-back |
| `BLOCKED` + `provider_resolution_status = CONFLICT` | email and phone belong to different providers, or another organization already manages them — needs a human decision |
| `IN_PROGRESS` + `association_status = AWAITING_PROVIDER` | normal: waiting on the provider's own consent |

## The contracts are not implemented yet

Per the audit, the provider resolve / sponsored-provision / association
endpoints do not exist in the driver services. Until they do, every attempt
records `UNAVAILABLE` and the saga sits in `RECONCILIATION_REQUIRED`. This is
the intended fail-closed behaviour, not a defect: no provider is marked
active on an unverified assumption.

To enable: implement `platformclients.ProviderContract` against the real
endpoints and wire it in `app.BuildComponents` in place of
`UnconfiguredProvider`. Sagas then drain automatically.

## Unknown outcomes are resolved by read-back

The reconciler never blindly repeats a provisioning write — that could create
a duplicate provider identity. It re-resolves the contact pair; if the
identity now exists, the earlier write landed and the saga advances. The
same applies to sub-wallets (`ReadAccount` before any re-request).

## Conflicts need a human

`provider_identity_conflict` and `provider_already_managed` are terminal for
the saga. Resolve with the provider and the owning service (correct the
contact pair, or offboard the previous organization), then create a new
membership. Never merge identities to clear the conflict.

## Manual re-drive

Sagas are picked up automatically. To bring one forward immediately:

```sql
UPDATE fleet_partner_provider_provisioning_sagas
SET next_attempt_at = now()
WHERE membership_id = $1;
```

Or call `POST /providers/{membershipId}/provisioning/retry`.
