import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import 'trip_model.dart';

class TripsApi {
  final ApiCoreConfig config;
  final http.Client _client;

  TripsApi(this.config, {http.Client? client})
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

  // ── GET /api/v1/provider/trips ───────────────────────────────────────────

  Future<ApiResult<List<TripModel>>> listTrips({
    required String accessToken,
  }) async {
    final uri = config.uri('/api/v1/provider/trips');
    _debugLog('[TRIPS] GET $uri');
    try {
      final response = await _client
          .get(uri, headers: _getHeaders(accessToken))
          .timeout(const Duration(seconds: 15));
      _debugLog('[TRIPS] GET status=${response.statusCode}');
      if (response.statusCode == 200) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          final data = body['data'];
          if (data is List) {
            return ApiSuccess(
              data
                  .whereType<Map<String, dynamic>>()
                  .map(TripModel.fromJson)
                  .toList(),
            );
          }
          return const ApiSuccess([]);
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[TRIPS] GET error: $e');
      return _networkFailure(e);
    }
  }

  // ── GET /api/v1/provider/trips/active ────────────────────────────────────

  Future<ApiResult<TripModel?>> getActiveTrip({
    required String accessToken,
  }) async {
    final uri = config.uri('/api/v1/provider/trips/active');
    _debugLog('[TRIPS] GET active $uri');
    try {
      final response = await _client
          .get(uri, headers: _getHeaders(accessToken))
          .timeout(const Duration(seconds: 15));
      _debugLog('[TRIPS] GET active status=${response.statusCode}');
      if (response.statusCode == 200) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          final data = body['data'];
          if (data == null) return const ApiSuccess(null);
          if (data is Map<String, dynamic>) {
            return ApiSuccess(TripModel.fromJson(data));
          }
          return const ApiSuccess(null);
        }
        return _failureFromResponse(response);
      }
      if (response.statusCode == 404) return const ApiSuccess(null);
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[TRIPS] GET active error: $e');
      return _networkFailure(e);
    }
  }

  // ── GET /api/v1/provider/trips/:id ───────────────────────────────────────

  Future<ApiResult<TripModel>> getTrip({
    required String accessToken,
    required String id,
  }) async {
    final uri = config.uri('/api/v1/provider/trips/$id');
    _debugLog('[TRIPS] GET /:id $uri');
    try {
      final response = await _client
          .get(uri, headers: _getHeaders(accessToken))
          .timeout(const Duration(seconds: 15));
      _debugLog('[TRIPS] GET /:id status=${response.statusCode}');
      if (response.statusCode == 200) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>? ?? {};
          return ApiSuccess(TripModel.fromJson(data));
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[TRIPS] GET /:id error: $e');
      return _networkFailure(e);
    }
  }

  // ── POST /api/v1/provider/trips/:id/arrived ──────────────────────────────

  Future<ApiResult<bool>> markArrived({
    required String accessToken,
    required String id,
  }) async {
    final uri = config.uri('/api/v1/provider/trips/$id/arrived');
    _debugLog('[TRIPS] POST arrived $uri');
    try {
      final response = await _client
          .post(uri, headers: _jsonHeaders(accessToken), body: jsonEncode({}))
          .timeout(const Duration(seconds: 15));
      _debugLog('[TRIPS] POST arrived status=${response.statusCode}');
      if (response.statusCode == 200 || response.statusCode == 201) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          return const ApiSuccess(true);
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[TRIPS] POST arrived error: $e');
      return _networkFailure(e);
    }
  }

  // ── POST /api/v1/provider/trips/:id/start ────────────────────────────────

  Future<ApiResult<bool>> startTrip({
    required String accessToken,
    required String id,
  }) async {
    final uri = config.uri('/api/v1/provider/trips/$id/start');
    _debugLog('[TRIPS] POST start $uri');
    try {
      final response = await _client
          .post(uri, headers: _jsonHeaders(accessToken), body: jsonEncode({}))
          .timeout(const Duration(seconds: 15));
      _debugLog('[TRIPS] POST start status=${response.statusCode}');
      if (response.statusCode == 200 || response.statusCode == 201) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          return const ApiSuccess(true);
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[TRIPS] POST start error: $e');
      return _networkFailure(e);
    }
  }

  // ── POST /api/v1/provider/trips/:id/proof ────────────────────────────────

  Future<ApiResult<bool>> submitProof({
    required String accessToken,
    required String id,
    required String filePath,
  }) async {
    final uri = config.uri('/api/v1/provider/trips/$id/proof');
    _debugLog('[TRIPS] POST proof $uri path=$filePath');
    try {
      final request = http.MultipartRequest('POST', uri);
      request.headers.addAll(_getHeaders(accessToken));
      request.files.add(await http.MultipartFile.fromPath('file', filePath));
      final streamed = await _client
          .send(request)
          .timeout(const Duration(seconds: 60));
      final response = await http.Response.fromStream(streamed);
      _debugLog('[TRIPS] POST proof status=${response.statusCode}');
      if (response.statusCode == 200 || response.statusCode == 201) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          return const ApiSuccess(true);
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[TRIPS] POST proof error: $e');
      return _networkFailure(e);
    }
  }

  // ── GET /api/v1/provider/trips/:id/proof ─────────────────────────────────

  Future<ApiResult<Map<String, dynamic>>> getProof({
    required String accessToken,
    required String id,
  }) async {
    final uri = config.uri('/api/v1/provider/trips/$id/proof');
    _debugLog('[TRIPS] GET proof $uri');
    try {
      final response = await _client
          .get(uri, headers: _getHeaders(accessToken))
          .timeout(const Duration(seconds: 15));
      _debugLog('[TRIPS] GET proof status=${response.statusCode}');
      if (response.statusCode == 200) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          return ApiSuccess((body['data'] as Map<String, dynamic>?) ?? {});
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[TRIPS] GET proof error: $e');
      return _networkFailure(e);
    }
  }

  // ── POST /api/v1/provider/trips/:id/complete ─────────────────────────────

  Future<ApiResult<bool>> completeTrip({
    required String accessToken,
    required String id,
  }) async {
    final uri = config.uri('/api/v1/provider/trips/$id/complete');
    _debugLog('[TRIPS] POST complete $uri');
    try {
      final response = await _client
          .post(uri, headers: _jsonHeaders(accessToken), body: jsonEncode({}))
          .timeout(const Duration(seconds: 15));
      _debugLog('[TRIPS] POST complete status=${response.statusCode}');
      if (response.statusCode == 200 || response.statusCode == 201) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          return const ApiSuccess(true);
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[TRIPS] POST complete error: $e');
      return _networkFailure(e);
    }
  }

  // ── POST /api/v1/provider/trips/:id/cancel ───────────────────────────────

  Future<ApiResult<bool>> cancelTrip({
    required String accessToken,
    required String id,
  }) async {
    final uri = config.uri('/api/v1/provider/trips/$id/cancel');
    _debugLog('[TRIPS] POST cancel $uri');
    try {
      final response = await _client
          .post(uri, headers: _jsonHeaders(accessToken), body: jsonEncode({}))
          .timeout(const Duration(seconds: 15));
      _debugLog('[TRIPS] POST cancel status=${response.statusCode}');
      if (response.statusCode == 200 || response.statusCode == 201) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          return const ApiSuccess(true);
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[TRIPS] POST cancel error: $e');
      return _networkFailure(e);
    }
  }
}
