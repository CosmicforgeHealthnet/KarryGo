import '../../../shared/widgets/dispatch_screen_frame.dart';
import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import 'withdrawal_receipt_screen.dart';

class PaymentProcessingScreen extends StatefulWidget {
  const PaymentProcessingScreen({super.key, required this.amount});

  final double amount;

  @override
  State<PaymentProcessingScreen> createState() =>
      _PaymentProcessingScreenState();
}

class _PaymentProcessingScreenState extends State<PaymentProcessingScreen>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller;

  @override
  void initState() {
    super.initState();

    _controller = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1200),
    )..repeat();

    Future.delayed(const Duration(milliseconds: 2500), () {
      if (!mounted) return;
      Navigator.of(context).pushReplacement(
        MaterialPageRoute(
          builder: (_) => WithdrawalReceiptScreen(amount: widget.amount),
        ),
      );
    });
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
        child: Column(
          children: [
            _buildTopBar(context),
            Expanded(
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 32),
                child: Column(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    _SpinnerRing(
                      controller: _controller,
                      trackColor: colors.navy10,
                      sweepColor: colors.brandTeal500,
                    ),
                    const SizedBox(height: 24),
                    Text(
                      'Processing withdrawal',
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        fontSize: 17,
                        fontWeight: FontWeight.w700,
                        color: colors.text,
                        letterSpacing: -0.3,
                        height: 1.2,
                      ),
                    ),
                    const SizedBox(height: 8),
                    Text(
                      'Please do not close the app.',
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        fontSize: 13,
                        fontWeight: FontWeight.w400,
                        color: colors.textMuted,
                        height: 1.5,
                      ),
                    ),
                  ],
                ),
              ),
            ),
            const SizedBox(height: 40),
          ],
        ),
      ),
    );
  }

  Widget _buildTopBar(BuildContext context) {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
      child: Row(
        children: [
          DispatchBackButton(onPressed: () => Navigator.of(context).pop()),
        ],
      ),
    );
  }
}

// ── Spinner ring ────────────────────────────────────────────────────

class _SpinnerRing extends StatelessWidget {
  const _SpinnerRing({
    required this.controller,
    required this.trackColor,
    required this.sweepColor,
  });

  final AnimationController controller;
  final Color trackColor;
  final Color sweepColor;

  static const double _size = 56;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: _size,
      height: _size,
      child: AnimatedBuilder(
        animation: controller,
        builder: (context, _) {
          return Transform.rotate(
            angle: controller.value * 6.28318, // 2π
            child: CustomPaint(
              painter: _RingPainter(
                trackColor: trackColor,
                sweepColor: sweepColor,
              ),
              size: const Size(_size, _size),
            ),
          );
        },
      ),
    );
  }
}

class _RingPainter extends CustomPainter {
  _RingPainter({required this.trackColor, required this.sweepColor});

  final Color trackColor;
  final Color sweepColor;

  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width / 2, size.height / 2);
    final radius = size.width / 2 - 4;

    final trackPaint = Paint()
      ..color = trackColor
      ..style = PaintingStyle.stroke
      ..strokeWidth = 4
      ..strokeCap = StrokeCap.round;
    canvas.drawCircle(center, radius, trackPaint);

    final arcPaint = Paint()
      ..color = sweepColor
      ..style = PaintingStyle.stroke
      ..strokeWidth = 4
      ..strokeCap = StrokeCap.round;

    const sweep = 3.14159 * 0.7;
    canvas.drawArc(
      Rect.fromCircle(center: center, radius: radius),
      -3.14159 / 2,
      sweep,
      false,
      arcPaint,
    );
  }

  @override
  bool shouldRepaint(covariant _RingPainter oldDelegate) =>
      oldDelegate.trackColor != trackColor ||
      oldDelegate.sweepColor != sweepColor;
}
