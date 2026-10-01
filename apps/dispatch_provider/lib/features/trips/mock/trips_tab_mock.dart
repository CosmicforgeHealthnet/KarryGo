import '../data/trip_model.dart';

/// Rich mock seed for the Activity tab. Covers every lifecycle state so the
/// filter tabs and status-specific detail screens can all be exercised.
class TripsTabMock {
  TripsTabMock._();

  static List<TripModel> seed() {
    final now = DateTime.now();
    return [
      // ── Active (in transit) ───────────────────────────────────────
      TripModel(
        id: 'trip-active-1',
        bookingId: 'PRK-2048731',
        statusCode: TripStatusCode.inProgress,
        rawStatus: 'in_progress',
        pickupAddress: 'Yaba, Lagos',
        dropoffAddress: 'Victoria Island, Lagos',
        customerName: 'Tunde Adebayo',
        customerPhone: '+234 802 555 1234',
        receiverName: 'Ngozi Chukwu',
        receiverPhone: '+234 802 555 6677',
        distanceKm: 4.8,
        estimatedFareKobo: 450000, // ₦4,500
        earningsKobo: 450000,
        createdAt: now.subtract(const Duration(minutes: 22)),
        startedAt: now.subtract(const Duration(minutes: 10)),
        itemSummary: 'Documents (Small, < 2kg)',
        pickupLat: 6.5090,
        pickupLng: 3.3710,
        dropoffLat: 6.4280,
        dropoffLng: 3.4210,
      ),

      // ── Completed ─────────────────────────────────────────────────
      TripModel(
        id: 'trip-completed-1',
        bookingId: 'PRK-2048658',
        statusCode: TripStatusCode.completed,
        rawStatus: 'completed',
        pickupAddress: 'Reja, Lagos',
        dropoffAddress: 'Lekki, Lagos',
        customerName: 'Tunde Adebayo',
        customerPhone: '+234 802 555 1234',
        receiverName: 'Tunde Adebayo',
        receiverPhone: '+234 802 555 1234',
        distanceKm: 4.8,
        estimatedFareKobo: 450000,
        earningsKobo: 450000,
        createdAt: now.subtract(const Duration(days: 1, hours: 3)),
        completedAt: now.subtract(const Duration(days: 1, hours: 2)),
        startedAt: now.subtract(const Duration(days: 1, hours: 3)),
        itemSummary: 'Documents (Small, < 2kg)',
        customerRating: 5.0,
        customerFeedback:
            'Great service and very professional! Picked up and delivered on time. Package was in perfect condition. Will definitely use again.',
        proofStatus: 'approved',
        proofPhotos: const ['photo_1.jpg', 'photo_2.jpg'],
        proofSignatureUrl: 'signature.png',
        refundStatus: null,
      ),
      TripModel(
        id: 'trip-completed-2',
        bookingId: 'PRK-2048651',
        statusCode: TripStatusCode.completed,
        rawStatus: 'completed',
        pickupAddress: 'Yaba, Lagos',
        dropoffAddress: 'Lekki, Lagos',
        customerName: 'Chiamaka Eze',
        customerPhone: '+234 803 111 2233',
        receiverName: 'Chiamaka Eze',
        receiverPhone: '+234 803 111 2233',
        distanceKm: 12.4,
        estimatedFareKobo: 550000,
        earningsKobo: 550000,
        createdAt: now.subtract(const Duration(days: 2, hours: 5)),
        completedAt: now.subtract(const Duration(days: 2, hours: 4)),
        itemSummary: 'Retail goods (5 packages)',
        customerRating: 4.0,
        customerFeedback: 'Smooth delivery, thanks.',
        proofStatus: 'approved',
        proofPhotos: const ['photo_1.jpg'],
      ),
      TripModel(
        id: 'trip-completed-3',
        bookingId: 'PRK-2048644',
        statusCode: TripStatusCode.completed,
        rawStatus: 'completed',
        pickupAddress: 'Festac, Lagos',
        dropoffAddress: 'Alaba, Lagos',
        customerName: 'Kunle Okafor',
        customerPhone: '+234 805 222 3344',
        receiverName: 'Kunle Okafor',
        receiverPhone: '+234 805 222 3344',
        distanceKm: 8.2,
        estimatedFareKobo: 420000,
        earningsKobo: 420000,
        createdAt: now.subtract(const Duration(days: 3, hours: 2)),
        completedAt: now.subtract(const Duration(days: 3, hours: 1)),
        itemSummary: 'Fragile (Small)',
        customerRating: 5.0,
        customerFeedback: 'Handled with care. Excellent.',
        proofStatus: 'approved',
      ),

      // ── Cancelled ─────────────────────────────────────────────────
      TripModel(
        id: 'trip-cancelled-1',
        bookingId: 'PRK-2048650',
        statusCode: TripStatusCode.cancelled,
        rawStatus: 'cancelled',
        pickupAddress: 'Surulere, Lagos',
        dropoffAddress: 'Abuja, FCT',
        customerName: 'Emeka Obi',
        customerPhone: '+234 809 333 4455',
        receiverName: 'Emeka Obi',
        receiverPhone: '+234 809 333 4455',
        distanceKm: 15.0,
        estimatedFareKobo: 0,
        earningsKobo: 0,
        createdAt: now.subtract(const Duration(days: 4, hours: 6)),
        cancelledAt: now.subtract(const Duration(days: 4, hours: 5)),
        itemSummary: 'Medical supplies',
        cancelReason: 'Customer requested to cancel before pickup.',
        refundStatus: 'N/A',
      ),

      // ── Failed ────────────────────────────────────────────────────
      TripModel(
        id: 'trip-failed-1',
        bookingId: 'PRK-2048612',
        statusCode: TripStatusCode.failed,
        rawStatus: 'failed',
        pickupAddress: 'Ajah, Lagos',
        dropoffAddress: 'Royal, Lagos',
        customerName: 'Fatima Bello',
        customerPhone: '+234 811 444 5566',
        receiverName: 'Fatima Bello',
        receiverPhone: '+234 811 444 5566',
        distanceKm: 6.5,
        estimatedFareKobo: 0,
        earningsKobo: 0,
        createdAt: now.subtract(const Duration(days: 5, hours: 8)),
        failedAt: now.subtract(const Duration(days: 5, hours: 7)),
        itemSummary: 'Retail package',
        failReason: 'Recipient unavailable.',
        proofPhotos: const ['attempt_1.jpg'],
      ),
    ];
  }

  static List<TripModel> all() => seed();
  static List<TripModel> active() =>
      seed().where((t) => t.statusCode.isActive).toList();
  static List<TripModel> completed() =>
      seed().where((t) => t.statusCode.isCompleted).toList();
  static List<TripModel> cancelled() =>
      seed().where((t) => t.statusCode.isCancelled).toList();
  static List<TripModel> failed() =>
      seed().where((t) => t.statusCode.isFailed).toList();
}