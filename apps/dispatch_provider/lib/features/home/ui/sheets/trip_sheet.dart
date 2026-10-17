import 'package:flutter/material.dart';
import '../../../requests/data/request_model.dart';
import '../../../../core/theme/theme_extensions.dart';
import '../../../../shared/widgets/journey_icon.dart';
import '../widgets/trip_stats.dart';
import '../widgets/route_info.dart';
import '../widgets/trip_peek_bar.dart';

class TripSheet extends StatelessWidget {
  const TripSheet({
    super.key,
    required this.request,
    required this.title,
    required this.subtitle,
    required this.showCall,
    required this.primaryLabel,
    required this.primaryEnabled,
    required this.secondaryLabel,
    required this.onPrimary,
    required this.onSecondary,
    required this.collapsed,
    required this.onToggleCollapse,
    this.peekStatusText = '',
    this.extra,
    this.busy = false,
  });
  final RequestModel request;
  final String title, subtitle, primaryLabel, secondaryLabel, peekStatusText;
  final bool showCall, primaryEnabled, collapsed, busy;
  final VoidCallback onPrimary, onSecondary, onToggleCollapse;
  final Widget? extra;
  @override
  Widget build(BuildContext context) {
    if (collapsed)
      return TripPeekBar(
        title: title,
        statusText: peekStatusText,
        onExpand: onToggleCollapse,
      );
    final c = context.palette;
    return Padding(
      padding: const EdgeInsets.all(20),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Container(
                width: 56,
                height: 56,
                decoration: BoxDecoration(
                  color: c.surfaceElevated,
                  borderRadius: BorderRadius.circular(18),
                ),
                child: const Center(
                  child: JourneyIcon(JourneySymbol.parcel, size: 34),
                ),
              ),
              const Spacer(),
              TextButton.icon(
                onPressed: onToggleCollapse,
                icon: const Icon(Icons.map_outlined, size: 18),
                label: const Text('View map'),
              ),
            ],
          ),
          const SizedBox(height: 24),
          Text(title, style: Theme.of(context).textTheme.headlineMedium),
          const SizedBox(height: 10),
          Text(subtitle, style: Theme.of(context).textTheme.bodyLarge),
          const SizedBox(height: 24),
          Container(
            padding: const EdgeInsets.all(18),
            decoration: BoxDecoration(
              color: c.surface,
              borderRadius: BorderRadius.circular(24),
              border: Border.all(color: c.divider),
            ),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(
                  request.customerName,
                  style: Theme.of(context).textTheme.titleMedium,
                ),
                const SizedBox(height: 12),
                TripStats(request: request),
                const Padding(
                  padding: EdgeInsets.symmetric(vertical: 18),
                  child: Divider(height: 1),
                ),
                RouteInfo(
                  pickup: request.pickupAddress,
                  dropoff: request.dropoffAddress,
                ),
                if (request.hasNotes) ...[
                  const SizedBox(height: 20),
                  Text(
                    'Delivery notes',
                    style: Theme.of(context).textTheme.titleSmall,
                  ),
                  const SizedBox(height: 6),
                  Text(
                    request.notes!,
                    style: Theme.of(context).textTheme.bodyMedium,
                  ),
                ],
                if (extra != null) ...[const SizedBox(height: 20), extra!],
              ],
            ),
          ),
          const SizedBox(height: 24),
          FilledButton(
            onPressed: primaryEnabled && !busy ? onPrimary : null,
            child: busy
                ? const SizedBox.square(
                    dimension: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : Text(primaryLabel, textAlign: TextAlign.center),
          ),
          const SizedBox(height: 12),
          OutlinedButton(
            onPressed: busy ? null : onSecondary,
            style: OutlinedButton.styleFrom(
              foregroundColor: c.danger,
              side: BorderSide(color: c.danger.withValues(alpha: .3)),
            ),
            child: Text(secondaryLabel, textAlign: TextAlign.center),
          ),
          const SizedBox(height: 16),
        ],
      ),
    );
  }
}
