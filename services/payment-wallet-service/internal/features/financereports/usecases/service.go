// Package financereportusecases turns the Wallet's read-only source snapshot
// into a Section 26 group finance report.
//
// It is REPORTING, never control: nothing here writes a row. Figures are
// integer minor units per currency, never summed across currencies, and none
// is labelled recognized revenue -- no recognition policy exists, so the net
// platform fee is a source figure pending policy.
package financereportusecases

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	repo "cosmicforge/logistics/services/payment-wallet-service/internal/features/financereports/repositories"
	"cosmicforge/logistics/shared/go/financereport"
)

const (
	AggregationVersion = "wallet-finance-report.v1"
	MaxWindow          = 93 * 24 * time.Hour
)

var ErrInvalidRequest = errors.New("invalid finance report request")

// Request is a validated report window. Boundaries are UTC instants chosen by
// the caller; the timezone is carried so a consumer knows which local
// calendar the caller used to choose them.
type Request struct {
	PeriodStart time.Time
	PeriodEnd   time.Time
	Timezone    string
}

// ParseRequest accepts exactly period_start, period_end and timezone, each
// once. Instants must be RFC 3339 in UTC (Z); the timezone must be an IANA
// name.
func ParseRequest(values map[string][]string) (Request, error) {
	allowed := map[string]bool{"period_start": true, "period_end": true, "timezone": true}
	for key, v := range values {
		if !allowed[key] || len(v) != 1 {
			return Request{}, ErrInvalidRequest
		}
	}
	for key := range allowed {
		if len(values[key]) != 1 || values[key][0] == "" {
			return Request{}, ErrInvalidRequest
		}
	}
	start, err := parseUTC(values["period_start"][0])
	if err != nil {
		return Request{}, err
	}
	end, err := parseUTC(values["period_end"][0])
	if err != nil {
		return Request{}, err
	}
	if !end.After(start) || end.Sub(start) > MaxWindow {
		return Request{}, ErrInvalidRequest
	}
	zone := values["timezone"][0]
	if len(zone) > 64 || zone == "Local" || !strings.Contains(zone, "/") && zone != "UTC" {
		return Request{}, ErrInvalidRequest
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return Request{}, ErrInvalidRequest
	}
	return Request{PeriodStart: start, PeriodEnd: end, Timezone: zone}, nil
}

func parseUTC(raw string) (time.Time, error) {
	if !strings.HasSuffix(raw, "Z") || len(raw) > 40 {
		return time.Time{}, ErrInvalidRequest
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, ErrInvalidRequest
	}
	return t.UTC(), nil
}

type Reader interface {
	Read(ctx context.Context, start, end time.Time) (repo.Snapshot, error)
}

type Service struct{ Reader Reader }

// Report computes and validates the report for one window.
func (s Service) Report(ctx context.Context, req Request) (financereport.Report, error) {
	snap, err := s.Reader.Read(ctx, req.PeriodStart, req.PeriodEnd)
	if err != nil {
		return financereport.Report{}, err
	}
	report := Build(req, snap)
	if err := report.Validate(); err != nil {
		return financereport.Report{}, fmt.Errorf("finance report failed its own validation: %w", err)
	}
	return report, nil
}

// Build is pure: the same snapshot always yields the same report.
func Build(req Request, snap repo.Snapshot) financereport.Report {
	report := financereport.Report{
		Contract:           financereport.Contract,
		Owner:              financereport.Owner,
		PeriodStart:        req.PeriodStart,
		PeriodEnd:          req.PeriodEnd,
		Timezone:           req.Timezone,
		DataAsOf:           snap.DataAsOf.UTC(),
		AggregationVersion: AggregationVersion,
		Unit:               "MINOR_UNITS",
		Definitions:        financereport.Measures(),
		Completeness:       financereport.CompletenessComplete,
	}

	var watermark time.Time
	observe := func(ts *time.Time) {
		if ts != nil && !ts.After(snap.DataAsOf) && ts.After(watermark) {
			watermark = *ts
		}
	}
	currencySet := map[string]bool{}
	for _, c := range snap.Currencies {
		currencySet[c] = true
	}
	indexed := map[string]map[string]repo.Bucket{}
	for measure, list := range snap.Measures {
		indexed[measure] = map[string]repo.Bucket{}
		for _, b := range list {
			indexed[measure][b.Currency] = b
			currencySet[b.Currency] = true
			observe(b.MaxTS)
		}
	}
	releases := map[string]int64{}
	for _, b := range snap.CommerceReleases {
		releases[b.Currency] = b.Count
		currencySet[b.Currency] = true
		observe(b.MaxTS)
	}
	observe(snap.UnattributableMaxTS)
	observe(snap.ReconciliationMaxTS)

	currencies := make([]string, 0, len(currencySet))
	for c := range currencySet {
		currencies = append(currencies, c)
	}
	sort.Strings(currencies)

	for _, currency := range currencies {
		for _, def := range financereport.Measures() {
			f := figure(def.Measure, currency, indexed[def.Measure][currency], snap, releases[currency])
			if f.Completeness == financereport.CompletenessPartial {
				report.Completeness = financereport.CompletenessPartial
			}
			report.Figures = append(report.Figures, f)
		}
	}
	if req.PeriodEnd.After(snap.DataAsOf) {
		// The period has not closed: window figures cover [start, data_as_of).
		report.Completeness = financereport.CompletenessPartial
		report.ErrorReason = "period_not_closed: window figures cover period_start to data_as_of only"
	}
	if watermark.IsZero() {
		// Nothing relevant exists yet: the empty source is current through
		// the snapshot itself.
		watermark = report.DataAsOf
	}
	report.SourceWatermark = watermark.UTC()
	return report
}

func figure(measure, currency string, b repo.Bucket, snap repo.Snapshot, commerceReleases int64) financereport.Figure {
	amount, count := b.Amount, b.Count
	f := financereport.Figure{Measure: measure, Currency: currency, Completeness: financereport.CompletenessComplete}
	f.AmountMinor, f.Count = &amount, &count
	switch measure {
	case repo.MeasureSettlements:
		if commerceReleases > 0 {
			f.Completeness = financereport.CompletenessPartial
			f.UnknownReason = fmt.Sprintf("commerce_allocation_releases_not_counted: %d commerce releases in window settle per allocation, not per intent", commerceReleases)
		}
	case repo.MeasureLiabilities:
		if amount < 0 {
			return unknown(measure, currency, "ledger_inconsistency: escrow balances net negative")
		}
	case repo.MeasureUnreconciled:
		if !snap.CompletedReconciliation {
			return unknown(measure, currency, "no_completed_reconciliation_run: open reconciliation findings cannot be known")
		}
		if snap.UnattributableOpen > 0 {
			return unknown(measure, currency, fmt.Sprintf("unattributable_open_items: %d unmatched provider events or open findings carry no currency", snap.UnattributableOpen))
		}
	}
	return f
}

func unknown(measure, currency, reason string) financereport.Figure {
	return financereport.Figure{Measure: measure, Currency: currency, Completeness: financereport.CompletenessPartial, UnknownReason: reason}
}
