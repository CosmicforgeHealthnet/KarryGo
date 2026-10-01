package referralrepositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/services/referral-service/internal/database"
	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
)

// ErrNotFound is returned when a looked-up row does not exist.
var ErrNotFound = errors.New("not found")

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// IsUniqueViolation reports a unique-constraint violation.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// LockIdentities serializes work on the given identity hashes for the rest of
// the transaction, so two sign-ups sharing a phone or email cannot each
// create "the" person. Locks are taken in sorted order to avoid deadlocks.
func (s *Store) LockIdentities(ctx context.Context, q database.Querier, keys ...string) error {
	var sorted []string
	for _, key := range keys {
		if key != "" {
			sorted = append(sorted, key)
		}
	}
	sort.Strings(sorted)
	for _, key := range sorted {
		if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 7071))`, key); err != nil {
			return fmt.Errorf("lock identity: %w", err)
		}
	}
	return nil
}

// PersonIDByFingerprint finds the person a phone or email hash belongs to.
func (s *Store) PersonIDByFingerprint(ctx context.Context, q database.Querier, kind, valueHash string) (string, error) {
	if valueHash == "" {
		return "", ErrNotFound
	}
	var personID string
	err := q.QueryRow(ctx, `SELECT person_id FROM person_fingerprints WHERE kind = $1 AND value_hash = $2 LIMIT 1`, kind, valueHash).Scan(&personID)
	return personID, notFound(err)
}

// PersonsSharingFingerprint lists other people holding the same hash (used
// for devices and bank accounts, which may legitimately be shared).
func (s *Store) PersonsSharingFingerprint(ctx context.Context, q database.Querier, kind, valueHash, exceptPersonID string) ([]string, error) {
	rows, err := q.Query(ctx, `
		SELECT person_id FROM person_fingerprints
		WHERE kind = $1 AND value_hash = $2 AND person_id <> $3
		ORDER BY first_seen_at LIMIT 20`, kind, valueHash, exceptPersonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// AddFingerprint records a hash for a person. A phone or email already owned
// by someone else is left alone (reported false) -- the caller flags it.
func (s *Store) AddFingerprint(ctx context.Context, q database.Querier, kind, valueHash, personID string) (bool, error) {
	if valueHash == "" {
		return false, nil
	}
	if kind == "phone" || kind == "email" {
		owner, err := s.PersonIDByFingerprint(ctx, q, kind, valueHash)
		if err == nil {
			return owner == personID, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return false, err
		}
	}
	_, err := q.Exec(ctx, `
		INSERT INTO person_fingerprints (kind, value_hash, person_id) VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, kind, valueHash, personID)
	if err != nil {
		return false, fmt.Errorf("add fingerprint: %w", err)
	}
	return true, nil
}

// RecordFingerprint records a shareable hash (device, bank) for a person and
// reports whether the person had not been seen with it before.
func (s *Store) RecordFingerprint(ctx context.Context, q database.Querier, kind, valueHash, personID string) (bool, error) {
	if valueHash == "" {
		return false, nil
	}
	tag, err := q.Exec(ctx, `
		INSERT INTO person_fingerprints (kind, value_hash, person_id) VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, kind, valueHash, personID)
	if err != nil {
		return false, fmt.Errorf("record fingerprint: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// CreatePerson inserts a person whose home account is the given actor.
func (s *Store) CreatePerson(ctx context.Context, q database.Querier, homeActorType, homeActorID string) (referralmodels.Person, error) {
	person := referralmodels.Person{
		ID: "rp_" + uuid.NewString(), Status: referralmodels.PersonActive,
		HomeActorType: homeActorType, HomeActorID: homeActorID,
	}
	err := q.QueryRow(ctx, `
		INSERT INTO referral_persons (id, status, home_actor_type, home_actor_id)
		VALUES ($1, $2, $3, $4) RETURNING created_at`,
		person.ID, person.Status, homeActorType, homeActorID).Scan(&person.CreatedAt)
	if err != nil {
		return referralmodels.Person{}, fmt.Errorf("create person: %w", err)
	}
	return person, nil
}

// GetPerson loads a person; lock takes a row lock for the transaction.
func (s *Store) GetPerson(ctx context.Context, q database.Querier, id string, lock bool) (referralmodels.Person, error) {
	query := `SELECT id, status, status_reason, home_actor_type, home_actor_id, power_user_since, created_at
		FROM referral_persons WHERE id = $1`
	if lock {
		query += ` FOR UPDATE`
	}
	var p referralmodels.Person
	err := q.QueryRow(ctx, query, id).Scan(&p.ID, &p.Status, &p.StatusReason, &p.HomeActorType, &p.HomeActorID, &p.PowerUserSince, &p.CreatedAt)
	return p, notFound(err)
}

// PersonHasFingerprint reports whether a person has been seen with a hash.
func (s *Store) PersonHasFingerprint(ctx context.Context, q database.Querier, personID, kind, valueHash string) (bool, error) {
	if valueHash == "" {
		return false, nil
	}
	var exists bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM person_fingerprints WHERE person_id = $1 AND kind = $2 AND value_hash = $3)`,
		personID, kind, valueHash).Scan(&exists)
	return exists, err
}

// SetPersonStatus changes a person's standing.
func (s *Store) SetPersonStatus(ctx context.Context, q database.Querier, id, status, reason string) error {
	tag, err := q.Exec(ctx, `UPDATE referral_persons SET status = $2, status_reason = $3, updated_at = now() WHERE id = $1`, id, status, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// InsertActor records an account.
func (s *Store) InsertActor(ctx context.Context, q database.Querier, actor referralmodels.Actor) error {
	_, err := q.Exec(ctx, `
		INSERT INTO referral_actors (actor_type, actor_id, person_id, registered_at, is_new_account, attribution_outcome, location_state)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		actor.ActorType, actor.ActorID, actor.PersonID, actor.RegisteredAt, actor.IsNewAccount, actor.AttributionOutcome, actor.LocationState)
	if err != nil {
		return fmt.Errorf("insert actor: %w", err)
	}
	return nil
}

// SetAttributionOutcome records what happened to a sign-up's code.
func (s *Store) SetAttributionOutcome(ctx context.Context, q database.Querier, actorType, actorID, outcome string) error {
	_, err := q.Exec(ctx, `UPDATE referral_actors SET attribution_outcome = $3 WHERE actor_type = $1 AND actor_id = $2`, actorType, actorID, outcome)
	return err
}

// GetActor loads one account.
func (s *Store) GetActor(ctx context.Context, q database.Querier, actorType, actorID string) (referralmodels.Actor, error) {
	var a referralmodels.Actor
	err := q.QueryRow(ctx, `
		SELECT actor_type, actor_id, person_id, registered_at, is_new_account, attribution_outcome, location_state
		FROM referral_actors WHERE actor_type = $1 AND actor_id = $2`, actorType, actorID).
		Scan(&a.ActorType, &a.ActorID, &a.PersonID, &a.RegisteredAt, &a.IsNewAccount, &a.AttributionOutcome, &a.LocationState)
	return a, notFound(err)
}

// ActorsForPerson lists a person's accounts, oldest first.
func (s *Store) ActorsForPerson(ctx context.Context, q database.Querier, personID string) ([]referralmodels.Actor, error) {
	rows, err := q.Query(ctx, `
		SELECT actor_type, actor_id, person_id, registered_at, is_new_account, attribution_outcome, location_state
		FROM referral_actors WHERE person_id = $1 ORDER BY registered_at, created_at`, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var actors []referralmodels.Actor
	for rows.Next() {
		var a referralmodels.Actor
		if err := rows.Scan(&a.ActorType, &a.ActorID, &a.PersonID, &a.RegisteredAt, &a.IsNewAccount, &a.AttributionOutcome, &a.LocationState); err != nil {
			return nil, err
		}
		actors = append(actors, a)
	}
	return actors, rows.Err()
}

// RecordOnboarding stores that a driver account became fully approved,
// keeping the earliest time if it is reported more than once. It does not
// need the account's sign-up announcement to have arrived first.
func (s *Store) RecordOnboarding(ctx context.Context, q database.Querier, actorType, actorID string, at time.Time) error {
	_, err := q.Exec(ctx, `
		INSERT INTO actor_onboarding (actor_type, actor_id, completed_at) VALUES ($1, $2, $3)
		ON CONFLICT (actor_type, actor_id) DO UPDATE SET completed_at = LEAST(actor_onboarding.completed_at, EXCLUDED.completed_at)`,
		actorType, actorID, at)
	return err
}

// PersonOnboardedAt is when the first of a person's driver accounts became
// fully approved, or nil if none has.
func (s *Store) PersonOnboardedAt(ctx context.Context, q database.Querier, personID string) (*time.Time, error) {
	var at *time.Time
	err := q.QueryRow(ctx, `
		SELECT min(o.completed_at) FROM actor_onboarding o
		JOIN referral_actors a ON a.actor_type = o.actor_type AND a.actor_id = o.actor_id
		WHERE a.person_id = $1`, personID).Scan(&at)
	return at, err
}

// InsertCode stores a person's code, reporting false if the code is already
// taken (or the person already has one). It never raises a unique violation,
// which would abort the surrounding transaction and make a retry impossible.
func (s *Store) InsertCode(ctx context.Context, q database.Querier, code, personID string) (bool, error) {
	tag, err := q.Exec(ctx, `INSERT INTO referral_codes (code, person_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, code, personID)
	if err != nil {
		return false, fmt.Errorf("insert code: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// CodeForPerson returns a person's code.
func (s *Store) CodeForPerson(ctx context.Context, q database.Querier, personID string) (string, error) {
	var code string
	err := q.QueryRow(ctx, `SELECT code FROM referral_codes WHERE person_id = $1`, personID).Scan(&code)
	return code, notFound(err)
}

// PersonIDByCode resolves a code to its owner.
func (s *Store) PersonIDByCode(ctx context.Context, q database.Querier, code string) (string, error) {
	var personID string
	err := q.QueryRow(ctx, `SELECT person_id FROM referral_codes WHERE code = $1`, code).Scan(&personID)
	return personID, notFound(err)
}

// CreateReferral inserts a referral relationship.
func (s *Store) CreateReferral(ctx context.Context, q database.Querier, r referralmodels.Referral) (referralmodels.Referral, error) {
	if r.ID == "" {
		r.ID = "ref_" + uuid.NewString()
	}
	if r.Status == "" {
		r.Status = referralmodels.ReferralActive
	}
	_, err := q.Exec(ctx, `
		INSERT INTO referrals (id, referrer_person_id, referred_person_id, code, code_source, attributed_at, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		r.ID, r.ReferrerPersonID, r.ReferredPersonID, r.Code, r.CodeSource, r.AttributedAt, r.Status)
	if err != nil {
		return referralmodels.Referral{}, fmt.Errorf("create referral: %w", err)
	}
	return r, nil
}

const referralColumns = `id, referrer_person_id, referred_person_id, code, code_source, attributed_at, status, status_reason`

func scanReferral(row pgx.Row) (referralmodels.Referral, error) {
	var r referralmodels.Referral
	err := row.Scan(&r.ID, &r.ReferrerPersonID, &r.ReferredPersonID, &r.Code, &r.CodeSource, &r.AttributedAt, &r.Status, &r.StatusReason)
	return r, notFound(err)
}

// ReferralForReferred finds the referral a person joined through, if any.
func (s *Store) ReferralForReferred(ctx context.Context, q database.Querier, referredPersonID string, lock bool) (referralmodels.Referral, error) {
	query := `SELECT ` + referralColumns + ` FROM referrals WHERE referred_person_id = $1`
	if lock {
		query += ` FOR UPDATE`
	}
	return scanReferral(q.QueryRow(ctx, query, referredPersonID))
}

// GetReferral loads one referral.
func (s *Store) GetReferral(ctx context.Context, q database.Querier, id string, lock bool) (referralmodels.Referral, error) {
	query := `SELECT ` + referralColumns + ` FROM referrals WHERE id = $1`
	if lock {
		query += ` FOR UPDATE`
	}
	return scanReferral(q.QueryRow(ctx, query, id))
}

// SetReferralStatus changes a referral's standing.
func (s *Store) SetReferralStatus(ctx context.Context, q database.Querier, id, status, reason string) error {
	tag, err := q.Exec(ctx, `UPDATE referrals SET status = $2, status_reason = $3, updated_at = now() WHERE id = $1`, id, status, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// EnsureTrack creates a referral's track for a model if it does not exist
// yet, reporting whether it was created.
func (s *Store) EnsureTrack(ctx context.Context, q database.Querier, referralID, model string, jobsRequired int, onboardingCompletedAt *time.Time) (bool, error) {
	tag, err := q.Exec(ctx, `
		INSERT INTO referral_tracks (referral_id, model, jobs_required, onboarding_completed_at)
		VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, referralID, model, jobsRequired, onboardingCompletedAt)
	if err != nil {
		return false, fmt.Errorf("ensure track: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

const trackColumns = `referral_id, model, status, jobs_required, jobs_counted, onboarding_completed_at, qualified_at,
	qualifying_source_service, qualifying_source_reference, window_starts_at, window_ends_at`

func scanTrack(row pgx.Row) (referralmodels.Track, error) {
	var t referralmodels.Track
	err := row.Scan(&t.ReferralID, &t.Model, &t.Status, &t.JobsRequired, &t.JobsCounted, &t.OnboardingCompletedAt, &t.QualifiedAt,
		&t.QualifyingSourceService, &t.QualifyingSourceReference, &t.WindowStartsAt, &t.WindowEndsAt)
	return t, notFound(err)
}

// GetTrack loads (and optionally locks) one track.
func (s *Store) GetTrack(ctx context.Context, q database.Querier, referralID, model string, lock bool) (referralmodels.Track, error) {
	query := `SELECT ` + trackColumns + ` FROM referral_tracks WHERE referral_id = $1 AND model = $2`
	if lock {
		query += ` FOR UPDATE`
	}
	return scanTrack(q.QueryRow(ctx, query, referralID, model))
}

// UpdateTrack writes a track's mutable progress.
func (s *Store) UpdateTrack(ctx context.Context, q database.Querier, t referralmodels.Track) error {
	_, err := q.Exec(ctx, `
		UPDATE referral_tracks SET status = $3, jobs_counted = $4, onboarding_completed_at = $5, qualified_at = $6,
			qualifying_source_service = $7, qualifying_source_reference = $8, window_starts_at = $9, window_ends_at = $10,
			updated_at = now()
		WHERE referral_id = $1 AND model = $2`,
		t.ReferralID, t.Model, t.Status, t.JobsCounted, t.OnboardingCompletedAt, t.QualifiedAt,
		t.QualifyingSourceService, t.QualifyingSourceReference, t.WindowStartsAt, t.WindowEndsAt)
	return err
}

// RaiseFlag records something for a human to review.
func (s *Store) RaiseFlag(ctx context.Context, q database.Querier, personID, referralID, flagType, severity string, evidence map[string]any) error {
	raw, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		INSERT INTO fraud_flags (id, person_id, referral_id, flag_type, severity, evidence)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4, $5, $6)`,
		"ff_"+uuid.NewString(), personID, referralID, flagType, severity, raw)
	if err != nil {
		return fmt.Errorf("raise flag: %w", err)
	}
	return nil
}
