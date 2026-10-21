import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../../../core/data/driver_client.dart';
import '../../../core/theme/karryam_theme.dart';
import '../../../core/widgets/karryam_ui.dart';

/// One past alert (taxi `GET /provider/notifications`).
class DriverNotification {
  const DriverNotification({
    required this.title,
    required this.body,
    this.createdAt,
  });
  final String title;
  final String body;
  final DateTime? createdAt;

  /// notification-service sends Go field names (Title, Body, CreatedAt);
  /// snake_case is accepted too, as in the truck and customer apps.
  factory DriverNotification.fromJson(Map<String, dynamic> j) {
    String pick(List<String> keys) {
      for (final k in keys) {
        final v = j[k];
        if (v is String && v.isNotEmpty) return v;
      }
      return '';
    }

    return DriverNotification(
      title: pick(['Title', 'title']),
      body: pick(['Body', 'body']),
      createdAt: DateTime.tryParse(
        pick(['CreatedAt', 'created_at']),
      )?.toLocal(),
    );
  }
}

/// The driver's notification history: ride offers, payouts and the rest.
class NotificationsScreen extends StatefulWidget {
  const NotificationsScreen({super.key, required this.taxi});
  final DriverClient taxi;

  @override
  State<NotificationsScreen> createState() => _NotificationsScreenState();
}

class _NotificationsScreenState extends State<NotificationsScreen> {
  List<DriverNotification>? _items;
  String? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() => _error = null);
    try {
      final data = await widget.taxi.get(
        '/provider/notifications',
        query: {'limit': '50'},
      );
      final items = asList(data)
          .map(DriverNotification.fromJson)
          .where((n) => n.title.isNotEmpty || n.body.isNotEmpty)
          .toList();
      if (mounted) setState(() => _items = items);
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    final items = _items;
    return KarryamPage(
      title: 'Notifications',
      body: _error != null && items == null
          ? KarryamPlaceholder.error(message: _error!, onRetry: _load)
          : items == null
          ? const KarryamPlaceholder.loading()
          : RefreshIndicator(
              onRefresh: _load,
              child: items.isEmpty
                  ? ListView(
                      children: const [
                        KarryamPlaceholder.empty(
                          icon: Icons.notifications_none_rounded,
                          message: 'No notifications yet.',
                        ),
                      ],
                    )
                  : ListView.separated(
                      padding: const EdgeInsets.all(20),
                      itemCount: items.length,
                      separatorBuilder: (_, _) => const SizedBox(height: 10),
                      itemBuilder: (context, i) {
                        final n = items[i];
                        return KarryamPanel(
                          child: Row(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Icon(
                                Icons.notifications_rounded,
                                color: colors.primary,
                                size: 20,
                              ),
                              const SizedBox(width: 12),
                              Expanded(
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    if (n.title.isNotEmpty)
                                      Text(
                                        n.title,
                                        style: const TextStyle(
                                          fontWeight: FontWeight.w600,
                                        ),
                                      ),
                                    if (n.body.isNotEmpty) Text(n.body),
                                    const SizedBox(height: 4),
                                    Text(
                                      formatDateTime(n.createdAt),
                                      style: TextStyle(
                                        color: colors.textMuted,
                                        fontSize: 12,
                                      ),
                                    ),
                                  ],
                                ),
                              ),
                            ],
                          ),
                        );
                      },
                    ),
            ),
    );
  }
}
