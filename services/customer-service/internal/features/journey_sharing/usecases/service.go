package shareusecases

import (
	"context"
	m "cosmicforge/logistics/services/customer-service/internal/features/journey_sharing/models"
	r "cosmicforge/logistics/services/customer-service/internal/features/journey_sharing/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/journeyshare"
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"time"
)

type Service struct {
	Repository              r.Repository
	Reader                  journeyshare.Reader
	Enabled                 bool
	TTL, CompletedRetention time.Duration
	Now                     func() time.Time
}
type Result struct {
	Session  m.Session `json:"session"`
	Token    string    `json:"token,omitempty"`
	Replayed bool      `json:"replayed"`
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s *Service) Gate() error {
	if !s.Enabled || s.Repository == nil || s.Reader == nil {
		return apperrors.Unavailable("Journey sharing is unavailable.", nil)
	}
	return nil
}
func command(owner, key, op string, input any) (r.Command, error) {
	if strings.TrimSpace(key) == "" || len(key) > 200 {
		return r.Command{}, apperrors.BadRequest("Idempotency-Key is required and must be bounded.", nil)
	}
	raw, _ := json.Marshal(input)
	return r.Command{Owner: owner, KeyHash: m.Digest(key), Fingerprint: m.Digest(op + ":" + string(raw)), Operation: op}, nil
}
func (s *Service) Create(ctx context.Context, owner, key string, in m.Create) (Result, error) {
	if err := s.Gate(); err != nil {
		return Result{}, err
	}
	if err := in.Validate(); err != nil {
		return Result{}, err
	}
	cmd, err := command(owner, key, "create", in)
	if err != nil {
		return Result{}, err
	}
	if prior, replay, e := s.Repository.Replay(ctx, cmd); e != nil || replay {
		return Result{Session: prior, Replayed: replay}, e
	}
	p, err := s.Reader.Read(ctx, in.Authority, in.SourceRef, owner)
	if err != nil {
		return Result{}, err
	}
	if p.CustomerID != owner || p.Authority != in.Authority || p.SourceRef != in.SourceRef || p.SourceVersion < 1 || p.ProductCode != in.ProductCode || !p.Eligible || p.Terminal {
		return Result{}, apperrors.NotFound("Journey is not shareable.", nil)
	}
	token, err := m.Token()
	if err != nil {
		return Result{}, apperrors.Unavailable("Share cannot be created.", nil)
	}
	now := s.now()
	ttl := s.TTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	cmd.Session = m.Session{ID: uuid.NewString(), CustomerID: owner, Authority: in.Authority, SourceRef: in.SourceRef, ProductCode: in.ProductCode, TokenHash: m.Digest(token), Permissions: in.Permissions, Status: "ACTIVE", ExpiresAt: now.Add(ttl), Version: 1, CreatedAt: now, UpdatedAt: now}
	out, replay, err := s.execute(ctx, cmd)
	if err != nil {
		return Result{}, err
	}
	if replay {
		token = ""
	}
	return Result{Session: out, Token: token, Replayed: replay}, nil
}
func (s *Service) Mutate(ctx context.Context, owner, id, key, op string, version int64) (Result, error) {
	if err := s.Gate(); err != nil {
		return Result{}, err
	}
	if version < 1 || (op != "rotate" && op != "revoke") {
		return Result{}, apperrors.BadRequest("A positive expected_version is required.", nil)
	}
	cmd, err := command(owner, key, op, struct {
		ID      string
		Version int64
	}{id, version})
	if err != nil {
		return Result{}, err
	}
	if prior, replay, e := s.Repository.Replay(ctx, cmd); e != nil || replay {
		return Result{Session: prior, Replayed: replay}, e
	}
	current, err := s.Repository.Get(ctx, id, owner)
	if err != nil {
		return Result{}, err
	}
	token := ""
	if op == "rotate" {
		p, e := s.Reader.Read(ctx, current.Authority, current.SourceRef, owner)
		if e != nil {
			return Result{}, e
		}
		if p.CustomerID != owner || p.Authority != current.Authority || p.SourceRef != current.SourceRef || p.ProductCode != current.ProductCode || p.SourceVersion < 1 || !p.Eligible || p.Terminal {
			return Result{}, apperrors.Conflict("Journey cannot be shared.", nil)
		}
		token, err = m.Token()
		if err != nil {
			return Result{}, err
		}
	}
	cmd.ExpectedVersion = version
	cmd.Session = m.Session{ID: id, TokenHash: m.Digest(token), UpdatedAt: s.now()}
	out, replay, err := s.execute(ctx, cmd)
	if err != nil {
		return Result{}, err
	}
	if replay {
		token = ""
	}
	return Result{Session: out, Token: token, Replayed: replay}, nil
}
func (s *Service) execute(ctx context.Context, cmd r.Command) (m.Session, bool, error) {
	out, replay, err := s.Repository.Execute(ctx, cmd)
	if err != nil {
		prior, found, readErr := s.Repository.Replay(ctx, cmd)
		if readErr == nil && found {
			return prior, true, nil
		}
	}
	return out, replay, err
}

// Sweep is bounded and observes source completion even when nobody opens a link.
func (s *Service) Sweep(ctx context.Context) error {
	if err := s.Gate(); err != nil {
		return err
	}
	now := s.now()
	if count, err := s.Repository.Expire(ctx, now, 200); err != nil {
		return err
	} else {
		for i := int64(0); i < count; i++ {
			journeyshare.Observe("expired", 0)
		}
	}
	sessions, err := s.Repository.Candidates(ctx, now, 50)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		p, e := s.Reader.Read(ctx, session.Authority, session.SourceRef, session.CustomerID)
		if e == nil && p.CustomerID == session.CustomerID && p.Authority == session.Authority && p.SourceRef == session.SourceRef && p.ProductCode == session.ProductCode && p.SourceVersion > 0 && p.Terminal {
			at := now
			if p.CompletedAt != nil {
				at = p.CompletedAt.UTC()
			}
			retention := s.CompletedRetention
			if retention <= 0 {
				retention = 15 * time.Minute
			}
			if _, e = s.Repository.ObserveCompletion(ctx, session.ID, session.Version, at, at.Add(retention)); e != nil {
				return e
			}
		}
		if err = s.Repository.Checked(ctx, session.ID, now); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) Public(ctx context.Context, token string) (journeyshare.PublicSnapshot, error) {
	if err := s.Gate(); err != nil {
		return journeyshare.PublicSnapshot{}, err
	}
	missing := apperrors.NotFound("Share was not found.", nil)
	if len(token) != 43 {
		return journeyshare.PublicSnapshot{}, missing
	}
	session, err := s.Repository.ByToken(ctx, m.Digest(token))
	if err != nil {
		return journeyshare.PublicSnapshot{}, err
	}
	now := s.now()
	if !session.Readable(now) {
		return journeyshare.PublicSnapshot{}, missing
	}
	p, err := s.Reader.Read(ctx, session.Authority, session.SourceRef, session.CustomerID)
	if err != nil {
		return journeyshare.PublicSnapshot{}, err
	}
	if p.CustomerID != session.CustomerID || p.Authority != session.Authority || p.SourceRef != session.SourceRef || p.SourceVersion < 1 || p.ProductCode != session.ProductCode || (!p.Eligible && !p.Terminal) {
		return journeyshare.PublicSnapshot{}, missing
	}
	if p.Terminal && session.Status == "ACTIVE" {
		completed := now
		if p.CompletedAt != nil {
			completed = p.CompletedAt.UTC()
		}
		retention := s.CompletedRetention
		if retention <= 0 {
			retention = 15 * time.Minute
		}
		session, err = s.Repository.ObserveCompletion(ctx, session.ID, session.Version, completed, completed.Add(retention))
		if err != nil {
			return journeyshare.PublicSnapshot{}, err
		}
	}
	// Re-read after authority I/O: concurrent revoke/rotation must not serve a cached capability.
	current, err := s.Repository.ByToken(ctx, m.Digest(token))
	if err != nil || !current.Readable(s.now()) {
		return journeyshare.PublicSnapshot{}, missing
	}
	if current.LastAccessAt == nil || current.LastAccessAt.Before(s.now().Add(-time.Minute)) {
		if err = s.Repository.Touch(ctx, current.ID, s.now()); err != nil {
			journeyshare.Observe("access_observation_error", 0)
		}
	}
	return p.Public(current.Permissions), nil
}
