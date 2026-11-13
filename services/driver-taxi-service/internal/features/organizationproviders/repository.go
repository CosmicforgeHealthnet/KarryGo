package organizationproviders

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/orgprovider"
)

// Operation types in the replay ledger.
const (
	opProvision  = "PROVISION_SPONSORED"
	opTransition = "LINK_TRANSITION"
)

// Repository owns every SQL statement of the feature.
type Repository struct{ db *pgxpool.Pool }

// NewRepository builds the repository.
func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const candidateSelect = `
	SELECT p.id::text, p.account_status, p.onboarding_status, p.identity_verification_status,
	       p.vehicle_verification_status, p.document_compliance_status,
	       COALESCE(p.managed_by_organization_id, ''), EXTRACT(EPOCH FROM p.updated_at)::bigint
	FROM taxi_providers p`

func scanCandidate(row pgx.Row) (Candidate, error) {
	var c Candidate
	err := row.Scan(&c.ProviderID, &c.AccountStatus, &c.OnboardingStatus, &c.IdentityVerificationStatus,
		&c.VehicleVerificationStatus, &c.DocumentComplianceStatus, &c.ManagedByOrgID, &c.SourceVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return Candidate{}, ErrNotFound
	}
	return c, err
}

// FindByEmail finds the driver whose account carries this email. The taxi
// OTP login resolves against the same column.
func (r *Repository) FindByEmail(ctx context.Context, email string) (Candidate, error) {
	return scanCandidate(r.db.QueryRow(ctx, candidateSelect+` WHERE lower(p.email) = $1 LIMIT 1`, email))
}

// FindByPhone finds the driver whose account carries this phone number.
func (r *Repository) FindByPhone(ctx context.Context, phone string) (Candidate, error) {
	return scanCandidate(r.db.QueryRow(ctx, candidateSelect+` WHERE p.phone = $1 LIMIT 1`, phone))
}

// GetProvider reads one driver's standing.
func (r *Repository) GetProvider(ctx context.Context, providerID string) (Candidate, error) {
	if _, err := uuid.Parse(providerID); err != nil {
		return Candidate{}, ErrNotFound
	}
	return scanCandidate(r.db.QueryRow(ctx, candidateSelect+` WHERE p.id = $1::uuid`, providerID))
}

// replay returns the stored answer for an idempotency key, ErrNotFound when
// there is none, or ErrIdempotencyConflict when the key was used for a
// different request.
func replay(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, operation, key, fingerprint string, out any) error {
	var raw []byte
	var stored string
	err := q.QueryRow(ctx, `SELECT response_body, request_fingerprint
		FROM taxi_organization_provider_operations
		WHERE operation_type = $1 AND idempotency_key = $2`, operation, key).Scan(&raw, &stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if stored != fingerprint {
		return ErrIdempotencyConflict
	}
	return json.Unmarshal(raw, out)
}

func recordOperation(ctx context.Context, tx pgx.Tx, operation, key, fingerprint, resource string, response any) error {
	raw, err := json.Marshal(response)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO taxi_organization_provider_operations
		(operation_type, idempotency_key, request_fingerprint, resource_id, response_body)
		VALUES ($1, $2, $3, $4, $5)`, operation, key, fingerprint, resource, raw)
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ProvisionSponsored creates one organization-sponsored driver account with
// onboarding marked complete so the independent onboarding screens are
// skipped. No verification dimension is marked: the driver still has to pass
// identity, vehicle and document checks. They sign in with the existing
// passwordless OTP flow, which finds this account by email or phone.
func (r *Repository) ProvisionSponsored(ctx context.Context, key, fingerprint string, in Sponsored) (orgprovider.ProviderView, error) {
	var out orgprovider.ProviderView
	if err := replay(ctx, r.db, opProvision, key, fingerprint, &out); !errors.Is(err, ErrNotFound) {
		return out, err
	}
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var id string
		var version int64
		if err := tx.QueryRow(ctx, `INSERT INTO taxi_providers
				(phone, email, first_name, last_name, onboarding_status, managed_by_organization_id, onboarding_mode)
			VALUES ($1, $2, $3, $4, 'complete', $5, 'ORGANIZATION_SPONSORED')
			RETURNING id::text, EXTRACT(EPOCH FROM updated_at)::bigint`,
			in.Phone, in.Email, in.FirstName, in.LastName, in.OrganizationID).Scan(&id, &version); err != nil {
			return err
		}
		out = orgprovider.ProviderView{ProviderID: id, AccountState: orgprovider.AccountPendingVerification, SourceVersion: version}
		return recordOperation(ctx, tx, opProvision, key, fingerprint, id, out)
	})
	if err != nil && isUniqueViolation(err) {
		// Either a concurrent identical request won (replay its answer) or the
		// contact belongs to someone else (refuse; never merge).
		if replayErr := replay(ctx, r.db, opProvision, key, fingerprint, &out); replayErr == nil || errors.Is(replayErr, ErrIdempotencyConflict) {
			return out, replayErr
		}
		return orgprovider.ProviderView{}, ErrIdentityConflict
	}
	return out, err
}

const linkColumns = `link_id::text, organization_id, COALESCE(branch_id, ''), membership_id,
	provider_id::text, state, version, decided_at, created_at`

func scanLink(row pgx.Row) (LinkRecord, error) {
	var l LinkRecord
	err := row.Scan(&l.LinkID, &l.OrganizationID, &l.BranchID, &l.MembershipID, &l.ProviderID,
		&l.Status, &l.SourceVersion, &l.DecidedAt, &l.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return LinkRecord{}, ErrNotFound
	}
	return l, err
}

func linkByKey(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, key, fingerprint string) (LinkRecord, error) {
	var stored string
	var l LinkRecord
	err := q.QueryRow(ctx, `SELECT `+linkColumns+`, request_fingerprint
		FROM taxi_organization_provider_links WHERE idempotency_key = $1`, key).
		Scan(&l.LinkID, &l.OrganizationID, &l.BranchID, &l.MembershipID, &l.ProviderID,
			&l.Status, &l.SourceVersion, &l.DecidedAt, &l.CreatedAt, &stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return LinkRecord{}, ErrNotFound
	}
	if err != nil {
		return LinkRecord{}, err
	}
	if stored != fingerprint {
		return LinkRecord{}, ErrIdempotencyConflict
	}
	return l, nil
}

func history(ctx context.Context, tx pgx.Tx, l LinkRecord, prior, actorType, actorID, reason, correlation string) error {
	_, err := tx.Exec(ctx, `INSERT INTO taxi_organization_provider_link_history
		(link_id, organization_id, prior_state, new_state, source_version, actor_type, actor_id, reason_code, correlation_id)
		VALUES ($1::uuid, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, $9)`,
		l.LinkID, l.OrganizationID, prior, l.Status, l.SourceVersion, actorType, actorID, reason, correlation)
	return err
}

// CreateLink records a REQUESTED association. The same Idempotency-Key always
// returns the association it created, in its CURRENT state, so a retried
// request is also a read-back.
func (r *Repository) CreateLink(ctx context.Context, key, fingerprint, caller, correlation string, in orgprovider.LinkRequest) (LinkRecord, bool, error) {
	if l, err := linkByKey(ctx, r.db, key, fingerprint); !errors.Is(err, ErrNotFound) {
		return l, false, err
	}
	var out LinkRecord
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var candidate Candidate
		candidate, err := scanCandidate(tx.QueryRow(ctx, candidateSelect+` WHERE p.id = $1::uuid FOR UPDATE`, in.ProviderID))
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return ErrIneligible
			}
			return err
		}
		// A driver sponsored by another organization, or one the platform has
		// suspended, cannot be associated. Verification is
		// not required here: Fleet's own activation gate waits for it.
		if candidate.AccountState() == orgprovider.AccountSuspended ||
			(candidate.ManagedByOrgID != "" && candidate.ManagedByOrgID != in.OrganizationID) {
			return ErrIneligible
		}
		out, err = scanLink(tx.QueryRow(ctx, `INSERT INTO taxi_organization_provider_links
				(organization_id, provider_id, membership_id, branch_id, state, idempotency_key, request_fingerprint)
			VALUES ($1, $2::uuid, $3, NULLIF($4, ''), 'REQUESTED', $5, $6)
			RETURNING `+linkColumns,
			in.OrganizationID, in.ProviderID, in.MembershipID, in.BranchID, key, fingerprint))
		if err != nil {
			return err
		}
		return history(ctx, tx, out, "", "SERVICE", caller, "association_requested", correlation)
	})
	if err != nil && isUniqueViolation(err) {
		if l, replayErr := linkByKey(ctx, r.db, key, fingerprint); replayErr == nil || errors.Is(replayErr, ErrIdempotencyConflict) {
			return l, false, replayErr
		}
		return LinkRecord{}, false, ErrLinkConflict
	}
	return out, err == nil, err
}

// GetLink reads one association inside the calling organization only.
func (r *Repository) GetLink(ctx context.Context, linkID, organizationID string) (LinkRecord, error) {
	if _, err := uuid.Parse(linkID); err != nil {
		return LinkRecord{}, ErrNotFound
	}
	return scanLink(r.db.QueryRow(ctx, `SELECT `+linkColumns+` FROM taxi_organization_provider_links
		WHERE link_id = $1::uuid AND organization_id = $2`, linkID, organizationID))
}

// GetLiveLinkByProvider reads a driver's live association with an organization.
func (r *Repository) GetLiveLinkByProvider(ctx context.Context, providerID, organizationID string) (LinkRecord, error) {
	if _, err := uuid.Parse(providerID); err != nil {
		return LinkRecord{}, ErrNotFound
	}
	return scanLink(r.db.QueryRow(ctx, `SELECT `+linkColumns+` FROM taxi_organization_provider_links
		WHERE provider_id = $1::uuid AND organization_id = $2 AND state IN ('REQUESTED','ACTIVE','SUSPENDED')
		ORDER BY created_at DESC LIMIT 1`, providerID, organizationID))
}

// TransitionLink applies an organization-initiated change under optimistic
// concurrency. The provider's consent transitions are not reachable here.
func (r *Repository) TransitionLink(ctx context.Context, linkID, key, fingerprint, caller, correlation string, in orgprovider.TransitionRequest) (LinkRecord, error) {
	if _, err := uuid.Parse(linkID); err != nil {
		return LinkRecord{}, ErrNotFound
	}
	var out LinkRecord
	if err := replay(ctx, r.db, opTransition, key, fingerprint, &out); !errors.Is(err, ErrNotFound) {
		return out, err
	}
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		current, err := scanLink(tx.QueryRow(ctx, `SELECT `+linkColumns+` FROM taxi_organization_provider_links
			WHERE link_id = $1::uuid AND organization_id = $2 FOR UPDATE`, linkID, in.OrganizationID))
		if err != nil {
			return err
		}
		// A concurrent identical request may have committed while we waited
		// for the row lock.
		if err := replay(ctx, tx, opTransition, key, fingerprint, &out); !errors.Is(err, ErrNotFound) {
			if err == nil {
				return errReplayed
			}
			return err
		}
		if current.SourceVersion != in.ExpectedVersion || !orgprovider.OrganizationMayTransition(current.Status, in.State) {
			return ErrInvalidStateVersion
		}
		out, err = scanLink(tx.QueryRow(ctx, `UPDATE taxi_organization_provider_links
			SET state = $2, branch_id = COALESCE(NULLIF($3, ''), branch_id), version = version + 1, updated_at = now()
			WHERE link_id = $1::uuid RETURNING `+linkColumns, linkID, in.State, in.BranchID))
		if err != nil {
			return err
		}
		if err := history(ctx, tx, out, current.Status, "SERVICE", caller, in.ReasonCode, correlation); err != nil {
			return err
		}
		return recordOperation(ctx, tx, opTransition, key, fingerprint, linkID, out)
	})
	if errors.Is(err, errReplayed) {
		return out, nil
	}
	return out, err
}

var errReplayed = errors.New("organizationproviders: replayed")

// ListForProvider lists a driver's associations, newest first.
func (r *Repository) ListForProvider(ctx context.Context, providerID string) ([]LinkRecord, error) {
	rows, err := r.db.Query(ctx, `SELECT `+linkColumns+` FROM taxi_organization_provider_links
		WHERE provider_id = $1::uuid ORDER BY created_at DESC LIMIT 50`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LinkRecord{}
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Decide records the driver's own accept/decline of a REQUESTED association.
// Repeating the same decision returns the decided association unchanged.
func (r *Repository) Decide(ctx context.Context, providerID, linkID string, accept bool, expectedVersion int64) (LinkRecord, error) {
	if _, err := uuid.Parse(linkID); err != nil {
		return LinkRecord{}, ErrNotFound
	}
	target := orgprovider.ProviderDecision(accept)
	var out LinkRecord
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		current, err := scanLink(tx.QueryRow(ctx, `SELECT `+linkColumns+` FROM taxi_organization_provider_links
			WHERE link_id = $1::uuid AND provider_id = $2::uuid FOR UPDATE`, linkID, providerID))
		if err != nil {
			return err
		}
		if current.Status == target && current.DecidedAt != nil {
			out = current
			return nil
		}
		if current.Status != orgprovider.StateRequested {
			return ErrDecisionNotAvailable
		}
		if expectedVersion > 0 && expectedVersion != current.SourceVersion {
			return ErrInvalidStateVersion
		}
		out, err = scanLink(tx.QueryRow(ctx, `UPDATE taxi_organization_provider_links
			SET state = $2, decided_at = now(), version = version + 1, updated_at = now()
			WHERE link_id = $1::uuid RETURNING `+linkColumns, linkID, target))
		if err != nil {
			return err
		}
		reason := "provider_accepted"
		if !accept {
			reason = "provider_declined"
		}
		return history(ctx, tx, out, current.Status, "PROVIDER", providerID, reason, "")
	})
	return out, err
}

// ClaimNonce records a single-use private-call nonce in the shared taxi nonce
// table, failing when this caller already used it.
func (r *Repository) ClaimNonce(ctx context.Context, caller, nonce, path string) error {
	if caller == "" || nonce == "" || len(nonce) > 128 {
		return ErrInvalid
	}
	_, err := r.db.Exec(ctx, `INSERT INTO taxi_service_nonces (caller_service, nonce, request_path, expires_at)
		VALUES ($1, $2, $3, now() + interval '10 minutes')`, caller, nonce, path)
	if isUniqueViolation(err) {
		return ErrIdempotencyConflict
	}
	return err
}
