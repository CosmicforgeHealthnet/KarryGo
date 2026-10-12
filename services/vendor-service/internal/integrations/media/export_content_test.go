package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExportContentReadBoundsAndRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Organization-ID") != "business" || r.Header.Get("X-Download-Token") != "token" || r.Header.Get("X-Service-Nonce") == "" {
			t.Error("missing scoped private access headers")
		}
		if r.URL.Path == privateBase+"/objects/redirect/content" {
			http.Redirect(w, r, "/elsewhere", http.StatusTemporaryRedirect)
			return
		}
		if r.URL.Path == "/elsewhere" {
			t.Error("private credentials followed redirect")
		}
		w.Write([]byte("rows"))
	}))
	defer server.Close()
	store := NewArtifactStore(server.URL, "secret", time.Second, time.Minute)
	content, err := store.ReadExportContent(context.Background(), "business", "file", "token", 4)
	if err != nil || string(content) != "rows" {
		t.Fatalf("valid read failed: %v", err)
	}
	if _, err = store.ReadExportContent(context.Background(), "business", "file", "token", 3); err == nil {
		t.Fatal("oversized content accepted")
	}
	if _, err = store.ReadExportContent(context.Background(), "business", "redirect", "token", 4); err == nil {
		t.Fatal("redirect accepted")
	}
}
