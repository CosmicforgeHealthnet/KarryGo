// Section 25C: real domain adapters for the generic import framework's
// Adapter port (internal/features/bulkimports/usecases.Adapter). Each one
// wraps the owning domain's own real usecase — CreateProfile, CreateAsset,
// Receipt — so the generic engine never writes a domain table itself and
// never bypasses that domain's own validation, permissions or safety rules.
package app

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/shared/go/apperrors"

	assetsfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets"
	bulkmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/models"
	bulkusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse"
	workforcefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// branchOf recovers the import job's branch from the tenant context the
// generic worker built for it (tenantFor in the usecases package sets
// PermittedBranchIDs to exactly one branch, the job's own).
func branchOf(t tenant.Context) string {
	if len(t.PermittedBranchIDs) > 0 {
		return t.PermittedBranchIDs[0]
	}
	return ""
}

func str(data map[string]any, key string) string {
	v, _ := data[key].(string)
	return v
}

// numeric reads a float64 defensively: a value round-tripped through JSON is
// always float64, but a value set directly in a unit test may be an int.
func numeric(data map[string]any, key string) float64 {
	switch v := data[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	default:
		return 0
	}
}

func intPtr(data map[string]any, key string) *int {
	switch v := data[key].(type) {
	case float64:
		n := int(v)
		return &n
	case int:
		return &v
	default:
		return nil
	}
}

func timePtr(data map[string]any, key string) *time.Time {
	raw := str(data, key)
	if raw == "" {
		return nil
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return &parsed
	}
	if parsed, err := time.Parse("2006-01-02", raw); err == nil {
		return &parsed
	}
	return nil
}

// asDuplicateConflict maps a raw unique-violation from a domain usecase that
// does not classify it itself (workforce.CreateProfile, assets.CreateAsset
// both return the driver error unwrapped) into a safe, row-level conflict —
// never the raw constraint-violation text, which can carry column/table
// detail.
func asDuplicateConflict(err error, message string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return apperrors.Conflict(message, nil)
	}
	return err
}

func invalidRow(code string) bulkusecases.RowOutcome {
	return bulkusecases.RowOutcome{Valid: false, ErrorCode: code}
}

// ---------------------------------------------------------------------------
// PRIVATE_ORDER — generic-engine counterpart to the legacy order-import
// adapter. Wraps the SAME OrdersPort adapter bulkimports_wiring.go already
// builds for the legacy flow; there is exactly one integration with orders,
// used by both engines.
// ---------------------------------------------------------------------------

type genericOrdersAdapter struct{ orders bulkusecases.OrdersPort }

func (genericOrdersAdapter) Type() string       { return bulkmodels.TypePrivateOrder }
func (genericOrdersAdapter) SchemaVersion() int { return 1 }
func (genericOrdersAdapter) RequiredColumns() []string {
	return []string{"rowReference", "customerId", "serviceCatalogItemId", "timezone"}
}
func (genericOrdersAdapter) OptionalColumns() []string {
	return []string{"customerReference", "contractId", "priority", "specialInstructions"}
}

func (genericOrdersAdapter) ValidateRow(raw map[string]string) bulkusecases.RowOutcome {
	ref := strings.TrimSpace(raw["rowReference"])
	if ref == "" {
		return invalidRow("row_reference_required")
	}
	customerID := strings.TrimSpace(raw["customerId"])
	if customerID == "" {
		return invalidRow("customer_required")
	}
	catalogItemID := strings.TrimSpace(raw["serviceCatalogItemId"])
	if catalogItemID == "" {
		return invalidRow("catalog_required")
	}
	tz := strings.TrimSpace(raw["timezone"])
	if _, err := time.LoadLocation(tz); err != nil {
		return invalidRow("timezone_invalid")
	}
	return bulkusecases.RowOutcome{
		Valid: true, ExternalReference: ref,
		Data: map[string]any{
			"rowReference": ref, "customerId": customerID, "customerReference": raw["customerReference"],
			"serviceCatalogItemId": catalogItemID, "contractId": raw["contractId"], "timezone": tz,
			"priority": raw["priority"], "specialInstructions": raw["specialInstructions"],
		},
	}
}

func (a genericOrdersAdapter) Commit(ctx context.Context, t tenant.Context, correlationID string,
	data map[string]any) (string, error) {
	row := bulkmodels.OrderRow{
		RowReference: str(data, "rowReference"), CustomerID: str(data, "customerId"),
		CustomerReference: str(data, "customerReference"), ServiceCatalogItemID: str(data, "serviceCatalogItemId"),
		ContractID: str(data, "contractId"), Timezone: str(data, "timezone"), Priority: str(data, "priority"),
		SpecialInstructions: str(data, "specialInstructions"),
	}
	return a.orders.CreateImported(ctx, t, bulkusecases.OrderInput{
		BranchID: branchOf(t), Row: row, SourceReference: correlationID,
	})
}

// ---------------------------------------------------------------------------
// WORKFORCE_PROFILE
// ---------------------------------------------------------------------------

type workforceImportAdapter struct{ workforce *workforcefeature.Service }

func (workforceImportAdapter) Type() string       { return bulkmodels.TypeWorkforceProfile }
func (workforceImportAdapter) SchemaVersion() int { return 1 }
func (workforceImportAdapter) RequiredColumns() []string {
	return []string{"membershipId", "employeeNumber", "relationshipType", "role"}
}
func (workforceImportAdapter) OptionalColumns() []string {
	return []string{"supervisorMembershipId", "startDate"}
}

func (workforceImportAdapter) ValidateRow(raw map[string]string) bulkusecases.RowOutcome {
	membershipID := strings.TrimSpace(raw["membershipId"])
	if membershipID == "" {
		return invalidRow("membership_required")
	}
	employeeNumber := strings.TrimSpace(raw["employeeNumber"])
	if employeeNumber == "" {
		return invalidRow("employee_number_required")
	}
	relationshipType := strings.ToUpper(strings.TrimSpace(raw["relationshipType"]))
	if relationshipType == "" {
		return invalidRow("relationship_type_required")
	}
	role := strings.TrimSpace(raw["role"])
	if role == "" {
		return invalidRow("role_required")
	}
	startDate := strings.TrimSpace(raw["startDate"])
	if startDate != "" {
		if _, err := time.Parse("2006-01-02", startDate); err != nil {
			return invalidRow("start_date_invalid")
		}
	}
	return bulkusecases.RowOutcome{
		Valid: true, ExternalReference: employeeNumber,
		Data: map[string]any{
			"membershipId": membershipID, "employeeNumber": employeeNumber, "relationshipType": relationshipType,
			"role": role, "supervisorMembershipId": raw["supervisorMembershipId"], "startDate": startDate,
		},
	}
}

func (a workforceImportAdapter) Commit(ctx context.Context, t tenant.Context, _ string,
	data map[string]any) (string, error) {
	startDate := time.Now().UTC()
	if raw := str(data, "startDate"); raw != "" {
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			startDate = parsed
		}
	}
	profile, err := a.workforce.CreateProfile(ctx, t, workforcefeature.CreateProfileInput{
		BranchID: branchOf(t), MembershipID: str(data, "membershipId"), EmployeeNumber: str(data, "employeeNumber"),
		RelationshipType: str(data, "relationshipType"), Role: str(data, "role"),
		SupervisorMembershipID: str(data, "supervisorMembershipId"), StartDate: startDate,
	})
	if err != nil {
		return "", asDuplicateConflict(err, "A workforce profile with this employee number or membership already exists.")
	}
	return profile.ID, nil
}

// ---------------------------------------------------------------------------
// ASSET
// ---------------------------------------------------------------------------

type assetImportAdapter struct{ assets *assetsfeature.Service }

func (assetImportAdapter) Type() string       { return bulkmodels.TypeAsset }
func (assetImportAdapter) SchemaVersion() int { return 1 }
func (assetImportAdapter) RequiredColumns() []string {
	return []string{"assetCode", "assetType"}
}
func (assetImportAdapter) OptionalColumns() []string {
	return []string{"manufacturer", "model", "year", "capacity", "capacityUnit", "ownershipType",
		"registrationReference", "platformVehicleId"}
}

func (assetImportAdapter) ValidateRow(raw map[string]string) bulkusecases.RowOutcome {
	assetCode := strings.TrimSpace(raw["assetCode"])
	if assetCode == "" {
		return invalidRow("asset_code_required")
	}
	assetType := strings.ToUpper(strings.TrimSpace(raw["assetType"]))
	if assetType == "" {
		return invalidRow("asset_type_required")
	}
	data := map[string]any{
		"assetCode": assetCode, "assetType": assetType, "manufacturer": raw["manufacturer"], "model": raw["model"],
		"capacityUnit": raw["capacityUnit"], "ownershipType": raw["ownershipType"],
		"registrationReference": raw["registrationReference"], "platformVehicleId": raw["platformVehicleId"],
	}
	if raw["year"] != "" {
		year, err := strconv.Atoi(strings.TrimSpace(raw["year"]))
		if err != nil {
			return invalidRow("year_invalid")
		}
		data["year"] = year
	}
	if raw["capacity"] != "" {
		capacity, err := strconv.ParseFloat(strings.TrimSpace(raw["capacity"]), 64)
		if err != nil {
			return invalidRow("capacity_invalid")
		}
		data["capacity"] = capacity
	}
	return bulkusecases.RowOutcome{Valid: true, ExternalReference: assetCode, Data: data}
}

func (a assetImportAdapter) Commit(ctx context.Context, t tenant.Context, _ string,
	data map[string]any) (string, error) {
	out, err := a.assets.CreateAsset(ctx, t, assetsfeature.Asset{
		BranchID: branchOf(t), AssetCode: str(data, "assetCode"), AssetType: str(data, "assetType"),
		Manufacturer: str(data, "manufacturer"), Model: str(data, "model"),
		CapacityUnit: str(data, "capacityUnit"), OwnershipType: str(data, "ownershipType"),
		RegistrationReference: str(data, "registrationReference"), PlatformVehicleID: str(data, "platformVehicleId"),
		Year: intPtr(data, "year"), Capacity: numeric(data, "capacity"),
	})
	if err != nil {
		return "", asDuplicateConflict(err, "An asset with this code already exists.")
	}
	return out.ID, nil
}

// ---------------------------------------------------------------------------
// WAREHOUSE_INVENTORY
// ---------------------------------------------------------------------------

type warehouseImportAdapter struct{ warehouse *warehouse.Service }

func (warehouseImportAdapter) Type() string       { return bulkmodels.TypeWarehouseInventory }
func (warehouseImportAdapter) SchemaVersion() int { return 1 }
func (warehouseImportAdapter) RequiredColumns() []string {
	return []string{"warehouseId", "locationId", "ownerType", "ownerReference", "sku", "quantity"}
}
func (warehouseImportAdapter) OptionalColumns() []string {
	return []string{"batchReference", "lotReference", "serialReference", "unit", "condition", "expiryAt"}
}

func (warehouseImportAdapter) ValidateRow(raw map[string]string) bulkusecases.RowOutcome {
	warehouseID := strings.TrimSpace(raw["warehouseId"])
	locationID := strings.TrimSpace(raw["locationId"])
	ownerType := strings.ToUpper(strings.TrimSpace(raw["ownerType"]))
	ownerReference := strings.TrimSpace(raw["ownerReference"])
	sku := strings.TrimSpace(raw["sku"])
	switch {
	case warehouseID == "":
		return invalidRow("warehouse_required")
	case locationID == "":
		return invalidRow("location_required")
	case ownerType == "":
		return invalidRow("owner_type_required")
	case ownerReference == "":
		return invalidRow("owner_reference_required")
	case sku == "":
		return invalidRow("sku_required")
	}
	quantity, err := strconv.ParseFloat(strings.TrimSpace(raw["quantity"]), 64)
	if err != nil || quantity <= 0 {
		return invalidRow("quantity_invalid")
	}
	externalRef := strings.Join([]string{warehouseID, locationID, sku, raw["batchReference"], raw["lotReference"],
		raw["serialReference"]}, "|")
	return bulkusecases.RowOutcome{
		Valid: true, ExternalReference: externalRef,
		Data: map[string]any{
			"warehouseId": warehouseID, "locationId": locationID, "ownerType": ownerType,
			"ownerReference": ownerReference, "sku": sku, "batchReference": raw["batchReference"],
			"lotReference": raw["lotReference"], "serialReference": raw["serialReference"], "unit": raw["unit"],
			"condition": raw["condition"], "expiryAt": raw["expiryAt"], "quantity": quantity,
		},
	}
}

// Commit passes the row's own reference (its row_reference, unique per
// version) as CorrelationID: warehouse.Receipt is itself idempotent by
// correlation id, so a retried commit of the SAME row version never double-
// receives stock, while a CORRECTED row (a new version, a new
// row_reference) is free to apply its corrected quantity rather than being
// silently short-circuited by the original version's now-stale result.
func (a warehouseImportAdapter) Commit(ctx context.Context, t tenant.Context, correlationID string,
	data map[string]any) (string, error) {
	item, err := a.warehouse.Receipt(ctx, t, warehouse.ReceiptInput{
		WarehouseID: str(data, "warehouseId"), LocationID: str(data, "locationId"), OwnerType: str(data, "ownerType"),
		OwnerReference: str(data, "ownerReference"), SKU: str(data, "sku"), BatchReference: str(data, "batchReference"),
		LotReference: str(data, "lotReference"), SerialReference: str(data, "serialReference"),
		Unit: str(data, "unit"), Condition: str(data, "condition"), ExpiryAt: timePtr(data, "expiryAt"),
		Quantity: numeric(data, "quantity"), CorrelationID: correlationID,
	})
	if err != nil {
		return "", err
	}
	return item.ID, nil
}
