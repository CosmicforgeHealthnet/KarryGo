import '../models/rider_notification.dart';

/// Stateful mock for the notifications feed. Two seed methods:
/// - [seed] — 6 notifications matching Figma 01 row types
/// - [seedForDetail] — one payload with full detail data (Figma 02)
class NotificationsMock {
  NotificationsMock._();

  static List<RiderNotification> seed() {
    final now = DateTime.now();
    return [
      RiderNotification(
        id: 'n-1',
        eventType: 'assignment_offer',
        title: 'New assignment offer',
        body: 'Victoria Island → Lekki · 12.6 km · ₦4,500',
        createdAt: now.subtract(const Duration(minutes: 5)),
        data: const {
          'pickup': 'Victoria Island, Lagos',
          'dropoff': 'Lekki, Lagos',
          'distance_km': 12.6,
          'earnings_kobo': 450000,
          'package': 'Documents (Small < 2kg)',
          'assignment_id': 'as-2048731',
        },
      ),
      RiderNotification(
        id: 'n-2',
        eventType: 'pickup_confirmed',
        title: 'Pickup confirmed',
        body: 'Customer is ready at 27 Admiralty Way, Lekki',
        createdAt: now.subtract(const Duration(minutes: 90)),
        data: const {
          'address': '27 Admiralty Way, Lekki',
        },
      ),
      RiderNotification(
        id: 'n-3',
        eventType: 'delivery_completed',
        title: 'Delivery completed',
        body: 'You earned ₦4,500 · Payment is being processed',
        createdAt: now.subtract(const Duration(hours: 2)),
        data: const {
          'earnings_kobo': 450000,
        },
      ),
      RiderNotification(
        id: 'n-4',
        eventType: 'payout_notice',
        title: 'Payout notice',
        body: '₦110,000 has been credited to your wallet',
        createdAt: now.subtract(const Duration(hours: 28)),
        data: const {
          'amount_kobo': 11000000,
        },
      ),
      RiderNotification(
        id: 'n-5',
        eventType: 'support_message',
        title: 'Support message',
        body: "We've replied to your ticket #SUP-4821",
        createdAt: now.subtract(const Duration(hours: 30)),
        data: const {
          'ticket_id': 'SUP-4821',
        },
      ),
      RiderNotification(
        id: 'n-6',
        eventType: 'system_update',
        title: 'System update',
        body: 'New features are now available in the Karryam app.',
        createdAt: now.subtract(const Duration(days: 2)),
        data: const {},
      ),
    ];
  }

  static void reset() {}
}