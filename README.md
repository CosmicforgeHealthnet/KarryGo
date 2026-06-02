# KarryGo

KarryGo is a multi-application logistics platform with four mobile apps, one admin web app, and one shared backend.

## Apps

- apps/customer: customer mobile app
- apps/taxi_provider: car/taxi provider mobile app
- apps/dispatch_provider: dispatch provider mobile app
- apps/truck_provider: truck provider mobile app
- apps/admin: admin web application

## Shared Flutter Packages

- packages/ui_kit: reusable KarryGo UI components and theme
- packages/api_core: shared API foundation for Flutter apps

## Backend

- backend: current Go API server, migrations, Docker setup, Nginx notes, and scripts

## Microservice Scaffold

The repo now includes a microservice-ready backend layout under `services/`.
The main business services are:

- services/customer-service: customer profiles, saved locations, preferences, and customer-facing request history
- services/taxi-service: taxi providers, taxi bookings, taxi matching, and taxi trip lifecycle
- services/dispatch-delivery-service: dispatch riders, package delivery bookings, rider matching, and proof of delivery
- services/hauling-service: truck providers, haulage bookings, truck matching, and cargo workflow

Shared platform services live alongside them:

- services/api-gateway
- services/payment-wallet-service
- services/notification-service
- services/support-dispute-service
- services/verification-compliance-service
- services/media-file-service
- services/admin-backoffice-service
- services/analytics-service

Shared Go platform code lives in `shared/go`, including the standard KarryGo
API error envelope and HTTP middleware. See `docs/microservices-architecture.md`
for the service boundaries and local development notes.
