package models

import (
	"errors"
	"time"
)

const (
	Window7D     = "7D"
	Window30D    = "30D"
	Window90D    = "90D"
	WindowCustom = "CUSTOM"
)

type Window struct {
	Label string
	Start time.Time
	End   time.Time
}

func ResolveWindow(label string, start, end time.Time, now time.Time) (Window, error) {
	now = now.UTC()
	switch label {
	case "", Window30D:
		return Window{Label: Window30D, Start: now.AddDate(0, 0, -30), End: now}, nil
	case Window7D:
		return Window{Label: Window7D, Start: now.AddDate(0, 0, -7), End: now}, nil
	case Window90D:
		return Window{Label: Window90D, Start: now.AddDate(0, 0, -90), End: now}, nil
	case WindowCustom:
		if start.IsZero() || end.IsZero() || !start.Before(end) {
			return Window{}, errors.New("custom window requires start before end")
		}
		if end.Sub(start) > 366*24*time.Hour {
			return Window{}, errors.New("custom window cannot exceed 366 days")
		}
		return Window{Label: WindowCustom, Start: start.UTC(), End: end.UTC()}, nil
	default:
		return Window{}, errors.New("unsupported window")
	}
}

type OperationalReport struct {
	BusinessID                      string            `json:"businessId"`
	BranchID                        string            `json:"branchId,omitempty"`
	WindowStart                     time.Time         `json:"windowStart"`
	WindowEnd                       time.Time         `json:"windowEnd"`
	DataAsOf                        time.Time         `json:"dataAsOf"`
	ReportVersion                   string            `json:"reportVersion"`
	Classification                  string            `json:"classification"`
	Sales                           SalesMetrics      `json:"sales"`
	Inventory                       InventoryMetrics  `json:"inventory"`
	Supplier                        SupplierMetrics   `json:"supplier"`
	Operations                      OperationMetrics  `json:"operations"`
	Fulfilment                      FulfilmentMetrics `json:"fulfilment"`
	Production                      ProductionMetrics `json:"production"`
	RFQ                             RFQMetrics        `json:"rfq"`
	CRM                             CRMMetrics        `json:"crm"`
	Demand                          DemandMetrics     `json:"demand"`
	PriceIntelligence               PriceMetrics      `json:"priceIntelligence"`
	GuaranteeStatus                 string            `json:"orderGuaranteeStatus"`
	UnavailableAuthoritativeMetrics []string          `json:"unavailableAuthoritativeMetrics,omitempty"`
}

type SalesMetrics struct {
	OrderCount         int64   `json:"orderCount"`
	CompletedValueKobo int64   `json:"completedValueKobo"`
	AverageOrderValue  float64 `json:"averageOrderValueKobo"`
	RepeatCustomers    int64   `json:"repeatCustomers"`
}

type InventoryMetrics struct {
	StockOutEvents        int64   `json:"stockOutEvents"`
	InventoryTurnover     float64 `json:"inventoryTurnover"`
	WasteExpiryEvents     int64   `json:"wasteExpiryEvents"`
	PurchaseCostTrendKobo int64   `json:"purchaseCostTrendKobo"`
}

type SupplierMetrics struct {
	OnTimeRate        float64 `json:"onTimeRate"`
	ShortDeliveryRate float64 `json:"shortDeliveryRate"`
	DamageRate        float64 `json:"damageRate"`
}

type OperationMetrics struct {
	AveragePreparationSeconds float64          `json:"averagePreparationSeconds"`
	AveragePickingSeconds     float64          `json:"averagePickingSeconds"`
	AcceptanceRate            float64          `json:"acceptanceRate"`
	VendorCancellationCount   int64            `json:"vendorCancellationCount"`
	CustomerCancellationCount int64            `json:"customerCancellationCount"`
	RefundReasons             map[string]int64 `json:"refundReasons"`
}

type FulfilmentMetrics struct {
	OwnRiderOrders         int64   `json:"ownRiderOrders"`
	PlatformOrders         int64   `json:"platformOrders"`
	OwnRiderCompletionRate float64 `json:"ownRiderCompletionRate"`
	PlatformCompletionRate float64 `json:"platformCompletionRate"`
	OwnRiderOnTimeRate     float64 `json:"ownRiderOnTimeRate"`
	PlatformOnTimeRate     float64 `json:"platformOnTimeRate"`
}

type ProductionMetrics struct {
	CloudKitchenUtilization float64 `json:"cloudKitchenUtilization"`
	PastryBakeryUtilization float64 `json:"pastryBakeryUtilization"`
}

type RFQMetrics struct {
	Received        int64   `json:"received"`
	QuotesSubmitted int64   `json:"quotesSubmitted"`
	Accepted        int64   `json:"accepted"`
	Rejected        int64   `json:"rejected"`
	Expired         int64   `json:"expired"`
	ConversionRate  float64 `json:"conversionRate"`
}

type CRMMetrics struct {
	New       int64  `json:"new"`
	Repeat    int64  `json:"repeat"`
	Lapsed    int64  `json:"lapsed"`
	HighValue int64  `json:"highValue"`
	Policy    string `json:"policyVersion"`
}

type DemandMetrics struct {
	DemandEvents                  int64  `json:"demandEvents"`
	StockOutDemandEvents          int64  `json:"stockOutDemandEvents"`
	LostOpportunityEstimateKobo   int64  `json:"lostOpportunityEstimateKobo"`
	HighDemandLowSupplyCategories int64  `json:"highDemandLowSupplyCategories"`
	CalculationVersion            string `json:"calculationVersion"`
	EstimateLabel                 string `json:"estimateLabel"`
}

type PriceMetrics struct {
	CalculationVersion     string `json:"calculationVersion"`
	MinimumCohort          int64  `json:"minimumCohort"`
	BenchmarksAvailable    int64  `json:"benchmarksAvailable"`
	LowSampleSuppressed    int64  `json:"lowSampleSuppressed"`
	CompetitorIdentityLeak bool   `json:"competitorIdentityLeak"`
}

type ReliabilityScore struct {
	BusinessID       string             `json:"businessId"`
	BranchID         string             `json:"branchId,omitempty"`
	WindowStart      time.Time          `json:"windowStart"`
	WindowEnd        time.Time          `json:"windowEnd"`
	PolicyVersion    string             `json:"policyVersion"`
	Score            *int               `json:"score,omitempty"`
	Status           string             `json:"status"`
	ComponentValues  map[string]float64 `json:"componentValues"`
	ComponentWeights map[string]float64 `json:"componentWeights"`
	SampleSize       int64              `json:"sampleSize"`
	CalculatedAt     time.Time          `json:"calculatedAt"`
}

func Rate(numerator, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
