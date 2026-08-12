# Universal Accept / Decline

This document describes the provider decision rule for KarryGo corporate
logistics, how it is enforced, and which enforcement points would have to be
removed to break it. It is normative: any change that contradicts this
document is a defect, not a configuration choice.

## 1. The rule

Every offer or assignment presented to a provider carries **both** actions:

```json
"allowedActions": ["accept", "decline"]
```

A provider who is employed, sponsored, contracted, salaried, on shift, or
onboarded by the organization has exactly the same decision rights as any
other provider. The right to decline is a property of being the human doing
the work, not a permission the organization grants.

### 1.1 What cannot remove Decline

None of the following may narrow `allowedActions`, and none of them is
consulted when the field is computed:

| Not a valid reason to remove Decline |
| --- |
| Organization setting or feature flag |
| Branch or department policy |
| Operating mode (`CENTRALIZED`, `DELEGATED`, `HYBRID`) |
| Provider role, seniority or employment relationship |
| Service category (dispatch, taxi, hauling) |
| Dispatch mode, priority, or SLA tier |
| Subscription plan or contract terms |
| Prior decline count or acceptance rate |
| Being sponsored, salaried, or organization-funded |

There is no `allowDecline`, `declineEnabled`, `forceAccept`, `autoAccept` or
equivalent field anywhere in the schema, configuration, or API. Adding one
would be a defect.

### 1.2 No acceptance by silence

Acceptance requires an explicit, authenticated, provider-originated action.
None of the following is acceptance:

- The response deadline passing
- A notification being delivered, displayed, or read
- The provider being online, on shift, or idle
- Location or telemetry activity
- A company dispatcher, admin, or owner acting "on behalf of" the provider
- A retry, replay, or reconciliation of a prior request

When a deadline passes, the assignment moves to `EXPIRED` — never to
`ACCEPTED`. Expiry releases the provider's reserved capacity, opens a
reassignment case, and returns the decision to a human dispatcher.

### 1.3 Delegated offers

In `DELEGATED` and `HYBRID` mode a provider may accept an organization's
marketplace offer directly. Accepting is still one provider's own decision:
one provider declining their offer recipient row does **not** close the
organization's offer or answer for anyone else. Each recipient holds an
independent Accept/Decline choice until the offer is won, expired, or
withdrawn.

## 2. Where the rule is enforced

Enforcement is layered so that no single edit can quietly disable it.

### 2.1 Server-authoritative action computation

`allowedActions` is never taken from a client, a stored column, or a policy
lookup. It is computed from state alone:

```go
// internal/features/marketplace/models/models.go
func (a Assignment) AllowedActions(now time.Time) []string {
	if a.Status == AssignmentPendingProviderResponse && now.Before(a.ResponseDeadline) {
		return []string{"accept", "decline"}
	}
	return []string{}
}
```

The function takes no organization, no settings, no role and no mode. There is
no code path that returns `["accept"]`.

### 2.2 Decline consults no feature flag

`Service.DeclineAssignment` deliberately checks no enablement flag. Accepting
work can be disabled — for example while a source service is degraded — but
declining work a provider has already been offered can never be disabled,
because that would strip a provider of the only exit from an assignment they
cannot safely perform.

### 2.3 Database constraints

- `fleet_partner_managed_provider_policies` carries
  `CHECK (provider_decline_required = true)`. The column exists solely so the
  constraint can refuse to store a policy that disables declining.
- `fleet_partner_assignment_responses` has `UNIQUE(assignment_id)` and is
  append-only via trigger: exactly one response per assignment, and it can
  never be edited into a different answer afterwards.
- A decline row requires a reason code (`CHECK`), so declines cannot be
  recorded as unexplained state changes.

### 2.4 Build-enforced prohibitions

`internal/architecture_test.go` fails the build if forbidden tokens appear in
the service — including auto-acceptance and decline-suppression vocabulary.
A future contributor cannot introduce `autoAccept` without the test failing.

### 2.5 Tests

| Test | Guarantees |
| --- | --- |
| `TestAssignmentAlwaysOffersAcceptAndDecline` | Both actions while pending; none once resolved; a passed deadline never yields actions |
| `TestExactlyOneProviderResponsePerAssignment` | One response per assignment; responses immutable |
| `TestConcurrentProviderResponsesSingleWinner` | Six concurrent accepts and declines produce exactly one winner and one row |
| `TestOfferRecipientDeclineDoesNotCloseOrganizationOffer` | One provider's decline does not answer for another |
| `TestDeclineReasonCatalogueAndSafetyClassification` | Closed 15-reason catalogue, safety classification, follow-ups |
| `TestProviderDeclineRightCannotBeDisabled` | The policy constraint rejects a disabled decline right |

## 3. Decline reasons

The catalogue is closed — 15 reasons, validated server-side. Free text is
accepted only as an optional note (max 500 characters) and is required only
for `OTHER`.

### 3.1 Safety- and welfare-protected reasons

These seven never count against a provider, never feed a performance metric
that penalises them, and never trigger an automated sanction:

`ILLNESS`, `FATIGUE_OR_REST_REQUIRED`, `UNSAFE_VEHICLE`, `VEHICLE_DEFECT`,
`CARGO_OR_SAFETY_CONCERN`, `SECURITY_CONCERN`, `PERSONAL_EMERGENCY`

Declines for these reasons open a reassignment case flagged for human review
rather than an automatic retry loop, and safety reasons additionally raise a
typed follow-up (for example a vehicle inspection hold) so the concern is
acted on rather than logged.

`DEVICE_OR_NETWORK_ISSUE` is deliberately **not** a safety classification and
also not misconduct: it raises a support-telemetry follow-up only. A provider
with a bad connection is a support problem, not a disciplinary one.

### 3.2 Reasons and sensitivity

Health and personal-emergency reasons are marked sensitive. They are recorded
for the organization's operational need to reassign, but are excluded from
provider-facing analytics and from any export that is not access-controlled.

## 4. What the organization *can* do

The rule constrains the provider's decision, not the organization's. An
organization may still:

- Choose which providers to offer work to
- Set a response deadline within the configured bounds
- Reassign work after a decline, expiry, or cancellation
- Review decline patterns through access-controlled reporting
- Take offline HR action under its own employment agreements

What it cannot do is make the software accept on a provider's behalf.

## 5. Reviewer checklist

A change is a violation of this document if it:

- [ ] Adds any field, flag, column or setting that suppresses `decline`
- [ ] Makes `AllowedActions` depend on anything other than status and deadline
- [ ] Adds a flag check to `DeclineAssignment`
- [ ] Transitions an assignment to `ACCEPTED` from a timer, worker, or job
- [ ] Lets an organization user create a provider response record
- [ ] Relaxes `UNIQUE(assignment_id)` on responses or their append-only trigger
- [ ] Removes `CHECK (provider_decline_required = true)`
- [ ] Treats a decline reason as a sanction trigger

See also [`marketplace-assignments.md`](marketplace-assignments.md) for the
assignment lifecycle and [`provider-data-minimization.md`](provider-data-minimization.md)
for what providers are and are not shown.
