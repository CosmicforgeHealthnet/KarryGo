/// Mock data for every screen in the Wallet tab.
/// Delete this file when the wallet tab goes fully live.
library;

class WalletTabMock {
  // ── 01 Wallet home ─────────────────────────────────────────────────
  static const double balanceNaira = 124500.00;
  static const double pendingNaira = 8200.00;
  static const double todayEarningsNaira = 12400.00;

  static const List<ActivityItem> recentActivity = [
    ActivityItem(
      id: 'TRX-482731',
      title: 'Delivery payment',
      subtitle: 'Today, 10:24 AM',
      amountNaira: 12000.00,
      isCredit: true,
      kind: ActivityKind.delivery,
    ),
    ActivityItem(
      id: 'TRX-482730',
      title: 'Withdrawal',
      subtitle: 'Tue, 28 May',
      amountNaira: 50000.00,
      isCredit: false,
      kind: ActivityKind.withdrawal,
    ),
    ActivityItem(
      id: 'ADJ-782931',
      title: 'Adjustment',
      subtitle: 'Mon, 27 May',
      amountNaira: 2500.00,
      isCredit: true,
      kind: ActivityKind.adjustment,
    ),
    ActivityItem(
      id: 'TRX-482728',
      title: 'Delivery payment',
      subtitle: '25 May, 4:18 PM',
      amountNaira: 8500.00,
      isCredit: true,
      kind: ActivityKind.delivery,
    ),
    ActivityItem(
      id: 'TRX-482727',
      title: 'Store delivery',
      subtitle: '24 May, 9:12 AM',
      amountNaira: 15000.00,
      isCredit: true,
      kind: ActivityKind.store,
    ),
  ];

  // ── 04 Transaction list ────────────────────────────────────────────
  static const List<TransactionItem> allTransactions = [
    TransactionItem(
      id: 'TRX-482731',
      title: 'Delivery payment',
      subtitle: 'Today, 10:24 AM',
      monthLabel: 'May 2024',
      amountNaira: 12000.00,
      isCredit: true,
      kind: TransactionKind.delivery,
    ),
    TransactionItem(
      id: 'TRX-482730',
      title: 'Withdrawal',
      subtitle: 'Tue, 28 May',
      monthLabel: 'May 2024',
      amountNaira: 50000.00,
      isCredit: false,
      kind: TransactionKind.withdrawal,
    ),
    TransactionItem(
      id: 'ADJ-782931',
      title: 'Adjustment',
      subtitle: 'Mon, 27 May',
      monthLabel: 'May 2024',
      amountNaira: 2500.00,
      isCredit: true,
      kind: TransactionKind.adjustment,
    ),
    TransactionItem(
      id: 'TRX-482728',
      title: 'Delivery payment',
      subtitle: '25 May, 4:18 PM',
      monthLabel: 'May 2024',
      amountNaira: 8500.00,
      isCredit: true,
      kind: TransactionKind.delivery,
    ),
    TransactionItem(
      id: 'TRX-482727',
      title: 'Store delivery',
      subtitle: '24 May, 9:12 AM',
      monthLabel: 'May 2024',
      amountNaira: 15000.00,
      isCredit: true,
      kind: TransactionKind.store,
    ),
    // Add earlier months to demonstrate grouping
    TransactionItem(
      id: 'TRX-482600',
      title: 'Delivery payment',
      subtitle: '28 Apr, 5:10 PM',
      monthLabel: 'April 2024',
      amountNaira: 9000.00,
      isCredit: true,
      kind: TransactionKind.delivery,
    ),
    TransactionItem(
      id: 'TRX-482599',
      title: 'Withdrawal',
      subtitle: '15 Apr, 11:02 AM',
      monthLabel: 'April 2024',
      amountNaira: 30000.00,
      isCredit: false,
      kind: TransactionKind.withdrawal,
    ),
  ];

  // ── 05 Transaction detail ──────────────────────────────────────────
  static const TransactionDetail transactionDetail = TransactionDetail(
    id: 'TRX-482731',
    title: 'Delivery payment',
    date: '28 May 2024, 10:24 AM',
    amountNaira: 12000.00,
    feeNaira: 0.00,
    status: 'Completed',
    reference: 'KAR-284731',
    type: 'Delivery to Victoria Island',
    from: 'Karryam',
  );

  // ── 07 Earnings summary ────────────────────────────────────────────
  static const double totalEarningsNaira = 424350.00;
  static const int deliveryCount = 48;
  static const double avgPerDeliveryNaira = 8840.00;
  static const Map<String, double> earningsByMonth = {
    'Jan': 62000.00,
    'Feb': 71000.00,
    'Mar': 84000.00,
    'Apr': 78000.00,
    'May': 129350.00,
  };

  // ── 08 Period filter ───────────────────────────────────────────────
  static const List<String> periods = [
    'This week',
    'This month',
    'Last 3 months',
    'This year',
    'Custom range',
  ];

  // ── 09 Earnings details ────────────────────────────────────────────
  static const List<DailyEarning> dailyBreakdown = [
    DailyEarning(date: '28 May 2024', deliveries: 3, amountNaira: 24000.00),
    DailyEarning(date: '27 May 2024', deliveries: 2, amountNaira: 18000.00),
    DailyEarning(date: '26 May 2024', deliveries: 4, amountNaira: 32000.00),
    DailyEarning(date: '25 May 2024', deliveries: 4, amountNaira: 28500.00),
    DailyEarning(date: '24 May 2024', deliveries: 2, amountNaira: 16000.00),
  ];

  // ── 10 Adjustment detail ───────────────────────────────────────────
  static const AdjustmentDetail adjustmentDetail = AdjustmentDetail(
    reference: 'ADJ-782931',
    date: '27 May 2024, 3:21 PM',
    amountNaira: 2500.00,
    type: 'Adjustment',
    reason: 'Customer tip',
    status: 'Completed',
  );

  // ── 13 Bank accounts ───────────────────────────────────────────────
  static const List<BankAccount> bankAccounts = [
    BankAccount(
      id: 'bank_1',
      bankName: 'Access Bank',
      accountNumber: '**** 4567',
      accountName: 'Daniel Okafor',
      isDefault: true,
      needsAttention: false,
    ),
    BankAccount(
      id: 'bank_2',
      bankName: 'GTBank',
      accountNumber: '**** 1234',
      accountName: 'Daniel Okafor',
      isDefault: false,
      needsAttention: true,
    ),
  ];

  // ── 19-21 Withdrawal ───────────────────────────────────────────────
  static const double withdrawalFeeNaira = 50.00;
  static const double minWithdrawalNaira = 500.00;

  // ── 26 Success ─────────────────────────────────────────────────────
  static const WithdrawalReceipt lastReceipt = WithdrawalReceipt(
    amountNaira: 50000.00,
    feeNaira: 50.00,
    netNaira: 49950.00,
    bankName: 'Access Bank',
    accountNumber: '**** 4567',
    reference: 'WDR-892731',
  );
}

// ── Data classes ─────────────────────────────────────────────────────

enum ActivityKind { delivery, withdrawal, adjustment, store }

class ActivityItem {
  const ActivityItem({
    required this.id,
    required this.title,
    required this.subtitle,
    required this.amountNaira,
    required this.isCredit,
    required this.kind,
  });

  final String id;
  final String title;
  final String subtitle;
  final double amountNaira;
  final bool isCredit;
  final ActivityKind kind;
}

enum TransactionKind { delivery, withdrawal, adjustment, store }

class TransactionItem {
  const TransactionItem({
    required this.id,
    required this.title,
    required this.subtitle,
    required this.monthLabel,
    required this.amountNaira,
    required this.isCredit,
    required this.kind,
  });

  final String id;
  final String title;
  final String subtitle;
  final String monthLabel;
  final double amountNaira;
  final bool isCredit;
  final TransactionKind kind;
}

class TransactionDetail {
  const TransactionDetail({
    required this.id,
    required this.title,
    required this.date,
    required this.amountNaira,
    required this.feeNaira,
    required this.status,
    required this.reference,
    required this.type,
    required this.from,
  });

  final String id;
  final String title;
  final String date;
  final double amountNaira;
  final double feeNaira;
  final String status;
  final String reference;
  final String type;
  final String from;
}

class DailyEarning {
  const DailyEarning({
    required this.date,
    required this.deliveries,
    required this.amountNaira,
  });

  final String date;
  final int deliveries;
  final double amountNaira;
}

class AdjustmentDetail {
  const AdjustmentDetail({
    required this.reference,
    required this.date,
    required this.amountNaira,
    required this.type,
    required this.reason,
    required this.status,
  });

  final String reference;
  final String date;
  final double amountNaira;
  final String type;
  final String reason;
  final String status;
}

class BankAccount {
  const BankAccount({
    required this.id,
    required this.bankName,
    required this.accountNumber,
    required this.accountName,
    required this.isDefault,
    required this.needsAttention,
  });

  final String id;
  final String bankName;
  final String accountNumber;
  final String accountName;
  final bool isDefault;
  final bool needsAttention;
}

class WithdrawalReceipt {
  const WithdrawalReceipt({
    required this.amountNaira,
    required this.feeNaira,
    required this.netNaira,
    required this.bankName,
    required this.accountNumber,
    required this.reference,
  });

  final double amountNaira;
  final double feeNaira;
  final double netNaira;
  final String bankName;
  final String accountNumber;
  final String reference;
}