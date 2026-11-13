package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
)

// Minimal valid headers for each accepted company-document format.
var previewFixtures = map[string]struct {
	documentType string
	mime         string
	content      []byte
}{
	"pdf":  {"CERTIFICATE_OF_INCORPORATION", "application/pdf", []byte("%PDF-1.4\n1 0 obj<</Type/Catalog>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF")},
	"jpeg": {"TAX_CLEARANCE", "image/jpeg", append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, []byte("jpeg-fixture")...)},
	"png":  {"INSURANCE_POLICY", "image/png", append([]byte("\x89PNG\r\n\x1a\n"), []byte("png-fixture")...)},
	"webp": {"OPERATING_LICENSE", "image/webp", []byte("RIFF\x10\x00\x00\x00WEBPVP8 webp-fixture")},
}

func uploadOnboardingDocument(t *testing.T, router *gin.Engine, token, orgID, documentType, mime string, content []byte) (documentID string, fileID string) {
	t.Helper()
	base := "/organizations/" + orgID + "/documents"
	headers := map[string]string{"Authorization": "Bearer " + token, "Origin": "https://fleet.example.test", "Idempotency-Key": "preview-" + documentType + strconv.Itoa(len(content))}
	code, body := documentCall(t, router, "POST", "/fleet-partner/v1"+base+"/upload-authorization", map[string]any{"documentType": documentType}, headers)
	if code != 200 {
		t.Fatalf("upload authorization %d: %v", code, body)
	}
	grant := documentData(t, body)
	sum := sha256.Sum256(content)
	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	for key, value := range map[string]string{
		"mediaObjectId": grant["mediaObjectId"].(string), "authorizationId": grant["authorizationId"].(string),
		"uploadToken": grant["uploadToken"].(string), "declaredMime": mime,
		"declaredSize": strconv.Itoa(len(content)), "declaredChecksum": hex.EncodeToString(sum[:]),
	} {
		_ = writer.WriteField(key, value)
	}
	part, _ := writer.CreateFormFile("file", "fixture")
	_, _ = part.Write(content)
	_ = writer.Close()
	request := httptest.NewRequest("POST", "/fleet-partner/v1"+base+"/upload-content", &form)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Origin", "https://fleet.example.test")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != 200 {
		t.Fatalf("upload content %d: %s", recorder.Code, recorder.Body.String())
	}
	fileID = grant["mediaObjectId"].(string)
	code, body = documentCall(t, router, "POST", "/fleet-partner/v1"+base, map[string]any{"documentType": documentType, "fileId": fileID}, headers)
	if code != 201 {
		t.Fatalf("attach %d: %v", code, body)
	}
	return documentData(t, body)["documentId"].(string), fileID
}

func previewRequest(router *gin.Engine, token, orgID, documentID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest("GET", "/fleet-partner/v1/organizations/"+orgID+"/documents/"+documentID+"/preview", nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	request.Header.Set("Origin", "https://fleet.example.test")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func signInExisting(t *testing.T, router *gin.Engine, h *authHarness, identifier string) string {
	t.Helper()
	if _, err := h.pool.Exec(context.Background(), `UPDATE fleet_partner_auth_otp_challenges SET resend_available_at=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	start := postJSON(router, "/fleet-partner/v1/auth/sign-in/start", map[string]string{"identifier": identifier}, nil)
	if start.Code != 202 {
		t.Fatal(start.Body.String())
	}
	otp, _ := h.sender.LastOTP()
	verified := postJSON(router, "/fleet-partner/v1/auth/verify", map[string]string{"challengeId": decodeEnvelope(t, start)["data"].(map[string]any)["challengeId"].(string), "code": otp.Code}, nil)
	if verified.Code != 200 {
		t.Fatal(verified.Body.String())
	}
	return decodeEnvelope(t, verified)["data"].(map[string]any)["tokens"].(map[string]any)["accessToken"].(string)
}

func TestOnboardingDocumentPreviewContract(t *testing.T) {
	var media *mediaTestServer
	router, h := buildOnboardingRouter(t, func(m *mediaTestServer) { media = m })
	ctx := context.Background()
	token := onboardingLogin(t, router, h, "preview-owner@example.com", "+2348012345611")
	company := map[string]any{"legalName": "Preview Company", "registrationNumber": "RC-PREVIEW-A", "organizationType": "PRIVATE_LIMITED", "countryCode": "NG", "timezone": "Africa/Lagos", "defaultCurrency": "NGN", "ownerName": "Preview Owner"}
	code, body := onboardCall(t, router, token, "POST", "/organizations", company, 0)
	if code != 201 {
		t.Fatal(body)
	}
	orgID := documentData(t, body)["organizationId"].(string)

	documents := map[string]string{}
	files := map[string]string{}
	for format, fixture := range previewFixtures {
		documents[format], files[format] = uploadOnboardingDocument(t, router, token, orgID, fixture.documentType, fixture.mime, fixture.content)
	}
	_, before := onboardCall(t, router, token, "GET", "/organizations/"+orgID+"/onboarding-status", nil, 0)

	// Every accepted format previews as its exact bytes and Media-detected type.
	for format, fixture := range previewFixtures {
		response := previewRequest(router, token, orgID, documents[format])
		if response.Code != 200 {
			t.Fatalf("%s preview %d: %s", format, response.Code, response.Body.String())
		}
		if got := response.Header().Get("Content-Type"); got != fixture.mime {
			t.Fatalf("%s content type %q", format, got)
		}
		if !bytes.Equal(response.Body.Bytes(), fixture.content) {
			t.Fatalf("%s preview returned different bytes", format)
		}
		headers := response.Header()
		if !strings.Contains(headers.Get("Cache-Control"), "no-store") || headers.Get("X-Content-Type-Options") != "nosniff" ||
			!strings.Contains(headers.Get("Content-Security-Policy"), "default-src 'none'") ||
			!strings.HasPrefix(headers.Get("Content-Disposition"), "attachment;") {
			t.Fatalf("%s unsafe response headers: %v", format, headers)
		}
		for key, values := range headers {
			for _, value := range values {
				if strings.Contains(strings.ToLower(key), "token") || strings.Contains(value, files[format]) || strings.Contains(value, media.URL()) {
					t.Fatalf("%s preview leaked a Media reference in %s", format, key)
				}
			}
		}
	}

	// Previewing is read-only: documents and onboarding progress are unchanged.
	_, listed := onboardCall(t, router, token, "GET", "/organizations/"+orgID+"/documents", nil, 0)
	if items := listed["data"].([]any); len(items) != 4 {
		t.Fatalf("document count changed: %v", listed)
	} else {
		for _, item := range items {
			if item.(map[string]any)["status"] != "READY" {
				t.Fatalf("document status changed: %v", item)
			}
		}
	}
	_, after := onboardCall(t, router, token, "GET", "/organizations/"+orgID+"/onboarding-status", nil, 0)
	beforeStages, _ := json.Marshal(documentData(t, before)["stages"])
	afterStages, _ := json.Marshal(documentData(t, after)["stages"])
	if !bytes.Equal(beforeStages, afterStages) {
		t.Fatalf("preview changed onboarding stages:\n%s\n%s", beforeStages, afterStages)
	}
	var audited int
	if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id=$1 AND action='organization.document_previewed'`, orgID).Scan(&audited); err != nil || audited != 4 {
		t.Fatalf("preview audit entries=%d err=%v", audited, err)
	}

	// No session, a malformed token, an invalid or unknown document id.
	if response := previewRequest(router, "", orgID, documents["pdf"]); response.Code != 401 {
		t.Fatalf("unauthenticated preview %d", response.Code)
	}
	if response := previewRequest(router, "not-a-token", orgID, documents["pdf"]); response.Code != 401 {
		t.Fatalf("invalid-token preview %d", response.Code)
	}
	for _, id := range []string{"not-a-document", "odoc_00000000000000000000000000000000", strings.Repeat("x", 300)} {
		if response := previewRequest(router, token, orgID, id); response.Code != 404 {
			t.Fatalf("unknown document %q preview %d", id, response.Code)
		}
	}

	// A different organization can reach neither the path nor the document.
	other := onboardingLogin(t, router, h, "preview-other@example.com", "+2348012345612")
	company["legalName"], company["registrationNumber"] = "Other Preview Company", "RC-PREVIEW-B"
	code, body = onboardCall(t, router, other, "POST", "/organizations", company, 0)
	if code != 201 {
		t.Fatal(body)
	}
	otherOrg := documentData(t, body)["organizationId"].(string)
	if response := previewRequest(router, other, orgID, documents["pdf"]); response.Code != 404 || bytes.Contains(response.Body.Bytes(), []byte("%PDF")) {
		t.Fatalf("cross-organization path preview %d", response.Code)
	}
	if response := previewRequest(router, other, otherOrg, documents["pdf"]); response.Code != 404 || bytes.Contains(response.Body.Bytes(), []byte("%PDF")) {
		t.Fatalf("cross-organization document preview %d", response.Code)
	}

	// A read-only member can preview but cannot change documents.
	onboardingLogin(t, router, h, "preview-reader@example.com", "+2348012345613")
	var readerIdentity string
	if err := h.pool.QueryRow(ctx, `SELECT id FROM fleet_partner_corporate_identities WHERE email_normalized='preview-reader@example.com'`).Scan(&readerIdentity); err != nil {
		t.Fatal(err)
	}
	if err := database.WithinTx(ctx, h.pool, func(tx pgx.Tx) error {
		member := &staffmodels.OrganizationUser{IdentityID: readerIdentity, Status: staffmodels.UserStatusActive}
		if err := staffrepos.NewMemberRepository(h.pool).Create(ctx, tx, orgID, member); err != nil {
			return err
		}
		return staffrepos.NewRoleRepository(h.pool).CreateBinding(ctx, tx, orgID, &staffmodels.RoleBinding{
			OrganizationUserID: member.ID, RoleID: "rol_system_operations",
			ScopeType: staffmodels.ScopeOrganization, GrantedBy: "test",
		})
	}); err != nil {
		t.Fatal(err)
	}
	readerEmail := "preview-reader@example.com"
	reader := signInExisting(t, router, h, readerEmail)
	if response := previewRequest(router, reader, orgID, documents["png"]); response.Code != 200 {
		t.Fatalf("read-only preview %d: %s", response.Code, response.Body.String())
	}
	if code, _ := onboardCall(t, router, reader, "DELETE", "/organizations/"+orgID+"/documents/"+documents["png"], nil, 0); code != 403 {
		t.Fatalf("read-only member removed a document: %d", code)
	}

	setObject := func(format string, change func(*mediaObjectRecord)) {
		media.mu.Lock()
		change(media.objects[files[format]])
		media.mu.Unlock()
	}
	expect := func(name, format string, want int) {
		t.Helper()
		response := previewRequest(router, token, orgID, documents[format])
		if response.Code != want {
			t.Fatalf("%s: preview %d want %d: %s", name, response.Code, want, response.Body.String())
		}
		if bytes.Equal(response.Body.Bytes(), previewFixtures[format].content) {
			t.Fatalf("%s: document bytes released", name)
		}
	}

	// Media's safety and lifecycle verdicts are never bypassed.
	setObject("webp", func(o *mediaObjectRecord) { o.SafetyStatus = "QUARANTINED" })
	expect("quarantined at Media", "webp", 409)
	setObject("webp", func(o *mediaObjectRecord) { o.SafetyStatus = "PENDING" })
	expect("scan not complete", "webp", 409)
	setObject("webp", func(o *mediaObjectRecord) { o.SafetyStatus = "SAFE" })
	media.mu.Lock()
	media.refuseDownload = true
	media.mu.Unlock()
	expect("Media refuses download", "webp", 409)
	media.mu.Lock()
	media.refuseDownload = false
	media.unavailable = true
	media.mu.Unlock()
	expect("Media unavailable", "webp", 503)
	media.mu.Lock()
	media.unavailable = false
	media.mu.Unlock()
	setObject("jpeg", func(o *mediaObjectRecord) { o.Content = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, []byte("tampered")...) })
	expect("integrity mismatch", "jpeg", 409)
	setObject("png", func(o *mediaObjectRecord) { o.DetectedMIME = "image/svg+xml" })
	expect("unsupported detected type", "png", 415)
	setObject("png", func(o *mediaObjectRecord) { o.DetectedMIME = "image/jpeg" })
	expect("bytes do not match detected type", "png", 415)
	setObject("pdf", func(o *mediaObjectRecord) { o.DeletionState = "DELETED" })
	expect("deleted at Media", "pdf", 404)

	// A removed attachment cannot be previewed even though Media still has it.
	setObject("webp", func(o *mediaObjectRecord) { o.SafetyStatus = "SAFE" })
	if response := previewRequest(router, token, orgID, documents["webp"]); response.Code != 200 {
		t.Fatalf("restored webp preview %d", response.Code)
	}
	if code, body := onboardCall(t, router, token, "DELETE", "/organizations/"+orgID+"/documents/"+documents["webp"], nil, 0); code != 200 {
		t.Fatal(body)
	}
	expect("removed attachment", "webp", 404)
}
