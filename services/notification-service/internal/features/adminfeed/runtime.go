package adminfeed

import (
	"context"
	"cosmicforge/logistics/shared/go/admincampaign"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicyenv"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"time"
)

var ErrConfiguration = errors.New("invalid notification admin feed configuration")

// Runtime only configures the existing source-side owner context mechanism.
// Ordinary campaign sending, approvals and delivery outcomes are unchanged.
func Runtime(ctx context.Context, db *pgxpool.Pool) (*adminowner.Handler, ScopeGuard, error) {
	if os.Getenv("NOTIFICATION_ADMIN_RESOURCE_READ_ENABLED") == "true" {
		return nil, ScopeGuard{}, ErrConfiguration
	}
	definitions := map[string]adminowner.Definition{"campaign-revisions": {Kind: "campaign-revisions", Capability: "hq.campaigns.readback", SourceContract: admincampaign.Contract}}
	h, err := adminowner.FromEnvironment(ctx, db, SourceReader{}, definitions, Source, "NOTIFICATION", "/internal/notification/v1/admin")
	if err != nil {
		return nil, ScopeGuard{}, err
	}
	guard := ScopeGuard{Owner: h}
	enabled := os.Getenv("NOTIFICATION_ADMIN_EVENTS_ENABLED") == "true" || os.Getenv("NOTIFICATION_ADMIN_EVENTS_SNAPSHOT_ENABLED") == "true"
	if !enabled {
		return h, guard, nil
	}
	var ready bool
	if db == nil {
		return nil, ScopeGuard{}, ErrConfiguration
	}
	if err := db.QueryRow(ctx, `SELECT to_regprocedure('notification_admin_event_scope(text)') IS NOT NULL`).Scan(&ready); err != nil || !ready {
		return nil, ScopeGuard{}, ErrConfiguration
	}
	if err := checkTransportSecrets(); err != nil {
		return nil, ScopeGuard{}, err
	}
	if !h.PublishEnabled {
		return nil, ScopeGuard{}, ErrConfiguration
	}
	h.Policy, err = adminpolicy.LoadFiles(os.Getenv("NOTIFICATION_ADMIN_ACCESS_POLICY_FILE"), os.Getenv("NOTIFICATION_ADMIN_ACCESS_POLICY_KEYS_FILE"), adminpolicyenv.Environment, time.Now())
	if err != nil || h.Policy == nil {
		return nil, ScopeGuard{}, ErrConfiguration
	}
	if err := adminpolicy.PinRevision(ctx, db, "admin_owner_policy_revisions", h.Policy); err != nil {
		return nil, ScopeGuard{}, ErrConfiguration
	}
	if !guard.Configured() {
		return nil, ScopeGuard{}, ErrConfiguration
	}
	return h, guard, nil
}
