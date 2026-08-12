package models

import "testing"

func TestValidate_DefaultsAreValid(t *testing.T) {
	if errs := Defaults("org_1").Validate(); len(errs) != 0 {
		t.Fatalf("platform defaults should validate cleanly, got %v", errs)
	}
}

func TestValidate_RejectsStepUpFreshnessBelowPlatformMinimum(t *testing.T) {
	s := Defaults("org_1")
	s.StepUpFreshnessSeconds = MinStepUpFreshnessSeconds - 1
	errs := s.Validate()
	if len(errs) == 0 {
		t.Fatal("expected a validation error for step-up freshness below the platform minimum")
	}
	if errs[0].Field != "stepUpFreshnessSeconds" {
		t.Fatalf("unexpected field: %s", errs[0].Field)
	}
}

func TestValidate_RejectsStepUpFreshnessAbovePlatformMaximum(t *testing.T) {
	s := Defaults("org_1")
	s.StepUpFreshnessSeconds = MaxStepUpFreshnessSeconds + 1
	if errs := s.Validate(); len(errs) == 0 {
		t.Fatal("expected a validation error for step-up freshness above the platform maximum")
	}
}

func TestValidate_RejectsUnknownEnums(t *testing.T) {
	s := Defaults("org_1")
	s.SessionRevocationPolicy = "SOMETIMES"
	s.AllowedProviderAccessPolicy = "ANYTHING"
	s.AuditCapturePolicy = "PARTIAL"
	errs := s.Validate()
	if len(errs) != 3 {
		t.Fatalf("expected 3 validation errors for 3 bad enums, got %d: %v", len(errs), errs)
	}
}

func TestValidate_RejectsNegativeValues(t *testing.T) {
	s := Defaults("org_1")
	s.SensitiveExportThresholdKobo = -1
	s.RetentionPreferenceDays = -1
	if errs := s.Validate(); len(errs) != 2 {
		t.Fatalf("expected 2 validation errors for negative amounts, got %d: %v", len(errs), errs)
	}
}
