package orders

// Section 21 Prompt 1 foundations.  This file deliberately keeps scheduled
// commerce, collaborative groups and RFQs in the Orders bounded context while
// delegating money, checkout and production capacity to their existing owners.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	vendorevents "cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkoutquote"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/services/vendor-service/internal/features/taxonomy"
)

var (
	ErrSection21Disabled       = errors.New("section21: disabled")
	ErrSection21NotFound       = errors.New("section21: not found")
	ErrSection21Conflict       = errors.New("section21: conflict")
	ErrSection21Forbidden      = errors.New("section21: forbidden")
	ErrSection21Expired        = errors.New("section21: expired")
	ErrSection21Capacity       = errors.New("section21: capacity unavailable")
	ErrSection21InvalidState   = errors.New("section21: invalid state transition")
	ErrSection21CheckoutNeeded = errors.New("section21: checkout quote required")
	ErrRestrictedGoods         = errors.New("restricted goods are not eligible for Vendor Commerce")
)

type Section21Options struct {
	DB         *pgxpool.Pool
	Orders     *Service
	Checkout   *checkoutquote.Service
	Production *production.Service
	Payments   PaymentAuthority
	Scopes     ScopeResolver
	Events     *vendorevents.OutboxRepository
	Enabled    bool
	Now        func() time.Time
}

type Section21Service struct {
	db         *pgxpool.Pool
	orders     *Service
	checkout   *checkoutquote.Service
	production *production.Service
	payments   PaymentAuthority
	scopes     ScopeResolver
	events     *vendorevents.OutboxRepository
	enabled    bool
	now        func() time.Time
}

func NewSection21Service(o Section21Options) *Section21Service {
	now := o.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Section21Service{db: o.DB, orders: o.Orders, checkout: o.Checkout, production: o.Production, payments: o.Payments, scopes: o.Scopes, events: o.Events, enabled: o.Enabled, now: now}
}

func (s *Section21Service) appendEvent(ctx context.Context, tx pgx.Tx, eventType, aggregateType, aggregateID, vendorID string, version int64, payload any) error {
	if s.events == nil {
		return nil
	}
	envelope, err := vendorevents.NewEnvelope(eventType, 1, aggregateType, aggregateID, version, vendorID, "", "", payload, false)
	if err != nil {
		return err
	}
	return s.events.Append(ctx, tx, envelope)
}

func (s *Section21Service) AuthorizeStaff(ctx context.Context, actor Actor, branchID string) error {
	if err := s.available(); err != nil {
		return err
	}
	if s.scopes == nil || strings.TrimSpace(actor.StaffID) == "" || strings.TrimSpace(actor.VendorID) == "" {
		return ErrSection21Forbidden
	}
	scope, err := s.scopes.ResolveScope(ctx, actor.StaffID, PermOrdersManage)
	if err != nil {
		return err
	}
	if !scope.Covers(strings.TrimSpace(branchID)) {
		return ErrSection21Forbidden
	}
	return nil
}

func (s *Section21Service) available() error {
	if s == nil || !s.enabled || s.db == nil {
		return ErrSection21Disabled
	}
	return nil
}

func newSecret() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw := hex.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(h[:]), nil
}

func tokenHash(raw string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(h[:])
}

func safeMultiply(a, b int64) (int64, bool) {
	if a < 0 || b < 0 || (a != 0 && b > (1<<63-1)/a) {
		return 0, false
	}
	return a * b, true
}
func sid(prefix string) string { return prefix + uuid.NewString() }

type CapacityRequirement struct {
	Required    bool                           `json:"required"`
	Capability  string                         `json:"capability"`
	BranchID    string                         `json:"branch_id"`
	WindowStart time.Time                      `json:"window_start"`
	WindowEnd   time.Time                      `json:"window_end"`
	Allocations []production.AllocationRequest `json:"allocations"`
}

type ScheduledOrderInput struct {
	BusinessID            string
	CustomerID            string
	QuoteID               string
	VendorOrderID         string
	BranchID              string
	DesiredDeliveryAt     time.Time
	DeliveryWindowStart   *time.Time
	DeliveryWindowEnd     *time.Time
	ProductionReadyBy     *time.Time
	ExpectedReadyAt       *time.Time
	FulfilmentPlanID      string
	TentativeVehicleClass string
	BulkRequirement       map[string]any
	Capacity              CapacityRequirement
	SourceRef             string
	SourceVersion         int64
	IdempotencyKey        string
}

type ScheduledOrder struct {
	ID, BusinessID, CustomerID, QuoteID, VendorOrderID, BranchID   string
	DesiredDeliveryAt, ProductionReadyBy, ExpectedReadyAt          *time.Time
	DeliveryWindowStart, DeliveryWindowEnd                         *time.Time
	CapacityReservationID, FulfilmentPlanID, TentativeVehicleClass string
	CapacityStatus, SchedulingStatus, SourceRef, IdempotencyKey    string
	BulkRequirement                                                map[string]any
	SourceVersion                                                  int64
	CreatedAt, AcceptedAt, UpdatedAt                               time.Time
}

func (s *Section21Service) AcceptScheduled(ctx context.Context, in ScheduledOrderInput) (ScheduledOrder, bool, error) {
	if err := s.available(); err != nil {
		return ScheduledOrder{}, false, err
	}
	in.BusinessID, in.CustomerID, in.BranchID, in.IdempotencyKey = strings.TrimSpace(in.BusinessID), strings.TrimSpace(in.CustomerID), strings.TrimSpace(in.BranchID), strings.TrimSpace(in.IdempotencyKey)
	if in.BusinessID == "" || in.CustomerID == "" || in.BranchID == "" || in.QuoteID == "" || in.IdempotencyKey == "" || in.DesiredDeliveryAt.IsZero() {
		return ScheduledOrder{}, false, ErrSection21Conflict
	}
	now := s.now().UTC()
	if !in.DesiredDeliveryAt.After(now) || (in.DeliveryWindowStart != nil && in.DeliveryWindowEnd != nil && !in.DeliveryWindowEnd.After(*in.DeliveryWindowStart)) {
		return ScheduledOrder{}, false, ErrSection21Conflict
	}
	if in.DeliveryWindowStart != nil && in.DeliveryWindowStart.Before(now) {
		return ScheduledOrder{}, false, ErrSection21Conflict
	}
	if in.ProductionReadyBy != nil && !in.ProductionReadyBy.Before(in.DesiredDeliveryAt) {
		return ScheduledOrder{}, false, ErrSection21Conflict
	}
	if in.SourceVersion <= 0 {
		in.SourceVersion = 1
	}
	if in.SourceRef == "" {
		in.SourceRef = "scheduled:" + in.IdempotencyKey
	}
	if s.checkout == nil {
		return ScheduledOrder{}, false, ErrSection21CheckoutNeeded
	}
	if in.TentativeVehicleClass != "" {
		in.TentativeVehicleClass = strings.ToUpper(strings.TrimSpace(in.TentativeVehicleClass))
		if in.TentativeVehicleClass != string(fulfilment.VehicleMotorcycle) && in.TentativeVehicleClass != string(fulfilment.VehicleVan) && in.TentativeVehicleClass != string(fulfilment.VehicleTruck) {
			return ScheduledOrder{}, false, ErrSection21Conflict
		}
	}
	bulkJSON, _ := json.Marshal(in.BulkRequirement)
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return ScheduledOrder{}, false, err
	}
	defer tx.Rollback(ctx)
	var existing ScheduledOrder
	var existingCap *string
	err = tx.QueryRow(ctx, `SELECT id,business_id,customer_id,COALESCE(quote_id,''),COALESCE(vendor_order_id,''),branch_id,desired_delivery_at,delivery_window_start,delivery_window_end,production_ready_by,expected_ready_at,capacity_reservation_id,capacity_status,COALESCE(fulfilment_plan_id,''),COALESCE(tentative_vehicle_class,''),scheduling_status,bulk_requirement,source_ref,source_version,idempotency_key,created_at,accepted_at,updated_at FROM commerce_scheduled_orders WHERE business_id=$1 AND idempotency_key=$2 FOR UPDATE`, in.BusinessID, in.IdempotencyKey).Scan(&existing.ID, &existing.BusinessID, &existing.CustomerID, &existing.QuoteID, &existing.VendorOrderID, &existing.BranchID, &existing.DesiredDeliveryAt, &existing.DeliveryWindowStart, &existing.DeliveryWindowEnd, &existing.ProductionReadyBy, &existing.ExpectedReadyAt, &existingCap, &existing.CapacityStatus, &existing.FulfilmentPlanID, &existing.TentativeVehicleClass, &existing.SchedulingStatus, &bulkJSON, &existing.SourceRef, &existing.SourceVersion, &existing.IdempotencyKey, &existing.CreatedAt, &existing.AcceptedAt, &existing.UpdatedAt)
	if err == nil {
		if existing.CustomerID != in.CustomerID || existing.BranchID != in.BranchID || existing.QuoteID != in.QuoteID {
			return ScheduledOrder{}, false, ErrSection21Conflict
		}
		existing.CapacityReservationID = valuePtr(existingCap)
		_ = json.Unmarshal(bulkJSON, &existing.BulkRequirement)
		if err = tx.Commit(ctx); err != nil {
			return ScheduledOrder{}, false, err
		}
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ScheduledOrder{}, false, err
	}
	approvedQuote, err := s.checkout.Get(ctx, in.CustomerID, in.QuoteID)
	if err != nil || approvedQuote.State != checkoutquote.StateApproved || approvedQuote.ApprovedAt == nil {
		return ScheduledOrder{}, false, ErrSection21CheckoutNeeded
	}
	var reservationID *string
	capacityStatus := "NOT_REQUIRED"
	if in.Capacity.Required {
		if s.production == nil || len(in.Capacity.Allocations) == 0 || in.Capacity.WindowStart.IsZero() || !in.Capacity.WindowEnd.After(in.Capacity.WindowStart) {
			return ScheduledOrder{}, false, ErrSection21Capacity
		}
		actor := production.Actor{VendorID: in.BusinessID, CommandRef: in.IdempotencyKey}
		res, _, reserveErr := s.production.ReserveInTx(ctx, tx, actor, production.Scope{Global: true}, production.ReserveInput{BranchID: in.BranchID, Capability: in.Capacity.Capability, WindowStart: in.Capacity.WindowStart, WindowEnd: in.Capacity.WindowEnd, Allocations: in.Capacity.Allocations, SourceType: "EXTERNAL_ORDER", SourceRef: in.SourceRef, Note: "section21 scheduled order"})
		if reserveErr != nil {
			return ScheduledOrder{}, false, fmt.Errorf("%w: %v", ErrSection21Capacity, reserveErr)
		}
		reservationID, capacityStatus = &res.ID, "RESERVED"
	}
	if !capacityStateCoherent(capacityStatus, valuePtr(reservationID)) {
		return ScheduledOrder{}, false, ErrSection21Capacity
	}
	id := sid("cso_")
	var out ScheduledOrder
	err = tx.QueryRow(ctx, `INSERT INTO commerce_scheduled_orders(id,business_id,customer_id,quote_id,vendor_order_id,branch_id,desired_delivery_at,delivery_window_start,delivery_window_end,production_ready_by,expected_ready_at,capacity_reservation_id,capacity_status,fulfilment_plan_id,tentative_vehicle_class,scheduling_status,bulk_requirement,source_ref,source_version,idempotency_key,accepted_at) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,NULLIF($14,''),NULLIF($15,''),'ACCEPTED',$16,$17,$18,$19,$20) RETURNING id,business_id,customer_id,COALESCE(quote_id,''),COALESCE(vendor_order_id,''),branch_id,desired_delivery_at,delivery_window_start,delivery_window_end,production_ready_by,expected_ready_at,capacity_reservation_id,capacity_status,COALESCE(fulfilment_plan_id,''),COALESCE(tentative_vehicle_class,''),scheduling_status,bulk_requirement,source_ref,source_version,idempotency_key,created_at,accepted_at,updated_at`, id, in.BusinessID, in.CustomerID, in.QuoteID, in.VendorOrderID, in.BranchID, in.DesiredDeliveryAt, in.DeliveryWindowStart, in.DeliveryWindowEnd, in.ProductionReadyBy, in.ExpectedReadyAt, reservationID, capacityStatus, in.FulfilmentPlanID, in.TentativeVehicleClass, bulkJSON, in.SourceRef, in.SourceVersion, in.IdempotencyKey, now).Scan(&out.ID, &out.BusinessID, &out.CustomerID, &out.QuoteID, &out.VendorOrderID, &out.BranchID, &out.DesiredDeliveryAt, &out.DeliveryWindowStart, &out.DeliveryWindowEnd, &out.ProductionReadyBy, &out.ExpectedReadyAt, &out.CapacityReservationID, &out.CapacityStatus, &out.FulfilmentPlanID, &out.TentativeVehicleClass, &out.SchedulingStatus, &bulkJSON, &out.SourceRef, &out.SourceVersion, &out.IdempotencyKey, &out.CreatedAt, &out.AcceptedAt, &out.UpdatedAt)
	if err != nil {
		return ScheduledOrder{}, false, err
	}
	_ = json.Unmarshal(bulkJSON, &out.BulkRequirement)
	if err = s.appendEvent(ctx, tx, "commerce.scheduled_order.accepted", "commerce_scheduled_order", out.ID, in.BusinessID, 1, out); err != nil {
		return ScheduledOrder{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ScheduledOrder{}, false, err
	}
	return out, true, nil
}

func valuePtr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (s *Section21Service) CancelScheduled(ctx context.Context, businessID, id string) error {
	if err := s.available(); err != nil {
		return err
	}
	var reservationID *string
	var branch string
	var version int64
	if err := s.db.QueryRow(ctx, `SELECT capacity_reservation_id,branch_id FROM commerce_scheduled_orders WHERE business_id=$1 AND id=$2 AND scheduling_status<>'CANCELLED'`, strings.TrimSpace(businessID), strings.TrimSpace(id)).Scan(&reservationID, &branch); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSection21NotFound
		}
		return err
	}
	if reservationID != nil && *reservationID != "" {
		if s.production == nil {
			return ErrSection21Capacity
		}
		if err := s.db.QueryRow(ctx, `SELECT version FROM capacity_reservations WHERE business_id=$1 AND id=$2`, businessID, *reservationID).Scan(&version); err != nil {
			return err
		}
		if _, _, err := s.production.Release(ctx, production.Actor{VendorID: businessID, CommandRef: "cancel:" + id}, production.Scope{Global: true}, *reservationID, production.ReleaseInput{ExpectedVersion: version, Reason: "scheduled order cancelled"}); err != nil {
			return err
		}
	}
	_, err := s.db.Exec(ctx, `UPDATE commerce_scheduled_orders SET scheduling_status='CANCELLED',capacity_status=CASE WHEN capacity_reservation_id IS NULL THEN capacity_status ELSE 'RELEASED' END,cancelled_at=now(),updated_at=now() WHERE business_id=$1 AND id=$2 AND scheduling_status<>'CANCELLED'`, strings.TrimSpace(businessID), strings.TrimSpace(id))
	return err
}

func (s *Section21Service) CancelScheduledByStaff(ctx context.Context, actor Actor, id string) error {
	if err := s.available(); err != nil {
		return err
	}
	var branch string
	if err := s.db.QueryRow(ctx, `SELECT branch_id FROM commerce_scheduled_orders WHERE business_id=$1 AND id=$2`, actor.VendorID, id).Scan(&branch); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSection21NotFound
		}
		return err
	}
	if err := s.AuthorizeStaff(ctx, actor, branch); err != nil {
		return err
	}
	return s.CancelScheduled(ctx, actor.VendorID, id)
}

type GroupSession struct {
	ID, OrganizerID, BusinessID, BranchID, Status, Currency, InviteToken string
	ClosingAt, InviteExpiresAt                                           time.Time
	BudgetMinor                                                          *int64
	Version                                                              int64
}
type GroupParticipant struct {
	ID, SessionID, DisplayName, Token string
	TokenExpiresAt                    time.Time
}
type GroupSelection struct {
	ID, SessionID, ParticipantID, ItemRef, VariantRef, Currency string
	Quantity, UnitPriceMinor                                    int64
	Snapshot                                                    map[string]any
}

func (s *Section21Service) CreateGroup(ctx context.Context, organizerID, businessID, branchID string, closingAt time.Time, budget *int64, currency string) (GroupSession, GroupParticipant, error) {
	if err := s.available(); err != nil {
		return GroupSession{}, GroupParticipant{}, err
	}
	now := s.now().UTC()
	if closingAt.Before(now.Add(time.Minute)) || strings.TrimSpace(organizerID) == "" || strings.TrimSpace(businessID) == "" || strings.TrimSpace(branchID) == "" {
		return GroupSession{}, GroupParticipant{}, ErrSection21Conflict
	}
	if currency == "" {
		currency = "EUR"
	}
	if budget != nil && *budget < 0 {
		return GroupSession{}, GroupParticipant{}, ErrSection21Conflict
	}
	invite, inviteHash, err := newSecret()
	if err != nil {
		return GroupSession{}, GroupParticipant{}, err
	}
	participantToken, participantHash, err := newSecret()
	if err != nil {
		return GroupSession{}, GroupParticipant{}, err
	}
	sessionID, participantID := sid("cgs_"), sid("cgp_")
	inviteExp := closingAt.Add(24 * time.Hour)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return GroupSession{}, GroupParticipant{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_group_sessions(id,organizer_id,business_id,branch_id,closing_at,budget_minor,currency,invite_token_hash,invite_expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, sessionID, organizerID, businessID, branchID, closingAt, budget, currency, inviteHash, inviteExp); err != nil {
		return GroupSession{}, GroupParticipant{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_group_participants(id,session_id,display_name,token_hash,token_expires_at) VALUES($1,$2,'Organizer',$3,$4)`, participantID, sessionID, participantHash, inviteExp); err != nil {
		return GroupSession{}, GroupParticipant{}, err
	}
	if err = s.appendEvent(ctx, tx, "commerce.group.created", "commerce_group_session", sessionID, businessID, 1, map[string]any{"organizer_id": organizerID, "branch_id": branchID}); err != nil {
		return GroupSession{}, GroupParticipant{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return GroupSession{}, GroupParticipant{}, err
	}
	return GroupSession{ID: sessionID, OrganizerID: organizerID, BusinessID: businessID, BranchID: branchID, Status: "OPEN", ClosingAt: closingAt, Currency: currency, InviteToken: invite, InviteExpiresAt: inviteExp, BudgetMinor: budget, Version: 1}, GroupParticipant{ID: participantID, SessionID: sessionID, DisplayName: "Organizer", Token: participantToken, TokenExpiresAt: inviteExp}, nil
}

func (s *Section21Service) JoinGroup(ctx context.Context, sessionID, inviteToken, displayName string) (GroupParticipant, error) {
	if err := s.available(); err != nil {
		return GroupParticipant{}, err
	}
	now := s.now().UTC()
	var status string
	var exp time.Time
	if err := s.db.QueryRow(ctx, `SELECT status,closing_at FROM commerce_group_sessions WHERE id=$1`, sessionID).Scan(&status, &exp); err != nil {
		return GroupParticipant{}, ErrSection21NotFound
	}
	if status != "OPEN" || !exp.After(now) {
		return GroupParticipant{}, ErrSection21Expired
	}
	var inviteExp time.Time
	if err := s.db.QueryRow(ctx, `SELECT invite_expires_at FROM commerce_group_sessions WHERE id=$1 AND invite_token_hash=$2 AND invite_revoked_at IS NULL`, sessionID, tokenHash(inviteToken)).Scan(&inviteExp); err != nil {
		return GroupParticipant{}, ErrSection21Forbidden
	}
	if !inviteExp.After(now) {
		return GroupParticipant{}, ErrSection21Expired
	}
	raw, hash, err := newSecret()
	if err != nil {
		return GroupParticipant{}, err
	}
	id := sid("cgp_")
	if _, err = s.db.Exec(ctx, `INSERT INTO commerce_group_participants(id,session_id,display_name,token_hash,token_expires_at) VALUES($1,$2,$3,$4,$5)`, id, sessionID, strings.TrimSpace(displayName), hash, inviteExp); err != nil {
		return GroupParticipant{}, err
	}
	return GroupParticipant{ID: id, SessionID: sessionID, DisplayName: displayName, Token: raw, TokenExpiresAt: inviteExp}, nil
}

func (s *Section21Service) UpsertGroupSelection(ctx context.Context, sessionID, participantToken string, in GroupSelection) (GroupSelection, error) {
	if err := s.available(); err != nil {
		return GroupSelection{}, err
	}
	if in.Quantity <= 0 || in.UnitPriceMinor < 0 || strings.TrimSpace(in.ItemRef) == "" {
		return GroupSelection{}, ErrSection21Conflict
	}
	now := s.now().UTC()
	var pid string
	var budget *int64
	var status string
	var closing time.Time
	if err := s.db.QueryRow(ctx, `SELECT p.id,g.budget_minor,g.status,g.closing_at FROM commerce_group_participants p JOIN commerce_group_sessions g ON g.id=p.session_id WHERE p.session_id=$1 AND p.token_hash=$2 AND p.revoked_at IS NULL AND p.token_expires_at>$3`, sessionID, tokenHash(participantToken), now).Scan(&pid, &budget, &status, &closing); err != nil {
		return GroupSelection{}, ErrSection21Forbidden
	}
	if status != "OPEN" || !closing.After(now) {
		return GroupSelection{}, ErrSection21Expired
	}
	var raw []byte
	raw, _ = json.Marshal(in.Snapshot)
	var total int64
	if err := s.db.QueryRow(ctx, `SELECT COALESCE(SUM(quantity*unit_price_minor),0) FROM commerce_group_selections WHERE session_id=$1 AND participant_id=$2 AND id<>$3`, sessionID, pid, in.ID).Scan(&total); err != nil {
		return GroupSelection{}, err
	}
	lineTotal, ok := safeMultiply(in.Quantity, in.UnitPriceMinor)
	if !ok || total > (1<<63-1)-lineTotal {
		return GroupSelection{}, ErrSection21Conflict
	}
	proposed := total + lineTotal
	if budget != nil && proposed > *budget {
		return GroupSelection{}, ErrSection21Conflict
	}
	if in.ID == "" {
		in.ID = sid("cgsx_")
	}
	_, err := s.db.Exec(ctx, `INSERT INTO commerce_group_selections(id,session_id,participant_id,item_ref,variant_ref,quantity,unit_price_minor,currency,snapshot) VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9) ON CONFLICT (id,participant_id) DO UPDATE SET item_ref=EXCLUDED.item_ref,variant_ref=EXCLUDED.variant_ref,quantity=EXCLUDED.quantity,unit_price_minor=EXCLUDED.unit_price_minor,currency=EXCLUDED.currency,snapshot=EXCLUDED.snapshot,updated_at=now()`, in.ID, sessionID, pid, in.ItemRef, in.VariantRef, in.Quantity, in.UnitPriceMinor, in.Currency, raw)
	if err != nil {
		return GroupSelection{}, err
	}
	in.SessionID = sessionID
	in.ParticipantID = pid
	return in, nil
}

func (s *Section21Service) DeleteGroupSelection(ctx context.Context, sessionID, participantToken, selectionID string) error {
	if err := s.available(); err != nil {
		return err
	}
	now := s.now().UTC()
	tag, err := s.db.Exec(ctx, `DELETE FROM commerce_group_selections WHERE id=$1 AND session_id=$2 AND participant_id IN (SELECT id FROM commerce_group_participants WHERE session_id=$2 AND token_hash=$3 AND revoked_at IS NULL AND token_expires_at>$4) AND EXISTS (SELECT 1 FROM commerce_group_sessions WHERE id=$2 AND status='OPEN' AND closing_at>$4)`, selectionID, sessionID, tokenHash(participantToken), now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSection21Forbidden
	}
	return nil
}

func (s *Section21Service) CloseGroup(ctx context.Context, organizerID, sessionID string) error {
	if err := s.available(); err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status, businessID string
	if err = tx.QueryRow(ctx, `SELECT status,business_id FROM commerce_group_sessions WHERE id=$1 AND organizer_id=$2 FOR UPDATE`, sessionID, organizerID).Scan(&status, &businessID); err != nil {
		return ErrSection21NotFound
	}
	if status == "CLOSED" || status == "CONVERTING" || status == "CONVERTED" {
		return tx.Commit(ctx)
	}
	if status != "OPEN" {
		return ErrSection21InvalidState
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_group_sessions SET status='CLOSED',closed_at=now(),version=version+1,updated_at=now() WHERE id=$1`, sessionID); err != nil {
		return err
	}
	if err = s.appendEvent(ctx, tx, "commerce.group.closed", "commerce_group_session", sessionID, businessID, 2, map[string]any{"session_id": sessionID}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Section21Service) ConvertGroup(ctx context.Context, organizerID, sessionID, quoteID, fingerprint string) (ParentOrder, bool, error) {
	if err := s.available(); err != nil {
		return ParentOrder{}, false, err
	}
	var existing *string
	var status string
	if err := s.db.QueryRow(ctx, `SELECT status,conversion_parent_order_id FROM commerce_group_sessions WHERE id=$1 AND organizer_id=$2`, sessionID, organizerID).Scan(&status, &existing); err != nil {
		return ParentOrder{}, false, ErrSection21NotFound
	}
	if existing != nil && *existing != "" && s.orders != nil {
		order, e := s.orders.GetForCustomer(ctx, organizerID, *existing)
		return order, false, e
	}
	if status != "CLOSED" && status != "CONVERTING" {
		return ParentOrder{}, false, ErrSection21InvalidState
	}
	if s.orders == nil {
		return ParentOrder{}, false, ErrSection21CheckoutNeeded
	}
	if _, err := s.db.Exec(ctx, `UPDATE commerce_group_sessions SET status='CONVERTING',conversion_quote_id=$3,updated_at=now() WHERE id=$1 AND organizer_id=$2 AND status IN ('CLOSED','CONVERTING')`, sessionID, organizerID, quoteID); err != nil {
		return ParentOrder{}, false, err
	}
	order, created, err := s.orders.CreateFromQuote(ctx, organizerID, quoteID, fingerprint)
	if err != nil {
		return ParentOrder{}, false, err
	}
	if _, err = s.db.Exec(ctx, `UPDATE commerce_group_sessions SET status='CONVERTED',conversion_parent_order_id=$3,converted_at=now(),updated_at=now() WHERE id=$1 AND organizer_id=$2`, sessionID, organizerID, order.ID); err != nil {
		return ParentOrder{}, false, err
	}
	_, _ = s.db.Exec(ctx, `INSERT INTO commerce_group_attributions(id,session_id,parent_order_id,participant_id,selection_id,snapshot) SELECT 'cga_'||id,$1,$2,participant_id,id,snapshot FROM commerce_group_selections WHERE session_id=$1 ON CONFLICT DO NOTHING`, sessionID, order.ID)
	return order, created, nil
}

type RFQInput struct {
	RequesterID, BusinessID                                                     string
	EligibilityScope                                                            map[string]any
	Servings                                                                    int64
	RequestedAt                                                                 time.Time
	DestinationSnapshot, MenuRequirements, PackagingRequirements, DeliveryTerms map[string]any
	BudgetMinor                                                                 *int64
	Currency                                                                    string
	Capacity                                                                    CapacityRequirement
	DeadlineAt, ExpiresAt                                                       time.Time
}
type RFQ struct {
	ID, RequesterID, BusinessID, Status string
	Servings                            int64
	RequestedAt, DeadlineAt, ExpiresAt  time.Time
	BudgetMinor                         *int64
	Currency                            string
	Version                             int64
	AcceptedQuoteID, CheckoutQuoteID    string
}
type RFQQuoteRevision struct {
	ID, QuoteID, RFQID, VendorID, BranchID string
	Revision                               int64
	PriceMinor                             int64
	Currency                               string
	Snapshot                               map[string]any
	Capacity                               CapacityRequirement
	VendorTerms                            map[string]any
	PaymentPolicy                          map[string]any
	DepositPolicy                          map[string]any
	CancellationPolicy                     map[string]any
	ExpiresAt                              time.Time
	Status                                 string
}

func (s *Section21Service) ListRFQQuotes(ctx context.Context, requesterID, rfqID string) ([]RFQQuoteRevision, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM commerce_rfqs WHERE id=$1 AND requester_id=$2)`, rfqID, requesterID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrSection21NotFound
	}
	rows, err := s.db.Query(ctx, `SELECT q.id,q.rfq_id,q.vendor_id,q.branch_id,r.id,r.revision,r.price_minor,r.currency,r.snapshot,r.capacity_requirement,r.vendor_terms,r.payment_policy,r.deposit_policy,r.cancellation_policy,r.expires_at,q.status FROM commerce_rfq_quotes q JOIN commerce_rfq_quote_revisions r ON r.quote_id=q.id AND r.revision=q.current_revision WHERE q.rfq_id=$1 ORDER BY q.created_at,q.id`, rfqID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]RFQQuoteRevision, 0)
	for rows.Next() {
		var q RFQQuoteRevision
		var raw, capraw, vendorRaw, paymentRaw, depositRaw, cancellationRaw []byte
		if err := rows.Scan(&q.QuoteID, &q.RFQID, &q.VendorID, &q.BranchID, &q.ID, &q.Revision, &q.PriceMinor, &q.Currency, &raw, &capraw, &vendorRaw, &paymentRaw, &depositRaw, &cancellationRaw, &q.ExpiresAt, &q.Status); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &q.Snapshot)
		_ = json.Unmarshal(capraw, &q.Capacity)
		_ = json.Unmarshal(vendorRaw, &q.VendorTerms)
		_ = json.Unmarshal(paymentRaw, &q.PaymentPolicy)
		_ = json.Unmarshal(depositRaw, &q.DepositPolicy)
		_ = json.Unmarshal(cancellationRaw, &q.CancellationPolicy)
		out = append(out, q)
	}
	return out, rows.Err()
}

func (s *Section21Service) CreateRFQ(ctx context.Context, in RFQInput) (RFQ, error) {
	if err := s.available(); err != nil {
		return RFQ{}, err
	}
	now := s.now().UTC()
	if strings.TrimSpace(in.RequesterID) == "" || in.Servings <= 0 || in.RequestedAt.IsZero() || !in.ExpiresAt.After(now) || !in.DeadlineAt.After(now) || in.ExpiresAt.Before(in.DeadlineAt) {
		return RFQ{}, ErrSection21Conflict
	}
	if in.Currency == "" {
		in.Currency = "EUR"
	}
	if err := validateRFQRestrictedGoods(in); err != nil {
		return RFQ{}, err
	}
	elig, _ := json.Marshal(in.EligibilityScope)
	dest, _ := json.Marshal(in.DestinationSnapshot)
	menu, _ := json.Marshal(in.MenuRequirements)
	pack, _ := json.Marshal(in.PackagingRequirements)
	terms, _ := json.Marshal(in.DeliveryTerms)
	capb, _ := json.Marshal(in.Capacity)
	id := sid("rfq_")
	_, err := s.db.Exec(ctx, `INSERT INTO commerce_rfqs(id,requester_id,business_id,eligibility_scope,servings,requested_at,destination_snapshot,menu_requirements,packaging_requirements,budget_minor,currency,delivery_terms,capacity_requirement,deadline_at,expires_at) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, id, in.RequesterID, in.BusinessID, elig, in.Servings, in.RequestedAt, dest, menu, pack, in.BudgetMinor, in.Currency, terms, capb, in.DeadlineAt, in.ExpiresAt)
	if err != nil {
		return RFQ{}, err
	}
	return RFQ{ID: id, RequesterID: in.RequesterID, BusinessID: in.BusinessID, Status: "DRAFT", Servings: in.Servings, RequestedAt: in.RequestedAt, DeadlineAt: in.DeadlineAt, ExpiresAt: in.ExpiresAt, BudgetMinor: in.BudgetMinor, Currency: in.Currency, Version: 1}, nil
}

func validateRFQRestrictedGoods(in RFQInput) error {
	decision := taxonomy.EvaluateRestrictedGoodsPayload(map[string]any{
		"eligibility_scope":      in.EligibilityScope,
		"menu_requirements":      in.MenuRequirements,
		"packaging_requirements": in.PackagingRequirements,
		"delivery_terms":         in.DeliveryTerms,
	})
	if !decision.Allowed {
		return fmt.Errorf("%w: %s (%s)", ErrRestrictedGoods, decision.ReasonCode, decision.PolicyVersion)
	}
	return nil
}
func (s *Section21Service) OpenRFQ(ctx context.Context, requesterID, id string) error {
	if err := s.available(); err != nil {
		return err
	}
	tag, err := s.db.Exec(ctx, `UPDATE commerce_rfqs SET status='OPEN',opened_at=now(),version=version+1,updated_at=now() WHERE id=$1 AND requester_id=$2 AND status='DRAFT' AND deadline_at>now()`, id, requesterID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSection21InvalidState
	}
	return nil
}
func (s *Section21Service) SubmitRFQQuote(ctx context.Context, rfqID, vendorID, branchID string, price int64, currency string, snapshot map[string]any, cap CapacityRequirement, expires time.Time, idempotency ...string) (RFQQuoteRevision, error) {
	if err := s.available(); err != nil {
		return RFQQuoteRevision{}, err
	}
	if price < 0 || strings.TrimSpace(vendorID) == "" || strings.TrimSpace(branchID) == "" || !expires.After(s.now().UTC()) {
		return RFQQuoteRevision{}, ErrSection21Conflict
	}
	var status string
	var scope []byte
	var deadline time.Time
	if err := s.db.QueryRow(ctx, `SELECT status,eligibility_scope,deadline_at FROM commerce_rfqs WHERE id=$1`, rfqID).Scan(&status, &scope, &deadline); err != nil {
		return RFQQuoteRevision{}, ErrSection21NotFound
	}
	if status != "OPEN" && status != "QUOTED" || !deadline.After(s.now().UTC()) {
		return RFQQuoteRevision{}, ErrSection21Expired
	}
	var eligible struct {
		VendorIDs []string `json:"vendor_ids"`
	}
	_ = json.Unmarshal(scope, &eligible)
	// There is no implicit "all vendors" pool. Until a capability/service-area
	// authority supplies an explicit eligible pool, quoting fails closed.
	if len(eligible.VendorIDs) == 0 || !contains(eligible.VendorIDs, vendorID) {
		return RFQQuoteRevision{}, ErrSection21Forbidden
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RFQQuoteRevision{}, err
	}
	defer tx.Rollback(ctx)
	key := ""
	if len(idempotency) > 0 {
		key = strings.TrimSpace(idempotency[0])
	}
	var qid string
	var rev int64
	if key != "" {
		var prior RFQQuoteRevision
		var raw, capraw, vendorRaw, paymentRaw, depositRaw, cancellationRaw []byte
		err = tx.QueryRow(ctx, `SELECT r.id,r.quote_id,r.rfq_id,q.vendor_id,q.branch_id,r.revision,r.price_minor,r.currency,r.snapshot,r.capacity_requirement,r.vendor_terms,r.payment_policy,r.deposit_policy,r.cancellation_policy,r.expires_at,q.status FROM commerce_rfq_quote_revisions r JOIN commerce_rfq_quotes q ON q.id=r.quote_id WHERE r.rfq_id=$1 AND r.idempotency_key=$2 FOR UPDATE`, rfqID, key).Scan(&prior.ID, &prior.QuoteID, &prior.RFQID, &prior.VendorID, &prior.BranchID, &prior.Revision, &prior.PriceMinor, &prior.Currency, &raw, &capraw, &vendorRaw, &paymentRaw, &depositRaw, &cancellationRaw, &prior.ExpiresAt, &prior.Status)
		if err == nil {
			_ = json.Unmarshal(raw, &prior.Snapshot)
			_ = json.Unmarshal(capraw, &prior.Capacity)
			_ = json.Unmarshal(vendorRaw, &prior.VendorTerms)
			_ = json.Unmarshal(paymentRaw, &prior.PaymentPolicy)
			_ = json.Unmarshal(depositRaw, &prior.DepositPolicy)
			_ = json.Unmarshal(cancellationRaw, &prior.CancellationPolicy)
			if err = tx.Commit(ctx); err != nil {
				return RFQQuoteRevision{}, err
			}
			return prior, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return RFQQuoteRevision{}, err
		}
	}
	err = tx.QueryRow(ctx, `SELECT id,current_revision FROM commerce_rfq_quotes WHERE rfq_id=$1 AND vendor_id=$2 AND branch_id=$3 FOR UPDATE`, rfqID, vendorID, branchID).Scan(&qid, &rev)
	if errors.Is(err, pgx.ErrNoRows) {
		qid = sid("rfq_q_")
		rev = 0
		if _, err = tx.Exec(ctx, `INSERT INTO commerce_rfq_quotes(id,rfq_id,vendor_id,branch_id) VALUES($1,$2,$3,$4)`, qid, rfqID, vendorID, branchID); err != nil {
			return RFQQuoteRevision{}, err
		}
	} else if err != nil {
		return RFQQuoteRevision{}, err
	}
	rev++
	raw, _ := json.Marshal(snapshot)
	capraw, _ := json.Marshal(cap)
	vendorRaw := snapshotMap(snapshot, "vendor_terms")
	paymentRaw := snapshotMapWithDefault(snapshot, "payment_policy", map[string]any{"type": "FULL_PAYMENT"})
	depositRaw := snapshotMap(snapshot, "deposit_policy")
	cancellationRaw := snapshotMap(snapshot, "cancellation_policy")
	rid := sid("rfq_r_")
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_rfq_quote_revisions(id,quote_id,rfq_id,revision,price_minor,currency,snapshot,capacity_requirement,vendor_terms,payment_policy,deposit_policy,cancellation_policy,created_by,expires_at,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NULLIF($15,''))`, rid, qid, rfqID, rev, price, currency, raw, capraw, vendorRaw, paymentRaw, depositRaw, cancellationRaw, vendorID, expires, key); err != nil {
		return RFQQuoteRevision{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_rfq_quotes SET current_revision=$2,updated_at=now() WHERE id=$1`, qid, rev); err != nil {
		return RFQQuoteRevision{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_rfqs SET status='QUOTED',version=version+1,updated_at=now() WHERE id=$1 AND status IN ('OPEN','QUOTED')`, rfqID); err != nil {
		return RFQQuoteRevision{}, err
	}
	if err = s.appendEvent(ctx, tx, "commerce.rfq.quote_submitted", "commerce_rfq", rfqID, vendorID, rev, map[string]any{"quote_id": qid, "revision": rev}); err != nil {
		return RFQQuoteRevision{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return RFQQuoteRevision{}, err
	}
	var vendorTerms, paymentPolicy, depositPolicy, cancellationPolicy map[string]any
	_ = json.Unmarshal(vendorRaw, &vendorTerms)
	_ = json.Unmarshal(paymentRaw, &paymentPolicy)
	_ = json.Unmarshal(depositRaw, &depositPolicy)
	_ = json.Unmarshal(cancellationRaw, &cancellationPolicy)
	return RFQQuoteRevision{ID: rid, QuoteID: qid, RFQID: rfqID, VendorID: vendorID, BranchID: branchID, Revision: rev, PriceMinor: price, Currency: currency, Snapshot: snapshot, Capacity: cap, VendorTerms: vendorTerms, PaymentPolicy: paymentPolicy, DepositPolicy: depositPolicy, CancellationPolicy: cancellationPolicy, ExpiresAt: expires, Status: "ACTIVE"}, nil
}

func snapshotMap(snapshot map[string]any, key string) []byte {
	value, ok := snapshot[key]
	if !ok || value == nil {
		return []byte(`{}`)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte(`{}`)
	}
	return raw
}

func snapshotMapWithDefault(snapshot map[string]any, key string, fallback map[string]any) []byte {
	if _, ok := snapshot[key]; !ok {
		raw, _ := json.Marshal(fallback)
		return raw
	}
	return snapshotMap(snapshot, key)
}
func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func rfqCheckoutBindingDigest(rfqID, quoteID string, revision int64, checkoutQuoteID, checkoutFingerprint, vendorID, branchID, currency string, price int64, reservationID string) string {
	payload := fmt.Sprintf("%s|%s|%d|%s|%s|%s|%s|%s|%d|%s", rfqID, quoteID, revision, checkoutQuoteID, checkoutFingerprint, vendorID, branchID, currency, price, reservationID)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func snapshotString(snapshot map[string]any, key string) string {
	if value, ok := snapshot[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func capacityStateCoherent(status, reservationID string) bool {
	return (status == "RESERVED") == (strings.TrimSpace(reservationID) != "") && status != "CONFLICT"
}

func (s *Section21Service) AcceptRFQQuote(ctx context.Context, requesterID, rfqID, quoteID string, revision int64, checkoutQuoteID string) (RFQ, error) {
	if err := s.available(); err != nil {
		return RFQ{}, err
	}
	if strings.TrimSpace(checkoutQuoteID) == "" || s.checkout == nil {
		return RFQ{}, ErrSection21CheckoutNeeded
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RFQ{}, err
	}
	defer tx.Rollback(ctx)
	var r RFQ
	var capraw, snapshotRaw, paymentPolicyRaw, cancellationPolicyRaw []byte
	var vendor, branch string
	var price int64
	var expires time.Time
	if err = tx.QueryRow(ctx, `SELECT id,requester_id,COALESCE(business_id,''),status,servings,requested_at,deadline_at,expires_at,budget_minor,currency,version,COALESCE(accepted_quote_id,''),COALESCE(checkout_quote_id,'') FROM commerce_rfqs WHERE id=$1 AND requester_id=$2 FOR UPDATE`, rfqID, requesterID).Scan(&r.ID, &r.RequesterID, &r.BusinessID, &r.Status, &r.Servings, &r.RequestedAt, &r.DeadlineAt, &r.ExpiresAt, &r.BudgetMinor, &r.Currency, &r.Version, &r.AcceptedQuoteID, &r.CheckoutQuoteID); err != nil {
		return RFQ{}, ErrSection21NotFound
	}
	if r.Status != "OPEN" && r.Status != "QUOTED" || !r.ExpiresAt.After(s.now().UTC()) {
		if r.Status == "ACCEPTED" && r.AcceptedQuoteID == quoteID && r.CheckoutQuoteID == checkoutQuoteID {
			var boundQuoteID, boundDigest string
			var boundRevision int64
			if err = tx.QueryRow(ctx, `SELECT accepted_quote_id,accepted_revision,binding_digest FROM commerce_rfq_checkout_bindings WHERE rfq_id=$1 AND checkout_quote_id=$2`, rfqID, checkoutQuoteID).Scan(&boundQuoteID, &boundRevision, &boundDigest); err != nil {
				return RFQ{}, ErrSection21Conflict
			}
			if boundQuoteID != quoteID || boundRevision != revision || strings.TrimSpace(boundDigest) == "" {
				return RFQ{}, ErrSection21Conflict
			}
			if err = tx.Commit(ctx); err != nil {
				return RFQ{}, err
			}
			return r, nil
		}
		return RFQ{}, ErrSection21Expired
	}
	checkout, err := s.checkout.Get(ctx, requesterID, checkoutQuoteID)
	if err != nil || checkout.Expired {
		return RFQ{}, fmt.Errorf("%w: %v", ErrSection21CheckoutNeeded, err)
	}
	if err = tx.QueryRow(ctx, `SELECT q.vendor_id,q.branch_id,rev.price_minor,rev.expires_at,rev.capacity_requirement,rev.snapshot,rev.payment_policy,rev.cancellation_policy FROM commerce_rfq_quotes q JOIN commerce_rfq_quote_revisions rev ON rev.quote_id=q.id AND rev.revision=$3 AND q.current_revision=$3 WHERE q.id=$1 AND q.rfq_id=$2 AND q.status='ACTIVE' FOR UPDATE`, quoteID, rfqID, revision).Scan(&vendor, &branch, &price, &expires, &capraw, &snapshotRaw, &paymentPolicyRaw, &cancellationPolicyRaw); err != nil {
		return RFQ{}, ErrSection21Conflict
	}
	if !expires.After(s.now().UTC()) {
		return RFQ{}, ErrSection21Expired
	}
	if !checkout.ExpiresAt.After(s.now().UTC()) || checkout.Currency != r.Currency || checkout.FinalPayableMinor != price {
		return RFQ{}, ErrSection21Conflict
	}
	var acceptedSnapshot map[string]any
	_ = json.Unmarshal(snapshotRaw, &acceptedSnapshot)
	if snapshotString(acceptedSnapshot, "rfq_id") != rfqID || snapshotString(acceptedSnapshot, "checkout_quote_id") != checkoutQuoteID || snapshotString(acceptedSnapshot, "checkout_quote_fingerprint") != checkout.QuoteFingerprint {
		return RFQ{}, ErrSection21Conflict
	}
	if v := snapshotString(acceptedSnapshot, "vendor_id"); v != "" && v != vendor {
		return RFQ{}, ErrSection21Conflict
	}
	if v := snapshotString(acceptedSnapshot, "branch_id"); v != "" && v != branch {
		return RFQ{}, ErrSection21Conflict
	}
	matchedGroup := false
	for _, group := range checkout.Groups {
		if group.BusinessID == vendor && group.BranchID == branch {
			matchedGroup = true
			break
		}
	}
	if !matchedGroup {
		return RFQ{}, ErrSection21Conflict
	}
	var cap CapacityRequirement
	_ = json.Unmarshal(capraw, &cap)
	var reservation *string
	if cap.Required {
		if s.production == nil || len(cap.Allocations) == 0 {
			return RFQ{}, ErrSection21Capacity
		}
		res, _, e := s.production.ReserveInTx(ctx, tx, production.Actor{VendorID: vendor, CommandRef: "rfq:" + rfqID}, production.Scope{Global: true}, production.ReserveInput{BranchID: branch, Capability: cap.Capability, WindowStart: cap.WindowStart, WindowEnd: cap.WindowEnd, Allocations: cap.Allocations, SourceType: "EXTERNAL_ORDER", SourceRef: rfqID, Note: "section21 rfq acceptance"})
		if e != nil {
			return RFQ{}, fmt.Errorf("%w: %v", ErrSection21Capacity, e)
		}
		reservation = &res.ID
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_rfq_quotes SET status='CLOSED',updated_at=now() WHERE rfq_id=$1 AND id<>$2 AND status='ACTIVE'`, rfqID, quoteID); err != nil {
		return RFQ{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_rfq_quotes SET status='ACCEPTED',accepted_revision=$3,updated_at=now() WHERE id=$1 AND rfq_id=$2`, quoteID, rfqID, revision); err != nil {
		return RFQ{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_rfqs SET status='ACCEPTED',accepted_quote_id=$2,checkout_quote_id=$3,accepted_at=now(),version=version+1 WHERE id=$1 AND status IN ('OPEN','QUOTED')`, rfqID, quoteID, checkoutQuoteID); err != nil {
		return RFQ{}, err
	}
	reservationRef := ""
	if reservation != nil {
		reservationRef = *reservation
	}
	digest := rfqCheckoutBindingDigest(rfqID, quoteID, revision, checkoutQuoteID, checkout.QuoteFingerprint, vendor, branch, checkout.Currency, price, reservationRef)
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_rfq_checkout_bindings(id,rfq_id,checkout_quote_id,accepted_quote_id,accepted_revision,requester_id,vendor_id,branch_id,price_minor,currency,capacity_reservation_id,binding_digest,expires_at,payment_policy,cancellation_policy) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, sid("rfqb_"), rfqID, checkoutQuoteID, quoteID, revision, requesterID, vendor, branch, price, checkout.Currency, reservation, digest, expires, paymentPolicyRaw, cancellationPolicyRaw); err != nil {
		return RFQ{}, err
	}
	if err = s.appendEvent(ctx, tx, "commerce.rfq.accepted", "commerce_rfq", rfqID, vendor, r.Version+1, map[string]any{"quote_id": quoteID, "revision": revision, "capacity_reservation_id": reservation}); err != nil {
		return RFQ{}, err
	}
	r.Status = "ACCEPTED"
	r.AcceptedQuoteID = quoteID
	r.CheckoutQuoteID = checkoutQuoteID
	r.Version++
	_ = price
	_ = reservation
	if err = tx.Commit(ctx); err != nil {
		return RFQ{}, err
	}
	return r, nil
}
func (s *Section21Service) ConvertRFQ(ctx context.Context, requesterID, rfqID string) error {
	if err := s.available(); err != nil {
		return err
	}
	if s.checkout == nil {
		return ErrSection21CheckoutNeeded
	}
	var bindingID, checkoutID, quoteID, vendor, branch, currency, digest, reservation, rfqStatus string
	var revision int64
	var price int64
	var expires time.Time
	var paymentRaw, cancellationRaw []byte
	if err := s.db.QueryRow(ctx, `SELECT b.id,b.checkout_quote_id,b.accepted_quote_id,b.accepted_revision,b.vendor_id,b.branch_id,b.price_minor,b.currency,COALESCE(b.capacity_reservation_id,''),b.binding_digest,b.expires_at,b.payment_policy,b.cancellation_policy,r.status FROM commerce_rfq_checkout_bindings b JOIN commerce_rfqs r ON r.id=b.rfq_id WHERE b.rfq_id=$1 AND r.requester_id=$2 AND r.status IN ('ACCEPTED','CONVERTED')`, rfqID, requesterID).Scan(&bindingID, &checkoutID, &quoteID, &revision, &vendor, &branch, &price, &currency, &reservation, &digest, &expires, &paymentRaw, &cancellationRaw, &rfqStatus); err != nil {
		return ErrSection21CheckoutNeeded
	}
	checkout, err := s.checkout.Get(ctx, requesterID, checkoutID)
	if err != nil || checkout.Expired || !checkout.ExpiresAt.After(s.now().UTC()) {
		return ErrSection21CheckoutNeeded
	}
	if checkout.Currency != currency || checkout.FinalPayableMinor != price || !expires.After(s.now().UTC()) {
		return ErrSection21Conflict
	}
	if rfqCheckoutBindingDigest(rfqID, quoteID, revision, checkoutID, checkout.QuoteFingerprint, vendor, branch, currency, price, reservation) != digest {
		return ErrSection21Conflict
	}
	if rfqStatus == "CONVERTED" {
		return nil
	}
	if err := s.ensureRFQPaymentAndOrder(ctx, requesterID, rfqID, bindingID, checkoutID, checkout.QuoteFingerprint, vendor, currency, price, paymentRaw, cancellationRaw); err != nil {
		return err
	}
	tag, err := s.db.Exec(ctx, `UPDATE commerce_rfqs SET status='CONVERTED',converted_at=now(),version=version+1,updated_at=now() WHERE id=$1 AND requester_id=$2 AND status='ACCEPTED' AND checkout_quote_id=$3`, rfqID, requesterID, checkoutID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var currentStatus string
		if readErr := s.db.QueryRow(ctx, `SELECT status FROM commerce_rfqs WHERE id=$1 AND requester_id=$2`, rfqID, requesterID).Scan(&currentStatus); readErr == nil && currentStatus == "CONVERTED" {
			return nil
		}
		return ErrSection21InvalidState
	}
	return s.completeRFQPaymentConversion(ctx, rfqID)
}
func (s *Section21Service) ExpireRFQs(ctx context.Context, limit int) (int64, error) {
	if err := s.available(); err != nil {
		return 0, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	tag, err := s.db.Exec(ctx, `UPDATE commerce_rfqs SET status='EXPIRED',version=version+1,updated_at=now() WHERE id IN (SELECT id FROM commerce_rfqs WHERE status IN ('OPEN','QUOTED') AND expires_at<=now() ORDER BY expires_at LIMIT $1)`, limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
