package orders

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// SubstitutionPolicy is the public, frozen customer vocabulary.  It is
// intentionally closed: callers cannot smuggle a vendor-specific policy into
// an order and later reinterpret it during picking.
type SubstitutionPolicy string

const (
	PolicyExactOnly                   SubstitutionPolicy = "EXACT_ONLY"
	PolicySpecificReplacement         SubstitutionPolicy = "SPECIFIC_REPLACEMENT"
	PolicySameBrand                   SubstitutionPolicy = "SAME_BRAND"
	PolicySimilarWithinPriceTolerance SubstitutionPolicy = "SIMILAR_WITHIN_PRICE_TOLERANCE"
	PolicyVendorSuggestAndApprove     SubstitutionPolicy = "VENDOR_SUGGEST_AND_APPROVE"
	PolicyAutoRefund                  SubstitutionPolicy = "AUTO_REFUND"
	PolicyContactFirst                SubstitutionPolicy = "CONTACT_FIRST"
)

var substitutionPolicies = map[SubstitutionPolicy]struct{}{
	PolicyExactOnly: {}, PolicySpecificReplacement: {}, PolicySameBrand: {},
	PolicySimilarWithinPriceTolerance: {}, PolicyVendorSuggestAndApprove: {},
	PolicyAutoRefund: {}, PolicyContactFirst: {},
}

func (p SubstitutionPolicy) Valid() bool { _, ok := substitutionPolicies[p]; return ok }

func ParseSubstitutionPolicy(value string) (SubstitutionPolicy, error) {
	p := SubstitutionPolicy(strings.ToUpper(strings.TrimSpace(value)))
	if !p.Valid() {
		return "", errors.New("invalid substitution policy")
	}
	return p, nil
}

type SubstitutionDecision string

const (
	SubstitutionForbidden                    SubstitutionDecision = "FORBIDDEN"
	SubstitutionAllowedWithoutNewDecision    SubstitutionDecision = "ALLOWED_WITHOUT_NEW_DECISION"
	SubstitutionRequiresCustomerDecision     SubstitutionDecision = "REQUIRES_CUSTOMER_DECISION"
	SubstitutionRequiresCommercialReapproval SubstitutionDecision = "REQUIRES_COMMERCIAL_REAPPROVAL"
	SubstitutionAutoRefund                   SubstitutionDecision = "AUTO_REFUND"
)

type SubstitutionEvaluation struct {
	Decision SubstitutionDecision
	Reason   string
}

type SubstitutionCaseState string

const (
	CaseOpen              SubstitutionCaseState = "OPEN"
	CaseAwaitingCustomer  SubstitutionCaseState = "AWAITING_CUSTOMER"
	CaseApproved          SubstitutionCaseState = "APPROVED"
	CaseRejected          SubstitutionCaseState = "REJECTED"
	CaseAutoRefundPending SubstitutionCaseState = "AUTO_REFUND_PENDING"
	CaseResolved          SubstitutionCaseState = "RESOLVED"
	CaseExpired           SubstitutionCaseState = "EXPIRED"
	CaseCancelled         SubstitutionCaseState = "CANCELLED"
)

type SubstitutionCase struct {
	ID, ParentOrderID, VendorOrderID, OrderLineID, CustomerID string
	BusinessID, BranchID                                      string
	UnavailableQuantity                                       int64
	OriginalLineSnapshot                                      json.RawMessage
	PolicySnapshot                                            json.RawMessage
	QualitySnapshot                                           json.RawMessage
	DeadlineAt                                                *time.Time
	FallbackPolicy                                            SubstitutionPolicy
	State                                                     SubstitutionCaseState
	Reason, IdempotencyKey, CorrelationRef                    string
	Version                                                   int64
	CreatedAt, UpdatedAt                                      time.Time
	ResolvedAt                                                *time.Time
}

type SubstitutionCandidate struct {
	Active, Sellable, SameBusiness, SameBranch           bool
	SameBrand, CategoryCompatible, MeasurementCompatible bool
	QualityCompatible, InventoryAvailable                bool
	OriginalPriceMinor, CandidatePriceMinor              int64
	ToleranceMinor                                       int64
	ToleranceBPS                                         int64
	RequiresCommercialApproval                           bool
}

// EvaluateSubstitution is the one deterministic policy evaluator used by
// vendor proposals, customer decisions and timeout workers.
func EvaluateSubstitution(policy SubstitutionPolicy, c SubstitutionCandidate) SubstitutionEvaluation {
	if !policy.Valid() {
		return SubstitutionEvaluation{SubstitutionForbidden, "INVALID_POLICY"}
	}
	if policy == PolicyExactOnly {
		return SubstitutionEvaluation{SubstitutionForbidden, "EXACT_ONLY"}
	}
	if !c.Active || !c.Sellable || !c.SameBusiness || !c.SameBranch || !c.MeasurementCompatible || !c.InventoryAvailable {
		return SubstitutionEvaluation{SubstitutionForbidden, "CANDIDATE_NOT_SELLABLE"}
	}
	switch policy {
	case PolicySpecificReplacement:
		if !c.CategoryCompatible {
			return SubstitutionEvaluation{SubstitutionForbidden, "CATEGORY_MISMATCH"}
		}
		if c.RequiresCommercialApproval {
			return SubstitutionEvaluation{SubstitutionRequiresCommercialReapproval, "PRICE_REAPPROVAL"}
		}
		return SubstitutionEvaluation{SubstitutionAllowedWithoutNewDecision, "SPECIFIC_REPLACEMENT"}
	case PolicySameBrand:
		if !c.SameBrand || !c.CategoryCompatible || !c.QualityCompatible {
			return SubstitutionEvaluation{SubstitutionForbidden, "BRAND_OR_QUALITY_MISMATCH"}
		}
		if c.RequiresCommercialApproval {
			return SubstitutionEvaluation{SubstitutionRequiresCommercialReapproval, "PRICE_REAPPROVAL"}
		}
		return SubstitutionEvaluation{SubstitutionAllowedWithoutNewDecision, "SAME_BRAND"}
	case PolicySimilarWithinPriceTolerance:
		if !c.CategoryCompatible || !c.QualityCompatible {
			return SubstitutionEvaluation{SubstitutionForbidden, "SIMILARITY_MISMATCH"}
		}
		delta := c.CandidatePriceMinor - c.OriginalPriceMinor
		if delta < 0 {
			delta = -delta
		}
		allowed := c.ToleranceMinor
		bps := c.OriginalPriceMinor * c.ToleranceBPS / 10000
		if bps > allowed {
			allowed = bps
		}
		if delta > allowed {
			return SubstitutionEvaluation{SubstitutionRequiresCommercialReapproval, "PRICE_OUTSIDE_TOLERANCE"}
		}
		return SubstitutionEvaluation{SubstitutionAllowedWithoutNewDecision, "SIMILAR_WITHIN_TOLERANCE"}
	case PolicyVendorSuggestAndApprove, PolicyContactFirst:
		return SubstitutionEvaluation{SubstitutionRequiresCustomerDecision, "CUSTOMER_DECISION_REQUIRED"}
	case PolicyAutoRefund:
		return SubstitutionEvaluation{SubstitutionAutoRefund, "AUTO_REFUND"}
	default:
		return SubstitutionEvaluation{SubstitutionForbidden, "UNSUPPORTED_POLICY"}
	}
}
