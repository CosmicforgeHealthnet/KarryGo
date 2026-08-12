import 'package:flutter/material.dart';

import '../models/rider_notification.dart';
import '../state/rider_notification_controller.dart';

const _kGreen = Color(0xFF05124D);
const _kGreenPale = Color(0xFFE6E8EE);
const _kText = Color(0xFF1A1A1A);
const _kMuted = Color(0xFF8A8A8A);

/// Notifications screen backed by the real notification-service feed (via the
/// dispatch-service proxy). Mirrors the truck provider app's Figma-2119 layout.
class NotificationsScreen extends StatefulWidget {
  const NotificationsScreen({super.key, required this.controller});

  final RiderNotificationController controller;

  @override
  State<NotificationsScreen> createState() => _NotificationsScreenState();
}

class _NotificationsScreenState extends State<NotificationsScreen> {
  @override
  void initState() {
    super.initState();
    widget.controller.openFeed();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      appBar: AppBar(
        backgroundColor: Colors.white,
        elevation: 0,
        leading: Padding(
          padding: const EdgeInsets.only(left: 12),
          child: GestureDetector(
            onTap: () => Navigator.of(context).pop(),
            child: Container(
              width: 40,
              height: 40,
              decoration: const BoxDecoration(
                color: Color(0xFFF7F8F7),
                shape: BoxShape.circle,
              ),
              child: const Icon(
                Icons.arrow_back_rounded,
                color: _kText,
                size: 20,
              ),
            ),
          ),
        ),
        title: const Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Notifications',
              style: TextStyle(
                color: _kText,
                fontWeight: FontWeight.w800,
                fontSize: 18,
              ),
            ),
            Text(
              'Manage your Notifications and Activities here.',
              style: TextStyle(color: _kMuted, fontSize: 11),
            ),
          ],
        ),
        titleSpacing: 0,
      ),
      body: AnimatedBuilder(
        animation: widget.controller,
        builder: (context, _) {
          final controller = widget.controller;
          final items = controller.notifications;

          if (controller.isLoading && items.isEmpty) {
            return const Center(
              child: CircularProgressIndicator(color: _kGreen),
            );
          }
          if (controller.error != null && items.isEmpty) {
            return _FeedMessage(
              icon: Icons.cloud_off_rounded,
              message: controller.error!,
              actionLabel: 'Try again',
              onAction: controller.refresh,
            );
          }
          if (items.isEmpty) {
            return _FeedMessage(
              icon: Icons.notifications_none_rounded,
              message:
                  'No notifications yet.\nDelivery updates will show up here.',
              actionLabel: 'Refresh',
              onAction: controller.refresh,
            );
          }

          final groups = _groupByDay(items);
          return RefreshIndicator(
            color: _kGreen,
            onRefresh: controller.refresh,
            child: ListView(
              physics: const AlwaysScrollableScrollPhysics(),
              padding: const EdgeInsets.symmetric(vertical: 8),
              children: [
                for (final entry in groups.entries) ...[
                  Padding(
                    padding: const EdgeInsets.fromLTRB(20, 16, 20, 6),
                    child: Text(
                      entry.key,
                      style: const TextStyle(
                        color: _kMuted,
                        fontSize: 12,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ),
                  for (final item in entry.value)
                    _NotificationRow(
                      item: item,
                      isUnread: controller.isUnread(item),
                    ),
                ],
              ],
            ),
          );
        },
      ),
    );
  }

  /// Groups notifications (already newest-first) into day buckets, preserving order.
  Map<String, List<RiderNotification>> _groupByDay(
    List<RiderNotification> items,
  ) {
    final groups = <String, List<RiderNotification>>{};
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
    return '${day.day} ${months[day.month - 1]}${day.year == now.year ? '' : ' ${day.year}'}';
  }
}

class _NotificationRow extends StatelessWidget {
  const _NotificationRow({required this.item, required this.isUnread});

  final RiderNotification item;
  final bool isUnread;

  @override
  Widget build(BuildContext context) {
    final message = item.body.isNotEmpty ? item.body : item.title;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 14),
      decoration: const BoxDecoration(
        border: Border(bottom: BorderSide(color: Color(0xFFF0F0F0))),
      ),
      child: Row(
        children: [
          Container(
            width: 12,
            height: 12,
            decoration: BoxDecoration(
              color: isUnread ? _kGreen : _kGreenPale,
              shape: BoxShape.circle,
            ),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Text(
              message,
              style: TextStyle(
                color: isUnread ? _kText : _kMuted,
                fontSize: 13,
                fontWeight: isUnread ? FontWeight.w600 : FontWeight.w400,
                height: 1.4,
              ),
            ),
          ),
          const SizedBox(width: 12),
          Text(
            _formatTime(item.createdAt),
            style: const TextStyle(color: _kMuted, fontSize: 11),
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

class _FeedMessage extends StatelessWidget {
  const _FeedMessage({
    required this.icon,
    required this.message,
    required this.actionLabel,
    required this.onAction,
  });

  final IconData icon;
  final String message;
  final String actionLabel;
  final Future<void> Function() onAction;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 48, color: _kMuted),
          const SizedBox(height: 12),
          Text(
            message,
            textAlign: TextAlign.center,
            style: const TextStyle(color: _kMuted, fontSize: 13, height: 1.5),
          ),
          const SizedBox(height: 16),
          TextButton(
            onPressed: onAction,
            child: Text(
              actionLabel,
              style: const TextStyle(
                color: _kGreen,
                fontWeight: FontWeight.w700,
              ),
            ),
          ),
        ],
      ),
    );
  }
}
