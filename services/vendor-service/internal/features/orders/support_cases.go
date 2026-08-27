package orders

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/supportcase"
)

type CommerceCaseAuthority interface {
	CreateCommerceCase(context.Context, supportcase.CreateRequest) (supportcase.Case, error)
	GetCommerceCaseByOperation(context.Context, string, string) (supportcase.Case, error)
}

type CommerceIssueInput struct {
	OperationRef   string
	Category       supportcase.Category
	Description    string
	VendorOrderRef string
	LineRefs       []string
}
type SupportCaseProjection struct {
	ID                 string               `json:"link_ref"`
	OperationRef       string               `json:"operation_ref"`
	CaseRef            string               `json:"case_ref,omitempty"`
	Category           supportcase.Category `json:"category"`
	ParentOrderRef     string               `json:"commerce_order_ref"`
	VendorOrderRef     string               `json:"vendor_order_ref"`
	VendorRef          string               `json:"vendor_ref"`
	Status             string               `json:"status"`
	Resolution         string               `json:"resolution,omitempty"`
	ResolutionVersion  int64                `json:"resolution_version"`
	FinancialActionRef string               `json:"financial_action_ref,omitempty"`
	RefundStatus       string               `json:"refund_status,omitempty"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
}

func (s *Service) SetCommerceCaseAuthority(authority CommerceCaseAuthority) {
	s.supportCases = authority
}

func (s *Service) CreateCustomerCommerceIssue(ctx context.Context, customerID, parentOrderID string, in CommerceIssueInput) (SupportCaseProjection, bool, error) {
	if err := s.limitSupportCase(ctx, "customer:"+strings.TrimSpace(customerID)); err != nil {
		return SupportCaseProjection{}, false, err
	}
	parent, err := s.store.GetParent(ctx, strings.TrimSpace(customerID), strings.TrimSpace(parentOrderID))
	if err != nil {
		return SupportCaseProjection{}, false, err
	}
	var order *VendorOrder
	for i := range parent.VendorOrders {
		if parent.VendorOrders[i].ID == strings.TrimSpace(in.VendorOrderRef) {
			order = &parent.VendorOrders[i]
			break
		}
	}
	if order == nil {
		return SupportCaseProjection{}, false, ErrOrderNotFound
	}
	return s.createCommerceIssue(ctx, parent, *order, in)
}

func (s *Service) CreateVendorCommerceIssue(ctx context.Context, actor Actor, vendorOrderID string, in CommerceIssueInput) (SupportCaseProjection, bool, error) {
	key := "vendor:" + strings.TrimSpace(actor.VendorID) + ":staff:" + strings.TrimSpace(actor.StaffID)
	if err := s.limitSupportCase(ctx, key); err != nil {
		return SupportCaseProjection{}, false, err
	}
	order, err := s.store.GetVendorOrderForVendor(ctx, actor.VendorID, strings.TrimSpace(vendorOrderID))
	if err != nil {
		return SupportCaseProjection{}, false, err
	}
	parent, err := s.store.GetParentByID(ctx, order.ParentOrderID)
	if err != nil {
		return SupportCaseProjection{}, false, err
	}
	in.VendorOrderRef = order.ID
	return s.createCommerceIssue(ctx, parent, order, in)
}

func (s *Service) limitSupportCase(ctx context.Context, key string) error {
	if s.abuseLimiter == nil || s.supportLimitMax <= 0 {
		return nil
	}
	return s.abuseLimiter.Allow(ctx, "support_case_create", key, s.supportLimitMax, s.supportLimitWindow)
}

func (s *Service) createCommerceIssue(ctx context.Context, parent ParentOrder, order VendorOrder, in CommerceIssueInput) (SupportCaseProjection, bool, error) {
	if s.supportCases == nil {
		return SupportCaseProjection{}, false, ErrDependencyMissing
	}
	in.OperationRef = strings.TrimSpace(in.OperationRef)
	in.Description = supportcase.SanitizeDescription(in.Description)
	request := supportcase.CreateRequest{SchemaVersion: supportcase.SchemaVersion, OperationRef: in.OperationRef, Category: in.Category, Priority: "normal", Subject: commerceIssueSubject(in.Category), Description: in.Description, CustomerRef: parent.CustomerID, Context: supportcase.Context{CommerceOrderRef: parent.ID, VendorOrderRef: order.ID, VendorRef: order.BusinessID, MissionRef: order.LogisticsMissionRef, LineRefs: boundedUniqueRefs(in.LineRefs)}}
	if err := s.store.EnrichSupportCaseContext(ctx, order, &request.Context); err != nil {
		return SupportCaseProjection{}, false, err
	}
	if err := s.store.ValidateSupportLineRefs(ctx, order.ID, request.Context.LineRefs); err != nil {
		return SupportCaseProjection{}, false, err
	}
	if err := request.Validate(); err != nil {
		return SupportCaseProjection{}, false, ErrOrderConflict
	}
	fingerprint, err := supportcase.Fingerprint(request)
	if err != nil {
		return SupportCaseProjection{}, false, err
	}
	link, created, err := s.store.EnsureSupportCaseLink(ctx, request, fingerprint, s.now().UTC())
	if err != nil {
		return SupportCaseProjection{}, false, err
	}
	if link.CaseRef != "" {
		return link, false, nil
	}
	remote, err := s.supportCases.CreateCommerceCase(ctx, request)
	if err != nil {
		return link, created, err
	}
	link, err = s.store.BindSupportCase(ctx, link.ID, remote, s.now().UTC())
	return link, created, err
}

func (s *Store) ValidateSupportLineRefs(ctx context.Context, vendorOrderID string, refs []string) error {
	for _, ref := range refs {
		var ok bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM commerce_vendor_order_lines WHERE id=$1 AND vendor_order_id=$2)`, ref, vendorOrderID).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return ErrOrderNotFound
		}
	}
	return nil
}

func (s *Store) EnrichSupportCaseContext(ctx context.Context, order VendorOrder, target *supportcase.Context) error {
	if target == nil {
		return ErrOrderConflict
	}
	var group string
	err := s.db.QueryRow(ctx, `SELECT group_id FROM commerce_fulfilment_units WHERE vendor_order_id=$1`, order.ID).Scan(&group)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	target.FulfilmentGroupRef = group
	rows, err := s.db.Query(ctx, `SELECT parcel_ref,id,manifest_version FROM commerce_fulfilment_parcels WHERE vendor_order_id=$1 ORDER BY sequence LIMIT 20`, order.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var parcel, id string
		var version int64
		if err = rows.Scan(&parcel, &id, &version); err != nil {
			rows.Close()
			return err
		}
		target.ParcelRefs = append(target.ParcelRefs, parcel)
		target.EvidenceRefs = append(target.EvidenceRefs, id+":manifest:v"+strconv.FormatInt(version, 10))
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	_ = s.db.QueryRow(ctx, `SELECT id FROM commerce_vendor_missions WHERE group_id=$1 ORDER BY version DESC LIMIT 1`, group).Scan(&target.VendorMissionRef)
	_ = s.db.QueryRow(ctx, `SELECT external_ref FROM commerce_platform_mission_refs WHERE group_id=$1 AND external_ref<>'' ORDER BY version DESC LIMIT 1`, group).Scan(&target.PlatformMissionRef)
	if target.MissionRef == "" {
		target.MissionRef = target.PlatformMissionRef
	}
	var etaEvidence, readinessEvidence string
	_ = s.db.QueryRow(ctx, `SELECT eta_id FROM commerce_delivery_eta_current WHERE parent_order_id=$1 ORDER BY updated_at DESC LIMIT 1`, order.ParentOrderID).Scan(&etaEvidence)
	_ = s.db.QueryRow(ctx, `SELECT timing_source_evidence FROM commerce_vendor_order_readiness WHERE vendor_order_id=$1 AND timing_source_evidence<>''`, order.ID).Scan(&readinessEvidence)
	target.EvidenceRefs = boundedUniqueRefs(append(target.EvidenceRefs, etaEvidence, readinessEvidence))
	evidenceRows, queryErr := s.db.Query(ctx, `SELECT evidence_ref,transfer_ref,return_ref FROM commerce_logistics_order_events WHERE vendor_order_id=$1 ORDER BY occurred_at DESC LIMIT 20`, order.ID)
	if queryErr == nil {
		defer evidenceRows.Close()
		for evidenceRows.Next() {
			var a, b, c string
			if scanErr := evidenceRows.Scan(&a, &b, &c); scanErr != nil {
				return scanErr
			}
			target.EvidenceRefs = boundedUniqueRefs(append(target.EvidenceRefs, a, b, c))
		}
		return evidenceRows.Err()
	}
	return queryErr
}

func (s *Service) ListCustomerSupportCases(ctx context.Context, customerID, parentOrderID string) ([]SupportCaseProjection, error) {
	if _, err := s.store.GetParent(ctx, customerID, parentOrderID); err != nil {
		return nil, err
	}
	return s.store.ListSupportCases(ctx, `customer_id=$1 AND parent_order_id=$2`, customerID, parentOrderID)
}
func (s *Service) GetCustomerSupportCase(ctx context.Context, customerID, parentOrderID, caseRef string) (SupportCaseProjection, error) {
	return s.store.GetSupportCase(ctx, `customer_id=$1 AND parent_order_id=$2 AND support_case_ref=$3`, customerID, parentOrderID, caseRef)
}
func (s *Service) ListVendorSupportCases(ctx context.Context, actor Actor, vendorOrderID string) ([]SupportCaseProjection, error) {
	if _, err := s.store.GetVendorOrderForVendor(ctx, actor.VendorID, vendorOrderID); err != nil {
		return nil, err
	}
	return s.store.ListSupportCases(ctx, `business_id=$1 AND vendor_order_id=$2`, actor.VendorID, vendorOrderID)
}

func (s *Service) ApplySupportResolution(ctx context.Context, event supportcase.ResolutionEvent) error {
	if event.SchemaVersion != supportcase.SchemaVersion || event.EventID == "" || event.CaseRef == "" || event.ResolutionVersion < 1 || !event.Category.Valid() {
		return ErrOrderConflict
	}
	raw, _ := json.Marshal(event)
	sum := sha256.Sum256(raw)
	_, err := s.store.ApplySupportResolution(ctx, event, hex.EncodeToString(sum[:]), s.now().UTC())
	if err != nil {
		return err
	}
	if event.Resolution == "REFUND_AUTHORIZED" {
		return s.executeSupportAuthorizedRefund(ctx, event.CaseRef)
	}
	return nil
}

func (s *Service) executeSupportAuthorizedRefund(ctx context.Context, caseRef string) error {
	record, err := s.store.GetRefundPolicyDecisionBySupportCase(ctx, caseRef)
	if err != nil {
		return err
	}
	terminal := record.Decision.Trigger == RefundTriggerVendorCancelled || record.Decision.Trigger == RefundTriggerVendorRejected
	result, err := s.executeCommerceRefund(ctx, record, terminal)
	status := result.Status
	if err != nil {
		status = "UNKNOWN"
	}
	_ = s.store.MarkSupportCaseRefundStatus(ctx, caseRef, status, s.now().UTC())
	return err
}

func (s *Store) GetRefundPolicyDecisionBySupportCase(ctx context.Context, caseRef string) (RefundPolicyRecord, error) {
	return scanRefundPolicyRecord(s.db.QueryRow(ctx, refundPolicySelect+` WHERE support_case_ref=$1`, strings.TrimSpace(caseRef)))
}
func (s *Store) MarkSupportCaseRefundStatus(ctx context.Context, caseRef, status string, now time.Time) error {
	_, err := s.db.Exec(ctx, `UPDATE commerce_support_case_links SET refund_status=$2,updated_at=$3 WHERE support_case_ref=$1`, caseRef, status, now)
	return err
}

func (s *Store) LinkRefundSupportCase(ctx context.Context, order VendorOrder, record RefundPolicyRecord, remote RefundSupportResult, now time.Time) error {
	request := supportcase.CreateRequest{SchemaVersion: supportcase.SchemaVersion, OperationRef: "commerce-refund-support-" + record.ID, Category: supportcase.RefundProblem, Priority: "high", Subject: "Commerce refund review required", Description: record.Decision.ReasonCode, CustomerRef: order.CustomerID, Context: supportcase.Context{CommerceOrderRef: order.ParentOrderID, VendorOrderRef: order.ID, VendorRef: order.BusinessID, MissionRef: record.MissionRef, RefundRef: "commerce-refund-support-" + record.ID}}
	fingerprint, err := supportcase.Fingerprint(request)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO commerce_support_case_links(id,operation_ref,request_fingerprint,support_case_ref,category,parent_order_id,vendor_order_id,customer_id,business_id,support_status,deterministic_policy_ref,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12) ON CONFLICT(customer_id,operation_ref) DO UPDATE SET support_case_ref=EXCLUDED.support_case_ref,support_status=EXCLUDED.support_status,updated_at=EXCLUDED.updated_at WHERE commerce_support_case_links.request_fingerprint=EXCLUDED.request_fingerprint AND (commerce_support_case_links.support_case_ref IS NULL OR commerce_support_case_links.support_case_ref=EXCLUDED.support_case_ref)`, "support-link-"+uuid.NewString(), request.OperationRef, fingerprint, remote.CaseID, request.Category, order.ParentOrderID, order.ID, order.CustomerID, order.BusinessID, remote.Status, record.ID, now)
	return err
}

func (s *Store) EnsureSupportCaseLink(ctx context.Context, request supportcase.CreateRequest, fingerprint string, now time.Time) (SupportCaseProjection, bool, error) {
	id := "support-link-" + uuid.NewString()
	tag, err := s.db.Exec(ctx, `INSERT INTO commerce_support_case_links(id,operation_ref,request_fingerprint,category,parent_order_id,vendor_order_id,customer_id,business_id,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$9) ON CONFLICT(customer_id,operation_ref) DO NOTHING`, id, request.OperationRef, fingerprint, request.Category, request.Context.CommerceOrderRef, request.Context.VendorOrderRef, request.CustomerRef, request.Context.VendorRef, now)
	if err != nil {
		return SupportCaseProjection{}, false, err
	}
	value, err := s.GetSupportCase(ctx, `customer_id=$1 AND operation_ref=$2`, request.CustomerRef, request.OperationRef)
	if err != nil {
		return value, false, err
	}
	if value.Category != request.Category {
		return value, false, ErrOrderConflict
	}
	var stored string
	err = s.db.QueryRow(ctx, `SELECT request_fingerprint FROM commerce_support_case_links WHERE id=$1`, value.ID).Scan(&stored)
	if err != nil {
		return value, false, err
	}
	if stored != fingerprint {
		return value, false, ErrOrderConflict
	}
	return value, tag.RowsAffected() == 1, nil
}
func (s *Store) BindSupportCase(ctx context.Context, id string, remote supportcase.Case, now time.Time) (SupportCaseProjection, error) {
	_, err := s.db.Exec(ctx, `UPDATE commerce_support_case_links SET support_case_ref=$2,support_status=$3,resolution=$4,resolution_version=$5,financial_action_ref=$6,updated_at=$7 WHERE id=$1 AND (support_case_ref IS NULL OR support_case_ref=$2)`, id, remote.CaseRef, remote.Status, remote.Resolution, remote.ResolutionVersion, remote.FinancialActionRef, now)
	if err != nil {
		return SupportCaseProjection{}, err
	}
	return s.GetSupportCase(ctx, `id=$1`, id)
}

const supportCaseProjectionColumns = `id,operation_ref,coalesce(support_case_ref,''),category,parent_order_id,vendor_order_id,business_id,support_status,resolution,resolution_version,financial_action_ref,refund_status,created_at,updated_at`

func (s *Store) GetSupportCase(ctx context.Context, where string, args ...any) (SupportCaseProjection, error) {
	var v SupportCaseProjection
	err := scanSupportCase(s.db.QueryRow(ctx, `SELECT `+supportCaseProjectionColumns+` FROM commerce_support_case_links WHERE `+where, args...), &v)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrOrderNotFound
	}
	return v, err
}
func (s *Store) ListSupportCases(ctx context.Context, where string, args ...any) ([]SupportCaseProjection, error) {
	rows, err := s.db.Query(ctx, `SELECT `+supportCaseProjectionColumns+` FROM commerce_support_case_links WHERE `+where+` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SupportCaseProjection{}
	for rows.Next() {
		var v SupportCaseProjection
		if err = scanSupportCase(rows, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func scanSupportCase(row pgx.Row, v *SupportCaseProjection) error {
	return row.Scan(&v.ID, &v.OperationRef, &v.CaseRef, &v.Category, &v.ParentOrderRef, &v.VendorOrderRef, &v.VendorRef, &v.Status, &v.Resolution, &v.ResolutionVersion, &v.FinancialActionRef, &v.RefundStatus, &v.CreatedAt, &v.UpdatedAt)
}

func (s *Store) ApplySupportResolution(ctx context.Context, event supportcase.ResolutionEvent, payloadHash string, now time.Time) (bool, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `INSERT INTO commerce_support_resolution_inbox(event_id,support_case_ref,resolution_version,payload_hash,processed_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(event_id) DO NOTHING`, event.EventID, event.CaseRef, event.ResolutionVersion, payloadHash, now)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		var existing string
		if err = tx.QueryRow(ctx, `SELECT payload_hash FROM commerce_support_resolution_inbox WHERE event_id=$1`, event.EventID).Scan(&existing); err != nil {
			return false, err
		}
		if existing != payloadHash {
			return false, ErrOrderConflict
		}
		return false, tx.Commit(ctx)
	}
	var customer, business, parent, vendorOrder string
	err = tx.QueryRow(ctx, `UPDATE commerce_support_case_links SET support_status='resolved',resolution=$2,resolution_version=$3,financial_action_ref=$4,source_event_id=$5,updated_at=$6 WHERE support_case_ref=$1 AND resolution_version<$3 RETURNING customer_id,business_id,parent_order_id,vendor_order_id`, event.CaseRef, event.Resolution, event.ResolutionVersion, event.FinancialActionRef, event.EventID, now).Scan(&customer, &business, &parent, &vendorOrder)
	if errors.Is(err, pgx.ErrNoRows) {
		var current int64
		err = tx.QueryRow(ctx, `SELECT resolution_version FROM commerce_support_case_links WHERE support_case_ref=$1`, event.CaseRef).Scan(&current)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrOrderNotFound
		}
		if err != nil {
			return false, err
		}
		return false, tx.Commit(ctx)
	}
	if err != nil {
		return false, err
	}
	if s.outbox != nil {
		err = s.outbox.Publish(ctx, tx, "commerce.support_case.resolved", "SupportCase", event.CaseRef, event.ResolutionVersion, business, event.EventID, event.EventID, map[string]any{"case_ref": event.CaseRef, "category": event.Category, "resolution": event.Resolution, "parent_order_ref": parent, "vendor_order_ref": vendorOrder, "business_ref": business, "customer_ref": customer, "financial_action_ref": event.FinancialActionRef, "source_version": event.ResolutionVersion})
		if err != nil {
			return false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func commerceIssueSubject(c supportcase.Category) string {
	return strings.ReplaceAll(strings.Title(strings.ToLower(strings.ReplaceAll(string(c), "_", " "))), " Or ", " or ")
}
func boundedUniqueRefs(refs []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, r := range refs {
		r = strings.TrimSpace(r)
		if r != "" && !seen[r] && len(r) <= 300 && len(out) < 20 {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}
