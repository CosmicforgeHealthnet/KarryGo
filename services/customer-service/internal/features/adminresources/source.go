package adminresources

import (
	"context"
	"cosmicforge/logistics/shared/go/adminowner"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Owner = "customer-service"

func Source() *adminowner.SQLSource {
	return adminowner.NewSQLSource(Owner, map[string]adminowner.SQLSpec{
		"customers":          {Table: "customers", StateColumn: "status", Dimensions: map[string]string{"onboarding": "onboarding_status", "verification": "", "analytics_consent": "", "marketing_consent": ""}},
		"booking-references": {Table: "customer_movement_history_projection", Predicate: "EXISTS(SELECT 1 FROM customers c WHERE c.id=t.customer_id)", ReferenceExpression: "t.authority||':'||t.source_ref", VersionColumn: "source_version", StateColumn: "state", BindingColumns: []string{"customer_id", "authority", "source_ref"}, Dimensions: map[string]string{"authority": "authority", "product": "product_code", "movement_type": "movement_type"}, References: map[string]string{"customer": "customer_id", "booking": "source_ref", "mission": "parent_context_ref"}},
		// These are the actual trusted-contact sharing switches, not proof of
		// legal consent or authority to reveal names/contact/channel details.
		"consents": {VersionColumn: "version", Table: "emergency_contacts", StateColumn: "", BindingColumns: []string{"customer_id"}, Dimensions: map[string]string{"active": "active", "trip_sharing_enabled": "trip_sharing_enabled", "emergency_alerts_enabled": "emergency_alerts_enabled"}, References: map[string]string{"customer": "customer_id"}},
	})
}
func Runtime(ctx context.Context, db *pgxpool.Pool) (*adminowner.Handler, error) {
	profile, _ := adminowner.ForOwner(Owner)
	return adminowner.FromEnvironment(ctx, db, Source(), profile.Definitions, Owner, profile.Prefix, profile.Path)
}
