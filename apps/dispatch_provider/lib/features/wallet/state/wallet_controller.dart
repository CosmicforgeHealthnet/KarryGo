import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../data/wallet_api.dart';
import '../data/wallet_models.dart';

class WalletController extends ChangeNotifier {
  final WalletApi api;
  final String? Function() getAccessToken;

  ProviderEarnings? _earnings;
  BankAccount? _bankAccount;
  bool _isLoading = false;
  String? _error;

  ProviderEarnings? get earnings => _earnings;
  BankAccount? get bankAccount => _bankAccount;
  bool get isLoading => _isLoading;
  String? get error => _error;

  WalletController({required this.api, required this.getAccessToken});

  static void _debugLog(String message) {
    if (kDebugMode) debugPrint(message);
  }

  String? _token() => getAccessToken();

  ApiFailure<T> _unauthorized<T>() => ApiFailure(
    const ApiException(
      code: ApiErrorCode.unauthorized,
      message: 'No access token available.',
    ),
  );

  void clearError() {
    _error = null;
    notifyListeners();
  }

  // ── Load earnings ─────────────────────────────────────────────────────────

  Future<ApiResult<ProviderEarnings>> loadEarnings() async {
    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();

    _isLoading = true;
    _error = null;
    notifyListeners();

    final result = await api.getEarnings(accessToken: token);
    result.when(
      success: (data) {
        _earnings = data;
        _debugLog('[WALLET] earnings loaded: available=${data.availableKobo}');
      },
      failure: (err) {
        _error = err.message;
        _debugLog('[WALLET] loadEarnings failed: ${err.message}');
      },
    );
    _isLoading = false;
    notifyListeners();
    return result;
  }

  // ── Resolve bank account ──────────────────────────────────────────────────

  Future<ApiResult<BankAccount>> resolveBankAccount({
    required String accountNumber,
    required String bankCode,
  }) async {
    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();
    return api.resolveBankAccount(
      accessToken: token,
      accountNumber: accountNumber,
      bankCode: bankCode,
    );
  }

  // ── Register bank account ─────────────────────────────────────────────────

  Future<ApiResult<BankAccount>> registerBankAccount({
    required String bankCode,
    required String bankName,
    required String accountNumber,
    String currency = 'NGN',
  }) async {
    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();
    final result = await api.registerBankAccount(
      accessToken: token,
      bankCode: bankCode,
      bankName: bankName,
      accountNumber: accountNumber,
      currency: currency,
    );
    result.when(
      success: (data) {
        _bankAccount = data;
        notifyListeners();
      },
      failure: (_) {},
    );
    return result;
  }

  void clearBankAccount() {
    _bankAccount = null;
    notifyListeners();
  }

  // ── Request withdrawal ────────────────────────────────────────────────────

  Future<ApiResult<Withdrawal>> requestWithdrawal({
    required String bankAccountId,
    required int amountKobo,
    String currency = 'NGN',
    required String idempotencyKey,
  }) async {
    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();
    return api.requestWithdrawal(
      accessToken: token,
      bankAccountId: bankAccountId,
      amountKobo: amountKobo,
      currency: currency,
      idempotencyKey: idempotencyKey,
    );
  }
}
