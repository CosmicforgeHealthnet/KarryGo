import 'package:flutter/material.dart';
import '../../../requests/data/request_model.dart';
import '../../../../core/theme/theme_extensions.dart';
import '../../../../shared/widgets/journey_icon.dart';

class TripStats extends StatelessWidget {
  const TripStats({super.key, required this.request});
  final RequestModel request;
  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Wrap(
      spacing: 10,
      runSpacing: 8,
      crossAxisAlignment: WrapCrossAlignment.center,
      children: [
        _chip(context, JourneySymbol.route, request.distanceDisplay),
        _chip(context, JourneySymbol.wallet, request.fareDisplayGrouped),
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 7),
          decoration: BoxDecoration(
            color: request.isExpress ? c.warningSoft : c.surfaceElevated,
            borderRadius: BorderRadius.circular(10),
          ),
          child: Text(
            request.serviceTierLabel ??
                (request.isExpress ? 'Express' : 'Standard'),
            style: Theme.of(context).textTheme.bodySmall?.copyWith(
              color: request.isExpress ? c.warning : c.textMuted,
            ),
          ),
        ),
      ],
    );
  }

  Widget _chip(BuildContext context, JourneySymbol icon, String label) => Row(
    mainAxisSize: MainAxisSize.min,
    children: [
      JourneyIcon(icon, size: 18, color: context.palette.textMuted),
      const SizedBox(width: 6),
      Text(label, style: Theme.of(context).textTheme.bodySmall),
    ],
  );
}
