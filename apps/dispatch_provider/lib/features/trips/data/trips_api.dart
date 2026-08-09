import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:http_parser/http_parser.dart';
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
          // Backend wraps the list as {"trips": [...], "total":..., "page":...,
          // "limit":...} (TripListResponse) rather than returning a bare array.
          final data = body['data'];
          final rawList = data is List
              ? data
              : data is Map<String, dynamic>
              ? data['trips']
              : null;
          if (rawList is List) {
            return ApiSuccess(
              rawList
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

  Future<ApiResult<PickupCredential>> markArrived({
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
          final data = body['data'] as Map<String, dynamic>? ?? {};
          final credential = data['pickup_credential'] as Map<String, dynamic>?;
          return ApiSuccess(
            credential != null
                ? PickupCredential.fromJson(credential)
                : PickupCredential.empty,
          );
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[TRIPS] POST arrived error: $e');
      return _networkFailure(e);
    }
  }

  // ── GET/POST /api/v1/provider/trips/:id/pickup-credential ───────────────

  /// Fetches the rider's own pickup credential for redisplay (e.g. after
  /// navigating away from the arrived-at-pickup screen).
  Future<ApiResult<PickupCredential>> getPickupCredential({
    required String accessToken,
    required String id,
  }) async {
    final uri = config.uri('/api/v1/provider/trips/$id/pickup-credential');
    try {
      final response = await _client
          .get(uri, headers: _getHeaders(accessToken))
          .timeout(const Duration(seconds: 15));
      if (response.statusCode == 200) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>? ?? {};
          return ApiSuccess(PickupCredential.fromJson(data));
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      return _networkFailure(e);
    }
  }

  /// Regenerates the rider's own pickup credential.
  Future<ApiResult<PickupCredential>> resendPickupCredential({
    required String accessToken,
    required String id,
  }) async {
    final uri = config.uri('/api/v1/provider/trips/$id/pickup-credential/resend');
    try {
      final response = await _client
          .post(uri, headers: _jsonHeaders(accessToken), body: jsonEncode({}))
          .timeout(const Duration(seconds: 15));
      if (response.statusCode == 200 || response.statusCode == 201) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>? ?? {};
          return ApiSuccess(PickupCredential.fromJson(data));
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
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

  /// Backend requires two separate image files — `delivery_photo` (JPEG/PNG,
  /// <=5MB) and `signature` (JPEG/PNG, <=3MB) — plus `receiver_name` and
  /// `receiver_phone` (must E.164-match the trip's expected receiver phone).
  Future<ApiResult<bool>> submitProof({
    required String accessToken,
    required String id,
    required String photoPath,
    required Uint8List signaturePng,
    required String receiverName,
    required String receiverPhone,
    required String code,
  }) async {
    final uri = config.uri('/api/v1/provider/trips/$id/proof');
    _debugLog('[TRIPS] POST proof $uri path=$photoPath');
    try {
      final photoExt = photoPath.split('.').last.toLowerCase();
      final photoType = photoExt == 'png'
          ? MediaType('image', 'png')
          : MediaType('image', 'jpeg');
      final request = http.MultipartRequest('POST', uri);
      request.headers.addAll(_getHeaders(accessToken));
      request.fields['receiver_name'] = receiverName;
      request.fields['receiver_phone'] = receiverPhone;
      request.fields['code'] = code;
      request.files.add(
        await http.MultipartFile.fromPath(
          'delivery_photo',
          photoPath,
          contentType: photoType,
        ),
      );
      request.files.add(
        http.MultipartFile.fromBytes(
          'signature',
          signaturePng,
          filename: 'signature.png',
          contentType: MediaType('image', 'png'),
        ),
      );
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

  /// Backend requires a non-empty `reason_code` from a fixed set (see
  /// [tripCancelReasons]) — an empty body always fails with a 400.
  Future<ApiResult<bool>> cancelTrip({
    required String accessToken,
    required String id,
    required String reasonCode,
    String reasonText = '',
  }) async {
    final uri = config.uri('/api/v1/provider/trips/$id/cancel');
    _debugLog('[TRIPS] POST cancel $uri reason=$reasonCode');
    try {
      final response = await _client
          .post(
            uri,
            headers: _jsonHeaders(accessToken),
            body: jsonEncode({
              'reason_code': reasonCode,
              'reason_text': reasonText,
            }),
          )
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
