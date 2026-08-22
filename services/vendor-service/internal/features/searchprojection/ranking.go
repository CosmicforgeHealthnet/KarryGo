package searchprojection

import (
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"
)

type searchTermKind string

const (
	searchTermName searchTermKind = "NAME"

	searchTermAlias searchTermKind = "ALIAS"

	searchTermSynonym searchTermKind = "SYNONYM"

	searchTermCategory searchTermKind = "CATEGORY"

	searchTermTag searchTermKind = "TAG"

	searchTermBrand searchTermKind = "BRAND"

	searchTermIdentifier searchTermKind = "IDENTIFIER"

	searchTermMeasurement searchTermKind = "MEASUREMENT"
)

type weightedSearchTerm struct {
	value string
	kind  searchTermKind
}

type rankedSearchKey struct {
	Sort SearchSort `json:"sort"`

	Score int64 `json:"score"`

	PriceMinor *int64 `json:"price_minor,omitempty"`

	DistanceMeters *int64 `json:"distance_meters,omitempty"`

	StableRef string `json:"stable_ref"`
}

func validateRankingProfile(
	profile RankingProfile,
	namespace string,
) error {
	if profile.ProfileCode == "" ||
		profile.NamespaceCode != namespace ||
		profile.Version <= 0 ||
		profile.MaxCandidates < 50 ||
		profile.MaxCandidates > 2000 ||
		profile.MaxTypoDistance < 0 ||
		profile.MaxTypoDistance > 2 ||
		profile.MaxNonTextBoost < 0 ||
		profile.MaxNonTextBoost >=
			profile.ExactNameWeight {
		return ErrRankingProfileInvalid
	}

	weights := []int{
		profile.ExactNameWeight,
		profile.PrefixWeight,
		profile.FullTextWeight,
		profile.AliasWeight,
		profile.SynonymWeight,
		profile.CategoryWeight,
		profile.TagWeight,
		profile.BrandWeight,
		profile.IdentifierWeight,
		profile.MeasurementWeight,
		profile.TypoWeight,
		profile.AvailabilityWeight,
		profile.ServiceabilityWeight,
		profile.TrustWeight,
		profile.ReliabilityWeight,
		profile.DistanceWeight,
		profile.CommercialWeight,
		profile.PromotionWeight,
	}

	for _, weight := range weights {
		if weight < 0 ||
			weight > 10000 {
			return ErrRankingProfileInvalid
		}
	}

	return nil
}

func normalizeEngineQuery(
	query string,
) (
	string,
	[]string,
	error,
) {
	normalized, err :=
		NormalizeSearchQuery(query)
	if err != nil {
		return "", nil, err
	}

	tokens := strings.Fields(
		normalized,
	)

	if len(tokens) == 0 ||
		len(tokens) > maxEngineQueryTokens {
		return "", nil, ErrInvalidArgument
	}

	for _, token := range tokens {
		if utf8.RuneCountInString(token) > 64 {
			return "", nil, ErrInvalidArgument
		}
	}

	return normalized, tokens, nil
}

func boundedLevenshtein(
	left string,
	right string,
	maxDistance int,
) (
	int,
	bool,
) {
	a := []rune(left)
	b := []rune(right)

	if maxDistance < 0 {
		return 0, false
	}

	lengthDelta :=
		len(a) - len(b)

	if lengthDelta < 0 {
		lengthDelta = -lengthDelta
	}

	if lengthDelta > maxDistance {
		return maxDistance + 1, false
	}

	previous := make(
		[]int,
		len(b)+1,
	)

	current := make(
		[]int,
		len(b)+1,
	)

	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(a); i++ {
		current[0] = i

		rowMinimum :=
			current[0]

		for j := 1; j <= len(b); j++ {
			cost := 0

			if a[i-1] != b[j-1] {
				cost = 1
			}

			deletion :=
				previous[j] + 1

			insertion :=
				current[j-1] + 1

			substitution :=
				previous[j-1] + cost

			current[j] =
				minInt(
					deletion,
					insertion,
					substitution,
				)

			if current[j] < rowMinimum {
				rowMinimum = current[j]
			}
		}

		if rowMinimum > maxDistance {
			return maxDistance + 1, false
		}

		previous,
			current =
			current,
			previous
	}

	distance :=
		previous[len(b)]

	return distance,
		distance <= maxDistance
}

func minInt(
	values ...int,
) int {
	result := values[0]

	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}

	return result
}

func termWeight(
	profile RankingProfile,
	kind searchTermKind,
) int {
	switch kind {
	case searchTermName:
		return profile.FullTextWeight

	case searchTermAlias:
		return profile.AliasWeight

	case searchTermSynonym:
		return profile.SynonymWeight

	case searchTermCategory:
		return profile.CategoryWeight

	case searchTermTag:
		return profile.TagWeight

	case searchTermBrand:
		return profile.BrandWeight

	case searchTermIdentifier:
		return profile.IdentifierWeight

	case searchTermMeasurement:
		return profile.MeasurementWeight

	default:
		return 0
	}
}

func normalizeWeightedTerms(
	terms []weightedSearchTerm,
) []weightedSearchTerm {
	result := make(
		[]weightedSearchTerm,
		0,
		len(terms),
	)

	seen := make(
		map[string]struct{},
	)

	for _, term := range terms {
		normalized, err :=
			NormalizeText(
				term.value,
			)
		if err != nil ||
			normalized == "" {
			continue
		}

		for _, token := range strings.Fields(
			normalized,
		) {
			key :=
				string(term.kind) +
					"\x00" +
					token

			if _, exists := seen[key]; exists {
				continue
			}

			seen[key] = struct{}{}

			result = append(
				result,
				weightedSearchTerm{
					value: token,
					kind:  term.kind,
				},
			)
		}
	}

	return result
}

func scoreTextMatch(
	normalizedQuery string,
	queryTokens []string,
	normalizedName string,
	terms []weightedSearchTerm,
	profile RankingProfile,
) (
	int64,
	bool,
) {
	normalizedTerms :=
		normalizeWeightedTerms(
			terms,
		)

	if len(normalizedTerms) == 0 {
		return 0, false
	}

	var score int64

	if normalizedName ==
		normalizedQuery {
		score += int64(
			profile.ExactNameWeight,
		)
	}

	for _, queryToken := range queryTokens {
		best := 0

		for _, term := range normalizedTerms {
			weight :=
				termWeight(
					profile,
					term.kind,
				)

			if queryToken ==
				term.value {
				if weight > best {
					best = weight
				}

				continue
			}

			if strings.HasPrefix(
				term.value,
				queryToken,
			) {
				prefixWeight :=
					profile.PrefixWeight

				if weight > 0 &&
					weight < prefixWeight {
					prefixWeight =
						weight
				}

				if prefixWeight > best {
					best =
						prefixWeight
				}

				continue
			}

			distance, matched :=
				boundedLevenshtein(
					queryToken,
					term.value,
					profile.MaxTypoDistance,
				)

			if !matched ||
				distance == 0 {
				continue
			}

			typoWeight :=
				profile.TypoWeight -
					((distance - 1) *
						(profile.TypoWeight / 3))

			if typoWeight < 1 {
				typoWeight = 1
			}

			if typoWeight > best {
				best =
					typoWeight
			}
		}

		// Every query token must find a bounded textual match.
		// Commercial/promotion signals cannot introduce an
		// otherwise irrelevant document.
		if best == 0 {
			return 0, false
		}

		score += int64(best)
	}

	return score, true
}

func applyNonTextBoost(
	textScore int64,
	profile RankingProfile,
	available bool,
	serviceable bool,
	trustBasisPoints int,
	reliabilityBasisPoints int,
	commercialBasisPoints int,
	promoted bool,
) int64 {
	if textScore <= 0 {
		return 0
	}

	boost := 0

	if available {
		boost +=
			profile.AvailabilityWeight
	}

	if serviceable {
		boost +=
			profile.ServiceabilityWeight
	}

	boost += boundedWeightedSignal(
		profile.TrustWeight,
		trustBasisPoints,
	)

	boost += boundedWeightedSignal(
		profile.ReliabilityWeight,
		reliabilityBasisPoints,
	)

	boost += boundedWeightedSignal(
		profile.CommercialWeight,
		commercialBasisPoints,
	)

	if promoted {
		boost +=
			profile.PromotionWeight
	}

	if boost >
		profile.MaxNonTextBoost {
		boost =
			profile.MaxNonTextBoost
	}

	return textScore +
		int64(boost)
}

func boundedWeightedSignal(
	weight int,
	basisPoints int,
) int {
	if basisPoints < 0 {
		basisPoints = 0
	}

	if basisPoints > 10000 {
		basisPoints = 10000
	}

	return weight *
		basisPoints /
		10000
}

func encodeSearchCursor(
	key rankedSearchKey,
) (
	string,
	error,
) {
	raw, err :=
		json.Marshal(key)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.
			EncodeToString(raw),
		nil
}

func decodeSearchCursor(
	value string,
	expectedSort SearchSort,
) (
	rankedSearchKey,
	error,
) {
	if value == "" {
		return rankedSearchKey{},
			nil
	}

	raw, err :=
		base64.RawURLEncoding.
			DecodeString(value)
	if err != nil {
		return rankedSearchKey{},
			ErrInvalidCursor
	}

	var key rankedSearchKey

	if err = json.Unmarshal(
		raw,
		&key,
	); err != nil {
		return rankedSearchKey{},
			ErrInvalidCursor
	}

	if key.Sort != expectedSort ||
		key.StableRef == "" {
		return rankedSearchKey{},
			ErrInvalidCursor
	}

	return key, nil
}

func sortFacetValues(
	counts map[string]int,
) []SearchFacetValue {
	values := make(
		[]SearchFacetValue,
		0,
		len(counts),
	)

	for value, count := range counts {
		values = append(
			values,
			SearchFacetValue{
				Value: value,
				Count: count,
			},
		)
	}

	sort.Slice(
		values,
		func(
			i int,
			j int,
		) bool {
			if values[i].Count !=
				values[j].Count {
				return values[i].Count >
					values[j].Count
			}

			return values[i].Value <
				values[j].Value
		},
	)

	if len(values) >
		maxFacetValues {
		values =
			values[:maxFacetValues]
	}

	return values
}
