package orders

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// DispatchCommerceEvent is the authenticated, versioned lifecycle envelope
// delivered from Dispatch's transactional outbox.
type DispatchCommerceEvent struct {
	EventID          string          `json:"eventId"`
	EventType        string          `json:"eventType"`
	SchemaVersion    int             `json:"schemaVersion"`
	SourceService    string          `json:"sourceService"`
	AggregateType    string          `json:"aggregateType"`
	AggregateID      string          `json:"aggregateId"`
	AggregateVersion int64           `json:"aggregateVersion"`
	OccurredAt       time.Time       `json:"occurredAt"`
	PayloadHash      string          `json:"payloadHash"`
	Payload          json.RawMessage `json:"payload"`
}

type dispatchCommercePayload struct {
	MissionRef           string     `json:"missionRef"`
	OperationRef         string     `json:"operationRef"`
	MissionSlotRef       string     `json:"missionSlotRef"`
	StopRef              string     `json:"stopRef"`
	State                string     `json:"state"`
	Status               string     `json:"status"`
	QueueState           string     `json:"queueState"`
	ReasonCode           string     `json:"reasonCode"`
	ParcelRefs           []string   `json:"parcelRefs"`
	SourceVersion        int64      `json:"sourceVersion"`
	EvidenceID           string     `json:"evidenceId"`
	EvidenceType         string     `json:"evidenceType"`
	ExceptionID          string     `json:"exceptionId"`
	RecoveryAction       string     `json:"recoveryAction"`
	ReturnID             string     `json:"returnId"`
	TransferRef          string     `json:"transferRef"`
	OldProviderRef       string     `json:"oldProviderRef"`
	NewProviderRef       string     `json:"newProviderRef"`
	ProviderRef          string     `json:"providerRef"`
	PlanVersion          int64      `json:"planVersion"`
	Mode                 string     `json:"mode"`
	PromisedBy           *time.Time `json:"promisedBy"`
	ReadyFrom            *time.Time `json:"readyFrom"`
	ReadyTo              *time.Time `json:"readyTo"`
	RouteProvider        string     `json:"routeProvider"`
	RouteEvidenceVersion string     `json:"routeEvidenceVersion"`
	RouteObservedAt      *time.Time `json:"routeObservedAt"`
	EstimatedSeconds     int64      `json:"estimatedSeconds"`
	PickupStopCount      int        `json:"pickupStopCount"`
	CompletedPickupStops int        `json:"completedPickupStops"`
}

type dispatchCommerceTarget struct {
	ParentOrderID, VendorOrderID, MissionSlotRef string
}

// ApplyDispatchCommerceEvent projects one Dispatch fact. Mission identity is
// resolved from Vendor's reserved operation reference; payload vendor IDs are
// never trusted. Duplicate/stale versions are absorbed by the existing order
// and fulfilment projection guards.
func (s *Service) ApplyDispatchCommerceEvent(ctx context.Context, envelope DispatchCommerceEvent) error {
	if s == nil || s.store == nil || s.store.db == nil || !s.enabled {
		return ErrDependencyMissing
	}
	if envelope.EventID == "" || envelope.SourceService != DispatchServiceCaller ||
		envelope.SchemaVersion != 1 || envelope.AggregateVersion < 1 || envelope.OccurredAt.IsZero() ||
		envelope.AggregateType != "COMMERCE_MISSION" || !validDispatchCommerceEventType(envelope.EventType) {
		return ErrOrderConflict
	}
	if len(envelope.Payload) == 0 || len(envelope.Payload) > 256<<10 ||
		!strings.EqualFold(strings.TrimSpace(envelope.PayloadHash), hashDispatchPayload(envelope.Payload)) {
		return ErrOrderConflict
	}
	var payload dispatchCommercePayload
	if json.Unmarshal(envelope.Payload, &payload) != nil {
		return ErrOrderConflict
	}
	if payload.OperationRef == "" || payload.MissionSlotRef == "" || payload.MissionRef != envelope.AggregateID ||
		payload.SourceVersion != envelope.AggregateVersion {
		return ErrOrderConflict
	}
	targets, err := s.store.dispatchCommerceTargets(ctx, payload.OperationRef, payload.ParcelRefs)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return ErrOrderNotFound
	}
	// Bind the event to Vendor's reserved platform-mission projection before
	// persisting even a read model. An authenticated Dispatch caller still may
	// not create evidence history for an unrelated operation/parcel scope.
	if isDispatchOperationalEvent(envelope.EventType) {
		if err := s.store.projectDispatchOperationalEvent(ctx, envelope, payload); err != nil {
			return err
		}
	}

	eventType, updatesOrder := dispatchOrderEvent(envelope.EventType, payload)
	base := LogisticsOrderEvent{
		MissionRef: payload.MissionSlotRef, Source: LogisticsDispatch,
		EventType: eventType, SourceVersion: envelope.AggregateVersion,
		OccurredAt: envelope.OccurredAt, Payload: envelope.Payload,
		ParcelRefs: payload.ParcelRefs,
	}
	if !updatesOrder {
		base.EventID = envelope.EventID
		base.ParentOrderID = targets[0].ParentOrderID
		if s.fulfilmentProjection != nil {
			return s.fulfilmentProjection.ProjectLogisticsEvent(ctx, base)
		}
		return nil
	}

	for _, target := range targets {
		event := base
		event.EventID = envelope.EventID + ":" + target.VendorOrderID
		event.ParentOrderID = target.ParentOrderID
		event.VendorOrderID = target.VendorOrderID
		if event.EventType == "DELIVERY_FAILED" {
			preserve := strings.EqualFold(payload.State, "RETURN_REQUIRED") || strings.EqualFold(payload.State, "RETURNING")
			event.Payload, _ = json.Marshal(map[string]any{
				"financial_policy": "AMBIGUOUS", "preserve_provider_earning": preserve,
				"dispatch_reason_code": payload.ReasonCode, "parcel_refs": payload.ParcelRefs,
			})
		}
		if _, _, err = s.ApplyLogisticsEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func isDispatchOperationalEvent(eventType string) bool {
	switch eventType {
	case "commerce.evidence.recorded.v1", "commerce.exception.reported.v1",
		"commerce.return.started.v1", "commerce.return.completed.v1",
		"commerce.provider_transfer.started.v1", "commerce.provider_transfer.completed.v1":
		return true
	default:
		return false
	}
}

func (s *Store) projectDispatchOperationalEvent(ctx context.Context, envelope DispatchCommerceEvent, payload dispatchCommercePayload) error {
	tag, err := s.db.Exec(ctx, `INSERT INTO commerce_platform_operational_events(
		id,event_id,canonical_mission_ref,operation_ref,mission_slot_ref,event_type,source_version,
		parcel_refs,evidence_ref,exception_ref,return_ref,transfer_ref,reason_code,payload_hash,payload,occurred_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (event_id) DO NOTHING`, "cpoe_"+envelope.EventID, envelope.EventID, payload.MissionRef,
		payload.OperationRef, payload.MissionSlotRef, envelope.EventType, envelope.AggregateVersion,
		payload.ParcelRefs, payload.EvidenceID, payload.ExceptionID, payload.ReturnID, payload.TransferRef,
		payload.ReasonCode, envelope.PayloadHash, envelope.Payload, envelope.OccurredAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var storedHash string
	if err = s.db.QueryRow(ctx, `SELECT payload_hash FROM commerce_platform_operational_events WHERE event_id=$1`, envelope.EventID).Scan(&storedHash); err != nil {
		return err
	}
	if !strings.EqualFold(storedHash, envelope.PayloadHash) {
		return ErrOrderConflict
	}
	return nil
}

func validDispatchCommerceEventType(eventType string) bool {
	switch eventType {
	case "commerce.mission.created.v1", "commerce.mission.changed.v1",
		"commerce.stop.changed.v1", "commerce.parcel.changed.v1",
		"commerce.evidence.recorded.v1", "commerce.exception.reported.v1",
		"commerce.return.started.v1", "commerce.return.completed.v1",
		"commerce.provider_transfer.started.v1", "commerce.provider_transfer.completed.v1":
		return true
	default:
		return false
	}
}

func dispatchOrderEvent(eventType string, payload dispatchCommercePayload) (string, bool) {
	switch eventType {
	case "commerce.evidence.recorded.v1":
		return "EVIDENCE_" + strings.ToUpper(strings.TrimSpace(payload.EvidenceType)), false
	case "commerce.exception.reported.v1":
		return "OPERATIONAL_EXCEPTION", true
	case "commerce.return.started.v1":
		return "RETURNING", true
	case "commerce.return.completed.v1":
		return "RETURNING", true
	case "commerce.provider_transfer.started.v1":
		return "PROVIDER_REASSIGNED", true
	case "commerce.provider_transfer.completed.v1":
		return "PROVIDER_REASSIGNED", true
	}
	if eventType == "commerce.mission.created.v1" {
		return "MISSION_CREATED", true
	}
	if eventType == "commerce.parcel.changed.v1" {
		return strings.ToUpper(strings.TrimSpace(payload.Status)), false
	}
	if eventType == "commerce.stop.changed.v1" {
		switch strings.ToUpper(strings.TrimSpace(payload.Status)) {
		case "ARRIVED":
			return "PROVIDER_ARRIVED", true
		case "PICKED_UP":
			return "PICKED_UP", true
		case "SKIPPED", "RETURNED":
			return "STOP_COMPLETED", true
		}
	}
	state := strings.ToUpper(strings.TrimSpace(payload.State))
	if state == "" {
		state = strings.ToUpper(strings.TrimSpace(payload.Status))
	}
	switch state {
	case "ASSIGNED":
		return "PROVIDER_ASSIGNED", true
	case "TRANSFER_PENDING", "REPLACEMENT_REQUIRED", "RETRYING":
		return "PROVIDER_REASSIGNED", true
	case "PICKING_UP":
		return "PROVIDER_ARRIVED", true
	case "IN_PROGRESS", "DELIVERING":
		return "DELIVERING", true
	case "COMPLETED", "DELIVERED":
		return "DELIVERED", true
	case "RETURN_REQUIRED", "RETURNING", "RETURNED":
		return "RETURNING", true
	case "FAILED", "CANCELLED", "EXHAUSTED":
		return "DELIVERY_FAILED", true
	default:
		return state, false
	}
}

func (s *Store) dispatchCommerceTargets(ctx context.Context, operationRef string, parcelRefs []string) ([]dispatchCommerceTarget, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT g.parent_order_id,p.vendor_order_id,pm.slot_ref
		  FROM commerce_platform_mission_refs r
		  JOIN commerce_fulfilment_groups g ON g.id=r.group_id
		  JOIN commerce_fulfilment_plan_missions pm ON pm.id=r.plan_mission_id
		  JOIN commerce_fulfilment_plan_mission_parcels mp ON mp.plan_mission_id=pm.id
		  JOIN commerce_fulfilment_parcels p ON p.id=mp.parcel_id
		 WHERE r.mission_ref=$1 AND r.authority='DISPATCH'
		   AND (cardinality($2::text[])=0 OR p.parcel_ref=ANY($2::text[]))
		 ORDER BY p.vendor_order_id`, strings.TrimSpace(operationRef), parcelRefs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []dispatchCommerceTarget
	for rows.Next() {
		var target dispatchCommerceTarget
		if err = rows.Scan(&target.ParentOrderID, &target.VendorOrderID, &target.MissionSlotRef); err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	if err = rows.Err(); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	return targets, nil
}

func hashDispatchPayload(raw json.RawMessage) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}
