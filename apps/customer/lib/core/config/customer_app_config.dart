class CustomerAppConfig {
  const CustomerAppConfig({
    required this.customerApiBaseUrl,
    required this.mediaFileApiBaseUrl,
    required this.mediaFileServiceToken,
    required this.supportApiBaseUrl,
    required this.walletApiBaseUrl,
    required this.haulingApiBaseUrl,
    required this.dispatchApiBaseUrl,
    required this.notificationWsUrl,
    required this.googleMapsApiKey,
  });

  static const defaultCustomerApiBaseUrl = 'http://127.0.0.1:8101/api/v1/customer';
  static const defaultMediaFileApiBaseUrl = 'http://127.0.0.1:8109/api/v1/media-files';
  static const defaultMediaFileServiceToken = 'development-media-token';
  static const defaultSupportApiBaseUrl = 'http://127.0.0.1:8107/api/v1/support-disputes';
  static const defaultWalletApiBaseUrl = 'http://127.0.0.1:8105/api/v1/payment-wallet';
  static const defaultHaulingApiBaseUrl = 'http://127.0.0.1:8104/api/v1/hauling';
  static const defaultDispatchApiBaseUrl = 'http://127.0.0.1:8103/api/v1/customer/dispatch';
  // Realtime websocket endpoint on notification-service (ws:// upgrade, token-gated).
  static const defaultNotificationWsUrl = 'ws://127.0.0.1:8106/api/v1/notifications/ws';
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
  final String notificationWsUrl;
  final String googleMapsApiKey;
}
