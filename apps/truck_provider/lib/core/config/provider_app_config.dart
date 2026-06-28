class ProviderAppConfig {
  const ProviderAppConfig({
    required this.haulingApiBaseUrl,
    required this.mediaFileApiBaseUrl,
    required this.mediaFileServiceToken,
    required this.notificationWsUrl,
    required this.paymentWalletApiBaseUrl,
    required this.supportApiBaseUrl,
  });

  static const defaultHaulingApiBaseUrl = 'http://192.168.1.138:8104/api/v1/hauling';
  static const defaultMediaFileApiBaseUrl = 'http://192.168.1.138:8109/api/v1/media-files';
  static const defaultMediaFileServiceToken = 'development-media-token';
  // Realtime websocket endpoint on notification-service (ws:// upgrade, token-gated).
  static const defaultNotificationWsUrl = 'ws://192.168.1.138:8106/api/v1/notifications/ws';
  // payment-wallet-service provider surface (balance, bank accounts, withdrawals).
  // The provider's hauling bearer token is accepted here (service="hauling").
  static const defaultPaymentWalletApiBaseUrl = 'http://192.168.1.138:8105/api/v1/payment-wallet';
  // support-dispute-service provider surface (complaints/disputes + chat).
  static const defaultSupportApiBaseUrl = 'http://192.168.1.138:8107/api/v1/support-disputes';

  /// Provider uploads register with media-file-service as this owner service.
  /// Must have a matching entry in the service's MEDIA_FILE_SERVICE_TOKENS.
  static const mediaOwnerService = 'hauling-service';

  factory ProviderAppConfig.fromEnvironment() => const ProviderAppConfig(
        haulingApiBaseUrl: String.fromEnvironment(
          'HAULING_API_BASE_URL',
          defaultValue: defaultHaulingApiBaseUrl,
        ),
        mediaFileApiBaseUrl: String.fromEnvironment(
          'MEDIA_FILE_API_BASE_URL',
          defaultValue: defaultMediaFileApiBaseUrl,
        ),
        mediaFileServiceToken: String.fromEnvironment(
          'MEDIA_FILE_SERVICE_TOKEN',
          defaultValue: defaultMediaFileServiceToken,
        ),
        notificationWsUrl: String.fromEnvironment(
          'NOTIFICATION_WS_URL',
          defaultValue: defaultNotificationWsUrl,
        ),
        paymentWalletApiBaseUrl: String.fromEnvironment(
          'PAYMENT_WALLET_API_BASE_URL',
          defaultValue: defaultPaymentWalletApiBaseUrl,
        ),
        supportApiBaseUrl: String.fromEnvironment(
          'SUPPORT_API_BASE_URL',
          defaultValue: defaultSupportApiBaseUrl,
        ),
      );

  final String haulingApiBaseUrl;
  final String mediaFileApiBaseUrl;
  final String mediaFileServiceToken;
  final String notificationWsUrl;
  final String paymentWalletApiBaseUrl;
  final String supportApiBaseUrl;
}
