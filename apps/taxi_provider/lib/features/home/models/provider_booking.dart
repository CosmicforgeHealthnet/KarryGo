import 'package:flutter/foundation.dart';

/// The ride lifecycle as the driver sees it, mirroring the service's own
/// status ladder.
enum BookingStatus {
  pendingMatch,
  awaitingAcceptance,
  accepted,
  enRoutePickup,
  arrivedAtPickup,
  inProgress,
  completed,
  cancelled,
  unmatched,
  scheduled,
  unknown;

  static BookingStatus fromString(String v) => switch (v) {
    'pending_match' => BookingStatus.pendingMatch,
    'awaiting_acceptance' => BookingStatus.awaitingAcceptance,
    'accepted' => BookingStatus.accepted,
    'en_route_pickup' => BookingStatus.enRoutePickup,
    'arrived_at_pickup' => BookingStatus.arrivedAtPickup,
    'pickup_verification_pending' => BookingStatus.arrivedAtPickup,
    'in_progress' => BookingStatus.inProgress,
    'completed' => BookingStatus.completed,
    'cancelled' => BookingStatus.cancelled,
    'unmatched' => BookingStatus.unmatched,
    'scheduled' => BookingStatus.scheduled,
    _ => BookingStatus.unknown,
  };

  /// An offer the driver has been handed but not yet answered.
  bool get isOffer => this == BookingStatus.awaitingAcceptance;

  /// Committed and not yet finished — this is the job that takes over the
  /// screen.
  bool get isActive => const [
    BookingStatus.accepted,
    BookingStatus.enRoutePickup,
    BookingStatus.arrivedAtPickup,
    BookingStatus.inProgress,
  ].contains(this);

  bool get isTerminal => const [
    BookingStatus.completed,
    BookingStatus.cancelled,
    BookingStatus.unmatched,
  ].contains(this);
}

enum PickupVerification {
  notRequired,
  pending,
  verified,
  unknown;

  static PickupVerification fromString(String v) => switch (v) {
    'not_required' => PickupVerification.notRequired,
    'pending' => PickupVerification.pending,
    'verified' => PickupVerification.verified,
    _ => PickupVerification.unknown,
  };
}

enum DeliveryVerification {
  notRequired,
  pending,
  verified,
  unknown;

  static DeliveryVerification fromString(String v) => switch (v) {
    'not_required' => DeliveryVerification.notRequired,
    'pending' => DeliveryVerification.pending,
    'verified' => DeliveryVerification.verified,
    _ => DeliveryVerification.unknown,
  };
}

@immutable
class BookingStop {
  const BookingStop({
    required this.sequence,
    required this.type,
    required this.address,
    required this.lat,
    required this.lng,
  });

  final int sequence;
  final String type;
  final String address;
  final double lat;
  final double lng;

  bool get isPickup => type == 'PICKUP';
  bool get isDropoff => type == 'DROPOFF';

  factory BookingStop.fromJson(Map<String, dynamic> json) {
    return BookingStop(
      sequence: (json['sequence'] as num?)?.toInt() ?? 0,
      type: (json['type'] as String?) ?? '',
      address: (json['address'] as String?) ?? '',
      lat: (json['lat'] as num?)?.toDouble() ?? 0,
      lng: (json['lng'] as num?)?.toDouble() ?? 0,
    );
  }
}

/// One booking as returned to the driver. Note the service never exposes any
/// verification secret here — codes come from their own endpoints.
@immutable
class ProviderBooking {
  const ProviderBooking({
    required this.id,
    required this.status,
    required this.pickupVerification,
    required this.deliveryVerification,
    required this.isParcel,
    required this.vehicleClass,
    required this.stops,
    this.fareEstimateKobo,
    this.fareFinalKobo,
    this.distanceKm,
    this.durationMinutes,
    this.rideNotes = '',
    this.receiverName = '',
    this.receiverPhone = '',
    this.packageDesc = '',
    this.packageSize = '',
    this.isFragile = false,
    this.createdAt,
  });

  final String id;
  final BookingStatus status;
  final PickupVerification pickupVerification;
  final DeliveryVerification deliveryVerification;
  final bool isParcel;
  final String vehicleClass;
  final List<BookingStop> stops;
  final int? fareEstimateKobo;
  final int? fareFinalKobo;
  final double? distanceKm;
  final int? durationMinutes;
  final String rideNotes;
  final String receiverName;
  final String receiverPhone;
  final String packageDesc;
  final String packageSize;
  final bool isFragile;
  final DateTime? createdAt;

  int get displayFareKobo => fareFinalKobo ?? fareEstimateKobo ?? 0;

  /// This booking with [stops] in place of its own. Lifecycle responses
  /// (accept, arrive, start) carry only the booking row, so the route the
  /// driver already has is carried over rather than lost.
  ProviderBooking withStops(List<BookingStop> stops) => ProviderBooking(
    id: id,
    status: status,
    pickupVerification: pickupVerification,
    deliveryVerification: deliveryVerification,
    isParcel: isParcel,
    vehicleClass: vehicleClass,
    stops: stops,
    fareEstimateKobo: fareEstimateKobo,
    fareFinalKobo: fareFinalKobo,
    distanceKm: distanceKm,
    durationMinutes: durationMinutes,
    rideNotes: rideNotes,
    receiverName: receiverName,
    receiverPhone: receiverPhone,
    packageDesc: packageDesc,
    packageSize: packageSize,
    isFragile: isFragile,
    createdAt: createdAt,
  );

  String get shortId =>
      id.length >= 8 ? id.substring(0, 8).toUpperCase() : id.toUpperCase();

  BookingStop? get pickup {
    for (final s in stops) {
      if (s.isPickup) return s;
    }
    return stops.isNotEmpty ? stops.first : null;
  }

  BookingStop? get dropoff {
    for (final s in stops.reversed) {
      if (s.isDropoff) return s;
    }
    return stops.isNotEmpty ? stops.last : null;
  }

  /// The driver must verify the passenger's code before the trip can start.
  bool get awaitingPickupVerification =>
      pickupVerification == PickupVerification.pending;

  /// A parcel whose receiver code has not been checked yet — completion is
  /// refused server-side until it is.
  bool get awaitingDeliveryVerification =>
      deliveryVerification == DeliveryVerification.pending;

  static DateTime? _date(dynamic v) {
    if (v is! String || v.isEmpty) return null;
    return DateTime.tryParse(v)?.toLocal();
  }

  factory ProviderBooking.fromJson(Map<String, dynamic> json) {
    final rawStops = json['stops'];
    return ProviderBooking(
      id: (json['id'] as String?) ?? '',
      status: BookingStatus.fromString((json['status'] as String?) ?? ''),
      pickupVerification: PickupVerification.fromString(
        (json['pickup_verification_status'] as String?) ?? '',
      ),
      deliveryVerification: DeliveryVerification.fromString(
        (json['delivery_verification_status'] as String?) ?? '',
      ),
      isParcel: (json['ride_purpose'] as String?) == 'parcel',
      vehicleClass: (json['vehicle_class'] as String?) ?? '',
      stops: rawStops is List
          ? rawStops
                .map((e) => BookingStop.fromJson(Map<String, dynamic>.from(e as Map)))
                .toList()
          : const [],
      fareEstimateKobo: (json['fare_estimate_kobo'] as num?)?.toInt(),
      fareFinalKobo: (json['fare_final_kobo'] as num?)?.toInt(),
      distanceKm: (json['distance_km'] as num?)?.toDouble(),
      durationMinutes: (json['duration_minutes'] as num?)?.toInt(),
      rideNotes: (json['ride_notes'] as String?) ?? '',
      receiverName: (json['receiver_name'] as String?) ?? '',
      receiverPhone: (json['receiver_phone'] as String?) ?? '',
      packageDesc: (json['package_desc'] as String?) ?? '',
      packageSize: (json['package_size'] as String?) ?? '',
      isFragile: json['is_fragile'] == true,
      createdAt: _date(json['created_at']),
    );
  }
}

/// The driver's current online state, as the service reports it.
@immutable
class AvailabilityState {
  const AvailabilityState({
    required this.online,
    this.vehicleId = '',
    this.heartbeatSeconds = 30,
  });

  final bool online;
  final String vehicleId;
  final int heartbeatSeconds;

  factory AvailabilityState.fromJson(Map<String, dynamic> json) {
    return AvailabilityState(
      online: json['online'] == true,
      vehicleId: (json['vehicle_id'] as String?) ?? '',
      heartbeatSeconds: (json['heartbeat_seconds'] as num?)?.toInt() ?? 30,
    );
  }
}
