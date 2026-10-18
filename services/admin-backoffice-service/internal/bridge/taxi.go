package bridge

import (
	"cosmicforge/logistics/services/admin-backoffice-service/internal/clients"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ConfigureDomainFlags is startup-only. Enabling a flag cannot supply a missing
// owner adapter or override an unapproved policy.
func (b *Bridge) ConfigureDomainFlags(flags config.DomainFlags) { b.features = flags }

// ConfigureTaxi is startup-only. No request can choose a client or destination.
func (b *Bridge) ConfigureTaxi(client *clients.Taxi, approvedRoles []string, policies ...*adminpolicy.Engine) error {
	if client == nil || len(approvedRoles) == 0 || len(approvedRoles) > 32 {
		return errors.New("Taxi owner and approved read roles required")
	}
	for _, role := range approvedRoles {
		if !security.Opaque(role) {
			return errors.New("invalid approved role")
		}
	}
	b.taxi = client
	b.taxiRoles = append([]string(nil), approvedRoles...)
	if len(policies) == 1 {
		b.accessPolicy = policies[0]
	} else {
		b.accessPolicy = nil
	}
	return nil
}
func hasRole(actual, approved []string) bool {
	for _, a := range actual {
		for _, r := range approved {
			if a == r {
				return true
			}
		}
	}
	return false
}
func taxiReadQuery(r *http.Request, staff, role string, policy *adminpolicy.Engine, now time.Time) (adminread.Query, url.Values, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return adminread.Query{}, nil, err
	}
	for _, key := range []string{"staff", "role", "policy_id", "policy_version", "policy_checksum"} {
		if _, exists := values[key]; exists {
			return adminread.Query{}, nil, adminread.ErrQuery
		}
	}
	values.Set("staff", staff)
	values.Set("role", role)
	values.Set("policy_id", policy.ID())
	values.Set("policy_version", strconv.FormatInt(policy.Version(), 10))
	values.Set("policy_checksum", policy.Checksum())
	q, err := adminread.Parse(values, now)
	return q, values, err
}
