package adminprivacy

// Complete-coverage reduction.
//
// A report snapshot is COMPLETE only when, for every fact family the report
// declares (analyticsadmin.Report.CoverageFamilies), the owner's signed
// manifests exactly tile the requested window for the requested scope and every
// attested member is a retained PROCESSED native fact. Scope membership comes
// from the owner's scoped manifest itself, or — for a global manifest — from
// the existing per-fact signed allocations, every one of which must exist.
// Anything short of that returns ErrCoverage and the caller falls back to the
// bounded PARTIAL_COVERAGE path (or denies, for reports with no partial form).
// Aggregate supply snapshots and demand buckets can never enter this path.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/big"
	"sort"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"github.com/jackc/pgx/v5"
)

func ownerForAuthority(authority string) string {
	switch authority {
	case networkanalytics.AuthorityTaxi:
		return "driver-taxi-service"
	case networkanalytics.AuthorityDispatch:
		return "driver-dispatch-delivery-service"
	case networkanalytics.AuthorityHauling:
		return "driver-hauling-service"
	case networkanalytics.AuthorityVendor:
		return "vendor-service"
	}
	return ""
}

// coveredSet is the verified complete membership of one family in one window.
type coveredSet struct {
	Proof    coverageProof
	Facts    []LineageFact
	Payloads map[string][]byte
	Kinds    map[string]networkanalytics.Kind
}

// unitField resolves the native field that attributes a fact to its unit. For
// provider-quality the provider plays different roles per fact kind; every
// role field resolves to the same canonical provider subject hash.
func unitFields(report, field string, kind networkanalytics.Kind) []string {
	if lineageReportFor(report) == "provider-quality" && field == "provider_id" {
		switch kind {
		case networkanalytics.KindRecoveryStarted:
			return []string{"original_provider_id", "provider_id"}
		case networkanalytics.KindRecoveryReplaced:
			return []string{"replacement_provider_id", "provider_id"}
		}
	}
	return []string{field}
}

// collectCovered verifies one family's coverage and reads every relevant
// member as a native unit fact. A relevant member that cannot be attributed to
// the approved unit makes the set incomplete; it is never silently dropped.
func collectCovered(ctx context.Context, tx pgx.Tx, authority admincontext.Authority, report, field, family string, q Query, start, end, now time.Time) (coveredSet, error) {
	out := coveredSet{Payloads: map[string][]byte{}, Kinds: map[string]networkanalytics.Kind{}}
	proof, err := VerifyCoverage(ctx, tx, authority, family, q.Scope, start, end, now)
	global := false
	if errors.Is(err, ErrCoverage) {
		globalFamily := analyticsadmin.FamilyNetworkEventsGlobal
		if family == analyticsadmin.FamilySupplyRoster {
			globalFamily = analyticsadmin.FamilySupplyRosterGlobal
		}
		proof, err = VerifyCoverage(ctx, tx, authority, globalFamily, q.Scope, start, end, now)
		global = true
	}
	if err != nil {
		return out, err
	}
	out.Proof = proof
	reader := LineageReader{Owner: authority.Owner}
	store := admincontext.Store{Reader: reader, Authority: authority}
	permitted := map[networkanalytics.Kind]bool{}
	for _, k := range lineageReports[report].kinds {
		permitted[k] = true
	}
	for _, member := range proof.Facts {
		var name, authorityHint string
		var payload []byte
		if err := tx.QueryRow(ctx, `SELECT event_name,authority,payload FROM analytics_network_events WHERE event_id=$1 AND status='PROCESSED'`, member.EventID).Scan(&name, &authorityHint, &payload); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return out, ErrCoverage
			}
			return out, err
		}
		_, kind, ok := networkanalytics.Classify(name, authorityHint)
		if !ok || !permitted[kind] || kind == networkanalytics.KindSupplySnapshot || kind == networkanalytics.KindDemandBucket {
			continue
		}
		var fact LineageFact
		err := ErrUnmapped
		for _, candidate := range unitFields(report, field, kind) {
			if fact, err = reader.ReadUnit(ctx, tx, report, candidate, member.EventID); err == nil {
				break
			}
		}
		if err != nil {
			if errors.Is(err, ErrUnmapped) {
				return out, ErrCoverage
			}
			return out, err
		}
		if fact.Authority != q.Filters["authority"] || fact.ProductCode != q.Filters["product_code"] {
			continue
		}
		if global {
			in, err := allocatedTo(ctx, tx, store, report, field, member.EventID, q.Scope, now)
			if err != nil {
				return out, err
			}
			if !in {
				continue
			}
		}
		out.Facts = append(out.Facts, fact)
		out.Payloads[fact.EventID] = payload
		out.Kinds[fact.EventID] = kind
	}
	return out, nil
}

// allocatedTo is the global-manifest membership rule: a fact belongs to the
// scope only through a current signed per-fact allocation; a fact with no
// valid allocation anywhere leaves scope membership unproven.
func allocatedTo(ctx context.Context, tx pgx.Tx, store admincontext.Store, report, field, ref string, scope adminpolicy.Scope, now time.Time) (bool, error) {
	kind, err := LineageKind(report, field)
	if err != nil {
		return false, ErrCoverage
	}
	capability := "analytics.cohorts." + report + ".export"
	if _, err := store.Resolve(ctx, tx, kind, ref, capability, scope, now); err == nil {
		return true, nil
	} else if !errors.Is(err, admincontext.ErrUnmapped) {
		return false, err
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT product,market,zone,organization_ref,tenant_ref FROM admin_owner_context_heads WHERE owner=$1 AND resource_kind=$2 AND resource_ref=$3 AND $4=ANY(capabilities) LIMIT 33`, store.Authority.Owner, kind, ref, capability)
	if err != nil {
		return false, err
	}
	var scopes []adminpolicy.Scope
	for rows.Next() {
		var s adminpolicy.Scope
		if err := rows.Scan(&s.Product, &s.Market, &s.Zone, &s.Organization, &s.Tenant); err != nil {
			rows.Close()
			return false, err
		}
		scopes = append(scopes, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	for _, s := range scopes {
		if s == scope {
			continue
		}
		if _, err := store.Resolve(ctx, tx, kind, ref, capability, s, now); err == nil {
			return false, nil
		}
	}
	return false, ErrCoverage
}

// completeAuthorization is the frozen witness of a COMPLETE release: the
// source version fingerprints every manifest revision and member fact used.
type completeAuthorization struct {
	Version       int               `json:"version"`
	Mode          string            `json:"mode"`
	Owner         string            `json:"owner"`
	Metric        string            `json:"metric"`
	Field         string            `json:"field"`
	SourceVersion string            `json:"source_version"`
	Scope         adminpolicy.Scope `json:"scope"`
	Start         time.Time         `json:"start"`
	End           time.Time         `json:"end"`
}

func isCompleteWitness(raw json.RawMessage) bool {
	var probe struct {
		Mode string `json:"mode"`
	}
	return json.Unmarshal(raw, &probe) == nil && probe.Mode == "COMPLETE"
}

// CompleteReducer computes cells for one report from complete covered sets.
type completeInput struct {
	Report, Field string
	Query         Query
	Sets          map[string]coveredSet // by family
	History       []coveredSet          // demand-forecast only
}

// completeSnapshot returns a COMPLETE snapshot or ErrCoverage. It never
// returns a partial snapshot.
func completeSnapshot(ctx context.Context, tx pgx.Tx, authorities map[string]admincontext.Authority, reportName, field string, q Query) (Snapshot, error) {
	var empty Snapshot
	report, ok := analyticsadmin.ReportNamed(reportName)
	if !ok || q.Metric != reportName || q.Capability != report.Permission || len(q.Filters) != 2 {
		return empty, ErrUnmapped
	}
	owner := ownerForAuthority(q.Filters["authority"])
	if reportName == "readiness" && owner != "vendor-service" || reportName != "readiness" && owner == "vendor-service" {
		return empty, ErrUnmapped
	}
	authority, ok := authorities[owner]
	if !ok || authority.Owner != owner {
		return empty, ErrUnmapped
	}
	if _, err := LineageKind(reportName, field); err != nil {
		return empty, err
	}
	now := time.Now().UTC()
	in := completeInput{Report: reportName, Field: field, Query: q, Sets: map[string]coveredSet{}}
	versions := []setVersion{}
	asOf := time.Time{}
	add := func(set coveredSet) {
		hashes := []string{}
		for _, fact := range set.Facts {
			hashes = append(hashes, admincontext.Fingerprint(fact))
			if fact.RecordedAt.After(asOf) {
				asOf = fact.RecordedAt
			}
		}
		sort.Strings(hashes)
		versions = append(versions, setVersion{set.Proof.Family, set.Proof.Manifests, hashes})
		if set.Proof.AsOf.After(asOf) {
			asOf = set.Proof.AsOf
		}
	}
	if reportName == "demand-forecast" {
		for week := 1; week <= networkanalytics.DemandForecastWeeks; week++ {
			shift := time.Duration(week) * 7 * 24 * time.Hour
			set, err := collectCovered(ctx, tx, authority, reportName, field, analyticsadmin.FamilyNetworkEvents, q, q.Start.Add(-shift), q.End.Add(-shift), now)
			if err != nil {
				return empty, err
			}
			in.History = append(in.History, set)
			add(set)
		}
	} else {
		for _, family := range report.CoverageFamilies() {
			lineageReport := lineageReportFor(reportName)
			unit := field
			if family == analyticsadmin.FamilySupplyRoster {
				// Roster membership is read under the supply contract; it is
				// the verified population the quality score may apply to.
				lineageReport, unit = "supply", "provider_id"
			}
			set, err := collectCovered(ctx, tx, authority, lineageReport, unit, family, q, q.Start, q.End, now)
			if err != nil {
				return empty, err
			}
			in.Sets[family] = set
			add(set)
		}
	}
	cells, err := reduceComplete(in)
	if err != nil {
		return empty, err
	}
	if asOf.IsZero() {
		return empty, ErrCoverage
	}
	sourceVersion := admincontext.Fingerprint(struct {
		Contract string
		Owner    string
		Sets     []setVersion
	}{report.Contract + ":" + field, owner, versions})
	reference := ""
	for _, v := range versions {
		if len(v.Manifests) > 0 && reference == "" {
			reference = v.Manifests[0].ContextID
		}
	}
	out := Snapshot{Facts: adminpolicy.Facts{Owner: "analytics-service", Reference: reference, SourceContract: report.Contract + ":" + field, Scope: q.Scope}, SourceVersion: sourceVersion, AsOf: asOf, Partial: false, Cells: cells}
	out.Authorization, err = json.Marshal(completeAuthorization{Version: 2, Mode: "COMPLETE", Owner: owner, Metric: reportName, Field: field, SourceVersion: sourceVersion, Scope: q.Scope, Start: q.Start, End: q.End})
	if err != nil {
		return empty, err
	}
	return out, nil
}

// authorizeComplete re-verifies a frozen COMPLETE release: the same manifest
// revisions and the same member facts must still verify under current owner
// signatures. Any revocation, superseding revision or changed fact denies the
// old release rather than resampling it.
func authorizeComplete(ctx context.Context, tx pgx.Tx, authorities map[string]admincontext.Authority, field string, q Query, version string, raw json.RawMessage) error {
	var witness completeAuthorization
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&witness) != nil {
		return ErrUnmapped
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrUnmapped
	}
	if witness.Version != 2 || witness.Mode != "COMPLETE" || witness.Metric != q.Metric || witness.Field != field || witness.SourceVersion != version || witness.Scope != q.Scope || !witness.Start.Equal(q.Start) || !witness.End.Equal(q.End) || witness.Owner != ownerForAuthority(q.Filters["authority"]) {
		return ErrUnmapped
	}
	snapshot, err := completeSnapshot(ctx, tx, authorities, q.Metric, field, q)
	if err != nil || snapshot.SourceVersion != version {
		return ErrUnmapped
	}
	return nil
}

type setVersion struct {
	Family    string
	Manifests []manifestRef
	Facts     []string
}

func number64(v int64) *int64 { return &v }

// reduceComplete computes the report's cells. Unit cohorts are distinct
// canonical subject hashes; values are exact integers.
func reduceComplete(in completeInput) ([]Cell, error) {
	report, _ := analyticsadmin.ReportNamed(in.Report)
	counts := map[string]int64{}
	units := map[string]map[string]bool{}
	unknown := map[string]bool{}
	for _, key := range report.Keys {
		units[key] = map[string]bool{}
	}
	addCount := func(key, subject string, value int64) error {
		if _, ok := units[key]; !ok || value < 0 || value > math.MaxInt64-counts[key] {
			return ErrSource
		}
		counts[key] += value
		if value > 0 {
			units[key][subject] = true
		}
		return nil
	}
	events := in.Sets[analyticsadmin.FamilyNetworkEvents]
	switch in.Report {
	case "mission-success":
		for _, fact := range events.Facts {
			key := map[networkanalytics.Kind]string{networkanalytics.KindMissionCompleted: "completed", networkanalytics.KindMissionFailed: "failed", networkanalytics.KindMissionCancelled: "cancelled", networkanalytics.KindGuaranteeBreached: "guarantee_breaches"}[events.Kinds[fact.EventID]]
			if key == "" {
				return nil, ErrSource
			}
			if err := addCount(key, fact.SubjectHash, 1); err != nil {
				return nil, err
			}
		}
	case "mission-outcomes", "recovery", "capability-mismatches", "declines", "demand", "provider-quality":
		for _, fact := range events.Facts {
			values, err := reportComponents(in.Report, fact, events.Payloads[fact.EventID])
			if err != nil {
				return nil, err
			}
			for key, value := range values {
				if err := addCount(key, fact.SubjectHash, value); err != nil {
					return nil, err
				}
			}
		}
		if in.Report == "demand" && !unmatchedKnown(in.Query.Filters["authority"]) {
			unknown["unmatched"] = true
		}
	case "recovery-resolution":
		if err := recoveryDurations(events, counts, units); err != nil {
			return nil, err
		}
	case "provider-quality-score":
		if in.Field != "provider_id" {
			// The score is per provider by definition; another unit has none.
			for _, key := range report.Keys {
				unknown[key] = true
			}
		} else if err := qualityScores(events, in.Sets[analyticsadmin.FamilySupplyRoster], counts, units); err != nil {
			return nil, err
		}
	case "readiness":
		for _, fact := range events.Facts {
			values, err := readinessComponents(events.Payloads[fact.EventID])
			if err != nil {
				return nil, err
			}
			for _, key := range readinessComponentKeys {
				value := values[key]
				if value > math.MaxInt64-counts[key] {
					return nil, ErrSource
				}
				counts[key] += value
				if key == "ready_count" || key == "on_time_count" && value > 0 || key == "late_count" && value > 0 || key == "lateness_seconds_total" && values["late_count"] > 0 || (key == "prep_count" || key == "prep_seconds_total") && values["prep_count"] > 0 {
					units[key][fact.SubjectHash] = true
				}
			}
		}
	case "supply":
		roster := in.Sets[analyticsadmin.FamilySupplyRoster]
		latest := map[string]LineageFact{}
		for _, fact := range roster.Facts {
			if previous, ok := latest[fact.SubjectHash]; !ok || fact.OccurredAt.After(previous.OccurredAt) || fact.OccurredAt.Equal(previous.OccurredAt) && fact.EventID > previous.EventID {
				latest[fact.SubjectHash] = fact
			}
		}
		for subject, fact := range latest {
			eligible, occupied, err := rosterState(roster.Payloads[fact.EventID])
			if err != nil {
				return nil, err
			}
			if occupied == nil {
				unknown["available_providers"], unknown["busy_providers"] = true, true
				continue
			}
			if *occupied {
				_ = addCount("busy_providers", subject, 1)
			} else if eligible == nil {
				unknown["available_providers"] = true
			} else if *eligible {
				_ = addCount("available_providers", subject, 1)
			}
		}
	case "demand-forecast":
		history := []int64{}
		subjects := map[string]bool{}
		for _, set := range in.History {
			var slot int64
			for _, fact := range set.Facts {
				values, err := reportComponents("demand-forecast", fact, set.Payloads[fact.EventID])
				if err != nil {
					return nil, err
				}
				if values["requests"] > 0 {
					slot += values["requests"]
					subjects[fact.SubjectHash] = true
				}
			}
			history = append(history, slot)
		}
		expected, trusted := networkanalytics.SameSlotMean(history)
		if !trusted || len(history) != networkanalytics.DemandForecastWeeks {
			return nil, ErrCoverage
		}
		counts["forecast_requests"] = int64(math.Floor(expected))
		units["forecast_requests"] = subjects
	default:
		return nil, ErrUnmapped
	}
	cells := []Cell{}
	for _, key := range report.Keys {
		if unknown[key] {
			cells = append(cells, Cell{Key: key})
			continue
		}
		cells = append(cells, Cell{Key: key, Count: number64(counts[key]), CohortSize: number64(int64(len(units[key])))})
	}
	return cells, nil
}

func caseKey(fact LineageFact, payload []byte) string {
	var p map[string]any
	_ = json.Unmarshal(payload, &p)
	for _, key := range []string{"recovery_case_id", "recovery_case_ref", "incident_id"} {
		if v, ok := p[key].(string); ok && v != "" {
			return v
		}
	}
	return fact.EntityType + ":" + fact.EntityID
}

// recoveryDurations times RESOLVED recovery cases whose detection and
// resolution are both owner-attested facts inside the covered window.
// Unresolved and failed cases are excluded; a resolution before its start is
// a contradictory source and fails closed.
func recoveryDurations(events coveredSet, counts map[string]int64, units map[string]map[string]bool) error {
	started := map[string]time.Time{}
	for _, fact := range events.Facts {
		if events.Kinds[fact.EventID] == networkanalytics.KindRecoveryStarted {
			key := caseKey(fact, events.Payloads[fact.EventID])
			if at, ok := started[key]; !ok || fact.OccurredAt.Before(at) {
				started[key] = fact.OccurredAt
			}
		}
	}
	durations := []int64{}
	seen := map[string]bool{}
	for _, fact := range events.Facts {
		if events.Kinds[fact.EventID] != networkanalytics.KindRecoveryResolved {
			continue
		}
		key := caseKey(fact, events.Payloads[fact.EventID])
		start, ok := started[key]
		if !ok || seen[key] {
			continue
		}
		if fact.OccurredAt.Before(start) {
			return ErrSource
		}
		seen[key] = true
		durations = append(durations, int64(fact.OccurredAt.Sub(start)/time.Second))
		for _, k := range []string{"resolved_with_duration", "resolution_seconds_mean", "resolution_seconds_median"} {
			units[k][fact.SubjectHash] = true
		}
	}
	counts["resolved_with_duration"] = int64(len(durations))
	counts["resolution_seconds_mean"], counts["resolution_seconds_median"] = meanMedian(durations)
	return nil
}

// meanMedian is floor(mean) and floor of the median (lower-middle average for
// an even count). Empty input yields zeros; its cohort is empty and therefore
// suppressed as a small cell by the disclosure rules.
func meanMedian(values []int64) (int64, int64) {
	if len(values) == 0 {
		return 0, 0
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	total := new(big.Int)
	for _, v := range values {
		total.Add(total, big.NewInt(v))
	}
	mean := new(big.Int).Quo(total, big.NewInt(int64(len(values)))).Int64()
	mid := len(values) / 2
	median := values[mid]
	if len(values)%2 == 0 {
		median = new(big.Int).Quo(new(big.Int).Add(big.NewInt(values[mid-1]), big.NewInt(values[mid])), big.NewInt(2)).Int64()
	}
	return mean, median
}

// qualityScores applies analyticsadmin.ProviderQualityScoreContract (v1) to
// verified roster members only. Exact integer arithmetic, floor to basis
// points; providers below the minimum sample are not scored.
func qualityScores(events, roster coveredSet, counts map[string]int64, units map[string]map[string]bool) error {
	members := map[string]bool{}
	for _, fact := range roster.Facts {
		members[fact.SubjectHash] = true
	}
	type tally struct{ completed, failed, recoveries int64 }
	tallies := map[string]*tally{}
	for _, fact := range events.Facts {
		t := tallies[fact.SubjectHash]
		if t == nil {
			t = &tally{}
			tallies[fact.SubjectHash] = t
		}
		switch events.Kinds[fact.EventID] {
		case networkanalytics.KindMissionCompleted:
			t.completed++
		case networkanalytics.KindMissionFailed:
			t.failed++
		case networkanalytics.KindRecoveryStarted:
			t.recoveries++
		}
	}
	scores := []int64{}
	for subject, t := range tallies {
		held := t.completed + t.failed + t.recoveries
		if !members[subject] || held < analyticsadmin.ProviderQualityMinimumSample {
			continue
		}
		score := int64(0)
		if t.completed+t.failed > 0 {
			numerator := new(big.Int).Mul(big.NewInt(10000*t.completed), big.NewInt(held-t.recoveries))
			denominator := new(big.Int).Mul(big.NewInt(t.completed+t.failed), big.NewInt(held))
			score = new(big.Int).Quo(numerator, denominator).Int64()
		}
		scores = append(scores, score)
		for _, key := range []string{"quality_scored_providers", "quality_score_v1_mean_bp", "quality_score_v1_median_bp"} {
			units[key][subject] = true
		}
	}
	counts["quality_scored_providers"] = int64(len(scores))
	counts["quality_score_v1_mean_bp"], counts["quality_score_v1_median_bp"] = meanMedian(scores)
	return nil
}

// rosterState reads the owner's eligibility/occupancy determination. JSON null
// is UNKNOWN; an absent or non-boolean field is a malformed source.
func rosterState(raw []byte) (*bool, *bool, error) {
	var p map[string]json.RawMessage
	if json.Unmarshal(raw, &p) != nil {
		return nil, nil, ErrSource
	}
	read := func(key string) (*bool, error) {
		value, ok := p[key]
		if !ok {
			return nil, ErrSource
		}
		if string(value) == "null" {
			return nil, nil
		}
		var b bool
		if json.Unmarshal(value, &b) != nil {
			return nil, ErrSource
		}
		return &b, nil
	}
	eligible, err := read("eligible")
	if err != nil {
		return nil, nil, err
	}
	occupied, err := read("occupied")
	return eligible, occupied, err
}

// CoveredSource serves reports that have no partial form: supply (needs the
// individual roster) and demand-forecast (needs complete owner-witnessed
// history). Without complete verified coverage they deny.
type CoveredSource struct {
	Report      string
	Authorities map[string]admincontext.Authority
	UnitField   string
}

func (s *CoveredSource) ResolveScope(ctx context.Context, tx pgx.Tx, q Query) (adminpolicy.Facts, error) {
	snapshot, err := s.Snapshot(ctx, tx, q)
	return snapshot.Facts, err
}
func (s *CoveredSource) Snapshot(ctx context.Context, tx pgx.Tx, q Query) (Snapshot, error) {
	if s == nil || q.Metric != s.Report || (s.Report != "supply" && s.Report != "demand-forecast" && derivedReports[s.Report] == "") {
		return Snapshot{}, ErrUnmapped
	}
	out, err := completeSnapshot(ctx, tx, s.Authorities, s.Report, s.UnitField, q)
	if errors.Is(err, ErrCoverage) {
		return Snapshot{}, ErrUnmapped
	}
	return out, err
}
func (s *CoveredSource) AuthorizeRelease(ctx context.Context, tx pgx.Tx, q Query, version string, raw json.RawMessage) error {
	if s == nil || q.Metric != s.Report || len(raw) == 0 || len(raw) > 65536 || !isCompleteWitness(raw) {
		return ErrUnmapped
	}
	return authorizeComplete(ctx, tx, s.Authorities, s.UnitField, q, version, raw)
}

// tryComplete is the shared prefix of the partial sources: COMPLETE when the
// owner coverage verifies, otherwise (ok=false) the caller's bounded partial
// path runs unchanged.
func tryComplete(ctx context.Context, tx pgx.Tx, authorities map[string]admincontext.Authority, report, field string, q Query) (Snapshot, bool, error) {
	out, err := completeSnapshot(ctx, tx, authorities, report, field, q)
	if errors.Is(err, ErrCoverage) || errors.Is(err, ErrUnmapped) {
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, err
	}
	return out, true, nil
}
