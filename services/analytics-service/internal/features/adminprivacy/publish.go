package adminprivacy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

const lineagePublishBase = analyticsadmin.SourceContextPath

var lineageOwners = []string{"driver-taxi-service", "driver-dispatch-delivery-service", "driver-hauling-service"}

// LineagePublisher accepts only independently signed owner allocations. It never
// issues approvals, computes missing mappings or declares complete coverage.
type LineagePublisher struct {
	handlers map[string]*adminowner.Handler
}

func (p *LineagePublisher) Register(root *gin.Engine) {
	for _, owner := range lineageOwners {
		h := p.handlers[owner]
		if h == nil {
			h = &adminowner.Handler{}
		}
		root.POST(lineagePublishBase+owner, h.PublishContext)
	}
}

// Publication is independent of HQ report rollout: revocations must still be
// ingestible when reads are killed. Its workload credentials cannot read reports.
func PublisherRuntime(ctx context.Context, db *pgxpool.Pool, legacy serviceauth.Secrets, readSecret []byte) (*LineagePublisher, error) {
	p := &LineagePublisher{handlers: map[string]*adminowner.Handler{}}
	if os.Getenv("ANALYTICS_ADMIN_CONTEXT_PUBLISH_ENABLED") != "true" {
		return p, nil
	}
	if db == nil {
		return nil, errors.New("Analytics publication database required")
	}
	authorities, err := sourceAuthorities(os.Getenv("ANALYTICS_ADMIN_SOURCE_CONTEXT_KEYS_FILE"))
	if err != nil || len(authorities) == 0 {
		return nil, errors.New("Analytics source publication authorities required")
	}
	raw, err := os.ReadFile(os.Getenv("ANALYTICS_ADMIN_CONTEXT_PUBLISH_SECRETS_FILE"))
	if err != nil || len(raw) > 65536 {
		return nil, errors.New("Analytics publisher credentials unavailable")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if uniqueKeyMap(d, 0) != nil {
		return nil, errors.New("ambiguous Analytics publisher credentials")
	}
	var encoded map[string]string
	if json.Unmarshal(raw, &encoded) != nil || len(encoded) != len(authorities) {
		return nil, errors.New("Analytics publisher credential owners mismatch")
	}
	used := [][]byte{}
	for owner, authority := range authorities {
		secret, err := base64.RawURLEncoding.DecodeString(encoded[owner])
		if err != nil || len(secret) < 32 {
			return nil, errors.New("invalid Analytics publisher credential")
		}
		for _, old := range append(used, readSecret) {
			if bytes.Equal(secret, old) {
				return nil, errors.New("Analytics publisher credential audience collision")
			}
		}
		for _, old := range legacy {
			if bytes.Equal(secret, old) {
				return nil, errors.New("Analytics publisher cannot reuse legacy credentials")
			}
		}
		used = append(used, secret)
		p.handlers[owner] = &adminowner.Handler{Owner: owner, DB: db, PublishEnabled: true, PublishSecret: secret, Contexts: &admincontext.Store{DB: db, Reader: LineageReader{Owner: owner}, Authority: authority}}
	}
	ready, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var ok bool
	if err := db.QueryRow(ready, `SELECT to_regclass('admin_owner_read_nonces') IS NOT NULL AND to_regclass('admin_owner_context_history') IS NOT NULL`).Scan(&ok); err != nil || !ok {
		return nil, errors.New("Analytics publication schema unavailable; apply normal migrations")
	}
	return p, nil
}
