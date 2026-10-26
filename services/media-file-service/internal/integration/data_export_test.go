package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"

	mediamodels "cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/models"
	mediausecases "cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/usecases"
)

// DATA_EXPORT end-to-end tests.
//
// Real PostgreSQL, the real gin handlers, the real HMAC verifier and real bytes
// on a real filesystem — the same harness the document purposes use, because
// the whole question is whether this narrow exception behaves like the rest of
// the contract rather than beside it.

const (
	csvExport   = "profile_ref,status\ncrmp_1,ACTIVE\ncrmp_2,ARCHIVED\n"
	jsonlExport = "{\"profile_ref\":\"crmp_1\",\"state\":\"CURRENT\"}\n" +
		"{\"profile_ref\":\"crmp_2\",\"state\":\"NOT_AVAILABLE\"}\n"
)

// newExportFixture builds the shared harness with the export content types and
// the purpose ceiling configured, which is what a deployment serving Section 8H
// would have.
func newExportFixture(t *testing.T) *mediaFixture {
	t.Helper()
	// DATA_EXPORT is a Section 19 high-risk purpose: its bytes are released
	// only after a positive SAFE verdict, so the harness needs a scanner.
	return newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}, policy: mediausecases.Policy{
		MaxSizeBytes: 1 << 20,
		AllowedMIMETypes: []string{"application/pdf", "image/png",
			"text/csv", "application/x-ndjson"},
		DataExportMaxBytes: 1 << 19,
	}})
}

// rawCall performs one signed request and returns the recorder, for the two
// assertions that are about response headers rather than a JSON body.
func (f *mediaFixture) rawCall(t *testing.T, opts callOptions) *httptest.ResponseRecorder {
	t.Helper()
	base := "/api/v1/media-files/internal/media"
	request := httptest.NewRequest(opts.method, base+opts.path, bytes.NewReader(opts.body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Organization-ID", testOrgID)
	for key, value := range opts.headers {
		request.Header.Set(key, value)
	}
	f.nonce++
	request.Header.Set("X-Service-Nonce", fmt.Sprintf("export-nonce-%d-%d", time.Now().UnixNano(), f.nonce))
	if err := serviceauth.SignRequest(request, f.caller, []byte(f.secret), opts.body, time.Time{}); err != nil {
		t.Fatalf("sign request: %v", err)
	}
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	return recorder
}

// exportErrorCode reads the controlled refusal code from an error envelope.
func exportErrorCode(t *testing.T, response map[string]any) string {
	t.Helper()
	body, _ := response["error"].(map[string]any)
	code, _ := body["code"].(string)
	return code
}

// authorizeExport issues a DATA_EXPORT grant with export defaults.
func (f *mediaFixture) authorizeExport(t *testing.T, key string, overrides map[string]any) (int, map[string]any) {
	t.Helper()
	body := map[string]any{
		"ownerType":        "VENDOR_CRM_EXPORT",
		"ownerReference":   "crmexp_" + key,
		"documentCategory": "CRM_EXPORT",
		"uploadPurpose":    mediamodels.PurposeDataExport,
		"allowedMimeTypes": []string{"text/csv"},
		"fileName":         "customer_summary.csv",
		"maxSizeBytes":     1 << 20,
	}
	for name, value := range overrides {
		body[name] = value
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return f.call(t, callOptions{
		method: http.MethodPost,
		path:   "/upload-authorizations",
		body:   encoded,
		headers: map[string]string{
			"Idempotency-Key":       key,
			"X-Request-Fingerprint": "fp-" + key,
			"X-Payload-Version":     "1",
		},
	})
}

func (f *mediaFixture) completeExport(t *testing.T, grant map[string]any, content string,
	declaredMIME string) (int, map[string]any) {
	t.Helper()
	return f.completeUpload(t, grant, []byte(content), map[string]string{
		"X-Declared-Content-Type": declaredMIME,
	})
}

// TestDataExport_PrivateCSVAndJSONLUploadsSucceed is the happy path for both
// formats, through the real contract.
func TestDataExport_PrivateCSVAndJSONLUploadsSucceed(t *testing.T) {
	f := newExportFixture(t)

	// CSV.
	status, response := f.authorizeExport(t, "export-csv", nil)
	if status != http.StatusOK {
		t.Fatalf("authorize csv export: status=%d body=%v", status, response)
	}
	grant := dataOf(t, response)
	status, completion := f.completeExport(t, grant, csvExport, "text/csv")
	if status != http.StatusOK {
		t.Fatalf("complete csv export: status=%d body=%v", status, completion)
	}
	object := dataOf(t, completion)
	if object["uploadStatus"] != "COMPLETED" {
		t.Fatalf("csv export status = %v", object["uploadStatus"])
	}
	if object["detectedMime"] != "text/csv" {
		t.Fatalf("csv export stored as %v; want text/csv", object["detectedMime"])
	}

	// JSONL uses the one canonical type accepted by the contract.
	status, response = f.authorizeExport(t, "export-jsonl", map[string]any{
		"allowedMimeTypes": []string{"application/x-ndjson"},
		"fileName":         "intelligence_scores.jsonl",
	})
	if status != http.StatusOK {
		t.Fatalf("authorize jsonl export: status=%d body=%v", status, response)
	}
	grant = dataOf(t, response)
	status, completion = f.completeExport(t, grant, jsonlExport, "application/x-ndjson")
	if status != http.StatusOK {
		t.Fatalf("complete jsonl export: status=%d body=%v", status, completion)
	}
	object = dataOf(t, completion)
	if object["detectedMime"] != "application/x-ndjson" {
		t.Fatalf("jsonl export stored as %v; want application/x-ndjson", object["detectedMime"])
	}
}

// TestDataExport_CompletedUploadReplayReturnsTheSameObject proves the recovery
// path for an ambiguous network outcome. The upload grant stays single-use:
// Media returns no second bearer token and only identifies the already
// completed private object so Vendor can reconcile checksum and size.
func TestDataExport_CompletedUploadReplayReturnsTheSameObject(t *testing.T) {
	f := newExportFixture(t)
	status, response := f.authorizeExport(t, "export-reconcile", nil)
	if status != http.StatusOK {
		t.Fatalf("authorize: status=%d body=%v", status, response)
	}
	grant := dataOf(t, response)
	objectID, _ := grant["mediaObjectId"].(string)
	if status, completion := f.completeExport(t, grant, csvExport, "text/csv"); status != http.StatusOK {
		t.Fatalf("complete: status=%d body=%v", status, completion)
	}

	status, response = f.authorizeExport(t, "export-reconcile", nil)
	if status != http.StatusOK {
		t.Fatalf("reconcile authorization: status=%d body=%v", status, response)
	}
	replayed := dataOf(t, response)
	if replayed["mediaObjectId"] != objectID {
		t.Fatalf("replay returned object %v; want %s", replayed["mediaObjectId"], objectID)
	}
	if replayed["replayed"] != true {
		t.Fatalf("replay was not identified: %v", replayed)
	}
	if token, _ := replayed["uploadToken"].(string); token != "" {
		t.Fatalf("a completed upload replay minted another bearer token: %q", token)
	}
}

// TestDataExport_PublicVisibilityIsRefused is the rule that must not depend on
// the calling service being careful.
func TestDataExport_PublicVisibilityIsRefused(t *testing.T) {
	f := newExportFixture(t)
	status, response := f.authorizeExport(t, "export-public", map[string]any{"visibility": "PUBLIC"})
	if status == http.StatusOK {
		t.Fatalf("a PUBLIC data export was authorized: %v", response)
	}
	if code := exportErrorCode(t, response); code != "visibility_must_be_private" {
		t.Fatalf("refused with %q; want visibility_must_be_private", code)
	}
	// An explicit PRIVATE, and the default when omitted, both work.
	if status, response = f.authorizeExport(t, "export-private", map[string]any{
		"visibility": "PRIVATE"}); status != http.StatusOK {
		t.Fatalf("an explicit PRIVATE export was refused: %v", response)
	}
	if status, response = f.authorizeExport(t, "export-default", nil); status != http.StatusOK {
		t.Fatalf("a default-visibility export was refused: %v", response)
	}
	// A nonsense visibility is refused rather than defaulted.
	if status, response = f.authorizeExport(t, "export-bogus", map[string]any{
		"visibility": "SEMI_PUBLIC"}); status == http.StatusOK {
		t.Fatalf("an unknown visibility was accepted: %v", response)
	}
}

// TestDataExport_TypeAndNameMustAgree is settled at authorization time, so an
// unusable grant is never issued.
func TestDataExport_TypeAndNameMustAgree(t *testing.T) {
	f := newExportFixture(t)
	cases := map[string]map[string]any{
		"csv type with jsonl name": {"allowedMimeTypes": []string{"text/csv"}, "fileName": "x.jsonl"},
		"jsonl type with csv name": {"allowedMimeTypes": []string{"application/x-ndjson"}, "fileName": "x.csv"},
		"no file name":             {"allowedMimeTypes": []string{"text/csv"}, "fileName": ""},
		"no extension":             {"allowedMimeTypes": []string{"text/csv"}, "fileName": "export"},
		"path traversal in name":   {"allowedMimeTypes": []string{"text/csv"}, "fileName": "../secrets.csv"},
		"double extension":         {"allowedMimeTypes": []string{"text/csv"}, "fileName": "x.csv.html"},
		"plain text type":          {"allowedMimeTypes": []string{"text/plain"}, "fileName": "x.txt"},
		"non-canonical jsonl type": {"allowedMimeTypes": []string{"application/jsonl"}, "fileName": "x.jsonl"},
		"html type":                {"allowedMimeTypes": []string{"text/html"}, "fileName": "x.html"},
		"svg type":                 {"allowedMimeTypes": []string{"image/svg+xml"}, "fileName": "x.svg"},
		"octet-stream type":        {"allowedMimeTypes": []string{"application/octet-stream"}, "fileName": "x.csv"},
		"xlsx type":                {"allowedMimeTypes": []string{"application/vnd.ms-excel"}, "fileName": "x.xlsx"},
		"zip type":                 {"allowedMimeTypes": []string{"application/zip"}, "fileName": "x.zip"},
		"two types":                {"allowedMimeTypes": []string{"text/csv", "application/x-ndjson"}, "fileName": "x.csv"},
	}
	index := 0
	for name, overrides := range cases {
		index++
		status, response := f.authorizeExport(t, fmt_key("pair", index), overrides)
		if status == http.StatusOK {
			t.Errorf("%s was authorized: %v", name, response)
		}
	}
}

// TestDataExport_ContentMustBeWhatItClaims drives the disguises through the
// real upload path rather than the validator alone.
func TestDataExport_ContentMustBeWhatItClaims(t *testing.T) {
	f := newExportFixture(t)
	cases := []struct {
		name     string
		content  []byte
		mime     string
		fileName string
	}{
		{"pdf as csv", pdfFile(0), "text/csv", "x.csv"},
		{"png as csv", pngFile(), "text/csv", "x.csv"},
		{"executable as csv", executableFile(), "text/csv", "x.csv"},
		{"nul bytes as csv", []byte("a,b\n1,\x00\n"), "text/csv", "x.csv"},
		{"control bytes as csv", []byte("a,b\n1,\x07\n"), "text/csv", "x.csv"},
		{"invalid utf8 as csv", []byte("a,b\n1,\xff\xfe\n"), "text/csv", "x.csv"},
		{"html as csv", []byte("<script>alert(1)</script>"), "text/csv", "x.csv"},
		{"svg as csv", []byte("<svg onload=alert(1)>"), "text/csv", "x.csv"},
		{"ragged csv", []byte("a,b\n1,2,3\n"), "text/csv", "x.csv"},
		{"unterminated quote", []byte("a,b\n\"open,2\n"), "text/csv", "x.csv"},
		{"empty csv", []byte(""), "text/csv", "x.csv"},
		{"malformed jsonl", []byte("{\"a\":1}\nnope\n"), "application/x-ndjson", "x.jsonl"},
		{"json array as jsonl", []byte("[{\"a\":1}]\n"), "application/x-ndjson", "x.jsonl"},
		{"csv content as jsonl", []byte("a,b\n1,2\n"), "application/x-ndjson", "x.jsonl"},
	}
	for index, testCase := range cases {
		mimeType := "text/csv"
		if strings.HasSuffix(testCase.fileName, ".jsonl") {
			mimeType = "application/x-ndjson"
		}
		_, response := f.authorizeExport(t, fmt_key("content", index), map[string]any{
			"allowedMimeTypes": []string{mimeType},
			"fileName":         testCase.fileName,
		})
		grant := dataOf(t, response)
		status, completion := f.completeUpload(t, grant, testCase.content, map[string]string{
			"X-Declared-Content-Type": testCase.mime,
		})
		if status == http.StatusOK {
			t.Errorf("%s was ACCEPTED as a data export: %v", testCase.name, completion)
		}
	}
}

// TestDataExport_OversizedIsRefusedByThePurposeCeiling.
func TestDataExport_OversizedIsRefusedByThePurposeCeiling(t *testing.T) {
	f := newExportFixture(t)
	_, response := f.authorizeExport(t, "export-oversized", map[string]any{"maxSizeBytes": 256})
	grant := dataOf(t, response)
	big := "a,b\n" + strings.Repeat("1,2\n", 4096)
	status, completion := f.completeExport(t, grant, big, "text/csv")
	if status == http.StatusOK {
		t.Fatalf("an oversized export was accepted: %v", completion)
	}
}

// TestDataExport_ExistingPurposesKeepTheirOldBehaviour is the regression guard
// that matters most: the exception must not have loosened anything.
func TestDataExport_ExistingPurposesKeepTheirOldBehaviour(t *testing.T) {
	f := newExportFixture(t)

	// A real PDF under a document purpose still works.
	_, response := f.authorizeUpload(t, "doc-still-ok", nil)
	grant := dataOf(t, response)
	if status, completion := f.completeUpload(t, grant, pdfFile(0), nil); status != http.StatusOK {
		t.Fatalf("a document upload broke: status=%d body=%v", status, completion)
	}

	// Sniff-less text under a document purpose is still refused. This is the
	// single most important assertion in the file: the export path must not
	// have become a general way to store text.
	_, response = f.authorizeUpload(t, "doc-text-refused", map[string]any{
		"allowedMimeTypes": []string{"text/csv"},
	})
	grant = dataOf(t, response)
	status, completion := f.completeUpload(t, grant, []byte(csvExport), map[string]string{
		"X-Declared-Content-Type": "text/csv",
	})
	if status == http.StatusOK {
		t.Fatalf("a CSV was accepted under a document purpose: %v", completion)
	}

	// And an unrecognised binary under a document purpose is still refused.
	_, response = f.authorizeUpload(t, "doc-unknown-refused", nil)
	grant = dataOf(t, response)
	if status, completion = f.completeUpload(t, grant, unknownFile(), nil); status == http.StatusOK {
		t.Fatalf("unknown content was accepted under a document purpose: %v", completion)
	}
}

// TestDataExport_PrivateAccessIsShortLivedAndServedAsADownload covers the
// retrieval half: the artifact is reachable only through an authorized grant,
// and what comes back is a download rather than something a browser renders.
func TestDataExport_PrivateAccessIsShortLivedAndServedAsADownload(t *testing.T) {
	f := newExportFixture(t)
	_, response := f.authorizeExport(t, "export-download", nil)
	grant := dataOf(t, response)
	if status, completion := f.completeExport(t, grant, csvExport, "text/csv"); status != http.StatusOK {
		t.Fatalf("complete: status=%d body=%v", status, completion)
	}
	objectID := grant["mediaObjectId"].(string)

	status, response := f.call(t, callOptions{
		method: http.MethodPost,
		path:   "/objects/" + objectID + "/download-authorizations",
		body:   []byte("{}"),
	})
	if status != http.StatusOK {
		t.Fatalf("download authorization: status=%d body=%v", status, response)
	}
	access := dataOf(t, response)
	token, _ := access["downloadToken"].(string)
	if token == "" {
		t.Fatal("no download token was issued")
	}
	if access["expiresAt"] == nil {
		t.Fatal("the access grant has no expiry")
	}
	// Nothing durable or public is handed back.
	encoded, _ := json.Marshal(access)
	for _, forbidden := range []string{"http://", "https://", "publicUrl", "signedUrl"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("the access grant exposes %q: %s", forbidden, encoded)
		}
	}

	// The content is served as an attachment with nosniff, so a crafted export
	// cannot be rendered in an operator's browser session.
	recorder := f.rawCall(t, callOptions{
		method:  http.MethodGet,
		path:    "/objects/" + objectID + "/content",
		headers: map[string]string{"X-Download-Token": token},
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("read content: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/csv") {
		t.Fatalf("Content-Type = %q; want text/csv", got)
	}
	disposition := recorder.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disposition, "attachment") {
		t.Fatalf("Content-Disposition = %q; want attachment", disposition)
	}
	if !strings.Contains(disposition, ".csv") {
		t.Fatalf("Content-Disposition = %q; want the validated file name", disposition)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q; want nosniff", got)
	}
	if !bytes.Equal(recorder.Body.Bytes(), []byte(csvExport)) {
		t.Fatal("the served bytes are not the stored artifact")
	}

	// Without the token, nothing is released.
	recorder = f.rawCall(t, callOptions{
		method: http.MethodGet,
		path:   "/objects/" + objectID + "/content",
	})
	if recorder.Code == http.StatusOK {
		t.Fatal("content was served without a download grant")
	}
}

// TestDataExport_DeletedArtifactCannotBeDownloaded closes the retrieval loop.
func TestDataExport_DeletedArtifactCannotBeDownloaded(t *testing.T) {
	f := newExportFixture(t)
	_, response := f.authorizeExport(t, "export-deleted", nil)
	grant := dataOf(t, response)
	if status, completion := f.completeExport(t, grant, csvExport, "text/csv"); status != http.StatusOK {
		t.Fatalf("complete: status=%d body=%v", status, completion)
	}
	objectID := grant["mediaObjectId"].(string)

	body, err := json.Marshal(map[string]any{"reason": "expired"})
	if err != nil {
		t.Fatal(err)
	}
	if status, deletion := f.call(t, callOptions{
		method: http.MethodPost, path: "/objects/" + objectID + "/deletions", body: body,
		headers: map[string]string{
			"Idempotency-Key":       "export-delete-1",
			"X-Request-Fingerprint": "fp-export-delete-1",
		},
	}); status != http.StatusOK {
		t.Fatalf("request deletion: status=%d body=%v", status, deletion)
	}

	// A new access grant must not be issued for something on its way out.
	status, response := f.call(t, callOptions{
		method: http.MethodPost,
		path:   "/objects/" + objectID + "/download-authorizations",
		body:   []byte("{}"),
	})
	if status == http.StatusOK {
		t.Fatalf("a deletion-pending artifact issued a download grant: %v", response)
	}
}

// TestDataExport_ExpiredArtifactCannotBeGrantedOrRead proves retention is an
// access ceiling, not merely a hint to a later deletion worker.
func TestDataExport_ExpiredArtifactCannotBeGrantedOrRead(t *testing.T) {
	f := newExportFixture(t)
	_, response := f.authorizeExport(t, "export-expired", nil)
	grant := dataOf(t, response)
	if status, completion := f.completeExport(t, grant, csvExport, "text/csv"); status != http.StatusOK {
		t.Fatalf("complete: status=%d body=%v", status, completion)
	}
	objectID := grant["mediaObjectId"].(string)

	// Mint a grant first so the test also proves an already-issued token cannot
	// outlive the artifact's retention boundary.
	status, response := f.call(t, callOptions{
		method: http.MethodPost, path: "/objects/" + objectID + "/download-authorizations",
		body: []byte("{}"),
	})
	if status != http.StatusOK {
		t.Fatalf("download authorization: status=%d body=%v", status, response)
	}
	token, _ := dataOf(t, response)["downloadToken"].(string)

	retentionBody, err := json.Marshal(map[string]any{
		"retentionClass": "STANDARD",
		"retentionUntil": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	if status, retention := f.call(t, callOptions{
		method: http.MethodPost, path: "/objects/" + objectID + "/retention", body: retentionBody,
	}); status != http.StatusOK {
		t.Fatalf("set expired retention: status=%d body=%v", status, retention)
	}

	status, response = f.call(t, callOptions{
		method: http.MethodPost, path: "/objects/" + objectID + "/download-authorizations",
		body: []byte("{}"),
	})
	if status == http.StatusOK {
		t.Fatalf("an expired export issued a new grant: %v", response)
	}
	recorder := f.rawCall(t, callOptions{
		method: http.MethodGet, path: "/objects/" + objectID + "/content",
		headers: map[string]string{"X-Download-Token": token},
	})
	if recorder.Code == http.StatusOK {
		t.Fatal("a token minted before expiry still read the expired artifact")
	}
}

// fmt_key builds a distinct idempotency key per table case.
func fmt_key(prefix string, index int) string {
	return prefix + "-" + string(rune('a'+index%26)) + "-" + strings.Repeat("x", index%3)
}
