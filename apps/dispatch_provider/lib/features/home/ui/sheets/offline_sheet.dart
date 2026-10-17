import 'package:flutter/material.dart';
import '../../../../core/theme/theme_extensions.dart';
import '../../../../shared/widgets/journey_icon.dart';

class OfflineSheet extends StatelessWidget {
  const OfflineSheet({super.key, required this.onGoOnline, this.busy = false});
  final VoidCallback onGoOnline;
  final bool busy;
  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Container(
      margin: const EdgeInsets.fromLTRB(12, 0, 12, 12),
      padding: const EdgeInsets.all(20),
      decoration: BoxDecoration(
        color: c.surface,
        borderRadius: BorderRadius.circular(28),
        border: Border.all(color: c.divider),
        boxShadow: c.cardShadow,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Container(
                width: 48,
                height: 48,
                decoration: BoxDecoration(
                  color: c.surfaceElevated,
                  borderRadius: BorderRadius.circular(16),
                ),
                child: const Center(
                  child: JourneyIcon(JourneySymbol.route, size: 28),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      'Ready when you are.',
                      style: Theme.of(context).textTheme.titleMedium,
                    ),
                    const SizedBox(height: 4),
                    Text(
                      'You are currently offline',
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 16),
          FilledButton(
            onPressed: busy ? null : onGoOnline,
            child: Row(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                if (busy)
                  const SizedBox.square(
                    dimension: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                else
                  const Icon(Icons.power_settings_new_rounded, size: 20),
                const SizedBox(width: 10),
                Text(busy ? 'Connecting...' : 'Go online'),
                const SizedBox(width: 16),
                const Icon(Icons.arrow_forward_rounded, size: 19),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
