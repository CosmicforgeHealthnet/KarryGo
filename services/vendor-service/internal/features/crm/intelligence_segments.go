package crm

// Section 8H — intelligence fields in the Section 8B segment registry.
//
// Eight fields, added to the existing closed registry rather than through a
// second engine. That matters more than it looks: a separate "score segment"
// mechanism would have been a second place for branch scope, privacy exclusion
// and unavailable-is-not-zero to be implemented, and therefore a second place
// for them to be implemented differently.
//
// The load-bearing property is the availability guard. A score column is
// readable only through a subquery that requires state='CURRENT', so a
// relationship whose score is NOT_AVAILABLE, STALE or FAILED yields SQL NULL
// rather than a number. Combined with the rule's outer `IS TRUE`, that means
// `churn.risk_score GT 70` cannot be satisfied by a missing score, and
// `rfm.composite_score LT 5` cannot be satisfied by a relationship that was
// never scored at all — which is exactly the trap a default of zero would set.

// intelligenceFields registers the eight Section 8H fields.
func intelligenceFields() []*SegmentField {
	guard := "intelligence"
	return []*SegmentField{
		{
			Key: "intelligence.rfm.state", Label: "RFM availability", Type: FieldEnum, Source: SourceIntelligence,
			Description: "Whether a current RFM score exists for this relationship at this scope.",
			Operators:   enumOperators,
			EnumValues:  []string{ScoreNotAvailable, ScoreCurrent, ScoreStale, ScoreFailed},
			// Deliberately unguarded by CURRENT: this field's whole purpose is
			// to let a rule ask about availability itself, so it must be able
			// to answer NOT_AVAILABLE for a relationship with no score row.
			compile: intelligenceStateCompiler(ModelRFM),
		},
		{
			Key: "intelligence.rfm.recency_score", Label: "RFM recency", Type: FieldInteger, Source: SourceIntelligence,
			Description: "How recently this relationship ordered, as a 1-5 quintile of this scope's scored cohort.",
			Operators:   numericOperators, AvailabilityGuard: guard,
			compile: intelligenceNumericCompiler(ModelRFM, "recency_score"),
		},
		{
			Key: "intelligence.rfm.frequency_score", Label: "RFM frequency", Type: FieldInteger, Source: SourceIntelligence,
			Description: "How often this relationship orders, as a 1-5 quintile of this scope's scored cohort.",
			Operators:   numericOperators, AvailabilityGuard: guard,
			compile: intelligenceNumericCompiler(ModelRFM, "frequency_score"),
		},
		{
			Key: "intelligence.rfm.monetary_score", Label: "RFM monetary", Type: FieldInteger, Source: SourceIntelligence,
			Description: "How much this relationship spends, as a 1-5 quintile of this scope's scored cohort.",
			Operators:   numericOperators, AvailabilityGuard: guard,
			compile: intelligenceNumericCompiler(ModelRFM, "monetary_score"),
		},
		{
			Key: "intelligence.rfm.composite_score", Label: "RFM composite", Type: FieldInteger, Source: SourceIntelligence,
			Description: "The sum of the three RFM quintiles, 3 to 15.",
			Operators:   numericOperators, AvailabilityGuard: guard,
			compile: intelligenceNumericCompiler(ModelRFM, "composite_score"),
		},
		{
			Key: "intelligence.churn.state", Label: "Churn availability", Type: FieldEnum, Source: SourceIntelligence,
			Description: "Whether a current churn risk score exists for this relationship at this scope.",
			Operators:   enumOperators,
			EnumValues:  []string{ScoreNotAvailable, ScoreCurrent, ScoreStale, ScoreFailed},
			compile:     intelligenceStateCompiler(ModelChurn),
		},
		{
			Key: "intelligence.churn.risk_score", Label: "Churn risk score", Type: FieldInteger, Source: SourceIntelligence,
			Description: "A deterministic 0-100 risk heuristic. It is not a probability and not a prediction.",
			Operators:   numericOperators, AvailabilityGuard: guard,
			compile: intelligenceNumericCompiler(ModelChurn, "risk_score"),
		},
		{
			Key: "intelligence.churn.risk_band", Label: "Churn risk band", Type: FieldEnum, Source: SourceIntelligence,
			Description: "The band of the churn risk heuristic: LOW, MEDIUM, HIGH or CRITICAL.",
			Operators:   enumOperators, AvailabilityGuard: guard,
			EnumValues: []string{RiskLow, RiskMedium, RiskHigh, RiskCritical},
			compile:    intelligenceBandCompiler(ModelChurn),
		},
	}
}

// intelligenceScoreSubquery renders the correlated lookup for one column of
// one model's CURRENT score, at the scope the rule is being evaluated in.
//
// The scope clause is what keeps branch intelligence branch-shaped: a
// branch-scoped segment reads the BRANCH-scope score for its own branch and
// can never reach the business-wide one, so a branch rule cannot infer trade
// at branches the author is not authorised to see.
func intelligenceScoreSubquery(builder *sqlBuilder, modelKey, column, extraWhere string) string {
	scope := IntelligenceScopeBusiness
	branch := ""
	if builder.branch() {
		scope = IntelligenceScopeBranch
		branch = builder.branchID
	}
	query := "SELECT si." + column + " FROM crm_customer_intelligence_scores si" +
		" WHERE si.business_id = p.business_id AND si.customer_profile_id = p.id" +
		" AND si.model_key = " + builder.bind(modelKey) +
		" AND si.scope = " + builder.bind(scope) +
		" AND COALESCE(si.branch_id, '') = " + builder.bind(branch) +
		" AND si.is_current"
	if extraWhere != "" {
		query += " AND " + extraWhere
	}
	return "(" + query + ")"
}

// intelligenceStateCompiler answers about availability. An absent row reads as
// NOT_AVAILABLE, because "we have never scored this relationship" and "we
// scored it and it came out unavailable" are the same answer to a rule author.
func intelligenceStateCompiler(modelKey string) func(*Condition, *sqlBuilder) string {
	return func(condition *Condition, builder *sqlBuilder) string {
		column := "COALESCE(" + intelligenceScoreSubquery(builder, modelKey, "state", "") +
			", " + builder.bind(ScoreNotAvailable) + ")"
		return guardExpression("", enumComparison(column, condition, builder))
	}
}

// intelligenceNumericCompiler answers about a score value, and only ever about
// a usable one: the CURRENT predicate inside the subquery is what makes an
// unavailable score match no numeric comparison instead of matching every
// comparison against zero.
func intelligenceNumericCompiler(modelKey, column string) func(*Condition, *sqlBuilder) string {
	return func(condition *Condition, builder *sqlBuilder) string {
		expression := intelligenceScoreSubquery(builder, modelKey, column,
			"si.state = "+builder.bind(ScoreCurrent))
		return guardExpression(expression+" IS NOT NULL", numericComparison(expression, condition, builder))
	}
}

// intelligenceBandCompiler answers about the churn band. Unlike the state
// field it is guarded on CURRENT, so a relationship without a usable score is
// not quietly LOW risk — the absence of a band is not the safest band.
func intelligenceBandCompiler(modelKey string) func(*Condition, *sqlBuilder) string {
	return func(condition *Condition, builder *sqlBuilder) string {
		expression := intelligenceScoreSubquery(builder, modelKey, "risk_band",
			"si.state = "+builder.bind(ScoreCurrent))
		return guardExpression(expression+" IS NOT NULL", enumComparison(expression, condition, builder))
	}
}

// intelligenceFieldKeys maps a model's dependency class onto the exact field
// keys that read it, so a rebuild restakes only the segments that actually
// reference that model. A churn rebuild leaves an RFM-only segment CURRENT.
func intelligenceFieldKeys(dependency string) []string {
	switch dependency {
	case "intelligence.rfm":
		return []string{
			"intelligence.rfm.state", "intelligence.rfm.recency_score",
			"intelligence.rfm.frequency_score", "intelligence.rfm.monetary_score",
			"intelligence.rfm.composite_score",
		}
	case "intelligence.churn":
		return []string{
			"intelligence.churn.state", "intelligence.churn.risk_score",
			"intelligence.churn.risk_band",
		}
	}
	return nil
}
