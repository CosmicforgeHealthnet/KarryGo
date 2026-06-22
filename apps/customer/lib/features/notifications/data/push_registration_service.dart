import 'dart:io' show Platform;

import 'package:flutter/foundation.dart';

import 'notification_api.dart';

/// Obtains an FCM device token and registers it with notification-service (via
/// the customer-service proxy) so background push reaches this device.
///
/// Firebase is integrated through the injected [tokenProvider] seam rather than
/// a hard dependency, so the app builds and the in-app/websocket channels work
/// even before the Firebase project config (`google-services.json` /
/// `GoogleService-Info.plist`) is added. Once `flutterfire configure` has been
/// run and `firebase_messaging` added, pass a provider that returns
/// `FirebaseMessaging.instance.getToken()` (after requesting permission) and
/// register `onTokenRefresh`.
class PushRegistrationService {
  PushRegistrationService({
    required NotificationApi api,
    required Future<String?> Function() tokenProvider,
  }) : _api = api,
       _tokenProvider = tokenProvider;

  final NotificationApi _api;
  final Future<String?> Function() _tokenProvider;

  /// Fetches the device token (if available) and registers it. Safe no-op when
  /// no token is available (e.g. Firebase not configured, permission denied).
  Future<void> register({required String accessToken}) async {
    try {
      final token = await _tokenProvider();
      if (token == null || token.isEmpty) return;
      await _api.registerDevice(
        accessToken: accessToken,
        token: token,
        platform: _platform(),
      );
    } catch (error) {
      if (kDebugMode) debugPrint('push registration error: $error');
    }
  }

  String _platform() {
    if (kIsWeb) return 'web';
    try {
      if (Platform.isIOS) return 'ios';
      if (Platform.isAndroid) return 'android';
    } catch (_) {}
    return 'unknown';
  }
}
