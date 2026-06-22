import 'package:flutter/material.dart';

import '../widgets/provider_app_colors.dart';

/// Notifications screen (Figma 2119).
class ProviderNotificationsScreen extends StatelessWidget {
  const ProviderNotificationsScreen({super.key});

  static const _items = [
    _NotifItem(
      group: 'Today',
      message: 'Your package has arrived!',
      time: '12:05 PM',
      isRead: false,
    ),
    _NotifItem(
      group: 'Today',
      message: 'You have uploaded a profile picture.',
      time: '12:05 PM',
      isRead: false,
    ),
    _NotifItem(
      group: 'Yesterday',
      message: 'Your Ride has arrived!',
      time: '12:05 PM',
      isRead: true,
    ),
    _NotifItem(
      group: 'Yesterday',
      message: "A driver confirmed your booking, he's on his way.",
      time: '12:05 PM',
      isRead: true,
    ),
    _NotifItem(
      group: '3 days ago',
      message: 'Your package has been picked.',
      time: '12:05 PM',
      isRead: true,
    ),
    _NotifItem(
      group: '3 days ago',
      message: 'Driver Canceled the order!',
      time: '12:05 PM',
      isRead: true,
    ),
    _NotifItem(
      group: '4 days Ago',
      message: 'Truck booking confirmed! Driver on his way.',
      time: '12:05 PM',
      isRead: false,
    ),
  ];

  @override
  Widget build(BuildContext context) {
    final groups = <String, List<_NotifItem>>{};
    for (final item in _items) {
      groups.putIfAbsent(item.group, () => []).add(item);
    }

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
              decoration: const BoxDecoration(color: Color(0xFFF7F8F7), shape: BoxShape.circle),
              child: const Icon(Icons.arrow_back_rounded, color: kProviderText, size: 20),
            ),
          ),
        ),
        title: const Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Notifications',
              style: TextStyle(color: kProviderText, fontWeight: FontWeight.w800, fontSize: 18),
            ),
            Text(
              'Manage your Notifications and Activities here.',
              style: TextStyle(color: kProviderMuted, fontSize: 11),
            ),
          ],
        ),
        titleSpacing: 0,
      ),
      body: ListView(
        padding: const EdgeInsets.symmetric(vertical: 8),
        children: [
          for (final entry in groups.entries) ...[
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 16, 20, 6),
              child: Text(
                entry.key,
                style: const TextStyle(
                  color: kProviderMuted,
                  fontSize: 12,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ),
            for (final item in entry.value)
              Container(
                margin: const EdgeInsets.symmetric(horizontal: 0, vertical: 0),
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
                        color: item.isRead ? kProviderGreenPale : kProviderGreen,
                        shape: BoxShape.circle,
                      ),
                    ),
                    const SizedBox(width: 14),
                    Expanded(
                      child: Text(
                        item.message,
                        style: TextStyle(
                          color: item.isRead ? kProviderMuted : kProviderText,
                          fontSize: 13,
                          fontWeight: item.isRead ? FontWeight.w400 : FontWeight.w600,
                          height: 1.4,
                        ),
                      ),
                    ),
                    const SizedBox(width: 12),
                    Text(
                      item.time,
                      style: const TextStyle(color: kProviderMuted, fontSize: 11),
                    ),
                  ],
                ),
              ),
          ],
        ],
      ),
    );
  }
}

class _NotifItem {
  const _NotifItem({
    required this.group,
    required this.message,
    required this.time,
    required this.isRead,
  });

  final String group;
  final String message;
  final String time;
  final bool isRead;
}
