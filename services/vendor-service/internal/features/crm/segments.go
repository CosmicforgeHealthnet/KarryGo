package crm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Segment types.
//
// STATIC membership is maintained by hand; DYNAMIC membership is derived from a
// validated rule and can never be hand-edited — a manually inserted member
// would silently disappear at the next materialization, so the API refuses the
// operation instead of accepting a write it cannot keep.
const (
	SegmentStatic  = "STATIC"
	SegmentDynamic = "DYNAMIC"
)

// Segment lifecycle.
const (
	SegmentDraft    = "DRAFT"
	SegmentActive   = "ACTIVE"
	SegmentArchived = "ARCHIVED"
)

// Evaluation status of a segment's materialized membership.
const (
	EvalNotEvaluated = "NOT_EVALUATED"
	EvalQueued       = "QUEUED"
	EvalRunning      = "RUNNING"
	EvalCurrent      = "CURRENT"
	EvalStale        = "STALE"
	EvalFailed       = "FAILED"
)

// Evaluation run status.
const (
	RunQueued     = "QUEUED"
	RunRunning    = "RUNNING"
	RunSuccess    = "SUCCESS"
	RunFailed     = "FAILED"
	RunDeadLetter = "DEAD_LETTER"
	RunSuperseded = "SUPERSEDED"
)

// Membership sources.
const (
	MembershipStatic  = "STATIC"
	MembershipDynamic = "DYNAMIC"
)

// Segment is a saved audience definition.
type Segment struct {
	ID         string
	BusinessID string

	Name           string
	NormalizedName string
	Description    *string

	SegmentType string
	ScopeType   string
	BranchID    *string
	Status      string

	CurrentDefinitionVersion int64
	EvaluationStatus         string
	CurrentEvaluationID      *string
	CurrentMemberCount       int
	DefinitionHash           *string
	LastEvaluatedAt          *time.Time
	StaleSince               *time.Time
	LastEvaluationErrorCode  *string

	CreatedByStaffRef *string
	UpdatedByStaffRef *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Version           int64
}

// Dynamic reports whether membership comes from a rule.
func (s Segment) Dynamic() bool { return s.SegmentType == SegmentDynamic }

// Scope renders the segment's evaluation scope.
func (s Segment) Scope() SegmentScope {
	return SegmentScope{Type: s.ScopeType, BranchID: derefOr(s.BranchID)}
}

// SegmentVersion is one immutable definition revision.
type SegmentVersion struct {
	ID                string
	BusinessID        string
	SegmentID         string
	DefinitionVersion int64
	SegmentType       string
	ScopeType         string
	BranchID          *string
	RuleSchemaVersion *int
	RuleJSON          []byte
	ExclusionJSON     []byte
	CanonicalRuleHash *string
	DependencySet     []string
	CreatedAt         time.Time
	ChangeReason      *string
}

// SegmentView is the safe wire projection of a segment.
type SegmentView struct {
	SegmentRef  string  `json:"segment_ref"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	SegmentType string  `json:"segment_type"`
	ScopeType   string  `json:"scope_type"`
	BranchRef   *string `json:"branch_ref,omitempty"`
	Status      string  `json:"status"`

	DefinitionVersion int64   `json:"definition_version"`
	DefinitionHash    *string `json:"definition_hash,omitempty"`

	// Membership is always reported alongside its freshness. A member count
	// without a state is how a stale audience gets mistaken for a live one.
	EvaluationStatus              string     `json:"evaluation_status"`
	MemberCount                   int        `json:"member_count"`
	MaterializedDefinitionVersion *int64     `json:"materialized_definition_version,omitempty"`
	LastEvaluatedAt               *time.Time `json:"last_evaluated_at,omitempty"`
	StaleSince                    *time.Time `json:"stale_since,omitempty"`
	LastEvaluationErrorCode       *string    `json:"last_evaluation_error_code,omitempty"`

	RuleExplanation string          `json:"rule_explanation,omitempty"`
	Rule            json.RawMessage `json:"rule,omitempty"`
	Exclusion       json.RawMessage `json:"exclusion_rule,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	Version   int64     `json:"version"`
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

func errSegmentNotFound() error {
	return crmError(apperrors.NotFound("Segment not found.", nil), CodeSegmentNotFound)
}
func errSegmentExists() error {
	return crmError(apperrors.Conflict("A segment with this name already exists.", nil), CodeSegmentAlreadyExists)
}
func errSegmentState(message string) error {
	return crmError(apperrors.Conflict(message, nil), CodeSegmentInvalidState)
}
func errStaticOnly() error {
	return crmError(apperrors.Conflict("Membership can only be edited on a static segment.", nil), CodeSegmentStaticOnly)
}
func errDynamicOnly() error {
	return crmError(apperrors.Conflict("This operation applies only to a dynamic segment.", nil), CodeSegmentDynamicOnly)
}

// assertSegmentationWrites gates every Section 8B command.
//
// Reads stay available when writes are disabled so a rollout can freeze changes
// without blinding operators, and the global commerce kill switch overrides
// everything.
func (s *Service) assertSegmentationWrites() error {
	if !s.flags.SegmentationEnabled || s.flags.CommerceKillSwitch {
		return errSegmentationDisabled()
	}
	if !s.flags.WritesEnabled {
		return errWritesDisabled()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

const segmentCols = `id,business_id,name,normalized_name,description,segment_type,scope_type,branch_id,status,
current_definition_version,evaluation_status,current_evaluation_id,current_member_count,definition_hash,
last_evaluated_at,stale_since,last_evaluation_error_code,created_by_staff_ref,updated_by_staff_ref,
created_at,updated_at,version`

func scanSegment(row rowScanner) (Segment, error) {
	var s Segment
	err := row.Scan(&s.ID, &s.BusinessID, &s.Name, &s.NormalizedName, &s.Description, &s.SegmentType,
		&s.ScopeType, &s.BranchID, &s.Status, &s.CurrentDefinitionVersion, &s.EvaluationStatus,
		&s.CurrentEvaluationID, &s.CurrentMemberCount, &s.DefinitionHash, &s.LastEvaluatedAt,
		&s.StaleSince, &s.LastEvaluationErrorCode, &s.CreatedByStaffRef, &s.UpdatedByStaffRef,
		&s.CreatedAt, &s.UpdatedAt, &s.Version)
	return s, err
}

// SegmentListOptions are the safe filters over the segment list.
type SegmentListOptions struct {
	Limit            int
	Offset           int
	SegmentType      string
	ScopeType        string
	BranchRef        string
	Status           string
	EvaluationStatus string
	Search           string
}

func (o SegmentListOptions) bounded() SegmentListOptions {
	if o.Limit <= 0 {
		o.Limit = DefaultPageSize
	}
	if o.Limit > MaxPageSize {
		o.Limit = MaxPageSize
	}
	if o.Offset < 0 {
		o.Offset = 0
	}
	return o
}

// ListSegments returns segments visible to the actor's scope.
//
// A branch-scoped actor never sees a BUSINESS-scope segment: its member count
// alone would disclose the size and shape of the business-wide customer base.
func (s *Service) ListSegments(ctx context.Context, actor Actor, scope Scope, opts SegmentListOptions) ([]Segment, bool, error) {
	opts = opts.bounded()
	where := []string{"g.business_id = $1"}
	args := []any{actor.VendorID}
	bind := func(value any) string {
		args = append(args, value)
		return "$" + itoa(len(args))
	}

	if !scope.Global {
		where = append(where, "g.scope_type = 'BRANCH' AND g.branch_id = ANY("+bind(scope.BranchIDs)+")")
	}
	if opts.SegmentType != "" {
		where = append(where, "g.segment_type = "+bind(opts.SegmentType))
	}
	if opts.ScopeType != "" {
		where = append(where, "g.scope_type = "+bind(opts.ScopeType))
	}
	if opts.Status != "" {
		where = append(where, "g.status = "+bind(opts.Status))
	}
	if opts.EvaluationStatus != "" {
		where = append(where, "g.evaluation_status = "+bind(opts.EvaluationStatus))
	}
	if opts.BranchRef != "" {
		if !scope.Global && !containsString(scope.BranchIDs, opts.BranchRef) {
			return nil, false, errScopeDenied(CodeSegmentScopeDenied)
		}
		where = append(where, "g.branch_id = "+bind(opts.BranchRef))
	}
	if opts.Search != "" {
		where = append(where, "g.normalized_name LIKE "+bind("%"+escapeLike(NormalizeTagName(opts.Search))+"%")+" ESCAPE '\\'")
	}

	limit := bind(opts.Limit + 1)
	offset := bind(opts.Offset)
	rows, err := s.db.Query(ctx, `SELECT `+segmentCols+` FROM crm_segments g
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY g.normalized_name ASC, g.id ASC LIMIT `+limit+` OFFSET `+offset, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var segments []Segment
	for rows.Next() {
		segment, scanErr := scanSegment(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		segments = append(segments, segment)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(segments) > opts.Limit
	if hasMore {
		segments = segments[:opts.Limit]
	}
	return segments, hasMore, nil
}

// GetSegment loads one segment, enforcing tenancy and scope.
func (s *Service) GetSegment(ctx context.Context, actor Actor, scope Scope, segmentID string) (Segment, error) {
	segment, err := s.loadSegment(ctx, s.db, actor.VendorID, segmentID)
	if err != nil {
		return Segment{}, err
	}
	if err = s.assertSegmentVisible(scope, segment); err != nil {
		return Segment{}, err
	}
	return segment, nil
}

func (s *Service) loadSegment(ctx context.Context, q querier, businessID, segmentID string) (Segment, error) {
	segment, err := scanSegment(q.QueryRow(ctx, `SELECT `+segmentCols+` FROM crm_segments WHERE business_id=$1 AND id=$2`,
		businessID, segmentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Segment{}, errSegmentNotFound()
	}
	return segment, err
}

func (s *Service) loadSegmentForUpdate(ctx context.Context, tx pgx.Tx, businessID, segmentID string) (Segment, error) {
	segment, err := scanSegment(tx.QueryRow(ctx, `SELECT `+segmentCols+` FROM crm_segments
		WHERE business_id=$1 AND id=$2 FOR UPDATE`, businessID, segmentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Segment{}, errSegmentNotFound()
	}
	return segment, err
}

func (s *Service) assertSegmentVisible(scope Scope, segment Segment) error {
	if scope.Global {
		return nil
	}
	if segment.ScopeType == ScopeBranch && segment.BranchID != nil && containsString(scope.BranchIDs, *segment.BranchID) {
		return nil
	}
	return errSegmentNotFound()
}

// CurrentVersion loads the immutable definition the segment currently points at.
func (s *Service) CurrentVersion(ctx context.Context, q querier, businessID, segmentID string, definitionVersion int64) (SegmentVersion, error) {
	var v SegmentVersion
	err := q.QueryRow(ctx, `SELECT id,business_id,segment_id,definition_version,segment_type,scope_type,branch_id,
		rule_schema_version,rule_json,exclusion_rule_json,canonical_rule_hash,dependency_set,created_at,change_reason
		FROM crm_segment_versions WHERE business_id=$1 AND segment_id=$2 AND definition_version=$3`,
		businessID, segmentID, definitionVersion).Scan(&v.ID, &v.BusinessID, &v.SegmentID, &v.DefinitionVersion,
		&v.SegmentType, &v.ScopeType, &v.BranchID, &v.RuleSchemaVersion, &v.RuleJSON, &v.ExclusionJSON,
		&v.CanonicalRuleHash, &v.DependencySet, &v.CreatedAt, &v.ChangeReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return SegmentVersion{}, errSegmentNotFound()
	}
	return v, err
}

// SegmentViewOf renders a segment, optionally with its current rule.
func (s *Service) SegmentViewOf(ctx context.Context, segment Segment, version *SegmentVersion) SegmentView {
	view := SegmentView{
		SegmentRef: segment.ID, Name: segment.Name, Description: segment.Description,
		SegmentType: segment.SegmentType, ScopeType: segment.ScopeType, BranchRef: segment.BranchID,
		Status: segment.Status, DefinitionVersion: segment.CurrentDefinitionVersion,
		DefinitionHash: segment.DefinitionHash, EvaluationStatus: segment.EvaluationStatus,
		MemberCount: segment.CurrentMemberCount, LastEvaluatedAt: segment.LastEvaluatedAt,
		StaleSince: segment.StaleSince, LastEvaluationErrorCode: segment.LastEvaluationErrorCode,
		CreatedAt: segment.CreatedAt, Version: segment.Version,
	}
	if version != nil {
		if len(version.RuleJSON) > 0 {
			view.Rule = version.RuleJSON
		}
		if len(version.ExclusionJSON) > 0 {
			view.Exclusion = version.ExclusionJSON
		}
		view.RuleExplanation = s.explainStoredRule(ctx, *version)
	}
	if segment.CurrentEvaluationID != nil {
		if materialized, err := s.materializedDefinitionVersion(ctx, segment); err == nil {
			view.MaterializedDefinitionVersion = materialized
		}
	}
	return view
}

func (s *Service) materializedDefinitionVersion(ctx context.Context, segment Segment) (*int64, error) {
	if segment.CurrentEvaluationID == nil {
		return nil, nil
	}
	var version int64
	err := s.db.QueryRow(ctx, `SELECT segment_definition_version FROM crm_segment_evaluations
		WHERE business_id=$1 AND id=$2`, segment.BusinessID, *segment.CurrentEvaluationID).Scan(&version)
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// explainStoredRule re-validates a stored definition to render its explanation.
// Doing it from the same validator the evaluator uses is what keeps the prose
// and the behaviour from drifting.
func (s *Service) explainStoredRule(_ context.Context, version SegmentVersion) string {
	if len(version.RuleJSON) == 0 {
		return ""
	}
	document := map[string]any{"schema_version": RuleSchemaVersion, "root": json.RawMessage(version.RuleJSON)}
	if len(version.ExclusionJSON) > 0 {
		document["exclusion"] = json.RawMessage(version.ExclusionJSON)
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return ""
	}
	scope := SegmentScope{Type: version.ScopeType, BranchID: derefOr(version.BranchID)}
	validated, err := ValidateRule(raw, s.registry, s.ruleLimits, scope)
	if err != nil {
		return ""
	}
	return DescribeRule(validated)
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

// CreateSegmentInput is a vendor request to define a segment.
type CreateSegmentInput struct {
	Name        string
	Description string
	SegmentType string
	ScopeType   string
	BranchRef   string
	Rule        json.RawMessage
}

// CreateSegment defines a segment in DRAFT.
func (s *Service) CreateSegment(ctx context.Context, actor Actor, scope Scope, in CreateSegmentInput) (Segment, error) {
	if err := s.assertSegmentationWrites(); err != nil {
		return Segment{}, err
	}
	name, normalized, err := validateSegmentName(in.Name)
	if err != nil {
		return Segment{}, err
	}
	description, err := validateTagDescription(in.Description)
	if err != nil {
		return Segment{}, err
	}
	segmentType := strings.ToUpper(strings.TrimSpace(in.SegmentType))
	if segmentType != SegmentStatic && segmentType != SegmentDynamic {
		return Segment{}, tagValidation("segment_type", "Segment type must be STATIC or DYNAMIC.")
	}
	scopeType := strings.ToUpper(strings.TrimSpace(in.ScopeType))
	if scopeType == "" {
		scopeType = ScopeBusiness
	}
	var branchID *string
	if branch := strings.TrimSpace(in.BranchRef); branch != "" {
		branchID = &branch
	}
	if err = authorizeObjectScope(scope, scopeType, branchID, CodeSegmentScopeDenied); err != nil {
		return Segment{}, err
	}
	if scopeType == ScopeBusiness {
		branchID = nil
	}

	segmentScope := SegmentScope{Type: scopeType, BranchID: derefOr(branchID)}
	var validated *ValidatedRule
	switch segmentType {
	case SegmentDynamic:
		if len(in.Rule) == 0 {
			return Segment{}, crmError(apperrors.Validation("A dynamic segment needs a rule.",
				[]apperrors.FieldViolation{{Field: "rule", Message: "A rule is required."}}), CodeSegmentRuleRequired)
		}
		if validated, err = ValidateRule(in.Rule, s.registry, s.ruleLimits, segmentScope); err != nil {
			s.metrics.Inc("crm_segment_rule_rejections_total")
			return Segment{}, err
		}
	case SegmentStatic:
		if len(in.Rule) > 0 {
			return Segment{}, crmError(apperrors.Validation("A static segment cannot have a rule.",
				[]apperrors.FieldViolation{{Field: "rule", Message: "Remove the rule or create a dynamic segment."}}),
				CodeSegmentRuleNotAllwd)
		}
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Segment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if branchID != nil {
		owned, ownErr := s.branchBelongsToBusiness(ctx, tx, actor.VendorID, *branchID)
		if ownErr != nil {
			return Segment{}, ownErr
		}
		if !owned {
			return Segment{}, crmError(apperrors.Forbidden("That branch does not belong to this business.", nil), CodeCrossTenant)
		}
	}
	if validated != nil {
		if err = s.validateRuleReferences(ctx, tx, actor.VendorID, segmentScope, validated); err != nil {
			return Segment{}, err
		}
	}

	var hash *string
	if validated != nil {
		hash = &validated.Hash
	}
	segment, err := scanSegment(tx.QueryRow(ctx, `INSERT INTO crm_segments
		(id,business_id,name,normalized_name,description,segment_type,scope_type,branch_id,status,
		 current_definition_version,evaluation_status,definition_hash,created_by_staff_ref,updated_by_staff_ref)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,'DRAFT',1,'NOT_EVALUATED',$9,NULLIF($10,''),NULLIF($10,''))
		RETURNING `+segmentCols,
		newID("crmseg_"), actor.VendorID, name, normalized, description, segmentType, scopeType, branchID,
		hash, actor.StaffID))
	if err != nil {
		if isUniqueViolation(err) {
			return Segment{}, errSegmentExists()
		}
		return Segment{}, err
	}
	if err = s.insertVersion(ctx, tx, actor, segment, 1, validated, "created"); err != nil {
		return Segment{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.segment.created", "crm_segment", segment.ID, derefOr(branchID),
		nil, map[string]any{"name": segment.Name, "segment_type": segmentType, "scope_type": scopeType}, ""); err != nil {
		return Segment{}, err
	}
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.segment.created", "crm_segment", segment.ID, segment.Version,
		map[string]any{"segment_ref": segment.ID, "segment_type": segmentType, "scope_type": scopeType,
			"definition_version": int64(1), "definition_hash": hash}); err != nil {
		return Segment{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Segment{}, err
	}
	s.metrics.Inc("crm_segments_total", "type", segmentType, "scope", scopeType)
	return segment, nil
}

// insertVersion writes one immutable definition revision.
func (s *Service) insertVersion(ctx context.Context, tx pgx.Tx, actor Actor, segment Segment,
	definitionVersion int64, rule *ValidatedRule, reason string) error {
	var (
		schemaVersion *int
		ruleJSON      []byte
		exclusionJSON []byte
		hash          *string
		dependencies  = []string{}
	)
	if rule != nil {
		version := rule.SchemaVersion
		schemaVersion = &version
		hash = &rule.Hash
		dependencies = rule.Dependencies
		var document canonicalDocument
		if err := json.Unmarshal(rule.Canonical, &document); err != nil {
			return err
		}
		var err error
		if ruleJSON, err = json.Marshal(document.Root); err != nil {
			return err
		}
		if document.Exclusion != nil {
			if exclusionJSON, err = json.Marshal(document.Exclusion); err != nil {
				return err
			}
		}
	}
	_, err := tx.Exec(ctx, `INSERT INTO crm_segment_versions
		(id,business_id,segment_id,definition_version,segment_type,scope_type,branch_id,rule_schema_version,
		 rule_json,exclusion_rule_json,canonical_rule_hash,dependency_set,created_by_staff_ref,change_reason)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NULLIF($13,''),$14)`,
		newID("crmsv_"), actor.VendorID, segment.ID, definitionVersion, segment.SegmentType, segment.ScopeType,
		segment.BranchID, schemaVersion, ruleJSON, exclusionJSON, hash, dependencies, actor.StaffID, reason)
	return err
}

// UpdateSegmentInput edits a segment. Type and scope are immutable: changing
// either would reinterpret every stored version and materialized generation.
type UpdateSegmentInput struct {
	Name            *string
	Description     *string
	Rule            json.RawMessage
	ChangeReason    string
	ExpectedVersion int64
}

// UpdateSegment renames a segment and/or replaces its rule.
//
// A rule change mints a new immutable version, marks the segment STALE and
// enqueues a fresh evaluation — but deliberately leaves the previous successful
// membership in place until the new run succeeds, so editing a live audience
// never empties it.
func (s *Service) UpdateSegment(ctx context.Context, actor Actor, scope Scope, segmentID string, in UpdateSegmentInput) (Segment, error) {
	if err := s.assertSegmentationWrites(); err != nil {
		return Segment{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Segment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := s.loadSegmentForUpdate(ctx, tx, actor.VendorID, segmentID)
	if err != nil {
		return Segment{}, err
	}
	if err = s.assertSegmentVisible(scope, current); err != nil {
		return Segment{}, err
	}
	if err = authorizeObjectScope(scope, current.ScopeType, current.BranchID, CodeSegmentScopeDenied); err != nil {
		return Segment{}, err
	}
	if current.Status == SegmentArchived {
		return Segment{}, errSegmentState("An archived segment cannot be changed.")
	}
	if in.ExpectedVersion > 0 && current.Version != in.ExpectedVersion {
		return Segment{}, errCRMVersionConflict()
	}

	name, normalized := current.Name, current.NormalizedName
	if in.Name != nil {
		if name, normalized, err = validateSegmentName(*in.Name); err != nil {
			return Segment{}, err
		}
	}
	description := current.Description
	if in.Description != nil {
		if description, err = validateTagDescription(*in.Description); err != nil {
			return Segment{}, err
		}
	}

	definitionVersion := current.CurrentDefinitionVersion
	hash := current.DefinitionHash
	evaluationStatus := current.EvaluationStatus
	var validated *ValidatedRule
	if len(in.Rule) > 0 {
		if !current.Dynamic() {
			return Segment{}, crmError(apperrors.Conflict("A static segment cannot have a rule.", nil), CodeSegmentRuleNotAllwd)
		}
		segmentScope := current.Scope()
		if validated, err = ValidateRule(in.Rule, s.registry, s.ruleLimits, segmentScope); err != nil {
			s.metrics.Inc("crm_segment_rule_rejections_total")
			return Segment{}, err
		}
		if err = s.validateRuleReferences(ctx, tx, actor.VendorID, segmentScope, validated); err != nil {
			return Segment{}, err
		}
		if hash == nil || *hash != validated.Hash {
			definitionVersion = current.CurrentDefinitionVersion + 1
			hash = &validated.Hash
			// A previously-current materialization no longer describes the new
			// definition. It stays readable, but it is honestly labelled STALE.
			if evaluationStatus == EvalCurrent || evaluationStatus == EvalStale || evaluationStatus == EvalFailed {
				evaluationStatus = EvalStale
			}
		}
	}

	updated, err := scanSegment(tx.QueryRow(ctx, `UPDATE crm_segments
		SET name=$1, normalized_name=$2, description=$3, current_definition_version=$4, definition_hash=$5,
			evaluation_status=$6,
			stale_since=CASE WHEN $6='STALE' AND stale_since IS NULL THEN now()
							 WHEN $6<>'STALE' THEN stale_since ELSE stale_since END,
			updated_by_staff_ref=NULLIF($7,''), updated_at=now(), version=version+1
		WHERE business_id=$8 AND id=$9 AND version=$10 AND status <> 'ARCHIVED'
		RETURNING `+segmentCols,
		name, normalized, description, definitionVersion, hash, evaluationStatus, actor.StaffID,
		actor.VendorID, segmentID, current.Version))
	if errors.Is(err, pgx.ErrNoRows) {
		return Segment{}, errCRMVersionConflict()
	}
	if err != nil {
		if isUniqueViolation(err) {
			return Segment{}, errSegmentExists()
		}
		return Segment{}, err
	}

	if definitionVersion != current.CurrentDefinitionVersion {
		reason := strings.TrimSpace(in.ChangeReason)
		if reason == "" {
			reason = "rule updated"
		}
		if err = s.insertVersion(ctx, tx, actor, updated, definitionVersion, validated, reason); err != nil {
			return Segment{}, err
		}
		if s.flags.SegmentMaterializationEnabled && !s.flags.CommerceKillSwitch {
			if _, err = s.enqueueEvaluationTx(ctx, tx, actor, updated); err != nil {
				return Segment{}, err
			}
		}
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.segment.updated", "crm_segment", segmentID, derefOr(current.BranchID),
		map[string]any{"name": current.Name, "definition_version": current.CurrentDefinitionVersion},
		map[string]any{"name": updated.Name, "definition_version": definitionVersion}, in.ChangeReason); err != nil {
		return Segment{}, err
	}
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.segment.updated", "crm_segment", segmentID, updated.Version,
		map[string]any{"segment_ref": segmentID, "definition_version": definitionVersion, "definition_hash": hash}); err != nil {
		return Segment{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Segment{}, err
	}
	return updated, nil
}

// SetSegmentStatus activates or archives a segment.
func (s *Service) SetSegmentStatus(ctx context.Context, actor Actor, scope Scope, segmentID, target string, expectedVersion int64) (Segment, error) {
	if err := s.assertSegmentationWrites(); err != nil {
		return Segment{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Segment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := s.loadSegmentForUpdate(ctx, tx, actor.VendorID, segmentID)
	if err != nil {
		return Segment{}, err
	}
	if err = s.assertSegmentVisible(scope, current); err != nil {
		return Segment{}, err
	}
	if err = authorizeObjectScope(scope, current.ScopeType, current.BranchID, CodeSegmentScopeDenied); err != nil {
		return Segment{}, err
	}
	if expectedVersion > 0 && current.Version != expectedVersion {
		return Segment{}, errCRMVersionConflict()
	}
	// DRAFT -> ACTIVE, DRAFT -> ARCHIVED and ACTIVE -> ARCHIVED are the whole
	// lifecycle. ARCHIVED is terminal: an audience that was retired must not
	// quietly come back with the membership it had months ago.
	switch {
	case current.Status == target:
		return current, nil
	case current.Status == SegmentArchived:
		return Segment{}, errSegmentState("An archived segment cannot be reactivated.")
	case target == SegmentActive && current.Status != SegmentDraft:
		return Segment{}, errSegmentState("Only a draft segment can be activated.")
	case target != SegmentActive && target != SegmentArchived:
		return Segment{}, tagValidation("status", "Unsupported segment status.")
	}

	updated, err := scanSegment(tx.QueryRow(ctx, `UPDATE crm_segments
		SET status=$1, updated_by_staff_ref=NULLIF($2,''), updated_at=now(), version=version+1
		WHERE business_id=$3 AND id=$4 AND version=$5 RETURNING `+segmentCols,
		target, actor.StaffID, actor.VendorID, segmentID, current.Version))
	if errors.Is(err, pgx.ErrNoRows) {
		return Segment{}, errCRMVersionConflict()
	}
	if err != nil {
		return Segment{}, err
	}

	action := "vendor.crm.segment.archived"
	if target == SegmentActive {
		action = "vendor.crm.segment.activated"
	}
	if target == SegmentActive && updated.Dynamic() && s.flags.SegmentMaterializationEnabled && !s.flags.CommerceKillSwitch {
		if _, err = s.enqueueEvaluationTx(ctx, tx, actor, updated); err != nil {
			return Segment{}, err
		}
	}
	if target == SegmentArchived {
		// Retiring a segment drops its materialized membership: keeping a live
		// audience attached to a retired definition is exactly how a stale
		// audience gets reused by a later campaign.
		if _, err = tx.Exec(ctx, `DELETE FROM crm_segment_memberships WHERE business_id=$1 AND segment_id=$2`,
			actor.VendorID, segmentID); err != nil {
			return Segment{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE crm_segments SET current_member_count=0, current_evaluation_id=NULL
			WHERE business_id=$1 AND id=$2`, actor.VendorID, segmentID); err != nil {
			return Segment{}, err
		}
		updated.CurrentMemberCount = 0
		updated.CurrentEvaluationID = nil
	}
	if err = s.auditTx(ctx, tx, actor, action, "crm_segment", segmentID, derefOr(current.BranchID),
		map[string]any{"status": current.Status}, map[string]any{"status": target}, ""); err != nil {
		return Segment{}, err
	}
	if err = s.publishTx(ctx, tx, actor, action, "crm_segment", segmentID, updated.Version,
		map[string]any{"segment_ref": segmentID, "status": target}); err != nil {
		return Segment{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Segment{}, err
	}
	return updated, nil
}

func validateSegmentName(name string) (display, normalized string, err error) {
	display = strings.TrimSpace(name)
	if display == "" {
		return "", "", tagValidation("name", "A segment name is required.")
	}
	if len([]rune(display)) < 2 || len([]rune(display)) > 120 {
		return "", "", tagValidation("name", "A segment name must be between 2 and 120 characters.")
	}
	for _, r := range display {
		if r < 0x20 || r == 0x7f {
			return "", "", tagValidation("name", "A segment name cannot contain control characters.")
		}
	}
	if strings.ContainsAny(display, "<>") {
		return "", "", tagValidation("name", "A segment name cannot contain markup characters.")
	}
	normalized = NormalizeTagName(display)
	if normalized == "" {
		return "", "", tagValidation("name", "A segment name must contain visible characters.")
	}
	return display, normalized, nil
}

// validateRuleReferences checks that every object a rule names is one the
// segment's own scope may see.
//
// Without this, a branch-scoped segment could reference another branch's
// private tag and use membership counts as an oracle for that branch's
// relationships — the rule would look legitimate because every field and
// operator is legal.
func (s *Service) validateRuleReferences(ctx context.Context, q querier, businessID string, scope SegmentScope, rule *ValidatedRule) error {
	var tagIDs []string
	var branchRefs []string
	var referenceErr error
	visit := func(condition *Condition) {
		switch condition.Field.Type {
		case FieldTagReference:
			tagIDs = append(tagIDs, condition.Strings[0])
		default:
			if condition.Field.Key == "relationship.branch_ref" {
				branchRefs = append(branchRefs, condition.Strings...)
			}
		}
	}
	walkConditions(rule.Root, visit)
	walkConditions(rule.Exclusion, visit)

	for _, branchRef := range branchRefs {
		if scope.Branch() && branchRef != scope.BranchID {
			referenceErr = ruleError(CodeSegmentScopeDenied, "root",
				"A branch-scoped segment can only reference its own branch.", nil)
		}
	}
	if referenceErr != nil {
		return referenceErr
	}
	if len(branchRefs) > 0 {
		var owned int
		if err := q.QueryRow(ctx, `SELECT count(*) FROM vendor_branches WHERE business_id=$1 AND id = ANY($2)`,
			businessID, dedupe(branchRefs)).Scan(&owned); err != nil {
			return err
		}
		if owned != len(dedupe(branchRefs)) {
			return ruleError(CodeSegmentScopeDenied, "root", "A referenced branch is not available.", nil)
		}
	}
	if len(tagIDs) == 0 {
		return nil
	}
	unique := dedupe(tagIDs)
	rows, err := q.Query(ctx, `SELECT id, scope_type, branch_id FROM crm_tags WHERE business_id=$1 AND id = ANY($2)`,
		businessID, unique)
	if err != nil {
		return err
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var id, scopeType string
		var branchID *string
		if err = rows.Scan(&id, &scopeType, &branchID); err != nil {
			return err
		}
		seen++
		if scopeType == ScopeBranch && (!scope.Branch() || branchID == nil || *branchID != scope.BranchID) {
			return ruleError(CodeSegmentScopeDenied, "root", "A referenced tag is not available at this scope.", nil)
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if seen != len(unique) {
		return ruleError(CodeSegmentScopeDenied, "root", "A referenced tag is not available.", nil)
	}
	return nil
}

func dedupe(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
