import 'dart:math' as math;
import 'package:flutter/material.dart';
import '../../core/theme/theme_extensions.dart';

enum EmptyStory { opportunities, activity, wallet, support, search }

class DispatchEmptyState extends StatelessWidget {
  const DispatchEmptyState({
    super.key,
    required this.title,
    required this.message,
    this.story = EmptyStory.activity,
    this.actionLabel,
    this.onAction,
  });
  final String title, message;
  final EmptyStory story;
  final String? actionLabel;
  final VoidCallback? onAction;
  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final icon = switch (story) {
      EmptyStory.opportunities => Icons.local_shipping_outlined,
      EmptyStory.activity => Icons.route_outlined,
      EmptyStory.wallet => Icons.account_balance_wallet_outlined,
      EmptyStory.support => Icons.forum_outlined,
      EmptyStory.search => Icons.search_rounded,
    };
    return Center(
      child: SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(28, 32, 28, 32),
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 360),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              ExcludeSemantics(
                child: SizedBox(
                  width: 164,
                  height: 146,
                  child: Stack(
                    alignment: Alignment.center,
                    children: [
                      Positioned(
                        left: 4,
                        top: 8,
                        child: Transform.rotate(
                          angle: -.12,
                          child: Container(
                            width: 116,
                            height: 112,
                            decoration: BoxDecoration(
                              color: c.surfaceElevated,
                              borderRadius: BorderRadius.circular(32),
                            ),
                          ),
                        ),
                      ),
                      Positioned(
                        right: 5,
                        top: 0,
                        child: CustomPaint(
                          size: const Size(145, 130),
                          painter: _JourneyTrail(c.divider),
                        ),
                      ),
                      Transform.rotate(
                        angle: .06,
                        child: Container(
                          width: 82,
                          height: 96,
                          decoration: BoxDecoration(
                            color: c.surface,
                            border: Border.all(color: c.divider, width: 1.5),
                            borderRadius: BorderRadius.circular(24),
                            boxShadow: [
                              BoxShadow(
                                color: Colors.black.withValues(alpha: .035),
                                blurRadius: 16,
                                offset: const Offset(0, 6),
                              ),
                            ],
                          ),
                          child: Center(
                            child: Icon(icon, size: 38, color: c.brandTeal600),
                          ),
                        ),
                      ),
                      Positioned(
                        right: 16,
                        bottom: 10,
                        child: Container(
                          width: 38,
                          height: 38,
                          decoration: BoxDecoration(
                            shape: BoxShape.circle,
                            color: c.teal10,
                            border: Border.all(color: c.surface, width: 4),
                          ),
                          child: Icon(
                            story == EmptyStory.search
                                ? Icons.more_horiz_rounded
                                : Icons.add_rounded,
                            size: 19,
                            color: c.brandTeal600,
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
              const SizedBox(height: 22),
              Text(
                title,
                textAlign: TextAlign.center,
                style: Theme.of(context).textTheme.titleLarge,
              ),
              const SizedBox(height: 10),
              Text(
                message,
                textAlign: TextAlign.center,
                style: Theme.of(context).textTheme.bodyLarge,
              ),
              if (onAction != null && actionLabel != null) ...[
                const SizedBox(height: 24),
                OutlinedButton(onPressed: onAction, child: Text(actionLabel!)),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

class _JourneyTrail extends CustomPainter {
  const _JourneyTrail(this.color);
  final Color color;
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 1.5
      ..style = PaintingStyle.stroke;
    final path = Path()
      ..moveTo(5, size.height * .8)
      ..cubicTo(
        size.width * .9,
        size.height * 1.3,
        size.width * .1,
        -20,
        size.width - 10,
        18,
      );
    for (final metric in path.computeMetrics()) {
      for (double d = 0; d < metric.length; d += 9) {
        canvas.drawPath(
          metric.extractPath(d, math.min(d + 4, metric.length)),
          paint,
        );
      }
    }
    canvas.drawCircle(Offset(size.width - 10, 18), 4, Paint()..color = color);
  }

  @override
  bool shouldRepaint(_JourneyTrail old) => old.color != color;
}
