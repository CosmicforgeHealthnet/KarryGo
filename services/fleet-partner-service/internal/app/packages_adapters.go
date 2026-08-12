package app

import (
	"context"
	"errors"
	"strings"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	packagesusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/usecases"
	shipmentsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/models"
	shipmentsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

type packagesShipmentsAdapter struct{ repo *shipmentsrepos.Repository }

func NewPackagesShipmentsAdapter(r *shipmentsrepos.Repository) packagesusecases.ShipmentsPort {
	return packagesShipmentsAdapter{r}
}

type packagesDeviceAdapter struct{ db database.Querier }

func NewPackagesDeviceAdapter(db database.Querier) packagesusecases.DevicePort {
	return packagesDeviceAdapter{db: db}
}

func (a packagesDeviceAdapter) ValidateTrustedDevice(ctx context.Context, t tenant.Context, reference string) (bool, error) {
	if t.ActorType == tenant.ActorTypePlatformService {
		// Provider-facing source services authenticate and revoke their own device
		// sessions before crossing the HMAC boundary. Fleet still requires the
		// asserted opaque device reference and independently enforces work scope.
		return strings.TrimSpace(reference) != "", nil
	}
	if t.ActorType != tenant.ActorTypeOrganizationUser || t.OrganizationUserID == "" {
		return false, nil
	}
	var trusted bool
	err := a.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_corporate_sessions
		WHERE id=$1 AND organization_id=$2 AND organization_user_id=$3 AND status='ACTIVE'
		AND revoked_at IS NULL AND refresh_expires_at>now())`, reference, t.OrganizationID,
		t.OrganizationUserID).Scan(&trusted)
	return trusted, err
}

type packagesProviderScopeAdapter struct{ db database.Querier }

func NewPackagesProviderScopeAdapter(db database.Querier) packagesusecases.ProviderScopePort {
	return packagesProviderScopeAdapter{db: db}
}

func (a packagesProviderScopeAdapter) ValidatePackageScope(ctx context.Context, t tenant.Context,
	shipmentID, _ string) (bool, error) {
	var allowed bool
	err := a.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_private_assignments
		WHERE organization_id=$1 AND shipment_id=$2 AND provider_id=$3
		AND status IN('ACCEPTED','READY_FOR_EXECUTION','IN_PROGRESS'))`, t.OrganizationID, shipmentID, t.ActorID).Scan(&allowed)
	return allowed, err
}
func (a packagesShipmentsAdapter) GetShipment(ctx context.Context, q database.Querier, org, id string) (packagesusecases.ShipmentInfo, error) {
	s, e := a.repo.Get(ctx, q, org, id)
	if errors.Is(e, shipmentsrepos.ErrNotFound) {
		return packagesusecases.ShipmentInfo{}, packagesusecases.ErrShipmentNotFound
	}
	return packagesusecases.ShipmentInfo{ID: s.ID, BranchID: s.BranchID, Status: s.Status, Editable: shipmentsmodels.ShipmentEditable(s.Status)}, e
}

type packagesMediaAdapter struct{ media platformclients.MediaContract }

func NewPackagesMediaAdapter(m platformclients.MediaContract) packagesusecases.MediaPort {
	if m == nil {
		return nil
	}
	return packagesMediaAdapter{m}
}
func (a packagesMediaAdapter) ValidateEvidence(ctx context.Context, t tenant.Context, id string) (bool, error) {
	state, e := a.media.GetFile(ctx, platformclients.CallMeta{OrganizationID: t.OrganizationID, RequestID: t.RequestID, CorrelationID: t.CorrelationID}, id)
	if e != nil {
		return false, e
	}
	safe := strings.EqualFold(state.ScanState, "SAFE") || strings.EqualFold(state.ScanState, "CLEAN")
	return state.OwnerID == t.OrganizationID && state.UploadState == "COMPLETED" && safe, nil
}
