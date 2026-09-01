package missionplanning

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Planner struct {
	MaxReadinessSkew      time.Duration
	MaxStops              int
	MaxWeightGrams        int64
	MaxVolumeML           int64
	CapacityPolicyVersion string
}

func NewPlanner() Planner {
	return Planner{MaxReadinessSkew: 45 * time.Minute, MaxStops: 8, MaxWeightGrams: 25000, MaxVolumeML: 120000, CapacityPolicyVersion: "dispatch_motorcycle_capacity_v1"}
}

func NewPlannerWithCapacity(maxWeightGrams, maxVolumeML int64, version string) Planner {
	p := NewPlanner()
	if maxWeightGrams > 0 {
		p.MaxWeightGrams = maxWeightGrams
	}
	if maxVolumeML > 0 {
		p.MaxVolumeML = maxVolumeML
	}
	if strings.TrimSpace(version) != "" {
		p.CapacityPolicyVersion = version
	}
	return p
}

func (p Planner) Build(in PlanRequest) (Plan, error) {
	if strings.TrimSpace(in.SourceRef) == "" || in.SourceVersion < 1 || strings.TrimSpace(in.OperationRef) == "" || len(in.Units) == 0 || len(in.Parcels) == 0 {
		return Plan{}, errors.New("source, operation, units and parcels are required")
	}
	if !strings.EqualFold(in.VehicleClass, "MOTORCYCLE") {
		return Plan{Outcome: OperationalReview, PolicyVersion: PolicyVersion, Reasons: []string{"VEHICLE_CAPACITY_POLICY_UNAVAILABLE"}}, nil
	}
	maxWeight, maxVolume := p.MaxWeightGrams, p.MaxVolumeML
	if in.MaxWeightGrams > 0 && in.MaxWeightGrams < maxWeight {
		maxWeight = in.MaxWeightGrams
	}
	if in.MaxVolumeML > 0 && in.MaxVolumeML < maxVolume {
		maxVolume = in.MaxVolumeML
	}
	if maxWeight <= 0 || maxVolume <= 0 {
		return Plan{}, errors.New("server capacity policy is unavailable")
	}
	in.MaxWeightGrams, in.MaxVolumeML = maxWeight, maxVolume
	units := append([]Unit(nil), in.Units...)
	sort.SliceStable(units, func(i, j int) bool {
		if units[i].Ready.From.Equal(units[j].Ready.From) {
			return units[i].UnitRef < units[j].UnitRef
		}
		return units[i].Ready.From.Before(units[j].Ready.From)
	})
	parcelByRef := map[string]Parcel{}
	for _, parcel := range in.Parcels {
		if parcel.ParcelRef == "" || parcel.UnitRef == "" || parcel.WeightGrams <= 0 || parcel.VolumeML <= 0 || parcelByRef[parcel.ParcelRef].ParcelRef != "" {
			return Plan{}, errors.New("invalid parcel declaration")
		}
		parcelByRef[parcel.ParcelRef] = parcel
	}
	seenUnit := map[string]bool{}
	seenMembership := map[string]bool{}
	for _, unit := range units {
		if unit.UnitRef == "" || unit.VendorRef == "" || unit.BranchRef == "" || unit.OrderRef == "" || seenUnit[unit.UnitRef] || unit.Ready.To.Before(unit.Ready.From) {
			return Plan{}, errors.New("invalid fulfilment unit")
		}
		seenUnit[unit.UnitRef] = true
		for _, ref := range unit.ParcelRefs {
			parcel, ok := parcelByRef[ref]
			if !ok || parcel.UnitRef != unit.UnitRef || seenMembership[ref] {
				return Plan{}, errors.New("parcel membership does not match fulfilment unit")
			}
			seenMembership[ref] = true
		}
	}
	if len(seenMembership) != len(parcelByRef) {
		return Plan{}, errors.New("every parcel must belong to exactly one fulfilment unit")
	}
	var totalWeight, totalVolume int64
	temperatures := map[string]bool{}
	for _, parcel := range in.Parcels {
		for _, handling := range parcel.Handling {
			if strings.EqualFold(handling, "MEDICAL") || strings.EqualFold(handling, "PHARMACEUTICAL") {
				return Plan{Outcome: RejectIncompatible, PolicyVersion: PolicyVersion, Reasons: []string{"REGULATED_GOODS_OUTSIDE_DISPATCH_POLICY"}}, nil
			}
		}
		if parcel.WeightGrams > in.MaxWeightGrams || parcel.VolumeML > in.MaxVolumeML {
			return Plan{Outcome: OperationalReview, PolicyVersion: PolicyVersion, Reasons: []string{"PARCEL_EXCEEDS_VEHICLE_CAPACITY"}}, nil
		}
		totalWeight += parcel.WeightGrams
		totalVolume += parcel.VolumeML
		if parcel.Temperature != "" {
			temperatures[strings.ToUpper(parcel.Temperature)] = true
		}
	}
	if temperatures["FROZEN"] && temperatures["AMBIENT_ONLY"] {
		return Plan{Outcome: RejectIncompatible, PolicyVersion: PolicyVersion, Reasons: []string{"TEMPERATURE_INCOMPATIBLE"}}, nil
	}
	if in.RouteEvidence == nil {
		return Plan{Outcome: OperationalReview, PolicyVersion: PolicyVersion, Reasons: []string{"ROUTE_AUTHORITY_UNAVAILABLE"}}, nil
	}
	if !in.RouteEvidence.Serviceable {
		return Plan{Outcome: RejectIncompatible, PolicyVersion: PolicyVersion, Reasons: []string{"ROUTE_NOT_SERVICEABLE"}, RouteEvidence: in.RouteEvidence}, nil
	}
	if len(units) > p.MaxStops {
		return Plan{Outcome: OperationalReview, PolicyVersion: PolicyVersion, Reasons: []string{"STOP_LIMIT_EXCEEDED"}}, nil
	}
	if units[len(units)-1].Ready.To.Before(units[0].Ready.From) {
		return Plan{Outcome: RejectIncompatible, PolicyVersion: PolicyVersion, Reasons: []string{"READINESS_WINDOWS_INVALID"}}, nil
	}
	if units[0].Ready.From.After(time.Now().UTC().Add(5 * time.Minute)) {
		return Plan{GroupRef: uuid.NewString(), PlanRef: in.OperationRef, Version: 1, Outcome: WaitReadiness, PolicyVersion: PolicyVersion, Reasons: []string{"PICKUPS_NOT_READY"}}, nil
	}
	outcome := Consolidate
	reasons := []string{}
	if units[len(units)-1].Ready.From.Sub(units[0].Ready.From) > p.MaxReadinessSkew {
		outcome = Split
		reasons = append(reasons, "READINESS_SKEW")
	}
	if totalWeight > in.MaxWeightGrams || totalVolume > in.MaxVolumeML {
		outcome = Split
		reasons = append(reasons, "VEHICLE_CAPACITY")
	}
	if strings.EqualFold(in.ServiceClass, "EXPRESS") && len(units) > 1 {
		outcome = Split
		reasons = append(reasons, "EXPRESS_ISOLATION")
	}
	if in.RouteEvidence.Degraded {
		outcome = Split
		reasons = append(reasons, "ROUTE_DEGRADED_CONSERVATIVE_SPLIT")
	}
	plan := Plan{GroupRef: uuid.NewString(), PlanRef: in.OperationRef, Version: 1, Outcome: outcome, PolicyVersion: PolicyVersion, Reasons: reasons,
		CapacityPolicyVersion: p.CapacityPolicyVersion, EffectiveMaxWeightGrams: maxWeight, EffectiveMaxVolumeML: maxVolume, RouteEvidence: in.RouteEvidence}
	groups := [][]Unit{units}
	if outcome == Split {
		groups = splitUnits(units, in, parcelByRef, p.MaxReadinessSkew)
		if len(units) > 1 && len(groups) < 2 {
			return Plan{}, errors.New("split policy did not produce multiple missions")
		}
	}
	for i, group := range groups {
		logicalRef := stableRef("mission", plan.GroupRef, unitRefs(group))
		m := Mission{MissionRef: uuid.NewString(), LogicalMissionRef: logicalRef, Index: i + 1, ManifestRef: stableRef("manifest", plan.GroupRef, logicalRef), ManifestVersion: 1}
		seq := 1
		for _, u := range group {
			m.Stops = append(m.Stops, Stop{StopRef: uuid.NewString(), Sequence: seq, Type: "PICKUP", Location: u.Pickup, Ready: u.Ready, ParcelRefs: append([]string(nil), u.ParcelRefs...)})
			seq++
			m.ParcelRefs = append(m.ParcelRefs, u.ParcelRefs...)
		}
		m.Stops = append(m.Stops, Stop{StopRef: uuid.NewString(), Sequence: seq, Type: "DELIVERY", Location: in.Receiver, ParcelRefs: append([]string(nil), m.ParcelRefs...)})
		plan.Missions = append(plan.Missions, m)
	}
	return plan, nil
}

func BindStableIdentities(plan *Plan, in *PlanRequest) {
	for i := range in.Parcels {
		in.Parcels[i].KarryGoParcelRef = stableRef("KGP", plan.GroupRef, in.Parcels[i].ParcelRef)
		in.Parcels[i].ScannableCode = stableRef("kgp_scan", plan.GroupRef, in.Parcels[i].ParcelRef)
	}
	for i := range plan.Missions {
		refs := append([]string(nil), plan.Missions[i].ParcelRefs...)
		sort.Strings(refs)
		logical := stableRef("mission", plan.GroupRef, strings.Join(refs, ","))
		plan.Missions[i].LogicalMissionRef = logical
		plan.Missions[i].ManifestRef = stableRef("manifest", plan.GroupRef, logical)
	}
	plan.Parcels = append([]Parcel(nil), in.Parcels...)
}

func splitUnits(units []Unit, in PlanRequest, parcels map[string]Parcel, maxSkew time.Duration) [][]Unit {
	var out [][]Unit
	var current []Unit
	var weight, volume int64
	for _, u := range units {
		var uw, uv int64
		for _, ref := range u.ParcelRefs {
			uw += parcels[ref].WeightGrams
			uv += parcels[ref].VolumeML
		}
		if uw > in.MaxWeightGrams || uv > in.MaxVolumeML {
			return [][]Unit{{u}}
		}
		splitForPolicy := len(current) > 0 && (strings.EqualFold(in.ServiceClass, "EXPRESS") || u.Ready.From.Sub(current[0].Ready.From) > maxSkew)
		if len(current) > 0 && (splitForPolicy || weight+uw > in.MaxWeightGrams || volume+uv > in.MaxVolumeML) {
			out = append(out, current)
			current = nil
			weight, volume = 0, 0
		}
		current = append(current, u)
		weight += uw
		volume += uv
	}
	if len(current) > 0 {
		out = append(out, current)
	}
	return out
}

func stableRef(prefix, scope, source string) string {
	sum := sha256.Sum256([]byte(scope + "\x00" + source))
	return prefix + "-" + hex.EncodeToString(sum[:12])
}

func unitRefs(units []Unit) string {
	refs := make([]string, 0, len(units))
	for _, unit := range units {
		refs = append(refs, unit.UnitRef)
	}
	sort.Strings(refs)
	return strings.Join(refs, ",")
}

func (p Plan) Validate() error {
	if !p.Outcome.Valid() {
		return fmt.Errorf("invalid planning outcome")
	}
	return nil
}
func (o Outcome) Valid() bool {
	switch o {
	case Consolidate, Split, WaitReadiness, OperationalReview, RejectIncompatible:
		return true
	}
	return false
}
