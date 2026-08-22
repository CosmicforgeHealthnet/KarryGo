package purchasing

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Purchasing operational alerts and their sweeps.
//
// These are the worker entry points. Each one is a bounded, cross-tenant scan
// that raises durable alerts, and each is safe to run repeatedly: the alert
// store carries a unique (business_id, dedupe_key), so a second pass in the
// same window conflicts instead of raising a duplicate.
//
// The workers themselves are registered on the existing durable job
// coordinator in cmd/main.go. Nothing here schedules, leases or retries; that
// is the platform's job infrastructure and is not reimplemented.

const (
	AlertPOExpectedSoon      = "PO_EXPECTED_SOON"
	AlertPOExpectedToday     = "PO_EXPECTED_TODAY"
	AlertPOOverdue           = "PO_OVERDUE"
	AlertPOMateriallyOverdue = "PO_MATERIALLY_OVERDUE"
	AlertDiscrepancyStale    = "DISCREPANCY_UNRESOLVED"
	AlertDiscrepancyEscalate = "DISCREPANCY_ESCALATED"

	AlertStatusOpen         = "OPEN"
	AlertStatusAcknowledged = "ACKNOWLEDGED"
	AlertStatusResolved     = "RESOLVED"

	SeverityInfo     = "INFO"
	SeverityWarning  = "WARNING"
	SeverityCritical = "CRITICAL"
)

// discrepancyStaleHours is when an unresolved discrepancy becomes a signal.
const discrepancyStaleHours = 48

// discrepancyEscalateHours is when it becomes an escalation.
const discrepancyEscalateHours = 168

// PurchasingAlert is one durable operational signal.
type PurchasingAlert struct {
	ID         string `json:"id"`
	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`

	AlertType string `json:"alert_type"`
	Severity  string `json:"severity"`
	Status    string `json:"status"`

	PurchaseOrderID string `json:"purchase_order_id"`
	SupplierID      string `json:"supplier_id"`
	DiscrepancyID   string `json:"discrepancy_id"`

	DedupeKey string          `json:"dedupe_key"`
	Details   json.RawMessage `json:"details"`

	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at"`
	ResolvedAt     *time.Time `json:"resolved_at"`
}

// deliveryAlertKey encodes the alert window. Including the expected date and
// the severity band means a purchase order that stays overdue does not raise a
// new alert every sweep, but one that crosses into "materially overdue" does
// raise the escalation exactly once.
func deliveryAlertKey(purchaseOrderID, alertType string, expected time.Time) string {
	return fmt.Sprintf("po:%s:%s:%s",
		purchaseOrderID, alertType, expected.UTC().Format("2006-01-02"))
}

func discrepancyAlertKey(discrepancyID, alertType string) string {
	return fmt.Sprintf("disc:%s:%s", discrepancyID, alertType)
}

// SweepDeliveryAlerts raises expected-delivery and overdue alerts.
//
// It also resolves alerts whose purchase order has moved on: an order that has
// since been received or cancelled no longer has an outstanding delivery, and
// leaving its alert open would be a false operational signal.
//
// Returns the number of alerts raised.
func (s *GoodsReceivingService) SweepDeliveryAlerts(
	ctx context.Context,
	limit int,
) (int, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}

	// Close out alerts whose subject is no longer outstanding. This runs
	// first so a delivered order never keeps an overdue alert open, and it is
	// idempotent because it only touches rows that are still OPEN.
	if _, err := s.db.Exec(ctx,
		`UPDATE vendor_purchasing_alerts a
		    SET status = 'RESOLVED',
		        resolved_at = now(),
		        updated_at = now()
		  WHERE a.status = 'OPEN'
		    AND a.purchase_order_id <> ''
		    AND EXISTS (
		        SELECT 1 FROM vendor_purchase_orders o
		         WHERE o.id = a.purchase_order_id
		           AND o.business_id = a.business_id
		           AND o.status NOT IN ('ORDERED','PARTIALLY_RECEIVED')
		    )`); err != nil {
		return 0, err
	}

	rows, err := s.db.Query(ctx,
		`SELECT
		    o.id, o.business_id, o.branch_id, o.supplier_id,
		    o.purchase_order_number, o.status, o.expected_delivery_date,
		    current_date - o.expected_delivery_date
		   FROM vendor_purchase_orders o
		  WHERE o.status IN ('ORDERED','PARTIALLY_RECEIVED')
		    AND o.expected_delivery_date IS NOT NULL
		    AND o.expected_delivery_date <= current_date + ($1::int)
		  ORDER BY o.expected_delivery_date ASC, o.id ASC
		  LIMIT $2`,
		expectedSoonDays, limit)
	if err != nil {
		return 0, err
	}

	type candidate struct {
		id, business, branch, supplier, number, status string
		expected                                       time.Time
		daysLate                                       int
	}

	pending := make([]candidate, 0, limit)

	for rows.Next() {
		var c candidate
		if err := rows.Scan(
			&c.id, &c.business, &c.branch, &c.supplier,
			&c.number, &c.status, &c.expected, &c.daysLate,
		); err != nil {
			rows.Close()
			return 0, err
		}
		pending = append(pending, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	raised := 0

	for _, c := range pending {
		alertType, severity := deliveryAlertFor(c.daysLate)

		details, _ := json.Marshal(map[string]any{
			"purchase_order_number":  c.number,
			"purchase_order_status":  c.status,
			"expected_delivery_date": c.expected.UTC().Format("2006-01-02"),
			"days_late":              c.daysLate,
		})

		created, err := s.raiseAlert(ctx, PurchasingAlert{
			BusinessID:      c.business,
			BranchID:        c.branch,
			AlertType:       alertType,
			Severity:        severity,
			PurchaseOrderID: c.id,
			SupplierID:      c.supplier,
			DedupeKey:       deliveryAlertKey(c.id, alertType, c.expected),
			Details:         details,
		})
		if err != nil {
			return raised, err
		}
		if created {
			raised++
		}
	}

	return raised, nil
}

// deliveryAlertFor maps lateness onto an alert type and severity.
func deliveryAlertFor(daysLate int) (string, string) {
	switch {
	case daysLate > materiallyOverdueDays:
		return AlertPOMateriallyOverdue, SeverityCritical
	case daysLate > 0:
		return AlertPOOverdue, SeverityWarning
	case daysLate == 0:
		return AlertPOExpectedToday, SeverityInfo
	default:
		return AlertPOExpectedSoon, SeverityInfo
	}
}

// SweepDiscrepancyEscalations raises and escalates unresolved discrepancies.
//
// It never mutates the discrepancy itself. A discrepancy is commercial
// evidence with its own decision workflow; ageing is an operational signal
// about it, not a change to it.
func (s *GoodsReceivingService) SweepDiscrepancyEscalations(
	ctx context.Context,
	limit int,
) (int, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}

	// Resolve alerts for discrepancies that have since been decided.
	if _, err := s.db.Exec(ctx,
		`UPDATE vendor_purchasing_alerts a
		    SET status = 'RESOLVED',
		        resolved_at = now(),
		        updated_at = now()
		  WHERE a.status = 'OPEN'
		    AND a.discrepancy_id <> ''
		    AND EXISTS (
		        SELECT 1 FROM vendor_supplier_discrepancies d
		         WHERE d.id = a.discrepancy_id
		           AND d.business_id = a.business_id
		           AND d.status NOT IN ('OPEN','APPROVAL_REQUIRED')
		    )`); err != nil {
		return 0, err
	}

	rows, err := s.db.Query(ctx,
		`SELECT
		    d.id, d.business_id, d.branch_id, d.supplier_id,
		    d.purchase_order_id, d.discrepancy_type, d.status,
		    floor(EXTRACT(EPOCH FROM (now() - d.created_at)) / 3600.0)::bigint
		   FROM vendor_supplier_discrepancies d
		  WHERE d.status IN ('OPEN','APPROVAL_REQUIRED')
		    AND d.created_at <= now() - ($1::int) * interval '1 hour'
		  ORDER BY d.created_at ASC, d.id ASC
		  LIMIT $2`,
		discrepancyStaleHours, limit)
	if err != nil {
		return 0, err
	}

	type candidate struct {
		id, business, branch, supplier, po, kind, status string
		ageHours                                         int64
	}

	pending := make([]candidate, 0, limit)

	for rows.Next() {
		var c candidate
		if err := rows.Scan(
			&c.id, &c.business, &c.branch, &c.supplier,
			&c.po, &c.kind, &c.status, &c.ageHours,
		); err != nil {
			rows.Close()
			return 0, err
		}
		pending = append(pending, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	raised := 0

	for _, c := range pending {
		alertType, severity := AlertDiscrepancyStale, SeverityWarning
		if c.ageHours >= discrepancyEscalateHours {
			alertType, severity = AlertDiscrepancyEscalate, SeverityCritical
		}

		details, _ := json.Marshal(map[string]any{
			"discrepancy_type":   c.kind,
			"discrepancy_status": c.status,
			"age_hours":          c.ageHours,
		})

		created, err := s.raiseAlert(ctx, PurchasingAlert{
			BusinessID:      c.business,
			BranchID:        c.branch,
			AlertType:       alertType,
			Severity:        severity,
			PurchaseOrderID: c.po,
			SupplierID:      c.supplier,
			DiscrepancyID:   c.id,
			DedupeKey:       discrepancyAlertKey(c.id, alertType),
			Details:         details,
		})
		if err != nil {
			return raised, err
		}
		if created {
			raised++
		}
	}

	return raised, nil
}

// raiseAlert inserts one alert, or does nothing if the same window already
// raised it. Returns whether a new alert was actually created.
//
// The uniqueness is enforced by the database, not by a read-then-write check,
// so two workers racing the same sweep cannot both insert.
func (s *GoodsReceivingService) raiseAlert(
	ctx context.Context,
	alert PurchasingAlert,
) (bool, error) {
	if len(alert.Details) == 0 {
		alert.Details = json.RawMessage(`{}`)
	}

	tag, err := s.db.Exec(ctx,
		`INSERT INTO vendor_purchasing_alerts (
		    id, business_id, branch_id, alert_type, severity, status,
		    purchase_order_id, supplier_id, discrepancy_id, dedupe_key, details
		 ) VALUES ($1,$2,$3,$4,$5,'OPEN',$6,$7,$8,$9,$10::jsonb)
		 ON CONFLICT (business_id, dedupe_key) DO NOTHING`,
		"pal_"+uuid.NewString(),
		alert.BusinessID,
		alert.BranchID,
		alert.AlertType,
		alert.Severity,
		alert.PurchaseOrderID,
		alert.SupplierID,
		alert.DiscrepancyID,
		alert.DedupeKey,
		string(alert.Details),
	)
	if err != nil {
		return false, err
	}

	return tag.RowsAffected() == 1, nil
}

// ListPurchasingAlerts returns operational alerts for the caller's scope.
//
// Business-wide alerts (empty branch) are visible to any caller with the
// permission; branch alerts only to callers holding that branch.
func (s *GoodsReceivingService) ListPurchasingAlerts(
	ctx context.Context,
	actor Actor,
	scope Scope,
	status string,
	branchFilter []string,
	limit int,
) ([]PurchasingAlert, error) {
	if s == nil || s.db == nil || !actor.Validate() || !scope.HasAny() {
		return nil, apperrors.Forbidden("Purchasing access is not permitted.", nil)
	}

	branches, restricted, err := resolvePerformanceBranches(scope, branchFilter)
	if err != nil {
		return nil, err
	}

	status = strings.ToUpper(strings.TrimSpace(status))
	switch status {
	case "", AlertStatusOpen, AlertStatusAcknowledged, AlertStatusResolved:
	default:
		return nil, apperrors.Validation(
			"Unknown alert status filter.", nil)
	}

	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.db.Query(ctx,
		`SELECT
		    id, business_id, branch_id, alert_type, severity, status,
		    purchase_order_id, supplier_id, discrepancy_id, dedupe_key,
		    details, created_at, updated_at, acknowledged_at, resolved_at
		   FROM vendor_purchasing_alerts
		  WHERE business_id = $1
		    AND ($2 = '' OR status = $2)
		    AND (NOT $3::boolean OR branch_id = '' OR branch_id = ANY($4::text[]))
		  ORDER BY created_at DESC, id DESC
		  LIMIT $5`,
		actor.VendorID, status, restricted, branches, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]PurchasingAlert, 0, limit)

	for rows.Next() {
		var a PurchasingAlert
		var details []byte
		if err := rows.Scan(
			&a.ID, &a.BusinessID, &a.BranchID, &a.AlertType, &a.Severity, &a.Status,
			&a.PurchaseOrderID, &a.SupplierID, &a.DiscrepancyID, &a.DedupeKey,
			&details, &a.CreatedAt, &a.UpdatedAt, &a.AcknowledgedAt, &a.ResolvedAt,
		); err != nil {
			return nil, err
		}
		a.Details = json.RawMessage(details)
		out = append(out, a)
	}

	return out, rows.Err()
}

// TransitionPurchasingAlert acknowledges or resolves one alert.
func (s *GoodsReceivingService) TransitionPurchasingAlert(
	ctx context.Context,
	actor Actor,
	scope Scope,
	alertID string,
	target string,
) (PurchasingAlert, error) {
	if s == nil || s.db == nil || !actor.Validate() || !scope.HasAny() {
		return PurchasingAlert{}, apperrors.Forbidden(
			"Purchasing access is not permitted.", nil)
	}

	target = strings.ToUpper(strings.TrimSpace(target))
	if target != AlertStatusAcknowledged && target != AlertStatusResolved {
		return PurchasingAlert{}, apperrors.Validation(
			"An alert can only be acknowledged or resolved.", nil)
	}

	alertID = strings.TrimSpace(alertID)
	if alertID == "" {
		return PurchasingAlert{}, notFound("Purchasing alert")
	}

	// Ownership and branch scope are both enforced in the statement, so a
	// foreign alert is refused by the database rather than by a prior read
	// that another request could race.
	var branchID string
	if err := s.db.QueryRow(ctx,
		`SELECT branch_id FROM vendor_purchasing_alerts
		  WHERE id = $1 AND business_id = $2`,
		alertID, actor.VendorID).Scan(&branchID); err != nil {
		return PurchasingAlert{}, notFound("Purchasing alert")
	}

	if branchID != "" && !scope.Covers(branchID) {
		return PurchasingAlert{}, notFound("Purchasing alert")
	}

	if _, err := s.db.Exec(ctx,
		`UPDATE vendor_purchasing_alerts
		    SET status = $3,
		        acknowledged_at = CASE
		            WHEN $3 = 'ACKNOWLEDGED' THEN now() ELSE acknowledged_at END,
		        resolved_at = CASE
		            WHEN $3 = 'RESOLVED' THEN now() ELSE resolved_at END,
		        updated_at = now()
		  WHERE id = $1 AND business_id = $2`,
		alertID, actor.VendorID, target); err != nil {
		return PurchasingAlert{}, err
	}

	alerts, err := s.ListPurchasingAlerts(ctx, actor, scope, "", nil, 200)
	if err != nil {
		return PurchasingAlert{}, err
	}
	for _, a := range alerts {
		if a.ID == alertID {
			return a, nil
		}
	}

	return PurchasingAlert{}, notFound("Purchasing alert")
}
