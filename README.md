# Cosmicforge Logistics

Cosmicforge Logistics is a multi-application logistics platform with four mobile apps, one admin web app, and a microservice backend.

## Purpose

Cosmicforge Logistics is designed to manage the full logistics journey in one connected platform. It brings customers, providers, and internal operations into a shared ecosystem so people can move between onboarding, booking, fulfillment, payment, support, and reporting without switching to separate systems.

At the product level, the application is meant to:

- give customers a single place to request rides, package delivery, and hauling services
- give taxi, dispatch, and truck providers focused apps for receiving and completing jobs
- support authentication, profiles, wallets, notifications, media uploads, and compliance workflows
- help operations teams handle disputes, moderation, verification, and performance tracking
- keep the codebase modular so each logistics line can grow independently without rewriting the whole platform

## Apps

- apps/customer: customer mobile app
- apps/taxi_provider: car/taxi provider mobile app
- apps/dispatch_provider: dispatch provider mobile app
- apps/truck_provider: truck provider mobile app
- apps/admin: admin web application

## Shared Flutter Packages

- packages/ui_kit: reusable Cosmicforge Logistics UI components and theme
- packages/api_core: shared API foundation for Flutter apps

## Backend Services

The active backend lives under `services/`.
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

Shared Go platform code lives in `shared/go`, including the standard Cosmicforge Logistics
API error envelope and HTTP middleware. See `docs/microservices-architecture.md`
for the service boundaries and local development notes.
