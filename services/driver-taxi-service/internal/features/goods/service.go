package goods

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	marketmodels "cosmicforge/logistics/services/taxi-service/internal/features/markets/models"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/services/taxi-service/internal/routing"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/handover"
)

type MarketResolver interface {
	ResolveZone(context.Context, geo.Point) (marketmodels.Zone, error)
}

type CandidateStore interface {
	NearbyProviders(context.Context, string, string, geo.Point, float64, int) ([]availabilityrepositories.ProviderStatus, error)
	SetDiscoverable(context.Context, string, string, string, bool) error
}

type SettlementAuthority interface {
	ReleaseCommerceDelivery(context.Context, string, string, string, string) error
}

type MediaAuthority interface {
	AuthorizeEvidence(context.Context, string, string) error
}

type CredentialNotifier interface {
	NotifyDeliveryCredential(context.Context, string, string, Credential) error
	NotifyPickupCredential(context.Context, string, string, Credential) error
}

type Options struct {
	Repository     *Repository
	Markets        MarketResolver
	Candidates     CandidateStore
	Routing        routing.RoutingClient
	Settlement     SettlementAuthority
	Media          MediaAuthority
	Notifier       CredentialNotifier
	Pricing        PricingPolicy
	Enabled        bool
	HandoverSecret []byte
	QuoteTTL       time.Duration
	MatchRadiusKM  float64
	Now            func() time.Time
}

type Service struct {
	repo       *Repository
	markets    MarketResolver
	candidates CandidateStore
	routes     routing.RoutingClient
	settlement SettlementAuthority
	media      MediaAuthority
	notifier   CredentialNotifier
	pricing    PricingPolicy
	enabled    bool
	secret     []byte
	quoteTTL   time.Duration
	radiusKM   float64
	now        func() time.Time
}

func NewService(options Options) *Service {
	if options.QuoteTTL <= 0 {
		options.QuoteTTL = 15 * time.Minute
	}
	if options.MatchRadiusKM <= 0 {
		options.MatchRadiusKM = 10
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return &Service{repo: options.Repository, markets: options.Markets, candidates: options.Candidates, routes: options.Routing,
		settlement: options.Settlement, media: options.Media, notifier: options.Notifier, pricing: options.Pricing, enabled: options.Enabled,
		secret: options.HandoverSecret, quoteTTL: options.QuoteTTL, radiusKM: options.MatchRadiusKM, now: options.Now}
}

func (s *Service) Enabled() bool { return s != nil && s.enabled }

func (s *Service) Quote(ctx context.Context, caller string, request commercequote.Request) (commercequote.Quote, error) {
	if !s.Enabled() {
		return commercequote.Quote{}, ErrDisabled
	}
	if message, ok := request.Validate(); !ok || request.VehicleClass != commercequote.VehicleCar || len(request.PickupStops) > 1 {
		_ = message
		return commercequote.Quote{}, ErrInvalid
	}
	if s.repo == nil || s.markets == nil || s.candidates == nil || s.routes == nil {
		return commercequote.Quote{}, ErrUnavailable
	}
	if !strings.EqualFold(strings.TrimSpace(request.Currency), s.pricing.Currency) {
		return s.refusedQuote(ctx, caller, request, commercequote.ReasonCurrencyUnsupported)
	}
	pickup := request.Pickup
	if len(request.PickupStops) == 1 {
		pickup = request.PickupStops[0].Location
	}
	pickupPoint := geo.Point{Lat: pickup.Latitude, Lng: pickup.Longitude}
	dropoffPoint := geo.Point{Lat: request.Dropoff.Latitude, Lng: request.Dropoff.Longitude}
	zone, err := s.markets.ResolveZone(ctx, pickupPoint)
	if err != nil {
		return s.refusedQuote(ctx, caller, request, commercequote.ReasonOutOfServiceArea)
	}
	dropZone, err := s.markets.ResolveZone(ctx, dropoffPoint)
	if err != nil || dropZone.MarketID != zone.MarketID {
		return s.refusedQuote(ctx, caller, request, commercequote.ReasonOutOfServiceArea)
	}
	route, err := s.routes.Route(ctx, pickupPoint, dropoffPoint)
	if err != nil {
		return commercequote.Quote{}, ErrUnavailable
	}
	ids, err := s.nearbyIDs(ctx, zone.MarketID, pickupPoint)
	if err != nil {
		return commercequote.Quote{}, ErrUnavailable
	}
	eligible, err := s.repo.EligibleCandidates(ctx, ids, request.Load)
	if err != nil {
		return commercequote.Quote{}, err
	}
	if len(eligible) == 0 {
		return s.saveQuote(ctx, caller, request, zone, route, nil, commercequote.ReasonNoProviderAvailable)
	}
	return s.saveQuote(ctx, caller, request, zone, route, eligible, commercequote.ReasonServiceable)
}

func (s *Service) refusedQuote(ctx context.Context, caller string, request commercequote.Request, reason string) (commercequote.Quote, error) {
	return s.repo.SaveQuote(ctx, caller, request, commercequote.Quote{Service: "TAXI", Serviceable: false, ReasonCode: reason, VehicleClass: commercequote.VehicleCar, Currency: strings.ToUpper(request.Currency), ExpiresAt: s.now().UTC().Add(s.quoteTTL)})
}

func (s *Service) saveQuote(ctx context.Context, caller string, request commercequote.Request, zone marketmodels.Zone, route routing.RouteResult, eligible []string, reason string) (commercequote.Quote, error) {
	distance := float64(route.DistanceM) / 1000
	minutes := int((route.DurationS + 59) / 60)
	fee, distanceMinor := s.pricing.Price(distance, minutes, len(request.Load.Capabilities))
	serviceable := reason == commercequote.ReasonServiceable
	quote := commercequote.Quote{Service: "TAXI", Serviceable: serviceable, ReasonCode: reason, VehicleClass: commercequote.VehicleCar, PickupRef: zone.MarketID, DropoffRef: zone.ID, DistanceKm: distance, EstimatedMinutes: minutes, Currency: strings.ToUpper(request.Currency), AvailableProviders: len(eligible), ExpiresAt: s.now().UTC().Add(s.quoteTTL)}
	if serviceable {
		quote.FeeMinor = fee
		quote.Breakdown = commercequote.Breakdown{BaseMinor: s.pricing.BaseMinor, DistanceMinor: distanceMinor, HandlingMinor: int64(len(request.Load.Capabilities)) * s.pricing.HandlingMinor}
	}
	return s.repo.SaveQuote(ctx, caller, request, quote)
}

func (s *Service) nearbyIDs(ctx context.Context, market string, pickup geo.Point) ([]string, error) {
	seen := map[string]bool{}
	var ids []string
	for _, class := range []string{"economy", "comfort", "xl"} {
		candidates, err := s.candidates.NearbyProviders(ctx, market, class, pickup, s.radiusKM, 50)
		if err != nil {
			return nil, err
		}
		for _, candidate := range candidates {
			if !seen[candidate.ProviderID] {
				seen[candidate.ProviderID] = true
				ids = append(ids, candidate.ProviderID)
			}
		}
	}
	return ids, nil
}

func (s *Service) CreateMission(ctx context.Context, caller string, request commercemission.Request) (Mission, error) {
	if !s.Enabled() {
		return Mission{}, ErrDisabled
	}
	if message, ok := request.Validate(); !ok || request.Service != commercemission.ServiceTaxi || request.VehicleClass != commercemission.VehicleCar || len(request.PickupStops) > 1 || len(request.Parcels) == 0 || !validMissionParcels(request) {
		_ = message
		return Mission{}, ErrInvalid
	}
	mission, err := s.repo.CreateMission(ctx, caller, request)
	if err != nil {
		return Mission{}, err
	}
	if mission.Status == StatusMatching {
		_ = s.Match(ctx, mission, request)
	}
	return s.repo.ReadMission(ctx, caller, request.MissionRef)
}

func (s *Service) Match(ctx context.Context, mission Mission, request commercemission.Request) error {
	pickup := request.Pickup
	if len(request.PickupStops) == 1 {
		pickup = request.PickupStops[0].Location
	}
	point := geo.Point{Lat: pickup.Latitude, Lng: pickup.Longitude}
	zone, err := s.markets.ResolveZone(ctx, point)
	if err != nil {
		return err
	}
	ids, err := s.nearbyIDs(ctx, zone.MarketID, point)
	if err != nil {
		return err
	}
	load := commercequote.Load{PackageCount: request.Load.PackageCount, TotalWeightGrams: request.Load.WeightGrams, Capabilities: handlingCapabilities(request.Handling)}
	if len(request.Parcels) > 0 {
		load.PackageCount, load.TotalWeightGrams = 0, 0
	}
	for _, parcel := range request.Parcels {
		load.TotalWeightGrams += parcel.WeightGrams
		load.PackageCount += parcel.PackageCount
		volume := parcel.LengthMm * parcel.WidthMm * parcel.HeightMm / 1000
		load.TotalVolumeMl += volume
		if dimension := max64(parcel.LengthMm, max64(parcel.WidthMm, parcel.HeightMm)); int(dimension) > load.LongestDimensionMm {
			load.LongestDimensionMm = int(dimension)
		}
		load.Capabilities = append(load.Capabilities, handlingCapabilities(parcel.Handling)...)
	}
	load.Capabilities = canonical(load.Capabilities)
	return s.matchLoad(ctx, mission, load, ids)
}

func (s *Service) matchLoad(ctx context.Context, mission Mission, load commercequote.Load, ids []string) error {
	eligible, err := s.repo.EligibleCandidates(ctx, ids, load)
	if err != nil {
		return err
	}
	for _, providerID := range eligible {
		vehicleID, e := s.repo.CandidateVehicle(ctx, providerID)
		if e != nil {
			continue
		}
		if e = s.repo.Offer(ctx, mission.ID, providerID, vehicleID); e == nil {
			return nil
		}
		if !errors.Is(e, ErrConflict) {
			return e
		}
	}
	return nil
}

func (s *Service) ReconcileMatching(ctx context.Context, limit int) error {
	missions, err := s.repo.MatchingMissions(ctx, limit)
	if err != nil {
		return err
	}
	for _, mission := range missions {
		point := geo.Point{Lat: mission.Pickup.Latitude, Lng: mission.Pickup.Longitude}
		zone, resolveErr := s.markets.ResolveZone(ctx, point)
		if resolveErr != nil {
			continue
		}
		ids, nearbyErr := s.nearbyIDs(ctx, zone.MarketID, point)
		if nearbyErr != nil {
			continue
		}
		load, loadErr := s.repo.MissionLoad(ctx, mission.ID)
		if loadErr != nil {
			continue
		}
		_ = s.matchLoad(ctx, mission, load, ids)
	}
	return nil
}

func (s *Service) ReadMission(ctx context.Context, caller, missionRef string) (Mission, error) {
	return s.repo.ReadMission(ctx, caller, missionRef)
}
func (s *Service) ReadOperation(ctx context.Context, caller, operationRef string) (Mission, error) {
	return s.repo.ReadMissionByOperation(ctx, caller, operationRef)
}
func (s *Service) Cancel(ctx context.Context, caller, missionRef, reason string, version int64) (Mission, error) {
	return s.repo.Cancel(ctx, caller, missionRef, reason, version)
}
func (s *Service) ProviderSettings(ctx context.Context, providerID string) (ProviderSettings, error) {
	return s.repo.GetProviderSettings(ctx, providerID)
}
func (s *Service) SetProviderAvailability(ctx context.Context, providerID string, enabled bool, version int64) (ProviderSettings, error) {
	return s.repo.SetGoodsAvailability(ctx, providerID, enabled, version)
}
func (s *Service) SetProviderCapability(ctx context.Context, providerID, status, operator string, version int64) (ProviderSettings, error) {
	return s.repo.SetCapability(ctx, providerID, status, operator, version)
}
func (s *Service) CargoProfile(ctx context.Context, providerID, vehicleID string) (CargoProfile, error) {
	return s.repo.GetCargoProfile(ctx, providerID, vehicleID)
}
func (s *Service) PutCargoProfile(ctx context.Context, providerID string, input CargoProfile, version int64) (CargoProfile, error) {
	return s.repo.PutCargoProfile(ctx, providerID, input, version)
}
func (s *Service) VerifyCargoProfile(ctx context.Context, providerID, vehicleID, operator string, verified bool, version int64) (CargoProfile, error) {
	return s.repo.VerifyCargoProfile(ctx, providerID, vehicleID, operator, verified, version)
}
func (s *Service) ListProviderMissions(ctx context.Context, providerID string) ([]Mission, error) {
	return s.repo.ListProviderMissions(ctx, providerID)
}
func (s *Service) GetProviderMission(ctx context.Context, providerID, missionRef string) (Mission, error) {
	return s.repo.ReadMissionForProvider(ctx, providerID, missionRef)
}

func (s *Service) ReassignProvider(ctx context.Context, operator, missionRef string, command ReassignProviderCommand) (Mission, error) {
	command.NewProviderID = strings.TrimSpace(command.NewProviderID)
	command.NewVehicleID = strings.TrimSpace(command.NewVehicleID)
	command.OperationRef = strings.TrimSpace(command.OperationRef)
	if command.NewProviderID == "" || command.NewVehicleID == "" || command.OperationRef == "" || command.ExpectedVersion < 1 {
		return Mission{}, ErrInvalid
	}
	mission, err := s.repo.ReadMissionForOperations(ctx, missionRef)
	if err != nil {
		return Mission{}, err
	}
	var fresh *HandoverRecord
	var credential Credential
	if mission.Status == StatusPickupHandoverRequired {
		version := int64(1)
		resend := 0
		if latest, latestErr := s.repo.LatestChallenge(ctx, mission.ID, StagePickup, false); latestErr == nil {
			version, resend = latest.Version+1, latest.ResendCount
		}
		var record HandoverRecord
		credential, record, err = s.newChallenge(mission, StagePickup, command.NewProviderID, mission.ReceiverRef, version, resend)
		if err != nil {
			return Mission{}, err
		}
		fresh = &record
	}
	updated, applied, err := s.repo.ReassignProvider(ctx, missionRef, operator, command, Fingerprint(command), fresh)
	if err != nil {
		return Mission{}, err
	}
	if fresh != nil && applied && s.notifier != nil {
		if err = s.notifier.NotifyPickupCredential(ctx, command.NewProviderID, missionRef, credential); err != nil {
			_ = s.repo.InvalidateChallenge(ctx, fresh.ChallengeRef, "NOTIFICATION_FAILED")
			return Mission{}, fmt.Errorf("%w: notify reassigned provider", ErrUnavailable)
		}
	}
	return updated, nil
}

func (s *Service) ChangeReceiver(ctx context.Context, caller, operator, missionRef string, command ReceiverChangeCommand) (Mission, error) {
	command.ReceiverRef = strings.TrimSpace(command.ReceiverRef)
	command.ReceiverName = strings.TrimSpace(command.ReceiverName)
	command.ReceiverPhone = strings.TrimSpace(command.ReceiverPhone)
	command.OperationRef = strings.TrimSpace(command.OperationRef)
	if command.ReceiverRef == "" || command.ReceiverName == "" || command.OperationRef == "" || command.ExpectedVersion < 1 {
		return Mission{}, ErrInvalid
	}
	mission, err := s.repo.ReadMission(ctx, caller, missionRef)
	if err != nil {
		return Mission{}, err
	}
	var fresh *HandoverRecord
	var credential Credential
	if latest, latestErr := s.repo.LatestChallenge(ctx, mission.ID, StageDelivery, false); latestErr == nil && latest.Status == "ACTIVE" {
		var record HandoverRecord
		credential, record, err = s.newChallenge(mission, StageDelivery, mission.ProviderID, command.ReceiverRef, latest.Version+1, latest.ResendCount)
		if err != nil {
			return Mission{}, err
		}
		fresh = &record
	}
	updated, applied, err := s.repo.ChangeReceiver(ctx, caller, missionRef, operator, command, Fingerprint(command), fresh)
	if err != nil {
		return Mission{}, err
	}
	if fresh != nil && applied {
		if s.notifier == nil {
			_ = s.repo.InvalidateChallenge(ctx, fresh.ChallengeRef, "NOTIFICATION_UNAVAILABLE")
			return Mission{}, ErrUnavailable
		}
		if err = s.notifier.NotifyDeliveryCredential(ctx, command.ReceiverRef, missionRef, credential); err != nil {
			_ = s.repo.InvalidateChallenge(ctx, fresh.ChallengeRef, "NOTIFICATION_FAILED")
			return Mission{}, fmt.Errorf("%w: notify changed receiver", ErrUnavailable)
		}
	}
	return updated, nil
}

func (s *Service) Accept(ctx context.Context, providerID, missionRef string) (Mission, error) {
	return s.repo.TransitionProvider(ctx, missionRef, providerID, StatusOffered, StatusAssigned, "", "taxi.goods.assigned.v1")
}
func (s *Service) EnRoutePickup(ctx context.Context, providerID, missionRef string) (Mission, error) {
	return s.repo.TransitionProvider(ctx, missionRef, providerID, StatusAssigned, StatusEnRoutePickup, "EN_ROUTE_PICKUP", "taxi.goods.en_route_pickup.v1")
}
func (s *Service) ArrivePickup(ctx context.Context, providerID, missionRef string) (Mission, error) {
	return s.repo.TransitionProvider(ctx, missionRef, providerID, StatusEnRoutePickup, StatusAtPickup, "AT_PICKUP", "taxi.goods.at_pickup.v1")
}
func (s *Service) InTransit(ctx context.Context, providerID, missionRef string) (Mission, error) {
	return s.repo.TransitionProvider(ctx, missionRef, providerID, StatusPickedUp, StatusInTransit, "ON_TRIP", "taxi.goods.in_transit.v1")
}
func (s *Service) ArriveDropoff(ctx context.Context, providerID, missionRef string) (Mission, error) {
	if _, err := s.repo.TransitionProvider(ctx, missionRef, providerID, StatusInTransit, StatusAtDropoff, "ON_TRIP", "taxi.goods.at_dropoff.v1"); err != nil {
		return Mission{}, err
	}
	return s.repo.TransitionProvider(ctx, missionRef, providerID, StatusAtDropoff, StatusDeliveryHandoverRequired, "ON_TRIP", "taxi.goods.delivery_handover_required.v1")
}

func (s *Service) IssuePickupCredential(ctx context.Context, providerID, missionRef string) (Credential, error) {
	mission, err := s.repo.ReadMissionForProvider(ctx, providerID, missionRef)
	if err != nil {
		return Credential{}, err
	}
	if mission.Status == StatusAtPickup {
		mission, err = s.repo.TransitionProvider(ctx, missionRef, providerID, StatusAtPickup, StatusPickupHandoverRequired, "AT_PICKUP", "taxi.goods.pickup_handover_required.v1")
		if err != nil {
			return Credential{}, err
		}
	}
	if mission.Status != StatusPickupHandoverRequired {
		return Credential{}, ErrConflict
	}
	return s.issue(ctx, mission, StagePickup, 0)
}

func (s *Service) issue(ctx context.Context, mission Mission, stage string, resend int) (Credential, error) {
	if len(s.secret) == 0 {
		return Credential{}, ErrUnavailable
	}
	version := int64(1)
	if latest, err := s.repo.LatestChallenge(ctx, mission.ID, stage, false); err == nil {
		if latest.Status == "ACTIVE" {
			return Credential{}, ErrConflict
		}
		version = latest.Version + 1
	}
	owner := mission.ReceiverRef
	if owner == "" {
		owner = mission.CustomerRef
	}
	credential, stored, err := s.newChallenge(mission, stage, mission.ProviderID, owner, version, resend)
	if err != nil {
		return Credential{}, err
	}
	if err = s.repo.InsertChallenge(ctx, stored); err != nil {
		return Credential{}, err
	}
	return credential, nil
}

func (s *Service) newChallenge(mission Mission, stage, providerID, owner string, version int64, resend int) (Credential, HandoverRecord, error) {
	ref := "taxi_goods_" + strings.ToLower(stage) + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	step := handover.StepPickup
	if stage == StageDelivery {
		step = handover.StepDelivery
	}
	now := s.now().UTC()
	record, err := handover.NewSecureCodeRecord(s.secret, "taxi-goods", step, ref, version, mission.ID, providerID, owner, resend, now)
	if err != nil {
		return Credential{}, HandoverRecord{}, err
	}
	stored := HandoverRecord{ChallengeRef: ref, MissionID: mission.ID, Stage: stage, ProviderID: providerID, ReceiverRef: owner, OTPHash: record.Hash, QRHash: record.QRHash, Version: version, ResendCount: resend, ExpiresAt: record.ExpiresAt, ResendAvailableAt: now.Add(handover.ResendCooldown)}
	credential := Credential{ChallengeRef: ref, Stage: stage, OTP: record.Code, QRToken: record.QRToken, Version: version, ExpiresAt: record.ExpiresAt}
	return credential.WithDirection(mission.MissionRef), stored, nil
}

func (s *Service) Resend(ctx context.Context, providerID, missionRef, stage string) (Credential, error) {
	mission, err := s.repo.ReadMissionForProvider(ctx, providerID, missionRef)
	if err != nil {
		return Credential{}, err
	}
	latest, err := s.repo.LatestChallenge(ctx, mission.ID, stage, false)
	if err != nil {
		return Credential{}, err
	}
	now := s.now().UTC()
	if latest.ResendCount >= handover.MaxResends || now.Before(latest.ResendAvailableAt) {
		return Credential{}, ErrResendUnavailable
	}
	ref := "taxi_goods_" + strings.ToLower(stage) + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	step := handover.StepPickup
	if stage == StageDelivery {
		step = handover.StepDelivery
	}
	owner := mission.ReceiverRef
	if owner == "" {
		owner = mission.CustomerRef
	}
	record, err := handover.NewSecureCodeRecord(s.secret, "taxi-goods", step, ref, latest.Version+1, mission.ID, providerID, owner, latest.ResendCount+1, now)
	if err != nil {
		return Credential{}, err
	}
	next := HandoverRecord{ChallengeRef: ref, MissionID: mission.ID, Stage: stage, ProviderID: providerID, ReceiverRef: owner, OTPHash: record.Hash, QRHash: record.QRHash, Version: latest.Version + 1, ResendCount: latest.ResendCount + 1, ExpiresAt: record.ExpiresAt, ResendAvailableAt: now.Add(handover.ResendCooldown)}
	if latest.Status == "ACTIVE" {
		err = s.repo.ReplaceChallenge(ctx, latest, next)
	} else {
		err = s.repo.InsertChallenge(ctx, next)
	}
	if err != nil {
		return Credential{}, err
	}
	credential := Credential{ChallengeRef: ref, Stage: stage, OTP: record.Code, QRToken: record.QRToken, Version: next.Version, ExpiresAt: record.ExpiresAt}
	if stage == StageDelivery {
		if s.notifier == nil {
			_ = s.repo.InvalidateChallenge(ctx, next.ChallengeRef, "NOTIFICATION_UNAVAILABLE")
			return Credential{}, ErrUnavailable
		}
		if err = s.notifier.NotifyDeliveryCredential(ctx, owner, mission.MissionRef, credential); err != nil {
			_ = s.repo.InvalidateChallenge(ctx, next.ChallengeRef, "NOTIFICATION_FAILED")
			return Credential{}, fmt.Errorf("%w: notify delivery credential", ErrUnavailable)
		}
	}
	return credential, nil
}

// ResendDelivery rotates both credentials and sends them only through the
// receiver notification boundary. The provider receives safe metadata.
func (s *Service) ResendDelivery(ctx context.Context, providerID, missionRef string) (Credential, error) {
	credential, err := s.Resend(ctx, providerID, missionRef, StageDelivery)
	if err != nil {
		return Credential{}, err
	}
	credential.OTP, credential.QRToken = "", ""
	return credential, nil
}

func (s *Service) VerifyPickup(ctx context.Context, caller, missionRef string, command VerifyCredential) (Mission, error) {
	command.ChallengeRef = strings.TrimSpace(command.ChallengeRef)
	command.Method = strings.ToUpper(strings.TrimSpace(command.Method))
	command.OperationRef = strings.TrimSpace(command.OperationRef)
	mission, err := s.repo.ReadMission(ctx, caller, missionRef)
	if err != nil {
		return Mission{}, err
	}
	if mission.Status != StatusPickupHandoverRequired && mission.Status != StatusPickedUp {
		return Mission{}, ErrConflict
	}
	candidate, err := s.credentialCandidate(ctx, mission, StagePickup, command)
	if err != nil {
		return Mission{}, err
	}
	updated, err := s.repo.VerifyAndTransition(ctx, caller, mission.ProviderID, missionRef, StagePickup, command, candidate, s.now().UTC())
	if err != nil {
		return Mission{}, err
	}
	credential, issueErr := s.issue(ctx, updated, StageDelivery, 0)
	if issueErr == nil {
		receiver := updated.ReceiverRef
		if receiver == "" {
			receiver = updated.CustomerRef
		}
		if s.notifier == nil {
			_ = s.repo.InvalidateChallenge(ctx, credential.ChallengeRef, "NOTIFICATION_UNAVAILABLE")
			return Mission{}, ErrUnavailable
		}
		if err = s.notifier.NotifyDeliveryCredential(ctx, receiver, updated.MissionRef, credential); err != nil {
			_ = s.repo.InvalidateChallenge(ctx, credential.ChallengeRef, "NOTIFICATION_FAILED")
			return Mission{}, fmt.Errorf("%w: notify delivery credential", ErrUnavailable)
		}
	} else if !errors.Is(issueErr, ErrConflict) {
		return Mission{}, issueErr
	}
	return updated, nil
}

func (s *Service) VerifyDelivery(ctx context.Context, providerID, missionRef string, command VerifyCredential) (Mission, error) {
	command.ChallengeRef = strings.TrimSpace(command.ChallengeRef)
	command.Method = strings.ToUpper(strings.TrimSpace(command.Method))
	command.OperationRef = strings.TrimSpace(command.OperationRef)
	command.ReceiverRef = strings.TrimSpace(command.ReceiverRef)
	mission, err := s.repo.ReadMissionForProvider(ctx, providerID, missionRef)
	if err != nil {
		return Mission{}, err
	}
	if mission.Status != StatusDeliveryHandoverRequired && mission.Status != StatusDelivered {
		return Mission{}, ErrConflict
	}
	candidate, err := s.credentialCandidate(ctx, mission, StageDelivery, command)
	if err != nil {
		return Mission{}, err
	}
	return s.repo.VerifyAndTransition(ctx, "", providerID, missionRef, StageDelivery, command, candidate, s.now().UTC())
}

func (s *Service) credentialCandidate(ctx context.Context, mission Mission, stage string, command VerifyCredential) (string, error) {
	method := strings.ToUpper(strings.TrimSpace(command.Method))
	command.OperationRef = strings.TrimSpace(command.OperationRef)
	if (method != "OTP" && method != "QR") || strings.TrimSpace(command.Credential) == "" || command.OperationRef == "" {
		return "", ErrInvalid
	}
	record, err := s.repo.LatestChallenge(ctx, mission.ID, stage, false)
	if err != nil {
		return "", err
	}
	if record.ChallengeRef != command.ChallengeRef || record.ProviderID != mission.ProviderID {
		return "", ErrCredentialInvalid
	}
	step := handover.StepPickup
	if stage == StageDelivery {
		step = handover.StepDelivery
	}
	secure := handover.CodeRecord{ChallengeRef: record.ChallengeRef, Version: record.Version, Service: "taxi-goods", Stage: step, BookingID: mission.ID, ProviderID: mission.ProviderID, OwnerID: record.ReceiverRef, ExpiresAt: record.ExpiresAt}
	expected := record.OTPHash
	if method == "QR" {
		expected = record.QRHash
	}
	candidate := "invalid"
	if handover.VerifySecureCredential(s.secret, method, secure, command.Credential, expected) {
		candidate = expected
	}
	return candidate, nil
}

func (s *Service) AddEvidence(ctx context.Context, providerID, missionRef string, command EvidenceCommand) (string, error) {
	mission, err := s.repo.ReadMissionForProvider(ctx, providerID, missionRef)
	if err != nil {
		return "", err
	}
	command.Stage = strings.ToUpper(strings.TrimSpace(command.Stage))
	if (command.Stage != StagePickup && command.Stage != StageDelivery) || strings.TrimSpace(command.MediaRef) == "" {
		return "", ErrInvalid
	}
	if command.Stage == StagePickup && !pickupEvidenceState(mission.Status) {
		return "", ErrConflict
	}
	if command.Stage == StageDelivery && mission.Status != StatusDeliveryHandoverRequired && mission.Status != StatusDelivered {
		return "", ErrConflict
	}
	if command.CapturedAt.IsZero() {
		command.CapturedAt = s.now().UTC()
	}
	if s.media == nil {
		return "", ErrUnavailable
	}
	if err = s.media.AuthorizeEvidence(ctx, mission.ID, command.MediaRef); err != nil {
		return "", ErrMediaScopeMismatch
	}
	return s.repo.AddEvidence(ctx, mission, providerID, command)
}

func (s *Service) Complete(ctx context.Context, providerID, missionRef string) (Mission, error) {
	mission, err := s.repo.ReadMissionForProvider(ctx, providerID, missionRef)
	if err != nil {
		return Mission{}, err
	}
	if mission.Status != StatusDelivered {
		return Mission{}, ErrConflict
	}
	if mission.DeliveryEvidenceRequired && strings.TrimSpace(mission.DeliveryEvidenceRef) == "" {
		return Mission{}, ErrEvidenceRequired
	}
	if mission.PickupEvidenceRequired && strings.TrimSpace(mission.PickupEvidenceRef) == "" {
		return Mission{}, ErrEvidenceRequired
	}
	updated, err := s.repo.TransitionProvider(ctx, missionRef, providerID, StatusDelivered, StatusCompleted, "AVAILABLE", "taxi.goods.completed.v1")
	if err != nil {
		return Mission{}, err
	}
	s.settle(ctx, updated)
	return updated, nil
}

func (s *Service) settle(ctx context.Context, mission Mission) {
	if s.settlement == nil {
		_ = s.repo.CompleteSettlement(ctx, mission.ID, false)
		return
	}
	ok := true
	for _, vendor := range mission.VendorOrderRefs {
		if err := s.settlement.ReleaseCommerceDelivery(ctx, mission.ParentOrderRef, vendor, mission.ProviderID, mission.ID); err != nil {
			ok = false
		}
	}
	_ = s.repo.CompleteSettlement(ctx, mission.ID, ok)
}

func (s *Service) ReconcileSettlements(ctx context.Context, limit int) error {
	missions, err := s.repo.PendingSettlements(ctx, limit)
	if err != nil {
		return err
	}
	for _, mission := range missions {
		s.settle(ctx, mission)
	}
	return nil
}

func (s *Service) RunSettlementWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		_ = s.ReconcileSettlements(ctx, 50)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) RunRecoveryWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		_ = s.ReconcileMatching(ctx, 50)
		_ = s.repo.ExpireChallenges(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) AssertDependencies() error {
	if !s.Enabled() {
		return nil
	}
	if s.repo == nil || s.markets == nil || s.candidates == nil || s.routes == nil || s.settlement == nil || s.media == nil || s.notifier == nil || len(s.secret) == 0 {
		return fmt.Errorf("%w: Taxi Goods dependencies are incomplete", ErrUnavailable)
	}
	return nil
}

func validMissionParcels(request commercemission.Request) bool {
	vendors := make(map[string]bool, len(request.VendorOrderRefs))
	for _, ref := range request.VendorOrderRefs {
		vendors[strings.TrimSpace(ref)] = true
	}
	for _, parcel := range request.Parcels {
		if !vendors[strings.TrimSpace(parcel.VendorOrderRef)] || strings.TrimSpace(parcel.ManifestRef) == "" || parcel.ManifestVersion < 1 {
			return false
		}
	}
	return true
}

func pickupEvidenceState(status string) bool {
	switch status {
	case StatusPickupHandoverRequired, StatusPickedUp, StatusInTransit, StatusAtDropoff,
		StatusDeliveryHandoverRequired, StatusDelivered:
		return true
	default:
		return false
	}
}
