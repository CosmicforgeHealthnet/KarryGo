package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// rowsCall performs one JSON request against a router and returns the status
// and the decoded envelope (data/meta on success, error on failure). It is the
// shared request helper for the requirement-row suites (rows_*_test.go).
func rowsCall(t *testing.T, router http.Handler, method, path string, body any,
	headers map[string]string) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	envelope := map[string]any{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &envelope)
	return recorder.Code, envelope
}

// rowsData unwraps the success envelope's "data" object.
func rowsData(t *testing.T, envelope map[string]any) map[string]any {
	t.Helper()
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("response has no data object: %v", envelope)
	}
	return data
}

// rowsList unwraps the success envelope's "data" array.
func rowsList(t *testing.T, envelope map[string]any) []any {
	t.Helper()
	data, ok := envelope["data"].([]any)
	if !ok {
		t.Fatalf("response has no data array: %v", envelope)
	}
	return data
}

// rowsErrorCode returns error.code from an error envelope.
func rowsErrorCode(envelope map[string]any) string {
	errBody, _ := envelope["error"].(map[string]any)
	code, _ := errBody["code"].(string)
	return code
}

// rowsCount runs a count(*) query.
func rowsCount(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}
