// Package commercemission executes a delivery for an already-paid commerce
// order (Vendor Service Section 17F).
//
// It is the execution counterpart to this service's commercequote package.
// That package answers "what would this cost"; this one is told to actually
// run it, for a fee the customer has ALREADY been charged under an approved
// commerce quote.
//
// Two boundaries this package must never cross:
//
//   - It does NOT re-price. The agreed fee is copied onto the booking verbatim.
//     Dispatch's own current price is irrelevant here: the customer's charge is
//     fixed by an approved quote, and any difference is an internal platform
//     cost. Re-pricing would silently break the commerce total-price guarantee.
//   - It moves no money and holds no escrow. Commerce already charged the
//     customer through payment-wallet-service, so the booking is created
//     already paid rather than holding a second time.
package commercemission

import (
	"context"
	"errors"
	"strings"
	"time"

	requestfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/request"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/commercemission"
)

// Broadcaster starts rider matching for a created booking. It is a port so
// this package never learns how matching works.
type Broadcaster interface {
	StartBroadcast(
		ctx context.Context, event requestfeature.BookingDispatchCreatedEvent,
	) (requestfeature.RequestBroadcast, error)
}

// Store persists missions and the bookings they execute as.
type Store interface {
	// FindByMissionRef returns an existing mission, so a retried request
	// returns the same answer rather than dispatching a second rider.
	FindByMissionRef(ctx context.Context, missionRef string) (Record, bool, error)

	// CreateMissionWithBooking writes the booking and the mission in ONE
	// transaction, so a mission never names a booking that does not exist and
	// a booking is never orphaned by a failed mission write.
	CreateMissionWithBooking(ctx context.Context, input CreateInput) (Record, error)

	// MarkRefused records a refusal, so commerce can read a durable reason
	// rather than only receiving one in a response it might not survive.
	MarkRefused(ctx context.Context, input RefusalInput) (Record, error)
}

// Options configures the service.
type Options struct {
	Store       Store
	Broadcast   Broadcaster
	ServiceName string
	Now         func() time.Time
}

// Service executes commerce missions.
type Service struct {
	store     Store
	broadcast Broadcaster
	now       func() time.Time
}

// New builds the service.
func New(options Options) *Service {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Service{store: options.Store, broadcast: options.Broadcast, now: now}
}

// Ready reports whether the service can execute a mission at all.
func (s *Service) Ready() bool {
	return s != nil && s.store != nil && s.broadcast != nil
}

// Create executes one commerce mission.
//
// Idempotency is the caller's mission reference: the same reference always
// returns the same mission, which is what makes it safe for a durable workflow
// on the commerce side to run this step more than once.
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
	if request.Service != commercemission.ServiceDispatch {
		// Dispatch executes dispatch missions. Being asked for another
		// authority's work is a caller error, not something to attempt.
		return commercemission.Mission{}, apperrors.BadRequest(
			"This authority executes DISPATCH missions only.", nil)
	}

	if existing, found, err := s.store.FindByMissionRef(ctx, request.MissionRef); err != nil {
		return commercemission.Mission{}, err
	} else if found {
		return existing.Mission(), nil
	}

	// Dispatch carries a motorcycle load. A mission it cannot physically serve
	// is REFUSED with a reason rather than accepted and silently failed later.
	if refusal, refused := s.refuse(request); refused {
		record, err := s.store.MarkRefused(ctx, RefusalInput{
			Request:     request,
			RefusalCode: refusal,
			CreatedAt:   s.now().UTC(),
		})
		if err != nil {
			return commercemission.Mission{}, err
		}
		return record.Mission(), nil
	}

	record, err := s.store.CreateMissionWithBooking(ctx, CreateInput{
		Request:   request,
		CreatedAt: s.now().UTC(),
	})
	if err != nil {
		return commercemission.Mission{}, err
	}

	// Matching starts only after the mission is durable, so a crash here
	// leaves a mission a retry can find rather than a rider chasing a booking
	// nothing recorded.
	event := requestfeature.BookingDispatchCreatedEvent{
		BookingID:      record.BookingID,
		CustomerID:     request.CustomerRef,
		PickupLat:      request.Pickup.Latitude,
		PickupLng:      request.Pickup.Longitude,
		DropoffLat:     request.Dropoff.Latitude,
		DropoffLng:     request.Dropoff.Longitude,
		PickupAddress:  request.Pickup.Address,
		DropoffAddress: request.Dropoff.Address,
		// The agreed fee, not a recomputed one.
		FareAmount:    request.AgreedFeeMinor,
		ServiceType:   "dispatch",
		PaymentMethod: "wallet",
		Currency:      request.Currency,
		ReceiverName:  receiverName(request),
		ReceiverPhone: request.Dropoff.Phone,
		PackageDesc:   request.Load.Description,
		PackageWeight: float64(request.Load.WeightGrams) / 1000,
		PackageSize:   packageSize(request.Load.WeightGrams),
		IsFragile:     request.Load.Fragile,
		ServiceTier:   "standard",
		OccurredAt:    s.now().UTC(),
	}
	if _, err = s.broadcast.StartBroadcast(ctx, event); err != nil {
		// The mission and booking are already durable. Reporting the failure
		// lets the commerce workflow retry the same mission reference, which
		// finds this mission and re-broadcasts rather than creating a second.
		return commercemission.Mission{}, apperrors.Unavailable(
			"Rider matching is temporarily unavailable.", err)
	}

	return record.Mission(), nil
}

// refuse decides whether dispatch can physically serve this mission.
//
// A refusal is an honest answer commerce can act on. Accepting work that
// cannot be done would look like success and fail much later, when the
// customer is already waiting.
func (s *Service) refuse(request commercemission.Request) (string, bool) {
	switch {
	case request.VehicleClass != "" &&
		request.VehicleClass != commercemission.VehicleMotorcycle:
		// Dispatch is a motorcycle fleet. A van or truck mission belongs to
		// another authority and is never silently carried on a bike.
		return "VEHICLE_CLASS_NOT_SERVED", true
	case request.Load.WeightGrams > maxRiderLoadGrams:
		return "LOAD_EXCEEDS_RIDER_CAPACITY", true
	case strings.TrimSpace(request.Dropoff.Phone) == "":
		// A rider cannot complete a delivery it cannot hand over.
		return "RECIPIENT_CONTACT_REQUIRED", true
	}
	return "", false
}

// maxRiderLoadGrams is the physical envelope of a motorcycle courier, matching
// what this service's own quote surface already promises.
const maxRiderLoadGrams = 25_000

func receiverName(request commercemission.Request) string {
	if name := strings.TrimSpace(request.Dropoff.Contact); name != "" {
		return name
	}
	return "Commerce recipient"
}

// packageSize maps a real weight onto this service's own size vocabulary. It
// affects handling, never the fee: the fee is the agreed one.
func packageSize(weightGrams int64) string {
	switch {
	case weightGrams > 10_000:
		return "large"
	case weightGrams > 3_000:
		return "medium"
	default:
		return "small"
	}
}

// ErrMissionNotFound is returned when a mission reference names nothing.
var ErrMissionNotFound = errors.New("commerce mission not found")
