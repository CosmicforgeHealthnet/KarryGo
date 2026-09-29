import 'package:flutter/material.dart';

import '../../../requests/data/request_model.dart';
import '../../../../core/theme/theme_extensions.dart';

class TripStats extends StatelessWidget {
  const TripStats({super.key, required this.request});
  final RequestModel request;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final tierLabel = request.serviceTierLabel ??
        (request.isExpress ? 'Express Delivery' : 'Standard Delivery');
    final tierColor = request.isExpress ? colors.warning : colors.textMuted;

    return Row(
      children: [
        Icon(Icons.av_timer, size: 14, color: colors.textMuted),
        const SizedBox(width: 4),
        Text(
          request.distanceDisplay,
          style: TextStyle(fontSize: 12, color: colors.textMuted),
        ),
        const SizedBox(width: 12),
        Icon(Icons.attach_money, size: 14, color: colors.textMuted),
        const SizedBox(width: 4),
        Text(
          request.fareDisplay,
          style: TextStyle(fontSize: 12, color: colors.textMuted),
        ),
        const SizedBox(width: 12),
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
          decoration: BoxDecoration(
            color: tierColor.withAlpha(26),
            borderRadius: BorderRadius.circular(4),
            border: Border.all(color: tierColor.withAlpha(77)),
          ),
          child: Text(
            tierLabel,
            style: TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w600,
              color: tierColor,
            ),
          ),
        ),
      ],
    );
  }
}