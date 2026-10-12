import 'package:flutter/material.dart';
import '../../../core/theme/vendor_theme.dart';

Future<bool?> showOrderFeedback(
  BuildContext context, {
  required String title,
  required String message,
  bool success = false,
  bool warning = false,
  bool ending = false,
  String? action,
}) => showGeneralDialog<bool>(
  context: context,
  barrierDismissible: true,
  barrierLabel: MaterialLocalizations.of(context).modalBarrierDismissLabel,
  barrierColor: Colors.black.withValues(alpha: .45),
  transitionDuration: MediaQuery.disableAnimationsOf(context)
      ? Duration.zero
      : const Duration(milliseconds: 280),
  transitionBuilder: (context, animation, secondaryAnimation, child) {
    final motion = CurvedAnimation(
      parent: animation,
      curve: Curves.easeOutCubic,
      reverseCurve: Curves.easeInCubic,
    );
    return FadeTransition(
      opacity: motion,
      child: ScaleTransition(
        scale: Tween<double>(begin: .96, end: 1).animate(motion),
        child: child,
      ),
    );
  },
  pageBuilder: (context, animation, secondaryAnimation) => OrderFeedbackDialog(
    title: title,
    message: message,
    success: success,
    warning: warning,
    ending: ending,
    action: action,
  ),
);

class OrderFeedbackDialog extends StatelessWidget {
  const OrderFeedbackDialog({
    super.key,
    required this.title,
    required this.message,
    this.success = false,
    this.warning = false,
    this.ending = false,
    this.action,
  });
  final String title, message;
  final bool success, warning, ending;
  final String? action;
  @override
  Widget build(BuildContext context) {
    final color = ending
        ? KarryamColors.error
        : success
        ? KarryamColors.tealInk
        : warning
        ? const Color(0xFF986013)
        : KarryamColors.error;
    return Dialog(
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(30)),
      insetPadding: const EdgeInsets.symmetric(horizontal: 26),
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(26),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TweenAnimationBuilder<double>(
              tween: Tween(
                begin: MediaQuery.disableAnimationsOf(context) ? 1 : .7,
                end: 1,
              ),
              duration: MediaQuery.disableAnimationsOf(context)
                  ? Duration.zero
                  : const Duration(milliseconds: 420),
              curve: Curves.easeOutCubic,
              builder: (_, scale, child) =>
                  Transform.scale(scale: scale, child: child),
              child: success && !ending
                  ? const _SuccessSeal()
                  : Container(
                      width: 92,
                      height: 92,
                      decoration: BoxDecoration(
                        color: (color.withValues(
                          alpha: .10,
                        )).forSurface(context),
                        borderRadius: BorderRadius.circular(30),
                        border: Border.all(
                          color: (color.withValues(
                            alpha: .16,
                          )).forOutline(context),
                        ),
                      ),
                      child: Icon(
                        ending
                            ? Icons.cancel_outlined
                            : success
                            ? Icons.check_circle_outline_rounded
                            : warning
                            ? Icons.info_outline_rounded
                            : Icons.error_outline_rounded,
                        color: (color).forInk(context),
                        size: 48,
                      ),
                    ),
            ),
            const SizedBox(height: 22),
            Text(
              title,
              textAlign: TextAlign.center,
              style: const TextStyle(
                fontSize: 24,
                fontWeight: FontWeight.w800,
                letterSpacing: -.6,
              ),
            ),
            const SizedBox(height: 10),
            Text(
              message,
              textAlign: TextAlign.center,
              style: const TextStyle(height: 1.6),
            ),
            const SizedBox(height: 24),
            SizedBox(
              width: double.infinity,
              child: FilledButton(
                style: FilledButton.styleFrom(
                  backgroundColor: KarryamColors.teal.forSurface(context),
                  foregroundColor: KarryamColors.navy,
                  padding: const EdgeInsets.symmetric(vertical: 16),
                ),
                onPressed: () => Navigator.pop(context, true),
                child: Text(action ?? 'Continue'),
              ),
            ),
            if (!success)
              TextButton(
                onPressed: () => Navigator.pop(context, false),
                child: const Text('Close'),
              ),
          ],
        ),
      ),
    );
  }
}

class _SuccessSeal extends StatelessWidget {
  const _SuccessSeal();
  @override
  Widget build(BuildContext context) => Semantics(
    label: 'Success',
    child: TweenAnimationBuilder<double>(
      tween: Tween(
        begin: MediaQuery.disableAnimationsOf(context) ? 1 : 0,
        end: 1,
      ),
      duration: MediaQuery.disableAnimationsOf(context)
          ? Duration.zero
          : const Duration(milliseconds: 850),
      curve: Curves.easeOutCubic,
      builder: (_, progress, _) => SizedBox(
        width: 112,
        height: 112,
        child: CustomPaint(painter: _SuccessPainter(progress)),
      ),
    ),
  );
}

class _SuccessPainter extends CustomPainter {
  _SuccessPainter(this.progress);
  final double progress;
  @override
  void paint(Canvas canvas, Size size) {
    final center = size.center(Offset.zero);
    final paint = Paint()..isAntiAlias = true;
    canvas.drawCircle(
      center,
      54,
      paint..color = KarryamColors.teal.withValues(alpha: .05),
    );
    canvas.drawCircle(
      center,
      44 + progress * 3,
      paint..color = KarryamColors.teal.withValues(alpha: .1),
    );
    final disc = Rect.fromCircle(center: center, radius: 36);
    canvas.drawCircle(
      center,
      36,
      paint
        ..shader = const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [KarryamColors.teal, KarryamColors.tealInk],
        ).createShader(disc),
    );
    paint.shader = null;
    canvas.drawCircle(
      center,
      44,
      paint
        ..color = KarryamColors.tealInk.withValues(alpha: .18)
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1,
    );
    paint
      ..style = PaintingStyle.stroke
      ..strokeWidth = 4.5
      ..strokeCap = StrokeCap.round
      ..strokeJoin = StrokeJoin.round
      ..color = Colors.white;
    final path = Path()
      ..moveTo(center.dx - 15, center.dy)
      ..lineTo(center.dx - 4, center.dy + 11)
      ..lineTo(center.dx + 17, center.dy - 12);
    final reveal = ((progress - .15) / .85).clamp(0.0, 1.0);
    for (final metric in path.computeMetrics()) {
      canvas.drawPath(metric.extractPath(0, metric.length * reveal), paint);
    }
    paint
      ..style = PaintingStyle.fill
      ..color = KarryamColors.tealInk.withValues(alpha: .5 * progress);
    canvas.drawCircle(Offset(center.dx + 43, center.dy - 34), 4, paint);
    canvas.drawCircle(Offset(center.dx - 47, center.dy + 23), 3, paint);
  }

  @override
  bool shouldRepaint(_SuccessPainter oldDelegate) =>
      oldDelegate.progress != progress;
}
