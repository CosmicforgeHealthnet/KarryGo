package serviceability

import (
	"testing"
	"time"
)

func normalPackage() PackageDeclaration {
	return PackageDeclaration{PackageType: "BOX", SizeCategory: "small", CategoryCode: "GROCERY", DeclaredWeightGrams: 1000, ItemCount: 1, PackagingCondition: "SEALED"}
}

func TestPackagePolicyDecisions(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	measured := int64(1300)
	tests := []struct {
		name   string
		mutate func(*PackageDeclaration)
		want   AdmissionResult
	}{
		{"normal", func(*PackageDeclaration) {}, AdmissionAccepted},
		{"fragile", func(p *PackageDeclaration) { p.Fragile = true }, AdmissionAcceptedWithConditions},
		{"prohibited category", func(p *PackageDeclaration) { p.CategoryCode = "PRESCRIPTION_MEDICINE" }, AdmissionRefusedProhibited},
		{"explicit prohibited declaration", func(p *PackageDeclaration) { p.Prohibited = true }, AdmissionRefusedProhibited},
		{"hazardous", func(p *PackageDeclaration) { p.HazardClass = "FLAMMABLE" }, AdmissionRefusedUnsafe},
		{"restricted", func(p *PackageDeclaration) { p.Restricted = true }, AdmissionOperationalReview},
		{"medical", func(p *PackageDeclaration) { p.Medical = true }, AdmissionOperationalReview},
		{"repack", func(p *PackageDeclaration) { p.PackagingCondition = "DAMAGED" }, AdmissionRequiresRepack},
		{"reprice", func(p *PackageDeclaration) { p.MeasuredWeightGrams = &measured }, AdmissionRequiresReprice},
		{"high value", func(p *PackageDeclaration) { p.HighValueClassification = "HIGH" }, AdmissionAcceptedWithConditions},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			declaration := normalPackage()
			test.mutate(&declaration)
			got := (DefaultPackagePolicy{}).Evaluate(declaration, now)
			if got.Result != test.want {
				t.Fatalf("result=%s want=%s", got.Result, test.want)
			}
			if got.PolicyVersion != PackagePolicyVersion || !got.EvaluatedAt.Equal(now) {
				t.Fatalf("policy evidence=%+v", got)
			}
		})
	}
}

func TestPackageFreeTextIsNotClassificationAuthority(t *testing.T) {
	declaration := normalPackage()
	declaration.Description = "prescription medicine words supplied only as free text"
	got := (DefaultPackagePolicy{}).Evaluate(declaration, time.Now())
	if got.Result != AdmissionAccepted {
		t.Fatalf("free text became policy authority: %+v", got)
	}
}
