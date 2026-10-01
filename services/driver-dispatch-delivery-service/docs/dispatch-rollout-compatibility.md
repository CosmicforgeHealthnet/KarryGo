# Dispatch rollout and compatibility

This is the operational contract for activating the capabilities already
implemented in Dispatch Sections 4–19. It does not enable a capability. The
checked-in defaults remain the legacy-compatible state.

## Modes and server-owned cohorts

- Serviceability/package: both flags false = `OFF`; exactly one true =
  `OBSERVE_ONLY`; both true = `ENFORCED`.
- Sequential matching: the enable flag admits the capability. When
  `DISPATCH_ASSIGNMENT_ROLLOUT_SERVICE_TIERS` is configured, other tiers fall
  back to `BROADCAST_FIRST_ACCEPT`.
- Secure handover: at least one customer, source-type, or source-ref allowlist
  must be configured. An optional service-tier allowlist narrows the selected
  cohort. These values are deployment-owned; request JSON is not authority.
- Organization dispatch remains an explicit strategy and still requires Fleet
  membership and provider eligibility. It does not change marketplace rank.
- Dispatch does not currently have a separate assignment-zone selector.
  Geographic eligibility is enforced by the serviceability/zone authority;
  this document does not claim unsupported arbitrary zone rollout.

## Feature matrix

| Capability | Actual configuration | Default | Dependencies | Disable behavior |
|---|---|---:|---|---|
| Route/package decision | `DISPATCH_ROUTE_SERVICEABILITY_ENABLED`, `DISPATCH_PACKAGE_POLICY_ENABLED` | false/false | route and zone adapters for useful decisions | off or observation; historical quotes remain |
| Sequential assignment | `DISPATCH_SEQUENTIAL_MATCHING_ENABLED` | false | provider eligibility, Trip adapter; optional tier cohort | new work broadcasts; accepted winners drain to Trip |
| Queue | `DISPATCH_QUEUE_ENABLED` | false | an enabled assignment strategy, Queue schema/worker/deadlines, Wallet adapter | no new Queue admission; durable entries pause, requested financial outcomes drain |
| Organization assignment | `DISPATCH_ORGANIZATION_ASSIGNMENT_ENABLED` | false | Fleet URL/caller/secret and membership authority | no new organization offers; accepted winner remains valid |
| Admin-assisted assignment | `DISPATCH_ADMIN_ASSISTED_ASSIGNMENT_ENABLED` | false | trusted admin HMAC, reason, version, provider eligibility | no new admin assignment |
| Multi-stop planning | `DISPATCH_MULTI_STOP_MISSIONS_ENABLED` | false | Vendor caller, route authority, manifest/custody, selected protected handover cohort, pickup and delivery evidence | no new plans; existing persisted plan remains |
| Assignment kill switch | `DISPATCH_ASSIGNMENT_KILL_SWITCH` | false | none | blocks new advanced assignment; drain worker reconciles winners/Wallet only |
| Secure handover | `DISPATCH_HANDOVER_CHALLENGE_ENABLED` | false | server cohort, handover key, notification readiness for activation | no new routes/admission; protected Trips fail closed; maintenance drains |
| Pickup evidence | `DISPATCH_PICKUP_EVIDENCE_GATE_ENABLED` | false | secure handover and media authority | persisted requirement remains for protected work |
| Delivery evidence | `DISPATCH_DELIVERY_EVIDENCE_GATE_ENABLED` | false | secure handover and media authority | persisted requirement remains for protected work |
| Handover QR | `DISPATCH_HANDOVER_QR_ENABLED` | false | secure handover | no new QR credential; OTP contract remains where allowed |
| Delivery recovery | `DISPATCH_DELIVERY_RECOVERY_ENABLED` | false | recovery persistence and operations ownership | no new recovery route; existing durable state needs operations review |
| Return | `DISPATCH_RETURN_WORKFLOW_ENABLED` | false | recovery, sequential assignment, route service | no new return action; existing state is not rewritten |
| Hub Hold | `DISPATCH_HUB_HOLD_ENABLED` | false | recovery and configured approved hubs | no new Hub Hold; existing state is not rewritten |
| Replacement handover | `DISPATCH_REPLACEMENT_HANDOVER_ENABLED` | false | recovery, sequential assignment, secure handover, Fleet/provider eligibility | insecure replacement is unavailable; protected custody pauses |
| Route alerts | `DISPATCH_ROUTE_ALERTS_ENABLED` | false | route-service URL and operations response | stops new alert evaluation; history remains |
| Offline replay | `DISPATCH_OFFLINE_REPLAY_ENABLED` | false | route alerts; server versions/idempotency remain authoritative | locally queued client actions receive normal retry/conflict; no offline authority |
| Vendor lifecycle delivery | `DISPATCH_COMMERCE_EVENT_DELIVERY_ENABLED` | false | Vendor URL/secret/consumer version | outbox remains durable; delivery pauses |
| Fleet relay/event delivery | `DISPATCH_FLEET_RELAY_ENABLED`, `DISPATCH_EVENT_DELIVERY_ENABLED` | false | Fleet URL/secret/consumer version | delivery pauses; durable outbox is retained |
| Analytics delivery | `DISPATCH_ANALYTICS_EVENT_DELIVERY_ENABLED` | false | Analytics URL/secret | Dispatch execution is unaffected; outbox delivery pauses |

Configuration rejects evidence or QR without handover, replacement without
recovery/sequential/handover, Queue without an assignment strategy, offline
replay without route alerts, route-dependent operations without a route URL,
Hub Hold without approved hubs, and multi-stop without a protected
`VENDOR_COMMERCE` cohort plus both evidence gates.

## Protected-work policy

Migration `000054_booking_rollout_policy_pinning` adds the immutable Booking
snapshot: mode, policy version, handover, pickup evidence, delivery evidence,
and provider-transfer requirements. Customer, legacy CommerceMission,
multi-stop Commerce, and mission-planning Booking creation write it at
admission. Provider replacement and Return workflows inherit the original
Booking snapshot, so retry/recovery cannot downgrade protected custody. Trip
reads it for each security-sensitive transition. The database trigger rejects
any later mutation. Existing challenge rows also keep pre-migration in-flight
work protected.

Therefore an operator turning a flag off can stop new admission, but cannot
make protected work eligible for verbal confirmation, legacy proof completion,
or unverified custody transfer. If the protected dependency is unavailable,
the result is a controlled unavailable/conflict response and operations review,
not a legacy downgrade.

## Kill-switch operations

| Capability | New work | Active work | Fallback / operations action |
|---|---|---|---|
| Serviceability/package | stop enforcement or observe | persisted quote/version remains | legacy creation only when policy permits |
| Sequential/Queue | Broadcast/no new Queue | accepted winners and Wallet requests drain; other Queue work pauses | monitor holds and resume or cancel through the authoritative command |
| Multi-stop | stop new plans | stored plans/manifests remain immutable | pause before custody; do not synthesize legacy mission state |
| Handover/evidence | stop new protected admission/routes | security fail-closed; expiry/notification maintenance drains | operations review; never verbal completion |
| Recovery/Return/Hub/Replacement | stop new commands | durable state is retained | operations review/emergency authority; never bypass custody |
| Route alerts/offline replay | stop new evaluation/replay admission | historical records unchanged | canonical online Trip commands remain authoritative |
| Vendor/Fleet/Analytics delivery | stop delivery | durable outbox retained | monitor backlog, restore consumer, replay idempotently |

## Activation sequence

1. **Schema/read models:** deploy additive migrations through `000054`; leave
   all capability flags false.
2. **Observe only:** enable exactly one serviceability/package flag for a
   controlled deployment and compare `dispatch_serviceability_observe_total`.
3. **Low-risk customer compatibility:** certify idempotency keys, nullable read
   projections, and location freshness with old clients.
4. **Assignment cohort:** enable sequential and then Queue for selected service
   tiers. Keep Broadcast available.
5. **Secure handover cohort:** internal customer IDs, then selected
   Vendor/Partner source refs/types, then direct-customer cohorts. Verify
   notification, media, support, and operations first.
6. **Multi-stop Commerce:** only after Vendor, handover, evidence, manifest,
   parcel identity, custody, and canonical Trip smoke tests pass.
7. **Advanced operations:** route alerts, recovery, Return, Hub Hold, and
   replacement only after dashboards, runbooks, authorized commands, approved
   hubs, and emergency/support ownership exist.
8. **Legacy retirement:** future work only. Requires minimum client versions,
   Partner/Vendor/Fleet compatibility, cross-service smoke certification,
   protected-flow stability, rollback policy, and explicit approval.

## Cross-service deployment order

1. Deploy the reference-writing binary required by migration `000052`.
2. Drain every active legacy numeric Trip, Commerce stop, and provider-transfer
   credential. Confirm the `000052` guard query is empty. If secure handover
   was previously enabled globally, stop new admission and also drain or
   explicitly inventory every active pre-challenge Booking; an unrecorded
   historical flag decision cannot be inferred by a migration.
3. Apply Dispatch migrations `000052` through `000054`. `000054` must precede
   the Section 20 binary because the binary reads the pinned columns.
4. Deploy Dispatch with all new admission flags false.
5. Deploy compatible Notification, Media, Vendor, Fleet, API Gateway,
   Analytics, and Admin/Support consumers as applicable.
6. Run HMAC, idempotency, outbox, notification, and protected Trip smoke tests.
7. Activate one controlled cohort at a time in the sequence above.

Never apply `000052` while an active raw numeric credential exists. Its guard
intentionally aborts rather than invalidating live delivery credentials. The
down migration is schema rollback only; it cannot recreate deleted secrets.

## Operations prerequisites and monitoring

Before advanced operations, provide an authorized operations read/command
surface, escalation owner, reason-code policy, audit access, support runbook,
Wallet reconciliation view, Vendor/Fleet consumer status, and approved Hub
configuration. Suggested alerts should be based on observed baselines rather
than hardcoded business claims: handover verification failures, Queue wait,
assignment failures, Wallet unknown outcomes, notification/outbox backlog,
replacement backlog, and abnormal route-alert volume.

Do not use customer, provider, Booking, or Mission identifiers as metric labels.
The rollout counters use bounded mode/outcome/service-tier vocabularies.
