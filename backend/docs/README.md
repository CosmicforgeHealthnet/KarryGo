# Backend Docs

## Error Response Shape

All API errors should use the same envelope:

```json
{
  "success": false,
  "error": {
    "code": "validation_failed",
    "message": "Check your details.",
    "request_id": "req-id",
    "fields": [
      {"field": "phone", "message": "Phone number is required."}
    ]
  }
}
```

Use `internal/platform/apperrors` to create errors and `internal/platform/httpx` to return them through Gin middleware.

## Redis Cache

Redis is configured through:

```text
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=
REDIS_DB=0
```

Use `cache.Store` for JSON cache reads/writes. Cache keys should be namespaced by feature, for example:

```text
user:{id}:profile
booking:{id}:summary
provider:{id}:location
```

## Background Jobs

Background work uses Redis through Asynq:

- `booking:expire_stale`: cleanup stale pending bookings
- `payout:release`: daily payout release process
- `notification:send`: push/SMS/email notification work

Cron schedules live in `internal/platform/jobs/scheduler.go`. Job handlers live in `internal/platform/jobs/worker.go`.