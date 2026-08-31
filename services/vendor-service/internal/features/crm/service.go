package crm

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/notifications"
)

// Actor is the authenticated vendor staff member behind a request. VendorID
// comes from the server-resolved session, never from the request body or path
// — that is what makes tenant spoofing impossible rather than merely unlikely.
type Actor struct {
	StaffID       string
	VendorID      string
	SessionID     string
	CorrelationID string
	CausationID   string
}

// Scope is the actor's authorised reach for one permission.
//
// Global means a vendor-wide grant: the actor may see business-level totals.
// A branch-scoped actor carries the exact branch ids their assignment covers
// and never sees anything else, including the existence of a relationship that
// only exists at an unauthorised branch.
type Scope struct {
	Global    bool
	BranchIDs []string
}

// Empty reports a scope that authorises nothing.
func (s Scope) Empty() bool { return !s.Global && len(s.BranchIDs) == 0 }

// ScopeResolver answers "at which scopes does this staff member hold this
// permission". It is a port so the CRM feature never imports the staff
// feature's repositories directly (see internal/architecture_test.go).
type ScopeResolver interface {
	ResolveScope(ctx context.Context, staffID, permission string) (Scope, error)
}

// AuditEntry mirrors the service-wide audit record shape.
type AuditEntry struct {
	ActorID, Action, AggregateType, AggregateID, VendorID, BranchID, Reason string
	Before, After                                                           any
}

// AuditRecorder writes an audit entry.
type AuditRecorder interface {
	Record(context.Context, AuditEntry) error
}

// EventPublisher appends a domain event onto the transactional outbox inside
// the caller's transaction.
type EventPublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType, aggregateType, aggregateID string,
		aggregateVersion int64, vendorID, correlationID, causationID string, payload any) error
}

// Flags is the Section 8A slice of the service feature-flag surface.
type Flags struct {
	// CRMEnabled gates the whole feature. When false, no CRM route is
	// registered at all.
	CRMEnabled bool
	// WritesEnabled gates vendor-facing lifecycle commands (archive/restore).
	// Reads stay available so a rollout can disable risky writes without
	// blinding operators.
	WritesEnabled bool
	// CustomerProjectionEnabled gates ingestion of relationship/order facts.
	CustomerProjectionEnabled bool
	// ConsentIntegrationEnabled gates consent ingestion. While false every
	// decision is reported UNKNOWN and marketing eligibility is false — the
	// flag can suppress marketing, never enable it.
	ConsentIntegrationEnabled bool
	// PrivacyIntegrationEnabled gates privacy ingestion.
	PrivacyIntegrationEnabled bool
	// SegmentationEnabled gates the whole Section 8B tags/segments surface.
	// With it false no 8B route is registered at all.
	SegmentationEnabled bool
	// SegmentMaterializationEnabled gates dynamic evaluation. With it false,
	// saved definitions stay readable and previews still work, but no new
	// membership is built and no segment is allowed to claim it is CURRENT —
	// a disabled materializer must degrade to "we do not know", never to a
	// silently frozen audience presented as live.
	SegmentMaterializationEnabled bool
	// InteractionsEnabled and TasksEnabled independently gate the two Section
	// 8C read surfaces. All writes additionally require WritesEnabled.
	InteractionsEnabled bool
	TasksEnabled        bool
	// LoyaltyEnabled gates the whole Section 8D surface. With it false no
	// loyalty route is registered, the Customer 360 module reports
	// NOT_IMPLEMENTED, and the expiry sweeper stands down without discarding
	// due work — points that fall due while it is off are settled when it
	// comes back, and can never be spent in the meantime.
	LoyaltyEnabled bool
	// CampaignsEnabled gates the whole Section 8E surface. With it false no
	// campaign route is registered at all.
	CampaignsEnabled bool
	// CampaignPreparationEnabled gates audience preparation specifically, the
	// expensive half of Section 8E. With it false, definitions stay readable
	// and editable and previews still answer, but no new audience is frozen —
	// and, critically, no already-prepared snapshot is disturbed. Turning
	// preparation off must never make an existing audience look empty.
	CampaignPreparationEnabled bool
	// CampaignExecutionEnabled is the independent Section 8F kill switch.
	// Disabling it stops scheduling and handoff without hiding historical runs.
	CampaignExecutionEnabled bool
	// IntelligenceEnabled gates the whole Section 8H intelligence surface.
	// With it false no intelligence route is registered, no score is computed,
	// the Customer 360 module reports NOT_IMPLEMENTED and the segment registry
	// omits the intelligence fields entirely — a rule cannot reference a score
	// this deployment does not compute.
	IntelligenceEnabled bool
	// IntelligenceWorkerEnabled independently stands the materializer down.
	// Definitions, existing scores and analytics stay readable; nothing new is
	// computed and nothing already current is disturbed, so turning the worker
	// off makes numbers stale rather than absent.
	IntelligenceWorkerEnabled bool
	// ExportsEnabled gates the whole Section 8H export surface. With it false
	// no export route is registered and the generator claims nothing.
	ExportsEnabled bool
	// CorporateEnabled gates the whole Section 8G corporate customer surface.
	// With it false no corporate route is registered at all, and the accepted
	// individual CRM behaviour is exactly what it was before 8G existed.
	CorporateEnabled bool
	// CommerceKillSwitch disables every commerce-adjacent write platform-wide.
	CommerceKillSwitch bool
}

// ConsentStaleAfter bounds how long a consent projection is trusted. Beyond
// it the decision is treated as stale and marketing fails closed, so a
// silently dead consent feed degrades to "cannot target" rather than to
// "target everyone with a cached ALLOW".
const ConsentStaleAfter = 30 * 24 * time.Hour

// Metrics is a small in-process counter set. Labels are deliberately
// low-cardinality: business, profile and customer references are never used as
// label values.
type Metrics struct {
	mu       sync.Mutex
	counters map[string]float64
}

func NewMetrics() *Metrics { return &Metrics{counters: map[string]float64{}} }

func (m *Metrics) Inc(name string, labels ...string) { m.Add(name, 1, labels...) }

func (m *Metrics) Add(name string, value float64, labels ...string) {
	if m == nil {
		return
	}
	parts := make([]string, 0, len(labels)/2)
	for i := 0; i+1 < len(labels); i += 2 {
		parts = append(parts, labels[i]+"="+labels[i+1])
	}
	sort.Strings(parts)
	key := name
	if len(parts) > 0 {
		key += "{" + strings.Join(parts, ",") + "}"
	}
	m.mu.Lock()
	m.counters[key] += value
	m.mu.Unlock()
}

func (m *Metrics) Snapshot() map[string]float64 {
	out := map[string]float64{}
	if m == nil {
		return out
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, value := range m.counters {
		out[key] = value
	}
	return out
}

// MetricsJSON renders the counter set for the periodic metrics log line.
func (s *Service) MetricsJSON() string {
	raw, err := json.Marshal(s.metrics.Snapshot())
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// Service is the CRM application service (Sections 8A and 8B).
type Service struct {
	db      *pgxpool.Pool
	scopes  ScopeResolver
	audit   AuditRecorder
	events  EventPublisher
	flags   Flags
	metrics *Metrics
	privacy *PrivacyRegistry
	now     func() time.Time

	// Section 8B segmentation state. The field registry is built once per
	// deployment from the flag set, so what a rule may reference is fixed at
	// startup rather than negotiated per request.
	registry   *SegmentFieldRegistry
	ruleLimits RuleLimits
	evaluation EvaluationConfig
	retention  GenerationRetainer

	// Section 8E campaign tuning.
	campaignEval CampaignEvaluationConfig
	notifier     notifications.Notifier

	// Section 8H intelligence and export tuning.
	intelligence IntelligenceConfig
	exports      ExportConfig
	artifacts    ArtifactStore
	stepUp       StepUpConsumer
}

// Options configures a Service.
type Options struct {
	DB     *pgxpool.Pool
	Scopes ScopeResolver
	Audit  AuditRecorder
	Events EventPublisher
	Flags  Flags
	// Now is injectable so privacy/consent staleness and out-of-order event
	// races can be tested deterministically.
	Now func() time.Time

	// RuleLimits and Evaluation are Section 8B tuning; zero values take the
	// documented defaults.
	RuleLimits RuleLimits
	Evaluation EvaluationConfig
	// Retention lets a later section pin materialized generations that are
	// still referenced. Nil means nothing is pinned; leaving it nil once
	// Section 8E exists would let segment cleanup delete membership a campaign
	// is still preparing from, so NewService installs the campaign retainer by
	// default and this field only exists to override it in tests.
	Retention GenerationRetainer
	// CampaignEvaluation is Section 8E tuning; the zero value takes the
	// documented defaults.
	CampaignEvaluation CampaignEvaluationConfig
	// Notifier is the sole Section 8F delivery handoff. It accepts opaque
	// customer references; Vendor never resolves or stores destinations.
	Notifier notifications.Notifier

	// Intelligence and Exports are Section 8H tuning; zero values take the
	// documented defaults.
	Intelligence IntelligenceConfig
	Exports      ExportConfig
	// Artifacts is where a generated export is stored. Nil means exports can
	// be requested and their metadata read, but no artifact can be produced or
	// released — which is a safe state, not a broken one: the alternative to a
	// private store is not a local file, it is no download at all.
	Artifacts ArtifactStore
	// StepUp consumes the short-lived elevation Section 5 issues. Nil disables
	// export creation and download entirely rather than silently skipping the
	// check, because an unenforced step-up is worse than an absent feature.
	StepUp StepUpConsumer
}

func NewService(opts Options) *Service {
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	limits := opts.RuleLimits
	if limits.MaxNodes == 0 {
		limits = DefaultRuleLimits()
	}
	evaluation := opts.Evaluation
	if evaluation.BatchSize == 0 {
		evaluation = DefaultEvaluationConfig()
	}
	campaignEval := opts.CampaignEvaluation
	if campaignEval.BatchSize == 0 {
		campaignEval = DefaultCampaignEvaluationConfig()
	}
	if campaignEval.HandoffReservation <= 0 {
		// A zero window would let a second worker adopt a delivery whose
		// handoff is still in flight, which is the one thing the reservation
		// exists to prevent.
		campaignEval.HandoffReservation = DefaultCampaignEvaluationConfig().HandoffReservation
	}
	service := &Service{
		db:      opts.DB,
		scopes:  opts.Scopes,
		audit:   opts.Audit,
		events:  opts.Events,
		flags:   opts.Flags,
		metrics: NewMetrics(),
		now:     now,

		registry:     NewSegmentFieldRegistry(opts.Flags),
		ruleLimits:   limits,
		evaluation:   evaluation,
		campaignEval: campaignEval,
		notifier:     opts.Notifier,

		intelligence: opts.Intelligence.withDefaults(),
		exports:      opts.Exports.withDefaults(),
		artifacts:    opts.Artifacts,
		stepUp:       opts.StepUp,
	}
	// Section 8E pins the segment generations its live snapshots depend on.
	// Installing the retainer here rather than at a call site means segment
	// cleanup cannot release membership a campaign is preparing from, whatever
	// order the composition root happens to wire things in.
	service.retention = opts.Retention
	if service.retention == nil {
		service.retention = campaignGenerationRetainer{service: service}
	}
	service.privacy = NewPrivacyRegistry(service)
	return service
}

// FieldRegistry exposes the segmentation field registry for the read endpoint.
func (s *Service) FieldRegistry() *SegmentFieldRegistry { return s.registry }

// RuleLimitSet exposes the configured complexity guardrails so a builder UI can
// stop a user before the server has to.
func (s *Service) RuleLimitSet() RuleLimits { return s.ruleLimits }

// EvaluationSettings exposes the materializer tuning.
func (s *Service) EvaluationSettings() EvaluationConfig { return s.evaluation }

// Flags exposes the configured flag set (used by the HTTP layer for gating).
func (s *Service) FlagSet() Flags { return s.flags }

// Metrics exposes the counter set for tests and the metrics log line.
func (s *Service) MetricsSnapshot() map[string]float64 { return s.metrics.Snapshot() }

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------
//
// Section 8A error codes reuse the platform envelope. Where a stable CRM
// semantic is needed the reason travels in the message and the code stays one
// of the shared families, matching how Sections 5-7 already behave.

func errProfileNotFound() error {
	// Deliberately indistinguishable from "exists but you may not see it", so
	// an unauthorised caller cannot enumerate customers by probing ids.
	return apperrors.NotFound("Customer relationship not found.", nil)
}
func errBranchScopeDenied() error {
	return apperrors.Forbidden("You do not have access to this customer relationship.", nil)
}
func errProfileRedacted() error {
	return apperrors.Conflict("This customer relationship has been redacted and cannot be changed.", nil)
}
func errProfileMerged() error {
	return apperrors.Conflict("This customer relationship has been merged and cannot be changed independently.", nil)
}
func errVersionConflict() error {
	return apperrors.Conflict("This customer relationship changed since you loaded it. Reload and try again.", nil)
}
func errWritesDisabled() error {
	return apperrors.Unavailable("Customer relationship management is temporarily unavailable.", nil)
}
func errSourceVersionStale() error {
	return apperrors.Conflict("A newer authoritative version is already applied.", nil)
}
func errSourceVersionConflict() error {
	return apperrors.Conflict("Conflicting authoritative data was received for the same source version.", nil)
}
func errCurrencyConflict() error {
	return apperrors.Conflict("Order currency does not match the business commerce currency.", nil)
}
func errAntiResurrectionBlocked() error {
	return apperrors.Conflict("Customer relationship is redacted and cannot be recreated.", nil)
}
func errCrossTenant() error {
	return apperrors.Forbidden("Cross-business customer reference is not permitted.", nil)
}

// ---------------------------------------------------------------------------
// Scope resolution
// ---------------------------------------------------------------------------

// Authorize resolves the actor's scope for a permission, denying outright when
// they hold no covering grant.
func (s *Service) Authorize(ctx context.Context, actor Actor, permission string) (Scope, error) {
	if s.scopes == nil {
		return Scope{}, apperrors.Forbidden("You do not have permission to do this.", nil)
	}
	scope, err := s.scopes.ResolveScope(ctx, actor.StaffID, permission)
	if err != nil {
		return Scope{}, err
	}
	if scope.Empty() {
		return Scope{}, apperrors.Forbidden("You do not have permission to do this.", nil)
	}
	return scope, nil
}

// ---------------------------------------------------------------------------
// Profile columns and scanning
// ---------------------------------------------------------------------------

const profileCols = `id,business_id,subject_type,customer_ref,guest_ref,external_ref,safe_display_label,
status,privacy_state,merge_state,acquisition_source_type,acquisition_source_ref,
first_completed_order_at,last_completed_order_at,observed_order_count,completed_order_count,gross_spend_minor,net_spend_minor,
average_order_value_minor,commercial_currency,last_order_ref,last_branch_ref,preferred_branch_ref,
last_capability_type,commercial_projection_state,commercial_projection_version,last_commercial_source_observed_at,
consent_projection_version,consent_observed_at,redacted_at,merged_into_profile_id,created_at,updated_at,version`

type rowScanner interface{ Scan(dest ...any) error }

func scanProfile(row rowScanner) (Profile, error) {
	var p Profile
	err := row.Scan(&p.ID, &p.BusinessID, &p.SubjectType, &p.CustomerRef, &p.GuestRef, &p.ExternalRef,
		&p.SafeDisplayLabel, &p.Status, &p.PrivacyState, &p.MergeState,
		&p.AcquisitionSourceType, &p.AcquisitionSourceRef,
		&p.FirstCompletedOrderAt, &p.LastCompletedOrderAt, &p.ObservedOrderCount, &p.CompletedOrderCount,
		&p.GrossSpendMinor, &p.NetSpendMinor, &p.AverageOrderValueMinor, &p.CommercialCurrency,
		&p.LastOrderRef, &p.LastBranchRef, &p.PreferredBranchRef, &p.LastCapabilityType,
		&p.CommercialProjectionState, &p.CommercialProjectionVersion, &p.LastCommercialSourceObservedAt,
		&p.ConsentProjectionVersion, &p.ConsentObservedAt, &p.RedactedAt, &p.MergedIntoProfileID,
		&p.CreatedAt, &p.UpdatedAt, &p.Version)
	return p, err
}

// ---------------------------------------------------------------------------
// Tenant- and scope-safe reads
// ---------------------------------------------------------------------------

// ListOptions are the safe filters a vendor may search relationships by.
// Deliberately absent: any filter over email, phone, address or date of birth
// — CRM does not hold those fields and must not appear to.
type ListOptions struct {
	Limit                     int
	Offset                    int
	SubjectType               string
	Status                    string
	PrivacyState              string
	BranchRef                 string
	CapabilityType            string
	CommercialProjectionState string
	MinCompletedOrders        *int
	MaxCompletedOrders        *int
	MinNetSpendMinor          *int64
	MaxNetSpendMinor          *int64
	LastOrderAfter            *time.Time
	LastOrderBefore           *time.Time
	AcquisitionSourceType     string
	Sort                      string
	Direction                 string
	// IncludeRedacted is never settable from an ordinary vendor request. It
	// exists for compliance reads and defaults to false so redacted profiles
	// are invisible to normal search.
	IncludeRedacted bool
}

var allowedProfileSorts = map[string]string{
	"last_completed_order_at": "p.last_completed_order_at",
	"completed_order_count":   "p.completed_order_count",
	"net_spend_minor":         "p.net_spend_minor",
	"created_at":              "p.created_at",
}

func (o ListOptions) bounded() ListOptions {
	if o.Limit <= 0 {
		o.Limit = 50
	}
	if o.Limit > 100 {
		o.Limit = 100
	}
	if o.Offset < 0 {
		o.Offset = 0
	}
	if strings.ToLower(o.Direction) == "asc" {
		o.Direction = "ASC"
	} else {
		o.Direction = "DESC"
	}
	if _, ok := allowedProfileSorts[o.Sort]; !ok {
		o.Sort = "last_completed_order_at"
	}
	return o
}

// ListProfiles returns relationships visible to the actor's scope.
//
// A branch-scoped actor is filtered by an EXISTS join over
// crm_customer_branch_relations inside SQL. Loading business-wide rows and
// discarding unauthorised ones in Go would leak both existence and totals
// through counts and pagination, so the restriction lives in the query.
func (s *Service) ListProfiles(ctx context.Context, actor Actor, scope Scope, opts ListOptions) ([]Profile, bool, error) {
	opts = opts.bounded()

	where := []string{"p.business_id = $1"}
	args := []any{actor.VendorID}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, strings.Replace(clause, "?", "$"+itoa(len(args)), 1))
	}

	if !opts.IncludeRedacted {
		where = append(where, "p.privacy_state <> 'REDACTED'")
	}
	// A merged-away profile is not an independent relationship and never
	// appears in ordinary listings; its survivor does.
	where = append(where, "p.merge_state <> 'MERGED_SOURCE'")

	if !scope.Global {
		args = append(args, scope.BranchIDs)
		where = append(where, `EXISTS (SELECT 1 FROM crm_customer_branch_relations r
			WHERE r.business_id = p.business_id AND r.profile_id = p.id AND r.branch_id = ANY($`+itoa(len(args))+`))`)
	}
	if opts.SubjectType != "" {
		add("p.subject_type = ?", opts.SubjectType)
	}
	if opts.Status != "" {
		add("p.status = ?", opts.Status)
	}
	if opts.PrivacyState != "" {
		add("p.privacy_state = ?", opts.PrivacyState)
	}
	if opts.CapabilityType != "" {
		add("p.last_capability_type = ?", opts.CapabilityType)
	}
	if opts.CommercialProjectionState != "" {
		add("p.commercial_projection_state = ?", opts.CommercialProjectionState)
	}
	if opts.AcquisitionSourceType != "" {
		add("p.acquisition_source_type = ?", opts.AcquisitionSourceType)
	}
	if opts.BranchRef != "" {
		// Asking about a branch you cannot see must not be answerable.
		if !scope.Global && !containsString(scope.BranchIDs, opts.BranchRef) {
			return nil, false, errBranchScopeDenied()
		}
		args = append(args, opts.BranchRef)
		where = append(where, `EXISTS (SELECT 1 FROM crm_customer_branch_relations r
			WHERE r.business_id = p.business_id AND r.profile_id = p.id AND r.branch_id = $`+itoa(len(args))+`)`)
	}
	if opts.MinCompletedOrders != nil {
		add("p.completed_order_count >= ?", *opts.MinCompletedOrders)
	}
	if opts.MaxCompletedOrders != nil {
		add("p.completed_order_count <= ?", *opts.MaxCompletedOrders)
	}
	if opts.MinNetSpendMinor != nil {
		add("p.net_spend_minor >= ?", *opts.MinNetSpendMinor)
	}
	if opts.MaxNetSpendMinor != nil {
		add("p.net_spend_minor <= ?", *opts.MaxNetSpendMinor)
	}
	if opts.LastOrderAfter != nil {
		add("p.last_completed_order_at >= ?", *opts.LastOrderAfter)
	}
	if opts.LastOrderBefore != nil {
		add("p.last_completed_order_at <= ?", *opts.LastOrderBefore)
	}

	args = append(args, opts.Limit+1, opts.Offset)
	// Only vendor_customer_profiles is in the outer FROM (the branch filters
	// are correlated EXISTS subqueries that alias their own table), so the
	// shared unqualified column list resolves unambiguously to p.
	query := `SELECT ` + profileCols + `
		FROM vendor_customer_profiles p
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY ` + allowedProfileSorts[opts.Sort] + ` ` + opts.Direction + ` NULLS LAST, p.id ASC
		LIMIT $` + itoa(len(args)-1) + ` OFFSET $` + itoa(len(args))

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var profiles []Profile
	for rows.Next() {
		profile, scanErr := scanProfile(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		profiles = append(profiles, profile)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(profiles) > opts.Limit
	if hasMore {
		profiles = profiles[:opts.Limit]
	}
	return profiles, hasMore, nil
}

// GetProfile loads one relationship, enforcing tenancy and branch scope.
//
// Every failure that could reveal existence — wrong business, unauthorised
// branch, redacted — is reported as not-found or a generic denial, so probing
// ids yields no information.
func (s *Service) GetProfile(ctx context.Context, actor Actor, scope Scope, profileID string) (Profile, error) {
	profile, err := scanProfile(s.db.QueryRow(ctx,
		`SELECT `+profileCols+` FROM vendor_customer_profiles WHERE business_id=$1 AND id=$2`,
		actor.VendorID, profileID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, errProfileNotFound()
	}
	if err != nil {
		return Profile{}, err
	}
	if profile.IsRedacted() {
		return Profile{}, errProfileNotFound()
	}
	if !scope.Global {
		visible, visErr := s.profileVisibleToBranches(ctx, actor.VendorID, profileID, scope.BranchIDs)
		if visErr != nil {
			return Profile{}, visErr
		}
		if !visible {
			return Profile{}, errProfileNotFound()
		}
	}
	return profile, nil
}

func (s *Service) profileVisibleToBranches(ctx context.Context, businessID, profileID string, branchIDs []string) (bool, error) {
	if len(branchIDs) == 0 {
		return false, nil
	}
	var visible bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM crm_customer_branch_relations
		WHERE business_id=$1 AND profile_id=$2 AND branch_id = ANY($3))`,
		businessID, profileID, branchIDs).Scan(&visible)
	return visible, err
}

// ---------------------------------------------------------------------------
// Lifecycle commands
// ---------------------------------------------------------------------------

// SetArchived archives or restores a relationship.
//
// It refuses on a redacted or merged-away profile: allowing archive/restore
// there would let an operator cycle a profile's visible state around a privacy
// decision they have no authority to revisit.
func (s *Service) SetArchived(ctx context.Context, actor Actor, scope Scope, profileID string, archived bool, expectedVersion int64) (Profile, error) {
	if !s.flags.WritesEnabled || s.flags.CommerceKillSwitch {
		return Profile{}, errWritesDisabled()
	}
	current, err := s.GetProfile(ctx, actor, scope, profileID)
	if err != nil {
		return Profile{}, err
	}
	if current.IsRedacted() {
		return Profile{}, errProfileRedacted()
	}
	if current.IsMergedSource() {
		return Profile{}, errProfileMerged()
	}
	if expectedVersion > 0 && current.Version != expectedVersion {
		return Profile{}, errVersionConflict()
	}

	target := StatusActive
	action := "vendor.crm.profile.restored"
	eventType := "vendor.crm.profile.restored"
	if archived {
		target = StatusArchived
		action = "vendor.crm.profile.archived"
		eventType = "vendor.crm.profile.archived"
	}
	if current.Status == target {
		return current, nil
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Profile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The status guard in the WHERE clause is the real concurrency control:
	// two simultaneous archives cannot both succeed.
	updated, err := scanProfile(tx.QueryRow(ctx,
		`UPDATE vendor_customer_profiles SET status=$1, updated_at=now(), version=version+1
		 WHERE business_id=$2 AND id=$3 AND version=$4
		   AND privacy_state NOT IN ('REDACTED','MERGED') AND merge_state <> 'MERGED_SOURCE'
		 RETURNING `+profileCols,
		target, actor.VendorID, profileID, current.Version))
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, errVersionConflict()
	}
	if err != nil {
		return Profile{}, err
	}
	// Every dynamic rule implicitly excludes archived relationships unless it
	// explicitly asks for them. Archive/restore can therefore change any
	// audience even when profile.status is not named in the stored dependency
	// set, so the tenant's dynamic segments must all be restaked.
	if err = s.invalidateAllDynamicSegmentsTx(ctx, tx, actor.VendorID); err != nil {
		return Profile{}, err
	}
	// Archiving or restoring changes who is in the scored cohort, and a
	// quintile is relative to that cohort, so every score in the tenant is now
	// computed against a set that no longer exists.
	if err = s.MarkIntelligenceStaleTx(ctx, tx, actor.VendorID); err != nil {
		return Profile{}, err
	}
	if err = s.auditTx(ctx, tx, actor, action, "vendor_customer_profile", updated.ID, "",
		map[string]any{"status": current.Status}, map[string]any{"status": updated.Status}, ""); err != nil {
		return Profile{}, err
	}
	if err = s.publishTx(ctx, tx, actor, eventType, "vendor_customer_profile", updated.ID, updated.Version,
		map[string]any{"profile_ref": updated.ID, "status": string(updated.Status), "subject_type": string(updated.SubjectType)}); err != nil {
		return Profile{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Profile{}, err
	}
	s.metrics.Inc("crm_profile_lifecycle_total", "action", string(target))
	return updated, nil
}

// ---------------------------------------------------------------------------
// Shared transactional helpers
// ---------------------------------------------------------------------------

func (s *Service) auditTx(ctx context.Context, tx pgx.Tx, actor Actor, action, aggregateType, aggregateID, branchID string, before, after any, reason string) error {
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	actorType := "staff"
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

func nilJSON(v any, raw []byte) any {
	if v == nil {
		return nil
	}
	return raw
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func newID(prefix string) string { return prefix + uuid.NewString() }

func errInvalid(message string) error { return apperrors.BadRequest(message, nil) }
