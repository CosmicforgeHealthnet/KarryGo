import 'package:flutter/material.dart';
import '../../../../core/theme/theme_extensions.dart';
import '../../../../shared/widgets/journey_icon.dart';
import '../../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../home/ui/widgets/route_info.dart';
import '../../data/request_model.dart';

class OpportunityCard extends StatelessWidget {
  const OpportunityCard({
    super.key,
    required this.request,
    required this.onTap,
    required this.onAccept,
    required this.onDecline,
    this.busy = false,
    this.detailed = false,
  });
  final RequestModel request;
  final VoidCallback onTap, onAccept, onDecline;
  final bool busy, detailed;
  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final unavailable =
        request.isExpired || request.lifecycle == RequestLifecycleState.taken;
    final updated = request.lifecycle == RequestLifecycleState.requoted;
    final status = request.isExpired
        ? 'Expired'
        : request.lifecycle == RequestLifecycleState.taken
        ? 'Taken'
        : updated
        ? 'Offer updated'
        : request.source.label;
    final color = unavailable
        ? c.textMuted
        : updated
        ? c.warning
        : c.brandTeal600;
    return DeliveryPanel(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Container(
                width: 44,
                height: 44,
                decoration: BoxDecoration(
                  color: c.surfaceElevated,
                  borderRadius: BorderRadius.circular(15),
                ),
                child: Center(
                  child: JourneyIcon(
                    request.source == RequestSource.assignment
                        ? JourneySymbol.partner
                        : JourneySymbol.parcel,
                    size: 27,
                  ),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      status,
                      style: Theme.of(
                        context,
                      ).textTheme.titleSmall?.copyWith(color: color),
                    ),
                    if (request.hasSourceOrg)
                      Text(
                        request.sourceOrgName!,
                        style: Theme.of(context).textTheme.bodySmall,
                      ),
                  ],
                ),
              ),
              if (!detailed)
                IconButton(
                  tooltip: 'View request details',
                  onPressed: busy ? null : onTap,
                  icon: const Icon(Icons.arrow_outward_rounded, size: 21),
                ),
            ],
          ),
          if (request.hasFare) ...[
            const SizedBox(height: 18),
            Text(
              'Estimated fare',
              style: Theme.of(context).textTheme.bodySmall,
            ),
            const SizedBox(height: 4),
            Wrap(
              spacing: 10,
              runSpacing: 6,
              crossAxisAlignment: WrapCrossAlignment.center,
              children: [
                Text(
                  request.fareDisplayGrouped,
                  style: Theme.of(context).textTheme.headlineSmall,
                ),
                if (updated && request.previousFareDisplayGrouped != null)
                  Text(
                    request.previousFareDisplayGrouped!,
                    style: Theme.of(context).textTheme.bodySmall?.copyWith(
                      decoration: TextDecoration.lineThrough,
                    ),
                  ),
              ],
            ),
          ],
          const SizedBox(height: 14),
          Wrap(
            spacing: 12,
            runSpacing: 8,
            children: [
              if (request.hasDistance)
                Text(
                  request.distanceDisplay,
                  style: Theme.of(context).textTheme.bodySmall,
                ),
              if (request.serviceTierLabel != null)
                Text(
                  request.serviceTierLabel!,
                  style: Theme.of(context).textTheme.bodySmall,
                ),
              if (!unavailable && request.minutesRemaining != null)
                Text(
                  request.minutesRemaining == 0
                      ? 'Under 1 min left'
                      : '${request.minutesRemaining} min left',
                  style: Theme.of(
                    context,
                  ).textTheme.bodySmall?.copyWith(color: c.warning),
                ),
            ],
          ),
          if (request.hasRoute) ...[
            const Padding(
              padding: EdgeInsets.symmetric(vertical: 16),
              child: Divider(height: 1),
            ),
            RouteInfo(
              pickup: request.pickupAddress,
              dropoff: request.dropoffAddress,
              compact: !detailed,
            ),
          ] else ...[
            const SizedBox(height: 12),
            Text(
              request.bookingId.isNotEmpty ? request.bookingId : request.id,
              style: Theme.of(context).textTheme.bodyMedium,
            ),
            const SizedBox(height: 6),
            Text(
              'This offer does not include route details yet.',
              style: Theme.of(context).textTheme.bodySmall,
            ),
          ],
          if (request.hasPackage) ...[
            const SizedBox(height: 16),
            Text(
              request.itemSummary!,
              style: Theme.of(context).textTheme.bodyMedium,
            ),
          ],
          if (detailed && request.hasNotes) ...[
            const SizedBox(height: 16),
            Text('Instructions', style: Theme.of(context).textTheme.titleSmall),
            const SizedBox(height: 6),
            Text(request.notes!, style: Theme.of(context).textTheme.bodyLarge),
          ],
          if (detailed && request.hasHandling) ...[
            const SizedBox(height: 12),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                for (final tag in request.handlingTags) Chip(label: Text(tag)),
              ],
            ),
          ],
          if (!detailed) ...[
            const SizedBox(height: 18),
            if (unavailable)
              OutlinedButton(
                onPressed: onTap,
                child: const Text('View details'),
              )
            else
              Row(
                children: [
                  Expanded(
                    child: OutlinedButton(
                      onPressed: busy ? null : onDecline,
                      style: OutlinedButton.styleFrom(
                        foregroundColor: c.danger,
                        side: BorderSide(color: c.danger.withValues(alpha: .3)),
                        padding: const EdgeInsets.symmetric(horizontal: 8),
                      ),
                      child: const Text('Decline'),
                    ),
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: FilledButton(
                      onPressed: busy ? null : onAccept,
                      style: FilledButton.styleFrom(
                        padding: const EdgeInsets.symmetric(horizontal: 8),
                      ),
                      child: busy
                          ? const SizedBox.square(
                              dimension: 18,
                              child: CircularProgressIndicator(strokeWidth: 2),
                            )
                          : const Text('Accept'),
                    ),
                  ),
                ],
              ),
          ],
        ],
      ),
    );
  }
}
