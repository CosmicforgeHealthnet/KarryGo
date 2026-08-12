package models

import (
	"testing"
	"time"
)

func TestValidVisibility(t *testing.T) {
	valid := []string{
		VisibilityInternal, VisibilityProviderVisible, VisibilityCustomerVisible,
		VisibilityRestricted, VisibilityCompanyConfidential,
	}
	for _, visibility := range valid {
		if !ValidVisibility(visibility) {
			t.Errorf("expected %q to be valid", visibility)
		}
	}
	invalid := []string{"", "PUBLIC", "internal", "unknown"}
	for _, visibility := range invalid {
		if ValidVisibility(visibility) {
			t.Errorf("expected %q to be invalid", visibility)
		}
	}
}

func TestRequiresElevatedAccess(t *testing.T) {
	elevated := []string{VisibilityRestricted, VisibilityCompanyConfidential}
	for _, visibility := range elevated {
		if !RequiresElevatedAccess(visibility) {
			t.Errorf("expected %q to require elevated access", visibility)
		}
	}
	ordinary := []string{VisibilityInternal, VisibilityProviderVisible, VisibilityCustomerVisible, ""}
	for _, visibility := range ordinary {
		if RequiresElevatedAccess(visibility) {
			t.Errorf("expected %q not to require elevated access", visibility)
		}
	}
}

func TestDocumentArchived(t *testing.T) {
	var document Document
	if document.Archived() {
		t.Fatal("a fresh document must not report archived")
	}
	now := time.Now()
	document.ArchivedAt = &now
	if !document.Archived() {
		t.Fatal("a document with ArchivedAt set must report archived")
	}
}
