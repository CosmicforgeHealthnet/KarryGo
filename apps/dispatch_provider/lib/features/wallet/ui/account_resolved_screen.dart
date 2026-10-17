import '../../../shared/widgets/journey_icon.dart';
import '../../../shared/widgets/dispatch_screen_frame.dart';
import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../mock/wallet_tab_mock.dart';

class AccountResolvedScreen extends StatefulWidget {
  const AccountResolvedScreen({super.key, required this.account});

  final BankAccount account;

  @override
  State<AccountResolvedScreen> createState() => _AccountResolvedScreenState();
}

class _AccountResolvedScreenState extends State<AccountResolvedScreen>
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

    _circleScale = Tween<double>(begin: 0.6, end: 1.0).animate(
      CurvedAnimation(
        parent: _controller,
        curve: const Interval(0.0, 0.42, curve: Curves.easeOutBack),
      ),
    );

    _checkProgress = CurvedAnimation(
      parent: _controller,
      curve: const Interval(0.38, 1.0, curve: Curves.easeOutCubic),
    );

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
              const SizedBox(height: 8),
              Row(
                children: [
                  DispatchBackButton(
                    onPressed: () => Navigator.of(context).pop(),
                  ),
                  const Spacer(),
                  Text(
                    'Account resolved',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                      color: colors.text,
                    ),
                  ),
                  const Spacer(),
                  const SizedBox(width: 34),
                ],
              ),
              const Spacer(),

              // ── Animated success check ───────────────────────────
              AnimatedBuilder(
                animation: _controller,
                builder: (context, _) {
                  return Transform.scale(
                    scale: _circleScale.value,
                    child: SizedBox(
                      width: 88,
                      height: 88,
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
              const SizedBox(height: 22),

              Text(
                'Account verified!',
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
                'Your bank account has been successfully verified and is ready to use.',
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w400,
                  color: colors.textMuted,
                  height: 1.5,
                ),
              ),
              const SizedBox(height: 32),

              _AccountCard(account: widget.account),

              const Spacer(),
            ],
          ),
        ),
      ),
      bottomNavigationBar: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 8, 24, 16),
          child: SizedBox(
            width: double.infinity,
            height: 56,
            child: FilledButton(
              onPressed: () => Navigator.of(context).pop(),
              style: FilledButton.styleFrom(
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(14),
                ),
                textStyle: const TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w700,
                ),
              ),
              child: const Text('Done'),
            ),
          ),
        ),
      ),
    );
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
  final double checkProgress;

  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width / 2, size.height / 2);
    final radius = size.width / 2;

    // Circle
    canvas.drawCircle(center, radius, Paint()..color = circleColor);

    // Tick path — two strokes: short arm down-right, then long arm up-right.
    final shortStart = Offset(size.width * 0.30, size.height * 0.52);
    final joint = Offset(size.width * 0.44, size.height * 0.66);
    final tip = Offset(size.width * 0.72, size.height * 0.36);

    final fullPath = Path()
      ..moveTo(shortStart.dx, shortStart.dy)
      ..lineTo(joint.dx, joint.dy)
      ..lineTo(tip.dx, tip.dy);

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

// ── Bank account card ──────────────────────────────────────────────

class _AccountCard extends StatelessWidget {
  const _AccountCard({required this.account});
  final BankAccount account;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: colors.divider, width: 1),
      ),
      child: Row(
        children: [
          Container(
            width: 44,
            height: 44,
            decoration: BoxDecoration(
              color: colors.navy10,
              shape: BoxShape.circle,
            ),
            child: JourneyIcon(
              JourneySymbol.bank,
              size: 22,
              color: colors.text,
            ),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  account.bankName,
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w700,
                    color: colors.text,
                    height: 1.2,
                  ),
                ),
                const SizedBox(height: 3),
                Text(
                  account.accountNumber,
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w500,
                    color: colors.textMuted,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
