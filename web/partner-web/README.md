# partner-web

Web front-end for the CosmicForge Healthnet logistics partner portal (business accounts that ship packages via KarryGo's API). Mirrors the mobile onboarding + dashboard flow, built with Next.js App Router.

## Stack
- Next.js 15 (App Router) + TypeScript
- Tailwind CSS v4
- Zustand (onboarding step state)
- Mock data in `src/lib/mock/` — no backend wired yet

## Structure
- `(auth)` — signup, OTP confirmation
- `(onboarding)` — business info, logo upload, profile photo, success
- `(dashboard)` — dashboard, deliveries, webhook logs, API docs, billing, settings

## Setup
```bash
npm install
npm run dev
```
