import 'dart:async';

import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/hauling_models.dart';
import 'hauling_trip_sheet_widgets.dart';

/// The driver's waiting time at pickup or drop-off, with a note before the
/// free time runs out. Loads when shown and when the trip status changes,
/// then ticks locally every second; hidden while the driver isn't waiting.
class HaulingWaitingCard extends StatefulWidget {
  const HaulingWaitingCard({super.key, required this.status, required this.load});

  final Object status;
  final Future<HaulingWaiting?> Function() load;

  @override
  State<HaulingWaitingCard> createState() => _HaulingWaitingCardState();
}

class _HaulingWaitingCardState extends State<HaulingWaitingCard> {
  HaulingWaiting? _waiting;
  Timer? _ticker;
  Timer? _refresher;

  @override
  void initState() {
    super.initState();
    _refresh();
    _ticker = Timer.periodic(const Duration(seconds: 1), (_) {
      if (mounted && _waiting?.running != null) setState(() {});
    });
    // The drop-off clock is started by the driver, so look again now and then.
    _refresher = Timer.periodic(const Duration(seconds: 30), (_) => _refresh());
  }

  @override
  void didUpdateWidget(HaulingWaitingCard old) {
    super.didUpdateWidget(old);
    if (old.status != widget.status) _refresh();
  }

  @override
  void dispose() {
    _ticker?.cancel();
    _refresher?.cancel();
    super.dispose();
  }

  Future<void> _refresh() async {
    final waiting = await widget.load();
    if (mounted && waiting != null) setState(() => _waiting = waiting);
  }

  @override
  Widget build(BuildContext context) {
    final w = _waiting;
    final stop = w?.running;
    if (w == null || stop == null || stop.arrivedAt == null) return const SizedBox.shrink();

    final now = DateTime.now();
    final waited = now.difference(stop.arrivedAt!);
    final fee = w.feeFor(waited);
    final freeUntil = stop.freeUntil;
    final inFreeTime = freeUntil != null && now.isBefore(freeUntil);
    final where = stop.stop == 'DROPOFF' ? 'drop-off' : 'pickup';
    final rate = '${formatHaulingFare(w.blockFeeKobo)} per ${w.blockMinutes} min';
    final note = inFreeTime
        ? 'Free for ${freeUntil.difference(now).inMinutes + 1} more min. After that, waiting costs $rate.'
        : 'Waiting is now charged at $rate, added to your trip.';

    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: inFreeTime ? CustomerFigmaColors.successSoft : const Color(0xFFFFF4DC),
        borderRadius: BorderRadius.circular(14),
      ),
      child: Row(
        children: [
          Icon(Icons.timer_outlined, color: inFreeTime ? CustomerFigmaColors.success : const Color(0xFF805500)),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'Driver waiting at $where · ${_clock(waited)}',
                  style: const TextStyle(fontSize: 14, fontWeight: FontWeight.w700, color: CustomerFigmaColors.text),
                ),
                const SizedBox(height: 4),
                Text(note, style: const TextStyle(fontSize: 12, height: 1.35, color: CustomerFigmaColors.muted)),
              ],
            ),
          ),
          if (fee > 0) ...[
            const SizedBox(width: 10),
            Text(
              formatHaulingFare(fee),
              style: const TextStyle(fontSize: 14, fontWeight: FontWeight.w800, color: Color(0xFF805500)),
            ),
          ],
        ],
      ),
    );
  }

  static String _clock(Duration d) {
    String two(int n) => n.toString().padLeft(2, '0');
    final h = d.inHours;
    final m = two(d.inMinutes.remainder(60));
    final s = two(d.inSeconds.remainder(60));
    return h > 0 ? '$h:$m:$s' : '$m:$s';
  }
}
