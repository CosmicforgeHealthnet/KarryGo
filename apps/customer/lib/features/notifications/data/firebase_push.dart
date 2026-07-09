import 'package:firebase_core/firebase_core.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter/foundation.dart';

import '../../../firebase_options.dart';

/// Thin, failure-tolerant wrapper around Firebase Messaging.
///
/// Every entry point degrades to a no-op when Firebase is unavailable on this
/// platform (no config yet, permission denied, no Play services) so the app
/// keeps working on websocket + polling alone. Background delivery needs no
/// Dart handler: the backend sends a `notification` payload, which the OS
/// renders in the system tray while the app is backgrounded or terminated.
class FirebasePush {
  static bool _initialized = false;

  /// Initializes Firebase once. Returns false when this platform has no
  /// Firebase config (e.g. iOS before the app is registered).
  static Future<bool> ensureInitialized() async {
    if (_initialized) return true;
    try {
      await Firebase.initializeApp(options: DefaultFirebaseOptions.currentPlatform);
      _initialized = true;
      return true;
    } catch (error) {
      if (kDebugMode) debugPrint('firebase init skipped: $error');
      return false;
    }
  }

  /// Requests notification permission and returns the FCM device token, or
  /// null when push is unavailable.
  static Future<String?> deviceToken() async {
    if (!await ensureInitialized()) return null;
    try {
      final messaging = FirebaseMessaging.instance;
      final settings = await messaging.requestPermission();
      if (settings.authorizationStatus == AuthorizationStatus.denied) return null;
      return await messaging.getToken();
    } catch (error) {
      if (kDebugMode) debugPrint('fcm token unavailable: $error');
      return null;
    }
  }

  /// Token rotations — re-register the device on each. Empty when unavailable.
  static Stream<String> tokenRefresh() async* {
    if (!await ensureInitialized()) return;
    yield* FirebaseMessaging.instance.onTokenRefresh;
  }

  /// Messages received while the app is in the foreground (the OS shows no
  /// tray notification for these). Empty when unavailable.
  static Stream<RemoteMessage> foregroundMessages() async* {
    if (!await ensureInitialized()) return;
    yield* FirebaseMessaging.onMessage;
  }
}
