// Package usecases implements Media-backed asynchronous bulk imports.
package usecases

import (
	"bytes"
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
)

type MediaFile struct {
	FileName, ContentType, Format string
	SizeBytes                     int64
	Content                       []byte
}
type MediaPort interface {
	LoadImport(context.Context, tenant.Context, string, int64) (MediaFile, error)
	WriteErrorReport(context.Context, tenant.Context, string, []byte) (string, error)
}
type OrderInput struct {
	BranchID        string
	Row             models.OrderRow
	SourceReference string
}
type OrdersPort interface {
	CreateImported(context.Context, tenant.Context, OrderInput) (string, error)
}
type EventPublisher interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}
type Flags struct{ Enabled, WritesEnabled, WorkerEnabled, KillSwitch bool }
type Options struct {
	Pool       *pgxpool.Pool
	Repository *repositories.Repository
	Media      MediaPort
	Orders     OrdersPort
	Events     EventPublisher
	Audit      *audit.Recorder
	Flags      Flags
	MaxBytes   int64
	MaxRows    int
	Now        func() time.Time

	// Adapters is the Section 25C generic-engine registry, keyed by import
	// type. Nil/empty is fully valid — every legacy (engine=LEGACY) method
	// above never reads it — it only bounds which import types the generic
	// engine (below, in generic.go) can actually commit.
	Adapters map[string]Adapter
	// GenericFlags gates the generic-engine surface independently of the
	// legacy Flags above, so enabling generic imports never implicitly
	// enables (or requires) the legacy order-import flow, and vice versa.
	GenericFlags GenericFlags
}
type Service struct{ opts Options }

func NewService(o Options) *Service {
	if o.Audit == nil {
		o.Audit = audit.NewRecorder()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.MaxBytes <= 0 {
		o.MaxBytes = 10 << 20
	}
	if o.MaxRows <= 0 {
		o.MaxRows = 5000
	}
	if o.Adapters == nil {
		o.Adapters = map[string]Adapter{}
	}
	return &Service{o}
}
func (s *Service) gate(w bool) error {
	if s.opts.Flags.KillSwitch {
		return errorcodes.Unavailable("bulk_imports_disabled", "Bulk imports are unavailable.")
	}
	if !s.opts.Flags.Enabled {
		return errorcodes.NotFound("bulk_import_not_found", "Bulk import not found.")
	}
	if w && !s.opts.Flags.WritesEnabled {
		return errorcodes.Forbidden("bulk_import_conflict", "Bulk-import writes are disabled.")
	}
	return nil
}
func (s *Service) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.opts.Pool, fn)
}

type CreateInput struct{ BranchID, MediaFileID, FileFormat string }

func fingerprint(org, branch, media, format string) string {
	x := sha256.Sum256([]byte(org + "\x00" + branch + "\x00" + media + "\x00" + format))
	return hex.EncodeToString(x[:])
}
func (s *Service) Create(ctx context.Context, t tenant.Context, in CreateInput) (models.Import, error) {
	if e := s.gate(true); e != nil {
		return models.Import{}, e
	}
	format := strings.ToUpper(strings.TrimSpace(in.FileFormat))
	if format != "CSV" && format != "JSON" {
		return models.Import{}, errorcodes.UnprocessableEntity("bulk_import_invalid_file", "Only CSV and JSON imports are supported.")
	}
	if in.BranchID == "" || !t.BranchPermitted(in.BranchID) || in.MediaFileID == "" {
		return models.Import{}, apperrors.BadRequest("branchId and mediaFileId are required.", nil)
	}
	out := models.Import{ID: ids.New(repositories.PrefixImport), OrganizationID: t.OrganizationID, BranchID: in.BranchID, MediaFileID: in.MediaFileID, FileFormat: format, Status: models.StatusUploaded, RequestFingerprint: fingerprint(t.OrganizationID, in.BranchID, in.MediaFileID, format), CreatedBy: t.ActorID}
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		out, x = s.opts.Repository.Create(ctx, tx, out)
		if errors.Is(x, repositories.ErrConflict) {
			return errorcodes.Conflict("bulk_import_conflict", "This import already exists.")
		}
		if x != nil {
			return x
		}
		if x = s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "bulk_imports.created", "bulk_import", out.ID)); x != nil {
			return x
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, "bulk_import.created.v1", "bulk_import", out.ID, out.Version, t.OrganizationID, t.CorrelationID, map[string]any{"importId": out.ID, "mediaFileId": out.MediaFileID})
		}
		return nil
	})
	return out, e
}

// isLegacy reports whether an import belongs to this domain's legacy
// (engine=LEGACY) surface. Every legacy read/write method below checks this
// so a generic-engine import id — reachable by guessing, since ids share one
// namespace across both engines — can never be read or mutated through the
// legacy path, and vice versa (see the mirroring checks in generic.go).
func isLegacy(i models.Import) bool { return i.Engine != models.EngineGeneric }

func (s *Service) Get(ctx context.Context, t tenant.Context, id string) (models.Import, []models.Row, error) {
	if e := s.gate(false); e != nil {
		return models.Import{}, nil, e
	}
	i, e := s.opts.Repository.Get(ctx, nil, t.OrganizationID, id)
	if errors.Is(e, repositories.ErrNotFound) || (e == nil && (!t.BranchPermitted(i.BranchID) || !isLegacy(i))) {
		return models.Import{}, nil, errorcodes.NotFound("bulk_import_not_found", "Bulk import not found.")
	}
	if e != nil {
		return i, nil, e
	}
	rows, e := s.opts.Repository.ListRows(ctx, nil, t.OrganizationID, id, "")
	return i, rows, e
}
func (s *Service) List(ctx context.Context, t tenant.Context) ([]models.Import, error) {
	if e := s.gate(false); e != nil {
		return nil, e
	}
	all, e := s.opts.Repository.List(ctx, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	out := make([]models.Import, 0, len(all))
	for _, i := range all {
		if isLegacy(i) {
			out = append(out, i)
		}
	}
	return out, nil
}
func (s *Service) Cancel(ctx context.Context, t tenant.Context, id string, version int64) (models.Import, error) {
	if e := s.gate(true); e != nil {
		return models.Import{}, e
	}
	if version <= 0 {
		return models.Import{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	i, e := s.opts.Repository.Get(ctx, nil, t.OrganizationID, id)
	if e != nil || !t.BranchPermitted(i.BranchID) || !isLegacy(i) {
		return models.Import{}, errorcodes.NotFound("bulk_import_not_found", "Bulk import not found.")
	}
	out, e := s.opts.Repository.Cancel(ctx, nil, t.OrganizationID, id, version)
	if e != nil {
		return out, errorcodes.Conflict("bulk_import_conflict", "Import can no longer be cancelled.")
	}
	return out, nil
}

func (s *Service) Replay(ctx context.Context, t tenant.Context, id string) error {
	if e := s.gate(true); e != nil {
		return e
	}
	i, e := s.opts.Repository.Get(ctx, nil, t.OrganizationID, id)
	if e != nil || !t.BranchPermitted(i.BranchID) || !isLegacy(i) {
		return errorcodes.NotFound("bulk_import_not_found", "Bulk import not found.")
	}
	if e = s.opts.Repository.Replay(ctx, t.OrganizationID, id); e != nil {
		return errorcodes.Conflict("bulk_import_conflict", "Only a failed import may be replayed.")
	}
	return nil
}
func parse(data []byte, format string) ([]models.OrderRow, error) {
	if format == "JSON" {
		var rows []models.OrderRow
		e := json.Unmarshal(data, &rows)
		return rows, e
	}
	r := csv.NewReader(bytes.NewReader(data))
	records, e := r.ReadAll()
	if e != nil {
		return nil, e
	}
	if len(records) < 2 {
		return []models.OrderRow{}, nil
	}
	header := map[string]int{}
	for i, h := range records[0] {
		header[strings.TrimSpace(h)] = i
	}
	get := func(rec []string, key string) string {
		i, ok := header[key]
		if !ok || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}
	out := make([]models.OrderRow, 0, len(records)-1)
	for n, rec := range records[1:] {
		out = append(out, models.OrderRow{RowReference: get(rec, "rowReference"), CustomerID: get(rec, "customerId"), CustomerReference: get(rec, "customerReference"), ServiceCatalogItemID: get(rec, "serviceCatalogItemId"), ContractID: get(rec, "contractId"), Timezone: get(rec, "timezone"), Priority: get(rec, "priority"), SpecialInstructions: get(rec, "specialInstructions")})
		if out[len(out)-1].RowReference == "" {
			out[len(out)-1].RowReference = fmt.Sprintf("row-%d", n+1)
		}
	}
	return out, nil
}
func rowFingerprint(r models.OrderRow) string {
	b, _ := json.Marshal(r)
	x := sha256.Sum256(b)
	return hex.EncodeToString(x[:])
}
