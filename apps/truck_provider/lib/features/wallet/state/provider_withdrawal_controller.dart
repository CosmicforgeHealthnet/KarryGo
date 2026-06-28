import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';

import '../data/provider_wallet_api.dart';
import '../models/wallet_models.dart';

/// Shared state for the multi-step provider withdrawal flow
/// (amount → confirm/bank → authorize → processing → receipt).
class ProviderWithdrawalController extends ChangeNotifier {
  ProviderWithdrawalController({
    required ProviderWalletApi api,
    required String? Function() accessToken,
  })  : _api = api,
        _accessToken = accessToken;

  final ProviderWalletApi _api;
  final String? Function() _accessToken;
  String? get _token => _accessToken();

  // ─── Balance + accounts ─────────────────────────────────────────────────────
  WalletBalance _balance = WalletBalance.empty;
  WalletBalance get balance => _balance;

  List<ProviderBankAccount> _bankAccounts = const [];
  List<ProviderBankAccount> get bankAccounts => _bankAccounts;

  bool _loading = false;
  bool get loading => _loading;

  String? _loadError;
  String? get loadError => _loadError;

  // ─── Amount entry (whole naira via keypad) ──────────────────────────────────
  String _amountInput = '';
  String get amountInput => _amountInput;
  int get amountNaira => int.tryParse(_amountInput) ?? 0;
  int get amountKobo => amountNaira * 100;
  bool get exceedsBalance => amountKobo > _balance.availableKobo;
  bool get canSubmitAmount => amountKobo > 0 && !exceedsBalance;

  // ─── Selected payout account ────────────────────────────────────────────────
  ProviderBankAccount? _selectedAccount;
  ProviderBankAccount? get selectedAccount => _selectedAccount;

  // ─── Add-bank-account sub-flow ──────────────────────────────────────────────
  bool _resolving = false;
  bool get resolving => _resolving;
  ResolvedBankAccount? _resolved;
  ResolvedBankAccount? get resolved => _resolved;
  bool _registering = false;
  bool get registering => _registering;
  String? _bankError;
  String? get bankError => _bankError;

  // ─── Withdrawal submission ──────────────────────────────────────────────────
  bool _submitting = false;
  bool get submitting => _submitting;
  String? _submitError;
  String? get submitError => _submitError;
  WithdrawalResult? _result;
  WithdrawalResult? get result => _result;

  // Idempotency key, generated once per withdrawal attempt so a retry is safe.
  String? _idempotencyKey;

  // ─── Lifecycle ──────────────────────────────────────────────────────────────

  /// Resets transient flow state and loads the live balance + saved accounts.
  Future<void> startNewWithdrawal() async {
    _amountInput = '';
    _selectedAccount = null;
    _result = null;
    _submitError = null;
    _idempotencyKey = null;
    _resolved = null;
    _bankError = null;
    await loadBalanceAndAccounts();
  }

  Future<void> loadBalanceAndAccounts() async {
    final token = _token;
    if (token == null) return;
    _loading = true;
    _loadError = null;
    notifyListeners();
    try {
      final balance = await _api.getBalance(accessToken: token);
      final accounts = await _api.listBankAccounts(accessToken: token);
      _balance = balance;
      _bankAccounts = accounts;
      // Default selection: first saved account.
      _selectedAccount ??= accounts.isNotEmpty ? accounts.first : null;
    } catch (e) {
      _loadError = _msg(e);
    } finally {
      _loading = false;
      notifyListeners();
    }
  }

  // ─── Amount keypad ──────────────────────────────────────────────────────────

  void appendDigit(String digit) {
    // Guard against leading zeros and overly long inputs.
    if (_amountInput.isEmpty && digit == '0') return;
    if (_amountInput.length >= 9) return;
    _amountInput += digit;
    notifyListeners();
  }

  void deleteDigit() {
    if (_amountInput.isEmpty) return;
    _amountInput = _amountInput.substring(0, _amountInput.length - 1);
    notifyListeners();
  }

  // ─── Account selection ──────────────────────────────────────────────────────

  void selectAccount(ProviderBankAccount account) {
    _selectedAccount = account;
    notifyListeners();
  }

  /// Resolves an account number against a bank (account-name lookup).
  Future<bool> resolveBank({required String accountNumber, required NigerianBank bank}) async {
    final token = _token;
    if (token == null) return false;
    _resolving = true;
    _bankError = null;
    _resolved = null;
    notifyListeners();
    try {
      _resolved = await _api.resolveBankAccount(
        accessToken: token,
        accountNumber: accountNumber,
        bankCode: bank.code,
      );
      return true;
    } catch (e) {
      _bankError = _msg(e);
      return false;
    } finally {
      _resolving = false;
      notifyListeners();
    }
  }

  /// Registers the resolved account as a payout destination and selects it.
  Future<bool> registerBank({required String accountNumber, required NigerianBank bank}) async {
    final token = _token;
    if (token == null) return false;
    _registering = true;
    _bankError = null;
    notifyListeners();
    try {
      final account = await _api.registerBankAccount(
        accessToken: token,
        bankCode: bank.code,
        bankName: bank.name,
        accountNumber: accountNumber,
      );
      _bankAccounts = [account, ..._bankAccounts.where((a) => a.id != account.id)];
      _selectedAccount = account;
      _resolved = null;
      return true;
    } catch (e) {
      _bankError = _msg(e);
      return false;
    } finally {
      _registering = false;
      notifyListeners();
    }
  }

  void clearBankError() {
    _bankError = null;
    _resolved = null;
    notifyListeners();
  }

  // ─── Withdrawal submission ──────────────────────────────────────────────────

  /// Submits the withdrawal. Returns true on success (result is populated).
  Future<bool> submitWithdrawal() async {
    final token = _token;
    final account = _selectedAccount;
    if (token == null || account == null) {
      _submitError = 'Select a payout account first.';
      notifyListeners();
      return false;
    }
    _submitting = true;
    _submitError = null;
    notifyListeners();

    _idempotencyKey ??= 'wd-${DateTime.now().microsecondsSinceEpoch}';
    try {
      _result = await _api.requestWithdrawal(
        accessToken: token,
        bankAccountId: account.id,
        amountKobo: amountKobo,
        idempotencyKey: _idempotencyKey!,
      );
      return true;
    } catch (e) {
      _submitError = _msg(e);
      // Force a fresh key on the next attempt after a hard failure.
      _idempotencyKey = null;
      return false;
    } finally {
      _submitting = false;
      notifyListeners();
    }
  }

  String _msg(Object e) => e is ApiException ? e.message : e.toString();
}
