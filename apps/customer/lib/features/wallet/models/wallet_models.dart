class WalletSummary {
  const WalletSummary({
    required this.ownerType,
    required this.ownerID,
    required this.currency,
    required this.availableKobo,
    required this.escrowKobo,
    required this.pendingKobo,
  });

  final String ownerType;
  final String ownerID;
  final String currency;

  /// Spendable balance.
  final int availableKobo;

  /// Funds held in escrow against active jobs.
  final int escrowKobo;

  /// Funds reserved by pending withdrawals/refunds/settlements.
  final int pendingKobo;

  /// Spendable balance in naira (major unit).
  double get balanceNaira => availableKobo / 100;

  double get escrowNaira => escrowKobo / 100;

  double get pendingNaira => pendingKobo / 100;

  String get formattedBalance => _formatNaira(availableKobo);

  String get formattedEscrow => _formatNaira(escrowKobo);

  String get formattedPending => _formatNaira(pendingKobo);

  factory WalletSummary.fromJson(Map<String, dynamic> json) {
    return WalletSummary(
      ownerType: json['owner_type']?.toString() ?? '',
      ownerID: json['owner_id']?.toString() ?? '',
      currency: json['currency']?.toString() ?? 'NGN',
      availableKobo: _int(json['available_kobo']),
      escrowKobo: _int(json['escrow_kobo']),
      pendingKobo: _int(json['pending_kobo']),
    );
  }

  static int _int(Object? v) {
    if (v is int) return v;
    if (v is num) return v.toInt();
    return int.tryParse(v?.toString() ?? '') ?? 0;
  }
}

/// Adds thousands separators to a plain numeric string like "204000.00".
String _thousands(String number) {
  return number.replaceAllMapped(
    RegExp(r'(\d{1,3})(?=(\d{3})+(?!\d))'),
    (m) => '${m[1]},',
  );
}

String _formatNaira(int kobo) {
  return '₦${_thousands((kobo / 100).toStringAsFixed(2))}';
}

/// Display status for a transaction, matching the mockup's chip styles.
enum WalletTxnStatus {
  pending,
  success,
  failed;

  String get label => switch (this) {
        WalletTxnStatus.pending => 'Pending',
        WalletTxnStatus.success => 'Successful',
        WalletTxnStatus.failed => 'Failed',
      };
}

class WalletTransaction {
  const WalletTransaction({
    required this.reference,
    required this.type,
    required this.side,
    required this.amountKobo,
    required this.currency,
    required this.description,
    required this.createdAt,
  });

  final String reference;

  /// Ledger transaction type, e.g. `topup`, `payment`, `refund`.
  final String type;

  /// Ledger side from the wallet owner's perspective: `credit` or `debit`.
  final String side;
  final int amountKobo;
  final String currency;
  final String description;
  final DateTime createdAt;

  double get amountNaira => amountKobo / 100;

  bool get isCredit => side == 'credit';

  String get formattedAmount {
    final sign = isCredit ? '+' : '-';
    final abs = amountNaira.abs();
    return '$sign₦${_thousands(abs.toStringAsFixed(2))}';
  }

  /// Friendly title derived from the ledger transaction type.
  String get title => switch (type) {
        'paystack_charge_success' => 'Wallet Top-up',
        'wallet_payment_hold' => 'Payment',
        'job_settlement' => 'Job Settlement',
        'wallet_refund_processed' || 'refund_processed' => 'Refund',
        'refund_reserved' => 'Refund Pending',
        'refund_failed' => 'Refund Failed',
        'withdrawal_paid' => 'Withdrawal',
        'withdrawal_reserved' => 'Withdrawal Pending',
        'withdrawal_reversed' || 'withdrawal_cancelled' => 'Withdrawal Reversed',
        _ => isCredit ? 'Credit' : 'Payment',
      };

  /// Display status derived from the transaction type, matching the mockup's
  /// pending / success / failed chip.
  WalletTxnStatus get status => switch (type) {
        'refund_reserved' || 'withdrawal_reserved' => WalletTxnStatus.pending,
        'refund_failed' => WalletTxnStatus.failed,
        'withdrawal_reversed' || 'withdrawal_cancelled' =>
          WalletTxnStatus.failed,
        _ => WalletTxnStatus.success,
      };

  factory WalletTransaction.fromJson(Map<String, dynamic> json) {
    return WalletTransaction(
      reference: json['reference']?.toString() ?? '',
      type: json['transaction_type']?.toString() ?? '',
      side: json['side']?.toString() ?? '',
      amountKobo: _int(json['amount_kobo']),
      currency: json['currency']?.toString() ?? 'NGN',
      description: json['memo']?.toString() ?? '',
      createdAt: DateTime.tryParse(json['created_at']?.toString() ?? '') ??
          DateTime.now(),
    );
  }

  static int _int(Object? v) {
    if (v is int) return v;
    if (v is num) return v.toInt();
    return int.tryParse(v?.toString() ?? '') ?? 0;
  }
}

/// Formats a kobo amount as a naira string with thousands separators, e.g.
/// `₦204,000.00`. Shared by wallet UI that works in kobo.
String formatKobo(int kobo) => _formatNaira(kobo);

/// A bank account a customer can withdraw to.
///
/// NOTE: the payment-wallet-service has no customer bank-account or withdrawal
/// endpoints yet, so these are mocked client-side. See
/// [WalletWithdrawalController] for the TODO marker.
class BankAccount {
  const BankAccount({
    required this.id,
    required this.bankName,
    required this.accountNumber,
    required this.accountName,
  });

  final String id;
  final String bankName;
  final String accountNumber;
  final String accountName;

  /// Masked number for list display, e.g. `•••• 4321`.
  String get maskedNumber {
    if (accountNumber.length <= 4) return accountNumber;
    return '•••• ${accountNumber.substring(accountNumber.length - 4)}';
  }
}

/// A funding payment provider option shown on the Fund Wallet screen.
enum WalletPaymentProvider {
  paystackCard('Debit/Credit Card', 'Pay with your card via Paystack'),
  bankTransfer('Bank Transfer', 'Pay via bank transfer');

  const WalletPaymentProvider(this.label, this.subtitle);

  final String label;
  final String subtitle;
}

class TopUpResult {
  const TopUpResult({
    required this.reference,
    required this.authorizationUrl,
    required this.amountKobo,
  });

  final String reference;
  final String authorizationUrl;
  final int amountKobo;

  factory TopUpResult.fromJson(Map<String, dynamic> json) {
    return TopUpResult(
      reference: json['reference']?.toString() ?? '',
      authorizationUrl: json['authorization_url']?.toString() ?? '',
      amountKobo: _int(json['amount_kobo']),
    );
  }

  static int _int(Object? v) {
    if (v is int) return v;
    if (v is num) return v.toInt();
    return int.tryParse(v?.toString() ?? '') ?? 0;
  }
}
