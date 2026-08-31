package production

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Actor is the authenticated vendor staff member behind a request. VendorID
// comes from the server-resolved session and never from the request.
type Actor struct {
	StaffID       string
	VendorID      string
	SessionID     string
	CorrelationID string
	CausationID   string
	CommandRef    string
}

// Scope is the actor's authorised reach for one permission.
type Scope struct {
	Global    bool
	BranchIDs []string
}

func (s Scope) Empty() bool { return !s.Global && len(s.BranchIDs) == 0 }

func (s Scope) Covers(branchID string) bool {
	if s.Global {
		return true
	}
	for _, id := range s.BranchIDs {
		if id == branchID {
			return true
		}
	}
	return false
}

// ScopeResolver answers "at which scopes does this staff member hold this
// permission". A port, so this feature never imports the staff repositories.
type ScopeResolver interface {
	ResolveScope(ctx context.Context, staffID, permission string) (Scope, error)
}

// AuditEntry mirrors the service-wide audit record shape.
type AuditEntry struct {
	ActorID, Action, AggregateType, AggregateID, VendorID, BranchID, Reason string
	Before, After                                                           any
}

type AuditRecorder interface {
	Record(context.Context, AuditEntry) error
}

// EventPublisher appends a domain event onto the transactional outbox inside
// the caller's transaction.
type EventPublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType, aggregateType, aggregateID string,
		aggregateVersion int64, vendorID, correlationID, causationID string, payload any) error
}

// PastryMilestoneBridge is an optional same-service adapter. Production owns
// job status; Pastry remains the authority for milestone state and commercial
// eligibility. The bridge is deliberately transaction-scoped so a mapped
// milestone cannot commit without the production transition that evidenced it.
type PastryMilestoneBridge interface {
	SyncProductionProgress(ctx context.Context, tx pgx.Tx, actor Actor, jobID, status string) error
}

// ETAReadinessProjector is the narrow same-service bridge to the Orders-owned
// customer ETA projection. Production supplies only its authoritative ready
// window; it never calculates delivery travel or confidence itself.
type ETAReadinessProjector interface {
	ProjectReadinessETA(ctx context.Context, tx pgx.Tx, vendorOrderID, parentOrderID string,
		windowStart, windowEnd *time.Time, delayed, actualReady bool, sourceEventID string,
		sourceVersion int64, occurredAt time.Time) error
}

// Flags gates the shared production platform and the two domains that use it.
type Flags struct {
	// ProductionEnabled gates the shared platform. With it false no production
	// route is registered at all, so a disabled platform has no attack surface
	// rather than routes that answer 503 and confirm it exists.
	ProductionEnabled bool
	// WritesEnabled gates every mutation; reads stay available so a rollout can
	// withhold changes without blinding operators.
	WritesEnabled bool
	// CommerceKillSwitch is the platform-wide stop. It can only ever withhold a
	// mutation, never permit one.
	CommerceKillSwitch bool
}

func (f Flags) writesAllowed() bool { return f.WritesEnabled && !f.CommerceKillSwitch }

// Service is the shared production application layer.
type Service struct {
	db     *pgxpool.Pool
	scopes ScopeResolver
	audit  AuditRecorder
	events EventPublisher
	pastry PastryMilestoneBridge
	eta    ETAReadinessProjector
	flags  Flags
	now    func() time.Time
}

func (s *Service) SetPastryMilestoneBridge(bridge PastryMilestoneBridge) {
	if s != nil {
		s.pastry = bridge
	}
}

func (s *Service) SetETAReadinessProjector(projector ETAReadinessProjector) {
	if s != nil {
		s.eta = projector
	}
}

// OrderActivationInput is the immutable operational snapshot supplied by the
// authoritative Orders activation point.  Production never looks up mutable
// catalogue data to construct a job.
type OrderActivationInput struct {
	VendorOrderID       string
	ReservationID       *string
	ProductionLineID    *string
	PromisedReadyAt     *time.Time
	ExpectedWindowStart *time.Time
	ExpectedWindowEnd   *time.Time
	ScheduledStart      *time.Time
	ScheduledEnd        *time.Time
	Priority            string
	Title               string
	SourceVersion       int64
}

// OrderActivationResult describes an exactly-once order-to-production link.
type OrderActivationResult struct {
	JobID      string `json:"job_id"`
	Created    bool   `json:"created"`
	Capability string `json:"capability"`
}

type Options struct {
	DB     *pgxpool.Pool
	Scopes ScopeResolver
	Audit  AuditRecorder
	Events EventPublisher
	Flags  Flags
	Now    func() time.Time
}

func NewService(opts Options) *Service {
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{db: opts.DB, scopes: opts.Scopes, audit: opts.Audit,
		events: opts.Events, flags: opts.Flags, now: now}
}

func (s *Service) FlagSet() Flags    { return s.flags }
func (s *Service) Now() time.Time    { return s.now() }
func (s *Service) DB() *pgxpool.Pool { return s.db }

// ActivateVendorOrder creates one queued ProductionJob for a paid, immutable
// VendorOrder.  The vendor order row is locked and the EXTERNAL_ORDER source
// identity is unique, so retries and concurrent callers converge on one job.
// Shop grocery allocations are deliberately ignored: they are not kitchen
// production.
func (s *Service) ActivateVendorOrder(ctx context.Context, actor Actor, in OrderActivationInput) (OrderActivationResult, error) {
	if s == nil || s.db == nil {
		return OrderActivationResult{}, apperrors.Unavailable("Production is not available.", nil)
	}
	if strings.TrimSpace(actor.VendorID) == "" || strings.TrimSpace(in.VendorOrderID) == "" {
		return OrderActivationResult{}, apperrors.BadRequest("Vendor order is required.", nil)
	}
	if in.Priority == "" {
		in.Priority = "NORMAL"
	}
	if !inVocabulary(Priorities, strings.ToUpper(in.Priority)) {
		return OrderActivationResult{}, validation(apperrors.FieldViolation{Field: "priority", Message: "Priority must be NORMAL, HIGH or URGENT."})
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return OrderActivationResult{}, err
	}
	defer tx.Rollback(ctx)
	var business, branch, domain, status, parentPayment, parentID string
	var orderVersion int64
	if err = tx.QueryRow(ctx, `SELECT v.business_id,v.branch_id,v.domain,v.status,v.version,p.payment_state,v.parent_order_id
		FROM commerce_vendor_orders v JOIN commerce_parent_orders p ON p.id=v.parent_order_id
		WHERE v.id=$1 AND v.business_id=$2 FOR UPDATE`, in.VendorOrderID, actor.VendorID).
		Scan(&business, &branch, &domain, &status, &orderVersion, &parentPayment, &parentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OrderActivationResult{}, errNotFound("Vendor order", CodeJobNotFound)
		}
		return OrderActivationResult{}, err
	}
	if parentPayment != "HELD" {
		return OrderActivationResult{}, coded(apperrors.Conflict("The order payment is not held.", nil), CodeInvalidTransition)
	}
	switch status {
	case "PAID", "SENT_TO_VENDOR", "ACCEPTED", "PREPARING":
	default:
		return OrderActivationResult{}, coded(apperrors.Conflict("The vendor order is not ready for production.", nil), CodeInvalidTransition)
	}
	capability, applicable := ProductionCapabilityForOrderDomain(domain)
	if !applicable {
		return OrderActivationResult{Capability: ""}, nil
	}
	var planned64 int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(SUM(quantity),0) FROM commerce_vendor_order_lines WHERE vendor_order_id=$1`, in.VendorOrderID).Scan(&planned64); err != nil {
		return OrderActivationResult{}, err
	}
	if planned64 < 1 {
		planned64 = 1
	}
	if planned64 > MaxCapacity {
		return OrderActivationResult{}, validation(apperrors.FieldViolation{Field: "planned_quantity", Message: "The production quantity is too large."})
	}
	planned := int(planned64)
	if in.PromisedReadyAt == nil {
		var commitment json.RawMessage
		if scanErr := tx.QueryRow(ctx, `SELECT delivery_commitment FROM commerce_vendor_order_lines WHERE vendor_order_id=$1 ORDER BY id LIMIT 1`, in.VendorOrderID).Scan(&commitment); scanErr == nil {
			var frozen struct {
				PromisedReadyAt     *time.Time `json:"promised_ready_at"`
				ExpectedWindowStart *time.Time `json:"expected_ready_window_start"`
				ExpectedWindowEnd   *time.Time `json:"expected_ready_window_end"`
			}
			if json.Unmarshal(commitment, &frozen) == nil {
				in.PromisedReadyAt = frozen.PromisedReadyAt
				if in.ExpectedWindowStart == nil {
					in.ExpectedWindowStart = frozen.ExpectedWindowStart
				}
				if in.ExpectedWindowEnd == nil {
					in.ExpectedWindowEnd = frozen.ExpectedWindowEnd
				}
			}
		}
	}
	if in.ReservationID != nil && strings.TrimSpace(*in.ReservationID) != "" {
		var reservationBusiness, reservationBranch, reservationStatus string
		if err = tx.QueryRow(ctx, `SELECT business_id,branch_id,status FROM capacity_reservations WHERE id=$1 FOR UPDATE`, strings.TrimSpace(*in.ReservationID)).Scan(&reservationBusiness, &reservationBranch, &reservationStatus); err != nil {
			return OrderActivationResult{}, err
		}
		if reservationBusiness != business || reservationBranch != branch || (reservationStatus != ReservationHeld && reservationStatus != ReservationConfirmed) {
			return OrderActivationResult{}, coded(apperrors.Conflict("The production capacity reservation is not usable.", nil), CodeCapacityUnavailable)
		}
	}
	// Resolve the configured workflow line from the production authority when
	// the trusted activation bridge did not provide one. Mutable catalogue data
	// is never consulted; absence simply leaves a generic job for an operator to
	// configure rather than fabricating a GRILL/FRYER/KITCHEN mapping.
	productionLineID := in.ProductionLineID
	if productionLineID != nil && strings.TrimSpace(*productionLineID) != "" {
		var lineBranch, lineCapability, lineStatus string
		if err = tx.QueryRow(ctx, `SELECT branch_id,capability,status FROM production_lines WHERE business_id=$1 AND id=$2`, business, strings.TrimSpace(*productionLineID)).Scan(&lineBranch, &lineCapability, &lineStatus); err != nil {
			return OrderActivationResult{}, err
		}
		if lineBranch != branch || lineCapability != capability || lineStatus != "ACTIVE" {
			return OrderActivationResult{}, coded(apperrors.Conflict("The configured production line is not usable for this order.", nil), CodeInvalidTransition)
		}
	} else {
		var configuredLine string
		if scanErr := tx.QueryRow(ctx, `SELECT id FROM production_lines WHERE business_id=$1 AND branch_id=$2 AND capability=$3 AND status='ACTIVE' ORDER BY code,id LIMIT 1`, business, branch, capability).Scan(&configuredLine); scanErr == nil {
			productionLineID = &configuredLine
		}
	}
	// Production receives no customer contact or address PII. Do not copy a
	// caller-provided display title into the operational aggregate.
	title := "Order " + in.VendorOrderID
	sourceVersion := in.SourceVersion
	if sourceVersion <= 0 {
		sourceVersion = orderVersion
	}
	jobID := newID("job_")
	var existing string
	err = tx.QueryRow(ctx, `INSERT INTO production_jobs(id,business_id,branch_id,capability,reservation_id,production_line_id,source_type,source_ref,source_version,title,status,priority,planned_quantity,scheduled_start,scheduled_end,promised_ready_at,expected_ready_window_start,expected_ready_window_end)
		VALUES($1,$2,$3,$4,$5,$6,'EXTERNAL_ORDER',$7,$8,$9,'QUEUED',$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (business_id,source_type,source_ref) WHERE source_type='EXTERNAL_ORDER' AND source_ref IS NOT NULL DO NOTHING
		RETURNING id`, jobID, business, branch, capability, in.ReservationID, productionLineID, in.VendorOrderID, sourceVersion, title, strings.ToUpper(in.Priority), planned, in.ScheduledStart, in.ScheduledEnd, in.PromisedReadyAt, in.ExpectedWindowStart, in.ExpectedWindowEnd).Scan(&existing)
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id FROM production_jobs WHERE business_id=$1 AND source_type='EXTERNAL_ORDER' AND source_ref=$2 FOR UPDATE`, business, in.VendorOrderID).Scan(&existing)
	}
	if err != nil {
		return OrderActivationResult{}, err
	}
	if created {
		if _, err = tx.Exec(ctx, `INSERT INTO production_job_events(id,business_id,job_id,event_type,to_status,note,actor_type,actor_id,correlation_id) VALUES($1,$2,$3,'CREATED','QUEUED','order activation','SYSTEM',NULLIF($4,''),NULLIF($5,''))`, newID("pje_"), business, existing, actor.StaffID, actor.CorrelationID); err != nil {
			return OrderActivationResult{}, err
		}
		if err = s.RecordMutation(ctx, tx, actor, "production.job.activated", "production.job.activated", "production_job", existing, branch, 1, nil, map[string]any{"status": "QUEUED", "source_ref": in.VendorOrderID}, map[string]any{"vendor_order_id": in.VendorOrderID, "capability": capability}, ""); err != nil {
			return OrderActivationResult{}, err
		}
	}
	if err = tx.QueryRow(ctx, `SELECT promised_ready_at,expected_ready_window_start,expected_ready_window_end FROM production_jobs WHERE business_id=$1 AND id=$2`, business, existing).Scan(&in.PromisedReadyAt, &in.ExpectedWindowStart, &in.ExpectedWindowEnd); err != nil {
		return OrderActivationResult{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO commerce_vendor_order_readiness(vendor_order_id,parent_order_id,business_id,branch_id,production_job_id,promised_ready_at,expected_window_start,expected_window_end,source_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (vendor_order_id) DO UPDATE SET expected_window_start=EXCLUDED.expected_window_start,expected_window_end=EXCLUDED.expected_window_end,source_version=GREATEST(commerce_vendor_order_readiness.source_version,EXCLUDED.source_version),updated_at=now()`, in.VendorOrderID, parentID, business, branch, existing, in.PromisedReadyAt, in.ExpectedWindowStart, in.ExpectedWindowEnd, sourceVersion)
	if err != nil {
		return OrderActivationResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return OrderActivationResult{}, err
	}
	return OrderActivationResult{JobID: existing, Created: created, Capability: capability}, nil
}

// ActivateVendorOrderSystem is the post-Wallet bridge used by Orders. It
// resolves tenant identity from the locked VendorOrder row rather than from a
// caller-provided business identifier.
func (s *Service) ActivateVendorOrderSystem(ctx context.Context, vendorOrderID string) error {
	var business string
	if err := s.db.QueryRow(ctx, `SELECT business_id FROM commerce_vendor_orders WHERE id=$1`, vendorOrderID).Scan(&business); err != nil {
		return err
	}
	_, err := s.ActivateVendorOrder(ctx, Actor{VendorID: business, CorrelationID: "payment-held:" + vendorOrderID}, OrderActivationInput{VendorOrderID: vendorOrderID})
	return err
}

// ---------------------------------------------------------------------------
// Permissions
// ---------------------------------------------------------------------------
//
// Four areas, deliberately separated. Production operations and commercial
// authority are different jobs: a chef must be able to run the line without
// being able to reprice the business, and a manager must be able to configure
// capacity without being handed the kitchen's milestone controls.
const (
	PermProductionRead    = "vendor.production.read"
	PermProductionManage  = "vendor.production.manage"  // pools, lines, rules, slots, blackouts
	PermProductionOperate = "vendor.production.operate" // reservations, jobs, milestones
)

func (s *Service) Authorize(ctx context.Context, actor Actor, permission string) (Scope, error) {
	if actor.StaffID == "" || actor.VendorID == "" {
		return Scope{}, apperrors.Unauthorized("Authentication is required.", nil)
	}
	if s.scopes == nil {
		return Scope{}, errForbidden()
	}
	scope, err := s.scopes.ResolveScope(ctx, actor.StaffID, permission)
	if err != nil {
		return Scope{}, err
	}
	if scope.Empty() {
		return Scope{}, errForbidden()
	}
	return scope, nil
}

func (s *Service) requireWrites() error {
	if !s.flags.ProductionEnabled {
		return apperrors.Unavailable("Production management is not enabled.", nil)
	}
	if !s.flags.writesAllowed() {
		return apperrors.Unavailable("Production changes are temporarily unavailable.", nil)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

const (
	CodePoolNotFound        = "PRODUCTION_CAPACITY_POOL_NOT_FOUND"
	CodeLineNotFound        = "PRODUCTION_LINE_NOT_FOUND"
	CodeRuleNotFound        = "PRODUCTION_CAPACITY_RULE_NOT_FOUND"
	CodeTemplateNotFound    = "PRODUCTION_SLOT_TEMPLATE_NOT_FOUND"
	CodeSlotNotFound        = "PRODUCTION_SLOT_NOT_FOUND"
	CodeSlotClosed          = "PRODUCTION_SLOT_CLOSED"
	CodeBlackoutNotFound    = "PRODUCTION_BLACKOUT_NOT_FOUND"
	CodeReservationNotFound = "PRODUCTION_RESERVATION_NOT_FOUND"
	CodeJobNotFound         = "PRODUCTION_JOB_NOT_FOUND"
	CodeCapacityUnavailable = "PRODUCTION_CAPACITY_UNAVAILABLE"
	CodeBlackoutConflict    = "PRODUCTION_BLACKOUT_CONFLICT"
	CodeRescheduleConflict  = "PRODUCTION_RESCHEDULE_CONFLICT"
	CodeStaleVersion        = "PRODUCTION_STALE_VERSION"
	CodeInvalidWindow       = "PRODUCTION_INVALID_WINDOW"
	CodeInvalidAllocation   = "PRODUCTION_INVALID_ALLOCATION"
	CodeInvalidTransition   = "PRODUCTION_INVALID_TRANSITION"
	CodeArchived            = "PRODUCTION_ENTITY_ARCHIVED"
	CodeCapabilityInactive  = "PRODUCTION_CAPABILITY_INACTIVE"
	CodeIdempotencyConflict = "PRODUCTION_IDEMPOTENCY_CONFLICT"
)

func coded(err *apperrors.Error, code string) error {
	if err.Details == nil {
		err.Details = map[string]any{}
	}
	err.Details["code"] = code
	return err
}

func errForbidden() error {
	return apperrors.Forbidden("You do not have permission to do this.", nil)
}

// errNotFound is deliberately indistinguishable from "exists but is not yours",
// so a caller cannot enumerate another tenant's production configuration.
func errNotFound(label, code string) error {
	return coded(apperrors.NotFound(label+" not found.", nil), code)
}

func errStaleVersion() error {
	return coded(apperrors.Conflict("This record changed since you loaded it. Reload and try again.", nil),
		CodeStaleVersion)
}

func errArchived(label string) error {
	return coded(apperrors.Conflict(label+" has been archived and can no longer be changed.", nil), CodeArchived)
}

func validation(violations ...apperrors.FieldViolation) error {
	return apperrors.Validation("Check your details.", violations)
}

// mapDB turns a database failure into this package's vocabulary. A raw SQL
// error never reaches a response body.
func mapDB(err error, label, code string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return errNotFound(label, code)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return coded(apperrors.Conflict(label+" with these details already exists.", nil), code)
		case "23503":
			return validation(apperrors.FieldViolation{Field: "reference",
				Message: "Referenced resource does not belong to this business."})
		case "23514":
			return validation(apperrors.FieldViolation{Field: "request",
				Message: "A supplied value is outside the supported range."})
		}
	}
	return err
}

// ---------------------------------------------------------------------------
// Transaction, audit, outbox and revision helpers
// ---------------------------------------------------------------------------

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type rowScanner interface{ Scan(dest ...any) error }

// inTx runs fn in a transaction, rolling back on any error so no reservation,
// audit row or outbox event can survive a mutation that did not commit.
func (s *Service) inTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) auditTx(ctx context.Context, tx pgx.Tx, actor Actor, action, aggregateType, aggregateID, branchID string, before, after any, reason string) error {
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	actorType := "vendor_staff"
	if actor.StaffID == "" {
		actorType = "system"
	}
	_, err := tx.Exec(ctx, `INSERT INTO vendor_audit_log
		(id,vendor_id,actor_type,actor_id,action,aggregate_type,aggregate_id,branch_id,correlation_id,before_data,after_data,reason)
		VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,NULLIF($8,''),NULLIF($9,''),$10,$11,NULLIF($12,''))`,
		"val_"+uuid.NewString(), actor.VendorID, actorType, actor.StaffID, action, aggregateType, aggregateID,
		branchID, actor.CorrelationID, nilJSON(before, beforeJSON), nilJSON(after, afterJSON), reason)
	return err
}

func (s *Service) publishTx(ctx context.Context, tx pgx.Tx, actor Actor, eventType, aggregateType, aggregateID string, aggregateVersion int64, payload any) error {
	if s.events == nil {
		return nil
	}
	return s.events.Publish(ctx, tx, eventType, aggregateType, aggregateID, aggregateVersion,
		actor.VendorID, actor.CorrelationID, actor.CausationID, payload)
}

type evidence struct {
	action        string
	eventType     string
	aggregateType string
	aggregateID   string
	branchID      string
	version       int64
	before, after any
	payload       any
	reason        string
}

// record writes audit and outbox together for one mutation, so "audited" and
// "published" are not decisions a new handler can forget to make.
func (s *Service) record(ctx context.Context, tx pgx.Tx, actor Actor, ev evidence) error {
	if err := s.auditTx(ctx, tx, actor, ev.action, ev.aggregateType, ev.aggregateID, ev.branchID,
		ev.before, ev.after, ev.reason); err != nil {
		return err
	}
	return s.publishTx(ctx, tx, actor, ev.eventType, ev.aggregateType, ev.aggregateID, ev.version, ev.payload)
}

// RecordMutation lets sibling Vendor domains keep their state change and its
// audit/outbox evidence in the same transaction without reaching into this
// package's private evidence representation.
func (s *Service) RecordMutation(ctx context.Context, tx pgx.Tx, actor Actor, action, eventType, aggregateType, aggregateID, branchID string, version int64, before, after, payload any, reason string) error {
	return s.record(ctx, tx, actor, evidence{action: action, eventType: eventType, aggregateType: aggregateType, aggregateID: aggregateID, branchID: branchID, version: version, before: before, after: after, payload: payload, reason: reason})
}

func nilJSON(v any, raw []byte) any {
	if v == nil {
		return nil
	}
	return raw
}

func newID(prefix string) string { return prefix + uuid.NewString() }

// bumpCalendarRevision advances the production calendar revision for one scope
// atomically, in SQL, inside the caller's transaction.
//
// There is deliberately no read-modify-write in Go: that is exactly how two
// simultaneous slot edits would claim the same revision and let a stale reader
// believe it was current.
func bumpCalendarRevision(ctx context.Context, tx pgx.Tx, businessID, branchID, capability string) (int64, error) {
	var revision int64
	err := tx.QueryRow(ctx, `
		INSERT INTO production_calendar_revisions (business_id, branch_id, capability, revision, updated_at)
		VALUES ($1,$2,$3,1,now())
		ON CONFLICT (business_id, branch_id, capability)
		DO UPDATE SET revision = production_calendar_revisions.revision + 1, updated_at = now()
		RETURNING revision`, businessID, branchID, capability).Scan(&revision)
	return revision, err
}

// CalendarRevision reads the current revision, or 0 when the scope has never
// had one. Zero is honest: it means "no production calendar here yet".
func (s *Service) CalendarRevision(ctx context.Context, q querier, businessID, branchID, capability string) (int64, error) {
	var revision int64
	err := q.QueryRow(ctx, `SELECT revision FROM production_calendar_revisions
		WHERE business_id=$1 AND branch_id=$2 AND capability=$3`, businessID, branchID, capability).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return revision, err
}

// requireBranchCapability reads Section 6/7 state rather than duplicating it.
// A production pool cannot exist for a capability the branch does not hold.
func (s *Service) requireBranchCapability(ctx context.Context, q querier, businessID, branchID, capability string) error {
	var status string
	err := q.QueryRow(ctx, `SELECT status FROM vendor_branch_capabilities
		WHERE business_id=$1 AND branch_id=$2 AND capability_type=$3`,
		businessID, branchID, capability).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return coded(apperrors.Conflict("This branch does not have the "+capability+" capability.", nil),
			CodeCapabilityInactive)
	}
	if err != nil {
		return err
	}
	if status != "ACTIVE" {
		return coded(apperrors.Conflict("The branch capability is not active.", nil), CodeCapabilityInactive)
	}
	return nil
}

// ValidateCapacityPoolReference prevents configuration from pointing at a
// pool owned by another branch or capability.
func (s *Service) ValidateCapacityPoolReference(ctx context.Context, q querier, businessID, branchID, capability string, poolID *string) error {
	if poolID == nil || strings.TrimSpace(*poolID) == "" {
		return nil
	}
	var exists bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM production_capacity_pools WHERE id=$1 AND business_id=$2 AND branch_id=$3 AND capability=$4 AND status='ACTIVE')`, strings.TrimSpace(*poolID), businessID, branchID, capability).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return coded(apperrors.Conflict("The capacity pool is not active for this branch and capability.", nil), CodeCapacityUnavailable)
	}
	return nil
}

func (s *Service) ValidateProductionLineReference(ctx context.Context, q querier, businessID, branchID, capability string, lineID *string) error {
	if lineID == nil || strings.TrimSpace(*lineID) == "" {
		return nil
	}
	var exists bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM production_lines WHERE id=$1 AND business_id=$2 AND branch_id=$3 AND capability=$4 AND status<>'ARCHIVED')`, strings.TrimSpace(*lineID), businessID, branchID, capability).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return coded(apperrors.Conflict("The production line is not active for this branch and capability.", nil), CodeLineNotFound)
	}
	return nil
}
