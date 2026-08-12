# Runbook: contract activation failure

## Symptom

`POST /fleet-partner/v1/contracts/{contractId}/activate` refuses.

## Activation revalidates everything

An approval is permission to activate a specific agreement, not a promise that
the world stood still. The customer may have been suspended, their risk may have
been escalated, a credit hold may have been applied and a signed document may
have been revoked, all since the reviewer said yes.

Every refusal below is that revalidation working.

## Diagnosis

| Error | Cause | Fix |
| --- | --- | --- |
| `contract_approval_required` | the contract is not `APPROVED` | submit it, and have the approval decided |
| `contract_activation_blocked` | the customer is not `ACTIVE` | move the customer to `ACTIVE` first |
| `customer_risk_restricted` | risk is `RESTRICTED` | reverse the risk decision through the risk workflow; do not work around it |
| `customer_credit_hold` | a blocking credit hold | release the hold, which needs its own approval |
| `contract_document_not_ready` | a mandatory document is not confirmed safe and ready | see below |
| `contract_version_conflict` | stale `expectedVersion` | re-read the contract and retry |

## Documents

Read the projection:

```
GET /fleet-partner/v1/contracts/{contractId}/documents
```

`usable` is true only when `statusProjection = READY` **and**
`readinessState = READY`.

| Projection | Meaning |
| --- | --- |
| `PENDING` | Media has not finished scanning |
| `UNSAFE` | Media rejected the file |
| `PENDING_REVIEW` | Media wants a human to look |
| `REVOKED` / `DELETED` | the object is gone or being deleted |
| `UNKNOWN` | **Media could not be asked** |

`UNKNOWN` blocks activation exactly as `UNSAFE` does. A document whose safety
could not be confirmed is treated as unsafe, because the alternative is bringing
an agreement into force on a file nobody has checked.

The most common cause of a persistent `UNKNOWN` is that **media-file-service is
unconfigured or unreachable** in this deployment. Check the media client
configuration and the integration kill switch.

Activation re-reads Media immediately before the transaction, so repairing the
connection is sufficient. Setting the projection to `READY` by hand will **not**
work — the pre-activation refresh overwrites it, which is the point.

## Approval mismatch

If the approval framework refuses with a payload mismatch, the contract's content
changed after it was approved. That is the binding working: the fingerprint
covers the effective dates, the service scope, the commercial snapshot, the
termination terms and the renewal terms.

Re-submit the contract to raise a fresh approval against the current content. Do
not attempt to re-point the old approval at new content.

## After a successful activation

An activated version is immutable, enforced by a database trigger. Any correction
from this point produces a **new version, a renewal, or a replacement contract**.

An UPDATE against `fleet_partner_contract_versions` will be refused, and that
refusal is the guarantee that the agreement in force is the one that was
approved.
