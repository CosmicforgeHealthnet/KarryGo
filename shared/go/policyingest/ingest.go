package policyingest

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/events"
)

// Store is the per-service persistence ingestion needs.
//
// Every method is scoped to one service's own schema, because each service
// owns its snapshots. What the shared package owns is the DECISION order, not
// the tables -- that is the part that must not diverge between Dispatch and
// Hauling, and the part that is easy to get subtly wrong.
type Store interface {
	// ClaimEvent records the event_id and returns false when it was already
	// recorded. It must be atomic: two concurrent deliveries of the same
	// publication must see exactly one true.
	ClaimEvent(ctx context.Context, record EventRecord) (claimed bool, err error)

	// AppliedSourceVersion returns the highest source_version already applied
	// for a policy kind, and whether any exists.
	AppliedSourceVersion(ctx context.Context, policyKind string) (int64, bool, error)

	// ApplySnapshot stores the immutable snapshot AND marks the event applied,
	// in one transaction.
	//
	// The two must be atomic or the pair can disagree: an applied snapshot
	// with an unmarked event would be re-applied on the next delivery, and a
	// marked event with no snapshot would be skipped forever.
	ApplySnapshot(ctx context.Context, snapshot Snapshot) error

	// MarkTerminal records a no-op or rejection outcome against the event.
	MarkTerminal(ctx context.Context, eventID string, outcome Outcome, reasonCode string) error

	// MarkPendingReadBack records that the document could not be fetched and
	// the result is unknown, so a worker can retry.
	MarkPendingReadBack(ctx context.Context, eventID, reasonCode string, nextAttemptAt time.Time) error

	// PendingReadBacks returns publications awaiting a retryable read-back.
	PendingReadBacks(ctx context.Context, now time.Time, limit int) ([]EventRecord, error)
}

// EventRecord is the inbox row for one publication.
type EventRecord struct {
	EventID       string
	PolicyKind    string
	PolicyVersion string
	SourceService string
	SourceVersion int64
	SchemaVersion int
	Checksum      string
	DocumentRef   string
	CorrelationID string
	OccurredAt    time.Time
	EffectiveAt   time.Time
	Attempts      int
}

// Snapshot is the immutable local policy version to store.
type Snapshot struct {
	EventID       string
	PolicyKind    string
	PolicyVersion string
	SourceVersion int64
	Checksum      string
	Document      json.RawMessage
	EffectiveAt   time.Time
	AppliedAt     time.Time
}

// DocumentReader fetches a referenced catalogue from the control plane.
//
// It returns three states rather than two, and that is the whole point: err
// with retryable=true means "we do not know whether Admin served this", which
// must NOT be recorded as a rejection.
type DocumentReader interface {
	ReadDocument(ctx context.Context, ref string) (document json.RawMessage, retryable bool, err error)
}

// Options configure an Ingestor.
type Options struct {
	Store Store
	// KnownKinds are the policy kinds this service keeps. A publication for
	// any other kind is IGNORED, because Admin broadcasts one catalogue to
	// every authority and each takes only its own.
	KnownKinds []string
	// Reader fetches referenced documents. nil means this deployment accepts
	// inline publications only, and a referenced one is then pending rather
	// than rejected -- a missing client is a deployment gap, not a bad event.
	Reader DocumentReader
	// RetryBackoff is how long a failed read-back waits. Zero uses a default.
	RetryBackoff time.Duration
	Now          func() time.Time
	// OnApplied is called after a snapshot is committed, for metrics.
	OnApplied func(policyKind, policyVersion string)
}

// Ingestor applies publications to a service's local snapshot store.
type Ingestor struct {
	store        Store
	kinds        map[string]bool
	reader       DocumentReader
	retryBackoff time.Duration
	now          func() time.Time
	onApplied    func(string, string)
}

// New builds an Ingestor.
func New(options Options) *Ingestor {
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	backoff := options.RetryBackoff
	if backoff <= 0 {
		backoff = time.Minute
	}
	kinds := make(map[string]bool, len(options.KnownKinds))
	for _, kind := range options.KnownKinds {
		kinds[strings.ToUpper(strings.TrimSpace(kind))] = true
	}
	return &Ingestor{
		store: options.Store, kinds: kinds, reader: options.Reader,
		retryBackoff: backoff, now: now, onApplied: options.OnApplied,
	}
}

// Ingest applies one published catalogue event.
//
// The order of checks is deliberate and is the part worth reading:
//
//  1. envelope and schema, because an unparseable event can be rejected
//     without touching any store;
//  2. policy kind, because ignoring another authority's catalogue must not
//     consume an event_id this service might later need;
//  3. event_id claim, so every later step happens at most once;
//  4. source_version, so a delayed publication cannot roll the network back;
//  5. document resolution, which is the only step that can be UNKNOWN;
//  6. checksum and pricing, on the document actually in hand;
//  7. the transactional apply.
func (i *Ingestor) Ingest(ctx context.Context, envelope events.Envelope) (Result, error) {
	if err := envelope.Validate(); err != nil {
		return Result{Outcome: OutcomeRejected, ReasonCode: ReasonMalformedDocument}, nil
	}
	if envelope.SchemaVersion != SchemaVersion {
		// Refused, not best-effort parsed. A control-plane document half
		// understood by an execution service is how a capability rule
		// silently stops being enforced.
		return Result{Outcome: OutcomeRejected, ReasonCode: ReasonUnsupportedSchema}, nil
	}

	var publication Publication
	if err := json.Unmarshal(envelope.Payload, &publication); err != nil {
		return Result{Outcome: OutcomeRejected, ReasonCode: ReasonMalformedDocument}, nil
	}
	if err := publication.Validate(); err != nil {
		return Result{Outcome: OutcomeRejected, ReasonCode: ReasonMalformedDocument}, nil
	}

	kind := publication.NormalizedKind()
	if len(i.kinds) > 0 && !i.kinds[kind] {
		// Not ours. Deliberately BEFORE the event claim: consuming an
		// event_id for a catalogue this service does not keep would make a
		// later, legitimate publication of the same id look like a replay.
		return Result{Outcome: OutcomeIgnored, ReasonCode: ReasonUnknownPolicyKind,
			PolicyKind: kind, PolicyVersion: publication.PolicyVersion}, nil
	}

	record := EventRecord{
		EventID: envelope.EventID, PolicyKind: kind,
		PolicyVersion: publication.PolicyVersion, SourceService: envelope.SourceService,
		SourceVersion: envelope.SourceVersion, SchemaVersion: envelope.SchemaVersion,
		Checksum: publication.Checksum, DocumentRef: publication.DocumentRef,
		CorrelationID: envelope.CorrelationID, OccurredAt: envelope.OccurredAt,
		EffectiveAt: publication.EffectiveAt,
	}
	claimed, err := i.store.ClaimEvent(ctx, record)
	if err != nil {
		return Result{}, err
	}
	if !claimed {
		return Result{Outcome: OutcomeDuplicate, ReasonCode: ReasonDuplicateEvent,
			PolicyKind: kind, PolicyVersion: publication.PolicyVersion}, nil
	}

	return i.complete(ctx, record, publication.Document)
}

// complete resolves the document and applies it. It is shared by the ingress
// path and the read-back retry worker, so a publication completed on retry
// goes through exactly the same checks as one completed on arrival.
func (i *Ingestor) complete(ctx context.Context, record EventRecord, inline json.RawMessage) (Result, error) {
	base := Result{PolicyKind: record.PolicyKind, PolicyVersion: record.PolicyVersion}

	applied, exists, err := i.store.AppliedSourceVersion(ctx, record.PolicyKind)
	if err != nil {
		return Result{}, err
	}
	if exists && record.SourceVersion <= applied {
		base.Outcome, base.ReasonCode = OutcomeStale, ReasonStaleSourceVersion
		return base, i.store.MarkTerminal(ctx, record.EventID, base.Outcome, base.ReasonCode)
	}

	document := inline
	if len(document) == 0 {
		document, err = i.readBack(ctx, record)
		if err != nil {
			var pending pendingError
			if errors.As(err, &pending) {
				// UNKNOWN, not failed. Admin may well have served this; the
				// publication is parked and retried rather than abandoned.
				base.Outcome, base.ReasonCode = OutcomeReconciliationRequired, pending.reason
				return base, i.store.MarkPendingReadBack(ctx, record.EventID, pending.reason,
					i.now().Add(i.backoffFor(record.Attempts)))
			}
			base.Outcome, base.ReasonCode = OutcomeRejected, ReasonReadBackRefused
			return base, i.store.MarkTerminal(ctx, record.EventID, base.Outcome, base.ReasonCode)
		}
	}

	if err := VerifyChecksum(document, record.Checksum); err != nil {
		// A corrupted policy is worse than an absent one: the first is
		// applied and believed. This is terminal -- refetching the same
		// reference would produce the same bytes.
		base.Outcome, base.ReasonCode = OutcomeRejected, ReasonChecksumMismatch
		return base, i.store.MarkTerminal(ctx, record.EventID, base.Outcome, base.ReasonCode)
	}
	if err := RejectPricingFields(document); err != nil {
		base.Outcome, base.ReasonCode = OutcomeRejected, ReasonPricingFieldRejected
		return base, i.store.MarkTerminal(ctx, record.EventID, base.Outcome, base.ReasonCode)
	}

	snapshot := Snapshot{
		EventID: record.EventID, PolicyKind: record.PolicyKind,
		PolicyVersion: record.PolicyVersion, SourceVersion: record.SourceVersion,
		Checksum: record.Checksum, Document: document,
		EffectiveAt: record.EffectiveAt, AppliedAt: i.now().UTC(),
	}
	if err := i.store.ApplySnapshot(ctx, snapshot); err != nil {
		return Result{}, err
	}
	if i.onApplied != nil {
		i.onApplied(record.PolicyKind, record.PolicyVersion)
	}
	base.Outcome, base.ReasonCode, base.Applied = OutcomeApplied, ReasonApplied, true
	return base, nil
}

// pendingError marks a read-back whose outcome is unknown.
type pendingError struct{ reason string }

func (e pendingError) Error() string { return "policyingest: read-back outcome unknown: " + e.reason }

func (i *Ingestor) readBack(ctx context.Context, record EventRecord) (json.RawMessage, error) {
	if strings.TrimSpace(record.DocumentRef) == "" {
		// Claimed inline but arrived empty. Nothing to fetch and nothing a
		// retry could change.
		return nil, errors.New("publication carries no document")
	}
	if i.reader == nil {
		// No client configured is a DEPLOYMENT gap, not a bad event. Parking
		// it means wiring the client later completes the publication instead
		// of losing it.
		return nil, pendingError{reason: ReasonReadBackUnavailable}
	}
	document, retryable, err := i.reader.ReadDocument(ctx, record.DocumentRef)
	if err == nil {
		return document, nil
	}
	if retryable {
		return nil, pendingError{reason: ReasonReadBackUnavailable}
	}
	return nil, err
}

// backoffFor grows the wait with the attempt count, capped, so a control plane
// that is down for an hour is not hammered once a minute by every authority.
func (i *Ingestor) backoffFor(attempts int) time.Duration {
	backoff := i.retryBackoff
	for n := 0; n < attempts && backoff < time.Hour; n++ {
		backoff *= 2
	}
	if backoff > time.Hour {
		backoff = time.Hour
	}
	return backoff
}

// ReconcilePending retries publications whose read-back outcome was unknown.
//
// It asks the authority again rather than guessing, which is the only safe
// resolution: the alternative -- deciding locally that an unanswered fetch
// meant failure -- would abandon a policy version the network has moved to.
func (i *Ingestor) ReconcilePending(ctx context.Context, limit int) (reconciled int, err error) {
	if limit <= 0 {
		limit = 20
	}
	pending, err := i.store.PendingReadBacks(ctx, i.now().UTC(), limit)
	if err != nil {
		return 0, err
	}
	for _, record := range pending {
		if ctx.Err() != nil {
			return reconciled, ctx.Err()
		}
		result, completeErr := i.complete(ctx, record, nil)
		if completeErr != nil {
			return reconciled, completeErr
		}
		if result.Outcome.Terminal() {
			reconciled++
		}
	}
	return reconciled, nil
}

// RunReconcileWorker retries pending read-backs on an interval.
func (i *Ingestor) RunReconcileWorker(ctx context.Context, interval time.Duration, onError func(error)) {
	if interval <= 0 {
		interval = 2 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if _, err := i.ReconcilePending(ctx, 0); err != nil && onError != nil {
				onError(err)
			}
		case <-ctx.Done():
			return
		}
	}
}
