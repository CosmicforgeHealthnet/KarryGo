import 'package:flutter/material.dart';

import '../../../../core/format/money_format.dart';
import '../../../home/ui/widgets/provider_app_colors.dart';

/// Smooth area-line chart of monthly earnings (Figma 2273). Plots 12 values
/// (Jan..Dec) with dashed gridlines, a gradient fill, and a tooltip on the
/// highest-earning month.
class EarningsChart extends StatelessWidget {
  const EarningsChart({super.key, required this.monthlyKobo, this.height = 230});

  /// Length-12 monthly earnings in kobo. Shorter/empty lists render a flat line.
  final List<int> monthlyKobo;
  final double height;

  static const _months = [
    'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
    'Jul', 'Aug', 'Sept', 'Oct', 'Nov', 'Dec',
  ];

  @override
  Widget build(BuildContext context) {
    final values = List<double>.generate(
      12,
      (i) => i < monthlyKobo.length ? monthlyKobo[i] / 100.0 : 0.0,
    );

    // Highlight the highest-earning month (when any earnings exist).
    var highlight = -1;
    var maxV = 0.0;
    for (var i = 0; i < values.length; i++) {
      if (values[i] > maxV) {
        maxV = values[i];
        highlight = i;
      }
    }

    return SizedBox(
      height: height,
      width: double.infinity,
      child: CustomPaint(
        painter: _ChartPainter(
          values: values,
          months: _months,
          highlightIndex: highlight,
          highlightLabel: highlight >= 0 ? '₦ ${formatNaira(values[highlight])}' : '',
        ),
      ),
    );
  }
}

class _ChartPainter extends CustomPainter {
  _ChartPainter({
    required this.values,
    required this.months,
    required this.highlightIndex,
    required this.highlightLabel,
  });

  final List<double> values;
  final List<String> months;
  final int highlightIndex;
  final String highlightLabel;

  static const _leftPad = 6.0;
  static const _rightPad = 6.0;
  static const _topPad = 34.0; // room for the tooltip
  static const _labelArea = 26.0; // room for month labels

  @override
  void paint(Canvas canvas, Size size) {
    final chartTop = _topPad;
    final chartBottom = size.height - _labelArea;
    final plotWidth = size.width - _leftPad - _rightPad;

    final maxV = values.fold<double>(0, (m, v) => v > m ? v : m);

    double xFor(int i) => _leftPad + (plotWidth * i / (values.length - 1));
    double yFor(double v) =>
        maxV <= 0 ? chartBottom : chartBottom - (v / maxV) * (chartBottom - chartTop) * 0.92;

    _drawGridlines(canvas, size, chartTop, chartBottom);

    final points = [
      for (var i = 0; i < values.length; i++) Offset(xFor(i), yFor(values[i])),
    ];
    final linePath = _smoothPath(points);

    // Gradient area fill below the line.
    final fillPath = Path.from(linePath)
      ..lineTo(points.last.dx, chartBottom)
      ..lineTo(points.first.dx, chartBottom)
      ..close();
    final fillPaint = Paint()
      ..shader = LinearGradient(
        begin: Alignment.topCenter,
        end: Alignment.bottomCenter,
        colors: [
          kProviderGreen.withValues(alpha: 0.45),
          kProviderGreen.withValues(alpha: 0.02),
        ],
      ).createShader(Rect.fromLTRB(0, chartTop, size.width, chartBottom));
    canvas.drawPath(fillPath, fillPaint);

    // The line itself.
    canvas.drawPath(
      linePath,
      Paint()
        ..color = kProviderGreen
        ..style = PaintingStyle.stroke
        ..strokeWidth = 2.5
        ..strokeCap = StrokeCap.round
        ..strokeJoin = StrokeJoin.round,
    );

    _drawMonthLabels(canvas, chartBottom, xFor);

    if (highlightIndex >= 0) {
      _drawHighlight(canvas, size, points[highlightIndex]);
    }
  }

  void _drawGridlines(Canvas canvas, Size size, double top, double bottom) {
    const lines = 4;
    final paint = Paint()
      ..color = kProviderBorder
      ..strokeWidth = 1;
    for (var i = 0; i <= lines; i++) {
      final y = top + (bottom - top) * i / lines;
      _dashedLine(canvas, Offset(0, y), Offset(size.width, y), paint);
    }
  }

  void _dashedLine(Canvas canvas, Offset start, Offset end, Paint paint) {
    const dash = 5.0;
    const gap = 4.0;
    final total = (end - start).distance;
    final dir = (end - start) / total;
    var d = 0.0;
    while (d < total) {
      final s = start + dir * d;
      final e = start + dir * (d + dash).clamp(0, total);
      canvas.drawLine(s, e, paint);
      d += dash + gap;
    }
  }

  /// Catmull-Rom spline → cubic Beziers for a smooth curve through the points.
  Path _smoothPath(List<Offset> pts) {
    final path = Path()..moveTo(pts.first.dx, pts.first.dy);
    for (var i = 0; i < pts.length - 1; i++) {
      final p0 = i == 0 ? pts[i] : pts[i - 1];
      final p1 = pts[i];
      final p2 = pts[i + 1];
      final p3 = i + 2 < pts.length ? pts[i + 2] : pts[i + 1];
      final c1 = Offset(p1.dx + (p2.dx - p0.dx) / 6, p1.dy + (p2.dy - p0.dy) / 6);
      final c2 = Offset(p2.dx - (p3.dx - p1.dx) / 6, p2.dy - (p3.dy - p1.dy) / 6);
      path.cubicTo(c1.dx, c1.dy, c2.dx, c2.dy, p2.dx, p2.dy);
    }
    return path;
  }

  void _drawMonthLabels(Canvas canvas, double chartBottom, double Function(int) xFor) {
    for (var i = 0; i < months.length; i++) {
      final tp = TextPainter(
        text: TextSpan(
          text: months[i],
          style: const TextStyle(color: kProviderMuted, fontSize: 10.5),
        ),
        textDirection: TextDirection.ltr,
      )..layout();
      tp.paint(canvas, Offset(xFor(i) - tp.width / 2, chartBottom + 8));
    }
  }

  void _drawHighlight(Canvas canvas, Size size, Offset point) {
    // Dot.
    canvas.drawCircle(point, 6, Paint()..color = Colors.white);
    canvas.drawCircle(
      point,
      6,
      Paint()
        ..color = kProviderGreen
        ..style = PaintingStyle.stroke
        ..strokeWidth = 3,
    );

    // Tooltip above the dot.
    final tp = TextPainter(
      text: TextSpan(
        text: highlightLabel,
        style: const TextStyle(color: Colors.white, fontSize: 11, fontWeight: FontWeight.w700),
      ),
      textDirection: TextDirection.ltr,
    )..layout();

    const padH = 10.0;
    const padV = 6.0;
    final bubbleW = tp.width + padH * 2;
    final bubbleH = tp.height + padV * 2;
    // Center over the dot, but keep the whole bubble within the chart bounds.
    final left = (point.dx - bubbleW / 2).clamp(2.0, size.width - bubbleW - 2.0);
    final top = point.dy - bubbleH - 12;
    final rect = RRect.fromRectAndRadius(
      Rect.fromLTWH(left, top, bubbleW, bubbleH),
      const Radius.circular(8),
    );
    canvas.drawRRect(rect, Paint()..color = kProviderGreen);
    // Little pointer triangle.
    final tri = Path()
      ..moveTo(point.dx - 5, top + bubbleH)
      ..lineTo(point.dx + 5, top + bubbleH)
      ..lineTo(point.dx, top + bubbleH + 6)
      ..close();
    canvas.drawPath(tri, Paint()..color = kProviderGreen);
    tp.paint(canvas, Offset(left + padH, top + padV));
  }

  @override
  bool shouldRepaint(covariant _ChartPainter old) =>
      old.values != values || old.highlightIndex != highlightIndex;
}
