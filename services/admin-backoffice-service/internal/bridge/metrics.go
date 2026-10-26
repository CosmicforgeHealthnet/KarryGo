package bridge

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// serveMetrics is mounted only on the loopback probe listener (127.0.0.1),
// never on the mTLS HQ listener or any public edge (Section 27). Labels are
// low-cardinality owner/entity names; no staff, customer, case or token data.
// Unknown values are omitted rather than reported as zero.
func (b *Bridge) serveMetrics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	var out strings.Builder
	gauge := func(name, help string, value any, labels ...string) {
		fmt.Fprintf(&out, "# HELP %s %s\n# TYPE %s gauge\n", name, help, name)
		if len(labels) > 0 {
			fmt.Fprintf(&out, "%s{%s} %v\n", name, strings.Join(labels, ","), value)
		} else {
			fmt.Fprintf(&out, "%s %v\n", name, value)
		}
	}
	gauge("karryam_admin_requests_denied_total", "Denied HQ requests since start", b.Denied.Load())
	gauge("karryam_admin_audit_failures_total", "Audit writes that failed (requests refused)", b.AuditFailures.Load())
	if m := b.EventMetrics; m != nil {
		gauge("karryam_admin_event_ingress_accepted_total", "Owner events accepted", m.IngressAccepted.Load())
		gauge("karryam_admin_event_ingress_quarantined_total", "Owner events quarantined", m.IngressQuarantined.Load())
		gauge("karryam_admin_event_ingress_bad_signature_total", "Owner event ingress HMAC failures", m.IngressBadSignature.Load())
		gauge("karryam_admin_event_ingress_replay_total", "Owner event ingress nonce replays", m.IngressReplay.Load())
	}
	if b.hqEvents != nil {
		if st, err := b.hqEvents.Status(ctx, 15*time.Minute); err == nil {
			gauge("karryam_admin_event_quarantine_rows", "Quarantined owner events (durable)", st.Quarantined)
			gauge("karryam_admin_hq_outbox_head_sequence", "Latest HQ feed sequence", st.HeadSequence)
			for _, src := range st.Sources {
				l := []string{`source="` + src.SourceService + `"`, `entity="` + src.EntityType + `"`}
				gauge("karryam_admin_projection_open_gaps", "Unrepaired projection version gaps", src.OpenGaps, l...)
				if src.LagSeconds != nil {
					gauge("karryam_admin_projection_lag_seconds", "Seconds since last projection update", *src.LagSeconds, l...)
				}
				stale := 0
				if src.Freshness != "LIVE" {
					stale = 1
				}
				gauge("karryam_admin_projection_not_live", "1 when projection is STALE or UNKNOWN", stale, l...)
			}
			var maxLag int64 = -1
			for _, lag := range st.Consumers {
				if lag > maxLag {
					maxLag = lag
				}
			}
			if maxLag >= 0 {
				gauge("karryam_admin_hq_consumer_lag_max", "Largest unacknowledged HQ feed backlog", maxLag)
			}
		}
		var unknownOwner, unknownCampaign, sosOpen int64
		var sosOldest *float64
		pool := b.hqEvents.DB
		if pool.QueryRow(ctx, `SELECT count(*) FROM admin_owner_command_operations WHERE status='UNKNOWN_OUTCOME'`).Scan(&unknownOwner) == nil {
			gauge("karryam_admin_owner_command_unknown_outcomes", "Owner commands awaiting original-identity reconciliation", unknownOwner)
		}
		if pool.QueryRow(ctx, `SELECT count(*) FROM admin_campaign_operations WHERE status='UNKNOWN_OUTCOME'`).Scan(&unknownCampaign) == nil {
			gauge("karryam_admin_campaign_unknown_outcomes", "Campaign submissions awaiting read-back", unknownCampaign)
		}
		if pool.QueryRow(ctx, `SELECT count(*), EXTRACT(EPOCH FROM now()-min((payload->>'created_at')::timestamptz)) FROM admin_projections WHERE entity_type='support_case' AND NOT tombstone AND payload->>'priority'='EMERGENCY' AND payload->>'status' NOT IN ('RESOLVED','CLOSED')`).Scan(&sosOpen, &sosOldest) == nil {
			gauge("karryam_admin_sos_open_cases", "Open emergency cases in the projection", sosOpen)
			if sosOldest != nil {
				gauge("karryam_admin_sos_oldest_open_age_seconds", "Age of the oldest open emergency case", int64(*sosOldest))
			}
		}
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(out.String()))
}
