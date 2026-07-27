import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../../notifications/models/app_notification.dart';
import '../../../notifications/state/notification_controller.dart';

/// Alerts tab — notification feed grouped by day (Figma design).
class CustomerNotificationsTab extends StatefulWidget {
  const CustomerNotificationsTab({super.key, required this.controller});

  final NotificationController controller;

  @override
  State<CustomerNotificationsTab> createState() => _CustomerNotificationsTabState();
}

class _CustomerNotificationsTabState extends State<CustomerNotificationsTab> {
  @override
  void initState() {
    super.initState();
    // Opening the tab loads the unread cutoff, clears the badge, reloads.
    WidgetsBinding.instance.addPostFrameCallback((_) => widget.controller.openFeed());
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: AnimatedBuilder(
          animation: widget.controller,
          builder: (context, _) {
            final controller = widget.controller;
            final items = controller.notifications;

            return RefreshIndicator(
              color: CustomerFigmaColors.primary,
              onRefresh: controller.loadFeed,
              child: ListView(
                physics: const AlwaysScrollableScrollPhysics(),
                padding: const EdgeInsets.fromLTRB(20, 16, 20, 24),
                children: [
                  const Text(
                    'Notifications',
                    style: TextStyle(
                      color: CustomerFigmaColors.text,
                      fontWeight: FontWeight.w800,
                      fontSize: 22,
                    ),
                  ),
                  const SizedBox(height: 4),
                  const Text(
                    'Manage your Notifications and Activities here.',
                    style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
                  ),
                  if (items.isEmpty)
                    Padding(
                      padding: EdgeInsets.only(top: MediaQuery.of(context).size.height * 0.16),
                      child: _EmptyState(loading: controller.isLoading, error: controller.error),
                    )
                  else
                    for (final entry in _groupByDay(items).entries) ...[
                      Padding(
                        padding: const EdgeInsets.only(top: 20, bottom: 10),
                        child: Text(
                          entry.key,
                          style: const TextStyle(
                            color: CustomerFigmaColors.text,
                            fontSize: 13,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                      ),
                      for (final item in entry.value)
                        Padding(
                          padding: const EdgeInsets.only(bottom: 10),
                          child: _NotificationCard(
                            item: item,
                            isUnread: controller.isUnread(item),
                          ),
                        ),
                    ],
                ],
              ),
            );
          },
        ),
      ),
    );
  }

  /// Groups notifications (already newest-first) into day buckets, preserving order.
  Map<String, List<AppNotification>> _groupByDay(List<AppNotification> items) {
    final groups = <String, List<AppNotification>>{};
    for (final item in items) {
      groups.putIfAbsent(_dayLabel(item.createdAt), () => []).add(item);
    }
    return groups;
  }

  String _dayLabel(DateTime time) {
    final now = DateTime.now();
    final today = DateTime(now.year, now.month, now.day);
    final day = DateTime(time.year, time.month, time.day);
    final diff = today.difference(day).inDays;
    if (diff <= 0) return 'Today';
    if (diff == 1) return 'Yesterday';
    if (diff < 7) return '$diff days ago';
    const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
    return '${day.day} ${months[day.month - 1]}${day.year == now.year ? '' : ' ${day.year}'}';
  }
}

class _NotificationCard extends StatelessWidget {
  const _NotificationCard({required this.item, required this.isUnread});

  final AppNotification item;
  final bool isUnread;

  @override
  Widget build(BuildContext context) {
    final message = item.body.isNotEmpty ? item.body : item.title;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(10),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.06),
            blurRadius: 12,
            offset: const Offset(0, 3),
          ),
        ],
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          Container(
            width: 12,
            height: 12,
            decoration: BoxDecoration(
              color: isUnread ? CustomerFigmaColors.primary : CustomerFigmaColors.primarySoft,
              shape: BoxShape.circle,
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              message.isEmpty ? 'Notification' : message,
              style: TextStyle(
                color: isUnread ? CustomerFigmaColors.text : CustomerFigmaColors.muted,
                fontSize: 13,
                fontWeight: isUnread ? FontWeight.w600 : FontWeight.w400,
                height: 1.4,
              ),
            ),
          ),
          const SizedBox(width: 12),
          Text(
            _formatTime(item.createdAt),
            style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 11),
          ),
        ],
      ),
    );
  }

  String _formatTime(DateTime time) {
    final hour12 = time.hour % 12 == 0 ? 12 : time.hour % 12;
    final minute = time.minute.toString().padLeft(2, '0');
    final period = time.hour < 12 ? 'AM' : 'PM';
    return '$hour12:$minute $period';
  }
}

class _EmptyState extends StatelessWidget {
  const _EmptyState({required this.loading, required this.error});

  final bool loading;
  final String? error;

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Container(
          width: 80,
          height: 80,
          decoration: const BoxDecoration(
            color: CustomerFigmaColors.primaryPale,
            shape: BoxShape.circle,
          ),
          child: const Icon(Icons.notifications_none_rounded, size: 38, color: CustomerFigmaColors.primary),
        ),
        const SizedBox(height: 20),
        Text(
          loading ? 'Loading…' : (error ?? 'No notifications'),
          style: const TextStyle(color: CustomerFigmaColors.text, fontSize: 17, fontWeight: FontWeight.w800),
        ),
        const SizedBox(height: 8),
        const Text(
          "We'll notify you of important updates here.",
          style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
        ),
      ],
    );
  }
}
