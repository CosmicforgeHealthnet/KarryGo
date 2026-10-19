import 'package:flutter/foundation.dart';

import '../../../core/data/driver_client.dart';

/// The driver's wallet (payment-wallet `GET /provider/earnings`).
@immutable
class WalletBalance {
  const WalletBalance({required this.availableKobo, required this.pendingKobo});
  final int availableKobo;
  final int pendingKobo;

  factory WalletBalance.fromJson(Map<String, dynamic> j) => WalletBalance(
    availableKobo: (j['available_kobo'] as num? ?? 0).toInt(),
    pendingKobo: (j['pending_kobo'] as num? ?? 0).toInt(),
  );
}

/// A saved payout bank account.
@immutable
class BankAccount {
  const BankAccount({
    required this.id,
    required this.bankName,
    required this.accountNumber,
    required this.accountName,
    required this.isDefault,
  });
  final String id;
  final String bankName;
  final String accountNumber;
  final String accountName;
  final bool isDefault;

  /// "GTBank •••• 6789".
  String get masked {
    final n = accountNumber;
    final tail = n.length > 4 ? n.substring(n.length - 4) : n;
    return '$bankName •••• $tail';
  }

  factory BankAccount.fromJson(Map<String, dynamic> j) => BankAccount(
    id: j['id'] as String? ?? '',
    bankName: j['bank_name'] as String? ?? '',
    accountNumber: j['account_number'] as String? ?? '',
    accountName: j['account_name'] as String? ?? '',
    isDefault: j['is_default'] == true,
  );
}

/// The driver's money: balance, payout accounts, transaction PIN and
/// withdrawals. payment-wallet accepts the taxi driver's own sign-in.
class WalletApi {
  WalletApi(this._client);
  final DriverClient _client;

  Future<WalletBalance> balance() async =>
      WalletBalance.fromJson(await _client.getMap('/provider/earnings'));

  Future<List<BankAccount>> bankAccounts() async => asList(
    await _client.get('/provider/bank-accounts'),
    'bank_accounts',
  ).map(BankAccount.fromJson).toList();

  /// Looks up the account holder's name for an account number.
  Future<String> resolveAccountName({
    required String bankCode,
    required String accountNumber,
  }) async {
    final data = asMap(
      await _client.post('/provider/bank-accounts/resolve', {
        'bank_code': bankCode,
        'account_number': accountNumber,
      }),
    );
    return data['account_name'] as String? ?? '';
  }

  Future<void> addBankAccount({
    required NigerianBank bank,
    required String accountNumber,
  }) => _client.post('/provider/bank-accounts', {
    'bank_code': bank.code,
    'bank_name': bank.name,
    'account_number': accountNumber,
    'currency': 'NGN',
  });

  Future<void> removeBankAccount(String id) =>
      _client.delete('/provider/bank-accounts/$id');

  Future<void> setDefault(String id) =>
      _client.put('/provider/bank-accounts/$id/default');

  Future<bool> hasPin() async =>
      (await _client.getMap('/provider/pin/status'))['pin_set'] == true;

  Future<void> setPin(String pin) =>
      _client.post('/provider/pin', {'pin': pin});

  /// Requests a payout. [idempotencyKey] must stay the same if the driver
  /// retries the same withdrawal, so a repeated tap cannot pay out twice.
  Future<String> withdraw({
    required String bankAccountId,
    required int amountKobo,
    required String pin,
    required String idempotencyKey,
  }) async {
    final data = asMap(
      await _client.post('/provider/withdrawals', {
        'bank_account_id': bankAccountId,
        'amount_kobo': amountKobo,
        'currency': 'NGN',
        'idempotency_key': idempotencyKey,
        'pin': pin,
      }),
    );
    return data['status'] as String? ?? 'pending';
  }
}

/// A Nigerian bank for the "add bank account" dropdown. Codes are the standard
/// CBN/Paystack bank codes used by the resolve/recipient endpoints.
@immutable
class NigerianBank {
  const NigerianBank(this.name, this.code);
  final String name;
  final String code;
}

const nigerianBanks = <NigerianBank>[
  NigerianBank('Access Bank', '044'),
  NigerianBank('Citibank', '023'),
  NigerianBank('Ecobank', '050'),
  NigerianBank('Fidelity Bank', '070'),
  NigerianBank('First Bank of Nigeria', '011'),
  NigerianBank('First City Monument Bank (FCMB)', '214'),
  NigerianBank('Guaranty Trust Bank (GTBank)', '058'),
  NigerianBank('Heritage Bank', '030'),
  NigerianBank('Keystone Bank', '082'),
  NigerianBank('Polaris Bank', '076'),
  NigerianBank('Providus Bank', '101'),
  NigerianBank('Stanbic IBTC Bank', '221'),
  NigerianBank('Standard Chartered Bank', '068'),
  NigerianBank('Sterling Bank', '232'),
  NigerianBank('Union Bank', '032'),
  NigerianBank('United Bank for Africa (UBA)', '033'),
  NigerianBank('Unity Bank', '215'),
  NigerianBank('Wema Bank', '035'),
  NigerianBank('Zenith Bank', '057'),
];
