class ProviderAppConfig {
  const ProviderAppConfig({
    required this.haulingApiBaseUrl,
    required this.mediaFileApiBaseUrl,
    required this.mediaFileServiceToken,
    required this.notificationWsUrl,
  });

  static const defaultHaulingApiBaseUrl = 'http://192.168.1.138:8104/api/v1/hauling';
  static const defaultMediaFileApiBaseUrl = 'http://192.168.1.138:8109/api/v1/media-files';
  static const defaultMediaFileServiceToken = 'development-media-token';
  // Realtime websocket endpoint on notification-service (ws:// upgrade, token-gated).
  static const defaultNotificationWsUrl = 'ws://192.168.1.138:8106/api/v1/notifications/ws';

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
      );

  final String haulingApiBaseUrl;
  final String mediaFileApiBaseUrl;
  final String mediaFileServiceToken;
  final String notificationWsUrl;
}
