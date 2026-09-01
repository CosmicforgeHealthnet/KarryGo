package deliveryfoundation

import (
	"fmt"
	"sort"
	"strings"
)

type SourceType string

const (
	SourceCustomer       SourceType = "CUSTOMER"
	SourcePartnerAPI     SourceType = "PARTNER_API"
	SourceVendorCommerce SourceType = "VENDOR_COMMERCE"
	SourceFleetPrivate   SourceType = "FLEET_PRIVATE"
	SourceAdminAssisted  SourceType = "ADMIN_ASSISTED"
)

func (s SourceType) Valid() bool {
	switch s {
	case SourceCustomer, SourcePartnerAPI, SourceVendorCommerce, SourceFleetPrivate, SourceAdminAssisted:
		return true
	default:
		return false
	}
}

type Source struct {
	Type           SourceType
	Ref            string
	Version        int64
	BookingID      string
	TripID         string
	PartnerID      string
	OrganizationID string
}

func (s Source) Validate() error {
	if !s.Type.Valid() || strings.TrimSpace(s.Ref) == "" || s.Version < 1 {
		return fmt.Errorf("invalid delivery source")
	}
	return nil
}

type DeliveryGroup struct {
	ID         string
	CustomerID string
	Source     Source
	State      GroupState
	Version    int64
}

func (g DeliveryGroup) Validate() error {
	if strings.TrimSpace(g.ID) == "" || g.Version < 1 || !g.State.Valid() {
		return fmt.Errorf("invalid delivery group")
	}
	if g.Source.Type == SourceCustomer && strings.TrimSpace(g.CustomerID) == "" {
		return fmt.Errorf("customer delivery group requires authenticated customer scope")
	}
	return g.Source.Validate()
}

type DeliveryMission struct {
	ID              string
	DeliveryGroupID string
	Source          Source
	State           MissionState
	Version         int64
}

func (m DeliveryMission) Validate() error {
	if strings.TrimSpace(m.ID) == "" || strings.TrimSpace(m.DeliveryGroupID) == "" || m.Version < 1 || !m.State.Valid() {
		return fmt.Errorf("invalid delivery mission")
	}
	return m.Source.Validate()
}

type StopType string

const (
	StopPickup           StopType = "PICKUP"
	StopProviderTransfer StopType = "PROVIDER_TRANSFER"
	StopDelivery         StopType = "DELIVERY"
	StopHubDrop          StopType = "HUB_DROP"
	StopHubPickup        StopType = "HUB_PICKUP"
	StopReturn           StopType = "RETURN"
)

func (s StopType) Valid() bool {
	switch s {
	case StopPickup, StopProviderTransfer, StopDelivery, StopHubDrop, StopHubPickup, StopReturn:
		return true
	default:
		return false
	}
}

type MissionStop struct {
	ID        string
	MissionID string
	Sequence  int
	Type      StopType
}

func ValidateStopSequence(stops []MissionStop) error {
	if len(stops) == 0 {
		return fmt.Errorf("mission requires at least one stop")
	}
	ordered := append([]MissionStop(nil), stops...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Sequence < ordered[j].Sequence })
	seen := map[string]bool{}
	for index, stop := range ordered {
		if strings.TrimSpace(stop.ID) == "" || strings.TrimSpace(stop.MissionID) == "" || !stop.Type.Valid() || stop.Sequence != index+1 || seen[stop.ID] {
			return fmt.Errorf("invalid mission stop sequence")
		}
		seen[stop.ID] = true
	}
	return nil
}

type Parcel struct {
	ID              string
	DeliveryGroupID string
	State           ParcelState
	Version         int64
}

func (p Parcel) Validate() error {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.DeliveryGroupID) == "" || p.Version < 1 || !p.State.Valid() {
		return fmt.Errorf("invalid parcel")
	}
	return nil
}

type ParcelManifest struct {
	ID         string
	MissionID  string
	Version    int64
	ParcelRefs []string
}

func (m ParcelManifest) Validate() error {
	if strings.TrimSpace(m.ID) == "" || strings.TrimSpace(m.MissionID) == "" || m.Version < 1 || len(m.ParcelRefs) == 0 {
		return fmt.Errorf("invalid parcel manifest")
	}
	seen := map[string]bool{}
	for _, ref := range m.ParcelRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			return fmt.Errorf("invalid parcel manifest membership")
		}
		seen[ref] = true
	}
	return nil
}
