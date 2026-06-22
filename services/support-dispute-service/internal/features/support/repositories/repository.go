package supportrepositories

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

type SupportRepository interface {
	CreateComplaint(ctx context.Context, input CreateComplaintInput) (supportmodels.Complaint, error)
	GetComplaintByID(ctx context.Context, id string) (supportmodels.Complaint, error)
	ListComplaintsByComplainant(ctx context.Context, complainantType supportmodels.ComplainantType, complainantID string, limit, offset int) ([]supportmodels.Complaint, error)
	ListComplaintsByServiceType(ctx context.Context, serviceType supportmodels.ServiceType, limit, offset int) ([]supportmodels.Complaint, error)
	UpdateComplaintStatus(ctx context.Context, id string, status supportmodels.ComplaintStatus, note *string) (supportmodels.Complaint, error)

	AddEvidence(ctx context.Context, input AddEvidenceInput) (supportmodels.Evidence, error)
	ListEvidence(ctx context.Context, complaintID string) ([]supportmodels.Evidence, error)

	CreateDispute(ctx context.Context, input CreateDisputeInput) (supportmodels.Dispute, error)
	GetDisputeByComplaintID(ctx context.Context, complaintID string) (supportmodels.Dispute, error)
	ResolveDispute(ctx context.Context, id string, outcome supportmodels.DisputeOutcome, note string, adjudicatorID string) (supportmodels.Dispute, error)

	FindActiveSupportChat(ctx context.Context, complainantType supportmodels.ComplainantType, complainantID string) (supportmodels.Complaint, error)
	CreateChatMessage(ctx context.Context, msg supportmodels.ChatMessage) (supportmodels.ChatMessage, error)
	ListChatMessages(ctx context.Context, complaintID string, limit, offset int) ([]supportmodels.ChatMessage, error)
}

type CreateComplaintInput struct {
	ComplainantType  supportmodels.ComplainantType
	ComplainantID    string
	ServiceType      supportmodels.ServiceType
	BookingReference *string
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
}

// PostgresSupportRepository is the pgxpool-backed implementation.
type PostgresSupportRepository struct {
	db *pgxpool.Pool
}

func NewPostgresSupportRepository(db *pgxpool.Pool) *PostgresSupportRepository {
	return &PostgresSupportRepository{db: db}
}

// ─── Complaints ────────────────────────────────────────────────────────────────

func (r *PostgresSupportRepository) CreateComplaint(ctx context.Context, input CreateComplaintInput) (supportmodels.Complaint, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO complaints
		  (complainant_type, complainant_id, service_type, booking_reference, subject, description)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, complainant_type, complainant_id, service_type,
		          booking_reference, subject, description, status,
		          assigned_to, resolution_note, resolved_at, created_at, updated_at
	`, input.ComplainantType, input.ComplainantID, input.ServiceType,
		input.BookingReference, input.Subject, input.Description)
	return scanComplaint(row)
}

func (r *PostgresSupportRepository) GetComplaintByID(ctx context.Context, id string) (supportmodels.Complaint, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, complainant_type, complainant_id, service_type,
		       booking_reference, subject, description, status,
		       assigned_to, resolution_note, resolved_at, created_at, updated_at
		FROM complaints WHERE id = $1
	`, id)
	c, err := scanComplaint(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return supportmodels.Complaint{}, apperrors.NotFound("Complaint not found.", err)
	}
	return c, err
}

func (r *PostgresSupportRepository) ListComplaintsByComplainant(ctx context.Context, complainantType supportmodels.ComplainantType, complainantID string, limit, offset int) ([]supportmodels.Complaint, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, complainant_type, complainant_id, service_type,
		       booking_reference, subject, description, status,
		       assigned_to, resolution_note, resolved_at, created_at, updated_at
		FROM complaints
		WHERE complainant_type = $1 AND complainant_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, complainantType, complainantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectComplaints(rows)
}

func (r *PostgresSupportRepository) ListComplaintsByServiceType(ctx context.Context, serviceType supportmodels.ServiceType, limit, offset int) ([]supportmodels.Complaint, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, complainant_type, complainant_id, service_type,
		       booking_reference, subject, description, status,
		       assigned_to, resolution_note, resolved_at, created_at, updated_at
		FROM complaints
		WHERE service_type = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, serviceType, limit, offset)
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
		RETURNING id::text, complainant_type, complainant_id, service_type,
		          booking_reference, subject, description, status,
		          assigned_to, resolution_note, resolved_at, created_at, updated_at
	`, id, status, note)
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
		  (complaint_id, service_type, booking_reference, respondent_type, respondent_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, complaint_id::text, service_type, booking_reference,
		          respondent_type, respondent_id, outcome, adjudicator_id,
		          adjudication_note, resolved_at, created_at, updated_at
	`, input.ComplaintID, input.ServiceType, input.BookingReference,
		input.RespondentType, input.RespondentID)
	return scanDispute(row)
}

func (r *PostgresSupportRepository) GetDisputeByComplaintID(ctx context.Context, complaintID string) (supportmodels.Dispute, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, complaint_id::text, service_type, booking_reference,
		       respondent_type, respondent_id, outcome, adjudicator_id,
		       adjudication_note, resolved_at, created_at, updated_at
		FROM disputes WHERE complaint_id = $1
	`, complaintID)
	d, err := scanDispute(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return supportmodels.Dispute{}, apperrors.NotFound("Dispute not found.", err)
	}
	return d, err
}

func (r *PostgresSupportRepository) ResolveDispute(ctx context.Context, id string, outcome supportmodels.DisputeOutcome, note string, adjudicatorID string) (supportmodels.Dispute, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE disputes
		SET outcome = $2, adjudication_note = $3, adjudicator_id = $4,
		    resolved_at = now(), updated_at = now()
		WHERE id = $1
		RETURNING id::text, complaint_id::text, service_type, booking_reference,
		          respondent_type, respondent_id, outcome, adjudicator_id,
		          adjudication_note, resolved_at, created_at, updated_at
	`, id, outcome, note, adjudicatorID)
	d, err := scanDispute(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return supportmodels.Dispute{}, apperrors.NotFound("Dispute not found.", err)
	}
	return d, err
}

// ─── Chat messages ────────────────────────────────────────────────────────────

func (r *PostgresSupportRepository) FindActiveSupportChat(ctx context.Context, complainantType supportmodels.ComplainantType, complainantID string) (supportmodels.Complaint, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, complainant_type, complainant_id, service_type,
		       booking_reference, subject, description, status,
		       assigned_to, resolution_note, resolved_at, created_at, updated_at
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

// ─── Scan helpers ─────────────────────────────────────────────────────────────

type scannable interface {
	Scan(dest ...interface{}) error
}

func scanComplaint(row scannable) (supportmodels.Complaint, error) {
	var c supportmodels.Complaint
	err := row.Scan(
		&c.ID, &c.ComplainantType, &c.ComplainantID, &c.ServiceType,
		&c.BookingReference, &c.Subject, &c.Description, &c.Status,
		&c.AssignedTo, &c.ResolutionNote, &c.ResolvedAt, &c.CreatedAt, &c.UpdatedAt,
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
		&d.RespondentType, &d.RespondentID, &d.Outcome, &d.AdjudicatorID,
		&d.AdjudicationNote, &d.ResolvedAt, &d.CreatedAt, &d.UpdatedAt,
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
