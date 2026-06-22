import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;

import '../models/wallet_models.dart';

class WalletApi {
  WalletApi({
    required ApiCoreConfig config,
    http.Client? client,
    this.onAuthFailure,
  })  : _config = config,
        _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final http.Client _client;

  /// Called whenever a request fails with an authentication error (401 /
  /// unauthorized), to drive a global logout on session expiry/revocation.
  final void Function()? onAuthFailure;

  Future<WalletSummary> getWallet({required String accessToken}) async {
    final data = await _sendJson('GET', '/wallets/me', accessToken: accessToken);
    return WalletSummary.fromJson(data);
  }

  Future<List<WalletTransaction>> getTransactions({
    required String accessToken,
    int limit = 50,
  }) async {
    final data = await _sendJson(
      'GET',
      '/wallets/me/transactions?limit=$limit',
      accessToken: accessToken,
    );
    final raw = data['transactions'];
    if (raw is! List) return [];
    return raw
        .map((e) =>
            WalletTransaction.fromJson(Map<String, dynamic>.from(e as Map)))
        .toList();
  }

  Future<TopUpResult> createTopUp({
    required String accessToken,
    required String customerEmail,
    required int amountKobo,
    required String idempotencyKey,
    String currency = 'NGN',
  }) async {
    final data = await _sendJson(
      'POST',
      '/topups',
      accessToken: accessToken,
      body: {
        'customer_email': customerEmail,
        'amount_kobo': amountKobo,
        'currency': currency,
        'idempotency_key': idempotencyKey,
      },
    );
    return TopUpResult.fromJson(data);
  }

  /// Verifies a top-up directly with the backend (which checks Paystack and
  /// credits the wallet). Lets funding complete without waiting on the async
  /// Paystack webhook. Returns the updated wallet summary.
  Future<WalletSummary> verifyTopUp({
    required String accessToken,
    required String reference,
  }) async {
    await _sendJson(
      'POST',
      '/topups/$reference/verify',
      accessToken: accessToken,
    );
    return getWallet(accessToken: accessToken);
  }

  void close() => _client.close();

  Future<Map<String, dynamic>> _sendJson(
    String method,
    String path, {
    required String accessToken,
    Map<String, dynamic>? body,
  }) async {
    try {
      final uri = _config.uri(path);
      final headers = {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
        'Authorization': 'Bearer $accessToken',
      };

      final response = switch (method) {
        'GET' => await _client.get(uri, headers: headers),
        'POST' => await _client.post(
            uri,
            headers: headers,
            body: jsonEncode(body ?? const {}),
          ),
        _ => throw UnsupportedError('Unsupported HTTP method: $method'),
      };

      final decoded = _decode(response);
      if (response.statusCode < 200 || response.statusCode >= 300) {
        throw ApiException.fromErrorEnvelope(decoded,
            statusCode: response.statusCode);
      }
      if (decoded['success'] != true) {
        throw ApiException.fromErrorEnvelope(decoded,
            statusCode: response.statusCode);
      }
      final rawData = decoded['data'];
      if (rawData is Map) return Map<String, dynamic>.from(rawData);
      return const {};
    } on ApiException catch (error) {
      if (error.isAuthFailure) onAuthFailure?.call();
      rethrow;
    } catch (e) {
      throw ApiException.network(e);
    }
  }

  Map<String, dynamic> _decode(http.Response response) {
    if (response.body.isEmpty) return const {'success': true, 'data': {}};
    final decoded = jsonDecode(response.body);
    if (decoded is Map) return Map<String, dynamic>.from(decoded);
    return const {
      'success': false,
      'error': {'code': 'unknown', 'message': 'Something went wrong.'}
    };
  }
}
