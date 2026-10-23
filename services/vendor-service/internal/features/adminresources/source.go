package adminresources

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/vendors"
	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminowner"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

const Owner = "vendor-service"

type SourceReader struct{ *adminowner.SQLSource }

func Source() *SourceReader {
	return &SourceReader{adminowner.NewSQLSource(Owner, map[string]adminowner.SQLSpec{
		"fulfilment-units":       {Table: "commerce_fulfilment_units", StateColumn: "readiness", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id JOIN vendor_businesses b ON b.id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id,'identity',t.id::text)", Dimensions: map[string]string{"service_class": "service_class", "vehicle_class": "vehicle_class"}, Counters: map[string]string{"required_weight_grams": "required_weight_grams", "required_package_count": "required_package_count"}, References: map[string]string{"parent_order": "parent_order_id", "vendor_order": "vendor_order_id", "fulfilment_group": "group_id", "business": "business_id", "branch": "branch_id"}},
		"delivery-mission-links": {Table: "(SELECT DISTINCT r.id,r.group_id,r.plan_id,r.plan_mission_id,r.authority,r.mission_ref,r.external_ref,r.link_state,r.status_projection,r.status_source_version,r.status_updated_at,r.refusal_code,r.replaces_id,r.version,r.created_at,r.updated_at,v.id AS vendor_order_id,v.parent_order_id,v.business_id,v.branch_id FROM commerce_platform_mission_refs r JOIN commerce_fulfilment_plan_mission_parcels mp ON mp.plan_mission_id=r.plan_mission_id AND mp.plan_id=r.plan_id AND mp.group_id=r.group_id JOIN commerce_fulfilment_parcels p ON p.id=mp.parcel_id AND p.group_id=r.group_id JOIN commerce_vendor_orders v ON v.id=p.vendor_order_id AND v.business_id=p.business_id AND v.branch_id=p.branch_id AND v.parent_order_id=p.parent_order_id)", StateColumn: "link_state", VersionColumn: "version", ReferenceExpression: "t.id||':'||t.vendor_order_id", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id JOIN vendor_businesses b ON b.id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id,'identity',t.id||':'||t.vendor_order_id)", Dimensions: map[string]string{"authority": "authority", "observed_status": "status_projection", "observed_at": "status_updated_at", "refusal_code": "refusal_code"}, Counters: map[string]string{"observed_source_version": "status_source_version"}, References: map[string]string{"parent_order": "parent_order_id", "vendor_order": "vendor_order_id", "fulfilment_group": "group_id", "platform_reference": "id", "operation": "mission_ref", "owner_mission": "external_ref", "replaces": "replaces_id", "business": "business_id", "branch": "branch_id"}},
		"vendor-delivery-status": {Table: "(SELECT m.id,m.group_id,m.plan_id,m.plan_mission_id,m.business_id,m.branch_id,m.status,m.version AS mission_version,m.created_at,GREATEST(m.updated_at,r.updated_at) AS updated_at,r.stage,r.version AS runtime_version,CASE WHEN r.vendor_mission_id IS NULL THEN NULL ELSE r.custody_verified_at IS NOT NULL END AS custody_verified FROM commerce_vendor_missions m LEFT JOIN commerce_vendor_mission_runtime r ON r.vendor_mission_id=m.id)", StateColumn: "status", VersionColumn: "", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id JOIN vendor_businesses b ON b.id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id,'identity',t.id::text)", Dimensions: map[string]string{"runtime_stage": "stage", "custody_verified": "custody_verified"}, Counters: map[string]string{"mission_version": "mission_version", "runtime_version": "runtime_version"}, References: map[string]string{"fulfilment_group": "group_id", "plan": "plan_id", "plan_mission": "plan_mission_id", "business": "business_id", "branch": "branch_id"}},
		"branch-hours":           {Table: "vendor_branch_hours", VersionColumn: "version", BindingColumns: []string{"business_id", "branch_id"}, Dimensions: map[string]string{"is_closed": "is_closed", "opens_at": "opens_at", "closes_at": "closes_at"}, Counters: map[string]string{"weekday": "weekday"}, References: map[string]string{"business": "business_id", "branch": "branch_id"}},
		"order-lines":            {Table: "commerce_vendor_order_lines", StateColumn: "", VersionColumn: "", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.created_at", JoinClause: "JOIN commerce_vendor_orders v ON v.id=t.vendor_order_id JOIN vendor_branches br ON br.id=v.branch_id AND br.business_id=v.business_id JOIN vendor_businesses b ON b.id=v.business_id", BindingExpression: "jsonb_build_object('business',v.business_id,'branch',v.branch_id,'vendor_order',v.id,'parent_order',v.parent_order_id)", Dimensions: map[string]string{"pricing_basis": "pricing_basis"}, Counters: map[string]string{"quantity": "quantity", "unit_price_minor": "unit_price_minor", "quoted_total_minor": "quoted_total_minor", "catalogue_revision": "catalogue_revision", "inventory_revision": "inventory_revision"}, References: map[string]string{"parent_order": "parent_order_id", "vendor_order": "vendor_order_id", "quote_line": "quote_line_ref", "cart_item": "cart_item_id", "product": "item_ref", "variant": "variant_ref", "listing": "listing_ref", "price": "price_reference"}},
		"payment-allocations":    {Table: "commerce_payment_legs", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN commerce_vendor_orders v ON v.id=t.vendor_order_id AND v.business_id=t.business_id JOIN vendor_branches br ON br.id=v.branch_id AND br.business_id=v.business_id JOIN vendor_businesses b ON b.id=v.business_id", BindingExpression: "jsonb_build_object('business',v.business_id,'branch',v.branch_id,'vendor_order',v.id,'parent_order',v.parent_order_id)", Dimensions: map[string]string{"authority_status": "authority_status", "currency": "currency"}, Counters: map[string]string{"amount_minor": "amount_minor", "refunded_minor": "refunded_minor", "settled_minor": "settled_minor"}, References: map[string]string{"parent_order": "parent_order_id", "vendor_order": "vendor_order_id", "payment_operation": "payment_operation_id", "wallet_intent": "intent_id"}},
		"fulfilment-parcels":     {Table: "commerce_fulfilment_parcels", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN commerce_vendor_orders v ON v.id=t.vendor_order_id AND v.business_id=t.business_id AND v.branch_id=t.branch_id JOIN vendor_branches br ON br.id=v.branch_id AND br.business_id=v.business_id JOIN vendor_businesses b ON b.id=v.business_id", BindingExpression: "jsonb_build_object('business',v.business_id,'branch',v.branch_id,'vendor_order',v.id,'parent_order',v.parent_order_id)", Dimensions: map[string]string{"custodian_type": "custodian_type", "temperature_control": "temperature_control"}, Counters: map[string]string{"sequence": "sequence", "manifest_version": "manifest_version", "weight_grams": "weight_grams"}, References: map[string]string{"parent_order": "parent_order_id", "vendor_order": "vendor_order_id", "fulfilment_group": "group_id", "unit": "unit_id", "parcel": "parcel_ref"}},
		"order-events":           {Table: "commerce_order_events", StateColumn: "to_status", VersionColumn: "", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.occurred_at", UpdatedAtExpression: "t.occurred_at", JoinClause: "JOIN commerce_vendor_orders v ON v.id=t.vendor_order_id AND v.parent_order_id=t.parent_order_id JOIN vendor_branches br ON br.id=v.branch_id AND br.business_id=v.business_id JOIN vendor_businesses b ON b.id=v.business_id", BindingExpression: "jsonb_build_object('business',v.business_id,'branch',v.branch_id,'vendor_order',v.id,'parent_order',v.parent_order_id)", Dimensions: map[string]string{"action": "action", "from_status": "from_status", "actor_type": "actor_type"}, Counters: map[string]string{"event_version": "event_version"}, References: map[string]string{"parent_order": "parent_order_id", "vendor_order": "vendor_order_id"}},
		"logistics-events":       {Table: "commerce_logistics_order_events", StateColumn: "event_type", VersionColumn: "", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.received_at", JoinClause: "JOIN commerce_vendor_orders v ON v.id=t.vendor_order_id AND v.parent_order_id=t.parent_order_id JOIN vendor_branches br ON br.id=v.branch_id AND br.business_id=v.business_id JOIN vendor_businesses b ON b.id=v.business_id", BindingExpression: "jsonb_build_object('business',v.business_id,'branch',v.branch_id,'vendor_order',v.id,'parent_order',v.parent_order_id)", Dimensions: map[string]string{"source": "source", "applied": "applied", "ignored_reason": "ignored_reason"}, Counters: map[string]string{"captured_source_version": "source_version"}, References: map[string]string{"parent_order": "parent_order_id", "vendor_order": "vendor_order_id", "mission": "mission_ref", "source_event": "event_id"}},
		"refund-decisions":       {Table: "commerce_refund_policy_decisions", StateColumn: "wallet_status", VersionColumn: "", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN commerce_vendor_orders v ON v.id=t.vendor_order_id AND v.parent_order_id=t.parent_order_id JOIN vendor_branches br ON br.id=v.branch_id AND br.business_id=v.business_id JOIN vendor_businesses b ON b.id=v.business_id", BindingExpression: "jsonb_build_object('business',v.business_id,'branch',v.branch_id,'vendor_order',v.id,'parent_order',v.parent_order_id)", Dimensions: map[string]string{"trigger": "trigger", "action": "action", "reason_code": "reason_code", "policy_version": "policy_version", "support_required": "support_required"}, Counters: map[string]string{"customer_refund_minor": "customer_refund_minor", "authorization_release_minor": "authorization_release_minor"}, References: map[string]string{"parent_order": "parent_order_id", "vendor_order": "vendor_order_id", "line": "order_line_id", "mission": "mission_ref", "wallet_operation": "wallet_operation_ref", "support_case": "support_case_ref"}},
		"parcel-returns":         {Table: "commerce_parcel_returns", StateColumn: "state", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN commerce_fulfilment_parcels p ON p.id=t.parcel_id AND p.group_id=t.group_id JOIN commerce_vendor_orders v ON v.id=p.vendor_order_id AND v.business_id=p.business_id AND v.branch_id=p.branch_id JOIN vendor_branches br ON br.id=v.branch_id AND br.business_id=v.business_id JOIN vendor_businesses b ON b.id=v.business_id", BindingExpression: "jsonb_build_object('business',v.business_id,'branch',v.branch_id,'vendor_order',v.id,'parent_order',v.parent_order_id)", Dimensions: map[string]string{"reason_code": "reason_code"}, Counters: map[string]string{}, References: map[string]string{"parcel": "parcel_id", "fulfilment_group": "group_id", "origin_platform_mission": "origin_platform_mission_ref_id", "origin_vendor_mission": "origin_vendor_mission_id", "return_platform_mission": "return_platform_mission_ref_id", "return_vendor_mission": "return_vendor_mission_id"}},
		"cases":                  {Table: "vendor_admin_cases", ReferenceExpression: "t.case_ref", StateColumn: "state", VersionColumn: "version", BindingColumns: []string{"business_id", "branch_id", "subject_kind", "subject_id"}, Dimensions: map[string]string{"category": "category", "severity": "severity", "subject_kind": "subject_kind"}, References: map[string]string{"business": "business_id", "branch": "branch_id", "subject": "subject_id"}, TimelineTable: "vendor_admin_case_history", TimelineParentColumn: "case_ref", TimelineTypeColumn: "action", TimelineTimeColumn: "occurred_at"},

		"merchants":               {Table: "vendor_businesses", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "", BindingExpression: "jsonb_build_object('business',t.id,'branch',''::text)", Dimensions: map[string]string{"currency": "currency", "timezone": "timezone"}, Counters: map[string]string{}, References: map[string]string{}},
		"merchant-capabilities":   {Table: "vendor_capabilities", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.updated_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',''::text)", Dimensions: map[string]string{"capability_type": "capability_type"}, Counters: map[string]string{}, References: map[string]string{"business": "business_id"}},
		"brands":                  {Table: "vendor_brands", StateColumn: "verification_state", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',''::text)", Dimensions: map[string]string{}, Counters: map[string]string{}, References: map[string]string{"business": "business_id"}},
		"branches":                {Table: "vendor_branches", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.id,'brand',t.brand_id)", Dimensions: map[string]string{"country_code": "country_code", "timezone": "timezone"}, Counters: map[string]string{}, References: map[string]string{"brand": "brand_id", "business": "business_id"}},
		"branch-settings":         {Table: "vendor_branch_settings", StateColumn: "", VersionColumn: "version", ReferenceExpression: "t.branch_id::text", CreatedAtExpression: "t.updated_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id)", Dimensions: map[string]string{"accepting_new_orders": "accepting_new_orders", "pickup_enabled": "pickup_enabled"}, Counters: map[string]string{}, References: map[string]string{"business": "business_id", "branch": "branch_id"}},
		"branch-capabilities":     {Table: "vendor_branch_capabilities", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id)", Dimensions: map[string]string{"capability_type": "capability_type"}, Counters: map[string]string{}, References: map[string]string{"capability": "vendor_capability_id", "business": "business_id", "branch": "branch_id"}},
		"verification-references": {Table: "vendor_verification_projections", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',''::text)", Dimensions: map[string]string{"dimension": "dimension", "subject_type": "subject_type", "source_service": "source_service", "expires_at": "expires_at"}, Counters: map[string]string{}, References: map[string]string{"verification": "source_ref", "subject": "subject_ref", "business": "business_id"}},
		"food-items":              {Table: "food_items", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id)", Dimensions: map[string]string{}, Counters: map[string]string{"base_price_minor": "base_price_minor"}, References: map[string]string{"menu": "menu_id", "business": "business_id", "branch": "branch_id"}},
		"food-variants":           {Table: "food_item_variants", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id JOIN food_items fi ON fi.id=t.item_id AND fi.business_id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',fi.branch_id)", Dimensions: map[string]string{}, Counters: map[string]string{"price_override_minor": "price_override_minor"}, References: map[string]string{"item": "item_id", "business": "business_id"}},
		"shop-products":           {Table: "shop_products", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',''::text)", Dimensions: map[string]string{"product_class": "product_class"}, Counters: map[string]string{}, References: map[string]string{"business": "business_id"}},
		"shop-variants":           {Table: "shop_variants", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',''::text)", Dimensions: map[string]string{"package_level": "package_level", "temperature_zone": "temperature_zone"}, Counters: map[string]string{"retail_price_minor": "retail_price_minor", "wholesale_price_minor": "wholesale_price_minor"}, References: map[string]string{"product": "product_id", "business": "business_id"}},
		"shop-listings":           {Table: "shop_branch_listings", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id)", Dimensions: map[string]string{"retail_enabled": "retail_enabled", "wholesale_enabled": "wholesale_enabled", "preorder_policy": "preorder_policy", "backorder_policy": "backorder_policy"}, Counters: map[string]string{}, References: map[string]string{"product": "product_id", "variant": "variant_id", "business": "business_id", "branch": "branch_id"}},
		"stock-balances":          {Table: "inventory_balance_availability", VersionColumn: "revision", ReferenceExpression: "t.id::text", CreatedAtExpression: "i.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id JOIN inventory_items i ON i.id=t.inventory_item_id AND i.business_id=t.business_id AND i.branch_id=t.branch_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id,'inventory_item',t.inventory_item_id)", DimensionQuery: "SELECT jsonb_build_object('item_type',i.item_type,'item_state',i.status,'tracking_mode',i.tracking_mode,'out_of_stock',t.available_quantity=0) FROM inventory_balance_availability t JOIN inventory_items i ON i.id=t.inventory_item_id AND i.business_id=t.business_id AND i.branch_id=t.branch_id WHERE t.id=$1", Counters: map[string]string{"on_hand_quantity": "on_hand_quantity", "reserved_quantity": "reserved_quantity", "quarantined_quantity": "quarantined_quantity", "in_transit_quantity": "in_transit_quantity", "backorder_quantity": "backorder_quantity", "incoming_quantity": "incoming_quantity", "available_quantity": "available_quantity", "expired_quantity": "expired_quantity"}, References: map[string]string{"inventory_item": "inventory_item_id", "business": "business_id", "branch": "branch_id"}},
		"stock-reservations":      {Table: "inventory_reservations", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id)", Dimensions: map[string]string{"source_type": "source_type", "expires_at": "expires_at"}, Counters: map[string]string{}, References: map[string]string{"source": "source_ref", "business": "business_id", "branch": "branch_id"}},
		"stock-exceptions":        {Table: "inventory_alerts", StateColumn: "status", VersionColumn: "", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id)", Dimensions: map[string]string{"alert_type": "alert_type", "severity": "severity"}, Counters: map[string]string{}, References: map[string]string{"inventory_item": "inventory_item_id", "business": "business_id", "branch": "branch_id"}},
		"production-jobs":         {Table: "production_jobs", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id)", Dimensions: map[string]string{"capability": "capability", "source_type": "source_type", "scheduled_start": "scheduled_start", "scheduled_end": "scheduled_end", "promised_ready_at": "promised_ready_at", "expected_ready_window_start": "expected_ready_window_start", "expected_ready_window_end": "expected_ready_window_end"}, Counters: map[string]string{"planned_quantity": "planned_quantity", "produced_quantity": "produced_quantity", "waste_quantity": "waste_quantity"}, References: map[string]string{"reservation": "reservation_id", "station": "production_line_id", "source": "source_ref", "business": "business_id", "branch": "branch_id"}},
		"production-batches":      {Table: "vendor_production_batches", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id)", Dimensions: map[string]string{"operating_model": "operating_model", "actual_consumption_recorded": "actual_consumption_recorded"}, Counters: map[string]string{"planned_output_numerator": "planned_output_numerator", "planned_output_denominator": "planned_output_denominator", "actual_output_numerator": "actual_output_numerator", "actual_output_denominator": "actual_output_denominator"}, References: map[string]string{"job": "production_job_id", "recipe": "recipe_id", "inventory_reservation": "inventory_reservation_id", "business": "business_id", "branch": "branch_id"}},
		"capacity-pools":          {Table: "production_capacity_pools", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id)", Dimensions: map[string]string{"capability": "capability", "unit_type": "unit_type"}, Counters: map[string]string{"default_capacity": "default_capacity"}, References: map[string]string{"business": "business_id", "branch": "branch_id"}},
		"capacity-reservations":   {Table: "capacity_reservations", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id)", Dimensions: map[string]string{"capability": "capability", "window_start": "window_start", "window_end": "window_end", "hold_expires_at": "hold_expires_at", "source_type": "source_type"}, Counters: map[string]string{}, References: map[string]string{"source": "source_ref", "business": "business_id", "branch": "branch_id"}},
		"production-exceptions":   {Table: "production_operational_alerts", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.detected_at", UpdatedAtExpression: "t.detected_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id)", Dimensions: map[string]string{"alert_type": "alert_type", "source_type": "source_type", "detected_at": "detected_at"}, Counters: map[string]string{}, References: map[string]string{"source": "source_ref", "business": "business_id", "branch": "branch_id"}},
		"kitchen-packages":        {Table: "cloud_kitchen_packages", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id)", Dimensions: map[string]string{"capability": "capability"}, Counters: map[string]string{"minimum_quantity": "minimum_quantity", "maximum_quantity": "maximum_quantity", "lead_time_minutes": "lead_time_minutes"}, References: map[string]string{"capacity_pool": "capacity_pool_id", "business": "business_id", "branch": "branch_id"}},
		"pastry-commitments":      {Table: "pastry_custom_requests", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id)", Dimensions: map[string]string{"event_at": "event_at", "change_cutoff_at": "change_cutoff_at"}, Counters: map[string]string{}, References: map[string]string{"business": "business_id", "branch": "branch_id"}},
		"vendor-orders":           {Table: "commerce_vendor_orders", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id,'parent_order',t.parent_order_id,'customer',t.customer_id)", Dimensions: map[string]string{"domain": "domain", "inventory_state": "inventory_state", "fulfilment_mode": "fulfilment_mode", "vendor_own_delivery_state": "vendor_own_delivery_state", "refund_state": "refund_state", "delivery_responsibility": "delivery_responsibility", "effective_delivery_responsibility": "effective_delivery_responsibility"}, Counters: map[string]string{"customer_payable_minor": "customer_payable_minor", "refund_allocated_minor": "refund_allocated_minor", "refund_refunded_minor": "refund_refunded_minor"}, References: map[string]string{"parent_order": "parent_order_id", "mission": "effective_logistics_mission_ref", "payment": "refund_operation_ref", "business": "business_id", "branch": "branch_id", "inventory_reservation": "inventory_reservation_ref", "effective_mission": "effective_logistics_mission_ref"}},
		"substitutions":           {Table: "(SELECT p.*,abs(p.price_difference_minor::numeric) AS price_difference_magnitude_minor,CASE WHEN p.price_difference_minor<0 THEN 'DECREASE' WHEN p.price_difference_minor>0 THEN 'INCREASE' ELSE 'UNCHANGED' END AS price_difference_direction FROM commerce_substitution_proposals p)", StateColumn: "status", VersionColumn: "version", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN commerce_vendor_orders v ON v.id=t.vendor_order_id AND v.business_id=t.business_id AND v.branch_id=t.branch_id AND v.parent_order_id=t.parent_order_id AND v.customer_id=t.customer_id JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',t.branch_id,'parent_order',t.parent_order_id,'vendor_order',t.vendor_order_id,'line',t.order_line_id,'customer',t.customer_id,'replacement_snapshot',t.replacement_snapshot)", Dimensions: map[string]string{"expires_at": "expires_at", "admin_inventory_processing_state": "admin_inventory_processing_state", "price_difference_direction": "price_difference_direction"}, Counters: map[string]string{"proposed_quantity": "proposed_quantity", "proposed_price_minor": "proposed_price_minor", "price_difference_magnitude_minor": "price_difference_magnitude_minor"}, References: map[string]string{"parent_order": "parent_order_id", "vendor_order": "vendor_order_id", "line": "order_line_id", "business": "business_id", "branch": "branch_id"}},
		"support-cases":           {Table: "commerce_support_case_links", StateColumn: "support_status", VersionColumn: "", ReferenceExpression: "t.id::text", CreatedAtExpression: "t.created_at", UpdatedAtExpression: "t.updated_at", JoinClause: "JOIN vendor_businesses b ON b.id=t.business_id", BindingExpression: "jsonb_build_object('business',t.business_id,'branch',''::text)", Dimensions: map[string]string{"category": "category", "refund_status": "refund_status"}, Counters: map[string]string{}, References: map[string]string{"parent_order": "parent_order_id", "vendor_order": "vendor_order_id", "support_case": "support_case_ref", "business": "business_id"}},
	})}
}

var nativeTargets = map[string]struct{ query, permission string }{
	"fulfilment-units":        {"SELECT t.business_id,t.branch_id FROM commerce_fulfilment_units t JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id JOIN vendor_businesses b ON b.id=t.business_id WHERE t.id::text=$1", "vendor.orders.read"},
	"delivery-mission-links":  {"SELECT t.business_id,t.branch_id FROM (SELECT DISTINCT r.id,r.group_id,r.plan_id,r.plan_mission_id,r.authority,r.mission_ref,r.external_ref,r.link_state,r.status_projection,r.status_source_version,r.status_updated_at,r.refusal_code,r.replaces_id,r.version,r.created_at,r.updated_at,v.id AS vendor_order_id,v.parent_order_id,v.business_id,v.branch_id FROM commerce_platform_mission_refs r JOIN commerce_fulfilment_plan_mission_parcels mp ON mp.plan_mission_id=r.plan_mission_id AND mp.plan_id=r.plan_id AND mp.group_id=r.group_id JOIN commerce_fulfilment_parcels p ON p.id=mp.parcel_id AND p.group_id=r.group_id JOIN commerce_vendor_orders v ON v.id=p.vendor_order_id AND v.business_id=p.business_id AND v.branch_id=p.branch_id AND v.parent_order_id=p.parent_order_id) t JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id JOIN vendor_businesses b ON b.id=t.business_id WHERE t.id||':'||t.vendor_order_id=$1", "vendor.orders.read"},
	"vendor-delivery-status":  {"SELECT t.business_id,t.branch_id FROM (SELECT m.id,m.group_id,m.plan_id,m.plan_mission_id,m.business_id,m.branch_id,m.status,m.version AS mission_version,m.created_at,GREATEST(m.updated_at,r.updated_at) AS updated_at,r.stage,r.version AS runtime_version,CASE WHEN r.vendor_mission_id IS NULL THEN NULL ELSE r.custody_verified_at IS NOT NULL END AS custody_verified FROM commerce_vendor_missions m LEFT JOIN commerce_vendor_mission_runtime r ON r.vendor_mission_id=m.id) t JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id JOIN vendor_businesses b ON b.id=t.business_id WHERE t.id::text=$1", "vendor.orders.read"},
	"branch-hours":            {"SELECT t.business_id,t.branch_id FROM vendor_branch_hours t JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id WHERE t.id=$1", "vendor.branches.read"},
	"order-lines":             {"SELECT v.business_id,v.branch_id FROM commerce_vendor_order_lines t JOIN commerce_vendor_orders v ON v.id=t.vendor_order_id JOIN vendor_branches br ON br.id=v.branch_id AND br.business_id=v.business_id JOIN vendor_businesses b ON b.id=v.business_id WHERE t.id::text=$1", "vendor.orders.read"},
	"payment-allocations":     {"SELECT v.business_id,v.branch_id FROM commerce_payment_legs t JOIN commerce_vendor_orders v ON v.id=t.vendor_order_id AND v.business_id=t.business_id JOIN vendor_branches br ON br.id=v.branch_id AND br.business_id=v.business_id JOIN vendor_businesses b ON b.id=v.business_id WHERE t.id::text=$1", "vendor.orders.read"},
	"fulfilment-parcels":      {"SELECT v.business_id,v.branch_id FROM commerce_fulfilment_parcels t JOIN commerce_vendor_orders v ON v.id=t.vendor_order_id AND v.business_id=t.business_id AND v.branch_id=t.branch_id JOIN vendor_branches br ON br.id=v.branch_id AND br.business_id=v.business_id JOIN vendor_businesses b ON b.id=v.business_id WHERE t.id::text=$1", "vendor.orders.read"},
	"order-events":            {"SELECT v.business_id,v.branch_id FROM commerce_order_events t JOIN commerce_vendor_orders v ON v.id=t.vendor_order_id AND v.parent_order_id=t.parent_order_id JOIN vendor_branches br ON br.id=v.branch_id AND br.business_id=v.business_id JOIN vendor_businesses b ON b.id=v.business_id WHERE t.id::text=$1", "vendor.orders.read"},
	"logistics-events":        {"SELECT v.business_id,v.branch_id FROM commerce_logistics_order_events t JOIN commerce_vendor_orders v ON v.id=t.vendor_order_id AND v.parent_order_id=t.parent_order_id JOIN vendor_branches br ON br.id=v.branch_id AND br.business_id=v.business_id JOIN vendor_businesses b ON b.id=v.business_id WHERE t.id::text=$1", "vendor.orders.read"},
	"refund-decisions":        {"SELECT v.business_id,v.branch_id FROM commerce_refund_policy_decisions t JOIN commerce_vendor_orders v ON v.id=t.vendor_order_id AND v.parent_order_id=t.parent_order_id JOIN vendor_branches br ON br.id=v.branch_id AND br.business_id=v.business_id JOIN vendor_businesses b ON b.id=v.business_id WHERE t.id::text=$1", "vendor.orders.read"},
	"parcel-returns":          {"SELECT v.business_id,v.branch_id FROM commerce_parcel_returns t JOIN commerce_fulfilment_parcels p ON p.id=t.parcel_id AND p.group_id=t.group_id JOIN commerce_vendor_orders v ON v.id=p.vendor_order_id AND v.business_id=p.business_id AND v.branch_id=p.branch_id JOIN vendor_branches br ON br.id=v.branch_id AND br.business_id=v.business_id JOIN vendor_businesses b ON b.id=v.business_id WHERE t.id::text=$1", "vendor.orders.read"},
	"merchants":               {"SELECT t.id,''::text FROM vendor_businesses t  WHERE t.id::text=$1", "vendor.business.read"},
	"merchant-capabilities":   {"SELECT t.business_id,''::text FROM vendor_capabilities t JOIN vendor_businesses b ON b.id=t.business_id WHERE t.id::text=$1", "vendor.capabilities.read"},
	"brands":                  {"SELECT t.business_id,''::text FROM vendor_brands t JOIN vendor_businesses b ON b.id=t.business_id WHERE t.id::text=$1", "vendor.brands.read"},
	"branches":                {"SELECT t.business_id,t.id FROM vendor_branches t JOIN vendor_businesses b ON b.id=t.business_id WHERE t.id::text=$1", "vendor.branches.read"},
	"branch-settings":         {"SELECT t.business_id,t.branch_id FROM vendor_branch_settings t JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id WHERE t.branch_id::text=$1", "vendor.branches.read"},
	"branch-capabilities":     {"SELECT t.business_id,t.branch_id FROM vendor_branch_capabilities t JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id WHERE t.id::text=$1", "vendor.branches.read"},
	"verification-references": {"SELECT t.business_id,''::text FROM vendor_verification_projections t JOIN vendor_businesses b ON b.id=t.business_id WHERE t.id::text=$1", "vendor.business.read"},
	"food-items":              {"SELECT t.business_id,t.branch_id FROM food_items t JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id WHERE t.id::text=$1", "vendor.food.catalogue.read"},
	"food-variants":           {"SELECT t.business_id,fi.branch_id FROM food_item_variants t JOIN vendor_businesses b ON b.id=t.business_id JOIN food_items fi ON fi.id=t.item_id AND fi.business_id=t.business_id WHERE t.id::text=$1", "vendor.food.catalogue.read"},
	"shop-products":           {"SELECT t.business_id,''::text FROM shop_products t JOIN vendor_businesses b ON b.id=t.business_id WHERE t.id::text=$1", "vendor.shop.read"},
	"shop-variants":           {"SELECT t.business_id,''::text FROM shop_variants t JOIN vendor_businesses b ON b.id=t.business_id WHERE t.id::text=$1", "vendor.shop.read"},
	"shop-listings":           {"SELECT t.business_id,t.branch_id FROM shop_branch_listings t JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id WHERE t.id::text=$1", "vendor.shop.read"},
	"stock-balances":          {"SELECT t.business_id,t.branch_id FROM inventory_balance_availability t JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id WHERE t.id::text=$1", "vendor.inventory.read"},
	"stock-reservations":      {"SELECT t.business_id,t.branch_id FROM inventory_reservations t JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id WHERE t.id::text=$1", "vendor.inventory.read"},
	"stock-exceptions":        {"SELECT t.business_id,t.branch_id FROM inventory_alerts t JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id WHERE t.id::text=$1", "vendor.inventory.read"},
	"production-jobs":         {"SELECT t.business_id,t.branch_id FROM production_jobs t JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id WHERE t.id::text=$1", "vendor.production.read"},
	"production-batches":      {"SELECT t.business_id,t.branch_id FROM vendor_production_batches t JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id WHERE t.id::text=$1", "vendor.production.read"},
	"capacity-pools":          {"SELECT t.business_id,t.branch_id FROM production_capacity_pools t JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id WHERE t.id::text=$1", "vendor.production.read"},
	"capacity-reservations":   {"SELECT t.business_id,t.branch_id FROM capacity_reservations t JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id WHERE t.id::text=$1", "vendor.production.read"},
	"production-exceptions":   {"SELECT t.business_id,t.branch_id FROM production_operational_alerts t JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id WHERE t.id::text=$1", "vendor.production.read"},
	"kitchen-packages":        {"SELECT t.business_id,t.branch_id FROM cloud_kitchen_packages t JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id WHERE t.id::text=$1", "vendor.production.read"},
	"pastry-commitments":      {"SELECT t.business_id,t.branch_id FROM pastry_custom_requests t JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id WHERE t.id::text=$1", "vendor.production.read"},
	"vendor-orders":           {"SELECT t.business_id,t.branch_id FROM commerce_vendor_orders t JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id WHERE t.id::text=$1", "vendor.orders.read"},
	"substitutions":           {"SELECT t.business_id,t.branch_id FROM commerce_substitution_proposals t JOIN commerce_vendor_orders v ON v.id=t.vendor_order_id AND v.business_id=t.business_id AND v.branch_id=t.branch_id AND v.parent_order_id=t.parent_order_id AND v.customer_id=t.customer_id JOIN vendor_businesses b ON b.id=t.business_id JOIN vendor_branches br ON br.id=t.branch_id AND br.business_id=t.business_id WHERE t.id::text=$1", "vendor.orders.read"},
	"support-cases":           {"SELECT t.business_id,''::text FROM commerce_support_case_links t JOIN vendor_businesses b ON b.id=t.business_id WHERE t.id::text=$1", "vendor.orders.read"},
}

// NativeTarget establishes the real merchant and optional branch; no supplied
// tenant or administrative case can change these relationships.
func NativeTarget(ctx context.Context, tx pgx.Tx, kind, id string) (string, string, string, error) {
	if kind == "cases" {
		var business, branch, subjectKind, subjectID string
		err := tx.QueryRow(ctx, `SELECT business_id,COALESCE(branch_id,''),subject_kind,subject_id FROM vendor_admin_cases WHERE case_ref=$1`, id).Scan(&business, &branch, &subjectKind, &subjectID)
		if err != nil {
			return "", "", "", err
		}
		if subjectKind == "cases" {
			return "", "", "", pgx.ErrNoRows
		}
		b, br, permission, err := NativeTarget(ctx, tx, subjectKind, subjectID)
		if err != nil {
			return "", "", "", err
		}
		if b != business || br != branch {
			return "", "", "", pgx.ErrNoRows
		}
		return b, br, permission, nil
	}
	target, ok := nativeTargets[kind]
	if !ok {
		return "", "", "", pgx.ErrNoRows
	}
	var business, branch string
	err := tx.QueryRow(ctx, target.query, id).Scan(&business, &branch)
	return business, branch, target.permission, err
}

// RequireNativeStaff intersects HQ delegation with current Vendor IAM. Staff
// enrollment is source-owned; no role or actor binding is installed by this code.
func RequireNativeStaff(ctx context.Context, tx pgx.Tx, staff, business, branch, permission string) error {
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_staff s JOIN vendor_staff_role_assignments a ON a.staff_id=s.id JOIN vendor_role_permissions rp ON rp.role_id=a.role_id JOIN vendor_permissions p ON p.id=rp.permission_id WHERE s.id=$1 AND s.vendor_id=$2 AND s.status='active' AND a.revoked_at IS NULL AND p.code=$4 AND (a.branch_id IS NULL OR ($3<>'' AND a.branch_id=$3)))`, staff, business, branch, permission).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return pgx.ErrNoRows
	}
	return nil
}
func (s *SourceReader) AuthorizeStaff(ctx context.Context, tx pgx.Tx, staff, kind, id string) error {
	business, branch, permission, err := NativeTarget(ctx, tx, kind, id)
	if err != nil {
		return err
	}
	return RequireNativeStaff(ctx, tx, staff, business, branch, permission)
}
func Runtime(ctx context.Context, db *pgxpool.Pool) (*adminowner.Handler, error) {
	p := adminowner.VendorProfile()
	return adminowner.FromEnvironment(ctx, db, Source(), p.Definitions, Owner, p.Prefix, p.Path)
}

func (s *SourceReader) Read(ctx context.Context, tx pgx.Tx, kind, id string, detail bool) (adminowner.Resource, error) {
	if kind == "cases" {
		if _, _, _, err := NativeTarget(ctx, tx, kind, id); err != nil {
			return adminowner.Resource{}, err
		}
	}
	item, err := s.SQLSource.Read(ctx, tx, kind, id, detail)
	if err != nil {
		return item, err
	}
	temporal := map[string]bool{"observed_at": true, "expires_at": true, "scheduled_start": true, "scheduled_end": true, "promised_ready_at": true, "expected_ready_window_start": true, "expected_ready_window_end": true, "window_start": true, "window_end": true, "hold_expires_at": true, "detected_at": true, "event_at": true, "change_cutoff_at": true}
	for i, d := range item.Dimensions {
		if temporal[d.Name] && d.Value != nil {
			at, e := time.Parse(time.RFC3339Nano, *d.Value)
			if e != nil {
				return adminowner.Resource{}, e
			}
			value := at.UTC().Format(time.RFC3339Nano)
			item.Dimensions[i].Value = &value
		}
	}
	if kind == "vendor-orders" {
		var parentState, quote, cart, checkout, plan string
		var parentVersion int64
		err = tx.QueryRow(ctx, `SELECT p.state,p.version,p.quote_id,p.cart_id,p.checkout_session_id,p.fulfilment_plan_id
          FROM commerce_vendor_orders v JOIN commerce_parent_orders p ON p.id=v.parent_order_id AND p.customer_id=v.customer_id
          WHERE v.id=$1`, id).Scan(&parentState, &parentVersion, &quote, &cart, &checkout, &plan)
		if err != nil {
			return adminowner.Resource{}, err
		}
		coverage := "ALLOCATION_ONLY_SIBLINGS_WITHHELD"
		for i, d := range item.Dimensions {
			switch d.Name {
			case "parent_state":
				item.Dimensions[i].Value = &parentState
			case "parent_view_coverage":
				item.Dimensions[i].Value = &coverage
			}
		}
		for i, c := range item.Counters {
			if c.Name == "parent_version" {
				item.Counters[i].Value = &parentVersion
			}
		}

		for _, r := range []adminowner.Reference{{Kind: "quote", Reference: quote}, {Kind: "cart", Reference: cart}, {Kind: "checkout", Reference: checkout}, {Kind: "fulfilment_plan", Reference: plan}} {
			if r.Reference != "" {
				item.References = append(item.References, r)
			}
		}
	}
	if kind == "branch-capabilities" {
		allowed, reason, e := vendors.AdministrativeBranchCapabilityEligibility(ctx, tx, id)
		if e != nil {
			return adminowner.Resource{}, e
		}
		for i, d := range item.Dimensions {
			switch d.Name {
			case "operationally_eligible":
				item.Dimensions[i].Flag = &allowed
			case "eligibility_reason":
				item.Dimensions[i].Value = &reason
			}
		}
	}
	if kind == "delivery-mission-links" {
		value := "OWNER_PROJECTION_ONLY"
		for i, d := range item.Dimensions {
			if d.Name == "data_authority" {
				item.Dimensions[i].Value = &value
			}
		}
	}
	// Commercial labels are a finite Vendor-only allowlist. No contacts,
	// addresses, case free text, verification documents or evidence URLs.
	labelQueries := map[string]string{"merchants": "SELECT COALESCE(NULLIF(trading_name,''),legal_name) FROM vendor_businesses WHERE id=$1", "brands": "SELECT display_name FROM vendor_brands WHERE id=$1", "branches": "SELECT display_name FROM vendor_branches WHERE id=$1", "food-items": "SELECT name FROM food_items WHERE id=$1", "food-variants": "SELECT name FROM food_item_variants WHERE id=$1", "shop-products": "SELECT name FROM shop_products WHERE id=$1", "shop-variants": "SELECT name FROM shop_variants WHERE id=$1"}
	if query, ok := labelQueries[kind]; ok {
		var label *string
		if e := tx.QueryRow(ctx, query, id).Scan(&label); e != nil {
			return adminowner.Resource{}, e
		}
		item.Label = label
	}
	item.SnapshotETag = ""
	item.SnapshotETag = admincontext.Fingerprint(item)
	return item, nil
}
func (s *SourceReader) Binding(ctx context.Context, tx pgx.Tx, kind, id string) (admincontext.RecordBinding, error) {
	if kind == "cases" {
		if _, _, _, err := NativeTarget(ctx, tx, kind, id); err != nil {
			return admincontext.RecordBinding{}, err
		}
	}
	return s.SQLSource.Binding(ctx, tx, kind, id)
}

// RequireNativeStaffForMutation holds the actual native authorization rows
// until the source transaction and immutable command receipt commit. A
// concurrent suspension/revocation therefore cannot race past this decision.
func RequireNativeStaffForMutation(ctx context.Context, tx pgx.Tx, staff, business, branch, permission string) error {
	var id string
	return tx.QueryRow(ctx, `SELECT s.id FROM vendor_staff s
 JOIN vendor_staff_role_assignments a ON a.staff_id=s.id
 JOIN vendor_role_permissions rp ON rp.role_id=a.role_id
 JOIN vendor_permissions p ON p.id=rp.permission_id
 WHERE s.id=$1 AND s.vendor_id=$2 AND s.status='active' AND a.revoked_at IS NULL
 AND p.code=$4 AND (a.branch_id IS NULL OR ($3<>'' AND a.branch_id=$3))
 ORDER BY a.id LIMIT 1 FOR SHARE OF s,a,rp`, staff, business, branch, permission).Scan(&id)
}
