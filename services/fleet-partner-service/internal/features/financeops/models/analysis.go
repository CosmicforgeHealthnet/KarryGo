package models

import (
	"sort"
	"time"
)

// Section 22/52 analytical finance: receivables ageing, profitability and
// cost allocation. All amounts are integer minor units in one explicit
// currency; a figure is never summed across currencies.

// AgeingBucket is the outstanding receivable amount in one age band.
type AgeingBucket struct {
	Bucket           string `json:"bucket"`
	OutstandingMinor int64  `json:"outstandingMinor"`
	ReceivableCount  int64  `json:"receivableCount"`
}

// CurrencyAgeing is the ageing of one currency's receivables.
type CurrencyAgeing struct {
	Currency         string           `json:"currency"`
	OutstandingMinor int64            `json:"outstandingMinor"`
	Buckets          []AgeingBucket   `json:"buckets"`
	Customers        []CustomerAgeing `json:"customers"`
}

// CustomerAgeing is one customer's outstanding amount per band.
type CustomerAgeing struct {
	CustomerID       string         `json:"customerId"`
	OutstandingMinor int64          `json:"outstandingMinor"`
	Buckets          []AgeingBucket `json:"buckets"`
}

// ReceivablesAgeing is the as-of ageing report.
type ReceivablesAgeing struct {
	AsOf       time.Time        `json:"asOf"`
	Currencies []CurrencyAgeing `json:"currencies"`
}

// AgeingBucketNames are the fixed bands in report order.
var AgeingBucketNames = []string{"CURRENT", "DAYS_1_30", "DAYS_31_60", "DAYS_61_90", "DAYS_OVER_90"}

// AgeingBucketFor places days past due into a band; zero or negative days
// are CURRENT (not yet due).
func AgeingBucketFor(daysPastDue int) string {
	switch {
	case daysPastDue <= 0:
		return "CURRENT"
	case daysPastDue <= 30:
		return "DAYS_1_30"
	case daysPastDue <= 60:
		return "DAYS_31_60"
	case daysPastDue <= 90:
		return "DAYS_61_90"
	}
	return "DAYS_OVER_90"
}

// AgeingRow is one open receivable as the ageing query returns it.
type AgeingRow struct {
	CustomerID, Currency string
	OutstandingMinor     int64
	DaysPastDue          int
}

// BuildAgeing groups open receivables by currency, band and customer.
func BuildAgeing(asOf time.Time, rows []AgeingRow) ReceivablesAgeing {
	type key struct{ currency, customer string }
	byCurrency := map[string]map[string]int64{}
	countByCurrency := map[string]map[string]int64{}
	byCustomer := map[key]map[string]int64{}
	for _, r := range rows {
		b := AgeingBucketFor(r.DaysPastDue)
		if byCurrency[r.Currency] == nil {
			byCurrency[r.Currency], countByCurrency[r.Currency] = map[string]int64{}, map[string]int64{}
		}
		byCurrency[r.Currency][b] += r.OutstandingMinor
		countByCurrency[r.Currency][b]++
		k := key{r.Currency, r.CustomerID}
		if byCustomer[k] == nil {
			byCustomer[k] = map[string]int64{}
		}
		byCustomer[k][b] += r.OutstandingMinor
	}
	out := ReceivablesAgeing{AsOf: asOf.UTC(), Currencies: []CurrencyAgeing{}}
	for _, currency := range sortedKeys(byCurrency) {
		ca := CurrencyAgeing{Currency: currency, Buckets: []AgeingBucket{}, Customers: []CustomerAgeing{}}
		for _, name := range AgeingBucketNames {
			ca.Buckets = append(ca.Buckets, AgeingBucket{Bucket: name, OutstandingMinor: byCurrency[currency][name], ReceivableCount: countByCurrency[currency][name]})
			ca.OutstandingMinor += byCurrency[currency][name]
		}
		var customers []string
		for k := range byCustomer {
			if k.currency == currency {
				customers = append(customers, k.customer)
			}
		}
		sortStrings(customers)
		for _, c := range customers {
			cust := CustomerAgeing{CustomerID: c, Buckets: []AgeingBucket{}}
			for _, name := range AgeingBucketNames {
				v := byCustomer[key{currency, c}][name]
				cust.Buckets = append(cust.Buckets, AgeingBucket{Bucket: name, OutstandingMinor: v})
				cust.OutstandingMinor += v
			}
			ca.Customers = append(ca.Customers, cust)
		}
		out.Currencies = append(out.Currencies, ca)
	}
	return out
}

// ProfitabilityDimensions are the supported analysis dimensions.
var ProfitabilityDimensions = map[string]bool{"CUSTOMER": true, "BRANCH": true, "PROVIDER": true, "ASSET": true, "COST_CENTRE": true}

// ProfitabilityModelVersion identifies the analytical method so a snapshot
// can always be explained against the rules that produced it.
const ProfitabilityModelVersion = "profitability-v1"

// ProfitabilityLine is one dimension value in one currency.
type ProfitabilityLine struct {
	DimensionID        string `json:"dimensionId"`
	Currency           string `json:"currency"`
	RevenueMinor       int64  `json:"revenueMinor"`
	DirectCostMinor    int64  `json:"directCostMinor"`
	AllocatedCostMinor int64  `json:"allocatedCostMinor"`
	MarginMinor        int64  `json:"marginMinor"`
	RevenueSources     int64  `json:"revenueSources"`
	CostSources        int64  `json:"costSources"`
	AllocationSources  int64  `json:"allocationSources"`
}

// ProfitabilityReport is the analysis for one dimension and period.
type ProfitabilityReport struct {
	Dimension     string              `json:"dimension"`
	PeriodStart   string              `json:"periodStart"`
	PeriodEnd     string              `json:"periodEnd"`
	ModelVersion  string              `json:"modelVersion"`
	Method        string              `json:"method"`
	Lines         []ProfitabilityLine `json:"lines"`
	SnapshotSetID string              `json:"snapshotSetId,omitempty"`
}

// ProfitabilityInput selects the dimension and period.
type ProfitabilityInput struct {
	Dimension string `json:"dimension"`
	From      string `json:"from"`
	To        string `json:"to"`
}

// ProfitabilityFact is one revenue, cost or allocation contribution.
type ProfitabilityFact struct {
	Kind, DimensionID, Currency string
	AmountMinor, Sources        int64
}

// BuildProfitability folds facts into lines; margin is revenue minus direct
// and allocated cost, per dimension value and currency.
func BuildProfitability(facts []ProfitabilityFact) []ProfitabilityLine {
	type key struct{ id, currency string }
	lines := map[key]*ProfitabilityLine{}
	var order []key
	for _, f := range facts {
		k := key{f.DimensionID, f.Currency}
		l := lines[k]
		if l == nil {
			l = &ProfitabilityLine{DimensionID: f.DimensionID, Currency: f.Currency}
			lines[k] = l
			order = append(order, k)
		}
		switch f.Kind {
		case "REVENUE":
			l.RevenueMinor += f.AmountMinor
			l.RevenueSources += f.Sources
		case "DIRECT_COST":
			l.DirectCostMinor += f.AmountMinor
			l.CostSources += f.Sources
		case "ALLOCATED_COST":
			l.AllocatedCostMinor += f.AmountMinor
			l.AllocationSources += f.Sources
		}
	}
	out := make([]ProfitabilityLine, 0, len(order))
	for _, k := range order {
		l := lines[k]
		l.MarginMinor = l.RevenueMinor - l.DirectCostMinor - l.AllocatedCostMinor
		out = append(out, *l)
	}
	sortLines(out)
	return out
}

// CostAllocationTarget is one target of an allocation set.
type CostAllocationTarget struct {
	TargetType  string `json:"targetType"`
	TargetID    string `json:"targetId"`
	AmountMinor int64  `json:"amountMinor"`
}

// CostAllocationInput allocates an approved expense across targets.
type CostAllocationInput struct {
	SourceType string                 `json:"sourceType"`
	SourceID   string                 `json:"sourceId"`
	Method     string                 `json:"method"`
	Reason     string                 `json:"reason"`
	Targets    []CostAllocationTarget `json:"targets"`
}

// CostAllocationTargetTypes are the supported targets.
var CostAllocationTargetTypes = map[string]bool{"BRANCH": true, "COST_CENTRE": true, "ASSET": true, "PROVIDER": true, "CUSTOMER": true, "ORDER": true}

// SplitEven splits total minor units across n targets with the remainder on
// the earliest targets, so the parts always sum exactly to the total.
func SplitEven(total int64, n int) []int64 {
	if n <= 0 {
		return nil
	}
	out := make([]int64, n)
	base, rem := total/int64(n), total%int64(n)
	for i := range out {
		out[i] = base
		if int64(i) < rem {
			out[i]++
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortStrings(v []string) { sort.Strings(v) }

func sortLines(v []ProfitabilityLine) {
	sort.Slice(v, func(i, j int) bool {
		if v[i].Currency != v[j].Currency {
			return v[i].Currency < v[j].Currency
		}
		return v[i].DimensionID < v[j].DimensionID
	})
}
