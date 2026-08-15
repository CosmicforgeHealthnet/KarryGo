package crm

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Corporate interactions and tasks are the accepted Section 8C engine with a
// corporate subject, not a parallel implementation of it.
//
// Everything that makes activity trustworthy — the safe-note rules, the
// immutable revision history, the OPEN/IN_PROGRESS/COMPLETED/CANCELLED state
// machine, the overdue derivation, the assignment eligibility check, the audit
// and outbox writes — is reached by calling the same code individual CRM calls.
// This file is a facade: it resolves and authorises the corporate subject,
// validates the optional corporate references, and then hands over.
//
// The alternative — corporate copies of those tables and that state machine —
// would have created two definitions of "completed" that could drift, and two
// places to fix the next concurrency bug.

// authorizeCorporateActivity proves the actor may reach a corporate account
// before any of its activity is read or written.
//
// Corporate activity visibility follows the ACCOUNT's branch scope, not the
// activity row's own visibility column. That is why corporate rows are stored
// with BUSINESS visibility and no branch: their reach is a property of the
// organization relationship, and duplicating it onto every interaction would be
// a second copy to keep in step.
func (s *Service) authorizeCorporateActivity(ctx context.Context, q querier, scope Scope, businessID, corporateID string) error {
	profile, err := s.loadCorporate(ctx, q, businessID, corporateID)
	if err != nil {
		return err
	}
	return s.assertCorporateVisible(ctx, q, scope, profile)
}

// corporateActivityRefs are the optional corporate references an interaction or
// task may carry. Each must belong to the same corporate account.
type corporateActivityRefs struct {
	ContactID    *string
	LocationID   *string
	CostCenterID *string
}

// resolveCorporateActivityRefs validates the optional references.
//
// The composite foreign keys already make a cross-account reference unstorable;
// this exists so the caller gets a stable, explaining error instead of a
// constraint violation, and so a redacted contact cannot be attached to
// anything new.
func (s *Service) resolveCorporateActivityRefs(ctx context.Context, tx pgx.Tx, businessID, corporateID string,
	contactRef, locationRef, costCenterRef string) (corporateActivityRefs, error) {
	var refs corporateActivityRefs

	if value := strings.TrimSpace(contactRef); value != "" {
		contact, err := s.lockContact(ctx, tx, businessID, corporateID, value)
		if err != nil {
			return refs, err
		}
		if !contact.Actionable() {
			// A redacted individual keeps their historical activity; nothing
			// new may be recorded against them.
			return refs, s.contactNotActionableError(contact)
		}
		refs.ContactID = &contact.ID
	}
	if value := strings.TrimSpace(locationRef); value != "" {
		if err := s.assertLocationBelongs(ctx, tx, businessID, corporateID, &value); err != nil {
			return refs, err
		}
		refs.LocationID = &value
	}
	if value := strings.TrimSpace(costCenterRef); value != "" {
		if err := s.assertCostCenterBelongs(ctx, tx, businessID, corporateID, &value); err != nil {
			return refs, err
		}
		refs.CostCenterID = &value
	}
	return refs, nil
}

// ---------------------------------------------------------------------------
// Corporate interactions
// ---------------------------------------------------------------------------

// CreateCorporateInteractionInput records a commercial conversation with an
// organization.
type CreateCorporateInteractionInput struct {
	Type, Direction, Outcome, Subject, Note string
	OccurredAt                              time.Time
	ContactRef, LocationRef, CostCenterRef  string
}

// CreateCorporateInteraction records an interaction against a corporate account.
//
// Validation is the Section 8C validator, unchanged: the same type, direction
// and outcome vocabularies, the same 200/5000 character bounds, and the same
// refusal of control characters and markup. There is deliberately no richer
// corporate note format.
func (s *Service) CreateCorporateInteraction(ctx context.Context, actor Actor, scope Scope, corporateID string, in CreateCorporateInteractionInput) (Interaction, error) {
	if err := s.assertCorporateWrites(); err != nil {
		return Interaction{}, err
	}
	if !s.flags.InteractionsEnabled {
		return Interaction{}, errWritesDisabled()
	}
	validated, err := validateInteractionInput(CreateInteractionInput{
		Type: in.Type, Direction: in.Direction, Outcome: in.Outcome, Subject: in.Subject,
		Note: in.Note, Visibility: VisibilityBusiness, OccurredAt: in.OccurredAt,
	}, s.now())
	if err != nil {
		return Interaction{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Interaction{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	profile, err := s.corporateForWrite(ctx, tx, actor, scope, corporateID)
	if err != nil {
		return Interaction{}, err
	}
	refs, err := s.resolveCorporateActivityRefs(ctx, tx, actor.VendorID, corporateID,
		in.ContactRef, in.LocationRef, in.CostCenterRef)
	if err != nil {
		return Interaction{}, err
	}

	interaction, err := scanInteraction(tx.QueryRow(ctx, `INSERT INTO crm_interactions
		(id,business_id,corporate_customer_id,corporate_contact_id,corporate_location_id,corporate_cost_center_id,
		 interaction_type,direction,outcome,subject,note_body,visibility_scope,occurred_at,recorded_by_staff_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'BUSINESS',$12,$13) RETURNING `+interactionCols,
		newID("crmi_"), actor.VendorID, profile.ID, refs.ContactID, refs.LocationID, refs.CostCenterID,
		validated.Type, validated.Direction, validated.Outcome, validated.Subject, validated.Note,
		validated.OccurredAt, actor.StaffID))
	if err != nil {
		return Interaction{}, err
	}
	// The same immutable revision history individual interactions get: the
	// CREATE snapshot is revision 1 and every correction appends the next.
	if err = s.appendInteractionRevision(ctx, tx, actor, interaction, ""); err != nil {
		return Interaction{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.interaction.created", "crm_interaction", interaction.ID, "",
		nil, safeCorporateActivityAudit(interaction.ID, profile.ID, refs, map[string]any{
			"type": interaction.Type, "direction": interaction.Direction, "outcome": interaction.Outcome,
			"version": interaction.Version}), ""); err != nil {
		return Interaction{}, err
	}
	// The event carries the shape of the interaction and never its note body.
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.interaction.created", "crm_interaction",
		interaction.ID, interaction.Version, map[string]any{
			"interaction_ref": interaction.ID, "corporate_ref": profile.ID, "type": interaction.Type,
			"direction": interaction.Direction, "outcome": interaction.Outcome,
			"occurred_at": interaction.OccurredAt, "version": interaction.Version}); err != nil {
		return Interaction{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Interaction{}, err
	}
	s.metrics.Inc("crm_corporate_interaction_create_total", "type", interaction.Type)
	return interaction, nil
}

// ListCorporateInteractions pages an account's commercial timeline.
func (s *Service) ListCorporateInteractions(ctx context.Context, actor Actor, scope Scope, corporateID string, opts InteractionListOptions) ([]Interaction, bool, error) {
	if _, err := s.GetCorporateCustomer(ctx, actor, scope, corporateID); err != nil {
		return nil, false, err
	}
	opts.CorporateCustomerRef = corporateID
	opts.CustomerProfileRef = ""
	// The branch filter is meaningless for a corporate row (its reach comes
	// from the account) and accepting one would silently return nothing.
	opts.BranchRef = ""
	return s.ListInteractions(ctx, actor, scope, opts)
}

// GetCorporateInteraction reads one interaction through its corporate account.
func (s *Service) GetCorporateInteraction(ctx context.Context, actor Actor, scope Scope, corporateID, interactionID string) (Interaction, error) {
	if _, err := s.GetCorporateCustomer(ctx, actor, scope, corporateID); err != nil {
		return Interaction{}, err
	}
	interaction, err := scanInteraction(s.db.QueryRow(ctx, `SELECT `+interactionCols+` FROM crm_interactions
		WHERE business_id=$1 AND corporate_customer_id=$2 AND id=$3 AND display_suppressed=false`,
		actor.VendorID, corporateID, interactionID))
	if errors.Is(err, pgx.ErrNoRows) {
		// The corporate id is part of the predicate, so an interaction from
		// another account presented under this URL is simply not found.
		return Interaction{}, interactionNotFound()
	}
	return interaction, err
}

// ---------------------------------------------------------------------------
// Corporate tasks
// ---------------------------------------------------------------------------

// CreateCorporateTaskInput is internal Vendor staff work about an organization.
// It is not an order job, a delivery job, a support dispute or a procurement
// workflow step.
type CreateCorporateTaskInput struct {
	Title, Description, Priority           string
	AssignedStaffRef                       string
	DueAt                                  *time.Time
	ContactRef, LocationRef, CostCenterRef string
	SourceInteractionRef                   string
}

// CreateCorporateTask opens a staff task about a corporate account.
//
// The state machine, priorities, history and assignee eligibility are Section
// 8C's, reached through the same helpers individual tasks use.
func (s *Service) CreateCorporateTask(ctx context.Context, actor Actor, scope Scope, corporateID string, in CreateCorporateTaskInput) (Task, error) {
	if err := s.assertCorporateWrites(); err != nil {
		return Task{}, err
	}
	if !s.flags.TasksEnabled {
		return Task{}, errWritesDisabled()
	}
	validated, err := validateTaskInput(CreateTaskInput{
		Title: in.Title, Description: in.Description, Priority: in.Priority,
		Visibility: VisibilityBusiness, AssignedStaffRef: in.AssignedStaffRef, DueAt: in.DueAt,
	})
	if err != nil {
		return Task{}, err
	}
	var assignee *string
	if value := strings.TrimSpace(validated.AssignedStaffRef); value != "" {
		assignee = &value
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Task{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	profile, err := s.corporateForWrite(ctx, tx, actor, scope, corporateID)
	if err != nil {
		return Task{}, err
	}
	refs, err := s.resolveCorporateActivityRefs(ctx, tx, actor.VendorID, corporateID,
		in.ContactRef, in.LocationRef, in.CostCenterRef)
	if err != nil {
		return Task{}, err
	}
	// Assignee eligibility is Section 8C's rule, evaluated against the branches
	// the corporate account is scoped to: a task about a branch-scoped account
	// cannot be handed to somebody who could not see the account.
	if err = s.assertCorporateAssignee(ctx, tx, actor.VendorID, profile, assignee); err != nil {
		return Task{}, err
	}
	var source *string
	if value := strings.TrimSpace(in.SourceInteractionRef); value != "" {
		if _, err = s.GetCorporateInteraction(ctx, actor, scope, corporateID, value); err != nil {
			return Task{}, err
		}
		source = &value
	}

	task, err := scanTask(tx.QueryRow(ctx, `INSERT INTO crm_tasks
		(id,business_id,corporate_customer_id,corporate_contact_id,corporate_location_id,corporate_cost_center_id,
		 visibility_scope,title,description,status,priority,assigned_staff_id,created_by_staff_id,due_at,
		 source_interaction_id)
		VALUES($1,$2,$3,$4,$5,$6,'BUSINESS',$7,$8,'OPEN',$9,$10,$11,$12,$13) RETURNING `+taskCols,
		newID("crmt_"), actor.VendorID, profile.ID, refs.ContactID, refs.LocationID, refs.CostCenterID,
		validated.Title, validated.Description, validated.Priority, assignee, actor.StaffID, validated.DueAt, source))
	if err != nil {
		return Task{}, err
	}
	if assignee != nil {
		yes := true
		task.AssigneeActive = &yes
	}
	if err = s.appendTaskHistory(ctx, tx, actor, task, "CREATED"); err != nil {
		return Task{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.task.created", "crm_task", task.ID, "",
		nil, safeCorporateActivityAudit(task.ID, profile.ID, refs, map[string]any{
			"status": task.Status, "priority": task.Priority, "assigned_staff_ref": task.AssignedStaffID,
			"due_at": task.DueAt, "version": task.Version}), ""); err != nil {
		return Task{}, err
	}
	// The event carries the task's shape and never its description.
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.task.created", "crm_task", task.ID, task.Version,
		map[string]any{"task_ref": task.ID, "corporate_ref": profile.ID, "status": task.Status,
			"priority": task.Priority, "assigned_staff_ref": task.AssignedStaffID, "due_at": task.DueAt,
			"version": task.Version}); err != nil {
		return Task{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Task{}, err
	}
	s.metrics.Inc("crm_corporate_task_create_total", "priority", task.Priority)
	return task, nil
}

// assertCorporateAssignee checks that a proposed assignee is an active member
// of this vendor's staff who could actually see the account.
//
// For a BRANCH_SET account the staff member must hold a role assignment
// covering one of its branches (or a business-wide one). Assigning work on an
// account somebody cannot open would produce a task nobody can act on.
func (s *Service) assertCorporateAssignee(ctx context.Context, tx pgx.Tx, businessID string, profile CorporateProfile, assignee *string) error {
	if assignee == nil {
		return nil
	}
	if profile.ScopeMode == CorporateScopeBusiness {
		var eligible bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_staff s
			WHERE s.id=$1 AND s.vendor_id=$2 AND s.status='active' AND EXISTS(
			 SELECT 1 FROM vendor_staff_role_assignments a WHERE a.staff_id=s.id AND a.revoked_at IS NULL
			   AND a.branch_id IS NULL))`, *assignee, businessID).Scan(&eligible); err != nil {
			return err
		}
		if !eligible {
			return crmError(apperrors.Validation("The selected assignee is not available for this corporate customer.", nil),
				CodeTaskAssigneeInvalid)
		}
		return nil
	}
	var eligible bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_staff s
		WHERE s.id=$1 AND s.vendor_id=$2 AND s.status='active' AND EXISTS(
		 SELECT 1 FROM vendor_staff_role_assignments a WHERE a.staff_id=s.id AND a.revoked_at IS NULL
		   AND (a.branch_id IS NULL OR a.branch_id IN (
			 SELECT branch_id FROM corporate_customer_branch_scopes
			 WHERE business_id=$2 AND corporate_customer_id=$3))))`,
		*assignee, businessID, profile.ID).Scan(&eligible); err != nil {
		return err
	}
	if !eligible {
		return crmError(apperrors.Validation("The selected assignee is not available for this corporate customer.", nil),
			CodeTaskAssigneeInvalid)
	}
	return nil
}

// ListCorporateTasks pages an account's staff tasks.
func (s *Service) ListCorporateTasks(ctx context.Context, actor Actor, scope Scope, corporateID string, opts TaskListOptions) ([]Task, bool, error) {
	if _, err := s.GetCorporateCustomer(ctx, actor, scope, corporateID); err != nil {
		return nil, false, err
	}
	opts.CorporateCustomerRef = corporateID
	opts.CustomerProfileRef = ""
	opts.BranchRef = ""
	return s.ListTasks(ctx, actor, scope, opts)
}

// GetCorporateTask reads one task through its corporate account.
func (s *Service) GetCorporateTask(ctx context.Context, actor Actor, scope Scope, corporateID, taskID string) (Task, error) {
	if _, err := s.GetCorporateCustomer(ctx, actor, scope, corporateID); err != nil {
		return Task{}, err
	}
	task, err := scanTaskRead(s.db.QueryRow(ctx, `SELECT `+taskReadCols("t")+` FROM crm_tasks t
		WHERE business_id=$1 AND corporate_customer_id=$2 AND id=$3 AND display_suppressed=false`,
		actor.VendorID, corporateID, taskID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, taskNotFound()
	}
	return task, err
}

// assertCorporateOwnsTask proves a task belongs to the account named in the
// URL before any Section 8C mutation is delegated to. Without it, a task id
// from account A presented under account B's path would be mutated with B's
// authorisation.
func (s *Service) assertCorporateOwnsTask(ctx context.Context, actor Actor, scope Scope, corporateID, taskID string) error {
	_, err := s.GetCorporateTask(ctx, actor, scope, corporateID, taskID)
	return err
}

// assertCorporateOwnsInteraction is the same nested-resource proof for
// interactions.
func (s *Service) assertCorporateOwnsInteraction(ctx context.Context, actor Actor, scope Scope, corporateID, interactionID string) error {
	_, err := s.GetCorporateInteraction(ctx, actor, scope, corporateID, interactionID)
	return err
}

// cancelCorporateTasksOnArchiveTx retires unfinished work when an account is
// archived.
//
// Cancelling rather than deleting, and recording it in the same immutable task
// history every other transition uses: a task that simply vanished would leave
// an operator wondering what happened to it, and one left OPEN forever would
// sit in queues for an account nobody works any more.
func (s *Service) cancelCorporateTasksOnArchiveTx(ctx context.Context, tx pgx.Tx, actor Actor, profile CorporateProfile) error {
	rows, err := tx.Query(ctx, `UPDATE crm_tasks
		SET status='CANCELLED', cancelled_at=now(), updated_at=now(), version=version+1
		WHERE business_id=$1 AND corporate_customer_id=$2 AND status IN ('OPEN','IN_PROGRESS')
		RETURNING `+taskCols, actor.VendorID, profile.ID)
	if err != nil {
		return err
	}
	var cancelled []Task
	for rows.Next() {
		task, scanErr := scanTask(rows)
		if scanErr != nil {
			rows.Close()
			return scanErr
		}
		cancelled = append(cancelled, task)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, task := range cancelled {
		if err = s.appendTaskHistory(ctx, tx, actor, task, "CANCELLED"); err != nil {
			return err
		}
	}
	if len(cancelled) > 0 {
		s.metrics.Add("crm_corporate_task_archive_cancel_total", float64(len(cancelled)))
	}
	return nil
}

// safeCorporateActivityAudit builds an activity audit payload for a corporate
// subject: references and shape only. The subject, note body and description
// are deliberately absent — Section 8C's own rule, applied unchanged.
func safeCorporateActivityAudit(activityID, corporateID string, refs corporateActivityRefs, extra map[string]any) map[string]any {
	payload := map[string]any{
		"activity_ref": activityID, "corporate_ref": corporateID,
		"contact_ref": refs.ContactID, "location_ref": refs.LocationID, "cost_center_ref": refs.CostCenterID,
	}
	for key, value := range extra {
		payload[key] = value
	}
	return payload
}

// ---------------------------------------------------------------------------
// Corporate read model
// ---------------------------------------------------------------------------

// CorporateSummary is the relationship overview for one account.
//
// It reports counts and a bounded slice of recent activity. The full child
// collections are deliberately not inlined: an account with two thousand
// contacts would otherwise make one read unbounded, and every collection has
// its own paginated endpoint.
type CorporateSummary struct {
	BranchRefs       []string
	ContactCount     int
	LocationCount    int
	CostCenterCount  int
	MembershipCount  int
	OpenTaskCount    int
	OverdueTaskCount int
	Commercial       *CorporateCommercialProjection `json:"commercial,omitempty"`
	RFQ              *CorporateRFQProjection        `json:"rfq,omitempty"`
}

// CorporateSummaryFor builds the relationship overview.
func (s *Service) CorporateSummaryFor(ctx context.Context, actor Actor, profile CorporateProfile) (CorporateSummary, error) {
	var summary CorporateSummary
	branches, err := s.BranchScopesFor(ctx, s.db, actor.VendorID, profile.ID)
	if err != nil {
		return summary, err
	}
	summary.BranchRefs = branches

	// Counts exclude archived children and privacy-suppressed contacts, so the
	// figures describe what an operator can actually act on rather than
	// everything the tables have ever held.
	if err = s.db.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM corporate_customer_contacts
			   WHERE business_id=$1 AND corporate_customer_id=$2 AND status='ACTIVE' AND privacy_suppressed=false),
			(SELECT count(*) FROM corporate_customer_locations
			   WHERE business_id=$1 AND corporate_customer_id=$2 AND status='ACTIVE'),
			(SELECT count(*) FROM corporate_customer_cost_centers
			   WHERE business_id=$1 AND corporate_customer_id=$2 AND status='ACTIVE'),
			(SELECT count(*) FROM corporate_customer_memberships
			   WHERE business_id=$1 AND corporate_customer_id=$2 AND status='ACTIVE'),
			(SELECT count(*) FROM crm_tasks
			   WHERE business_id=$1 AND corporate_customer_id=$2 AND display_suppressed=false
				 AND status IN ('OPEN','IN_PROGRESS')),
			(SELECT count(*) FROM crm_tasks
			   WHERE business_id=$1 AND corporate_customer_id=$2 AND display_suppressed=false
				 AND status IN ('OPEN','IN_PROGRESS') AND due_at < $3)`,
		actor.VendorID, profile.ID, s.now()).Scan(&summary.ContactCount, &summary.LocationCount,
		&summary.CostCenterCount, &summary.MembershipCount, &summary.OpenTaskCount,
		&summary.OverdueTaskCount); err != nil {
		return summary, err
	}
	var commercial CorporateCommercialProjection
	err = s.db.QueryRow(ctx, `SELECT state,order_count,completed_order_count,recurring_order_count,bulk_order_count,net_spend_minor,currency,first_order_at,last_order_at,last_bulk_order_at,last_recurring_order_at,last_branch_ref FROM crm_corporate_commercial_projections WHERE business_id=$1 AND corporate_customer_id=$2`, actor.VendorID, profile.ID).Scan(&commercial.State, &commercial.OrderCount, &commercial.CompletedOrderCount, &commercial.RecurringOrderCount, &commercial.BulkOrderCount, &commercial.NetSpendMinor, &commercial.Currency, &commercial.FirstOrderAt, &commercial.LastOrderAt, &commercial.LastBulkOrderAt, &commercial.LastRecurringOrderAt, &commercial.LastBranchRef)
	if err == nil {
		summary.Commercial = &commercial
	} else if err != pgx.ErrNoRows {
		return summary, err
	}
	var rfq CorporateRFQProjection
	err = s.db.QueryRow(ctx, `SELECT state,rfq_count,quoted_rfq_count,accepted_quote_count,rejected_quote_count,converted_rfq_count,quote_conversion_bps,resolved_rfq_count,latest_rfq_ref,latest_rfq_outcome FROM crm_corporate_rfq_projections WHERE business_id=$1 AND corporate_customer_id=$2`, actor.VendorID, profile.ID).Scan(&rfq.State, &rfq.RFQCount, &rfq.QuotedRFQCount, &rfq.AcceptedQuoteCount, &rfq.RejectedQuoteCount, &rfq.ConvertedRFQCount, &rfq.QuoteConversionBPS, &rfq.ResolvedRFQCount, &rfq.LatestRFQRef, &rfq.LatestOutcome)
	if err == nil {
		summary.RFQ = &rfq
	} else if err != pgx.ErrNoRows {
		return summary, err
	}
	return summary, nil
}

// RecentCorporateInteractions returns a bounded slice of the account's timeline
// for the overview.
func (s *Service) RecentCorporateInteractions(ctx context.Context, actor Actor, corporateID string, limit int) ([]Interaction, error) {
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	rows, err := s.db.Query(ctx, `SELECT `+interactionCols+` FROM crm_interactions
		WHERE business_id=$1 AND corporate_customer_id=$2 AND display_suppressed=false
		ORDER BY occurred_at DESC, id DESC LIMIT $3`, actor.VendorID, corporateID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Interaction, 0, limit)
	for rows.Next() {
		interaction, scanErr := scanInteraction(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, interaction)
	}
	return items, rows.Err()
}
