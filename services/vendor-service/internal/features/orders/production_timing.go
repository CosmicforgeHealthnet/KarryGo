package orders

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ProductionReadiness is an Orders-owned timing projection. It is explicitly
// separate from VendorOrder status and all commercial amounts.
type ProductionReadiness struct {
	VendorOrderID        string          `json:"vendor_order_id"`
	ParentOrderID        string          `json:"parent_order_id"`
	ProductionJobID      string          `json:"production_job_id"`
	PromisedReadyAt      *time.Time      `json:"promised_ready_at,omitempty"`
	ExpectedWindowStart  *time.Time      `json:"expected_window_start,omitempty"`
	ExpectedWindowEnd    *time.Time      `json:"expected_window_end,omitempty"`
	LogisticsWindowStart *time.Time      `json:"logistics_window_start,omitempty"`
	LogisticsWindowEnd   *time.Time      `json:"logistics_window_end,omitempty"`
	CustomerETAStatus    string          `json:"customer_eta_status"`
	CustomerETAStart     *time.Time      `json:"customer_eta_start,omitempty"`
	CustomerETAEnd       *time.Time      `json:"customer_eta_end,omitempty"`
	TimingRevision       int64           `json:"timing_revision"`
	TimingSourceEvidence json.RawMessage `json:"timing_source_evidence"`
	SourceVersion        int64           `json:"source_version"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

// GetProductionReadiness reads the timing projection for one vendor-owned
// order. Tenant scope is part of the predicate, so foreign rows are not
// enumerable.
func (s *Store) GetProductionReadiness(ctx context.Context, businessID, vendorOrderID string) (ProductionReadiness, error) {
	var out ProductionReadiness
	err := s.db.QueryRow(ctx, `SELECT vendor_order_id,parent_order_id,production_job_id,promised_ready_at,expected_window_start,expected_window_end,logistics_window_start,logistics_window_end,customer_eta_status,customer_eta_start,customer_eta_end,timing_revision,timing_source_evidence,source_version,updated_at FROM commerce_vendor_order_readiness WHERE business_id=$1 AND vendor_order_id=$2`, businessID, vendorOrderID).Scan(&out.VendorOrderID, &out.ParentOrderID, &out.ProductionJobID, &out.PromisedReadyAt, &out.ExpectedWindowStart, &out.ExpectedWindowEnd, &out.LogisticsWindowStart, &out.LogisticsWindowEnd, &out.CustomerETAStatus, &out.CustomerETAStart, &out.CustomerETAEnd, &out.TimingRevision, &out.TimingSourceEvidence, &out.SourceVersion, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProductionReadiness{}, ErrOrderNotFound
	}
	return out, err
}

// ListCustomerETA returns the authoritative timing projection for every
// allocation in one customer-owned order. Missing logistics timing remains
// NOT_AVAILABLE; this method never synthesizes an ETA from a fixed offset.
func (s *Store) ListCustomerETA(ctx context.Context, customerID, parentOrderID string) ([]ProductionReadiness, error) {
	rows, err := s.db.Query(ctx, `SELECT r.vendor_order_id,r.parent_order_id,r.production_job_id,r.promised_ready_at,r.expected_window_start,r.expected_window_end,r.logistics_window_start,r.logistics_window_end,r.customer_eta_status,r.customer_eta_start,r.customer_eta_end,r.timing_revision,r.timing_source_evidence,r.source_version,r.updated_at FROM commerce_vendor_order_readiness r JOIN commerce_vendor_orders v ON v.id=r.vendor_order_id JOIN commerce_parent_orders p ON p.id=r.parent_order_id WHERE p.customer_id=$1 AND r.parent_order_id=$2 ORDER BY r.vendor_order_id`, customerID, parentOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ProductionReadiness{}
	for rows.Next() {
		var item ProductionReadiness
		if err = rows.Scan(&item.VendorOrderID, &item.ParentOrderID, &item.ProductionJobID, &item.PromisedReadyAt, &item.ExpectedWindowStart, &item.ExpectedWindowEnd, &item.LogisticsWindowStart, &item.LogisticsWindowEnd, &item.CustomerETAStatus, &item.CustomerETAStart, &item.CustomerETAEnd, &item.TimingRevision, &item.TimingSourceEvidence, &item.SourceVersion, &item.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
