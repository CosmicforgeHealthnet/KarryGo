import 'package:flutter/material.dart';
import '../../../requests/data/request_model.dart';
import '../../../../core/theme/theme_extensions.dart';
import '../../../../shared/widgets/journey_icon.dart';
import 'trip_stats.dart';
import 'route_info.dart';

class RequestCard extends StatelessWidget {
  const RequestCard({
    super.key,
    required this.request,
    required this.onAccept,
    required this.onReject,
    this.busy = false,
    this.detailed = false,
  });
  final RequestModel request;
  final VoidCallback onAccept;
  final VoidCallback onReject;
  final bool busy;
  final bool detailed;
  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Container(
      padding: EdgeInsets.all(detailed ? 16 : 12),
      decoration: BoxDecoration(
        color: c.surface,
        borderRadius: BorderRadius.circular(22),
        border: Border.all(color: c.divider),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Container(
                width: 40,
                height: 40,
                decoration: BoxDecoration(
                  color: c.surfaceElevated,
                  borderRadius: BorderRadius.circular(14),
                ),
                child: const Center(
                  child: JourneyIcon(JourneySymbol.parcel, size: 25),
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      request.customerName,
                      style: Theme.of(context).textTheme.titleSmall,
                    ),
                    const SizedBox(height: 4),
                    Text(
                      'Delivery request',
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                  ],
                ),
              ),
              if (!detailed)
                TextButton(
                  onPressed: busy ? null : () => _details(context),
                  child: const Text('Details'),
                ),
            ],
          ),
          const SizedBox(height: 10),
          TripStats(request: request),
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 10),
            child: Divider(height: 1),
          ),
          RouteInfo(
            pickup: request.pickupAddress,
            dropoff: request.dropoffAddress,
            compact: !detailed,
          ),
          if (detailed && (request.itemSummary?.isNotEmpty ?? false)) ...[
            const SizedBox(height: 12),
            Text('Items', style: Theme.of(context).textTheme.titleSmall),
            const SizedBox(height: 6),
            Text(
              request.itemSummary!,
              style: Theme.of(context).textTheme.bodyMedium,
            ),
          ],
          if (detailed && request.handlingTags.isNotEmpty) ...[
            const SizedBox(height: 12),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                for (final tag in request.handlingTags) Chip(label: Text(tag)),
              ],
            ),
          ],
          if (detailed && request.hasNotes) ...[
            const SizedBox(height: 12),
            Text(
              'Delivery notes',
              style: Theme.of(context).textTheme.titleSmall,
            ),
            const SizedBox(height: 6),
            Text(request.notes!, style: Theme.of(context).textTheme.bodyMedium),
          ],
          const SizedBox(height: 12),
          Row(
            children: [
              Expanded(
                child: OutlinedButton(
                  onPressed: busy ? null : onReject,
                  style: OutlinedButton.styleFrom(
                    foregroundColor: c.danger,
                    side: BorderSide(color: c.danger.withValues(alpha: .3)),
                    padding: const EdgeInsets.symmetric(horizontal: 10),
                  ),
                  child: const Text('Decline', maxLines: 1),
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: FilledButton(
                  onPressed: busy ? null : onAccept,
                  style: FilledButton.styleFrom(
                    padding: const EdgeInsets.symmetric(horizontal: 12),
                  ),
                  child: busy
                      ? const SizedBox.square(
                          dimension: 20,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : const Text('Accept', maxLines: 1),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }

  Future<void> _details(BuildContext context) => showModalBottomSheet<void>(
    context: context,
    useSafeArea: true,
    isScrollControlled: true,
    showDragHandle: true,
    barrierColor: Colors.black.withValues(alpha: .5),
    builder: (sheet) => FractionallySizedBox(
      heightFactor: .88,
      child: SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(16, 0, 16, 24),
        child: RequestCard(
          request: request,
          detailed: true,
          onAccept: () {
            Navigator.pop(sheet);
            onAccept();
          },
          onReject: () {
            Navigator.pop(sheet);
            onReject();
          },
        ),
      ),
    ),
  );
}
