// Package admincommands implements Support's closed Secure Admin command
// dictionary (shared/go/admincommand.SupportSpec) against Support's own state
// machines, inside the transaction that also records the immutable decision.
//
// Support adjudicates; it never moves money. RECORD_DISPUTE_REMEDY only links
// a Wallet operation the Bridge has already observed CONFIRMED.
package admincommands

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"cosmicforge/logistics/services/support-dispute-service/internal/features/adminresources"
	supportclients "cosmicforge/logistics/services/support-dispute-service/internal/features/support/clients"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Outcome is the owner's report on every applied command.
const Outcome = "APPLIED_BY_SUPPORT_OWNER"

// EvidenceValidator is Media's verdict for an object bound to this case.
type EvidenceValidator interface {
	Validate(ctx context.Context, objectID, bindingType, bindingID string) (supportclients.EvidenceVerdict, error)
}

// ErrMediaUnavailable: Media could not be asked. Never a domain refusal.
var ErrMediaUnavailable = errors.New("media evidence validation unavailable")

type Executor struct {
	// Media validates ATTACH_EVIDENCE. Nil refuses the action as unavailable.
	Media EvidenceValidator
}

func Spec() admincommand.Spec { return admincommand.SupportSpec() }

// permissions maps each action onto its native Support grant.
var permissions = map[string]string{
	"ASSIGN_CASE":           adminresources.PermissionCaseManage,
	"REPLY_TO_CASE":         adminresources.PermissionCaseManage,
	"TRANSITION_CASE":       adminresources.PermissionCaseManage,
	"ESCALATE_CASE":         adminresources.PermissionCaseManage,
	"CLOSE_CASE":            adminresources.PermissionCaseManage,
	"ATTACH_EVIDENCE":       adminresources.PermissionEvidenceAttach,
	"ESCALATE_SOS":          adminresources.PermissionSOSManage,
	"RESOLVE_DISPUTE":       adminresources.PermissionDisputeDecide,
	"RECORD_DISPUTE_REMEDY": adminresources.PermissionDisputeDecide,
}

// Permission returns the native grant an action requires.
func Permission(action string) string { return permissions[action] }

func (e Executor) AuthorizeStaff(ctx context.Context, tx pgx.Tx, staff, kind, id string) error {
	return adminresources.Source().AuthorizeStaff(ctx, tx, staff, kind, id)
}

func parseID(id string) (uuid.UUID, bool) {
	parsed, err := uuid.Parse(id)
	return parsed, err == nil && parsed != uuid.Nil && parsed.String() == id
}

func (e Executor) Current(ctx context.Context, tx pgx.Tx, kind, id string) (int64, string, error) {
	ref, ok := parseID(id)
	if !ok {
		return 0, "", admincommand.ErrNotFound
	}
	var version int64
	var state string
	var err error
	switch kind {
	case "cases":
		err = tx.QueryRow(ctx, `SELECT admin_version, status::text FROM complaints WHERE id = $1`, ref).Scan(&version, &state)
	case "sos-cases":
		err = tx.QueryRow(ctx, `SELECT admin_version, status::text FROM complaints WHERE id = $1 AND lower(priority) = 'emergency'`, ref).Scan(&version, &state)
	case "disputes":
		err = tx.QueryRow(ctx, `SELECT admin_version, outcome::text FROM disputes WHERE id = $1`, ref).Scan(&version, &state)
	default:
		return 0, "", admincommand.ErrNotFound
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", admincommand.ErrNotFound
	}
	if err != nil {
		return 0, "", err
	}
	return version, strings.ToUpper(state), nil
}

func (e Executor) Apply(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	var out admincommand.Transition
	permission := permissions[in.Action]
	if permission == "" {
		return out, admincommand.ErrInvalid
	}
	if err := adminresources.RequireMutationGrant(ctx, tx, in.Staff, permission); err != nil {
		if errors.Is(err, adminresources.ErrNoGrant) {
			return out, adminpolicy.ErrDenied
		}
		return out, err
	}
	ref, ok := parseID(in.SubjectID)
	if !ok {
		return out, admincommand.ErrNotFound
	}
	var err error
	switch in.SubjectKind {
	case "cases", "sos-cases":
		out, err = e.applyCase(ctx, tx, ref, in)
	case "disputes":
		out, err = e.applyDispute(ctx, tx, ref, in)
	default:
		return out, admincommand.ErrNotFound
	}
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && (pg.Code == "23505" || pg.Code == "23514") {
			return admincommand.Transition{}, admincommand.ErrConflict
		}
		return admincommand.Transition{}, err
	}
	return out, nil
}

type caseRow struct {
	version              int64
	status, priority     string
	escalation, assignee *string
}

func terminal(status string) bool { return status == "resolved" || status == "closed" }

// transitions is the TRANSITION_CASE adjacency: OPEN <-> UNDER_REVIEW <->
// AWAITING_EVIDENCE, plus ESCALATED -> UNDER_REVIEW to de-escalate. Terminal
// states (RESOLVED, CLOSED) have no exit.
var transitions = map[string]map[string]bool{
	"open":              {"under_review": true},
	"under_review":      {"open": true, "awaiting_evidence": true},
	"awaiting_evidence": {"under_review": true},
	"escalated":         {"under_review": true},
}

func (e Executor) applyCase(ctx context.Context, tx pgx.Tx, ref uuid.UUID, in admincommand.Execution) (admincommand.Transition, error) {
	var out admincommand.Transition
	var row caseRow
	err := tx.QueryRow(ctx, `SELECT admin_version, status::text, lower(priority), escalation_level, assigned_to FROM complaints WHERE id = $1 FOR UPDATE`, ref).
		Scan(&row.version, &row.status, &row.priority, &row.escalation, &row.assignee)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, admincommand.ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if in.SubjectKind == "sos-cases" && row.priority != "emergency" {
		return out, admincommand.ErrNotFound
	}
	if row.version != in.ExpectedVersion {
		return out, admincommand.ErrConflict
	}
	if terminal(row.status) {
		// Every case action is refused on a resolved or closed case.
		return out, admincommand.ErrRejected
	}
	next := row.status
	event := map[string]any{}
	var eventType string
	switch in.Action {
	case "ASSIGN_CASE":
		assignee := in.Params["assignee_ref"]
		if row.assignee != nil && *row.assignee == assignee {
			return out, admincommand.ErrRejected
		}
		if _, err := tx.Exec(ctx, `UPDATE complaints SET assigned_to = $2, updated_at = now() WHERE id = $1`, ref, assignee); err != nil {
			return out, err
		}
		eventType, event["assignee"] = "admin_assigned", assignee
	case "REPLY_TO_CASE":
		clientID, ok := parseID(in.Params["client_message_id"])
		if !ok {
			return out, admincommand.ErrInvalid
		}
		var existing string
		err := tx.QueryRow(ctx, `SELECT id::text FROM support_chat_messages WHERE complaint_id = $1 AND client_message_id = $2`, ref, clientID).Scan(&existing)
		if err == nil {
			// The same client message is already stored: never a second copy.
			return out, admincommand.ErrConflict
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return out, err
		}
		var messageID string
		if err := tx.QueryRow(ctx, `INSERT INTO support_chat_messages (complaint_id, sender_type, sender_id, content, client_message_id)
 VALUES ($1, 'admin', $2, $3, $4) RETURNING id::text`, ref, in.Staff, in.Params["message"], clientID).Scan(&messageID); err != nil {
			return out, err
		}
		// Delivery to the complainant happens after commit, tracked separately.
		if _, err := tx.Exec(ctx, `INSERT INTO support_admin_reply_deliveries (message_id, complaint_id) VALUES ($1, $2)`, messageID, ref); err != nil {
			return out, err
		}
		eventType, event["message_id"], event["client_message_id"] = "admin_replied", messageID, clientID.String()
	case "TRANSITION_CASE":
		target := strings.ToLower(in.Params["status"])
		if !transitions[row.status][target] {
			return out, admincommand.ErrRejected
		}
		if _, err := tx.Exec(ctx, `UPDATE complaints SET status = $2::complaint_status, updated_at = now() WHERE id = $1`, ref, target); err != nil {
			return out, err
		}
		next, eventType, event["status"] = target, "status_changed", target
	case "ESCALATE_CASE", "ESCALATE_SOS":
		level := in.Params["escalation"]
		eventType = "case_escalated"
		if in.Action == "ESCALATE_SOS" {
			if row.priority != "emergency" {
				return out, admincommand.ErrRejected
			}
			level, eventType = in.Params["level"], "sos_escalated"
		}
		if row.status == "escalated" && row.escalation != nil && *row.escalation == level {
			return out, admincommand.ErrRejected
		}
		if _, err := tx.Exec(ctx, `UPDATE complaints SET status = 'escalated', escalation_level = $2, updated_at = now() WHERE id = $1`, ref, level); err != nil {
			return out, err
		}
		next, event["escalation_level"] = "escalated", level
	case "CLOSE_CASE":
		closure := in.Params["closure"]
		next = "closed"
		if closure == "RESOLVED" {
			next = "resolved"
		}
		if _, err := tx.Exec(ctx, `UPDATE complaints SET status = $2::complaint_status, resolved_at = now(), updated_at = now() WHERE id = $1`, ref, next); err != nil {
			return out, err
		}
		eventType, event["closure"], event["status"] = "case_closed", closure, next
	case "ATTACH_EVIDENCE":
		objectID := in.Params["media_object_id"]
		var duplicate bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM complaint_evidence WHERE complaint_id = $1 AND media_asset_id = $2)`, ref, objectID).Scan(&duplicate); err != nil {
			return out, err
		}
		if duplicate {
			return out, admincommand.ErrRejected
		}
		if e.Media == nil {
			return out, ErrMediaUnavailable
		}
		verdict, err := e.Media.Validate(ctx, objectID, "COMPLAINT", ref.String())
		if err != nil {
			return out, ErrMediaUnavailable
		}
		if !verdict.Usable {
			return out, admincommand.ErrRejected
		}
		if _, err := tx.Exec(ctx, `INSERT INTO complaint_evidence (complaint_id, uploader_type, uploader_id, media_asset_id) VALUES ($1, 'admin', $2, $3)`, ref, in.Staff, objectID); err != nil {
			return out, err
		}
		eventType, event["media_object_id"] = "evidence_added", objectID
	default:
		return out, admincommand.ErrInvalid
	}
	if err := recordEvent(ctx, tx, ref, in, eventType, event); err != nil {
		return out, err
	}
	var version int64
	if err := tx.QueryRow(ctx, `UPDATE complaints SET admin_version = admin_version + 1 WHERE id = $1 RETURNING admin_version`, ref).Scan(&version); err != nil {
		return out, err
	}
	if version != in.ExpectedVersion+1 {
		return out, admincommand.ErrConflict
	}
	return admincommand.Transition{PriorVersion: row.version, PriorState: strings.ToUpper(row.status), ResultingState: strings.ToUpper(next), Outcome: Outcome}, nil
}

var outcomes = map[string]string{"FAVOUR_COMPLAINANT": "favour_complainant", "FAVOUR_RESPONDENT": "favour_respondent", "SPLIT": "split", "DISMISSED": "dismissed"}

func (e Executor) applyDispute(ctx context.Context, tx pgx.Tx, ref uuid.UUID, in admincommand.Execution) (admincommand.Transition, error) {
	var out admincommand.Transition
	var version int64
	var outcome, remedy string
	var complaintID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT admin_version, outcome::text, remedy_state, complaint_id FROM disputes WHERE id = $1 FOR UPDATE`, ref).Scan(&version, &outcome, &remedy, &complaintID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, admincommand.ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if version != in.ExpectedVersion {
		return out, admincommand.ErrConflict
	}
	next := outcome
	event := map[string]any{"dispute_id": ref.String()}
	var eventType string
	switch in.Action {
	case "RESOLVE_DISPUTE":
		target, ok := outcomes[in.Params["outcome"]]
		if !ok {
			return out, admincommand.ErrInvalid
		}
		if outcome != "pending" {
			return out, admincommand.ErrRejected
		}
		if _, err := tx.Exec(ctx, `UPDATE disputes SET outcome = $2::dispute_outcome, adjudicator_id = $3, resolved_at = now(), updated_at = now() WHERE id = $1`, ref, target, in.Staff); err != nil {
			return out, err
		}
		next, eventType, event["outcome"] = target, "dispute_resolved", target
	case "RECORD_DISPUTE_REMEDY":
		if outcome != "favour_complainant" && outcome != "split" {
			return out, admincommand.ErrRejected
		}
		if remedy != "NONE" {
			// A remedy is recorded exactly once.
			return out, admincommand.ErrRejected
		}
		operation, ok := parseID(in.Params["wallet_operation_id"])
		amount, err := strconv.ParseInt(in.Params["amount_minor"], 10, 64)
		if !ok || err != nil || amount <= 0 {
			return out, admincommand.ErrRejected
		}
		if _, err := tx.Exec(ctx, `UPDATE disputes SET remedy_state = 'RECORDED', remedy_wallet_operation_id = $2, remedy_amount_minor = $3,
 remedy_recorded_at = now(), remedy_recorded_by = $4, updated_at = now() WHERE id = $1`, ref, operation, amount, in.Staff); err != nil {
			return out, err
		}
		eventType, event["wallet_operation_id"], event["amount_minor"] = "dispute_remedy_recorded", operation.String(), amount
	default:
		return out, admincommand.ErrInvalid
	}
	if err := recordEvent(ctx, tx, complaintID, in, eventType, event); err != nil {
		return out, err
	}
	var recorded int64
	if err := tx.QueryRow(ctx, `UPDATE disputes SET admin_version = admin_version + 1 WHERE id = $1 RETURNING admin_version`, ref).Scan(&recorded); err != nil {
		return out, err
	}
	if recorded != in.ExpectedVersion+1 {
		return out, admincommand.ErrConflict
	}
	return admincommand.Transition{PriorVersion: version, PriorState: strings.ToUpper(outcome), ResultingState: strings.ToUpper(next), Outcome: Outcome}, nil
}

// recordEvent writes Support's own audit row in the command transaction. The
// payload carries codes and opaque references only, never message text.
func recordEvent(ctx context.Context, tx pgx.Tx, complaintID uuid.UUID, in admincommand.Execution, eventType string, payload map[string]any) error {
	payload["action"], payload["decision_id"], payload["reason_code"], payload["operation_id"] = in.Action, in.DecisionID, in.ReasonCode, in.OperationID
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO complaint_events (complaint_id, actor_type, actor_id, event_type, payload) VALUES ($1, 'admin', $2, $3, $4)`, complaintID, in.Staff, eventType, raw)
	return err
}

var _ admincommand.Executor = Executor{}
var _ adminowner.StaffAuthorizer = Executor{}
