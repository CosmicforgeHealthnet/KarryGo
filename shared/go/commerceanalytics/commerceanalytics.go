package commerceanalytics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	EventSchemaVersion = 1

	EventOrderCompleted        = "commerce.order.completed"
	EventOrderAccepted         = "commerce.order.accepted"
	EventOrderRejected         = "commerce.order.rejected"
	EventOrderCancelled        = "commerce.order.cancelled"
	EventOrderRefunded         = "commerce.order.refunded"
	EventItemAvailability      = "commerce.item.availability_changed"
	EventInventoryMovement     = "commerce.inventory.movement"
	EventWasteExpiry           = "commerce.inventory.waste_expiry"
	EventPurchaseReceived      = "commerce.purchase.received"
	EventSupplierDelivery      = "commerce.supplier.delivery"
	EventPreparationTiming     = "commerce.preparation.timing"
	EventPickingTiming         = "commerce.picking.timing"
	EventFulfilmentPerformance = "commerce.fulfilment.performance"
	EventProductionCapacity    = "commerce.production.capacity"
	EventRFQStateChanged       = "commerce.rfq.state_changed"
	EventRFQQuoteSubmitted     = "commerce.rfq.quote_submitted"
	EventRFQAccepted           = "commerce.rfq.accepted"
	EventCRMRelationship       = "commerce.crm.relationship"
	EventSearchDemand          = "commerce.search.demand"
	EventSupportOutcome        = "commerce.support.outcome"
	EventSupportCaseResolved   = "commerce.support_case.resolved"
	EventPriceSnapshot         = "commerce.price.snapshot"
	EventGuaranteeEvidence     = "commerce.guarantee.evidence"
)

const (
	Window7D     = "7D"
	Window30D    = "30D"
	Window90D    = "90D"
	WindowCustom = "CUSTOM"
)

const (
	GuaranteeStatusFutureDisabled = "FUTURE_DISABLED"
	ReliabilityPolicyVersion      = "vendor_reliability_v1"
	DemandPolicyVersion           = "demand_stockout_v1"
	CohortPolicyVersion           = "crm_cohort_v1"
	PriceBenchmarkVersion         = "nearby_price_aggregate_v1"
	MinimumBenchmarkCohort        = 5
)

type Envelope struct {
	EventID         string          `json:"eventId"`
	EventType       string          `json:"eventType"`
	SchemaVersion   int             `json:"schemaVersion"`
	OccurredAt      time.Time       `json:"occurredAt"`
	SourceService   string          `json:"sourceService"`
	SourceAggregate string          `json:"sourceAggregate"`
	SourceID        string          `json:"sourceId"`
	SourceVersion   int64           `json:"sourceVersion"`
	BusinessID      string          `json:"businessId,omitempty"`
	VendorID        string          `json:"vendorId,omitempty"`
	BranchID        string          `json:"branchId,omitempty"`
	CorrelationID   string          `json:"correlationId,omitempty"`
	Payload         json.RawMessage `json:"payload"`
}

func ValidateEnvelope(e Envelope) error {
	if strings.TrimSpace(e.EventID) == "" {
		return errors.New("event id is required")
	}
	if !KnownEventType(e.EventType) {
		return errors.New("unsupported event type")
	}
	if e.SchemaVersion != EventSchemaVersion {
		return errors.New("unsupported schema version")
	}
	if e.SourceService == "" || e.SourceAggregate == "" || e.SourceID == "" {
		return errors.New("source identity is required")
	}
	if e.SourceVersion < 1 {
		return errors.New("source version is required")
	}
	if e.BusinessID == "" && e.VendorID == "" {
		return errors.New("business or vendor reference is required")
	}
	if e.OccurredAt.IsZero() {
		return errors.New("occurredAt is required")
	}
	if len(e.Payload) == 0 || !json.Valid(e.Payload) {
		return errors.New("valid payload is required")
	}
	return nil
}

func KnownEventType(eventType string) bool {
	switch eventType {
	case EventOrderCompleted, EventOrderAccepted, EventOrderRejected, EventOrderCancelled,
		EventOrderRefunded, EventItemAvailability, EventInventoryMovement, EventWasteExpiry,
		EventPurchaseReceived, EventSupplierDelivery, EventPreparationTiming, EventPickingTiming,
		EventFulfilmentPerformance, EventProductionCapacity, EventRFQStateChanged,
		EventRFQQuoteSubmitted, EventRFQAccepted, EventCRMRelationship, EventSearchDemand,
		EventSupportOutcome, EventSupportCaseResolved, EventPriceSnapshot, EventGuaranteeEvidence:
		return true
	default:
		return false
	}
}

func Fingerprint(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

type OrderCompletedPayload struct {
	OrderID        string `json:"orderId"`
	CustomerRef    string `json:"customerRef,omitempty"`
	CategoryID     string `json:"categoryId,omitempty"`
	ItemID         string `json:"itemId,omitempty"`
	ItemName       string `json:"itemName,omitempty"`
	Quantity       int64  `json:"quantity,omitempty"`
	OrderValueKobo int64  `json:"orderValueKobo"`
}

type OperationalStatePayload struct {
	OrderID string `json:"orderId"`
	Reason  string `json:"reason,omitempty"`
}

type InventoryMovementPayload struct {
	ItemID       string `json:"itemId"`
	CategoryID   string `json:"categoryId,omitempty"`
	Quantity     int64  `json:"quantity"`
	MovementType string `json:"movementType"`
	Reason       string `json:"reason,omitempty"`
}

type PurchaseReceivedPayload struct {
	ItemID           string `json:"itemId"`
	CategoryID       string `json:"categoryId,omitempty"`
	SupplierID       string `json:"supplierId"`
	OrderedQuantity  int64  `json:"orderedQuantity"`
	AcceptedQuantity int64  `json:"acceptedQuantity"`
	DamagedQuantity  int64  `json:"damagedQuantity"`
	UnitCostKobo     int64  `json:"unitCostKobo"`
	ExpectedAt       string `json:"expectedAt,omitempty"`
	ReceivedAt       string `json:"receivedAt,omitempty"`
	NormalizedUnit   string `json:"normalizedUnit,omitempty"`
}

type DurationPayload struct {
	OrderID         string `json:"orderId"`
	DurationSeconds int64  `json:"durationSeconds"`
	Workflow        string `json:"workflow,omitempty"`
}

type FulfilmentPayload struct {
	OrderID              string `json:"orderId"`
	Mode                 string `json:"mode"`
	OnTimeReadiness      bool   `json:"onTimeReadiness"`
	PickupLatencySeconds int64  `json:"pickupLatencySeconds,omitempty"`
	DeliveryDelaySeconds int64  `json:"deliveryDelaySeconds,omitempty"`
	Completed            bool   `json:"completed"`
	FailureReason        string `json:"failureReason,omitempty"`
}

type ProductionCapacityPayload struct {
	ProductionType string `json:"productionType"`
	AvailableUnits int64  `json:"availableUnits"`
	UsedUnits      int64  `json:"usedUnits"`
}

type RFQPayload struct {
	RFQID string `json:"rfqId"`
	State string `json:"state"`
}

type CRMRelationshipPayload struct {
	CustomerRef  string `json:"customerRef"`
	Cohort       string `json:"cohort"`
	LifetimeKobo int64  `json:"lifetimeKobo,omitempty"`
	LastOrderAt  string `json:"lastOrderAt,omitempty"`
}

type SearchDemandPayload struct {
	SearchID          string `json:"searchId"`
	CategoryID        string `json:"categoryId,omitempty"`
	ItemID            string `json:"itemId,omitempty"`
	Unavailable       bool   `json:"unavailable"`
	OutOfStock        bool   `json:"outOfStock"`
	RequestedQuantity int64  `json:"requestedQuantity,omitempty"`
}

type SupportOutcomePayload struct {
	CaseRef      string `json:"caseRef"`
	Category     string `json:"category"`
	Outcome      string `json:"outcome"`
	VendorFault  bool   `json:"vendorFault"`
	RefundReason string `json:"refundReason,omitempty"`
}

type PriceSnapshotPayload struct {
	ItemID                     string `json:"itemId"`
	CategoryID                 string `json:"categoryId,omitempty"`
	OwnPriceKobo               int64  `json:"ownPriceKobo"`
	NearbyMedianKobo           int64  `json:"nearbyMedianKobo,omitempty"`
	NearbyMinKobo              int64  `json:"nearbyMinKobo,omitempty"`
	NearbyMaxKobo              int64  `json:"nearbyMaxKobo,omitempty"`
	SampleSize                 int64  `json:"sampleSize"`
	ExternalVendorCount        int64  `json:"externalVendorCount"`
	RequestingVendorExcluded   bool   `json:"requestingVendorExcluded"`
	ComparableProductAuthority string `json:"comparableProductAuthority,omitempty"`
	GeoBucketRef               string `json:"geoBucketRef,omitempty"`
	UnitNormalized             bool   `json:"unitNormalized"`
	WindowRef                  string `json:"windowRef,omitempty"`
	ComparableSetRef           string `json:"comparableSetRef,omitempty"`
}

type ReliabilityWeightPolicy struct {
	Version string             `json:"version"`
	Source  string             `json:"source"`
	Weights map[string]float64 `json:"weights"`
}

func CurrentReliabilityWeightPolicy() ReliabilityWeightPolicy {
	return ReliabilityWeightPolicy{
		Version: ReliabilityPolicyVersion,
		Source:  "technical_default_no_approved_business_policy_found",
		Weights: map[string]float64{
			"acceptance":                         0.20,
			"on_time_readiness":                  0.20,
			"vendor_cancellation":                0.20,
			"availability_substitution_accuracy": 0.20,
			"quality_issue_rate":                 0.20,
		},
	}
}

func PriceBenchmarkAllowed(p PriceSnapshotPayload) bool {
	if p.SampleSize < MinimumBenchmarkCohort || p.ExternalVendorCount < MinimumBenchmarkCohort {
		return false
	}
	if !p.RequestingVendorExcluded || !p.UnitNormalized {
		return false
	}
	if strings.TrimSpace(p.ComparableProductAuthority) == "" || strings.TrimSpace(p.GeoBucketRef) == "" || strings.TrimSpace(p.WindowRef) == "" {
		return false
	}
	return true
}

func CoarseBenchmarkRange(minKobo, maxKobo int64) (int64, int64) {
	const bucket = int64(100)
	if minKobo <= 0 || maxKobo <= 0 || maxKobo < minKobo {
		return 0, 0
	}
	low := (minKobo / bucket) * bucket
	high := ((maxKobo + bucket - 1) / bucket) * bucket
	if high == low {
		high += bucket
	}
	return low, high
}

type GuaranteeEvidencePayload struct {
	OrderID               string   `json:"orderId"`
	AcceptedOrder         bool     `json:"acceptedOrder"`
	Prepared              bool     `json:"prepared"`
	ProviderArrived       bool     `json:"providerArrived"`
	PromisedWindowHeld    bool     `json:"promisedWindowHeld"`
	MissingPaidItem       bool     `json:"missingPaidItem"`
	ExcludedCauses        []string `json:"excludedCauses,omitempty"`
	ReliabilityVersionRef string   `json:"reliabilityVersionRef,omitempty"`
}
