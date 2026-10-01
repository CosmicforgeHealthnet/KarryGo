import 'package:flutter/material.dart';

import '../../../../core/theme/theme_extensions.dart';

class DriverPin extends StatelessWidget {
  const DriverPin({super.key});

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        SizedBox(
          width: 40,
          height: 46,
          child: CustomPaint(
            painter: _PinPainter(color: colors.brandNavy900),
            child: const Align(
              alignment: Alignment(0, -0.2),
              child: SizedBox(
                width: 20,
                height: 20,
                child: Image(
                  image: AssetImage('assets/figma/splash_logo.png'),
                  fit: BoxFit.contain,
                ),
              ),
            ),
          ),
        ),
        const SizedBox(height: 3),
        Container(
          width: 6,
          height: 6,
          decoration: BoxDecoration(
            color: colors.brandTeal500,
            shape: BoxShape.circle,
          ),
        ),
      ],
    );
  }
}

class _PinPainter extends CustomPainter {
  _PinPainter({required this.color});
  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final r = size.width / 2;
    final center = Offset(r, r);
    final path = Path()
      ..addOval(Rect.fromCircle(center: center, radius: r))
      ..moveTo(center.dx - r * 0.55, center.dy + r * 0.6)
      ..quadraticBezierTo(
        center.dx,
        size.height,
        center.dx + r * 0.55,
        center.dy + r * 0.6,
      )
      ..close();

    canvas.drawPath(
      path,
      Paint()
        ..color = Colors.black.withValues(alpha: 0.15)
        ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 3),
    );
    canvas.drawPath(path, Paint()..color = color);
    canvas.drawPath(
      path,
      Paint()
        ..color = Colors.white
        ..style = PaintingStyle.stroke
        ..strokeWidth = 3,
    );
  }

  @override
  bool shouldRepaint(covariant _PinPainter oldDelegate) =>
      oldDelegate.color != color;
}