package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	mkthttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/http"
	models "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	usecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/providerreceipt"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
)

func TestProviderOfferReceiptExactImmutableDecisionAndIsolation(t *testing.T) {
	for _, decision := range []string{models.ResponseAccepted, models.ResponseDeclined} {
		t.Run(decision, func(t *testing.T) {
			pool := testPool(t)
			ctx := context.Background()
			service := newOfferService(t, pool)
			org := createOrganization(t, pool, "Offer receipt "+decision, "RC-OR-"+decision)
			projection := seedProjection(t, pool, org.ID, "request-receipt-"+decision, models.ProjectionOffered)
			provider := seedProviderMembership(t, pool, org.ID, "provider-receipt-"+decision)
			offer, recipients := seedOfferWithRecipients(t, pool, org.ID, projection.ID, []*providermodels.Membership{provider}, time.Hour)
			input := usecases.OfferResponseInput{Actor: dispatchActor(*provider.ProviderID), OfferID: offer.ID, ClientResponseID: "original-offer-key", ExpectedVersion: recipients[0].Version}
			if decision == models.ResponseDeclined {
				input.ReasonCode = models.DeclineUnsafeVehicle
				input.Note = "PRIVATE_OFFER_NOTE"
				input.EvidenceFileIDs = []string{"protected-file-reference"}
			} else {
				input.DispatchReservationID = "original-source-reservation"
				input.DispatchRequestVersion = 7
			}
			apply := service.AcceptOffer
			if decision == models.ResponseDeclined {
				apply = service.DeclineOffer
			}
			if _, err := apply(ctx, input); err != nil {
				t.Fatal(err)
			}
			receipt, err := service.ProviderOfferDecisionReceipt(ctx, offer.ID, *provider.ProviderID, models.SourceDispatch, input.ClientResponseID)
			if err != nil {
				t.Fatal(err)
			}
			intent := providerreceipt.Intent{OfferID: offer.ID, ProviderID: *provider.ProviderID, ClientResponseID: input.ClientResponseID, Decision: decision, ReasonCode: input.ReasonCode, Note: input.Note, Evidence: input.EvidenceFileIDs}
			if decision == models.ResponseAccepted {
				intent.ReservationID = input.DispatchReservationID
				intent.RequestVersion = input.DispatchRequestVersion
			}
			if receipt.IntentDigest != intent.Digest() || receipt.OfferID != offer.ID || receipt.AssignmentID != "" || receipt.ContractVersion != providerreceipt.OfferVersion {
				t.Fatalf("incorrect offer proof: %+v", receipt)
			}
			gin.SetMode(gin.TestMode)
			engine := gin.New()
			engine.Use(httpx.ErrorHandler())
			secret := []byte("test-offer-owner-receipt-secret")
			verifier := serviceauth.NewVerifier(serviceauth.Secrets{"driver-dispatch-delivery-service": secret, "driver-hauling-service": secret}, time.Minute)
			group := engine.Group("/internal/fleet-partner/v1", func(c *gin.Context) {
				caller, err := verifier.VerifyRequest(c.Request)
				if err != nil {
					c.AbortWithStatus(401)
					return
				}
				c.Set("service_name", caller)
				c.Next()
			})
			mkthttp.RegisterInternalMarketplaceRoutes(group, mkthttp.NewInternalHandler(service, []string{"driver-dispatch-delivery-service", "driver-hauling-service"}))
			for _, test := range []struct {
				name, caller, extra string
				signed              bool
				status              int
			}{{"signed", "driver-dispatch-delivery-service", "", true, 200}, {"unsigned", "driver-dispatch-delivery-service", "", false, 401}, {"foreign-service", "driver-hauling-service", "", true, 404}, {"tenant-injection", "driver-dispatch-delivery-service", "&tenantId=another", true, 400}, {"duplicate-provider", "driver-dispatch-delivery-service", "&providerId=another", true, 400}} {
				t.Run("http/"+test.name, func(t *testing.T) {
					path := "/internal/fleet-partner/v1/marketplace-offers/" + offer.ID + "/provider-decision-receipt?" + url.Values{"providerId": {*provider.ProviderID}, "clientResponseId": {input.ClientResponseID}}.Encode() + test.extra
					req := httptest.NewRequest(http.MethodGet, path, nil)
					if test.signed {
						if err := serviceauth.SignRequest(req, test.caller, secret, nil, time.Now()); err != nil {
							t.Fatal(err)
						}
					}
					res := httptest.NewRecorder()
					engine.ServeHTTP(res, req)
					if res.Code != test.status {
						t.Fatalf("%d %s", res.Code, res.Body.String())
					}
					if strings.Contains(res.Body.String(), "PRIVATE_OFFER_NOTE") || strings.Contains(res.Body.String(), "protected-file-reference") {
						t.Fatal("HTTP offer receipt disclosed private evidence")
					}
				})
			}
			raw, _ := json.Marshal(receipt)
			for _, private := range []string{input.Note, "protected-file-reference", org.ID, provider.ID} {
				if private != "" && strings.Contains(string(raw), private) {
					t.Fatalf("offer receipt disclosed private source material: %s", private)
				}
			}
			changes := []string{"provider", "service", "note", "evidence", "fresh-key"}
			if decision == models.ResponseAccepted {
				changes = append(changes, "reservation", "request-version")
			}
			for _, change := range changes {
				t.Run(change, func(t *testing.T) {
					changed := input
					switch change {
					case "provider":
						changed.Actor.ExecutingProviderID = "another-provider"
					case "service":
						changed.Actor.SourceService = models.SourceHauling
					case "note":
						changed.Note = "changed-note"
					case "evidence":
						changed.EvidenceFileIDs = []string{"another-file"}
					case "fresh-key":
						changed.ClientResponseID = "new-key"
					case "reservation":
						changed.DispatchReservationID = "another-reservation"
					case "request-version":
						changed.DispatchRequestVersion++
					}
					if _, err := apply(ctx, changed); err == nil {
						t.Fatal("changed offer intent replayed original decision")
					}
				})
			}
			replay, err := apply(ctx, input)
			if err != nil || !replay.Replayed || replay.Status != decision {
				t.Fatalf("original offer intent did not replay: %+v %v", replay, err)
			}
			opposite := input
			if decision == models.ResponseAccepted {
				opposite.ReasonCode = models.DeclineUnsafeVehicle
				if _, err := service.DeclineOffer(ctx, opposite); err == nil {
					t.Fatal("acceptance replayed as decline")
				}
			} else {
				if _, err := service.AcceptOffer(ctx, opposite); err == nil {
					t.Fatal("decline replayed as acceptance")
				}
			}
			for _, bad := range []struct{ provider, service, key string }{{"another-provider", models.SourceDispatch, input.ClientResponseID}, {*provider.ProviderID, models.SourceHauling, input.ClientResponseID}, {*provider.ProviderID, models.SourceDispatch, "fresh-key"}} {
				if _, err := service.ProviderOfferDecisionReceipt(ctx, offer.ID, bad.provider, bad.service, bad.key); err == nil {
					t.Fatal("offer receipt crossed original identity boundary")
				}
			}
			for _, sql := range []string{`UPDATE fleet_partner_provider_offer_decision_receipts SET intent_digest=repeat('a',64)`, `DELETE FROM fleet_partner_provider_offer_decision_receipts`, `TRUNCATE fleet_partner_provider_offer_decision_receipts`} {
				if _, err := pool.Exec(ctx, sql); err == nil {
					t.Fatal("offer decision proof mutable")
				}
			}
			if _, err := pool.Exec(ctx, `UPDATE fleet_partner_organization_request_offers SET status='CANCELLED',version=version+1 WHERE id=$1`, offer.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := service.ProviderOfferDecisionReceipt(ctx, offer.ID, *provider.ProviderID, models.SourceDispatch, input.ClientResponseID); err != nil {
				t.Fatalf("later lifecycle erased original offer decision: %v", err)
			}
		})
	}
}

func TestProviderOfferReceiptFailureRollsBackNativeAcceptance(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newOfferService(t, pool)
	org := createOrganization(t, pool, "Receipt rollback", "RC-RECEIPT-ROLLBACK")
	projection := seedProjection(t, pool, org.ID, "rollback-receipt-request", models.ProjectionOffered)
	provider := seedProviderMembership(t, pool, org.ID, "rollback-receipt-provider")
	offer, recipients := seedOfferWithRecipients(t, pool, org.ID, projection.ID, []*providermodels.Membership{provider}, time.Hour)
	if _, err := pool.Exec(ctx, `CREATE FUNCTION test_offer_receipt_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test-only receipt failure'; END $$;CREATE TRIGGER test_offer_receipt_failure BEFORE INSERT ON fleet_partner_provider_offer_decision_receipts FOR EACH ROW EXECUTE FUNCTION test_offer_receipt_failure()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS test_offer_receipt_failure ON fleet_partner_provider_offer_decision_receipts;DROP FUNCTION IF EXISTS test_offer_receipt_failure()`)
	})
	if _, err := service.AcceptOffer(ctx, usecases.OfferResponseInput{Actor: dispatchActor(*provider.ProviderID), OfferID: offer.ID, ClientResponseID: "rollback-key", ExpectedVersion: recipients[0].Version}); err == nil {
		t.Fatal("receipt failure acknowledged acceptance")
	}
	var status string
	var assignments int
	if err := pool.QueryRow(ctx, `SELECT status FROM fleet_partner_provider_offer_recipients WHERE id=$1`, recipients[0].ID).Scan(&status); err != nil || status != models.RecipientPending {
		t.Fatalf("native recipient mutation escaped rollback: %s %v", status, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_provider_assignments WHERE offer_id=$1`, offer.ID).Scan(&assignments); err != nil || assignments != 0 {
		t.Fatalf("native assignment escaped rollback: %d %v", assignments, err)
	}
}

func TestProviderOfferLegacyMutableDecisionCannotInventReceipt(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newOfferService(t, pool)
	org := createOrganization(t, pool, "Legacy receipt fence", "RC-LEGACY-RECEIPT")
	projection := seedProjection(t, pool, org.ID, "legacy-receipt-request", models.ProjectionOffered)
	provider := seedProviderMembership(t, pool, org.ID, "legacy-receipt-provider")
	offer, recipients := seedOfferWithRecipients(t, pool, org.ID, projection.ID, []*providermodels.Membership{provider}, time.Hour)
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_provider_offer_recipients SET status='DECLINED',client_response_id='legacy-key',reason_code='UNSAFE_VEHICLE',responded_at=now(),version=version+1 WHERE id=$1`, recipients[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ProviderOfferDecisionReceipt(ctx, offer.ID, *provider.ProviderID, models.SourceDispatch, "legacy-key"); err == nil {
		t.Fatal("mutable legacy fields were presented as immutable decision proof")
	}
	if _, err := service.DeclineOffer(ctx, usecases.OfferResponseInput{Actor: dispatchActor(*provider.ProviderID), OfferID: offer.ID, ClientResponseID: "legacy-key", ReasonCode: models.DeclineUnsafeVehicle}); err == nil {
		t.Fatal("legacy decision replayed without original immutable intent")
	}
	var receipts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_provider_offer_decision_receipts WHERE offer_id=$1`, offer.ID).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("legacy proof invented: %d %v", receipts, err)
	}
}
