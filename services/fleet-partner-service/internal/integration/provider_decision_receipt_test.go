package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	mkthttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/http"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/providerreceipt"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
)

func TestProviderDecisionReceiptHistoricalIdentityAndIsolation(t *testing.T) {
	f := newAckFixture(t, "Receipt owner", "RC-RECEIPT-1", false)
	ctx := context.Background()
	reason, note := "UNSAFE_VEHICLE", "PRIVATE_DECISION_NOTE"
	response := &mktmodels.Response{OrganizationID: f.orgID, AssignmentID: f.assignment.ID, MembershipID: f.membership.ID, Response: mktmodels.ResponseDeclined, ClientResponseID: "original-decision", ReasonCode: &reason, ReasonText: &note, EvidenceFileIDs: []string{"private-evidence-b", "private-evidence-a"}}
	if err := f.repo.CreateResponse(ctx, f.pool, response); err != nil {
		t.Fatal(err)
	}
	// Fixture represents subsequent lifecycle evolution; no runtime mutation is
	// exposed by the receipt endpoint. The decision must still be readable.
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_provider_assignments SET status='CANCELLED',version=version+1 WHERE id=$1`, f.assignment.ID); err != nil {
		t.Fatal(err)
	}
	receipt, err := f.service.ProviderDecisionReceipt(ctx, f.assignment.ID, f.assignment.ExecutingProviderID, mktmodels.SourceDispatch, response.ClientResponseID)
	if err != nil {
		t.Fatal(err)
	}
	want := (providerreceipt.Intent{AssignmentID: f.assignment.ID, ProviderID: f.assignment.ExecutingProviderID, ClientResponseID: response.ClientResponseID, Decision: mktmodels.ResponseDeclined, ReasonCode: reason, Note: note, Evidence: []string{"private-evidence-a", "private-evidence-b"}}).Digest()
	if receipt.IntentDigest != want || receipt.ContractVersion != providerreceipt.Version || receipt.ReceiptID != response.ID || receipt.RecordedAt.IsZero() {
		t.Fatalf("incorrect durable receipt: %+v", receipt)
	}
	if _, err = f.service.VerifyProviderAcceptance(ctx, f.assignment.ID); err == nil {
		t.Fatal("historical receipt was confused with execution permission")
	}
	t.Run("same-key-cannot-change-decision", func(t *testing.T) {
		if _, err := f.service.AcceptAssignmentForProvider(ctx, mktusecases.AcceptAssignmentInput{Actor: f.providerActor(), AssignmentID: f.assignment.ID, ClientResponseID: response.ClientResponseID}); err == nil {
			t.Fatal("decline was replayed as acceptance")
		}
		for _, change := range []string{"reason", "note", "evidence"} {
			input := mktusecases.DeclineAssignmentInput{Actor: f.providerActor(), AssignmentID: f.assignment.ID, ClientResponseID: response.ClientResponseID, ReasonCode: reason, Note: note, EvidenceFileIDs: response.EvidenceFileIDs}
			switch change {
			case "reason":
				input.ReasonCode = "ALREADY_BUSY"
			case "note":
				input.Note = "changed"
			case "evidence":
				input.EvidenceFileIDs = []string{"new-file"}
			}
			if _, err := f.service.DeclineAssignmentForProvider(ctx, input); err == nil {
				t.Fatalf("changed %s replayed as original decision", change)
			}
		}
		replay, err := f.service.DeclineAssignmentForProvider(ctx, mktusecases.DeclineAssignmentInput{Actor: f.providerActor(), AssignmentID: f.assignment.ID, ClientResponseID: response.ClientResponseID, ReasonCode: reason, Note: note, EvidenceFileIDs: []string{"private-evidence-a", "private-evidence-b", "private-evidence-a"}})
		if err != nil || !replay.Replayed {
			t.Fatalf("same semantic decision cannot read original outcome: %+v %v", replay, err)
		}
	})
	for _, test := range []struct{ name, assignment, provider, service, key string }{
		{"foreign-provider", f.assignment.ID, "another-provider", mktmodels.SourceDispatch, response.ClientResponseID},
		{"foreign-service", f.assignment.ID, f.assignment.ExecutingProviderID, mktmodels.SourceTaxi, response.ClientResponseID},
		{"fresh-key", f.assignment.ID, f.assignment.ExecutingProviderID, mktmodels.SourceDispatch, "fresh-key"},
		{"unmapped-history", "missing-assignment", f.assignment.ExecutingProviderID, mktmodels.SourceDispatch, response.ClientResponseID},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := f.service.ProviderDecisionReceipt(ctx, test.assignment, test.provider, test.service, test.key); err == nil {
				t.Fatal("unowned or unrecorded decision disclosed")
			}
		})
	}
	secondOrg := createOrganization(t, f.pool, "Other receipt tenant", "RC-RECEIPT-2")
	secondProjection := seedProjection(t, f.pool, secondOrg.ID, "receipt-other-request", mktmodels.ProjectionOrganizationAccepted)
	secondMembership := seedProviderMembership(t, f.pool, secondOrg.ID, "receipt-other-provider")
	secondAssignment := seedAssignment(t, f.pool, secondOrg.ID, secondProjection.ID, secondMembership, time.Now().Add(time.Hour))
	if _, err := f.service.ProviderDecisionReceipt(ctx, secondAssignment.ID, f.assignment.ExecutingProviderID, mktmodels.SourceDispatch, response.ClientResponseID); err == nil {
		t.Fatal("receipt crossed canonical organization/provider binding")
	}
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_assignment_responses SET client_response_id='rewrite' WHERE id=$1`, response.ID); err == nil {
		t.Fatal("immutable decision could be rewritten")
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	secret := []byte("receipt-test-only-hmac-secret-32-bytes")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"driver-dispatch-delivery-service": secret, "driver-taxi-service": secret, "customer-service": secret}, time.Minute)
	group := engine.Group("/internal/fleet-partner/v1", func(c *gin.Context) {
		caller, err := verifier.VerifyRequest(c.Request)
		if err != nil {
			c.AbortWithStatus(401)
			return
		}
		c.Set("service_name", caller)
		c.Next()
	})
	mkthttp.RegisterInternalMarketplaceRoutes(group, mkthttp.NewInternalHandler(f.service, []string{"driver-dispatch-delivery-service", "driver-taxi-service"}))
	base := "/internal/fleet-partner/v1/assignments/" + f.assignment.ID + "/provider-decision-receipt?" + url.Values{"providerId": {f.assignment.ExecutingProviderID}, "clientResponseId": {response.ClientResponseID}}.Encode()
	for _, test := range []struct {
		name, caller, suffix, body string
		signed                     bool
		status                     int
	}{
		{"signed", "driver-dispatch-delivery-service", "", "", true, 200},
		{"unsigned", "driver-dispatch-delivery-service", "", "", false, 401},
		{"non-owner-service", "driver-taxi-service", "", "", true, 404},
		{"not-allowlisted", "customer-service", "", "", true, 403},
		{"duplicate-query", "driver-dispatch-delivery-service", "&providerId=other", "", true, 400},
		{"tenant-injection", "driver-dispatch-delivery-service", "&tenantId=another", "", true, 400},
		{"malformed-query", "driver-dispatch-delivery-service", "&invalid=%zz", "", true, 400},
		{"body", "driver-dispatch-delivery-service", "", "{}", true, 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, base+test.suffix, bytes.NewBufferString(test.body))
			if test.signed {
				if err := serviceauth.SignRequest(req, test.caller, secret, []byte(test.body), time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			res := httptest.NewRecorder()
			engine.ServeHTTP(res, req)
			if res.Code != test.status {
				t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
			}
			if test.status == 200 {
				var envelope struct {
					Data providerreceipt.Receipt `json:"data"`
				}
				if json.Unmarshal(res.Body.Bytes(), &envelope) != nil || envelope.Data.IntentDigest != want {
					t.Fatal("wire receipt differs")
				}
			}
			for _, private := range []string{note, "private-evidence", f.orgID, f.membership.ID} {
				if strings.Contains(res.Body.String(), private) {
					t.Fatalf("sensitive field disclosed: %s", private)
				}
			}
		})
	}
}

func TestProviderDecisionReceiptPrivateAssignmentOriginalKeyAndCanonicalProvider(t *testing.T) {
	f, private, _, assignment := privateWorkerFixture(t, "pex_private_receipt")
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `DELETE FROM fleet_partner_private_execution_operations WHERE id='pex_private_receipt';UPDATE fleet_partner_private_assignments SET status='PENDING_PROVIDER_RESPONSE' WHERE id='pas_pex_private_receipt'`); err != nil {
		t.Fatal(err)
	}
	input := privateusecases.RespondInput{ProviderID: "provider_private", Decision: "DECLINE", ReasonCode: "FATIGUE_OR_REST_REQUIRED", SafeNote: " PRIVATE_DECISION_NOTE ", IdempotencyKey: "original-private-key", RequestFingerprint: "private-receipt-fingerprint", ExpectedVersion: 1}
	if _, err := private.Respond(ctx, f.tenant.OrganizationID, assignment, input); err != nil {
		t.Fatal(err)
	}
	reader := mktusecases.NewService(mktusecases.Options{Pool: f.pool})
	receipt, err := reader.ProviderDecisionReceipt(ctx, assignment, input.ProviderID, "DISPATCH", input.IdempotencyKey)
	if err != nil {
		t.Fatal(err)
	}
	intent := providerreceipt.Intent{AssignmentID: assignment, ProviderID: input.ProviderID, ClientResponseID: input.IdempotencyKey, Decision: "DECLINED", ReasonCode: input.ReasonCode, Note: strings.TrimSpace(input.SafeNote)}
	if receipt.IntentDigest != intent.Digest() || receipt.Decision != "DECLINED" || receipt.ReceiptID == "" {
		t.Fatalf("private decision identity differs: %+v", receipt)
	}
	changed := input
	changed.IdempotencyKey = "fresh-private-key"
	if _, err := private.Respond(ctx, f.tenant.OrganizationID, assignment, changed); err == nil {
		t.Fatal("fresh key replayed the old private decision")
	}
	for _, test := range []struct{ name, provider, service, key string }{{"foreign-provider", "another-provider", "DISPATCH", input.IdempotencyKey}, {"foreign-service", input.ProviderID, "HAULING", input.IdempotencyKey}, {"fresh-key", input.ProviderID, "DISPATCH", "fresh-private-key"}} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := reader.ProviderDecisionReceipt(ctx, assignment, test.provider, test.service, test.key); err == nil {
				t.Fatal("private receipt crossed provider/service/original key boundary")
			}
		})
	}
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_private_assignments SET status='CANCELLED',version=version+1 WHERE id=$1`, assignment); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ProviderDecisionReceipt(ctx, assignment, input.ProviderID, "DISPATCH", input.IdempotencyKey); err != nil {
		t.Fatalf("later cancellation erased original private decision: %v", err)
	}
	var executions int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_private_execution_operations WHERE assignment_id=$1`, assignment).Scan(&executions); err != nil || executions != 0 {
		t.Fatalf("decline/readback created execution: %d %v", executions, err)
	}
}
