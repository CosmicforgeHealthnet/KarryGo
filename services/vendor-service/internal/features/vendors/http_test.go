package vendors

import (
	"context"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type denyAll struct{}

func (denyAll) Authorize(context.Context, string, string, *string) (bool, error) { return false, nil }

type branchAOnly struct{}

func (branchAOnly) Authorize(_ context.Context, _ string, _ string, branchID *string) (bool, error) {
	return branchID != nil && *branchID == "branch-a", nil
}

func TestRegisterRoutesExposesCompleteSection6Surface(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterRoutes(r.Group("/api/v1/vendor"), NewHandler(nil, nil, true, false))
	routes := r.Routes()
	if len(routes) != 32 {
		t.Fatalf("registered %d Section 6 routes, want 32", len(routes))
	}
	want := map[string]bool{"GET /api/v1/vendor/capability-types": true, "POST /api/v1/vendor/businesses": true, "PUT /api/v1/vendor/businesses/:businessId/branches/:branchId/capabilities/:capabilityId": true}
	for _, route := range routes {
		delete(want, route.Method+" "+route.Path)
	}
	if len(want) != 0 {
		t.Fatalf("missing representative routes: %v", want)
	}
}

func TestAuthorizationIsDenyByDefaultBeforePersistence(t *testing.T) {
	s := NewService((*pgxpool.Pool)(nil), denyAll{}, nil, nil)
	actor := Actor{StaffID: "staff", VendorID: "business"}
	if _, err := s.ListBusinesses(context.Background(), actor); err == nil {
		t.Fatal("business read should be denied")
	}
	if _, err := s.CreateCapability(context.Background(), actor, "business", CapabilityRestaurant); err == nil {
		t.Fatal("capability write should be denied")
	}
	if _, err := s.GetBranch(context.Background(), actor, "business", "branch"); err == nil {
		t.Fatal("branch read should be denied")
	}
}

func TestBranchScopeCannotBeEscalatedByPath(t *testing.T) {
	s := NewService((*pgxpool.Pool)(nil), branchAOnly{}, nil, nil)
	actor := Actor{StaffID: "manager", VendorID: "business"}
	if _, err := s.GetBranch(context.Background(), actor, "business", "branch-b"); err == nil {
		t.Fatal("branch A manager accessed branch B")
	}
}

func TestWritableDTOsRejectAuthoritySpoofing(t *testing.T) {
	for _, raw := range []string{`{"legal_name":"Valid Business","currency":"NGN","timezone":"Africa/Lagos","vendor_id":"spoof"}`, `{"legal_name":"Valid Business","currency":"NGN","timezone":"Africa/Lagos","status":"ACTIVE"}`, `{"display_name":"Brand","verification_state":"VERIFIED"}`} {
		var dst any
		if strings.Contains(raw, "display_name") {
			dst = &brandRequest{}
		} else {
			dst = &createBusinessRequest{}
		}
		if strict([]byte(raw), dst) == nil {
			t.Fatalf("authority field accepted: %s", raw)
		}
	}
}
