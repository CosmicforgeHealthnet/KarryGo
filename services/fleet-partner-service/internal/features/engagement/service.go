package engagement

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/shared/go/apperrors"
)

type Events interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}
type Options struct {
	Pool         *pgxpool.Pool
	Events       Events
	Audit        *audit.Recorder
	Queue        *jobs.Queue
	Media        platformclients.MediaContract
	Support      SupportPort
	Notification NotificationPort
	Flags        Flags
	Now          func() time.Time
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
func (s *Service) gate(on, write bool) error {
	if s.o.Flags.KillSwitch {
		return apperrors.Unavailable("Fleet integrations are disabled.", nil)
	}
	if !on {
		if write {
			return apperrors.Forbidden("This engagement capability is disabled.", nil)
		}
		return apperrors.NotFound("Resource not found.", nil)
	}
	return nil
}
func (s *Service) tx(ctx context.Context, f func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.o.Pool, f)
}
func (s *Service) record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, v int64, p any) error {
	if e := s.o.Audit.Record(ctx, tx, audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), kind, id)); e != nil {
		return e
	}
	if s.o.Events != nil {
		return s.o.Events.Publish(ctx, tx, event, kind, id, v, t.OrganizationID, t.CorrelationID, p)
	}
	return nil
}
func jsonRow(row pgx.Row) (map[string]any, error) {
	var raw []byte
	if e := row.Scan(&raw); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("Resource not found.", nil)
		}
		return nil, e
	}
	var out map[string]any
	if e := json.Unmarshal(raw, &out); e != nil {
		return nil, e
	}
	return out, nil
}
func jsonList(rows pgx.Rows) ([]map[string]any, error) {
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var raw []byte
		if e := rows.Scan(&raw); e != nil {
			return nil, e
		}
		var v map[string]any
		if e := json.Unmarshal(raw, &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func rawOr(v json.RawMessage, f string) json.RawMessage {
	if len(v) == 0 || !json.Valid(v) {
		return json.RawMessage(f)
	}
	return v
}
func expectedRequired(v int64) error {
	if v <= 0 {
		return apperrors.Conflict("expectedVersion is required.", nil)
	}
	return nil
}
func branchAllowed(t tenant.Context, b string) bool { return b == "" || t.BranchPermitted(b) }
func (s *Service) validateMedia(ctx context.Context, t tenant.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if s.o.Media == nil || !s.o.Media.Configured() {
		return apperrors.Unavailable("Media validation is unavailable.", nil)
	}
	for _, id := range ids {
		m, e := s.o.Media.GetFile(ctx, platformclients.CallMeta{OrganizationID: t.OrganizationID, CorrelationID: t.CorrelationID}, id)
		if e != nil {
			return apperrors.Unavailable("Media validation failed.", e)
		}
		if !m.Ready() || m.OwnerID != t.OrganizationID {
			return apperrors.Forbidden("Media is not owned and safe for this organization.", nil)
		}
	}
	return nil
}
