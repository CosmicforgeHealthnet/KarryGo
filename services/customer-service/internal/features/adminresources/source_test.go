package adminresources

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/adminowner"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"github.com/jackc/pgx/v5"
)

func TestActualOwnerMigrationsAndSourceDictionary(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	source := Source()
	// Every primary projection must compile against the actual migrated owner
	// schema. An unknown record remains absent; it never becomes a global record.
	for kind, spec := range source.Specs {
		t.Run(kind, func(t *testing.T) {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := source.Read(ctx, tx, kind, "00000000-0000-4000-8000-000000000000", false); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("absent source record: %v", err)
			}
			if spec.DimensionQuery != "" {
				var raw []byte
				if err := tx.QueryRow(ctx, spec.DimensionQuery, "00000000-0000-4000-8000-000000000000").Scan(&raw); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	if _, err := db.Exec(ctx, `INSERT INTO customers(id,email,first_name) VALUES('11111111-1111-4111-8111-111111111111','admin-owner-test@example.invalid','PRIVATE_NAME')`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	resource, err := source.Read(ctx, tx, "customers", "11111111-1111-4111-8111-111111111111", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := adminowner.ValidateResource(resource, source.Specs["customers"].Definition); err != nil {
		t.Fatal(err)
	}
	if resource.SourceVersion != nil {
		t.Fatal("incomplete lifecycle counter presented as resource version")
	}
	raw, _ := json.Marshal(resource)
	if strings.Contains(string(raw), "PRIVATE_") {
		t.Fatal("source private fields leaked")
	}
	binding, err := source.Binding(ctx, tx, "customers", resource.Reference)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Canonical.Organization != "" || binding.Canonical.Tenant != "" || binding.Canonical.Market != "" || binding.Canonical.Zone != "" {
		t.Fatal("provider geography/tenant inferred without source authority")
	}
}

func TestCustomerProjectionCannotAuthorizeOrphanCustomerReference(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	source := Source()
	if _, err := db.Exec(ctx, `INSERT INTO customer_movement_history_projection(customer_id,authority,source_ref,source_version,movement_type,state,last_event_id,last_event_at) VALUES('22222222-2222-4222-8222-222222222222','DISPATCH','orphan-movement',1,'DISPATCH','assigned','test-event',now())`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := source.Binding(ctx, tx, "booking-references", "DISPATCH:orphan-movement"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("orphan projection supplied customer authority: %v", err)
	}
}

// Section 17: the marketing_consent dimension reflects the consent authority
// and stays unknown (null) when nothing was ever recorded.
func TestCustomerMarketingConsentDimension(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	source := Source()
	const id = "33333333-3333-4333-8333-333333333333"
	if _, err := db.Exec(ctx, `INSERT INTO customers(id,email) VALUES($1,'mc-dimension@example.invalid')`, id); err != nil {
		t.Fatal(err)
	}
	read := func() *string {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		resource, err := source.Read(ctx, tx, "customers", id, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range resource.Dimensions {
			if d.Name == "marketing_consent" {
				return d.Value
			}
		}
		t.Fatal("marketing_consent dimension missing")
		return nil
	}
	if v := read(); v != nil {
		t.Fatalf("unrecorded consent presented as %q", *v)
	}
	if _, err := db.Exec(ctx, `INSERT INTO customer_marketing_consents(customer_id,channel,status,version,captured_at,withdrawn_at,capture_source) VALUES($1,'email','WITHDRAWN',1,now(),now(),'CUSTOMER_APP')`, id); err != nil {
		t.Fatal(err)
	}
	if v := read(); v == nil || *v != "WITHDRAWN" {
		t.Fatalf("withdrawn consent: %v", v)
	}
	if _, err := db.Exec(ctx, `INSERT INTO customer_marketing_consents(customer_id,channel,status,market,version,captured_at,capture_source) VALUES($1,'push','OPTED_IN','NG-LAG',1,now(),'CUSTOMER_APP')`, id); err != nil {
		t.Fatal(err)
	}
	if v := read(); v == nil || *v != "OPTED_IN" {
		t.Fatalf("opted-in consent: %v", v)
	}
}
