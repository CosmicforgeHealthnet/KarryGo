package bridge

import (
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"errors"
	"net/http"
	"net/url"
	"time"
)

func (b *Bridge) ConfigureScopedAnalytics(client *analyticsadmin.Client, policy *adminpolicy.Engine) error {
	if client == nil || b.scopedAnalytics != nil {
		return errors.New("invalid scoped Analytics configuration")
	}
	b.scopedAnalytics = client
	b.scopedAnalyticsPolicy = policy
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
		_, err = analyticsadmin.Values(query, filters)
	}
	return query, filters, parsed, err
}
