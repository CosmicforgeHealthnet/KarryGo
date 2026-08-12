package financeops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/shared/go/apperrors"
)

type Events interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}

type PostingPort interface {
	Submit(context.Context, platformclients.CallMeta, PostingRequest) (PostingOutcome, error)
	ReadByKey(context.Context, platformclients.CallMeta, PostingRequest) (PostingOutcome, error)
	Configured(operationType string) bool
}

type Options struct {
	Pool   *pgxpool.Pool
	Events Events
	Audit  *audit.Recorder
	Queue  *jobs.Queue
	Media  platformclients.MediaContract
	Wallet PostingPort
	Flags  Flags
	Now    func() time.Time
}

type Service struct{ o Options }

func NewService(o Options) *Service {
	if o.Audit == nil {
		o.Audit = audit.NewRecorder()
	}
	if o.Queue == nil {
		o.Queue = jobs.NewQueue(o.Pool)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Service{o: o}
}

func (s *Service) gate(enabled, write bool) error {
	if s.o.Flags.KillSwitch {
		return apperrors.Unavailable("Fleet integrations are disabled.", nil)
	}
	if !enabled {
		if write {
			return apperrors.Forbidden("This finance capability is disabled.", nil)
		}
		return apperrors.NotFound("Resource not found.", nil)
	}
	return nil
}

func (s *Service) tx(ctx context.Context, f func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.o.Pool, f)
}

func (s *Service) record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, version int64, payload any) error {
	if err := s.o.Audit.Record(ctx, tx, audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), kind, id)); err != nil {
		return err
	}
	if s.o.Events != nil {
		return s.o.Events.Publish(ctx, tx, event, kind, id, version, t.OrganizationID, t.CorrelationID, payload)
	}
	return nil
}

func normalizeCurrency(v string) (string, error) {
	v = strings.ToUpper(strings.TrimSpace(v))
	if len(v) != 3 {
		return "", apperrors.Validation("Currency must be a three-letter ISO code.", nil)
	}
	for _, r := range v {
		if r < 'A' || r > 'Z' {
			return "", apperrors.Validation("Currency must be a three-letter ISO code.", nil)
		}
	}
	return v, nil
}

func jsonRow(row pgx.Row) (map[string]any, error) {
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("Resource not found.", nil)
		}
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func jsonList(rows pgx.Rows) ([]map[string]any, error) {
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var v map[string]any
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func branchAllowed(t tenant.Context, branch string) bool {
	return branch == "" || t.BranchPermitted(branch)
}

func (s *Service) validateMedia(ctx context.Context, t tenant.Context, fileID string) error {
	if strings.TrimSpace(fileID) == "" {
		return nil
	}
	if s.o.Media == nil || !s.o.Media.Configured() {
		return apperrors.Unavailable("Media validation is unavailable.", nil)
	}
	meta, err := s.o.Media.GetFile(ctx, platformclients.CallMeta{OrganizationID: t.OrganizationID, CorrelationID: t.CorrelationID}, fileID)
	if err != nil {
		return apperrors.Unavailable("Media validation failed.", err)
	}
	if !meta.Ready() || meta.OwnerID != t.OrganizationID {
		return apperrors.Forbidden("Media is not owned and safe for this organization.", nil)
	}
	return nil
}

func fingerprint(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func rawOr(v json.RawMessage, fallback string) json.RawMessage {
	if len(v) == 0 || !json.Valid(v) {
		return json.RawMessage(fallback)
	}
	return v
}

func notFoundOnNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.NotFound("Resource not found.", nil)
	}
	return err
}

func expectedRequired(v int64) error {
	if v <= 0 {
		return apperrors.Conflict("expectedVersion is required.", nil)
	}
	return nil
}

func statusString(v map[string]any) string { x, _ := v["status"].(string); return x }

func int64Value(v map[string]any, key string) int64 {
	switch x := v[key].(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	case json.Number:
		n, _ := x.Int64()
		return n
	}
	return 0
}

func operationMeta(t tenant.Context, key string) platformclients.CallMeta {
	return platformclients.CallMeta{OrganizationID: t.OrganizationID, CorrelationID: t.CorrelationID, IdempotencyKey: key}
}

var _ = fmt.Sprintf
var _ = ids.New
