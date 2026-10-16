import 'package:flutter/foundation.dart';

// ─────────────────────────────────────────────────────────────────────────────
// AppConfig — single source of truth for the backend base URL.
//
// HOW TO SELECT THE RIGHT URL
// ───────────────────────────
// The defaults below point at this dev machine's LAN IP (192.168.1.112), so a
// physical Android phone on the same Wi-Fi reaches the backend directly with a
// plain `flutter run` — no adb reverse tunnels. macOS may prompt once to allow
// incoming connections; you must Allow.
//
// Pass --dart-define=DISPATCH_PROVIDER_API_BASE_URL=<url> whenever the host
// differs — a different dev machine, a new DHCP lease, an emulator, or the
// loopback + adb reverse setup.
//
// ── Physical Android over Wi-Fi (default — no flags needed) ──────────────────
// flutter run -d <DEVICE_ID>
//
//   If your LAN IP has changed:
//   ipconfig getifaddr en0             ← find your Mac's Wi-Fi IP
//   flutter run -d <DEVICE_ID> \
//     --dart-define=DISPATCH_PROVIDER_API_BASE_URL=http://192.168.x.x:8103
//
// ── Physical Android over USB (fallback — requires adb reverse) ──────────────
// bash scripts/adb-reverse.sh          ← sets up every port this app needs
// adb reverse --list                   ← verify the tunnels are active
// flutter run -d <DEVICE_ID> \
//   --dart-define=DISPATCH_PROVIDER_API_BASE_URL=http://127.0.0.1:8103
//
//   If you get a "service protocol" error add --disable-dds.
//
// ── Chrome / desktop ─────────────────────────────────────────────────────────
// flutter run -d chrome \
//   --dart-define=DISPATCH_PROVIDER_API_BASE_URL=http://localhost:8103
//
// ── Android emulator ─────────────────────────────────────────────────────────
// flutter run \
//   --dart-define=DISPATCH_PROVIDER_API_BASE_URL=http://10.0.2.2:8103
//
// ── Production / VPS ─────────────────────────────────────────────────────────
// flutter run \
//   --dart-define=DISPATCH_PROVIDER_API_BASE_URL=https://api.yourdomain.com
// ─────────────────────────────────────────────────────────────────────────────

class AppConfig {
  static const _configuredApiBaseUrl = String.fromEnvironment(
    'DISPATCH_PROVIDER_API_BASE_URL',
  );

  static const _configuredWalletBaseUrl = String.fromEnvironment(
    'DISPATCH_PROVIDER_WALLET_BASE_URL',
  );

  static const _configuredNotificationWsUrl = String.fromEnvironment(
    'NOTIFICATION_WS_URL',
  );

  static const _configuredSupportBaseUrl = String.fromEnvironment(
    'SUPPORT_API_BASE_URL',
  );

  /// Fallback when no dart-define is supplied.
  /// This dev machine's LAN IP, so a physical Android phone on the same Wi-Fi
  /// reaches the backend without adb-reverse tunnels. Note this is a bare
  /// origin — callers append their own /api/v1/... paths.
  static const developmentDefaultApiBaseUrl = 'http://192.168.1.112:8103';

  static const developmentDefaultWalletBaseUrl =
      'http://192.168.1.112:8105/api/v1/payment-wallet';

  /// Notification-service websocket for realtime pushes.
  static const developmentDefaultNotificationWsUrl =
      'ws://192.168.1.112:8106/api/v1/notifications/ws';

  /// support-dispute-service (disputes + live chat).
  static const developmentDefaultSupportBaseUrl =
      'http://192.168.1.112:8107/api/v1/support-disputes';

  /// The resolved backend base URL.  dart-define always wins; falls back to
  /// [developmentDefaultApiBaseUrl] when not set.
  static String get apiBaseUrl {
    final configured = _configuredApiBaseUrl.trim();
    return _resolvedUrl(configured, developmentDefaultApiBaseUrl,
        'DISPATCH_PROVIDER_API_BASE_URL');
  }

  /// The resolved wallet service base URL.
  static String get walletBaseUrl {
    final configured = _configuredWalletBaseUrl.trim();
    return _resolvedUrl(configured, developmentDefaultWalletBaseUrl,
        'DISPATCH_PROVIDER_WALLET_BASE_URL');
  }

  /// The resolved notification websocket URL (realtime fast path).
  static String get notificationWsUrl {
    final configured = _configuredNotificationWsUrl.trim();
    return _resolvedUrl(configured, developmentDefaultNotificationWsUrl,
        'NOTIFICATION_WS_URL', websocket: true);
  }

  /// The resolved support-dispute-service base URL.
  static String get supportBaseUrl {
    final configured = _configuredSupportBaseUrl.trim();
    return _resolvedUrl(configured, developmentDefaultSupportBaseUrl,
        'SUPPORT_API_BASE_URL');
  }

  /// Short human-readable label describing which backend mode is active.
  /// Used only for debug logging — never shown to end users.
  static String get backendModeHint {
    final url = apiBaseUrl;
    if (url.contains('localhost')) {
      return 'desktop-localhost '
          '⚠️  NOT reachable on physical Android without adb reverse';
    }
    if (url.contains('127.0.0.1')) {
      return 'physical-phone-usb (adb reverse tcp:8103 tcp:8103 required)';
    }
    if (url.contains('10.0.2.2')) return 'android-emulator';
    if (url.startsWith('https://')) {
      return 'production/vps (${Uri.tryParse(url)?.host ?? url})';
    }
    if (RegExp(r'(192\.168|10\.\d+\.\d+)\.\d+').hasMatch(url)) {
      return 'physical-phone-wifi (lan ip — may change if DHCP)';
    }
    return 'custom ($url)';
  }

  /// True when the resolved URL is a loopback address (localhost or 127.0.0.1).
  /// On a physical Android device this means the phone's own loopback — the
  /// laptop's backend is NOT reachable unless `adb reverse tcp:8103 tcp:8103`
  /// is active.
  static bool get isLocalhostUrl {
    final url = apiBaseUrl;
    return url.contains('localhost') || url.contains('127.0.0.1');
  }

  /// A distributable app must never silently connect to a developer's LAN.
  /// Local HTTP remains available for debug/test builds only.
  static String _resolvedUrl(
    String configured,
    String developmentFallback,
    String environmentKey, {
    bool websocket = false,
  }) {
    if (kReleaseMode && configured.isEmpty) {
      throw StateError('$environmentKey must be supplied for release builds.');
    }
    final value = _stripTrailingSlashes(
      configured.isEmpty ? developmentFallback : configured,
    );
    if (kReleaseMode && !_isSecurePublicUrl(value, websocket: websocket)) {
      throw StateError('$environmentKey must use a public secure endpoint.');
    }
    return value;
  }

  static bool _isSecurePublicUrl(String raw, {bool websocket = false}) {
    final uri = Uri.tryParse(raw);
    if (uri == null ||
        uri.scheme != (websocket ? 'wss' : 'https') ||
        uri.host.isEmpty ||
        uri.userInfo.isNotEmpty ||
        uri.hasFragment) {
      return false;
    }
    final host = uri.host.toLowerCase();
    if (host == 'localhost' ||
        host.endsWith('.localhost') ||
        host.endsWith('.local') ||
        host == '::1' ||
        host.contains(':') ||
        RegExp(r'^[0-9.]+$').hasMatch(host)) {
      // Require a DNS hostname rather than a private or literal IP address.
      return false;
    }
    return true;
  }

  static String _stripTrailingSlashes(String value) {
    var normalized = value.trim();
    while (normalized.endsWith('/') && normalized.length > 1) {
      normalized = normalized.substring(0, normalized.length - 1);
    }
    return normalized;
  }
}
