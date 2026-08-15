package crm

// Segment dependency keys introduced by the Section 8 completion.
//
// These name what a rule READS, which is what dependency-scoped invalidation
// needs: when a loyalty ladder is redefined, only segments that actually
// mention a loyalty field are restaked. Invalidating everything on every
// projection change would be correct and useless — a business with a hundred
// segments would rebuild all of them every time one customer's points moved.
const (
	DependencyLoyaltyTier        = "crm.loyalty.tier"
	DependencyCustomerValue      = "crm.customer_value"
	DependencyRelationshipSignal = "crm.relationship_signals"
	DependencyOrderAffinity      = "crm.order_affinity"
	DependencyServiceRecovery    = "crm.service_recovery"
	DependencyCorporateRFQ       = "crm.corporate_rfq"
)

// Field sources for the completion's derived fields. Loyalty and relationship
// signals are their own groups rather than being folded into INTELLIGENCE,
// because a builder UI grouping "at risk" next to "churn risk score" would
// imply they are the same measurement read two ways — they are not: one is the
// score, the other is a policy applied to it.
const (
	SourceLoyalty            = "LOYALTY"
	SourceRelationshipSignal = "RELATIONSHIP_SIGNAL"
	SourceCustomerValue      = "CUSTOMER_VALUE"
)
