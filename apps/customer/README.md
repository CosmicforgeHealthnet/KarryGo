# Cosmicforge Logistics Customer App

Flutter customer app for phone-based onboarding and authentication.

## Auth Flow

- Splash checks for a saved session.
- Onboarding introduces ride booking, package delivery, and truck hauling.
- Phone entry calls `POST /api/v1/customer/auth/start`.
- OTP verification calls `POST /api/v1/customer/auth/verify`.
- Saved sessions are validated with `GET /api/v1/customer/me`.
- Expired access tokens are refreshed with `POST /api/v1/customer/auth/refresh`.
- Logout calls `POST /api/v1/customer/auth/logout` and clears local auth state.

The first profile-completion step is frontend-only until the customer profile update API exists.

## API Base URL

The app reads the customer API base URL from `CUSTOMER_API_BASE_URL`.

Default:

```bash
http://localhost:8101/api/v1/customer
```

Desktop, web, or iOS simulator:

```bash
flutter run --dart-define=CUSTOMER_API_BASE_URL=http://localhost:8101/api/v1/customer
```

Android emulator:

```bash
flutter run --dart-define=CUSTOMER_API_BASE_URL=http://10.0.2.2:8101/api/v1/customer
```

## Local Checks

```bash
flutter pub get
flutter analyze
flutter test
```
