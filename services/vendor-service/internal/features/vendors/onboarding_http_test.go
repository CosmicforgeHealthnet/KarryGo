package vendors

import (
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestRegisterOnboardingRoutesExposesExactSection7PublicSurface(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterOnboardingRoutes(router.Group("/api/v1/vendor"), &Handler{})
	count := 0
	for _, route := range router.Routes() {
		if len(route.Path) >= 25 && route.Path[:25] == "/api/v1/vendor/onboarding" {
			count++
		}
	}
	if count != 19 {
		t.Fatalf("Section 7 public route count=%d want 19", count)
	}
}

func TestVerificationStatusVocabularyAndInternalCallbackRoute(t *testing.T) {
	if !validVerificationState(VerificationPendingReview) {
		t.Fatal("PENDING_REVIEW must be a valid VerificationStatus")
	}
	if validVerificationState(VerificationState("PENDING")) {
		t.Fatal("plain PENDING must not be accepted as a VerificationStatus")
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterInternalOnboardingRoutes(router, &Handler{}, serviceauth.NewVerifier(serviceauth.Secrets{"verification-compliance-service": []byte("secret")}, 5*time.Minute))
	for _, route := range router.Routes() {
		if route.Method == "POST" && route.Path == "/internal/vendor/v1/verification-events" {
			return
		}
		if route.Path == "/api/v1/vendor/internal/vendor/v1/verification-events" {
			t.Fatal("callback route was accidentally mounted below the public Vendor API base")
		}
	}
	t.Fatal("root-level internal Vendor verification callback route is missing")
}
func TestSection7WritableDTOsRejectVerificationAuthoritySpoofing(t *testing.T) {
	for _, raw := range [][]byte{[]byte(`{"status":"VERIFIED","subject_type":"BUSINESS","subject_ref":"v","document_refs":["d"]}`), []byte(`{"source_version":99,"account_number":"123456","bank_code":"058","step_up_grant_id":"g"}`), []byte(`{"business_id":"other","capacity":"OWNER","first_name":"A","last_name":"B","email":"a@example.test","phone":"1234567","authorized":true,"expected_version":0}`)} {
		var target any
		if string(raw[2:8]) == "status" {
			target = &verificationRequest{}
		} else if string(raw[2:8]) == "source" {
			target = &bankProfileRequest{}
		} else {
			target = &representativeRequest{}
		}
		if err := strict(raw, target); err == nil {
			t.Fatalf("authority field accepted: %s", raw)
		}
	}
}
