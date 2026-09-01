class CustomerAppConfig {
  const CustomerAppConfig({
    required this.customerApiBaseUrl,
    required this.mediaFileApiBaseUrl,
    required this.mediaFileServiceToken,
    required this.supportApiBaseUrl,
    required this.walletApiBaseUrl,
    required this.haulingApiBaseUrl,
    required this.dispatchApiBaseUrl,
    required this.taxiApiBaseUrl,
    required this.notificationWsUrl,
    required this.googleMapsApiKey,
  });

  // Defaults point at this dev machine's LAN IP so a physical phone on the same
  // Wi-Fi reaches the backend directly, with no `adb reverse` tunnels. Override
  // any of these with --dart-define when the host differs (a different dev
  // machine, a new DHCP lease, or the 127.0.0.1 + adb reverse setup).
  static const defaultCustomerApiBaseUrl = 'http://192.168.1.112:8101/api/v1/customer';
  static const defaultMediaFileApiBaseUrl = 'http://192.168.1.112:8109/api/v1/media-files';
  static const defaultMediaFileServiceToken = 'development-media-token';
  static const defaultSupportApiBaseUrl = 'http://192.168.1.112:8107/api/v1/support-disputes';
  static const defaultWalletApiBaseUrl = 'http://192.168.1.112:8105/api/v1/payment-wallet';
  static const defaultHaulingApiBaseUrl = 'http://192.168.1.112:8104/api/v1/hauling';
  static const defaultDispatchApiBaseUrl = 'http://192.168.1.112:8103/api/v1/customer/dispatch';
  static const defaultTaxiApiBaseUrl = 'http://192.168.1.112:8102/api/v1/taxi';
  // Realtime websocket endpoint on notification-service (ws:// upgrade, token-gated).
  static const defaultNotificationWsUrl = 'ws://192.168.1.112:8106/api/v1/notifications/ws';
  // No default on purpose — a real key must always come from
  // --dart-define=GOOGLE_MAPS_API_KEY=... (never commit one here). PlacesApi
  // already surfaces a friendly MISSING_KEY error when this is empty.
  static const defaultGoogleMapsApiKey = '';

  factory CustomerAppConfig.fromEnvironment() {
    return const CustomerAppConfig(
      customerApiBaseUrl: String.fromEnvironment(
        'CUSTOMER_API_BASE_URL',
        defaultValue: defaultCustomerApiBaseUrl,
      ),
      mediaFileApiBaseUrl: String.fromEnvironment(
        'MEDIA_FILE_API_BASE_URL',
        defaultValue: defaultMediaFileApiBaseUrl,
      ),
      mediaFileServiceToken: String.fromEnvironment(
        'MEDIA_FILE_SERVICE_TOKEN',
        defaultValue: defaultMediaFileServiceToken,
      ),
      supportApiBaseUrl: String.fromEnvironment(
        'SUPPORT_API_BASE_URL',
        defaultValue: defaultSupportApiBaseUrl,
      ),
      walletApiBaseUrl: String.fromEnvironment(
        'WALLET_API_BASE_URL',
        defaultValue: defaultWalletApiBaseUrl,
      ),
      haulingApiBaseUrl: String.fromEnvironment(
        'HAULING_API_BASE_URL',
        defaultValue: defaultHaulingApiBaseUrl,
      ),
      dispatchApiBaseUrl: String.fromEnvironment(
        'DISPATCH_API_BASE_URL',
        defaultValue: defaultDispatchApiBaseUrl,
      ),
      taxiApiBaseUrl: String.fromEnvironment(
        'TAXI_API_BASE_URL',
        defaultValue: defaultTaxiApiBaseUrl,
      ),
      notificationWsUrl: String.fromEnvironment(
        'NOTIFICATION_WS_URL',
        defaultValue: defaultNotificationWsUrl,
      ),
      googleMapsApiKey: String.fromEnvironment(
        'GOOGLE_MAPS_API_KEY',
        defaultValue: defaultGoogleMapsApiKey,
      ),
    );
  }

  final String customerApiBaseUrl;
  final String mediaFileApiBaseUrl;
  final String mediaFileServiceToken;
  final String supportApiBaseUrl;
  final String walletApiBaseUrl;
  final String haulingApiBaseUrl;
  final String dispatchApiBaseUrl;

  /// Note this is the taxi service's whole API base; unlike the dispatch URL
  /// it carries no `/customer` segment, because taxi splits its customer and
  /// provider surfaces below this prefix.
  final String taxiApiBaseUrl;
  final String notificationWsUrl;
  final String googleMapsApiKey;
}
