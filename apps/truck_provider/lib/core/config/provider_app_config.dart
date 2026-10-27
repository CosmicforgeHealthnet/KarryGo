import 'package:flutter/foundation.dart';

class ProviderAppConfig {
  const ProviderAppConfig({
    required this.haulingApiBaseUrl,
    required this.notificationWsUrl,
    required this.paymentWalletApiBaseUrl,
    required this.supportApiBaseUrl,
    required this.referralApiBaseUrl,
  });

  // Release builds talk to the live gateway; debug builds default to this dev
  // machine's LAN IP so a physical phone on the same Wi-Fi reaches local
  // services directly. Any of these can be overridden with --dart-define (a
  // different dev machine, a new DHCP lease, or 127.0.0.1 + adb reverse).
  static const _gateway = 'https://api-gateway.staging.karryam.com';
  static const _lan = 'http://192.168.1.112';

  static const defaultHaulingApiBaseUrl =
      kReleaseMode ? '$_gateway/api/v1/hauling' : '$_lan:8104/api/v1/hauling';
  // Realtime websocket endpoint on notification-service (ws upgrade, token-gated).
  static const defaultNotificationWsUrl = kReleaseMode
      ? 'wss://api-gateway.staging.karryam.com/api/v1/notifications/ws'
      : 'ws://192.168.1.112:8106/api/v1/notifications/ws';
  // payment-wallet-service provider surface (balance, bank accounts, withdrawals).
  // The provider's hauling bearer token is accepted here (service="hauling").
  static const defaultPaymentWalletApiBaseUrl =
      kReleaseMode ? '$_gateway/api/v1/payment-wallet' : '$_lan:8105/api/v1/payment-wallet';
  // support-dispute-service provider surface (complaints/disputes + chat).
  static const defaultSupportApiBaseUrl =
      kReleaseMode ? '$_gateway/api/v1/support-disputes' : '$_lan:8107/api/v1/support-disputes';

  // referral-service (code, dashboard). Accepts the driver's own login token.
  static const defaultReferralApiBaseUrl =
      kReleaseMode ? '$_gateway/api/v1/referrals' : '$_lan:8114/api/v1/referrals';

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
        referralApiBaseUrl: String.fromEnvironment(
          'REFERRAL_API_BASE_URL',
          defaultValue: defaultReferralApiBaseUrl,
        ),
      );

  final String haulingApiBaseUrl;
  final String notificationWsUrl;
  final String paymentWalletApiBaseUrl;
  final String supportApiBaseUrl;
  final String referralApiBaseUrl;
}
