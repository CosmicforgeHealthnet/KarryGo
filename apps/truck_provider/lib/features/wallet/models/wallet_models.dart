import 'package:flutter/foundation.dart';

/// Provider wallet balance (payment-wallet-service `GET /provider/earnings`).
@immutable
class WalletBalance {
  const WalletBalance({
    required this.availableKobo,
    required this.escrowKobo,
    required this.pendingKobo,
    required this.currency,
  });

  final int availableKobo;
  final int escrowKobo;
  final int pendingKobo;
  final String currency;

  double get availableNaira => availableKobo / 100;
  double get escrowNaira => escrowKobo / 100;
  double get pendingNaira => pendingKobo / 100;

  static const empty = WalletBalance(
    availableKobo: 0,
    escrowKobo: 0,
    pendingKobo: 0,
    currency: 'NGN',
  );

  factory WalletBalance.fromJson(Map<String, dynamic> j) => WalletBalance(
        availableKobo: (j['available_kobo'] as num? ?? 0).toInt(),
        escrowKobo: (j['escrow_kobo'] as num? ?? 0).toInt(),
        pendingKobo: (j['pending_kobo'] as num? ?? 0).toInt(),
        currency: j['currency'] as String? ?? 'NGN',
      );
}

/// A saved provider payout bank account.
@immutable
class ProviderBankAccount {
  const ProviderBankAccount({
    required this.id,
    required this.bankCode,
    required this.bankName,
    required this.accountNumber,
    required this.accountName,
    required this.status,
  });

  final String id;
  final String bankCode;
  final String bankName;
  final String accountNumber;
  final String accountName;
  final String status;

  factory ProviderBankAccount.fromJson(Map<String, dynamic> j) => ProviderBankAccount(
        id: j['id'] as String? ?? '',
        bankCode: j['bank_code'] as String? ?? '',
        bankName: j['bank_name'] as String? ?? '',
        accountNumber: j['account_number'] as String? ?? '',
        accountName: j['account_name'] as String? ?? '',
        status: j['status'] as String? ?? '',
      );
}

/// Result of resolving an account number against a bank (account-name lookup).
@immutable
class ResolvedBankAccount {
  const ResolvedBankAccount({required this.accountNumber, required this.accountName});

  final String accountNumber;
  final String accountName;

  factory ResolvedBankAccount.fromJson(Map<String, dynamic> j) => ResolvedBankAccount(
        accountNumber: j['account_number'] as String? ?? '',
        accountName: j['account_name'] as String? ?? '',
      );
}

/// Result of a withdrawal request.
@immutable
class WithdrawalResult {
  const WithdrawalResult({
    required this.id,
    required this.reference,
    required this.amountKobo,
    required this.status,
  });

  final String id;
  final String reference;
  final int amountKobo;
  final String status;

  double get amountNaira => amountKobo / 100;

  factory WithdrawalResult.fromJson(Map<String, dynamic> j) => WithdrawalResult(
        id: j['id'] as String? ?? '',
        reference: j['reference'] as String? ?? '',
        amountKobo: (j['amount_kobo'] as num? ?? 0).toInt(),
        status: j['status'] as String? ?? 'pending',
      );
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
