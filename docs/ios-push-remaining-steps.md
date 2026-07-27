# iOS Push Notifications — Remaining Manual Steps

Status: everything on the code/config side is DONE. iOS push cannot deliver
until the Apple-account steps below are completed. Estimated effort once an
Apple Developer account exists: ~10 minutes.

## Already done (this repo)

- Both iOS apps are registered in Firebase project `logistics-93b9f`:
  - Customer: `com.cosmicforge.logistics.customer` → app id `1:461709902538:ios:72471c4f0f275502876a33`
  - Truck provider: `com.cosmicforge.logistics.truckprovider` → app id `1:461709902538:ios:4988156b53cad8e7876a33`
- `ios/Runner/GoogleService-Info.plist` downloaded into both apps and added to
  the Xcode project (file reference + Resources build phase).
- `lib/firebase_options.dart` in both apps now has the iOS options
  (`DefaultFirebaseOptions.ios`); the `TargetPlatform.iOS` case no longer throws.
- `ios/Runner/Runner.entitlements` created in both apps with
  `aps-environment = development`, wired via `CODE_SIGN_ENTITLEMENTS` on all
  Runner build configurations.
- `ios/Runner/Info.plist` in both apps: `UIBackgroundModes` includes
  `remote-notification`.
- The Flutter push helper (`features/notifications/data/firebase_push.dart` in
  both apps) is platform-neutral: it initializes Firebase, requests notification
  permission, and registers the device token — no Android-only guards.
- Backend needs no change: notification-service sends via FCM v1, which routes
  to APNs automatically once the APNs key is uploaded.

## Remaining steps (need an Apple Developer account, $99/yr)

1. **Create/join an Apple Developer account** at
   https://developer.apple.com/programs/enroll/ (organization enrollment needs
   a D-U-N-S number; individual enrollment is faster).

2. **Register the two bundle IDs** in the Apple Developer portal
   (Certificates, Identifiers & Profiles → Identifiers → +):
   - `com.cosmicforge.logistics.customer`
   - `com.cosmicforge.logistics.truckprovider`
   Enable the **Push Notifications** capability on each.

3. **Create an APNs auth key** (Keys → + → check "Apple Push Notifications
   service (APNs)" → download the `.p8` file — downloadable only once, store it
   safely, e.g. next to `credentials/firebase.json`). Note the **Key ID** and
   your **Team ID** (top-right of the portal).

4. **Upload the APNs key to Firebase** (one upload covers both apps):
   Firebase console → project `logistics-93b9f` → Project settings → Cloud
   Messaging → select each iOS app → "APNs Authentication Key" → Upload the
   `.p8` with its Key ID + Team ID.

5. **Set the signing team in Xcode** for each app: open
   `ios/Runner.xcworkspace`, target Runner → Signing & Capabilities → select the
   team. Verify the "Push Notifications" and "Background Modes → Remote
   notifications" capabilities show up (they are pre-wired by the entitlements
   file and Info.plist).

6. **Test on a physical iPhone** (simulators cannot receive APNs pushes on
   Intel Macs; Apple-silicon simulators on macOS 13+ can, but a device is the
   real check):
   ```bash
   cd apps/truck_provider
   flutter run --dart-define=HAULING_API_BASE_URL=http://<host>:8104/api/v1/hauling
   ```
   Log in, accept the notification permission prompt, background the app, then
   create a booking from the customer app — a notification should appear in the
   iOS notification tray.

7. **Before App Store release**: switch `aps-environment` in both
   `ios/Runner/Runner.entitlements` from `development` to `production` (or rely
   on Xcode's archive step, which rewrites it automatically for distribution
   builds).

## Notes

- The `GoogleService-Info.plist` / `firebase_options.dart` values are client
  identifiers, not secrets — safe to commit.
- Media uploads and all other Firebase usage keep working regardless of these
  steps; only *push delivery to iOS devices* is gated on the APNs key.
