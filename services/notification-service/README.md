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
