import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../../notifications/models/app_notification.dart';
import '../../../notifications/state/notification_controller.dart';

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
    // Opening the tab marks the live-push badge as seen.
    WidgetsBinding.instance.addPostFrameCallback((_) => widget.controller.markAllRead());
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      appBar: AppBar(
        backgroundColor: Colors.white,
        elevation: 0,
        centerTitle: false,
        title: const Text(
          'Notifications',
          style: TextStyle(
            color: CustomerFigmaColors.text,
            fontWeight: FontWeight.w800,
            fontSize: 18,
          ),
        ),
      ),
      body: AnimatedBuilder(
        animation: widget.controller,
        builder: (context, _) {
          final controller = widget.controller;
          final items = controller.notifications;

          if (items.isEmpty) {
            return RefreshIndicator(
              onRefresh: controller.loadFeed,
              child: ListView(
                children: [
                  SizedBox(height: MediaQuery.of(context).size.height * 0.18),
                  _EmptyState(loading: controller.isLoading, error: controller.error),
                ],
              ),
            );
          }

          return RefreshIndicator(
            onRefresh: controller.loadFeed,
            child: ListView.separated(
              padding: const EdgeInsets.symmetric(vertical: 8),
              itemCount: items.length,
              separatorBuilder: (_, _) => const Divider(height: 1, indent: 72),
              itemBuilder: (context, index) => _NotificationTile(item: items[index]),
            ),
          );
        },
      ),
    );
  }
}

class _NotificationTile extends StatelessWidget {
  const _NotificationTile({required this.item});

  final AppNotification item;

  @override
  Widget build(BuildContext context) {
    return ListTile(
      contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 6),
      leading: Container(
        width: 44,
        height: 44,
        decoration: const BoxDecoration(
          color: CustomerFigmaColors.primaryPale,
          shape: BoxShape.circle,
        ),
        child: Icon(_iconFor(item.eventType), color: CustomerFigmaColors.primary, size: 22),
      ),
      title: Text(
        item.title.isEmpty ? 'Notification' : item.title,
        style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 15, color: CustomerFigmaColors.text),
      ),
      subtitle: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (item.body.isNotEmpty) ...[
            const SizedBox(height: 2),
            Text(item.body, style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 13)),
          ],
          const SizedBox(height: 4),
          Text(_relativeTime(item.createdAt),
              style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 11)),
        ],
      ),
      isThreeLine: item.body.isNotEmpty,
    );
  }

  IconData _iconFor(String eventType) {
    if (eventType.startsWith('payment') || eventType.startsWith('withdrawal') || eventType.startsWith('refund')) {
      return Icons.account_balance_wallet_rounded;
    }
    if (eventType.startsWith('cargo') || eventType.startsWith('booking')) {
      return Icons.local_shipping_rounded;
    }
    return Icons.notifications_rounded;
  }

  String _relativeTime(DateTime time) {
    final diff = DateTime.now().difference(time);
    if (diff.inMinutes < 1) return 'Just now';
    if (diff.inMinutes < 60) return '${diff.inMinutes}m ago';
    if (diff.inHours < 24) return '${diff.inHours}h ago';
    if (diff.inDays < 7) return '${diff.inDays}d ago';
    return '${time.day}/${time.month}/${time.year}';
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
