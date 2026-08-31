package commerceanalytics

import (
	"bytes"
	"encoding/json"
	"errors"
)

// DispatchRouteMetric is deliberately not an operational route or location fix.
// Only coarse, non-identifying measurements may leave the route authority.
type DispatchRouteMetric struct {
	RouteVersion          int64  `json:"route_version"`
	AlertType             string `json:"alert_type,omitempty"`
	Severity              string `json:"severity,omitempty"`
	State                 string `json:"state"`
	PolicyVersion         string `json:"policy_version"`
	DistanceBucketKM      int64  `json:"distance_bucket_km,omitempty"`
	DurationBucketMinutes int64  `json:"duration_bucket_minutes,omitempty"`
}

func IsDispatchRouteEvent(name string) bool {
	switch name {
	case "dispatch.route.created.v1", "dispatch.route.versioned.v1", "dispatch.route.rerouted.v1", "dispatch.route_alert.opened.v1":
		return true
	}
	return false
}

func ValidateDispatchRouteMetric(raw json.RawMessage) error {
	var p DispatchRouteMetric
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&p) != nil || p.RouteVersion < 1 || p.PolicyVersion != "dispatch_route_integrity_v1" || p.DistanceBucketKM < 0 || p.DurationBucketMinutes < 0 {
		return errors.New("invalid minimized route metric")
	}
	switch p.State {
	case "CURRENT", "SUPERSEDED", "OPEN", "RESOLVED":
	default:
		return errors.New("invalid route metric state")
	}
	switch p.AlertType {
	case "", "ROUTE_DEVIATION", "EXCESSIVE_STATIONARY", "WRONG_WAY_MOVEMENT", "STALE_LOCATION":
	default:
		return errors.New("invalid route alert type")
	}
	switch p.Severity {
	case "", "LOW", "MEDIUM", "HIGH", "CRITICAL":
	default:
		return errors.New("invalid route severity")
	}
	return nil
}
