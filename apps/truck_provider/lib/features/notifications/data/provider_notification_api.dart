import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;

/// REST client for the provider notification proxy on driver-hauling-service
/// (`/api/v1/hauling/provider/notifications`). Uses the provider bearer token;
/// the hauling service brokers the HMAC call to notification-service.
class ProviderNotificationApi {
  ProviderNotificationApi({required ApiCoreConfig config, http.Client? client})
    : _config = config,
      _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final http.Client _client;

  /// Mints a short-lived realtime token used to open the notification websocket.
  Future<RealtimeToken> fetchRealtimeToken({required String accessToken}) async {
    final data = await _post('/provider/notifications/realtime-token', const {}, accessToken: accessToken);
    return RealtimeToken(
      token: (data['token'] as String?) ?? '',
      expiresIn: (data['expires_in'] as num?)?.toInt() ?? 0,
    );
  }

  /// Registers an FCM device token so push notifications reach this device.
  Future<void> registerDevice({
    required String accessToken,
    required String token,
    required String platform,
    String app = 'truck_provider',
  }) async {
    await _post(
      '/provider/notifications/devices',
      {'token': token, 'platform': platform, 'app': app},
      accessToken: accessToken,
    );
  }

  void close() => _client.close();

  Future<Map<String, dynamic>> _post(
    String path,
    Map<String, dynamic> body, {
    required String accessToken,
  }) async {
    try {
      final response = await _client.post(
        _config.uri(path),
        headers: {
          'Content-Type': 'application/json',
          'Accept': 'application/json',
          'Authorization': 'Bearer $accessToken',
        },
        body: jsonEncode(body),
      );
      final decoded = _decode(response);
      if (response.statusCode < 200 || response.statusCode >= 300 || decoded['success'] != true) {
        throw ApiException.fromErrorEnvelope(decoded, statusCode: response.statusCode);
      }
      final data = decoded['data'];
      if (data is Map) return Map<String, dynamic>.from(data);
      return const {};
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException.network(e);
    }
  }

  Map<String, dynamic> _decode(http.Response response) {
    if (response.body.isEmpty) return const {'success': true, 'data': {}};
    final decoded = jsonDecode(response.body);
    if (decoded is Map) return Map<String, dynamic>.from(decoded);
    return const {
      'success': false,
      'error': {'code': 'unknown', 'message': 'Unexpected response.'},
    };
  }
}

/// Short-lived realtime websocket token.
class RealtimeToken {
  const RealtimeToken({required this.token, required this.expiresIn});

  final String token;
  final int expiresIn;

  bool get isValid => token.isNotEmpty;
}
