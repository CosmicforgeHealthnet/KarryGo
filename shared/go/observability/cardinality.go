package observability

import "strings"

// OBS-H001: which label KEYS are safe.
//
// A Prometheus-style series is created per distinct label combination, so a
// label carrying an identifier creates one series per entity. An order id or a
// correlation id does not merely make the dashboard noisy — it grows the
// registry without bound for as long as the service runs, and the first
// symptom is usually the monitoring system falling over rather than anything
// pointing back here.
//
// Identifiers belong in logs and traces, where they are searchable and where
// nothing multiplies. This check is about the KEY, not the value, because the
// key is what a reviewer can actually see at the call site.
//
// It deliberately does not reject a key merely for naming a domain concept:
// provider_mode is three values and is exactly the kind of breakdown that
// makes a dashboard useful. provider_id is unbounded. The difference is
// identifier-ness, not the word "provider".
func LabelKeyIsLowCardinality(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return false
	}
	// camelCase is checked on the ORIGINAL key: lowercasing turns orderId
	// into orderid, which no snake_case rule would catch. Both spellings
	// appear in this repository, so both have to be rejected.
	if strings.HasSuffix(key, "Id") || strings.HasSuffix(key, "ID") {
		return false
	}
	normalized := strings.NewReplacer("-", "_", ".", "_").Replace(k)
	if normalized == "id" || strings.HasSuffix(normalized, "_id") {
		return false
	}
	for _, unbounded := range []string{
		"uuid", "correlation", "causation", "request_id", "idempotency",
		"reference", "_ref", "email", "phone", "token", "key", "hash",
	} {
		if strings.Contains(normalized, unbounded) {
			return false
		}
	}
	return true
}

// UnsafeLabelKeys returns the keys in a label set that must not be used.
// Empty means the set is safe.
func UnsafeLabelKeys(labels map[string]string) []string {
	var unsafe []string
	for key := range labels {
		if !LabelKeyIsLowCardinality(key) {
			unsafe = append(unsafe, key)
		}
	}
	return unsafe
}
