import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';

/// A labelled address point used in route summary cards (pickup / dropoff).
class HaulingRoutePoint extends StatelessWidget {
  const HaulingRoutePoint({
    super.key,
    required this.label,
    required this.address,
    required this.color,
  });

  final String label;
  final String address;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Container(
          width: 12,
          height: 12,
          decoration: BoxDecoration(color: color, shape: BoxShape.circle),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                label,
                style: const TextStyle(
                  color: CustomerFigmaColors.muted,
                  fontSize: 11,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 2),
              Text(
                address,
                style: const TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 13,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}
