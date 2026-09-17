package goods

import "cosmicforge/logistics/shared/go/observability"

// goodsMetrics records low-cardinality Taxi Goods counters. A nil registry
// makes every call a no-op.
type goodsMetrics struct{ m *observability.Metrics }

func (g goodsMetrics) inc(name string, labels map[string]string) {
	if g.m != nil {
		g.m.IncCounter(name, labels, 1)
	}
}

func (g goodsMetrics) cargoRejection(reason string) {
	g.inc("taxi_match_hard_rejections_total", map[string]string{"purpose": "GOODS", "reason": reason})
	switch reason {
	case "CARGO_WEIGHT_EXCEEDED", "CARGO_VOLUME_EXCEEDED", "CARGO_DIMENSION_EXCEEDED", "CARGO_PACKAGE_COUNT_EXCEEDED":
		g.inc("taxi_goods_cargo_fit_failures_total", map[string]string{"reason": reason})
	}
}

func (g goodsMetrics) preview(serviceable bool) {
	g.inc("taxi_goods_preview_requests_total", nil)
	if serviceable {
		g.inc("taxi_goods_preview_serviceable_total", nil)
	} else {
		g.inc("taxi_goods_preview_rejected_total", nil)
	}
}

func (g goodsMetrics) multiPickupMatch(pickups int) {
	if g.m == nil {
		return
	}
	g.m.SetGauge("taxi_goods_pickups_per_mission", nil, float64(pickups))
	if pickups > 1 {
		g.inc("taxi_goods_multi_pickup_missions_total", nil)
	}
}

func (g goodsMetrics) pickupDelay() { g.inc("taxi_goods_pickup_delay_total", nil) }
func (g goodsMetrics) recovery(outcome string) {
	g.inc("taxi_recovery_"+outcome+"_total", map[string]string{"subject": "GOODS_MISSION"})
}
func (g goodsMetrics) custodyTransfer() { g.inc("taxi_custody_transfer_total", nil) }
