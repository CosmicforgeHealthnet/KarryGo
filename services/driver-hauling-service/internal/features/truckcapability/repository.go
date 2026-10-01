package truckcapability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a truck or eligibility row does not exist.
var ErrNotFound = errors.New("truckcapability: not found")

// ErrVersionConflict is returned when a conditional write loses a race.
var ErrVersionConflict = errors.New("truckcapability: capability version conflict")

// Repository is the Postgres store.
type Repository struct{ db *pgxpool.Pool }

// NewRepository builds the store.
func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// capabilityColumns is the single column list every capability read uses.
//
// It is one constant rather than four inlined copies because this service has
// already been bitten by that: migration 005's session notes record four
// hand-maintained provider column lists drifting apart. A shared constant
// makes adding a column a one-line change instead of a four-line one with
// three chances to forget.
const capabilityColumns = `
	t.id, t.provider_id, t.truck_type, t.status, p.status,
	t.max_payload_grams,
	t.internal_length_mm, t.internal_width_mm, t.internal_height_mm, t.usable_volume_ml,
	t.opening_width_mm, t.opening_height_mm,
	t.enclosed, t.weather_protected, t.upright_safe, t.fragile_safe,
	t.refrigerated, t.temperature_min_c, t.temperature_max_c, t.temperature_class,
	t.tail_lift, t.crane, t.hydraulic, t.loading_ramp,
	t.axle_count, t.trailer_capable, t.supported_container_sizes,
	t.supported_goods_classes, t.prohibited_goods_classes,
	t.insurance_valid, t.inspection_ref, t.certification_ref,
	t.capability_version, t.capability_source_version, t.capability_observed_at`

const capabilityFrom = ` FROM trucks t JOIN truck_providers p ON p.id = t.provider_id`

func scanCapability(row pgx.Row) (Record, error) {
	var record Record
	var truckStatus, providerStatus string
	var temperatureMin, temperatureMax *int16
	var axleCount *int32

	if err := row.Scan(
		&record.TruckID, &record.ProviderID, &record.TruckType, &truckStatus, &providerStatus,
		&record.MaxPayloadGrams,
		&record.InternalLengthMm, &record.InternalWidthMm, &record.InternalHeightMm, &record.UsableVolumeMl,
		&record.OpeningWidthMm, &record.OpeningHeightMm,
		&record.Enclosed, &record.WeatherProtected, &record.UprightSafe, &record.FragileSafe,
		&record.Refrigerated, &temperatureMin, &temperatureMax, &record.TemperatureClass,
		&record.TailLift, &record.Crane, &record.Hydraulic, &record.LoadingRamp,
		&axleCount, &record.TrailerCapable, &record.SupportedContainerSizes,
		&record.SupportedGoodsClasses, &record.ProhibitedGoodsClasses,
		&record.InsuranceValid, &record.InspectionRef, &record.CertificationRef,
		&record.CapabilityVersion, &record.CapabilitySourceVersion, &record.ObservedAt,
	); err != nil {
		return Record{}, err
	}

	record.Active = strings.EqualFold(truckStatus, "active")
	// A truck belongs to its provider: a suspended provider's truck cannot
	// work, however well surveyed it is. Folding the provider gate in here
	// means one Evaluate call answers the whole question and no caller can
	// forget the second check.
	record.Verified = strings.EqualFold(providerStatus, "active")

	if temperatureMin != nil {
		value := int(*temperatureMin)
		record.TemperatureMinC = &value
	}
	if temperatureMax != nil {
		value := int(*temperatureMax)
		record.TemperatureMaxC = &value
	}
	if axleCount != nil {
		value := int(*axleCount)
		record.AxleCount = &value
	}
	return record, nil
}

// GetByTruck loads one truck's capability.
func (r *Repository) GetByTruck(ctx context.Context, truckID string) (Record, error) {
	if r == nil || r.db == nil {
		return Record{}, ErrNotFound
	}
	row := r.db.QueryRow(ctx, `SELECT`+capabilityColumns+capabilityFrom+` WHERE t.id = $1`, truckID)
	record, err := scanCapability(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	return record, err
}

// LoadForMatching batch-loads capabilities for a candidate set.
//
// It is a batch rather than a loop because matching evaluates every nearby
// provider on the hot path: a per-candidate query turns one match into fifty
// round trips, and the timeout that follows would look like "no trucks
// available" rather than "our matcher is slow".
func (r *Repository) LoadForMatching(ctx context.Context, truckIDs []string) (map[string]Record, error) {
	out := make(map[string]Record, len(truckIDs))
	if r == nil || r.db == nil || len(truckIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT`+capabilityColumns+capabilityFrom+` WHERE t.id = ANY($1::uuid[])`, truckIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		record, err := scanCapability(rows)
		if err != nil {
			return nil, err
		}
		out[record.TruckID] = record
	}
	return out, rows.Err()
}

// LoadProviderTrucksForMatching loads the capability of every active truck
// belonging to each provider in the candidate set.
//
// Matching starts from providers, not trucks, and a provider's availability
// record may name no truck at all. Loading by provider is what lets the
// matcher ask "does this provider have ANY truck that fits" instead of
// silently skipping a provider whose availability row happened to be blank --
// which the old code did, by falling through to proximity-only.
func (r *Repository) LoadProviderTrucksForMatching(ctx context.Context, providerIDs []string) (map[string][]Record, error) {
	out := make(map[string][]Record, len(providerIDs))
	if r == nil || r.db == nil || len(providerIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT`+capabilityColumns+capabilityFrom+
			` WHERE t.provider_id = ANY($1::uuid[]) AND t.status = 'active' ORDER BY t.created_at`,
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

// UpdateCapability applies a partial capability edit.
//
// The SQL uses COALESCE-style conditional assignment driven by a "set" flag
// per field, so an absent field keeps its value while an explicitly cleared
// one becomes NULL. Doing it in one statement rather than read-modify-write
// keeps the update atomic against a concurrent edit; the version guard then
// turns a lost race into a refusal rather than a silent overwrite.
func (r *Repository) UpdateCapability(ctx context.Context, input UpdateInput, now time.Time) (Record, error) {
	if r == nil || r.db == nil {
		return Record{}, ErrNotFound
	}

	const statement = `
UPDATE trucks SET
	max_payload_grams   = CASE WHEN $3  THEN $4::BIGINT   ELSE max_payload_grams   END,
	internal_length_mm  = CASE WHEN $5  THEN $6::BIGINT   ELSE internal_length_mm  END,
	internal_width_mm   = CASE WHEN $7  THEN $8::BIGINT   ELSE internal_width_mm   END,
	internal_height_mm  = CASE WHEN $9  THEN $10::BIGINT  ELSE internal_height_mm  END,
	usable_volume_ml    = CASE WHEN $11 THEN $12::BIGINT  ELSE usable_volume_ml    END,
	opening_width_mm    = CASE WHEN $13 THEN $14::BIGINT  ELSE opening_width_mm    END,
	opening_height_mm   = CASE WHEN $15 THEN $16::BIGINT  ELSE opening_height_mm   END,
	enclosed            = CASE WHEN $17 THEN $18::BOOLEAN ELSE enclosed            END,
	weather_protected   = CASE WHEN $19 THEN $20::BOOLEAN ELSE weather_protected   END,
	upright_safe        = CASE WHEN $21 THEN $22::BOOLEAN ELSE upright_safe        END,
	fragile_safe        = CASE WHEN $23 THEN $24::BOOLEAN ELSE fragile_safe        END,
	refrigerated        = CASE WHEN $25 THEN $26::BOOLEAN ELSE refrigerated        END,
	temperature_min_c   = CASE WHEN $27 THEN $28::SMALLINT ELSE temperature_min_c  END,
	temperature_max_c   = CASE WHEN $29 THEN $30::SMALLINT ELSE temperature_max_c  END,
	temperature_class   = COALESCE($31::TEXT, temperature_class),
	tail_lift           = CASE WHEN $32 THEN $33::BOOLEAN ELSE tail_lift           END,
	crane               = CASE WHEN $34 THEN $35::BOOLEAN ELSE crane               END,
	hydraulic           = CASE WHEN $36 THEN $37::BOOLEAN ELSE hydraulic           END,
	loading_ramp        = CASE WHEN $38 THEN $39::BOOLEAN ELSE loading_ramp        END,
	axle_count          = CASE WHEN $40 THEN $41::INT     ELSE axle_count          END,
	trailer_capable     = CASE WHEN $42 THEN $43::BOOLEAN ELSE trailer_capable     END,
	supported_container_sizes = COALESCE($44::TEXT[], supported_container_sizes),
	supported_goods_classes   = COALESCE($45::TEXT[], supported_goods_classes),
	prohibited_goods_classes  = COALESCE($46::TEXT[], prohibited_goods_classes),
	inspection_ref      = COALESCE($47::TEXT, inspection_ref),
	capability_version  = capability_version + 1,
	capability_observed_at = $48,
	updated_at = now()
WHERE id = $1 AND provider_id = $2
  AND ($49 = 0 OR capability_version = $49)
RETURNING capability_version`

	var version int64
	err := r.db.QueryRow(ctx, statement,
		input.TruckID, input.ProviderID,
		input.MaxPayloadGrams.Set, input.MaxPayloadGrams.Value,
		input.InternalLengthMm.Set, input.InternalLengthMm.Value,
		input.InternalWidthMm.Set, input.InternalWidthMm.Value,
		input.InternalHeightMm.Set, input.InternalHeightMm.Value,
		input.UsableVolumeMl.Set, input.UsableVolumeMl.Value,
		input.OpeningWidthMm.Set, input.OpeningWidthMm.Value,
		input.OpeningHeightMm.Set, input.OpeningHeightMm.Value,
		input.Enclosed.Set, input.Enclosed.Value,
		input.WeatherProtected.Set, input.WeatherProtected.Value,
		input.UprightSafe.Set, input.UprightSafe.Value,
		input.FragileSafe.Set, input.FragileSafe.Value,
		input.Refrigerated.Set, input.Refrigerated.Value,
		input.TemperatureMinC.Set, smallint(input.TemperatureMinC.Value),
		input.TemperatureMaxC.Set, smallint(input.TemperatureMaxC.Value),
		input.TemperatureClass,
		input.TailLift.Set, input.TailLift.Value,
		input.Crane.Set, input.Crane.Value,
		input.Hydraulic.Set, input.Hydraulic.Value,
		input.LoadingRamp.Set, input.LoadingRamp.Value,
		input.AxleCount.Set, integer(input.AxleCount.Value),
		input.TrailerCapable.Set, input.TrailerCapable.Value,
		input.SupportedContainerSizes,
		input.SupportedGoodsClasses,
		input.ProhibitedGoodsClasses,
		input.InspectionRef,
		now.UTC(),
		input.ExpectedVersion,
	).Scan(&version)

	if errors.Is(err, pgx.ErrNoRows) {
		// No row matched. Either the truck is not this provider's, or the
		// version guard refused. Distinguishing them needs a second read,
		// which is worth it: "someone else edited this" and "that is not your
		// truck" call for completely different responses.
		if input.ExpectedVersion != 0 {
			if _, lookupErr := r.GetByTruck(ctx, input.TruckID); lookupErr == nil {
				return Record{}, ErrVersionConflict
			}
		}
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, err
	}
	return r.GetByTruck(ctx, input.TruckID)
}

func smallint(value *int) *int16 {
	if value == nil {
		return nil
	}
	converted := int16(*value)
	return &converted
}

func integer(value *int) *int32 {
	if value == nil {
		return nil
	}
	converted := int32(*value)
	return &converted
}

// ── Product eligibility ──────────────────────────────────────────────────────

const eligibilityColumns = `id, truck_id, provider_id, product_code, status, policy_version,
	certification_ref, effective_at, expires_at, reason_code, source_version, created_at, updated_at`

func scanEligibility(row pgx.Row) (ProductEligibility, error) {
	var eligibility ProductEligibility
	var status string
	err := row.Scan(&eligibility.ID, &eligibility.TruckID, &eligibility.ProviderID,
		&eligibility.ProductCode, &status, &eligibility.PolicyVersion,
		&eligibility.CertificationRef, &eligibility.EffectiveAt, &eligibility.ExpiresAt,
		&eligibility.ReasonCode, &eligibility.SourceVersion,
		&eligibility.CreatedAt, &eligibility.UpdatedAt)
	eligibility.Status = EligibilityStatus(status)
	return eligibility, err
}

// ListEligibility returns every product admission for a truck.
func (r *Repository) ListEligibility(ctx context.Context, truckID string) ([]ProductEligibility, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+eligibilityColumns+` FROM hauling_truck_product_eligibility
		 WHERE truck_id = $1 ORDER BY product_code`, truckID)
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
// trucks, for the matcher.
func (r *Repository) LoadEligibilityForProduct(ctx context.Context, truckIDs []string, productCode string) (map[string]ProductEligibility, error) {
	out := make(map[string]ProductEligibility, len(truckIDs))
	if r == nil || r.db == nil || len(truckIDs) == 0 || strings.TrimSpace(productCode) == "" {
		return out, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+eligibilityColumns+` FROM hauling_truck_product_eligibility
		 WHERE truck_id = ANY($1::uuid[]) AND product_code = $2`, truckIDs, productCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		eligibility, err := scanEligibility(rows)
		if err != nil {
			return nil, err
		}
		out[eligibility.TruckID] = eligibility
	}
	return out, rows.Err()
}

// UpsertEligibility applies a certification decision.
//
// The source_version guard is what makes the endpoint replay-safe: a callback
// that arrives late, or twice, carrying a version no newer than the stored one
// changes nothing. Without it a delayed "approved" message could resurrect a
// certification that was revoked after it was sent.
func (r *Repository) UpsertEligibility(ctx context.Context, eligibility ProductEligibility) (ProductEligibility, bool, error) {
	if r == nil || r.db == nil {
		return ProductEligibility{}, false, ErrNotFound
	}
	const statement = `
INSERT INTO hauling_truck_product_eligibility
	(truck_id, provider_id, product_code, status, policy_version, certification_ref,
	 effective_at, expires_at, reason_code, source_version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (truck_id, product_code) DO UPDATE SET
	status = EXCLUDED.status,
	policy_version = EXCLUDED.policy_version,
	certification_ref = EXCLUDED.certification_ref,
	effective_at = EXCLUDED.effective_at,
	expires_at = EXCLUDED.expires_at,
	reason_code = EXCLUDED.reason_code,
	source_version = EXCLUDED.source_version,
	updated_at = now()
WHERE hauling_truck_product_eligibility.source_version < EXCLUDED.source_version
RETURNING ` + eligibilityColumns

	row := r.db.QueryRow(ctx, statement,
		eligibility.TruckID, eligibility.ProviderID, eligibility.ProductCode,
		string(eligibility.Status), eligibility.PolicyVersion, eligibility.CertificationRef,
		eligibility.EffectiveAt, eligibility.ExpiresAt, eligibility.ReasonCode,
		eligibility.SourceVersion)

	stored, err := scanEligibility(row)
	if errors.Is(err, pgx.ErrNoRows) {
		// The guard refused: the stored row is at least as new. Return what is
		// stored so the caller can see it was a no-op rather than a failure.
		existing, lookupErr := r.getEligibility(ctx, eligibility.TruckID, eligibility.ProductCode)
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

func (r *Repository) getEligibility(ctx context.Context, truckID, productCode string) (ProductEligibility, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+eligibilityColumns+` FROM hauling_truck_product_eligibility
		 WHERE truck_id = $1 AND product_code = $2`, truckID, productCode)
	eligibility, err := scanEligibility(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProductEligibility{}, ErrNotFound
	}
	return eligibility, err
}

// ExpireLapsedEligibility flips ELIGIBLE rows whose window has closed.
//
// The fit check also evaluates the window at read time, so this sweeper is not
// what keeps a lapsed certification from dispatching -- it is what makes the
// stored status honest, so an operator reading the table sees the truth
// instead of a row that claims ELIGIBLE and behaves otherwise.
func (r *Repository) ExpireLapsedEligibility(ctx context.Context, now time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	tag, err := r.db.Exec(ctx, `
UPDATE hauling_truck_product_eligibility
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
// Failures here are the caller's to swallow: evidence is valuable, but a
// booking must never fail to match because an operations table was
// unavailable. The caller logs and continues.
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
		var truckID any
		if strings.TrimSpace(decision.TruckID) != "" {
			truckID = decision.TruckID
		}
		batch.Queue(`
INSERT INTO hauling_match_decisions
	(booking_id, mission_ref, provider_id, truck_id, decision, reason_codes,
	 evidence, distance_m, capability_version, policy_version, shadow)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			decision.BookingID, decision.MissionRef, decision.ProviderID, truckID,
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

// maxEvidenceEntries bounds what one decision records. Evidence is for an
// operator reading a handful of rows, not a data lake; an unbounded map would
// let a pathological requirement write kilobytes per candidate per booking.
const maxEvidenceEntries = 12

// maxEvidenceValueLength bounds each value for the same reason.
const maxEvidenceValueLength = 200

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
	tag, err := r.db.Exec(ctx, `DELETE FROM hauling_match_decisions WHERE created_at < $1`, cutoff.UTC())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ── Policy snapshots ─────────────────────────────────────────────────────────

// SavePolicySnapshot stores an immutable policy document.
//
// A repeat of a (kind, version) already stored is a no-op rather than an
// error: pulling the same policy twice is normal, and a snapshot is by
// definition the same document every time. What it must never do is overwrite,
// because committed work pins a version.
func (r *Repository) SavePolicySnapshot(ctx context.Context, snapshot PolicySnapshot) error {
	if r == nil || r.db == nil {
		return nil
	}
	payload := snapshot.Payload
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	_, err := r.db.Exec(ctx, `
INSERT INTO hauling_policy_snapshots (policy_kind, policy_version, payload, checksum, effective_at)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (policy_kind, policy_version) DO NOTHING`,
		snapshot.PolicyKind, snapshot.PolicyVersion, payload, snapshot.Checksum,
		snapshot.EffectiveAt.UTC())
	return err
}

// LatestPolicyVersion returns the newest effective snapshot version for a kind.
//
// It returns the fallback rather than an error when nothing is stored, because
// matching must not stop for a policy that has never been pulled. A deployment
// with no admin connection still matches, against the built-in default, and
// records that version in its evidence so the gap is visible afterwards.
func (r *Repository) LatestPolicyVersion(ctx context.Context, kind, fallback string) string {
	if r == nil || r.db == nil {
		return fallback
	}
	var version string
	err := r.db.QueryRow(ctx, `
SELECT policy_version FROM hauling_policy_snapshots
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
// policy, which is invisible otherwise precisely because matching keeps
// working against the last snapshot.
func (r *Repository) PolicyLagSeconds(ctx context.Context, kind string, now time.Time) (float64, bool) {
	if r == nil || r.db == nil {
		return 0, false
	}
	var appliedAt time.Time
	err := r.db.QueryRow(ctx, `
SELECT applied_at FROM hauling_policy_snapshots
 WHERE policy_kind = $1 ORDER BY applied_at DESC LIMIT 1`, kind).Scan(&appliedAt)
	if err != nil {
		return 0, false
	}
	return now.UTC().Sub(appliedAt).Seconds(), true
}

// FleetCompleteness reports how much of the active fleet has been surveyed.
//
// This is the number that decides whether fail-closed enforcement is safe to
// switch on, so it is computed from the same facts the fit engine refuses on
// rather than from a proxy like "has any capability row".
func (r *Repository) FleetCompleteness(ctx context.Context) (surveyed, total int64, err error) {
	if r == nil || r.db == nil {
		return 0, 0, nil
	}
	err = r.db.QueryRow(ctx, `
SELECT
	COUNT(*) FILTER (WHERE max_payload_grams IS NOT NULL),
	COUNT(*)
FROM trucks WHERE status = 'active'`).Scan(&surveyed, &total)
	return surveyed, total, err
}

func (r *Repository) String() string {
	return fmt.Sprintf("truckcapability.Repository(db=%t)", r != nil && r.db != nil)
}
