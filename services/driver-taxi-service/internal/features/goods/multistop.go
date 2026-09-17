package goods

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/services/taxi-service/internal/capability"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/custodyops"
	"cosmicforge/logistics/shared/go/geo"
)

// PolicySource returns the locally applied, immutable service-product policy.
type PolicySource interface {
	CurrentPolicy(ctx context.Context) (capability.Policy, error)
}

// Capabilities gates the Section 7 Taxi Goods capabilities independently.
type ExecutionFlags struct {
	CapabilityEnabled     bool // TAXI_GOODS_CAPABILITY_ENABLED
	MultiPickupEnabled    bool // TAXI_GOODS_MULTI_PICKUP_ENABLED
	SecureHandoverEnabled bool // TAXI_GOODS_SECURE_HANDOVER_ENABLED
	CumulativeFitEnabled  bool // TAXI_GOODS_CUMULATIVE_FIT_ENABLED
	RecoveryEnabled       bool // TAXI_RECOVERY_ENABLED
}

// MultiPickupOperational reports whether multi-pickup execution is fully
// active. Capability responses only advertise multi-pickup when it is.
func (c ExecutionFlags) MultiPickupOperational() bool {
	return c.CapabilityEnabled && c.MultiPickupEnabled && c.SecureHandoverEnabled && c.CumulativeFitEnabled
}

// JITLead is how far before the earliest pickup readiness Taxi starts
// looking for a provider, so providers are not assigned to wait idle.
const JITLead = 10 * time.Minute

// BuildMultiStopCommit validates a multi-pickup commit against policy and
// derives ordered stops, per-stop cargo and the per-stop manifest custody
// ledger. It is pure: no provider, vehicle or fee is decided here.
func BuildMultiStopCommit(request commercemission.Request, policy capability.Policy, caps ExecutionFlags) (MultiStopCommit, string, error) {
	if !caps.MultiPickupOperational() {
		return MultiStopCommit{}, capability.ReasonMultiPickupUnsupported, ErrDisabled
	}
	if request.Plan == nil || strings.TrimSpace(request.Plan.PlanRef) == "" || request.Plan.PlanVersion < 1 || strings.TrimSpace(request.Plan.MissionSlotRef) == "" {
		return MultiStopCommit{}, capability.ReasonRouteConstraintFailed, ErrInvalid
	}
	if len(request.PickupStops) < 2 || len(request.Parcels) == 0 {
		return MultiStopCommit{}, capability.ReasonRouteConstraintFailed, ErrInvalid
	}
	stopRefs := map[string]commercemission.PickupStop{}
	var stops []MissionStop
	for i, stop := range request.PickupStops {
		ref := strings.TrimSpace(stop.StopRef)
		if ref == "" || stopRefs[ref].StopRef != "" {
			return MultiStopCommit{}, capability.ReasonRouteConstraintFailed, ErrInvalid
		}
		stopRefs[ref] = stop
		ms := MissionStop{Sequence: i + 1, StopType: custodyops.StopPickup, ExternalStopRef: ref, VendorRef: stop.VendorRef, VendorBranchRef: stop.BranchRef,
			Latitude: stop.Location.Latitude, Longitude: stop.Location.Longitude, SecureHandover: true, Status: "PLANNED"}
		if !stop.ReadyWindow.From.IsZero() {
			from := stop.ReadyWindow.From.UTC()
			ms.ReadinessWindowStart = &from
		}
		if !stop.ReadyWindow.To.IsZero() {
			to := stop.ReadyWindow.To.UTC()
			ms.ReadinessWindowEnd = &to
		}
		stops = append(stops, ms)
	}
	dropoffRef := "dropoff"
	if ref := strings.TrimSpace(request.Dropoff.Ref); ref != "" {
		dropoffRef = ref
	}
	if stopRefs[dropoffRef].StopRef != "" {
		return MultiStopCommit{}, capability.ReasonRouteConstraintFailed, ErrInvalid
	}
	stops = append(stops, MissionStop{Sequence: len(stops) + 1, StopType: custodyops.StopDropoff, ExternalStopRef: dropoffRef,
		Latitude: request.Dropoff.Latitude, Longitude: request.Dropoff.Longitude, SecureHandover: true, Status: "PLANNED"})

	cargoByStop := map[string]*capability.StopCargo{}
	manifests := map[string]*custodyops.ManifestInput{}
	var manifestOrder []string
	for _, parcel := range request.Parcels {
		stopRef := strings.TrimSpace(parcel.PickupStopRef)
		if stopRefs[stopRef].StopRef == "" || strings.TrimSpace(parcel.ManifestRef) == "" || parcel.ManifestVersion < 1 {
			return MultiStopCommit{}, capability.ReasonRouteConstraintFailed, ErrInvalid
		}
		count := max64(parcel.PackageCount, 1)
		cargo := cargoByStop[stopRef]
		if cargo == nil {
			cargo = &capability.StopCargo{StopRef: stopRef}
			cargoByStop[stopRef] = cargo
		}
		cargo.WeightGrams += parcel.WeightGrams
		cargo.VolumeML += parcel.LengthMm * parcel.WidthMm * parcel.HeightMm / 1000
		cargo.PackageCount += count
		cargo.MaxLengthMM, cargo.MaxWidthMM, cargo.MaxHeightMM = max64(cargo.MaxLengthMM, parcel.LengthMm), max64(cargo.MaxWidthMM, parcel.WidthMm), max64(cargo.MaxHeightMM, parcel.HeightMm)
		handling := mergeHandling(request.Handling, parcel.Handling, stopRefs[stopRef].Handling)
		cargo.Fragile = cargo.Fragile || handling.Fragile
		cargo.UprightRequired = cargo.UprightRequired || handling.Upright
		cargo.SecureCargoRequired = cargo.SecureCargoRequired || handling.HighValue
		if class := temperatureClass(handling); class != capability.TemperatureAmbient {
			switch {
			case cargo.TemperatureClass == "" || cargo.TemperatureClass == capability.TemperatureAmbient:
				cargo.TemperatureClass = class
			case cargo.TemperatureClass != class && !contains(cargo.MixedTemperatureClasses, class):
				cargo.MixedTemperatureClasses = append(cargo.MixedTemperatureClasses, class)
			}
		}
		key := stopRef + "|" + strings.TrimSpace(parcel.ManifestRef) + "|" + fmt.Sprint(parcel.ManifestVersion)
		manifest := manifests[key]
		if manifest == nil {
			manifest = &custodyops.ManifestInput{ManifestRef: strings.TrimSpace(parcel.ManifestRef), ManifestVersion: parcel.ManifestVersion, PickupStopRef: stopRef,
				SourceAuthority: commercemission.SignatureCaller, VendorRef: parcel.VendorRef, VendorBranchRef: parcel.BranchRef}
			manifests[key] = manifest
			manifestOrder = append(manifestOrder, key)
		}
		manifest.Packages = append(manifest.Packages, custodyops.PackageInput{PackageRef: strings.TrimSpace(parcel.ParcelRef), PackageCount: count,
			WeightGrams: parcel.WeightGrams, VolumeML: parcel.LengthMm * parcel.WidthMm * parcel.HeightMm / 1000})
		manifest.SourceManifestChecksum = checksumParts(manifest.SourceManifestChecksum, parcel)
	}
	var cargo []capability.StopCargo
	keys := make([]string, 0, len(cargoByStop))
	for key := range cargoByStop {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		cargo = append(cargo, *cargoByStop[key])
	}
	for ref := range stopRefs {
		if cargoByStop[ref] == nil {
			// Every pickup stop must hand over at least one package.
			return MultiStopCommit{}, capability.ReasonRouteConstraintFailed, ErrInvalid
		}
	}
	mission := capability.GoodsMission{Stops: cargo, Dropoffs: 1}
	distinct := map[string]bool{}
	for _, c := range cargo {
		distinct[c.StopRef] = true
	}
	product, reason := selectGoodsProduct(policy, request, mission, len(distinct), caps)
	if reason != capability.ReasonEligible {
		return MultiStopCommit{}, reason, ErrConflict
	}
	var manifestInputs []custodyops.ManifestInput
	for _, key := range manifestOrder {
		manifestInputs = append(manifestInputs, *manifests[key])
	}
	custody := custodyops.RegisterMissionCommand{
		OperationRef: request.OperationReference(), CorrelationID: request.ParentOrderRef, MissionRef: request.MissionRef,
		TenantRef: request.ParentOrderRef, ReceiverChallengeRef: "delivery:" + request.MissionRef, SourceVersion: 1,
		Manifests: manifestInputs,
	}
	for _, stop := range stops {
		custody.Stops = append(custody.Stops, custodyops.StopInput{StopRef: stop.ExternalStopRef, StopType: stop.StopType, Sequence: stop.Sequence})
	}
	if err := custody.Normalize(); err != nil {
		return MultiStopCommit{}, capability.ReasonRouteConstraintFailed, ErrInvalid
	}
	return MultiStopCommit{Request: request, Product: product, PolicyVersion: policy.Version, Stops: stops, Cargo: cargo, Custody: custody}, capability.ReasonEligible, nil
}

func selectGoodsProduct(policy capability.Policy, request commercemission.Request, mission capability.GoodsMission, pickups int, caps ExecutionFlags) (capability.Product, string) {
	mission.Stops = uniqueStopMission(mission.Stops, pickups)
	best := capability.ReasonProductNotEligible
	for _, product := range policy.ActiveProducts(capability.PurposeGoods, "") {
		candidate := mission
		candidate.ProductCode = product.Code
		decision := capability.EvaluateGoodsPolicy(candidate, product, caps.MultiPickupEnabled)
		if decision.Eligible {
			return product, capability.ReasonEligible
		}
		best = decision.Reason
	}
	return capability.Product{}, best
}

// uniqueStopMission collapses same-stop cargo entries so the pickup count
// seen by policy is the number of physical stops.
func uniqueStopMission(cargo []capability.StopCargo, pickups int) []capability.StopCargo {
	seen := map[string]bool{}
	out := make([]capability.StopCargo, 0, pickups)
	for _, c := range cargo {
		if !seen[c.StopRef] {
			seen[c.StopRef] = true
			out = append(out, c)
		}
	}
	return out
}

func mergeHandling(values ...commercemission.Handling) commercemission.Handling {
	var out commercemission.Handling
	for _, h := range values {
		out.Fragile = out.Fragile || h.Fragile
		out.Upright = out.Upright || h.Upright
		out.Hot = out.Hot || h.Hot
		out.Chilled = out.Chilled || h.Chilled
		out.Frozen = out.Frozen || h.Frozen
		out.HighValue = out.HighValue || h.HighValue
	}
	return out
}

func temperatureClass(h commercemission.Handling) string {
	switch {
	case h.Frozen:
		return capability.TemperatureFrozen
	case h.Chilled:
		return capability.TemperatureChilled
	case h.Hot:
		return capability.TemperatureHot
	}
	return capability.TemperatureAmbient
}

func checksumParts(previous string, parcel commercemission.Parcel) string {
	sum := sha256.Sum256([]byte(previous + "|" + parcel.ParcelRef + "|" + string(parcel.Manifest) + fmt.Sprintf("|%d|%d", parcel.WeightGrams, parcel.PackageCount)))
	return hex.EncodeToString(sum[:])
}

// PreviewPickup is one proposed pickup of a pre-checkout cluster.
type PreviewPickup struct {
	StopRef     string                    `json:"stop_ref"`
	Location    commercequote.Point       `json:"location"`
	ReadyWindow commercequote.ReadyWindow `json:"ready_window,omitempty"`
	Cargo       capability.StopCargo      `json:"cargo"`
}

// ServiceabilityRequest is the side-effect-free pre-checkout question "can
// Taxi execute this cluster?". It deliberately has no provider, vehicle or
// fleet member field; unknown fields are rejected at the HTTP boundary.
type ServiceabilityRequest struct {
	PreviewRef    string              `json:"preview_ref"`
	Purpose       string              `json:"purpose"`
	ProductCode   string              `json:"product_code,omitempty"`
	CartVersion   int64               `json:"cart_version,omitempty"`
	Pickups       []PreviewPickup     `json:"pickups"`
	Dropoff       commercequote.Point `json:"dropoff"`
	CorrelationID string              `json:"correlation_id,omitempty"`
}

// ServiceabilityResponse carries serviceability evidence only: no fee, no
// provider identity, no reservation.
type ServiceabilityResponse struct {
	PreviewRef           string    `json:"preview_ref"`
	CartVersion          int64     `json:"cart_version,omitempty"`
	Authority            string    `json:"authority"`
	Serviceable          bool      `json:"serviceable"`
	ReasonCode           string    `json:"reason_code"`
	DetailReasonCode     string    `json:"detail_reason_code"`
	ProductCode          string    `json:"product_code,omitempty"`
	ProductVersion       int64     `json:"product_version,omitempty"`
	PolicyVersion        string    `json:"policy_version"`
	CapabilityVersion    string    `json:"capability_version"`
	PickupCount          int       `json:"pickup_count"`
	MaximumPickups       int       `json:"maximum_pickups"`
	EligibleVehicleCount int       `json:"eligible_vehicle_count"`
	EvaluatedAt          time.Time `json:"evaluated_at"`
}

const PurposeCommercePreview = "COMMERCE_PREVIEW"

// PreviewServiceability answers a pre-checkout cluster with ZERO execution
// side effects. It reads policy, markets, live positions and capabilities and
// writes nothing: no booking, mission, offer, lock, reservation, custody,
// notification or payment hold.
func (s *Service) PreviewServiceability(ctx context.Context, request ServiceabilityRequest) (ServiceabilityResponse, error) {
	out := ServiceabilityResponse{PreviewRef: request.PreviewRef, CartVersion: request.CartVersion, Authority: "TAXI", EvaluatedAt: s.now().UTC()}
	if !s.Enabled() || !s.caps.CapabilityEnabled {
		return out, ErrDisabled
	}
	if strings.TrimSpace(request.PreviewRef) == "" || request.Purpose != PurposeCommercePreview || len(request.Pickups) == 0 || s.policies == nil {
		return out, ErrInvalid
	}
	policy, err := s.policies.CurrentPolicy(ctx)
	if err != nil {
		return out, ErrUnavailable
	}
	out.PolicyVersion, out.CapabilityVersion, out.PickupCount = policy.Version, executionCapabilityVersion(policy, s.caps), len(request.Pickups)
	out.MaximumPickups = s.maximumPickups(policy)
	refuse := func(reason string) (ServiceabilityResponse, error) {
		out.DetailReasonCode, out.ReasonCode = reason, capability.CoarseReason(reason)
		return out, nil
	}
	stops := make([]capability.StopCargo, 0, len(request.Pickups))
	seen := map[string]bool{}
	for _, pickup := range request.Pickups {
		pickup.Cargo.StopRef = strings.TrimSpace(pickup.StopRef)
		if pickup.Cargo.StopRef == "" || seen[pickup.Cargo.StopRef] || pickup.Cargo.Validate() != nil {
			return out, ErrInvalid
		}
		seen[pickup.Cargo.StopRef] = true
		stops = append(stops, pickup.Cargo)
	}
	mission := capability.GoodsMission{Stops: stops, Dropoffs: 1}
	var product capability.Product
	reason := capability.ReasonProductNotEligible
	for _, candidate := range policy.ActiveProducts(capability.PurposeGoods, "") {
		if request.ProductCode != "" && !strings.EqualFold(candidate.Code, request.ProductCode) {
			continue
		}
		mission.ProductCode = candidate.Code
		if decision := capability.EvaluateGoodsPolicy(mission, candidate, s.caps.MultiPickupOperational()); decision.Eligible {
			product, reason = candidate, capability.ReasonEligible
			break
		} else {
			reason = decision.Reason
		}
	}
	if reason != capability.ReasonEligible {
		return refuse(reason)
	}
	out.ProductCode, out.ProductVersion = product.Code, product.Version
	first := geo.Point{Lat: request.Pickups[0].Location.Latitude, Lng: request.Pickups[0].Location.Longitude}
	zone, err := s.markets.ResolveZone(ctx, first)
	if err != nil {
		return refuse(capability.ReasonRouteConstraintFailed)
	}
	for _, point := range append(pickupPoints(request.Pickups), geo.Point{Lat: request.Dropoff.Latitude, Lng: request.Dropoff.Longitude}) {
		other, err := s.markets.ResolveZone(ctx, point)
		if err != nil || other.MarketID != zone.MarketID {
			return refuse(capability.ReasonRouteConstraintFailed)
		}
	}
	ids, err := s.nearbyIDs(ctx, zone.MarketID, first)
	if err != nil {
		return out, ErrUnavailable
	}
	candidates, err := s.goodsCandidates(ctx, ids)
	if err != nil {
		return out, err
	}
	mission.ProductCode = product.Code
	eligible, detail := s.evaluateCandidates(candidates, mission, product)
	out.EligibleVehicleCount = len(eligible)
	if len(eligible) == 0 {
		if detail == "" {
			detail = capability.ReasonNoEligibleProvider
		}
		return refuse(detail)
	}
	out.Serviceable, out.ReasonCode, out.DetailReasonCode = true, capability.CoarseReason(capability.ReasonEligible), capability.ReasonEligible
	return out, nil
}

// evaluateCandidates runs the hard gates and returns eligible candidates in
// proximity order, plus the most informative rejection reason.
func (s *Service) evaluateCandidates(candidates []capability.Candidate, mission capability.GoodsMission, product capability.Product) ([]capability.Candidate, string) {
	var eligible []capability.Candidate
	counts := map[string]int{}
	now := s.now().UTC()
	for _, candidate := range candidates {
		decision := capability.EvaluateGoods(candidate, mission, product, s.caps.MultiPickupOperational(), now)
		if decision.Eligible {
			eligible = append(eligible, candidate)
			continue
		}
		counts[decision.Reason]++
		s.metrics.cargoRejection(decision.Reason)
	}
	best, bestCount := "", 0
	for reason, count := range counts {
		if count > bestCount || (count == bestCount && reason < best) {
			best, bestCount = reason, count
		}
	}
	return eligible, best
}

func pickupPoints(pickups []PreviewPickup) []geo.Point {
	out := make([]geo.Point, 0, len(pickups))
	for _, p := range pickups {
		out = append(out, geo.Point{Lat: p.Location.Latitude, Lng: p.Location.Longitude})
	}
	return out
}

func (s *Service) maximumPickups(policy capability.Policy) int {
	if !s.caps.MultiPickupOperational() {
		if policy.MaxPickups("") > 0 {
			return 1
		}
		return 0
	}
	return policy.MaxPickups("")
}

func executionCapabilityVersion(policy capability.Policy, caps ExecutionFlags) string {
	return fmt.Sprintf("%s|multi=%t", policy.Version, caps.MultiPickupOperational())
}

// CapabilityResponse is Taxi's unified logistics capability contract (v2).
type CapabilityResponse struct {
	Authority              string              `json:"authority"`
	ContractVersion        int                 `json:"contract_version"`
	CapabilityVersion      string              `json:"capability_version"`
	PolicyVersion          string              `json:"policy_version"`
	MultiPickupSupported   bool                `json:"multi_pickup_supported"`
	MaximumPickups         int                 `json:"maximum_pickups"`
	MaximumDropoffs        int                 `json:"maximum_dropoffs"`
	SecurePickupHandover   bool                `json:"secure_pickup_handover"`
	SecureDeliveryHandover bool                `json:"secure_delivery_handover"`
	Products               []ProductCapability `json:"products"`
	CargoPredicates        []string            `json:"cargo_predicates"`
	ServiceabilityRules    []string            `json:"serviceability_constraints"`
}

type ProductCapability struct {
	ProductCode            string `json:"product_code"`
	ProductVersion         int64  `json:"product_version"`
	MaximumPickups         int    `json:"maximum_pickups"`
	SecurePickupHandover   bool   `json:"secure_pickup_handover"`
	SecureDeliveryHandover bool   `json:"secure_delivery_handover"`
}

// Capability reports what Taxi can actually execute right now. Multi-pickup
// is advertised only when execution, secure handover and cumulative fit are
// all active.
func (s *Service) Capability(ctx context.Context) (CapabilityResponse, error) {
	if !s.Enabled() || !s.caps.CapabilityEnabled || s.policies == nil {
		return CapabilityResponse{}, ErrDisabled
	}
	policy, err := s.policies.CurrentPolicy(ctx)
	if err != nil {
		return CapabilityResponse{}, ErrUnavailable
	}
	operational := s.caps.MultiPickupOperational()
	out := CapabilityResponse{Authority: "TAXI", ContractVersion: 2, CapabilityVersion: executionCapabilityVersion(policy, s.caps), PolicyVersion: policy.Version,
		MaximumDropoffs: 1, MaximumPickups: s.maximumPickups(policy),
		CargoPredicates:     []string{"WEIGHT", "VOLUME", "DIMENSIONS", "PACKAGE_COUNT", "FRAGILE", "UPRIGHT", "TEMPERATURE", "WEATHER_PROTECTION", "SECURE_CARGO", "CARGO_SEPARATION"},
		ServiceabilityRules: []string{"SINGLE_MARKET", "ONE_FINAL_DROPOFF", "CUMULATIVE_CARGO_FIT", "NO_PROVIDER_IDENTITY_IN_PREVIEW"}}
	for _, product := range policy.ActiveProducts(capability.PurposeGoods, "") {
		max := product.MaxPickups
		if !operational {
			max = 1
		}
		out.Products = append(out.Products, ProductCapability{ProductCode: product.Code, ProductVersion: product.Version, MaximumPickups: max,
			SecurePickupHandover: product.SecurePickupHandover, SecureDeliveryHandover: product.SecureDeliveryHandover})
		out.SecurePickupHandover = out.SecurePickupHandover || product.SecurePickupHandover
		out.SecureDeliveryHandover = out.SecureDeliveryHandover || product.SecureDeliveryHandover
	}
	out.MultiPickupSupported = operational && out.MaximumPickups > 1
	return out, nil
}

// createMultiStopMission is CreateMission's multi-pickup branch.
func (s *Service) createMultiStopMission(ctx context.Context, caller string, request commercemission.Request) (Mission, error) {
	if s.policies == nil {
		return Mission{}, ErrUnavailable
	}
	policy, err := s.policies.CurrentPolicy(ctx)
	if err != nil {
		return Mission{}, ErrUnavailable
	}
	commit, _, err := BuildMultiStopCommit(request, policy, s.caps)
	if err != nil {
		return Mission{}, err
	}
	mission, err := s.repo.CreateMultiStopMission(ctx, caller, commit)
	if errors.Is(err, ErrStalePlan) {
		return Mission{}, fmt.Errorf("%w: %v", ErrConflict, err)
	}
	if err != nil {
		return Mission{}, err
	}
	if mission.Status == StatusMatching && s.readyToMatch(commit.Stops) {
		_ = s.matchMultiStop(ctx, mission)
	}
	return s.repo.ReadMission(ctx, caller, request.MissionRef)
}

// readyToMatch implements just-in-time assignment: providers are only sought
// once the earliest pickup readiness is within JITLead.
func (s *Service) readyToMatch(stops []MissionStop) bool {
	var earliest *time.Time
	for _, stop := range stops {
		if stop.StopType == custodyops.StopPickup && stop.ReadinessWindowStart != nil && (earliest == nil || stop.ReadinessWindowStart.Before(*earliest)) {
			earliest = stop.ReadinessWindowStart
		}
	}
	return earliest == nil || !earliest.After(s.now().UTC().Add(JITLead))
}

// matchMultiStop selects a provider for a multi-pickup mission. Taxi alone
// chooses; hard gates run before any ordering is honoured.
func (s *Service) matchMultiStop(ctx context.Context, mission Mission) error {
	stops, cargo, product, err := s.repo.MissionShape(ctx, mission.ID)
	if err != nil || len(stops) == 0 {
		return err
	}
	if !s.readyToMatch(stops) || s.policies == nil {
		return nil
	}
	policy, err := s.policies.CurrentPolicy(ctx)
	if err != nil {
		return err
	}
	if current, ok := policy.Product(product.Code); ok {
		product = current
	}
	first := geo.Point{Lat: stops[0].Latitude, Lng: stops[0].Longitude}
	zone, err := s.markets.ResolveZone(ctx, first)
	if err != nil {
		return err
	}
	ids, err := s.nearbyIDs(ctx, zone.MarketID, first)
	if err != nil {
		return err
	}
	candidates, err := s.goodsCandidates(ctx, ids)
	if err != nil {
		return err
	}
	eligible, _ := s.evaluateCandidates(candidates, capability.GoodsMission{ProductCode: product.Code, Stops: cargo, Dropoffs: 1}, product)
	s.metrics.multiPickupMatch(len(stops) - 1)
	for _, candidate := range eligible {
		if err = s.repo.Offer(ctx, mission.ID, candidate.ProviderID, candidate.VehicleID); err == nil {
			return nil
		} else if !errors.Is(err, ErrConflict) {
			return err
		}
	}
	return nil
}

// Stop lifecycle for multi-pickup missions.

func (s *Service) StopEnRoute(ctx context.Context, providerID, missionRef, stopRef string) (Mission, error) {
	return s.repo.TransitionStop(ctx, missionRef, providerID, stopRef, []string{StatusAssigned, StatusEnRoutePickup}, StatusEnRoutePickup, "EN_ROUTE_PICKUP", []string{"PLANNED", "DELAYED"}, "EN_ROUTE", "taxi.goods.pickup.en_route.v1")
}

func (s *Service) StopArrived(ctx context.Context, providerID, missionRef, stopRef string) (Mission, error) {
	return s.repo.TransitionStop(ctx, missionRef, providerID, stopRef, []string{StatusEnRoutePickup}, StatusAtPickup, "AT_PICKUP", []string{"EN_ROUTE"}, "ARRIVED", "taxi.goods.pickup.arrived.v1")
}

// StopHandover opens the secure pickup handover for exactly one stop.
func (s *Service) StopHandover(ctx context.Context, providerID, missionRef, stopRef string) (Credential, error) {
	if !s.caps.SecureHandoverEnabled {
		return Credential{}, ErrDisabled
	}
	mission, err := s.repo.TransitionStop(ctx, missionRef, providerID, stopRef, []string{StatusAtPickup}, StatusPickupHandoverRequired, "AT_PICKUP", []string{"ARRIVED"}, "HANDOVER_PENDING", "taxi.goods.pickup_handover_required.v1")
	if err != nil {
		return Credential{}, err
	}
	version := int64(1)
	if latest, latestErr := s.repo.LatestChallenge(ctx, mission.ID, StagePickup, false); latestErr == nil {
		if latest.Status == "ACTIVE" {
			return Credential{}, ErrConflict
		}
		version = latest.Version + 1
	}
	owner := mission.ReceiverRef
	if owner == "" {
		owner = mission.CustomerRef
	}
	credential, record, err := s.newChallenge(mission, StagePickup, mission.ProviderID, owner, version, 0)
	if err != nil {
		return Credential{}, err
	}
	record.StopRef = strings.TrimSpace(stopRef)
	if err = s.repo.InsertChallenge(ctx, record); err != nil {
		return Credential{}, err
	}
	return credential, nil
}

// ReportPickupDelay records a readiness delay after commitment. It never
// creates a replacement mission; Vendor receives a replan request instead.
func (s *Service) ReportPickupDelay(ctx context.Context, caller, missionRef, stopRef string, readyAt time.Time) (Mission, error) {
	if readyAt.IsZero() || strings.TrimSpace(stopRef) == "" {
		return Mission{}, ErrInvalid
	}
	s.metrics.pickupDelay()
	return s.repo.MarkStopDelayed(ctx, caller, missionRef, strings.TrimSpace(stopRef), readyAt.UTC())
}

// ReplanRoute versions a new stop order without touching manifests,
// handover history or custody.
func (s *Service) ReplanRoute(ctx context.Context, caller, missionRef string, command RouteReplanCommand) (int64, error) {
	if strings.TrimSpace(command.OperationRef) == "" || strings.TrimSpace(command.ReasonCode) == "" || command.ExpectedRouteVersion < 1 || len(command.StopOrder) < 2 {
		return 0, ErrInvalid
	}
	return s.repo.ReplanRoute(ctx, caller, missionRef, strings.TrimSpace(command.OperationRef), strings.ToUpper(strings.TrimSpace(command.ReasonCode)), command.StopOrder, command.ExpectedRouteVersion)
}

type RouteReplanCommand struct {
	OperationRef         string   `json:"operation_ref"`
	ReasonCode           string   `json:"reason_code"`
	StopOrder            []string `json:"stop_order"`
	ExpectedRouteVersion int64    `json:"expected_route_version"`
}

type PickupDelayCommand struct {
	StopRef string    `json:"stop_ref"`
	ReadyAt time.Time `json:"ready_at"`
}

// isMultiStop is used to refuse mission-level pickup shortcuts on missions
// whose pickups must go through the per-stop lifecycle.
func (s *Service) isMultiStop(ctx context.Context, missionRef string) bool {
	var mode string
	if s.repo == nil || s.repo.db == nil {
		return false
	}
	if err := s.repo.db.QueryRow(ctx, `SELECT custody_mode FROM taxi_goods_missions WHERE mission_ref=$1`, missionRef).Scan(&mode); err != nil {
		return false
	}
	return mode == custodyops.ModeMultiManifest
}
