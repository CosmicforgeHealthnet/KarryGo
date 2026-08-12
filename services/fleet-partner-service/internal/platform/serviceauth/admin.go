package serviceauth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/riskregistry"
)

// internalRoutePrefix bounds which route patterns a credential may ever
// declare: only the internal HMAC route class is reachable by a service
// credential at all, so validating against tenant/public route paths here
// would be meaningless -- and a credential accidentally scoped to one by
// typo should be refused, not silently inert.
const internalRoutePrefix = "/internal/fleet-partner/v1"

var validMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
}

// knownInternalRoutePaths is the closed set of real, currently-mounted
// internal route paths, derived from internal/platform/riskregistry -- the
// SAME closed table TestRouteRiskRegistry_CoversEveryMountedRoute already
// keeps in sync with what internal/app.BuildRouter actually mounts. A
// credential's AllowedRoutePatterns may only ever name a path from this set
// (or the Wildcard) -- never an invented, misspelled or stale route
// identifier that could never match a real request, which would silently
// make a "scoped" credential either useless or (worse, if the typo happens
// to coincide with a future real route) unexpectedly permissive later.
func knownInternalRoutePaths() map[string]bool {
	paths := map[string]bool{}
	for _, entry := range riskregistry.All() {
		if strings.HasPrefix(entry.Path, internalRoutePrefix) {
			paths[entry.Path] = true
		}
	}
	return paths
}

// ErrValidationFailed wraps a scope-declaration validation failure with the
// offending field, so the HTTP layer can render a precise 422.
type ErrValidationFailed struct {
	Field  string
	Detail string
}

func (e ErrValidationFailed) Error() string {
	return fmt.Sprintf("serviceauth: %s: %s", e.Field, e.Detail)
}

func validateScope(methods, routes, events []string) error {
	if len(methods) == 0 {
		return ErrValidationFailed{"allowedMethods", "at least one method or \"*\" is required"}
	}
	for _, m := range methods {
		if m != Wildcard && !validMethods[strings.ToUpper(m)] {
			return ErrValidationFailed{"allowedMethods", fmt.Sprintf("%q is not a recognized HTTP method", m)}
		}
	}
	if len(routes) == 0 {
		return ErrValidationFailed{"allowedRoutePatterns", "at least one route pattern or \"*\" is required"}
	}
	known := knownInternalRoutePaths()
	for _, r := range routes {
		if r == Wildcard {
			continue
		}
		if !strings.HasPrefix(r, internalRoutePrefix) {
			return ErrValidationFailed{"allowedRoutePatterns",
				fmt.Sprintf("%q is not an internal route path (must start with %s)", r, internalRoutePrefix)}
		}
		if !known[r] {
			return ErrValidationFailed{"allowedRoutePatterns",
				fmt.Sprintf("%q is not a currently registered internal route", r)}
		}
	}
	// events has no closed registry to validate against -- event type
	// vocabularies are domain-defined per producer (finance, marketplace,
	// documents), not centrally enumerable the way routes are.
	_ = events
	return nil
}

// Actor identifies who performed an administrative action. Deliberately not
// tenant.Context: this table has no organization_id at all (service
// credentials are a platform-wide concern, not a per-organization one), so
// the actor here is always a platform operator or an automated rotation
// job, never an organization user -- see migration 000071's own comment on
// why this permission is granted to no tenant role.
type Actor struct {
	ActorType string
	ActorID   string
}

// Administrator implements the credential lifecycle: create, rotate,
// revoke, list. Every mutation is audited in the same transaction as the
// row change, matching this service's universal atomic mutation+audit
// convention.
type Administrator struct {
	pool     *pgxpool.Pool
	store    *Store
	recorder *audit.Recorder
}

// NewAdministrator builds the administrator.
func NewAdministrator(pool *pgxpool.Pool, recorder *audit.Recorder) *Administrator {
	if recorder == nil {
		recorder = audit.NewRecorder()
	}
	return &Administrator{pool: pool, store: NewStore(pool), recorder: recorder}
}

// CreateInput describes a brand-new caller's first credential (version 1).
type CreateInput struct {
	Actor                Actor
	ServiceName          string
	AllowedMethods       []string
	AllowedRoutePatterns []string
	AllowedEventTypes    []string
	ExpiresAt            *time.Time
	VersionNote          string
	RequestID            string
	CorrelationID        string
}

// Create inserts version 1 for a service name that has no existing
// credential at all. Refuses (Conflict) if one already exists -- Rotate is
// the only way to add a version to an existing caller, so version history
// can never have a gap or overlap introduced by two different code paths.
func (a *Administrator) Create(ctx context.Context, input CreateInput) (Credential, error) {
	serviceName := strings.TrimSpace(input.ServiceName)
	if serviceName == "" {
		return Credential{}, ErrValidationFailed{"serviceName", "is required"}
	}
	if err := validateScope(input.AllowedMethods, input.AllowedRoutePatterns, input.AllowedEventTypes); err != nil {
		return Credential{}, err
	}

	var created Credential
	err := database.WithinTx(ctx, a.pool, func(tx pgx.Tx) error {
		txStore := NewStore(tx)
		existing, err := txStore.LatestVersion(ctx, serviceName)
		if err != nil {
			return err
		}
		if existing > 0 {
			return apperrors.Conflict("A credential already exists for this service; use rotate instead.", nil)
		}
		created = Credential{
			ID:          ids.New(ids.PrefixServiceCredential),
			ServiceName: serviceName, Version: 1, ActiveFrom: time.Now().UTC(),
			AllowedMethods: input.AllowedMethods, AllowedRoutePatterns: input.AllowedRoutePatterns,
			AllowedEventTypes: input.AllowedEventTypes, ExpiresAt: input.ExpiresAt,
			CreatedBy: input.Actor.ActorID, VersionNote: input.VersionNote,
		}
		if err := txStore.Create(ctx, tx, created); err != nil {
			return err
		}
		return a.recorder.Record(ctx, tx, audit.Entry{
			OrganizationID: platformOrganizationSentinel, ActorType: input.Actor.ActorType, ActorID: input.Actor.ActorID,
			Action: "service_credential.created", ResourceType: "service_credential", ResourceID: created.ID,
			NewValues: map[string]any{
				"serviceName": serviceName, "version": 1,
				"allowedMethods": input.AllowedMethods, "allowedRoutePatterns": input.AllowedRoutePatterns,
				"allowedEventTypes": input.AllowedEventTypes,
			},
			RequestID: input.RequestID, CorrelationID: input.CorrelationID,
		})
	})
	if err != nil {
		return Credential{}, err
	}
	return created, nil
}

// platformOrganizationSentinel is the OrganizationID audit.Entry requires
// (it is mandatory -- see audit.Recorder.Record) for an action that has no
// real organization at all. A fixed, documented, non-opaque-ID-shaped
// sentinel rather than an empty string: audit.Record already refuses an
// empty OrganizationID outright, and a real-looking but fake org id would
// risk being mistaken for one. Chosen to sort predictably and read
// obviously as "not a real organization" in any audit query or export.
const platformOrganizationSentinel = "PLATFORM"

// RotateInput describes adding a new version for an existing caller.
type RotateInput struct {
	Actor                Actor
	ServiceName          string
	AllowedMethods       []string
	AllowedRoutePatterns []string
	AllowedEventTypes    []string
	ExpiresAt            *time.Time
	// OverlapWindow bounds how long the PREVIOUS latest active version
	// keeps working after this rotation -- long enough for the caller to
	// pick up its new secret, short enough that the old one cannot be used
	// indefinitely. A zero value defaults to 24h.
	OverlapWindow time.Duration
	VersionNote   string
	RequestID     string
	CorrelationID string
}

// Rotate creates version N+1 (active immediately) and bounds every
// currently-active older version's expiry to now()+OverlapWindow, never
// EXTENDING an already-sooner expiry (a credential someone already
// deliberately set to expire sooner keeps that shorter deadline). Refuses
// (NotFound) if serviceName has no existing credential -- Create is the
// only way to establish version 1.
func (a *Administrator) Rotate(ctx context.Context, input RotateInput) (Credential, error) {
	serviceName := strings.TrimSpace(input.ServiceName)
	if serviceName == "" {
		return Credential{}, ErrValidationFailed{"serviceName", "is required"}
	}
	if err := validateScope(input.AllowedMethods, input.AllowedRoutePatterns, input.AllowedEventTypes); err != nil {
		return Credential{}, err
	}
	overlap := input.OverlapWindow
	if overlap <= 0 {
		overlap = 24 * time.Hour
	}

	var rotated Credential
	err := database.WithinTx(ctx, a.pool, func(tx pgx.Tx) error {
		txStore := NewStore(tx)
		latest, err := txStore.LatestVersion(ctx, serviceName)
		if err != nil {
			return err
		}
		if latest == 0 {
			return apperrors.NotFound("No existing credential for this service; use create for the first version.", nil)
		}

		newExpiry := time.Now().UTC().Add(overlap)
		active, err := txStore.ActiveCandidates(ctx, serviceName)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, candidate := range active {
			if !candidate.IsActive(now) {
				continue
			}
			bound := newExpiry
			if candidate.ExpiresAt != nil && candidate.ExpiresAt.Before(bound) {
				bound = *candidate.ExpiresAt // never extend an already-sooner expiry
			}
			if err := txStore.SetExpiry(ctx, tx, candidate.ID, bound); err != nil {
				return err
			}
		}

		rotated = Credential{
			ID:          ids.New(ids.PrefixServiceCredential),
			ServiceName: serviceName, Version: latest + 1, ActiveFrom: now,
			AllowedMethods: input.AllowedMethods, AllowedRoutePatterns: input.AllowedRoutePatterns,
			AllowedEventTypes: input.AllowedEventTypes, ExpiresAt: input.ExpiresAt,
			CreatedBy: input.Actor.ActorID, VersionNote: input.VersionNote,
		}
		if err := txStore.Create(ctx, tx, rotated); err != nil {
			return err
		}
		return a.recorder.Record(ctx, tx, audit.Entry{
			OrganizationID: platformOrganizationSentinel, ActorType: input.Actor.ActorType, ActorID: input.Actor.ActorID,
			Action: "service_credential.rotated", ResourceType: "service_credential", ResourceID: rotated.ID,
			NewValues: map[string]any{
				"serviceName": serviceName, "newVersion": latest + 1, "previousVersion": latest,
				"overlapWindowSeconds": int(overlap.Seconds()),
			},
			RequestID: input.RequestID, CorrelationID: input.CorrelationID,
		})
	})
	if err != nil {
		return Credential{}, err
	}
	return rotated, nil
}

// RevokeInput describes an immediate revocation.
type RevokeInput struct {
	Actor         Actor
	CredentialID  string
	Reason        string
	RequestID     string
	CorrelationID string
}

// Revoke immediately invalidates one specific credential version --
// distinct from expiry (a scheduled, already-known end time): revocation
// is an unplanned, immediate stop, e.g. a suspected secret leak.
func (a *Administrator) Revoke(ctx context.Context, input RevokeInput) error {
	if strings.TrimSpace(input.CredentialID) == "" {
		return ErrValidationFailed{"credentialId", "is required"}
	}
	if strings.TrimSpace(input.Reason) == "" {
		return ErrValidationFailed{"reason", "is required"}
	}
	return database.WithinTx(ctx, a.pool, func(tx pgx.Tx) error {
		txStore := NewStore(tx)
		existing, err := txStore.GetByID(ctx, input.CredentialID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return apperrors.NotFound("Credential not found.", nil)
			}
			return err
		}
		if err := txStore.Revoke(ctx, tx, input.CredentialID, input.Reason); err != nil {
			if errors.Is(err, ErrNotFound) {
				return apperrors.Conflict("Credential is already revoked.", nil)
			}
			return err
		}
		return a.recorder.Record(ctx, tx, audit.Entry{
			OrganizationID: platformOrganizationSentinel, ActorType: input.Actor.ActorType, ActorID: input.Actor.ActorID,
			Action: "service_credential.revoked", ResourceType: "service_credential", ResourceID: input.CredentialID,
			Reason: input.Reason,
			NewValues: map[string]any{
				"serviceName": existing.ServiceName, "version": existing.Version,
			},
			RequestID: input.RequestID, CorrelationID: input.CorrelationID,
		})
	})
}

// List returns every credential (all versions, including revoked) for
// serviceName -- safe by construction, no secret column exists to redact.
func (a *Administrator) List(ctx context.Context, serviceName string) ([]Credential, error) {
	return a.store.List(ctx, serviceName)
}

// ListServices returns every distinct caller name with at least one
// credential.
func (a *Administrator) ListServices(ctx context.Context) ([]string, error) {
	return a.store.ListAllServices(ctx)
}
