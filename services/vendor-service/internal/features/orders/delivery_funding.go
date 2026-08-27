package orders

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/google/uuid"
)

type DeliveryFundingSnapshot struct {
	Currency                   string `json:"currency"`
	AuthoritativeDeliveryMinor int64  `json:"authoritative_delivery_minor"`
	CustomerContributionMinor  int64  `json:"customer_contribution_minor"`
	VendorSubsidyMinor         int64  `json:"vendor_subsidy_minor"`
	PlatformSubsidyMinor       int64  `json:"platform_subsidy_minor"`
	PlatformApprovalRef        string `json:"platform_approval_ref,omitempty"`
}

type HybridFundingSponsor string

const (
	HybridFundingVendor   HybridFundingSponsor = "VENDOR"
	HybridFundingPlatform HybridFundingSponsor = "PLATFORM"
	HybridFundingCombined HybridFundingSponsor = "COMBINED"
)

func (f DeliveryFundingSnapshot) Validate() error {
	if len(f.Currency) != 3 || f.AuthoritativeDeliveryMinor < 0 || f.CustomerContributionMinor < 0 || f.VendorSubsidyMinor < 0 || f.PlatformSubsidyMinor < 0 || f.CustomerContributionMinor+f.VendorSubsidyMinor+f.PlatformSubsidyMinor != f.AuthoritativeDeliveryMinor {
		return ErrAllocationImbalance
	}
	return nil
}

// ReconcileHybridFunding preserves the customer-approved contribution and
// allocates only the automatic fallback gap to the configured sponsors.
func ReconcileHybridFunding(current DeliveryFundingSnapshot, actualMinor int64, sponsor HybridFundingSponsor, vendorShareBPS int) (DeliveryFundingSnapshot, error) {
	if current.Validate() != nil || actualMinor < current.CustomerContributionMinor || vendorShareBPS < 0 || vendorShareBPS > 10000 {
		return DeliveryFundingSnapshot{}, ErrAllocationImbalance
	}
	result := DeliveryFundingSnapshot{Currency: current.Currency, AuthoritativeDeliveryMinor: actualMinor, CustomerContributionMinor: current.CustomerContributionMinor}
	gap := actualMinor - result.CustomerContributionMinor
	switch sponsor {
	case HybridFundingVendor:
		result.VendorSubsidyMinor = gap
	case HybridFundingPlatform:
		result.PlatformSubsidyMinor = gap
	case HybridFundingCombined:
		if gap > 0 && vendorShareBPS > 0 {
			result.VendorSubsidyMinor = gap * int64(vendorShareBPS) / 10000
		}
		result.PlatformSubsidyMinor = gap - result.VendorSubsidyMinor
	default:
		return DeliveryFundingSnapshot{}, ErrAllocationImbalance
	}
	return result, result.Validate()
}

type DeliveryFundingRequest struct {
	SourceReference, VendorOrderID, SponsoringVendorID, ProviderType, ProviderID, Currency          string
	AuthoritativeDeliveryMinor, CustomerContributionMinor, VendorSubsidyMinor, PlatformSubsidyMinor int64
	CustomerPaymentReference, IdempotencyKey                                                        string
	PlatformApprovalRef                                                                             string
}
type DeliveryFundingResult struct {
	Reference, Status                                                                               string
	VendorOrderID, SponsoringVendorID                                                               string
	AuthoritativeDeliveryMinor, CustomerContributionMinor, VendorSubsidyMinor, PlatformSubsidyMinor int64
}

func (r DeliveryFundingResult) Applied() bool { return r.Status == "APPLIED" }

type DeliveryFundingAuthority interface {
	ApplyDeliveryFunding(context.Context, DeliveryFundingRequest) (DeliveryFundingResult, error)
	GetDeliveryFunding(context.Context, string) (DeliveryFundingResult, error)
}

func (s *Service) SetDeliveryFundingAuthority(authority DeliveryFundingAuthority) {
	s.deliveryFunding = authority
}

func (s *Service) ensureDeliveryFunding(ctx context.Context, order ParentOrder, vendor VendorOrder, providerType, providerID string) error {
	if vendor.DeliveryGrossMinor == 0 || vendor.DeliveryVendorFundedMinor+vendor.DeliveryPlatformFundedMinor == 0 {
		return nil
	}
	if s.deliveryFunding == nil {
		return ErrDependencyMissing
	}
	snapshot := DeliveryFundingSnapshot{Currency: order.Currency, AuthoritativeDeliveryMinor: vendor.DeliveryGrossMinor, CustomerContributionMinor: vendor.DeliveryCustomerMinor, VendorSubsidyMinor: vendor.DeliveryVendorFundedMinor, PlatformSubsidyMinor: vendor.DeliveryPlatformFundedMinor}
	if err := snapshot.Validate(); err != nil {
		return err
	}
	platformApprovalRef := ""
	var err error
	if snapshot.PlatformSubsidyMinor > 0 {
		platformApprovalRef, err = s.platformFundingApproval(ctx, order, vendor)
		if err != nil || platformApprovalRef == "" {
			return ErrPaymentAuthority
		}
		snapshot.PlatformApprovalRef = platformApprovalRef
	}
	var paymentRef string
	if snapshot.CustomerContributionMinor > 0 {
		if err := s.store.db.QueryRow(ctx, `SELECT intent_reference FROM commerce_payment_legs WHERE vendor_order_id=$1 AND status IN ('HELD','SETTLED') ORDER BY created_at LIMIT 1`, vendor.ID).Scan(&paymentRef); err != nil {
			return ErrPaymentConflict
		}
	}
	sourceRef := "delivery-funding:" + vendor.ID
	key := sourceRef
	raw, _ := json.Marshal(snapshot)
	_, err = s.store.db.Exec(ctx, `INSERT INTO commerce_delivery_funding_operations(id,parent_order_id,vendor_order_id,business_id,currency,authoritative_delivery_minor,
		customer_contribution_minor,vendor_subsidy_minor,platform_subsidy_minor,idempotency_key,source_snapshot)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (vendor_order_id) DO NOTHING`, "funding_"+uuid.NewString(), order.ID, vendor.ID, vendor.BusinessID, order.Currency, snapshot.AuthoritativeDeliveryMinor, snapshot.CustomerContributionMinor, snapshot.VendorSubsidyMinor, snapshot.PlatformSubsidyMinor, key, raw)
	if err != nil {
		return err
	}
	var status, reference string
	if err = s.store.db.QueryRow(ctx, `SELECT wallet_status,wallet_operation_ref FROM commerce_delivery_funding_operations WHERE vendor_order_id=$1`, vendor.ID).Scan(&status, &reference); err != nil {
		return err
	}
	if status == "APPLIED" {
		return nil
	}
	request := DeliveryFundingRequest{SourceReference: sourceRef, VendorOrderID: vendor.ID, SponsoringVendorID: vendor.BusinessID, ProviderType: providerType, ProviderID: providerID, Currency: order.Currency,
		AuthoritativeDeliveryMinor: snapshot.AuthoritativeDeliveryMinor, CustomerContributionMinor: snapshot.CustomerContributionMinor, VendorSubsidyMinor: snapshot.VendorSubsidyMinor, PlatformSubsidyMinor: snapshot.PlatformSubsidyMinor, CustomerPaymentReference: paymentRef, IdempotencyKey: key}
	request.PlatformApprovalRef = platformApprovalRef
	var result DeliveryFundingResult
	if status == "UNKNOWN" && strings.TrimSpace(reference) != "" {
		result, err = s.deliveryFunding.GetDeliveryFunding(ctx, sourceRef)
	} else {
		result, err = s.deliveryFunding.ApplyDeliveryFunding(ctx, request)
	}
	if err != nil {
		_, _ = s.store.db.Exec(ctx, `UPDATE commerce_delivery_funding_operations SET wallet_status='UNKNOWN',updated_at=now() WHERE vendor_order_id=$1`, vendor.ID)
		return err
	}
	if !result.Applied() || result.VendorOrderID != vendor.ID || result.SponsoringVendorID != vendor.BusinessID || result.AuthoritativeDeliveryMinor != snapshot.AuthoritativeDeliveryMinor || result.CustomerContributionMinor != snapshot.CustomerContributionMinor || result.VendorSubsidyMinor != snapshot.VendorSubsidyMinor || result.PlatformSubsidyMinor != snapshot.PlatformSubsidyMinor {
		return ErrPaymentAuthority
	}
	tag, err := s.store.db.Exec(ctx, `UPDATE commerce_delivery_funding_operations SET wallet_operation_ref=$2,wallet_status='APPLIED',version=version+1,updated_at=now() WHERE vendor_order_id=$1 AND wallet_status<>'APPLIED'`, vendor.ID, result.Reference)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	return nil
}

func (s *Service) platformFundingApproval(ctx context.Context, order ParentOrder, vendor VendorOrder) (string, error) {
	seen := map[string]bool{}
	var adjustments []AdjustmentSource
	if len(vendor.PromotionSnapshot) > 0 && json.Unmarshal(vendor.PromotionSnapshot, &adjustments) == nil {
		for _, adjustment := range adjustments {
			if ref := strings.TrimSpace(adjustment.PlatformApprovalRef); ref != "" {
				seen[ref] = true
			}
		}
	}
	if len(seen) == 0 {
		var raw []byte
		if err := s.store.db.QueryRow(ctx, `SELECT delivery_policy_snapshot FROM commerce_fulfilment_plans WHERE id=$1`, order.FulfilmentPlanID).Scan(&raw); err != nil {
			return "", err
		}
		var policy struct {
			PlatformApprovalRef string `json:"hybrid_platform_approval_ref"`
		}
		if json.Unmarshal(raw, &policy) == nil && strings.TrimSpace(policy.PlatformApprovalRef) != "" {
			seen[strings.TrimSpace(policy.PlatformApprovalRef)] = true
		}
	}
	values := make([]string, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	sort.Strings(values)
	return strings.Join(values, ","), nil
}
