import 'package:flutter/material.dart';
import '../../../core/theme/vendor_theme.dart';

class CatalogueArt extends StatelessWidget {
  const CatalogueArt({super.key, required this.food, this.size = 80});
  final bool food;
  final double size;
  @override
  Widget build(BuildContext context) => ExcludeSemantics(
    child: SizedBox(
      width: size,
      height: size,
      child: CustomPaint(
        painter: _CataloguePainter(
          food,
          (food ? const Color(0xFFC28027) : KarryamColors.tealInk).forInk(
            context,
          ),
          Colors.white.forSurface(context),
          const Color(0xFFFFEBC4).forSurface(context),
          const Color(0xFF99DED2).forSurface(context),
          const Color(0xFFBBEAE1).forSurface(context),
          KarryamColors.navy.forInk(context),
        ),
      ),
    ),
  );
}

class _CataloguePainter extends CustomPainter {
  const _CataloguePainter(
    this.food,
    this.accent,
    this.paper,
    this.warm,
    this.top,
    this.side,
    this.ink,
  );
  final Color accent, paper, warm, top, side, ink;
  final bool food;
  @override
  void paint(Canvas canvas, Size size) {
    canvas.save();
    canvas.scale(size.width / 100, size.height / 100);
    canvas.drawOval(
      const Rect.fromLTWH(13, 83, 77, 10),
      Paint()..color = accent.withValues(alpha: .09),
    );
    final shape = Path();
    if (food) {
      canvas.save();
      canvas.translate(50, 48);
      canvas.rotate(-.12);
      canvas.translate(-50, -48);
      canvas.drawRRect(
        RRect.fromRectAndRadius(
          const Rect.fromLTWH(21, 15, 64, 73),
          const Radius.circular(12),
        ),
        Paint()..color = accent.withValues(alpha: .17),
      );
      canvas.drawRRect(
        RRect.fromRectAndRadius(
          const Rect.fromLTWH(16, 9, 64, 73),
          const Radius.circular(12),
        ),
        Paint()..color = paper,
      );
      canvas.drawRRect(
        RRect.fromRectAndRadius(
          const Rect.fromLTWH(16, 9, 12, 73),
          const Radius.circular(5),
        ),
        Paint()..color = accent,
      );
      canvas.drawCircle(const Offset(51, 33), 13, Paint()..color = warm);
      canvas.drawCircle(
        const Offset(51, 33),
        8,
        Paint()..color = accent.withValues(alpha: .32),
      );
      final stroke = Paint()
        ..color = accent
        ..strokeWidth = 2.8
        ..strokeCap = StrokeCap.round;
      canvas.drawLine(const Offset(36, 55), const Offset(68, 55), stroke);
      canvas.drawLine(
        const Offset(36, 63),
        const Offset(62, 63),
        stroke..color = accent.withValues(alpha: .35),
      );
      canvas.drawLine(const Offset(36, 70), const Offset(53, 70), stroke);
      canvas.restore();
      canvas.drawCircle(
        const Offset(83, 18),
        7,
        Paint()..color = KarryamColors.teal,
      );
      shape.moveTo(79, 18);
      shape.lineTo(83, 14);
      shape.lineTo(87, 18);
      shape.moveTo(83, 14);
      shape.lineTo(83, 22);
    } else {
      shape.moveTo(15, 37);
      shape.lineTo(72, 28);
      shape.lineTo(87, 39);
      shape.lineTo(30, 48);
      shape.close();
      canvas.drawPath(shape, Paint()..color = top);
      shape.reset();
      shape.moveTo(30, 48);
      shape.lineTo(87, 39);
      shape.lineTo(84, 80);
      shape.lineTo(30, 90);
      shape.close();
      canvas.drawPath(shape, Paint()..color = paper);
      shape.reset();
      shape.moveTo(15, 37);
      shape.lineTo(30, 48);
      shape.lineTo(30, 90);
      shape.lineTo(17, 79);
      shape.close();
      canvas.drawPath(shape, Paint()..color = side);
      canvas.drawLine(
        const Offset(56, 45),
        const Offset(56, 63),
        Paint()
          ..color = accent
          ..strokeWidth = 8,
      );
      canvas.drawRRect(
        RRect.fromRectAndRadius(
          const Rect.fromLTWH(65, 66, 12, 6),
          const Radius.circular(2),
        ),
        Paint()..color = accent.withValues(alpha: .3),
      );
      canvas.drawRRect(
        RRect.fromRectAndRadius(
          const Rect.fromLTWH(31, 8, 39, 19),
          const Radius.circular(8),
        ),
        Paint()..color = KarryamColors.tealInk,
      );
      canvas.drawCircle(
        const Offset(39, 17),
        2.5,
        Paint()..color = Colors.white,
      );
      for (var i = 0; i < 4; i++) {
        canvas.drawLine(
          Offset(47 + i * 4, 13),
          Offset(47 + i * 4, 21),
          Paint()
            ..color = Colors.white.withValues(alpha: .85)
            ..strokeWidth = 1.5,
        );
      }
      shape.reset();
      shape.moveTo(79, 20);
      shape.lineTo(84, 20);
      shape.moveTo(81.5, 17.5);
      shape.lineTo(81.5, 22.5);
    }
    canvas.drawPath(
      shape,
      Paint()
        ..color = food ? ink : accent
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1.7
        ..strokeCap = StrokeCap.round,
    );
    canvas.restore();
  }

  @override
  bool shouldRepaint(_CataloguePainter old) =>
      old.food != food ||
      old.accent != accent ||
      old.paper != paper ||
      old.warm != warm ||
      old.top != top ||
      old.side != side ||
      old.ink != ink;
}
