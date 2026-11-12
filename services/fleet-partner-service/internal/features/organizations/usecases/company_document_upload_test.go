package usecases

import (
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// These cases guard the actual private Media contract, not the unrelated
// legacy media_assets purpose. A contract mismatch previously caused 422
// before a user's PDF bytes were ever uploaded.
func TestCompanyDocumentUploadUsesMediaPrivatePurpose(t *testing.T) {
	for _, input := range []string{"", "COMPANY_DOCUMENT", "company_document"} {
		purpose, err := companyDocumentPurpose(input)
		if err != nil || purpose != "COMPANY_DOCUMENT" {
			t.Fatalf("input %q: purpose=%q err=%v, want COMPANY_DOCUMENT", input, purpose, err)
		}
	}
	for _, input := range []string{"document_file", "DOCUMENT_FILE", "DATA_EXPORT", "PROVIDER_DOCUMENT"} {
		if purpose, err := companyDocumentPurpose(input); err == nil {
			t.Fatalf("input %q must be rejected, got purpose %q", input, purpose)
		}
	}
}

func TestCompanyDocumentOwnershipUsesCanonicalMediaScope(t *testing.T) {
	good := platformclients.FileMetadata{OwnerType: "ORGANIZATION", OwnerID: "org-1"}
	if !belongsToCompanyDocument(good, "org-1") {
		t.Fatal("valid company document is not recognized as owned")
	}
	for _, candidate := range []platformclients.FileMetadata{
		{OwnerType: "ORGANIZATION", OwnerID: "org-other"},
		{OwnerType: "VENDOR_BUSINESS", OwnerID: "org-1"},
		{OwnerType: "organization", OwnerID: "org-1"},
		{OwnerType: "", OwnerID: "org-1"},
	} {
		if belongsToCompanyDocument(candidate, "org-1") {
			t.Fatalf("incorrectly allowed owner type %q, owner ID %q", candidate.OwnerType, candidate.OwnerID)
		}
	}
}
