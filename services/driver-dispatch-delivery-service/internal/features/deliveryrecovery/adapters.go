package deliveryrecovery

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/assignment"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/commercequote"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/serviceability"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"cosmicforge/logistics/shared/go/walletclient"
)

type TripHandoverInvalidator struct {
	db     *pgxpool.Pool
	secure HandoverInvalidator
}

func NewTripHandoverInvalidator(db *pgxpool.Pool, secure HandoverInvalidator) *TripHandoverInvalidator {
	return &TripHandoverInvalidator{db: db, secure: secure}
}
func (a *TripHandoverInvalidator) InvalidateForReceiverChange(ctx context.Context, tripID string) error {
	if a == nil || a.db == nil {
		return errors.New("handover authority unavailable")
	}
	if a.secure != nil {
		if err := a.secure.InvalidateForReceiverChange(ctx, tripID); err != nil {
			return err
		}
	}
	_, err := a.db.Exec(ctx, `UPDATE trips SET delivery_code='',delivery_code_hash='',delivery_code_expires_at=NULL,updated_at=now() WHERE id=$1::uuid`, tripID)
	return err
}

type Section6ReturnRouter struct {
	planner commercequote.RoutePlanner
	zones   serviceability.ZoneEvaluator
}

func NewSection6ReturnRouter(planner commercequote.RoutePlanner, zones serviceability.ZoneEvaluator) *Section6ReturnRouter {
	return &Section6ReturnRouter{planner: planner, zones: zones}
}

func (r *Section6ReturnRouter) ReturnRoute(ctx context.Context, trip TripContext) (RouteDecision, error) {
	if r == nil || r.planner == nil || r.zones == nil {
		return RouteDecision{}, errors.New("route authority unavailable")
	}
	returnPickup := serviceability.Point{Latitude: trip.DropoffLat, Longitude: trip.DropoffLng}
	returnDropoff := serviceability.Point{Latitude: trip.PickupLat, Longitude: trip.PickupLng}
	pickupInside, dropoffInside, restricted, available := r.zones.Evaluate(returnPickup, returnDropoff)
	if !available || !pickupInside || !dropoffInside || restricted {
		return RouteDecision{}, errors.New("return route is outside the approved operating zone")
	}
	plan, err := r.planner.Plan(ctx, []commercequote.RoutePoint{
		{Ref: "failed-delivery", Latitude: trip.DropoffLat, Longitude: trip.DropoffLng},
		{Ref: "return-sender", Latitude: trip.PickupLat, Longitude: trip.PickupLng},
	}, dispatchcommerce.ModeFastest)
	if err != nil {
		return RouteDecision{}, err
	}
	return RouteDecision{Reference: "return-route:" + trip.TripID + ":" + plan.EvidenceVersion, Provider: plan.Provider,
		EvidenceVersion: plan.EvidenceVersion, DistanceMeters: plan.DistanceMeters, DurationSeconds: plan.DurationSeconds,
		Snapshot: map[string]any{"provider": plan.Provider, "evidence_version": plan.EvidenceVersion, "distance_meters": plan.DistanceMeters,
			"duration_seconds": plan.DurationSeconds, "ordered_refs": plan.OrderedRefs}}, nil
}

type AssignmentReplacementAdapter struct{ service *assignment.Service }

func NewAssignmentReplacementAdapter(service *assignment.Service) *AssignmentReplacementAdapter {
	return &AssignmentReplacementAdapter{service: service}
}
func (a *AssignmentReplacementAdapter) ReconcileIncident(ctx context.Context, id string) (bool, error) {
	if a == nil || a.service == nil {
		return false, errors.New("assignment authority unavailable")
	}
	return a.service.ReconcileReplacement(ctx, id)
}
func (a *AssignmentReplacementAdapter) RequestReplacement(ctx context.Context, missionID, strategy, operation string) error {
	if a == nil || a.service == nil {
		return errors.New("assignment authority unavailable")
	}
	_, err := a.service.Start(ctx, assignment.StartRequest{MissionID: missionID, Strategy: assignment.Strategy(strategy),
		IdempotencyKey: operation, ReasonCode: "RECOVERY"}, false)
	return err
}
func (a *AssignmentReplacementAdapter) DispatchMission(ctx context.Context, missionID, operation string) error {
	if a == nil || a.service == nil {
		return errors.New("assignment authority unavailable")
	}
	_, err := a.service.Start(ctx, assignment.StartRequest{MissionID: missionID, Strategy: assignment.SequentialNearest,
		IdempotencyKey: operation, ReasonCode: "RETURN_EXECUTION"}, false)
	return err
}

type ConfiguredHubAuthority struct{ hubs map[string]string }

func NewConfiguredHubAuthority(spec string) *ConfiguredHubAuthority {
	a := &ConfiguredHubAuthority{hubs: map[string]string{}}
	for _, raw := range strings.Split(spec, ";") {
		parts := strings.SplitN(strings.TrimSpace(raw), "=", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
			a.hubs[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return a
}
func (a *ConfiguredHubAuthority) AuthorizeHub(_ context.Context, hubRef string) (string, error) {
	if a == nil {
		return "", apperrors.New(409, "HUB_AUTHORITY_UNAVAILABLE", "Approved hub hold is unavailable.", nil)
	}
	ref, ok := a.hubs[strings.TrimSpace(hubRef)]
	if !ok {
		return "", apperrors.New(409, "HUB_NOT_APPROVED", "This location is not an approved hold hub.", nil)
	}
	return ref, nil
}

type WalletRecoveryAuthority struct{ client walletclient.Client }

func NewWalletRecoveryAuthority(client walletclient.Client) *WalletRecoveryAuthority {
	return &WalletRecoveryAuthority{client: client}
}
func (a *WalletRecoveryAuthority) ValidateRecoveryAuthorization(ctx context.Context, reference, customerID string, minimum int64) error {
	intent, err := a.client.GetPayment(ctx, strings.TrimSpace(reference))
	if err != nil {
		return apperrors.Unavailable("Wallet authorization could not be verified.", err)
	}
	if intent.CustomerID != customerID || intent.AmountKobo < minimum || (intent.Status != "held" && intent.Status != "completed") {
		return apperrors.New(409, "FINANCIAL_AUTHORIZATION_MISMATCH", "Wallet authorization does not cover this recovery.", nil)
	}
	return nil
}
