import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../../core/network/provider_session.dart';
import '../models/rider_notification.dart';
import 'notifications_screen.dart' show notificationIcon, formatNotifTime;

class NotificationDetailScreen extends StatelessWidget {
  const NotificationDetailScreen({super.key, required this.notification});

  final RiderNotification notification;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final (icon, tint) = notificationIcon(notification.eventType, c);
    final data = notification.data;

    final pickup = data['pickup'] as String?;
    final dropoff = data['dropoff'] as String?;
    final distance = data['distance_km'];
    final earnings = data['earnings_kobo'];
    final package = data['package'] as String?;

    final isAssignment =
        notification.eventType.toLowerCase().contains('assignment') ||
        notification.eventType.toLowerCase().contains('opportunity');

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            // ── Header ──
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
              child: Row(
                children: [
                  IconButton(
                    tooltip: 'Back',
                    onPressed: () => Navigator.of(context).pop(),
                    icon: const Icon(Icons.arrow_back_rounded),
                  ),
                  const SizedBox(width: 4),
                  Text(
                    'Notification',
                    style: TextStyle(
                      color: c.text,
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ],
              ),
            ),

            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                children: [
                  // ── Type header ──
                  Row(
                    children: [
                      Container(
                        width: 40,
                        height: 40,
                        alignment: Alignment.center,
                        decoration: BoxDecoration(
                          color: tint.withValues(alpha: 0.12),
                          borderRadius: BorderRadius.circular(12),
                        ),
                        child: Icon(icon, size: 22, color: tint),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              _titleCase(notification.eventType),
                              style: TextStyle(
                                fontSize: 14,
                                fontWeight: FontWeight.w700,
                                color: c.text,
                              ),
                            ),
                            const SizedBox(height: 2),
                            Text(
                              formatNotifTime(notification.createdAt),
                              style: TextStyle(
                                fontSize: 12,
                                fontWeight: FontWeight.w500,
                                color: c.textMuted,
                              ),
                            ),
                          ],
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 28),

                  // ── Big title ──
                  Text(
                    notification.title.isNotEmpty
                        ? notification.title
                        : notification.body,
                    style: TextStyle(
                      fontSize: 24,
                      fontWeight: FontWeight.w800,
                      color: c.text,
                      letterSpacing: -0.4,
                      height: 1.25,
                    ),
                  ),

                  if (pickup != null && dropoff != null) ...[
                    const SizedBox(height: 20),
                    _RouteRow(pickup: pickup, dropoff: dropoff),
                  ],

                  const SizedBox(height: 24),

                  // ── KV card ──
                  if (distance != null ||
                      earnings != null ||
                      package != null) ...[
                    Container(
                      padding: const EdgeInsets.all(16),
                      decoration: BoxDecoration(
                        color: c.surface,
                        borderRadius: BorderRadius.circular(14),
                        border: Border.all(color: c.divider),
                      ),
                      child: Column(
                        children: [
                          if (distance != null)
                            _KV(
                              icon: Icons.straighten_rounded,
                              label: 'Distance',
                              value: '${distance}km',
                            ),
                          if (distance != null &&
                              (earnings != null || package != null))
                            const SizedBox(height: 14),
                          if (earnings != null)
                            _KV(
                              icon: Icons.payments_outlined,
                              label: 'Estimated earnings',
                              value: _naira(earnings),
                            ),
                          if (earnings != null && package != null)
                            const SizedBox(height: 14),
                          if (package != null)
                            _KV(
                              icon: Icons.inventory_2_outlined,
                              label: 'Package',
                              value: package,
                            ),
                        ],
                      ),
                    ),
                    const SizedBox(height: 20),
                  ],

                  if (notification.body.isNotEmpty &&
                      notification.title.isNotEmpty &&
                      pickup == null) ...[
                    Text(
                      notification.body,
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w500,
                        color: c.textMuted,
                        height: 1.5,
                      ),
                    ),
                    const SizedBox(height: 20),
                  ],

                  // ── Info chip ──
                  if (isAssignment)
                    Container(
                      padding: const EdgeInsets.all(14),
                      decoration: BoxDecoration(
                        color: c.info.withValues(alpha: 0.10),
                        borderRadius: BorderRadius.circular(12),
                      ),
                      child: Row(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Icon(
                            Icons.info_outline_rounded,
                            size: 18,
                            color: c.info,
                          ),
                          const SizedBox(width: 10),
                          Expanded(
                            child: Text(
                              'Be one of the first to accept. This opportunity is available for a limited time.',
                              style: TextStyle(
                                fontSize: 13,
                                fontWeight: FontWeight.w500,
                                color: c.info,
                                height: 1.45,
                              ),
                            ),
                          ),
                        ],
                      ),
                    ),
                ],
              ),
            ),

            // ── CTAs ──
            if (isAssignment)
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 8),
                child: SizedBox(
                  height: 52,
                  width: double.infinity,
                  child: FilledButton(
                    onPressed: () {
                      ProviderSession.of(context).openOpportunities?.call();
                    },
                    style: FilledButton.styleFrom(
                      textStyle: const TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    child: const Text('View opportunities'),
                  ),
                ),
              ),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: OutlinedButton(
                  onPressed: () => Navigator.of(context).pop(),
                  style: OutlinedButton.styleFrom(
                    foregroundColor: c.text,
                    side: BorderSide(color: c.border, width: 1.2),
                    shape: const StadiumBorder(),
                    textStyle: const TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('Back to notifications'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  static String _titleCase(String s) {
    if (s.isEmpty) return s;
    return s
        .split('_')
        .map((w) => w.isEmpty ? w : '${w[0].toUpperCase()}${w.substring(1)}')
        .join(' ');
  }

  static String _naira(dynamic kobo) {
    final n = (kobo is num ? kobo.toInt() : 0) ~/ 100;
    final s = n.toString();
    final buf = StringBuffer();
    for (var i = 0; i < s.length; i++) {
      if (i > 0 && (s.length - i) % 3 == 0) buf.write(',');
      buf.write(s[i]);
    }
    return '₦${buf.toString()}';
  }
}

class _RouteRow extends StatelessWidget {
  const _RouteRow({required this.pickup, required this.dropoff});
  final String pickup;
  final String dropoff;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(Icons.location_on_outlined, size: 20, color: c.success),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                pickup,
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w700,
                  color: c.text,
                ),
              ),
            ),
          ],
        ),
        Padding(
          padding: const EdgeInsets.only(left: 9, top: 4, bottom: 4),
          child: Container(width: 2, height: 16, color: c.divider),
        ),
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(Icons.location_on_outlined, size: 20, color: c.textMuted),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                dropoff,
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w700,
                  color: c.text,
                ),
              ),
            ),
          ],
        ),
      ],
    );
  }
}

class _KV extends StatelessWidget {
  const _KV({required this.icon, required this.label, required this.value});
  final IconData icon;
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Row(
      children: [
        Icon(icon, size: 20, color: c.textMuted),
        const SizedBox(width: 12),
        Expanded(
          child: Text(
            label,
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w500,
              color: c.textMuted,
            ),
          ),
        ),
        Text(
          value,
          style: TextStyle(
            fontSize: 15,
            fontWeight: FontWeight.w800,
            color: c.text,
            letterSpacing: -0.2,
          ),
        ),
      ],
    );
  }
}
