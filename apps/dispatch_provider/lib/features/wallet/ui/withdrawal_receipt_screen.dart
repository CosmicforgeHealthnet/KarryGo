import 'dart:ui' as ui;

import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';

class WithdrawalReceiptScreen extends StatefulWidget {
  const WithdrawalReceiptScreen({super.key, required this.amount});

  final double amount;

  @override
  State<WithdrawalReceiptScreen> createState() =>
      _WithdrawalReceiptScreenState();
}

class _WithdrawalReceiptScreenState extends State<WithdrawalReceiptScreen>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller;
  late final Animation<double> _circleScale;
  late final Animation<double> _checkProgress;

  @override
  void initState() {
    super.initState();

    _controller = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1100),
    );

    // 1) Circle scales in first — slight overshoot for a "pop" feel.
    _circleScale = Tween<double>(begin: 0.6, end: 1.0).animate(
      CurvedAnimation(
        parent: _controller,
        curve: const Interval(0.0, 0.42, curve: Curves.easeOutBack),
      ),
    );

    // 2) Check strokes draw only after the circle has mostly settled.
    _checkProgress = CurvedAnimation(
      parent: _controller,
      curve: const Interval(0.38, 1.0, curve: Curves.easeOutCubic),
    );

    // Play once on entry.
    _controller.forward();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return Scaffold(
      backgroundColor: colors.background,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 24),
          child: Column(
            children: [
              const Spacer(flex: 2),

              // Animated check
              AnimatedBuilder(
                animation: _controller,
                builder: (context, _) {
                  return Transform.scale(
                    scale: _circleScale.value,
                    child: SizedBox(
                      width: 96,
                      height: 96,
                      child: CustomPaint(
                        painter: _SuccessCheckPainter(
                          circleColor: colors.success,
                          checkColor: Colors.white,
                          checkProgress: _checkProgress.value,
                        ),
                      ),
                    ),
                  );
                },
              ),
              const SizedBox(height: 28),

              Text(
                'Withdrawal successful!',
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.w700,
                  color: colors.text,
                  letterSpacing: -0.4,
                  height: 1.2,
                ),
              ),
              const SizedBox(height: 12),

              Text(
                _formatNaira(widget.amount),
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 34,
                  fontWeight: FontWeight.w700,
                  color: colors.success,
                  letterSpacing: -0.8,
                  height: 1.05,
                ),
              ),
              const SizedBox(height: 10),

              Text(
                'has been sent to your bank account.',
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w400,
                  color: colors.textMuted,
                  height: 1.5,
                ),
              ),

              const Spacer(flex: 3),

              // Primary action
              SizedBox(
                width: double.infinity,
                height: 56,
                child: FilledButton(
                  onPressed: () {
                    // TODO: open receipt detail
                  },
                  style: FilledButton.styleFrom(
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(14),
                    ),
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('View receipt'),
                ),
              ),
              const SizedBox(height: 12),

              // Secondary action
              SizedBox(
                width: double.infinity,
                height: 56,
                child: OutlinedButton(
                  onPressed: () =>
                      Navigator.of(context).popUntil((route) => route.isFirst),
                  style: OutlinedButton.styleFrom(
                    foregroundColor: colors.text,
                    side: BorderSide(color: colors.border, width: 1.2),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(14),
                    ),
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('Back to wallet'),
                ),
              ),
              const SizedBox(height: 20),
            ],
          ),
        ),
      ),
    );
  }

  static String _formatNaira(double value) {
    final whole = value.round().toString();
    final buffer = StringBuffer();
    for (int i = 0; i < whole.length; i++) {
      if (i != 0 && (whole.length - i) % 3 == 0) buffer.write(',');
      buffer.write(whole[i]);
    }
    return '₦$buffer';
  }
}

// ── Success check painter (circle + drawn-in tick) ─────────────────

class _SuccessCheckPainter extends CustomPainter {
  _SuccessCheckPainter({
    required this.circleColor,
    required this.checkColor,
    required this.checkProgress,
  });

  final Color circleColor;
  final Color checkColor;

  /// 0.0 = tick not yet drawn, 1.0 = fully drawn.
  final double checkProgress;

  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width / 2, size.height / 2);
    final radius = size.width / 2;

    // ── Circle ──────────────────────────────────────────────────
    canvas.drawCircle(center, radius, Paint()..color = circleColor);

    // ── Tick path ───────────────────────────────────────────────
    // Two strokes: short arm down-right, long arm up-right. Coordinates are
    // relative to a 100×100 box so they map cleanly to any size.
    final shortStart = Offset(size.width * 0.30, size.height * 0.52);
    final joint = Offset(size.width * 0.44, size.height * 0.66);
    final tip = Offset(size.width * 0.72, size.height * 0.36);

    final fullPath = Path()
      ..moveTo(shortStart.dx, shortStart.dy)
      ..lineTo(joint.dx, joint.dy)
      ..lineTo(tip.dx, tip.dy);

    // Extract a portion of the path based on checkProgress. Drawing the
    // whole path via PathMetric lets us animate the stroke like a pen.
    final metrics = fullPath.computeMetrics().toList();
    if (metrics.isEmpty || checkProgress <= 0) return;
    final metric = metrics.first;
    final drawLength = metric.length * checkProgress.clamp(0.0, 1.0);
    final partialPath = metric.extractPath(0.0, drawLength);

    final checkPaint = Paint()
      ..color = checkColor
      ..style = PaintingStyle.stroke
      ..strokeWidth = size.width * 0.085
      ..strokeCap = StrokeCap.round
      ..strokeJoin = StrokeJoin.round;

    canvas.drawPath(partialPath, checkPaint);
  }

  @override
  bool shouldRepaint(covariant _SuccessCheckPainter oldDelegate) =>
      oldDelegate.circleColor != circleColor ||
      oldDelegate.checkColor != checkColor ||
      oldDelegate.checkProgress != checkProgress;
}
