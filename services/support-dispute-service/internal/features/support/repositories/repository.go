package supportrepositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

type SupportRepository interface {
	CreateComplaint(ctx context.Context, input CreateComplaintInput) (supportmodels.Complaint, error)
	GetComplaintByID(ctx context.Context, id string) (supportmodels.Complaint, error)
	ListComplaintsByComplainant(ctx context.Context, complainantType supportmodels.ComplainantType, complainantID string, limit, offset int) ([]supportmodels.Complaint, error)
	ListComplaints(ctx context.Context, filter ComplaintFilter, limit, offset int) ([]supportmodels.Complaint, error)
	UpdateComplaintStatus(ctx context.Context, id string, status supportmodels.ComplaintStatus, note *string) (supportmodels.Complaint, error)
	UpdateComplaintIdentity(ctx context.Context, id string, name, phone *string) (supportmodels.Complaint, error)

	AddEvidence(ctx context.Context, input AddEvidenceInput) (supportmodels.Evidence, error)
	ListEvidence(ctx context.Context, complaintID string) ([]supportmodels.Evidence, error)

	CreateDispute(ctx context.Context, input CreateDisputeInput) (supportmodels.Dispute, error)
	GetDisputeByComplaintID(ctx context.Context, complaintID string) (supportmodels.Dispute, error)
	GetDisputeByID(ctx context.Context, id string) (supportmodels.Dispute, error)
	ListDisputes(ctx context.Context, filter DisputeFilter, limit, offset int) ([]supportmodels.Dispute, error)
	ResolveDispute(ctx context.Context, id string, outcome supportmodels.DisputeOutcome, note string, adjudicatorID string) (supportmodels.Dispute, error)

	FindActiveSupportChat(ctx context.Context, complainantType supportmodels.ComplainantType, complainantID string) (supportmodels.Complaint, error)
	CreateChatMessage(ctx context.Context, msg supportmodels.ChatMessage) (supportmodels.ChatMessage, error)
	ListChatMessages(ctx context.Context, complaintID string, limit, offset int) ([]supportmodels.ChatMessage, error)
	CountUnread(ctx context.Context, complaintID, readerSenderType string) (int, error)
	MarkRead(ctx context.Context, complaintID, readerSenderType string) error

	RecordEvent(ctx context.Context, complaintID, actorType, actorID, eventType string, payload map[string]any) error
	ListEvents(ctx context.Context, complaintID string) ([]supportmodels.ComplaintEvent, error)

	ListHelpArticles(ctx context.Context, audience string) ([]supportmodels.HelpArticle, error)
}

type CreateComplaintInput struct {
	ComplainantType  supportmodels.ComplainantType
	ComplainantID    string
	ComplainantName  *string
	ComplainantPhone *string
	ServiceType      supportmodels.ServiceType
	BookingReference *string
	Category         *string
	Priority         string
	IncidentLat      *float64
	IncidentLng      *float64
	Subject          string
	Description      string
}

type AddEvidenceInput struct {
	ComplaintID  string
	UploaderType supportmodels.ComplainantType
	UploaderID   string
	MediaAssetID *string
	MediaURL     *string
	Note         *string
}

type CreateDisputeInput struct {
	ComplaintID      string
	ServiceType      supportmodels.ServiceType
	BookingReference *string
	RespondentType   supportmodels.ComplainantType
	RespondentID     string
	RespondentName   *string
	RespondentPhone  *string
}

// ComplaintFilter narrows the admin complaint list. Empty fields are ignored.
type ComplaintFilter struct {
	ServiceType     supportmodels.ServiceType
	Status          supportmodels.ComplaintStatus
	ComplainantType supportmodels.ComplainantType
	Priority        string
}

// DisputeFilter narrows the admin dispute list. Empty fields are ignored.
type DisputeFilter struct {
	ServiceType supportmodels.ServiceType
	Outcome     supportmodels.DisputeOutcome
}

// complaintColumns is the canonical select/return projection for complaints.
// scanComplaint reads these in this exact order.
const complaintColumns = `id::text, complainant_type, complainant_id, complainant_name, complainant_phone,
	service_type, booking_reference, category, priority, incident_lat, incident_lng,
	subject, description, status, assigned_to, resolution_note, resolved_at, created_at, updated_at`

const disputeColumns = `id::text, complaint_id::text, service_type, booking_reference,
	respondent_type, respondent_id, respondent_name, respondent_phone,
	outcome, adjudicator_id, adjudication_note, resolved_at, created_at, updated_at`

// PostgresSupportRepository is the pgxpool-backed implementation.
type PostgresSupportRepository struct {
	db *pgxpool.Pool
}

func NewPostgresSupportRepository(db *pgxpool.Pool) *PostgresSupportRepository {
	return &PostgresSupportRepository{db: db}
}

// ─── Complaints ────────────────────────────────────────────────────────────────

func (r *PostgresSupportRepository) CreateComplaint(ctx context.Context, input CreateComplaintInput) (supportmodels.Complaint, error) {
	priority := input.Priority
	if priority == "" {
		priority = supportmodels.PriorityNormal
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO complaints
		  (complainant_type, complainant_id, complainant_name, complainant_phone,
		   service_type, booking_reference, category, priority, incident_lat, incident_lng,
		   subject, description)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING `+complaintColumns,
		input.ComplainantType, input.ComplainantID, input.ComplainantName, input.ComplainantPhone,
		input.ServiceType, input.BookingReference, input.Category, priority, input.IncidentLat, input.IncidentLng,
		input.Subject, input.Description)
	return scanComplaint(row)
}

func (r *PostgresSupportRepository) GetComplaintByID(ctx context.Context, id string) (supportmodels.Complaint, error) {
	row := r.db.QueryRow(ctx, `SELECT `+complaintColumns+` FROM complaints WHERE id = $1`, id)
	c, err := scanComplaint(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return supportmodels.Complaint{}, apperrors.NotFound("Complaint not found.", err)
	}
	return c, err
}

func (r *PostgresSupportRepository) ListComplaintsByComplainant(ctx context.Context, complainantType supportmodels.ComplainantType, complainantID string, limit, offset int) ([]supportmodels.Complaint, error) {
	// readerSenderType mirrors the complainant_type for chat unread accounting.
	rows, err := r.db.Query(ctx, `
		SELECT `+complaintColumns+`,
		  (SELECT count(*) FROM support_chat_messages m
		     WHERE m.complaint_id = complaints.id
		       AND m.is_read = false
		       AND m.sender_type <> $1) AS unread_count
		FROM complaints
		WHERE complainant_type = $1 AND complainant_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, string(complainantType), complainantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectComplaintsWithUnread(rows)
}

func (r *PostgresSupportRepository) ListComplaints(ctx context.Context, filter ComplaintFilter, limit, offset int) ([]supportmodels.Complaint, error) {
	var conds []string
	var args []any
	add := func(col string, val string) {
		if val == "" {
			return
		}
		args = append(args, val)
		conds = append(conds, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	add("service_type", string(filter.ServiceType))
	add("status", string(filter.Status))
	add("complainant_type", string(filter.ComplainantType))
	add("priority", filter.Priority)

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	args = append(args, limit, offset)
	query := fmt.Sprintf(`
		SELECT %s FROM complaints %s
		ORDER BY CASE priority WHEN 'emergency' THEN 0 WHEN 'high' THEN 1 ELSE 2 END, created_at DESC
		LIMIT $%d OFFSET $%d
	`, complaintColumns, where, len(args)-1, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectComplaints(rows)
}

func (r *PostgresSupportRepository) UpdateComplaintStatus(ctx context.Context, id string, status supportmodels.ComplaintStatus, note *string) (supportmodels.Complaint, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE complaints
		SET status = $2,
		    resolution_note = COALESCE($3, resolution_note),
		    resolved_at = CASE WHEN $2 IN ('resolved','closed') THEN now() ELSE resolved_at END,
		    updated_at = now()
		WHERE id = $1
		RETURNING `+complaintColumns,
		id, status, note)
	c, err := scanComplaint(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return supportmodels.Complaint{}, apperrors.NotFound("Complaint not found.", err)
	}
	return c, err
}

func (r *PostgresSupportRepository) UpdateComplaintIdentity(ctx context.Context, id string, name, phone *string) (supportmodels.Complaint, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE complaints
		SET complainant_name = COALESCE($2, complainant_name),
		    complainant_phone = COALESCE($3, complainant_phone),
		    updated_at = now()
		WHERE id = $1
		RETURNING `+complaintColumns,
		id, name, phone)
	c, err := scanComplaint(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return supportmodels.Complaint{}, apperrors.NotFound("Complaint not found.", err)
	}
	return c, err
}

// ─── Evidence ─────────────────────────────────────────────────────────────────

func (r *PostgresSupportRepository) AddEvidence(ctx context.Context, input AddEvidenceInput) (supportmodels.Evidence, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO complaint_evidence
		  (complaint_id, uploader_type, uploader_id, media_asset_id, media_url, note)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, complaint_id::text, uploader_type, uploader_id,
		          media_asset_id, media_url, note, created_at
	`, input.ComplaintID, input.UploaderType, input.UploaderID,
		input.MediaAssetID, input.MediaURL, input.Note)
	return scanEvidence(row)
}

func (r *PostgresSupportRepository) ListEvidence(ctx context.Context, complaintID string) ([]supportmodels.Evidence, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, complaint_id::text, uploader_type, uploader_id,
		       media_asset_id, media_url, note, created_at
		FROM complaint_evidence
		WHERE complaint_id = $1
		ORDER BY created_at ASC
	`, complaintID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []supportmodels.Evidence
	for rows.Next() {
		e, err := scanEvidence(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// ─── Disputes ─────────────────────────────────────────────────────────────────

func (r *PostgresSupportRepository) CreateDispute(ctx context.Context, input CreateDisputeInput) (supportmodels.Dispute, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO disputes
		  (complaint_id, service_type, booking_reference, respondent_type, respondent_id,
		   respondent_name, respondent_phone)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+disputeColumns,
		input.ComplaintID, input.ServiceType, input.BookingReference,
		input.RespondentType, input.RespondentID, input.RespondentName, input.RespondentPhone)
	return scanDispute(row)
}

func (r *PostgresSupportRepository) GetDisputeByComplaintID(ctx context.Context, complaintID string) (supportmodels.Dispute, error) {
	row := r.db.QueryRow(ctx, `SELECT `+disputeColumns+` FROM disputes WHERE complaint_id = $1 ORDER BY created_at DESC LIMIT 1`, complaintID)
	d, err := scanDispute(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return supportmodels.Dispute{}, apperrors.NotFound("Dispute not found.", err)
	}
	return d, err
}

func (r *PostgresSupportRepository) GetDisputeByID(ctx context.Context, id string) (supportmodels.Dispute, error) {
	row := r.db.QueryRow(ctx, `SELECT `+disputeColumns+` FROM disputes WHERE id = $1`, id)
	d, err := scanDispute(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return supportmodels.Dispute{}, apperrors.NotFound("Dispute not found.", err)
	}
	return d, err
}

func (r *PostgresSupportRepository) ListDisputes(ctx context.Context, filter DisputeFilter, limit, offset int) ([]supportmodels.Dispute, error) {
	var conds []string
	var args []any
	add := func(col, val string) {
		if val == "" {
			return
		}
		args = append(args, val)
		conds = append(conds, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	add("service_type", string(filter.ServiceType))
	add("outcome", string(filter.Outcome))

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	args = append(args, limit, offset)
	query := fmt.Sprintf(`SELECT %s FROM disputes %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		disputeColumns, where, len(args)-1, len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []supportmodels.Dispute
	for rows.Next() {
		d, err := scanDispute(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (r *PostgresSupportRepository) ResolveDispute(ctx context.Context, id string, outcome supportmodels.DisputeOutcome, note string, adjudicatorID string) (supportmodels.Dispute, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE disputes
		SET outcome = $2, adjudication_note = $3, adjudicator_id = $4,
		    resolved_at = now(), updated_at = now()
		WHERE id = $1
		RETURNING `+disputeColumns,
		id, outcome, note, adjudicatorID)
	d, err := scanDispute(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return supportmodels.Dispute{}, apperrors.NotFound("Dispute not found.", err)
	}
	return d, err
}

// ─── Chat messages ────────────────────────────────────────────────────────────

func (r *PostgresSupportRepository) FindActiveSupportChat(ctx context.Context, complainantType supportmodels.ComplainantType, complainantID string) (supportmodels.Complaint, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+complaintColumns+`
		FROM complaints
		WHERE complainant_type = $1
		  AND complainant_id = $2
		  AND service_type = 'platform'
		  AND status NOT IN ('resolved', 'closed')
		ORDER BY created_at DESC
		LIMIT 1
	`, complainantType, complainantID)
	c, err := scanComplaint(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return supportmodels.Complaint{}, apperrors.NotFound("No active support chat found.", err)
	}
	return c, err
}

func (r *PostgresSupportRepository) CreateChatMessage(ctx context.Context, msg supportmodels.ChatMessage) (supportmodels.ChatMessage, error) {
	var result supportmodels.ChatMessage
	err := r.db.QueryRow(ctx, `
		INSERT INTO support_chat_messages (complaint_id, sender_type, sender_id, content, media_url)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, complaint_id::text, sender_type, sender_id, content, media_url, is_read, created_at
	`, msg.ComplaintID, msg.SenderType, msg.SenderID, msg.Content, msg.MediaURL).Scan(
		&result.ID, &result.ComplaintID, &result.SenderType, &result.SenderID,
		&result.Content, &result.MediaURL, &result.IsRead, &result.CreatedAt,
	)
	return result, err
}

func (r *PostgresSupportRepository) ListChatMessages(ctx context.Context, complaintID string, limit, offset int) ([]supportmodels.ChatMessage, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, complaint_id::text, sender_type, sender_id, content, media_url, is_read, created_at
		FROM support_chat_messages
		WHERE complaint_id = $1
		ORDER BY created_at ASC
		LIMIT $2 OFFSET $3
	`, complaintID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []supportmodels.ChatMessage
	for rows.Next() {
		var m supportmodels.ChatMessage
		if err := rows.Scan(
			&m.ID, &m.ComplaintID, &m.SenderType, &m.SenderID,
			&m.Content, &m.MediaURL, &m.IsRead, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (r *PostgresSupportRepository) CountUnread(ctx context.Context, complaintID, readerSenderType string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM support_chat_messages
		WHERE complaint_id = $1 AND is_read = false AND sender_type <> $2
	`, complaintID, readerSenderType).Scan(&count)
	return count, err
}

func (r *PostgresSupportRepository) MarkRead(ctx context.Context, complaintID, readerSenderType string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE support_chat_messages
		SET is_read = true
		WHERE complaint_id = $1 AND sender_type <> $2 AND is_read = false
	`, complaintID, readerSenderType)
	return err
}

// ─── Events (audit trail) ──────────────────────────────────────────────────────

func (r *PostgresSupportRepository) RecordEvent(ctx context.Context, complaintID, actorType, actorID, eventType string, payload map[string]any) error {
	var raw []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		raw = b
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO complaint_events (complaint_id, actor_type, actor_id, event_type, payload)
		VALUES ($1, $2, $3, $4, $5)
	`, complaintID, actorType, actorID, eventType, raw)
	return err
}

func (r *PostgresSupportRepository) ListEvents(ctx context.Context, complaintID string) ([]supportmodels.ComplaintEvent, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, complaint_id::text, actor_type, actor_id, event_type, payload, created_at
		FROM complaint_events
		WHERE complaint_id = $1
		ORDER BY created_at ASC
	`, complaintID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []supportmodels.ComplaintEvent
	for rows.Next() {
		var e supportmodels.ComplaintEvent
		var raw []byte
		if err := rows.Scan(&e.ID, &e.ComplaintID, &e.ActorType, &e.ActorID, &e.EventType, &raw, &e.CreatedAt); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &e.Payload)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// ─── Help articles ─────────────────────────────────────────────────────────────

func (r *PostgresSupportRepository) ListHelpArticles(ctx context.Context, audience string) ([]supportmodels.HelpArticle, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, audience, category, title, body, sort_order, created_at
		FROM help_articles
		WHERE is_published = true AND (audience = 'all' OR audience = $1)
		ORDER BY sort_order ASC, created_at ASC
	`, audience)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []supportmodels.HelpArticle
	for rows.Next() {
		var a supportmodels.HelpArticle
		if err := rows.Scan(&a.ID, &a.Audience, &a.Category, &a.Title, &a.Body, &a.SortOrder, &a.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// ─── Scan helpers ─────────────────────────────────────────────────────────────

type scannable interface {
	Scan(dest ...interface{}) error
}

func scanComplaint(row scannable) (supportmodels.Complaint, error) {
	var c supportmodels.Complaint
	err := row.Scan(
		&c.ID, &c.ComplainantType, &c.ComplainantID, &c.ComplainantName, &c.ComplainantPhone,
		&c.ServiceType, &c.BookingReference, &c.Category, &c.Priority, &c.IncidentLat, &c.IncidentLng,
		&c.Subject, &c.Description, &c.Status, &c.AssignedTo, &c.ResolutionNote, &c.ResolvedAt,
		&c.CreatedAt, &c.UpdatedAt,
	)
	return c, err
}

func scanComplaintWithUnread(row scannable) (supportmodels.Complaint, error) {
	var c supportmodels.Complaint
	err := row.Scan(
		&c.ID, &c.ComplainantType, &c.ComplainantID, &c.ComplainantName, &c.ComplainantPhone,
		&c.ServiceType, &c.BookingReference, &c.Category, &c.Priority, &c.IncidentLat, &c.IncidentLng,
		&c.Subject, &c.Description, &c.Status, &c.AssignedTo, &c.ResolutionNote, &c.ResolvedAt,
		&c.CreatedAt, &c.UpdatedAt, &c.UnreadCount,
	)
	return c, err
}

func scanEvidence(row scannable) (supportmodels.Evidence, error) {
	var e supportmodels.Evidence
	err := row.Scan(
		&e.ID, &e.ComplaintID, &e.UploaderType, &e.UploaderID,
		&e.MediaAssetID, &e.MediaURL, &e.Note, &e.CreatedAt,
	)
	return e, err
}

func scanDispute(row scannable) (supportmodels.Dispute, error) {
	var d supportmodels.Dispute
	err := row.Scan(
		&d.ID, &d.ComplaintID, &d.ServiceType, &d.BookingReference,
		&d.RespondentType, &d.RespondentID, &d.RespondentName, &d.RespondentPhone,
		&d.Outcome, &d.AdjudicatorID, &d.AdjudicationNote, &d.ResolvedAt, &d.CreatedAt, &d.UpdatedAt,
	)
	return d, err
}

func collectComplaints(rows pgx.Rows) ([]supportmodels.Complaint, error) {
	var result []supportmodels.Complaint
	for rows.Next() {
		c, err := scanComplaint(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func collectComplaintsWithUnread(rows pgx.Rows) ([]supportmodels.Complaint, error) {
	var result []supportmodels.Complaint
	for rows.Next() {
		c, err := scanComplaintWithUnread(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}
