package searchprojection

import "testing"

func TestSection13DUnicodeNormalization(
	t *testing.T,
) {
	got, err :=
		NormalizeText(
			"  ＪＯＬＬＯＦ   Rice  ",
		)
	if err != nil {
		t.Fatal(err)
	}

	if got != "jollof rice" {
		t.Fatalf(
			"normalized=%q want=%q",
			got,
			"jollof rice",
		)
	}
}

func TestSection13DPunctuationDoesNotDestroyNigerianTerms(
	t *testing.T,
) {
	got, err :=
		NormalizeText(
			"Puff-Puff / Moi-Moi / Egusi",
		)
	if err != nil {
		t.Fatal(err)
	}

	if got !=
		"puff puff moi moi egusi" {
		t.Fatalf(
			"normalized=%q",
			got,
		)
	}
}

func TestSection13DNoRawSearchSyntaxPreserved(
	t *testing.T,
) {
	got, err :=
		NormalizeSearchQuery(
			`rice:* & !jollof`,
		)
	if err != nil {
		t.Fatal(err)
	}

	if got !=
		"rice jollof" {
		t.Fatalf(
			"normalized query=%q",
			got,
		)
	}
}

func TestSection13DShopMeasurementVocabularyNormalization(
	t *testing.T,
) {
	values := []string{
		"Paint",
		"Half Paint",
		"Derica",
		"50kg",
		"Bag",
		"Crate",
		"Basket",
	}

	normalized, err :=
		normalizeDistinctValues(values)
	if err != nil {
		t.Fatal(err)
	}

	if len(normalized) != 7 {
		t.Fatalf(
			"measurement terms=%d want=7",
			len(normalized),
		)
	}

	if normalized[0] != "paint" ||
		normalized[1] != "half paint" ||
		normalized[2] != "derica" ||
		normalized[3] != "50kg" {
		t.Fatalf(
			"unexpected normalized measurement terms: %#v",
			normalized,
		)
	}
}
