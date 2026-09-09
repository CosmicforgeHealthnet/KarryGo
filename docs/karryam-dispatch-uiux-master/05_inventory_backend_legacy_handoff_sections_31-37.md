# 31. Master Design Inventory and Expected Scale

The design should not be managed by chasing an arbitrary frame count. The completion criterion is coverage: every supported provider intent and every materially different server/permission/offline state must have a designed UX. The inventory above defines the minimum logical templates; state variants, overlays and permission/error/recovery frames expand the actual canvas considerably.

| Domain | Minimum logical templates | Expected state depth |
| --- | --- | --- |
| Launch, Authentication and Session | 9 | 3-8 materially different states per major interactive template |
| Onboarding and Provider Setup | 10 | 3-8 materially different states per major interactive template |
| Identity, Face, Licence and Readiness Verification | 15 | 3-8 materially different states per major interactive template |
| Vehicle and Vehicle Documents | 9 | 3-8 materially different states per major interactive template |
| Home, Availability and Location | 8 | 3-8 materially different states per major interactive template |
| Opportunities and Assignment Sources | 8 | 3-8 materially different states per major interactive template |
| Activity, Trips and History | 7 | 2-6 materially different states per template |
| Mission Core - Pickup | 8 | 3-8 materially different states per major interactive template |
| Mission Core - Delivery | 9 | 3-8 materially different states per major interactive template |
| Multi-stop Missions, Manifest and Parcels | 7 | 2-6 materially different states per template |
| Secure Handover - OTP and QR | 8 | 3-8 materially different states per major interactive template |
| Evidence and Proof | 6 | 2-6 materially different states per template |
| Delivery Recovery | 10 | 3-8 materially different states per major interactive template |
| Breakdown, Return, Hub Hold and Provider Transfer | 8 | 3-8 materially different states per major interactive template |
| Route Integrity and Offline Replay | 8 | 3-8 materially different states per major interactive template |
| Wallet and Earnings | 6 | 2-6 materially different states per template |
| Bank Accounts | 6 | 2-6 materially different states per template |
| Withdrawals and Transaction PIN | 9 | 3-8 materially different states per major interactive template |
| Notifications | 5 | 2-6 materially different states per template |
| Account, Profile and Preferences | 8 | 3-8 materially different states per major interactive template |
| Phone, Privacy and Security | 6 | 2-6 materially different states per template |
| Support Cases and Evidence | 8 | 3-8 materially different states per major interactive template |
| Support Chat | 4 | 2-6 materially different states per template |
| Safety and SOS | 6 | 2-6 materially different states per template |
| Global Sheets, Dialogs and Microflows | 18 | 3-8 materially different states per major interactive template |

The current master inventory defines 206 minimum logical templates/overlays. Once loading, empty, validation, permission, rejection, expiry, offline, conflict, processing and success states are represented, the production design workspace should reasonably contain several hundred visual frames/states. This is expected and is more representative of the real product than the earlier 78-screen V1.

# 32. Backend-to-UX Contract Rules

## 32.1 Dispatch mission state

- Trip statuses include assigned, en_route_pickup, arrived_pickup, in_progress, proof_submitted, completed, cancelled and failed.
- Canonical mission detail may contain mission state/version, ordered stops, manifest reference/version/status, parcels, allowed_actions, handover state, recovery state and route integrity state.
- UI actions must be generated from canonical state/allowed actions rather than assumed from a fixed sequence.
- After parcel custody, ordinary cancellation may be prohibited; return or secure transfer becomes the legal flow.
## 32.2 Idempotency and versioning

- Persist one idempotency key per user intent and reuse it when retrying the same intent.
- Version conflict: stop local progression, refetch canonical state, explain that the job changed.
- Manifest conflict: discard stale capture/scan context and refetch exact manifest state.
- Double taps must never mint multiple operation keys for the same visible intent.
## 32.3 Secure handover

- Stages: PICKUP, DELIVERY, PROVIDER_TRANSFER and RETURN.
- Challenge states: ACTIVE, CONSUMED, INVALIDATED, EXPIRED and LOCKED.
- Methods: OTP and QR. Protected flows fail closed.
- Challenge/credential UI must never expose sensitive values in ordinary notifications, logs, analytics or clipboard shortcuts.
## 32.4 Availability and location

- Availability states: online, offline, busy. Busy is system controlled and should not be a manual provider mode.
- Readiness blockers include not verified, account suspended, no verified vehicle, licence expired and fleet restricted.
- Location updates are rate limited and cached; UI needs degraded/stale-location states rather than pretending freshness.
## 32.5 Opportunity sources

- Four opportunity sources coexist: direct requests, assignment offers, organization offers and company assignments.
- Organization offer summaries may intentionally omit fare amount. Do not render a zero-value fare placeholder.
- Offer expiry and optimistic-version conflicts require dedicated “no longer available / refresh” UX.
## 32.6 Financial routes

- Provider wallet backend confirms earnings, bank accounts, withdrawals and transaction PIN.
- Organization/fleet finance backend also supports provider sub-wallets, allocations, withdrawals and statements.
- The provider UI must not invent fees, settlement schedules or balances that are not present in the response contract.
## 32.7 Support and SOS

- Provider routes support complaints, evidence, support chat, messages, read state and SOS.
- Provider dispute escalation must not be assumed from customer-only dispute routes; use support case/complaint terminology unless a separate provider escalation contract is verified.
- FAQs and support categories exist as backend-provided public/self-service surfaces.

# 33. Legacy / Product Decision Register

The following items appear in the current or historical frontend but conflict with, exceed or are not fully proven by the current audited provider contracts. They must remain visibly marked in design planning until resolved.

| Item | Current evidence | Design treatment | Decision needed |
| --- | --- | --- | --- |
| Password / change-password screens | Current provider auth is passwordless OTP; legacy frontend contains password-style screens. | Do not promote into final navigation. Keep on Legacy/Decisions board. | Confirm removal or identify current backend owner. |
| Email verification screens | Signup collects email, but standalone legacy email-verification flow is not established as required in current dispatch auth contract. | Keep as VERIFY only. | Confirm whether email challenge is product requirement. |
| Provider dispute escalation | Support service registers customer-only dispute escalation; provider gets complaint/chat/evidence/SOS. | Use Support Case terminology. | Confirm whether providers ever get formal dispute escalation. |
| Advanced incentives / bonuses / heatmaps | Not established in current audit. | Do not design as functional. | Product/backend scope required. |
| Live customer messaging | Not established as dispatch provider messaging contract. | Do not invent chat with customer. | Confirm communications service/phone masking contract. |
| System/Light/Dark tri-state | Profile settings currently expose dark-mode boolean. | Design simple dark-mode toggle unless persistence model changes. | Confirm whether system mode becomes backend/local-only setting. |
| Call masking | Receiver phone may exist in trip data, but masking/telephony contract is not established. | Use abstract contact action and privacy notes. | Confirm production call/masking integration. |

# 34. Design Workspace Architecture

Whether the team works in Figma, Penpot or a Flutter/Widgetbook code-first catalog, the information architecture should remain stable. Avoid a single giant undifferentiated board. Organize by product domain, then by happy path, alternate states, errors/recovery and engineering notes.

- 00 Cover & audit notes
- 01 Foundations
- 02 Components
- 03 Launch & Auth
- 04 Onboarding
- 05 Account Setup
- 06 Identity & Verification
- 07 Vehicle
- 08 Home & Availability
- 09 Opportunities
- 10 Activity / Trips
- 11 Mission - Pickup
- 12 Mission - Delivery
- 13 Multi-stop & Parcels
- 14 Secure Handover
- 15 Evidence
- 16 Delivery Recovery
- 17 Returns / Hub Hold / Transfer
- 18 Route Integrity & Offline
- 19 Wallet
- 20 Earnings & Transactions
- 21 Bank Accounts
- 22 Withdrawals & PIN
- 23 Notifications
- 24 Account & Profile
- 25 Security & Privacy
- 26 Support & Cases
- 27 Support Chat
- 28 Safety & SOS
- 29 Global States & Bottom Sheets
- 30 Prototype Flows
- 31 Backend Contract Matrix
- 32 Legacy / Decisions / Deprecated
## 34.1 Board layout rule

- Row 1 - happy path / primary route templates.
- Row 2 - loading, empty, pending and alternate states.
- Row 3 - errors, restrictions, expiry, conflicts and recovery.
- Row 4 - bottom sheets, dialogs, permission education and confirmation overlays.
- Row 5 - backend contract annotations and developer notes.

# 35. Required Prototype Flows

1. New provider: launch → signup → OTP → account setup → identity/face/licence/vehicle → verification pending → ready → online.

2. Returning provider: launch → session restore → active mission restoration or home.

3. Go online: readiness check → permissions/location → online → opportunity received.

4. Direct opportunity: inbox/interrupt → detail → accept → mission created → pickup.

5. Organization/company assignment: source-specific offer → accept/decline → canonical mission.

6. Pickup custody: arrive → pickup credential/scan/evidence → manifest confirmed → start mission.

7. Delivery: navigate → arrive → OTP/QR → evidence/recipient confirmation → complete.

8. Multi-stop: active stop → scan parcels → complete stop → next stop → final completion.

9. Receiver unavailable: attempt → contact → grace period → recovery selection → retry/reschedule/return/hub/operations review.

10. Breakdown/replacement: report breakdown → replacement assigned → secure provider transfer → custody confirmed.

11. Return to sender: return initiated → navigate → verify sender → secure handover → evidence → return confirmed.

12. Offline: network loss → permitted action saved locally → sync queue → server confirmation or conflict recovery.

13. Wallet: wallet home → bank account setup → transaction PIN → withdrawal → receipt.

14. Support: help → create case → attach evidence → agent chat → resolved.

15. SOS: safety hub/shortcut → confirm SOS → send → authoritative confirmation.

# 36. Design QA and Engineering Handoff Requirements

## 36.1 Every frame must include

- Screen ID and canonical state name.
- Backend/source owner or “local-only” label.
- Entry condition and exit/action condition.
- Loading/error/offline/conflict notes where relevant.
- Data fields shown and explicit note when a value may be absent.
- Privacy sensitivity annotation for OTP/QR/receiver/financial/support data.
- Accessibility notes for non-standard interactions such as map, scanner, signature or SOS.
- Component references rather than detached one-off visual patterns.
## 36.2 Acceptance criteria for design completeness

1. All logical templates in Sections 6-30 are represented in the design workspace or explicitly marked Not Applicable by product decision.

2. All backend-supported opportunity, mission, custody, recovery, wallet and support flows have a happy path and materially distinct failure/recovery states.

3. No legacy password/dispute/communications feature is silently promoted into production UX without a verified contract.

4. Every active mission state can be resumed after app restart or tab navigation.

5. Every destructive or financial action has a clear confirmation and authoritative result state.

6. Offline queued operations are visually distinct from server-confirmed operations.

7. Secure handover and manifest conflicts fail closed.

8. Permissions are educated before request and recoverable after denial.

9. Critical operational actions meet mobile accessibility and outdoor readability standards.

10. Engineering can map every major frame to a controller/API/state without guessing product behavior.

# 37. Final Design Verdict

| Master scope The earlier 78-screen Figma should be treated as V1 reference material only. The audited product requires a much broader design system covering at least 206 logical screen/overlay templates before state expansion. The final workspace will naturally contain several hundred visual frames once operational variants, permission states, failures, conflicts and recovery paths are shown. |
| --- |

The redesign should therefore begin from this master specification rather than by polishing the existing 78 frames individually. Build the foundation and component system once, then populate each product-domain board in sequence. Backend contracts remain the functional authority; the old design may be used as a visual/coverage benchmark when accessible, but it must not override newer backend capabilities or current implementation truth.
