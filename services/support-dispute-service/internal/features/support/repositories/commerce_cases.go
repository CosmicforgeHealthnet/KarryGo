package supportrepositories

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/supportcase"
)

type CommerceCaseRepository interface {
	CreateCommerceCase(context.Context, supportcase.CreateRequest, string) (supportcase.Case, bool, error)
	GetCommerceCaseByOperation(context.Context, string, string) (supportcase.Case, error)
	GetCommerceCaseByRef(context.Context, string, string) (supportcase.Case, error)
	ResolveCommerceCase(context.Context, string, string, string, string, time.Time) (supportcase.ResolutionEvent, error)
}

const commerceCaseColumns = `id::text, source_idempotency_key, commerce_category, status::text,
	coalesce(resolution_type,''), resolution_version, coalesce(financial_action_ref,''), source_version, updated_at`

func (r *PostgresSupportRepository) CreateCommerceCase(ctx context.Context, in supportcase.CreateRequest, fingerprint string) (supportcase.Case, bool, error) {
	contextJSON, err := json.Marshal(in.Context)
	if err != nil {
		return supportcase.Case{}, false, err
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return supportcase.Case{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var value supportcase.Case
	err = scanCommerceCase(tx.QueryRow(ctx, `INSERT INTO complaints
		(complainant_type,complainant_id,service_type,booking_reference,category,priority,subject,description,
		 organization_reference,source_idempotency_key,source_schema_version,request_fingerprint,commerce_category,commerce_context)
		 VALUES ('customer',$1,'platform',$2,'other',$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 ON CONFLICT (organization_reference,source_idempotency_key) WHERE organization_reference IS NOT NULL AND source_idempotency_key IS NOT NULL
		 DO NOTHING RETURNING `+commerceCaseColumns,
		in.CustomerRef, in.Context.CommerceOrderRef, normalizedPriority(in.Priority), strings.TrimSpace(in.Subject), strings.TrimSpace(in.Description),
		in.Context.VendorRef, in.OperationRef, in.SchemaVersion, fingerprint, in.Category, contextJSON), &value)
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		var storedFingerprint string
		err = scanCommerceCase(tx.QueryRow(ctx, `SELECT `+commerceCaseColumns+` FROM complaints WHERE organization_reference=$1 AND source_idempotency_key=$2`, in.Context.VendorRef, in.OperationRef), &value)
		if err == nil {
			err = tx.QueryRow(ctx, `SELECT request_fingerprint FROM complaints WHERE id=$1`, value.CaseRef).Scan(&storedFingerprint)
		}
		if err == nil && storedFingerprint != fingerprint {
			return supportcase.Case{}, false, apperrors.Conflict("Idempotency key was already used with a different commerce case.", nil)
		}
	}
	if err != nil {
		return supportcase.Case{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return supportcase.Case{}, false, err
	}
	return value, created, nil
}

func (r *PostgresSupportRepository) GetCommerceCaseByOperation(ctx context.Context, vendorRef, operationRef string) (supportcase.Case, error) {
	var value supportcase.Case
	err := scanCommerceCase(r.db.QueryRow(ctx, `SELECT `+commerceCaseColumns+` FROM complaints WHERE organization_reference=$1 AND source_idempotency_key=$2 AND commerce_category IS NOT NULL`, vendorRef, operationRef), &value)
	if errors.Is(err, pgx.ErrNoRows) {
		return value, apperrors.NotFound("Commerce support case not found.", err)
	}
	return value, err
}

func (r *PostgresSupportRepository) GetCommerceCaseByRef(ctx context.Context, vendorRef, caseRef string) (supportcase.Case, error) {
	var value supportcase.Case
	err := scanCommerceCase(r.db.QueryRow(ctx, `SELECT `+commerceCaseColumns+` FROM complaints WHERE organization_reference=$1 AND id=$2 AND commerce_category IS NOT NULL`, vendorRef, caseRef), &value)
	if errors.Is(err, pgx.ErrNoRows) {
		return value, apperrors.NotFound("Commerce support case not found.", err)
	}
	return value, err
}

func (r *PostgresSupportRepository) ResolveCommerceCase(ctx context.Context, caseRef, resolution, financialRef, actor string, now time.Time) (supportcase.ResolutionEvent, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return supportcase.ResolutionEvent{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var event supportcase.ResolutionEvent
	var raw []byte
	err = tx.QueryRow(ctx, `UPDATE complaints SET status='resolved',resolution_type=$2,resolution_version=resolution_version+1,
		financial_action_ref=NULLIF($3,''),resolution_note=$2,resolved_at=$4,updated_at=$4,source_version=source_version+1
		WHERE id=$1 AND commerce_category IS NOT NULL
		RETURNING id::text,source_idempotency_key,commerce_category,resolution_type,resolution_version,
		coalesce(financial_action_ref,''),complainant_id,commerce_context,resolved_at`, caseRef, resolution, financialRef, now).Scan(
		&event.CaseRef, &event.OperationRef, &event.Category, &event.Resolution, &event.ResolutionVersion, &event.FinancialActionRef, &event.CustomerRef, &raw, &event.ResolvedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return event, apperrors.NotFound("Commerce support case not found.", err)
	}
	if err != nil {
		return event, err
	}
	if err = json.Unmarshal(raw, &event.Context); err != nil {
		return event, err
	}
	event.SchemaVersion = supportcase.SchemaVersion
	payload, err := json.Marshal(event)
	if err != nil {
		return event, err
	}
	err = tx.QueryRow(ctx, `WITH e AS (SELECT gen_random_uuid() id), inserted AS (
		INSERT INTO support_case_outbox(id,event_type,aggregate_ref,aggregate_version,payload)
		SELECT e.id,'commerce.support_case.resolved',$1,$2,$3::jsonb || jsonb_build_object('event_id',e.id::text) FROM e RETURNING id::text)
		SELECT id FROM inserted`, caseRef, event.ResolutionVersion, payload).Scan(&event.EventID)
	if err != nil {
		return event, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO complaint_events(complaint_id,actor_type,actor_id,event_type,payload) VALUES($1,'admin',$2,'commerce_case_resolved',$3)`, caseRef, actor, payload)
	if err != nil {
		return event, err
	}
	if err = tx.Commit(ctx); err != nil {
		return event, err
	}
	return event, nil
}

func scanCommerceCase(row pgx.Row, value *supportcase.Case) error {
	return row.Scan(&value.CaseRef, &value.OperationRef, &value.Category, &value.Status, &value.Resolution, &value.ResolutionVersion, &value.FinancialActionRef, &value.Version, &value.UpdatedAt)
}

func normalizedPriority(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "high" || value == "emergency" {
		return value
	}
	return "normal"
}

var _ CommerceCaseRepository = (*PostgresSupportRepository)(nil)
