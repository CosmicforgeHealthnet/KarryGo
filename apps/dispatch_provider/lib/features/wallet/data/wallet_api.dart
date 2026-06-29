import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import 'wallet_models.dart';

class WalletApi {
  final ApiCoreConfig config;
  final http.Client _client;

  WalletApi(this.config, {http.Client? client})
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
            'Cannot connect to wallet service. Check backend URL/network and try again.',
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

  // ── GET /provider/earnings ───────────────────────────────────────────────────

  Future<ApiResult<ProviderEarnings>> getEarnings({
    required String accessToken,
  }) async {
    final uri = config.uri('/provider/earnings');
    _debugLog('[WALLET] GET $uri');
    try {
      final response = await _client
          .get(uri, headers: _getHeaders(accessToken))
          .timeout(const Duration(seconds: 15));
      _debugLog('[WALLET] GET earnings status=${response.statusCode}');
      if (response.statusCode == 200) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          final data = body['data'];
          if (data is Map<String, dynamic>) {
            return ApiSuccess(ProviderEarnings.fromJson(data));
          }
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[WALLET] GET earnings error: $e');
      return _networkFailure(e);
    }
  }

  // ── POST /provider/bank-accounts/resolve ─────────────────────────────────────

  Future<ApiResult<BankAccount>> resolveBankAccount({
    required String accessToken,
    required String accountNumber,
    required String bankCode,
  }) async {
    final uri = config.uri('/provider/bank-accounts/resolve');
    _debugLog('[WALLET] POST $uri');
    try {
      final response = await _client
          .post(
            uri,
            headers: _jsonHeaders(accessToken),
            body: jsonEncode({
              'account_number': accountNumber,
              'bank_code': bankCode,
            }),
          )
          .timeout(const Duration(seconds: 20));
      _debugLog('[WALLET] resolve bank account status=${response.statusCode}');
      if (response.statusCode == 200) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          final data = body['data'];
          if (data is Map<String, dynamic>) {
            return ApiSuccess(BankAccount.fromJson(data));
          }
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[WALLET] resolve bank account error: $e');
      return _networkFailure(e);
    }
  }

  // ── POST /provider/bank-accounts ─────────────────────────────────────────────

  Future<ApiResult<BankAccount>> registerBankAccount({
    required String accessToken,
    required String bankCode,
    required String bankName,
    required String accountNumber,
    String currency = 'NGN',
  }) async {
    final uri = config.uri('/provider/bank-accounts');
    _debugLog('[WALLET] POST $uri');
    try {
      final response = await _client
          .post(
            uri,
            headers: _jsonHeaders(accessToken),
            body: jsonEncode({
              'bank_code': bankCode,
              'bank_name': bankName,
              'account_number': accountNumber,
              'currency': currency,
            }),
          )
          .timeout(const Duration(seconds: 15));
      _debugLog('[WALLET] register bank account status=${response.statusCode}');
      if (response.statusCode == 200 || response.statusCode == 201) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          final data = body['data'];
          if (data is Map<String, dynamic>) {
            return ApiSuccess(BankAccount.fromJson(data));
          }
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[WALLET] register bank account error: $e');
      return _networkFailure(e);
    }
  }

  // ── POST /provider/withdrawals ────────────────────────────────────────────────

  Future<ApiResult<Withdrawal>> requestWithdrawal({
    required String accessToken,
    required String bankAccountId,
    required int amountKobo,
    String currency = 'NGN',
    required String idempotencyKey,
  }) async {
    final uri = config.uri('/provider/withdrawals');
    _debugLog('[WALLET] POST $uri amountKobo=$amountKobo');
    try {
      final response = await _client
          .post(
            uri,
            headers: _jsonHeaders(accessToken),
            body: jsonEncode({
              'bank_account_id': bankAccountId,
              'amount_kobo': amountKobo,
              'currency': currency,
              'idempotency_key': idempotencyKey,
            }),
          )
          .timeout(const Duration(seconds: 20));
      _debugLog('[WALLET] request withdrawal status=${response.statusCode}');
      if (response.statusCode == 200 || response.statusCode == 201) {
        final body = _tryDecode(response.body);
        if (body != null && body['success'] == true) {
          final data = body['data'];
          if (data is Map<String, dynamic>) {
            return ApiSuccess(Withdrawal.fromJson(data));
          }
        }
        return _failureFromResponse(response);
      }
      return _failureFromResponse(response);
    } catch (e) {
      _debugLog('[WALLET] request withdrawal error: $e');
      return _networkFailure(e);
    }
  }
}
