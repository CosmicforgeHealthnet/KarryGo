// ─────────────────────────────────────────────────────────────────────────────
// AppConfig — single source of truth for the backend base URL.
//
// HOW TO SELECT THE RIGHT URL
// ───────────────────────────
// Always pass --dart-define=DISPATCH_PROVIDER_API_BASE_URL=<url> at run time.
// If no dart-define is set the app falls back to http://localhost:8103, which
// only works on Chrome/Windows.  On a physical Android phone localhost means
// the phone itself, so every API call will fail with "Cannot connect".
//
// ── Chrome / Windows desktop ─────────────────────────────────────────────────
// flutter run -d chrome \
//   --dart-define=DISPATCH_PROVIDER_API_BASE_URL=http://localhost:8103
//
// ── Android emulator ─────────────────────────────────────────────────────────
// flutter run \
//   --dart-define=DISPATCH_PROVIDER_API_BASE_URL=http://10.0.2.2:8103
//
// ── Physical Android over USB (recommended — requires adb reverse) ───────────
// adb reverse tcp:8103 tcp:8103
// adb reverse --list                   ← verify the tunnel is active
// flutter run -d <DEVICE_ID> \
//   --dart-define=DISPATCH_PROVIDER_API_BASE_URL=http://127.0.0.1:8103
//
//   Example device ID: RFCY51N8EJB
//   flutter run -d RFCY51N8EJB \
//     --dart-define=DISPATCH_PROVIDER_API_BASE_URL=http://127.0.0.1:8103
//
//   If you get a "service protocol" error add --disable-dds:
//   flutter run -d RFCY51N8EJB --disable-dds \
//     --dart-define=DISPATCH_PROVIDER_API_BASE_URL=http://127.0.0.1:8103
//
// ── Physical Android over Wi-Fi ──────────────────────────────────────────────
// ipconfig                             ← find your laptop's Wi-Fi IP
// flutter run -d <DEVICE_ID> \
//   --dart-define=DISPATCH_PROVIDER_API_BASE_URL=http://192.168.x.x:8103
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

  /// Fallback when no dart-define is supplied.
  /// Uses 127.0.0.1 (explicit IPv4) so adb-reverse tunnels work on Android
  /// without needing --dart-define. Chrome/Windows also accepts 127.0.0.1.
  static const developmentDefaultApiBaseUrl = 'http://127.0.0.1:8103';

  static const developmentDefaultWalletBaseUrl =
      'http://127.0.0.1:8105/api/v1/payment-wallet';

  /// The resolved backend base URL.  dart-define always wins; falls back to
  /// [developmentDefaultApiBaseUrl] when not set.
  static String get apiBaseUrl {
    final configured = _configuredApiBaseUrl.trim();
    if (configured.isNotEmpty) {
      return _stripTrailingSlashes(configured);
    }
    return developmentDefaultApiBaseUrl;
  }

  /// The resolved wallet service base URL.
  static String get walletBaseUrl {
    final configured = _configuredWalletBaseUrl.trim();
    if (configured.isNotEmpty) return _stripTrailingSlashes(configured);
    return developmentDefaultWalletBaseUrl;
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

  static String _stripTrailingSlashes(String value) {
    var normalized = value.trim();
    while (normalized.endsWith('/') && normalized.length > 1) {
      normalized = normalized.substring(0, normalized.length - 1);
    }
    return normalized;
  }
}
