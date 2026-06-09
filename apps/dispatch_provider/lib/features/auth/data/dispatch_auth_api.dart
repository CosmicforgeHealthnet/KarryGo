import 'dart:async';
import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:karrygo_api_core/karrygo_api_core.dart';
import '../models/dispatch_auth_models.dart';

class DispatchAuthApi {
  final ApiCoreConfig config;
  final http.Client _client;

  DispatchAuthApi(this.config, {http.Client? client})
      : _client = client ?? http.Client();

  // ── Private helpers ───────────────────────────────────────────────────────

  /// Sends [request] and returns the raw [http.Response].
  /// Throws only for genuine connection/transport failures
  /// (SocketException, ClientException, TimeoutException, etc.).
  /// Non-200 HTTP status codes are NOT exceptions — they are returned as-is.
  Future<http.Response> _send(http.Request request) async {
    final streamed = await _client.send(request).timeout(const Duration(seconds: 10));
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
        ApiException.fromErrorEnvelope(
          parsed,
          statusCode: response.statusCode,
        ),
      );
    }
    // Body was not valid JSON (e.g. empty body, proxy HTML page).
    return ApiFailure(ApiException(
      code: _statusToCode(response.statusCode),
      message: 'Request failed (HTTP ${response.statusCode}).',
      statusCode: response.statusCode,
    ));
  }

  // ── Legacy start (phone-only) ──────────────────────────────────────────────

  /// Legacy /auth/start — kept for backward compat; new code should use
  /// [signupStart] or [loginStart].
  Future<ApiResult<AuthStartResponse>> start(String phoneNumber) async {
    final uri = config.uri('/api/v1/auth/start');
    final Map<String, dynamic> requestBody = {'phone_number': phoneNumber};
    final encodedBody = jsonEncode(requestBody);

    debugPrint('=== [AUTH START REQUEST] ===');
    debugPrint('full URL: $uri | phone: $phoneNumber');

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

      debugPrint('=== [AUTH START RESPONSE] status=${response.statusCode} ===');
      debugPrint(response.body);

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200 && body['success'] == true) {
        return ApiSuccess(AuthStartResponse.fromJson(body['data'] as Map<String, dynamic>));
      }
      return ApiFailure(ApiException.fromErrorEnvelope(body, statusCode: response.statusCode));
    } catch (e, stackTrace) {
      debugPrint('=== [AUTH START EXCEPTION] $e\n$stackTrace ===');
      return ApiFailure(ApiException.network(e));
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

    debugPrint('=== [SIGNUP START REQUEST] url=$uri | phone=$phoneNumber ===');

    // ── Step 1: Send the HTTP request ────────────────────────────────────
    final http.Response response;
    try {
      final req = http.Request('POST', uri)
        ..headers['Content-Type'] = 'application/json'
        ..headers['Accept'] = 'application/json'
        ..body = jsonEncode({'phone_number': phoneNumber, 'email': email});
      response = await _send(req);
    } on TimeoutException catch (e, st) {
      debugPrint('=== [SIGNUP START TIMEOUT] $e\n$st ===');
      return ApiFailure(ApiException(
        code: ApiErrorCode.network,
        message: 'The request timed out. Please try again.',
        cause: e,
      ));
    } catch (e, st) {
      debugPrint('=== [SIGNUP START NETWORK ERROR] $e\n$st ===');
      return ApiFailure(ApiException.network(e));
    }

    // ── Step 2: Parse the HTTP response ──────────────────────────────────
    debugPrint('=== [SIGNUP START RESPONSE] status=${response.statusCode} ===');
    debugPrint(response.body);

    if (response.statusCode == 200) {
      final body = _tryDecode(response.body);
      if (body != null && body['success'] == true) {
        try {
          return ApiSuccess(
            AuthStartResponse.fromJson(body['data'] as Map<String, dynamic>),
          );
        } catch (e) {
          debugPrint('[SIGNUP START] 200 body parse error: $e');
          return ApiFailure(ApiException(
            code: ApiErrorCode.unknown,
            message: 'Unexpected response from server.',
            statusCode: response.statusCode,
          ));
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

    debugPrint('=== [LOGIN START REQUEST] url=$uri ===');

    // ── Step 1: Send the HTTP request ────────────────────────────────────
    // Only SocketException / TimeoutException / ClientException end up here
    // as thrown errors — a 404 response is NOT an exception.
    final http.Response response;
    try {
      final req = http.Request('POST', uri)
        ..headers['Content-Type'] = 'application/json'
        ..headers['Accept'] = 'application/json'
        ..body = jsonEncode({'identifier': identifier});
      response = await _send(req);
    } on TimeoutException catch (e, st) {
      debugPrint('=== [LOGIN START TIMEOUT] $e\n$st ===');
      return ApiFailure(ApiException(
        code: ApiErrorCode.network,
        message: 'The request timed out. Please try again.',
        cause: e,
      ));
    } catch (e, st) {
      // Genuine connection failure — server not reachable.
      debugPrint('=== [LOGIN START NETWORK ERROR] $e\n$st ===');
      return ApiFailure(ApiException.network(e));
    }

    // ── Step 2: Parse the HTTP response ──────────────────────────────────
    // Reaches here for ANY HTTP status code (200, 404, 409, …).
    // JSON parse errors do NOT bubble up as network errors.
    debugPrint('=== [LOGIN START RESPONSE] status=${response.statusCode} ===');
    debugPrint(response.body);

    if (response.statusCode == 200) {
      final body = _tryDecode(response.body);
      if (body != null && body['success'] == true) {
        try {
          return ApiSuccess(
            AuthStartResponse.fromJson(body['data'] as Map<String, dynamic>),
          );
        } catch (e) {
          debugPrint('[LOGIN START] 200 body parse error: $e');
          return ApiFailure(ApiException(
            code: ApiErrorCode.unknown,
            message: 'Unexpected response from server.',
            statusCode: response.statusCode,
          ));
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
    final Map<String, dynamic> requestBody = {
      'identifier': identifier,
      'otp_code': otpCode,
      'purpose': purpose,
    };
    if (deviceId != null) requestBody['device_id'] = deviceId;
    if (deviceType != null) requestBody['device_type'] = deviceType;
    final encodedBody = jsonEncode(requestBody);

    debugPrint('=== [AUTH VERIFY REQUEST] url=$uri | purpose=$purpose ===');
    debugPrint(encodedBody);

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

      debugPrint('=== [AUTH VERIFY RESPONSE] status=${response.statusCode} ===');
      debugPrint(response.body);

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200 && body['success'] == true) {
        return ApiSuccess(AuthVerifyResponse.fromJson(body['data'] as Map<String, dynamic>));
      }
      return ApiFailure(ApiException.fromErrorEnvelope(body, statusCode: response.statusCode));
    } catch (e, stackTrace) {
      debugPrint('=== [AUTH VERIFY EXCEPTION] $e\n$stackTrace ===');
      return ApiFailure(ApiException.network(e));
    }
  }

  // ── Refresh ───────────────────────────────────────────────────────────────

  Future<ApiResult<AuthRefreshResponse>> refresh(String refreshToken) async {
    try {
      final uri = config.uri('/api/v1/auth/refresh');
      final response = await _client.post(
        uri,
        headers: {'Content-Type': 'application/json'},
        body: jsonEncode({'refresh_token': refreshToken}),
      );

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200 && body['success'] == true) {
        return ApiSuccess(AuthRefreshResponse.fromJson(body['data'] as Map<String, dynamic>));
      }
      return ApiFailure(ApiException.fromErrorEnvelope(body, statusCode: response.statusCode));
    } catch (e) {
      return ApiFailure(ApiException.network(e));
    }
  }

  // ── Logout ────────────────────────────────────────────────────────────────

  Future<ApiResult<String>> logout({
    required String accessToken,
    String? refreshToken,
  }) async {
    try {
      final uri = config.uri('/api/v1/auth/logout');
      final response = await _client.post(
        uri,
        headers: {
          'Content-Type': 'application/json',
          'Authorization': 'Bearer $accessToken',
        },
        body: refreshToken != null ? jsonEncode({'refresh_token': refreshToken}) : null,
      );

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200 && body['success'] == true) {
        final data = body['data'] as Map<String, dynamic>;
        return ApiSuccess(data['message'] as String? ?? 'Logged out successfully.');
      }
      return ApiFailure(ApiException.fromErrorEnvelope(body, statusCode: response.statusCode));
    } catch (e) {
      return ApiFailure(ApiException.network(e));
    }
  }
}
