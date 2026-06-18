package mediaclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadSendsMultipartServiceAuthAndParsesAsset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/uploads" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Karrygo-Service") != "customer-service" {
			t.Fatalf("missing service header")
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("missing bearer token")
		}
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		if r.FormValue("owner_service") != "customer-service" || r.FormValue("owner_id") != "customer-1" {
			t.Fatalf("unexpected form fields: %+v", r.Form)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"id":                  "asset-1",
				"owner_service":       "customer-service",
				"owner_id":            "customer-1",
				"purpose":             "profile_photo",
				"original_filename":   "avatar.jpg",
				"content_type":        "image/jpeg",
				"size_bytes":          12,
				"checksum_sha256":     "checksum",
				"bucket":              "bucket",
				"path":                "media/customer-service/profile_photo/customer-1/asset-1/avatar.jpg",
				"url":                 "https://storage.googleapis.com/bucket/media/customer-service/profile_photo/customer-1/asset-1/avatar.jpg",
				"status":              "active",
				"uploaded_by_service": "customer-service",
			},
		})
	}))
	defer server.Close()

	client := New(Config{
		BaseURL:     server.URL,
		ServiceName: "customer-service",
		Token:       "token",
	})

	asset, err := client.Upload(context.Background(), UploadRequest{
		OwnerID:     "customer-1",
		Purpose:     "profile_photo",
		Filename:    "avatar.jpg",
		ContentType: "image/jpeg",
		Body:        strings.NewReader("file-bytes"),
	})
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if asset.ID != "asset-1" || asset.PublicURL == "" {
		t.Fatalf("unexpected asset: %+v", asset)
	}
}

func TestGetByIDReturnsServiceError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error": map[string]interface{}{
				"code":    "not_found",
				"message": "Media file could not be found.",
			},
		})
	}))
	defer server.Close()

	client := New(Config{
		BaseURL:     server.URL,
		ServiceName: "customer-service",
		Token:       "token",
	})

	_, err := client.GetByID(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	mediaErr, ok := err.(Error)
	if !ok || mediaErr.StatusCode != http.StatusNotFound || mediaErr.Problem.Code != "not_found" {
		t.Fatalf("unexpected error: %v", err)
	}
}
