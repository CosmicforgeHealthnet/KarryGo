package webhooks

import (
	"net"
	"net/url"
	"strings"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
)

// validateWebhookURL enforces HTTPS and blocks localhost/private/link-local hosts
// to prevent SSRF against internal services.
func validateWebhookURL(raw string) *delivery.FieldError {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return &delivery.FieldError{Field: "url", Message: "This field is required."}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return &delivery.FieldError{Field: "url", Message: "Must be a valid URL."}
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return &delivery.FieldError{Field: "url", Message: "Webhook URL must use https."}
	}
	host := u.Hostname()
	if host == "" {
		return &delivery.FieldError{Field: "url", Message: "Webhook URL must include a host."}
	}
	if isBlockedHost(host) {
		return &delivery.FieldError{Field: "url", Message: "Webhook URL must not point to localhost or a private address."}
	}
	return nil
}

func isBlockedHost(host string) bool {
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// Hostname (not a literal IP). Allow; DNS-rebinding defense is out of scope.
		return false
	}
	return isBlockedIP(ip)
}

func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	// Explicit RFC1918 / unique-local coverage (IsPrivate already covers these,
	// kept for clarity of intent).
	for _, cidr := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"} {
		_, n, _ := net.ParseCIDR(cidr)
		if n != nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

// validateAndNormalizeEvents ensures every event is allowed and dedupes while
// preserving first-seen order.
func validateAndNormalizeEvents(events []string) ([]string, *delivery.FieldError) {
	if len(events) == 0 {
		return nil, &delivery.FieldError{Field: "events", Message: "At least one event is required."}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(events))
	for _, e := range events {
		ev := strings.TrimSpace(e)
		if _, ok := allowedPartnerEvents[ev]; !ok {
			return nil, &delivery.FieldError{Field: "events", Message: "Unknown event type: " + ev + "."}
		}
		if _, dup := seen[ev]; dup {
			continue
		}
		seen[ev] = struct{}{}
		out = append(out, ev)
	}
	return out, nil
}
