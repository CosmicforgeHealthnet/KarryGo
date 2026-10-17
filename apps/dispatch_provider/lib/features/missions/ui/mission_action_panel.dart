import 'package:flutter/material.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/journey_icon.dart';

class MissionTaskAction {
  const MissionTaskAction({
    required this.label,
    required this.onTap,
    required this.symbol,
    this.description,
    this.color,
  });
  final String label;
  final String? description;
  final JourneySymbol symbol;
  final VoidCallback onTap;
  final Color? color;
}

class MissionActionPanel extends StatelessWidget {
  const MissionActionPanel({
    super.key,
    this.primary,
    this.actions = const [],
    this.title = 'Delivery tools',
    this.busy = false,
    this.busyLabel,
  });
  final MissionTaskAction? primary;
  final List<MissionTaskAction> actions;
  final String title;
  final bool busy;
  final String? busyLabel;
  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.stretch,
    children: [
      if (primary != null) ...[
        Text('Next step', style: Theme.of(context).textTheme.titleSmall),
        const SizedBox(height: 12),
        if (primary!.description != null) ...[
          Text(primary!.description!),
          const SizedBox(height: 14),
        ],
        FilledButton.icon(
          onPressed: busy ? null : primary!.onTap,
          icon: busy && busyLabel == primary!.label
              ? const SizedBox.square(
                  dimension: 18,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : JourneyIcon(
                  primary!.symbol,
                  color: context.palette.btnPrimaryFg,
                  size: 22,
                ),
          label: Text(primary!.label),
        ),
        const SizedBox(height: 24),
      ],
      if (actions.isNotEmpty) ...[
        Text(title, style: Theme.of(context).textTheme.titleSmall),
        const SizedBox(height: 12),
        DeliveryPanel(
          padding: EdgeInsets.zero,
          child: Column(
            children: [
              for (var i = 0; i < actions.length; i++) ...[
                if (i > 0) const Divider(height: 1, indent: 74, endIndent: 16),
                AccountMenuTile(
                  title: actions[i].label,
                  subtitle: actions[i].description,
                  symbol: actions[i].symbol,
                  color: actions[i].color,
                  onTap: busy ? null : actions[i].onTap,
                  trailing: busy && busyLabel == actions[i].label
                      ? const SizedBox.square(
                          dimension: 18,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : Icon(
                          Icons.chevron_right_rounded,
                          color: context.palette.textMuted,
                        ),
                ),
              ],
            ],
          ),
        ),
      ],
    ],
  );
}
