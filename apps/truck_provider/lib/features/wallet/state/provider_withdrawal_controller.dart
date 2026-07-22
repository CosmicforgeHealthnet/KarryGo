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

  // ─── Transaction PIN ────────────────────────────────────────────────────────
  /// Null until the status has been fetched; then whether a PIN exists.
  bool? _pinSet;
  bool? get pinSet => _pinSet;

  /// The PIN typed on the authorization screen; sent with the withdrawal so
  /// payment-wallet verifies it server-side (with lockout).
  String _enteredPin = '';
  String get enteredPin => _enteredPin;

  bool _savingPin = false;
  bool get savingPin => _savingPin;
  String? _pinError;
  String? get pinError => _pinError;

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
    _enteredPin = '';
    _pinError = null;
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
      _selectedAccount ??= _defaultAccount(accounts);
      // PIN status decides whether the auth screen shows "create" or "enter";
      // failure is non-fatal (the auth screen re-checks before submitting).
      try {
        _pinSet = await _api.getPinStatus(accessToken: token);
      } catch (_) {}
    } catch (e) {
      _loadError = _msg(e);
    } finally {
      _loading = false;
      notifyListeners();
    }
  }

  /// The provider's persisted default account, falling back to the first
  /// listed account (the backend lists default-first anyway).
  ProviderBankAccount? _defaultAccount(List<ProviderBankAccount> accounts) {
    if (accounts.isEmpty) return null;
    for (final account in accounts) {
      if (account.isDefault) return account;
    }
    return accounts.first;
  }

  /// Accounts-only refresh for the Payments & Withdrawals screen.
  Future<void> loadAccounts() async {
    final token = _token;
    if (token == null) return;
    _loading = true;
    _loadError = null;
    notifyListeners();
    try {
      _bankAccounts = await _api.listBankAccounts(accessToken: token);
    } catch (e) {
      _loadError = _msg(e);
    } finally {
      _loading = false;
      notifyListeners();
    }
  }

  /// Removes a saved payout account. Returns true on success; failures land in
  /// [bankError].
  Future<bool> deleteAccount(ProviderBankAccount account) async {
    final token = _token;
    if (token == null) return false;
    _bankError = null;
    notifyListeners();
    try {
      await _api.deleteBankAccount(accessToken: token, accountId: account.id);
      if (_selectedAccount?.id == account.id) _selectedAccount = null;
      // Re-fetch so a promoted default (server-side) is reflected.
      _bankAccounts = await _api.listBankAccounts(accessToken: token);
      _selectedAccount ??= _defaultAccount(_bankAccounts);
      return true;
    } catch (e) {
      _bankError = _msg(e);
      return false;
    } finally {
      notifyListeners();
    }
  }

  /// Persists the account as the provider's default withdrawal account.
  Future<bool> makeDefault(ProviderBankAccount account) async {
    final token = _token;
    if (token == null) return false;
    _bankError = null;
    // Optimistic flip so the radio responds instantly.
    _bankAccounts = [
      for (final a in _bankAccounts)
        ProviderBankAccount(
          id: a.id,
          bankCode: a.bankCode,
          bankName: a.bankName,
          accountNumber: a.accountNumber,
          accountName: a.accountName,
          status: a.status,
          isDefault: a.id == account.id,
        ),
    ];
    notifyListeners();
    try {
      await _api.setDefaultBankAccount(accessToken: token, accountId: account.id);
      return true;
    } catch (e) {
      _bankError = _msg(e);
      // Roll back to the server's truth.
      try {
        _bankAccounts = await _api.listBankAccounts(accessToken: token);
      } catch (_) {}
      return false;
    } finally {
      notifyListeners();
    }
  }

  // ─── Transaction PIN ────────────────────────────────────────────────────────

  // ignore: use_setters_to_change_properties
  void setEnteredPin(String pin) {
    _enteredPin = pin;
  }

  /// Creates the provider's transaction PIN. Returns true on success.
  Future<bool> setupPin(String pin) async {
    final token = _token;
    if (token == null) return false;
    _savingPin = true;
    _pinError = null;
    notifyListeners();
    try {
      await _api.setPin(accessToken: token, pin: pin);
      _pinSet = true;
      return true;
    } catch (e) {
      _pinError = _msg(e);
      return false;
    } finally {
      _savingPin = false;
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
        pin: _enteredPin,
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
