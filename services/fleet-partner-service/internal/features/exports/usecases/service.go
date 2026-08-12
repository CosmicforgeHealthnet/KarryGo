// Package usecases implements the Section 25D generic export framework.
//
// Generic export code never writes a domain table and never builds
// unrestricted SQL against another feature's schema. It calls a registered
// Exporter, one per dataset, whose adapter lives in internal/app so this
// package never imports another feature's repositories directly — the same
// port-in-composition-root rule bulk imports and orders already follow.
package usecases

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Exporter produces one dataset's rows for an export job. Every adapter:
//   - reads through its own domain's usecases/repository, tenant- and
//     branch-scoped exactly as an ordinary list call would be;
//   - returns a fixed, stable column set — never "whatever fields happened
//     to be on the struct";
//   - never returns more than the caller's maxRows bound.
type Exporter interface {
	// Type is the export_type this adapter serves, matching models.ValidExportType.
	Type() string
	// Columns is the fixed CSV header.
	Columns() []string
	// Rows returns up to maxRows data rows (header excluded) for the tenant
	// and filters.
	Rows(ctx context.Context, t tenant.Context, filters map[string]any, maxRows int) ([][]string, error)
}

// DownloadGrant is what a caller receives to fetch a completed export's file.
// It is returned once and never stored, matching the documents download
// contract this mirrors.
type DownloadGrant struct {
	Token       string
	Path        string
	ExpiresAt   time.Time
	ContentType string
	SizeBytes   int64
}

// MediaPort is the narrow slice of the Media authority this package needs:
// write the generated file once, and issue a fresh, short-lived read grant
// each time a caller asks — never a durable link.
type MediaPort interface {
	WriteExportFile(ctx context.Context, t tenant.Context, exportID string, content []byte) (mediaFileID string, err error)
	AuthorizeDownload(ctx context.Context, t tenant.Context, mediaFileID string) (DownloadGrant, error)
}

// Flags gate the write and worker paths. Every one defaults to false.
type Flags struct {
	Enabled       bool
	WritesEnabled bool
	WorkerEnabled bool
	KillSwitch    bool
}

// Options configure the export service.
type Options struct {
	Pool       *pgxpool.Pool
	Repository *repositories.Repository
	// Exporters is keyed by export type. A type recognized by
	// models.ValidExportType with no registered adapter here still refuses
	// at request time — recognizing the type and having wired an adapter for
	// it are two different facts.
	Exporters map[string]Exporter
	Media     MediaPort
	Metrics   *observability.Metrics
	Flags     Flags
	// MaxRows bounds one export's generation. A dataset with more rows than
	// this is truncated to the bound rather than the job failing outright.
	MaxRows int
	// RetentionTTL is how long a READY export stays downloadable before the
	// expiry worker marks it EXPIRED.
	RetentionTTL time.Duration
	Now          func() time.Time
}

// Service is the Section 25D export service.
type Service struct{ opts Options }

// NewService builds the service.
func NewService(opts Options) *Service {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Metrics == nil {
		opts.Metrics = observability.NewMetrics()
	}
	if opts.MaxRows <= 0 {
		opts.MaxRows = 50000
	}
	if opts.RetentionTTL <= 0 {
		opts.RetentionTTL = 7 * 24 * time.Hour
	}
	if opts.Exporters == nil {
		opts.Exporters = map[string]Exporter{}
	}
	return &Service{opts: opts}
}

func (s *Service) requireEnabled() error {
	if s.opts.Flags.KillSwitch {
		return errorcodes.Unavailable(errorcodes.FleetServiceUnavailable, "Exports are temporarily unavailable.")
	}
	if !s.opts.Flags.Enabled {
		return errorcodes.NotFound(errorcodes.ExportNotFound, "Exports are not available for this organization.")
	}
	return nil
}

func (s *Service) writesAllowed() error {
	if err := s.requireEnabled(); err != nil {
		return err
	}
	if !s.opts.Flags.WritesEnabled {
		return errorcodes.Unavailable(errorcodes.FleetServiceUnavailable, "Export requests are not enabled for this deployment.")
	}
	return nil
}

// RequestExportInput is the validated shape of a create-export request.
type RequestExportInput struct {
	Actor            tenant.Context
	BranchID         string
	ExportType       string
	Format           string
	Filters          map[string]any
	RequestedColumns []string
	IdempotencyKey   string
}

// RequestExport validates and queues a new export job. The job starts
// PENDING; a worker generates the file asynchronously — this call never
// blocks on dataset size.
func (s *Service) RequestExport(ctx context.Context, in RequestExportInput) (models.Job, error) {
	if err := s.writesAllowed(); err != nil {
		return models.Job{}, err
	}
	if err := in.Actor.Validate(); err != nil {
		return models.Job{}, errorcodes.Forbidden(errorcodes.ActorNotAuthorized, "Organization scope is required.")
	}
	exportType := strings.ToUpper(strings.TrimSpace(in.ExportType))
	format := strings.ToUpper(strings.TrimSpace(in.Format))
	if format == "" {
		format = models.FormatCSV
	}
	key := strings.TrimSpace(in.IdempotencyKey)

	switch {
	case !models.ValidExportType(exportType):
		return models.Job{}, errorcodes.BadRequest(errorcodes.ExportTypeInvalid, "exportType is not supported.")
	case !models.ValidFormat(format):
		return models.Job{}, errorcodes.BadRequest(errorcodes.ExportTypeInvalid, "format is not supported.")
	case key == "":
		return models.Job{}, errorcodes.BadRequest(errorcodes.ValidationFailed, "An idempotency key is required.")
	}
	if _, registered := s.opts.Exporters[exportType]; !registered {
		return models.Job{}, errorcodes.Unavailable(errorcodes.ExportTypeInvalid,
			"This export type is recognized but not yet available in this deployment.")
	}

	fingerprint := exportFingerprint(in.Actor.OrganizationID, in.BranchID, exportType, format, in.Filters, in.RequestedColumns)
	if replay, err := s.findReplay(ctx, in.Actor.OrganizationID, key, fingerprint); err != nil {
		return models.Job{}, err
	} else if replay != nil {
		return *replay, nil
	}

	job := models.Job{
		OrganizationID:     in.Actor.OrganizationID,
		BranchID:           in.BranchID,
		ExportType:         exportType,
		Format:             format,
		FilterSnapshot:     models.JSONMap(in.Filters).Clone(),
		RequestedColumns:   in.RequestedColumns,
		Status:             models.StatusPending,
		RequestFingerprint: fingerprint,
		IdempotencyKey:     key,
		CreatedBy:          in.Actor.ActorID,
	}
	created, err := s.opts.Repository.Create(ctx, nil, job)
	if err != nil {
		if errors.Is(err, repositories.ErrConflict) {
			return models.Job{}, errorcodes.Conflict(errorcodes.IdempotencyConflictCode,
				"An export request is already in progress.")
		}
		return models.Job{}, err
	}
	s.opts.Metrics.IncCounter("fleet_partner_exports_requested_total",
		map[string]string{"exportType": exportType}, 1)
	return created, nil
}

func (s *Service) findReplay(ctx context.Context, organizationID, key, fingerprint string) (*models.Job, error) {
	existing, err := s.opts.Repository.FindByIdempotencyKey(ctx, nil, organizationID, key)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if existing.RequestFingerprint != fingerprint {
		return nil, errorcodes.Conflict(errorcodes.IdempotencyConflictCode,
			"This idempotency key was already used for a different export request.")
	}
	return &existing, nil
}

// Get returns one export job inside the tenant scope.
func (s *Service) Get(ctx context.Context, organizationID, id string) (models.Job, error) {
	job, err := s.opts.Repository.Get(ctx, nil, organizationID, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return models.Job{}, errorcodes.NotFound(errorcodes.ExportNotFound, "Export job not found.")
	}
	return job, err
}

// List returns the organization's export jobs, newest first.
func (s *Service) List(ctx context.Context, organizationID string, limit int) ([]models.Job, error) {
	if organizationID == "" {
		return nil, errorcodes.Forbidden(errorcodes.ActorNotAuthorized, "Organization scope is required.")
	}
	return s.opts.Repository.List(ctx, organizationID, limit)
}

// Cancel stops a job that has not yet completed.
func (s *Service) Cancel(ctx context.Context, organizationID, id string, expectedVersion int64) (models.Job, error) {
	if err := s.writesAllowed(); err != nil {
		return models.Job{}, err
	}
	job, err := s.opts.Repository.Cancel(ctx, nil, organizationID, id, expectedVersion)
	if errors.Is(err, repositories.ErrConflict) {
		return models.Job{}, errorcodes.Conflict(errorcodes.ExportNotCancelable,
			"This export can no longer be cancelled.")
	}
	return job, err
}

// Replay moves a DEAD_LETTER job back to PENDING for another attempt.
func (s *Service) Replay(ctx context.Context, organizationID, id string) error {
	if err := s.writesAllowed(); err != nil {
		return err
	}
	err := s.opts.Repository.Replay(ctx, organizationID, id)
	if errors.Is(err, repositories.ErrConflict) {
		return errorcodes.Conflict(errorcodes.ExportNotReplayable,
			"Only a dead-lettered export can be replayed.")
	}
	return err
}

// RequestDownloadURL re-authorizes access to a completed export's file. The
// grant is short-lived and never persisted, matching the documents contract.
func (s *Service) RequestDownloadURL(ctx context.Context, t tenant.Context, id string) (DownloadGrant, error) {
	if err := s.requireEnabled(); err != nil {
		return DownloadGrant{}, err
	}
	job, err := s.Get(ctx, t.OrganizationID, id)
	if err != nil {
		return DownloadGrant{}, err
	}
	if !job.Downloadable(s.opts.Now()) {
		return DownloadGrant{}, s.downloadRefusal(job)
	}
	if s.opts.Media == nil {
		return DownloadGrant{}, errorcodes.Unavailable(errorcodes.MediaAuthorityUnavailable,
			"The export service is temporarily unavailable. Please try again.")
	}
	grant, err := s.opts.Media.AuthorizeDownload(ctx, t, job.MediaFileID)
	if err != nil {
		return DownloadGrant{}, err
	}
	s.opts.Metrics.IncCounter("fleet_partner_export_downloads_total",
		map[string]string{"exportType": job.ExportType}, 1)
	return grant, nil
}

func (s *Service) downloadRefusal(job models.Job) error {
	switch job.Status {
	case models.StatusExpired:
		return errorcodes.Forbidden(errorcodes.ExportExpiredCode, "This export has expired.")
	case models.StatusCancelled:
		return errorcodes.Forbidden(errorcodes.ExportCancelledCode, "This export was cancelled.")
	case models.StatusFailed, models.StatusDeadLetter:
		return errorcodes.Forbidden(errorcodes.ExportFailedCode, "This export failed to generate.")
	case models.StatusPending, models.StatusRunning:
		return errorcodes.Conflict(errorcodes.ExportNotReady, "This export is still being generated.")
	default:
		return errorcodes.Forbidden(errorcodes.ExportNotReady, "This export cannot be downloaded.")
	}
}

// exportFingerprint hashes the meaningful inputs of an export request so a
// reused idempotency key describing a different request is refused rather
// than silently replayed.
func exportFingerprint(organizationID, branchID, exportType, format string,
	filters map[string]any, columns []string) string {
	hasher := sha256.New()
	parts := []string{organizationID, branchID, exportType, format}
	if raw, err := json.Marshal(sortedFilters(filters)); err == nil {
		parts = append(parts, string(raw))
	}
	sortedColumns := append([]string(nil), columns...)
	sort.Strings(sortedColumns)
	parts = append(parts, strings.Join(sortedColumns, "|"))
	for _, part := range parts {
		_, _ = hasher.Write([]byte(part))
		_, _ = hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

// sortedFilters produces a deterministic representation of the filter map so
// two logically identical requests hash the same way regardless of Go's
// randomized map iteration order.
func sortedFilters(filters map[string]any) []string {
	keys := make([]string, 0, len(filters))
	for k := range filters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s=%v", k, filters[k]))
	}
	return out
}

// buildCSV renders a header and rows to CSV bytes.
func buildCSV(columns []string, rows [][]string) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write(columns); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
