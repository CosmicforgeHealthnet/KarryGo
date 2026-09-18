package bikecapability

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a bike or eligibility row does not exist.
var ErrNotFound = errors.New("bikecapability: not found")

// ErrVersionConflict is returned when a conditional write loses a race.
var ErrVersionConflict = errors.New("bikecapability: capability version conflict")

// Repository is the Postgres store.
type Repository struct{ db *pgxpool.Pool }

// NewRepository builds the store.
func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// capabilitySelect is the single read every capability lookup uses.
//
// The LEFT JOIN is deliberate and load-bearing: a bike that has never been
// surveyed must still come back, as a row whose capability columns are all
// NULL and whose `surveyed` flag is false. An INNER JOIN would make an
// unsurveyed bike indistinguishable from a deleted one, and the fit engine
// would then report a missing record as a dependency problem.
const capabilitySelect = `
SELECT b.id, b.provider_id, b.bike_type, b.is_active,
       (b.verification_status = 'verified') AS verified,
       c.bike_id IS NOT NULL AS surveyed,
       c.max_payload_grams,
       c.box_length_mm, c.box_width_mm, c.box_height_mm,
       c.enclosed_box, c.weather_protected, c.upright_safe, c.fragile_safe,
       c.thermal_bag, COALESCE(c.temperature_class, ''),
       COALESCE(c.capability_version, 0), COALESCE(c.source_version, 0),
       c.observed_at
  FROM bikes b
  LEFT JOIN dispatch_bike_capabilities c ON c.bike_id = b.id`

func scanCapability(row pgx.Row) (Record, error) {
	var record Record
	err := row.Scan(
		&record.BikeID, &record.ProviderID, &record.BikeType, &record.Active,
		&record.Verified, &record.Surveyed,
		&record.MaxPayloadGrams,
		&record.BoxLengthMm, &record.BoxWidthMm, &record.BoxHeightMm,
		&record.EnclosedBox, &record.WeatherProtected, &record.UprightSafe, &record.FragileSafe,
		&record.ThermalBag, &record.TemperatureClass,
		&record.CapabilityVersion, &record.SourceVersion, &record.ObservedAt,
	)
	return record, err
}

// GetByBike loads one bike's capability.
func (r *Repository) GetByBike(ctx context.Context, bikeID string) (Record, error) {
	if r == nil || r.db == nil {
		return Record{}, ErrNotFound
	}
	record, err := scanCapability(r.db.QueryRow(ctx, capabilitySelect+` WHERE b.id = $1`, bikeID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	return record, err
}

// LoadProviderBikes batch-loads the active bikes of a candidate set.
//
// It is a batch rather than a loop because assignment evaluates every nearby
// provider on the hot path: a per-candidate query turns one match into fifty
// round trips, and the resulting timeout would look like "no riders
// available" rather than "our matcher is slow".
func (r *Repository) LoadProviderBikes(ctx context.Context, providerIDs []string) (map[string][]Record, error) {
	out := make(map[string][]Record, len(providerIDs))
	if r == nil || r.db == nil || len(providerIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx,
		capabilitySelect+` WHERE b.provider_id = ANY($1::uuid[]) AND b.is_active ORDER BY b.is_primary DESC, b.created_at`,
		providerIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		record, err := scanCapability(rows)
		if err != nil {
			return nil, err
		}
		out[record.ProviderID] = append(out[record.ProviderID], record)
	}
	return out, rows.Err()
}

// UpsertCapability applies a partial capability edit.
//
// The INSERT ... ON CONFLICT shape is what makes a first survey and a later
// edit the same call: a bike with no capability row yet gets one, and an
// existing row keeps every field the caller did not mention. Doing it in one
// statement rather than read-modify-write keeps it atomic against a concurrent
// edit, and the version guard turns a lost race into a refusal rather than a
// silent overwrite.
func (r *Repository) UpsertCapability(ctx context.Context, input UpdateInput, now time.Time) (Record, error) {
	if r == nil || r.db == nil {
		return Record{}, ErrNotFound
	}

	// Ownership first. A bike that is not this provider's must look absent
	// rather than forbidden: saying "that exists but is not yours" leaks the
	// existence of another provider's asset.
	var ownerID string
	err := r.db.QueryRow(ctx, `SELECT provider_id FROM bikes WHERE id = $1`, input.BikeID).Scan(&ownerID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && ownerID != input.ProviderID) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, err
	}

	const statement = `
INSERT INTO dispatch_bike_capabilities AS c
	(bike_id, provider_id, max_payload_grams, box_length_mm, box_width_mm, box_height_mm,
	 enclosed_box, weather_protected, upright_safe, fragile_safe,
	 thermal_bag, temperature_class, capability_version, observed_at)
VALUES ($1, $2,
	CASE WHEN $3  THEN $4::BIGINT  ELSE NULL END,
	CASE WHEN $5  THEN $6::BIGINT  ELSE NULL END,
	CASE WHEN $7  THEN $8::BIGINT  ELSE NULL END,
	CASE WHEN $9  THEN $10::BIGINT ELSE NULL END,
	CASE WHEN $11 THEN $12::BOOLEAN ELSE NULL END,
	CASE WHEN $13 THEN $14::BOOLEAN ELSE NULL END,
	CASE WHEN $15 THEN $16::BOOLEAN ELSE NULL END,
	CASE WHEN $17 THEN $18::BOOLEAN ELSE NULL END,
	CASE WHEN $19 THEN $20::BOOLEAN ELSE NULL END,
	COALESCE($21::TEXT, ''), 1, $22)
ON CONFLICT (bike_id) DO UPDATE SET
	max_payload_grams = CASE WHEN $3  THEN $4::BIGINT   ELSE c.max_payload_grams END,
	box_length_mm     = CASE WHEN $5  THEN $6::BIGINT   ELSE c.box_length_mm     END,
	box_width_mm      = CASE WHEN $7  THEN $8::BIGINT   ELSE c.box_width_mm      END,
	box_height_mm     = CASE WHEN $9  THEN $10::BIGINT  ELSE c.box_height_mm     END,
	enclosed_box      = CASE WHEN $11 THEN $12::BOOLEAN ELSE c.enclosed_box      END,
	weather_protected = CASE WHEN $13 THEN $14::BOOLEAN ELSE c.weather_protected END,
	upright_safe      = CASE WHEN $15 THEN $16::BOOLEAN ELSE c.upright_safe      END,
	fragile_safe      = CASE WHEN $17 THEN $18::BOOLEAN ELSE c.fragile_safe      END,
	thermal_bag       = CASE WHEN $19 THEN $20::BOOLEAN ELSE c.thermal_bag       END,
	temperature_class = COALESCE($21::TEXT, c.temperature_class),
	capability_version = c.capability_version + 1,
	observed_at = $22,
	updated_at = now()
WHERE $23 = 0 OR c.capability_version = $23
RETURNING c.capability_version`

	var version int64
	err = r.db.QueryRow(ctx, statement,
		input.BikeID, input.ProviderID,
		input.MaxPayloadGrams.Set, input.MaxPayloadGrams.Value,
		input.BoxLengthMm.Set, input.BoxLengthMm.Value,
		input.BoxWidthMm.Set, input.BoxWidthMm.Value,
		input.BoxHeightMm.Set, input.BoxHeightMm.Value,
		input.EnclosedBox.Set, input.EnclosedBox.Value,
		input.WeatherProtected.Set, input.WeatherProtected.Value,
		input.UprightSafe.Set, input.UprightSafe.Value,
		input.FragileSafe.Set, input.FragileSafe.Value,
		input.ThermalBag.Set, input.ThermalBag.Value,
		input.TemperatureClass,
		now.UTC(),
		input.ExpectedVersion,
	).Scan(&version)

	if errors.Is(err, pgx.ErrNoRows) {
		// The conflict target matched but the version guard refused. The bike
		// is known to exist and to be this provider's (checked above), so this
		// can only be a concurrent edit.
		return Record{}, ErrVersionConflict
	}
	if err != nil {
		return Record{}, err
	}
	return r.GetByBike(ctx, input.BikeID)
}

// ── Product eligibility ──────────────────────────────────────────────────────

const eligibilityColumns = `bike_id, provider_id, product_code, status, policy_version,
	certification_ref, effective_at, expires_at, reason_code, source_version, updated_at`

func scanEligibility(row pgx.Row) (ProductEligibility, error) {
	var eligibility ProductEligibility
	var status string
	err := row.Scan(&eligibility.BikeID, &eligibility.ProviderID, &eligibility.ProductCode,
		&status, &eligibility.PolicyVersion, &eligibility.CertificationRef,
		&eligibility.EffectiveAt, &eligibility.ExpiresAt, &eligibility.ReasonCode,
		&eligibility.SourceVersion, &eligibility.UpdatedAt)
	eligibility.Status = EligibilityStatus(status)
	return eligibility, err
}

// ListEligibility returns every product admission for a bike.
func (r *Repository) ListEligibility(ctx context.Context, bikeID string) ([]ProductEligibility, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+eligibilityColumns+` FROM dispatch_vehicle_product_eligibility
		 WHERE bike_id = $1 ORDER BY product_code`, bikeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProductEligibility
	for rows.Next() {
		eligibility, err := scanEligibility(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, eligibility)
	}
	return out, rows.Err()
}

// LoadEligibilityForProduct batch-loads one product's admissions for a set of
// bikes, for the matcher.
func (r *Repository) LoadEligibilityForProduct(ctx context.Context, bikeIDs []string, productCode string) (map[string]ProductEligibility, error) {
	out := make(map[string]ProductEligibility, len(bikeIDs))
	if r == nil || r.db == nil || len(bikeIDs) == 0 || strings.TrimSpace(productCode) == "" {
		return out, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+eligibilityColumns+` FROM dispatch_vehicle_product_eligibility
		 WHERE bike_id = ANY($1::uuid[]) AND product_code = $2`, bikeIDs, productCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		eligibility, err := scanEligibility(rows)
		if err != nil {
			return nil, err
		}
		out[eligibility.BikeID] = eligibility
	}
	return out, rows.Err()
}

// UpsertEligibility applies a certification decision.
//
// The source_version guard makes the endpoint replay-safe: a callback that
// arrives late, or twice, carrying a version no newer than the stored one
// changes nothing. Without it a delayed "approved" could resurrect a
// certification revoked after it was sent.
func (r *Repository) UpsertEligibility(ctx context.Context, eligibility ProductEligibility) (ProductEligibility, bool, error) {
	if r == nil || r.db == nil {
		return ProductEligibility{}, false, ErrNotFound
	}
	const statement = `
INSERT INTO dispatch_vehicle_product_eligibility AS e
	(bike_id, provider_id, product_code, status, policy_version, certification_ref,
	 effective_at, expires_at, reason_code, source_version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (bike_id, product_code) DO UPDATE SET
	status = EXCLUDED.status,
	policy_version = EXCLUDED.policy_version,
	certification_ref = EXCLUDED.certification_ref,
	effective_at = EXCLUDED.effective_at,
	expires_at = EXCLUDED.expires_at,
	reason_code = EXCLUDED.reason_code,
	source_version = EXCLUDED.source_version,
	updated_at = now()
WHERE e.source_version < EXCLUDED.source_version
RETURNING ` + eligibilityColumns

	stored, err := scanEligibility(r.db.QueryRow(ctx, statement,
		eligibility.BikeID, eligibility.ProviderID, eligibility.ProductCode,
		string(eligibility.Status), eligibility.PolicyVersion, eligibility.CertificationRef,
		eligibility.EffectiveAt, eligibility.ExpiresAt, eligibility.ReasonCode,
		eligibility.SourceVersion))

	if errors.Is(err, pgx.ErrNoRows) {
		// The guard refused: the stored row is at least as new. Return what is
		// stored so the caller sees a no-op rather than a failure.
		existing, lookupErr := r.getEligibility(ctx, eligibility.BikeID, eligibility.ProductCode)
		if lookupErr != nil {
			return ProductEligibility{}, false, lookupErr
		}
		return existing, false, nil
	}
	if err != nil {
		return ProductEligibility{}, false, err
	}
	return stored, true, nil
}

func (r *Repository) getEligibility(ctx context.Context, bikeID, productCode string) (ProductEligibility, error) {
	eligibility, err := scanEligibility(r.db.QueryRow(ctx,
		`SELECT `+eligibilityColumns+` FROM dispatch_vehicle_product_eligibility
		 WHERE bike_id = $1 AND product_code = $2`, bikeID, productCode))
	if errors.Is(err, pgx.ErrNoRows) {
		return ProductEligibility{}, ErrNotFound
	}
	return eligibility, err
}

// ExpireLapsedEligibility flips ELIGIBLE rows whose window has closed.
//
// The fit check also evaluates the window at read time, so this is not what
// stops a lapsed certification dispatching -- it is what makes the stored
// status honest, so an operator reading the table sees the truth rather than a
// row claiming ELIGIBLE and behaving otherwise.
func (r *Repository) ExpireLapsedEligibility(ctx context.Context, now time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	tag, err := r.db.Exec(ctx, `
UPDATE dispatch_vehicle_product_eligibility
   SET status = 'EXPIRED', reason_code = 'CERTIFICATION_EXPIRED', updated_at = now()
 WHERE status = 'ELIGIBLE' AND expires_at IS NOT NULL AND expires_at <= $1`, now.UTC())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ── Match decision evidence ──────────────────────────────────────────────────

// RecordDecisions writes the hard-fit outcome for a candidate set.
//
// Failures are the caller's to swallow: evidence is valuable, but an
// assignment must never fail because an operations table was unavailable.
func (r *Repository) RecordDecisions(ctx context.Context, decisions []MatchDecision) error {
	if r == nil || r.db == nil || len(decisions) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, decision := range decisions {
		evidence, err := json.Marshal(boundedEvidence(decision.Evidence))
		if err != nil {
			evidence = []byte("{}")
		}
		var bikeID any
		if strings.TrimSpace(decision.BikeID) != "" {
			bikeID = decision.BikeID
		}
		batch.Queue(`
INSERT INTO dispatch_match_decisions
	(mission_id, booking_id, provider_id, bike_id, decision, reason_codes,
	 evidence, distance_m, capability_version, policy_version, shadow)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			decision.MissionID, decision.BookingID, decision.ProviderID, bikeID,
			decision.Decision, decision.ReasonCodes, evidence, decision.DistanceM,
			decision.CapabilityVersion, decision.PolicyVersion, decision.Shadow)
	}
	results := r.db.SendBatch(ctx, batch)
	defer results.Close()
	for range decisions {
		if _, err := results.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// maxEvidenceEntries and maxEvidenceValueLength bound what one decision
// records. Evidence is for an operator reading a handful of rows, not a data
// lake; an unbounded map would let a pathological requirement write kilobytes
// per candidate per assignment.
const (
	maxEvidenceEntries     = 12
	maxEvidenceValueLength = 200
)

func boundedEvidence(evidence map[string]string) map[string]string {
	if len(evidence) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(evidence))
	for key, value := range evidence {
		if len(out) >= maxEvidenceEntries {
			break
		}
		if len(value) > maxEvidenceValueLength {
			value = value[:maxEvidenceValueLength]
		}
		out[key] = value
	}
	return out
}

// PurgeDecisionsBefore enforces retention.
func (r *Repository) PurgeDecisionsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	tag, err := r.db.Exec(ctx, `DELETE FROM dispatch_match_decisions WHERE created_at < $1`, cutoff.UTC())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ── Policy snapshots ─────────────────────────────────────────────────────────

// SavePolicySnapshot stores an immutable policy document.
//
// A repeat of a (kind, version) already stored is a no-op, not an error:
// pulling the same policy twice is normal and a snapshot is by definition the
// same document every time. What it must never do is overwrite, because
// committed missions pin a version.
func (r *Repository) SavePolicySnapshot(ctx context.Context, snapshot PolicySnapshot) error {
	if r == nil || r.db == nil {
		return nil
	}
	payload := snapshot.Payload
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	_, err := r.db.Exec(ctx, `
INSERT INTO dispatch_policy_snapshots (policy_kind, policy_version, payload, checksum, effective_at)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (policy_kind, policy_version) DO NOTHING`,
		snapshot.PolicyKind, snapshot.PolicyVersion, payload, snapshot.Checksum,
		snapshot.EffectiveAt.UTC())
	return err
}

// LatestPolicyVersion returns the newest effective snapshot version for a kind.
//
// It falls back rather than erroring because assignment must not stop for a
// policy that has never been pulled. A deployment with no admin connection
// still matches, against the built-in default, and records that version in its
// evidence so the gap is visible afterwards.
func (r *Repository) LatestPolicyVersion(ctx context.Context, kind, fallback string) string {
	if r == nil || r.db == nil {
		return fallback
	}
	var version string
	err := r.db.QueryRow(ctx, `
SELECT policy_version FROM dispatch_policy_snapshots
 WHERE policy_kind = $1 AND effective_at <= now()
 ORDER BY effective_at DESC LIMIT 1`, kind).Scan(&version)
	if err != nil || strings.TrimSpace(version) == "" {
		return fallback
	}
	return version
}

// PolicyLagSeconds reports how old the newest snapshot of a kind is.
//
// A growing lag is the signal that a deployment has quietly stopped receiving
// policy, which is otherwise invisible precisely because assignment keeps
// working against the last snapshot.
func (r *Repository) PolicyLagSeconds(ctx context.Context, kind string, now time.Time) (float64, bool) {
	if r == nil || r.db == nil {
		return 0, false
	}
	var appliedAt time.Time
	if err := r.db.QueryRow(ctx, `
SELECT applied_at FROM dispatch_policy_snapshots
 WHERE policy_kind = $1 ORDER BY applied_at DESC LIMIT 1`, kind).Scan(&appliedAt); err != nil {
		return 0, false
	}
	return now.UTC().Sub(appliedAt).Seconds(), true
}

// FleetCompleteness reports how much of the active fleet has been surveyed.
//
// This is the number that decides whether fail-closed enforcement is safe, so
// it counts a fact the fit engine actually refuses on rather than a proxy like
// "has a capability row at all".
func (r *Repository) FleetCompleteness(ctx context.Context) (surveyed, total int64, err error) {
	if r == nil || r.db == nil {
		return 0, 0, nil
	}
	err = r.db.QueryRow(ctx, `
SELECT
	COUNT(c.bike_id) FILTER (WHERE c.max_payload_grams IS NOT NULL),
	COUNT(b.id)
FROM bikes b
LEFT JOIN dispatch_bike_capabilities c ON c.bike_id = b.id
WHERE b.is_active`).Scan(&surveyed, &total)
	return surveyed, total, err
}
