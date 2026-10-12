package orders

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkoutquote"
	"time"
)

type SchedulingQuote struct {
	QuoteID, CustomerID, BusinessID, BranchID, Currency string
	QuoteVersion                                        int64
	ExpiresAt                                           time.Time
	ApprovedAt                                          *time.Time
	Address                                             checkoutquote.AddressSnapshot
	Lines                                               []checkoutquote.LineSnapshot
}

func approvedQuoteContainsBranch(q checkoutquote.Quote, vendor, branch string) bool {
	if q.State != checkoutquote.StateApproved || q.ApprovedAt == nil {
		return false
	}
	for _, line := range q.Lines {
		if line.BusinessID == vendor && line.BranchID == branch {
			return true
		}
	}
	return false
}
func schedulingQuote(q checkoutquote.Quote, vendor, branch string) (SchedulingQuote, error) {
	if !approvedQuoteContainsBranch(q, vendor, branch) {
		return SchedulingQuote{}, ErrSection21NotFound
	}
	out := SchedulingQuote{QuoteID: q.ID, CustomerID: q.CustomerID, BusinessID: vendor, BranchID: branch, Currency: q.Currency, QuoteVersion: q.QuoteVersion, ExpiresAt: q.ExpiresAt, ApprovedAt: q.ApprovedAt, Address: q.Address, Lines: []checkoutquote.LineSnapshot{}}
	for _, line := range q.Lines {
		if line.BusinessID == vendor && line.BranchID == branch {
			out.Lines = append(out.Lines, line)
		}
	}
	return out, nil
}
func (s *Section21Service) ListSchedulingQuotes(ctx context.Context, actor Actor, f ScheduledReadFilter) ([]SchedulingQuote, error) {
	f = normalizeScheduledFilter(f)
	scope, err := s.scheduledReadScope(ctx, actor, f.BranchID)
	if err != nil {
		return nil, err
	}
	branches := scope.BranchIDs
	if branches == nil {
		branches = []string{}
	}
	rows, err := s.db.Query(ctx, `SELECT DISTINCT q.id,q.customer_id,l.snapshot->>'branch_id',q.expires_at FROM commerce_quotes q JOIN commerce_quote_approvals a ON a.quote_id=q.id JOIN commerce_quote_lines l ON l.quote_id=q.id WHERE q.expires_at>$5 AND l.snapshot->>'business_id'=$1 AND ($2::boolean OR l.snapshot->>'branch_id'=ANY($3::text[])) AND ($4::text='' OR l.snapshot->>'branch_id'=$4) ORDER BY q.expires_at,q.id,l.snapshot->>'branch_id' LIMIT $6 OFFSET $7`, actor.VendorID, scope.Global, branches, f.BranchID, s.now().UTC(), f.Limit+1, f.Offset)
	if err != nil {
		return nil, err
	}
	type reference struct{ id, customer, branch string }
	refs := []reference{}
	for rows.Next() {
		var r reference
		var expiry time.Time
		if err = rows.Scan(&r.id, &r.customer, &r.branch, &expiry); err != nil {
			rows.Close()
			return nil, err
		}
		refs = append(refs, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []SchedulingQuote{}
	for _, r := range refs {
		q, err := s.GetSchedulingQuote(ctx, actor, r.id, r.branch)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, nil
}
func (s *Section21Service) GetSchedulingQuote(ctx context.Context, actor Actor, id, branch string) (SchedulingQuote, error) {
	if _, err := s.scheduledReadScope(ctx, actor, branch); err != nil {
		return SchedulingQuote{}, err
	}
	if s.checkout == nil {
		return SchedulingQuote{}, ErrSection21CheckoutNeeded
	}
	var customer string
	err := s.db.QueryRow(ctx, `SELECT q.customer_id FROM commerce_quotes q JOIN commerce_quote_approvals a ON a.quote_id=q.id WHERE q.id=$1 AND q.expires_at>$4 AND EXISTS(SELECT 1 FROM commerce_quote_lines l WHERE l.quote_id=q.id AND l.snapshot->>'business_id'=$2 AND l.snapshot->>'branch_id'=$3)`, id, actor.VendorID, branch, s.now().UTC()).Scan(&customer)
	if err != nil {
		return SchedulingQuote{}, ErrSection21NotFound
	}
	q, err := s.checkout.Get(ctx, customer, id)
	if err != nil {
		return SchedulingQuote{}, ErrSection21CheckoutNeeded
	}
	return schedulingQuote(q, actor.VendorID, branch)
}
