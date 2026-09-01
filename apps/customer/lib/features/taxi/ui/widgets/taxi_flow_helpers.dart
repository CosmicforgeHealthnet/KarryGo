import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';

export '../../../hauling/ui/widgets/hauling_trip_sheet_widgets.dart'
    show formatHaulingFare;

/// Wraps each taxi flow view in a consistent scaffold, mirroring
/// `haulingFlowScaffold` so the three booking flows feel identical.
Widget taxiFlowScaffold({
  required Widget body,
  String? title,
  VoidCallback? onBack,
  Widget? bottom,
  Color bg = CustomerFigmaColors.surface,
}) {
  return Scaffold(
    backgroundColor: bg,
    appBar: title != null
        ? AppBar(
            backgroundColor: bg,
            elevation: 0,
            surfaceTintColor: Colors.transparent,
            leading: onBack != null
                ? IconButton(
                    onPressed: onBack,
                    icon: const Icon(Icons.arrow_back_rounded),
                    color: CustomerFigmaColors.text,
                  )
                : null,
            title: Text(
              title,
              style: const TextStyle(
                color: CustomerFigmaColors.text,
                fontWeight: FontWeight.w800,
                fontSize: 17,
              ),
            ),
          )
        : null,
    body: SafeArea(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 8, 20, 20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Expanded(child: body),
            ?bottom,
          ],
        ),
      ),
    ),
  );
}

Widget taxiSectionLabel(String text) => Padding(
  padding: const EdgeInsets.only(bottom: 6),
  child: Text(
    text,
    style: const TextStyle(
      color: CustomerFigmaColors.text,
      fontSize: 13,
      fontWeight: FontWeight.w800,
    ),
  ),
);

/// "12 min" style duration label.
String taxiDurationLabel(int minutes) =>
    minutes <= 1 ? '1 min' : '$minutes min';

/// "8.1 km" style distance label.
String taxiDistanceLabel(double km) => '${km.toStringAsFixed(1)} km';

/// The green/orange dotted route summary shown above the fare tiles and on
/// the active-trip sheet.
class TaxiRouteSummary extends StatelessWidget {
  const TaxiRouteSummary({
    super.key,
    required this.pickupAddress,
    required this.dropoffAddress,
  });

  final String pickupAddress;
  final String dropoffAddress;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: CustomerFigmaColors.field,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: CustomerFigmaColors.border),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Column(
            children: [
              const Icon(
                Icons.radio_button_checked,
                size: 16,
                color: CustomerFigmaColors.success,
              ),
              SizedBox(
                height: 26,
                child: CustomPaint(
                  size: const Size(2, 26),
                  painter: _TaxiDottedLinePainter(),
                ),
              ),
              const Icon(
                Icons.location_on,
                size: 16,
                color: Color(0xFFE9A13B),
              ),
            ],
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                _line(pickupAddress.isEmpty ? 'Pick-up' : pickupAddress),
                const SizedBox(height: 22),
                _line(dropoffAddress.isEmpty ? 'Destination' : dropoffAddress),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _line(String text) => Text(
    text,
    maxLines: 1,
    overflow: TextOverflow.ellipsis,
    style: const TextStyle(
      color: CustomerFigmaColors.text,
      fontSize: 13,
      fontWeight: FontWeight.w600,
    ),
  );
}

class _TaxiDottedLinePainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = CustomerFigmaColors.primarySoft
      ..strokeWidth = 2
      ..strokeCap = StrokeCap.round;
    const dash = 3.0;
    const gap = 3.0;
    var y = 0.0;
    while (y < size.height) {
      canvas.drawLine(
        Offset(size.width / 2, y),
        Offset(size.width / 2, (y + dash).clamp(0, size.height)),
        paint,
      );
      y += dash + gap;
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
