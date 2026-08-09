package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	assetsfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement"
	exportsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/repositories"
	exportsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops"
	ordersusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/usecases"
	safetyfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/safety"
	searchusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/usecases"
	shipmentsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/usecases"
	warehousefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse"
	workforcefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// ordersExporter adapts orders/usecases.Service (a read port, not the
// repository directly) to the exports Exporter contract. It pages through
// the existing tenant- and branch-scoped List call exactly as a caller of the
// ordinary orders list endpoint would — a generic export can never reach a
// domain table on its own.
type ordersExporter struct{ orders *ordersusecases.Service }

func (ordersExporter) Type() string { return "ORDERS" }

func (ordersExporter) Columns() []string {
	return []string{"id", "orderNumber", "branchId", "customerId", "customerReference",
		"serviceCatalogItemId", "contractId", "status", "priority", "timezone", "createdAt", "updatedAt"}
}

func (a ordersExporter) Rows(ctx context.Context, t tenant.Context, filters map[string]any,
	maxRows int) ([][]string, error) {
	status, _ := filters["status"].(string)
	customerID, _ := filters["customerId"].(string)

	var rows [][]string
	cursor := ""
	for len(rows) < maxRows {
		batch := maxRows - len(rows)
		if batch > 200 {
			batch = 200
		}
		orders, next, err := a.orders.List(ctx, t, ordersusecases.ListInput{
			Status: status, CustomerID: customerID, Cursor: cursor, Limit: batch,
		})
		if err != nil {
			return nil, err
		}
		for _, order := range orders {
			rows = append(rows, []string{
				order.ID, order.OrderNumber, order.BranchID, order.CustomerID, order.CustomerReference,
				order.ServiceCatalogItemID, order.ContractID, order.Status, order.Priority, order.Timezone,
				order.CreatedAt.UTC().Format(time.RFC3339), order.UpdatedAt.UTC().Format(time.RFC3339),
			})
			if len(rows) >= maxRows {
				break
			}
		}
		if next == "" || next == cursor {
			break
		}
		cursor = next
	}
	return rows, nil
}

// shipmentsExporter adapts shipments/usecases.Service to the exports
// Exporter contract, mirroring ordersExporter exactly: it pages through the
// existing tenant- and branch-scoped List call, never a domain table
// directly. DeclaredValueAmountMinor/Currency are deliberately omitted from
// the export columns — a declared cargo value is exactly the kind of
// commercial figure this service's other export/statement surfaces
// (TestProviderStatementExcludesCommercialData) are careful never to leak
// into a general operational listing.
type shipmentsExporter struct{ shipments *shipmentsusecases.Service }

func (shipmentsExporter) Type() string { return "SHIPMENTS" }

func (shipmentsExporter) Columns() []string {
	return []string{"id", "orderId", "branchId", "originStopId", "destinationStopId",
		"requiredVehicleClass", "requiredServiceCapability", "packageCount", "status",
		"createdAt", "updatedAt"}
}

func (a shipmentsExporter) Rows(ctx context.Context, t tenant.Context, filters map[string]any,
	maxRows int) ([][]string, error) {
	status, _ := filters["status"].(string)
	orderID, _ := filters["orderId"].(string)

	var rows [][]string
	cursor := ""
	for len(rows) < maxRows {
		batch := maxRows - len(rows)
		if batch > 200 {
			batch = 200
		}
		shipments, next, err := a.shipments.List(ctx, t, shipmentsusecases.ListInput{
			Status: status, OrderID: orderID, Cursor: cursor, Limit: batch,
		})
		if err != nil {
			return nil, err
		}
		for _, s := range shipments {
			rows = append(rows, []string{
				s.ID, s.OrderID, s.BranchID, s.OriginStopID, s.DestinationStopID,
				s.RequiredVehicleClass, s.RequiredServiceCapability, strconv.Itoa(s.PackageCount), s.Status,
				s.CreatedAt.UTC().Format(time.RFC3339), s.UpdatedAt.UTC().Format(time.RFC3339),
			})
			if len(rows) >= maxRows {
				break
			}
		}
		if next == "" || next == cursor {
			break
		}
		cursor = next
	}
	return rows, nil
}

// workforceExporter adapts workforce.Service's provider-profile listing to
// the exports Exporter contract.
type workforceExporter struct{ workforce *workforcefeature.Service }

func (workforceExporter) Type() string { return "PROVIDERS_WORKFORCE" }

func (workforceExporter) Columns() []string {
	return []string{"id", "branchId", "membershipId", "employeeNumber", "relationshipType",
		"role", "status", "startDate", "createdAt", "updatedAt"}
}

func (a workforceExporter) Rows(ctx context.Context, t tenant.Context, filters map[string]any,
	maxRows int) ([][]string, error) {
	status, _ := filters["status"].(string)

	var rows [][]string
	offset := 0
	for len(rows) < maxRows {
		limit := maxRows - len(rows)
		if limit > 200 {
			limit = 200
		}
		profiles, err := a.workforce.ListProfiles(ctx, t,
			workforcefeature.Page{Status: status, Limit: limit, Offset: offset})
		if err != nil {
			return nil, err
		}
		if len(profiles) == 0 {
			break
		}
		for _, p := range profiles {
			rows = append(rows, []string{
				p.ID, p.BranchID, p.MembershipID, p.EmployeeNumber, p.RelationshipType, p.Role, p.Status,
				p.StartDate.UTC().Format("2006-01-02"),
				p.CreatedAt.UTC().Format(time.RFC3339), p.UpdatedAt.UTC().Format(time.RFC3339),
			})
			if len(rows) >= maxRows {
				break
			}
		}
		if len(profiles) < limit {
			break
		}
		offset += limit
	}
	return rows, nil
}

// assetsExporter adapts assets.Service's fleet-asset listing to the exports
// Exporter contract.
type assetsExporter struct{ assets *assetsfeature.Service }

func (assetsExporter) Type() string { return "ASSETS_MAINTENANCE" }

func (assetsExporter) Columns() []string {
	return []string{"id", "branchId", "assetCode", "assetType", "manufacturer", "model",
		"operationalStatus", "lifecycleStatus", "odometer", "operatingHours", "createdAt", "updatedAt"}
}

func (a assetsExporter) Rows(ctx context.Context, t tenant.Context, filters map[string]any,
	maxRows int) ([][]string, error) {
	status, _ := filters["status"].(string)

	var rows [][]string
	offset := 0
	for len(rows) < maxRows {
		limit := maxRows - len(rows)
		if limit > 200 {
			limit = 200
		}
		assets, err := a.assets.ListAssets(ctx, t, assetsfeature.Page{Status: status, Limit: limit, Offset: offset})
		if err != nil {
			return nil, err
		}
		if len(assets) == 0 {
			break
		}
		for _, asset := range assets {
			rows = append(rows, []string{
				asset.ID, asset.BranchID, asset.AssetCode, asset.AssetType, asset.Manufacturer, asset.Model,
				asset.OperationalStatus, asset.LifecycleStatus,
				strconv.FormatFloat(asset.Odometer, 'f', -1, 64), strconv.FormatFloat(asset.OperatingHours, 'f', -1, 64),
				asset.CreatedAt.UTC().Format(time.RFC3339), asset.UpdatedAt.UTC().Format(time.RFC3339),
			})
			if len(rows) >= maxRows {
				break
			}
		}
		if len(assets) < limit {
			break
		}
		offset += limit
	}
	return rows, nil
}

// warehouseInventoryExporter adapts warehouse.Service's inventory-item
// listing to the exports Exporter contract. ListItems returns the whole
// tenant- and branch-scoped set in one call (no internal pagination), so
// this adapter's only job is truncating it to maxRows — matching the
// established "BoundedGenerationTruncatesToMaxRows" convention every other
// exporter in this service obeys.
type warehouseInventoryExporter struct{ warehouse *warehousefeature.Service }

func (warehouseInventoryExporter) Type() string { return "WAREHOUSE_INVENTORY" }

func (warehouseInventoryExporter) Columns() []string {
	return []string{"id", "warehouseId", "locationId", "sku", "quantityProjection",
		"reservedQuantity", "unit", "status", "createdAt", "updatedAt"}
}

type complianceIncidentsExporter struct {
	safety *safetyfeature.OperationsService
}

func (complianceIncidentsExporter) Type() string { return "COMPLIANCE_INCIDENTS" }
func (complianceIncidentsExporter) Columns() []string {
	return []string{"id", "branchId", "incidentType", "severity", "status", "occurredAt", "locationReference", "routeId", "assignmentId", "tripReference", "externalReference", "version", "createdAt", "updatedAt"}
}
func (a complianceIncidentsExporter) Rows(ctx context.Context, t tenant.Context, filters map[string]any, maxRows int) ([][]string, error) {
	incidents, err := a.safety.ListIncidents(ctx, t)
	if err != nil {
		return nil, err
	}
	status, _ := filters["status"].(string)
	rows := make([][]string, 0, min(len(incidents), maxRows))
	for _, i := range incidents {
		if status != "" && i.Status != status {
			continue
		}
		if len(rows) >= maxRows {
			break
		}
		rows = append(rows, []string{i.ID, i.BranchID, i.IncidentType, i.Severity, i.Status,
			i.OccurredAt.UTC().Format(time.RFC3339), i.LocationReference, i.RouteID, i.AssignmentID,
			i.TripReference, i.ExternalReference, strconv.FormatInt(i.Version, 10),
			i.CreatedAt.UTC().Format(time.RFC3339), i.UpdatedAt.UTC().Format(time.RFC3339)})
	}
	return rows, nil
}

type invoicesReceivablesExporter struct{ finance *financeops.Service }

func (invoicesReceivablesExporter) Type() string { return "INVOICES_RECEIVABLES" }
func (invoicesReceivablesExporter) Columns() []string {
	return []string{"recordType", "id", "branchId", "invoiceId", "customerReference", "invoiceNumber", "currency", "status", "subtotalMinor", "taxTotalMinor", "totalMinor", "outstandingAmountMinor", "dueAt", "version", "createdAt", "updatedAt"}
}
func safeMapString(m map[string]any, key string) string {
	v := m[key]
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func mapFirst(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v := safeMapString(m, key); v != "" {
			return v
		}
	}
	return ""
}
func (a invoicesReceivablesExporter) Rows(ctx context.Context, t tenant.Context, filters map[string]any, maxRows int) ([][]string, error) {
	invoices, err := a.finance.ListInvoices(ctx, t)
	if err != nil {
		return nil, err
	}
	receivables, err := a.finance.ListReceivables(ctx, t)
	if err != nil {
		return nil, err
	}
	rows := make([][]string, 0, min(len(invoices)+len(receivables), maxRows))
	appendRow := func(kind string, m map[string]any) {
		if len(rows) >= maxRows {
			return
		}
		rows = append(rows, []string{kind, mapFirst(m, "id"), mapFirst(m, "branch_id", "branchId"), mapFirst(m, "invoice_id", "invoiceId", "id"), mapFirst(m, "customer_reference", "customerReference", "customer_id"), mapFirst(m, "invoice_number", "invoiceNumber", "id"), mapFirst(m, "currency"), mapFirst(m, "status"), mapFirst(m, "subtotal_minor", "subtotalMinor"), mapFirst(m, "tax_total_minor", "taxTotalMinor"), mapFirst(m, "total_minor", "totalMinor"), mapFirst(m, "outstanding_amount_minor", "outstandingAmountMinor"), mapFirst(m, "due_at", "dueAt", "due_date"), mapFirst(m, "version", "source_version"), mapFirst(m, "created_at", "createdAt"), mapFirst(m, "updated_at", "updatedAt")})
	}
	for _, m := range invoices {
		appendRow("INVOICE", m)
	}
	for _, m := range receivables {
		appendRow("RECEIVABLE", m)
	}
	return rows, nil
}

type fulfilmentOrdersExporter struct{ warehouse *warehousefeature.Service }

func (fulfilmentOrdersExporter) Type() string { return "FULFILMENT_ORDERS" }
func (fulfilmentOrdersExporter) Columns() []string {
	return []string{"id", "warehouseId", "customerReference", "sourceOrderId", "priority", "status", "requestedDispatchAt", "assignedTeamReference", "proofPolicyReference", "shipmentReference", "manifestReference", "version", "createdAt", "updatedAt"}
}
func (a fulfilmentOrdersExporter) Rows(ctx context.Context, t tenant.Context, filters map[string]any, maxRows int) ([][]string, error) {
	orders, err := a.warehouse.ListFulfilment(ctx, t)
	if err != nil {
		return nil, err
	}
	status, _ := filters["status"].(string)
	rows := make([][]string, 0, min(len(orders), maxRows))
	for _, o := range orders {
		if status != "" && o.Status != status {
			continue
		}
		if len(rows) >= maxRows {
			break
		}
		requested := ""
		if o.RequestedDispatchAt != nil {
			requested = o.RequestedDispatchAt.UTC().Format(time.RFC3339)
		}
		rows = append(rows, []string{o.ID, o.WarehouseID, o.CustomerReference, o.SourceOrderID, o.Priority, o.Status, requested, o.AssignedTeamReference, o.ProofPolicyReference, o.ShipmentReference, o.ManifestReference, strconv.FormatInt(o.Version, 10), o.CreatedAt.UTC().Format(time.RFC3339), o.UpdatedAt.UTC().Format(time.RFC3339)})
	}
	return rows, nil
}

type supportCasesExporter struct{ engagement *engagement.Service }

func (supportCasesExporter) Type() string { return "SUPPORT_CASE_PROJECTIONS" }
func (supportCasesExporter) Columns() []string {
	return []string{"id", "supportServiceCaseId", "caseType", "status", "priority", "tripReference", "orderReference", "assignmentReference", "ownerReference", "sourceVersion", "gapStatus", "lastSyncedAt", "createdAt", "updatedAt"}
}
func (a supportCasesExporter) Rows(ctx context.Context, t tenant.Context, filters map[string]any, maxRows int) ([][]string, error) {
	cases, err := a.engagement.ListSupportCases(ctx, t)
	if err != nil {
		return nil, err
	}
	status, _ := filters["status"].(string)
	rows := make([][]string, 0, min(len(cases), maxRows))
	for _, m := range cases {
		if status != "" && mapFirst(m, "status") != status {
			continue
		}
		if len(rows) >= maxRows {
			break
		}
		rows = append(rows, []string{mapFirst(m, "id"), mapFirst(m, "support_service_case_id"), mapFirst(m, "case_type"), mapFirst(m, "status"), mapFirst(m, "priority"), mapFirst(m, "trip_reference"), mapFirst(m, "order_reference"), mapFirst(m, "assignment_reference"), mapFirst(m, "owner_reference"), mapFirst(m, "source_version"), mapFirst(m, "gap_status"), mapFirst(m, "last_synced_at"), mapFirst(m, "created_at"), mapFirst(m, "updated_at")})
	}
	return rows, nil
}

func (a warehouseInventoryExporter) Rows(ctx context.Context, t tenant.Context, filters map[string]any,
	maxRows int) ([][]string, error) {
	items, err := a.warehouse.ListItems(ctx, t)
	if err != nil {
		return nil, err
	}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		if len(rows) >= maxRows {
			break
		}
		rows = append(rows, []string{
			item.ID, item.WarehouseID, item.LocationID, item.SKU,
			strconv.FormatFloat(item.QuantityProjection, 'f', -1, 64),
			strconv.FormatFloat(item.ReservedQuantity, 'f', -1, 64), item.Unit, item.Status,
			item.CreatedAt.UTC().Format(time.RFC3339), item.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	return rows, nil
}

// exportsMediaAdapter writes and re-reads a generated export file through
// Media, mirroring bulk imports' error-report adapter exactly: fleet stores
// only the reference Media hands back, never the bytes and never a durable
// link.
type exportsMediaAdapter struct {
	media platformclients.MediaAuthority
}

func (a exportsMediaAdapter) meta(t tenant.Context, key string) platformclients.CallMeta {
	return platformclients.CallMeta{
		OrganizationID: t.OrganizationID, RequestID: t.RequestID, CorrelationID: t.CorrelationID,
		IdempotencyKey: key,
	}
}

func (a exportsMediaAdapter) WriteExportFile(ctx context.Context, t tenant.Context, exportID string,
	content []byte) (string, error) {
	grant, err := a.media.AuthorizeUpload(ctx, a.meta(t, "export-file-"+exportID), platformclients.MediaUploadRequest{
		OwnerType: "EXPORT_JOB", OwnerReference: exportID, DocumentCategory: "EXPORT_FILE",
		UploadPurpose: "EXPORT_FILE", AllowedMIMETypes: []string{"text/csv"},
		MaxSizeBytes: int64(len(content)) + 1024, RetentionClass: "OPERATIONS",
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(content)
	state, err := a.media.CompleteUpload(ctx, a.meta(t, "export-file-"+exportID), grant,
		"text/csv", hex.EncodeToString(sum[:]), content)
	if err != nil {
		return "", err
	}
	return state.MediaObjectID, nil
}

func (a exportsMediaAdapter) AuthorizeDownload(ctx context.Context, t tenant.Context,
	mediaFileID string) (exportsusecases.DownloadGrant, error) {
	grant, err := a.media.AuthorizeDownload(ctx, a.meta(t, "export-download-"+mediaFileID+"-"+t.RequestID), mediaFileID)
	if err != nil {
		return exportsusecases.DownloadGrant{}, err
	}
	expiresAt, _ := time.Parse(time.RFC3339, grant.ExpiresAt)
	return exportsusecases.DownloadGrant{
		Token: grant.Token, Path: grant.Path, ExpiresAt: expiresAt,
		ContentType: grant.ContentType, SizeBytes: grant.SizeBytes,
	}, nil
}

// BuildExportsService assembles the Section 25D generic export service.
//
// It is always constructible. With the enablement flag off its routes still
// mount but answer not-found, matching the established Section 15/25
// convention: an absent capability must be observable, not indistinguishable
// from a deployment that never had it.
//
// shipments, workforce, assets and warehouse are the SAME instances the rest
// of the composition root uses (not fresh copies): those services need
// dependencies — a restriction gate, the media port, warehouse's proof
// binding — that only the composition root can assemble, so this reuses
// them rather than re-deriving them here. Any of the four may be nil (its
// feature flag is off or it failed to construct); that dataset's export type
// is then refused as "recognized but not available" rather than the whole
// export service failing to build.
//
// All nine Section 25 dataset types are registered when their owning-domain
// service is present. Each adapter maps an explicit safe column allowlist.
func BuildExportsService(cfg config.Config, pool *pgxpool.Pool, publisher *events.DomainPublisher,
	metrics *observability.Metrics, search *searchusecases.Service,
	shipments *shipmentsusecases.Service, workforce *workforcefeature.Service,
	assets *assetsfeature.Service, warehouse *warehousefeature.Service,
	safety *safetyfeature.OperationsService, finance *financeops.Service,
	engagementService *engagement.Service) *exportsusecases.Service {
	orders := BuildOrdersService(cfg, pool, nil, publisher, search)

	var media exportsusecases.MediaPort
	mediaCfg := cfg.Clients["media-file-service"]
	if !cfg.IntegrationsKillSwitch && mediaCfg.Configured() {
		client := platformclients.NewMediaFileClient(platformclients.Options{
			ServiceName: "media-file-service", BaseURL: mediaCfg.BaseURL, Secret: mediaCfg.Secret,
			Timeout: cfg.HTTPClientTimeout, Metrics: metrics,
		})
		media = exportsMediaAdapter{media: client}
	}

	exporters := map[string]exportsusecases.Exporter{"ORDERS": ordersExporter{orders: orders}}
	if shipments != nil {
		exporters["SHIPMENTS"] = NewShipmentsExporter(shipments)
	}
	if workforce != nil {
		exporters["PROVIDERS_WORKFORCE"] = NewWorkforceExporter(workforce)
	}
	if assets != nil {
		exporters["ASSETS_MAINTENANCE"] = NewAssetsExporter(assets)
	}
	if warehouse != nil {
		exporters["WAREHOUSE_INVENTORY"] = NewWarehouseInventoryExporter(warehouse)
		exporters["FULFILMENT_ORDERS"] = fulfilmentOrdersExporter{warehouse: warehouse}
	}
	if safety != nil {
		exporters["COMPLIANCE_INCIDENTS"] = complianceIncidentsExporter{safety: safety}
	}
	if finance != nil {
		exporters["INVOICES_RECEIVABLES"] = invoicesReceivablesExporter{finance: finance}
	}
	if engagementService != nil {
		exporters["SUPPORT_CASE_PROJECTIONS"] = supportCasesExporter{engagement: engagementService}
	}

	return exportsusecases.NewService(exportsusecases.Options{
		Pool:       pool,
		Repository: exportsrepos.NewRepository(pool),
		Exporters:  exporters,
		Media:      media,
		Metrics:    metrics,
		Flags: exportsusecases.Flags{
			Enabled: cfg.ExportsEnabled, WritesEnabled: cfg.ExportsWritesEnabled,
			WorkerEnabled: cfg.ExportsWorkerEnabled,
			KillSwitch:    cfg.ExportsKillSwitch || cfg.IntegrationsKillSwitch,
		},
		MaxRows:      cfg.ExportsMaxRows,
		RetentionTTL: time.Duration(cfg.ExportsRetentionHours) * time.Hour,
	})
}

// NewShipmentsExporter builds the SHIPMENTS export adapter. Exported so it
// can be composed directly — by BuildExportsService, by a caller that wants
// a narrower export service, or by a test that wants to exercise the real
// adapter against a fake media/idempotency backend rather than the full
// composition root.
func NewShipmentsExporter(shipments *shipmentsusecases.Service) exportsusecases.Exporter {
	return shipmentsExporter{shipments: shipments}
}

// NewWorkforceExporter builds the PROVIDERS_WORKFORCE export adapter.
func NewWorkforceExporter(workforce *workforcefeature.Service) exportsusecases.Exporter {
	return workforceExporter{workforce: workforce}
}

// NewAssetsExporter builds the ASSETS_MAINTENANCE export adapter.
func NewAssetsExporter(assets *assetsfeature.Service) exportsusecases.Exporter {
	return assetsExporter{assets: assets}
}

// NewWarehouseInventoryExporter builds the WAREHOUSE_INVENTORY export
// adapter.
func NewWarehouseInventoryExporter(warehouse *warehousefeature.Service) exportsusecases.Exporter {
	return warehouseInventoryExporter{warehouse: warehouse}
}

// NewComplianceIncidentsExporter builds the safe compliance/incident adapter.
func NewComplianceIncidentsExporter(safety *safetyfeature.OperationsService) exportsusecases.Exporter {
	return complianceIncidentsExporter{safety: safety}
}

// NewInvoicesReceivablesExporter builds the safe invoice/receivable adapter.
func NewInvoicesReceivablesExporter(finance *financeops.Service) exportsusecases.Exporter {
	return invoicesReceivablesExporter{finance: finance}
}

// NewFulfilmentOrdersExporter builds the fulfilment-order adapter.
func NewFulfilmentOrdersExporter(warehouse *warehousefeature.Service) exportsusecases.Exporter {
	return fulfilmentOrdersExporter{warehouse: warehouse}
}

// NewSupportCasesExporter builds the data-minimized support projection adapter.
func NewSupportCasesExporter(engagementService *engagement.Service) exportsusecases.Exporter {
	return supportCasesExporter{engagement: engagementService}
}
