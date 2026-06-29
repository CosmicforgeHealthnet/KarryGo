import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import 'availability_models.dart';

class AvailabilityApi {
  final ApiCoreConfig config;
  final http.Client _client;

  AvailabilityApi(this.config, {http.Client? client})
    : _client = client ?? http.Client();

  static void _debugLog(String message) {
    if (kDebugMode) debugPrint(message);
  }

  Map<String, String> _jsonHeaders(String accessToken) => {
    'Content-Type': 'application/json',
    'Accept': 'application/json',
    'Authorization': 'Bearer $accessToken',
  };

  Map<String, String> _getHeaders(String accessToken) => {
    'Accept': 'application/json',
    'Authorization': 'Bearer $accessToken',
  };

  static Map<String, dynamic>? _tryDecode(String body) {
    try {
      final decoded = jsonDecode(body);
      if (decoded is Map<String, dynamic>) return decoded;
    } catch (_) {}
    return null;
  }

  static ApiFailure<T> _networkFailure<T>(Object error) {
    return ApiFailure(
      ApiException(
        code: ApiErrorCode.network,
        message:
            'Cannot connect to Cosmicforge Logistics server. Check backend URL/network and try again.',
        cause: error,
      ),
    );
  }

  static ApiFailure<T> _failureFromResponse<T>(http.Response response) {
    final parsed = _tryDecode(response.body);
    if (parsed != null) {
      return ApiFailure(
        ApiException.fromErrorEnvelope(parsed, statusCode: response.statusCode),
      );
    }
    return ApiFailure(
      ApiException(
        code: _statusToCode(response.statusCode),
        message: 'Request failed (HTTP ${response.statusCode}).',
        statusCode: response.statusCode,
      ),
    );
  }

  static String _statusToCode(int statusCode) {
    return switch (statusCode) {
      400 => ApiErrorCode.validationFailed,
      401 => ApiErrorCode.unauthorized,
      403 => ApiErrorCode.forbidden,
      404 => ApiErrorCode.notFound,
      409 => ApiErrorCode.conflict,
      429 => ApiErrorCode.rateLimited,
      503 => ApiErrorCode.serviceUnavailable,
      _ => ApiErrorCode.unknown,
    };
  }

  // ── GET /api/v1/provider/availability ────────────────────────────────────

  Future<ApiResult<ProviderAvailability>> getAvailability({
    required String accessToken,
  }) async {
    final uri = config.uri('/api/v1/provider/availability');
    _debugLog('[AVAILABILITY] GET $uri');
    try {
      final response = await _client
          .get(uri, headers: _getHeaders(accessToken))
          .timeout(const Duration(seconds: 15));
      _debugLog('[AVAILABILITY] GET status=${response.statusCode}');
      if (response.statusCode == 200) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          final data =
              body['data'] as Map<String, dynamic>? ?? <String, dynamic>{};
          return ApiSuccess(ProviderAvailability.fromJson(data));
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[AVAILABILITY] GET error: $e');
      return _networkFailure(e);
    }
  }

  // ── PATCH /api/v1/provider/availability ──────────────────────────────────

  /// [status] must be "online" or "offline". Never send "busy" — backend manages
  /// busy from trip events.
  Future<ApiResult<ProviderAvailability>> updateAvailabilityStatus({
    required String accessToken,
    required String status,
  }) async {
    final uri = config.uri('/api/v1/provider/availability');
    _debugLog('[AVAILABILITY] PATCH $uri  status=$status');
    try {
      final response = await _client
          .patch(
            uri,
            headers: _jsonHeaders(accessToken),
            body: jsonEncode({'status': status}),
          )
          .timeout(const Duration(seconds: 15));
      _debugLog('[AVAILABILITY] PATCH status=${response.statusCode}');
      if (response.statusCode == 200) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          final data =
              body['data'] as Map<String, dynamic>? ?? <String, dynamic>{};
          return ApiSuccess(ProviderAvailability.fromJson(data));
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[AVAILABILITY] PATCH error: $e');
      return _networkFailure(e);
    }
  }

  // ── GET /api/v1/provider/availability/session/current ────────────────────

  Future<ApiResult<AvailabilitySession>> getCurrentSession({
    required String accessToken,
  }) async {
    final uri = config.uri('/api/v1/provider/availability/session/current');
    _debugLog('[AVAILABILITY] GET current session $uri');
    try {
      final response = await _client
          .get(uri, headers: _getHeaders(accessToken))
          .timeout(const Duration(seconds: 15));
      _debugLog('[AVAILABILITY] GET session status=${response.statusCode}');
      if (response.statusCode == 200) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          final data =
              body['data'] as Map<String, dynamic>? ?? <String, dynamic>{};
          return ApiSuccess(AvailabilitySession.fromJson(data));
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[AVAILABILITY] GET session error: $e');
      return _networkFailure(e);
    }
  }

  // ── POST /api/v1/provider/location ───────────────────────────────────────

  /// Backend requires the provider to be online or busy. Returns 403/400 if
  /// offline.
  Future<ApiResult<ProviderLocation>> updateLocation({
    required String accessToken,
    required double lat,
    required double lng,
    double? heading,
    double? speed,
    double? accuracy,
  }) async {
    final uri = config.uri('/api/v1/provider/location');
    final payload = <String, dynamic>{'lat': lat, 'lng': lng};
    if (heading != null) payload['heading'] = heading;
    if (speed != null) payload['speed'] = speed;
    if (accuracy != null) payload['accuracy'] = accuracy;
    _debugLog('[LOCATION] POST $uri  lat=$lat  lng=$lng');
    try {
      final response = await _client
          .post(
            uri,
            headers: _jsonHeaders(accessToken),
            body: jsonEncode(payload),
          )
          .timeout(const Duration(seconds: 15));
      _debugLog('[LOCATION] POST status=${response.statusCode}');
      if (response.statusCode == 200) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          final data =
              body['data'] as Map<String, dynamic>? ?? <String, dynamic>{};
          return ApiSuccess(ProviderLocation.fromJson(data));
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[LOCATION] POST error: $e');
      return _networkFailure(e);
    }
  }

  // ── GET /api/v1/provider/location ────────────────────────────────────────

  Future<ApiResult<ProviderLocation>> getLocation({
    required String accessToken,
  }) async {
    final uri = config.uri('/api/v1/provider/location');
    _debugLog('[LOCATION] GET $uri');
    try {
      final response = await _client
          .get(uri, headers: _getHeaders(accessToken))
          .timeout(const Duration(seconds: 15));
      _debugLog('[LOCATION] GET status=${response.statusCode}');
      if (response.statusCode == 200) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          final data =
              body['data'] as Map<String, dynamic>? ?? <String, dynamic>{};
          return ApiSuccess(ProviderLocation.fromJson(data));
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[LOCATION] GET error: $e');
      return _networkFailure(e);
    }
  }
}
