package privateexecution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"cosmicforge/logistics/shared/go/geo"
)

// Zone is the operating zone a corporate booking is placed in.
//
// It is a local value rather than the markets package's own type so this
// feature depends on a two-field shape instead of a whole feature, which keeps
// the dependency pointing the way the service layout intends.
type Zone struct {
	ID       string
	MarketID string
}

// ZoneResolver finds the operating zone containing a point.
//
// A taxi booking requires a market and a zone, both NOT NULL. Fleet's private
// orders carry neither: they carry a pickup coordinate. Resolving it here is
// what keeps a corporate trip inside a market this service actually operates,
// instead of parked in whichever zone happened to be first.
type ZoneResolver interface {
	ResolveZone(ctx context.Context, point geo.Point) (Zone, error)
}

// Service holds the rules for accepting a Fleet execution command.
type Service struct {
	repo       *Repository
	verifier   AcceptanceVerifier
	zones      ZoneResolver
	enabled    bool
	killSwitch bool
}

// NewService builds the feature service.
//
// enabled and killSwitch are separate on purpose: the first is a rollout
// decision that can be planned, the second is an operational stop that has to
// work immediately and independently of whatever the rollout flag says.
func NewService(repo *Repository, verifier AcceptanceVerifier, zones ZoneResolver,
	enabled, killSwitch bool) *Service {
	return &Service{repo: repo, verifier: verifier, zones: zones,
		enabled: enabled, killSwitch: killSwitch}
}

// Available reports whether this surface is live.
func (s *Service) Available() bool { return s != nil && s.enabled && !s.killSwitch }

func fingerprint(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// validate refuses a request that could not produce a coherent trip.
//
// Every reference is required because each one is load-bearing later: without
// the assignment there is nothing to verify acceptance against, without the
// membership the settlement has no organization, and without the operation id
// Fleet cannot read back after a timeout.
func validate(in CreateRequest) error {
	required := map[string]string{
		"organizationId":           in.OrganizationID,
		"settlementOrganizationId": in.SettlementOrganizationID,
		"executingProviderId":      in.ExecutingProviderID,
		"organizationMembershipId": in.OrganizationMembershipID,
		"privateOrderId":           in.PrivateOrderID,
		"privateAssignmentId":      in.PrivateAssignmentID,
		"providerResponseId":       in.ProviderResponseID,
		"fleetOperationId":         in.FleetOperationID,
		"vehicleId":                in.VehicleID,
		"idempotencyKey":           in.IdempotencyKey,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("required private execution reference %s is missing", name)
		}
	}
	if in.ServiceCategory != ServiceCategoryTaxi {
		return fmt.Errorf("serviceCategory must be %s", ServiceCategoryTaxi)
	}
	if strings.TrimSpace(in.Route.PickupAddress) == "" || strings.TrimSpace(in.Route.DropoffAddress) == "" {
		return errors.New("route snapshot must name a pickup and a dropoff")
	}
	// Null island is a coordinate that parses and means nothing. A booking
	// placed there resolves to no zone at all, or worse, to whichever zone is
	// nearest the Gulf of Guinea.
	if in.Route.PickupLatitude == 0 && in.Route.PickupLongitude == 0 {
		return errors.New("route snapshot must carry a real pickup coordinate")
	}
	return nil
}

// Create executes Fleet's command.
//
// The order is deliberate: cheap local refusals first, then the idempotency
// replay, then the authoritative check against Fleet, and only then the
// database write. Verifying acceptance before the replay check would make
// every retry re-ask Fleet a question already answered, turning a Fleet outage
// into a Taxi outage for work already accepted.
func (s *Service) Create(ctx context.Context, in CreateRequest) (View, error) {
	if !s.Available() {
		return View{}, ErrDisabled
	}
	if err := validate(in); err != nil {
		return View{}, err
	}

	// The fingerprint covers the request's meaning, not its envelope: the
	// idempotency key identifies the operation and the correlation id only
	// traces it, so neither belongs in the digest of what was asked for.
	digestable := in
	digestable.IdempotencyKey = ""
	digestable.CorrelationID = ""
	digest := fingerprint(digestable)

	if prior, err := s.repo.GetKey(ctx, in.IdempotencyKey); err == nil {
		if prior.RequestFingerprint != digest {
			return View{}, ErrConflict
		}
		return prior.View, nil
	} else if !errors.Is(err, ErrNotFound) {
		return View{}, err
	}

	if s.verifier == nil {
		return View{}, ErrDisabled
	}
	acceptance, err := s.verifier.Verify(ctx, in.PrivateAssignmentID)
	if err != nil {
		return View{}, err
	}
	// Every field is compared, not just the assignment id. A partial match
	// would mean Fleet accepted a different provider, a different
	// organization, or a different response than the one this command names --
	// and executing it anyway would dispatch a driver nobody agreed to.
	if acceptance.AssignmentID != in.PrivateAssignmentID ||
		acceptance.OrganizationID != in.OrganizationID ||
		acceptance.ProviderID != in.ExecutingProviderID ||
		acceptance.ProviderService != ServiceCategoryTaxi ||
		acceptance.OrganizationMembershipID != in.OrganizationMembershipID ||
		acceptance.ProviderResponseID != in.ProviderResponseID ||
		acceptance.ResponseStatus != "ACCEPTED" {
		return View{}, errors.New("provider acceptance does not match private execution")
	}

	if s.zones == nil {
		return View{}, ErrDisabled
	}
	zone, err := s.zones.ResolveZone(ctx, geo.Point{
		Lat: in.Route.PickupLatitude, Lng: in.Route.PickupLongitude})
	if err != nil {
		return View{}, ErrUnserviceable
	}

	stored, err := s.repo.Create(ctx, in, digest, zone)
	if errors.Is(err, ErrConflict) {
		// Two concurrent retries of the same command: the loser reads what the
		// winner wrote rather than reporting a conflict for work that did
		// succeed.
		prior, readErr := s.repo.GetKey(ctx, in.IdempotencyKey)
		if readErr != nil {
			return View{}, readErr
		}
		if prior.RequestFingerprint != digest {
			return View{}, ErrConflict
		}
		return prior.View, nil
	}
	if err != nil {
		return View{}, err
	}
	return stored.View, nil
}

// Read is Fleet's authoritative read-back after an ambiguous create.
//
// It stays available even when the feature is disabled. Turning private
// execution off must not hide executions it already produced: Fleet would read
// nothing, conclude the create never happened, and retry into a duplicate the
// moment the flag came back on.
func (s *Service) Read(ctx context.Context, operationID string) (View, error) {
	stored, err := s.repo.GetOperation(ctx, operationID)
	return stored.View, err
}

// ReadByBooking resolves a mapping from the taxi booking it created.
func (s *Service) ReadByBooking(ctx context.Context, bookingID string) (View, error) {
	stored, err := s.repo.GetBooking(ctx, bookingID)
	return stored.View, err
}

// Cancel withdraws a corporate trip.
func (s *Service) Cancel(ctx context.Context, operationID string, in CancelRequest) (CancelView, error) {
	if !s.Available() {
		return CancelView{}, ErrDisabled
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		return CancelView{}, errors.New("idempotency key is required")
	}
	digest := fingerprint(map[string]string{"operationId": operationID, "reason": in.Reason})
	return s.repo.Cancel(ctx, operationID, in.IdempotencyKey, digest, in.Reason)
}

// ReadCancellation answers whether a cancellation landed.
//
// Like Read it stays available while disabled, because this is precisely the
// call Fleet makes when it does not know what happened, and answering
// "unavailable" to that question is answering nothing.
func (s *Service) ReadCancellation(ctx context.Context, operationID string) (CancelView, error) {
	stored, err := s.repo.GetOperation(ctx, operationID)
	if err != nil {
		return CancelView{}, err
	}
	status := stored.CancellationStatus
	if status == CancellationConfirmed {
		status = StatusCancelled
	}
	return CancelView{Status: status, SourceVersion: stored.SourceVersion}, nil
}
