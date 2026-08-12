package tenant

import (
	"context"
	"testing"
)

func TestRequireFailsClosedWithoutContext(t *testing.T) {
	if _, err := Require(context.Background()); err == nil {
		t.Fatal("missing tenant context must fail closed")
	}
}

func TestRequireFailsClosedWithInvalidContext(t *testing.T) {
	ctx := Into(context.Background(), Context{ActorType: ActorTypeSystem, ActorID: "sys"})
	if _, err := Require(ctx); err == nil {
		t.Fatal("tenant context without organization must fail closed")
	}
}

func TestRoundTrip(t *testing.T) {
	original := Context{
		OrganizationID: "org_1",
		ActorType:      ActorTypeOrganizationUser,
		ActorID:        "usr_1",
	}
	resolved, err := Require(Into(context.Background(), original))
	if err != nil {
		t.Fatalf("Require() error = %v", err)
	}
	if resolved.OrganizationID != "org_1" {
		t.Fatalf("resolved = %+v", resolved)
	}
}

func TestBranchPermitted(t *testing.T) {
	all := Context{OrganizationID: "org_1", ActorType: ActorTypeSystem, ActorID: "s"}
	if !all.BranchPermitted("brn_any") {
		t.Fatal("empty permitted list means all branches")
	}
	scoped := Context{OrganizationID: "org_1", ActorType: ActorTypeSystem, ActorID: "s", PermittedBranchIDs: []string{"brn_1"}}
	if !scoped.BranchPermitted("brn_1") || scoped.BranchPermitted("brn_2") {
		t.Fatal("branch scoping not enforced")
	}
}

func TestUnconfiguredResolverFailsClosed(t *testing.T) {
	_, err := UnconfiguredResolver{}.ResolveMembership(context.Background(), "usr_1")
	if err != ErrResolverUnconfigured {
		t.Fatalf("expected ErrResolverUnconfigured, got %v", err)
	}
}
