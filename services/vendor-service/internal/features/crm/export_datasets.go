package crm

import (
	"strings"
)

// Section 8H — the export dataset registry.
//
// This file is the entire boundary between an export request and the database.
// A request selects a dataset key from a closed set; the dataset's SQL, its
// scope predicates and its field list are written here by hand. There is no
// table name, column name, operator, sort or expression anywhere that could
// come from a client, and every filter value travels as a bind parameter.
//
// The field lists are equally deliberate. Each is an allowlist, not a
// projection of whatever the query happens to return, so adding a column to a
// CRM table can never silently widen an export. Nothing in any list is a
// canonical contact detail: no email address, phone number, postal address,
// date of birth, government identifier, authentication material, payment
// credential or notification destination appears in any dataset, because the
// tables behind them do not hold one.
//
// Free-text CRM content is excluded on purpose. Interaction note bodies, task
// descriptions, loyalty adjustment reasons, campaign message bodies and
// support evidence are all higher-risk than the structured facts around them,
// and a bulk file is the worst possible place for prose a staff member wrote
// about a customer. A future audited export can carry them under its own
// controls; the default export cannot.

// exportField is one allowlisted column of one dataset.
type exportField struct {
	// Name is the header/JSON key seen by the consumer.
	Name string
	// Expr is the SQL expression that produces it, written here in Go.
	Expr string
}

// exportDataset is a code-defined, versioned dataset descriptor.
type exportDataset struct {
	Key   string
	Label string
	// FieldSetVersion changes whenever the field list changes, so a consumer
	// can tell whether two files of the same type share a shape.
	FieldSetVersion int
	Fields          []exportField
	// Permission notes the scope of the disclosure for documentation; access
	// is enforced by the export permissions themselves.
	Description string
	// build renders the FROM/WHERE for one request. It is the only place a
	// filter reaches SQL, and it always applies tenant and branch scope first.
	build func(q *exportQuery)
	// RequiresFlag names a feature flag the dataset's source needs; an export
	// of a disabled domain is refused rather than returning an empty file that
	// would read as "there is none of this".
	RequiresFlag func(Flags) bool
}

// exportQuery accumulates a dataset's SQL and its bind parameters.
type exportQuery struct {
	from   string
	where  []string
	order  string
	args   []any
	filter ExportFilter
	scope  string
	branch string
	// asOf is the privacy fence: rows are selected against relationships that
	// were already eligible at this instant.
	asOf string
}

func (q *exportQuery) bind(value any) string {
	q.args = append(q.args, value)
	return "$" + itoa(len(q.args))
}

func (q *exportQuery) and(condition string) { q.where = append(q.where, condition) }

// exportDatasets is the closed registry.
var exportDatasets = map[string]exportDataset{
	ExportCustomerSummary: {
		Key: ExportCustomerSummary, Label: "Customer summary", FieldSetVersion: 1,
		Description: "One row per customer relationship: opaque reference, status, acquisition, " +
			"commercial availability and permitted aggregates, and intelligence scores.",
		Fields: []exportField{
			{"profile_ref", "p.id"},
			{"subject_type", "p.subject_type"},
			{"status", "p.status"},
			{"acquisition_source_type", "COALESCE(p.acquisition_source_type,'')"},
			{"created_at", "to_char(p.created_at,'YYYY-MM-DD\"T\"HH24:MI:SSOF')"},
			{"commercial_projection_state", "p.commercial_projection_state"},
			{"completed_order_count", "CASE WHEN p.commercial_projection_state='NOT_AVAILABLE' THEN NULL ELSE p.completed_order_count END"},
			{"net_spend_minor", "CASE WHEN p.commercial_projection_state='NOT_AVAILABLE' THEN NULL ELSE p.net_spend_minor END"},
			{"commercial_currency", "COALESCE(p.commercial_currency,'')"},
			{"last_completed_order_at", "to_char(p.last_completed_order_at,'YYYY-MM-DD\"T\"HH24:MI:SSOF')"},
			{"rfm_state", "COALESCE(rfm.state,'NOT_AVAILABLE')"},
			{"rfm_composite_score", "rfm.composite_score"},
			{"churn_state", "COALESCE(churn.state,'NOT_AVAILABLE')"},
			{"churn_risk_score", "churn.risk_score"},
			{"churn_risk_band", "COALESCE(churn.risk_band,'')"},
		},
		build: func(q *exportQuery) {
			q.from = `vendor_customer_profiles p
				LEFT JOIN crm_customer_intelligence_scores rfm
					ON rfm.business_id=p.business_id AND rfm.customer_profile_id=p.id
					AND rfm.model_key='RFM_V1' AND rfm.is_current AND rfm.state='CURRENT'
					AND rfm.scope=` + q.bind(q.scope) + ` AND COALESCE(rfm.branch_id,'')=` + q.bind(q.branch) + `
				LEFT JOIN crm_customer_intelligence_scores churn
					ON churn.business_id=p.business_id AND churn.customer_profile_id=p.id
					AND churn.model_key='CHURN_V1' AND churn.is_current AND churn.state='CURRENT'
					AND churn.scope=` + q.bind(q.scope) + ` AND COALESCE(churn.branch_id,'')=` + q.bind(q.branch)
			q.order = "p.id"
			if q.scope == IntelligenceScopeBranch {
				// A branch export sees only relationships that actually trade
				// with that branch, so the row count itself cannot be used to
				// infer the size of the business-wide book.
				q.and(`EXISTS (SELECT 1 FROM crm_customer_branch_relations br
					WHERE br.business_id=p.business_id AND br.profile_id=p.id
					  AND br.branch_id=` + q.bind(q.branch) + `)`)
			}
			for _, band := range q.filter.RiskBands {
				_ = band
			}
			if len(q.filter.RiskBands) > 0 {
				q.and("churn.risk_band = ANY(" + q.bind(q.filter.RiskBands) + ")")
			}
			if len(q.filter.ScoreStates) > 0 {
				q.and("COALESCE(rfm.state,'NOT_AVAILABLE') = ANY(" + q.bind(q.filter.ScoreStates) + ")")
			}
			if len(q.filter.Statuses) > 0 {
				q.and("p.status = ANY(" + q.bind(q.filter.Statuses) + ")")
			}
		},
	},

	ExportSegmentMembers: {
		Key: ExportSegmentMembers, Label: "Segment members", FieldSetVersion: 1,
		Description: "The exact current membership of one segment, bound to the generation that produced it.",
		Fields: []exportField{
			{"segment_ref", "m.segment_id"},
			{"generation", "m.generation::text"},
			{"profile_ref", "m.profile_id"},
			{"membership_source", "m.membership_source"},
			{"profile_status", "p.status"},
			{"commercial_projection_state", "p.commercial_projection_state"},
		},
		build: func(q *exportQuery) {
			// Bound to the segment's CURRENT generation, so an export can never
			// mix two rebuilds or resurrect a superseded audience.
			q.from = `crm_segment_memberships m
				JOIN crm_segments g ON g.business_id=m.business_id AND g.id=m.segment_id
					AND g.current_evaluation_id IS NOT DISTINCT FROM m.evaluation_id
				JOIN vendor_customer_profiles p ON p.business_id=m.business_id AND p.id=m.profile_id`
			q.order = "m.profile_id"
			q.and("m.segment_id = " + q.bind(q.filter.SegmentRef))
			if q.scope == IntelligenceScopeBranch {
				q.and("g.scope_type='BRANCH' AND g.branch_id = " + q.bind(q.branch))
			}
		},
		RequiresFlag: func(f Flags) bool { return f.SegmentationEnabled },
	},

	ExportCorporateCustomers: {
		Key: ExportCorporateCustomers, Label: "Corporate customers", FieldSetVersion: 1,
		Description: "Organizational CRM data. Contact rows are counted, never listed: a corporate " +
			"export describes the organization, not the people inside it.",
		Fields: []exportField{
			{"corporate_ref", "c.id"},
			{"corporate_customer_code", "c.corporate_customer_code"},
			{"organization_name", "c.organization_name"},
			{"organization_type", "c.organization_type"},
			{"status", "c.status"},
			{"scope_mode", "c.scope_mode"},
			{"account_manager_staff_ref", "COALESCE(c.account_manager_staff_id,'')"},
			{"relationship_started_at", "to_char(c.relationship_started_at,'YYYY-MM-DD\"T\"HH24:MI:SSOF')"},
			{"location_count", `(SELECT count(*) FROM corporate_customer_locations l
				WHERE l.business_id=c.business_id AND l.corporate_customer_id=c.id AND l.status<>'ARCHIVED')`},
			{"cost_center_count", `(SELECT count(*) FROM corporate_customer_cost_centers cc
				WHERE cc.business_id=c.business_id AND cc.corporate_customer_id=c.id AND cc.status<>'ARCHIVED')`},
			{"contact_count", `(SELECT count(*) FROM corporate_customer_contacts ct
				WHERE ct.business_id=c.business_id AND ct.corporate_customer_id=c.id
				  AND ct.status='ACTIVE' AND ct.privacy_suppressed=false)`},
		},
		build: func(q *exportQuery) {
			q.from = "corporate_customer_profiles c"
			q.order = "c.id"
			if q.scope == IntelligenceScopeBranch {
				q.and(`(c.scope_mode='BUSINESS' OR EXISTS (SELECT 1 FROM corporate_customer_branch_scopes bs
					WHERE bs.business_id=c.business_id AND bs.corporate_customer_id=c.id
					  AND bs.branch_id=` + q.bind(q.branch) + `))`)
			}
			if len(q.filter.Statuses) > 0 {
				q.and("c.status = ANY(" + q.bind(q.filter.Statuses) + ")")
			}
		},
		RequiresFlag: func(f Flags) bool { return f.CorporateEnabled },
	},

	ExportLoyaltySummary: {
		Key: ExportLoyaltySummary, Label: "Loyalty summary", FieldSetVersion: 1,
		Description: "One row per loyalty account. Balances are points, never money: there is no " +
			"currency, cash equivalence or wallet field, because loyalty points are not a financial instrument.",
		Fields: []exportField{
			{"profile_ref", "a.customer_profile_id"},
			{"account_state", "a.status"},
			{"available_points", "a.current_points"},
			{"lifetime_credited_points", "a.lifetime_credited_points"},
			{"lifetime_debited_points", "a.lifetime_debited_points"},
			{"next_expiry_at", `to_char((SELECT min(l.expires_at) FROM crm_loyalty_point_lots l
				WHERE l.business_id=a.business_id AND l.loyalty_account_id=a.id
				  AND l.remaining_points > 0 AND l.expires_at IS NOT NULL),'YYYY-MM-DD"T"HH24:MI:SSOF')`},
			{"next_expiry_points", `(SELECT COALESCE(sum(l.remaining_points),0) FROM crm_loyalty_point_lots l
				WHERE l.business_id=a.business_id AND l.loyalty_account_id=a.id AND l.remaining_points > 0
				  AND l.expires_at = (SELECT min(l2.expires_at) FROM crm_loyalty_point_lots l2
					WHERE l2.business_id=a.business_id AND l2.loyalty_account_id=a.id
					  AND l2.remaining_points > 0 AND l2.expires_at IS NOT NULL))`},
		},
		build: func(q *exportQuery) {
			q.from = `crm_loyalty_accounts a
				JOIN vendor_customer_profiles p ON p.business_id=a.business_id AND p.id=a.customer_profile_id`
			q.order = "a.customer_profile_id"
			if q.scope == IntelligenceScopeBranch {
				q.and(`EXISTS (SELECT 1 FROM crm_customer_branch_relations br
					WHERE br.business_id=p.business_id AND br.profile_id=p.id
					  AND br.branch_id=` + q.bind(q.branch) + `)`)
			}
		},
		RequiresFlag: func(f Flags) bool { return f.LoyaltyEnabled },
	},

	ExportCampaignSummary: {
		Key: ExportCampaignSummary, Label: "Campaign performance summary", FieldSetVersion: 1,
		Description: "Aggregate operational campaign figures. It counts deliveries; it never names " +
			"a recipient and never carries a destination.",
		Fields: []exportField{
			{"campaign_ref", "r.campaign_id"},
			{"campaign_version", "r.campaign_definition_version::text"},
			{"run_ref", "r.id"},
			{"run_status", "r.status"},
			{"scheduled_for", "to_char(r.scheduled_for,'YYYY-MM-DD\"T\"HH24:MI:SSOF')"},
			{"completed_at", "to_char(r.completed_at,'YYYY-MM-DD\"T\"HH24:MI:SSOF')"},
			{"target_count", "r.total_deliveries"},
			{"handed_off_count", "r.handed_off_count"},
			{"delivered_count", "r.delivered_count"},
			{"suppressed_count", "r.suppressed_count"},
			{"failed_count", "r.failed_count"},
		},
		build: func(q *exportQuery) {
			q.from = `crm_campaign_runs r
				JOIN crm_campaigns cp ON cp.business_id=r.business_id AND cp.id=r.campaign_id`
			q.order = "r.id"
			if q.filter.CampaignRef != "" {
				q.and("r.campaign_id = " + q.bind(q.filter.CampaignRef))
			}
			if q.scope == IntelligenceScopeBranch {
				q.and("cp.scope_type='BRANCH' AND cp.branch_id = " + q.bind(q.branch))
			}
			if len(q.filter.Statuses) > 0 {
				q.and("r.status = ANY(" + q.bind(q.filter.Statuses) + ")")
			}
		},
		RequiresFlag: func(f Flags) bool { return f.CampaignsEnabled },
	},

	ExportIntelligenceScores: {
		Key: ExportIntelligenceScores, Label: "Intelligence scores", FieldSetVersion: 1,
		Description: "Current scores with their model identity, state and reasons. Unavailable rows " +
			"are included with their reason so the file is honest about coverage.",
		Fields: []exportField{
			{"profile_ref", "s.customer_profile_id"},
			{"scope", "s.scope"},
			{"branch_ref", "COALESCE(s.branch_id,'')"},
			{"model_key", "s.model_key"},
			{"model_version", "s.model_version::text"},
			{"state", "s.state"},
			{"recency_score", "s.recency_score"},
			{"frequency_score", "s.frequency_score"},
			{"monetary_score", "s.monetary_score"},
			{"composite_score", "s.composite_score"},
			{"risk_score", "s.risk_score"},
			{"risk_band", "COALESCE(s.risk_band,'')"},
			{"reason_codes", "array_to_string(s.reason_codes,'|')"},
			{"source_as_of", "to_char(s.source_as_of,'YYYY-MM-DD\"T\"HH24:MI:SSOF')"},
			{"computed_at", "to_char(s.computed_at,'YYYY-MM-DD\"T\"HH24:MI:SSOF')"},
		},
		build: func(q *exportQuery) {
			q.from = `crm_customer_intelligence_scores s
				JOIN vendor_customer_profiles p ON p.business_id=s.business_id AND p.id=s.customer_profile_id`
			q.order = "s.customer_profile_id, s.model_key"
			q.and("s.is_current")
			q.and("s.scope = " + q.bind(q.scope))
			q.and("COALESCE(s.branch_id,'') = " + q.bind(q.branch))
			if q.filter.ModelKey != "" {
				q.and("s.model_key = " + q.bind(q.filter.ModelKey))
			}
			if len(q.filter.ScoreStates) > 0 {
				q.and("s.state = ANY(" + q.bind(q.filter.ScoreStates) + ")")
			}
			if len(q.filter.RiskBands) > 0 {
				q.and("s.risk_band = ANY(" + q.bind(q.filter.RiskBands) + ")")
			}
		},
		RequiresFlag: func(f Flags) bool { return f.IntelligenceEnabled },
	},
}

// AllExportTypes lists the registry in a stable order.
var AllExportTypes = []string{
	ExportCustomerSummary, ExportSegmentMembers, ExportCorporateCustomers,
	ExportLoyaltySummary, ExportCampaignSummary, ExportIntelligenceScores,
}

// ExportDatasetFor returns a dataset descriptor.
func ExportDatasetFor(key string) (exportDataset, bool) {
	dataset, ok := exportDatasets[key]
	return dataset, ok
}

// ExportFieldNames lists a dataset's allowlisted field names.
func ExportFieldNames(key string) []string {
	dataset, ok := exportDatasets[key]
	if !ok {
		return nil
	}
	names := make([]string, 0, len(dataset.Fields))
	for _, field := range dataset.Fields {
		names = append(names, field.Name)
	}
	return names
}

// buildExportQuery renders one dataset's SQL for one job.
//
// Tenant scope is applied first and unconditionally, and the privacy exclusion
// is applied to every dataset whose rows are about a customer relationship, so
// a dataset descriptor cannot forget either.
func buildExportQuery(dataset exportDataset, export Export) (string, []any) {
	q := &exportQuery{
		filter: export.Filter, scope: export.Scope, branch: derefOr(export.BranchID),
	}
	// The business predicate uses the first bind slot of every dataset. Each
	// dataset's FROM clause aliases its root table so this one condition works
	// for all of them.
	business := q.bind(export.BusinessID)
	dataset.build(q)

	root := datasetRootAlias(dataset.Key)
	conditions := append([]string{root + ".business_id = " + business}, q.where...)
	// Privacy exclusion is applied here rather than inside each descriptor, so
	// a dataset added later cannot forget it. Every dataset whose rows are
	// about an individual relationship joins the profile as `p`, and a redacted
	// or merged-away relationship is never exported.
	if exportProfileColumn(dataset.Key) != "" {
		conditions = append(conditions,
			"p.privacy_state <> 'REDACTED'", "p.merge_state <> 'MERGED_SOURCE'")
	}
	fields := make([]string, 0, len(dataset.Fields))
	for _, field := range dataset.Fields {
		fields = append(fields, field.Expr)
	}
	query := "SELECT " + strings.Join(fields, ", ") + " FROM " + q.from +
		" WHERE " + strings.Join(conditions, " AND ") +
		" ORDER BY " + q.order
	return query, q.args
}

// datasetRootAlias names the alias carrying business_id for each dataset.
func datasetRootAlias(key string) string {
	switch key {
	case ExportCustomerSummary:
		return "p"
	case ExportSegmentMembers:
		return "m"
	case ExportCorporateCustomers:
		return "c"
	case ExportLoyaltySummary:
		return "a"
	case ExportCampaignSummary:
		return "r"
	case ExportIntelligenceScores:
		return "s"
	}
	return ""
}

// exportProfileColumn names the column holding a customer profile reference for
// datasets whose rows are about one, so the publish-time privacy
// re-certification knows what to check. An empty result means the dataset is
// not about individual customers and needs no certification.
func exportProfileColumn(key string) string {
	switch key {
	case ExportCustomerSummary:
		return "profile_ref"
	case ExportSegmentMembers:
		return "profile_ref"
	case ExportLoyaltySummary:
		return "profile_ref"
	case ExportIntelligenceScores:
		return "profile_ref"
	}
	return ""
}
