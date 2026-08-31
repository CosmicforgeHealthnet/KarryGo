package routeintegrity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"
	"time"
)

func (p Policy) normalized() Policy {
	if p.CorridorMeters <= 0 {
		p.CorridorMeters = 250
	}
	if p.WrongWayToleranceDegrees <= 0 {
		p.WrongWayToleranceDegrees = 110
	}
	if p.StationaryThreshold <= 0 {
		p.StationaryThreshold = 10 * time.Minute
	}
	if p.StaleThreshold <= 0 {
		p.StaleThreshold = 2 * time.Minute
	}
	if p.HighRiskCorridorMeters <= 0 {
		p.HighRiskCorridorMeters = 120
	}
	if p.HighRiskStaleThreshold <= 0 {
		p.HighRiskStaleThreshold = time.Minute
	}
	if p.MaximumAccuracyMeters <= 0 {
		p.MaximumAccuracyMeters = 150
	}
	if p.MaximumClientClockSkew <= 0 {
		p.MaximumClientClockSkew = 10 * time.Minute
	}
	return p
}

func commandFingerprint(providerID string, command DeviceCommand) string {
	raw, _ := json.Marshal(struct {
		Provider string
		Command  DeviceCommand
	}{providerID, command})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func canonicalCommandType(v string) string { return strings.ToUpper(strings.TrimSpace(v)) }

func haversineMeters(a, b Point) float64 {
	const earth = 6371000.0
	lat1, lat2 := a.Latitude*math.Pi/180, b.Latitude*math.Pi/180
	dlat := (b.Latitude - a.Latitude) * math.Pi / 180
	dlon := (b.Longitude - a.Longitude) * math.Pi / 180
	h := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dlon/2)*math.Sin(dlon/2)
	return 2 * earth * math.Asin(math.Sqrt(h))
}

func distanceToSegmentMeters(p, a, b Point) float64 {
	// Local equirectangular projection is stable at delivery-route scale.
	lat0 := p.Latitude * math.Pi / 180
	x := func(q Point) float64 { return (q.Longitude - p.Longitude) * math.Pi / 180 * math.Cos(lat0) * 6371000 }
	y := func(q Point) float64 { return (q.Latitude - p.Latitude) * math.Pi / 180 * 6371000 }
	ax, ay, bx, by := x(a), y(a), x(b), y(b)
	dx, dy := bx-ax, by-ay
	t := 0.0
	if denom := dx*dx + dy*dy; denom > 0 {
		t = -(ax*dx + ay*dy) / denom
	}
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return math.Hypot(ax+t*dx, ay+t*dy)
}

func distanceToRouteMeters(p Point, points []Point) float64 {
	if len(points) == 0 {
		return math.Inf(1)
	}
	if len(points) == 1 {
		return haversineMeters(p, points[0])
	}
	best := math.Inf(1)
	for i := 1; i < len(points); i++ {
		if d := distanceToSegmentMeters(p, points[i-1], points[i]); d < best {
			best = d
		}
	}
	return best
}

func bearingDegrees(a, b Point) float64 {
	lat1, lat2 := a.Latitude*math.Pi/180, b.Latitude*math.Pi/180
	dlon := (b.Longitude - a.Longitude) * math.Pi / 180
	y := math.Sin(dlon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dlon)
	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360)
}
func angleDelta(a, b float64) float64 {
	d := math.Abs(a - b)
	if d > 180 {
		return 360 - d
	}
	return d
}
