package adminevents

import "time"

// SnapshotPath is each owner's HMAC-protected backfill route suffix. A page is
// a stable, id-ordered view of current entity state bounded by Watermark; the
// Bridge buffers incremental events and applies them after the snapshot, with
// version fencing making overlap harmless.
const SnapshotContract = "karryam.owner-admin-snapshot.v1"

type SnapshotPage struct {
	Contract   string        `json:"contract"`
	Source     string        `json:"source_service"`
	EntityType string        `json:"entity_type"`
	Watermark  time.Time     `json:"watermark"`
	Events     []SourceEvent `json:"events"`
	NextCursor string        `json:"next_cursor"`
	Complete   bool          `json:"complete"`
}

// Validate checks the page and every event (adapter-registered, same source).
func (p SnapshotPage) Validate(source string) error {
	if p.Contract != SnapshotContract || p.Source != source || p.Watermark.IsZero() || len(p.Events) > 200 || (p.NextCursor != "" && !Opaque(p.NextCursor)) || p.Complete != (p.NextCursor == "") {
		return ErrInvalid
	}
	for _, ev := range p.Events {
		if ev.SourceService != source || ev.EntityType != p.EntityType || ev.Validate() != nil {
			return ErrInvalid
		}
		if _, ok := Lookup(ev.SourceService, ev.EventType); !ok {
			return ErrUnknown
		}
	}
	return nil
}
