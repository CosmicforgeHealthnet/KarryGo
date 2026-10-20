package adminprivacy

// This validates producer-owned lineage over retained canonical Analytics
// events. It is not a report, a scope grant, or a cohort-count estimator.
// Independent source-owner contexts may bind these facts; the reporting
// adapter must also establish complete membership before counting a cohort.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// LineageFact is owner-internal and retains native event identity for binding.
// It is never an HQ response. The subject hash is a deduplication key, not an
// anonymity guarantee. Repeated provider facts share one key across role fields.
type LineageFact struct {
	Authority, ProductCode                                       string
	Owner, EventID, EntityType, EntityID, EventName, PayloadHash string
	NativeVersion                                                int64
	OccurredAt, RecordedAt                                       time.Time
	Report, Field, UnitFamily, SubjectHash, SourceContract       string
}

type LineageReader struct{ Owner string }

type lineageReport struct{ kinds []networkanalytics.Kind }

// Every legacy report is named explicitly, so a caller cannot introduce an
// arbitrary metric, event predicate, SQL fragment or source table.
var lineageReports = map[string]lineageReport{
	"mission-success":       {[]networkanalytics.Kind{networkanalytics.KindMissionCompleted, networkanalytics.KindMissionFailed, networkanalytics.KindMissionCancelled, networkanalytics.KindGuaranteeBreached}},
	"recovery":              {[]networkanalytics.Kind{networkanalytics.KindRecoveryStarted, networkanalytics.KindRecoveryReplaced, networkanalytics.KindRecoveryResolved, networkanalytics.KindRecoveryFailed}},
	"capability-mismatches": {[]networkanalytics.Kind{networkanalytics.KindCapabilityMismatch}},
	"declines":              {[]networkanalytics.Kind{networkanalytics.KindProviderDeclined}},
	"demand":                {[]networkanalytics.Kind{networkanalytics.KindDemandObserved, networkanalytics.KindDemandBucket}},
	"provider-quality":      {[]networkanalytics.Kind{networkanalytics.KindMissionCompleted, networkanalytics.KindMissionFailed, networkanalytics.KindMissionCancelled, networkanalytics.KindProviderArrived, networkanalytics.KindProviderDeclined, networkanalytics.KindRecoveryStarted, networkanalytics.KindRecoveryReplaced}},
	"readiness":             {[]networkanalytics.Kind{networkanalytics.KindBranchReadiness}},
	"supply":                {[]networkanalytics.Kind{networkanalytics.KindSupplySnapshot}},
	"mission-outcomes":      {[]networkanalytics.Kind{networkanalytics.KindMissionCommitted, networkanalytics.KindMissionAssigned, networkanalytics.KindMissionCompleted, networkanalytics.KindMissionFailed, networkanalytics.KindMissionCancelled, networkanalytics.KindRecoveryResolved, networkanalytics.KindRecoveryFailed, networkanalytics.KindGuaranteeBreached}},
	"demand-forecast":       {[]networkanalytics.Kind{networkanalytics.KindDemandObserved, networkanalytics.KindDemandBucket}},
}

func canonicalLineageOwner(owner string) (string, string) {
	switch owner {
	case "driver-taxi-service", "taxi-service":
		return "driver-taxi-service", networkanalytics.AuthorityTaxi
	case "driver-dispatch-delivery-service", "dispatch-delivery-service":
		return "driver-dispatch-delivery-service", networkanalytics.AuthorityDispatch
	case "driver-hauling-service", "hauling-service":
		return "driver-hauling-service", networkanalytics.AuthorityHauling
	case "vendor-service":
		return owner, networkanalytics.AuthorityVendor
	}
	return "", ""
}

// Kind binds a report AND the proposed native unit field. Neither the report
// name nor a source approval establishes that this is the approved privacy
// unit: governance must approve the exact contract and complete cohort source.
func LineageKind(report, field string) (string, error) {
	if _, ok := lineageReports[report]; !ok {
		return "", ErrUnmapped
	}
	switch field {
	case "provider_id", "original_provider_id", "replacement_provider_id", "branch_id", "source_entity_id":
	default:
		return "", ErrUnmapped
	}
	return report + ":" + field, nil
}

func (r LineageReader) ReadUnit(ctx context.Context, tx pgx.Tx, report, field, eventID string) (LineageFact, error) {
	var out LineageFact
	kind, err := LineageKind(report, field)
	expected, expectedAuthority := canonicalLineageOwner(r.Owner)
	if err != nil || expected == "" || r.Owner != expected || !adminread.Opaque(eventID) {
		return out, ErrUnmapped
	}
	var payload []byte
	var source, storedKind, authority string
	err = tx.QueryRow(ctx, `SELECT event_id,event_name,source_service,source_entity_type,source_entity_id,source_version,occurred_at,ingested_at,payload,payload_hash,kind,authority FROM analytics_network_events WHERE event_id=$1 AND status='PROCESSED' AND octet_length(payload::text)<=1048576`, eventID).Scan(&out.EventID, &out.EventName, &source, &out.EntityType, &out.EntityID, &out.NativeVersion, &out.OccurredAt, &out.RecordedAt, &payload, &out.PayloadHash, &storedKind, &authority)
	if err == pgx.ErrNoRows {
		return out, ErrUnmapped
	}
	if err != nil {
		return out, err
	}
	nativeOwner, _ := canonicalLineageOwner(source)
	if nativeOwner != expected || out.NativeVersion < 1 || out.OccurredAt.IsZero() || out.RecordedAt.IsZero() {
		return LineageFact{}, ErrUnmapped
	}
	var values map[string]any
	if json.Unmarshal(payload, &values) != nil {
		return LineageFact{}, ErrSource
	}
	normalized, err := json.Marshal(values)
	if err != nil {
		return LineageFact{}, ErrSource
	}
	sum := sha256.Sum256(normalized)
	if hex.EncodeToString(sum[:]) != out.PayloadHash {
		return LineageFact{}, ErrSource
	}
	hint, _ := values["authority"].(string)
	actualAuthority, actualKind, _ := networkanalytics.Classify(out.EventName, hint)
	if actualAuthority != expectedAuthority || authority != actualAuthority || storedKind != string(actualKind) {
		return LineageFact{}, ErrUnmapped
	}
	permitted := false
	for _, k := range lineageReports[report].kinds {
		permitted = permitted || k == actualKind
	}
	if !permitted {
		return LineageFact{}, ErrUnmapped
	}

	// Vendor readiness is emitted from the native vendor_order outbox transaction,
	// not a caller-created branch snapshot. Business IDs remain analytical facts;
	// they are not canonical organization/tenant authorization here.
	if actualKind == networkanalytics.KindBranchReadiness {
		order, err := uuid.Parse(out.EntityID)
		if out.EntityType != "vendor_order" || err != nil || order == uuid.Nil || order.String() != out.EntityID {
			return LineageFact{}, ErrUnmapped
		}
	}
	// Counts in a supply snapshot or demand bucket carry no individual roster.
	// Even an incidental provider_id cannot prove membership of that aggregate.
	if actualKind == networkanalytics.KindSupplySnapshot || actualKind == networkanalytics.KindDemandBucket {
		return LineageFact{}, ErrUnmapped
	}
	subject, family := "", ""
	switch field {
	case "provider_id", "original_provider_id", "replacement_provider_id":
		subject, _ = values[field].(string)
		family = "PROVIDER"
	case "branch_id":
		subject, _ = values[field].(string)
		family = "BRANCH"
	case "source_entity_id":
		switch out.EntityType {
		case "booking", "taxi_booking", "taxi_goods_mission", "haulage_booking", "trip", "dispatch_booking", "delivery_mission":
			family = "SOURCE_ENTITY." + out.EntityType

		case "vendor_order":
			if report != "readiness" || expected != "vendor-service" {
				return LineageFact{}, ErrUnmapped
			}
			family = "SOURCE_ENTITY.vendor_order"
		case "recovery_case", "delivery_recovery_action":
			family = "SOURCE_ENTITY." + out.EntityType
		default:
			return LineageFact{}, ErrUnmapped
		}
		subject = out.EntityID
	}
	// Provider and branch primary keys are canonical UUIDs in these owners.
	// Equivalent UUID encodings must not inflate the distinct-subject count.
	if family == "PROVIDER" || family == "BRANCH" {
		id, err := uuid.Parse(subject)
		if err != nil || id == uuid.Nil {
			return LineageFact{}, ErrUnmapped
		}
		subject = id.String()
	}
	if !adminread.Opaque(subject) {
		return LineageFact{}, ErrUnmapped
	}
	out.OccurredAt = out.OccurredAt.UTC()
	out.RecordedAt = out.RecordedAt.UTC()
	out.Authority = actualAuthority
	out.ProductCode = networkanalytics.ProductFor(out.EventName, out.EntityType, values)
	out.Owner = expected
	out.Report = report
	out.Field = field
	out.UnitFamily = family
	digest := sha256.Sum256([]byte(expected + "\x00" + family + "\x00" + subject))
	out.SubjectHash = hex.EncodeToString(digest[:])
	out.SourceContract = "analytics.cohort-lineage.v1:" + kind
	return out, nil
}

// Binding deliberately resolves no geographic/tenant authority from the event's
// analytical zone (which may be GPS-derived). Such dimensions stay unknown
// until independently allocated by the affected source owner and approved by
// governance. The exact native event and unit hash are covered by the signature.
func (r LineageReader) Binding(ctx context.Context, tx pgx.Tx, kind, ref string) (admincontext.RecordBinding, error) {
	report, field, ok := strings.Cut(kind, ":")
	if !ok {
		return admincontext.RecordBinding{}, admincontext.ErrUnmapped
	}
	fact, err := r.ReadUnit(ctx, tx, report, field, ref)
	if err != nil {
		if err == ErrUnmapped {
			return admincontext.RecordBinding{}, admincontext.ErrUnmapped
		}
		return admincontext.RecordBinding{}, err
	}
	return admincontext.RecordBinding{Fingerprint: admincontext.Fingerprint(fact), SourceContract: fact.SourceContract, Canonical: adminpolicy.Scope{Product: "karryam"}}, nil
}
