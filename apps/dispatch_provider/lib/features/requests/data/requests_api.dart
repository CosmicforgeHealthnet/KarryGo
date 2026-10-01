import 'dart:convert';
import 'dart:math';

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

  // ── Broadcast: GET /api/v1/provider/requests ─────────────────────────────

  Future<ApiResult<List<RequestModel>>> listBroadcasts({
    required String accessToken,
  }) =>
      _listSimple(
        accessToken: accessToken,
        path: '/api/v1/provider/requests',
        logTag: 'REQUESTS:broadcast',
      );

  // ── Assignment: GET /api/v1/provider/assignment-offers ───────────────────

  Future<ApiResult<List<RequestModel>>> listAssignmentOffers({
    required String accessToken,
  }) =>
      _listSimple(
        accessToken: accessToken,
        path: '/api/v1/provider/assignment-offers',
        logTag: 'REQUESTS:assignment',
      );

  // ── Commerce: GET /api/v1/provider/organization-offers ───────────────────

  Future<ApiResult<List<RequestModel>>> listOrganizationOffers({
    required String accessToken,
  }) =>
      _listSimple(
        accessToken: accessToken,
        path: '/api/v1/provider/organization-offers',
        logTag: 'REQUESTS:commerce',
        listKey: 'offers',
      );

  // ── Commerce: GET /api/v1/provider/company-assignments ───────────────────

  Future<ApiResult<List<RequestModel>>> listCompanyAssignments({
    required String accessToken,
  }) =>
      _listSimple(
        accessToken: accessToken,
        path: '/api/v1/provider/company-assignments',
        logTag: 'REQUESTS:assignments',
        listKey: 'assignments',
      );

  /// Shared list reader. Handles both a bare array under `data` and an
  /// object with a named key (`{"offers": [...]}`).
  Future<ApiResult<List<RequestModel>>> _listSimple({
    required String accessToken,
    required String path,
    required String logTag,
    String? listKey,
  }) async {
    final uri = config.uri(path);
    _debugLog('[$logTag] GET $uri');
    try {
      final response = await _client
          .get(uri, headers: _getHeaders(accessToken))
          .timeout(const Duration(seconds: 15));
      _debugLog('[$logTag] status=${response.statusCode}');

      // 404 = feature flag off; 503 = same. Both mean "no data from this
      // source" — not an error the user needs to see.
      if (response.statusCode == 404 || response.statusCode == 503) {
        return const ApiSuccess([]);
      }

      if (response.statusCode == 200) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          final data = body['data'];
          final raw = _extractList(data, listKey);
          return ApiSuccess(
            raw
                .whereType<Map<String, dynamic>>()
                .map(RequestModel.fromJson)
                .toList(),
          );
        }
        // Marketplace uses a different envelope: {"data": ..., "meta": ...}
        if (body != null && body.containsKey('data')) {
          final raw = _extractList(body['data'], listKey);
          return ApiSuccess(
            raw
                .whereType<Map<String, dynamic>>()
                .map(RequestModel.fromJson)
                .toList(),
          );
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[$logTag] error: $e');
      return _networkFailure(e);
    }
  }

  static List<dynamic> _extractList(dynamic data, String? listKey) {
    if (data is List) return data;
    if (data is Map<String, dynamic>) {
      if (listKey != null && data[listKey] is List) {
        return data[listKey] as List;
      }
      // Common fallbacks.
      for (final k in ['items', 'offers', 'assignments', 'data']) {
        if (data[k] is List) return data[k] as List;
      }
    }
    return const [];
  }

  // ── Accept ────────────────────────────────────────────────────────────────

  Future<ApiResult<bool>> accept({
    required String accessToken,
    required RequestModel request,
  }) async {
    final path = _acceptPath(request);
    final body = _acceptBody(request);
    _debugLog('[REQUESTS] POST accept $path');
    try {
      final headers = _jsonHeaders(accessToken);
      headers['Idempotency-Key'] = _newIdempotencyKey();
      final response = await _client
          .post(
            config.uri(path),
            headers: headers,
            body: jsonEncode(body),
          )
          .timeout(const Duration(seconds: 15));
      _debugLog('[REQUESTS] accept status=${response.statusCode}');
      if (response.statusCode == 200 ||
          response.statusCode == 201 ||
          response.statusCode == 202) {
        final parsed = _tryDecode(response.body);
        // Two envelopes exist: {"success": true} and {"data": {...}}.
        if (parsed == null || parsed['success'] == true || parsed.containsKey('data')) {
          return const ApiSuccess(true);
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[REQUESTS] accept error: $e');
      return _networkFailure(e);
    }
  }

  // ── Reject / decline ─────────────────────────────────────────────────────

  Future<ApiResult<bool>> reject({
    required String accessToken,
    required RequestModel request,
    required String reason,
  }) async {
    final path = _rejectPath(request);
    final body = _rejectBody(request, reason);
    _debugLog('[REQUESTS] POST reject $path');
    try {
      final headers = _jsonHeaders(accessToken);
      headers['Idempotency-Key'] = _newIdempotencyKey();
      final response = await _client
          .post(
            config.uri(path),
            headers: headers,
            body: jsonEncode(body),
          )
          .timeout(const Duration(seconds: 15));
      _debugLog('[REQUESTS] reject status=${response.statusCode}');
      if (response.statusCode == 200 ||
          response.statusCode == 201 ||
          response.statusCode == 202) {
        final parsed = _tryDecode(response.body);
        if (parsed == null || parsed['success'] == true || parsed.containsKey('data')) {
          return const ApiSuccess(true);
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[REQUESTS] reject error: $e');
      return _networkFailure(e);
    }
  }

  // ── Per-source path & body ───────────────────────────────────────────────

  static String _acceptPath(RequestModel r) => switch (r.kind) {
        RequestServerKind.broadcast =>
          '/api/v1/provider/requests/${r.id}/accept',
        RequestServerKind.assignmentOffer =>
          '/api/v1/provider/assignment-offers/${r.id}/accept',
        RequestServerKind.organizationOffer =>
          '/api/v1/provider/organization-offers/${r.id}/accept',
        RequestServerKind.companyAssignment =>
          '/api/v1/provider/company-assignments/${r.id}/accept',
      };

  static String _rejectPath(RequestModel r) => switch (r.kind) {
        RequestServerKind.broadcast =>
          '/api/v1/provider/requests/${r.id}/reject',
        RequestServerKind.assignmentOffer =>
          '/api/v1/provider/assignment-offers/${r.id}/decline',
        RequestServerKind.organizationOffer =>
          '/api/v1/provider/organization-offers/${r.id}/decline',
        RequestServerKind.companyAssignment =>
          '/api/v1/provider/company-assignments/${r.id}/decline',
      };

  static Map<String, dynamic> _acceptBody(RequestModel r) => switch (r.kind) {
        RequestServerKind.broadcast => <String, dynamic>{},
        RequestServerKind.assignmentOffer => {
            'expected_version': r.version,
          },
        RequestServerKind.organizationOffer => {
            'clientResponseId': _newIdempotencyKey(),
            'expectedVersion': r.version,
            'requestId': r.bookingId,
          },
        RequestServerKind.companyAssignment => {
            'clientResponseId': _newIdempotencyKey(),
            'expectedVersion': r.version,
            'requestId': r.bookingId,
          },
      };

  static Map<String, dynamic> _rejectBody(RequestModel r, String reason) =>
      switch (r.kind) {
        RequestServerKind.broadcast => <String, dynamic>{
            // Backend whitelist is strictly {too_far, busy, other}.
            'reason': _normalizeBroadcastReason(reason),
          },
        RequestServerKind.assignmentOffer => {
            'expected_version': r.version,
            'reason': reason,
          },
        RequestServerKind.organizationOffer => {
            'clientResponseId': _newIdempotencyKey(),
            'expectedVersion': r.version,
            'reasonCode': reason,
          },
        RequestServerKind.companyAssignment => {
            'clientResponseId': _newIdempotencyKey(),
            'expectedVersion': r.version,
            'reasonCode': reason,
          },
      };

  static String _normalizeBroadcastReason(String input) {
    const allowed = {'too_far', 'busy', 'other'};
    return allowed.contains(input) ? input : 'other';
  }

  static final _rand = Random();
  static String _newIdempotencyKey() {
    final now = DateTime.now().microsecondsSinceEpoch;
    final r = _rand.nextInt(1 << 30);
    return 'req-$now-$r';
  }
}