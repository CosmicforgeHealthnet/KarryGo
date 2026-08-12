package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	mkthttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/http"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	privatemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

const stageOneServiceSecret = "stage-one-cross-service-hmac-secret"

type crossServiceDispatchReady struct {
	URL               string `json:"url"`
	ProviderID        string `json:"providerId"`
	PrivateProviderID string `json:"privateProviderId"`
	PrivateVehicleID  string `json:"privateVehicleId"`
	RequestID         string `json:"requestId"`
	Token             string `json:"token"`
}

type executionHTTPObservation struct {
	AssignmentID string
	Status       string
	Idempotency  string
	LockFree     bool
	Err          error
}

type controlledExecutionTransport struct {
	backendURL string
	fleetPool  *pgxpool.Pool
	release    chan struct{}
	started    chan executionHTTPObservation

	mu           sync.Mutex
	postCount    int
	readCount    int
	postKeys     []string
	readBackKeys []string
	lastStatus   int
	lastBody     string
}

func (p *controlledExecutionTransport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read request", http.StatusBadGateway)
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/marketplace/executions" {
		var input struct {
			FleetAssignmentID string `json:"fleetAssignmentId"`
		}
		observation := executionHTTPObservation{Idempotency: r.Header.Get("Idempotency-Key")}
		if err := json.Unmarshal(body, &input); err != nil {
			observation.Err = fmt.Errorf("decode execution request: %w", err)
		} else {
			observation.AssignmentID = input.FleetAssignmentID
			observation.Status, observation.LockFree, observation.Err = p.observeCommitPoint(r.Context(), input.FleetAssignmentID)
		}
		p.mu.Lock()
		p.postCount++
		p.postKeys = append(p.postKeys, observation.Idempotency)
		p.mu.Unlock()
		p.started <- observation
		<-p.release

		resp, responseBody, forwardErr := p.forward(r, body)
		if forwardErr != nil {
			http.Error(w, "dispatch unavailable", http.StatusBadGateway)
			return
		}
		resp.Body.Close()
		p.mu.Lock()
		p.lastStatus = resp.StatusCode
		p.lastBody = string(responseBody)
		p.mu.Unlock()
		// Deliberately lose the response only after the real dispatch handler
		// returned, which means its PostgreSQL transaction has committed.
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "cannot drop response", http.StatusInternalServerError)
			return
		}
		connection, _, hijackErr := hijacker.Hijack()
		if hijackErr != nil {
			return
		}
		_ = connection.Close()
		_ = responseBody
		return
	}

	if r.Method == http.MethodGet && bytes.Contains([]byte(r.URL.Path), []byte("/executions/by-idempotency/")) {
		p.mu.Lock()
		p.readCount++
		p.readBackKeys = append(p.readBackKeys, r.Header.Get("Idempotency-Key"))
		p.mu.Unlock()
	}
	resp, responseBody, err := p.forward(r, body)
	if err != nil {
		http.Error(w, "dispatch unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(responseBody)
}

func (p *controlledExecutionTransport) observeCommitPoint(ctx context.Context, assignmentID string) (string, bool, error) {
	var status string
	if err := p.fleetPool.QueryRow(ctx, `
		SELECT status FROM fleet_partner_provider_assignments WHERE id = $1`, assignmentID).Scan(&status); err != nil {
		return "", false, err
	}
	tx, err := p.fleetPool.Begin(ctx)
	if err != nil {
		return status, false, err
	}
	defer tx.Rollback(ctx)
	var lockedID string
	err = tx.QueryRow(ctx, `
		SELECT id FROM fleet_partner_provider_assignments WHERE id = $1 FOR UPDATE NOWAIT`, assignmentID).Scan(&lockedID)
	if err != nil {
		return status, false, err
	}
	return status, lockedID == assignmentID, nil
}

func (p *controlledExecutionTransport) forward(original *http.Request, body []byte) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(original.Context(), original.Method,
		p.backendURL+original.URL.RequestURI(), bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header = original.Header.Clone()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, nil, err
	}
	return resp, responseBody, nil
}

func (p *controlledExecutionTransport) countsAndKeys() (int, int, []string, []string, int, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.postCount, p.readCount, append([]string(nil), p.postKeys...),
		append([]string(nil), p.readBackKeys...), p.lastStatus, p.lastBody
}

type workerPass struct {
	result mktusecases.ExecutionStartResult
	err    error
}

// TestAcceptanceToOneCanonicalTrip is the final Stage 1 cross-service proof.
func TestAcceptanceToOneCanonicalTrip(t *testing.T) {
	dispatchDSN := os.Getenv("DISPATCH_TEST_DATABASE_URL")
	if dispatchDSN == "" {
		t.Skip("DISPATCH_TEST_DATABASE_URL not set; skipping cross-service PostgreSQL integration test")
	}
	fleetPool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)

	fleetListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for fleet server: %v", err)
	}
	fleetURL := "http://" + fleetListener.Addr().String()

	readyFile := filepath.Join(t.TempDir(), "dispatch-ready.json")
	dispatchDir, err := filepath.Abs(filepath.Join("..", "..", "..", "driver-dispatch-delivery-service"))
	if err != nil {
		t.Fatalf("resolve dispatch service directory: %v", err)
	}
	childCtx, stopChild := context.WithCancel(ctx)
	command := exec.CommandContext(childCtx, "go", "test", "-count=1",
		"-run", "^TestAcceptanceToCanonicalTripDispatchHarness$", "./internal/router")
	command.Dir = dispatchDir
	var childOutput bytes.Buffer
	command.Stdout = &childOutput
	command.Stderr = &childOutput
	command.Env = append(os.Environ(),
		"KARRYGO_CROSS_SERVICE_DISPATCH_HELPER=1",
		"KARRYGO_CROSS_SERVICE_READY_FILE="+readyFile,
		"KARRYGO_CROSS_SERVICE_FLEET_URL="+fleetURL,
		"DISPATCH_TEST_DATABASE_URL="+dispatchDSN,
	)
	if err := command.Start(); err != nil {
		t.Fatalf("start dispatch helper: %v", err)
	}
	childDone := make(chan error, 1)
	go func() { childDone <- command.Wait() }()
	var dispatchURL string
	childExited := false
	t.Cleanup(func() {
		if dispatchURL != "" {
			req, _ := http.NewRequest(http.MethodPost, dispatchURL+"/__cross_service_stop", nil)
			if req != nil {
				response, requestErr := http.DefaultClient.Do(req)
				if requestErr == nil {
					response.Body.Close()
				}
			}
		}
		if !childExited {
			select {
			case childErr := <-childDone:
				if childErr != nil {
					t.Errorf("dispatch helper exit: %v\n%s", childErr, childOutput.String())
				}
			case <-time.After(10 * time.Second):
				stopChild()
				childErr := <-childDone
				t.Errorf("dispatch helper did not stop cleanly: %v\n%s", childErr, childOutput.String())
			}
		}
	})

	var ready crossServiceDispatchReady
	deadline := time.Now().Add(60 * time.Second)
	for {
		raw, readErr := os.ReadFile(readyFile)
		if readErr == nil && json.Unmarshal(raw, &ready) == nil && ready.URL != "" {
			break
		}
		select {
		case childErr := <-childDone:
			childExited = true
			t.Fatalf("dispatch helper exited before readiness: %v\n%s", childErr, childOutput.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("dispatch helper did not become ready\n%s", childOutput.String())
		}
		time.Sleep(25 * time.Millisecond)
	}
	dispatchURL = ready.URL

	dispatchPool, err := pgxpool.New(ctx, dispatchDSN)
	if err != nil {
		t.Fatalf("connect dispatch database for assertions: %v", err)
	}
	defer dispatchPool.Close()

	transport := &controlledExecutionTransport{
		backendURL: dispatchURL,
		fleetPool:  fleetPool,
		release:    make(chan struct{}),
		started:    make(chan executionHTTPObservation, 1),
	}
	executionProxy := httptest.NewServer(transport)
	defer executionProxy.Close()

	metrics := observability.NewMetrics()
	service := mktusecases.NewService(mktusecases.Options{
		Pool:            fleetPool,
		Projections:     mktrepos.NewProjectionRepository(fleetPool),
		Offers:          mktrepos.NewOfferRepository(fleetPool),
		Assignments:     mktrepos.NewAssignmentRepository(fleetPool),
		ExecutionStarts: mktrepos.NewExecutionStartRepository(fleetPool),
		Executions: platformclients.BuildExecutionSet(platformclients.Options{
			ServiceName: "driver-dispatch-delivery-service",
			BaseURL:     executionProxy.URL,
			Secret:      []byte(stageOneServiceSecret),
			Timeout:     5 * time.Second,
			Metrics:     metrics,
		}),
		Events:            events.NewDomainPublisher(events.NewOutboxRepository(fleetPool)),
		PrivateAcceptance: app.NewPrivateAcceptanceReader(fleetPool),
		Metrics:           metrics,
	})
	internalHandler := mkthttp.NewInternalHandler(service, []string{"driver-dispatch-delivery-service"})
	fleetConfig := config.Config{AppEnv: "test", MaxRequestBodyBytes: 1 << 20}
	fleetRouter := app.BuildRouter(app.Options{
		Config:      fleetConfig,
		Metrics:     metrics,
		TokenSigner: auth.NewTokenSigner([]byte("unused-cross-service-user-token-secret")),
		Verifier: serviceauth.NewVerifier(serviceauth.Secrets{
			"driver-dispatch-delivery-service": []byte(stageOneServiceSecret),
		}, time.Minute),
		InternalRoutes: func(group *gin.RouterGroup) {
			mkthttp.RegisterInternalMarketplaceRoutes(group, internalHandler)
			mkthttp.RegisterProviderResponseRoutes(group, internalHandler)
		},
	})
	var relayKeysMu sync.Mutex
	var relayKeys []string
	fleetServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && bytes.Contains([]byte(r.URL.Path), []byte("/provider-accept")) {
			relayKeysMu.Lock()
			relayKeys = append(relayKeys, r.Header.Get("Idempotency-Key"))
			relayKeysMu.Unlock()
		}
		fleetRouter.ServeHTTP(w, r)
	}), ReadHeaderTimeout: 5 * time.Second}
	fleetServeDone := make(chan error, 1)
	go func() { fleetServeDone <- fleetServer.Serve(fleetListener) }()
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = fleetServer.Shutdown(shutdownCtx)
		<-fleetServeDone
	}()

	org := createOrganization(t, fleetPool, "Stage One E2E Org", "RC-STAGE1-E2E")
	projection := seedProjection(t, fleetPool, org.ID, ready.RequestID, mktmodels.ProjectionOffered)
	provider := seedProviderMembership(t, fleetPool, org.ID, ready.ProviderID)
	offer, recipients := seedOfferWithRecipients(t, fleetPool, org.ID, projection.ID,
		[]*providermodels.Membership{provider}, 5*time.Minute)
	clientResponseID := "stage-one-provider-response"

	providerResponse := postProviderAcceptance(t, dispatchURL, ready.Token, offer.ID, map[string]any{
		"clientResponseId":         clientResponseID,
		"expectedVersion":          recipients[0].Version,
		"requestId":                ready.RequestID,
		"organizationId":           org.ID,
		"organizationMembershipId": provider.ID,
	})
	if providerResponse.StatusCode != http.StatusOK {
		t.Fatalf("provider acceptance HTTP = %d, body=%s", providerResponse.StatusCode, providerResponse.Body)
	}
	if trips := countRows(t, dispatchPool, `SELECT count(*) FROM trips`); trips != 0 {
		t.Fatalf("provider response waited for canonical execution; trips=%d before worker", trips)
	}

	if got := countRows(t, dispatchPool, `SELECT count(*) FROM dispatch_organization_reservations`); got != 1 {
		t.Fatalf("dispatch reservations=%d, want 1", got)
	}
	if got := countRows(t, dispatchPool, `SELECT count(*) FROM dispatch_fleet_relay_operations`); got != 1 {
		t.Fatalf("dispatch relays=%d, want 1", got)
	}
	var reservationSource, reservationKey, reservationID string
	if err := dispatchPool.QueryRow(ctx, `
		SELECT acceptance_source, idempotency_key, id
		FROM dispatch_organization_reservations`).Scan(&reservationSource, &reservationKey, &reservationID); err != nil {
		t.Fatalf("read dispatch reservation: %v", err)
	}
	if reservationSource != "PROVIDER_DIRECT" || reservationKey != clientResponseID {
		t.Fatalf("reservation source/key=%s/%s", reservationSource, reservationKey)
	}
	var relayKey string
	if err := dispatchPool.QueryRow(ctx, `SELECT idempotency_key FROM dispatch_fleet_relay_operations`).Scan(&relayKey); err != nil {
		t.Fatalf("read relay key: %v", err)
	}
	if relayKey != clientResponseID {
		t.Fatalf("relay idempotency key=%s, want %s", relayKey, clientResponseID)
	}
	relayKeysMu.Lock()
	seenRelayKeys := append([]string(nil), relayKeys...)
	relayKeysMu.Unlock()
	if len(seenRelayKeys) == 0 {
		t.Fatal("dispatch made no authenticated relay delivery")
	}
	for _, key := range seenRelayKeys {
		if key != clientResponseID {
			t.Fatalf("relay HTTP keys=%v, want original key on every delivery", seenRelayKeys)
		}
	}

	if got := waitForRowCount(t, fleetPool, 20*time.Second, `SELECT count(*) FROM fleet_partner_organization_acceptances`); got != 1 {
		t.Fatalf("organization acceptances=%d, want 1", got)
	}
	if got := countRows(t, fleetPool, `SELECT count(*) FROM fleet_partner_provider_assignments`); got != 1 {
		t.Fatalf("assignments=%d, want 1", got)
	}
	if got := countRows(t, fleetPool, `SELECT count(*) FROM fleet_partner_assignment_responses`); got != 1 {
		t.Fatalf("provider responses=%d, want 1", got)
	}
	if got := countRows(t, fleetPool, `SELECT count(*) FROM fleet_partner_execution_start_operations`); got != 1 {
		t.Fatalf("execution-start operations=%d, want 1", got)
	}

	assignments := mktrepos.NewAssignmentRepository(fleetPool)
	executionStarts := mktrepos.NewExecutionStartRepository(fleetPool)
	assignment, err := assignments.GetByID(ctx, org.ID, assignmentID(t, fleetPool))
	if err != nil {
		t.Fatalf("load assignment: %v", err)
	}
	if assignment.Status != mktmodels.AssignmentAccepted || len(assignment.AllowedActions(time.Now())) != 0 {
		t.Fatalf("delegated assignment status/actions=%s/%v; provider must not accept twice",
			assignment.Status, assignment.AllowedActions(time.Now()))
	}
	operation, err := executionStarts.GetByAssignment(ctx, assignment.ID)
	if err != nil {
		t.Fatalf("load execution start: %v", err)
	}
	originalExecutionKey := operation.IdempotencyKey

	workerConfig := mktusecases.ExecutionStartWorkerConfig{
		Owner: "stage-one-execution-worker", Lease: 10 * time.Second,
		BatchSize: 1, MaxAttempts: 3, BaseBackoff: time.Millisecond,
	}
	passes := make(chan workerPass, 2)
	for range 2 {
		go func() {
			result, runErr := service.RunExecutionStart(ctx, workerConfig)
			passes <- workerPass{result: result, err: runErr}
		}()
	}
	observation := <-transport.started
	if observation.Err != nil {
		t.Fatalf("observe dispatch execution call: %v", observation.Err)
	}
	if observation.AssignmentID != assignment.ID ||
		observation.Status != mktmodels.AssignmentExecutionStartPending || !observation.LockFree {
		t.Fatalf("dispatch call observation=%+v", observation)
	}
	if observation.Idempotency != originalExecutionKey {
		t.Fatalf("execution POST key=%s, want %s", observation.Idempotency, originalExecutionKey)
	}

	cancelErr := service.Cancel(ctx, mktusecases.CancelInput{
		Tenant: tenant.Context{OrganizationID: org.ID, OrganizationUserID: "usr-stage-one",
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: "usr-stage-one"},
		AssignmentID: assignment.ID,
		Reason:       "operator requested cancellation while outcome unresolved",
	})
	if cancelErr == nil {
		t.Fatal("cancellation succeeded after EXECUTION_START_PENDING commit point")
	}
	stillPending, err := assignments.GetByID(ctx, org.ID, assignment.ID)
	if err != nil || stillPending.Status != mktmodels.AssignmentExecutionStartPending {
		t.Fatalf("assignment after rejected cancellation=%v, err=%v", stillPending, err)
	}

	close(transport.release)
	claimed, unknown := 0, 0
	for range 2 {
		pass := <-passes
		if pass.err != nil {
			t.Fatalf("execution worker pass: %v", pass.err)
		}
		claimed += pass.result.Claimed
		unknown += pass.result.Unknown
	}
	if claimed != 1 || unknown != 1 {
		t.Fatalf("concurrent worker claims=%d unknown=%d, want 1/1", claimed, unknown)
	}
	if got := countRows(t, dispatchPool, `SELECT count(*) FROM trips`); got != 1 {
		_, _, _, _, status, body := transport.countsAndKeys()
		t.Fatalf("canonical trips after lost response=%d, want 1; dispatch=%d %s", got, status, body)
	}
	var canonicalTripID string
	if err := dispatchPool.QueryRow(ctx, `SELECT id FROM trips`).Scan(&canonicalTripID); err != nil {
		t.Fatalf("read canonical trip: %v", err)
	}
	unknownOperation, err := executionStarts.GetByAssignment(ctx, assignment.ID)
	if err != nil || unknownOperation.Status != mktmodels.ExecutionStartUnknown {
		t.Fatalf("execution operation after lost response=%v, err=%v", unknownOperation, err)
	}
	if unknownOperation.IdempotencyKey != originalExecutionKey {
		t.Fatalf("unknown operation key changed: %s -> %s", originalExecutionKey, unknownOperation.IdempotencyKey)
	}

	time.Sleep(5 * time.Millisecond)
	readBackPass, err := service.RunExecutionStart(ctx, workerConfig)
	if err != nil {
		t.Fatalf("execution read-back pass: %v", err)
	}
	if readBackPass.Claimed != 1 || readBackPass.Confirmed != 1 {
		t.Fatalf("read-back pass=%+v", readBackPass)
	}
	postCount, readCount, postKeys, readKeys, _, _ := transport.countsAndKeys()
	if postCount != 1 || readCount != 1 {
		t.Fatalf("execution HTTP post/read-back=%d/%d, want 1/1", postCount, readCount)
	}
	if len(postKeys) != 1 || len(readKeys) != 1 ||
		postKeys[0] != originalExecutionKey || readKeys[0] != originalExecutionKey {
		t.Fatalf("execution idempotency keys post=%v read-back=%v original=%s",
			postKeys, readKeys, originalExecutionKey)
	}

	finalAssignment, err := assignments.GetByID(ctx, org.ID, assignment.ID)
	if err != nil {
		t.Fatalf("load final assignment: %v", err)
	}
	if finalAssignment.Status != mktmodels.AssignmentReadyForExecution ||
		finalAssignment.SourceExecutionID == nil || *finalAssignment.SourceExecutionID != canonicalTripID {
		t.Fatalf("final assignment status/trip=%s/%v, want READY_FOR_EXECUTION/%s",
			finalAssignment.Status, finalAssignment.SourceExecutionID, canonicalTripID)
	}
	confirmedOperation, err := executionStarts.GetByAssignment(ctx, assignment.ID)
	if err != nil || confirmedOperation.TripID == nil || *confirmedOperation.TripID != canonicalTripID {
		t.Fatalf("confirmed operation trip=%v, err=%v", confirmedOperation, err)
	}

	lastPass, err := service.RunExecutionStart(ctx, workerConfig)
	if err != nil || lastPass.Claimed != 0 {
		t.Fatalf("idempotent worker rerun=%+v, err=%v", lastPass, err)
	}
	time.Sleep(250 * time.Millisecond)
	if got := countRows(t, dispatchPool, `SELECT count(*) FROM trips`); got != 1 {
		t.Fatalf("canonical trips after worker reruns=%d, want 1", got)
	}
	if got := countRows(t, dispatchPool, `SELECT count(*) FROM dispatch_fleet_relay_operations`); got != 1 {
		t.Fatalf("relay rows after worker reruns=%d, want 1", got)
	}
	if got := countRows(t, fleetPool, `SELECT count(*) FROM fleet_partner_assignment_responses`); got != 1 {
		t.Fatalf("provider responses after worker reruns=%d, want 1", got)
	}

	// Section 15 private-order boundary: use the production Fleet adapter and
	// worker against the real Dispatch private contract. Dispatch verifies the
	// accepted private assignment by calling the live Fleet internal endpoint.
	privateFixture, privateService, privateRepo, privateAssignmentID :=
		privateWorkerFixtureOnPool(t, fleetPool, "pex_blackbox_dispatch")
	privateDecisionID := "prd_blackbox_dispatch"
	if _, err = privateFixture.pool.Exec(ctx, `UPDATE fleet_partner_private_assignments SET
		provider_membership_id='mem_blackbox_dispatch',provider_id=$2,
		vehicle_or_truck_reference=$3,execution_service='DISPATCH',status='ACCEPTED'
		WHERE id=$1`, privateAssignmentID, ready.PrivateProviderID, ready.PrivateVehicleID); err != nil {
		t.Fatalf("prepare private assignment: %v", err)
	}
	if _, err = privateFixture.pool.Exec(ctx, `INSERT INTO fleet_partner_private_assignment_decisions(
		id,organization_id,assignment_id,provider_id,decision,idempotency_key,request_fingerprint)
		SELECT $2,organization_id,id,$3,'ACCEPT',$4,$4 FROM fleet_partner_private_assignments WHERE id=$1`,
		privateAssignmentID, privateDecisionID, ready.PrivateProviderID, "accept-blackbox-dispatch"); err != nil {
		t.Fatalf("record private provider acceptance: %v", err)
	}
	settlementID := privateFixture.tenant.OrganizationID
	if _, err = privateFixture.pool.Exec(ctx, `UPDATE fleet_partner_private_execution_operations SET
		settlement_organization_id=$2,executing_provider_id=$3,
		organization_membership_id='mem_blackbox_dispatch',provider_response_id=$4,
		vehicle_or_truck_reference=$5,status='PENDING',next_attempt_at=now(),
		execution_payload='{"pickupAddress":"blackbox private pickup","pickupLatitude":6.45,
		"pickupLongitude":3.39,"dropoffAddress":"blackbox private dropoff",
		"dropoffLatitude":6.52,"dropoffLongitude":3.37}'::jsonb
		WHERE id=$1`, "pex_blackbox_dispatch", settlementID, ready.PrivateProviderID,
		privateDecisionID, ready.PrivateVehicleID); err != nil {
		t.Fatalf("prepare private execution: %v", err)
	}
	privatePort := app.BuildPrivateExecutionAdapter(config.Config{
		DispatchServiceURL: dispatchURL, DispatchServiceSecret: []byte(stageOneServiceSecret),
		DispatchServiceTimeout: 5 * time.Second, HTTPClientTimeout: 5 * time.Second,
	}, metrics)
	privateResult, err := privateService.RunExecutionPass(ctx, privatePort,
		privateusecases.ExecutionWorkerConfig{Owner: "private-blackbox-dispatch", Lease: 5 * time.Second,
			BatchSize: 1, MaxAttempts: 3, BaseBackoff: time.Millisecond})
	if err != nil || privateResult.Confirmed != 1 {
		current, _ := privateRepo.GetExecution(ctx, nil, privateFixture.tenant.OrganizationID, "pex_blackbox_dispatch")
		readOutcome, readErr := privatePort.ReadBack(ctx, privateFixture.tenant, current)
		t.Fatalf("private Dispatch worker result=%+v err=%v readback=%+v readbackErr=%v",
			privateResult, err, readOutcome, readErr)
	}
	privateOperation, err := privateRepo.GetExecution(ctx, nil,
		privateFixture.tenant.OrganizationID, "pex_blackbox_dispatch")
	if err != nil || privateOperation.Status != privatemodels.ExecutionConfirmed ||
		privateOperation.CanonicalTripReference == "" {
		t.Fatalf("private Dispatch operation=%+v err=%v", privateOperation, err)
	}
	var sourcePrivateTrips, distinctSettlement int
	if err = dispatchPool.QueryRow(ctx, `SELECT count(*) FROM dispatch_private_execution_mappings
		WHERE fleet_operation_id='pex_blackbox_dispatch' AND canonical_trip_id=$1`,
		privateOperation.CanonicalTripReference).Scan(&sourcePrivateTrips); err != nil {
		t.Fatal(err)
	}
	if err = dispatchPool.QueryRow(ctx, `SELECT count(*) FROM dispatch_private_execution_mappings
		WHERE fleet_operation_id='pex_blackbox_dispatch' AND settlement_organization_id<>executing_provider_id::text`).Scan(&distinctSettlement); err != nil {
		t.Fatal(err)
	}
	if sourcePrivateTrips != 1 || distinctSettlement != 1 {
		t.Fatalf("private source trips=%d distinct settlement rows=%d", sourcePrivateTrips, distinctSettlement)
	}
	readyAssignment, err := privateRepo.GetAssignment(ctx, nil,
		privateFixture.tenant.OrganizationID, privateAssignmentID)
	if err != nil || readyAssignment.Status != privatemodels.StatusReady {
		t.Fatalf("ready private assignment=%+v err=%v", readyAssignment, err)
	}
	cancelTenant := privateFixture.tenant
	cancelTenant.CorrelationID = "private-blackbox-cancel"
	if _, err = privateService.Cancel(ctx, cancelTenant, privateAssignmentID, readyAssignment.Version); err != nil {
		t.Fatalf("schedule private Dispatch cancellation: %v", err)
	}
	cancelPass, err := privateService.RunExecutionPass(ctx, privatePort,
		privateusecases.ExecutionWorkerConfig{Owner: "private-blackbox-cancel", Lease: 5 * time.Second,
			BatchSize: 1, MaxAttempts: 3, BaseBackoff: time.Millisecond})
	if err != nil || cancelPass.Cancelled != 1 {
		t.Fatalf("private Dispatch cancellation pass=%+v err=%v", cancelPass, err)
	}
	cancelledOperation, err := privateRepo.GetExecution(ctx, nil,
		privateFixture.tenant.OrganizationID, "pex_blackbox_dispatch")
	if err != nil || cancelledOperation.Status != privatemodels.ExecutionCancelled {
		t.Fatalf("cancelled private operation=%+v err=%v", cancelledOperation, err)
	}
	var sourceCancellation string
	if err = dispatchPool.QueryRow(ctx, `SELECT cancellation_status FROM dispatch_private_execution_mappings
		WHERE fleet_operation_id='pex_blackbox_dispatch'`).Scan(&sourceCancellation); err != nil {
		t.Fatal(err)
	}
	if sourceCancellation != "CONFIRMED" {
		t.Fatalf("source cancellation status=%s", sourceCancellation)
	}
	lastPrivatePass, err := privateService.RunExecutionPass(ctx, privatePort,
		privateusecases.ExecutionWorkerConfig{Owner: "private-blackbox-dispatch", Lease: 5 * time.Second, BatchSize: 1})
	if err != nil || lastPrivatePass.Claimed != 0 {
		t.Fatalf("private worker duplicate pass=%+v err=%v", lastPrivatePass, err)
	}
}

type providerHTTPResult struct {
	StatusCode int
	Body       string
}

func postProviderAcceptance(t *testing.T, baseURL string, token string, offerID string, body any) providerHTTPResult {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode provider acceptance: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost,
		baseURL+"/api/v1/provider/organization-offers/"+offerID+"/accept", bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("build provider acceptance request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "stage-one-provider-http")
	req.Header.Set("X-Correlation-ID", "stage-one-provider-http")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("provider acceptance HTTP: %v", err)
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(resp.Body)
	return providerHTTPResult{StatusCode: resp.StatusCode, Body: string(responseBody)}
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}

func waitForRowCount(t *testing.T, pool *pgxpool.Pool, timeout time.Duration, query string, args ...any) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		got := countRows(t, pool, query, args...)
		if got > 0 || time.Now().After(deadline) {
			return got
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func assignmentID(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `SELECT id FROM fleet_partner_provider_assignments`).Scan(&id); err != nil {
		t.Fatalf("read assignment id: %v", err)
	}
	return id
}
