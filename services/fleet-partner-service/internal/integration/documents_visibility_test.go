package integration_test

import (
	"context"
	"net/http"
	"testing"

	documentmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/models"
	documentusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/usecases"
)

// --- visibility and archival -----------------------------------------------
//
// Section 25 gap-close: a file reference defaults to INTERNAL, RESTRICTED and
// COMPANY_CONFIDENTIAL require a permission ordinary documents.download does
// not carry, and archiving takes a document out of default listings without
// touching its lifecycle state.

// TestDocumentDefaultVisibilityIsInternal proves a document created without
// naming a tier lands on the strictest default rather than an open one.
func TestDocumentDefaultVisibilityIsInternal(t *testing.T) {
	f := newDocumentFixture(t, "Doc Visibility Default Co", "DOC-VIS-1")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "uploader")

	ticket := f.createDocument(t, member, "INSURANCE_CERTIFICATE", nil)

	if ticket.Document.Visibility != documentmodels.VisibilityInternal {
		t.Fatalf("visibility = %s, want INTERNAL", ticket.Document.Visibility)
	}
}

// createRestrictedDocument uploads a SAFE document at the given elevated
// visibility tier, ready to be downloaded.
func createRestrictedDocument(t *testing.T, f *documentFixture, member string,
	visibility string) *documentusecases.UploadTicket {
	t.Helper()
	ticket, err := f.service.RequestUpload(context.Background(), documentusecases.RequestUploadInput{
		Actor: f.actor(member), OwnerType: "COMPANY", OwnerID: f.orgID,
		Category: "COMPLIANCE", DocumentType: "BOARD_RESOLUTION",
		AllowedMIMETypes: []string{"application/pdf"}, MaxSizeBytes: 1 << 20,
		Visibility:     visibility,
		IdempotencyKey: "doc-" + visibility + "-" + member,
	})
	if err != nil {
		t.Fatalf("request upload: %v", err)
	}
	f.media.completeUpload(ticket.Document.MediaObjectID, "SAFE", []byte("%PDF-1.7 test"))
	if _, err := f.deliverMediaEvent(t, ticket.Document.MediaObjectID,
		"media.upload_completed.v1", 2, nil); err != nil {
		t.Fatalf("deliver upload event: %v", err)
	}
	return ticket
}

// TestRestrictedVisibilityRequiresElevatedPermission proves ordinary
// documents.download is not enough for RESTRICTED or COMPANY_CONFIDENTIAL
// content: the company administrator, who can download every other document,
// is refused; the compliance and safety manager, who additionally holds
// documents.download_restricted, is let through.
func TestRestrictedVisibilityRequiresElevatedPermission(t *testing.T) {
	for _, visibility := range []string{
		documentmodels.VisibilityRestricted, documentmodels.VisibilityCompanyConfidential,
	} {
		t.Run(visibility, func(t *testing.T) {
			f := newDocumentFixture(t, "Doc Restricted "+visibility+" Co", "DOC-VIS-"+visibility)
			owner := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "owner")
			ticket := createRestrictedDocument(t, f, owner, visibility)
			path := "/fleet-partner/v1/documents/" + ticket.Document.ID

			admin := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "admin")
			adminRouter := f.router(t, admin, nil)
			if status, response := documentCall(t, adminRouter, http.MethodPost,
				path+"/download-requests", nil, nil); status != http.StatusForbidden {
				t.Fatalf("admin download status = %d body=%v, want 403 (ordinary download does not extend to %s)",
					status, response, visibility)
			}

			compliance := seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "compliance")
			complianceRouter := f.router(t, compliance, nil)
			status, response := documentCall(t, complianceRouter, http.MethodPost,
				path+"/download-requests", nil, nil)
			if status != http.StatusOK {
				t.Fatalf("compliance download status = %d body=%v, want 200", status, response)
			}
			grant := documentData(t, response)
			if token, _ := grant["downloadToken"].(string); token == "" {
				t.Fatal("no download grant was issued")
			}
		})
	}
}

// TestArchiveIsVersionGuardedAndIdempotentlyRefused proves archive obeys
// optimistic concurrency and cannot be applied twice, matching every other
// mutation in this domain.
func TestArchiveIsVersionGuardedAndIdempotentlyRefused(t *testing.T) {
	f := newDocumentFixture(t, "Doc Archive Co", "DOC-VIS-ARCHIVE")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "archiver")
	ticket := f.createDocument(t, member, "INSURANCE_CERTIFICATE", nil)
	router := f.router(t, member, nil)
	path := "/fleet-partner/v1/documents/" + ticket.Document.ID

	// A stale expected version is refused.
	if status, response := documentCall(t, router, http.MethodPost, path+"/archive",
		map[string]any{"expectedVersion": ticket.Document.Version + 1}, nil); status != http.StatusConflict {
		t.Fatalf("stale archive status = %d body=%v, want 409", status, response)
	}

	status, response := documentCall(t, router, http.MethodPost, path+"/archive",
		map[string]any{"expectedVersion": ticket.Document.Version}, nil)
	if status != http.StatusOK {
		t.Fatalf("archive status = %d body=%v, want 200", status, response)
	}
	document := documentData(t, response)
	if archived, _ := document["archived"].(bool); !archived {
		t.Fatalf("document not reported archived: %v", document)
	}
	if archivedAt, _ := document["archivedAt"].(string); archivedAt == "" {
		t.Fatal("archivedAt was not set")
	}

	// Archiving an already-archived document is refused rather than silently
	// re-applied, matching revoke's existing convention in this domain.
	if status, response := documentCall(t, router, http.MethodPost, path+"/archive",
		map[string]any{}, nil); status != http.StatusConflict {
		t.Fatalf("re-archive status = %d body=%v, want 409", status, response)
	}
}

// TestArchivedDocumentsAreExcludedFromDefaultListing proves archive takes a
// document out of default visibility without deleting or otherwise changing
// it: an explicit includeArchived=true still finds it.
func TestArchivedDocumentsAreExcludedFromDefaultListing(t *testing.T) {
	f := newDocumentFixture(t, "Doc Archive List Co", "DOC-VIS-ARCHIVE-LIST")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "lister")
	ticket := f.createDocument(t, member, "INSURANCE_CERTIFICATE", nil)
	router := f.router(t, member, nil)
	path := "/fleet-partner/v1/documents/" + ticket.Document.ID

	if status, response := documentCall(t, router, http.MethodPost, path+"/archive",
		map[string]any{"expectedVersion": ticket.Document.Version}, nil); status != http.StatusOK {
		t.Fatalf("archive status = %d body=%v, want 200", status, response)
	}

	status, response := documentCall(t, router, http.MethodGet, "/fleet-partner/v1/documents", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("list status = %d body=%v, want 200", status, response)
	}
	if listContainsDocument(t, response, ticket.Document.ID) {
		t.Fatal("an archived document must not appear in the default listing")
	}

	status, response = documentCall(t, router, http.MethodGet,
		"/fleet-partner/v1/documents?includeArchived=true", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("list (includeArchived) status = %d body=%v, want 200", status, response)
	}
	if !listContainsDocument(t, response, ticket.Document.ID) {
		t.Fatal("includeArchived=true must still find the archived document")
	}
}

func listContainsDocument(t *testing.T, response map[string]any, documentID string) bool {
	t.Helper()
	data := documentData(t, response)
	documents, ok := data["documents"].([]any)
	if !ok {
		t.Fatalf("response carries no documents array: %v", response)
	}
	for _, entry := range documents {
		view, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := view["id"].(string); id == documentID {
			return true
		}
	}
	return false
}
