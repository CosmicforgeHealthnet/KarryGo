# Runbook: SLA UNKNOWN outcome

## Symptom

`sla_unknown_outcomes_total` is rising, or a service report shows observations
with outcome `UNKNOWN` or `INSUFFICIENT_DATA`.

## UNKNOWN is not a bug to be normalized away

An SLA that could not be measured **has not been met — it has not been
measured**. `UNKNOWN` and `INSUFFICIENT_DATA` are first-class outcomes and are
never collapsed into `MET`, anywhere: not in the evaluator, not in the worker,
not in the API and not in `organization.sla.outcome_recorded.v1`.

Reporting them as `MET` would let a monitoring outage read as perfect service.
That is the failure this design exists to prevent.

Do **not** "fix" an `UNKNOWN` by updating the row. Observations are append-only
and enforced by trigger; a corrected measurement arrives as a new row at a new
source version.

## Why an observation is UNKNOWN

| Cause | How to tell | Fix |
| --- | --- | --- |
| Version gap — measurements were missed | `source_version` jumped by more than 1 for that `source_reference` | repair the source's delivery; it re-sends the missing versions |
| Metric or unit could not be interpreted | the observation's `measurement_unit` does not match the SLA's metric | fix the source's payload |
| Older version arrived late | outcome is `NOT_EVALUATED` rather than `UNKNOWN` | nothing — it is recorded honestly and does not become the newest word |

`INSUFFICIENT_DATA` means the source reported no measured value at all. That is
usually correct: there was nothing in the window to measure.

Inspect:

```sql
SELECT source_type, source_reference, source_version, measurement_unit,
       outcome, window_start, window_end, created_at
FROM fleet_partner_sla_observations
WHERE sla_id = '<slaId>'
ORDER BY created_at DESC LIMIT 50;
```

Compare the unit against the SLA:

```
GET /fleet-partner/v1/service-level-agreements/{slaId}
```

Supported metric and unit pairs are listed in
[../contracts-and-slas.md](../contracts-and-slas.md). The list is closed: an SLA
whose unit does not fit its metric would produce `UNKNOWN` forever, which is why
both are validated at creation.

## No observations at all

1. Confirm the SLA is `ACTIVE`. A `DRAFT` or `SUSPENDED` SLA is not measured.
2. Confirm the source service is on the approved list —
   `driver-dispatch-delivery-service`, `driver-hauling-service`,
   `support-dispute-service`. Any other caller is refused with
   `sla_source_unknown`, however well signed its request is.
3. Confirm the source is sending to
   `POST /internal/fleet-partner/v1/crm/sla-observations` with a valid HMAC v2
   signature, a fresh timestamp and a nonce.
4. A `sla_not_found` response to the source means the organization ID in its
   payload does not own that SLA. The organization is checked against the SLA's
   actual owner and never trusted from the body.

## Conflicts

`sla_observation_conflict` means the source re-sent the **same version** with a
**different payload hash**. A source that changes its mind about a window it
already reported must say so with a new version. Fix the source; do not delete
the stored observation.

A same-version, same-hash re-send is a duplicate and returns the stored row with
`duplicate: true`. That is not an error.

## Escalation

A `MISSED` outcome raises a notification intent and, where a real contract
exists, a support-case intent. It creates **no penalty, no customer credit, no
provider deduction, no refund and no wallet journal** — the event carries
`financialEffect: "NONE"`.

Whether a missed SLA costs anybody money is a commercial decision made outside
this service. If somebody is asking why no credit was issued, that is why.
