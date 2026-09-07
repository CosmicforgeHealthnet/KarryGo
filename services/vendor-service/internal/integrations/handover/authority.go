package handover

import (
	"context"
	"errors"
	"reflect"
)

var (
	ErrUnavailable = errors.New("HANDOVER_NOT_AVAILABLE")
	ErrInvalid     = errors.New("HANDOVER_SCOPE_INVALID")
	ErrUnknown     = errors.New("HANDOVER_OUTCOME_UNKNOWN")
	ErrRejected    = errors.New("HANDOVER_REJECTED")
)

// Binding is resolved from the frozen fulfilment mission, never from a caller's
// choice of execution service. It contains references, not challenge state.
type Binding struct {
	Authority       string `json:"authority"`
	MissionRef      string `json:"mission_ref"`
	FulfilmentRef   string `json:"fulfilment_ref"`
	StopRef         string `json:"stop_ref,omitempty"`
	ManifestRef     string `json:"manifest_ref,omitempty"`
	ManifestVersion int64  `json:"manifest_version,omitempty"`
	// Vendor-owned execution bindings are resolved from Vendor's canonical
	// mission and sent only to the external authority; they are not challenge
	// state and contain no credential material.
	BusinessID    string   `json:"business_id,omitempty"`
	BranchID      string   `json:"branch_id,omitempty"`
	ProviderRef   string   `json:"provider_ref,omitempty"`
	ReceiverRef   string   `json:"receiver_ref,omitempty"`
	ReceiverPhone string   `json:"-"`
	ParcelRefs    []string `json:"parcel_refs,omitempty"`
}

// Observation is an allowlisted projection of a canonical response. Never
// store the upstream response body: authorities may expose additional fields.
type Observation struct {
	Binding
	HandoverRef   string `json:"handover_ref"`
	Stage         string `json:"stage"`
	Status        string `json:"status"`
	SourceVersion int64  `json:"source_version"`
}

func (b Binding) Valid() bool {
	return (b.Authority == "DISPATCH" || b.Authority == "HAULING" || b.Authority == "TAXI" || b.Authority == "VENDOR_OWN_DELIVERY") && b.MissionRef != "" && b.FulfilmentRef != ""
}

// Confirmation's credential is deliberately excluded from all generic JSON
// serialization. Only an authority-specific transport may encode it in memory.
type Confirmation struct {
	Binding
	HandoverRef   string
	Method        string
	Credential    string `json:"-"`
	OperationRef  string
	CorrelationID string
}

// Authority has no issue, resend, expiry, attempt or binding mutation methods.
// Provider workflows create challenges; Vendor only reads and relays pickup.
type Authority interface {
	Read(context.Context, Binding, string, string) (Observation, error)
	ConfirmPickup(context.Context, Confirmation) (Observation, error)
}

// Unavailable is the production boundary for an absent approved dependency.
// It cannot manufacture a handover or fall back to local verification.
type Unavailable struct{}

func (Unavailable) Read(context.Context, Binding, string, string) (Observation, error) {
	return Observation{}, ErrUnavailable
}
func (Unavailable) ConfirmPickup(context.Context, Confirmation) (Observation, error) {
	return Observation{}, ErrUnavailable
}

// Router consumes frozen execution identity. Missing capabilities fail closed.
type Router map[string]Authority

func (r Router) Read(ctx context.Context, b Binding, stage, correlation string) (Observation, error) {
	if !b.Valid() || (stage != "PICKUP" && stage != "DELIVERY") {
		return Observation{}, ErrInvalid
	}
	a := r[b.Authority]
	if a == nil {
		return Observation{}, ErrUnavailable
	}
	o, err := a.Read(ctx, b, stage, correlation)
	if err != nil {
		return Observation{}, err
	}
	return validateObservation(o, b, stage)
}

func (r Router) ConfirmPickup(ctx context.Context, c Confirmation) (Observation, error) {
	if !c.Binding.Valid() || c.HandoverRef == "" || c.OperationRef == "" || c.CorrelationID == "" || c.Credential == "" || len(c.Credential) > 4096 || (c.Method != "OTP" && c.Method != "QR") {
		return Observation{}, ErrInvalid
	}
	a := r[c.Authority]
	if a == nil {
		return Observation{}, ErrUnavailable
	}
	o, err := a.ConfirmPickup(ctx, c)
	if errors.Is(err, ErrUnknown) {
		// No automatic second mutation. A cancelled request cannot make a
		// read-back succeed; its durable caller retries observation later.
		o, err = a.Read(ctx, c.Binding, "PICKUP", c.CorrelationID)
		if err != nil || o.HandoverRef != c.HandoverRef || o.Status != "CONSUMED" {
			return Observation{}, ErrUnknown
		}
	}
	if err != nil {
		return Observation{}, err
	}
	if o.HandoverRef != c.HandoverRef {
		return Observation{}, ErrInvalid
	}
	return validateObservation(o, c.Binding, "PICKUP")
}

func validateObservation(o Observation, b Binding, stage string) (Observation, error) {
	if !reflect.DeepEqual(o.Binding, b) || o.Stage != stage || o.HandoverRef == "" || o.SourceVersion < 1 {
		return Observation{}, ErrInvalid
	}
	switch o.Status {
	case "ACTIVE", "CONSUMED", "INVALIDATED", "EXPIRED", "LOCKED":
		return o, nil
	default:
		return Observation{}, ErrInvalid
	}
}
