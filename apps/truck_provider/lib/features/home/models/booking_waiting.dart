/// Waiting time at pickup and drop-off, as the server computes it.
///
/// The server is the authority on the fee; the app only ticks the running
/// clock forward between refreshes so the timer moves every second.
class WaitingStop {
  const WaitingStop({
    required this.stop,
    this.arrivedAt,
    this.leftAt,
    this.freeUntil,
    required this.waitedSeconds,
    required this.chargeableBlocks,
    required this.feeKobo,
    required this.running,
  });

  final String stop;
  final DateTime? arrivedAt;
  final DateTime? leftAt;
  final DateTime? freeUntil;
  final int waitedSeconds;
  final int chargeableBlocks;
  final int feeKobo;
  final bool running;

  bool get reached => arrivedAt != null;

  factory WaitingStop.fromJson(Map<String, dynamic> j) => WaitingStop(
        stop: j['stop'] as String? ?? '',
        arrivedAt: DateTime.tryParse(j['arrived_at'] as String? ?? '')?.toLocal(),
        leftAt: DateTime.tryParse(j['left_at'] as String? ?? '')?.toLocal(),
        freeUntil: DateTime.tryParse(j['free_until'] as String? ?? '')?.toLocal(),
        waitedSeconds: (j['waited_seconds'] as num?)?.toInt() ?? 0,
        chargeableBlocks: (j['chargeable_blocks'] as num?)?.toInt() ?? 0,
        feeKobo: (j['fee_kobo'] as num?)?.toInt() ?? 0,
        running: j['running'] as bool? ?? false,
      );
}

class BookingWaiting {
  const BookingWaiting({
    required this.freeMinutes,
    required this.blockMinutes,
    required this.blockFeeKobo,
    required this.pickup,
    required this.dropoff,
    required this.totalFeeKobo,
    required this.feeStatus,
  });

  final int freeMinutes;
  final int blockMinutes;
  final int blockFeeKobo;
  final WaitingStop pickup;
  final WaitingStop dropoff;
  final int totalFeeKobo;

  /// PENDING, PAID, OWED, CASH, NONE or NOT_APPLICABLE.
  final String feeStatus;

  static const empty = WaitingStop(stop: '', waitedSeconds: 0, chargeableBlocks: 0, feeKobo: 0, running: false);

  factory BookingWaiting.fromJson(Map<String, dynamic> j) {
    final stops = (j['stops'] as List? ?? const [])
        .whereType<Map>()
        .map((m) => WaitingStop.fromJson(Map<String, dynamic>.from(m)))
        .toList();
    WaitingStop find(String name) => stops.firstWhere((s) => s.stop == name, orElse: () => empty);
    return BookingWaiting(
      freeMinutes: (j['free_minutes'] as num?)?.toInt() ?? 0,
      blockMinutes: (j['block_minutes'] as num?)?.toInt() ?? 0,
      blockFeeKobo: (j['block_fee_kobo'] as num?)?.toInt() ?? 0,
      pickup: find('PICKUP'),
      dropoff: find('DROPOFF'),
      totalFeeKobo: (j['total_fee_kobo'] as num?)?.toInt() ?? 0,
      feeStatus: j['fee_status'] as String? ?? '',
    );
  }

  /// The fee a running stop would cost after waiting [waited] in total, using
  /// the same rule as the server: free time, then every started block.
  int feeFor(Duration waited) {
    if (blockMinutes <= 0) return 0;
    final over = waited - Duration(minutes: freeMinutes);
    if (over <= Duration.zero) return 0;
    final block = Duration(minutes: blockMinutes);
    final blocks = (over.inSeconds + block.inSeconds - 1) ~/ block.inSeconds;
    return blocks * blockFeeKobo;
  }
}
