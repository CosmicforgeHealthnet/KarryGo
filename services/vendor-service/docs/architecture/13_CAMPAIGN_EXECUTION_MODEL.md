# Campaign execution model (Section 8F)

Section 8F executes an exact Section 8E `CURRENT` snapshot and immutable campaign version. Creating a run copies snapshot-eligible target/channel pairs into durable delivery rows; the snapshot is evidence, never permanent permission.

The scheduler claims due runs with `FOR UPDATE SKIP LOCKED`. Dispatchers claim due deliveries the same way. Before a handoff the dispatcher holds the profile row as the shared privacy/merge/frequency fence, verifies the run is still `RUNNING`, revalidates current marketing consent, checks current privacy/identity/service-recovery state, enforces quiet hours and the versioned frequency cap, and verifies channel availability. Only then does it call Notification Service using the delivery's stable idempotency key. A successful handoff and contact-history row commit together. Notification itself owns destination resolution and provider delivery.

Cancellation and pause lock the run. Pending/deferred/retryable deliveries become cancelled atomically when a run is cancelled; already handed-off messages cannot be recalled and remain historical facts. Privacy or consent that wins the profile lock suppresses the delivery; a handoff that wins commits before the later privacy change, so there is no handoff after that privacy transaction commits.

Delivery events are projected monotonically into append-only history. Vendor does not become provider authority and stores no raw callback payload. Run/delivery tables, audit rows, and domain outbox events carry opaque references only.

`VENDOR_CRM_CAMPAIGN_EXECUTION_ENABLED` stops new execution commands, scheduling, dispatch, and callback registration without hiding historical reads. The global commerce kill switch also stops workers. SMS is unavailable because Notification Service has no SMS channel contract.
