import 'package:flutter/foundation.dart';

/// A single row in the provider's recent-transactions list.
@immutable
class EarningsTransaction {
  const EarningsTransaction({
    required this.id,
    required this.bookingId,
    required this.kind,
    required this.title,
    required this.subtitle,
    required this.amountKobo,
    required this.status,
    required this.isTrip,
    required this.occurredAt,
  });

  final String id;
  final String bookingId;

  /// `credit` (earned from a trip) or `withdrawal`.
  final String kind;
  final String title;
  final String subtitle;
  final int amountKobo;

  /// `completed`, `pending`, or `failed`.
  final String status;
  final bool isTrip;
  final DateTime occurredAt;

  double get amountNaira => amountKobo / 100;

  bool get isCredit => kind == EarningsTransaction.kindCredit;

  static const kindCredit = 'credit';
  static const kindWithdrawal = 'withdrawal';

  static const statusCompleted = 'completed';
  static const statusPending = 'pending';
  static const statusFailed = 'failed';

  factory EarningsTransaction.fromJson(Map<String, dynamic> j) => EarningsTransaction(
        id: j['id'] as String? ?? '',
        bookingId: j['booking_id'] as String? ?? '',
        kind: j['kind'] as String? ?? kindCredit,
        title: j['title'] as String? ?? '',
        subtitle: j['subtitle'] as String? ?? '',
        amountKobo: (j['amount_kobo'] as num? ?? 0).toInt(),
        status: j['status'] as String? ?? statusCompleted,
        isTrip: j['is_trip'] as bool? ?? false,
        occurredAt: DateTime.tryParse(j['occurred_at'] as String? ?? '')?.toLocal() ??
            DateTime.fromMillisecondsSinceEpoch(0),
      );
}

/// Trip-earnings projection backing the provider Earnings/Wallet screen.
@immutable
class ProviderEarnings {
  const ProviderEarnings({
    required this.availableBalanceKobo,
    required this.pendingBalanceKobo,
    required this.todayEarningsKobo,
    required this.tripsCompletedToday,
    required this.hoursOnline,
    required this.totalEarningsKobo,
    required this.summaryYear,
    required this.monthlyEarningsKobo,
    required this.transactions,
  });

  final int availableBalanceKobo;
  final int pendingBalanceKobo;
  final int todayEarningsKobo;
  final int tripsCompletedToday;
  final int hoursOnline;
  final int totalEarningsKobo;
  final int summaryYear;

  /// Jan..Dec (length 12) completed-trip earnings in kobo for [summaryYear].
  final List<int> monthlyEarningsKobo;
  final List<EarningsTransaction> transactions;

  double get availableBalanceNaira => availableBalanceKobo / 100;
  double get pendingBalanceNaira => pendingBalanceKobo / 100;
  double get todayEarningsNaira => todayEarningsKobo / 100;
  double get totalEarningsNaira => totalEarningsKobo / 100;

  static const empty = ProviderEarnings(
    availableBalanceKobo: 0,
    pendingBalanceKobo: 0,
    todayEarningsKobo: 0,
    tripsCompletedToday: 0,
    hoursOnline: 0,
    totalEarningsKobo: 0,
    summaryYear: 0,
    monthlyEarningsKobo: [],
    transactions: [],
  );

  factory ProviderEarnings.fromJson(Map<String, dynamic> j) {
    final rawTxns = j['transactions'];
    final txns = rawTxns is List
        ? rawTxns
            .map((e) => EarningsTransaction.fromJson(Map<String, dynamic>.from(e as Map)))
            .toList()
        : <EarningsTransaction>[];
    final rawMonthly = j['monthly_earnings_kobo'];
    final monthly = rawMonthly is List
        ? rawMonthly.map((e) => (e as num? ?? 0).toInt()).toList()
        : <int>[];
    return ProviderEarnings(
      availableBalanceKobo: (j['available_balance_kobo'] as num? ?? 0).toInt(),
      pendingBalanceKobo: (j['pending_balance_kobo'] as num? ?? 0).toInt(),
      todayEarningsKobo: (j['today_earnings_kobo'] as num? ?? 0).toInt(),
      tripsCompletedToday: (j['trips_completed_today'] as num? ?? 0).toInt(),
      hoursOnline: (j['hours_online'] as num? ?? 0).toInt(),
      totalEarningsKobo: (j['total_earnings_kobo'] as num? ?? 0).toInt(),
      summaryYear: (j['summary_year'] as num? ?? 0).toInt(),
      monthlyEarningsKobo: monthly,
      transactions: txns,
    );
  }
}

/// Buckets transactions into the "Today / Yesterday / Last Week / Earlier"
/// groups the Earnings screen renders, preserving the server's recency order.
class EarningsTransactionGroup {
  EarningsTransactionGroup(this.label, this.items);
  final String label;
  final List<EarningsTransaction> items;

  static List<EarningsTransactionGroup> group(
    List<EarningsTransaction> txns, {
    DateTime? now,
  }) {
    final reference = now ?? DateTime.now();
    final today = DateTime(reference.year, reference.month, reference.day);
    final yesterday = today.subtract(const Duration(days: 1));
    final weekStart = today.subtract(const Duration(days: 7));

    final todayItems = <EarningsTransaction>[];
    final yesterdayItems = <EarningsTransaction>[];
    final lastWeekItems = <EarningsTransaction>[];
    final earlierItems = <EarningsTransaction>[];

    for (final t in txns) {
      final d = DateTime(t.occurredAt.year, t.occurredAt.month, t.occurredAt.day);
      if (d == today) {
        todayItems.add(t);
      } else if (d == yesterday) {
        yesterdayItems.add(t);
      } else if (d.isAfter(weekStart)) {
        lastWeekItems.add(t);
      } else {
        earlierItems.add(t);
      }
    }

    return [
      if (todayItems.isNotEmpty) EarningsTransactionGroup('Today', todayItems),
      if (yesterdayItems.isNotEmpty) EarningsTransactionGroup('Yesterday', yesterdayItems),
      if (lastWeekItems.isNotEmpty) EarningsTransactionGroup('Last Week', lastWeekItems),
      if (earlierItems.isNotEmpty) EarningsTransactionGroup('Earlier', earlierItems),
    ];
  }
}
