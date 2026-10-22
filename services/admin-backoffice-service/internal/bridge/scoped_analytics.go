package bridge

import (
	"context"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (b *Bridge) ConfigureScopedAnalytics(client *analyticsadmin.Client, policy *adminpolicy.Engine, rates ...bool) error {
	if client == nil || b.scopedAnalytics != nil {
		return errors.New("invalid scoped Analytics configuration")
	}
	b.scopedAnalytics = client
	b.scopedAnalyticsPolicy = policy
	b.scopedAnalyticsRate = len(rates) == 1 && rates[0]
	return nil
}
func (b *Bridge) scopedAnalyticsAvailable(roles, zones, tenants []string) bool {
	rule, err := b.scopedAnalyticsPolicy.Rule(analyticsadmin.MissionSuccessPermission, b.now())
	return b.scopedAnalytics != nil && b.features.AnalyticsReads && err == nil && rule.Owner == "analytics-service" && analyticsadmin.ValidMissionSourceContract(rule.SourceContract) && hasRole(roles, rule.Roles) && len(zones) > 0 && len(tenants) > 0
}
func scopedAnalyticsQuery(r *http.Request, staff, role string, policy *adminpolicy.Engine, now time.Time) (adminread.Query, analyticsadmin.Filters, url.Values, error) {
	var empty adminread.Query
	var filters analyticsadmin.Filters
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return empty, filters, nil, err
	}
	if len(values["authority"]) != 1 || len(values["product_code"]) != 1 || values.Has("limit") || values.Has("cursor") {
		return empty, filters, nil, errors.New("invalid scoped Analytics filters")
	}
	filters = analyticsadmin.Filters{Authority: values.Get("authority"), ProductCode: values.Get("product_code")}
	values.Del("authority")
	values.Del("product_code")
	values.Set("limit", "1")
	clone := r.Clone(r.Context())
	copyURL := *r.URL
	clone.URL = &copyURL
	clone.URL.RawQuery = values.Encode()
	query, parsed, err := taxiReadQuery(clone, staff, role, policy, now)
	if err == nil {
		reportName := strings.TrimPrefix(r.URL.Path, Base+"/analytics/scoped/network/")
		reportName = strings.TrimSuffix(reportName, "/rate")
		report, ok := analyticsadmin.ReportNamed(reportName)
		if !ok {
			return empty, filters, nil, errors.New("unknown scoped report")
		}
		_, err = report.Values(query, filters)
	}
	return query, filters, parsed, err
}

func (b *Bridge) scopedRateApproved() bool {
	counts, e := b.scopedAnalyticsPolicy.Rule(analyticsadmin.MissionSuccessPermission, b.now())
	if e != nil {
		return false
	}
	rate, e := b.scopedAnalyticsPolicy.Rule(analyticsadmin.MissionRatePermission, b.now())
	return b.scopedAnalyticsRate && e == nil && rate.Owner == "analytics-service" && analyticsadmin.MissionRateSourceContract(counts.SourceContract) != "" && rate.SourceContract == analyticsadmin.MissionRateSourceContract(counts.SourceContract)
}

func (b *Bridge) scopedRateAvailable(roles, zones, tenants, permissions []string) bool {
	if !b.scopedAnalyticsAvailable(roles, zones, tenants) || !b.scopedRateApproved() {
		return false
	}
	rate, _ := b.scopedAnalyticsPolicy.Rule(analyticsadmin.MissionRatePermission, b.now())
	return hasRole(roles, rate.Roles) && config.Contains(permissions, analyticsadmin.MissionSuccessPermission)
}

func scopedReportCapability(capability string) (analyticsadmin.Report, bool) {
	if capability == analyticsadmin.MissionRatePermission {
		r, _ := analyticsadmin.ReportNamed("mission-success")
		return r, true
	}
	for _, report := range analyticsadmin.Reports() {
		if capability == report.Permission {
			return report, true
		}
	}
	return analyticsadmin.Report{}, false
}
func (b *Bridge) scopedReportAvailable(report analyticsadmin.Report, roles, zones, tenants []string) bool {
	rule, err := b.scopedAnalyticsPolicy.Rule(report.Permission, b.now())
	return b.scopedAnalytics != nil && b.features.AnalyticsReads && err == nil && rule.Owner == "analytics-service" && report.ValidSourceContract(rule.SourceContract) && hasRole(roles, rule.Roles) && len(zones) > 0 && len(tenants) > 0
}
func (b *Bridge) scopedReportRead(ctx context.Context, name string, q adminread.Query, f analyticsadmin.Filters) (analyticsadmin.Release, error) {
	switch name {
	case "mission-success":
		return b.scopedAnalytics.MissionSuccess(ctx, q, f)
	case "mission-outcomes":
		return b.scopedAnalytics.MissionOutcomes(ctx, q, f)
	case "recovery":
		return b.scopedAnalytics.Recovery(ctx, q, f)
	case "capability-mismatches":
		return b.scopedAnalytics.CapabilityMismatches(ctx, q, f)
	case "declines":
		return b.scopedAnalytics.Declines(ctx, q, f)
	case "demand":
		return b.scopedAnalytics.Demand(ctx, q, f)
	case "demand-forecast":
		return b.scopedAnalytics.DemandForecast(ctx, q, f)
	case "supply":
		return b.scopedAnalytics.Supply(ctx, q, f)
	case "provider-quality":
		return b.scopedAnalytics.ProviderQuality(ctx, q, f)
	case "readiness":
		return b.scopedAnalytics.Readiness(ctx, q, f)
	}
	return analyticsadmin.Release{}, analyticsadmin.ErrInvalid
}
