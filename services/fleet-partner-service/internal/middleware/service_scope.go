package middleware

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"

	platformserviceauth "cosmicforge/logistics/services/fleet-partner-service/internal/platform/serviceauth"
)

// ServiceCredentialKey holds the *platformserviceauth.Credential resolved by
// InternalServiceScopeAuth, for handlers or later middleware that want the
// specific credential version/scope, not just the caller name
// ServiceCallerKey already carries.
const ServiceCredentialKey = "service_credential"

// InternalServiceScopeAuth resolves the caller's specific credential
// version (trying every one of that caller's active candidate secrets --
// see platformserviceauth.ScopeVerifier.Resolve, what makes rotation
// overlap possible) and authorizes it against this request's method, route
// template and (if the body carries one) event type -- Section 28's
// route/method/event scoping, layered on top of the identical HMAC
// signature check InternalServiceAuth already performs, never replacing or
// weakening it.
//
// Sets ServiceCallerKey exactly as InternalServiceAuth does (so every
// existing per-handler requireAllowedCaller check keeps working unchanged,
// as defense-in-depth, not as a replaced mechanism) plus ServiceCredentialKey
// for anything that wants the resolved credential itself.
//
// A nil verifier disables the check (the request proceeds unauthenticated
// by THIS middleware) -- matching every other optional dependency in this
// service's Options struct; app.BuildRouter only mounts this middleware at
// all when opts.ScopeVerifier is non-nil, falling back to the plain
// InternalServiceAuth otherwise, so a nil verifier reaching here should be
// unreachable in practice, not a silent downgrade path a real deployment
// takes.
func InternalServiceScopeAuth(verifier *platformserviceauth.ScopeVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		if verifier == nil {
			c.Next()
			return
		}
		serviceName := c.GetHeader("X-Service-Name")
		if serviceName == "" {
			httpx.Abort(c, apperrors.Unauthorized("Service authentication is required.", nil))
			return
		}

		cred, err := verifier.Resolve(c.Request.Context(), serviceName, c.Request)
		if err != nil {
			httpx.Abort(c, err)
			return
		}

		routePattern := c.FullPath()
		eventType := peekEventType(c)
		if err := verifier.Authorize(cred, c.Request.Method, routePattern, eventType); err != nil {
			httpx.Abort(c, err)
			return
		}

		c.Set(ServiceCallerKey, serviceName)
		c.Set(ServiceCredentialKey, cred)
		if err := verifier.TouchLastUsed(c.Request.Context(), cred); err != nil {
			// Bookkeeping only -- an otherwise fully-authorized request must
			// never fail because this write did. Logged via the standard
			// request logger's error field, not propagated.
			c.Set("service_credential_touch_error", err.Error())
		}
		c.Next()
	}
}

// eventTypeCandidateFields are checked, in order, against the request
// body's top-level JSON object for an event-type value. "eventType" is the
// one real field name across every event-carrying internal route in this
// codebase today -- marketplace's usecases.InboundEvent, finance's
// internal/events.Envelope (also what documents' media-events ingestion
// unmarshals into) all tag it `json:"eventType"`, confirmed by reading each
// struct definition, not assumed. Kept as a slice (not a bare constant) so
// a future producer that genuinely needs a different field name is a
// one-line addition here rather than a new bespoke peek function --
// deliberately generic rather than a route-keyed map, so a future
// event-carrying route needs no middleware change to be covered, only a
// credential scoped with the right allowedEventTypes entries.
var eventTypeCandidateFields = []string{"eventType"}

// peekEventType reads the request body (already read and reset at least
// once upstream by HMAC signature verification, so this adds one more
// bounded read-and-reset, not a new I/O concern) looking for a top-level
// string event-type field. Returns "" -- meaning "not an event-carrying
// request, do not scope-check event type" -- on any parse failure, a
// missing field, or a non-string value: a route that does not actually
// carry an event type must never be accidentally gated by this check.
func peekEventType(c *gin.Context) string {
	if c.Request.Body == nil {
		return ""
	}
	body, err := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil || len(body) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	for _, field := range eventTypeCandidateFields {
		if value, ok := payload[field].(string); ok && value != "" {
			return value
		}
	}
	return ""
}
