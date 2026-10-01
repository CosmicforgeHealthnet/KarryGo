import 'dart:async';

import 'package:flutter/material.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../../../../core/theme/dispatch_palette.dart';
import '../../../../core/theme/theme_extensions.dart';

/// Shows the "Getting you online…" modal and waits for [task] to complete.
/// Returns whatever [task] returned.
Future<ApiResult<T>> showGoOnlineProcessing<T>(
  BuildContext context, {
  required Future<ApiResult<T>> task,
}) async {
  await showDialog<void>(
    context: context,
    barrierDismissible: false,
        barrierColor: Colors.black.withValues(alpha: 0.82),
    builder: (_) => _GoOnlineProcessingDialog(task: task),
  );
  return await task;
}

class _GoOnlineProcessingDialog extends StatefulWidget {
  const _GoOnlineProcessingDialog({required this.task});
  final Future<ApiResult<dynamic>> task;

  @override
  State<_GoOnlineProcessingDialog> createState() =>
      _GoOnlineProcessingDialogState();
}

class _GoOnlineProcessingDialogState extends State<_GoOnlineProcessingDialog>
    with SingleTickerProviderStateMixin {
  static const _steps = [
    'Checking your profile',
    'Verifying requirements',
    'Confirming location',
    'Connecting to dispatch',
  ];

  int _step = 0;
  Timer? _timer;
  late final AnimationController _pulse;

  @override
  void initState() {
    super.initState();
    _pulse = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1900),
    )..repeat();

    // Advance the checklist one row every 380 ms while the API/mock task
    // runs in parallel. Task completion short-circuits everything.
      _timer = Timer.periodic(const Duration(milliseconds: 800), (t) {
      if (!mounted || _step >= _steps.length - 1) {
        t.cancel();
        return;
      }
      setState(() => _step++);
    });

    widget.task.whenComplete(_onTaskComplete);
  }

  void _onTaskComplete() {
    if (!mounted) return;
    _timer?.cancel();
    setState(() => _step = _steps.length);
      Future.delayed(const Duration(milliseconds: 700), () {
      if (!mounted) return;
      Navigator.of(context).pop();
    });
  }

  @override
  void dispose() {
    _timer?.cancel();
    _pulse.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return Dialog(
      backgroundColor: Colors.transparent,
      insetPadding: const EdgeInsets.symmetric(horizontal: 24),
      child: Container(
        width: double.infinity,
        padding: const EdgeInsets.fromLTRB(24, 32, 24, 26),
        decoration: BoxDecoration(
          color: colors.surface,
          borderRadius: BorderRadius.circular(28),
          boxShadow: [
            BoxShadow(
              color: Colors.black.withValues(alpha: 0.14),
              blurRadius: 40,
              offset: const Offset(0, 18),
            ),
          ],
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            // ── Hero icon with ripple ────────────────────────────────
            _PulsingHero(controller: _pulse, colors: colors),
            const SizedBox(height: 26),

            // ── Title ────────────────────────────────────────────────
            Text(
              'Getting you online',
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 21,
                fontWeight: FontWeight.w800,
                color: colors.text,
                letterSpacing: -0.5,
                height: 1.15,
              ),
            ),
            const SizedBox(height: 8),
            Text(
              'This will only take a few seconds.',
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 13.5,
                fontWeight: FontWeight.w400,
                color: colors.textMuted,
                height: 1.45,
              ),
            ),

            const SizedBox(height: 30),

            // ── Checklist ────────────────────────────────────────────
            Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                for (int i = 0; i < _steps.length; i++) ...[
                  _StepRow(
                    label: _steps[i],
                    state: i < _step
                        ? _StepState.done
                        : (i == _step && _step < _steps.length
                            ? _StepState.inProgress
                            : _StepState.pending),
                  ),
                  if (i != _steps.length - 1) const SizedBox(height: 16),
                ],
              ],
            ),

            const SizedBox(height: 26),

            // ── Progress hairline ────────────────────────────────────
            _ProgressBar(
              value: _step >= _steps.length
                  ? 1.0
                  : (_step + 1) / (_steps.length + 1),
              color: colors.brandTeal500,
              trackColor: colors.navy10,
            ),
          ],
        ),
      ),
    );
  }
}

// ── Hero icon + animated pulse ──────────────────────────────────────────

class _PulsingHero extends StatelessWidget {
  const _PulsingHero({required this.controller, required this.colors});
  final AnimationController controller;
  final DispatchPalette colors;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 108,
      height: 108,
      child: AnimatedBuilder(
        animation: controller,
        builder: (context, _) {
          final t = controller.value;
          return Stack(
            alignment: Alignment.center,
            children: [
              // Outer ripple — expands and fades out.
              Container(
                width: 76 + (t * 30),
                height: 76 + (t * 30),
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: colors.brandTeal500.withValues(alpha: (1 - t) * 0.14),
                ),
              ),
              // Inner ripple — slightly behind the hero tile.
              Container(
                width: 68 + (t * 12),
                height: 68 + (t * 12),
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: colors.brandTeal500.withValues(alpha: (1 - t) * 0.10),
                ),
              ),
              // Icon tile
              Container(
                width: 64,
                height: 64,
                decoration: BoxDecoration(
                  gradient: LinearGradient(
                    begin: Alignment.topLeft,
                    end: Alignment.bottomRight,
                    colors: [
                      colors.brandNavy900,
                      colors.brandNavy900.withValues(alpha: 0.86),
                    ],
                  ),
                  borderRadius: BorderRadius.circular(20),
                  boxShadow: [
                    BoxShadow(
                      color: colors.brandNavy900.withValues(alpha: 0.28),
                      blurRadius: 18,
                      offset: const Offset(0, 8),
                    ),
                  ],
                ),
                child: const Icon(
                  Icons.satellite_alt_rounded,
                  size: 30,
                  color: Colors.white,
                ),
              ),
            ],
          );
        },
      ),
    );
  }
}

// ── Progress hairline ───────────────────────────────────────────────────

class _ProgressBar extends StatelessWidget {
  const _ProgressBar({
    required this.value,
    required this.color,
    required this.trackColor,
  });
  final double value;
  final Color color;
  final Color trackColor;

  @override
  Widget build(BuildContext context) {
    return ClipRRect(
      borderRadius: BorderRadius.circular(4),
      child: Stack(
        children: [
          Container(height: 4, color: trackColor),
          AnimatedFractionallySizedBox(
            duration: const Duration(milliseconds: 450),
            curve: Curves.easeOutCubic,
            widthFactor: value.clamp(0.0, 1.0),
            child: Container(
              height: 4,
              decoration: BoxDecoration(
                color: color,
                borderRadius: BorderRadius.circular(4),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

// ── Checklist row ───────────────────────────────────────────────────────

enum _StepState { pending, inProgress, done }

class _StepRow extends StatelessWidget {
  const _StepRow({required this.label, required this.state});
  final String label;
  final _StepState state;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        _LeadingDot(state: state, colors: colors),
        const SizedBox(width: 14),
        Expanded(
          child: Text(
            label,
            style: TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w600,
              color: state == _StepState.pending
                  ? colors.textMuted
                  : colors.text,
              height: 1.3,
              letterSpacing: -0.1,
            ),
          ),
        ),
      ],
    );
  }
}

class _LeadingDot extends StatelessWidget {
  const _LeadingDot({required this.state, required this.colors});
  final _StepState state;
  final DispatchPalette colors;

  @override
  Widget build(BuildContext context) {
    switch (state) {
      case _StepState.done:
        return Container(
          width: 26,
          height: 26,
          decoration: BoxDecoration(
            color: colors.success,
            shape: BoxShape.circle,
          ),
          child: const Icon(
            Icons.check_rounded,
            size: 16,
            color: Colors.white,
          ),
        );

      case _StepState.inProgress:
        return SizedBox(
          width: 26,
          height: 26,
          child: Padding(
            padding: const EdgeInsets.all(2.5),
            child: CircularProgressIndicator(
              strokeWidth: 2.6,
              strokeCap: StrokeCap.round,
              color: colors.brandTeal500,
            ),
          ),
        );

      case _StepState.pending:
        return Container(
          width: 26,
          height: 26,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            border: Border.all(color: colors.navy20, width: 2),
          ),
        );
    }
  }
}