package managedmembers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestMemberAndAssetListsPageWithoutTruncation covers spec 6/36: the member
// list used to stop at 100 rows and the asset list had no LIMIT at all. Both
// now page by keyset on (timestamp DESC, id DESC). The test clock is fixed, so
// every row shares one timestamp and only the id tie-breaker keeps pages
// disjoint -- the case a timestamp-only cursor would get wrong.
func TestMemberAndAssetListsPageWithoutTruncation(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	seedOrganizations(t, pool, "org-page-1", "org-page-2")
	service := newTestService(pool, stubAuthority{})
	scope := tenantOf("org-page-1")

	var memberID string
	for i := 0; i < 5; i++ {
		member, err := service.CreateMember(ctx, scope, fmt.Sprintf("EMP-PAGE-%d", i), "Member")
		if err != nil {
			t.Fatal(err)
		}
		memberID = member.ID
		if _, err = service.LinkAsset(ctx, scope, LinkAssetRequest{Authority: AuthorityHauling,
			AssetID: fmt.Sprintf("truck-page-%d", i), MemberID: member.ID, SourceVersion: 1}); err != nil {
			t.Fatal(err)
		}
	}
	// Another organization's rows never appear on these pages.
	if _, err := service.CreateMember(ctx, tenantOf("org-page-2"), "EMP-OTHER", "Other"); err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	page := Page{Limit: 2}
	for round := 0; ; round++ {
		members, more, err := service.ListMembersPage(ctx, scope, page)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range members {
			if seen[m.ID] || m.OrganizationID != "org-page-1" {
				t.Fatalf("member %s repeated or foreign (org %s)", m.ID, m.OrganizationID)
			}
			seen[m.ID] = true
		}
		if !more {
			break
		}
		if round > 5 {
			t.Fatal("member pages never ended")
		}
		last := members[len(members)-1]
		page = Page{Limit: 2, After: &last.CreatedAt, AfterID: last.ID}
	}
	if len(seen) != 5 {
		t.Fatalf("paged %d members, want 5", len(seen))
	}

	seenAssets := map[string]bool{}
	page = Page{Limit: 2}
	for round := 0; ; round++ {
		assets, more, err := service.ListAssetsPage(ctx, scope, "", page)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range assets {
			if seenAssets[a.ID] {
				t.Fatalf("asset %s repeated", a.ID)
			}
			seenAssets[a.ID] = true
		}
		if !more {
			break
		}
		if round > 5 {
			t.Fatal("asset pages never ended")
		}
		last := assets[len(assets)-1]
		page = Page{Limit: 2, After: &last.LinkedAt, AfterID: last.ID}
	}
	if len(seenAssets) != 5 {
		t.Fatalf("paged %d assets, want 5", len(seenAssets))
	}
	one, more, err := service.ListAssetsPage(ctx, scope, memberID, Page{})
	if err != nil || len(one) != 1 || more {
		t.Fatalf("member filter = %d more=%v %v", len(one), more, err)
	}
}

// TestAuthorityRefusalUsesTheStandardErrorEnvelope pins spec 6: the refusal
// used to be written as {"success":false,"error":{"code":"AUTHORITY_REFUSED"}},
// the only error on the corporate surface without a requestId or details.
func TestAuthorityRefusalUsesTheStandardErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/fleet-partner/v1/managed-members/x/links", nil)
	write(c, nil, ErrAuthorityRefused)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", recorder.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, legacy := body["success"]; legacy {
		t.Fatalf("legacy envelope field present: %s", recorder.Body.String())
	}
	envelope, ok := body["error"].(map[string]any)
	if !ok || envelope["code"] != "authority_refused" || envelope["message"] == "" {
		t.Fatalf("error envelope = %s", recorder.Body.String())
	}
	for _, key := range []string{"details", "requestId", "retryable"} {
		if _, present := envelope[key]; !present {
			t.Fatalf("error envelope missing %s: %s", key, recorder.Body.String())
		}
	}
}
