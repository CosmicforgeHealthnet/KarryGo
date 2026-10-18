package bridge

import (
	"cosmicforge/logistics/services/admin-backoffice-service/internal/clients"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"cosmicforge/logistics/shared/go/adminread"
	"errors"
	"net/http"
	"net/url"
	"time"
)

// ConfigureTaxi is startup-only. No request can choose a client or destination.
func (b *Bridge) ConfigureTaxi(client *clients.Taxi, approvedRoles []string) error {
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
func taxiReadQuery(r *http.Request, staff string, now time.Time) (adminread.Query, url.Values, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return adminread.Query{}, nil, err
	}
	if _, exists := values["staff"]; exists {
		return adminread.Query{}, nil, adminread.ErrQuery
	}
	values.Set("staff", staff)
	q, err := adminread.Parse(values, now)
	return q, values, err
}
