package integration_test

// Encryption at rest through the real private HTTP surface (FLEET-7h.c):
// a Fleet-style upload is completed, read back through a short-lived content
// grant, and the object the storage backend holds is ciphertext.

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	uploadclients "cosmicforge/logistics/services/media-file-service/internal/features/uploads/clients"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestPrivateObjectsAreEncryptedAtRestAndServedDecrypted(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{
		scanner: cleanScanner{},
		encryptionKeys: []uploadclients.EncryptionKey{
			{ID: "it-k1", Key: bytes.Repeat([]byte{0x5a}, 32)},
		},
	})
	_, response := f.authorizeUpload(t, "encrypted-at-rest", nil)
	grant := dataOf(t, response)
	content := pdfFile(256)
	if status, body := f.completeUpload(t, grant, content, nil); status != http.StatusOK {
		t.Fatalf("completion status = %d body = %v", status, body)
	}
	objectID := grant["mediaObjectId"].(string)

	var key string
	var size int64
	if err := f.pool.QueryRow(context.Background(),
		`SELECT storage_object_key, size_bytes FROM media_objects WHERE id = $1`, objectID).Scan(&key, &size); err != nil {
		t.Fatalf("read object: %v", err)
	}
	if size != int64(len(content)) {
		t.Fatalf("recorded size = %d, want plaintext size %d", size, len(content))
	}

	// The raw backend holds ciphertext, never the uploaded bytes.
	atRest, err := f.storage.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("read raw object: %v", err)
	}
	if bytes.Contains(atRest, []byte("%PDF")) || bytes.Contains(atRest, content[:32]) {
		t.Fatal("object at rest contains plaintext")
	}
	if !bytes.HasPrefix(atRest, []byte{0x89, 'K', 'G', 'M', 0x01}) {
		t.Fatal("object at rest lacks the encryption header")
	}

	// A content grant serves the decrypted original.
	status, response := f.call(t, callOptions{
		method: http.MethodPost, path: "/objects/" + objectID + "/download-authorizations",
	})
	if status != http.StatusOK {
		t.Fatalf("download authorization status = %d body = %v", status, response)
	}
	token, _ := dataOf(t, response)["downloadToken"].(string)
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/media-files/internal/media/objects/"+objectID+"/content", nil)
	request.Header.Set("X-Organization-ID", testOrgID)
	request.Header.Set("X-Service-Nonce", "encrypted-content-nonce")
	request.Header.Set("X-Download-Token", token)
	if err := serviceauth.SignRequest(request, testCaller, []byte(testSecret), nil, time.Time{}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("content status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Equal(recorder.Body.Bytes(), content) {
		t.Fatal("served content does not match the uploaded plaintext")
	}
}
