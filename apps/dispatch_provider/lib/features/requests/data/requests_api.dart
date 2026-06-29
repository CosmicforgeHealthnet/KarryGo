import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import 'request_model.dart';

class RequestsApi {
  final ApiCoreConfig config;
  final http.Client _client;

  RequestsApi(this.config, {http.Client? client})
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

  // ── GET /api/v1/provider/requests ────────────────────────────────────────

  Future<ApiResult<List<RequestModel>>> listRequests({
    required String accessToken,
  }) async {
    final uri = config.uri('/api/v1/provider/requests');
    _debugLog('[REQUESTS] GET $uri');
    try {
      final response = await _client
          .get(uri, headers: _getHeaders(accessToken))
          .timeout(const Duration(seconds: 15));
      _debugLog('[REQUESTS] GET status=${response.statusCode}');
      if (response.statusCode == 200) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          final data = body['data'];
          if (data is List) {
            final requests = data
                .whereType<Map<String, dynamic>>()
                .map(RequestModel.fromJson)
                .toList();
            return ApiSuccess(requests);
          }
          return const ApiSuccess([]);
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[REQUESTS] GET error: $e');
      return _networkFailure(e);
    }
  }

  // ── GET /api/v1/provider/requests/:id ────────────────────────────────────

  Future<ApiResult<RequestModel>> getRequest({
    required String accessToken,
    required String id,
  }) async {
    final uri = config.uri('/api/v1/provider/requests/$id');
    _debugLog('[REQUESTS] GET $uri');
    try {
      final response = await _client
          .get(uri, headers: _getHeaders(accessToken))
          .timeout(const Duration(seconds: 15));
      _debugLog('[REQUESTS] GET/:id status=${response.statusCode}');
      if (response.statusCode == 200) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          final data =
              body['data'] as Map<String, dynamic>? ?? <String, dynamic>{};
          return ApiSuccess(RequestModel.fromJson(data));
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[REQUESTS] GET/:id error: $e');
      return _networkFailure(e);
    }
  }

  // ── POST /api/v1/provider/requests/:id/accept ────────────────────────────

  Future<ApiResult<bool>> acceptRequest({
    required String accessToken,
    required String id,
  }) async {
    final uri = config.uri('/api/v1/provider/requests/$id/accept');
    _debugLog('[REQUESTS] POST accept $uri');
    try {
      final response = await _client
          .post(uri, headers: _jsonHeaders(accessToken), body: jsonEncode({}))
          .timeout(const Duration(seconds: 15));
      _debugLog('[REQUESTS] POST accept status=${response.statusCode}');
      if (response.statusCode == 200 || response.statusCode == 201) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          return const ApiSuccess(true);
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[REQUESTS] POST accept error: $e');
      return _networkFailure(e);
    }
  }

  // ── POST /api/v1/provider/requests/:id/reject ────────────────────────────

  Future<ApiResult<bool>> rejectRequest({
    required String accessToken,
    required String id,
    required String reason,
  }) async {
    final uri = config.uri('/api/v1/provider/requests/$id/reject');
    _debugLog('[REQUESTS] POST reject $uri');
    try {
      final response = await _client
          .post(
            uri,
            headers: _jsonHeaders(accessToken),
            body: jsonEncode({'reason': reason}),
          )
          .timeout(const Duration(seconds: 15));
      _debugLog('[REQUESTS] POST reject status=${response.statusCode}');
      if (response.statusCode == 200 || response.statusCode == 201) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          return const ApiSuccess(true);
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[REQUESTS] POST reject error: $e');
      return _networkFailure(e);
    }
  }
}
