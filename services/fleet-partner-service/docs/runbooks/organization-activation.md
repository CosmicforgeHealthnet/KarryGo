# Runbook — Organization stuck before activation

## Symptom

An organization has completed onboarding but never becomes `ACTIVE`, or
`fleet_partner_organization_activation_blocked_total` is climbing.

## First: read the gates

Activation is evaluated, never commanded. Ask the evaluator what is failing:

```text
GET /fleet-partner/v1/organizations/{organizationId}/onboarding-status
```

The response lists every stage with its status, missing fields and blocking
issues, plus verification and wallet-provisioning state.

The gates, in the order they are checked:

| Gate | Passes when |
| --- | --- |
| `organization_under_review` | status is `UNDER_REVIEW` |
| `onboarding_stages_complete` | every required stage is `COMPLETE` |
| `verification_approved` | the authoritative service approved it |
| `organization_wallet_active` | `payment-wallet-service` confirmed an account |
| `active_default_branch` | at least one `ACTIVE` branch exists |
| `settings_valid` | settings exist with a sane assignment timeout |

## Common causes

**Still `BUSINESS_SUBMITTED`, not `UNDER_REVIEW`.** The submission was
recorded but the dispatch to verification failed or timed out. Check:

```sql
SELECT status, submission_version, synced_at
FROM fleet_partner_organization_verification_requests
WHERE organization_id = $1 ORDER BY submission_version DESC LIMIT 1;
```

`PENDING_DISPATCH` or `UNKNOWN_OUTCOME` means the reconciler will retry by
read-back. The verification contract is not implemented yet (see the audit),
so this is expected until it is wired.

**Wallet not `ACTIVE`.** Check:

```sql
SELECT status, attempts, last_error_code
FROM fleet_partner_organization_wallet_provisioning WHERE organization_id = $1;
```

`RECONCILIATION_REQUIRED` means an unknown outcome — the reconciler reads
back rather than re-requesting. `FAILED_RETRYABLE` retries on schedule. Force
one with `POST /organizations/{id}/wallet-provisioning/retry`.

## Do not

- Do not update `organizations.status` directly. Every transition must go
  through the use case so history, audit and the outbox stay consistent; a
  manual `UPDATE` silently breaks all three.
- Do not mark verification approved locally. That decision belongs to
  `verification-compliance-service`.
- Do not insert a placeholder `wallet_account_id` to clear the gate.
