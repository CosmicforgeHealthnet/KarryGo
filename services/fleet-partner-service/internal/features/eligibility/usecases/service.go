// Package usecases evaluates and records eligibility decisions.
//
// One rule shapes everything here: a gate may only open on an answer somebody
// can be held to. That means a decision, taken under a named policy version,
// citing the authoritative sources and versions it relied on, with an expiry.
// An unreachable authority, data older than policy allows, or sources that
// disagree all produce a recorded UNKNOWN, STALE or RECONCILING — never a
// quiet pass and never an exception the caller can ignore.
package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// EventEligibilityDecided is emitted for every recorded decision.
const EventEligibilityDecided = "organization.eligibility.decided.v1"

// EventPublisher appends corporate domain events to the outbox.
type EventPublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType string, aggregateType string,
		aggregateID string, aggregateVersion int64, organizationID string,
		correlationID string, payload any) error
}

// Options configure the eligibility service.
type Options struct {
	Pool      *pgxpool.Pool
	Decisions *repositories.Repository
	// Vehicles and Providers are the authoritative clients. A nil client is
	// treated as "authority unavailable", which fails closed — never as
	// "authority unnecessary".
	Vehicles  platformclients.VehicleAuthority
	Providers platformclients.ProviderAuthority
	// Authority maps allow Dispatch and Hauling to be enabled independently.
	// The singular fields remain test-friendly defaults for older fixtures.
	VehicleAuthorities  map[string]platformclients.VehicleAuthority
	ProviderAuthorities map[string]platformclients.ProviderAuthority
	Events              EventPublisher
	Audit               *audit.Recorder
	Metrics             *observability.Metrics
	Now                 func() time.Time
}

// Service evaluates, records and enforces eligibility.
type Service struct {
	opts Options
}

// NewService builds the service.
func NewService(opts Options) *Service {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Metrics == nil {
		opts.Metrics = observability.NewMetrics()
	}
	if opts.Audit == nil {
		opts.Audit = audit.NewRecorder()
	}
	return &Service{opts: opts}
}

// Actor is the caller an evaluation is attributed to.
type Actor struct {
	OrganizationID string
	ActorType      string
	ActorID        string
}

// EvaluateInput is one eligibility question.
type EvaluateInput struct {
	Actor         Actor
	OperationType string
	SubjectType   string
	// SubjectID is the organization id for COMPANY, the provider membership id
	// for PROVIDER, and the authority's opaque vehicle id for VEHICLE.
	SubjectID string

	// BranchID scopes the decision and is checked when policy requires it.
	BranchID string
	// MembershipID names the provider a vehicle must be associated with.
	MembershipID string
	// ServiceCategory is the fleet service category the operation is for.
	ServiceCategory string
	// ExpectedVehicleType is the vehicle type the operation requires, stated
	// in the owning authority's own vocabulary. Fleet's service category is a
	// different vocabulary and is deliberately not translated into this one:
	// guessing a mapping would make every vehicle either always match or never
	// match, and both are worse than not asking.
	ExpectedVehicleType string
	// ExcludeAssignmentID lets a re-evaluation of existing work ignore the
	// assignment being re-evaluated, so an assignment does not conflict with
	// itself.
	ExcludeAssignmentID string
	// AssetReference is the fleet-side vehicle reference for conflict checks.
	AssetReference string

	CorrelationID string
	CausationID   string
}

// evaluation accumulates what the checks found.
type evaluation struct {
	reasons []string
	sources []models.DecisionSource
	// unknown and stale are tracked separately from reasons because they
	// change the STATUS, not merely the explanation. A missing document makes
	// a subject ineligible; an unreachable authority makes the whole answer
	// unknown, and the two must not be reported the same way.
	unknown     bool
	stale       bool
	reconciling bool
}

func (e *evaluation) block(reason string) {
	for _, existing := range e.reasons {
		if existing == reason {
			return
		}
	}
	e.reasons = append(e.reasons, reason)
}

func (e *evaluation) blockUnknown(reason string) {
	e.unknown = true
	e.block(reason)
}

func (e *evaluation) blockStale(reason string) {
	e.stale = true
	e.block(reason)
}

func (e *evaluation) blockReconciling(reason string) {
	e.reconciling = true
	e.block(reason)
}

func (e *evaluation) source(source models.DecisionSource) {
	e.sources = append(e.sources, source)
}

// status derives the recorded status.
//
// Order matters and is deliberate. UNKNOWN outranks STALE and both outrank
// INELIGIBLE, because the strongest true statement wins: if an authority could
// not be reached at all, saying "ineligible because a document is missing"
// would imply the rest of the picture was checked when it was not.
func (e *evaluation) status() string {
	switch {
	case e.unknown:
		return models.StatusUnknown
	case e.reconciling:
		return models.StatusReconciling
	case e.stale:
		return models.StatusStale
	case len(e.reasons) > 0:
		return models.StatusIneligible
	default:
		return models.StatusEligible
	}
}

// Evaluate takes a fresh decision and records it.
//
// Order of operations, and why:
//  1. shape and tenant scope are checked first so a malformed or cross-tenant
//     question never reaches an authority;
//  2. the governing policy version is resolved once and carried through, so
//     every check and the recorded decision agree on which rules applied;
//  3. authoritative calls happen BEFORE the transaction, because an external
//     call must not hold a database lock;
//  4. the decision, its reasons, its sources, its history, the audit row and
//     the outbox event commit together — a decision whose evidence did not
//     survive the same commit is not evidence.
func (s *Service) Evaluate(ctx context.Context, input EvaluateInput) (*models.Decision, error) {
	if err := s.validateShape(&input); err != nil {
		return nil, err
	}

	policy, err := s.opts.Decisions.ActivePolicy(ctx, nil,
		input.Actor.OrganizationID, input.OperationType)
	if errors.Is(err, repositories.ErrNotFound) {
		// No governing policy is not an open gate. Without a policy there is
		// no statement of what "eligible" would even mean here.
		return nil, errorcodes.UnprocessableEntity(errorcodes.EligibilityPolicyMissing,
			"No active eligibility policy governs this operation.")
	}
	if err != nil {
		return nil, err
	}

	result := &evaluation{}
	switch input.SubjectType {
	case models.SubjectCompany:
		s.checkCompany(ctx, input, policy, result)
	case models.SubjectProvider:
		s.checkCompany(ctx, input, policy, result)
		s.checkProvider(ctx, input, policy, result)
	case models.SubjectVehicle:
		s.checkCompany(ctx, input, policy, result)
		s.checkVehicle(ctx, input, policy, result)
	}

	now := s.opts.Now().UTC()
	decision := &models.Decision{
		OrganizationID:   input.Actor.OrganizationID,
		SubjectType:      input.SubjectType,
		SubjectID:        input.SubjectID,
		OperationType:    input.OperationType,
		PolicyID:         policy.ID,
		PolicyKey:        policy.PolicyKey,
		PolicyVersion:    policy.Version,
		Status:           result.status(),
		Reasons:          result.reasons,
		Sources:          result.sources,
		InputFingerprint: decisionFingerprint(input, policy),
		CheckedAt:        now,
		ExpiresAt:        now.Add(policy.DecisionTTL()),
		ActorType:        input.Actor.ActorType,
		ActorID:          input.Actor.ActorID,
		CorrelationID:    input.CorrelationID,
		CausationID:      input.CausationID,
	}
	if input.BranchID != "" {
		branchID := input.BranchID
		decision.BranchID = &branchID
	}
	sort.Strings(decision.Reasons)

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if recordErr := s.opts.Decisions.Record(ctx, tx,
			input.Actor.OrganizationID, decision); recordErr != nil {
			if errors.Is(recordErr, repositories.ErrConflict) {
				return errorcodes.Conflict(errorcodes.EligibilityDecisionConflict,
					"Another evaluation for this subject completed first.")
			}
			return recordErr
		}

		if histErr := s.opts.Decisions.AppendHistory(ctx, tx, &models.HistoryEntry{
			OrganizationID: decision.OrganizationID,
			DecisionID:     decision.ID,
			SubjectType:    decision.SubjectType,
			SubjectID:      decision.SubjectID,
			OperationType:  decision.OperationType,
			Action:         models.ActionEvaluated,
			ToStatus:       decision.Status,
			ActorType:      decision.ActorType,
			ActorID:        decision.ActorID,
			ReasonCode:     firstReason(decision.Reasons),
			CorrelationID:  decision.CorrelationID,
			CausationID:    decision.CausationID,
		}); histErr != nil {
			return histErr
		}

		if auditErr := s.opts.Audit.Record(ctx, tx, audit.Entry{
			OrganizationID: decision.OrganizationID,
			BranchID:       input.BranchID,
			ActorType:      strings.ToLower(decision.ActorType),
			ActorID:        decision.ActorID,
			Action:         "eligibility.decided",
			ResourceType:   "eligibility_decision",
			ResourceID:     decision.ID,
			NewValues: map[string]any{
				"subjectType":   decision.SubjectType,
				"subjectId":     decision.SubjectID,
				"operationType": decision.OperationType,
				"status":        decision.Status,
				"policyVersion": decision.PolicyVersion,
				"reasons":       decision.Reasons,
			},
			RequestID:     decision.CorrelationID,
			CorrelationID: decision.CorrelationID,
		}); auditErr != nil {
			return auditErr
		}

		if s.opts.Events == nil {
			return nil
		}
		return s.opts.Events.Publish(ctx, tx, EventEligibilityDecided, "eligibility_decision",
			decision.ID, 1, decision.OrganizationID, decision.CorrelationID,
			map[string]any{
				"decisionId":     decision.ID,
				"organizationId": decision.OrganizationID,
				"subjectType":    decision.SubjectType,
				"subjectId":      decision.SubjectID,
				"operationType":  decision.OperationType,
				"status":         decision.Status,
				"policyKey":      decision.PolicyKey,
				"policyVersion":  decision.PolicyVersion,
				"reasons":        decision.Reasons,
				"checkedAt":      decision.CheckedAt,
				"expiresAt":      decision.ExpiresAt,
				// Stated on the wire so a consumer never has to infer whether
				// this decision may authorize anything.
				"authorizes": decision.Authorizes(now),
			})
	})
	if err != nil {
		return nil, err
	}

	s.opts.Metrics.IncCounter("fleet_partner_eligibility_decisions_total",
		map[string]string{"subject": decision.SubjectType, "operation": decision.OperationType,
			"status": decision.Status}, 1)
	return decision, nil
}

// Require is the enforcement entry point every protected use case calls.
//
// It reuses a current, unexpired decision whose inputs match, and evaluates a
// fresh one otherwise. A decision that does not authorize is returned as a
// typed error carrying the leading reason code, so a caller cannot accidentally
// proceed by ignoring a boolean.
func (s *Service) Require(ctx context.Context, input EvaluateInput) (*models.Decision, error) {
	if err := s.validateShape(&input); err != nil {
		return nil, err
	}

	decision, err := s.reusable(ctx, input)
	if err != nil {
		return nil, err
	}
	if decision == nil {
		decision, err = s.Evaluate(ctx, input)
		if err != nil {
			return nil, err
		}
	}

	if !decision.Authorizes(s.opts.Now().UTC()) {
		return decision, s.blockedError(decision)
	}
	return decision, nil
}

// reusable returns a decision that may be reused, or nil.
//
// Reuse requires all four of: the decision is current, unexpired, ELIGIBLE and
// taken over the same inputs under the same policy version. The fingerprint
// check is what stops a decision taken for one branch, membership or service
// category being spent on another.
func (s *Service) reusable(ctx context.Context, input EvaluateInput) (*models.Decision, error) {
	existing, err := s.opts.Decisions.Current(ctx, nil, input.Actor.OrganizationID,
		input.SubjectType, input.SubjectID, input.OperationType)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	policy, err := s.opts.Decisions.ActivePolicy(ctx, nil,
		input.Actor.OrganizationID, input.OperationType)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, errorcodes.UnprocessableEntity(errorcodes.EligibilityPolicyMissing,
			"No active eligibility policy governs this operation.")
	}
	if err != nil {
		return nil, err
	}

	// A newer policy version means the old answer was decided under rules that
	// no longer apply. The historical decision keeps its meaning; it simply
	// stops being reusable.
	if existing.PolicyVersion != policy.Version || existing.PolicyID != policy.ID {
		return nil, nil
	}
	if existing.InputFingerprint != decisionFingerprint(input, policy) {
		return nil, nil
	}
	if !existing.Authorizes(s.opts.Now().UTC()) {
		return nil, nil
	}
	return existing, nil
}

// blockedError turns a non-authorizing decision into a typed refusal.
func (s *Service) blockedError(decision *models.Decision) error {
	reason := decision.BlockingReason(s.opts.Now().UTC())
	switch decision.Status {
	case models.StatusUnknown, models.StatusReconciling:
		return errorcodes.Unavailable(errorcodes.EligibilityUnknown,
			"Eligibility could not be confirmed right now. Please try again.")
	case models.StatusStale:
		return errorcodes.Unavailable(errorcodes.EligibilityStale,
			"Eligibility information is out of date. Please try again shortly.")
	default:
		return errorcodes.UnprocessableEntity(errorcodes.EligibilityBlocked,
			"This action is blocked: "+humanReason(reason))
	}
}

// Current returns the live decision for a subject, without evaluating.
func (s *Service) Current(ctx context.Context, organizationID string, subjectType string,
	subjectID string, operationType string) (*models.Decision, error) {
	decision, err := s.opts.Decisions.Current(ctx, nil, organizationID,
		subjectType, subjectID, operationType)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, errorcodes.NotFound(errorcodes.EligibilityDecisionNotFound,
			"No eligibility decision has been recorded.")
	}
	return decision, err
}

// Get returns one decision inside the organization scope.
func (s *Service) Get(ctx context.Context, organizationID string,
	decisionID string) (*models.Decision, error) {
	decision, err := s.opts.Decisions.GetByID(ctx, organizationID, decisionID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, errorcodes.NotFound(errorcodes.EligibilityDecisionNotFound,
			"Eligibility decision not found.")
	}
	return decision, err
}

// List returns decisions inside the tenant and branch scope.
func (s *Service) List(ctx context.Context, query tenant.Query,
	filter repositories.ListFilter) ([]models.Decision, error) {
	if query.OrganizationID == "" {
		return nil, errorcodes.Forbidden(errorcodes.ActorNotAuthorized,
			"Organization scope is required.")
	}
	return s.opts.Decisions.List(ctx, query, filter)
}

// History returns the append-only history of one decision.
func (s *Service) History(ctx context.Context, organizationID string,
	decisionID string) ([]models.HistoryEntry, error) {
	if _, err := s.Get(ctx, organizationID, decisionID); err != nil {
		return nil, err
	}
	return s.opts.Decisions.ListHistory(ctx, organizationID, decisionID)
}

// Policy resolves the active policy version for an operation.
func (s *Service) Policy(ctx context.Context, organizationID string,
	policyKey string) (*models.Policy, error) {
	policy, err := s.opts.Decisions.ActivePolicy(ctx, nil, organizationID, policyKey)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, errorcodes.NotFound(errorcodes.EligibilityPolicyMissing,
			"No active policy governs this operation.")
	}
	return policy, err
}

// ListPolicies returns the policy versions visible to an organization.
func (s *Service) ListPolicies(ctx context.Context, organizationID string,
	policyKey string) ([]models.Policy, error) {
	return s.opts.Decisions.ListPolicies(ctx, organizationID, policyKey)
}

// PublishPolicy records a new tenant policy version.
//
// Publishing never rewrites history: the outgoing version is archived, not
// edited, and every decision that cited it keeps citing exactly what it said.
func (s *Service) PublishPolicy(ctx context.Context, actor Actor,
	policy *models.Policy) (*models.Policy, error) {
	if actor.OrganizationID == "" {
		return nil, errorcodes.Forbidden(errorcodes.ActorNotAuthorized,
			"Organization scope is required.")
	}
	if strings.TrimSpace(policy.PolicyKey) == "" {
		return nil, errorcodes.BadRequest(errorcodes.ValidationFailed,
			"A policy key is required.")
	}
	if policy.OperationType == "" {
		policy.OperationType = policy.PolicyKey
	}
	if policy.DecisionTTLSeconds <= 0 {
		policy.DecisionTTLSeconds = 900
	}
	if policy.SourceMaxAgeSeconds <= 0 {
		policy.SourceMaxAgeSeconds = 900
	}
	if policy.MinimumApprovals <= 0 {
		policy.MinimumApprovals = 1
	}
	if policy.ApprovalTTLSeconds <= 0 {
		policy.ApprovalTTLSeconds = 86400
	}

	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if publishErr := s.opts.Decisions.PublishPolicy(ctx, tx,
			actor.OrganizationID, policy); publishErr != nil {
			return publishErr
		}
		return s.opts.Audit.Record(ctx, tx, audit.Entry{
			OrganizationID: actor.OrganizationID,
			ActorType:      strings.ToLower(actor.ActorType),
			ActorID:        actor.ActorID,
			Action:         "eligibility.policy.published",
			ResourceType:   "eligibility_policy",
			ResourceID:     policy.ID,
			NewValues: map[string]any{
				"policyKey":        policy.PolicyKey,
				"version":          policy.Version,
				"minimumApprovals": policy.MinimumApprovals,
				"selfApproval":     policy.SelfApprovalAllowed,
			},
		})
	})
	if err != nil {
		return nil, err
	}
	return policy, nil
}

// validateShape rejects malformed input before any I/O.
func (s *Service) validateShape(input *EvaluateInput) error {
	input.SubjectType = strings.ToUpper(strings.TrimSpace(input.SubjectType))
	input.OperationType = strings.ToUpper(strings.TrimSpace(input.OperationType))
	input.SubjectID = strings.TrimSpace(input.SubjectID)
	input.Actor.ActorType = models.NormalizeActorType(input.Actor.ActorType)

	switch {
	case input.Actor.OrganizationID == "":
		return errorcodes.Forbidden(errorcodes.ActorNotAuthorized,
			"Organization scope is required.")
	case !models.ValidSubjectType(input.SubjectType):
		return errorcodes.BadRequest(errorcodes.ValidationFailed,
			"subjectType must be COMPANY, PROVIDER or VEHICLE.")
	case input.OperationType == "":
		return errorcodes.BadRequest(errorcodes.ValidationFailed,
			"An operation type is required.")
	case input.SubjectID == "":
		return errorcodes.BadRequest(errorcodes.ValidationFailed,
			"A subject identifier is required.")
	case len(input.SubjectID) > 200:
		return errorcodes.BadRequest(errorcodes.ValidationFailed,
			"The subject identifier is too long.")
	}

	// A COMPANY decision is always about the calling organization. Accepting a
	// different id here would make the endpoint an oracle for other tenants'
	// standing.
	if input.SubjectType == models.SubjectCompany &&
		input.SubjectID != input.Actor.OrganizationID {
		return errorcodes.NotFound(errorcodes.EligibilityDecisionNotFound, "Subject not found.")
	}
	return nil
}

// decisionFingerprint hashes everything that could change the answer.
//
// The policy version is part of the hash: the same question under different
// rules is a different question, and a decision taken under version 1 must not
// silently satisfy a caller operating under version 2.
func decisionFingerprint(input EvaluateInput, policy *models.Policy) string {
	hasher := sha256.New()
	for _, part := range []string{
		input.Actor.OrganizationID, input.SubjectType, input.SubjectID,
		input.OperationType, input.BranchID, input.MembershipID,
		input.ServiceCategory, input.ExpectedVehicleType, input.AssetReference,
		policy.ID, policy.PolicyKey,
	} {
		_, _ = hasher.Write([]byte(part))
		_, _ = hasher.Write([]byte{0})
	}
	_, _ = hasher.Write([]byte(itoa(policy.Version)))
	return hex.EncodeToString(hasher.Sum(nil))
}

func itoa(value int64) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}

func firstReason(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	return reasons[0]
}

// humanReason turns a controlled reason code into safe public text.
//
// Deliberately coarse. A blocked operator needs to know which area to fix, not
// which specific document of which specific provider failed — the detail is in
// the decision record, behind the read permission.
func humanReason(reason string) string {
	switch {
	case strings.HasPrefix(reason, "company_document"),
		strings.HasPrefix(reason, "provider_document"),
		strings.HasPrefix(reason, "vehicle_document"):
		return "a required document is missing, rejected or expired."
	case strings.HasSuffix(reason, "_conflict"):
		return "the subject is already committed to other work."
	case strings.HasPrefix(reason, "company_subscription"):
		return "the organization subscription is not active."
	case strings.Contains(reason, "verification"), strings.Contains(reason, "unverified"):
		return "verification is not complete."
	case strings.Contains(reason, "suspend"), strings.Contains(reason, "restrict"):
		return "the subject is suspended or restricted."
	case strings.Contains(reason, "not_found"):
		return "the subject could not be found."
	case strings.Contains(reason, "available"):
		return "the subject is not available."
	default:
		return "an eligibility requirement is not met."
	}
}
