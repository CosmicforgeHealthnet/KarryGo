package marketplace_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
)

const responderSecret = "fleet-responder-secret"

// capturedRequest records what the real client actually sent on the wire.
type capturedRequest struct {
	Path           string
	IdempotencyKey string
	RequestID      string
	CorrelationID  string
	ServiceName    string
	Body           map[string]any
	Authorized     bool
}

// fleetResponderStub is a real Gin server shaped like fleet's internal API,
// enforcing fleet's real service authentication and caller allowlist.
type fleetResponderStub struct {
	server *httptest.Server

	mu       sync.Mutex
	captured []capturedRequest

	// Response controls.
	status   int
	bodyJSON string
	rawBody  string
	delay    time.Duration
	// allowlisted is the set of callers fleet will accept.
	allowlisted map[string]bool
}

func newFleetResponderStub(t *testing.T) *fleetResponderStub {
	t.Helper()
	stub := &fleetResponderStub{
		status:      http.StatusOK,
		allowlisted: map[string]bool{dispatchService: true},
	}
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	verifier := serviceauth.NewVerifier(
		serviceauth.Secrets{
			dispatchService:    []byte(responderSecret),
			"customer-service": []byte(responderSecret),
		}, time.Minute)

	handler := func(c *gin.Context) {
		record := capturedRequest{
			Path:           c.Request.URL.Path,
			IdempotencyKey: c.GetHeader("Idempotency-Key"),
			RequestID:      c.GetHeader("X-Request-ID"),
			CorrelationID:  c.GetHeader("X-Correlation-ID"),
			ServiceName:    c.GetHeader(serviceauth.HeaderServiceName),
		}

		name, err := verifier.VerifyRequest(c.Request)
		if err != nil {
			stub.record(record)
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
				"code": "unauthorized", "message": "Service authentication is required."}})
			return
		}
		// Signature proves WHO; the allowlist decides whether they may act.
		if !stub.allowlisted[name] {
			stub.record(record)
			c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
				"code": "forbidden", "message": "Service is not allowed."}})
			return
		}
		record.Authorized = true

		raw, _ := c.GetRawData()
		_ = json.Unmarshal(raw, &record.Body)
		stub.record(record)

		if stub.delay > 0 {
			time.Sleep(stub.delay)
		}
		if stub.rawBody != "" {
			c.Data(stub.status, "application/json", []byte(stub.rawBody))
			return
		}
		c.Data(stub.status, "application/json", []byte(stub.bodyJSON))
	}

	group := engine.Group("/internal/fleet-partner/v1")
	group.POST("/marketplace-offers/:offerId/provider-accept", handler)
	group.POST("/marketplace-offers/:offerId/provider-decline", handler)
	group.POST("/assignments/:assignmentId/provider-accept", handler)
	group.POST("/assignments/:assignmentId/provider-decline", handler)

	stub.server = httptest.NewServer(engine)
	t.Cleanup(stub.server.Close)
	return stub
}

func (f *fleetResponderStub) record(r capturedRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.captured = append(f.captured, r)
}

func (f *fleetResponderStub) last() capturedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.captured) == 0 {
		return capturedRequest{}
	}
	return f.captured[len(f.captured)-1]
}

func (f *fleetResponderStub) responder(t *testing.T, secret string, timeout time.Duration) marketplace.FleetResponder {
	t.Helper()
	responder, ok := marketplace.NewHTTPFleetResponder(marketplace.ResponderConfig{
		BaseURL: f.server.URL, ServiceName: dispatchService,
		Secret: []byte(secret), Timeout: timeout,
	})
	if !ok {
		t.Fatal("expected a configured responder")
	}
	return responder
}

func offerRelay(operationType string, offerID string, key string) marketplace.RelayOperation {
	return marketplace.RelayOperation{
		OperationType: operationType, ProviderID: "prov-1",
		OfferID: &offerID, IdempotencyKey: key, RequestFingerprint: "fp-1",
		Payload: []byte(`{"providerId":"prov-1","providerService":"DISPATCH","clientResponseId":"c1"}`),
	}
}

func assignmentRelay(operationType string, assignmentID string, key string) marketplace.RelayOperation {
	return marketplace.RelayOperation{
		OperationType: operationType, ProviderID: "prov-1",
		AssignmentID: &assignmentID, IdempotencyKey: key, RequestFingerprint: "fp-1",
		Payload: []byte(`{"providerId":"prov-1","providerService":"DISPATCH","clientResponseId":"c1"}`),
	}
}

func okOfferBody() string {
	return `{"data":{"offerId":"off-1","recipientId":"rec-1","status":"ACCEPTED","allowedActions":[]}}`
}

func okAssignmentBody() string {
	return `{"data":{"assignmentId":"asg-1","status":"ACCEPTED","allowedActions":[]}}`
}

func TestResponderDeliversAllFourDecisionKinds(t *testing.T) {
	cases := []struct {
		name      string
		operation marketplace.RelayOperation
		body      string
		wantPath  string
	}{
		{"offer accept", offerRelay(marketplace.RelayProviderOfferAccept, "off-1", "k1"),
			okOfferBody(), "/internal/fleet-partner/v1/marketplace-offers/off-1/provider-accept"},
		{"offer decline", offerRelay(marketplace.RelayProviderOfferDecline, "off-1", "k2"),
			`{"data":{"offerId":"off-1","recipientId":"rec-1","status":"DECLINED"}}`,
			"/internal/fleet-partner/v1/marketplace-offers/off-1/provider-decline"},
		{"assignment accept", assignmentRelay(marketplace.RelayProviderAssignmentAccept, "asg-1", "k3"),
			okAssignmentBody(), "/internal/fleet-partner/v1/assignments/asg-1/provider-accept"},
		{"assignment decline", assignmentRelay(marketplace.RelayProviderAssignmentDecline, "asg-1", "k4"),
			`{"data":{"assignmentId":"asg-1","status":"DECLINED"}}`,
			"/internal/fleet-partner/v1/assignments/asg-1/provider-decline"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			stub := newFleetResponderStub(t)
			stub.bodyJSON = testCase.body
			responder := stub.responder(t, responderSecret, 3*time.Second)

			result, err := responder.Deliver(context.Background(),
				marketplace.CallMeta{RequestID: "req-abc", CorrelationID: "corr-xyz"},
				testCase.operation)
			if err != nil {
				t.Fatalf("deliver: %v", err)
			}
			if !result.Accepted {
				t.Fatalf("result = %+v, want accepted", result)
			}

			sent := stub.last()
			if sent.Path != testCase.wantPath {
				t.Fatalf("path = %s, want %s", sent.Path, testCase.wantPath)
			}
			// The ORIGINAL key must reach fleet unchanged.
			if sent.IdempotencyKey != testCase.operation.IdempotencyKey {
				t.Fatalf("idempotency key = %q, want %q",
					sent.IdempotencyKey, testCase.operation.IdempotencyKey)
			}
			if sent.RequestID != "req-abc" || sent.CorrelationID != "corr-xyz" {
				t.Fatalf("correlation not propagated: requestId=%q correlationId=%q",
					sent.RequestID, sent.CorrelationID)
			}
			if sent.ServiceName != dispatchService {
				t.Fatalf("service identity = %q", sent.ServiceName)
			}
			if !sent.Authorized {
				t.Fatal("request was not authorized by fleet")
			}
			// The provider's decision payload arrives intact.
			if sent.Body["clientResponseId"] != "c1" {
				t.Fatalf("decision payload not delivered: %v", sent.Body)
			}
		})
	}
}

func TestResponderRejectedWhenSignatureOrCallerIsWrong(t *testing.T) {
	// Wrong secret: fleet refuses. Credentials cannot be fixed by retrying, so
	// this is permanent and must surface to an operator.
	stub := newFleetResponderStub(t)
	stub.bodyJSON = okOfferBody()
	wrong := stub.responder(t, "the-wrong-secret", 3*time.Second)

	result, err := wrong.Deliver(context.Background(), marketplace.CallMeta{},
		offerRelay(marketplace.RelayProviderOfferAccept, "off-1", "k-wrong"))
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if result.Accepted {
		t.Fatal("a wrong secret produced success")
	}
	if !result.Permanent {
		t.Fatalf("wrong secret = %+v; must be permanent", result)
	}

	// Non-allowlisted caller: signature valid, authorization refused.
	stranger := newFleetResponderStub(t)
	stranger.bodyJSON = okOfferBody()
	stranger.allowlisted = map[string]bool{"some-other-service": true}
	responder := stranger.responder(t, responderSecret, 3*time.Second)

	result, err = responder.Deliver(context.Background(), marketplace.CallMeta{},
		offerRelay(marketplace.RelayProviderOfferAccept, "off-1", "k-stranger"))
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if result.Accepted || !result.Permanent {
		t.Fatalf("non-allowlisted caller = %+v; must be a permanent refusal", result)
	}
}

func TestResponderClassifiesPermanentFleetRejections(t *testing.T) {
	permanent := []string{
		"assignment_not_owned_by_provider", "assignment_already_resolved",
		"assignment_response_expired", "marketplace_offer_expired",
		"marketplace_offer_cancelled", "marketplace_offer_already_resolved",
		"expected_version_mismatch", "idempotency_conflict",
		"decline_reason_required", "decline_reason_invalid", "decline_note_required",
		"validation_error", "provider_acceptance_cancelled", "provider_acceptance_expired",
	}
	for _, code := range permanent {
		t.Run(code, func(t *testing.T) {
			stub := newFleetResponderStub(t)
			stub.status = http.StatusConflict
			stub.bodyJSON = `{"error":{"code":"` + code + `","message":"refused"}}`
			responder := stub.responder(t, responderSecret, 3*time.Second)

			result, err := responder.Deliver(context.Background(), marketplace.CallMeta{},
				offerRelay(marketplace.RelayProviderOfferAccept, "off-1", "k-"+code))
			if err != nil {
				t.Fatalf("deliver: %v", err)
			}
			if result.Accepted {
				t.Fatalf("%s was treated as success", code)
			}
			if !result.Permanent {
				t.Fatalf("%s = %+v; must be permanent so retries stop", code, result)
			}
			if result.ErrorCode != code {
				t.Fatalf("error code = %q, want %q", result.ErrorCode, code)
			}
		})
	}
}

func TestResponderClassifiesTemporaryFailures(t *testing.T) {
	for _, status := range []int{
		http.StatusServiceUnavailable, http.StatusBadGateway,
		http.StatusGatewayTimeout, http.StatusTooManyRequests,
	} {
		stub := newFleetResponderStub(t)
		stub.status = status
		stub.bodyJSON = `{"error":{"code":"fleet_unavailable","message":"try later"}}`
		responder := stub.responder(t, responderSecret, 3*time.Second)

		result, err := responder.Deliver(context.Background(), marketplace.CallMeta{},
			assignmentRelay(marketplace.RelayProviderAssignmentAccept, "asg-1", "k-temp"))
		if err != nil {
			t.Fatalf("status %d returned an error rather than a retryable result: %v", status, err)
		}
		if result.Accepted {
			t.Fatalf("status %d was treated as success", status)
		}
		if result.Permanent {
			t.Fatalf("status %d = %+v; a transient fleet fault must stay retryable", status, result)
		}
	}
}

func TestResponderTreatsTimeoutAsUnknownOutcome(t *testing.T) {
	stub := newFleetResponderStub(t)
	stub.bodyJSON = okAssignmentBody()
	// Fleet answers, but far too slowly for our timeout. The request WAS
	// transmitted, so fleet may already have recorded the decision.
	stub.delay = 400 * time.Millisecond
	responder := stub.responder(t, responderSecret, 50*time.Millisecond)

	operation := assignmentRelay(marketplace.RelayProviderAssignmentAccept, "asg-1", "k-timeout")
	_, err := responder.Deliver(context.Background(), marketplace.CallMeta{}, operation)
	if err == nil {
		t.Fatal("a timeout must not be reported as a definite outcome")
	}
	if err != marketplace.ErrFleetUnavailable {
		t.Fatalf("timeout error = %v, want ErrFleetUnavailable (unknown outcome)", err)
	}

	// The relay is untouched: same key, same fingerprint, so the retry is a
	// redelivery of the same decision rather than a second one.
	if operation.IdempotencyKey != "k-timeout" || operation.RequestFingerprint != "fp-1" {
		t.Fatal("the relay identity was mutated by a failed delivery")
	}
}

func TestResponderTreatsConnectionResetAsUnknownOutcome(t *testing.T) {
	// A listener that accepts then immediately closes, reproducing a reset
	// after the request may have been transmitted.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); <-done })

	responder, ok := marketplace.NewHTTPFleetResponder(marketplace.ResponderConfig{
		BaseURL: "http://" + listener.Addr().String(), ServiceName: dispatchService,
		Secret: []byte(responderSecret), Timeout: 2 * time.Second,
	})
	if !ok {
		t.Fatal("expected configured responder")
	}

	_, err = responder.Deliver(context.Background(), marketplace.CallMeta{},
		offerRelay(marketplace.RelayProviderOfferAccept, "off-1", "k-reset"))
	if err != marketplace.ErrFleetUnavailable {
		t.Fatalf("connection reset = %v, want ErrFleetUnavailable", err)
	}
}

func TestResponderFailsClosedOnMalformedSuccess(t *testing.T) {
	// A 2xx whose body is unparseable or incomplete is NOT success. Fleet may
	// have committed, or a proxy may have produced it; confirming the relay
	// would lose the provider's decision.
	cases := map[string]string{
		"not json":       `<html>gateway</html>`,
		"empty body":     ``,
		"empty data":     `{"data":{}}`,
		"missing status": `{"data":{"offerId":"off-1"}}`,
		"no identifiers": `{"data":{"status":"ACCEPTED"}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			stub := newFleetResponderStub(t)
			stub.rawBody = body
			responder := stub.responder(t, responderSecret, 3*time.Second)

			result, err := responder.Deliver(context.Background(), marketplace.CallMeta{},
				offerRelay(marketplace.RelayProviderOfferAccept, "off-1", "k-"+name))
			if err == nil && result.Accepted {
				t.Fatalf("%s was treated as a confirmed delivery", name)
			}
		})
	}
}

func TestResponderBoundsOversizedResponseBodies(t *testing.T) {
	stub := newFleetResponderStub(t)
	// A valid envelope followed by a very large tail. The client must not
	// attempt to buffer an unbounded body.
	stub.rawBody = `{"data":{"offerId":"off-1","recipientId":"rec-1","status":"ACCEPTED"},"pad":"` +
		strings.Repeat("x", 4<<20) + `"}`
	responder := stub.responder(t, responderSecret, 10*time.Second)

	// Whatever the classification, it must return promptly and never panic or
	// hang; a truncated body simply fails closed.
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = responder.Deliver(context.Background(), marketplace.CallMeta{},
			offerRelay(marketplace.RelayProviderOfferAccept, "off-1", "k-big"))
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("an oversized response body was not bounded")
	}
}

func TestResponderUnconfiguredFailsClosed(t *testing.T) {
	for _, config := range []marketplace.ResponderConfig{
		{},
		{BaseURL: "http://fleet"},
		{BaseURL: "http://fleet", ServiceName: dispatchService},
		{ServiceName: dispatchService, Secret: []byte("s")},
	} {
		responder, ok := marketplace.NewHTTPFleetResponder(config)
		if ok {
			t.Fatalf("incomplete config %+v reported configured", config)
		}
		if responder.Configured() {
			t.Fatal("the fallback must report unconfigured")
		}
		result, err := responder.Deliver(context.Background(), marketplace.CallMeta{},
			offerRelay(marketplace.RelayProviderOfferAccept, "off-1", "k-unconf"))
		if result.Accepted {
			t.Fatal("the fail-closed fallback reported success")
		}
		if err != marketplace.ErrFleetUnavailable {
			t.Fatalf("fallback error = %v, want ErrFleetUnavailable", err)
		}
	}
}

func TestResponderDoesNotRetainFleetErrorMessages(t *testing.T) {
	// Fleet messages can quote request content. Only the stable CODE is kept,
	// because ErrorCode is persisted on the relay row and written to logs.
	stub := newFleetResponderStub(t)
	stub.status = http.StatusConflict
	stub.bodyJSON = `{"error":{"code":"validation_error",` +
		`"message":"provider +2348012345678 rejected for customer Jane Doe"}}`
	responder := stub.responder(t, responderSecret, 3*time.Second)

	result, err := responder.Deliver(context.Background(), marketplace.CallMeta{},
		offerRelay(marketplace.RelayProviderOfferAccept, "off-1", "k-redact"))
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if result.ErrorCode != "validation_error" {
		t.Fatalf("error code = %q", result.ErrorCode)
	}
	encoded, _ := json.Marshal(result)
	for _, leaked := range []string{"2348012345678", "Jane Doe", "rejected for customer"} {
		if bytes.Contains(encoded, []byte(leaked)) {
			t.Fatalf("delivery result retained sensitive fleet message content %q: %s",
				leaked, encoded)
		}
	}
}

func TestResponderRejectsMalformedRelayPermanently(t *testing.T) {
	stub := newFleetResponderStub(t)
	responder := stub.responder(t, responderSecret, 3*time.Second)

	// An offer relay with no offer id can never be routed; retrying it forever
	// would just consume the attempt budget.
	result, err := responder.Deliver(context.Background(), marketplace.CallMeta{},
		marketplace.RelayOperation{
			OperationType: marketplace.RelayProviderOfferAccept,
			ProviderID:    "prov-1", IdempotencyKey: "k-malformed",
		})
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if !result.Permanent || result.Accepted {
		t.Fatalf("malformed relay = %+v; must be permanently rejected", result)
	}
}
