package app

import (
	"context"
	"errors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	ordersmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/models"
	ordersrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/repositories"
	shipmentsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/usecases"
)

// ordersPortAdapter implements shipmentsusecases.OrdersPort over the
// Section 15 private-orders repository. Shipments depends on orders only
// through this narrow port — never by importing orders/repositories
// directly — matching the CRMPort/CatalogPort pattern orders itself uses for
// its own Section 14 dependency.
type ordersPortAdapter struct{ repo *ordersrepos.Repository }

// NewShipmentsOrdersAdapter builds the shipments -> orders port adapter.
func NewShipmentsOrdersAdapter(repo *ordersrepos.Repository) shipmentsusecases.OrdersPort {
	return ordersPortAdapter{repo: repo}
}

func (a ordersPortAdapter) GetOrder(ctx context.Context, q database.Querier,
	organizationID, orderID string) (shipmentsusecases.OrderInfo, error) {
	order, err := a.repo.Get(ctx, q, organizationID, orderID)
	if err != nil {
		if errors.Is(err, ordersrepos.ErrNotFound) {
			return shipmentsusecases.OrderInfo{}, shipmentsusecases.ErrPortNotFound
		}
		return shipmentsusecases.OrderInfo{}, err
	}
	confirmed := order.Status != ordersmodels.StatusDraft &&
		order.Status != ordersmodels.StatusAwaitingApproval &&
		!ordersmodels.Terminal(order.Status)
	return shipmentsusecases.OrderInfo{ID: order.ID, BranchID: order.BranchID, Confirmed: confirmed}, nil
}

func (a ordersPortAdapter) ListOrderStopIDs(ctx context.Context, q database.Querier,
	organizationID, orderID string) ([]string, error) {
	stops, err := a.repo.ListStops(ctx, q, organizationID, orderID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(stops))
	for _, stop := range stops {
		ids = append(ids, stop.ID)
	}
	return ids, nil
}
