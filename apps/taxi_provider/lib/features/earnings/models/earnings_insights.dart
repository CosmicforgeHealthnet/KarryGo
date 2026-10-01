import 'package:flutter/foundation.dart';

/// The driver's own report over a window, from
/// `GET /provider/earnings/insights`.
///
/// Gross earnings here are trip-fare reporting from the taxi service's own
/// booking rows — NOT a wallet balance. Taxi has no provider wallet surface
/// yet, so nothing in this app claims a withdrawable figure.
@immutable
class EarningsInsights {
  const EarningsInsights({
    required this.tripsCompleted,
    required this.grossEarningsKobo,
    required this.cancelledByCustomer,
    required this.cancelledByProvider,
    required this.onlineTimeSeconds,
    required this.activeTripTimeSeconds,
    this.averageTripDurationMinutes,
    this.utilizationPercent,
    this.earningsPerOnlineHourKobo,
    this.isLedger = false,
  });

  final int tripsCompleted;
  final int grossEarningsKobo;
  final int cancelledByCustomer;
  final int cancelledByProvider;
  final double onlineTimeSeconds;
  final double activeTripTimeSeconds;
  final double? averageTripDurationMinutes;
  final double? utilizationPercent;
  final double? earningsPerOnlineHourKobo;

  /// The service flags whether these numbers came from a real ledger. They do
  /// not today, so the UI says so rather than implying a settled balance.
  final bool isLedger;

  double get onlineHours => onlineTimeSeconds / 3600;

  factory EarningsInsights.fromJson(Map<String, dynamic> json) {
    double? optDouble(String key) => (json[key] as num?)?.toDouble();
    return EarningsInsights(
      tripsCompleted: (json['trips_completed'] as num?)?.toInt() ?? 0,
      grossEarningsKobo: (json['gross_earnings_kobo'] as num?)?.toInt() ?? 0,
      cancelledByCustomer:
          (json['cancelled_by_customer_count'] as num?)?.toInt() ?? 0,
      cancelledByProvider:
          (json['cancelled_by_provider_count'] as num?)?.toInt() ?? 0,
      onlineTimeSeconds: optDouble('online_time_seconds') ?? 0,
      activeTripTimeSeconds: optDouble('active_trip_time_seconds') ?? 0,
      averageTripDurationMinutes: optDouble('average_trip_duration_minutes'),
      utilizationPercent: optDouble('utilization_percent'),
      earningsPerOnlineHourKobo: optDouble('earnings_per_online_hour_kobo'),
      isLedger: json['is_ledger'] == true,
    );
  }
}
