import 'package:flutter/material.dart';
import '../../core/theme/theme_extensions.dart';

enum JourneySymbol {
  route,
  parcel,
  wallet,
  profile,
  contact,
  shield,
  photo,
  review,
  fleet,
  partner,
  bank,
  preferences,
  transfer,
  earnings,
  signOut,
  key,
}

class JourneyIcon extends StatelessWidget {
  const JourneyIcon(this.symbol, {super.key, this.size = 28, this.color});
  final JourneySymbol symbol;
  final double size;
  final Color? color;
  @override
  Widget build(BuildContext context) => SizedBox.square(
    dimension: size,
    child: CustomPaint(
      painter: _JourneyPainter(symbol, color ?? context.palette.brandTeal600),
    ),
  );
}

class _JourneyPainter extends CustomPainter {
  const _JourneyPainter(this.symbol, this.color);
  final JourneySymbol symbol;
  final Color color;
  @override
  void paint(Canvas canvas, Size size) {
    canvas.scale(size.width / 32, size.height / 32);
    final ink = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1.8
      ..strokeCap = StrokeCap.round
      ..strokeJoin = StrokeJoin.round;
    void line(double a, double b, double c, double d) =>
        canvas.drawLine(Offset(a, b), Offset(c, d), ink);
    void box(double a, double b, double w, double h) => canvas.drawRRect(
      RRect.fromRectAndRadius(
        Rect.fromLTWH(a, b, w, h),
        const Radius.circular(3),
      ),
      ink,
    );
    void check(double x, double y) => canvas.drawPath(
      Path()
        ..moveTo(x, y)
        ..lineTo(x + 3, y + 3)
        ..lineTo(x + 8, y - 3),
      ink,
    );
    switch (symbol) {
      case JourneySymbol.route:
        canvas.drawCircle(const Offset(7, 7), 3, ink);
        canvas.drawCircle(const Offset(25, 25), 3, ink);
        canvas.drawPath(
          Path()
            ..moveTo(7, 13)
            ..lineTo(7, 19)
            ..quadraticBezierTo(7, 25, 13, 25)
            ..lineTo(18, 25),
          ink,
        );
        canvas.drawPath(
          Path()
            ..moveTo(14, 7)
            ..lineTo(20, 7)
            ..quadraticBezierTo(25, 7, 25, 13)
            ..lineTo(25, 17),
          ink,
        );
        line(22, 14, 25, 17);
        line(28, 14, 25, 17);
      case JourneySymbol.parcel:
        canvas.drawPath(
          Path()
            ..moveTo(5, 10)
            ..lineTo(16, 4)
            ..lineTo(27, 10)
            ..lineTo(27, 23)
            ..lineTo(16, 29)
            ..lineTo(5, 23)
            ..close(),
          ink,
        );
        line(5, 10, 16, 16);
        line(16, 16, 27, 10);
        line(16, 16, 16, 29);
        line(11, 7, 22, 13);
        line(22, 13, 22, 18);
      case JourneySymbol.wallet:
        box(4, 8, 24, 19);
        box(20, 14, 9, 8);
        canvas.drawCircle(const Offset(24, 18), 1, ink);
        line(7, 8, 22, 4);
        line(22, 4, 25, 8);
      case JourneySymbol.profile:
        canvas.drawCircle(const Offset(16, 10), 5, ink);
        canvas.drawPath(
          Path()
            ..moveTo(5, 27)
            ..cubicTo(5, 16, 27, 16, 27, 27),
          ink,
        );
        line(11, 27, 21, 27);
      case JourneySymbol.contact:
        box(5, 5, 21, 23);
        canvas.drawCircle(const Offset(13, 13), 3, ink);
        canvas.drawArc(
          const Rect.fromLTWH(8, 18, 10, 7),
          3.14,
          3.14,
          false,
          ink,
        );
        line(22, 11, 29, 11);
        line(22, 17, 29, 17);
        line(22, 23, 29, 23);
      case JourneySymbol.shield:
        canvas.drawPath(
          Path()
            ..moveTo(16, 3)
            ..lineTo(27, 7)
            ..lineTo(26, 18)
            ..quadraticBezierTo(24, 25, 16, 29)
            ..quadraticBezierTo(8, 25, 6, 18)
            ..lineTo(5, 7)
            ..close(),
          ink,
        );
        check(11, 16);
      case JourneySymbol.photo:
        box(4, 8, 24, 20);
        canvas.drawCircle(const Offset(16, 18), 6, ink);
        line(10, 8, 12, 4);
        line(12, 4, 20, 4);
        line(20, 4, 22, 8);
        line(24, 12, 25, 12);
      case JourneySymbol.review:
        box(6, 5, 20, 24);
        box(11, 3, 10, 5);
        check(11, 18);
        line(11, 24, 21, 24);
      case JourneySymbol.fleet:
        box(3, 6, 18, 13);
        box(11, 14, 18, 13);
        line(7, 11, 16, 11);
        line(16, 20, 24, 20);
      case JourneySymbol.bank:
        canvas.drawPath(
          Path()
            ..moveTo(3, 11)
            ..lineTo(16, 4)
            ..lineTo(29, 11)
            ..close(),
          ink,
        );
        line(4, 28, 28, 28);
        line(6, 24, 26, 24);
        for (final x in [8.0, 16.0, 24.0]) {
          line(x, 14, x, 24);
        }
        canvas.drawCircle(const Offset(16, 8), .8, ink);
      case JourneySymbol.preferences:
        for (final y in [8.0, 16.0, 24.0]) {
          line(4, y, 28, y);
        }
        box(9, 5, 4, 6);
        box(20, 13, 4, 6);
        box(9, 21, 4, 6);
      case JourneySymbol.transfer:
        line(4, 10, 27, 10);
        line(22, 5, 27, 10);
        line(22, 15, 27, 10);
        line(28, 23, 5, 23);
        line(10, 18, 5, 23);
        line(10, 28, 5, 23);
      case JourneySymbol.earnings:
        line(5, 27, 28, 27);
        box(7, 18, 4, 9);
        box(15, 13, 4, 14);
        box(23, 7, 4, 20);
        line(5, 13, 15, 4);
        line(10, 4, 15, 4);
        line(15, 4, 15, 9);
      case JourneySymbol.signOut:
        canvas.drawPath(
          Path()
            ..moveTo(13, 4)
            ..lineTo(5, 4)
            ..lineTo(5, 28)
            ..lineTo(13, 28),
          ink,
        );
        line(13, 16, 28, 16);
        line(23, 11, 28, 16);
        line(23, 21, 28, 16);
      case JourneySymbol.key:
        canvas.drawCircle(const Offset(11, 11), 7, ink);
        line(16, 16, 28, 28);
        line(22, 22, 26, 18);
        line(25, 25, 29, 21);
        canvas.drawCircle(const Offset(9, 9), 1.2, ink);
      case JourneySymbol.partner:
        box(4, 9, 24, 19);
        line(11, 9, 11, 5);
        line(11, 5, 21, 5);
        line(21, 5, 21, 9);
        line(4, 17, 28, 17);
        box(13, 15, 6, 5);
    }
  }

  @override
  bool shouldRepaint(_JourneyPainter old) =>
      old.symbol != symbol || old.color != color;
}
