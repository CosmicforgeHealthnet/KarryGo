# Production operations

Entry: More > Production operations. Capacity availability is under Capacity; resource and calendar management is under Capacity > Manage capacity. The workspace now covers the workflows for all 54 public Production route patterns. This does not certify deployed Live behavior or complete capability-specific commerce.

## Jobs and handoff

The queue preserves backend promised-deadline/priority ordering, uses ACTIVE/READY/BLOCKED/COMPLETED/ALL filters and follows opaque X-Next-Cursor continuation in Live. No branch query is invented; the server applies staff scope. Details show planned/produced/waste, station steps and history.

Manual creation discovers actual branches and production lines. Saved reservation bookmarks contain references only; candidate reservations are read again and filtered to the selected branch/capability and active capacity before assignment. No order source, customer promise or frozen source evidence is fabricated. Customer-order activation is distinct from manual jobs and remains an Orders integration boundary.

Jobs support queue, start/resume, block, cancel, ready and complete. The named start/ready/complete routes are used for those actions. Required station steps must be COMPLETED or SKIPPED before READY; the server enforces this too. Produced and waste quantities are whole numbers whose total cannot exceed planned quantity.

READY records packaging_profile and packaging_evidence as nonempty objects, as required by the actual handler. The backend has no fixed child-field checklist or public evidence-object read route. A flexible detail editor accepts vendor-recorded labels/values; unchanged typed values are preserved on edit. Mock note records are fixtures; Live mode requires explicit entries. Do not treat these notes as verified certificates, media uploads or a platform-defined packaging checklist. The backend freezes ready evidence and returns its reference on the job.

Station steps support actual transitions, required flags, sequence/phase and version checks. Delay reports use the seven backend reasons, a bounded note and a new operational ready window; promised_ready_at is preserved. Failed evidence reads disable step editing.

## Capacity controls

- Pools: read/detail/create, edit actual mutable name/capacity/buffers, archive. Code/scope/unit are creation fields; the current update handler does not change them.
- Lines: read/detail/create/edit/archive, class, pool reference and setup/cleanup buffers.
- Rules: scoped pool/line, item family, exact maximum, ISO weekdays, optional local window/effective dates and specificity.
- Slots: create/edit, open/close/blackout/archive, actual datetime window and optional override.
- Templates: weekly schedule, optional dates and override, actual branch timezone, bounded horizon, edit/archive and explicit generation. Existing template resource assignment is preserved because the update handler changes schedule fields only.
- Blackouts: create branch/capability/pool/line windows using the backend reason codes; cancel active records. There is no public blackout detail route, so the detail screen refreshes the collection.
- Alerts: read, acknowledge OPEN, resolve unresolved records with actual version and optional bounded note. No alert creation/delivery authority is invented.

Forms use calendar/time pickers and weekday chips. References are read from accessible collections and filtered by the chosen branch/capability. Server denial disables the corresponding action.

## Reservations

Create from a pool detail, allocate positive quantities across same-scope active pools and optional lines/item families, preview and confirm saving. Zero hold duration creates a confirmed reservation; positive duration creates a temporary hold. Preview never guarantees capacity: saving rechecks it.

Details show actual allocation/release quantities, state, hold expiry and capacity consumption. Supported actions: confirm held capacity, reschedule its existing allocations to a new window, partial release and full release. The reschedule handler ignores request allocations, so the frontend does not offer an allocation replacement.

There is no public reservations-list route. Reservations are reached from actual job references, newly created results, a supplied reference or recently opened references. Bookmarks are scoped by mode/backend/vendor/staff, contain only IDs and are re-read from the server. They are not a server reservation directory.

The existing ReleaseInput had no JSON tags, which made the generated snake_case contract empty even though the service requires an expected version. Canonical expected_version/partial/reason tags, the OpenAPI request and the frontend contract now agree. This exposes existing full/partial release behavior without adding a new business feature.

## Recovery and modes

Every mutation freezes its body, parameters, original idempotency key and version before submission. Unknown outcome blocks new writes and offers the saved request. Reopening restores the same command. Conflict/forbidden responses require refresh; failed recovery blocks editing.

Mock supports mutable jobs/steps, ready/completion, delays, shared capacity resources, slots/templates/blackouts, alerts, reservations, holds/expiry, usage, release and reschedule. Retries replay successful results before version checks. Batch job references resolve actual same-branch nonterminal Mock jobs.

Mock calendar evaluation is a lightweight simulation, not the full server capacity algorithm. Template generation supports the supplied Africa/Lagos and UTC fixtures; other timezone generation fails explicitly. DST/general IANA scheduling, complex multi-day rule overlap and concurrent server locking remain authoritative backend behavior. Actual deployed Live verification remains pending.

## Validation

Two focused smoke checks pass: lost queue transition recovery and lost partial-release recovery with exactly-once effects, plus required-step gating and ready/completed job transitions. Focused static analysis covers the touched frontend files. Broad final tests and full deployed integration checks remain deferred.
