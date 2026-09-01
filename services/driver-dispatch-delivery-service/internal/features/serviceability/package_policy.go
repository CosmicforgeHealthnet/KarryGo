package serviceability

import (
	"strings"
	"time"
)

const PackagePolicyVersion = "dispatch_package_admission_v1"

type PackagePolicyEvaluator interface {
	Evaluate(PackageDeclaration, time.Time) PackageAdmission
}

type DefaultPackagePolicy struct{}

var prohibitedCategoryCodes = map[string]bool{
	"ILLEGAL_DRUGS": true, "WEAPONS": true, "EXPLOSIVES": true,
	"PRESCRIPTION_MEDICINE": true, "OTC_MEDICINE": true, "PHARMACEUTICAL": true,
}

func (DefaultPackagePolicy) Evaluate(p PackageDeclaration, now time.Time) PackageAdmission {
	result := PackageAdmission{Result: AdmissionAccepted, PolicyVersion: PackagePolicyVersion, EvaluatedAt: now.UTC()}
	category := strings.ToUpper(strings.TrimSpace(p.CategoryCode))
	hazard := strings.ToUpper(strings.TrimSpace(p.HazardClass))
	condition := strings.ToUpper(strings.TrimSpace(p.PackagingCondition))

	if p.Prohibited || prohibitedCategoryCodes[category] {
		result.Result = AdmissionRefusedProhibited
		result.ReasonCodes = []string{"PROHIBITED_CLASSIFICATION"}
		return result
	}
	if hazard == "EXPLOSIVE" || hazard == "FLAMMABLE" || hazard == "CORROSIVE" || hazard == "TOXIC" {
		result.Result = AdmissionRefusedUnsafe
		result.ReasonCodes = []string{"HAZARDOUS_CLASSIFICATION"}
		return result
	}
	if condition == "DAMAGED" || condition == "OPEN" || condition == "INADEQUATE" {
		result.Result = AdmissionRequiresRepack
		result.ReasonCodes = []string{"PACKAGING_REQUIRES_REPACK"}
		return result
	}
	if p.MeasuredWeightGrams != nil && *p.MeasuredWeightGrams > p.DeclaredWeightGrams+p.DeclaredWeightGrams/10 {
		result.Result = AdmissionRequiresReprice
		result.ReasonCodes = []string{"MEASURED_WEIGHT_MISMATCH"}
		result.MismatchClass = "WEIGHT_ABOVE_DECLARATION"
		return result
	}
	if p.Restricted || p.Medical || p.TemperatureSensitive || category == "ALCOHOL" || category == "TOBACCO" {
		result.Result = AdmissionOperationalReview
		result.ReasonCodes = []string{"CONTROLLED_CATEGORY_REVIEW"}
		return result
	}
	if p.Fragile || p.Perishable || strings.TrimSpace(p.SpecialHandling) != "" || strings.TrimSpace(p.HighValueClassification) != "" {
		result.Result = AdmissionAcceptedWithConditions
		result.ReasonCodes = []string{"SPECIAL_HANDLING_REQUIRED"}
		result.Conditions = []string{"HANDLE_AS_DECLARED"}
	}
	return result
}
