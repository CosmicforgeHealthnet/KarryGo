import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;

import '../models/app_notification.dart';

/// REST client for the customer notification proxy exposed by customer-service
/// (`/api/v1/customer/notifications`). All calls use the customer bearer token;
/// the owning service brokers the HMAC-signed call to notification-service.
class NotificationApi {
  NotificationApi({
    required ApiCoreConfig config,
    http.Client? client,
    this.onAuthFailure,
  }) : _config = config,
       _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final http.Client _client;
  final void Function()? onAuthFailure;

  /// Loads the recipient's recent notifications (most recent first).
  Future<List<AppNotification>> listNotifications({
    required String accessToken,
    int limit = 50,
  }) async {
    final raw = await _sendJsonList(
      'GET',
      '/notifications?limit=$limit',
      accessToken: accessToken,
    );
    return raw
        .map((e) => AppNotification.fromJson(Map<String, dynamic>.from(e as Map)))
        .toList();
  }

  /// Mints a short-lived realtime token used to open the notification websocket.
  Future<RealtimeToken> fetchRealtimeToken({required String accessToken}) async {
    final data = await _sendJson(
      'POST',
      '/notifications/realtime-token',
      accessToken: accessToken,
    );
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
    String app = 'customer',
  }) async {
    await _sendJson(
      'POST',
      '/notifications/devices',
      accessToken: accessToken,
      body: {'token': token, 'platform': platform, 'app': app},
    );
  }

  void close() => _client.close();

  // ─── HTTP helpers ──────────────────────────────────────────────────────────

  Future<Map<String, dynamic>> _sendJson(
    String method,
    String path, {
    Map<String, dynamic>? body,
    String? accessToken,
  }) async {
    try {
      final uri = _config.uri(path);
      final headers = {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
        if (accessToken != null) 'Authorization': 'Bearer $accessToken',
      };

      final response = switch (method) {
        'GET' => await _client.get(uri, headers: headers),
        'POST' => await _client.post(uri, headers: headers, body: jsonEncode(body ?? const {})),
        _ => throw UnsupportedError('Unsupported HTTP method: $method'),
      };

      final decoded = _decodeResponse(response);
      if (response.statusCode < 200 || response.statusCode >= 300 || decoded['success'] != true) {
        throw ApiException.fromErrorEnvelope(decoded, statusCode: response.statusCode);
      }
      final rawData = decoded['data'];
      if (rawData is Map) return Map<String, dynamic>.from(rawData);
      return const {};
    } on ApiException catch (error) {
      if (error.isAuthFailure) onAuthFailure?.call();
      rethrow;
    } catch (error) {
      throw ApiException.network(error);
    }
  }

  Future<List<dynamic>> _sendJsonList(
    String method,
    String path, {
    String? accessToken,
  }) async {
    try {
      final uri = _config.uri(path);
      final headers = {
        'Accept': 'application/json',
        if (accessToken != null) 'Authorization': 'Bearer $accessToken',
      };
      final response = await _client.get(uri, headers: headers);
      final decoded = _decodeResponse(response);
      if (response.statusCode < 200 || response.statusCode >= 300) {
        throw ApiException.fromErrorEnvelope(decoded, statusCode: response.statusCode);
      }
      final rawData = decoded['data'];
      if (rawData is List) return rawData;
      return const [];
    } on ApiException catch (error) {
      if (error.isAuthFailure) onAuthFailure?.call();
      rethrow;
    } catch (error) {
      throw ApiException.network(error);
    }
  }

  Map<String, dynamic> _decodeResponse(http.Response response) {
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
