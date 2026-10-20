# Dispatch handover state read model

Implementation: tested, default disabled, production BLOCKED_POLICY.
Source contract: `dispatch.admin-handover-state.v1`.
Permission: `dispatch.handover.read`.

Native reads use `/internal/dispatch/v1/admin/resources/handover-states` and its
`/{opaque-row-id}` detail. The private Bridge uses
`/internal/karryam-admin/v1/dispatch/resources/handover-states` and detail. Existing
separate source/Bridge resource read flags and credentials apply. A dedicated
signed governance rule and independently signed current source binding are both
required; the broad `dispatch.admin-resources.v1` approval cannot enable this view.
No production rule, role, scope tuple or owner signature is provided.

The projection returns an opaque row identifier, stored status, stage,
notification state, native creation/update timestamps and parent Trip reference.
It excludes challenge_ref, context key, OTP/QR hashes and encrypted credentials,
recipient references/contact destinations, attempts, retry budgets, operation
fingerprints, location and evidence references. It grants no challenge issuance,
resend, verification, override, custody completion or evidence download authority.
Stored ACTIVE is an observation, not a statement that a credential is valid now.
The full source version remains null: native notification leasing can change
notification state without incrementing optimistic_version.

Trip planning organization is the canonical tenant; settlement is never used.
The challenge Booking must match the actual Trip and its Customer. Legacy Trip
bindings must match the native trip/stop/manifest convention. Planned handovers
must match the actual mission Trip and Booking, group, stop identity/type and
exact captured manifest reference/version. Optional group Trip/Booking/Customer/
organization values must not contradict the Trip. A newer active manifest does
not replace the original frozen version in a historical handover. Contradictory
links disappear from reads and invalidate the signed binding. Market and zone
remain unknown until an independent owner allocation is approved.

Validation uses all actual Dispatch migrations, legacy and planned rows, signed
native HTTP output, independent wire schema, wrong tenant/old-contract denial,
foreign Booking/mission/stop/group checks, missing frozen manifest denial and a
later manifest version. The full source-authority suite runs all five owners;
mandatory Linux race CI must pass on the delivered head. Initial mapping, roles,
purposes, privacy classification/retention and source-contract approval remain
BLOCKED_POLICY. Richer handover/incident/remediation and case-bound Media remain
separate engineering work.
