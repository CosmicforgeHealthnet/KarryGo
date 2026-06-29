import 'dart:async';
import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../models/dispatch_auth_models.dart';

class DispatchAuthApi {
  final ApiCoreConfig config;
  final http.Client _client;

  DispatchAuthApi(this.config, {http.Client? client})
    : _client = client ?? http.Client();

  static const _sensitiveJsonKeys = {'access_token', 'refresh_token'};

  static void _debugLog(String message) {
    if (kDebugMode) {
      debugPrint(message);
    }
  }

  static bool _looksLikeEmail(String value) => value.contains('@');

  static const backendConnectionMessage =
      'Cannot connect to Cosmicforge Logistics server. Check backend URL/network and try again.';

  static ApiFailure<T> _networkFailure<T>(Object error) {
    return ApiFailure(
      ApiException(
        code: ApiErrorCode.network,
        message: backendConnectionMessage,
        cause: error,
      ),
    );
  }

  static String normalizeNigerianPhoneNumber(String value) {
    final compact = value.trim().replaceAll(RegExp(r'[\s\-()]'), '');
    if (compact.contains('@')) return compact.toLowerCase();
    if (compact.startsWith('+') && !compact.startsWith('+234')) {
      return compact;
    }

    var subscriber = compact;
    if (subscriber.startsWith('+234')) {
      subscriber = subscriber.substring(4);
    } else if (subscriber.startsWith('234')) {
      subscriber = subscriber.substring(3);
    }
    while (subscriber.startsWith('0')) {
      subscriber = subscriber.substring(1);
    }

    if (RegExp(r'^\d{10}$').hasMatch(subscriber)) {
      return '+234$subscriber';
    }
    return compact;
  }

  static String _maskToken(String token) {
    if (token.isEmpty) return '<empty>';
    if (token.length <= 10) return '<redacted:${token.length} chars>';
    return '${token.substring(0, 6)}...${token.substring(token.length - 4)}';
  }

  static Object? _redactSensitiveJson(Object? value) {
    if (value is Map) {
      return value.map((key, entryValue) {
        final normalizedKey = key.toString().toLowerCase();
        if (_sensitiveJsonKeys.contains(normalizedKey)) {
          return MapEntry(key, _maskToken(entryValue?.toString() ?? ''));
        }
        return MapEntry(key, _redactSensitiveJson(entryValue));
      });
    }
    if (value is List) {
      return value.map(_redactSensitiveJson).toList();
    }
    return value;
  }

  static String _safeResponseBody(String body) {
    try {
      return jsonEncode(_redactSensitiveJson(jsonDecode(body)));
    } catch (_) {
      return body.length > 500 ? '${body.substring(0, 500)}...' : body;
    }
  }

  // ── Private helpers ───────────────────────────────────────────────────────

  /// Sends [request] and returns the raw [http.Response].
  /// Throws only for genuine connection/transport failures
  /// (SocketException, ClientException, TimeoutException, etc.).
  /// Non-200 HTTP status codes are NOT exceptions — they are returned as-is.
  Future<http.Response> _send(http.Request request) async {
    final streamed = await _client
        .send(request)
        .timeout(const Duration(seconds: 10));
    return http.Response.fromStream(streamed);
  }

  /// Tries to parse [body] as a JSON object.
  /// Returns [null] on any parse error so callers can fall back gracefully.
  static Map<String, dynamic>? _tryDecode(String body) {
    try {
      final decoded = jsonDecode(body);
      if (decoded is Map<String, dynamic>) return decoded;
    } catch (_) {}
    return null;
  }

  /// Maps an HTTP [statusCode] to an [ApiErrorCode] constant.
  /// Used as a last-resort fallback when the response body cannot be parsed.
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

  /// Returns an [ApiFailure] for a non-200 HTTP response.
  ///
  /// Priority:
  ///   1. Parse the JSON error envelope `{"error":{"code":"...","message":"..."}}`.
  ///   2. If body is unreadable, fall back to HTTP-status mapping — never
  ///      [ApiErrorCode.network], because a response WAS received.
  static ApiFailure<T> _failureFromResponse<T>(http.Response response) {
    final parsed = _tryDecode(response.body);
    if (parsed != null) {
      return ApiFailure(
        ApiException.fromErrorEnvelope(parsed, statusCode: response.statusCode),
      );
    }
    // Body was not valid JSON (e.g. empty body, proxy HTML page).
    return ApiFailure(
      ApiException(
        code: _statusToCode(response.statusCode),
        message: 'Request failed (HTTP ${response.statusCode}).',
        statusCode: response.statusCode,
      ),
    );
  }

  // ── Legacy start (phone-only) ──────────────────────────────────────────────

  /// Legacy /auth/start — kept for backward compat; new code should use
  /// [signupStart] or [loginStart].
  ///
  /// Pass [email] to also deliver the OTP to an email address. Optional —
  /// when omitted the backend falls back to SMS-only delivery.
  Future<ApiResult<AuthStartResponse>> start(String phoneNumber, {String? email}) async {
    final uri = config.uri('/api/v1/auth/start');
    final normalizedPhone = normalizeNigerianPhoneNumber(phoneNumber);
    final Map<String, dynamic> requestBody = {'phone_number': normalizedPhone};
    if (email != null && email.isNotEmpty) requestBody['email'] = email;
    final encodedBody = jsonEncode(requestBody);

    _debugLog('=== [AUTH START REQUEST] ===');
    _debugLog('url=$uri | phone=$normalizedPhone');

    try {
      final response = await _client
          .post(
            uri,
            headers: {
              'Content-Type': 'application/json',
              'Accept': 'application/json',
            },
            body: encodedBody,
          )
          .timeout(const Duration(seconds: 10));

      _debugLog('=== [AUTH START RESPONSE] status=${response.statusCode} ===');
      _debugLog(_safeResponseBody(response.body));

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200 && body['success'] == true) {
        return ApiSuccess(
          AuthStartResponse.fromJson(body['data'] as Map<String, dynamic>),
        );
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e, stackTrace) {
      _debugLog('=== [AUTH START EXCEPTION] $e\n$stackTrace ===');
      return _networkFailure(e);
    }
  }

  // ── Signup start ──────────────────────────────────────────────────────────

  /// POST /api/v1/auth/signup/start — sends OTP to phone after validating
  /// that neither phone nor email is already registered.
  ///
  /// Returns [ApiFailure] with [ApiErrorCode.conflict] when the account
  /// already exists (409).  Only transport failures produce
  /// [ApiErrorCode.network].
  Future<ApiResult<AuthStartResponse>> signupStart({
    required String phoneNumber,
    required String email,
  }) async {
    final uri = config.uri('/api/v1/auth/signup/start');
    final normalizedPhone = normalizeNigerianPhoneNumber(phoneNumber);

    _debugLog(
      '=== [SIGNUP START REQUEST] url=$uri | phone=$normalizedPhone ===',
    );

    // ── Step 1: Send the HTTP request ────────────────────────────────────
    final http.Response response;
    try {
      final req = http.Request('POST', uri)
        ..headers['Content-Type'] = 'application/json'
        ..headers['Accept'] = 'application/json'
        ..body = jsonEncode({'phone_number': normalizedPhone, 'email': email});
      response = await _send(req);
    } on TimeoutException catch (e, st) {
      _debugLog('=== [SIGNUP START TIMEOUT] $e\n$st ===');
      return _networkFailure(e);
    } catch (e, st) {
      _debugLog('=== [SIGNUP START NETWORK ERROR] $e\n$st ===');
      return _networkFailure(e);
    }

    // ── Step 2: Parse the HTTP response ──────────────────────────────────
    _debugLog('=== [SIGNUP START RESPONSE] status=${response.statusCode} ===');
    _debugLog(_safeResponseBody(response.body));

    if (response.statusCode == 200) {
      final body = _tryDecode(response.body);
      if (body != null && body['success'] == true) {
        try {
          return ApiSuccess(
            AuthStartResponse.fromJson(body['data'] as Map<String, dynamic>),
          );
        } catch (e) {
          _debugLog('[SIGNUP START] 200 body parse error: $e');
          return ApiFailure(
            ApiException(
              code: ApiErrorCode.unknown,
              message: 'Unexpected response from server.',
              statusCode: response.statusCode,
            ),
          );
        }
      }
      return _failureFromResponse(response);
    }

    return _failureFromResponse(response);
  }

  // ── Login start ───────────────────────────────────────────────────────────

  /// POST /api/v1/auth/login/start — identifier can be phone (E.164) or email.
  ///
  /// Returns [ApiFailure] with [ApiErrorCode.notFound] (code `"not_found"`)
  /// when the backend responds 404.  Only genuine transport failures
  /// (no server reachable, timeout) produce [ApiErrorCode.network].
  Future<ApiResult<AuthStartResponse>> loginStart({
    required String identifier,
  }) async {
    final uri = config.uri('/api/v1/auth/login/start');
    final normalizedIdentifier = _looksLikeEmail(identifier)
        ? identifier.trim().toLowerCase()
        : normalizeNigerianPhoneNumber(identifier);

    _debugLog('=== [LOGIN START REQUEST] url=$uri ===');

    // ── Step 1: Send the HTTP request ────────────────────────────────────
    // Only SocketException / TimeoutException / ClientException end up here
    // as thrown errors — a 404 response is NOT an exception.
    final http.Response response;
    try {
      final req = http.Request('POST', uri)
        ..headers['Content-Type'] = 'application/json'
        ..headers['Accept'] = 'application/json'
        ..body = jsonEncode({'identifier': normalizedIdentifier});
      response = await _send(req);
    } on TimeoutException catch (e, st) {
      _debugLog('=== [LOGIN START TIMEOUT] $e\n$st ===');
      return _networkFailure(e);
    } catch (e, st) {
      // Genuine connection failure — server not reachable.
      _debugLog('=== [LOGIN START NETWORK ERROR] $e\n$st ===');
      return _networkFailure(e);
    }

    // ── Step 2: Parse the HTTP response ──────────────────────────────────
    // Reaches here for ANY HTTP status code (200, 404, 409, …).
    // JSON parse errors do NOT bubble up as network errors.
    _debugLog('=== [LOGIN START RESPONSE] status=${response.statusCode} ===');
    _debugLog(_safeResponseBody(response.body));

    if (response.statusCode == 200) {
      final body = _tryDecode(response.body);
      if (body != null && body['success'] == true) {
        try {
          return ApiSuccess(
            AuthStartResponse.fromJson(body['data'] as Map<String, dynamic>),
          );
        } catch (e) {
          _debugLog('[LOGIN START] 200 body parse error: $e');
          return ApiFailure(
            ApiException(
              code: ApiErrorCode.unknown,
              message: 'Unexpected response from server.',
              statusCode: response.statusCode,
            ),
          );
        }
      }
      return _failureFromResponse(response);
    }

    return _failureFromResponse(response);
  }

  // ── Verify ────────────────────────────────────────────────────────────────

  /// POST /api/v1/auth/verify — sends identifier (phone or email) + otp_code +
  /// purpose ("login" or "signup").  Also accepts legacy phone_number-only calls
  /// (purpose="" triggers the backward-compat upsert path on the backend).
  Future<ApiResult<AuthVerifyResponse>> verify({
    required String identifier,
    required String otpCode,
    required String purpose,
    String? deviceId,
    String? deviceType,
  }) async {
    final uri = config.uri('/api/v1/auth/verify');
    final isEmailIdentifier = _looksLikeEmail(identifier);
    final normalizedIdentifier = isEmailIdentifier
        ? identifier.trim().toLowerCase()
        : normalizeNigerianPhoneNumber(identifier);
    final Map<String, dynamic> requestBody = {'otp_code': otpCode};
    if (isEmailIdentifier) {
      requestBody['identifier'] = normalizedIdentifier;
    } else {
      requestBody['phone_number'] = normalizedIdentifier;
    }
    if (purpose.isNotEmpty) requestBody['purpose'] = purpose;
    if (deviceId != null) requestBody['device_id'] = deviceId;
    if (deviceType != null) requestBody['device_type'] = deviceType;
    final encodedBody = jsonEncode(requestBody);

    _debugLog('=== [AUTH VERIFY REQUEST] url=$uri | purpose=$purpose ===');
    _debugLog('body fields: ${requestBody.keys.join(', ')}');

    try {
      final response = await _client
          .post(
            uri,
            headers: {
              'Content-Type': 'application/json',
              'Accept': 'application/json',
            },
            body: encodedBody,
          )
          .timeout(const Duration(seconds: 30));

      _debugLog('=== [AUTH VERIFY RESPONSE] status=${response.statusCode} ===');
      _debugLog(_safeResponseBody(response.body));

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200 && body['success'] == true) {
        return ApiSuccess(
          AuthVerifyResponse.fromJson(body['data'] as Map<String, dynamic>),
        );
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e, stackTrace) {
      _debugLog('=== [AUTH VERIFY EXCEPTION] $e\n$stackTrace ===');
      return _networkFailure(e);
    }
  }

  // ── Refresh ───────────────────────────────────────────────────────────────

  Future<ApiResult<AuthRefreshResponse>> refresh(String refreshToken) async {
    try {
      final uri = config.uri('/api/v1/auth/refresh');
      _debugLog('=== [AUTH REFRESH REQUEST] url=$uri ===');
      final response = await _client
          .post(
            uri,
            headers: {
              'Content-Type': 'application/json',
              'Accept': 'application/json',
            },
            body: jsonEncode({'refresh_token': refreshToken}),
          )
          .timeout(const Duration(seconds: 10));
      _debugLog(
        '=== [AUTH REFRESH RESPONSE] status=${response.statusCode} ===',
      );
      _debugLog(_safeResponseBody(response.body));

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200 && body['success'] == true) {
        return ApiSuccess(
          AuthRefreshResponse.fromJson(body['data'] as Map<String, dynamic>),
        );
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e, stackTrace) {
      _debugLog('=== [AUTH REFRESH EXCEPTION] $e\n$stackTrace ===');
      return _networkFailure(e);
    }
  }

  // ── Logout ────────────────────────────────────────────────────────────────

  Future<ApiResult<String>> logout({
    required String accessToken,
    String? refreshToken,
  }) async {
    try {
      final uri = config.uri('/api/v1/auth/logout');
      _debugLog('=== [AUTH LOGOUT REQUEST] url=$uri ===');
      final response = await _client
          .post(
            uri,
            headers: {
              'Content-Type': 'application/json',
              'Accept': 'application/json',
              'Authorization': 'Bearer $accessToken',
            },
            body: refreshToken != null
                ? jsonEncode({'refresh_token': refreshToken})
                : null,
          )
          .timeout(const Duration(seconds: 10));
      _debugLog('=== [AUTH LOGOUT RESPONSE] status=${response.statusCode} ===');
      _debugLog(_safeResponseBody(response.body));

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200 && body['success'] == true) {
        final data = body['data'] as Map<String, dynamic>;
        return ApiSuccess(
          data['message'] as String? ?? 'Logged out successfully.',
        );
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e, stackTrace) {
      _debugLog('=== [AUTH LOGOUT EXCEPTION] $e\n$stackTrace ===');
      return _networkFailure(e);
    }
  }
}
