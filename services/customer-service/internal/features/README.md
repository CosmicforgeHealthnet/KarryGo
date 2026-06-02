# Customer Service Feature Structure

Customer-service is organized feature-first under `internal/features`.

Use this pattern for new customer-service features:

```text
features/
  feature_name/
    http/          # routes, handlers, request/response DTOs
    usecases/      # business workflows
    models/        # feature-owned data structures
    repositories/  # database or Redis persistence
    clients/       # outgoing integrations, when needed
```

Current features:

- `auth`: phone OTP login, refresh sessions, logout, and authenticated
  `/me` access.
- `profile`: customer profile models and Postgres repository.

Keep shared service infrastructure outside feature folders:

- `config`
- `database`
- migrations

Keep reusable cross-service helpers in `shared/go`, such as
`shared/go/phonenumber`.

This keeps each customer feature easy to assign, review, and extend without
mixing unrelated files in one folder.
