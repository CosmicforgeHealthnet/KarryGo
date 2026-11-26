// Package models holds the Section 24/54 analytics domain: report
// definitions and runs, dashboards, explainable recommendations and
// human-governed automation rules.
package models

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Flags gate the analytics capabilities; all default off.
type Flags struct {
	Reports, Recommendations, Automation, Workers, KillSwitch bool
}

// ReportTypes are the supported report and dashboard kinds.
var ReportTypes = map[string]bool{"EXECUTIVE": true, "OPERATIONS": true, "FLEET": true, "WAREHOUSE": true, "WORKFORCE": true, "FINANCE": true, "SLA": true, "CUSTOMER": true}

// ReportDefinitionInput defines (and optionally schedules) a report.
type ReportDefinitionInput struct {
	Name             string          `json:"name"`
	ReportType       string          `json:"reportType"`
	Filters          json.RawMessage `json:"filters"`
	ScheduleCron     string          `json:"scheduleCron"`
	ScheduleTimezone string          `json:"scheduleTimezone"`
}

// ReportFilters are the allowlisted report filters.
type ReportFilters struct {
	BranchID string `json:"branchId,omitempty"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
}

// ParseReportFilters strictly decodes report filters; unknown keys are
// refused rather than ignored (spec 6: reject unknown filters).
func ParseReportFilters(raw json.RawMessage) (ReportFilters, error) {
	var f ReportFilters
	if len(raw) == 0 || string(raw) == "null" {
		return f, nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return f, fmt.Errorf("unsupported report filter: %w", err)
	}
	for _, d := range []string{f.From, f.To} {
		if d == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", d); err != nil {
			return f, fmt.Errorf("report dates must be YYYY-MM-DD")
		}
	}
	return f, nil
}

// PermissionSnapshot is what the requester could see when the run was
// requested; generation honours it even if the requester later loses access
// (EXP-001).
type PermissionSnapshot struct {
	ActorID            string   `json:"actorId"`
	PermittedBranchIDs []string `json:"permittedBranchIds"`
	CapturedAt         string   `json:"capturedAt"`
}

// ReportSection is one titled table of a report.
type ReportSection struct {
	Title string           `json:"title"`
	Rows  []map[string]any `json:"rows"`
}

// ReportResult is a generated report with its freshness.
type ReportResult struct {
	ReportType  string          `json:"reportType"`
	GeneratedAt time.Time       `json:"generatedAt"`
	Filters     ReportFilters   `json:"filters"`
	Sections    []ReportSection `json:"sections"`
	// DataSources names the authoritative tables each section was read
	// from, so a figure can always be traced to its source.
	DataSources []string `json:"dataSources"`
}

// RowCount is the number of rows across sections.
func (r ReportResult) RowCount() int {
	n := 0
	for _, s := range r.Sections {
		n += len(s.Rows)
	}
	return n
}

// RecommendationTypes are the explainable recommendation kinds.
var RecommendationTypes = map[string]bool{"PREVENTIVE_MAINTENANCE": true, "ANOMALY_REVIEW": true, "DELAY_RISK": true}

// Recommendation is one model output awaiting a human decision.
type Recommendation struct {
	Type, SubjectType, SubjectID string
	ModelCode, ModelVersion      string
	Confidence                   float64
	Explanation                  map[string]any
	InputReference               map[string]any
	ActionDraft                  map[string]any
}

// RecommendationDecision is a human decision on a recommendation.
type RecommendationDecision struct {
	Decision        string `json:"decision"`
	Reason          string `json:"reason"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

// GenerateInput selects which models to run.
type GenerateInput struct {
	Types []string `json:"types"`
}

// AutomationRuleInput drafts an automation rule.
type AutomationRuleInput struct {
	Name            string          `json:"name"`
	TriggerType     string          `json:"triggerType"`
	TriggerConfig   json.RawMessage `json:"triggerConfig"`
	ConditionTree   json.RawMessage `json:"conditionTree"`
	ActionPlan      json.RawMessage `json:"actionPlan"`
	ExpectedVersion int64           `json:"expectedVersion"`
	Reason          string          `json:"reason"`
}

// TriggerConfig is an EVENT trigger.
type TriggerConfig struct {
	EventType string `json:"eventType"`
}

// Condition is one predicate over the triggering event payload.
type Condition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

// ConditionTree is a conjunction of predicates (empty matches every event).
type ConditionTree struct {
	All []Condition `json:"all"`
}

// Action is one step of an action plan.
type Action struct {
	Type   string         `json:"type"`
	Params map[string]any `json:"params"`
}

// ActionPlan is the ordered list of actions a rule runs.
type ActionPlan struct {
	Actions []Action `json:"actions"`
}

// AutomationTriggerEvents is the approved trigger catalogue: operational
// facts an organization may automate on.
var AutomationTriggerEvents = map[string]bool{
	"exception.opened.v1": true, "exception.escalated.v1": true,
	"warehouse_receipt.exception.v1": true, "pick_task.short.v1": true,
	"invoice.issued.v1": true, "corporate.shipment.status_changed.v1": true,
	"organization.assignment.declined.v1": true, "organization.assignment.expired.v1": true,
	"asset.blocked.v1": true,
}

// AutomationActions is the approved action catalogue. It deliberately
// contains no protected action: nothing moves money, suspends or disciplines
// a provider, changes a live assignment, or lifts a safety or compliance
// restriction (spec 24, 54.2). An action outside this catalogue is refused
// when the rule is drafted and again when it executes.
var AutomationActions = map[string]bool{
	"RAISE_EXCEPTION":    true,
	"QUEUE_NOTIFICATION": true,
}

// ConditionOps are the supported predicate operators.
var ConditionOps = map[string]bool{"eq": true, "neq": true, "in": true, "gte": true, "lte": true, "exists": true}

// ValidateRule decodes and validates a rule definition strictly.
func ValidateRule(in AutomationRuleInput) (TriggerConfig, ConditionTree, ActionPlan, error) {
	var tc TriggerConfig
	var ct ConditionTree
	var ap ActionPlan
	if strings.ToUpper(in.TriggerType) != "EVENT" {
		return tc, ct, ap, fmt.Errorf("triggerType must be EVENT")
	}
	if err := strict(in.TriggerConfig, &tc); err != nil || !AutomationTriggerEvents[tc.EventType] {
		return tc, ct, ap, fmt.Errorf("trigger event is not in the approved catalogue")
	}
	if len(in.ConditionTree) > 0 {
		if err := strict(in.ConditionTree, &ct); err != nil {
			return tc, ct, ap, fmt.Errorf("conditionTree is invalid: %v", err)
		}
	}
	if len(ct.All) > 20 {
		return tc, ct, ap, fmt.Errorf("at most 20 conditions are allowed")
	}
	for _, c := range ct.All {
		if !ConditionOps[c.Op] || !strings.HasPrefix(c.Field, "payload.") || len(c.Field) > 120 {
			return tc, ct, ap, fmt.Errorf("unsupported condition %q %q", c.Field, c.Op)
		}
	}
	if err := strict(in.ActionPlan, &ap); err != nil {
		return tc, ct, ap, fmt.Errorf("actionPlan is invalid: %v", err)
	}
	if len(ap.Actions) == 0 || len(ap.Actions) > 5 {
		return tc, ct, ap, fmt.Errorf("an action plan has between 1 and 5 actions")
	}
	for _, a := range ap.Actions {
		if !AutomationActions[a.Type] {
			return tc, ct, ap, fmt.Errorf("action %q is not in the approved action catalogue", a.Type)
		}
		if err := ValidateActionParams(a); err != nil {
			return tc, ct, ap, err
		}
	}
	return tc, ct, ap, nil
}

// ValidateActionParams checks one action's parameters.
func ValidateActionParams(a Action) error {
	str := func(k string) string { v, _ := a.Params[k].(string); return v }
	switch a.Type {
	case "RAISE_EXCEPTION":
		if str("exceptionType") == "" || str("severity") == "" || str("summary") == "" {
			return fmt.Errorf("RAISE_EXCEPTION needs exceptionType, severity and summary")
		}
	case "QUEUE_NOTIFICATION":
		if str("recipientRole") == "" || str("templateCode") == "" {
			return fmt.Errorf("QUEUE_NOTIFICATION needs recipientRole and templateCode")
		}
	}
	return nil
}

func strict(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return fmt.Errorf("required")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// Evaluate applies a condition tree to an event payload and returns the
// per-condition trace, so every execution is explainable.
func Evaluate(tree ConditionTree, payload map[string]any) (bool, []map[string]any) {
	trace := []map[string]any{}
	ok := true
	for _, c := range tree.All {
		got, present := lookup(payload, strings.TrimPrefix(c.Field, "payload."))
		match := compare(c.Op, got, present, c.Value)
		trace = append(trace, map[string]any{"field": c.Field, "op": c.Op, "expected": c.Value, "actual": got, "matched": match})
		ok = ok && match
	}
	return ok, trace
}

func lookup(m map[string]any, path string) (any, bool) {
	var cur any = m
	for _, part := range strings.Split(path, ".") {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = mm[part]; !ok {
			return nil, false
		}
	}
	return cur, true
}

func compare(op string, got any, present bool, want any) bool {
	switch op {
	case "exists":
		return present
	case "eq":
		return present && fmt.Sprint(got) == fmt.Sprint(want)
	case "neq":
		return !present || fmt.Sprint(got) != fmt.Sprint(want)
	case "in":
		list, _ := want.([]any)
		for _, w := range list {
			if present && fmt.Sprint(got) == fmt.Sprint(w) {
				return true
			}
		}
		return false
	case "gte", "lte":
		g, gok := got.(float64)
		w, wok := want.(float64)
		if !present || !gok || !wok {
			return false
		}
		if op == "gte" {
			return g >= w
		}
		return g <= w
	}
	return false
}

// SortedKeys returns map keys in order (stable explanations and hashes).
func SortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Raw returns v, or the default document d when v is empty.
func Raw(v json.RawMessage, d string) json.RawMessage {
	if len(v) == 0 {
		return json.RawMessage(d)
	}
	return v
}
