package adminevents

// Compiled allowlist of owner events exported to HQ. Free text (chat bodies,
// subjects, notes), names, phones, coordinates, document numbers and bank
// data have no field kind and can never be added here.
const (
	SupportCaseChanged       = "support.case.changed.v1"
	VerificationCaseChanged  = "verification.vendor_case.changed.v1"
	NotificationCampaignStat = "notification.campaign.changed.v1"
)

func init() {
	opt := Field{Kind: FieldOpaqueNullable}
	count := Field{Kind: FieldCount, Required: true}
	Register(Adapter{SourceService: "support-dispute-service", SourceType: SupportCaseChanged, ExportType: "karryam.support.case.updated.v1", EntityType: "support_case", Classification: ClassOperationsMinim, Fields: map[string]Field{
		"status":                 {Kind: FieldEnum, Required: true, Enum: []string{"OPEN", "UNDER_REVIEW", "AWAITING_EVIDENCE", "RESOLVED", "CLOSED", "ESCALATED"}},
		"priority":               {Kind: FieldEnum, Required: true, Enum: []string{"NORMAL", "HIGH", "EMERGENCY"}},
		"service_type":           {Kind: FieldOpaque, Required: true},
		"category":               opt,
		"complainant_type":       {Kind: FieldOpaque, Required: true},
		"context_type":           opt,
		"locale":                 opt,
		"sla_state":              {Kind: FieldEnum, Required: true, Enum: []string{"POLICY_UNAVAILABLE"}},
		"consent_classification": {Kind: FieldEnum, Required: true, Enum: []string{"OPERATIONS_MINIMIZED", "SAFETY_EVIDENCE"}},
		"assignee":               opt,
		"escalation_level":       opt,
		"mission_ref":            opt,
		"order_ref":              opt,
		"dispute_ref":            opt,
		"unread_count":           count,
		"message_count":          count,
		"evidence_count":         count,
		"created_at":             {Kind: FieldTime, Required: true},
		"updated_at":             {Kind: FieldTime, Required: true},
	}})
	Register(Adapter{SourceService: "verification-compliance-service", SourceType: VerificationCaseChanged, ExportType: "karryam.verification.case.updated.v1", EntityType: "vendor_verification_case", Classification: ClassOperationsMinim, Fields: map[string]Field{
		"status":                    {Kind: FieldEnum, Required: true, Enum: []string{"NOT_STARTED", "IN_PROGRESS", "PENDING_REVIEW", "VERIFIED", "REJECTED", "EXPIRED", "SUSPENDED"}},
		"dimension":                 {Kind: FieldEnum, Required: true, Enum: []string{"IDENTITY", "BUSINESS", "BANK", "LOCATION", "BRAND", "FOOD_KITCHEN"}},
		"subject_type":              {Kind: FieldOpaque, Required: true},
		"business_ref":              {Kind: FieldOpaque, Required: true},
		"automation_recommendation": {Kind: FieldEnum, Enum: []string{"FORMAT_VALID", "SOURCE_UNVERIFIED", "REVIEW_REQUIRED", "AUTO_ELIGIBLE", "NOT_EVALUATED"}},
		"reason_code":               opt,
		"expires_at":                {Kind: FieldTimeNullable},
		"updated_at":                {Kind: FieldTime, Required: true},
	}})
	Register(Adapter{SourceService: "notification-service", SourceType: NotificationCampaignStat, ExportType: "karryam.marketing.campaign.updated.v1", EntityType: "campaign_revision", Classification: ClassInternal, Fields: map[string]Field{
		"campaign_id":           {Kind: FieldOpaque, Required: true},
		"revision":              count,
		"state":                 {Kind: FieldEnum, Required: true, Enum: []string{"ACCEPTED_BY_KARRYGO", "PLANNED", "QUEUED", "SENT", "DELIVERED", "REJECTED", "FAILED", "SUPPRESSED", "CANCELLED", "COMPLETED"}},
		"rejection_code":        opt,
		"audience_count":        count,
		"eligible_count":        count,
		"suppressed_count":      count,
		"queued_count":          count,
		"sent_count":            count,
		"logged_only_count":     count,
		"failed_count":          count,
		"delivered_count":       {Kind: FieldCount},
		"delivery_confirmation": {Kind: FieldEnum, Required: true, Enum: []string{"UNAVAILABLE", "PROVIDER_RECEIPTS"}},
		"updated_at":            {Kind: FieldTime, Required: true},
	}})
}
