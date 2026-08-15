package crm

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Section 8H — intelligence reads and analytics.
//
// Every read here leads with availability. A caller is told what is known
// before it is told a number, because the alternative — a dashboard showing a
// confident distribution built from three scored customers, or worse from none
// — is how missing data turns into a business decision.

// ScoreView is one relationship's score for one model.
type ScoreView struct {
	ModelKey     string     `json:"model_key"`
	ModelVersion int        `json:"model_version"`
	Scope        string     `json:"scope"`
	BranchRef    *string    `json:"branch_ref,omitempty"`
	State        string     `json:"state"`
	ReasonCodes  []string   `json:"reason_codes"`
	SourceAsOf   time.Time  `json:"source_as_of"`
	ComputedAt   time.Time  `json:"computed_at"`
	AsOfStale    bool       `json:"stale"`
	GeneratedAt  *time.Time `json:"-"`

	// RFM_V1 output, present only when this is a CURRENT RFM score.
	RecencyScore   *int `json:"recency_score,omitempty"`
	FrequencyScore *int `json:"frequency_score,omitempty"`
	MonetaryScore  *int `json:"monetary_score,omitempty"`
	CompositeScore *int `json:"composite_score,omitempty"`

	// CHURN_V1 output, present only when this is a CURRENT churn score. The
	// risk score is a heuristic rank, never a probability.
	RiskScore *int    `json:"risk_score,omitempty"`
	RiskBand  *string `json:"risk_band,omitempty"`
}

// CustomerIntelligence is the per-relationship intelligence block.
type CustomerIntelligence struct {
	Scope     string     `json:"scope"`
	BranchRef *string    `json:"branch_ref,omitempty"`
	RFM       *ScoreView `json:"rfm,omitempty"`
	Churn     *ScoreView `json:"churn,omitempty"`
	// RFMState and ChurnState always answer, even when no score row exists, so
	// an absent block is never read as "this customer has no risk".
	RFMState   string `json:"rfm_state"`
	ChurnState string `json:"churn_state"`
}

const scoreViewColumns = `model_key,model_version,scope,branch_id,state,reason_codes,
	source_as_of,computed_at,recency_score,frequency_score,monetary_score,composite_score,
	risk_score,risk_band`

func scanScoreView(row pgx.Row) (ScoreView, error) {
	var v ScoreView
	err := row.Scan(&v.ModelKey, &v.ModelVersion, &v.Scope, &v.BranchRef, &v.State, &v.ReasonCodes,
		&v.SourceAsOf, &v.ComputedAt, &v.RecencyScore, &v.FrequencyScore, &v.MonetaryScore,
		&v.CompositeScore, &v.RiskScore, &v.RiskBand)
	return v, err
}

// readingScope decides which intelligence scope a caller may read.
//
// A global actor reads BUSINESS. A branch actor reads the BRANCH scope of a
// branch they hold and is never handed the business figure, because a
// business-wide score summarises trade at branches they cannot see. When a
// branch actor holds several branches, the caller must name one: silently
// picking the first would present one branch's numbers as if they were the
// actor's whole reach.
func readingScope(scope Scope, branchRef string) (string, *string, bool) {
	if branchRef != "" {
		if !scope.Global && !containsString(scope.BranchIDs, branchRef) {
			return "", nil, false
		}
		ref := branchRef
		return IntelligenceScopeBranch, &ref, true
	}
	if scope.Global {
		return IntelligenceScopeBusiness, nil, true
	}
	if len(scope.BranchIDs) == 1 {
		ref := scope.BranchIDs[0]
		return IntelligenceScopeBranch, &ref, true
	}
	return "", nil, false
}

// CustomerIntelligenceFor reads one relationship's scores at the caller's scope.
func (s *Service) CustomerIntelligenceFor(ctx context.Context, actor Actor, scope Scope,
	profile Profile, branchRef string) (CustomerIntelligence, error) {
	if err := s.assertIntelligenceAvailable(); err != nil {
		return CustomerIntelligence{}, err
	}
	scopeType, branchID, ok := readingScope(scope, branchRef)
	if !ok {
		return CustomerIntelligence{}, errIntelligenceScopeDenied()
	}
	out := CustomerIntelligence{
		Scope: scopeType, BranchRef: branchID,
		RFMState: ScoreNotAvailable, ChurnState: ScoreNotAvailable,
	}
	// A redacted or merged-source relationship has no readable intelligence at
	// all. The suppression handler already demoted its rows; this check means a
	// read cannot depend on that having run first.
	if profile.PrivacyState == PrivacyRedacted || profile.MergeState == MergeSource {
		return out, nil
	}

	for _, key := range AllModelKeys {
		view, err := scanScoreView(s.db.QueryRow(ctx, `SELECT `+scoreViewColumns+`
			FROM crm_customer_intelligence_scores
			WHERE business_id=$1 AND customer_profile_id=$2 AND model_key=$3
			  AND scope=$4 AND COALESCE(branch_id,'')=$5 AND is_current`,
			actor.VendorID, profile.ID, key, scopeType, derefOr(branchID)))
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return CustomerIntelligence{}, err
		}
		state, err := s.scopeStateFor(ctx, actor.VendorID, key, scopeType, branchID)
		if err != nil {
			return CustomerIntelligence{}, err
		}
		// A score belonging to a scope whose inputs have moved is reported
		// stale rather than silently presented as current.
		if state.Status == IntelligenceScopeStale && view.State == ScoreCurrent {
			view.AsOfStale = true
		}
		copyOf := view
		switch key {
		case ModelRFM:
			out.RFM = &copyOf
			out.RFMState = view.State
		case ModelChurn:
			out.Churn = &copyOf
			out.ChurnState = view.State
		}
	}
	return out, nil
}

// scopeStateFor reads one pointer row, answering NOT_AVAILABLE when none exists.
func (s *Service) scopeStateFor(ctx context.Context, businessID, modelKey, scopeType string,
	branchID *string) (IntelligenceScopeState, error) {
	state, err := scanScopeState(s.db.QueryRow(ctx, `SELECT `+scopeStateColumns+`
		FROM crm_intelligence_scope_states
		WHERE business_id=$1 AND model_key=$2 AND scope=$3 AND COALESCE(branch_id,'')=$4`,
		businessID, modelKey, scopeType, derefOr(branchID)))
	if errors.Is(err, pgx.ErrNoRows) {
		return IntelligenceScopeState{
			BusinessID: businessID, BranchID: branchID, Scope: scopeType,
			ModelKey: modelKey, Status: IntelligenceScopeNotAvailable,
		}, nil
	}
	return state, err
}

// attachIntelligenceBlock contributes the Section 8H Customer 360 module
// through the same composition point Sections 8B-8D use.
func (s *Service) attachIntelligenceBlock(ctx context.Context, actor Actor, scope Scope,
	profile Profile, view *Customer360) error {
	if !s.flags.IntelligenceEnabled {
		return nil
	}
	scopeType, branchID, ok := readingScope(scope, "")
	if !ok {
		// A multi-branch actor with no single branch to read is told the module
		// is unavailable rather than shown another branch's numbers.
		return nil
	}
	_ = scopeType
	_ = branchID
	intelligence, err := s.CustomerIntelligenceFor(ctx, actor, scope, profile, "")
	if err != nil {
		return err
	}
	view.Intelligence = &intelligence
	view.Modules["intelligence"] = ModuleAvailable
	return nil
}

// ---------------------------------------------------------------------------
// Analytics
// ---------------------------------------------------------------------------

// IntelligenceOverview is the operational answer to "can we act on this".
type IntelligenceOverview struct {
	Scope     string                  `json:"scope"`
	BranchRef *string                 `json:"branch_ref,omitempty"`
	Models    []ModelAvailabilityView `json:"models"`
}

// ModelAvailabilityView reports one model's freshness and coverage.
//
// The counts are the honest part. `unavailable_count` is a first-class number
// rather than an absence, so a business with no order data reads as "1,200
// relationships, 0 scored, 1,200 unavailable" instead of "0 customers".
type ModelAvailabilityView struct {
	ModelKey     string     `json:"model_key"`
	ModelVersion *int       `json:"model_version,omitempty"`
	Label        string     `json:"label"`
	Status       string     `json:"status"`
	Generation   *int64     `json:"generation,omitempty"`
	SourceAsOf   *time.Time `json:"source_as_of,omitempty"`
	ComputedAt   *time.Time `json:"computed_at,omitempty"`
	StaleSince   *time.Time `json:"stale_since,omitempty"`
	ErrorCode    *string    `json:"error_code,omitempty"`

	ProfileCount     int `json:"profile_count"`
	AvailableCount   int `json:"available_count"`
	UnavailableCount int `json:"unavailable_count"`
}

// IntelligenceOverviewFor reports availability and freshness per model.
func (s *Service) IntelligenceOverviewFor(ctx context.Context, actor Actor, scope Scope,
	branchRef string) (IntelligenceOverview, error) {
	if err := s.assertIntelligenceAvailable(); err != nil {
		return IntelligenceOverview{}, err
	}
	scopeType, branchID, ok := readingScope(scope, branchRef)
	if !ok {
		return IntelligenceOverview{}, errIntelligenceScopeDenied()
	}
	out := IntelligenceOverview{Scope: scopeType, BranchRef: branchID}
	for _, model := range IntelligenceModels() {
		state, err := s.scopeStateFor(ctx, actor.VendorID, model.Key, scopeType, branchID)
		if err != nil {
			return IntelligenceOverview{}, err
		}
		out.Models = append(out.Models, ModelAvailabilityView{
			ModelKey: model.Key, ModelVersion: state.ModelVersion, Label: model.Label,
			Status: state.Status, Generation: state.Generation, SourceAsOf: state.SourceAsOf,
			ComputedAt: state.ComputedAt, StaleSince: state.StaleSince, ErrorCode: state.ErrorCode,
			ProfileCount: state.ProfileCount, AvailableCount: state.AvailableCount,
			UnavailableCount: state.UnavailableCount,
		})
	}
	return out, nil
}

// DistributionBucket is one row of a distribution.
type DistributionBucket struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// RFMDistribution is the composite-score histogram plus availability.
type RFMDistribution struct {
	Availability ModelAvailabilityView `json:"availability"`
	Composite    []DistributionBucket  `json:"composite_distribution"`
	Recency      []DistributionBucket  `json:"recency_distribution"`
	Frequency    []DistributionBucket  `json:"frequency_distribution"`
	Monetary     []DistributionBucket  `json:"monetary_distribution"`
	Unavailable  []DistributionBucket  `json:"unavailable_reasons"`
}

// ChurnDistribution is the risk-band histogram plus availability.
type ChurnDistribution struct {
	Availability ModelAvailabilityView `json:"availability"`
	Bands        []DistributionBucket  `json:"band_distribution"`
	Unavailable  []DistributionBucket  `json:"unavailable_reasons"`
}

// RFMDistributionFor computes the RFM histograms at the caller's scope.
func (s *Service) RFMDistributionFor(ctx context.Context, actor Actor, scope Scope,
	branchRef string) (RFMDistribution, error) {
	overview, scopeType, branchID, err := s.modelAvailability(ctx, actor, scope, branchRef, ModelRFM)
	if err != nil {
		return RFMDistribution{}, err
	}
	out := RFMDistribution{Availability: overview}
	for _, dimension := range []struct {
		column string
		target *[]DistributionBucket
	}{
		{"composite_score", &out.Composite},
		{"recency_score", &out.Recency},
		{"frequency_score", &out.Frequency},
		{"monetary_score", &out.Monetary},
	} {
		buckets, err := s.scoreDistribution(ctx, actor.VendorID, ModelRFM, scopeType, branchID,
			dimension.column+"::text")
		if err != nil {
			return RFMDistribution{}, err
		}
		*dimension.target = buckets
	}
	if out.Unavailable, err = s.unavailableReasons(ctx, actor.VendorID, ModelRFM, scopeType, branchID); err != nil {
		return RFMDistribution{}, err
	}
	return out, nil
}

// ChurnDistributionFor computes the churn histograms at the caller's scope.
func (s *Service) ChurnDistributionFor(ctx context.Context, actor Actor, scope Scope,
	branchRef string) (ChurnDistribution, error) {
	overview, scopeType, branchID, err := s.modelAvailability(ctx, actor, scope, branchRef, ModelChurn)
	if err != nil {
		return ChurnDistribution{}, err
	}
	out := ChurnDistribution{Availability: overview}
	if out.Bands, err = s.scoreDistribution(ctx, actor.VendorID, ModelChurn, scopeType, branchID,
		"risk_band"); err != nil {
		return ChurnDistribution{}, err
	}
	if out.Unavailable, err = s.unavailableReasons(ctx, actor.VendorID, ModelChurn, scopeType, branchID); err != nil {
		return ChurnDistribution{}, err
	}
	return out, nil
}

func (s *Service) modelAvailability(ctx context.Context, actor Actor, scope Scope, branchRef,
	modelKey string) (ModelAvailabilityView, string, *string, error) {
	if err := s.assertIntelligenceAvailable(); err != nil {
		return ModelAvailabilityView{}, "", nil, err
	}
	scopeType, branchID, ok := readingScope(scope, branchRef)
	if !ok {
		return ModelAvailabilityView{}, "", nil, errIntelligenceScopeDenied()
	}
	model, _ := ModelFor(modelKey)
	state, err := s.scopeStateFor(ctx, actor.VendorID, modelKey, scopeType, branchID)
	if err != nil {
		return ModelAvailabilityView{}, "", nil, err
	}
	return ModelAvailabilityView{
		ModelKey: modelKey, ModelVersion: state.ModelVersion, Label: model.Label,
		Status: state.Status, Generation: state.Generation, SourceAsOf: state.SourceAsOf,
		ComputedAt: state.ComputedAt, StaleSince: state.StaleSince, ErrorCode: state.ErrorCode,
		ProfileCount: state.ProfileCount, AvailableCount: state.AvailableCount,
		UnavailableCount: state.UnavailableCount,
	}, scopeType, branchID, nil
}

// scoreDistribution counts current scores by one column. The column is a Go
// literal chosen by the caller in this file, never a request value.
func (s *Service) scoreDistribution(ctx context.Context, businessID, modelKey, scopeType string,
	branchID *string, column string) ([]DistributionBucket, error) {
	rows, err := s.db.Query(ctx, `SELECT `+column+` AS bucket, count(*)
		FROM crm_customer_intelligence_scores
		WHERE business_id=$1 AND model_key=$2 AND scope=$3 AND COALESCE(branch_id,'')=$4
		  AND is_current AND state='CURRENT'
		GROUP BY 1 ORDER BY 1`, businessID, modelKey, scopeType, derefOr(branchID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DistributionBucket{}
	for rows.Next() {
		var bucket DistributionBucket
		var key *string
		if err = rows.Scan(&key, &bucket.Count); err != nil {
			return nil, err
		}
		bucket.Key = derefOr(key)
		out = append(out, bucket)
	}
	return out, rows.Err()
}

// unavailableReasons counts why relationships have no usable score. It is what
// turns "0 scored" from a mystery into an actionable statement.
func (s *Service) unavailableReasons(ctx context.Context, businessID, modelKey, scopeType string,
	branchID *string) ([]DistributionBucket, error) {
	rows, err := s.db.Query(ctx, `SELECT reason, count(*) FROM (
			SELECT unnest(reason_codes) AS reason FROM crm_customer_intelligence_scores
			WHERE business_id=$1 AND model_key=$2 AND scope=$3 AND COALESCE(branch_id,'')=$4
			  AND is_current AND state <> 'CURRENT') r
		GROUP BY reason ORDER BY reason`, businessID, modelKey, scopeType, derefOr(branchID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DistributionBucket{}
	for rows.Next() {
		var bucket DistributionBucket
		if err = rows.Scan(&bucket.Key, &bucket.Count); err != nil {
			return nil, err
		}
		out = append(out, bucket)
	}
	return out, rows.Err()
}
