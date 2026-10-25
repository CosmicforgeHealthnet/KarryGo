// Package usecases owns the pricing policy workflow: draft, submit, approve,
// activate, roll back, and resolve.
//
// Authorisation lives here rather than in the HTTP layer because the same rules
// must hold for the ops CLI, a future admin UI and any internal caller. The
// database enforces the critical invariants as well (a version's body freezes
// when it leaves DRAFT; a requester cannot approve their own change), so a bug
// here cannot quietly permit what the schema forbids.
package usecases

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/pricing-service/internal/database"

	"cosmicforge/logistics/services/pricing-service/internal/engine"
	approvalmodels "cosmicforge/logistics/services/pricing-service/internal/features/approvals/models"
	"cosmicforge/logistics/services/pricing-service/internal/features/policies/models"
	"cosmicforge/logistics/services/pricing-service/internal/platform/audit"
	"cosmicforge/logistics/services/pricing-service/internal/platform/ids"
	"cosmicforge/logistics/services/pricing-service/internal/platform/operators"
)

// Store is the persistence port. It is an interface so the workflow can be tested
// without a database, and so this package never imports another feature's
// repositories.
type Store interface {
	EnsurePolicy(ctx context.Context, policy models.Policy) (models.Policy, error)
	GetPolicy(ctx context.Context, id string) (models.Policy, error)
	NextVersionNo(ctx context.Context, policyID string) (int, error)
	CreateVersion(ctx context.Context, version models.Version) error
	GetVersion(ctx context.Context, id string) (models.Version, error)
	UpdateDraftBody(ctx context.Context, id string, body []byte, expectedBodyHash, bodyHash, changeReason string) error
	TransitionStatus(ctx context.Context, id, from, to, expectedBodyHash string) error
	RecordEvent(ctx context.Context, versionID, event, from, to, performedBy, reason string) error
	Activate(ctx context.Context, activation models.Activation) error
	LatestActivation(ctx context.Context, policyID string, asOf time.Time) (models.Activation, error)
	Resolve(ctx context.Context, kind string, chain models.GeographyChain, asOf time.Time) (models.Resolution, error)
	ListVersions(ctx context.Context, policyID string, limit int) ([]models.Version, error)
}

// Approvals is the approval port, implemented by an adapter over the approvals
// feature. Policies never import that feature's repositories directly.
type Approvals interface {
	Request(ctx context.Context, subjectType, subjectID, subjectHash, requestedBy, reason string) (string, error)
	DecideForSubject(ctx context.Context, subjectType, subjectID, decidedBy, reason, decision string) error
	ApprovedFor(ctx context.Context, subjectType, subjectID, subjectHash string) (approvalmodels.Request, error)
}

// Operators checks a person's authority.
type Operators interface {
	Require(ctx context.Context, id string, role string) (operators.Operator, error)
}

// Options configures the service.
//
// Store and Approvals are FACTORIES rather than instances. A policy change, its
// lifecycle event, its approval decision and its audit row must all commit
// together, so the workflow needs stores bound to one transaction. Passing
// factories keeps that plumbing in one place and keeps this package free of any
// other feature's repositories.
type Options struct {
	Pool      *pgxpool.Pool
	Store     func(database.Querier) Store
	Approvals func(database.Querier) Approvals
	Ops       Operators
	Audit     audit.Recorder
	// FourEyeRequired mirrors configuration. When false (development only) a
	// missing approval does not block activation; the database still records who
	// did what, and self-approval remains impossible.
	FourEyeRequired bool
	Now             func() time.Time
}

// Service is the policy workflow.
type Service struct {
	pool            *pgxpool.Pool
	storeFor        func(database.Querier) Store
	approvalsFor    func(database.Querier) Approvals
	ops             Operators
	audit           audit.Recorder
	fourEyeRequired bool
	now             func() time.Time
}

// NewService builds the workflow.
func NewService(opts Options) *Service {
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		pool:            opts.Pool,
		storeFor:        opts.Store,
		approvalsFor:    opts.Approvals,
		ops:             opts.Ops,
		audit:           opts.Audit,
		fourEyeRequired: opts.FourEyeRequired,
		now:             now,
	}
}

// store returns a store for reads outside a transaction.
func (s *Service) store() Store { return s.storeFor(s.pool) }

// approvals returns an approvals port for reads outside a transaction.
func (s *Service) approvals() Approvals { return s.approvalsFor(s.pool) }

// inTx runs fn against transaction-bound ports, committing on success.
func (s *Service) inTx(ctx context.Context, fn func(Store, Approvals, database.Querier) error) error {
	return database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(s.storeFor(tx), s.approvalsFor(tx), tx)
	})
}

// DraftInput creates a new version.
type DraftInput struct {
	Kind           string
	GeographyLevel string
	GeographyRef   string
	SchemaVersion  int
	Body           []byte
	PerformedBy    string
	ActorCaller    string
	ChangeReason   string
	CloneFrom      string
	RequestID      string
}

// CreateDraft validates a body and stores it as a new draft edition.
//
// The body is decoded, validated and re-encoded canonically before storage, so
// what is stored is always the canonical form and its hash is stable against
// reformatting. An invalid body is refused here rather than at activation, when
// it would be far more expensive to discover.
func (s *Service) CreateDraft(ctx context.Context, in DraftInput) (models.Version, error) {
	if err := models.ValidateDraft(in.Kind, in.GeographyLevel, in.GeographyRef,
		in.PerformedBy, in.ChangeReason, in.SchemaVersion); err != nil {
		return models.Version{}, err
	}
	if _, err := s.ops.Require(ctx, in.PerformedBy, operators.RoleEditor); err != nil {
		return models.Version{}, err
	}

	canonicalBody, bodyHash, err := engine.CanonicalBody(in.Kind, in.SchemaVersion, in.Body)
	if err != nil {
		return models.Version{}, err
	}

	var created models.Version
	err = s.inTx(ctx, func(store Store, approvals Approvals, q database.Querier) error {
		policy, err := store.EnsurePolicy(ctx, models.Policy{
			ID:             ids.New(ids.PrefixPolicy),
			Kind:           in.Kind,
			GeographyLevel: in.GeographyLevel,
			GeographyRef:   in.GeographyRef,
			CreatedBy:      in.PerformedBy,
		})
		if err != nil {
			return err
		}

		versionNo, err := store.NextVersionNo(ctx, policy.ID)
		if err != nil {
			return err
		}

		created = models.Version{
			ID:            ids.New(ids.PrefixPolicyVersion),
			PolicyID:      policy.ID,
			VersionNo:     versionNo,
			Kind:          in.Kind,
			SchemaVersion: in.SchemaVersion,
			Status:        models.StatusDraft,
			Body:          canonicalBody,
			BodyHash:      bodyHash,
			ClonedFrom:    in.CloneFrom,
			CreatedBy:     in.PerformedBy,
			ChangeReason:  in.ChangeReason,
		}
		if err := store.CreateVersion(ctx, created); err != nil {
			return err
		}
		if err := store.RecordEvent(ctx, created.ID, "DRAFTED", "", models.StatusDraft, in.PerformedBy, in.ChangeReason); err != nil {
			return err
		}
		return s.audit.Record(ctx, q, audit.Entry{
			ActorCaller: in.ActorCaller,
			PerformedBy: in.PerformedBy,
			Action:      "POLICY_VERSION_DRAFTED",
			SubjectType: "policy_version",
			SubjectID:   created.ID,
			Reason:      in.ChangeReason,
			After:       map[string]any{"kind": in.Kind, "version_no": versionNo, "body_hash": bodyHash},
			RequestID:   in.RequestID,
		})
	})
	if err != nil {
		return models.Version{}, err
	}
	return created, nil
}

// UpdateDraft replaces a draft's body.
func (s *Service) UpdateDraft(ctx context.Context, versionID string, body []byte, performedBy, actorCaller, changeReason, requestID string, expectedBodyHash ...string) (models.Version, error) {
	if changeReason == "" {
		return models.Version{}, errors.New("policies: a change reason is required")
	}
	if _, err := s.ops.Require(ctx, performedBy, operators.RoleEditor); err != nil {
		return models.Version{}, err
	}

	existing, err := s.store().GetVersion(ctx, versionID)
	if err != nil {
		return models.Version{}, err
	}
	if existing.Status != models.StatusDraft {
		return models.Version{}, fmt.Errorf("%w: version %s is %s", models.ErrNotDraft, versionID, existing.Status)
	}
	expected, err := expectedHash(existing.BodyHash, expectedBodyHash)
	if err != nil {
		return models.Version{}, err
	}

	canonicalBody, bodyHash, err := engine.CanonicalBody(existing.Kind, existing.SchemaVersion, body)
	if err != nil {
		return models.Version{}, err
	}

	err = s.inTx(ctx, func(store Store, approvals Approvals, q database.Querier) error {
		if err := store.UpdateDraftBody(ctx, versionID, canonicalBody, expected, bodyHash, changeReason); err != nil {
			return err
		}
		if err := store.RecordEvent(ctx, versionID, "DRAFT_UPDATED", models.StatusDraft, models.StatusDraft, performedBy, changeReason); err != nil {
			return err
		}
		return s.audit.Record(ctx, q, audit.Entry{
			ActorCaller: actorCaller, PerformedBy: performedBy,
			Action: "POLICY_VERSION_DRAFT_UPDATED", SubjectType: "policy_version", SubjectID: versionID,
			Reason:    changeReason,
			Before:    map[string]any{"body_hash": existing.BodyHash},
			After:     map[string]any{"body_hash": bodyHash},
			RequestID: requestID,
		})
	})
	if err != nil {
		return models.Version{}, err
	}
	return s.store().GetVersion(ctx, versionID)
}

// Submit moves a draft to SUBMITTED and opens an approval request bound to the
// body's hash.
func (s *Service) Submit(ctx context.Context, versionID, performedBy, actorCaller, reason, requestID string, expectedBodyHash ...string) (string, error) {
	if reason == "" {
		return "", errors.New("policies: a reason is required to submit a change for approval")
	}
	if _, err := s.ops.Require(ctx, performedBy, operators.RoleEditor); err != nil {
		return "", err
	}

	version, err := s.store().GetVersion(ctx, versionID)
	if err != nil {
		return "", err
	}
	if version.Status != models.StatusDraft {
		return "", fmt.Errorf("%w: version %s is %s", models.ErrNotDraft, versionID, version.Status)
	}
	expected, err := expectedHash(version.BodyHash, expectedBodyHash)
	if err != nil {
		return "", err
	}

	var approvalID string
	err = s.inTx(ctx, func(store Store, approvals Approvals, q database.Querier) error {
		if err := store.TransitionStatus(ctx, versionID, models.StatusDraft, models.StatusSubmitted, expected); err != nil {
			return err
		}
		// The approval binds to the body hash, so editing after approval
		// invalidates it rather than smuggling new numbers past the approver.
		id, err := approvals.Request(ctx, approvalmodels.SubjectPolicyVersion, versionID, version.BodyHash, performedBy, reason)
		if err != nil {
			return err
		}
		approvalID = id
		if err := store.RecordEvent(ctx, versionID, "SUBMITTED", models.StatusDraft, models.StatusSubmitted, performedBy, reason); err != nil {
			return err
		}
		return s.audit.Record(ctx, q, audit.Entry{
			ActorCaller: actorCaller, PerformedBy: performedBy,
			Action: "POLICY_VERSION_SUBMITTED", SubjectType: "policy_version", SubjectID: versionID,
			Reason:    reason,
			After:     map[string]any{"approval_request_id": approvalID, "body_hash": version.BodyHash},
			RequestID: requestID,
		})
	})
	if err != nil {
		return "", err
	}
	return approvalID, nil
}

// Approve marks a submitted version APPROVED. It does not make it live: that is
// a separate, deliberate activation step, so a rate can be approved now and take
// effect at a chosen moment.
func (s *Service) Approve(ctx context.Context, versionID, performedBy, actorCaller, reason, requestID string, expectedBodyHash ...string) error {
	return s.decide(ctx, versionID, performedBy, actorCaller, reason, requestID, expectedBodyHash, true)
}

// Reject returns a submitted version to its author.
func (s *Service) Reject(ctx context.Context, versionID, performedBy, actorCaller, reason, requestID string, expectedBodyHash ...string) error {
	return s.decide(ctx, versionID, performedBy, actorCaller, reason, requestID, expectedBodyHash, false)
}

func (s *Service) decide(ctx context.Context, versionID, performedBy, actorCaller, reason, requestID string, expectedBodyHash []string, approve bool) error {
	if reason == "" {
		return errors.New("policies: a reason is required to decide a change")
	}
	if _, err := s.ops.Require(ctx, performedBy, operators.RoleApprover); err != nil {
		return err
	}

	version, err := s.store().GetVersion(ctx, versionID)
	if err != nil {
		return err
	}
	if version.Status != models.StatusSubmitted {
		return fmt.Errorf("policies: version %s is %s, not submitted for approval", versionID, version.Status)
	}
	expected, err := expectedHash(version.BodyHash, expectedBodyHash)
	if err != nil {
		return err
	}

	to := models.StatusRejected
	action := "POLICY_VERSION_REJECTED"
	decision := approvalmodels.StatusRejected
	if approve {
		to = models.StatusApproved
		action = "POLICY_VERSION_APPROVED"
		decision = approvalmodels.StatusApproved
	}

	return s.inTx(ctx, func(store Store, approvals Approvals, q database.Querier) error {
		if err := approvals.DecideForSubject(ctx, approvalmodels.SubjectPolicyVersion, versionID, performedBy, reason, decision); err != nil {
			return err
		}
		if err := store.TransitionStatus(ctx, versionID, models.StatusSubmitted, to, expected); err != nil {
			return err
		}
		if err := store.RecordEvent(ctx, versionID, action, models.StatusSubmitted, to, performedBy, reason); err != nil {
			return err
		}
		return s.audit.Record(ctx, q, audit.Entry{
			ActorCaller: actorCaller, PerformedBy: performedBy,
			Action: action, SubjectType: "policy_version", SubjectID: versionID,
			Reason:    reason,
			Before:    map[string]any{"status": models.StatusSubmitted},
			After:     map[string]any{"status": to, "body_hash": version.BodyHash},
			RequestID: requestID,
		})
	})
}

// ActivateInput makes an approved version live.
type ActivateInput struct {
	VersionID        string
	EffectiveFrom    time.Time
	PerformedBy      string
	ActorCaller      string
	Reason           string
	RequestID        string
	ExpectedBodyHash string
	// Rollback marks this activation as a return to an earlier version.
	Rollback bool
}

// Activate makes an approved version live from a moment.
//
// Scheduling is supported (an effective_from in the future), which is how a rate
// change goes live at an announced time rather than whenever an operator happens
// to click. Back-dating is refused: it would silently restate quotes that have
// already been issued under the previous version.
func (s *Service) Activate(ctx context.Context, in ActivateInput) (models.Activation, error) {
	if in.Reason == "" {
		return models.Activation{}, errors.New("policies: a reason is required to activate a version")
	}
	if _, err := s.ops.Require(ctx, in.PerformedBy, operators.RoleApprover); err != nil {
		return models.Activation{}, err
	}

	version, err := s.store().GetVersion(ctx, in.VersionID)
	if err != nil {
		return models.Activation{}, err
	}
	if version.Status != models.StatusApproved {
		return models.Activation{}, fmt.Errorf("%w: version %s is %s", models.ErrNotApproved, in.VersionID, version.Status)
	}
	if in.ExpectedBodyHash != "" && in.ExpectedBodyHash != version.BodyHash {
		return models.Activation{}, fmt.Errorf("%w: expected body hash does not match %s", models.ErrStaleVersion, in.VersionID)
	}

	// The approval must still match the body being activated.
	approval, err := s.approvals().ApprovedFor(ctx, approvalmodels.SubjectPolicyVersion, in.VersionID, version.BodyHash)
	if err != nil && s.fourEyeRequired {
		return models.Activation{}, err
	}

	effectiveFrom := in.EffectiveFrom
	now := s.now()
	if effectiveFrom.IsZero() {
		effectiveFrom = now
	}
	if effectiveFrom.Before(now.Add(-time.Minute)) {
		return models.Activation{}, fmt.Errorf(
			"policies: cannot activate from %s, which is in the past: back-dating would restate quotes already issued",
			effectiveFrom.Format(time.RFC3339))
	}

	action := models.ActionActivate
	if in.Rollback {
		action = models.ActionRollback
	}

	// Supersede whatever is live now, if anything.
	var supersedes string
	if previous, prevErr := s.store().LatestActivation(ctx, version.PolicyID, now); prevErr == nil {
		supersedes = previous.ID
	}

	activation := models.Activation{
		ID:                ids.New(ids.PrefixActivation),
		PolicyID:          version.PolicyID,
		VersionID:         in.VersionID,
		EffectiveFrom:     effectiveFrom,
		Action:            action,
		ActivatedBy:       in.PerformedBy,
		ApprovalRequestID: approval.ID,
		Supersedes:        supersedes,
		Reason:            in.Reason,
	}

	err = s.inTx(ctx, func(store Store, approvals Approvals, q database.Querier) error {
		if err := store.Activate(ctx, activation); err != nil {
			return err
		}
		if err := store.RecordEvent(ctx, in.VersionID, "ACTIVATED", models.StatusApproved, models.StatusApproved, in.PerformedBy, in.Reason); err != nil {
			return err
		}
		return s.audit.Record(ctx, q, audit.Entry{
			ActorCaller: in.ActorCaller, PerformedBy: in.PerformedBy,
			Action: "POLICY_VERSION_ACTIVATED", SubjectType: "policy_version", SubjectID: in.VersionID,
			Reason: in.Reason,
			After: map[string]any{
				"activation_id":  activation.ID,
				"effective_from": effectiveFrom.UTC().Format(time.RFC3339),
				"action":         action,
				"supersedes":     supersedes,
			},
			RequestID: in.RequestID,
		})
	})
	if err != nil {
		return models.Activation{}, err
	}
	return activation, nil
}

func expectedHash(current string, supplied []string) (string, error) {
	if len(supplied) == 0 {
		return current, nil
	}
	if len(supplied) != 1 || supplied[0] == "" || supplied[0] != current {
		return "", fmt.Errorf("%w: expected body hash does not match", models.ErrStaleVersion)
	}
	return supplied[0], nil
}

// Resolve returns the version that prices a kind for a place at a moment.
func (s *Service) Resolve(ctx context.Context, kind string, chain models.GeographyChain, asOf time.Time) (models.Resolution, error) {
	if asOf.IsZero() {
		asOf = s.now()
	}
	return s.store().Resolve(ctx, kind, chain, asOf)
}

// GetVersion returns one version.
func (s *Service) GetVersion(ctx context.Context, id string) (models.Version, error) {
	return s.store().GetVersion(ctx, id)
}

// ListVersions returns a lineage's editions.
func (s *Service) ListVersions(ctx context.Context, policyID string, limit int) ([]models.Version, error) {
	return s.store().ListVersions(ctx, policyID, limit)
}

// GetPolicy returns a lineage, which tells a caller which place a version applies
// to.
func (s *Service) GetPolicy(ctx context.Context, id string) (models.Policy, error) {
	return s.store().GetPolicy(ctx, id)
}
