package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	assetsfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets"
	eligibilitymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	eligibilityusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/usecases"
	ordersmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/models"
	ordersrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/repositories"
	privatemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	privaterepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/repositories"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	shipmentsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/models"
	shipmentsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/repositories"
	workforcefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

type privateWorkAdapter struct {
	orders    *ordersrepos.Repository
	shipments *shipmentsrepos.Repository
}

func (a privateWorkAdapter) Resolve(ctx context.Context, q database.Querier, organizationID, origin, id string) (privateusecases.WorkInfo, error) {
	var orderID, shipmentID string
	switch origin {
	case "PRIVATE_ORDER":
		orderID = id
	case "PRIVATE_SHIPMENT":
		shipment, err := a.shipments.Get(ctx, q, organizationID, id)
		if err != nil {
			return privateusecases.WorkInfo{}, err
		}
		if shipment.Status != shipmentsmodels.ShipmentReady && shipment.Status != shipmentsmodels.ShipmentPlanned {
			return privateusecases.WorkInfo{BranchID: shipment.BranchID}, nil
		}
		shipmentID, orderID = shipment.ID, shipment.OrderID
	case "ROUTE_STOP":
		var sourceOrderStopID, routeOrderID, routeShipmentID string
		err := q.QueryRow(ctx, `SELECT COALESCE(source_order_stop_id,''),COALESCE(order_id,''),COALESCE(shipment_id,'')
			FROM fleet_partner_route_stops WHERE organization_id=$1 AND id=$2`, organizationID, id).
			Scan(&sourceOrderStopID, &routeOrderID, &routeShipmentID)
		if err != nil {
			return privateusecases.WorkInfo{}, err
		}
		orderID, shipmentID = routeOrderID, routeShipmentID
		if shipmentID == "" && sourceOrderStopID != "" {
			err = q.QueryRow(ctx, `SELECT id,order_id FROM fleet_partner_shipments
				WHERE organization_id=$1 AND (origin_stop_id=$2 OR destination_stop_id=$2)
					AND status IN('READY','PLANNED') ORDER BY created_at LIMIT 1`, organizationID, sourceOrderStopID).Scan(&shipmentID, &orderID)
			if err != nil {
				return privateusecases.WorkInfo{}, err
			}
		}
	default:
		return privateusecases.WorkInfo{}, errors.New("unsupported private work origin")
	}
	order, err := a.orders.Get(ctx, q, organizationID, orderID)
	if err != nil {
		return privateusecases.WorkInfo{}, err
	}
	if shipmentID == "" {
		err = q.QueryRow(ctx, `SELECT id FROM fleet_partner_shipments
			WHERE organization_id=$1 AND order_id=$2 AND status IN('READY','PLANNED')
			ORDER BY created_at LIMIT 1`, organizationID, orderID).Scan(&shipmentID)
		if err != nil {
			return privateusecases.WorkInfo{}, err
		}
	}
	shipment, err := a.shipments.Get(ctx, q, organizationID, shipmentID)
	if err != nil {
		return privateusecases.WorkInfo{}, err
	}
	pickup, err := a.orders.GetStop(ctx, q, organizationID, shipment.OriginStopID)
	if err != nil {
		return privateusecases.WorkInfo{}, err
	}
	dropoff, err := a.orders.GetStop(ctx, q, organizationID, shipment.DestinationStopID)
	if err != nil {
		return privateusecases.WorkInfo{}, err
	}
	payload := privateExecutionPayload(pickup, dropoff, shipment)
	ready := (order.Status == ordersmodels.StatusConfirmed || order.Status == ordersmodels.StatusPlanned) &&
		pickup.Latitude != nil && pickup.Longitude != nil && dropoff.Latitude != nil && dropoff.Longitude != nil &&
		payload.PickupAddress != "" && payload.DropoffAddress != ""
	return privateusecases.WorkInfo{
		PrivateOrderID: order.ID, ShipmentID: shipmentID, BranchID: order.BranchID,
		ContractReference: order.ContractID, QuotationReference: order.QuotationID,
		Ready: ready, ExecutionPayload: payload,
	}, nil
}

func privateExecutionPayload(pickup, dropoff ordersmodels.OrderStop, shipment shipmentsmodels.Shipment) privatemodels.ExecutionPayload {
	value := func(values ordersmodels.JSONMap, keys ...string) string {
		for _, key := range keys {
			if raw, ok := values[key]; ok {
				if text := strings.TrimSpace(fmt.Sprint(raw)); text != "" && text != "<nil>" {
					return text
				}
			}
		}
		return ""
	}
	number := func(values shipmentsmodels.JSONMap, keys ...string) int {
		for _, key := range keys {
			switch raw := values[key].(type) {
			case float64:
				return int(raw)
			case int:
				return raw
			case int64:
				return int(raw)
			}
		}
		return 0
	}
	payload := privatemodels.ExecutionPayload{
		PickupAddress:    value(pickup.AddressSnapshot, "formattedAddress", "address", "line1"),
		DropoffAddress:   value(dropoff.AddressSnapshot, "formattedAddress", "address", "line1"),
		CargoDescription: value(ordersmodels.JSONMap(shipment.CargoProfile), "description", "cargoDescription"),
		CargoType:        value(ordersmodels.JSONMap(shipment.CargoProfile), "cargoType", "type"),
		CargoWeightKG:    number(shipment.CargoProfile, "weightKg", "cargoWeightKg"),
	}
	if pickup.Latitude != nil {
		payload.PickupLatitude = *pickup.Latitude
	}
	if pickup.Longitude != nil {
		payload.PickupLongitude = *pickup.Longitude
	}
	if dropoff.Latitude != nil {
		payload.DropoffLatitude = *dropoff.Latitude
	}
	if dropoff.Longitude != nil {
		payload.DropoffLongitude = *dropoff.Longitude
	}
	return payload
}

type privateEligibilityAdapter struct {
	service      *eligibilityusecases.Service
	workforce    *workforcefeature.Service
	assets       *assetsfeature.Service
	restrictions interface {
		Restricted(context.Context, string, string, string) (bool, error)
	}
}

func (a privateEligibilityAdapter) Validate(ctx context.Context, organizationID, membershipID, providerID, executionService, vehicleReference string) (bool, error) {
	if a.restrictions == nil {
		return false, errors.New("restriction authority is unavailable")
	}
	for _, subject := range [][2]string{{"ORGANIZATION", organizationID}, {"PROVIDER", membershipID}, {"ASSET", vehicleReference}} {
		if subject[1] == "" {
			continue
		}
		restricted, err := a.restrictions.Restricted(ctx, organizationID, subject[0], subject[1])
		if err != nil || restricted {
			return false, err
		}
	}
	if a.service == nil {
		return false, errors.New("eligibility service is unavailable")
	}
	decision, err := a.service.Evaluate(ctx, eligibilityusecases.EvaluateInput{
		Actor:           eligibilityusecases.Actor{OrganizationID: organizationID, ActorType: eligibilitymodels.ActorSystem, ActorID: "private-assignment-gate"},
		OperationType:   eligibilitymodels.OperationAssignment,
		SubjectType:     eligibilitymodels.SubjectProvider,
		SubjectID:       membershipID,
		MembershipID:    membershipID,
		ServiceCategory: executionService,
		AssetReference:  vehicleReference,
		CorrelationID:   providerID,
	})
	if err != nil || decision == nil || decision.Status != eligibilitymodels.StatusEligible {
		return false, err
	}
	if a.workforce == nil || a.assets == nil {
		return false, errors.New("workforce or asset eligibility service is unavailable")
	}
	workforceDecision, err := a.workforce.Evaluate(ctx, organizationID, membershipID, executionService, "", time.Now())
	if err != nil || !workforceDecision.Eligible {
		return false, err
	}
	if vehicleReference != "" {
		assetDecision, assetErr := a.assets.Evaluate(ctx, organizationID, vehicleReference, membershipID, "", 0, executionService, time.Now())
		if assetErr != nil || !assetDecision.Eligible {
			return false, assetErr
		}
	}
	return true, nil
}

func BuildPrivateOperationsService(cfg config.Config, pool *pgxpool.Pool, publisher *events.DomainPublisher, metrics *observability.Metrics, workforce *workforcefeature.Service, assets *assetsfeature.Service, restrictions interface {
	Restricted(context.Context, string, string, string) (bool, error)
}) *privateusecases.Service {
	dispatchConfigured := cfg.DispatchServiceURL != "" && len(cfg.DispatchServiceSecret) > 0
	haulingConfigured := cfg.Clients["driver-hauling-service"].Configured()
	return privateusecases.NewService(privateusecases.Options{
		Pool:        pool,
		Repository:  privaterepos.NewRepository(pool),
		Work:        privateWorkAdapter{orders: ordersrepos.NewRepository(pool), shipments: shipmentsrepos.NewRepository(pool)},
		Eligibility: privateEligibilityAdapter{service: eligibilityusecases.Build(cfg, pool, metrics), workforce: workforce, assets: assets, restrictions: restrictions},
		Events:      publisher,
		Flags: privateusecases.Flags{
			Enabled:         cfg.PrivateOrdersEnabled,
			WritesEnabled:   cfg.PrivateAssignmentsWritesEnabled,
			WorkerEnabled:   cfg.PrivateAssignmentsWorkerEnabled || cfg.PrivateExecutionWorkerEnabled,
			DispatchEnabled: cfg.PrivateDispatchExecutionEnabled && dispatchConfigured,
			TaxiEnabled:     false,
			HaulingEnabled:  cfg.PrivateHaulingExecutionEnabled && haulingConfigured,
			KillSwitch:      cfg.PrivateOperationsKillSwitch || cfg.IntegrationsKillSwitch,
		},
		ResponseTTL: cfg.PrivateAssignmentResponseTTL,
	})
}
