package adminsnapshot

import (
	"context"
	"cosmicforge/logistics/shared/go/adminevents"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/outbox"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

var ErrScope = errors.New("verification event authorization unavailable")

// ScopeGuard checks explicit administrative allocations without changing business
// ownership or asserting that an unknown dimension is a native source fact.
type ScopeGuard struct{ Owner *adminowner.Handler }

func (g ScopeGuard) Configured() bool {
	if !(g.Owner != nil && g.Owner.Owner == Source && g.Owner.DB != nil && g.Owner.Contexts != nil && g.Owner.Contexts.DB == g.Owner.DB && g.Owner.Contexts.Reader != nil && g.Owner.Contexts.Authority.Owner == Source && len(g.Owner.Contexts.Authority.Keys) > 0 && g.Owner.Policy != nil) {
		return false
	}
	rule, err := g.Owner.Policy.Rule("verification.vendor_cases.read", time.Now())
	if err != nil || rule.Owner != Source || rule.SourceContract != "verification.admin-resources.v1" {
		return false
	}
	for _, purpose := range rule.Purposes {
		if purpose == "hq_event_sync" {
			return true
		}
	}
	return false
}

func (g ScopeGuard) Check(ctx context.Context, tx pgx.Tx, ev adminevents.SourceEvent) error {
	if !g.Configured() || tx == nil || ev.Validate() != nil || ev.SourceService != Source || ev.EntityType != EntityType || ev.EventType != adminevents.VerificationCaseChanged {
		return ErrScope
	}
	now := time.Now()
	rule, err := g.Owner.Policy.Rule("verification.vendor_cases.read", now)
	if err != nil || rule.Owner != Source || rule.SourceContract != "verification.admin-resources.v1" {
		return ErrScope
	}
	feedPurpose := false
	for _, purpose := range rule.Purposes {
		feedPurpose = feedPurpose || purpose == "hq_event_sync"
	}
	if !feedPurpose {
		return ErrScope
	}
	scope := adminpolicy.Scope{Product: ev.Product, Market: ev.Scope.Market, Zone: ev.Scope.Zone, Organization: ev.Scope.Organization, Tenant: ev.Scope.Tenant}
	allowed := false
	for _, approved := range rule.Scopes {
		allowed = allowed || approved == scope
	}
	if !allowed {
		return ErrScope
	}
	// This performs Ed25519 verification and checks current history/head, expiry,
	// revocation, capability and the actual record's canonical binding/fingerprint.
	if _, _, err = g.Owner.Contexts.ResolveWithExpiry(ctx, tx, "vendor-cases", ev.EntityRef, "verification.vendor_cases.read", scope, now); err != nil {
		return ErrScope
	}
	return nil
}

func (g ScopeGuard) Export(ctx context.Context, record outbox.Record) error {
	if !g.Configured() {
		return ErrScope
	}
	ev, err := adminevents.Decode(record.Payload)
	if err != nil || record.ID != ev.EventID || record.EventType != ev.EventType {
		return ErrScope
	}
	tx, err := g.Owner.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return ErrScope
	}
	defer tx.Rollback(ctx)
	if err = g.Check(ctx, tx, ev); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
