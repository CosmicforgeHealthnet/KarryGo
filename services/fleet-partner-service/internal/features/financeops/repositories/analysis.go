package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// AgeingRows reads every open receivable with its days past due as of asOf.
// Branch scope follows the invoice's branch; an empty scope means the whole
// organization.
func (r *Repository) AgeingRows(ctx context.Context, organizationID string, branches []string, asOf time.Time) ([]models.AgeingRow, error) {
	rows, err := r.pool.Query(ctx, `SELECT i.customer_id,re.currency,re.outstanding_amount_minor,($3::date - re.due_at::date)
 FROM fleet_partner_receivable_entries re JOIN fleet_partner_invoices i ON i.id=re.invoice_id AND i.organization_id=re.organization_id
 WHERE re.organization_id=$1 AND re.status IN('OPEN','PARTIALLY_PAID','OVERDUE') AND re.outstanding_amount_minor>0
 AND (COALESCE(cardinality($2::text[]),0)=0 OR i.branch_id=ANY($2::text[]))`, organizationID, branches, asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.AgeingRow{}
	for rows.Next() {
		var v models.AgeingRow
		if err = rows.Scan(&v.CustomerID, &v.Currency, &v.OutstandingMinor, &v.DaysPastDue); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// dimensionSQL returns, per dimension, the revenue, direct-cost and
// allocation fact queries. Each query takes ($1 org, $2 from, $3 to,
// $4 branches) and returns (dimension_id, currency, amount_minor, sources).
func dimensionSQL(dimension string) (revenue, cost []string, allocation string) {
	branchInvoice := `(COALESCE(cardinality($4::text[]),0)=0 OR i.branch_id=ANY($4::text[]))`
	branchExpense := `(COALESCE(cardinality($4::text[]),0)=0 OR e.branch_id=ANY($4::text[]))`
	unallocated := `NOT EXISTS(SELECT 1 FROM fleet_partner_cost_allocations a WHERE a.organization_id=e.organization_id AND a.source_type='EXPENSE' AND a.source_id=e.id)`
	expenseWindow := `e.organization_id=$1 AND e.status IN('APPROVED','POSTING','POSTED') AND COALESCE(e.approved_at,e.created_at)::date BETWEEN $2::date AND $3::date AND ` + branchExpense + ` AND ` + unallocated
	invoiceRevenue := func(col string) string {
		return `SELECT COALESCE(` + col + `,'UNASSIGNED'),i.currency,sum(i.total_minor)::bigint,count(*) FROM fleet_partner_invoices i
 WHERE i.organization_id=$1 AND i.status IN('ISSUED','PARTIALLY_PAID','PAID','OVERDUE','CREDITED') AND i.issued_at::date BETWEEN $2::date AND $3::date AND ` + branchInvoice + ` GROUP BY 1,2`
	}
	creditNotes := func(col string) string {
		return `SELECT COALESCE(` + col + `,'UNASSIGNED'),cn.currency,-sum(cn.amount_minor)::bigint,count(*) FROM fleet_partner_credit_notes cn JOIN fleet_partner_invoices i ON i.id=cn.invoice_id
 WHERE cn.organization_id=$1 AND cn.status<>'REVERSED' AND cn.created_at::date BETWEEN $2::date AND $3::date AND ` + branchInvoice + ` GROUP BY 1,2`
	}
	expenseCost := func(col, join, extra string) string {
		if extra != "" {
			extra += " AND "
		}
		return `SELECT COALESCE(` + col + `,'UNASSIGNED'),e.currency,sum(e.amount_minor)::bigint,count(*) FROM fleet_partner_expenses e ` + join + ` WHERE ` + extra + expenseWindow + ` GROUP BY 1,2`
	}
	payrollCost := func(col string) string {
		return `SELECT COALESCE(` + col + `,'UNASSIGNED'),ci.currency,sum(ci.amount_minor)::bigint,count(*) FROM fleet_partner_provider_compensation_items ci JOIN fleet_partner_payroll_periods p ON p.id=ci.payroll_period_id
 WHERE ci.organization_id=$1 AND ci.status IN('APPROVED','POSTING','POSTED') AND p.period_end BETWEEN $2::date AND $3::date AND (COALESCE(cardinality($4::text[]),0)=0 OR p.branch_id=ANY($4::text[])) GROUP BY 1,2`
	}
	alloc := func(target string) string {
		return `SELECT a.target_id,a.currency,sum(a.amount_minor)::bigint,count(*) FROM fleet_partner_cost_allocations a
 JOIN fleet_partner_expenses e ON e.id=a.source_id AND a.source_type='EXPENSE'
 WHERE a.organization_id=$1 AND a.target_type='` + target + `' AND a.occurred_at::date BETWEEN $2::date AND $3::date AND ` + branchExpense + ` GROUP BY 1,2`
	}
	switch dimension {
	case "CUSTOMER":
		return []string{invoiceRevenue("i.customer_id"), creditNotes("i.customer_id")},
			[]string{expenseCost("po.customer_id", `JOIN fleet_partner_private_orders po ON po.id=e.order_id AND po.organization_id=e.organization_id`, "")}, alloc("CUSTOMER")
	case "BRANCH":
		return []string{invoiceRevenue("i.branch_id"), creditNotes("i.branch_id")},
			[]string{expenseCost("e.branch_id", "", ""), payrollCost("p.branch_id")}, alloc("BRANCH")
	case "PROVIDER":
		return nil, []string{expenseCost("e.provider_membership_id", "", "e.provider_membership_id IS NOT NULL"), payrollCost("ci.membership_id")}, alloc("PROVIDER")
	case "ASSET":
		return nil, []string{expenseCost("e.asset_id", "", "e.asset_id IS NOT NULL")}, alloc("ASSET")
	case "COST_CENTRE":
		return nil, []string{expenseCost("e.cost_centre", "", "")}, alloc("COST_CENTRE")
	}
	return nil, nil, ""
}

// ProfitabilityFacts reads every revenue, direct-cost and allocated-cost
// contribution for a dimension and period.
func (r *Repository) ProfitabilityFacts(ctx context.Context, organizationID, dimension string, branches []string, from, to time.Time) ([]models.ProfitabilityFact, error) {
	revenue, cost, allocation := dimensionSQL(dimension)
	if allocation == "" {
		return nil, fmt.Errorf("unsupported dimension %q", dimension)
	}
	var out []models.ProfitabilityFact
	run := func(kind, sql string) error {
		rows, err := r.pool.Query(ctx, sql, organizationID, from, to, branches)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			f := models.ProfitabilityFact{Kind: kind}
			if err = rows.Scan(&f.DimensionID, &f.Currency, &f.AmountMinor, &f.Sources); err != nil {
				return err
			}
			out = append(out, f)
		}
		return rows.Err()
	}
	for _, q := range revenue {
		if err := run("REVENUE", q); err != nil {
			return nil, err
		}
	}
	for _, q := range cost {
		if err := run("DIRECT_COST", q); err != nil {
			return nil, err
		}
	}
	if err := run("ALLOCATED_COST", allocation); err != nil {
		return nil, err
	}
	return out, nil
}

// InsertProfitabilitySnapshot appends one snapshot line.
func (r *Repository) InsertProfitabilitySnapshot(ctx context.Context, q database.Querier, organizationID, setID, dimension string, from, to time.Time, l models.ProfitabilityLine, actor string) error {
	trace, _ := json.Marshal(map[string]any{"revenueSources": l.RevenueSources, "costSources": l.CostSources, "allocationSources": l.AllocationSources})
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_profitability_snapshots(id,organization_id,snapshot_set_id,dimension_type,dimension_id,period_start,period_end,currency,revenue_minor,direct_cost_minor,allocated_cost_minor,margin_minor,input_trace,model_version,generated_by)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		ids.New("pfs"), organizationID, setID, dimension, l.DimensionID, from, to, l.Currency, l.RevenueMinor, l.DirectCostMinor, l.AllocatedCostMinor, l.MarginMinor, trace, models.ProfitabilityModelVersion, actor)
	return err
}

// ListProfitabilitySnapshots lists snapshot lines, newest first.
func (r *Repository) ListProfitabilitySnapshots(ctx context.Context, organizationID string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(s) FROM (SELECT id,snapshot_set_id AS "snapshotSetId",dimension_type AS "dimensionType",dimension_id AS "dimensionId",period_start AS "periodStart",period_end AS "periodEnd",currency,revenue_minor AS "revenueMinor",direct_cost_minor AS "directCostMinor",allocated_cost_minor AS "allocatedCostMinor",margin_minor AS "marginMinor",input_trace AS "inputTrace",model_version AS "modelVersion",generated_at AS "generatedAt" FROM fleet_partner_profitability_snapshots WHERE organization_id=$1 ORDER BY generated_at DESC,id LIMIT 500) s`, organizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// ExpenseForAllocation locks an expense and returns its amount, currency
// and status.
func (r *Repository) ExpenseForAllocation(ctx context.Context, q database.Querier, organizationID, id string) (amount int64, currency, status, branch string, err error) {
	err = r.q(q).QueryRow(ctx, `SELECT amount_minor,currency,status,COALESCE(branch_id,'') FROM fleet_partner_expenses WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, id).Scan(&amount, &currency, &status, &branch)
	return
}

// AllocationSetFor returns the existing allocation set of a source, if any.
func (r *Repository) AllocationSetFor(ctx context.Context, q database.Querier, organizationID, sourceType, sourceID string) (string, error) {
	var set string
	err := r.q(q).QueryRow(ctx, `SELECT allocation_set_id FROM fleet_partner_cost_allocations WHERE organization_id=$1 AND source_type=$2 AND source_id=$3 LIMIT 1`, organizationID, sourceType, sourceID).Scan(&set)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return set, err
}

// InsertCostAllocation appends one allocation row.
func (r *Repository) InsertCostAllocation(ctx context.Context, q database.Querier, organizationID, setID string, in models.CostAllocationInput, target models.CostAllocationTarget, currency, actor string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_cost_allocations(id,organization_id,allocation_set_id,source_type,source_id,target_type,target_id,amount_minor,currency,method,reason,created_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		ids.New("cal"), organizationID, setID, in.SourceType, in.SourceID, target.TargetType, target.TargetID, target.AmountMinor, currency, in.Method, in.Reason, actor)
	return err
}

// ListCostAllocations lists allocation rows, newest first.
func (r *Repository) ListCostAllocations(ctx context.Context, organizationID string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(a) FROM (SELECT id,allocation_set_id AS "allocationSetId",source_type AS "sourceType",source_id AS "sourceId",target_type AS "targetType",target_id AS "targetId",amount_minor AS "amountMinor",currency,method,reason,created_by AS "createdBy",occurred_at AS "occurredAt" FROM fleet_partner_cost_allocations WHERE organization_id=$1 ORDER BY occurred_at DESC,id LIMIT 500) a`, organizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// AllocationTargetExists checks a target belongs to the organization.
// COST_CENTRE accepts a configured cost centre code.
func (r *Repository) AllocationTargetExists(ctx context.Context, q database.Querier, organizationID, targetType, targetID string) (bool, error) {
	sql := map[string]string{
		"BRANCH":      `SELECT EXISTS(SELECT 1 FROM fleet_partner_branches WHERE organization_id=$1 AND id=$2)`,
		"COST_CENTRE": `SELECT EXISTS(SELECT 1 FROM fleet_partner_cost_centres WHERE organization_id=$1 AND (id=$2 OR code=$2))`,
		"ASSET":       `SELECT EXISTS(SELECT 1 FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2)`,
		"PROVIDER":    `SELECT EXISTS(SELECT 1 FROM fleet_partner_organization_provider_memberships WHERE organization_id=$1 AND id=$2)`,
		"CUSTOMER":    `SELECT EXISTS(SELECT 1 FROM fleet_partner_company_customers WHERE organization_id=$1 AND id=$2)`,
		"ORDER":       `SELECT EXISTS(SELECT 1 FROM fleet_partner_private_orders WHERE organization_id=$1 AND id=$2)`,
	}[targetType]
	if sql == "" {
		return false, nil
	}
	var ok bool
	err := r.q(q).QueryRow(ctx, sql, organizationID, targetID).Scan(&ok)
	return ok, err
}
