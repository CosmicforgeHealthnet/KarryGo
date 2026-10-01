package apiresponse

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/providercompat"
)

// ProviderFinancialPrivacy resolves the provider's corporate financial
// visibility policy once per request and projects the response body on the
// way out (HLG-021).
//
// It is a response middleware rather than a per-handler call because Hauling
// writes responses from many handlers with no single envelope helper.
// Intercepting the writer makes the projection TOTAL: a handler added later
// is covered automatically, and there is no call site a future change can
// forget.
//
// Fail-closed: if Fleet cannot be reached the request is refused with a
// retryable dependency error rather than served with an unmasked body. A
// Fleet outage must never result in a corporate driver being shown their
// organization's commercial data.
//
// A nil reader means the deployment has no Fleet integration at all: every
// provider is independent and the response contract is byte-for-byte the
// pre-corporate one.
// providerRole and providerService identify a Hauling driver's own bearer
// token. The middleware is mounted at the router root so no provider route
// can be added outside it, which means it MUST distinguish a driver from a
// customer: both carry claims, and reading a customer id as a provider id
// would ask Fleet about the wrong subject entirely.
func ProviderFinancialPrivacy(reader providercompat.CorporateContextReader, enabled bool, providerRole, providerService string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := sharedauth.ClaimsFromContext(c)
		if !ok || claims.Subject == "" || claims.Role != providerRole || claims.Service != providerService {
			c.Next()
			return
		}
		c.Set(KeyProviderID, claims.Subject)

		managed := false
		var policy *providercompat.FinancialVisibilityPolicy
		if reader != nil && enabled {
			corporate, err := reader.ReadCorporateContext(c.Request.Context(), "HAULING", claims.Subject)
			if err != nil {
				httpx.RespondError(c, apperrors.Unavailable("Provider visibility authority is unavailable.", err))
				c.Abort()
				return
			}
			managed = corporate.IsManagedProvider
			if managed {
				typed := corporate.FinancialVisibilityPolicy
				policy = &typed
				c.Set(KeyPolicy, typed)
			}
		}
		c.Set(KeyManaged, managed)

		if !managed {
			// An independent provider's body is untouched. Skipping the
			// capture entirely is what guarantees HLG-024: no independent
			// response can be altered by this middleware at all.
			c.Next()
			return
		}

		capture := &projectingWriter{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = capture
		c.Next()
		capture.flush(managed, policy)
	}
}

// projectingWriter buffers a JSON response so the body can be projected
// before it reaches the provider.
type projectingWriter struct {
	gin.ResponseWriter
	body    *bytes.Buffer
	status  int
	flushed bool
}

func (w *projectingWriter) WriteHeader(status int) {
	// The status is recorded but NOT forwarded yet: writing it now would
	// commit headers before Content-Length can be corrected for the projected
	// body.
	w.status = status
}

func (w *projectingWriter) Write(payload []byte) (int, error) {
	return w.body.Write(payload)
}

func (w *projectingWriter) WriteString(payload string) (int, error) {
	return w.body.WriteString(payload)
}

// flush projects the buffered body and writes it through.
func (w *projectingWriter) flush(managed bool, policy *providercompat.FinancialVisibilityPolicy) {
	if w.flushed {
		return
	}
	w.flushed = true
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	payload := w.body.Bytes()
	if isJSON(w.Header().Get("Content-Type")) {
		payload = ProjectJSON(payload, managed, policy)
	}
	// Content-Length is rewritten because concealing a field changes the
	// body's length; a stale header would truncate the response.
	w.Header().Set("Content-Length", itoa(len(payload)))
	w.ResponseWriter.WriteHeader(status)
	if len(payload) > 0 {
		_, _ = w.ResponseWriter.Write(payload)
	}
}

func isJSON(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "json")
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := [20]byte{}
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}
