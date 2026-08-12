package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

func TestFleetClientToRealHaulingAuthority(t *testing.T) {
	haulingDSN := os.Getenv("HAULING_TEST_DATABASE_URL")
	if haulingDSN == "" {
		t.Skip("HAULING_TEST_DATABASE_URL is required for Fleet-Hauling black-box integration")
	}
	if !strings.Contains(strings.ToLower(haulingDSN), "test") {
		t.Fatal("refusing to use a Hauling database whose URL is not test-specific")
	}

	organizationID, membershipID := uuid.NewString(), uuid.NewString()
	assignmentID, responseID := uuid.NewString(), uuid.NewString()
	providerID, truckID := uuid.NewString(), uuid.NewString()
	orderID, shipmentID, operationID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	fleetAcceptance := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/assignments/"+assignmentID+"/provider-acceptance") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{
			"assignmentId": assignmentID, "organizationId": organizationID,
			"providerId": providerID, "providerService": "HAULING",
			"organizationMembershipId": membershipID, "providerResponseId": responseID,
			"responseStatus": "ACCEPTED"}})
	}))
	defer fleetAcceptance.Close()

	haulingRoot, err := filepath.Abs(filepath.Join("..", "..", "..", "driver-hauling-service"))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "hauling-e2e")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd")
	build.Dir = haulingRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Hauling service: %v\n%s", err, output)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	secret := "fleet-hauling-blackbox-secret"
	command := exec.Command(binary)
	command.Dir = haulingRoot
	command.Env = append(os.Environ(),
		"APP_ENV=test", "HTTP_ADDR="+address, "HAULING_MIGRATION=true",
		"HAULING_DATABASE_URL="+haulingDSN, "HAULING_REDIS_ADDR=127.0.0.1:1",
		"HAULING_PROVIDER_TOKEN_SECRET=provider-token", "HAULING_PROVIDER_REFRESH_SECRET=provider-refresh",
		"HAULING_PROVIDER_OTP_SECRET=provider-otp", "HAULING_CUSTOMER_TOKEN_SECRET=customer-token",
		"HAULING_FLEET_INTEGRATION_ENABLED=true", "HAULING_FLEET_INTEGRATION_KILL_SWITCH=false",
		"HAULING_FLEET_ALLOWED_CALLERS=fleet-partner-service", "HAULING_SERVICE_SECRETS=fleet-partner-service="+secret,
		"HAULING_PRIVATE_EXECUTION_ENABLED=true", "FLEET_PARTNER_SERVICE_URL="+fleetAcceptance.URL,
		"FLEET_PARTNER_SERVICE_CALLER=driver-hauling-service", "FLEET_PARTNER_SERVICE_SECRET=hauling-to-fleet-secret",
		"HAULING_OUTBOX_WORKER_ENABLED=true", "HAULING_FLEET_EVENT_URL=http://127.0.0.1:1/events",
		"HAULING_FLEET_EVENT_SECRET=fleet-event", "HAULING_PAYMENT_URL=http://127.0.0.1:1",
		"HAULING_PAYMENT_SECRET=wallet", "HAULING_NOTIFICATION_URL=http://127.0.0.1:1",
		"HAULING_NOTIFICATION_SECRET=notification",
	)
	var logs bytes.Buffer
	command.Stdout, command.Stderr = &logs, &logs
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
	})

	baseURL := "http://" + address
	deadline := time.Now().Add(30 * time.Second)
	for {
		response, requestErr := http.Get(baseURL + "/health")
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("Hauling service did not become healthy: %v\n%s", requestErr, logs.String())
		}
		time.Sleep(100 * time.Millisecond)
	}

	pool, err := pgxpool.New(context.Background(), haulingDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(), `INSERT INTO truck_providers(id,phone,email,status,onboarding_status,service_type) VALUES($1,$2,$3,'active','complete','hauling')`, providerID, "+234"+providerID[:10], providerID+"@example.invalid"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO trucks(id,provider_id,truck_type,capacity_kg,plate_number,status)
		VALUES($1,$2,'flatbed',5000,$3,'active')`, truckID, providerID, "BB-"+truckID[:8]); err != nil {
		t.Fatal(err)
	}

	client := platformclients.NewHaulingProviderClient(platformclients.Options{
		ServiceName: "fleet-partner-service", BaseURL: baseURL, Secret: []byte(secret),
		Timeout: 5 * time.Second, Metrics: observability.NewMetrics(),
	})
	meta := platformclients.CallMeta{OrganizationID: organizationID, RequestID: uuid.NewString(), CorrelationID: uuid.NewString(), CausationID: uuid.NewString()}
	request := platformclients.HaulingLinkRequest{OrganizationID: organizationID, MembershipID: membershipID, ProviderID: providerID, IdempotencyKey: "blackbox-create-" + providerID}
	link, err := client.CreateProviderLink(context.Background(), meta, request)
	if err != nil {
		t.Fatalf("Fleet create provider link: %v\n%s", err, logs.String())
	}
	replay, err := client.CreateProviderLink(context.Background(), meta, request)
	if err != nil || replay.LinkID != link.LinkID || replay.SourceVersion != link.SourceVersion {
		t.Fatalf("Fleet idempotent replay=%+v err=%v", replay, err)
	}
	active, err := client.TransitionProviderLink(context.Background(), meta, link.LinkID, platformclients.HaulingLinkTransition{
		State: "ACTIVE", ExpectedVersion: link.SourceVersion, OrganizationID: organizationID,
		ReasonCode: "BLACKBOX", IdempotencyKey: "blackbox-active-" + providerID,
	})
	if err != nil || active.Status != "ACTIVE" {
		t.Fatalf("Fleet activate provider link=%+v err=%v", active, err)
	}
	readBack, err := client.ReadProviderLink(context.Background(), meta, link.LinkID)
	if err != nil || readBack.SourceVersion != active.SourceVersion {
		t.Fatalf("Fleet read-back=%+v err=%v", readBack, err)
	}
	if readBack.OrganizationID != organizationID || readBack.ProviderID != providerID {
		t.Fatal(fmt.Sprintf("data-minimized authority mismatch: %+v", readBack))
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO hauling_organization_truck_links
		(organization_id,truck_id,state) VALUES($1,$2,'ACTIVE')`, organizationID, truckID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO hauling_provider_availability
		(provider_id,state,shift_open,changed_by) VALUES($1,'AVAILABLE',true,'blackbox')
		ON CONFLICT(provider_id) DO UPDATE SET state='AVAILABLE',shift_open=true`, providerID); err != nil {
		t.Fatal(err)
	}

	privateClient := platformclients.NewPrivateExecutionClient(platformclients.New(platformclients.Options{
		ServiceName: "fleet-partner-service", BaseURL: baseURL, Secret: []byte(secret),
		Timeout: 5 * time.Second, Metrics: observability.NewMetrics(),
	}), "/api/v1/hauling/internal/private-executions")
	privateMeta := platformclients.CallMeta{OrganizationID: organizationID, RequestID: operationID,
		CorrelationID: uuid.NewString(), CausationID: uuid.NewString(), IdempotencyKey: "hauling-private-" + operationID}
	created, err := privateClient.Create(context.Background(), privateMeta, platformclients.PrivateExecutionRequest{
		OrganizationID: organizationID, SettlementOrganizationID: uuid.NewString(),
		ExecutingProviderID: providerID, OrganizationMembershipID: membershipID,
		PrivateOrderID: orderID, ShipmentID: shipmentID, PrivateAssignmentID: assignmentID,
		ProviderResponseID: responseID, FleetOperationID: operationID,
		ServiceCategory: "HAULING", TruckID: truckID, CorrelationID: privateMeta.CorrelationID,
		Route: platformclients.PrivateRouteSnapshot{PickupAddress: "blackbox pickup", DropoffAddress: "blackbox dropoff"},
		Cargo: platformclients.PrivateCargoSnapshot{CargoType: "general", WeightKG: 100},
	})
	if err != nil || created.Status != "CONFIRMED" || created.CanonicalBookingID == "" {
		t.Fatalf("Fleet create private Hauling execution=%+v err=%v\n%s", created, err, logs.String())
	}
	privateMeta.IdempotencyKey = ""
	readExecution, err := privateClient.Read(context.Background(), privateMeta, operationID)
	if err != nil || readExecution.CanonicalBookingID != created.CanonicalBookingID {
		t.Fatalf("Fleet private Hauling readback=%+v err=%v", readExecution, err)
	}
	privateMeta.IdempotencyKey = "hauling-private-" + operationID + ":cancel"
	cancelled, err := privateClient.Cancel(context.Background(), privateMeta, operationID, "blackbox cancellation")
	if err != nil || cancelled.Status != "CANCELLED" || cancelled.SourceVersion != 2 {
		t.Fatalf("Fleet private Hauling cancellation=%+v err=%v", cancelled, err)
	}
	privateMeta.IdempotencyKey = ""
	cancelReadback, err := privateClient.ReadCancellation(context.Background(), privateMeta, operationID)
	if err != nil || cancelReadback.Status != "CANCELLED" {
		t.Fatalf("Fleet private Hauling cancellation readback=%+v err=%v", cancelReadback, err)
	}
}
