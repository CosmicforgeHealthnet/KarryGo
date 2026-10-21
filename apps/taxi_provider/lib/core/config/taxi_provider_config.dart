import 'package:flutter/foundation.dart';

/// Build-time configuration for the taxi driver app.
///
/// Release builds default to the live gateway. Debug builds default to this
/// dev machine's LAN IP so a physical phone on the same Wi-Fi reaches the
/// backend with a plain `flutter run` — the same convention the customer,
/// truck and dispatch apps follow. Override with
/// `--dart-define=TAXI_API_BASE_URL=...` on a different network, or fall back
/// to `adb reverse tcp:8102 tcp:8102` and point at 127.0.0.1.
///
/// The wallet and support addresses follow the taxi one: behind the public
/// gateway every service shares one origin, and in local development each
/// service runs on its own port. Either can be overridden with its own
/// dart-define.
class TaxiProviderConfig {
  const TaxiProviderConfig({
    required this.taxiApiBaseUrl,
    required this.walletApiBaseUrl,
    required this.supportApiBaseUrl,
  });

  /// The taxi service's whole API base. Provider and customer surfaces both
  /// live below this prefix, so it carries no `/provider` segment.
  static const defaultTaxiApiBaseUrl = kReleaseMode
      ? 'https://api-gateway.staging.karryam.com/api/v1/taxi'
      : 'http://192.168.1.112:8102/api/v1/taxi';

  factory TaxiProviderConfig.fromEnvironment() {
    const taxi = String.fromEnvironment(
      'TAXI_API_BASE_URL',
      defaultValue: defaultTaxiApiBaseUrl,
    );
    const wallet = String.fromEnvironment('PAYMENT_WALLET_API_BASE_URL');
    const support = String.fromEnvironment('SUPPORT_API_BASE_URL');
    return TaxiProviderConfig(
      taxiApiBaseUrl: taxi,
      walletApiBaseUrl: wallet.isNotEmpty
          ? wallet
          : siblingBaseUrl(taxi, '/api/v1/payment-wallet', 8105),
      supportApiBaseUrl: support.isNotEmpty
          ? support
          : siblingBaseUrl(taxi, '/api/v1/support-disputes', 8107),
    );
  }

  /// Another service's base next to [taxiBase]: the same origin behind the
  /// gateway, or [devPort] when [taxiBase] targets taxi's own dev port 8102.
  static String siblingBaseUrl(String taxiBase, String path, int devPort) {
    final uri = Uri.parse(taxiBase);
    final port = uri.hasPort && uri.port == 8102 ? devPort : null;
    return uri.replace(port: port, path: path).toString();
  }

  final String taxiApiBaseUrl;
  final String walletApiBaseUrl;
  final String supportApiBaseUrl;
}
