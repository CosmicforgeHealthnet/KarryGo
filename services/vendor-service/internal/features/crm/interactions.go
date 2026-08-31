package crm

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

const (
	InteractionNote            = "NOTE"
	InteractionPhoneCall       = "PHONE_CALL"
	InteractionInPerson        = "IN_PERSON"
	InteractionEmail           = "EMAIL"
	InteractionSMS             = "SMS"
	InteractionInApp           = "IN_APP"
	InteractionSocial          = "SOCIAL"
	InteractionSalesFollowUp   = "SALES_FOLLOW_UP"
	InteractionServiceFollowUp = "SERVICE_FOLLOW_UP"
	InteractionFeedback        = "FEEDBACK"
	InteractionAccountReview   = "ACCOUNT_REVIEW"
	InteractionOther           = "OTHER"

	DirectionInbound       = "INBOUND"
	DirectionOutbound      = "OUTBOUND"
	DirectionInternal      = "INTERNAL"
	DirectionNotApplicable = "NOT_APPLICABLE"

	OutcomeNone             = "NONE"
	OutcomeSuccessful       = "SUCCESSFUL"
	OutcomeNoResponse       = "NO_RESPONSE"
	OutcomeFollowUpRequired = "FOLLOW_UP_REQUIRED"
	OutcomeResolved         = "RESOLVED"
	OutcomeUnresolved       = "UNRESOLVED"
	OutcomeCustomerDeclined = "CUSTOMER_DECLINED"
	OutcomeVendorDeclined   = "VENDOR_DECLINED"
	OutcomeEscalated        = "ESCALATED"
	OutcomeCancelled        = "CANCELLED"

	VisibilityBusiness = "BUSINESS"
	VisibilityBranch   = "BRANCH"

	CodeInteractionNotFound  = "CRM_INTERACTION_NOT_FOUND"
	CodeInteractionScope     = "CRM_INTERACTION_SCOPE_DENIED"
	CodeInteractionImmutable = "CRM_INTERACTION_IMMUTABLE"
	CodeNoteTooLong          = "CRM_NOTE_TOO_LONG"
	CodeNoteInvalid          = "CRM_NOTE_INVALID"
)

var interactionTypes = map[string]bool{
	InteractionNote: true, InteractionPhoneCall: true, InteractionInPerson: true,
	InteractionEmail: true, InteractionSMS: true, InteractionInApp: true,
	InteractionSocial: true, InteractionSalesFollowUp: true,
	InteractionServiceFollowUp: true, InteractionFeedback: true,
	InteractionAccountReview: true, InteractionOther: true,
}
var interactionDirections = map[string]bool{
	DirectionInbound: true, DirectionOutbound: true, DirectionInternal: true, DirectionNotApplicable: true,
}
var interactionOutcomes = map[string]bool{
	OutcomeNone: true, OutcomeSuccessful: true, OutcomeNoResponse: true,
	OutcomeFollowUpRequired: true, OutcomeResolved: true, OutcomeUnresolved: true,
	OutcomeCustomerDeclined: true, OutcomeVendorDeclined: true, OutcomeEscalated: true, OutcomeCancelled: true,
}

// Interaction is a vendor-local relationship event. It intentionally carries
// no canonical customer contact or identity fields.
//
// An interaction has exactly one subject: either an individual customer
// relationship or (from Section 8G) a corporate customer. The database enforces
// that with a CHECK, so a subject-less or double-subject row cannot be stored.
type Interaction struct {
	ID, BusinessID, CustomerProfileID, OriginalProfileID string
	BranchID                                             *string
	Type, Direction, Outcome, Subject, NoteBody          string
	VisibilityScope, Source                              string
	OccurredAt                                           time.Time
	RecordedByStaffID                                    string
	DisplaySuppressed                                    bool
	CreatedAt, UpdatedAt                                 time.Time
	Version                                              int64

	// Section 8G corporate subject. Nil on every individual interaction, which
	// is what keeps the accepted Section 8C behaviour untouched.
	CorporateCustomerID   *string
	CorporateContactID    *string
	CorporateLocationID   *string
	CorporateCostCenterID *string
	RelatedOrderRef       *string
	RelatedRFQRef         *string
	RelatedSupportCaseRef *string
}

// IsCorporate reports whether this interaction belongs to an organization
// rather than to an individual customer relationship.
func (v Interaction) IsCorporate() bool { return v.CorporateCustomerID != nil }

type InteractionView struct {
	InteractionRef        string    `json:"interaction_ref"`
	CustomerProfileRef    string    `json:"customer_profile_ref"`
	BranchRef             *string   `json:"branch_ref,omitempty"`
	Type                  string    `json:"type"`
	Direction             string    `json:"direction"`
	Outcome               string    `json:"outcome"`
	Subject               string    `json:"subject"`
	Note                  string    `json:"note"`
	Visibility            string    `json:"visibility"`
	OccurredAt            time.Time `json:"occurred_at"`
	RecordedByRef         string    `json:"recorded_by_staff_ref"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	Version               int64     `json:"version"`
	RelatedOrderRef       *string   `json:"related_order_ref,omitempty"`
	RelatedRFQRef         *string   `json:"related_rfq_ref,omitempty"`
	RelatedSupportCaseRef *string   `json:"related_support_case_ref,omitempty"`
}

func InteractionViewOf(v Interaction) InteractionView {
	return InteractionView{
		InteractionRef: v.ID, CustomerProfileRef: v.CustomerProfileID, BranchRef: v.BranchID,
		Type: v.Type, Direction: v.Direction, Outcome: v.Outcome, Subject: v.Subject,
		Note: v.NoteBody, Visibility: v.VisibilityScope, OccurredAt: v.OccurredAt,
		RecordedByRef: v.RecordedByStaffID, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, Version: v.Version,
		RelatedOrderRef: v.RelatedOrderRef, RelatedRFQRef: v.RelatedRFQRef, RelatedSupportCaseRef: v.RelatedSupportCaseRef,
	}
}

type InteractionRevision struct {
	RevisionRef, InteractionID, Subject, NoteBody, Outcome, CorrectionReason, ChangedByStaffID string
	RevisionVersion                                                                            int64
	OccurredAt, CreatedAt                                                                      time.Time
}

type InteractionRevisionView struct {
	RevisionRef      string    `json:"revision_ref"`
	Version          int64     `json:"version"`
	Subject          string    `json:"subject"`
	Note             string    `json:"note"`
	Outcome          string    `json:"outcome"`
	OccurredAt       time.Time `json:"occurred_at"`
	CorrectionReason string    `json:"correction_reason,omitempty"`
	ChangedByRef     string    `json:"changed_by_staff_ref"`
	CreatedAt        time.Time `json:"created_at"`
}

type CreateInteractionInput struct {
	CustomerProfileRef                                    string
	BranchRef                                             string
	Type                                                  string
	Direction                                             string
	Outcome                                               string
	Subject                                               string
	Note                                                  string
	Visibility                                            string
	OccurredAt                                            time.Time
	RelatedOrderRef, RelatedRFQRef, RelatedSupportCaseRef string
}

type CorrectInteractionInput struct {
	Subject          *string
	Note             *string
	Outcome          *string
	OccurredAt       *time.Time
	CorrectionReason string
	ExpectedVersion  int64
}

type InteractionListOptions struct {
	CustomerProfileRef                                      string
	Type, Direction, Outcome, BranchRef, RecordedBy, Search string
	OccurredFrom, OccurredTo                                *time.Time
	Limit, Offset                                           int

	// CorporateCustomerRef selects the corporate timeline instead of the
	// individual one. Left empty — which is every accepted Section 8C caller —
	// the list is restricted to individual interactions, so corporate rows can
	// never appear in a customer timeline they do not belong to.
	CorporateCustomerRef string
}

func (o InteractionListOptions) bounded() InteractionListOptions {
	if o.Limit <= 0 {
		o.Limit = 50
	}
	if o.Limit > 200 {
		o.Limit = 200
	}
	if o.Offset < 0 {
		o.Offset = 0
	}
	return o
}

// interactionCols reads an interaction whatever its subject.
//
// Section 8G made the subject columns nullable so the same engine can carry a
// corporate interaction (see migration 000022). The two individual columns are
// COALESCEd to the empty string rather than made pointers throughout, which
// keeps every accepted Section 8C call site and view untouched: an individual
// row still scans exactly as it always did, and a corporate row is recognised
// by its corporate reference rather than by a nil check scattered everywhere.
const interactionCols = `id,business_id,COALESCE(customer_profile_id,'') AS customer_profile_id,
COALESCE(original_profile_id,'') AS original_profile_id,branch_id,
interaction_type,direction,outcome,subject,note_body,visibility_scope,occurred_at,
recorded_by_staff_id,source,display_suppressed,created_at,updated_at,version,
corporate_customer_id,corporate_contact_id,corporate_location_id,corporate_cost_center_id,
related_order_ref,related_rfq_ref,related_support_case_ref`

func scanInteraction(row rowScanner) (Interaction, error) {
	var v Interaction
	err := row.Scan(&v.ID, &v.BusinessID, &v.CustomerProfileID, &v.OriginalProfileID, &v.BranchID,
		&v.Type, &v.Direction, &v.Outcome, &v.Subject, &v.NoteBody, &v.VisibilityScope, &v.OccurredAt,
		&v.RecordedByStaffID, &v.Source, &v.DisplaySuppressed, &v.CreatedAt, &v.UpdatedAt, &v.Version,
		&v.CorporateCustomerID, &v.CorporateContactID, &v.CorporateLocationID, &v.CorporateCostCenterID,
		&v.RelatedOrderRef, &v.RelatedRFQRef, &v.RelatedSupportCaseRef)
	return v, err
}

func interactionNotFound() error {
	return crmError(apperrors.NotFound("Interaction not found.", nil), CodeInteractionNotFound)
}

func interactionValidation(field, message, code string) error {
	err := apperrors.Validation("Check your interaction details.", []apperrors.FieldViolation{{Field: field, Message: message}})
	if code != "" {
		err.Details = map[string]any{"code": code}
	}
	return err
}

func normalizePlainText(value string, max int, required, singleLine bool, field string) (string, error) {
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
	value = strings.TrimSpace(value)
	if required && value == "" {
		return "", interactionValidation(field, "This field is required.", CodeNoteInvalid)
	}
	if len([]rune(value)) > max {
		code := CodeNoteInvalid
		if field == "note" || field == "description" {
			code = CodeNoteTooLong
		}
		return "", interactionValidation(field, "This field is too long.", code)
	}
	if singleLine && strings.Contains(value, "\n") {
		return "", interactionValidation(field, "This field must be one line.", CodeNoteInvalid)
	}
	for _, r := range value {
		if (unicode.IsControl(r) && r != '\n') || unicode.Is(unicode.Cf, r) {
			return "", interactionValidation(field, "Plain text contains an unsupported control character.", CodeNoteInvalid)
		}
	}
	if strings.ContainsAny(value, "<>") || strings.Contains(strings.ToLower(value), "data:") {
		return "", interactionValidation(field, "Only safe plain text is accepted.", CodeNoteInvalid)
	}
	return value, nil
}

func validateInteractionInput(in CreateInteractionInput, now time.Time) (CreateInteractionInput, error) {
	in.Type = strings.ToUpper(strings.TrimSpace(in.Type))
	in.Direction = strings.ToUpper(strings.TrimSpace(in.Direction))
	in.Outcome = strings.ToUpper(strings.TrimSpace(in.Outcome))
	if in.Outcome == "" {
		in.Outcome = OutcomeNone
	}
	in.Visibility = strings.ToUpper(strings.TrimSpace(in.Visibility))
	if in.Visibility == "" {
		in.Visibility = VisibilityBusiness
	}
	if !interactionTypes[in.Type] {
		return in, interactionValidation("type", "Unsupported interaction type.", CodeNoteInvalid)
	}
	if !interactionDirections[in.Direction] {
		return in, interactionValidation("direction", "Unsupported interaction direction.", CodeNoteInvalid)
	}
	if !interactionOutcomes[in.Outcome] {
		return in, interactionValidation("outcome", "Unsupported interaction outcome.", CodeNoteInvalid)
	}
	if in.Visibility != VisibilityBusiness && in.Visibility != VisibilityBranch {
		return in, interactionValidation("visibility", "Visibility must be BUSINESS or BRANCH.", CodeInteractionScope)
	}
	if in.Visibility == VisibilityBranch && strings.TrimSpace(in.BranchRef) == "" {
		return in, interactionValidation("branch_ref", "A branch is required for branch visibility.", CodeInteractionScope)
	}
	if in.Visibility == VisibilityBusiness && strings.TrimSpace(in.BranchRef) != "" {
		return in, interactionValidation("branch_ref", "A business interaction cannot name a branch.", CodeInteractionScope)
	}
	var err error
	if in.Subject, err = normalizePlainText(in.Subject, 200, true, true, "subject"); err != nil {
		return in, err
	}
	if in.Note, err = normalizePlainText(in.Note, 5000, false, false, "note"); err != nil {
		return in, err
	}
	if in.OccurredAt.IsZero() {
		in.OccurredAt = now
	}
	in.OccurredAt = in.OccurredAt.UTC()
	if in.OccurredAt.After(now.Add(5 * time.Minute)) {
		return in, interactionValidation("occurred_at", "Occurrence time cannot be in the future.", CodeNoteInvalid)
	}
	return in, nil
}

func authorizeActivityScope(scope Scope, visibility string, branchID *string) error {
	if visibility == VisibilityBusiness {
		if !scope.Global {
			return crmError(apperrors.Forbidden("This business-wide CRM record is not available.", nil), CodeInteractionScope)
		}
		return nil
	}
	if branchID == nil || (!scope.Global && !containsString(scope.BranchIDs, *branchID)) {
		return crmError(apperrors.NotFound("CRM record not found.", nil), CodeInteractionNotFound)
	}
	return nil
}

// lockActionableProfile is the shared privacy/merge fence for 8C writes.
func (s *Service) lockActionableProfile(ctx context.Context, tx pgx.Tx, actor Actor, scope Scope, profileID, visibility string, branchID *string) (Profile, error) {
	profile, err := scanProfile(tx.QueryRow(ctx, `/* crm_activity_profile_fence */ SELECT `+profileCols+` FROM vendor_customer_profiles
		WHERE business_id=$1 AND id=$2 FOR UPDATE`, actor.VendorID, profileID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, errProfileNotFound()
	}
	if err != nil {
		return Profile{}, err
	}
	if profile.IsRedacted() {
		return Profile{}, crmError(apperrors.Conflict("This customer is redacted.", nil), CodeProfileRedacted)
	}
	if profile.IsMergedSource() {
		return Profile{}, crmError(apperrors.Conflict("This customer has been merged.", nil), CodeProfileMerged)
	}
	if err = authorizeActivityScope(scope, visibility, branchID); err != nil {
		return Profile{}, err
	}
	if branchID != nil {
		var visible bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM crm_customer_branch_relations
			WHERE business_id=$1 AND profile_id=$2 AND branch_id=$3)`, actor.VendorID, profileID, *branchID).Scan(&visible)
		if err != nil {
			return Profile{}, err
		}
		if !visible {
			return Profile{}, crmError(apperrors.NotFound("Customer relationship not found.", nil), CodeInteractionScope)
		}
	}
	return profile, nil
}

func (s *Service) CreateInteraction(ctx context.Context, actor Actor, scope Scope, in CreateInteractionInput) (Interaction, error) {
	if !s.flags.InteractionsEnabled || !s.flags.WritesEnabled || s.flags.CommerceKillSwitch {
		return Interaction{}, errWritesDisabled()
	}
	var err error
	if in, err = validateInteractionInput(in, s.now()); err != nil {
		return Interaction{}, err
	}
	var branchID *string
	if in.Visibility == VisibilityBranch {
		branch := strings.TrimSpace(in.BranchRef)
		branchID = &branch
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Interaction{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = s.lockActionableProfile(ctx, tx, actor, scope, strings.TrimSpace(in.CustomerProfileRef), in.Visibility, branchID); err != nil {
		return Interaction{}, err
	}
	interaction, err := scanInteraction(tx.QueryRow(ctx, `INSERT INTO crm_interactions
		(id,business_id,customer_profile_id,original_profile_id,branch_id,interaction_type,direction,outcome,
		 subject,note_body,visibility_scope,occurred_at,recorded_by_staff_id,related_order_ref,related_rfq_ref,related_support_case_ref)
		VALUES($1,$2,$3,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING `+interactionCols,
		newID("crmi_"), actor.VendorID, strings.TrimSpace(in.CustomerProfileRef), branchID, in.Type, in.Direction,
		in.Outcome, in.Subject, in.Note, in.Visibility, in.OccurredAt, actor.StaffID, optional(in.RelatedOrderRef), optional(in.RelatedRFQRef), optional(in.RelatedSupportCaseRef)))
	if err != nil {
		return Interaction{}, err
	}
	if err = s.appendInteractionRevision(ctx, tx, actor, interaction, ""); err != nil {
		return Interaction{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.interaction.created", "crm_interaction", interaction.ID, derefOr(branchID), nil,
		map[string]any{"profile_ref": interaction.CustomerProfileID, "type": interaction.Type, "direction": interaction.Direction, "outcome": interaction.Outcome, "version": interaction.Version}, ""); err != nil {
		return Interaction{}, err
	}
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.interaction.created", "crm_interaction", interaction.ID, interaction.Version,
		map[string]any{"interaction_ref": interaction.ID, "customer_profile_ref": interaction.CustomerProfileID, "branch_ref": branchID, "type": interaction.Type, "direction": interaction.Direction, "outcome": interaction.Outcome, "occurred_at": interaction.OccurredAt, "version": interaction.Version}); err != nil {
		return Interaction{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Interaction{}, err
	}
	s.metrics.Inc("crm_interaction_create_total", "type", interaction.Type)
	return interaction, nil
}

func (s *Service) appendInteractionRevision(ctx context.Context, tx pgx.Tx, actor Actor, v Interaction, reason string) error {
	_, err := tx.Exec(ctx, `INSERT INTO crm_interaction_revisions
		(id,business_id,interaction_id,revision_version,subject,note_body,outcome,occurred_at,correction_reason,changed_by_staff_id,related_order_ref,related_rfq_ref,related_support_case_ref)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, newID("crmir_"), actor.VendorID, v.ID, v.Version,
		v.Subject, v.NoteBody, v.Outcome, v.OccurredAt, reason, actor.StaffID, v.RelatedOrderRef, v.RelatedRFQRef, v.RelatedSupportCaseRef)
	return err
}

func activityScopeSQL(alias string, scope Scope, args *[]any) string {
	if scope.Global {
		return "TRUE"
	}
	*args = append(*args, scope.BranchIDs)
	return alias + `.visibility_scope='BRANCH' AND ` + alias + `.branch_id = ANY($` + itoa(len(*args)) + `)`
}

func (s *Service) ListInteractions(ctx context.Context, actor Actor, scope Scope, opts InteractionListOptions) ([]Interaction, bool, error) {
	opts = opts.bounded()
	args := []any{actor.VendorID}
	where := []string{"i.business_id=$1", "i.display_suppressed=false"}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, strings.Replace(clause, "?", "$"+itoa(len(args)), 1))
	}
	// The subject is always pinned. An unfiltered list would otherwise mix a
	// business's corporate timeline into its individual one the moment Section
	// 8G started writing rows.
	//
	// The branch rule differs by subject, which is why it lives here rather
	// than in one shared clause: an individual interaction is reached through
	// its own visibility scope, while a corporate interaction is reached
	// through the corporate account's branch scope — already established by the
	// caller before this list runs.
	if opts.CorporateCustomerRef != "" {
		add("i.corporate_customer_id=?", opts.CorporateCustomerRef)
	} else {
		where = append(where, "i.corporate_customer_id IS NULL", activityScopeSQL("i", scope, &args))
	}
	if opts.CustomerProfileRef != "" {
		add("i.customer_profile_id=?", opts.CustomerProfileRef)
	}
	if opts.Type != "" {
		add("i.interaction_type=?", strings.ToUpper(opts.Type))
	}
	if opts.Direction != "" {
		add("i.direction=?", strings.ToUpper(opts.Direction))
	}
	if opts.Outcome != "" {
		add("i.outcome=?", strings.ToUpper(opts.Outcome))
	}
	if opts.RecordedBy != "" {
		add("i.recorded_by_staff_id=?", opts.RecordedBy)
	}
	if opts.BranchRef != "" {
		if !scope.Global && !containsString(scope.BranchIDs, opts.BranchRef) {
			return nil, false, interactionNotFound()
		}
		add("i.branch_id=?", opts.BranchRef)
	}
	if opts.OccurredFrom != nil {
		add("i.occurred_at>=?", opts.OccurredFrom.UTC())
	}
	if opts.OccurredTo != nil {
		add("i.occurred_at<=?", opts.OccurredTo.UTC())
	}
	if opts.Search != "" {
		if len([]rune(opts.Search)) > 120 {
			return nil, false, interactionValidation("search", "Search text is too long.", CodeNoteInvalid)
		}
		add("(i.subject ILIKE '%' || ? || '%' OR i.note_body ILIKE '%' || $"+itoa(len(args)+1)+" || '%')", opts.Search)
	}
	args = append(args, opts.Limit+1, opts.Offset)
	rows, err := s.db.Query(ctx, `SELECT `+interactionCols+` FROM crm_interactions i WHERE `+strings.Join(where, " AND ")+`
		ORDER BY i.occurred_at DESC,i.id DESC LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]Interaction, 0, opts.Limit)
	for rows.Next() {
		v, scanErr := scanInteraction(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(items) > opts.Limit
	if hasMore {
		items = items[:opts.Limit]
	}
	return items, hasMore, nil
}

// GetInteraction reads one INDIVIDUAL interaction. A corporate interaction is
// reported not found here: it is reachable only through its corporate account,
// where the caller's reach over that account is what authorises the read.
func (s *Service) GetInteraction(ctx context.Context, actor Actor, scope Scope, id string) (Interaction, error) {
	v, err := scanInteraction(s.db.QueryRow(ctx, `SELECT `+interactionCols+` FROM crm_interactions
		WHERE business_id=$1 AND id=$2 AND display_suppressed=false AND corporate_customer_id IS NULL`,
		actor.VendorID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Interaction{}, interactionNotFound()
	}
	if err != nil {
		return Interaction{}, err
	}
	if err = authorizeActivityScope(scope, v.VisibilityScope, v.BranchID); err != nil {
		return Interaction{}, interactionNotFound()
	}
	return v, nil
}

// interactionAnySubject resolves an interaction whichever kind of subject it
// has, authorising each kind by its own rule. It is the single place that
// knows both, so no caller has to.
func (s *Service) interactionAnySubject(ctx context.Context, actor Actor, scope Scope, id string) (Interaction, error) {
	v, err := scanInteraction(s.db.QueryRow(ctx, `SELECT `+interactionCols+` FROM crm_interactions
		WHERE business_id=$1 AND id=$2 AND display_suppressed=false`, actor.VendorID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Interaction{}, interactionNotFound()
	}
	if err != nil {
		return Interaction{}, err
	}
	if v.IsCorporate() {
		if err = s.authorizeCorporateActivity(ctx, s.db, scope, actor.VendorID, *v.CorporateCustomerID); err != nil {
			return Interaction{}, interactionNotFound()
		}
		return v, nil
	}
	if err = authorizeActivityScope(scope, v.VisibilityScope, v.BranchID); err != nil {
		return Interaction{}, interactionNotFound()
	}
	return v, nil
}

func (s *Service) CorrectInteraction(ctx context.Context, actor Actor, scope Scope, id string, in CorrectInteractionInput) (Interaction, error) {
	if !s.flags.InteractionsEnabled || !s.flags.WritesEnabled || s.flags.CommerceKillSwitch {
		return Interaction{}, errWritesDisabled()
	}
	if in.ExpectedVersion < 1 {
		return Interaction{}, interactionValidation("expected_version", "Expected version is required.", CodeVersionConflict)
	}
	if in.Subject == nil && in.Note == nil && in.Outcome == nil && in.OccurredAt == nil {
		return Interaction{}, interactionValidation("correction", "At least one corrected field is required.", CodeInteractionImmutable)
	}
	reason, err := normalizePlainText(in.CorrectionReason, 500, true, false, "correction_reason")
	if err != nil {
		return Interaction{}, err
	}
	probe, err := scanInteraction(s.db.QueryRow(ctx, `SELECT `+interactionCols+` FROM crm_interactions WHERE business_id=$1 AND id=$2`, actor.VendorID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Interaction{}, interactionNotFound()
	}
	if err != nil {
		return Interaction{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Interaction{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// The fence depends on the subject: an individual interaction locks the
	// customer's profile row, a corporate one locks the corporate account. Both
	// are the same discipline applied to the thing that actually owns the row.
	if probe.IsCorporate() {
		if _, err = s.corporateForWrite(ctx, tx, actor, scope, *probe.CorporateCustomerID); err != nil {
			return Interaction{}, err
		}
	} else if _, err = s.lockActionableProfile(ctx, tx, actor, scope, probe.CustomerProfileID, probe.VisibilityScope, probe.BranchID); err != nil {
		return Interaction{}, err
	}
	current, err := scanInteraction(tx.QueryRow(ctx, `SELECT `+interactionCols+` FROM crm_interactions
		WHERE business_id=$1 AND id=$2 AND display_suppressed=false FOR UPDATE`, actor.VendorID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Interaction{}, interactionNotFound()
	}
	if err != nil {
		return Interaction{}, err
	}
	if current.Version != in.ExpectedVersion {
		return Interaction{}, crmError(apperrors.Conflict("Interaction changed since it was loaded.", nil), CodeVersionConflict)
	}
	updated := current
	if in.Subject != nil {
		updated.Subject, err = normalizePlainText(*in.Subject, 200, true, true, "subject")
	}
	if err == nil && in.Note != nil {
		updated.NoteBody, err = normalizePlainText(*in.Note, 5000, false, false, "note")
	}
	if err == nil && in.Outcome != nil {
		value := strings.ToUpper(strings.TrimSpace(*in.Outcome))
		if !interactionOutcomes[value] {
			err = interactionValidation("outcome", "Unsupported interaction outcome.", CodeNoteInvalid)
		} else {
			updated.Outcome = value
		}
	}
	if err == nil && in.OccurredAt != nil {
		value := in.OccurredAt.UTC()
		if value.After(s.now().Add(5 * time.Minute)) {
			err = interactionValidation("occurred_at", "Occurrence time cannot be in the future.", CodeNoteInvalid)
		} else {
			updated.OccurredAt = value
		}
	}
	if err != nil {
		return Interaction{}, err
	}
	updated, err = scanInteraction(tx.QueryRow(ctx, `UPDATE crm_interactions SET subject=$1,note_body=$2,outcome=$3,occurred_at=$4,
		updated_at=now(),version=version+1 WHERE business_id=$5 AND id=$6 AND version=$7 RETURNING `+interactionCols,
		updated.Subject, updated.NoteBody, updated.Outcome, updated.OccurredAt, actor.VendorID, id, in.ExpectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return Interaction{}, crmError(apperrors.Conflict("Interaction changed since it was loaded.", nil), CodeVersionConflict)
	}
	if err != nil {
		return Interaction{}, err
	}
	if err = s.appendInteractionRevision(ctx, tx, actor, updated, reason); err != nil {
		return Interaction{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.interaction.corrected", "crm_interaction", updated.ID, derefOr(updated.BranchID),
		map[string]any{"outcome": current.Outcome, "occurred_at": current.OccurredAt, "version": current.Version},
		map[string]any{"outcome": updated.Outcome, "occurred_at": updated.OccurredAt, "version": updated.Version}, reason); err != nil {
		return Interaction{}, err
	}
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.interaction.corrected", "crm_interaction", updated.ID, updated.Version,
		map[string]any{"interaction_ref": updated.ID, "customer_profile_ref": updated.CustomerProfileID, "branch_ref": updated.BranchID, "type": updated.Type, "outcome": updated.Outcome, "occurred_at": updated.OccurredAt, "version": updated.Version}); err != nil {
		return Interaction{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Interaction{}, err
	}
	return updated, nil
}

func (s *Service) ListInteractionRevisions(ctx context.Context, actor Actor, scope Scope, id string) ([]InteractionRevision, error) {
	// Subject-agnostic on purpose: an immutable revision history reads the same
	// way for a corporate interaction as for an individual one, and each is
	// authorised by its own subject's rule inside the resolver.
	if _, err := s.interactionAnySubject(ctx, actor, scope, id); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT id,interaction_id,revision_version,subject,note_body,outcome,occurred_at,
		correction_reason,changed_by_staff_id,created_at FROM crm_interaction_revisions
		WHERE business_id=$1 AND interaction_id=$2 ORDER BY revision_version`, actor.VendorID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []InteractionRevision
	for rows.Next() {
		var v InteractionRevision
		if err = rows.Scan(&v.RevisionRef, &v.InteractionID, &v.RevisionVersion, &v.Subject, &v.NoteBody, &v.Outcome, &v.OccurredAt, &v.CorrectionReason, &v.ChangedByStaffID, &v.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	return items, rows.Err()
}
