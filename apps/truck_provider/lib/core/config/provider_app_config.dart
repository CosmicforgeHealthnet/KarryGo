class ProviderAppConfig {
  const ProviderAppConfig({
    required this.haulingApiBaseUrl,
    required this.notificationWsUrl,
    required this.paymentWalletApiBaseUrl,
    required this.supportApiBaseUrl,
  });

  // Defaults point at this dev machine's LAN IP so a physical phone on the same
  // Wi-Fi reaches the backend directly, with no `adb reverse` tunnels. Override
  // any of these with --dart-define when the host differs (a different dev
  // machine, a new DHCP lease, or the 127.0.0.1 + adb reverse setup).
  static const defaultHaulingApiBaseUrl = 'http://192.168.1.112:8104/api/v1/hauling';
  // Realtime websocket endpoint on notification-service (ws:// upgrade, token-gated).
  static const defaultNotificationWsUrl = 'ws://192.168.1.112:8106/api/v1/notifications/ws';
  // payment-wallet-service provider surface (balance, bank accounts, withdrawals).
  // The provider's hauling bearer token is accepted here (service="hauling").
  static const defaultPaymentWalletApiBaseUrl = 'http://192.168.1.112:8105/api/v1/payment-wallet';
  // support-dispute-service provider surface (complaints/disputes + chat).
  static const defaultSupportApiBaseUrl = 'http://192.168.1.112:8107/api/v1/support-disputes';

  factory ProviderAppConfig.fromEnvironment() => const ProviderAppConfig(
        haulingApiBaseUrl: String.fromEnvironment(
          'HAULING_API_BASE_URL',
          defaultValue: defaultHaulingApiBaseUrl,
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
  final String notificationWsUrl;
  final String paymentWalletApiBaseUrl;
  final String supportApiBaseUrl;
}
