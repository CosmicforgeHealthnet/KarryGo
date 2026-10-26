// Package hqops orchestrates HQ department intents that are not owner
// resource commands: marketing campaign submission (Sections 17/25), group
// finance report passthrough (26), the cross-service privacy disposition
// register (30) and controlled encrypted exports (30). It stores orchestration
// evidence only; notification-service, payment-wallet-service and each data
// owner remain the authorities.
package hqops

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/clients"
	"cosmicforge/logistics/shared/go/admincampaign"
	"cosmicforge/logistics/shared/go/financereport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDisabled = errors.New("HQ capability disabled")
	ErrInvalid  = errors.New("invalid HQ request")
	ErrApproval = errors.New("approval missing, foreign, expired or not bound to this intent")
	ErrConflict = errors.New("identity conflict")
	ErrNotFound = errors.New("not found")
	ErrDenied   = errors.New("denied")
	ErrExpired  = errors.New("expired")
)

// ApprovalWindow bounds pending and approved-unused HQ approvals.
const ApprovalWindow = 12 * time.Hour

type Scope struct{ Market, Zone, Organization, Tenant string }

type Approval struct {
	ID           string     `json:"approval_id"`
	Kind         string     `json:"kind"`
	SubjectRef   string     `json:"subject_ref"`
	IntentDigest string     `json:"intent_digest"`
	Status       string     `json:"status"`
	ExpiresAt    time.Time  `json:"expires_at"`
	DecidedAt    *time.Time `json:"decided_at,omitempty"`
}

type Service struct {
	DB             *pgxpool.Pool
	Campaigns      *clients.Campaigns
	Finance        *clients.Finance
	CampaignWrites bool
	CampaignReads  bool
	FinanceReads   bool
	PrivacyWrites  bool
	ExportKey      []byte
	ExportKeyID    string
	Now            func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func digest(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:])
}

// ProposeApproval records an intent awaiting a second, different staff member.
// Re-proposing the same id with the same content is idempotent.
func (s *Service) ProposeApproval(ctx context.Context, staff, purpose string, sc Scope, id, kind, subject, intentDigest string) (Approval, error) {
	if _, err := uuid.Parse(id); err != nil || (kind != "CAMPAIGN" && kind != "EXPORT") || subject == "" || len(intentDigest) != 64 {
		return Approval{}, ErrInvalid
	}
	now := s.now()
	_, err := s.DB.Exec(ctx, `INSERT INTO admin_hq_approvals(id,kind,subject_ref,intent_digest,requester_ref,purpose_code,market,zone,organization_ref,tenant_ref,status,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'PENDING',$11)`, id, kind, subject, intentDigest, staff, purpose, sc.Market, sc.Zone, sc.Organization, sc.Tenant, now.Add(ApprovalWindow))
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		a, e := s.approval(ctx, id)
		if e != nil {
			return Approval{}, e
		}
		if a.Kind != kind || a.SubjectRef != subject || a.IntentDigest != intentDigest {
			return Approval{}, ErrConflict
		}
		return a, nil
	}
	if err != nil {
		return Approval{}, err
	}
	return s.approval(ctx, id)
}

func (s *Service) approval(ctx context.Context, id string) (Approval, error) {
	var a Approval
	err := s.DB.QueryRow(ctx, `SELECT id::text,kind,subject_ref,intent_digest,status,expires_at,decided_at FROM admin_hq_approvals WHERE id::text=$1`, id).Scan(&a.ID, &a.Kind, &a.SubjectRef, &a.IntentDigest, &a.Status, &a.ExpiresAt, &a.DecidedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

func (s *Service) ReadApproval(ctx context.Context, id string, sc Scope) (Approval, error) {
	var ok bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_hq_approvals WHERE id::text=$1 AND market=$2 AND zone=$3 AND organization_ref=$4 AND tenant_ref=$5)`, id, sc.Market, sc.Zone, sc.Organization, sc.Tenant).Scan(&ok); err != nil {
		return Approval{}, err
	}
	if !ok {
		return Approval{}, ErrNotFound
	}
	return s.approval(ctx, id)
}

// Decide applies the second person's decision. The database guard and the
// requester<>approver CHECK make self-approval impossible even under races.
func (s *Service) Decide(ctx context.Context, id, approver string, sc Scope, approve bool) (Approval, error) {
	status := "REJECTED"
	if approve {
		status = "APPROVED"
	}
	tag, err := s.DB.Exec(ctx, `UPDATE admin_hq_approvals SET status=$2,approver_ref=$3,decided_at=$4,updated_at=$4 WHERE id::text=$1 AND status='PENDING' AND requester_ref<>$3 AND expires_at>$4 AND market=$5 AND zone=$6 AND organization_ref=$7 AND tenant_ref=$8`, id, status, approver, s.now(), sc.Market, sc.Zone, sc.Organization, sc.Tenant)
	if err != nil {
		return Approval{}, err
	}
	if tag.RowsAffected() != 1 {
		return Approval{}, ErrApproval
	}
	return s.approval(ctx, id)
}

// consume binds an APPROVED approval to exactly one consumer inside tx.
func consume(ctx context.Context, tx pgx.Tx, id, kind, subject, intentDigest, requester, consumer string, sc Scope, now time.Time) (string, time.Time, error) {
	var approver string
	var decided time.Time
	err := tx.QueryRow(ctx, `UPDATE admin_hq_approvals SET status='CONSUMED',consumed_by=$7,updated_at=$8 WHERE id::text=$1 AND kind=$2 AND subject_ref=$3 AND intent_digest=$4 AND requester_ref=$5 AND status='APPROVED' AND expires_at>$8 AND market=$9 AND zone=$10 AND organization_ref=$11 AND tenant_ref=$12 AND approver_ref<>$6 RETURNING approver_ref,decided_at`, id, kind, subject, intentDigest, requester, requester, consumer, now, sc.Market, sc.Zone, sc.Organization, sc.Tenant).Scan(&approver, &decided)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, ErrApproval
	}
	return approver, decided, err
}

// CampaignSubject is the approval subject for one campaign revision.
func CampaignSubject(campaignID string, revision int64) string {
	return "campaign:" + campaignID + ":" + strconv.FormatInt(revision, 10)
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

type CampaignOperation struct {
	OperationID   string                `json:"operation_id"`
	CampaignID    string                `json:"campaign_id"`
	Revision      int64                 `json:"revision"`
	IntentDigest  string                `json:"intent_digest"`
	Status        string                `json:"status"`
	RejectionCode string                `json:"rejection_code,omitempty"`
	OwnerStatus   *admincampaign.Status `json:"owner_status"`
	AsOf          time.Time             `json:"as_of"`
}

// SubmitCampaign: approval consumed and operation recorded atomically, then
// exactly one owner submission. The Bridge, not HQ, attaches requested_by and
// the approval identity, so a forged approval object cannot reach the owner.
// A lost response becomes UNKNOWN_OUTCOME and is resolved by read-back only.
func (s *Service) SubmitCampaign(ctx context.Context, staff string, sc Scope, actorMarkets []string, in admincampaign.Intent, approvalID string) (CampaignOperation, error) {
	if s.Campaigns == nil || !s.CampaignWrites || !s.CampaignReads {
		return CampaignOperation{}, ErrDisabled
	}
	in.RequestedBy = staff
	in.Approvals = []admincampaign.Approval{{ApprovalID: approvalID, ApproverRef: "pending", ApprovedAt: s.now()}}
	if in.Validate(s.now()) != nil {
		return CampaignOperation{}, ErrInvalid
	}
	for _, m := range in.Markets {
		allowed := false
		for _, a := range actorMarkets {
			allowed = allowed || a == m
		}
		if !allowed {
			return CampaignOperation{}, ErrDenied
		}
	}
	digest := in.Digest()
	now := s.now()
	opID := uuid.NewString()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return CampaignOperation{}, err
	}
	defer tx.Rollback(ctx)
	var existing CampaignOperation
	err = tx.QueryRow(ctx, `SELECT id::text,intent_digest FROM admin_campaign_operations WHERE idempotency_key=$1 OR (campaign_id::text=$2 AND revision=$3)`, in.IdempotencyKey, in.CampaignID, in.Revision).Scan(&existing.OperationID, &existing.IntentDigest)
	if err == nil {
		if existing.IntentDigest != digest {
			return CampaignOperation{}, ErrConflict
		}
		tx.Rollback(ctx)
		return s.CampaignReadback(ctx, existing.OperationID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CampaignOperation{}, err
	}
	approver, decided, err := consume(ctx, tx, approvalID, "CAMPAIGN", CampaignSubject(in.CampaignID, in.Revision), digest, staff, opID, sc, now)
	if err != nil {
		return CampaignOperation{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO admin_campaign_operations(id,campaign_id,revision,idempotency_key,intent_digest,approval_id,staff_ref,status) VALUES($1,$2,$3,$4,$5,$6,$7,'PENDING')`, opID, in.CampaignID, in.Revision, in.IdempotencyKey, digest, approvalID, staff); err != nil {
		return CampaignOperation{}, ErrConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return CampaignOperation{}, err
	}
	in.Approvals = []admincampaign.Approval{{ApprovalID: approvalID, ApproverRef: approver, ApprovedAt: decided}}
	status, err := s.Campaigns.Submit(ctx, in)
	return s.settleCampaign(ctx, opID, digest, status, err)
}

func (s *Service) settleCampaign(ctx context.Context, opID, digest string, status admincampaign.Status, err error) (CampaignOperation, error) {
	switch {
	case err == nil && status.IntentDigest == digest:
		raw, _ := json.Marshal(status)
		_, e := s.DB.Exec(ctx, `UPDATE admin_campaign_operations SET status='CONFIRMED',owner_status=$2,updated_at=now() WHERE id::text=$1 AND status IN ('PENDING','UNKNOWN_OUTCOME','CONFIRMED')`, opID, raw)
		if e != nil {
			return CampaignOperation{}, e
		}
	case errors.Is(err, clients.ErrOwnerRefused), errors.Is(err, clients.ErrOwnerConflict):
		code := "OWNER_REFUSED"
		if errors.Is(err, clients.ErrOwnerConflict) {
			code = "OWNER_CONFLICT"
		}
		if _, e := s.DB.Exec(ctx, `UPDATE admin_campaign_operations SET status='REJECTED',rejection_code=$2,updated_at=now() WHERE id::text=$1 AND status='PENDING'`, opID, code); e != nil {
			return CampaignOperation{}, e
		}
	default:
		// Transport loss, timeout, malformed or mismatched proof: never assume.
		if _, e := s.DB.Exec(ctx, `UPDATE admin_campaign_operations SET status='UNKNOWN_OUTCOME',updated_at=now() WHERE id::text=$1 AND status='PENDING'`, opID); e != nil {
			return CampaignOperation{}, e
		}
	}
	return s.load(ctx, opID)
}

func (s *Service) load(ctx context.Context, opID string) (CampaignOperation, error) {
	var op CampaignOperation
	var raw []byte
	err := s.DB.QueryRow(ctx, `SELECT id::text,campaign_id::text,revision,intent_digest,status,rejection_code,owner_status FROM admin_campaign_operations WHERE id::text=$1`, opID).Scan(&op.OperationID, &op.CampaignID, &op.Revision, &op.IntentDigest, &op.Status, &op.RejectionCode, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return op, ErrNotFound
	}
	if err != nil {
		return op, err
	}
	if len(raw) > 0 {
		var st admincampaign.Status
		if json.Unmarshal(raw, &st) == nil {
			op.OwnerStatus = &st
		}
	}
	op.AsOf = s.now()
	return op, nil
}

// CampaignReadback reads the owner's truth by original identity. It never
// resubmits; an owner 404 for an UNKNOWN_OUTCOME stays UNKNOWN_OUTCOME.
func (s *Service) CampaignReadback(ctx context.Context, opID string) (CampaignOperation, error) {
	if s.Campaigns == nil || !s.CampaignReads {
		return CampaignOperation{}, ErrDisabled
	}
	op, err := s.load(ctx, opID)
	if err != nil || op.Status == "REJECTED" {
		return op, err
	}
	status, err := s.Campaigns.Status(ctx, op.CampaignID, op.Revision)
	if errors.Is(err, clients.ErrNotFound) {
		return op, nil
	}
	if err != nil {
		op.AsOf = s.now()
		return op, nil
	}
	if status.IntentDigest != op.IntentDigest {
		return op, ErrConflict
	}
	return s.settleCampaign(ctx, opID, op.IntentDigest, status, nil)
}

func (s *Service) CancelCampaign(ctx context.Context, staff, opID, cancellationKey, reason string) (CampaignOperation, error) {
	if s.Campaigns == nil || !s.CampaignWrites {
		return CampaignOperation{}, ErrDisabled
	}
	op, err := s.load(ctx, opID)
	if err != nil {
		return op, err
	}
	if op.Status != "CONFIRMED" {
		return op, ErrConflict
	}
	status, err := s.Campaigns.Cancel(ctx, admincampaign.Cancellation{ContractVersion: admincampaign.Contract, CampaignID: op.CampaignID, Revision: op.Revision, CancellationKey: cancellationKey, RequestedBy: staff, ReasonCode: reason})
	if err != nil {
		if errors.Is(err, clients.ErrOwnerRefused) || errors.Is(err, clients.ErrOwnerConflict) {
			return op, ErrConflict
		}
		return op, clients.ErrUnavailable
	}
	return s.settleCampaign(ctx, opID, op.IntentDigest, status, nil)
}

// FinanceReport is a validated read-only passthrough of Wallet's aggregates.
func (s *Service) FinanceReport(ctx context.Context, start, end time.Time, tz string) (financereport.Report, error) {
	if s.Finance == nil || !s.FinanceReads {
		return financereport.Report{}, ErrDisabled
	}
	if _, err := time.LoadLocation(tz); err != nil || tz == "" || tz == "Local" || !end.After(start) || end.Sub(start) > 93*24*time.Hour {
		return financereport.Report{}, ErrInvalid
	}
	return s.Finance.Report(ctx, start, end, tz)
}

// Privacy holders (Section 30). Only customer-service has an approved owner
// privacy command contract today; every other holder needs an approved
// retention/erasure policy before any disposition other than POLICY_REQUIRED.
var privacyHolders = map[string]string{
	"customer-service":                 "PENDING_OWNER_ACTION",
	"driver-taxi-service":              "POLICY_REQUIRED",
	"driver-dispatch-delivery-service": "POLICY_REQUIRED",
	"driver-hauling-service":           "POLICY_REQUIRED",
	"vendor-service":                   "POLICY_REQUIRED",
	"payment-wallet-service":           "POLICY_REQUIRED",
	"media-file-service":               "POLICY_REQUIRED",
	"notification-service":             "POLICY_REQUIRED",
	"support-dispute-service":          "POLICY_REQUIRED",
	"referral-service":                 "POLICY_REQUIRED",
	"fleet-partner-service":            "POLICY_REQUIRED",
	// Aggregates only, no personal identifiers by design.
	"analytics-service": "NOT_HELD_BY_OWNER",
	// Bridge projections carry no subject identifiers (minimized by adapter).
	"admin-backoffice-service": "NOT_HELD_BY_OWNER",
}

type Disposition struct {
	Owner       string    `json:"owner"`
	Disposition string    `json:"disposition"`
	EvidenceRef string    `json:"evidence_ref"`
	RecordedAt  time.Time `json:"recorded_at"`
}
type PrivacyRequest struct {
	RequestID    string        `json:"request_id"`
	SubjectKind  string        `json:"subject_kind"`
	RequestType  string        `json:"request_type"`
	ReceivedOn   string        `json:"received_on"`
	LegalHold    bool          `json:"legal_hold"`
	Dispositions []Disposition `json:"dispositions"`
	Complete     bool          `json:"complete"`
}

// RegisterPrivacyRequest stores only a digest of the subject reference and
// opens a disposition per holder. Nothing is deleted here; owners act.
func (s *Service) RegisterPrivacyRequest(ctx context.Context, staff, id, subjectKind, subjectRef, requestType, receivedOn string, legalHold bool) (PrivacyRequest, error) {
	if !s.PrivacyWrites {
		return PrivacyRequest{}, ErrDisabled
	}
	if _, err := uuid.Parse(id); err != nil || subjectRef == "" || len(subjectRef) > 128 {
		return PrivacyRequest{}, ErrInvalid
	}
	if _, err := time.Parse("2006-01-02", receivedOn); err != nil {
		return PrivacyRequest{}, ErrInvalid
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return PrivacyRequest{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO admin_privacy_requests(id,subject_kind,subject_digest,request_type,received_on,registered_by,legal_hold) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, subjectKind, digest("privacy-subject-v1", subjectKind, subjectRef), requestType, receivedOn, staff, legalHold)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		tx.Rollback(ctx)
		return s.PrivacyRegister(ctx, id)
	}
	if err != nil {
		return PrivacyRequest{}, ErrInvalid
	}
	for owner, initial := range privacyHolders {
		d := initial
		if legalHold && d != "NOT_HELD_BY_OWNER" {
			d = "LEGAL_HOLD"
		}
		if _, err = tx.Exec(ctx, `INSERT INTO admin_privacy_dispositions(request_id,owner,disposition,recorded_by) VALUES($1,$2,$3,$4)`, id, owner, d, staff); err != nil {
			return PrivacyRequest{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return PrivacyRequest{}, err
	}
	return s.PrivacyRegister(ctx, id)
}

// RecordOwnerFulfilment accepts only verifiable owner evidence: a CONFIRMED
// customer-service FULFIL_PRIVACY_REQUEST operation orchestrated here.
func (s *Service) RecordOwnerFulfilment(ctx context.Context, staff, requestID, owner, operationID string) (PrivacyRequest, error) {
	if !s.PrivacyWrites {
		return PrivacyRequest{}, ErrDisabled
	}
	if owner != "customer-service" {
		return PrivacyRequest{}, ErrDenied
	}
	var hold, confirmed bool
	if err := s.DB.QueryRow(ctx, `SELECT legal_hold FROM admin_privacy_requests WHERE id::text=$1`, requestID).Scan(&hold); errors.Is(err, pgx.ErrNoRows) {
		return PrivacyRequest{}, ErrNotFound
	} else if err != nil {
		return PrivacyRequest{}, err
	}
	if hold {
		return PrivacyRequest{}, ErrDenied
	}
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_owner_command_operations WHERE id::text=$1 AND owner='customer-service' AND action='FULFIL_PRIVACY_REQUEST' AND status='CONFIRMED')`, operationID).Scan(&confirmed); err != nil {
		return PrivacyRequest{}, err
	}
	if !confirmed {
		return PrivacyRequest{}, ErrApproval
	}
	if _, err := s.DB.Exec(ctx, `INSERT INTO admin_privacy_dispositions(request_id,owner,disposition,evidence_ref,recorded_by) VALUES($1,$2,'FULFILLED_BY_OWNER',$3,$4)`, requestID, owner, "owner-operation:"+operationID, staff); err != nil {
		return PrivacyRequest{}, err
	}
	return s.PrivacyRegister(ctx, requestID)
}

func (s *Service) PrivacyRegister(ctx context.Context, id string) (PrivacyRequest, error) {
	var r PrivacyRequest
	var received time.Time
	err := s.DB.QueryRow(ctx, `SELECT id::text,subject_kind,request_type,received_on,legal_hold FROM admin_privacy_requests WHERE id::text=$1`, id).Scan(&r.RequestID, &r.SubjectKind, &r.RequestType, &received, &r.LegalHold)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	r.ReceivedOn = received.Format("2006-01-02")
	rows, err := s.DB.Query(ctx, `SELECT DISTINCT ON (owner) owner,disposition,evidence_ref,recorded_at FROM admin_privacy_dispositions WHERE request_id::text=$1 ORDER BY owner,id DESC`, id)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	r.Complete = true
	for rows.Next() {
		var d Disposition
		if err := rows.Scan(&d.Owner, &d.Disposition, &d.EvidenceRef, &d.RecordedAt); err != nil {
			return r, err
		}
		switch d.Disposition {
		case "FULFILLED_BY_OWNER", "RETAINED_LEGAL_OBLIGATION", "NOT_HELD_BY_OWNER", "PROJECTION_TOMBSTONED":
		default:
			r.Complete = false
		}
		r.Dispositions = append(r.Dispositions, d)
	}
	return r, rows.Err()
}

// ExportSubject binds an export approval to its exact parameters.
func ExportSubject(kind string, start, end time.Time, rowLimit int, sc Scope) (string, string) {
	d := digest("export-v1", kind, start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339), itoa(int64(rowLimit)), sc.Market, sc.Zone, sc.Organization, sc.Tenant)
	return "export:" + d[:32], d
}

type Export struct {
	ExportID  string    `json:"export_id"`
	RowCount  int       `json:"row_count"`
	Token     string    `json:"download_token,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	SHA256    string    `json:"plaintext_sha256"`
}

// CreateExport materializes a minimized, approved, bounded support-queue export
// encrypted at rest (AES-256-GCM). The token is returned once and stored only
// as a hash; download is single-use, requester-bound and expires.
func (s *Service) CreateExport(ctx context.Context, staff, purpose string, sc Scope, start, end time.Time, rowLimit int, approvalID string) (Export, error) {
	if len(s.ExportKey) != 32 || s.ExportKeyID == "" {
		return Export{}, ErrDisabled
	}
	if !end.After(start) || end.Sub(start) > 93*24*time.Hour || rowLimit < 1 || rowLimit > 10000 {
		return Export{}, ErrInvalid
	}
	subject, d := ExportSubject("SUPPORT_CASE_QUEUE", start, end, rowLimit, sc)
	id := uuid.NewString()
	now := s.now()
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return Export{}, err
	}
	defer tx.Rollback(ctx)
	if _, _, err = consume(ctx, tx, approvalID, "EXPORT", subject, d, staff, id, sc, now); err != nil {
		return Export{}, err
	}
	rows, err := tx.Query(ctx, `SELECT source_service,entity_ref,entity_version,payload->>'status',payload->>'priority',payload->>'service_type',payload->>'category',received_at FROM admin_projections
WHERE product='karryam' AND entity_type='support_case' AND NOT tombstone AND received_at>=$1 AND received_at<$2
 AND (market='' OR market=$3) AND (zone='' OR zone=$4) AND (organization_ref='' OR organization_ref=$5) AND (tenant_ref='' OR tenant_ref=$6)
ORDER BY received_at,entity_ref LIMIT $7`, start, end, sc.Market, sc.Zone, sc.Organization, sc.Tenant, rowLimit)
	if err != nil {
		return Export{}, err
	}
	var buf strings.Builder
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"composite_id", "source_version", "status", "priority", "service_type", "category", "received_at"})
	count := 0
	for rows.Next() {
		var src, ref string
		var version int64
		var status, priority, service, category *string
		var received time.Time
		if err := rows.Scan(&src, &ref, &version, &status, &priority, &service, &category, &received); err != nil {
			rows.Close()
			return Export{}, err
		}
		v := func(p *string) string {
			if p == nil {
				return ""
			}
			return *p
		}
		_ = w.Write([]string{"karryam:" + src + ":" + ref, itoa(version), v(status), v(priority), v(service), v(category), received.UTC().Format(time.RFC3339)})
		count++
	}
	rows.Close()
	w.Flush()
	plain := []byte(buf.String())
	block, _ := aes.NewCipher(s.ExportKey)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return Export{}, err
	}
	sealed := gcm.Seal(nil, nonce, plain, []byte(id))
	tokenRaw := make([]byte, 32)
	if _, err = rand.Read(tokenRaw); err != nil {
		return Export{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenRaw)
	sum := sha256.Sum256(plain)
	expires := now.Add(15 * time.Minute)
	if _, err = tx.Exec(ctx, `INSERT INTO admin_exports(id,kind,requested_by,approval_id,purpose_code,market,zone,organization_ref,tenant_ref,row_limit,period_start,period_end,row_count,key_id,nonce,ciphertext,plaintext_sha256,token_hash,expires_at) VALUES($1,'SUPPORT_CASE_QUEUE',$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, id, staff, approvalID, purpose, sc.Market, sc.Zone, sc.Organization, sc.Tenant, rowLimit, start, end, count, s.ExportKeyID, nonce, sealed, hex.EncodeToString(sum[:]), digest("export-token-v1", id, token), expires); err != nil {
		return Export{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Export{}, err
	}
	return Export{ExportID: id, RowCount: count, Token: token, ExpiresAt: expires, SHA256: hex.EncodeToString(sum[:])}, nil
}

// DownloadExport delivers the plaintext once to the requesting staff member.
func (s *Service) DownloadExport(ctx context.Context, staff, id, token string) ([]byte, error) {
	if len(s.ExportKey) != 32 {
		return nil, ErrDisabled
	}
	var requester, keyID, tokenHash string
	var nonce, sealed []byte
	var expires time.Time
	err := s.DB.QueryRow(ctx, `SELECT requested_by,key_id,nonce,ciphertext,token_hash,expires_at FROM admin_exports WHERE id::text=$1`, id).Scan(&requester, &keyID, &nonce, &sealed, &tokenHash, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	record := func(outcome string) error {
		_, e := s.DB.Exec(ctx, `INSERT INTO admin_export_downloads(export_id,staff_ref,outcome) VALUES($1,$2,$3)`, id, staff, outcome)
		return e
	}
	if requester != staff || subtle.ConstantTimeCompare([]byte(tokenHash), []byte(digest("export-token-v1", id, token))) != 1 || keyID != s.ExportKeyID {
		_ = record("DENIED")
		return nil, ErrDenied
	}
	if !s.now().Before(expires) {
		_ = record("EXPIRED")
		return nil, ErrExpired
	}
	block, _ := aes.NewCipher(s.ExportKey)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, sealed, []byte(id))
	if err != nil {
		return nil, ErrDenied
	}
	if err := record("DELIVERED"); err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return nil, ErrConflict // already delivered once
		}
		return nil, err
	}
	return plain, nil
}
