package taxonomy

import (
	"os"
	"testing"
)

func loadSemanticSeedForTest(
	t *testing.T,
) SemanticSeedBundle {
	t.Helper()

	raw, err := os.ReadFile(
		"seeds/semantics.v1.json",
	)
	if err != nil {
		t.Fatal(err)
	}

	bundle, err := DecodeSemanticSeed(raw)
	if err != nil {
		t.Fatal(err)
	}

	return bundle
}

func TestSection13C2SemanticSeedContract(t *testing.T) {
	bundle := loadSemanticSeedForTest(t)

	if bundle.SchemaVersion != 1 {
		t.Fatalf(
			"schema=%d want=1",
			bundle.SchemaVersion,
		)
	}

	if bundle.SeedKey !=
		"KARRYGO_SEMANTICS_V1" {
		t.Fatalf(
			"seed key=%s",
			bundle.SeedKey,
		)
	}

	if len(bundle.Tags) != 16 {
		t.Fatalf(
			"tags=%d want=16",
			len(bundle.Tags),
		)
	}

	if len(bundle.DietaryClaims) != 20 {
		t.Fatalf(
			"claims=%d want=20",
			len(bundle.DietaryClaims),
		)
	}
}

func TestSection13C2HighRiskClaimsRequireStrongPolicy(
	t *testing.T,
) {
	bundle := loadSemanticSeedForTest(t)

	for _, claim := range bundle.DietaryClaims {
		if claim.RiskLevel != DietaryRiskHigh {
			continue
		}

		if claim.VerificationPolicy ==
			DietaryVerificationDeclaration {
			t.Fatalf(
				"high-risk claim %s cannot be declaration-only",
				claim.Code,
			)
		}
	}
}

func TestSection13C2CriticalClaimsExist(t *testing.T) {
	bundle := loadSemanticSeedForTest(t)

	required := map[string]bool{
		"FOOD|VEGAN":       false,
		"FOOD|HALAL":       false,
		"FOOD|GLUTEN_FREE": false,
		"FOOD|NUT_FREE":    false,

		"SHOP|VEGAN":       false,
		"SHOP|HALAL":       false,
		"SHOP|GLUTEN_FREE": false,
		"SHOP|NUT_FREE":    false,
	}

	for _, claim := range bundle.DietaryClaims {
		key :=
			string(claim.Namespace) +
				"|" +
				claim.Code

		if _, exists := required[key]; exists {
			required[key] = true
		}
	}

	for key, found := range required {
		if !found {
			t.Fatalf(
				"required claim missing: %s",
				key,
			)
		}
	}
}

func TestSection13C2NamespaceScopedTags(t *testing.T) {
	bundle := loadSemanticSeedForTest(t)

	food := 0
	shop := 0

	for _, tag := range bundle.Tags {
		switch tag.Namespace {
		case NamespaceFood:
			food++

		case NamespaceShop:
			shop++

		default:
			t.Fatalf(
				"unexpected namespace %s",
				tag.Namespace,
			)
		}
	}

	if food != 8 || shop != 8 {
		t.Fatalf(
			"tag distribution FOOD=%d SHOP=%d",
			food,
			shop,
		)
	}
}

func TestSection13C2SemanticCodes(t *testing.T) {
	bundle := loadSemanticSeedForTest(t)

	for _, tag := range bundle.Tags {
		if err :=
			validateSemanticCode(tag.Code); err != nil {
			t.Fatalf(
				"invalid tag code %s",
				tag.Code,
			)
		}
	}

	for _, claim := range bundle.DietaryClaims {
		if err :=
			validateSemanticCode(claim.Code); err != nil {
			t.Fatalf(
				"invalid claim code %s",
				claim.Code,
			)
		}
	}
}
