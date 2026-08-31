package crm

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type CustomerValueBandDefinition struct {
	ID                     string `json:"id"`
	BusinessID             string `json:"business_id"`
	BandKey                string `json:"band_key"`
	DisplayName            string `json:"display_name"`
	MinimumNetSpendMinor   int64  `json:"minimum_net_spend_minor"`
	MinimumCompletedOrders int    `json:"minimum_completed_orders"`
	Rank                   int    `json:"rank"`
	Status                 string `json:"status"`
	Version                int64  `json:"version"`
}

type CustomerValueProjection struct {
	State       string    `json:"state"`
	Band        string    `json:"band,omitempty"`
	Rank        *int      `json:"rank,omitempty"`
	ReasonCodes []string  `json:"reason_codes"`
	SourceAsOf  time.Time `json:"source_as_of"`
	Version     int64     `json:"version"`
}

func (s *Service) GetCustomerValueBands(ctx context.Context, businessID string) ([]CustomerValueBandDefinition, error) {
	rows, err := s.db.Query(ctx, `SELECT id,business_id,band_key,display_name,minimum_net_spend_minor,minimum_completed_orders,rank,status,version FROM crm_customer_value_band_definitions WHERE business_id=$1 AND status='ACTIVE' ORDER BY rank`, businessID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CustomerValueBandDefinition{}
	for rows.Next() {
		var d CustomerValueBandDefinition
		if err := rows.Scan(&d.ID, &d.BusinessID, &d.BandKey, &d.DisplayName, &d.MinimumNetSpendMinor, &d.MinimumCompletedOrders, &d.Rank, &d.Status, &d.Version); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Service) ReplaceCustomerValueBands(ctx context.Context, actor Actor, defs []CustomerValueBandDefinition) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, actor.VendorID); err != nil {
		return err
	}
	if len(defs) > 4 {
		return fmt.Errorf("at most four customer value bands are supported")
	}
	if _, err = tx.Exec(ctx, `UPDATE crm_customer_value_band_definitions SET status='ARCHIVED',archived_at=now(),updated_at=now() WHERE business_id=$1 AND status='ACTIVE'`, actor.VendorID); err != nil {
		return err
	}
	for i := range defs {
		if defs[i].BandKey == "" || defs[i].Rank < 1 || defs[i].Rank > 4 || defs[i].MinimumNetSpendMinor < 0 || defs[i].MinimumCompletedOrders < 0 {
			return fmt.Errorf("invalid customer value band")
		}
		defs[i].ID = newID("crmval_")
		if defs[i].Status == "" {
			defs[i].Status = "ACTIVE"
		}
		if _, err = tx.Exec(ctx, `INSERT INTO crm_customer_value_band_definitions (id,business_id,band_key,display_name,minimum_net_spend_minor,minimum_completed_orders,rank,status) VALUES($1,$2,$3,$4,$5,$6,$7,'ACTIVE')`, defs[i].ID, actor.VendorID, defs[i].BandKey, defs[i].DisplayName, defs[i].MinimumNetSpendMinor, defs[i].MinimumCompletedOrders, defs[i].Rank); err != nil {
			return err
		}
	}
	profiles, qerr := tx.Query(ctx, `SELECT id FROM vendor_customer_profiles WHERE business_id=$1 AND privacy_state <> 'REDACTED'`, actor.VendorID)
	if qerr != nil {
		return qerr
	}
	var profileIDs []string
	for profiles.Next() {
		var id string
		if err := profiles.Scan(&id); err != nil {
			profiles.Close()
			return err
		}
		profileIDs = append(profileIDs, id)
	}
	if err := profiles.Err(); err != nil {
		profiles.Close()
		return err
	}
	profiles.Close()
	for _, id := range profileIDs {
		if err := s.rebuildCustomerValue(ctx, tx, actor.VendorID, id, s.now()); err != nil {
			return err
		}
		if err := s.rebuildRelationshipSignals(ctx, tx, actor.VendorID, id, s.now()); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

type RelationshipSignalsView struct {
	State       string    `json:"state"`
	Repeat      *bool     `json:"repeat,omitempty"`
	Lapsed      *bool     `json:"lapsed,omitempty"`
	HighValue   *bool     `json:"high_value,omitempty"`
	AtRisk      *bool     `json:"at_risk,omitempty"`
	BulkCapable *bool     `json:"bulk_capable,omitempty"`
	Corporate   *bool     `json:"corporate,omitempty"`
	Recurring   *bool     `json:"recurring,omitempty"`
	ReasonCodes []string  `json:"reason_codes"`
	SourceAsOf  time.Time `json:"source_as_of"`
	Version     int64     `json:"version"`
}

type RelationshipSignalConfig struct {
	RepeatOrderThreshold int   `json:"repeat_order_threshold"`
	LapsedAfterDays      int   `json:"lapsed_after_days"`
	AtRiskChurnThreshold int   `json:"at_risk_churn_threshold"`
	HighValueMinimumRank int   `json:"high_value_minimum_rank"`
	Version              int64 `json:"version"`
}

func (s *Service) UpsertRelationshipSignalConfig(ctx context.Context, actor Actor, cfg RelationshipSignalConfig) error {
	if cfg.RepeatOrderThreshold == 0 {
		cfg.RepeatOrderThreshold = 2
	}
	if cfg.LapsedAfterDays == 0 {
		cfg.LapsedAfterDays = 90
	}
	if cfg.AtRiskChurnThreshold == 0 {
		cfg.AtRiskChurnThreshold = 70
	}
	if cfg.HighValueMinimumRank == 0 {
		cfg.HighValueMinimumRank = 3
	}
	_, err := s.db.Exec(ctx, `INSERT INTO crm_relationship_signal_configs (business_id,repeat_order_threshold,lapsed_after_days,at_risk_churn_threshold,high_value_minimum_rank,version) VALUES($1,$2,$3,$4,$5,1) ON CONFLICT (business_id) DO UPDATE SET repeat_order_threshold=EXCLUDED.repeat_order_threshold,lapsed_after_days=EXCLUDED.lapsed_after_days,at_risk_churn_threshold=EXCLUDED.at_risk_churn_threshold,high_value_minimum_rank=EXCLUDED.high_value_minimum_rank,updated_at=now(),version=crm_relationship_signal_configs.version+1`, actor.VendorID, cfg.RepeatOrderThreshold, cfg.LapsedAfterDays, cfg.AtRiskChurnThreshold, cfg.HighValueMinimumRank)
	return err
}

func (s *Service) rebuildCustomerValue(ctx context.Context, tx pgx.Tx, businessID, profileID string, asOf time.Time) error {
	var defs []CustomerValueBandDefinition
	rows, err := tx.Query(ctx, `SELECT id,business_id,band_key,display_name,minimum_net_spend_minor,minimum_completed_orders,rank,status,version FROM crm_customer_value_band_definitions WHERE business_id=$1 AND status='ACTIVE' ORDER BY rank`, businessID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var d CustomerValueBandDefinition
		if err := rows.Scan(&d.ID, &d.BusinessID, &d.BandKey, &d.DisplayName, &d.MinimumNetSpendMinor, &d.MinimumCompletedOrders, &d.Rank, &d.Status, &d.Version); err != nil {
			rows.Close()
			return err
		}
		defs = append(defs, d)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	state := "NOT_AVAILABLE"
	band := any(nil)
	reasons := []string{"VALUE_BANDS_NOT_CONFIGURED"}
	var rank any
	if len(defs) > 0 {
		var spend int64
		var count int
		if err := tx.QueryRow(ctx, `SELECT net_spend_minor,completed_order_count FROM vendor_customer_profiles WHERE business_id=$1 AND id=$2`, businessID, profileID).Scan(&spend, &count); err != nil {
			return err
		}
		state = "NO_BAND"
		reasons = []string{"THRESHOLD_NOT_MET"}
		for _, d := range defs {
			if spend >= d.MinimumNetSpendMinor && count >= d.MinimumCompletedOrders {
				state = "CURRENT"
				band = d.BandKey
				rank = d.Rank
				reasons = nil
				break
			}
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO crm_customer_value_projections (business_id,profile_id,scope,model_key,model_version,definition_hash,state,band,reason_codes,source_as_of,version) VALUES($1,$2,'BUSINESS','CUSTOMER_VALUE_V1',1,'CUSTOMER_VALUE_V1', $3,$4,$5,$6,1) ON CONFLICT (business_id,profile_id,scope,model_key) DO UPDATE SET state=EXCLUDED.state,band=EXCLUDED.band,reason_codes=EXCLUDED.reason_codes,source_as_of=EXCLUDED.source_as_of,computed_at=now(),version=crm_customer_value_projections.version+1`, businessID, profileID, state, band, reasons, asOf)
	_ = rank
	return err
}

func (s *Service) RebuildCustomerValue(ctx context.Context, tx pgx.Tx, actor Actor, profileID string, asOf time.Time) error {
	return s.rebuildCustomerValue(ctx, tx, actor.VendorID, profileID, asOf)
}

func (s *Service) ReadCustomerValue(ctx context.Context, businessID, profileID string) (CustomerValueProjection, error) {
	var p CustomerValueProjection
	var band *string
	var rank *int
	var reasons []string
	err := s.db.QueryRow(ctx, `SELECT state,band,reason_codes,source_as_of,version FROM crm_customer_value_projections WHERE business_id=$1 AND profile_id=$2 AND scope='BUSINESS' AND model_key='CUSTOMER_VALUE_V1'`, businessID, profileID).Scan(&p.State, &band, &reasons, &p.SourceAsOf, &p.Version)
	if err == pgx.ErrNoRows {
		return CustomerValueProjection{State: "NOT_AVAILABLE", ReasonCodes: []string{"VALUE_NOT_COMPUTED"}}, nil
	}
	if err != nil {
		return p, err
	}
	if band != nil {
		p.Band = *band
		if err := s.db.QueryRow(ctx, `SELECT rank FROM crm_customer_value_band_definitions WHERE business_id=$1 AND band_key=$2 AND status='ACTIVE'`, businessID, *band).Scan(&rank); err == nil {
			p.Rank = rank
		}
	}
	p.ReasonCodes = reasons
	return p, nil
}

func (s *Service) rebuildRelationshipSignals(ctx context.Context, tx pgx.Tx, businessID, profileID string, asOf time.Time) error {
	var count int
	var last *time.Time
	if err := tx.QueryRow(ctx, `SELECT completed_order_count,last_completed_order_at FROM vendor_customer_profiles WHERE business_id=$1 AND id=$2`, businessID, profileID).Scan(&count, &last); err != nil {
		return err
	}
	var threshold, days, risk, high int
	err := tx.QueryRow(ctx, `SELECT repeat_order_threshold,lapsed_after_days,at_risk_churn_threshold,high_value_minimum_rank FROM crm_relationship_signal_configs WHERE business_id=$1`, businessID).Scan(&threshold, &days, &risk, &high)
	if err == pgx.ErrNoRows {
		threshold, days, risk, high = 2, 90, 70, 3
	} else if err != nil {
		return err
	}
	var valueRank *int
	_ = tx.QueryRow(ctx, `SELECT d.rank FROM crm_customer_value_projections p JOIN crm_customer_value_band_definitions d ON d.business_id=p.business_id AND d.band_key=p.band WHERE p.business_id=$1 AND p.profile_id=$2 AND p.state='CURRENT'`, businessID, profileID).Scan(&valueRank)
	var repeat, lapsed, highValue, atRisk, bulkCapable, corporate, recurring *bool
	if last != nil {
		v := count >= threshold
		repeat = &v
		v2 := asOf.Sub(*last) >= time.Duration(days)*24*time.Hour
		lapsed = &v2
	}
	if valueRank != nil {
		v := *valueRank >= high
		highValue = &v
	}
	var bulkCount, recurringCount, corporateCount int
	_ = tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE order_type='BULK'), count(*) FILTER (WHERE order_type='RECURRING'), count(*) FILTER (WHERE corporate_customer_id IS NOT NULL) FROM crm_customer_order_facts WHERE business_id=$1 AND profile_id=$2`, businessID, profileID).Scan(&bulkCount, &recurringCount, &corporateCount)
	if bulkCount > 0 {
		v := true
		bulkCapable = &v
	}
	if recurringCount > 0 {
		v := true
		recurring = &v
	}
	if corporateCount > 0 {
		v := true
		corporate = &v
	}
	var riskScore *int
	_ = tx.QueryRow(ctx, `SELECT risk_score FROM crm_customer_intelligence_scores WHERE business_id=$1 AND customer_profile_id=$2 AND scope='BUSINESS' AND model_key='CHURN_V1' AND state='CURRENT' AND is_current=true ORDER BY computed_at DESC LIMIT 1`, businessID, profileID).Scan(&riskScore)
	if riskScore != nil {
		v := *riskScore >= risk
		atRisk = &v
	}
	state := "CURRENT"
	reasons := []string{}
	if last == nil {
		state = "NOT_AVAILABLE"
		reasons = []string{"COMMERCIAL_NOT_AVAILABLE"}
		repeat = nil
		lapsed = nil
	}
	hash := fmt.Sprintf("RELATIONSHIP_SIGNALS_V1:%d:%d:%d:%d", threshold, days, risk, high)
	_, err = tx.Exec(ctx, `INSERT INTO crm_relationship_signals (business_id,profile_id,scope,model_version,definition_hash,state,repeat_signal,lapsed_signal,high_value_signal,at_risk_signal,bulk_capable_signal,corporate_signal,recurring_signal,reason_codes,source_as_of,last_completed_order_at,completed_order_count,value_band_rank,churn_risk_score,version) VALUES($1,$2,'BUSINESS',1,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,1) ON CONFLICT (business_id,profile_id,scope) DO UPDATE SET definition_hash=EXCLUDED.definition_hash,state=EXCLUDED.state,repeat_signal=EXCLUDED.repeat_signal,lapsed_signal=EXCLUDED.lapsed_signal,high_value_signal=EXCLUDED.high_value_signal,at_risk_signal=EXCLUDED.at_risk_signal,bulk_capable_signal=EXCLUDED.bulk_capable_signal,corporate_signal=EXCLUDED.corporate_signal,recurring_signal=EXCLUDED.recurring_signal,reason_codes=EXCLUDED.reason_codes,source_as_of=EXCLUDED.source_as_of,last_completed_order_at=EXCLUDED.last_completed_order_at,completed_order_count=EXCLUDED.completed_order_count,value_band_rank=EXCLUDED.value_band_rank,churn_risk_score=EXCLUDED.churn_risk_score,computed_at=now(),version=crm_relationship_signals.version+1`, businessID, profileID, hash, state, repeat, lapsed, highValue, atRisk, bulkCapable, corporate, recurring, reasons, asOf, last, count, valueRank, riskScore)
	return err
}

func (s *Service) ReadRelationshipSignals(ctx context.Context, businessID, profileID string) (RelationshipSignalsView, error) {
	var v RelationshipSignalsView
	err := s.db.QueryRow(ctx, `SELECT state,repeat_signal,lapsed_signal,high_value_signal,at_risk_signal,bulk_capable_signal,corporate_signal,recurring_signal,reason_codes,source_as_of,version FROM crm_relationship_signals WHERE business_id=$1 AND profile_id=$2 AND scope='BUSINESS'`, businessID, profileID).Scan(&v.State, &v.Repeat, &v.Lapsed, &v.HighValue, &v.AtRisk, &v.BulkCapable, &v.Corporate, &v.Recurring, &v.ReasonCodes, &v.SourceAsOf, &v.Version)
	if err == pgx.ErrNoRows {
		return RelationshipSignalsView{State: "NOT_AVAILABLE", ReasonCodes: []string{"SIGNALS_NOT_COMPUTED"}}, nil
	}
	return v, err
}
