package livetrack

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/geo"
)

// Store is the narrow interface service usecases depend on, so tests can
// supply a fake and a service without Redis can supply NoopStore.
type Store interface {
	// PutFix records a position. When discoverable is false the entity stays
	// readable by id but leaves the proximity index -- that is how a
	// provider on a job keeps a live map marker without being offered new
	// work.
	PutFix(ctx context.Context, fix Fix, discoverable bool) error
	// GetFix returns the last position, or ok=false once it has expired.
	GetFix(ctx context.Context, entityID string) (Fix, bool, error)
	// SetDiscoverable adds or removes an entity from the proximity index
	// without disturbing its last position.
	SetDiscoverable(ctx context.Context, entityID string, discoverable bool) error
	// Remove drops the position and index entries and notifies subscribers.
	Remove(ctx context.Context, entityID string) error
	// Nearby returns entities inside the radius, nearest first.
	Nearby(ctx context.Context, query NearbyQuery) ([]NearbyEntity, error)
}

// Key builders. Exported so operators can inspect a namespace by hand and so
// a service can subscribe to a channel without reimplementing the format.

// FixKey is where one entity's last position lives.
func FixKey(namespace, entityID string) string {
	return fmt.Sprintf("%s:track:fix:%s", namespace, entityID)
}

// CellKey remembers which index cell an entity was last written into, so a
// move can retract the previous entry.
func CellKey(namespace, entityID string) string {
	return fmt.Sprintf("%s:track:cell:%s", namespace, entityID)
}

// IndexKey is the sorted set holding every entity currently inside one cell,
// scored by fix time.
func IndexKey(namespace string, precision int, cell geo.Cell) string {
	return fmt.Sprintf("%s:track:idx:%d:%s", namespace, precision, cell)
}

// ChannelKey is the pub/sub channel carrying one entity's updates. A service
// that already shipped a different channel name to its clients can override it
// via Config.ChannelKeyFunc.
func ChannelKey(namespace, entityID string) string {
	return fmt.Sprintf("%s:track:chan:%s", namespace, entityID)
}

// channelKey resolves the publish channel, honouring any configured override.
func (s *RedisStore) channelKey(entityID string) string {
	if s != nil && s.cfg.ChannelKeyFunc != nil {
		return s.cfg.ChannelKeyFunc(entityID)
	}
	return ChannelKey(s.cfg.Namespace, entityID)
}

// ChannelFor reports where a store publishes an entity's updates, so a
// websocket handler subscribes to exactly what the store writes to.
func (s *RedisStore) ChannelFor(entityID string) string { return s.channelKey(entityID) }

// RedisStore is the Redis-backed Store.
type RedisStore struct {
	client *redis.Client
	cfg    Config
}

// NewRedisStore builds a store for one namespace. A nil client yields a store
// whose methods are no-ops, so callers never have to branch on Redis being
// configured.
func NewRedisStore(client *redis.Client, cfg Config) *RedisStore {
	return &RedisStore{client: client, cfg: cfg.WithDefaults()}
}

// Config returns the effective configuration, defaults applied.
func (s *RedisStore) Config() Config {
	if s == nil {
		return Config{}.WithDefaults()
	}
	return s.cfg
}

func (s *RedisStore) disabled() bool { return s == nil || s.client == nil }

// indexTTL is how long an untouched cell sorted set survives. Cells are pruned
// by score on read; this only reclaims cells nothing writes to any more.
func (s *RedisStore) indexTTL() time.Duration { return s.cfg.FixTTL * 4 }

func (s *RedisStore) PutFix(ctx context.Context, fix Fix, discoverable bool) error {
	if s.disabled() {
		return nil
	}
	if fix.EntityID == "" {
		return fmt.Errorf("livetrack: fix is missing an entity id")
	}
	if !fix.Point().Valid() {
		return fmt.Errorf("livetrack: fix for %s has an out-of-range coordinate", fix.EntityID)
	}
	if fix.RecordedAt.IsZero() {
		fix.RecordedAt = time.Now().UTC()
	}
	if fix.ReceivedAt.IsZero() {
		fix.ReceivedAt = time.Now().UTC()
	}

	payload, err := json.Marshal(fix)
	if err != nil {
		return fmt.Errorf("livetrack: marshal fix: %w", err)
	}

	// The cell recorded last time, so a move retracts the stale entry rather
	// than leaving the entity indexed in two places until its score ages out.
	previous, err := s.currentCell(ctx, fix.EntityID)
	if err != nil {
		return err
	}
	cell := geo.Encode(fix.Point(), s.cfg.IndexPrecision())

	pipe := s.client.Pipeline()
	pipe.Set(ctx, FixKey(s.cfg.Namespace, fix.EntityID), payload, s.cfg.FixTTL)

	if previous != "" && previous != cell {
		s.pipeDeindex(ctx, pipe, fix.EntityID, previous)
	}

	if discoverable {
		score := float64(fix.RecordedAt.UnixMilli())
		for _, precision := range s.cfg.CellPrecisions {
			key := IndexKey(s.cfg.Namespace, precision, cell.Parent(precision))
			pipe.ZAdd(ctx, key, redis.Z{Score: score, Member: fix.EntityID})
			pipe.Expire(ctx, key, s.indexTTL())
		}
		pipe.Set(ctx, CellKey(s.cfg.Namespace, fix.EntityID), string(cell), s.cfg.FixTTL)
	} else {
		s.pipeDeindex(ctx, pipe, fix.EntityID, cell)
		if previous != "" {
			s.pipeDeindex(ctx, pipe, fix.EntityID, previous)
		}
		pipe.Del(ctx, CellKey(s.cfg.Namespace, fix.EntityID))
	}

	if update, err := json.Marshal(LocationUpdate(fix)); err == nil {
		pipe.Publish(ctx, s.channelKey(fix.EntityID), update)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	return nil
}

func (s *RedisStore) GetFix(ctx context.Context, entityID string) (Fix, bool, error) {
	if s.disabled() {
		return Fix{}, false, nil
	}
	payload, err := s.client.Get(ctx, FixKey(s.cfg.Namespace, entityID)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return Fix{}, false, nil
		}
		return Fix{}, false, err
	}
	var fix Fix
	if err := json.Unmarshal(payload, &fix); err != nil {
		return Fix{}, false, err
	}
	return fix, true, nil
}

func (s *RedisStore) SetDiscoverable(ctx context.Context, entityID string, discoverable bool) error {
	if s.disabled() {
		return nil
	}
	if !discoverable {
		cell, err := s.currentCell(ctx, entityID)
		if err != nil {
			return err
		}
		pipe := s.client.Pipeline()
		if cell != "" {
			s.pipeDeindex(ctx, pipe, entityID, cell)
		}
		pipe.Del(ctx, CellKey(s.cfg.Namespace, entityID))
		_, err = pipe.Exec(ctx)
		return err
	}

	// Re-indexing needs a position to index at, so replay the stored fix.
	fix, ok, err := s.GetFix(ctx, entityID)
	if err != nil || !ok {
		return err
	}
	return s.PutFix(ctx, fix, true)
}

func (s *RedisStore) Remove(ctx context.Context, entityID string) error {
	if s.disabled() {
		return nil
	}
	cell, err := s.currentCell(ctx, entityID)
	if err != nil {
		return err
	}

	pipe := s.client.Pipeline()
	pipe.Del(ctx, FixKey(s.cfg.Namespace, entityID))
	pipe.Del(ctx, CellKey(s.cfg.Namespace, entityID))
	if cell != "" {
		s.pipeDeindex(ctx, pipe, entityID, cell)
	}
	if update, err := json.Marshal(OfflineUpdate(entityID)); err == nil {
		pipe.Publish(ctx, s.channelKey(entityID), update)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (s *RedisStore) Nearby(ctx context.Context, query NearbyQuery) ([]NearbyEntity, error) {
	if s.disabled() {
		return []NearbyEntity{}, nil
	}
	if query.Limit <= 0 {
		query.Limit = DefaultNearbyLimit
	}
	if query.RadiusKm <= 0 {
		return []NearbyEntity{}, nil
	}

	precision := geo.PickPrecision(query.Center, query.RadiusKm, s.cfg.CellPrecisions, s.cfg.MaxCoverCells)
	cells := geo.CoverRadius(query.Center, query.RadiusKm, precision)
	if len(cells) == 0 {
		return []NearbyEntity{}, nil
	}

	ids, err := s.candidatesIn(ctx, precision, cells)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []NearbyEntity{}, nil
	}

	fixes, missing, err := s.hydrate(ctx, ids)
	if err != nil {
		return nil, err
	}
	// An indexed id with no fix means the fix expired; drop it from the
	// index so the cell does not accumulate ghosts.
	s.evict(ctx, cells, missing)

	results := make([]NearbyEntity, 0, len(fixes))
	for _, fix := range fixes {
		distance := geo.HaversineKm(query.Center, fix.Point())
		if distance > query.RadiusKm {
			continue
		}
		results = append(results, NearbyEntity{Fix: fix, DistanceKm: distance})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].DistanceKm == results[j].DistanceKm {
			return results[i].Fix.EntityID < results[j].Fix.EntityID
		}
		return results[i].DistanceKm < results[j].DistanceKm
	})
	if len(results) > query.Limit {
		results = results[:query.Limit]
	}
	return results, nil
}

// candidatesIn prunes then reads the covered cells in a single pipeline.
//
// The prune is what keeps the index honest: a fix older than the TTL can never
// be returned, however long ago the entity stopped reporting. It runs over
// every configured precision covering the queried area, not just the precision
// being read, so a run of short-radius queries still cleans up the coarse
// index that only wide sweeps consult.
func (s *RedisStore) candidatesIn(ctx context.Context, precision int, cells []geo.Cell) ([]string, error) {
	cutoff := strconv.FormatInt(time.Now().Add(-s.cfg.FixTTL).UnixMilli(), 10)

	pipe := s.client.Pipeline()

	pruned := make(map[string]struct{}, len(cells)*len(s.cfg.CellPrecisions))
	for _, cell := range cells {
		for _, p := range s.cfg.CellPrecisions {
			key := IndexKey(s.cfg.Namespace, p, cell.Parent(p))
			if _, dup := pruned[key]; dup {
				continue
			}
			pruned[key] = struct{}{}
			pipe.ZRemRangeByScore(ctx, key, "-inf", "("+cutoff)
		}
	}

	reads := make([]*redis.StringSliceCmd, len(cells))
	for i, cell := range cells {
		reads[i] = pipe.ZRange(ctx, IndexKey(s.cfg.Namespace, precision, cell), 0, -1)
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}

	seen := make(map[string]struct{}, s.cfg.MaxCandidates)
	ids := make([]string, 0, s.cfg.MaxCandidates)
	for _, read := range reads {
		members, err := read.Result()
		if err != nil && err != redis.Nil {
			return nil, err
		}
		for _, id := range members {
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
			if len(ids) >= s.cfg.MaxCandidates {
				return ids, nil
			}
		}
	}
	return ids, nil
}

// hydrate reads every candidate's fix in one pipeline and reports which ids
// had none.
func (s *RedisStore) hydrate(ctx context.Context, ids []string) (fixes []Fix, missing []string, err error) {
	pipe := s.client.Pipeline()
	reads := make([]*redis.StringCmd, len(ids))
	for i, id := range ids {
		reads[i] = pipe.Get(ctx, FixKey(s.cfg.Namespace, id))
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, nil, err
	}

	fixes = make([]Fix, 0, len(ids))
	for i, read := range reads {
		payload, err := read.Bytes()
		if err != nil {
			if err == redis.Nil {
				missing = append(missing, ids[i])
				continue
			}
			return nil, nil, err
		}
		var fix Fix
		if err := json.Unmarshal(payload, &fix); err != nil {
			missing = append(missing, ids[i])
			continue
		}
		if fix.EntityID == "" {
			fix.EntityID = ids[i]
		}
		fixes = append(fixes, fix)
	}
	return fixes, missing, nil
}

// evict removes ids whose fix has gone from every cell the query touched, at
// every indexed precision. Best-effort: a failure here costs a repeated filter
// next query, nothing more.
func (s *RedisStore) evict(ctx context.Context, cells []geo.Cell, ids []string) {
	if len(ids) == 0 {
		return
	}
	members := make([]any, len(ids))
	for i, id := range ids {
		members[i] = id
	}

	pipe := s.client.Pipeline()
	seen := make(map[string]struct{}, len(cells)*len(s.cfg.CellPrecisions))
	for _, cell := range cells {
		for _, p := range s.cfg.CellPrecisions {
			key := IndexKey(s.cfg.Namespace, p, cell.Parent(p))
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			pipe.ZRem(ctx, key, members...)
		}
	}
	_, _ = pipe.Exec(ctx)
}

// currentCell reads the cell an entity was last indexed into.
func (s *RedisStore) currentCell(ctx context.Context, entityID string) (geo.Cell, error) {
	value, err := s.client.Get(ctx, CellKey(s.cfg.Namespace, entityID)).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", err
	}
	return geo.Cell(value), nil
}

// pipeDeindex queues removal of an entity from every indexed precision of a
// cell. Coarser cells are prefixes of the finest one, so one cell is enough to
// address them all.
func (s *RedisStore) pipeDeindex(ctx context.Context, pipe redis.Pipeliner, entityID string, cell geo.Cell) {
	for _, precision := range s.cfg.CellPrecisions {
		pipe.ZRem(ctx, IndexKey(s.cfg.Namespace, precision, cell.Parent(precision)), entityID)
	}
}

// Subscribe opens a pub/sub subscription to one entity's updates. Services
// bridge the returned channel straight onto a websocket; keeping the key
// format here means a subscriber can never drift from the publisher.
func Subscribe(ctx context.Context, client *redis.Client, namespace, entityID string) *redis.PubSub {
	return client.Subscribe(ctx, ChannelKey(namespace, entityID))
}

// NoopStore satisfies Store without persisting anything, for services running
// without Redis and for tests that do not care about tracking.
type NoopStore struct{}

func (NoopStore) PutFix(context.Context, Fix, bool) error             { return nil }
func (NoopStore) GetFix(context.Context, string) (Fix, bool, error)   { return Fix{}, false, nil }
func (NoopStore) SetDiscoverable(context.Context, string, bool) error { return nil }
func (NoopStore) Remove(context.Context, string) error                { return nil }
func (NoopStore) Nearby(context.Context, NearbyQuery) ([]NearbyEntity, error) {
	return []NearbyEntity{}, nil
}
