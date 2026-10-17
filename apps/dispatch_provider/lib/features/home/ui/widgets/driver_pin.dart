import 'package:flutter/material.dart';
import '../../../../core/theme/theme_extensions.dart';

class DriverPin extends StatefulWidget {
  const DriverPin({super.key, this.online = false});
  final bool online;
  @override
  State<DriverPin> createState() => _DriverPinState();
}

class _DriverPinState extends State<DriverPin>
    with SingleTickerProviderStateMixin {
  late final AnimationController _pulse = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 2400),
  );
  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (MediaQuery.disableAnimationsOf(context)) {
      _pulse.stop();
    } else {
      _pulse.repeat();
    }
  }

  @override
  void dispose() {
    _pulse.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Semantics(
    label: 'Your location',
    child: AnimatedBuilder(
      animation: _pulse,
      builder: (context, _) => CustomPaint(
        painter: _RiderPainter(
          context.palette.brandTeal500,
          _pulse.value,
          widget.online,
        ),
      ),
    ),
  );
}

class _RiderPainter extends CustomPainter {
  const _RiderPainter(this.color, this.phase, this.online);
  final Color color;
  final double phase;
  final bool online;
  @override
  void paint(Canvas canvas, Size size) {
    final center = size.center(Offset.zero);
    canvas.translate(center.dx, center.dy);
    canvas.scale(.8);
    canvas.translate(-center.dx, -center.dy);
    canvas.drawCircle(
      center,
      25,
      Paint()..color = color.withValues(alpha: .10),
    );
    canvas.drawCircle(
      center,
      25 + phase * 18,
      Paint()
        ..color = color.withValues(alpha: (1 - phase) * (online ? .25 : .12))
        ..style = PaintingStyle.stroke
        ..strokeWidth = 2,
    );
    canvas.save();
    canvas.translate(center.dx, center.dy);
    canvas.drawOval(
      const Rect.fromLTWH(-20, -28, 40, 64),
      Paint()
        ..color = Colors.black.withValues(alpha: .16)
        ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 5),
    );
    final tyre = Paint()..color = const Color(0xFF081F44);
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        const Rect.fromLTWH(-5, -34, 10, 18),
        const Radius.circular(4),
      ),
      tyre,
    );
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        const Rect.fromLTWH(-5, 20, 10, 18),
        const Radius.circular(4),
      ),
      tyre,
    );
    final body = Path()
      ..moveTo(-8, -25)
      ..quadraticBezierTo(0, -31, 8, -25)
      ..lineTo(13, -8)
      ..lineTo(10, 23)
      ..quadraticBezierTo(0, 30, -10, 23)
      ..lineTo(-13, -8)
      ..close();
    canvas.drawShadow(body, const Color(0xFF081F44), 5, true);
    canvas.drawPath(
      body,
      Paint()
        ..shader = LinearGradient(
          colors: [const Color(0xFFA9FFF0), color, const Color(0xFF087C78)],
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
        ).createShader(const Rect.fromLTWH(-14, -27, 28, 56)),
    );
    canvas.drawPath(
      body,
      Paint()
        ..color = Colors.white
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1.8,
    );
    canvas.drawLine(
      const Offset(-18, -13),
      const Offset(18, -13),
      Paint()
        ..color = const Color(0xFF081F44)
        ..strokeWidth = 4
        ..strokeCap = StrokeCap.round,
    );
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        const Rect.fromLTWH(-6, -24, 12, 5),
        const Radius.circular(2),
      ),
      Paint()..color = const Color(0xFFE8FFF8),
    );
    canvas.drawOval(const Rect.fromLTWH(-9, -7, 18, 25), tyre);
    canvas.drawCircle(
      const Offset(0, -3),
      8,
      Paint()..color = const Color(0xFF19355A),
    );
    canvas.drawArc(
      const Rect.fromLTWH(-6, -9, 12, 12),
      3.5,
      2.4,
      false,
      Paint()
        ..color = Colors.white.withValues(alpha: .8)
        ..style = PaintingStyle.stroke
        ..strokeWidth = 2,
    );
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        const Rect.fromLTWH(-7, 13, 14, 12),
        const Radius.circular(3),
      ),
      Paint()..color = const Color(0xFF081F44),
    );
    canvas.drawLine(
      const Offset(-4, 18),
      const Offset(4, 18),
      Paint()
        ..color = color
        ..strokeWidth = 2
        ..strokeCap = StrokeCap.round,
    );
    canvas.restore();
  }

  @override
  bool shouldRepaint(_RiderPainter old) =>
      old.phase != phase || old.color != color || old.online != online;
}
