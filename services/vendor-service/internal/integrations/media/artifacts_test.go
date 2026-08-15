package media

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/crm"
)

func TestArtifactStorePutPersistsPrivateExportAndRetention(t *testing.T) {
	content := []byte("profile_ref,status\ncrmp_1,ACTIVE\n")
	wantChecksum := sha256Hex(content)
	retainUntil := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	var contentCalls int
	var retentionUntil string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/upload-authorizations"):
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode authorization: %v", err)
			}
			if body["uploadPurpose"] != purposeDataExport || body["visibility"] != "PRIVATE" ||
				body["fileName"] != "customer-summary.csv" {
				t.Errorf("unsafe authorization body: %v", body)
			}
			writeJSON(t, w, map[string]any{"success": true, "data": map[string]any{
				"uploadAuthorizationId": "mua_1", "mediaObjectId": "mo_1",
				"uploadToken": "upload-token", "replayed": false,
			}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/objects/mo_1/content"):
			contentCalls++
			received, _ := io.ReadAll(r.Body)
			if string(received) != string(content) ||
				r.Header.Get("X-Declared-Checksum-SHA256") != wantChecksum {
				t.Errorf("uploaded bytes or checksum differ")
			}
			writeObject(t, w, "mo_1", int64(len(content)), wantChecksum)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/objects/mo_1/retention"):
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode retention: %v", err)
			}
			retentionUntil, _ = body["retentionUntil"].(string)
			writeJSON(t, w, map[string]any{"success": true, "data": map[string]any{"mediaObjectId": "mo_1"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	store := NewArtifactStore(server.URL, "test-secret", time.Second, time.Minute)
	stored, err := store.Put(t.Context(), crm.PutArtifactInput{
		BusinessID: "vb_1", ExportID: "crmexp_1", Filename: "customer-summary.csv",
		MediaType: "text/csv", Content: content, IdempotencyKey: "crm-export-crmexp_1",
		RetainUntil: retainUntil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stored.Ref != "mo_1" || stored.Bytes != int64(len(content)) || stored.Checksum != wantChecksum {
		t.Fatalf("stored artifact = %+v", stored)
	}
	if contentCalls != 1 {
		t.Fatalf("content upload calls = %d; want 1", contentCalls)
	}
	if retentionUntil != retainUntil.Format(time.RFC3339) {
		t.Fatalf("retentionUntil = %q; want %q", retentionUntil, retainUntil.Format(time.RFC3339))
	}
}

func TestArtifactStorePutReconcilesCompletedIdempotentUpload(t *testing.T) {
	content := []byte("{\"profile_ref\":\"crmp_1\"}\n")
	wantChecksum := sha256Hex(content)
	var contentCalls int
	var objectReads int
	var retentionCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/upload-authorizations"):
			writeJSON(t, w, map[string]any{"success": true, "data": map[string]any{
				"uploadAuthorizationId": "mua_1", "mediaObjectId": "mo_existing",
				"uploadToken": "", "replayed": true,
			}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/content"):
			contentCalls++
			http.Error(w, "upload grant was already consumed", http.StatusGone)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/objects/mo_existing"):
			objectReads++
			writeObject(t, w, "mo_existing", int64(len(content)), wantChecksum)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/objects/mo_existing/retention"):
			retentionCalls++
			writeJSON(t, w, map[string]any{"success": true, "data": map[string]any{"mediaObjectId": "mo_existing"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	store := NewArtifactStore(server.URL, "test-secret", time.Second, time.Minute)
	stored, err := store.Put(t.Context(), crm.PutArtifactInput{
		BusinessID: "vb_1", ExportID: "crmexp_1", Filename: "scores.jsonl",
		MediaType: "application/x-ndjson", Content: content,
		IdempotencyKey: "crm-export-crmexp_1", RetainUntil: time.Now().UTC().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if stored.Ref != "mo_existing" || stored.Checksum != wantChecksum {
		t.Fatalf("reconciled artifact = %+v", stored)
	}
	if contentCalls != 0 || objectReads != 1 || retentionCalls != 1 {
		t.Fatalf("calls: upload=%d object=%d retention=%d", contentCalls, objectReads, retentionCalls)
	}
}

func TestArtifactStoreDeleteUsesMediaIdempotencyContract(t *testing.T) {
	var retentionCalls int
	var deletionCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/objects/mo_1/retention"):
			retentionCalls++
			writeJSON(t, w, map[string]any{"success": true, "data": map[string]any{"mediaObjectId": "mo_1"}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/objects/mo_1/deletions"):
			deletionCalls++
			if r.Header.Get("Idempotency-Key") == "" || r.Header.Get("X-Request-Fingerprint") == "" {
				t.Error("deletion omitted Media idempotency headers")
			}
			writeJSON(t, w, map[string]any{"success": true, "data": map[string]any{
				"status": "COMPLETED", "blockReason": "",
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	store := NewArtifactStore(server.URL, "test-secret", time.Second, time.Minute)
	if err := store.Delete(t.Context(), "vb_1", "mo_1", "expired"); err != nil {
		t.Fatal(err)
	}
	if retentionCalls != 1 || deletionCalls != 1 {
		t.Fatalf("calls: retention=%d deletion=%d", retentionCalls, deletionCalls)
	}
}

func writeObject(t *testing.T, w http.ResponseWriter, id string, size int64, checksum string) {
	t.Helper()
	writeJSON(t, w, map[string]any{"success": true, "data": map[string]any{
		"mediaObjectId": id, "uploadPurpose": purposeDataExport, "visibility": "PRIVATE",
		"detectedMime": func() string {
			if strings.Contains(id, "existing") {
				return "application/x-ndjson"
			}
			return "text/csv"
		}(),
		"sizeBytes": size, "checksumSha256": checksum,
		"uploadStatus": "COMPLETED", "deletionState": "ACTIVE",
	}})
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encode response: %v", err)
	}
}
