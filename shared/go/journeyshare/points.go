package journeyshare

import (
	"context"
	"encoding/json"
	"github.com/redis/go-redis/v9"
	"math"
	"time"
)

// BoundPointStore is an optional safe latest-point projection updated ONLY by
// existing provider ingestion. Bindings are resolved by the owning authority.
type BoundPointStore struct {
	Client    redis.UniversalClient
	Namespace string
}
type Point struct {
	Lat, Lng, Heading, Accuracy float64
	At                          time.Time
}

func (s BoundPointStore) key(ref, provider, binding string) string {
	return "journeyshare:point:" + s.Namespace + ":" + ref + ":" + provider + ":" + binding
}
func (s BoundPointStore) Put(ctx context.Context, ref, provider, binding string, p Point) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return pointScript.Run(ctx, s.Client, []string{s.key(ref, provider, binding)}, raw, p.At.UnixMicro(), (10 * time.Minute).Milliseconds()).Err()
}

var pointScript = redis.NewScript(`local old=redis.call('HGET',KEYS[1],'at'); if old and tonumber(old)>=tonumber(ARGV[2]) then return 0 end; redis.call('HSET',KEYS[1],'data',ARGV[1],'at',ARGV[2]); redis.call('PEXPIRE',KEYS[1],ARGV[3]); return 1`)

func (s BoundPointStore) Read(ctx context.Context, ref, provider, binding string, fresh time.Duration) (Location, error) {
	out := Location{State: "UNAVAILABLE"}
	raw, err := s.Client.HGet(ctx, s.key(ref, provider, binding), "data").Bytes()
	if err == redis.Nil {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	var p Point
	if json.Unmarshal(raw, &p) != nil {
		return out, nil
	}
	if p.At.IsZero() || p.At.After(time.Now().UTC()) || math.IsNaN(p.Lat) || math.IsNaN(p.Lng) || p.Lat < -90 || p.Lat > 90 || p.Lng < -180 || p.Lng > 180 {
		return out, nil
	}
	state := "STALE"
	if time.Since(p.At) <= fresh {
		state = "FRESH"
	}
	heading, accuracy := &p.Heading, &p.Accuracy
	if *heading < 0 || *heading > 360 || math.IsNaN(*heading) || math.IsInf(*heading, 0) {
		heading = nil
	}
	if *accuracy < 0 || math.IsNaN(*accuracy) || math.IsInf(*accuracy, 0) {
		accuracy = nil
	}
	return Location{Available: true, State: state, Lat: &p.Lat, Lng: &p.Lng, Heading: heading, AccuracyM: accuracy, UpdatedAt: &p.At}, nil
}
