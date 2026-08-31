package searchprojection

import (
	"encoding/json"
	"strconv"
	"strings"
)

func normalizeFilterValues(
	values []string,
) (
	[]string,
	error,
) {
	if len(values) >
		maxFilterValuesPerKind {
		return nil,
			ErrInvalidArgument
	}

	return normalizeDistinctValues(
		values,
	)
}

func normalizeFoodFilters(
	input FoodSearchFilters,
) (
	FoodSearchFilters,
	error,
) {
	var err error

	if input.ProviderTypes, err =
		normalizeFilterValues(
			input.ProviderTypes,
		); err != nil {
		return FoodSearchFilters{}, err
	}

	if input.Cuisines, err =
		normalizeFilterValues(
			input.Cuisines,
		); err != nil {
		return FoodSearchFilters{}, err
	}

	if input.MealPeriods, err =
		normalizeFilterValues(
			input.MealPeriods,
		); err != nil {
		return FoodSearchFilters{}, err
	}

	if input.DishTypes, err =
		normalizeFilterValues(
			input.DishTypes,
		); err != nil {
		return FoodSearchFilters{}, err
	}

	if input.FoodGroups, err =
		normalizeFilterValues(
			input.FoodGroups,
		); err != nil {
		return FoodSearchFilters{}, err
	}

	if input.Occasions, err =
		normalizeFilterValues(
			input.Occasions,
		); err != nil {
		return FoodSearchFilters{}, err
	}

	if input.DietaryClaims, err =
		normalizeFilterValues(
			input.DietaryClaims,
		); err != nil {
		return FoodSearchFilters{}, err
	}

	if input.PreferenceClaims, err =
		normalizeFilterValues(
			input.PreferenceClaims,
		); err != nil {
		return FoodSearchFilters{}, err
	}

	if input.MinPriceMinor != nil &&
		*input.MinPriceMinor < 0 {
		return FoodSearchFilters{},
			ErrInvalidArgument
	}

	if input.MaxPriceMinor != nil &&
		*input.MaxPriceMinor < 0 {
		return FoodSearchFilters{},
			ErrInvalidArgument
	}

	if input.MinPriceMinor != nil &&
		input.MaxPriceMinor != nil &&
		*input.MinPriceMinor >
			*input.MaxPriceMinor {
		return FoodSearchFilters{},
			ErrInvalidArgument
	}

	return input, nil
}

func normalizeShopFilters(
	input ShopSearchFilters,
) (
	ShopSearchFilters,
	error,
) {
	var err error

	if input.CanonicalCategories, err =
		normalizeFilterValues(
			input.CanonicalCategories,
		); err != nil {
		return ShopSearchFilters{}, err
	}

	if input.Brands, err =
		normalizeFilterValues(
			input.Brands,
		); err != nil {
		return ShopSearchFilters{}, err
	}

	if input.OperatingModels, err =
		normalizeFilterValues(
			input.OperatingModels,
		); err != nil {
		return ShopSearchFilters{}, err
	}

	if input.ProductClasses, err =
		normalizeFilterValues(
			input.ProductClasses,
		); err != nil {
		return ShopSearchFilters{}, err
	}

	if input.SellUnits, err =
		normalizeFilterValues(
			input.SellUnits,
		); err != nil {
		return ShopSearchFilters{}, err
	}

	if input.Measurements, err =
		normalizeFilterValues(
			input.Measurements,
		); err != nil {
		return ShopSearchFilters{}, err
	}

	if input.FreshAttributes, err =
		normalizeFilterValues(
			input.FreshAttributes,
		); err != nil {
		return ShopSearchFilters{}, err
	}

	if input.TemperatureZones, err =
		normalizeFilterValues(
			input.TemperatureZones,
		); err != nil {
		return ShopSearchFilters{}, err
	}

	if input.MinPriceMinor != nil &&
		*input.MinPriceMinor < 0 {
		return ShopSearchFilters{},
			ErrInvalidArgument
	}

	if input.MaxPriceMinor != nil &&
		*input.MaxPriceMinor < 0 {
		return ShopSearchFilters{},
			ErrInvalidArgument
	}

	if input.MinPriceMinor != nil &&
		input.MaxPriceMinor != nil &&
		*input.MinPriceMinor >
			*input.MaxPriceMinor {
		return ShopSearchFilters{},
			ErrInvalidArgument
	}

	return input, nil
}

func stringSlicesIntersect(
	filter []string,
	document []string,
) bool {
	if len(filter) == 0 {
		return true
	}

	documentSet :=
		make(
			map[string]struct{},
			len(document),
		)

	for _, value := range document {
		normalized, err :=
			NormalizeText(value)
		if err != nil ||
			normalized == "" {
			continue
		}

		documentSet[normalized] =
			struct{}{}
	}

	for _, value := range filter {
		if _, exists :=
			documentSet[value]; exists {
			return true
		}
	}

	return false
}

func jsonObject(
	raw json.RawMessage,
) map[string]any {
	if len(raw) == 0 {
		return nil
	}

	var value map[string]any

	if json.Unmarshal(
		raw,
		&value,
	) != nil {
		return nil
	}

	return value
}

func jsonBool(
	raw json.RawMessage,
	keys ...string,
) (
	bool,
	bool,
) {
	object := jsonObject(raw)

	for _, key := range keys {
		value, exists :=
			object[key]

		if !exists {
			continue
		}

		result, ok :=
			value.(bool)

		if ok {
			return result, true
		}
	}

	return false, false
}

func jsonInt64(
	raw json.RawMessage,
	keys ...string,
) (
	int64,
	bool,
) {
	object := jsonObject(raw)

	for _, key := range keys {
		value, exists :=
			object[key]

		if !exists {
			continue
		}

		switch typed :=
			value.(type) {
		case float64:
			return int64(typed), true

		case string:
			parsed, err :=
				strconv.ParseInt(
					typed,
					10,
					64,
				)

			if err == nil {
				return parsed, true
			}
		}
	}

	return 0, false
}

func jsonString(
	raw json.RawMessage,
	keys ...string,
) (
	string,
	bool,
) {
	object := jsonObject(raw)

	for _, key := range keys {
		value, exists :=
			object[key]

		if !exists {
			continue
		}

		text, ok :=
			value.(string)

		if ok &&
			strings.TrimSpace(text) != "" {
			normalized, err :=
				NormalizeText(text)

			if err == nil &&
				normalized != "" {
				return normalized, true
			}
		}
	}

	return "", false
}

func jsonBasisPoints(
	raw json.RawMessage,
	keys ...string,
) int {
	value, ok :=
		jsonInt64(
			raw,
			keys...,
		)

	if !ok {
		return 0
	}

	if value < 0 {
		return 0
	}

	if value > 10000 {
		return 10000
	}

	return int(value)
}

func flattenJSONStrings(
	raw json.RawMessage,
) []string {
	if len(raw) == 0 {
		return nil
	}

	var value any

	if json.Unmarshal(
		raw,
		&value,
	) != nil {
		return nil
	}

	result := make(
		[]string,
		0,
	)

	seen :=
		make(
			map[string]struct{},
		)

	var walk func(any)

	walk = func(input any) {
		switch typed :=
			input.(type) {
		case string:
			normalized, err :=
				NormalizeText(typed)

			if err != nil ||
				normalized == "" {
				return
			}

			if _, exists :=
				seen[normalized]; exists {
				return
			}

			seen[normalized] =
				struct{}{}

			result = append(
				result,
				normalized,
			)

		case []any:
			for _, child := range typed {
				walk(child)
			}

		case map[string]any:
			for _, child := range typed {
				walk(child)
			}
		}
	}

	walk(value)

	return result
}

func boolFilterMatches(
	filter *bool,
	value bool,
	exists bool,
) bool {
	if filter == nil {
		return true
	}

	if !exists {
		return false
	}

	return value ==
		*filter
}

func priceFilterMatches(
	minimum *int64,
	maximum *int64,
	value *int64,
) bool {
	if minimum == nil &&
		maximum == nil {
		return true
	}

	if value == nil {
		return false
	}

	if minimum != nil &&
		*value < *minimum {
		return false
	}

	if maximum != nil &&
		*value > *maximum {
		return false
	}

	return true
}
