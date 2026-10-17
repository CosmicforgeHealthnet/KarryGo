import '../../../shared/widgets/dispatch_empty_state.dart';
import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/async_content.dart';
import '../models/rider_notification.dart';
import '../state/rider_notification_controller.dart';
import 'notification_detail_screen.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 01 — Notifications feed
// Header · All/Unread/Archived tabs · icon-led rows with time + unread dot
// ═══════════════════════════════════════════════════════════════════════════

class NotificationsScreen extends StatefulWidget {
  const NotificationsScreen({super.key, required this.controller});

  final RiderNotificationController controller;

  @override
  State<NotificationsScreen> createState() => _NotificationsScreenState();
}

enum _FeedTab { all, unread }

class _NotificationsScreenState extends State<NotificationsScreen> {
  _FeedTab _tab = _FeedTab.all;

  @override
  void initState() {
    super.initState();
    widget.controller.openFeed();
  }

  List<RiderNotification> _filter(List<RiderNotification> all) {
    switch (_tab) {
      case _FeedTab.all:
        return all;
      case _FeedTab.unread:
        return all.where(widget.controller.isUnread).toList();
    }
  }

  Future<void> _openDetail(RiderNotification item) async {
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => NotificationDetailScreen(notification: item),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        bottom: false,
        child: Column(
          children: [
            // ── Header ──
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
              child: Row(
                children: [
                  GestureDetector(
                    onTap: () => Navigator.of(context).pop(),
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Icon(
                        Icons.arrow_back_rounded,
                        size: 22,
                        color: c.text,
                      ),
                    ),
                  ),
                  const SizedBox(width: 4),
                  Expanded(
                    child: Text(
                      'Notifications',
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                        color: c.text,
                        letterSpacing: -0.2,
                      ),
                    ),
                  ),
                ],
              ),
            ),

            // ── Tabs ──
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 12),
              child: Row(
                children: [
                  _TabPill(
                    label: 'All',
                    active: _tab == _FeedTab.all,
                    onTap: () => setState(() => _tab = _FeedTab.all),
                  ),
                  const SizedBox(width: 8),
                  _TabPill(
                    label: 'Unread',
                    active: _tab == _FeedTab.unread,
                    onTap: () => setState(() => _tab = _FeedTab.unread),
                  ),
                ],
              ),
            ),

            // ── Feed ──
            Expanded(
              child: AnimatedBuilder(
                animation: widget.controller,
                builder: (context, _) {
                  final items = _filter(widget.controller.notifications);

                  if (widget.controller.isLoading &&
                      widget.controller.notifications.isEmpty) {
                    return const ContentSkeleton();
                  }
                  if (widget.controller.error != null &&
                      widget.controller.notifications.isEmpty) {
                    return _Message(
                      icon: Icons.cloud_off_rounded,
                      text: widget.controller.error!,
                      actionLabel: 'Try again',
                      onAction: widget.controller.refresh,
                    );
                  }
                  if (items.isEmpty) {
                    return _Message(
                      icon: Icons.notifications_none_rounded,
                      text: _tab == _FeedTab.unread
                          ? "You're all caught up."
                          : 'No notifications yet.',
                      actionLabel: 'Refresh',
                      onAction: widget.controller.refresh,
                    );
                  }

                  return RefreshIndicator(
                    color: c.brandTeal500,
                    onRefresh: widget.controller.refresh,
                    child: ListView.separated(
                      physics: const AlwaysScrollableScrollPhysics(),
                      padding: const EdgeInsets.fromLTRB(20, 4, 20, 24),
                      itemCount: items.length,
                      separatorBuilder: (_, __) =>
                          Divider(height: 1, color: c.divider),
                      itemBuilder: (context, i) {
                        final item = items[i];
                        return _NotificationRow(
                          item: item,
                          unread: widget.controller.isUnread(item),
                          onTap: () => _openDetail(item),
                        );
                      },
                    ),
                  );
                },
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Tab pill
// ═══════════════════════════════════════════════════════════════════════════

class _TabPill extends StatelessWidget {
  const _TabPill({
    required this.label,
    required this.active,
    required this.onTap,
  });

  final String label;
  final bool active;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
        decoration: BoxDecoration(
          color: active ? c.brandNavy900 : c.navy10,
          borderRadius: BorderRadius.circular(999),
        ),
        child: Text(
          label,
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w700,
            color: active ? c.brandWhite : c.textMuted,
          ),
        ),
      ),
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Row
// ═══════════════════════════════════════════════════════════════════════════

class _NotificationRow extends StatelessWidget {
  const _NotificationRow({
    required this.item,
    required this.unread,
    required this.onTap,
  });

  final RiderNotification item;
  final bool unread;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final (icon, tint) = notificationIcon(item.eventType, c);

    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 14),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // ── Icon ──
            Container(
              width: 42,
              height: 42,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: tint.withValues(alpha: 0.12),
                borderRadius: BorderRadius.circular(12),
              ),
              child: Icon(icon, size: 22, color: tint),
            ),
            const SizedBox(width: 14),

            // ── Text ──
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Expanded(
                        child: Text(
                          item.title.isNotEmpty ? item.title : item.body,
                          style: TextStyle(
                            fontSize: 14,
                            fontWeight: unread
                                ? FontWeight.w700
                                : FontWeight.w600,
                            color: c.text,
                            letterSpacing: -0.1,
                          ),
                        ),
                      ),
                      const SizedBox(width: 8),
                      Text(
                        formatNotifTime(item.createdAt),
                        style: TextStyle(
                          fontSize: 11.5,
                          fontWeight: FontWeight.w500,
                          color: c.textMuted,
                        ),
                      ),
                      if (unread) ...[
                        const SizedBox(width: 6),
                        Container(
                          width: 7,
                          height: 7,
                          decoration: BoxDecoration(
                            color: c.danger,
                            shape: BoxShape.circle,
                          ),
                        ),
                      ],
                    ],
                  ),
                  if (item.body.isNotEmpty && item.title.isNotEmpty) ...[
                    const SizedBox(height: 4),
                    Text(
                      item.body,
                      style: TextStyle(
                        fontSize: 12.5,
                        fontWeight: FontWeight.w500,
                        color: c.textMuted,
                        height: 1.35,
                      ),
                    ),
                  ],
                ],
              ),
            ),

            const SizedBox(width: 8),
            Icon(Icons.chevron_right_rounded, size: 20, color: c.textMuted),
          ],
        ),
      ),
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Helpers — icon mapping per event type (matches Figma 01)
// ═══════════════════════════════════════════════════════════════════════════

(IconData, Color) notificationIcon(String eventType, dynamic c) {
  final t = eventType.toLowerCase();
  if (t.contains('assignment') || t.contains('opportunity')) {
    return (Icons.inventory_2_outlined, const Color(0xFFF57C00));
  }
  if (t.contains('pickup')) {
    return (Icons.location_on_outlined, c.info);
  }
  if (t.contains('delivery_completed') || t.contains('completed')) {
    return (Icons.check_circle_outlined, c.success);
  }
  if (t.contains('payout') || t.contains('payment')) {
    return (Icons.account_balance_outlined, c.info);
  }
  if (t.contains('support') || t.contains('message')) {
    return (Icons.chat_bubble_outline, const Color(0xFF8B5CF6));
  }
  if (t.contains('system') || t.contains('update')) {
    return (Icons.campaign_outlined, c.brandNavy900);
  }
  return (Icons.notifications_outlined, c.textMuted);
}

String formatNotifTime(DateTime t) {
  final now = DateTime.now();
  final today = DateTime(now.year, now.month, now.day);
  final day = DateTime(t.year, t.month, t.day);
  final diff = today.difference(day).inDays;

  if (diff == 0) {
    final h12 = t.hour % 12 == 0 ? 12 : t.hour % 12;
    final m = t.minute.toString().padLeft(2, '0');
    final p = t.hour < 12 ? 'AM' : 'PM';
    return '$h12:$m $p';
  }
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
  return '${day.day} ${months[day.month - 1]}';
}

class _Message extends StatelessWidget {
  const _Message({
    required this.icon,
    required this.text,
    required this.actionLabel,
    required this.onAction,
  });

  final IconData icon;
  final String text;
  final String actionLabel;
  final Future<void> Function() onAction;

  @override
  Widget build(BuildContext context) {
    if (icon == Icons.cloud_off_rounded)
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: ActionNotice(message: text, isError: true, onRetry: onAction),
        ),
      );
    return DispatchEmptyState(
      story: EmptyStory.support,
      title: text,
      message: 'Updates about your account and deliveries appear here.',
      actionLabel: actionLabel,
      onAction: onAction,
    );
  }
}
