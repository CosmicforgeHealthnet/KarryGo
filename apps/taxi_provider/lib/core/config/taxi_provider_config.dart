/// Build-time configuration for the taxi driver app.
///
/// Defaults point at this dev machine's LAN IP so a physical phone on the
/// same Wi-Fi reaches the backend with a plain `flutter run` — the same
/// convention the customer, truck and dispatch apps follow. Override with
/// `--dart-define=TAXI_API_BASE_URL=...` on a different network, or fall back
/// to `adb reverse tcp:8102 tcp:8102` and point at 127.0.0.1.
class TaxiProviderConfig {
  const TaxiProviderConfig({required this.taxiApiBaseUrl});

  /// The taxi service's whole API base. Provider and customer surfaces both
  /// live below this prefix, so it carries no `/provider` segment.
  static const defaultTaxiApiBaseUrl = 'http://192.168.1.112:8102/api/v1/taxi';

  factory TaxiProviderConfig.fromEnvironment() {
    return const TaxiProviderConfig(
      taxiApiBaseUrl: String.fromEnvironment(
        'TAXI_API_BASE_URL',
        defaultValue: defaultTaxiApiBaseUrl,
      ),
    );
  }

  final String taxiApiBaseUrl;
}
