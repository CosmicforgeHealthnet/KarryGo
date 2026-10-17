import '../../requests/data/request_model.dart';
import '../../requests/mock/requests_tab_mock.dart';
import '../../trips/data/trip_model.dart';

/// Shared mock state for the Dispatch Provider app.
class DispatchHomeMock {
  DispatchHomeMock._();

  // ── Header display values ────────────────────────────────────────────
  static const String todayEarnings = 'N24,350';
  static const String acceptanceRate = '92%';

  // ── Trip peek-bar values ─────────────────────────────────────────────
  static const int pickupEtaMinutes = 8;
  static const int deliveryEtaMinutes = 12;

  // ── Shared mutable state ─────────────────────────────────────────────
  static bool isOnline = false;
  static TripModel? activeTrip;

  /// Pending requests come from the richer Opportunities mock so the Home
  /// incoming sheet and the Opportunities tab show the same data.
  static List<RequestModel> pendingRequests = _seedRequests();

  static void reset() {
    isOnline = false;
    activeTrip = null;
    pendingRequests = _seedRequests();
  }

  static void seedPendingRequests() {
    pendingRequests = _seedRequests();
  }

  static void clearPendingRequests() {
    pendingRequests = [];
  }

  // ── Seed requests ────────────────────────────────────────────────────
  static List<RequestModel> _seedRequests() => RequestsTabMock.seed();

  // ── Trip helpers ─────────────────────────────────────────────────────

  static TripModel tripFromRequest(RequestModel req) => TripModel(
    id: 'mock-trip-${req.id}',
    bookingId: req.bookingId,
    statusCode: TripStatusCode.assigned,
    rawStatus: 'assigned',
    pickupAddress: req.pickupAddress,
    dropoffAddress: req.dropoffAddress,
    customerName: req.customerName,
    customerPhone: req.receiverPhone,
    receiverName: req.receiverName,
    receiverPhone: req.receiverPhone,
    distanceKm: req.distanceKm,
    estimatedFareKobo: req.estimatedFareKobo,
    createdAt: DateTime.now(),
    notes: req.notes,
    pickupLat: req.pickupLat,
    pickupLng: req.pickupLng,
    dropoffLat: req.dropoffLat,
    dropoffLng: req.dropoffLng,
  );

  static TripModel advanceTrip(TripModel trip, TripStatusCode newStatus) {
    return TripModel(
      id: trip.id,
      bookingId: trip.bookingId,
      statusCode: newStatus,
      rawStatus: _rawStatusFor(newStatus),
      pickupAddress: trip.pickupAddress,
      dropoffAddress: trip.dropoffAddress,
      customerName: trip.customerName,
      customerPhone: trip.customerPhone,
      customerPhoto: trip.customerPhoto,
      receiverName: trip.receiverName,
      receiverPhone: trip.receiverPhone,
      distanceKm: trip.distanceKm,
      estimatedFareKobo: trip.estimatedFareKobo,
      createdAt: trip.createdAt,
      notes: trip.notes,
      pickupLat: trip.pickupLat,
      pickupLng: trip.pickupLng,
      dropoffLat: trip.dropoffLat,
      dropoffLng: trip.dropoffLng,
      routePickupPolyline: trip.routePickupPolyline,
      routeDropoffPolyline: trip.routeDropoffPolyline,
    );
  }

  static String _rawStatusFor(TripStatusCode code) => switch (code) {
    TripStatusCode.assigned => 'assigned',
    TripStatusCode.enRoutePickup => 'en_route_pickup',
    TripStatusCode.arrivedPickup => 'arrived_pickup',
    TripStatusCode.inProgress => 'in_progress',
    TripStatusCode.proofSubmitted => 'proof_submitted',
    TripStatusCode.completed => 'completed',
    TripStatusCode.cancelled => 'cancelled',
    TripStatusCode.failed => 'failed',
  };
}
