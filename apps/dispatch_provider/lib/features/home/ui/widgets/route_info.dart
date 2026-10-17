import 'package:flutter/material.dart';
import '../../../../core/theme/theme_extensions.dart';
import '../../../../shared/widgets/journey_icon.dart';

class RouteInfo extends StatelessWidget {
  const RouteInfo({
    super.key,
    required this.pickup,
    required this.dropoff,
    this.compact = false,
  });
  final String pickup;
  final String dropoff;
  final bool compact;
  @override
  Widget build(BuildContext context) => Column(
    mainAxisSize: MainAxisSize.min,
    children: [
      _stop(context, 'PICKUP', pickup, JourneySymbol.parcel),
      Padding(
        padding: const EdgeInsets.only(left: 17),
        child: Align(
          alignment: Alignment.centerLeft,
          child: Container(
            height: compact ? 8 : 14,
            width: 1,
            color: context.palette.divider,
          ),
        ),
      ),
      _stop(context, 'DELIVERY', dropoff, JourneySymbol.route),
    ],
  );
  Widget _stop(
    BuildContext context,
    String title,
    String address,
    JourneySymbol symbol,
  ) => Row(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Container(
        width: 36,
        height: 36,
        decoration: BoxDecoration(
          color: context.palette.surfaceElevated,
          borderRadius: BorderRadius.circular(12),
        ),
        child: Center(child: JourneyIcon(symbol, size: 21)),
      ),
      const SizedBox(width: 12),
      Expanded(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              title,
              style: Theme.of(context).textTheme.labelSmall?.copyWith(
                fontSize: 9,
                letterSpacing: 1.5,
                color: context.palette.textMuted,
              ),
            ),
            const SizedBox(height: 4),
            Text(
              address,
              maxLines: compact ? 2 : null,
              overflow: compact ? TextOverflow.ellipsis : null,
              style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                color: context.palette.text,
                fontWeight: FontWeight.w600,
                fontSize: compact ? 12 : 13,
                height: compact ? 1.3 : 1.5,
              ),
            ),
          ],
        ),
      ),
    ],
  );
}
