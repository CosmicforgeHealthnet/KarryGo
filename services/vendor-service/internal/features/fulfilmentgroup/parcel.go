package fulfilmentgroup

// The parcel: the thing that physically moves, and the only identity in this
// aggregate a human is holding in their hands.
//
// Everything here exists to protect one property. Once parcel P exists, it is
// still P after a replan, after a provider is reassigned, after its mission is
// replaced, after custody transfers to a second provider, and after it comes
// back. Deleting and recreating it would be simpler for the code and a lie to
// every person tracking it.

import (
	"encoding/json"
	"strings"
	"time"
)

// ParcelStatus is the parcel's own projection state.
//
// It is a projection: for a platform-executed parcel the authoritative custody
// facts belong to the logistics service, and this value is derived from the
// evidence it sends rather than typed in by vendor staff.
type ParcelStatus string

const (
	ParcelCreated        ParcelStatus = "CREATED"
	ParcelPacked         ParcelStatus = "PACKED"
	ParcelReady          ParcelStatus = "READY"
	ParcelInTransit      ParcelStatus = "IN_TRANSIT"
	ParcelDelivered      ParcelStatus = "DELIVERED"
	ParcelReturnRequired ParcelStatus = "RETURN_REQUIRED"
	ParcelReturning      ParcelStatus = "RETURNING"
	ParcelReturned       ParcelStatus = "RETURNED"
	ParcelFailed         ParcelStatus = "FAILED"
	ParcelCancelled      ParcelStatus = "CANCELLED"
)

// Valid reports whether the status is known.
func (s ParcelStatus) Valid() bool {
	switch s {
	case ParcelCreated, ParcelPacked, ParcelReady, ParcelInTransit,
		ParcelDelivered, ParcelReturnRequired, ParcelReturning,
		ParcelReturned, ParcelFailed, ParcelCancelled:
		return true
	default:
		return false
	}
}

// InCustody reports whether the parcel has left the vendor's own hands.
//
// This is the line that seals a manifest: after it, what the parcel contained
// is history, and history is not editable from a catalogue screen.
func (s ParcelStatus) InCustody() bool {
	switch s {
	case ParcelCreated, ParcelPacked, ParcelReady, ParcelCancelled:
		return false
	default:
		return true
	}
}

// CustodianType is who is holding the parcel.
type CustodianType string

const (
	CustodianVendor   CustodianType = "VENDOR"
	CustodianPlatform CustodianType = "PLATFORM_PROVIDER"
	CustodianCustomer CustodianType = "CUSTOMER"
	CustodianNone     CustodianType = "NONE"
)

// Valid reports whether the custodian type is known.
func (c CustodianType) Valid() bool {
	switch c {
	case CustodianVendor, CustodianPlatform, CustodianCustomer, CustodianNone:
		return true
	default:
		return false
	}
}

// TemperatureControl is a handling requirement the packing side declares.
type TemperatureControl string

const (
	TemperatureNone    TemperatureControl = "NONE"
	TemperatureChilled TemperatureControl = "CHILLED"
	TemperatureFrozen  TemperatureControl = "FROZEN"
	TemperatureHot     TemperatureControl = "HOT"
)

// Valid reports whether the temperature requirement is known.
func (t TemperatureControl) Valid() bool {
	switch t {
	case TemperatureNone, TemperatureChilled, TemperatureFrozen, TemperatureHot:
		return true
	default:
		return false
	}
}

// ManifestLine is one order line's presence in a parcel.
//
// It stores REFERENCES and the quantity actually packed, not a copy of the
// catalogue record. A parcel manifest is evidence of what was handed over, and
// duplicating product data here would create a second, silently-diverging
// product truth.
type ManifestLine struct {
	OrderLineRef string `json:"order_line_ref"`
	ItemRef      string `json:"item_ref,omitempty"`
	Description  string `json:"description,omitempty"`

	Quantity int64  `json:"quantity"`
	UnitRef  string `json:"unit_ref,omitempty"`

	// MeasuredQuantity carries variable-measure evidence where the domain has
	// it (weighed goods), in the unit named by UnitRef.
	MeasuredQuantity int64 `json:"measured_quantity,omitempty"`
}

// Manifest is what a parcel represented at handover.
type Manifest struct {
	Lines []ManifestLine `json:"lines"`

	// PackageCount is how many physical items are inside this one parcel — a
	// bag holding six sealed containers is one parcel with a package count of
	// six, which is a different fact from six parcels.
	PackageCount int64 `json:"package_count"`

	Notes string `json:"notes,omitempty"`

	// MediaRefs and SpecialHandlingRefs point at evidence owned elsewhere
	// (media-file-service, handling instructions). References, not payloads.
	MediaRefs           []string `json:"media_refs,omitempty"`
	SpecialHandlingRefs []string `json:"special_handling_refs,omitempty"`

	// SealedAt is the instant this content became history. Set from custody
	// evidence, never from the request body.
	SealedAt *time.Time `json:"sealed_at,omitempty"`
}

// Validate reports the first reason a manifest could not be recorded.
func (m Manifest) Validate() error {
	if m.PackageCount < 0 {
		return ErrInvalid
	}
	if len(m.Lines) > maxManifestLines {
		return ErrInvalid
	}
	for _, line := range m.Lines {
		if strings.TrimSpace(line.OrderLineRef) == "" {
			return ErrInvalid
		}
		if line.Quantity < 0 || line.MeasuredQuantity < 0 {
			return ErrInvalid
		}
	}
	return nil
}

// maxManifestLines bounds one parcel's manifest. A parcel with more lines than
// this is not a parcel, and an unbounded JSON document is a denial-of-service
// surface rather than evidence.
const maxManifestLines = 500

// Encode renders the manifest for storage.
func (m Manifest) Encode() (json.RawMessage, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, ErrInvalid
	}
	return raw, nil
}

// DecodeManifest reads a stored manifest.
func DecodeManifest(raw json.RawMessage) Manifest {
	var manifest Manifest
	if len(raw) == 0 {
		return manifest
	}
	// A manifest that will not decode is reported as empty rather than as an
	// error: a read of a delivered parcel must not fail because of one
	// malformed historical row.
	_ = json.Unmarshal(raw, &manifest)
	return manifest
}

// Parcel is a stable, traceable physical package.
type Parcel struct {
	ID string `json:"parcel_id"`

	GroupID       string `json:"fulfilment_group_id"`
	UnitID        string `json:"fulfilment_unit_id"`
	ParentOrderID string `json:"parent_order_id"`
	VendorOrderID string `json:"vendor_order_id"`

	// Source authority. A parcel always resolves to exactly one vendor and one
	// branch, and never to a caller-supplied pair.
	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`

	ParcelRef string `json:"parcel_ref"`
	Sequence  int    `json:"sequence"`

	Status ParcelStatus `json:"status"`

	CustodianType CustodianType `json:"custodian_type"`
	CustodianRef  string        `json:"custodian_ref,omitempty"`

	Manifest        Manifest `json:"manifest"`
	ManifestVersion int64    `json:"manifest_version"`

	WeightGrams int64 `json:"weight_grams"`
	LengthMm    int64 `json:"length_mm,omitempty"`
	WidthMm     int64 `json:"width_mm,omitempty"`
	HeightMm    int64 `json:"height_mm,omitempty"`

	Fragile            bool               `json:"fragile"`
	TemperatureControl TemperatureControl `json:"temperature_control"`

	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Sealed reports whether the manifest is now history.
func (p Parcel) Sealed() bool {
	return p.Manifest.SealedAt != nil || p.Status.InCustody()
}

// ParcelInput is a packing request.
//
// It carries no group, unit ownership, vendor or branch: all four are resolved
// from the unit the parcel is being packed into. IdempotencyKey is what makes
// a retried packing worker safe — the same key returns the same parcel rather
// than producing a second physical box.
type ParcelInput struct {
	IdempotencyKey string

	Manifest Manifest

	WeightGrams int64
	LengthMm    int64
	WidthMm     int64
	HeightMm    int64

	Fragile            bool
	TemperatureControl TemperatureControl
}

// Validate reports the first reason a parcel could not be packed.
func (in ParcelInput) Validate() error {
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		return ErrInvalid
	}
	if in.WeightGrams < 0 || in.LengthMm < 0 || in.WidthMm < 0 || in.HeightMm < 0 {
		return ErrInvalid
	}
	temperature := in.TemperatureControl
	if temperature == "" {
		temperature = TemperatureNone
	}
	if !temperature.Valid() {
		return ErrInvalid
	}
	return in.Manifest.Validate()
}

// Normalized returns the input with defaults applied.
func (in ParcelInput) Normalized() ParcelInput {
	if in.TemperatureControl == "" {
		in.TemperatureControl = TemperatureNone
	}
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	return in
}

// CustodyEventType is a point on the parcel's journey.
type CustodyEventType string

const (
	CustodyCreated         CustodyEventType = "CREATED"
	CustodyPacked          CustodyEventType = "PACKED"
	CustodyReady           CustodyEventType = "READY"
	CustodyHandedOver      CustodyEventType = "HANDED_OVER"
	CustodyTransferred     CustodyEventType = "TRANSFERRED"
	CustodyDelivered       CustodyEventType = "DELIVERED"
	CustodyFailed          CustodyEventType = "FAILED"
	CustodyReturnInitiated CustodyEventType = "RETURN_INITIATED"
	CustodyReturned        CustodyEventType = "RETURNED"
)

// Valid reports whether the custody event type is known.
func (c CustodyEventType) Valid() bool {
	switch c {
	case CustodyCreated, CustodyPacked, CustodyReady, CustodyHandedOver,
		CustodyTransferred, CustodyDelivered, CustodyFailed,
		CustodyReturnInitiated, CustodyReturned:
		return true
	default:
		return false
	}
}

// PreCustody reports whether the event describes movement that has not yet
// left the vendor.
func (c CustodyEventType) PreCustody() bool {
	return c == CustodyCreated || c == CustodyPacked || c == CustodyReady
}

// CustodySource is who reported a custody fact.
//
// It matters because a fact reported by the vendor and the same fact reported
// by the logistics authority carry different weight, and the chain has to say
// which one it was.
type CustodySource string

const (
	SourceVendor   CustodySource = "VENDOR"
	SourceDispatch CustodySource = "DISPATCH"
	SourceHauling  CustodySource = "HAULING"
	SourceTaxi     CustodySource = "TAXI"
	SourcePlatform CustodySource = "PLATFORM"
)

// Valid reports whether the source is known.
func (c CustodySource) Valid() bool {
	switch c {
	case SourceVendor, SourceDispatch, SourceHauling, SourceTaxi, SourcePlatform:
		return true
	default:
		return false
	}
}

// CustodyEvent is one append-only link in a parcel's custody chain.
type CustodyEvent struct {
	ID      string `json:"custody_event_id"`
	GroupID string `json:"-"`

	ParcelID  string           `json:"parcel_id"`
	EventType CustodyEventType `json:"event_type"`

	FromCustodianType CustodianType `json:"from_custodian_type"`
	FromCustodianRef  string        `json:"from_custodian_ref,omitempty"`
	ToCustodianType   CustodianType `json:"to_custodian_type"`
	ToCustodianRef    string        `json:"to_custodian_ref,omitempty"`

	Source        CustodySource `json:"source"`
	SourceEventID string        `json:"source_event_id"`
	SourceVersion int64         `json:"source_version"`

	PlatformMissionRefID string `json:"platform_mission_ref_id,omitempty"`
	VendorMissionID      string `json:"vendor_mission_id,omitempty"`

	HandoverRef string `json:"handover_ref,omitempty"`
	ProofRef    string `json:"proof_ref,omitempty"`
	ReasonCode  string `json:"reason_code,omitempty"`

	OccurredAt time.Time `json:"occurred_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// Validate reports the first reason a custody event could not be recorded.
func (e CustodyEvent) Validate() error {
	switch {
	case strings.TrimSpace(e.ParcelID) == "":
		return ErrInvalid
	case !e.EventType.Valid():
		return ErrInvalid
	case !e.Source.Valid():
		return ErrInvalid
	case strings.TrimSpace(e.SourceEventID) == "":
		return ErrInvalid
	case e.SourceVersion < 0:
		return ErrInvalid
	case !e.ToCustodianType.Valid():
		return ErrInvalid
	case e.FromCustodianType != "" && !e.FromCustodianType.Valid():
		return ErrInvalid
	case e.OccurredAt.IsZero():
		return ErrInvalid
	}
	// Movement is executed by exactly one mission. Neither would leave the
	// event unattributable; both would make it unclear who was carrying it.
	if !e.EventType.PreCustody() {
		hasPlatform := strings.TrimSpace(e.PlatformMissionRefID) != ""
		hasVendor := strings.TrimSpace(e.VendorMissionID) != ""
		if hasPlatform == hasVendor {
			return ErrInvalid
		}
	}
	return nil
}

// ParcelStatusFor maps a custody event onto the parcel's projected status.
//
// A parcel is never moved backwards by this mapping; the caller compares the
// result against the current status before applying it, so a late-arriving
// IN_TRANSIT cannot un-deliver a delivered parcel.
func ParcelStatusFor(event CustodyEventType) ParcelStatus {
	switch event {
	case CustodyCreated:
		return ParcelCreated
	case CustodyPacked:
		return ParcelPacked
	case CustodyReady:
		return ParcelReady
	case CustodyHandedOver, CustodyTransferred:
		return ParcelInTransit
	case CustodyDelivered:
		return ParcelDelivered
	case CustodyFailed:
		return ParcelFailed
	case CustodyReturnInitiated:
		return ParcelReturning
	case CustodyReturned:
		return ParcelReturned
	default:
		return ParcelCreated
	}
}

// parcelProgress ranks parcel statuses so a stale event can be recognised.
//
// Return states rank above delivery attempts deliberately: a return is a later
// fact than the failed delivery that caused it, and must not be overwritten by
// a delayed report of that failure.
func parcelProgress(status ParcelStatus) int {
	switch status {
	case ParcelCreated:
		return 1
	case ParcelPacked:
		return 2
	case ParcelReady:
		return 3
	case ParcelInTransit:
		return 4
	case ParcelFailed:
		return 5
	case ParcelDelivered:
		return 6
	case ParcelReturnRequired:
		return 7
	case ParcelReturning:
		return 8
	case ParcelReturned:
		return 9
	case ParcelCancelled:
		return 10
	default:
		return 0
	}
}

// AdvanceParcelStatus reports whether a projected status may be applied.
//
// Regression is refused rather than errored: an at-least-once pipeline will
// deliver an old event again, and treating that as a failure would turn a
// normal redelivery into an alarm.
func AdvanceParcelStatus(current, next ParcelStatus) bool {
	if !current.Valid() || !next.Valid() {
		return false
	}
	return parcelProgress(next) > parcelProgress(current)
}

// ReturnState is where a return journey stands.
type ReturnState string

const (
	ReturnInitiated ReturnState = "INITIATED"
	ReturnInProcess ReturnState = "IN_RETURN"
	ReturnCompleted ReturnState = "RETURNED"
	ReturnAbandoned ReturnState = "ABANDONED"
)

// Valid reports whether the return state is known.
func (r ReturnState) Valid() bool {
	switch r {
	case ReturnInitiated, ReturnInProcess, ReturnCompleted, ReturnAbandoned:
		return true
	default:
		return false
	}
}

// ParcelReturn is a return journey for one parcel.
//
// The parcel it names is the SAME parcel that went out. A return may require a
// brand-new logistics mission, and that new mission reference is recorded
// here — but the box is not new, and nothing in this type creates a second
// parcel identity to represent it.
type ParcelReturn struct {
	ID       string `json:"parcel_return_id"`
	ParcelID string `json:"parcel_id"`
	GroupID  string `json:"-"`

	ReasonCode string `json:"reason_code"`

	OriginPlatformMissionRefID string `json:"origin_platform_mission_ref_id,omitempty"`
	OriginVendorMissionID      string `json:"origin_vendor_mission_id,omitempty"`
	ReturnPlatformMissionRefID string `json:"return_platform_mission_ref_id,omitempty"`
	ReturnVendorMissionID      string `json:"return_vendor_mission_id,omitempty"`

	State ReturnState `json:"state"`

	Version     int64      `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}
