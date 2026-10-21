package integration_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/models"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/usecases"
)

func TestNativeMediaDownloadGrantCannotOutliveEvidenceRevocation(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}, requireSafeForDownload: true})
	status, reply := f.authorizeUpload(t, "native-revocation", nil)
	if status != http.StatusOK {
		t.Fatal(status, reply)
	}
	upload := dataOf(t, reply)
	content := pdfFile(32)
	if status, reply := f.completeUpload(t, upload, content, nil); status != http.StatusOK {
		t.Fatal(status, reply)
	}
	id := upload["mediaObjectId"].(string)
	caller := usecases.Caller{Service: testCaller, OrganizationID: testOrgID}
	grant, err := f.service.AuthorizeDownload(context.Background(), caller, id)
	if err != nil {
		t.Fatal(err)
	}
	if bytes, _, err := f.service.ReadContent(context.Background(), id, grant.Token); err != nil || len(bytes) == 0 {
		t.Fatal("native pre-revocation grant", err)
	}
	object, err := f.service.GetObject(context.Background(), caller, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RevokeEvidence(context.Background(), usecases.RevokeInput{Caller: caller, MediaObjectID: id, ExpectedVersion: object.Version, Reason: "TEST_ONLY_REVOCATION"}); err != nil {
		t.Fatal(err)
	}
	if bytes, _, err := f.service.ReadContent(context.Background(), id, grant.Token); !errors.Is(err, usecases.ErrNotDownloadable) || len(bytes) != 0 {
		t.Fatal("old token served revoked evidence", err)
	}
	if _, err := f.service.AuthorizeDownload(context.Background(), caller, id); !errors.Is(err, usecases.ErrNotDownloadable) {
		t.Fatal("new revoked grant minted", err)
	}
	if (models.Object{UploadStatus: models.UploadCompleted, DeletionState: models.DeletionActive, SafetyStatus: models.SafetySafe, EvidenceState: models.EvidenceRevoked}).Downloadable() {
		t.Fatal("revoked evidence considered downloadable")
	}
}
