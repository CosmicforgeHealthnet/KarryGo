package searchprojection

// Section 13I ranking profile administration.
//
// A ranking profile is the weight vector the search engine scores with. It is
// platform configuration, not vendor configuration: one profile decides the
// order every vendor's results appear in, so changing it is a platform act
// reachable only through the internal HMAC-v2 surface.
//
// What this file deliberately is NOT: an advertising system. There is no
// campaign, no budget, no auction and no per-vendor boost. The only commercial
// levers are the two bounded weights the Section 13E model already defines
// (CommercialWeight and PromotionWeight), and both are capped by
// MaxNonTextBoost, which is itself required to stay below ExactNameWeight.
// That inequality is the whole safeguard: however hard a result is promoted,
// the non-text boost it can accumulate is strictly smaller than the score an
// exact textual name match earns, so a promoted irrelevant result cannot
// outrank a relevant one. It is enforced three times over - by the database
// CHECK constraints, by validateRankingProfile, and by the update path here -
// because it is the one ranking rule whose violation would be invisible in
// normal operation and corrosive to every search result on the platform.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// rankingProfileColumns is the single column list every ranking read uses.
const rankingProfileColumns = `
    id,
    profile_code,
    namespace_code,
    version,
    exact_name_weight,
    prefix_weight,
    full_text_weight,
    alias_weight,
    synonym_weight,
    category_weight,
    tag_weight,
    brand_weight,
    identifier_weight,
    measurement_weight,
    typo_weight,
    availability_weight,
    serviceability_weight,
    trust_weight,
    reliability_weight,
    distance_weight,
    commercial_weight,
    promotion_weight,
    max_non_text_boost,
    max_typo_distance,
    max_candidates`

// PostgresRankingProfileStore reads and administers ranking profiles.
type PostgresRankingProfileStore struct {
	pool *pgxpool.Pool
}

func NewPostgresRankingProfileStore(pool *pgxpool.Pool) *PostgresRankingProfileStore {
	return &PostgresRankingProfileStore{pool: pool}
}

func scanRankingProfile(row pgx.Row) (RankingProfile, error) {
	var profile RankingProfile
	err := row.Scan(
		&profile.ID,
		&profile.ProfileCode,
		&profile.NamespaceCode,
		&profile.Version,
		&profile.ExactNameWeight,
		&profile.PrefixWeight,
		&profile.FullTextWeight,
		&profile.AliasWeight,
		&profile.SynonymWeight,
		&profile.CategoryWeight,
		&profile.TagWeight,
		&profile.BrandWeight,
		&profile.IdentifierWeight,
		&profile.MeasurementWeight,
		&profile.TypoWeight,
		&profile.AvailabilityWeight,
		&profile.ServiceabilityWeight,
		&profile.TrustWeight,
		&profile.ReliabilityWeight,
		&profile.DistanceWeight,
		&profile.CommercialWeight,
		&profile.PromotionWeight,
		&profile.MaxNonTextBoost,
		&profile.MaxTypoDistance,
		&profile.MaxCandidates,
	)
	return profile, err
}

// ListRankingProfiles returns the active profiles for one namespace.
func (store *PostgresRankingProfileStore) ListRankingProfiles(
	ctx context.Context,
	namespace SearchCacheNamespace,
) ([]RankingProfile, error) {
	if store == nil || store.pool == nil || !validSearchNamespace(namespace) {
		return nil, ErrInvalidArgument
	}

	rows, err := store.pool.Query(
		ctx,
		`SELECT`+rankingProfileColumns+`
           FROM search_ranking_profiles
          WHERE namespace_code=$1
            AND active=TRUE
          ORDER BY profile_code`,
		string(namespace),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RankingProfile
	for rows.Next() {
		profile, scanErr := scanRankingProfile(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, profile)
	}
	return out, rows.Err()
}

// GetRankingProfile reads one profile inside one namespace. The namespace is
// part of the predicate, not a filter applied afterwards, so a FOOD profile
// code cannot be read through the SHOP path.
func (store *PostgresRankingProfileStore) GetRankingProfile(
	ctx context.Context,
	namespace SearchCacheNamespace,
	profileCode string,
) (RankingProfile, error) {
	if store == nil || store.pool == nil || !validSearchNamespace(namespace) {
		return RankingProfile{}, ErrInvalidArgument
	}
	profileCode = strings.TrimSpace(profileCode)
	if profileCode == "" {
		return RankingProfile{}, ErrInvalidArgument
	}

	profile, err := scanRankingProfile(store.pool.QueryRow(
		ctx,
		`SELECT`+rankingProfileColumns+`
           FROM search_ranking_profiles
          WHERE namespace_code=$1
            AND profile_code=$2
            AND active=TRUE`,
		string(namespace),
		profileCode,
	))
	if err != nil {
		if err == pgx.ErrNoRows {
			return RankingProfile{}, ErrRankingProfileNotFound
		}
		return RankingProfile{}, err
	}
	return profile, nil
}

// RankingWeightUpdate carries the weights an administrator wants to set. Every
// field is optional; a nil field leaves the stored weight alone, so a caller
// adjusting one lever cannot accidentally reset the rest to zero.
type RankingWeightUpdate struct {
	ExactNameWeight *int `json:"exact_name_weight,omitempty"`
	PrefixWeight    *int `json:"prefix_weight,omitempty"`
	FullTextWeight  *int `json:"full_text_weight,omitempty"`
	AliasWeight     *int `json:"alias_weight,omitempty"`
	SynonymWeight   *int `json:"synonym_weight,omitempty"`
	CategoryWeight  *int `json:"category_weight,omitempty"`
	TagWeight       *int `json:"tag_weight,omitempty"`

	BrandWeight       *int `json:"brand_weight,omitempty"`
	IdentifierWeight  *int `json:"identifier_weight,omitempty"`
	MeasurementWeight *int `json:"measurement_weight,omitempty"`

	TypoWeight *int `json:"typo_weight,omitempty"`

	AvailabilityWeight   *int `json:"availability_weight,omitempty"`
	ServiceabilityWeight *int `json:"serviceability_weight,omitempty"`
	TrustWeight          *int `json:"trust_weight,omitempty"`
	ReliabilityWeight    *int `json:"reliability_weight,omitempty"`
	DistanceWeight       *int `json:"distance_weight,omitempty"`
	CommercialWeight     *int `json:"commercial_weight,omitempty"`
	PromotionWeight      *int `json:"promotion_weight,omitempty"`

	MaxNonTextBoost *int `json:"max_non_text_boost,omitempty"`
	MaxTypoDistance *int `json:"max_typo_distance,omitempty"`
	MaxCandidates   *int `json:"max_candidates,omitempty"`
}

func applyWeight(current int, update *int) int {
	if update == nil {
		return current
	}
	return *update
}

// apply merges an update onto a profile without writing anything.
func (update RankingWeightUpdate) apply(profile RankingProfile) RankingProfile {
	profile.ExactNameWeight = applyWeight(profile.ExactNameWeight, update.ExactNameWeight)
	profile.PrefixWeight = applyWeight(profile.PrefixWeight, update.PrefixWeight)
	profile.FullTextWeight = applyWeight(profile.FullTextWeight, update.FullTextWeight)
	profile.AliasWeight = applyWeight(profile.AliasWeight, update.AliasWeight)
	profile.SynonymWeight = applyWeight(profile.SynonymWeight, update.SynonymWeight)
	profile.CategoryWeight = applyWeight(profile.CategoryWeight, update.CategoryWeight)
	profile.TagWeight = applyWeight(profile.TagWeight, update.TagWeight)
	profile.BrandWeight = applyWeight(profile.BrandWeight, update.BrandWeight)
	profile.IdentifierWeight = applyWeight(profile.IdentifierWeight, update.IdentifierWeight)
	profile.MeasurementWeight = applyWeight(profile.MeasurementWeight, update.MeasurementWeight)
	profile.TypoWeight = applyWeight(profile.TypoWeight, update.TypoWeight)
	profile.AvailabilityWeight = applyWeight(profile.AvailabilityWeight, update.AvailabilityWeight)
	profile.ServiceabilityWeight = applyWeight(profile.ServiceabilityWeight, update.ServiceabilityWeight)
	profile.TrustWeight = applyWeight(profile.TrustWeight, update.TrustWeight)
	profile.ReliabilityWeight = applyWeight(profile.ReliabilityWeight, update.ReliabilityWeight)
	profile.DistanceWeight = applyWeight(profile.DistanceWeight, update.DistanceWeight)
	profile.CommercialWeight = applyWeight(profile.CommercialWeight, update.CommercialWeight)
	profile.PromotionWeight = applyWeight(profile.PromotionWeight, update.PromotionWeight)
	profile.MaxNonTextBoost = applyWeight(profile.MaxNonTextBoost, update.MaxNonTextBoost)
	profile.MaxTypoDistance = applyWeight(profile.MaxTypoDistance, update.MaxTypoDistance)
	profile.MaxCandidates = applyWeight(profile.MaxCandidates, update.MaxCandidates)
	return profile
}

// UpdateRankingProfile validates the proposed weights and stores them.
//
// The proposal is validated with validateRankingProfile - the same predicate
// the engine applies when it loads a profile to search with - so a weight
// vector that would be refused at query time is refused here instead of being
// stored and quietly breaking every subsequent search. The promotion bound is
// then checked explicitly, because that is the invariant an operator is most
// likely to try to push against.
func (store *PostgresRankingProfileStore) UpdateRankingProfile(
	ctx context.Context,
	namespace SearchCacheNamespace,
	profileCode string,
	update RankingWeightUpdate,
) (RankingProfile, error) {
	current, err := store.GetRankingProfile(ctx, namespace, profileCode)
	if err != nil {
		return RankingProfile{}, err
	}

	proposed := update.apply(current)

	if err = validateRankingProfile(proposed, string(namespace)); err != nil {
		return RankingProfile{}, err
	}

	// Commercial and promotional weight may never exceed the non-text boost
	// ceiling, and that ceiling may never reach an exact name match's weight.
	if proposed.PromotionWeight > proposed.MaxNonTextBoost ||
		proposed.CommercialWeight > proposed.MaxNonTextBoost ||
		proposed.MaxNonTextBoost >= proposed.ExactNameWeight {
		return RankingProfile{}, ErrRankingProfileInvalid
	}

	updated, err := scanRankingProfile(store.pool.QueryRow(
		ctx,
		`UPDATE search_ranking_profiles
            SET exact_name_weight=$3,
                prefix_weight=$4,
                full_text_weight=$5,
                alias_weight=$6,
                synonym_weight=$7,
                category_weight=$8,
                tag_weight=$9,
                brand_weight=$10,
                identifier_weight=$11,
                measurement_weight=$12,
                typo_weight=$13,
                availability_weight=$14,
                serviceability_weight=$15,
                trust_weight=$16,
                reliability_weight=$17,
                distance_weight=$18,
                commercial_weight=$19,
                promotion_weight=$20,
                max_non_text_boost=$21,
                max_typo_distance=$22,
                max_candidates=$23,
                updated_at=now()
          WHERE namespace_code=$1
            AND profile_code=$2
            AND active=TRUE
      RETURNING`+rankingProfileColumns,
		string(namespace),
		profileCode,
		proposed.ExactNameWeight,
		proposed.PrefixWeight,
		proposed.FullTextWeight,
		proposed.AliasWeight,
		proposed.SynonymWeight,
		proposed.CategoryWeight,
		proposed.TagWeight,
		proposed.BrandWeight,
		proposed.IdentifierWeight,
		proposed.MeasurementWeight,
		proposed.TypoWeight,
		proposed.AvailabilityWeight,
		proposed.ServiceabilityWeight,
		proposed.TrustWeight,
		proposed.ReliabilityWeight,
		proposed.DistanceWeight,
		proposed.CommercialWeight,
		proposed.PromotionWeight,
		proposed.MaxNonTextBoost,
		proposed.MaxTypoDistance,
		proposed.MaxCandidates,
	))
	if err != nil {
		if err == pgx.ErrNoRows {
			return RankingProfile{}, ErrRankingProfileNotFound
		}
		return RankingProfile{}, err
	}
	return updated, nil
}
