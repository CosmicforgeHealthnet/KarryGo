package bridge

import (
	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"errors"
	"net/http"
	"net/url"
)

// The foundation accepts only scoped, bounded query primitives. Owner-specific
// filters must be added with the adapter and schema that actually enforce them.
func strictQuery(r *http.Request) (url.Values, error) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, err
	}
	for k, v := range q {
		if (k != "market" && k != "organization" && k != "purpose") || len(v) != 1 || !security.Opaque(v[0]) {
			return nil, errors.New("invalid query")
		}
	}
	if q.Get("market") == "" || q.Get("organization") == "" || q.Get("purpose") == "" {
		return nil, errors.New("scope required")
	}
	return q, nil
}
