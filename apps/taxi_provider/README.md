# taxi_provider

A new Flutter project.

## Getting Started

This project is a starting point for a Flutter application.

A few resources to get you started if this is your first Flutter project:

- [Learn Flutter](https://docs.flutter.dev/get-started/learn-flutter)
- [Write your first Flutter app](https://docs.flutter.dev/get-started/codelab)
- [Flutter learning resources](https://docs.flutter.dev/reference/learning-resources)

For help getting started with Flutter development, view the
[online documentation](https://docs.flutter.dev/), which offers tutorials,
samples, guidance on mobile development, and a full API reference.

## Karryam driver UI

The taxi app uses its own Inter typography and navy/teal semantic theme. Profile → App settings → Appearance offers Light, Dark, and System. The choice applies immediately and persists locally; System follows the device setting. Home opens on a map with a draggable availability/request sheet; profile displays the real driver and vehicle fields. UI animations honor the system reduced-motion setting. Other apps and the shared UI kit are unchanged.

### Native Google Maps setup

The Maps and Firebase plugins require iOS 15.5 or later; the Runner deployment target is set accordingly.

The map shows automatically when the build carries a native Maps key (the app asks the platform at startup). Without one, the UI displays an explicit unavailable state and keeps trip addresses and actions usable.

- Android: add `GOOGLE_MAPS_API_KEY=<restricted Android Maps SDK key>` to the ignored `android/local.properties`. Restrict the key to the taxi app package and signing certificate.
- iOS: define the Xcode build setting `GOOGLE_MAPS_API_KEY` for Runner in your local/CI configuration. `Info.plist` supplies it to Google Maps initialization. Use a key restricted to the Runner bundle identifier with the Maps SDK for iOS enabled.
- No dart-define is needed. `--dart-define=GOOGLE_MAPS_ENABLED=false` forces the placeholder, `=true` skips the check. Do not commit credentials.

The map shows real tiles and the booking's pickup/drop-off markers. Location is used only when already authorized; availability retains its existing permission flow. The initial city overview is Lagos, not an assertion of the driver's actual location. No invented road route, turn-by-turn directions, or live ETA is drawn: the current taxi API exposes stops and trip estimates, not a routed polyline. Web/desktop builds show the unavailable state.

### Verification

Run `flutter analyze` and `flutter test` from this directory. The redesign tests cover light/dark layout at 360px with enlarged text, real pending/suspended statuses, map fallback, parcel verification, and cancellation visibility. Authentication/API regression tests remain in place. Test real Maps tiles, camera permissions, and location behavior on devices with native keys before release.

The browser design prototype remains available under `design/`; its fixtures are not used in production Flutter code.

Latest local verification: analyzer clean, 17 tests passed, Android debug APK built successfully. The APK is generated at `build/app/outputs/flutter-apk/app-debug.apk`. Native iOS compilation and device verification with real Maps credentials remain to be performed.
