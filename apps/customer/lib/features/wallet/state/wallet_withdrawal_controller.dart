import 'dart:async';

import 'package:flutter/foundation.dart';

import '../models/wallet_models.dart';
import '../ui/widgets/wallet_amount_keypad.dart';

enum WalletWithdrawalStatus {
  enterAmount,
  selectAccount,
  authorize,
  submitting,
  success,
}

/// Drives the withdrawal flow (mockup #8 -> #11).
///
/// TODO(backend): the payment-wallet-service has no customer withdrawal or
/// bank-account endpoints yet (withdrawals are provider-only and debit the
/// provider earnings account). Everything here is mocked client-side: the bank
/// accounts are a fixed demo list and submission fakes success. Replace with a
/// real customer withdrawal API + customer bank-account management once built.
class WalletWithdrawalController extends ChangeNotifier {
  WalletWithdrawalController({required this.availableKobo});

  /// Spendable balance, used to validate the requested amount.
  final int availableKobo;

  WalletWithdrawalStatus _status = WalletWithdrawalStatus.enterAmount;
  WalletWithdrawalStatus get status => _status;

  /// Raw amount string typed on the keypad, e.g. "200" or "1500.50".
  String _amountText = '';
  String get amountText => _amountText;

  int get amountKobo {
    final value = double.tryParse(_amountText) ?? 0;
    return (value * 100).round();
  }

  String get amountDisplay {
    if (_amountText.isEmpty) return '₦0';
    final value = double.tryParse(_amountText) ?? 0;
    return formatKobo((value * 100).round());
  }

  /// Demo bank accounts (mock). See class-level TODO.
  final List<BankAccount> bankAccounts = const [
    BankAccount(
      id: 'acct-1',
      bankName: 'Guaranty Trust Bank',
      accountNumber: '0123456789',
      accountName: 'Ada Okafor',
    ),
    BankAccount(
      id: 'acct-2',
      bankName: 'Access Bank',
      accountNumber: '0987654321',
      accountName: 'Ada Okafor',
    ),
  ];

  BankAccount? _selectedAccount;
  BankAccount? get selectedAccount => _selectedAccount;

  String _pin = '';
  String get pin => _pin;
  static const int pinLength = 4;

  String? _amountError;
  String? get amountError => _amountError;

  bool get amountValid =>
      amountKobo >= 10000 && amountKobo <= availableKobo;

  // ── Amount step ──────────────────────────────────────────────────────────

  void appendAmount(String digitOrDot) {
    _amountText = applyAmountKey(_amountText, digitOrDot);
    _amountError = null;
    notifyListeners();
  }

  void backspaceAmountKey() {
    _amountText = backspaceAmount(_amountText);
    _amountError = null;
    notifyListeners();
  }

  void confirmAmount() {
    if (amountKobo < 10000) {
      _amountError = 'Minimum withdrawal is ${formatKobo(10000)}.';
      notifyListeners();
      return;
    }
    if (amountKobo > availableKobo) {
      _amountError = 'Amount exceeds your available balance.';
      notifyListeners();
      return;
    }
    _status = WalletWithdrawalStatus.selectAccount;
    notifyListeners();
  }

  // ── Account step ─────────────────────────────────────────────────────────

  void selectAccount(BankAccount account) {
    _selectedAccount = account;
    notifyListeners();
  }

  void confirmAccount() {
    if (_selectedAccount == null) return;
    _status = WalletWithdrawalStatus.authorize;
    notifyListeners();
  }

  // ── PIN step (UI-only, no server check) ──────────────────────────────────

  void appendPin(String digit) {
    if (_pin.length >= pinLength) return;
    _pin = '$_pin$digit';
    notifyListeners();
    if (_pin.length == pinLength) {
      _submit();
    }
  }

  void backspacePin() {
    if (_pin.isEmpty) return;
    _pin = _pin.substring(0, _pin.length - 1);
    notifyListeners();
  }

  Future<void> _submit() async {
    _status = WalletWithdrawalStatus.submitting;
    notifyListeners();
    // TODO(backend): replace with real customer withdrawal API call.
    await Future<void>.delayed(const Duration(seconds: 2));
    _status = WalletWithdrawalStatus.success;
    notifyListeners();
  }

  // ── Navigation ───────────────────────────────────────────────────────────

  void back() {
    switch (_status) {
      case WalletWithdrawalStatus.selectAccount:
        _status = WalletWithdrawalStatus.enterAmount;
      case WalletWithdrawalStatus.authorize:
        _pin = '';
        _status = WalletWithdrawalStatus.selectAccount;
      default:
        break;
    }
    notifyListeners();
  }
}
