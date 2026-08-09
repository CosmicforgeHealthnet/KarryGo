package app

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	assetsfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets"
	bulkmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/models"
	bulkrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/repositories"
	bulkusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/usecases"
	ordersmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/models"
	ordersusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/usecases"
	searchusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse"
	workforcefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"crypto/sha256"
	"encoding/hex"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
)

type bulkOrdersAdapter struct{ orders *ordersusecases.Service }

func (a bulkOrdersAdapter) CreateImported(ctx context.Context, t tenant.Context, in bulkusecases.OrderInput) (string, error) {
	order, e := a.orders.Create(ctx, t, ordersusecases.CreateInput{BranchID: in.BranchID, CustomerID: in.Row.CustomerID, CustomerReference: in.Row.CustomerReference, ServiceCatalogItemID: in.Row.ServiceCatalogItemID, ServiceFields: ordersmodels.JSONMap(in.Row.ServiceFields), ContractID: in.Row.ContractID, Timezone: in.Row.Timezone, Priority: in.Row.Priority, SpecialInstructions: in.Row.SpecialInstructions, Source: ordersmodels.SourceBulkImport, SourceReference: in.SourceReference})
	return order.ID, e
}

type bulkMediaAdapter struct {
	media platformclients.MediaAuthority
}

func (a bulkMediaAdapter) meta(t tenant.Context, key string) platformclients.CallMeta {
	return platformclients.CallMeta{OrganizationID: t.OrganizationID, RequestID: t.RequestID, CorrelationID: t.CorrelationID, IdempotencyKey: key}
}
func (a bulkMediaAdapter) LoadImport(ctx context.Context, t tenant.Context, id string, max int64) (bulkusecases.MediaFile, error) {
	state, e := a.media.ReadObject(ctx, a.meta(t, ""), id)
	if e != nil {
		return bulkusecases.MediaFile{}, e
	}
	safe := strings.EqualFold(state.SafetyStatus, "SAFE") || strings.EqualFold(state.SafetyStatus, "CLEAN")
	if state.OrganizationID != t.OrganizationID || !safe || state.UploadStatus != "COMPLETED" || !strings.Contains(strings.ToUpper(state.UploadPurpose), "IMPORT") {
		return bulkusecases.MediaFile{}, platformclients.ErrNotConfigured
	}
	grant, e := a.media.AuthorizeDownload(ctx, a.meta(t, "bulk-import-read-"+id), id)
	if e != nil {
		return bulkusecases.MediaFile{}, e
	}
	content, e := a.media.ReadContent(ctx, a.meta(t, "bulk-import-read-"+id), grant, max)
	format := "CSV"
	if strings.Contains(strings.ToLower(state.DetectedMIME), "json") {
		format = "JSON"
	}
	return bulkusecases.MediaFile{ContentType: state.DetectedMIME, Format: format, SizeBytes: state.SizeBytes, Content: content}, e
}
func (a bulkMediaAdapter) WriteErrorReport(ctx context.Context, t tenant.Context, importID string, content []byte) (string, error) {
	grant, e := a.media.AuthorizeUpload(ctx, a.meta(t, "bulk-import-report-"+importID), platformclients.MediaUploadRequest{OwnerType: "BULK_IMPORT", OwnerReference: importID, DocumentCategory: "IMPORT_ERROR_REPORT", UploadPurpose: "BULK_IMPORT_ERROR_REPORT", AllowedMIMETypes: []string{"application/json"}, MaxSizeBytes: int64(len(content)) + 1024, RetentionClass: "OPERATIONS"})
	if e != nil {
		return "", e
	}
	sum := sha256.Sum256(content)
	state, e := a.media.CompleteUpload(ctx, a.meta(t, "bulk-import-report-"+importID), grant, "application/json", hex.EncodeToString(sum[:]), content)
	return state.MediaObjectID, e
}

// BuildBulkImportsService assembles both the legacy (engine=LEGACY)
// order-import service and, additively, the Section 25C generic-engine
// registry on the SAME Service — see internal/features/bulkimports/usecases
// generic.go/generic_worker.go for why the two share one aggregate rather
// than being two separate services.
//
// workforce/assets/warehouseSvc are the SAME already-constructed instances
// wiring.go builds for their own features (not fresh ones) — the generic
// import adapters call their real CreateProfile/CreateAsset/Receipt usecases
// exactly as those features' own HTTP handlers do, never a duplicate or
// parallel code path.
func BuildBulkImportsService(cfg config.Config, pool *pgxpool.Pool, publisher *events.DomainPublisher,
	metrics *observability.Metrics, workforceSvc *workforcefeature.Service, assetsSvc *assetsfeature.Service,
	warehouseSvc *warehouse.Service, search *searchusecases.Service) *bulkusecases.Service {
	var mp bulkusecases.MediaPort
	mediaCfg := cfg.Clients["media-file-service"]
	if !cfg.IntegrationsKillSwitch && mediaCfg.Configured() {
		media := platformclients.NewMediaFileClient(platformclients.Options{ServiceName: "media-file-service", BaseURL: mediaCfg.BaseURL, Secret: mediaCfg.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics})
		mp = bulkMediaAdapter{media: media}
	}
	orders := BuildOrdersService(cfg, pool, nil, publisher, search)
	ordersPort := bulkOrdersAdapter{orders}

	adapters := map[string]bulkusecases.Adapter{
		bulkmodels.TypePrivateOrder:       genericOrdersAdapter{orders: ordersPort},
		bulkmodels.TypeWorkforceProfile:   workforceImportAdapter{workforce: workforceSvc},
		bulkmodels.TypeAsset:              assetImportAdapter{assets: assetsSvc},
		bulkmodels.TypeWarehouseInventory: warehouseImportAdapter{warehouse: warehouseSvc},
	}

	return bulkusecases.NewService(bulkusecases.Options{
		Pool: pool, Repository: bulkrepos.NewRepository(pool), Media: mp, Orders: ordersPort, Events: publisher,
		Flags:    bulkusecases.Flags{Enabled: cfg.BulkImportsEnabled, WritesEnabled: cfg.BulkImportsWritesEnabled, WorkerEnabled: cfg.BulkImportsWorkerEnabled, KillSwitch: cfg.BulkImportsKillSwitch || cfg.IntegrationsKillSwitch},
		MaxBytes: cfg.BulkImportsMaxBytes, MaxRows: cfg.BulkImportsMaxRows,
		Adapters: adapters,
		GenericFlags: bulkusecases.GenericFlags{
			Enabled: cfg.ImportsEnabled, WritesEnabled: cfg.ImportsWritesEnabled,
			ValidationWorker: cfg.ImportsValidationWorkerEnabled, CommitWorker: cfg.ImportsCommitWorkerEnabled,
			RetryWorker: cfg.ImportsRetryWorkerEnabled, KillSwitch: cfg.BulkImportsKillSwitch || cfg.IntegrationsKillSwitch,
		},
	})
}
