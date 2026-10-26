package adminevents

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/outbox"
)

// OutboxTable is the conventional owner table name. Each owner creates it in
// its own migration with the shared/go/outbox shape (see OutboxSchema).
const OutboxTable = "admin_event_outbox"

// IngressPath is the Bridge's private owner-event ingress (never public).
const IngressPath = "/internal/karryam-admin-events/v1/events"

// OutboxSchema is embedded verbatim by owner migrations.
const OutboxSchema = `CREATE TABLE IF NOT EXISTS admin_event_outbox (
 id text PRIMARY KEY,
 event_type text NOT NULL,
 schema_version int NOT NULL CHECK (schema_version > 0),
 aggregate_type text NOT NULL,
 aggregate_id text NOT NULL,
 payload jsonb NOT NULL,
 status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','PUBLISHED','DEAD_LETTERED')),
 available_at timestamptz NOT NULL DEFAULT now(),
 attempts int NOT NULL DEFAULT 0 CHECK (attempts >= 0),
 lease_owner text,
 lease_expires_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 published_at timestamptz,
 last_error_redacted text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS admin_event_outbox_due_idx ON admin_event_outbox (status, available_at) WHERE status = 'PENDING';`

// Emit appends a validated event inside the caller's business transaction.
// The stored payload is the exact SourceEvent bytes delivered to the Bridge.
func Emit(ctx context.Context, tx outbox.Querier, ev SourceEvent) error {
	if err := ev.Validate(); err != nil {
		return err
	}
	if _, ok := Lookup(ev.SourceService, ev.EventType); !ok {
		return ErrUnknown
	}
	raw, err := json.Marshal(ev)
	if err != nil || len(raw) > MaxEventBytes {
		return ErrInvalid
	}
	_, err = tx.Exec(ctx, `INSERT INTO `+OutboxTable+` (id,event_type,schema_version,aggregate_type,aggregate_id,payload,status,available_at,attempts,created_at)
VALUES ($1,$2,1,$3,$4,$5,'PENDING',now(),0,now()) ON CONFLICT (id) DO NOTHING`, ev.EventID, ev.EventType, ev.EntityType, ev.EntityRef, raw)
	return err
}

// rawPublisher delivers the stored SourceEvent bytes (not an outbox envelope).
type rawPublisher struct{ inner *outbox.HTTPPublisher }

func (p rawPublisher) Publish(ctx context.Context, r outbox.Record) error {
	return p.inner.Publish(ctx, r)
}

// RelayFromEnvironment builds the owner relay worker. Default disabled: with
// <PREFIX>_ADMIN_EVENTS_ENABLED unset nothing is read and nil is returned.
// The relay signs as the owner's own service name with a dedicated secret.
func RelayFromEnvironment(db outbox.Querier, prefix, serviceName string) (*outbox.Worker, error) {
	if os.Getenv(prefix+"_ADMIN_EVENTS_ENABLED") != "true" {
		return nil, nil
	}
	origin := os.Getenv(prefix + "_ADMIN_EVENTS_BRIDGE_ORIGIN")
	raw, err := os.ReadFile(os.Getenv(prefix + "_ADMIN_EVENTS_SECRET_FILE"))
	if origin == "" || err != nil || len(raw) > 4096 {
		return nil, errors.New("admin event relay requires a fixed Bridge origin and dedicated secret file")
	}
	secret, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(secret) < 32 {
		return nil, errors.New("invalid admin event relay secret")
	}
	store, err := outbox.NewStore(db, OutboxTable)
	if err != nil {
		return nil, err
	}
	pub, err := outbox.NewHTTPPublisher(origin, IngressPath, serviceName, secret, 5*time.Second)
	if err != nil {
		return nil, err
	}
	return outbox.NewWorker(store, rawPublisher{pub}, outbox.WorkerConfig{}), nil
}
