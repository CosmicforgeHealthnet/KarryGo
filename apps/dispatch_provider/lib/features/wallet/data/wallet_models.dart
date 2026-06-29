class ProviderEarnings {
  final int availableKobo;
  final int pendingKobo;
  final String currency;

  const ProviderEarnings({
    required this.availableKobo,
    required this.pendingKobo,
    required this.currency,
  });

  String get availableNaira => '₦${(availableKobo / 100).toStringAsFixed(2)}';
  String get pendingNaira => '₦${(pendingKobo / 100).toStringAsFixed(2)}';

  factory ProviderEarnings.fromJson(Map<String, dynamic> json) {
    return ProviderEarnings(
      availableKobo: (json['available_kobo'] as num?)?.toInt() ?? 0,
      pendingKobo: (json['pending_kobo'] as num?)?.toInt() ?? 0,
      currency: (json['currency'] as String?) ?? 'NGN',
    );
  }
}

class BankAccount {
  final String id;
  final String bankCode;
  final String bankName;
  final String accountNumber;
  final String accountName;
  final String currency;

  const BankAccount({
    required this.id,
    required this.bankCode,
    required this.bankName,
    required this.accountNumber,
    required this.accountName,
    required this.currency,
  });

  factory BankAccount.fromJson(Map<String, dynamic> json) {
    return BankAccount(
      id: (json['id'] as String?) ?? '',
      bankCode: (json['bank_code'] as String?) ?? '',
      bankName: (json['bank_name'] as String?) ?? '',
      accountNumber: (json['account_number'] as String?) ?? '',
      accountName: (json['account_name'] as String?) ?? '',
      currency: (json['currency'] as String?) ?? 'NGN',
    );
  }
}

class Withdrawal {
  final String id;
  final String reference;
  final int amountKobo;
  final String currency;
  final String status;

  const Withdrawal({
    required this.id,
    required this.reference,
    required this.amountKobo,
    required this.currency,
    required this.status,
  });

  double get amountNaira => amountKobo / 100;

  factory Withdrawal.fromJson(Map<String, dynamic> json) {
    return Withdrawal(
      id: (json['id'] as String?) ?? '',
      reference: (json['reference'] as String?) ?? '',
      amountKobo: (json['amount_kobo'] as num?)?.toInt() ?? 0,
      currency: (json['currency'] as String?) ?? 'NGN',
      status: (json['status'] as String?) ?? '',
    );
  }
}
