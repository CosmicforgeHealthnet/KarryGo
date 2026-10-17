import 'package:flutter/material.dart';
import '../../core/theme/theme_extensions.dart';
import '../../features/trips/data/trip_model.dart';

class DeliveryCollectionHeader extends StatelessWidget {
  const DeliveryCollectionHeader({
    super.key,
    required this.title,
    required this.subtitle,
    this.action,
  });
  final String title, subtitle;
  final Widget? action;
  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.fromLTRB(20, 20, 20, 18),
    child: Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(title, style: Theme.of(context).textTheme.headlineMedium),
              const SizedBox(height: 7),
              Text(subtitle, style: Theme.of(context).textTheme.bodyMedium),
            ],
          ),
        ),
        if (action != null) ...[const SizedBox(width: 8), action!],
      ],
    ),
  );
}

class DeliveryPanel extends StatelessWidget {
  const DeliveryPanel({
    super.key,
    required this.child,
    this.padding = const EdgeInsets.all(18),
    this.onTap,
  });
  final Widget child;
  final EdgeInsets padding;
  final VoidCallback? onTap;
  @override
  Widget build(BuildContext context) => Material(
    clipBehavior: Clip.antiAlias,
    shape: RoundedRectangleBorder(
      borderRadius: BorderRadius.circular(24),
      side: BorderSide(color: context.palette.divider),
    ),
    color: context.palette.surface,
    child: InkWell(
      onTap: onTap,
      child: Padding(padding: padding, child: child),
    ),
  );
}

class DeliveryFilter extends StatelessWidget {
  const DeliveryFilter({
    super.key,
    required this.label,
    required this.selected,
    required this.onTap,
  });
  final String label;
  final bool selected;
  final VoidCallback onTap;
  @override
  Widget build(BuildContext context) => ChoiceChip(
    label: Text(label),
    selected: selected,
    showCheckmark: false,
    onSelected: (_) => onTap(),
    padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 10),
    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
    labelStyle: Theme.of(context).textTheme.bodyMedium?.copyWith(
      fontWeight: FontWeight.w700,
      color: selected
          ? context.palette.brandTeal600
          : context.palette.textMuted,
    ),
  );
}

class TripStatusBadge extends StatelessWidget {
  const TripStatusBadge({super.key, required this.status});
  final TripStatusCode status;
  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final color = status.isCompleted
        ? c.success
        : status.isCancelled || status.isFailed
        ? c.danger
        : status == TripStatusCode.arrivedPickup ||
              status == TripStatusCode.proofSubmitted
        ? c.warning
        : c.brandTeal600;
    final label = switch (status) {
      TripStatusCode.assigned => 'Assigned',
      TripStatusCode.enRoutePickup => 'To pickup',
      TripStatusCode.arrivedPickup => 'At pickup',
      TripStatusCode.inProgress => 'In progress',
      TripStatusCode.proofSubmitted => 'Proof submitted',
      TripStatusCode.completed => 'Completed',
      TripStatusCode.cancelled => 'Cancelled',
      TripStatusCode.failed => 'Failed',
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 7),
      decoration: BoxDecoration(
        color: color.withValues(alpha: .1),
        borderRadius: BorderRadius.circular(10),
      ),
      child: Text(
        label,
        style: Theme.of(context).textTheme.bodySmall?.copyWith(
          color: color,
          fontWeight: FontWeight.w700,
        ),
      ),
    );
  }
}

String deliveryDate(DateTime date, {bool time = false}) {
  final d = date.toLocal();
  const months = [
    'Jan',
    'Feb',
    'Mar',
    'Apr',
    'May',
    'Jun',
    'Jul',
    'Aug',
    'Sep',
    'Oct',
    'Nov',
    'Dec',
  ];
  return '${d.day} ${months[d.month - 1]} ${d.year}${time ? ' / ${d.hour.toString().padLeft(2, '0')}:${d.minute.toString().padLeft(2, '0')}' : ''}';
}
