import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;

import '../models/wallet_models.dart';

/// Client for the payment-wallet-service provider surface. Uses the provider's
/// hauling bearer token (payment-wallet accepts it via service="hauling").
class ProviderWalletApi {
  ProviderWalletApi({required ApiCoreConfig config, http.Client? client})
      : _config = config,
        _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final http.Client _client;

  Future<WalletBalance> getBalance({required String accessToken}) async {
    final data = await _get('/provider/earnings', accessToken: accessToken);
    return WalletBalance.fromJson(data);
  }

  Future<List<ProviderBankAccount>> listBankAccounts({required String accessToken}) async {
    final data = await _get('/provider/bank-accounts', accessToken: accessToken);
    final raw = data['bank_accounts'];
    if (raw is List) {
      return raw
          .map((e) => ProviderBankAccount.fromJson(Map<String, dynamic>.from(e as Map)))
          .toList();
    }
    return const [];
  }

  Future<ResolvedBankAccount> resolveBankAccount({
    required String accessToken,
    required String accountNumber,
    required String bankCode,
  }) async {
    final data = await _post(
      '/provider/bank-accounts/resolve',
      {'account_number': accountNumber, 'bank_code': bankCode},
      accessToken: accessToken,
    );
    return ResolvedBankAccount.fromJson(data);
  }

  Future<ProviderBankAccount> registerBankAccount({
    required String accessToken,
    required String bankCode,
    required String bankName,
    required String accountNumber,
  }) async {
    final data = await _post(
      '/provider/bank-accounts',
      {
        'bank_code': bankCode,
        'bank_name': bankName,
        'account_number': accountNumber,
        'currency': 'NGN',
      },
      accessToken: accessToken,
    );
    return ProviderBankAccount.fromJson(data);
  }

  Future<WithdrawalResult> requestWithdrawal({
    required String accessToken,
    required String bankAccountId,
    required int amountKobo,
    required String idempotencyKey,
  }) async {
    final data = await _post(
      '/provider/withdrawals',
      {
        'bank_account_id': bankAccountId,
        'amount_kobo': amountKobo,
        'currency': 'NGN',
        'idempotency_key': idempotencyKey,
      },
      accessToken: accessToken,
    );
    return WithdrawalResult.fromJson(data);
  }

  void close() => _client.close();

  // ─── HTTP helpers ─────────────────────────────────────────────────────────

  Future<Map<String, dynamic>> _get(String path, {required String accessToken}) async {
    try {
      final response = await _client.get(_config.uri(path), headers: _headers(accessToken));
      return _unwrap(response);
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException.network(e);
    }
  }

  Future<Map<String, dynamic>> _post(
    String path,
    Map<String, dynamic> body, {
    required String accessToken,
  }) async {
    try {
      final response = await _client.post(
        _config.uri(path),
        headers: _headers(accessToken),
        body: jsonEncode(body),
      );
      return _unwrap(response);
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException.network(e);
    }
  }

  Map<String, String> _headers(String accessToken) => {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
        'Authorization': 'Bearer $accessToken',
      };

  Map<String, dynamic> _unwrap(http.Response response) {
    final decoded = response.body.isEmpty
        ? <String, dynamic>{'success': true, 'data': <String, dynamic>{}}
        : Map<String, dynamic>.from(jsonDecode(response.body) as Map);
    if (response.statusCode < 200 || response.statusCode >= 300 || decoded['success'] != true) {
      throw ApiException.fromErrorEnvelope(decoded, statusCode: response.statusCode);
    }
    final raw = decoded['data'];
    if (raw is Map) return Map<String, dynamic>.from(raw);
    return const {};
  }
}
