package app

import (
	"context"
	"errors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	crmmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	crmrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	ordersmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/models"
	ordersusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/usecases"
	catalogmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/models"
	catalogrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/repositories"
)

// The Section 15 orders feature depends on Section 14 CRM and the Section 15
// service catalogue only through ordersusecases.CRMPort/CatalogPort — never
// by importing crm/repositories or servicecatalog/repositories directly from
// inside the orders package. These adapters are the one place, in the
// composition root, that is allowed to know about both concrete
// repositories and translate between their domain types.

// ordersCRMAdapter implements ordersusecases.CRMPort over the Section 14 CRM
// repository.
type ordersCRMAdapter struct{ repo *crmrepos.Repository }

// NewOrdersCRMAdapter builds the Section 15 -> Section 14 CRM port adapter.
func NewOrdersCRMAdapter(repo *crmrepos.Repository) ordersusecases.CRMPort {
	return ordersCRMAdapter{repo: repo}
}

func (a ordersCRMAdapter) GetCustomer(ctx context.Context, q database.Querier,
	organizationID, customerID string) (ordersusecases.CustomerInfo, error) {
	customer, err := a.repo.GetCustomer(ctx, q, organizationID, customerID)
	if err != nil {
		if errors.Is(err, crmrepos.ErrNotFound) {
			return ordersusecases.CustomerInfo{}, ordersusecases.ErrPortNotFound
		}
		return ordersusecases.CustomerInfo{}, err
	}
	blocked, reason := customer.BlocksNewCommitment()
	return ordersusecases.CustomerInfo{
		ID: customer.ID, BranchID: customer.BranchID, LegalName: customer.LegalName,
		DisplayName: customer.DisplayName, CustomerType: customer.CustomerType,
		Blocked: blocked, BlockedReason: reason,
	}, nil
}

func (a ordersCRMAdapter) CreditHoldBlocks(ctx context.Context, q database.Querier,
	organizationID, customerID string) (bool, error) {
	profile, err := a.repo.GetCreditProfile(ctx, q, organizationID, customerID)
	if errors.Is(err, crmrepos.ErrNotFound) {
		// No credit policy set is not a refusal Fleet invents on the
		// organization's behalf.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return crmmodels.BlockingHold(profile.HoldStatus), nil
}

func (a ordersCRMAdapter) GetContract(ctx context.Context, q database.Querier,
	organizationID, contractID string) (ordersusecases.ContractInfo, error) {
	contract, err := a.repo.GetContract(ctx, q, organizationID, contractID)
	if err != nil {
		if errors.Is(err, crmrepos.ErrNotFound) {
			return ordersusecases.ContractInfo{}, ordersusecases.ErrPortNotFound
		}
		return ordersusecases.ContractInfo{}, err
	}
	return ordersusecases.ContractInfo{
		ID: contract.ID, BranchID: contract.BranchID, CustomerID: contract.CustomerID,
		ActiveVersionID: contract.ActiveVersionID, Active: contract.Status == crmmodels.ContractActive,
		EffectiveFrom: contract.EffectiveFrom, EffectiveTo: contract.EffectiveTo,
	}, nil
}

func (a ordersCRMAdapter) GetQuotation(ctx context.Context, q database.Querier,
	organizationID, quotationID string) (ordersusecases.QuotationInfo, error) {
	quotation, err := a.repo.GetQuotation(ctx, q, organizationID, quotationID)
	if err != nil {
		if errors.Is(err, crmrepos.ErrNotFound) {
			return ordersusecases.QuotationInfo{}, ordersusecases.ErrPortNotFound
		}
		return ordersusecases.QuotationInfo{}, err
	}
	return ordersusecases.QuotationInfo{
		ID: quotation.ID, CustomerID: quotation.CustomerID, AcceptedVersionID: quotation.AcceptedVersionID,
	}, nil
}

func (a ordersCRMAdapter) GetConversionIntent(ctx context.Context, q database.Querier,
	organizationID, acceptedVersionID string) (ordersusecases.ConversionIntentInfo, error) {
	intent, err := a.repo.GetConversionIntent(ctx, q, organizationID, acceptedVersionID)
	if err != nil {
		if errors.Is(err, crmrepos.ErrNotFound) {
			return ordersusecases.ConversionIntentInfo{}, ordersusecases.ErrPortNotFound
		}
		return ordersusecases.ConversionIntentInfo{}, err
	}
	return ordersusecases.ConversionIntentInfo{ID: intent.ID, QuotationID: intent.QuotationID}, nil
}

func (a ordersCRMAdapter) ConversionIntentConsumption(ctx context.Context, q database.Querier,
	organizationID, intentID string) (bool, string, error) {
	consumed, privateOrderID, err := a.repo.ConversionIntentConsumption(ctx, q, organizationID, intentID)
	if err != nil {
		if errors.Is(err, crmrepos.ErrNotFound) {
			return false, "", ordersusecases.ErrPortNotFound
		}
		return false, "", err
	}
	return consumed, privateOrderID, nil
}

func (a ordersCRMAdapter) ConsumeConversionIntent(ctx context.Context, q database.Querier,
	organizationID, intentID, privateOrderID string) error {
	err := a.repo.ConsumeConversionIntent(ctx, q, organizationID, intentID, privateOrderID)
	if errors.Is(err, crmrepos.ErrNotFound) {
		return ordersusecases.ErrPortNotFound
	}
	return err
}

// ordersCatalogAdapter implements ordersusecases.CatalogPort over the
// Section 15 service catalogue repository.
type ordersCatalogAdapter struct{ repo *catalogrepos.Repository }

// NewOrdersCatalogAdapter builds the orders -> service catalogue port adapter.
func NewOrdersCatalogAdapter(repo *catalogrepos.Repository) ordersusecases.CatalogPort {
	return ordersCatalogAdapter{repo: repo}
}

func (a ordersCatalogAdapter) GetCatalogItem(ctx context.Context, q database.Querier,
	organizationID, itemID string) (ordersusecases.CatalogItemInfo, error) {
	item, err := a.repo.Get(ctx, q, organizationID, itemID)
	if err != nil {
		if errors.Is(err, catalogrepos.ErrNotFound) {
			return ordersusecases.CatalogItemInfo{}, ordersusecases.ErrPortNotFound
		}
		return ordersusecases.CatalogItemInfo{}, err
	}
	return ordersusecases.CatalogItemInfo{
		ID: item.ID, BranchID: item.BranchID, Active: item.Status == catalogmodels.StatusActive,
		RequiredFieldsSchema: ordersmodels.JSONMap(item.RequiredFieldsSchema),
		Snapshot:             ordersmodels.JSONMap(item.Snapshot()),
	}, nil
}
