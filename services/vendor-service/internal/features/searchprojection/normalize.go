package searchprojection

import (
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	maxNormalizationRunes = 4096
	maxSearchQueryRunes   = 256

	maxExpansionValues = 256
	maxSearchTokens    = 512
)

var unicodeCaseFolder = cases.Fold()

func NormalizeText(
	value string,
) (string, error) {
	if utf8.RuneCountInString(value) >
		maxNormalizationRunes {
		return "", ErrInvalidArgument
	}

	value = norm.NFKC.String(value)
	value = unicodeCaseFolder.String(value)

	var builder strings.Builder
	builder.Grow(len(value))

	spacePending := false

	for _, r := range value {
		switch {
		case unicode.IsLetter(r),
			unicode.IsNumber(r),
			unicode.IsMark(r):

			if spacePending &&
				builder.Len() > 0 {
				builder.WriteByte(' ')
			}

			builder.WriteRune(r)
			spacePending = false

		case unicode.IsSpace(r),
			unicode.IsPunct(r),
			unicode.IsSymbol(r),
			unicode.IsControl(r):

			spacePending = true

		default:
			spacePending = true
		}
	}

	return strings.TrimSpace(
		builder.String(),
	), nil
}

func NormalizeSearchQuery(
	value string,
) (string, error) {
	if strings.TrimSpace(value) == "" ||
		utf8.RuneCountInString(value) >
			maxSearchQueryRunes {
		return "", ErrInvalidArgument
	}

	normalized, err := NormalizeText(value)
	if err != nil {
		return "", err
	}

	if normalized == "" {
		return "", ErrInvalidArgument
	}

	return normalized, nil
}

func normalizeDistinctValues(
	values []string,
) ([]string, error) {
	if len(values) > maxExpansionValues {
		return nil, ErrInvalidArgument
	}

	result := make(
		[]string,
		0,
		len(values),
	)

	seen := make(map[string]struct{})

	for _, value := range values {
		normalized, err := NormalizeText(value)
		if err != nil {
			return nil, err
		}

		if normalized == "" {
			continue
		}

		if _, exists := seen[normalized]; exists {
			continue
		}

		seen[normalized] = struct{}{}

		result = append(
			result,
			normalized,
		)
	}

	return result, nil
}

func buildSearchMaterial(
	name string,
	groups ...[]string,
) (
	normalizedName string,
	normalizedGroups [][]string,
	tokens []string,
	searchText string,
	err error,
) {
	normalizedName, err =
		NormalizeText(name)
	if err != nil {
		return "", nil, nil, "", err
	}

	if normalizedName == "" {
		return "", nil, nil, "", ErrInvalidArgument
	}

	allPhrases := []string{
		normalizedName,
	}

	tokenSeen := make(map[string]struct{})

	appendTokens := func(
		value string,
	) error {
		for _, token := range strings.Fields(value) {
			if _, exists := tokenSeen[token]; exists {
				continue
			}

			if len(tokenSeen) >= maxSearchTokens {
				return ErrInvalidArgument
			}

			tokenSeen[token] = struct{}{}
			tokens = append(tokens, token)
		}

		return nil
	}

	if err = appendTokens(
		normalizedName,
	); err != nil {
		return "", nil, nil, "", err
	}

	normalizedGroups = make(
		[][]string,
		0,
		len(groups),
	)

	phraseSeen := map[string]struct{}{
		normalizedName: {},
	}

	for _, group := range groups {
		normalized, normalizeErr :=
			normalizeDistinctValues(group)
		if normalizeErr != nil {
			return "", nil, nil, "", normalizeErr
		}

		normalizedGroups = append(
			normalizedGroups,
			normalized,
		)

		for _, phrase := range normalized {
			if _, exists := phraseSeen[phrase]; !exists {
				phraseSeen[phrase] = struct{}{}

				allPhrases = append(
					allPhrases,
					phrase,
				)
			}

			if err = appendTokens(
				phrase,
			); err != nil {
				return "", nil, nil, "", err
			}
		}
	}

	return normalizedName,
		normalizedGroups,
		tokens,
		strings.Join(allPhrases, " "),
		nil
}

func normalizedJSONObject(
	raw json.RawMessage,
) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}

	var object map[string]any

	if err := json.Unmarshal(
		raw,
		&object,
	); err != nil {
		return nil, ErrInvalidArgument
	}

	return json.Marshal(object)
}

func normalizeSearchRequest(
	request SearchRequest,
) (
	SearchRequest,
	string,
	error,
) {
	if request.Generation <= 0 {
		return SearchRequest{},
			"",
			ErrInvalidArgument
	}

	query, err :=
		NormalizeSearchQuery(
			request.Query,
		)
	if err != nil {
		return SearchRequest{},
			"",
			err
	}

	request.BranchRef =
		strings.TrimSpace(
			request.BranchRef,
		)

	if request.Limit == 0 {
		request.Limit = 20
	}

	if request.Limit < 1 ||
		request.Limit > 100 {
		return SearchRequest{},
			"",
			ErrInvalidArgument
	}

	return request, query, nil
}
