package inventory

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSection14AActorIsTenantScoped(t *testing.T) {
	valid := Actor{
		StaffID:    "staff-a",
		BusinessID: "business-a",
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("valid actor: %v", err)
	}

	missingTenant := Actor{
		StaffID: "staff-a",
	}

	if err := missingTenant.Validate(); err == nil {
		t.Fatal("actor without tenant was accepted")
	}

	missingIdentity := Actor{
		BusinessID: "business-a",
	}

	if err := missingIdentity.Validate(); err == nil {
		t.Fatal("actor without identity was accepted")
	}
}

func TestSection14ABranchScopeCannotEscape(t *testing.T) {
	scope, err := ExactBranchScope(
		"business-a",
		"branch-a",
	)
	if err != nil {
		t.Fatalf("ExactBranchScope: %v", err)
	}

	if !scope.Covers(
		"business-a",
		"branch-a",
	) {
		t.Fatal("legitimate branch rejected")
	}

	if scope.Covers(
		"business-a",
		"branch-b",
	) {
		t.Fatal("scope escaped to another branch")
	}

	if scope.Covers(
		"business-b",
		"branch-a",
	) {
		t.Fatal("scope escaped to another business")
	}
}

func TestSection14ABusinessScopeCannotCrossTenant(t *testing.T) {
	scope, err := BusinessScope("business-a")
	if err != nil {
		t.Fatalf("BusinessScope: %v", err)
	}

	if !scope.Covers(
		"business-a",
		"branch-a",
	) {
		t.Fatal("legitimate branch rejected")
	}

	if scope.Covers(
		"business-b",
		"branch-a",
	) {
		t.Fatal("business scope escaped tenant")
	}
}

func TestSection14AFlagsFailClosed(t *testing.T) {
	actor := Actor{
		StaffID:    "staff-a",
		BusinessID: "business-a",
	}

	scope, err := BusinessScope("business-a")
	if err != nil {
		t.Fatalf("BusinessScope: %v", err)
	}

	disabled := NewService(Dependencies{})

	err = disabled.RequireRead(
		context.Background(),
		actor,
		scope,
	)

	if !errors.Is(err, ErrFeatureDisabled) {
		t.Fatalf(
			"RequireRead = %v; want ErrFeatureDisabled",
			err,
		)
	}

	readOnly := NewService(
		Dependencies{
			Flags: Flags{
				Enabled: true,
			},
		},
	)

	err = readOnly.RequireWrite(
		context.Background(),
		actor,
		scope,
	)

	if !errors.Is(err, ErrWritesDisabled) {
		t.Fatalf(
			"RequireWrite = %v; want ErrWritesDisabled",
			err,
		)
	}

	enabled := NewService(
		Dependencies{
			Flags: Flags{
				Enabled:       true,
				WritesEnabled: true,
			},
		},
	)

	if err := enabled.RequireWrite(
		context.Background(),
		actor,
		scope,
	); err != nil {
		t.Fatalf("enabled write: %v", err)
	}
}

func TestSection14AActorCannotUseForeignBusinessScope(t *testing.T) {
	service := NewService(
		Dependencies{
			Flags: Flags{
				Enabled: true,
			},
		},
	)

	scope, err := BusinessScope("business-b")
	if err != nil {
		t.Fatalf("BusinessScope: %v", err)
	}

	err = service.RequireRead(
		context.Background(),
		Actor{
			StaffID:    "staff-a",
			BusinessID: "business-a",
		},
		scope,
	)

	if !errors.Is(err, ErrScopeViolation) {
		t.Fatalf(
			"RequireRead = %v; want ErrScopeViolation",
			err,
		)
	}
}

func TestSection14AFixedClock(t *testing.T) {
	want := time.Date(
		2026,
		time.August,
		17,
		22,
		0,
		0,
		0,
		time.FixedZone("test", 2*60*60),
	)

	clock := FixedClock{Time: want}

	a := clock.Now()
	b := clock.Now()

	if !a.Equal(b) {
		t.Fatal("fixed clock changed")
	}

	if !a.Equal(want.UTC()) {
		t.Fatalf(
			"Now = %v; want %v",
			a,
			want.UTC(),
		)
	}
}

type section14AFakeResolver struct{}

func (section14AFakeResolver) ResolveInventorySource(
	_ context.Context,
	_ Actor,
	_ BranchScope,
	source SourceReference,
) (ResolvedInventorySource, error) {
	return ResolvedInventorySource{
		Source:     source,
		BusinessID: "business-a",
		Active:     true,
	}, nil
}

func TestSection14APortsDoNotRequireShop(t *testing.T) {
	var resolver InventoryItemResolver = section14AFakeResolver{}

	result, err := resolver.ResolveInventorySource(
		context.Background(),
		Actor{
			StaffID:    "staff-a",
			BusinessID: "business-a",
		},
		BranchScope{
			BusinessID: "business-a",
		},
		SourceReference{
			Type: "TEST",
			Ref:  "source-1",
		},
	)
	if err != nil {
		t.Fatalf("ResolveInventorySource: %v", err)
	}

	if result.Source.Ref != "source-1" {
		t.Fatalf(
			"source ref = %q",
			result.Source.Ref,
		)
	}
}
