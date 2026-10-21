import 'package:flutter/material.dart';

import '../../../../core/format/date_format.dart';
import '../../../../core/widgets/karryam_ui.dart';
import '../../../notifications/models/provider_notification.dart';
import '../../../notifications/state/provider_notification_controller.dart';
import '../widgets/provider_app_colors.dart';

/// The driver's notification feed, grouped by day.
class ProviderNotificationsScreen extends StatefulWidget {
  const ProviderNotificationsScreen({super.key, required this.controller});

  final ProviderNotificationController controller;

  @override
  State<ProviderNotificationsScreen> createState() => _ProviderNotificationsScreenState();
}

class _ProviderNotificationsScreenState extends State<ProviderNotificationsScreen> {
  @override
  void initState() {
    super.initState();
    widget.controller.openFeed();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: const KTopBar(title: 'Notifications'),
      body: AnimatedBuilder(
        animation: widget.controller,
        builder: (context, _) {
          final controller = widget.controller;
          final items = controller.notifications;

          if (controller.isLoading && items.isEmpty) {
            return const Center(child: CircularProgressIndicator());
          }
          if (controller.error != null && items.isEmpty) {
            return Center(
              child: KEmptyState(
                icon: Icons.cloud_off_rounded,
                title: "Couldn't load updates",
                message: controller.error!,
                actionLabel: 'Try again',
                onAction: controller.refresh,
              ),
            );
          }

          final groups = <String, List<ProviderNotification>>{};
          for (final item in items) {
            groups.putIfAbsent(formatRelativeDay(item.createdAt), () => []).add(item);
          }

          return RefreshIndicator(
            onRefresh: controller.refresh,
            child: ListView(
              physics: const AlwaysScrollableScrollPhysics(),
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 32),
              children: [
                const KHeading(
                  title: 'Your updates.',
                  subtitle: 'Requests, trips, payouts and account news.',
                ),
                if (items.isEmpty)
                  const KEmptyState(
                    icon: Icons.notifications_none_rounded,
                    title: 'Nothing new yet',
                    message: 'Booking and payout updates will show up here.',
                  ),
                for (final entry in groups.entries) ...[
                  const SizedBox(height: 22),
                  KEyebrow(entry.key),
                  const SizedBox(height: 10),
                  KGroup(
                    children: [
                      for (final item in entry.value)
                        _NotificationRow(item: item, isUnread: controller.isUnread(item)),
                    ],
                  ),
                ],
              ],
            ),
          );
        },
      ),
    );
  }
}

class _NotificationRow extends StatelessWidget {
  const _NotificationRow({required this.item, required this.isUnread});

  final ProviderNotification item;
  final bool isUnread;

  IconData get _icon {
    final t = '${item.eventType} ${item.title}'.toLowerCase();
    if (t.contains('withdraw') || t.contains('payout') || t.contains('transfer')) return Icons.north_east_rounded;
    if (t.contains('cancel') || t.contains('unmatched')) return Icons.close_rounded;
    if (t.contains('deliver') || t.contains('complete')) return Icons.check_rounded;
    if (t.contains('verif') || t.contains('approv')) return Icons.verified_outlined;
    if (t.contains('match') || t.contains('request')) return Icons.inbox_rounded;
    if (t.contains('pickup') || t.contains('picked')) return Icons.inventory_2_outlined;
    return Icons.notifications_none_rounded;
  }

  @override
  Widget build(BuildContext context) {
    final hasTitle = item.title.isNotEmpty;
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          KIconTile(
            _icon,
            color: isUnread ? kTealText : kProviderMuted,
            background: isUnread ? kTealTint : const Color(0xFFEEF0F3),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  hasTitle ? item.title : item.body,
                  style: TextStyle(
                    fontSize: 14,
                    fontWeight: isUnread ? FontWeight.w700 : FontWeight.w600,
                    color: kProviderText,
                  ),
                ),
                if (hasTitle && item.body.isNotEmpty) ...[
                  const SizedBox(height: 3),
                  Text(
                    item.body,
                    style: const TextStyle(fontSize: 12.5, height: 1.4, color: kProviderMuted),
                  ),
                ],
                const SizedBox(height: 4),
                Text(formatClock(item.createdAt), style: const TextStyle(fontSize: 11.5, color: kProviderMuted)),
              ],
            ),
          ),
          if (isUnread)
            Container(
              margin: const EdgeInsets.only(left: 8, top: 4),
              width: 8,
              height: 8,
              decoration: const BoxDecoration(color: kTeal, shape: BoxShape.circle),
            ),
        ],
      ),
    );
  }
}
