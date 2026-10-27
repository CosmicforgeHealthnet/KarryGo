import 'dart:async';

import 'package:flutter/material.dart';

import '../../../../core/format/money_format.dart';
import '../../../../core/widgets/karryam_ui.dart';
import '../../models/booking_waiting.dart';
import 'provider_app_colors.dart';

/// Waiting time at pickup and drop-off on the active trip.
///
/// Loads the server's view when shown and whenever the trip status changes,
/// then ticks the running clock locally every second. [onArrivedAtDropoff] is
/// offered while the cargo is on its way and the drop-off clock has not
/// started.
class WaitingTimerCard extends StatefulWidget {
  const WaitingTimerCard({
    super.key,
    required this.status,
    required this.load,
    this.onArrivedAtDropoff,
    this.feeNote = false,
    this.isCash = false,
  });

  final String status;

  /// Adds a line saying how the waiting fee will be paid (on the completion
  /// screen); [isCash] says the driver collects it.
  final bool feeNote;
  final bool isCash;
  final Future<BookingWaiting?> Function() load;
  final Future<BookingWaiting?> Function()? onArrivedAtDropoff;

  @override
  State<WaitingTimerCard> createState() => _WaitingTimerCardState();
}

class _WaitingTimerCardState extends State<WaitingTimerCard> {
  BookingWaiting? _waiting;
  Timer? _ticker;
  Timer? _refresher;
  bool _arriving = false;

  @override
  void initState() {
    super.initState();
    _refresh();
    _ticker = Timer.periodic(const Duration(seconds: 1), (_) {
      if (mounted && _running != null) setState(() {});
    });
    // The server stays the authority: refresh now and then so the fee shown
    // never drifts from what will be charged.
    _refresher = Timer.periodic(const Duration(seconds: 60), (_) => _refresh());
  }

  @override
  void didUpdateWidget(WaitingTimerCard old) {
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

  Future<void> _arrive() async {
    final arrive = widget.onArrivedAtDropoff;
    if (arrive == null || _arriving) return;
    setState(() => _arriving = true);
    final waiting = await arrive();
    if (!mounted) return;
    setState(() {
      _arriving = false;
      if (waiting != null) _waiting = waiting;
    });
  }

  WaitingStop? get _running {
    final w = _waiting;
    if (w == null) return null;
    if (w.dropoff.running) return w.dropoff;
    if (w.pickup.running) return w.pickup;
    return null;
  }

  @override
  Widget build(BuildContext context) {
    final w = _waiting;
    final cargoAboard = widget.status == 'picked_up' || widget.status == 'en_route_delivery';
    final canArrive = cargoAboard && widget.onArrivedAtDropoff != null && !(w?.dropoff.reached ?? false);

    if (w == null) {
      if (!canArrive) return const SizedBox.shrink();
      return _ArriveButton(loading: _arriving, onTap: _arrive);
    }

    final running = _running;
    final children = <Widget>[];
    if (running != null) {
      children.add(_RunningClock(waiting: w, stop: running));
    }
    for (final stop in [w.pickup, w.dropoff]) {
      if (stop.reached && !stop.running) {
        children.add(_ClosedStopRow(stop: stop));
      }
    }
    if (canArrive) {
      children.add(_ArriveButton(loading: _arriving, onTap: _arrive));
    }
    if (widget.feeNote) {
      var total = 0;
      for (final stop in [w.pickup, w.dropoff]) {
        if (stop.running && stop.arrivedAt != null) {
          total += w.feeFor(DateTime.now().difference(stop.arrivedAt!));
        } else {
          total += stop.feeKobo;
        }
      }
      if (total > 0) {
        final amount = formatNairaShort(total / 100);
        children.add(KNotice(
          widget.isCash
              ? 'Waiting fee: $amount. Collect it in cash with the fare.'
              : "Waiting fee: $amount. It is charged to the customer's wallet and paid to you, "
                  'minus the usual commission.',
          tone: KTone.warning,
          icon: Icons.timer_outlined,
        ));
      }
    }
    if (children.isEmpty) return const SizedBox.shrink();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        for (var i = 0; i < children.length; i++) ...[
          if (i > 0) const SizedBox(height: 10),
          children[i],
        ],
      ],
    );
  }
}

String _stopLabel(String stop) => stop == 'DROPOFF' ? 'drop-off' : 'pickup';

String formatWaited(Duration d) {
  final h = d.inHours;
  final m = d.inMinutes.remainder(60);
  final s = d.inSeconds.remainder(60);
  String two(int n) => n.toString().padLeft(2, '0');
  return h > 0 ? '$h:${two(m)}:${two(s)}' : '${two(m)}:${two(s)}';
}

class _RunningClock extends StatelessWidget {
  const _RunningClock({required this.waiting, required this.stop});
  final BookingWaiting waiting;
  final WaitingStop stop;

  @override
  Widget build(BuildContext context) {
    final now = DateTime.now();
    final waited = stop.arrivedAt == null ? Duration.zero : now.difference(stop.arrivedAt!);
    final fee = waiting.feeFor(waited);
    final freeUntil = stop.freeUntil;
    final inFreeTime = freeUntil != null && now.isBefore(freeUntil);
    final note = inFreeTime
        ? 'Free for ${freeUntil.difference(now).inMinutes + 1} more min. Then '
            '${formatNairaShort(waiting.blockFeeKobo / 100)} per ${waiting.blockMinutes} min.'
        : 'Charged ${formatNairaShort(waiting.blockFeeKobo / 100)} per started ${waiting.blockMinutes} min.';

    return KCard(
      child: Row(
        children: [
          Container(
            width: 44,
            height: 44,
            decoration: BoxDecoration(
              color: inFreeTime ? kProviderGreenSoft : kWarningTint,
              shape: BoxShape.circle,
            ),
            child: Icon(Icons.timer_outlined, color: inFreeTime ? kTealText : kWarning),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'Waiting at ${_stopLabel(stop.stop)}',
                  style: TextStyle(fontSize: 13, color: kProviderMuted),
                ),
                const SizedBox(height: 2),
                Text(
                  formatWaited(waited),
                  style: TextStyle(
                    fontSize: 22,
                    fontWeight: FontWeight.w800,
                    color: kProviderText,
                    fontFeatures: const [FontFeature.tabularFigures()],
                  ),
                ),
                const SizedBox(height: 2),
                Text(note, style: TextStyle(fontSize: 12, color: kProviderMuted, height: 1.35)),
              ],
            ),
          ),
          const SizedBox(width: 10),
          Text(
            formatNairaShort(fee / 100),
            style: TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w800,
              color: fee > 0 ? kWarning : kTealText,
            ),
          ),
        ],
      ),
    );
  }
}

class _ClosedStopRow extends StatelessWidget {
  const _ClosedStopRow({required this.stop});
  final WaitingStop stop;

  @override
  Widget build(BuildContext context) {
    final waited = Duration(seconds: stop.waitedSeconds);
    return Row(
      children: [
        Icon(Icons.timer_outlined, size: 18, color: kProviderMuted),
        const SizedBox(width: 8),
        Expanded(
          child: Text(
            'Waited ${formatWaited(waited)} at ${_stopLabel(stop.stop)}',
            style: TextStyle(fontSize: 13, color: kProviderMuted),
          ),
        ),
        Text(
          stop.feeKobo > 0 ? '+${formatNairaShort(stop.feeKobo / 100)}' : 'Free',
          style: TextStyle(fontSize: 13, fontWeight: FontWeight.w700, color: kProviderText),
        ),
      ],
    );
  }
}

class _ArriveButton extends StatelessWidget {
  const _ArriveButton({required this.loading, required this.onTap});
  final bool loading;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return OutlinedButton.icon(
      onPressed: loading ? null : onTap,
      icon: loading
          ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
          : const Icon(Icons.flag_outlined),
      label: const Text("I've arrived at drop-off"),
      style: OutlinedButton.styleFrom(
        minimumSize: const Size.fromHeight(48),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
      ),
    );
  }
}
