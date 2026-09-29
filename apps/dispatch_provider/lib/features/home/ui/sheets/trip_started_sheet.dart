import 'package:flutter/material.dart';

import '../../../requests/data/request_model.dart';
import '../../../../core/theme/theme_extensions.dart';
import '../widgets/trip_peek_bar.dart';

class TripStartedSheet extends StatelessWidget {
  const TripStartedSheet({
    super.key,
    required this.request,
    required this.onEndTrip,
    required this.collapsed,
    required this.onToggleCollapse,
    this.peekStatusText = '',
    this.onReportFailure,
  });

  final RequestModel request;
  final VoidCallback onEndTrip;
  final bool collapsed;
  final VoidCallback onToggleCollapse;
  final String peekStatusText;

  /// When provided, a "Report delivery failure" link renders below
  /// Complete delivery. Omit to hide it.
  final VoidCallback? onReportFailure;

  @override
  Widget build(BuildContext context) {
    if (collapsed) {
      return TripPeekBar(
        title: "You're on a delivery",
        statusText: peekStatusText,
        onExpand: onToggleCollapse,
      );
    }

    final colors = context.palette;
    final bottomPad = MediaQuery.of(context).padding.bottom;
    final maxHeight = MediaQuery.of(context).size.height * 0.78;

    return Container(
      margin: EdgeInsets.fromLTRB(12, 0, 12, 12 + bottomPad),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(24),
        boxShadow: colors.cardShadow,
      ),
      clipBehavior: Clip.antiAlias,
      constraints: BoxConstraints(maxHeight: maxHeight),
      child: SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(20, 18, 20, 18),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Align(
              alignment: Alignment.centerRight,
              child: GestureDetector(
                onTap: onToggleCollapse,
                behavior: HitTestBehavior.opaque,
                child: Container(
                  width: 36,
                  height: 36,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(
                    color: colors.navy10,
                    shape: BoxShape.circle,
                  ),
                  child: Icon(
                    Icons.keyboard_arrow_down_rounded,
                    size: 22,
                    color: colors.textMuted,
                  ),
                ),
              ),
            ),

            Text(
              "You're on a delivery",
              style: TextStyle(
                fontSize: 19,
                fontWeight: FontWeight.w800,
                color: colors.text,
                letterSpacing: -0.4,
                height: 1.15,
              ),
            ),
            const SizedBox(height: 5),
            Text(
              'Status updates automatically while you complete this mission.',
              style: TextStyle(
                fontSize: 13.5,
                fontWeight: FontWeight.w400,
                color: colors.textMuted,
                height: 1.4,
              ),
            ),

            const SizedBox(height: 20),

            Container(
              padding: const EdgeInsets.fromLTRB(16, 18, 16, 18),
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(16),
                border: Border.all(color: colors.divider),
              ),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  _TimelineStep(
                    status: _StepStatus.done,
                    label: 'Picked up',
                    value: request.pickupAddress,
                    isFirst: true,
                  ),
                  const SizedBox(height: 2),
                  _TimelineStep(
                    status: _StepStatus.done,
                    label: 'On the way',
                    value: request.dropoffAddress,
                  ),
                  const SizedBox(height: 2),
                  _TimelineStep(
                    status: _StepStatus.pending,
                    label: 'Complete delivery',
                    value: 'Arriving soon',
                    isLast: true,
                  ),
                ],
              ),
            ),

            const SizedBox(height: 20),

            SizedBox(
              height: 52,
              child: FilledButton(
                onPressed: onEndTrip,
                style: FilledButton.styleFrom(
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(999),
                  ),
                ),
                child: const Text(
                  'Complete delivery',
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
            ),

            if (onReportFailure != null) ...[
              const SizedBox(height: 8),
              SizedBox(
                width: double.infinity,
                height: 40,
                child: TextButton(
                  onPressed: onReportFailure,
                  style: TextButton.styleFrom(
                    foregroundColor: colors.warning,
                    minimumSize: const Size(double.infinity, 40),
                    textStyle: const TextStyle(
                      fontSize: 13.5,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('Report delivery failure'),
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

enum _StepStatus { done, pending }

class _TimelineStep extends StatelessWidget {
  const _TimelineStep({
    required this.status,
    required this.label,
    required this.value,
    this.isFirst = false,
    this.isLast = false,
  });

  final _StepStatus status;
  final String label;
  final String value;
  final bool isFirst;
  final bool isLast;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final isDone = status == _StepStatus.done;
    final accent = isDone ? colors.success : colors.navy40;

    return IntrinsicHeight(
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 22,
            child: Column(
              children: [
                SizedBox(
                  height: isFirst ? 4 : 12,
                  child: isFirst
                      ? null
                      : Center(
                          child: Container(
                            width: 2,
                            color: isDone ? colors.success : colors.navy20,
                          ),
                        ),
                ),
                Container(
                  width: 20,
                  height: 20,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(
                    color: isDone ? accent : Colors.transparent,
                    shape: BoxShape.circle,
                    border: isDone
                        ? null
                        : Border.all(color: accent, width: 2),
                  ),
                  child: isDone
                      ? const Icon(
                          Icons.check_rounded,
                          size: 13,
                          color: Colors.white,
                        )
                      : null,
                ),
                if (!isLast)
                  Expanded(
                    child: Center(
                      child: Container(width: 2, color: colors.navy20),
                    ),
                  ),
              ],
            ),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Padding(
              padding: EdgeInsets.only(bottom: isLast ? 0 : 22, top: 1),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    label,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                      color: isDone ? colors.text : colors.textMuted,
                      letterSpacing: -0.1,
                    ),
                  ),
                  const SizedBox(height: 3),
                  Text(
                    value,
                    style: TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w400,
                      color: colors.textMuted,
                      height: 1.4,
                    ),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}