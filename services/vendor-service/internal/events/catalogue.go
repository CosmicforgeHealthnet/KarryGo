package events

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

var ErrUnsupportedEventVersion = errors.New("events: unsupported schema version")

const (
	CatalogueActive          = "ACTIVE"
	CatalogueDependencyGated = "DEPENDENCY_GATED"
	CatalogueFutureDisabled  = "FUTURE_DISABLED"
)

// CatalogueEvent is one authority-reviewed Section 44 event contract. Aliases
// record accepted repository-native names; they are mappings, not a second
// canonical event. An empty alias list is valid only for a dependency-gated or
// future-disabled contract.
type CatalogueEvent struct {
	Name                     string   `json:"name"`
	Version                  int      `json:"version"`
	Producer                 string   `json:"producer"`
	Authority                string   `json:"authority"`
	Status                   string   `json:"status"`
	Aliases                  []string `json:"implementedEventNames"`
	TransactionalSource      string   `json:"transactionalSource"`
	PayloadFields            []string `json:"payloadFields"`
	ActiveConsumers          []string `json:"activeConsumers"`
	DependencyGatedConsumers []string `json:"dependencyGatedConsumers"`
	Notes                    string   `json:"notes,omitempty"`
}

// Section44Catalogue returns a stable, canonical catalogue. It deliberately
// distinguishes active emitters/inbound projections from contracts reserved
// behind real dependencies. It never claims a merely proposed consumer exists.
func Section44Catalogue() []CatalogueEvent {
	events := []CatalogueEvent{
		active("vendor.business.activated", ServiceName, "vendor-service", []string{"vendor.business.activated"}, "vendor business activation transaction", fields("business_ref", "activation_version", "occurred_at"), nil, []string{"admin projection", "analytics", "notification"}),
		active("vendor.branch.status_changed", ServiceName, "vendor-service", []string{"vendor.branch.status_changed"}, "vendor branch lifecycle transaction", fields("business_ref", "branch_ref", "previous_status", "new_status", "branch_version", "reason_code"), nil, []string{"search projection", "analytics"}),
		active("vendor.catalog.item_changed", ServiceName, "vendor-service", []string{"food.item.created", "food.item.updated", "food.item.archived", "shop.product.changed", "shop.listing.changed", "pastry.product.changed"}, "Food/Shop/Pastry catalogue transaction", fields("business_ref", "branch_ref", "catalogue_family", "item_ref", "change_type", "item_version", "catalogue_revision"), nil, []string{"search projection", "analytics"}),
		active("vendor.inventory.stock_changed", ServiceName, "vendor-service", []string{"shop.inventory.adjusted", "shop.inventory.receipt.posted", "shop.inventory.reservation.expired", "shop.inventory.lot.expired"}, "inventory mutation transaction", fields("business_ref", "branch_ref", "stock_basis_ref", "stock_revision", "change_reason", "availability_state"), nil, []string{"search projection", "analytics"}),
		active("commerce.checkout.paid", ServiceName, "payment-wallet-service", []string{"commerce.checkout.paid"}, "Wallet-confirmed payment projection transaction", fields("parent_order_ref", "quote_ref", "quote_version", "payment_operation_ref", "currency", "approved_total_minor", "payment_authority_status_ref"), nil, []string{"analytics"}),
		active("commerce.order.created", ServiceName, "vendor-service", []string{"commerce.order.created"}, "ParentOrder creation transaction", fields("parent_order_ref", "quote_ref", "order_version", "vendor_order_count", "currency", "occurred_at"), nil, []string{"analytics"}),
		active("vendor.order.accepted", ServiceName, "vendor-service", []string{"commerce.vendor_order.accepted"}, "VendorOrder expectedVersion transition", fields("vendor_order_ref", "parent_order_ref", "business_ref", "branch_ref", "order_version", "accepted_at"), nil, []string{"notification", "analytics"}),
		active("vendor.order.rejected", ServiceName, "vendor-service", []string{"commerce.vendor_order.rejected"}, "VendorOrder expectedVersion transition", fields("vendor_order_ref", "parent_order_ref", "business_ref", "branch_ref", "order_version", "reason_code"), nil, []string{"notification", "analytics"}),
		active("vendor.order.ready", ServiceName, "vendor-service", []string{"commerce.vendor_order.ready", "commerce.vendor_order.ready_for_pickup"}, "VendorOrder expectedVersion transition", fields("vendor_order_ref", "branch_ref", "order_version", "ready_at", "fulfilment_ref"), nil, []string{"notification", "analytics"}),
		active("commerce.fulfilment.plan_created", ServiceName, "vendor-service", []string{"commerce.fulfilment.plan_activated"}, "fulfilment plan activation transaction", fields("checkout_ref", "plan_ref", "plan_version", "mode", "mission_count", "expires_at"), nil, []string{"notification"}),
		active("commerce.fulfilment.plan_replanned", ServiceName, "vendor-service", []string{"commerce.fulfilment.plan_activated"}, "versioned fulfilment replan transaction", fields("previous_plan_ref", "previous_plan_version", "new_plan_ref", "new_plan_version", "reason_code", "commercial_impact"), nil, []string{"notification"}),
		active("commerce.mission.requested", ServiceName, "vendor-service", []string{"commerce.fulfilment.mission_requested"}, "mission reference creation transaction", fields("commerce_operation_ref", "vendor_order_ref", "service_family", "mission_operation_id", "authority_ref"), nil, nil),
		inbound("delivery.provider_assigned", "logistics-authority", "Dispatch/Hauling/Taxi Goods/VendorMission", []string{"PROVIDER_ASSIGNED", "PROVIDER_REASSIGNED"}, fields("mission_ref", "provider_ref", "assignment_version", "assigned_at")),
		inbound("delivery.pickup.verified", "logistics-custody-authority", "Dispatch/Hauling/Taxi Goods/VendorMission", []string{"PICKUP_VERIFIED", "HANDED_OVER"}, fields("mission_ref", "custody_version", "occurred_at")),
		inbound("delivery.parcel.picked_up", "logistics-custody-authority", "Dispatch/Hauling/Taxi Goods/VendorMission", []string{"PICKED_UP"}, fields("mission_ref", "parcel_refs", "custody_version", "occurred_at")),
		inbound("delivery.receiver.verified", "logistics-custody-authority", "Dispatch/Hauling/Taxi Goods/VendorMission", []string{"RECEIVER_VERIFIED"}, fields("mission_ref", "verification_ref", "custody_version", "occurred_at")),
		inbound("delivery.completed", "logistics-authority", "Dispatch/Hauling/Taxi Goods/VendorMission", []string{"DELIVERED", "COMPLETED"}, fields("mission_ref", "outcome_ref", "outcome_version", "occurred_at")),
		inbound("delivery.failed", "logistics-authority", "Dispatch/Hauling/Taxi Goods/VendorMission", []string{"DELIVERY_FAILED", "FAILED"}, fields("mission_ref", "failure_code", "outcome_version", "recovery_ref")),
		inbound("delivery.returned", "logistics-custody-authority", "Dispatch/Hauling/Taxi Goods/VendorMission", []string{"RETURNED", "commerce.fulfilment.return_initiated"}, fields("mission_ref", "return_manifest_ref", "return_version", "destination_authority_ref")),
		inbound("commerce.refund.completed", "payment-wallet-service", "payment-wallet-service", []string{"commerce.vendor_order.refunded", "commerce.vendor_order.partially_refunded", "wallet.refund.status_changed.v1"}, fields("refund_operation_ref", "order_ref", "amount_minor", "currency", "authority_version")),
		inbound("commerce.vendor_settlement.completed", "payment-wallet-service", "payment-wallet-service", []string{"wallet.settlement.status_changed.v1"}, fields("settlement_operation_ref", "vendor_order_ref", "currency", "amount_minor", "authority_version")),
		active("commerce.quote.locked", ServiceName, "vendor-service", []string{"commerce.quote.locked"}, "immutable quote approval and outbox transaction", fields("quote_ref", "quote_version", "quote_fingerprint", "expires_at", "currency", "final_total_minor", "maximum_authorized_total_minor"), nil, nil),
		active("commerce.fulfilment.option_selected", ServiceName, "vendor-service", []string{"commerce.fulfilment.plan_activated"}, "selected plan activation transaction", fields("checkout_ref", "quote_ref", "plan_ref", "plan_version", "mode"), nil, nil),
		active("commerce.allocation.created", ServiceName, "vendor-service", []string{"commerce.order.created", "commerce.vendor_order.created"}, "ParentOrder/VendorOrder allocation transaction", fields("parent_order_ref", "vendor_order_ref", "allocation_ref", "allocation_type", "beneficiary_ref", "currency", "amount_minor"), nil, nil),
		active("commerce.substitution.proposed", ServiceName, "vendor-service", []string{"commerce.vendor_order.substitution_proposed"}, "substitution proposal transaction", fields("proposal_ref", "vendor_order_ref", "line_ref", "proposal_version", "expires_at", "financial_delta_classification"), nil, []string{"notification"}),
		active("commerce.substitution.resolved", ServiceName, "vendor-service", []string{"commerce.vendor_order.substitution_approved", "commerce.vendor_order.substitution_rejected"}, "substitution resolution transaction", fields("proposal_ref", "decision", "decision_version", "resulting_order_version"), nil, []string{"notification"}),
		active("commerce.variable_weight.finalized", ServiceName, "vendor-service", []string{"commerce.vendor_order.variable_measurement_finalized"}, "variable measurement finalization transaction", fields("vendor_order_ref", "line_ref", "measurement_ref", "measurement_version", "quantity", "financial_adjustment_ref"), nil, nil),
		gated("commerce.optimization.proposal_created", ServiceName, "vendor-service", fields("proposal_ref", "proposal_version", "objective", "candidate_count", "expires_at"), "commerce basket optimization feature flag"),
		active("commerce.recurring_basket.generated", ServiceName, "vendor-service", []string{"commerce.recurring_occurrence.generated"}, "recurring occurrence uniqueness transaction", fields("template_ref", "occurrence_ref", "proposal_ref", "idempotency_ref", "generated_at", "proposal_version"), nil, nil),
		active("commerce.rfq.quote_submitted", ServiceName, "vendor-service", []string{"commerce.rfq.quote_submitted"}, "RFQ quote transaction", fields("rfq_ref", "quote_ref", "quote_revision", "business_ref", "status", "occurred_at"), nil, []string{"analytics"}),
		active("commerce.rfq.quote_revised", ServiceName, "vendor-service", []string{"commerce.rfq.counter_offer.submitted", "commerce.rfq.quote_submitted"}, "RFQ quote revision transaction", fields("rfq_ref", "quote_ref", "quote_revision", "business_ref", "status", "occurred_at"), nil, []string{"analytics"}),
		active("commerce.rfq.quote_accepted", ServiceName, "vendor-service", []string{"commerce.rfq.accepted"}, "RFQ acceptance transaction", fields("rfq_ref", "quote_ref", "quote_revision", "business_ref", "status", "occurred_at"), nil, []string{"analytics"}),
		active("vendor.verification.badge_changed", ServiceName, "verification-compliance-service", []string{"vendor.verification.projection_changed"}, "authoritative verification projection transaction", fields("business_ref", "verification_dimension", "badge_projection", "authority_ref", "authority_version", "projection_version"), nil, nil),
		gated("vendor.reliability.updated", ServiceName, "vendor-service", fields("business_ref", "snapshot_ref", "metric_version", "coverage_window", "updated_at"), "Vendor CRM intelligence worker flag"),
		gated("vendor.intelligence.snapshot_updated", ServiceName, "vendor-service", fields("business_ref", "snapshot_ref", "snapshot_version", "coverage_period", "freshness_state"), "Vendor CRM intelligence worker flag"),
		gated("commerce.budget_basket.proposal_created", ServiceName, "vendor-service", fields("request_ref", "proposal_ref", "proposal_version", "budget_minor", "currency", "expires_at", "candidate_count"), "commerce basket optimization feature flag"),
		active("commerce.recurring_basket.approved", ServiceName, "vendor-service", []string{"commerce.recurring_schedule.active"}, "recurring schedule approval transaction", fields("template_ref", "occurrence_ref", "proposal_ref", "approved_quote_version", "approval_version", "occurred_at"), nil, nil),
		future("commerce.order_guarantee.evaluated"),
		future("commerce.order_guarantee.compensation_issued"),
	}
	for i := range events {
		slices.Sort(events[i].Aliases)
		slices.Sort(events[i].PayloadFields)
		slices.Sort(events[i].ActiveConsumers)
		slices.Sort(events[i].DependencyGatedConsumers)
	}
	return events
}

func active(name, producer, authority string, aliases []string, source string, payload, consumers, gatedConsumers []string) CatalogueEvent {
	return CatalogueEvent{Name: name, Version: 1, Producer: producer, Authority: authority, Status: CatalogueActive, Aliases: aliases, TransactionalSource: source, PayloadFields: payload, ActiveConsumers: consumers, DependencyGatedConsumers: gatedConsumers}
}

func inbound(name, producer, authority string, aliases, payload []string) CatalogueEvent {
	return CatalogueEvent{Name: name, Version: 1, Producer: producer, Authority: authority, Status: CatalogueActive, Aliases: aliases, TransactionalSource: "authority event inbox and projection transaction", PayloadFields: payload, ActiveConsumers: []string{ServiceName}}
}

func gated(name, producer, authority string, payload []string, gate string) CatalogueEvent {
	return CatalogueEvent{Name: name, Version: 1, Producer: producer, Authority: authority, Status: CatalogueDependencyGated, PayloadFields: payload, DependencyGatedConsumers: []string{gate}, Notes: "No active emitter is claimed while the dependency is disabled."}
}

func future(name string) CatalogueEvent {
	return CatalogueEvent{Name: name, Version: 1, Producer: ServiceName, Authority: "future Order Guarantee authority", Status: CatalogueFutureDisabled, PayloadFields: []string{}, Notes: "Reserved only; no emitter, worker, route, table, or Wallet call exists."}
}

func fields(values ...string) []string { return values }

// ValidateCatalogueVersion gives inbox adapters a single fail-closed version
// gate. Unknown event types and newer/older versions are never reinterpreted as
// the current payload contract.
func ValidateCatalogueVersion(eventType string, version int) error {
	for _, event := range Section44Catalogue() {
		if event.Name == eventType {
			if event.Version == version && event.Status != CatalogueFutureDisabled {
				return nil
			}
			return fmt.Errorf("%w: %s@v%d", ErrUnsupportedEventVersion, eventType, version)
		}
	}
	return fmt.Errorf("%w: %s@v%d", ErrUnsupportedEventVersion, eventType, version)
}

var forbiddenCatalogueFieldFragments = []string{
	"phone", "email", "full_address", "otp", "passcode", "qr_secret", "access_token",
	"payment_credential", "bank_credential", "pan", "cvv", "api_key", "hmac_secret",
	"service_secret", "gateway_secret", "private_key", "signed_media_url",
}

// ValidateSection44Catalogue enforces the machine-contract invariants used by
// the deterministic parity artifact and the certification tests.
func ValidateSection44Catalogue(catalogue []CatalogueEvent) error {
	seen := map[string]bool{}
	for _, event := range catalogue {
		if strings.TrimSpace(event.Name) == "" || event.Version < 1 || strings.TrimSpace(event.Producer) == "" || strings.TrimSpace(event.Authority) == "" {
			return fmt.Errorf("event catalogue: incomplete identity for %q", event.Name)
		}
		key := fmt.Sprintf("%s@v%d", event.Name, event.Version)
		if seen[key] {
			return fmt.Errorf("event catalogue: duplicate canonical event %s", key)
		}
		seen[key] = true
		if event.Status == CatalogueActive && len(event.Aliases) == 0 {
			return fmt.Errorf("event catalogue: active event %s has no implementation mapping", event.Name)
		}
		if event.Status == CatalogueFutureDisabled && len(event.Aliases) != 0 {
			return fmt.Errorf("event catalogue: future event %s claims an emitter", event.Name)
		}
		for _, field := range event.PayloadFields {
			lower := strings.ToLower(field)
			for _, forbidden := range forbiddenCatalogueFieldFragments {
				if strings.Contains(lower, forbidden) {
					return fmt.Errorf("event catalogue: %s exposes forbidden field %s", event.Name, field)
				}
			}
		}
		if event.Status == CatalogueActive && event.Producer == ServiceName && len(event.Aliases) == 0 {
			return fmt.Errorf("event catalogue: active Vendor event %s has no implemented emitter mapping", event.Name)
		}
	}
	return nil
}
