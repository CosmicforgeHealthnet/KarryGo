// Package commercemission executes a haul for an already-paid commerce order
// (Vendor Service Section 17F).
//
// It is the execution counterpart to this service's commercequote package.
// That package answers "what would this cost"; this one is told to actually
// run it, for a fee the customer has ALREADY been charged under an approved
// commerce quote.
//
// Two boundaries this package must never cross:
//
//   - It does NOT re-price. The agreed fee is copied onto the booking verbatim.
//     Hauling's own current price is irrelevant here: the customer's charge is
//     fixed by an approved quote, and any difference is an internal platform
//     cost. Re-pricing would silently break the commerce total-price guarantee.
//   - It moves no money and holds no escrow. Commerce already charged the
//     customer through payment-wallet-service, so the booking is created with
//     the commerce payment reference rather than holding a second time.
package commercemission

import (
	"context"
	"errors"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/commercemission"
)

// Matcher starts provider matching for a created booking. It is a port so this
// package never learns how matching works.
type Matcher interface {
	StartCommerceMatching(
		ctx context.Context, bookingID string, pickupLat, pickupLng float64,
	)
}

// Store persists missions and the bookings they execute as.
type Store interface {
	FindByMissionRef(ctx context.Context, missionRef string) (Record, bool, error)
	FindByOperationRef(ctx context.Context, operationRef string) (Record, bool, error)

	// CreateMissionWithBooking writes the booking and the mission in ONE
	// transaction, so a mission never names a booking that does not exist.
	CreateMissionWithBooking(ctx context.Context, input CreateInput) (Record, error)

	MarkRefused(ctx context.Context, input RefusalInput) (Record, error)
	Cancel(ctx context.Context, input CancelInput) (Record, error)
}

// Options configures the service.
type Options struct {
	Store   Store
	Matcher Matcher
	Now     func() time.Time
}

// Service executes commerce missions.
type Service struct {
	store   Store
	matcher Matcher
	now     func() time.Time
}

// New builds the service.
func New(options Options) *Service {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Service{store: options.Store, matcher: options.Matcher, now: now}
}

// Ready reports whether the service can execute a mission at all.
func (s *Service) Ready() bool {
	return s != nil && s.store != nil && s.matcher != nil
}

// Create executes one commerce mission.
//
// Idempotency is the caller's mission reference, which is what makes it safe
// for a durable workflow on the commerce side to run this step more than once.
func (s *Service) Create(
	ctx context.Context, request commercemission.Request,
) (commercemission.Mission, error) {
	if !s.Ready() {
		return commercemission.Mission{}, apperrors.Unavailable(
			"Commerce mission execution is unavailable.", nil)
	}
	if reason, ok := request.Validate(); !ok {
		return commercemission.Mission{}, apperrors.BadRequest(reason, nil)
	}
	if request.Service != commercemission.ServiceHauling {
		// Hauling executes hauling missions. Being asked for another
		// authority's work is a caller error, not something to attempt.
		return commercemission.Mission{}, apperrors.BadRequest(
			"This authority executes HAULING missions only.", nil)
	}

	fingerprint := request.Fingerprint()
	if existing, found, err := s.store.FindByOperationRef(ctx, request.OperationReference()); err != nil {
		return commercemission.Mission{}, err
	} else if found {
		if strings.TrimSpace(existing.RequestFingerprint) != "" &&
			existing.RequestFingerprint != fingerprint {
			return commercemission.Mission{}, apperrors.Conflict(
				"Mission operation already exists for different terms.", nil)
		}
		return existing.Mission(), nil
	}

	// Work hauling cannot serve is REFUSED with a reason rather than accepted
	// and silently failed later, when the customer is already waiting.
	if refusal, refused := s.refuse(request); refused {
		record, err := s.store.MarkRefused(ctx, RefusalInput{
			Request:            request,
			RequestFingerprint: fingerprint,
			RefusalCode:        refusal,
			CreatedAt:          s.now().UTC(),
		})
		if err != nil {
			return commercemission.Mission{}, err
		}
		return record.Mission(), nil
	}

	record, err := s.store.CreateMissionWithBooking(ctx, CreateInput{
		Request:            request,
		RequestFingerprint: fingerprint,
		CreatedAt:          s.now().UTC(),
	})
	if err != nil {
		return commercemission.Mission{}, err
	}

	// Matching starts only after the mission is durable, so a crash here
	// leaves a mission a retry can find rather than a provider chasing a
	// booking nothing recorded.
	s.matcher.StartCommerceMatching(
		ctx, record.BookingID, primaryPickup(request).Latitude, primaryPickup(request).Longitude,
	)

	return record.Mission(), nil
}

func (s *Service) Read(ctx context.Context, operationRef string) (commercemission.Mission, error) {
	if !s.Ready() {
		return commercemission.Mission{}, apperrors.Unavailable(
			"Commerce mission execution is unavailable.", nil)
	}
	record, found, err := s.store.FindByOperationRef(ctx, strings.TrimSpace(operationRef))
	if err != nil {
		return commercemission.Mission{}, err
	}
	if !found {
		return commercemission.Mission{}, apperrors.NotFound("Commerce mission was not found.", nil)
	}
	return record.Mission(), nil
}

func (s *Service) Cancel(ctx context.Context, request commercemission.CancelRequest) (commercemission.Mission, error) {
	if !s.Ready() {
		return commercemission.Mission{}, apperrors.Unavailable(
			"Commerce mission execution is unavailable.", nil)
	}
	operationRef := strings.TrimSpace(request.OperationRef)
	if operationRef == "" {
		operationRef = strings.TrimSpace(request.MissionRef)
	}
	if operationRef == "" {
		return commercemission.Mission{}, apperrors.BadRequest("operation_ref is required.", nil)
	}
	record, err := s.store.Cancel(ctx, CancelInput{
		OperationRef: operationRef,
		ReasonCode:   request.ReasonCode,
		CancelledAt:  s.now().UTC(),
	})
	if errors.Is(err, ErrMissionNotFound) {
		return commercemission.Mission{}, apperrors.NotFound("Commerce mission was not found.", nil)
	}
	if err != nil {
		return commercemission.Mission{}, err
	}
	return record.Mission(), nil
}

// refuse decides whether hauling can serve this mission.
func (s *Service) refuse(request commercemission.Request) (string, bool) {
	switch {
	case request.VehicleClass != "" &&
		request.VehicleClass != commercemission.VehicleTruck &&
		request.VehicleClass != commercemission.VehicleVan:
		// Hauling runs trucks and vans. A motorcycle mission belongs to
		// dispatch and is never silently carried on a truck.
		return "VEHICLE_CLASS_NOT_SERVED", true
	case request.Load.WeightGrams <= 0:
		// A haul with no weight cannot be matched to a truck class.
		return "LOAD_WEIGHT_REQUIRED", true
	case strings.TrimSpace(request.Dropoff.Phone) == "":
		return "RECIPIENT_CONTACT_REQUIRED", true
	}
	return "", false
}

// cargoWeightKg converts the contract's grams to the kilograms hauling's own
// booking and matching logic work in. It affects matching, never the fee.
func cargoWeightKg(weightGrams int64) int {
	kilograms := weightGrams / 1000
	if kilograms < 1 {
		return 1
	}
	return int(kilograms)
}

func receiverName(request commercemission.Request) string {
	if name := strings.TrimSpace(request.Dropoff.Contact); name != "" {
		return name
	}
	return "Commerce recipient"
}

func cargoDescription(request commercemission.Request) string {
	if description := strings.TrimSpace(request.Load.Description); description != "" {
		return description
	}
	return "Commerce consignment"
}

// ErrMissionNotFound is returned when a mission reference names nothing.
var ErrMissionNotFound = errors.New("commerce mission not found")
