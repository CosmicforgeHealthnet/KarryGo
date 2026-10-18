// Package adminread defines minimized owner snapshots for the private HQ bridge.
// It contains no transport proxy or domain mutation API.
package adminread

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

const TaxiPath = "/internal/taxi/v1/admin/rides"
const TaxiPermission = "taxi.rides.read"
const Caller = "admin-backoffice-service"

var ErrQuery = errors.New("invalid bounded admin query")
var opaque = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,127}$`)

func Opaque(s string) bool { return opaque.MatchString(s) }

type Scope struct {
	Market       string `json:"market"`
	Zone         string `json:"zone"`
	Organization string `json:"organization"`
}
type Query struct {
	Scope   Scope     `json:"scope"`
	Staff   string    `json:"staff"`
	Purpose string    `json:"purpose"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Limit   int       `json:"limit"`
	Cursor  string    `json:"-"`
}

func (q Query) Validate(now time.Time) error {
	if !Opaque(q.Scope.Market) || !Opaque(q.Scope.Zone) || !Opaque(q.Scope.Organization) || !Opaque(q.Staff) || !Opaque(q.Purpose) || q.Start.IsZero() || !q.End.After(q.Start) || q.End.Sub(q.Start) > 31*24*time.Hour || q.End.After(now.Add(time.Minute)) || q.Limit < 1 || q.Limit > 100 || len(q.Cursor) > 4096 {
		return ErrQuery
	}
	return nil
}
func (q Query) Values() url.Values {
	v := url.Values{"market": {q.Scope.Market}, "zone": {q.Scope.Zone}, "organization": {q.Scope.Organization}, "staff": {q.Staff}, "purpose": {q.Purpose}, "start": {q.Start.UTC().Format(time.RFC3339Nano)}, "end": {q.End.UTC().Format(time.RFC3339Nano)}, "limit": {strconv.Itoa(q.Limit)}}
	if q.Cursor != "" {
		v.Set("cursor", q.Cursor)
	}
	return v
}
func Parse(v url.Values, now time.Time) (Query, error) {
	q := Query{Scope: Scope{v.Get("market"), v.Get("zone"), v.Get("organization")}, Staff: v.Get("staff"), Purpose: v.Get("purpose"), Cursor: v.Get("cursor")}
	for k, vals := range v {
		if len(vals) != 1 {
			return q, ErrQuery
		}
		switch k {
		case "market", "zone", "organization", "staff", "purpose", "start", "end", "limit", "cursor":
		default:
			return q, ErrQuery
		}
	}
	var err error
	if q.Start, err = time.Parse(time.RFC3339Nano, v.Get("start")); err != nil {
		return q, ErrQuery
	}
	if q.End, err = time.Parse(time.RFC3339Nano, v.Get("end")); err != nil {
		return q, ErrQuery
	}
	if q.Limit, err = strconv.Atoi(v.Get("limit")); err != nil {
		return q, ErrQuery
	}
	q.Start = q.Start.UTC()
	q.End = q.End.UTC()
	return q, q.Validate(now)
}

// SourceVersion is deliberately nullable: Taxi's current source_version is a
// Fleet contract counter, not a revision of every lifecycle write. UpdatedAt
// is the authoritative row timestamp; neither is a safe command precondition.
type Ride struct {
	ID            string    `json:"id"`
	Status        string    `json:"status"`
	Mode          string    `json:"mode"`
	SourceContext string    `json:"source_context"`
	SourceVersion *int64    `json:"source_version"`
	UpdatedAt     time.Time `json:"updated_at"`
	CreatedAt     time.Time `json:"created_at"`
}
type Event struct {
	ID   string    `json:"id"`
	Type string    `json:"type"`
	At   time.Time `json:"at"`
}
type TaxiSnapshot struct {
	ContractVersion  string    `json:"contract_version"`
	Scope            Scope     `json:"scope"`
	AsOf             time.Time `json:"as_of"`
	Freshness        string    `json:"freshness"`
	Coverage         string    `json:"coverage"`
	Items            []Ride    `json:"items"`
	Events           []Event   `json:"events"`
	HistoryTruncated bool      `json:"history_truncated"`
	NextCursor       string    `json:"next_cursor,omitempty"`
}
