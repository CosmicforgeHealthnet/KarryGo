import 'dart:math' as math;
import 'package:flutter/material.dart';
import '../theme/karryam_theme.dart';

enum DriverArt { offline, searching, driving, verifying, complete }

class DriverStateArt extends StatefulWidget {
  const DriverStateArt({super.key, required this.kind, this.height = 140});
  final DriverArt kind;
  final double height;
  @override
  State<DriverStateArt> createState() => _DriverStateArtState();
}

class _DriverStateArtState extends State<DriverStateArt>
    with SingleTickerProviderStateMixin {
  late final AnimationController _motion = AnimationController(
    vsync: this,
    duration: const Duration(seconds: 3),
  );
  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (MediaQuery.disableAnimationsOf(context)) {
      _motion.stop();
      _motion.value = .5;
    } else {
      _motion.repeat();
    }
  }

  @override
  void dispose() {
    _motion.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => ExcludeSemantics(
    child: SizedBox(
      height: widget.height,
      child: AnimatedBuilder(
        animation: _motion,
        builder: (context, _) => CustomPaint(
          size: Size.infinite,
          painter: _StatePainter(
            widget.kind,
            _motion.value,
            KarryamColors.of(context),
          ),
        ),
      ),
    ),
  );
}

class _StatePainter extends CustomPainter {
  _StatePainter(this.kind, this.progress, this.colors);
  final DriverArt kind;
  final double progress;
  final KarryamColors colors;
  @override
  void paint(Canvas canvas, Size size) {
    canvas.translate(size.width / 2, size.height / 2);
    final scale = math.min(size.width / 260, size.height / 140);
    canvas.scale(scale);
    final line = Paint()
      ..color = colors.border
      ..strokeWidth = 2
      ..style = PaintingStyle.stroke;
    final teal = Paint()..color = KarryamColors.teal;
    if (kind == DriverArt.searching) {
      for (var i = 0; i < 2; i++) {
        final phase = (progress + i * .5) % 1;
        canvas.drawCircle(
          Offset.zero,
          22 + phase * 40,
          Paint()
            ..color = KarryamColors.teal.withValues(alpha: (1 - phase) * .5)
            ..style = PaintingStyle.stroke
            ..strokeWidth = 2,
        );
      }
    }
    if (kind == DriverArt.complete) {
      canvas.drawCircle(Offset.zero, 40, teal);
      canvas.drawPath(
        Path()
          ..moveTo(-18, 0)
          ..lineTo(-4, 14)
          ..lineTo(22, -17),
        Paint()
          ..color = KarryamColors.navy
          ..style = PaintingStyle.stroke
          ..strokeWidth = 5
          ..strokeCap = StrokeCap.round,
      );
      return;
    }
    if (kind == DriverArt.verifying) {
      canvas.drawRRect(
        RRect.fromRectAndRadius(
          const Rect.fromLTWH(-28, -54, 56, 108),
          const Radius.circular(12),
        ),
        Paint()..color = colors.text,
      );
      canvas.drawRRect(
        RRect.fromRectAndRadius(
          const Rect.fromLTWH(-22, -40, 44, 78),
          const Radius.circular(5),
        ),
        Paint()..color = colors.card,
      );
      for (var x = 0; x < 3; x++) {
        for (var y = 0; y < 3; y++) {
          canvas.drawRect(
            Rect.fromLTWH(-15 + x * 12, -20 + y * 12, 8, 8),
            Paint()..color = colors.text,
          );
        }
      }
      canvas.drawLine(
        Offset(-24, -32 + progress * 65),
        Offset(24, -32 + progress * 65),
        Paint()
          ..color = KarryamColors.teal
          ..strokeWidth = 3,
      );
      return;
    }
    canvas.drawLine(const Offset(-100, 40), const Offset(100, 40), line);
    canvas.translate(0, math.sin(progress * math.pi * 2) * 2);
    final car = Path()
      ..moveTo(-58, 22)
      ..lineTo(-58, 0)
      ..lineTo(-42, -8)
      ..lineTo(-26, -34)
      ..lineTo(26, -34)
      ..lineTo(47, -8)
      ..lineTo(60, 0)
      ..lineTo(60, 22)
      ..close();
    canvas.drawPath(car, Paint()..color = colors.text);
    canvas.drawPath(
      Path()
        ..moveTo(-33, -9)
        ..lineTo(-21, -27)
        ..lineTo(-4, -27)
        ..lineTo(-4, -9)
        ..close(),
      teal,
    );
    canvas.drawPath(
      Path()
        ..moveTo(3, -27)
        ..lineTo(22, -27)
        ..lineTo(37, -9)
        ..lineTo(3, -9)
        ..close(),
      teal,
    );
    for (final x in [-35.0, 37.0]) {
      canvas.drawCircle(Offset(x, 23), 12, Paint()..color = KarryamColors.navy);
      canvas.drawCircle(
        Offset(x, 23),
        5,
        Paint()..color = const Color(0xFFA7B0BB),
      );
    }
  }

  @override
  bool shouldRepaint(covariant _StatePainter old) =>
      old.progress != progress ||
      old.kind != kind ||
      old.colors.dark != colors.dark;
}

class KarryamEntrance extends StatelessWidget {
  const KarryamEntrance({super.key, required this.child});
  final Widget child;
  @override
  Widget build(BuildContext context) => TweenAnimationBuilder<double>(
    tween: Tween(begin: 0, end: 1),
    duration: MediaQuery.disableAnimationsOf(context)
        ? Duration.zero
        : const Duration(milliseconds: 420),
    curve: Curves.easeOutCubic,
    child: child,
    builder: (context, value, child) => Opacity(
      opacity: value,
      child: Transform.translate(
        offset: Offset(0, 20 * (1 - value)),
        child: child,
      ),
    ),
  );
}
