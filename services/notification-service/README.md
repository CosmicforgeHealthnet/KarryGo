# Notification Service

Owns push, SMS, email, in-app notifications, delivery attempts, and retry
handling for async platform updates.

## Current Implementation

This service is the central notification hub for Karry Go. Other services can
submit the same notification request shape through either:

- `POST /api/v1/notifications/send`
- Redis Stream `notification:requests`

The optional `channels` field controls delivery media. If it is omitted, the
service uses template defaults or falls back to `push`, `websocket`, and
`in_app`.

Supported v1 channels:

- `push`: Firebase Cloud Messaging when Firebase env vars are configured;
  otherwise logs locally.
- `email`: cPanel SMTP when SMTP env vars are configured; otherwise logs
  locally.
- `websocket`: live delivery to connected clients.
- `in_app`: durable notification record.

HTTP service-to-service endpoints use shared HMAC headers from
`shared/go/serviceauth`, not customer/provider bearer tokens.

## Local Bootstrap

Use the helper script from the repo root for a full local setup:

```bash
./scripts/notification-local-bootstrap.sh
```

It starts local Postgres on `5438`, Redis on `6385`, creates the
`notification_service` database if needed, applies the notification migrations,
and prints the command to run the service.

If you prefer to do it manually:

```bash
mkdir -p /tmp/postgres-notification
initdb -D /tmp/postgres-notification
pg_ctl -D /tmp/postgres-notification -o "-p 5438" start

mkdir -p /tmp/redis-6385
redis-server --port 6385 --dir /tmp/redis-6385 --daemonize yes

psql -p 5438 -d template1
```

Inside `psql`:

```sql
CREATE USER cosmicforge_logistics WITH PASSWORD 'cosmicforge_logistics';
CREATE DATABASE notification_service OWNER cosmicforge_logistics;
```

Then apply the migration:

```bash
psql "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5438/notification_service?sslmode=disable" \
  -f services/notification-service/migrations/001_notifications.sql
```
