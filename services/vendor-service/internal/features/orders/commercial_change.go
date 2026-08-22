package orders

// Section 17F customer commercial changes and post-payment adjustments.
//
// A change is a new commercial agreement linked to the existing ParentOrder;
// it never edits the original quote.  The quote authority owns revalidation,
// pricing, promotions, tax and fulfilment serviceability.  This package only
// records the chain, obtains explicit approval, and asks Wallet to move the
// resulting delta.

import (
	"context"
	"errors"
	"strings"
	"time"
)

type CommercialChangeState string

const (
	ChangeRequested        CommercialChangeState = "CHANGE_REQUESTED"
	ChangeRequoteReady     CommercialChangeState = "REQUOTE_READY"
	ChangeAwaitingApproval CommercialChangeState = "AWAITING_CUSTOMER_APPROVAL"
	ChangeApproved         CommercialChangeState = "APPROVED"
	ChangeRejected         CommercialChangeState = "REJECTED"
	ChangeExpired          CommercialChangeState = "EXPIRED"
	ChangeApplied          CommercialChangeState = "APPLIED"
)

type CommercialWalletStatus string

const (
	WalletNone            CommercialWalletStatus = "NONE"
	WalletRequested       CommercialWalletStatus = "REQUESTED"
	WalletHeld            CommercialWalletStatus = "HELD"
	WalletRefundRequested CommercialWalletStatus = "REFUND_REQUESTED"
	WalletRefunded        CommercialWalletStatus = "REFUNDED"
	WalletReleased        CommercialWalletStatus = "RELEASED"
	WalletUnknown         CommercialWalletStatus = "UNKNOWN"
	WalletFailed          CommercialWalletStatus = "FAILED"
)

var (
	ErrCommercialChangeNotFound = errors.New("commercial change not found")
	ErrCommercialChangeConflict = errors.New("commercial change conflicts with its current state")
	ErrCommercialChangeExpired  = errors.New("replacement quote expired")
	ErrCommercialChangeRequired = errors.New("replacement quote approval is required")
	ErrAdjustmentNotAvailable   = errors.New("commercial adjustment is not available")
)

// ReplacementQuote is the safe projection needed to link a new immutable
// checkout quote.  It deliberately contains no caller-supplied amount.
type ReplacementQuote struct {
	ID                 string
	Version            int64
	Fingerprint        string
	CustomerID         string
	Address            AddressSnapshot
	FulfilmentPlanID   string
	SelectedMode       string
	TotalMinor         int64
	MaxAuthorizedMinor int64
	Currency           string
	ExpiresAt          time.Time
	Approved           bool
}

// CommercialChangeQuoteAuthority is implemented by the composition root over
// Section 17D's quote service.  Requoting therefore re-runs all existing
// checkout, inventory, logistics, promotion and pricing authorities.
type CommercialChangeQuoteAuthority interface {
	CreateReplacementQuote(ctx context.Context, customerID, checkoutSessionID, fulfilmentPlanID string) (ReplacementQuote, error)
	GetReplacementQuote(ctx context.Context, customerID, quoteID string) (ReplacementQuote, error)
	ApproveReplacementQuote(ctx context.Context, customerID, quoteID, fingerprint string) (ReplacementQuote, error)
}

// SubstitutionReplacementRequest carries the server-resolved Shop candidate
// into the same Section 17D quote authority used by ordinary commercial
// changes. It is optional for backwards-compatible test/deployment adapters.
type SubstitutionCommercialChangeRequest struct {
	CustomerID, CheckoutSessionID, FulfilmentPlanID, CartItemID string
	ItemRef, VariantRef, ListingRef, SellUnitRef                string
	UnitPriceMinor                                              int64
}

type SubstitutionCommercialChangeQuoteAuthority interface {
	CreateSubstitutionReplacementQuote(ctx context.Context, request SubstitutionCommercialChangeRequest) (ReplacementQuote, error)
}

// CommercialChange is the durable customer-visible chain.
type CommercialChange struct {
	ID                  string                 `json:"change_id"`
	ParentOrderID       string                 `json:"parent_order_id"`
	CustomerID          string                 `json:"-"`
	OriginalQuote       PaymentBinding         `json:"original_quote"`
	OriginalAddress     AddressSnapshot        `json:"original_address_snapshot"`
	OriginalPlanID      string                 `json:"original_fulfilment_plan_id"`
	OriginalTotal       int64                  `json:"original_total_minor"`
	RequestedSessionID  string                 `json:"requested_checkout_session_id"`
	RequestedPlanID     string                 `json:"requested_fulfilment_plan_id"`
	ReplacementQuote    ReplacementQuote       `json:"replacement_quote"`
	DeltaMinor          int64                  `json:"delta_minor"`
	Currency            string                 `json:"currency"`
	State               CommercialChangeState  `json:"state"`
	ApprovalFingerprint string                 `json:"approval_fingerprint,omitempty"`
	WalletOperationRef  string                 `json:"wallet_operation_ref,omitempty"`
	WalletStatus        CommercialWalletStatus `json:"wallet_status"`
	Version             int64                  `json:"version"`
	ApprovedAt          *time.Time             `json:"approved_at,omitempty"`
	AppliedAt           *time.Time             `json:"applied_at,omitempty"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
}

// ChangeRequest accepts only new authoritative checkout/plan references.
// Customer identity, totals, fingerprints and allocations are server-owned.
type ChangeRequest struct {
	CheckoutSessionID string `json:"checkout_session_id"`
	FulfilmentPlanID  string `json:"fulfilment_plan_id"`
}

// AdjustmentEvidence is immutable financial evidence for a cheaper fulfilment
// benefit or a customer-change delta.
type AdjustmentEvidence struct {
	ID                      string                 `json:"adjustment_id"`
	ParentOrderID           string                 `json:"parent_order_id"`
	ChangeID                string                 `json:"change_id,omitempty"`
	VendorOrderID           string                 `json:"vendor_order_id,omitempty"`
	Type                    string                 `json:"adjustment_type"`
	OriginalCommitmentMinor int64                  `json:"original_commitment_minor"`
	EligibleFinalMinor      int64                  `json:"eligible_final_minor"`
	DifferenceMinor         int64                  `json:"difference_minor"`
	Policy                  string                 `json:"policy"`
	Action                  string                 `json:"action"`
	IdempotencyKey          string                 `json:"-"`
	WalletReference         string                 `json:"wallet_reference,omitempty"`
	WalletStatus            CommercialWalletStatus `json:"wallet_status"`
	Version                 int64                  `json:"version"`
	CreatedAt               time.Time              `json:"created_at"`
	UpdatedAt               time.Time              `json:"updated_at"`
}

func (s *Service) SetCommercialChangeAuthority(authority CommercialChangeQuoteAuthority) {
	s.changeQuotes = authority
}

func (s *Service) CommercialChangesEnabled() bool {
	return s != nil && s.enabled && s.changeQuotes != nil
}

// RequestCommercialChange creates a replacement quote first, then stores the
// durable link.  A failure to requote leaves the old order untouched.
func (s *Service) RequestCommercialChange(ctx context.Context, customerID, parentOrderID string, request ChangeRequest) (CommercialChange, error) {
	if !s.CommercialChangesEnabled() {
		return CommercialChange{}, ErrDependencyMissing
	}
	customerID = strings.TrimSpace(customerID)
	parentOrderID = strings.TrimSpace(parentOrderID)
	request.CheckoutSessionID = strings.TrimSpace(request.CheckoutSessionID)
	request.FulfilmentPlanID = strings.TrimSpace(request.FulfilmentPlanID)
	if customerID == "" || parentOrderID == "" || request.CheckoutSessionID == "" || request.FulfilmentPlanID == "" {
		return CommercialChange{}, ErrCommercialChangeConflict
	}
	parent, err := s.store.GetParent(ctx, customerID, parentOrderID)
	if err != nil {
		return CommercialChange{}, err
	}
	if request.CheckoutSessionID == parent.CheckoutSessionID && request.FulfilmentPlanID == parent.FulfilmentPlanID {
		return CommercialChange{}, ErrCommercialChangeConflict
	}
	if existing, lookupErr := s.store.ExistingCommercialChange(ctx, customerID, parentOrderID, request.CheckoutSessionID, request.FulfilmentPlanID); lookupErr == nil {
		return existing, nil
	} else if !errors.Is(lookupErr, ErrCommercialChangeNotFound) {
		return CommercialChange{}, lookupErr
	}
	quote, err := s.changeQuotes.CreateReplacementQuote(ctx, customerID, request.CheckoutSessionID, request.FulfilmentPlanID)
	if err != nil {
		return CommercialChange{}, err
	}
	if quote.CustomerID != customerID || quote.ID == "" || quote.Fingerprint == "" || quote.Currency != parent.Currency {
		return CommercialChange{}, ErrCommercialChangeConflict
	}
	return s.store.CreateCommercialChange(ctx, parent, request, quote, s.now().UTC())
}

func (s *Service) GetCommercialChange(ctx context.Context, customerID, changeID string) (CommercialChange, error) {
	if !s.CommercialChangesEnabled() {
		return CommercialChange{}, ErrDependencyMissing
	}
	return s.store.GetCommercialChange(ctx, strings.TrimSpace(customerID), strings.TrimSpace(changeID))
}

// ApproveCommercialChange approves the replacement quote and only then asks
// Wallet to move a positive delta or refund a cheaper commitment.
func (s *Service) ApproveCommercialChange(ctx context.Context, customerID, changeID, fingerprint string) (CommercialChange, error) {
	if !s.CommercialChangesEnabled() || s.payments == nil {
		return CommercialChange{}, ErrDependencyMissing
	}
	change, err := s.store.GetCommercialChange(ctx, strings.TrimSpace(customerID), strings.TrimSpace(changeID))
	if err != nil {
		return CommercialChange{}, err
	}
	if change.State == ChangeApplied {
		return change, nil
	}
	if change.State == ChangeApproved {
		if change.ApprovalFingerprint != strings.TrimSpace(fingerprint) {
			return CommercialChange{}, ErrFingerprintMismatch
		}
		return s.applyApprovedCommercialChange(ctx, change)
	}
	if change.State != ChangeAwaitingApproval && change.State != ChangeRequoteReady {
		return CommercialChange{}, ErrCommercialChangeConflict
	}
	if !change.ReplacementQuote.ExpiresAt.After(s.now().UTC()) {
		_ = s.store.MarkCommercialChangeExpired(ctx, customerID, change.ID, s.now().UTC())
		return CommercialChange{}, ErrCommercialChangeExpired
	}
	approved, err := s.changeQuotes.ApproveReplacementQuote(ctx, customerID, change.ReplacementQuote.ID, strings.TrimSpace(fingerprint))
	if err != nil {
		return CommercialChange{}, err
	}
	if approved.ID != change.ReplacementQuote.ID || approved.CustomerID != customerID ||
		approved.Version != change.ReplacementQuote.Version ||
		approved.Fingerprint != change.ReplacementQuote.Fingerprint ||
		approved.Fingerprint != strings.TrimSpace(fingerprint) ||
		approved.TotalMinor != change.ReplacementQuote.TotalMinor ||
		approved.Currency != change.Currency || !approved.Approved {
		return CommercialChange{}, ErrFingerprintMismatch
	}
	change, err = s.store.MarkCommercialChangeApproved(ctx, change.ID, customerID, approved, fingerprint, s.now().UTC())
	if err != nil {
		return CommercialChange{}, err
	}
	return s.applyApprovedCommercialChange(ctx, change)
}

func (s *Service) RejectCommercialChange(ctx context.Context, customerID, changeID string) (CommercialChange, error) {
	if !s.CommercialChangesEnabled() {
		return CommercialChange{}, ErrDependencyMissing
	}
	return s.store.RejectCommercialChange(ctx, strings.TrimSpace(customerID), strings.TrimSpace(changeID), s.now().UTC())
}

func (s *Service) applyApprovedCommercialChange(ctx context.Context, change CommercialChange) (CommercialChange, error) {
	if change.State == ChangeApplied {
		return change, nil
	}
	if change.DeltaMinor > 0 {
		adjustment, err := s.store.EnsureChangeAdjustment(ctx, change, "POSITIVE_DELTA", "AUTHORIZE", "customer-approved replacement quote", "commerce-change-"+change.ID, s.now().UTC())
		if err != nil {
			return change, err
		}
		if change.WalletStatus == WalletNone || change.WalletStatus == WalletRequested || change.WalletStatus == WalletUnknown {
			if err = s.store.MarkCommercialWalletRequested(ctx, change.ID, s.now().UTC()); err != nil {
				return change, err
			}
			if err = s.store.MarkAdjustmentRequested(ctx, adjustment.ID, s.now().UTC()); err != nil {
				return change, err
			}
			result, err := s.payments.HoldLeg(ctx, HoldLegRequest{
				IdempotencyKey: "commerce-change-" + change.ID,
				SourceService:  PaymentSourceService, SourceReference: change.ID,
				CustomerID: change.CustomerID, AmountKobo: change.DeltaMinor,
				Currency: change.Currency, Metadata: map[string]any{"parent_order_id": change.ParentOrderID, "change_id": change.ID},
			})
			if err != nil {
				_, _ = s.store.MarkCommercialWallet(ctx, change.ID, "", WalletUnknown, s.now().UTC())
				_, _ = s.store.MarkAdjustmentWallet(ctx, adjustment.ID, "", WalletUnknown, s.now().UTC())
				return change, ErrPaymentAuthority
			}
			if !result.Held() {
				return change, ErrPaymentAuthority
			}
			change, err = s.store.MarkCommercialWallet(ctx, change.ID, result.Reference, WalletHeld, s.now().UTC())
			if err != nil {
				return change, err
			}
			if _, err = s.store.MarkAdjustmentWallet(ctx, adjustment.ID, result.Reference, WalletHeld, s.now().UTC()); err != nil {
				return change, err
			}
		} else if change.WalletStatus != WalletHeld {
			return change, ErrPaymentAuthority
		}
	} else if change.DeltaMinor < 0 {
		adjustment, err := s.store.EnsureChangeAdjustment(ctx, change, "CHEAPER_REPLAN", "REFUND", "customer benefit for eligible final delivery charge", "commerce-change-refund-"+change.ID, s.now().UTC())
		if err != nil {
			return change, err
		}
		if change.WalletStatus != WalletRefunded {
			operation, err := s.store.GetOperationForOrder(ctx, change.CustomerID, change.ParentOrderID)
			if err != nil {
				return change, err
			}
			leg, err := s.store.FirstRefundableLeg(operation)
			if err != nil {
				return change, err
			}
			if err = s.store.MarkCommercialWalletRequested(ctx, change.ID, s.now().UTC()); err != nil {
				return change, err
			}
			if err = s.store.MarkAdjustmentRequested(ctx, adjustment.ID, s.now().UTC()); err != nil {
				return change, err
			}
			result, err := s.payments.RefundLeg(ctx, RefundLegRequest{
				IdempotencyKey:   "commerce-change-refund-" + change.ID,
				PaymentReference: leg.IntentReference, SourceService: PaymentSourceService,
				SourceReference: refundSourceReference(leg, operation), AmountKobo: -change.DeltaMinor,
				Currency: leg.Currency, Reason: "customer commercial change benefit",
			})
			if err != nil {
				_, _ = s.store.MarkCommercialWallet(ctx, change.ID, "", WalletUnknown, s.now().UTC())
				_, _ = s.store.MarkAdjustmentWallet(ctx, adjustment.ID, "", WalletUnknown, s.now().UTC())
				return change, ErrPaymentAuthority
			}
			if !result.Refunded() || (result.AmountKobo != 0 && result.AmountKobo != -change.DeltaMinor) {
				_, _ = s.store.MarkCommercialWallet(ctx, change.ID, result.Reference, WalletUnknown, s.now().UTC())
				_, _ = s.store.MarkAdjustmentWallet(ctx, adjustment.ID, result.Reference, WalletUnknown, s.now().UTC())
				return change, ErrPaymentAuthority
			}
			change, err = s.store.MarkCommercialWallet(ctx, change.ID, result.Reference, WalletRefunded, s.now().UTC())
			if err != nil {
				return change, err
			}
			// Recorded on the leg as well, so a later benefit against the same
			// leg cannot take it past its own authorized amount unnoticed.
			if _, err = s.store.MarkAdjustmentRefundedOnLeg(
				ctx, adjustment.ID, leg.ID, result.Reference, -change.DeltaMinor, s.now().UTC(),
			); err != nil {
				return change, err
			}
		}
	}
	return s.store.ApplyCommercialChange(ctx, change, s.now().UTC())
}

// ApplyCheaperFulfilmentBenefit compares customer-facing delivery commitment
// with the eligible final customer amount.  Internal provider operating cost
// is intentionally not an input to this policy.
func (s *Service) ApplyCheaperFulfilmentBenefit(ctx context.Context, parentOrderID string, eligibleFinalCustomerMinor int64, policy string) (AdjustmentEvidence, error) {
	if !s.enabled || s.payments == nil {
		return AdjustmentEvidence{}, ErrDependencyMissing
	}
	if strings.TrimSpace(policy) == "" {
		return AdjustmentEvidence{}, ErrAdjustmentNotAvailable
	}
	order, err := s.store.GetParentByID(ctx, strings.TrimSpace(parentOrderID))
	if err != nil {
		return AdjustmentEvidence{}, err
	}
	var committed int64
	for _, vendor := range order.VendorOrders {
		committed += vendor.DeliveryCustomerMinor
	}
	if eligibleFinalCustomerMinor < 0 || eligibleFinalCustomerMinor >= committed {
		return AdjustmentEvidence{}, ErrAdjustmentNotAvailable
	}
	difference := committed - eligibleFinalCustomerMinor
	operation, err := s.store.GetOperationForOrder(ctx, order.CustomerID, order.ID)
	if err != nil {
		return AdjustmentEvidence{}, err
	}
	leg, err := s.store.FirstRefundableDeliveryLeg(operation, order)
	if err != nil {
		return AdjustmentEvidence{}, err
	}
	if difference > leg.AmountMinor {
		return AdjustmentEvidence{}, ErrAdjustmentNotAvailable
	}
	key := "commerce-cheaper-replan-" + order.ID
	adjustment, created, err := s.store.EnsureAdjustment(ctx, order, leg, difference, committed, eligibleFinalCustomerMinor, policy, key, s.now().UTC())
	if err != nil {
		return AdjustmentEvidence{}, err
	}
	if !created && adjustment.WalletStatus == WalletRefunded {
		return adjustment, nil
	}
	if err = s.store.MarkAdjustmentRequested(ctx, adjustment.ID, s.now().UTC()); err != nil {
		return adjustment, err
	}
	result, err := s.payments.RefundLeg(ctx, RefundLegRequest{IdempotencyKey: key, PaymentReference: leg.IntentReference, SourceService: PaymentSourceService, SourceReference: refundSourceReference(leg, operation), AmountKobo: difference, Currency: leg.Currency, Reason: policy})
	if err != nil {
		_, _ = s.store.MarkAdjustmentWallet(ctx, adjustment.ID, "", WalletUnknown, s.now().UTC())
		return adjustment, ErrPaymentAuthority
	}
	if !result.Refunded() || (result.AmountKobo != 0 && result.AmountKobo != difference) {
		_, _ = s.store.MarkAdjustmentWallet(ctx, adjustment.ID, result.Reference, WalletUnknown, s.now().UTC())
		return adjustment, ErrPaymentAuthority
	}
	// Recorded on the leg as well as the adjustment, so the schema's own
	// over-refund guard can see this money.
	return s.store.MarkAdjustmentRefundedOnLeg(
		ctx, adjustment.ID, leg.ID, result.Reference, difference, s.now().UTC(),
	)
}

// RecoverCommercialChanges converges approvals that were interrupted around a
// Wallet call. It is safe after a process restart because positive deltas use
// the deterministic HoldLeg key and cheaper benefits use the deterministic
// refund key.
func (s *Service) RecoverCommercialChanges(ctx context.Context, limit int) (int, error) {
	if !s.CommercialChangesEnabled() || s.payments == nil {
		return 0, nil
	}
	changes, err := s.store.ListCommercialChangesForRecovery(ctx, limit)
	if err != nil {
		return 0, err
	}
	var applied int
	for _, change := range changes {
		if change.DeltaMinor > 0 && change.WalletOperationRef != "" {
			if result, readErr := s.payments.GetLeg(ctx, change.WalletOperationRef); readErr == nil && result.Held() {
				change, _ = s.store.MarkCommercialWallet(ctx, change.ID, change.WalletOperationRef, WalletHeld, s.now().UTC())
			}
		}
		if _, applyErr := s.applyApprovedCommercialChange(ctx, change); applyErr == nil {
			applied++
		}
	}
	return applied, nil
}

func (s *Service) RecoverCommercialAdjustments(ctx context.Context, limit int) (int, error) {
	if !s.enabled || s.payments == nil {
		return 0, nil
	}
	adjustments, err := s.store.ListAdjustmentsForRecovery(ctx, limit)
	if err != nil {
		return 0, err
	}
	var recovered int
	for _, adjustment := range adjustments {
		if adjustment.Action == "AUTHORIZE" {
			continue
		}
		order, loadErr := s.store.GetParentByID(ctx, adjustment.ParentOrderID)
		if loadErr != nil {
			continue
		}
		operation, loadErr := s.store.GetOperationForOrder(ctx, order.CustomerID, order.ID)
		if loadErr != nil {
			continue
		}
		var leg PaymentLegRecord
		if adjustment.VendorOrderID != "" {
			for _, candidate := range operation.Legs {
				if candidate.VendorOrderID == adjustment.VendorOrderID {
					leg = candidate
					break
				}
			}
		} else if candidate, legErr := s.store.FirstRefundableLeg(operation); legErr == nil {
			leg = candidate
		}
		if leg.IntentReference == "" {
			continue
		}
		result, refundErr := s.payments.RefundLeg(ctx, RefundLegRequest{IdempotencyKey: adjustment.IdempotencyKey, PaymentReference: leg.IntentReference, SourceService: PaymentSourceService, SourceReference: refundSourceReference(leg, operation), AmountKobo: adjustment.DifferenceMinor, Currency: leg.Currency, Reason: adjustment.Policy})
		if refundErr != nil || !result.Refunded() || (result.AmountKobo != 0 && result.AmountKobo != adjustment.DifferenceMinor) {
			continue
		}
		if _, markErr := s.store.MarkAdjustmentWallet(ctx, adjustment.ID, result.Reference, WalletRefunded, s.now().UTC()); markErr == nil {
			recovered++
		}
	}
	return recovered, nil
}
