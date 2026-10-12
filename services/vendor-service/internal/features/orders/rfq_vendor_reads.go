package orders

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

type VendorRFQ struct {
	ID, Status                                                                  string
	Servings                                                                    int64
	RequestedAt, DeadlineAt, ExpiresAt                                          time.Time
	BudgetMinor                                                                 *int64
	Currency                                                                    string
	Version                                                                     int64
	DestinationSnapshot, MenuRequirements, PackagingRequirements, DeliveryTerms map[string]any
	Capacity                                                                    CapacityRequirement
}

const vendorRFQColumns = `id,status,servings,requested_at,deadline_at,expires_at,budget_minor,currency,version,destination_snapshot,menu_requirements,packaging_requirements,delivery_terms,capacity_requirement`
const vendorRFQPredicate = ` WHERE status <> 'DRAFT' AND eligibility_scope @> jsonb_build_object('vendor_ids',jsonb_build_array($1::text))`

func scanVendorRFQ(row pgx.Row) (VendorRFQ, error) {
	var out VendorRFQ
	var destination, menu, packaging, delivery, capacity []byte
	if err := row.Scan(&out.ID, &out.Status, &out.Servings, &out.RequestedAt, &out.DeadlineAt, &out.ExpiresAt, &out.BudgetMinor, &out.Currency, &out.Version, &destination, &menu, &packaging, &delivery, &capacity); err != nil {
		return out, err
	}
	for _, value := range []struct {
		raw    []byte
		target any
	}{{destination, &out.DestinationSnapshot}, {menu, &out.MenuRequirements}, {packaging, &out.PackagingRequirements}, {delivery, &out.DeliveryTerms}, {capacity, &out.Capacity}} {
		if err := json.Unmarshal(value.raw, value.target); err != nil {
			return out, err
		}
	}
	return out, nil
}
func (s *Section21Service) ListVendorRFQs(ctx context.Context, actor Actor, f ScheduledReadFilter) ([]VendorRFQ, error) {
	if _, err := s.scheduledReadScope(ctx, actor, ""); err != nil {
		return nil, err
	}
	f = normalizeScheduledFilter(f)
	rows, err := s.db.Query(ctx, `SELECT `+vendorRFQColumns+` FROM commerce_rfqs`+vendorRFQPredicate+` ORDER BY requested_at,id LIMIT $2 OFFSET $3`, actor.VendorID, f.Limit+1, f.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VendorRFQ{}
	for rows.Next() {
		value, err := scanVendorRFQ(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}
func (s *Section21Service) GetVendorRFQ(ctx context.Context, actor Actor, id string) (VendorRFQ, error) {
	if _, err := s.scheduledReadScope(ctx, actor, ""); err != nil {
		return VendorRFQ{}, err
	}
	out, err := scanVendorRFQ(s.db.QueryRow(ctx, `SELECT `+vendorRFQColumns+` FROM commerce_rfqs`+vendorRFQPredicate+` AND id=$2`, actor.VendorID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrSection21NotFound
	}
	return out, err
}
func (s *Section21Service) ListVendorRFQQuotes(ctx context.Context, actor Actor, id string) ([]RFQQuoteRevision, error) {
	scope, err := s.scheduledReadScope(ctx, actor, "")
	if err != nil {
		return nil, err
	}
	if _, err = s.GetVendorRFQ(ctx, actor, id); err != nil {
		return nil, err
	}
	branches := scope.BranchIDs
	if branches == nil {
		branches = []string{}
	}
	rows, err := s.db.Query(ctx, `SELECT q.id,q.rfq_id,q.vendor_id,q.branch_id,r.id,r.revision,r.price_minor,r.currency,r.snapshot,r.capacity_requirement,r.vendor_terms,r.payment_policy,r.deposit_policy,r.cancellation_policy,r.expires_at,q.status FROM commerce_rfq_quotes q JOIN commerce_rfq_quote_revisions r ON r.quote_id=q.id WHERE q.rfq_id=$1 AND q.vendor_id=$2 AND ($3::boolean OR q.branch_id=ANY($4::text[])) ORDER BY q.id,r.revision DESC`, id, actor.VendorID, scope.Global, branches)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RFQQuoteRevision{}
	for rows.Next() {
		var q RFQQuoteRevision
		var snapshot, capacity, vendor, payment, deposit, cancellation []byte
		if err = rows.Scan(&q.QuoteID, &q.RFQID, &q.VendorID, &q.BranchID, &q.ID, &q.Revision, &q.PriceMinor, &q.Currency, &snapshot, &capacity, &vendor, &payment, &deposit, &cancellation, &q.ExpiresAt, &q.Status); err != nil {
			return nil, err
		}
		for _, value := range []struct {
			raw    []byte
			target any
		}{{snapshot, &q.Snapshot}, {capacity, &q.Capacity}, {vendor, &q.VendorTerms}, {payment, &q.PaymentPolicy}, {deposit, &q.DepositPolicy}, {cancellation, &q.CancellationPolicy}} {
			if err = json.Unmarshal(value.raw, value.target); err != nil {
				return nil, err
			}
		}
		out = append(out, q)
	}
	return out, rows.Err()
}
