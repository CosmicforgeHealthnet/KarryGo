package idempotency

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// testRouter wires one endpoint through Execute to a handler that: echoes the
// request body back, counts how many times it actually ran (so a test can
// prove a replay does NOT re-execute it), and can be told to fail instead.
type testHandlerState struct {
	runs     int
	failWith error // when set, the handler calls httpx.Abort with this instead of succeeding
}

func newTestRouter(t *testing.T, store *Store, state *testHandlerState) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/customer/bookings", func(c *gin.Context) {
		scopeID := c.GetHeader("X-Test-Scope")
		Execute(c, store, scopeID, "POST /customer/bookings", func() {
			state.runs++
			if state.failWith != nil {
				httpx.Abort(c, state.failWith)
				return
			}
			var body map[string]any
			_ = c.ShouldBindJSON(&body)
			c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"echo": body, "run": state.runs}})
		})
	})
	return router
}

func doRequest(router *gin.Engine, method, path, idempotencyKey, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Scope", "customer-1")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestExecute_MissingKeyIsRejected(t *testing.T) {
	store := NewStore(testDB(t), time.Hour)
	router := newTestRouter(t, store, &testHandlerState{})

	rec := doRequest(router, "POST", "/customer/bookings", "", `{"pickup":"a"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without an Idempotency-Key, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestExecute_FirstCallRunsTheHandlerAndReturnsItsResponse(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, time.Hour)
	key := uuid.NewString()
	cleanupKey(t, db, "customer-1", "POST /customer/bookings", key)
	state := &testHandlerState{}
	router := newTestRouter(t, store, state)

	rec := doRequest(router, "POST", "/customer/bookings", key, `{"pickup":"a"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if state.runs != 1 {
		t.Fatalf("expected the handler to run exactly once, got %d", state.runs)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body was not valid JSON: %v (%s)", err, rec.Body.String())
	}
}

// The core end-to-end guarantee from the addendum: a mobile client that
// retries the exact same POST /customer/bookings after a timeout must get
// back the SAME booking, not a second one.
func TestExecute_RetryWithSameKeyAndBodyReplaysWithoutRerunningHandler(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, time.Hour)
	key := uuid.NewString()
	cleanupKey(t, db, "customer-1", "POST /customer/bookings", key)
	state := &testHandlerState{}
	router := newTestRouter(t, store, state)

	first := doRequest(router, "POST", "/customer/bookings", key, `{"pickup":"a"}`)
	second := doRequest(router, "POST", "/customer/bookings", key, `{"pickup":"a"}`)

	if state.runs != 1 {
		t.Fatalf("the handler must run exactly once across both calls, ran %d times", state.runs)
	}
	if first.Code != second.Code || first.Body.String() != second.Body.String() {
		t.Fatalf("the replayed response must be byte-identical to the original:\nfirst:  %d %s\nsecond: %d %s",
			first.Code, first.Body.String(), second.Code, second.Body.String())
	}
}

func TestExecute_SameKeyDifferentBodyReturnsIdempotencyConflict(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, time.Hour)
	key := uuid.NewString()
	cleanupKey(t, db, "customer-1", "POST /customer/bookings", key)
	router := newTestRouter(t, store, &testHandlerState{})

	doRequest(router, "POST", "/customer/bookings", key, `{"pickup":"a"}`)
	second := doRequest(router, "POST", "/customer/bookings", key, `{"pickup":"DIFFERENT"}`)

	if second.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", second.Code, second.Body.String())
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &body); err != nil {
		t.Fatalf("response was not the standard error envelope: %v (%s)", err, second.Body.String())
	}
	if body.Error.Code != "idempotency_conflict" {
		t.Fatalf("expected wire code idempotency_conflict per addendum §7.1, got %q", body.Error.Code)
	}
}

// A handler-level failure (not an idempotency-layer failure) must still be
// captured and replayed correctly if it is a cacheable outcome -- proving
// Execute's capture-writer interacts correctly with httpx.Abort, not just
// with a handler that calls c.JSON directly.
func TestExecute_HandlerValidationFailureIsCapturedAndReplayed(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, time.Hour)
	key := uuid.NewString()
	cleanupKey(t, db, "customer-1", "POST /customer/bookings", key)
	state := &testHandlerState{failWith: apperrors.Validation("Check your details.", []apperrors.FieldViolation{
		{Field: "pickup_lat", Message: "Pickup coordinates are required."},
	})}
	router := newTestRouter(t, store, state)

	first := doRequest(router, "POST", "/customer/bookings", key, `{"pickup":"a"}`)
	if first.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 from the handler's own validation error, got %d: %s", first.Code, first.Body.String())
	}
	if state.runs != 1 {
		t.Fatalf("expected exactly 1 run, got %d", state.runs)
	}

	second := doRequest(router, "POST", "/customer/bookings", key, `{"pickup":"a"}`)
	if state.runs != 1 {
		t.Fatalf("a cacheable validation failure must be replayed, not re-executed; runs=%d", state.runs)
	}
	if second.Code != first.Code || second.Body.String() != first.Body.String() {
		t.Fatalf("replayed error response should be byte-identical to the original:\nfirst:  %d %s\nsecond: %d %s",
			first.Code, first.Body.String(), second.Code, second.Body.String())
	}
}

// A 5xx must never be cached: caching it would durably lock the caller out
// of ever succeeding with that key, even after the transient failure clears.
func TestExecute_ServerErrorIsNotCachedAndCanBeRetried(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, time.Hour)
	key := uuid.NewString()
	cleanupKey(t, db, "customer-1", "POST /customer/bookings", key)
	state := &testHandlerState{failWith: apperrors.Unavailable("Payment provider is unreachable.", nil)}
	router := newTestRouter(t, store, state)

	first := doRequest(router, "POST", "/customer/bookings", key, `{"pickup":"a"}`)
	if first.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", first.Code, first.Body.String())
	}

	// The dependency recovers; the same key must be allowed to try again.
	state.failWith = nil
	second := doRequest(router, "POST", "/customer/bookings", key, `{"pickup":"a"}`)
	if second.Code != http.StatusCreated {
		t.Fatalf("expected the retry to succeed once the transient failure clears, got %d: %s", second.Code, second.Body.String())
	}
	if state.runs != 2 {
		t.Fatalf("a non-cacheable failure must allow re-execution, ran %d times", state.runs)
	}
}
