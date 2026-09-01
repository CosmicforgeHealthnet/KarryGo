import 'package:flutter/foundation.dart';

export '../../hauling/models/hauling_models.dart'
    show ProviderSnapshot, ProviderLocation, RealtimeToken;

// ─── Booking status ──────────────────────────────────────────────────────────

/// Passenger-facing ride status, mirroring driver-taxi-service's
/// `bookingmodels.Status` ladder exactly.
///
/// `pickup_verification_pending` is declared by the service but deliberately
/// never assigned to a booking's status (the finer state lives in
/// [PickupVerification] instead), so it is mapped here only so an unexpected
/// value never degrades to [unknown].
enum TaxiBookingStatus {
  scheduled,
  pendingMatch,
  awaitingAcceptance,
  accepted,
  enRoutePickup,
  arrivedAtPickup,
  inProgress,
  completed,
  cancelled,
  unmatched,
  unknown;

  static TaxiBookingStatus fromString(String v) {
    return switch (v) {
      'scheduled' => TaxiBookingStatus.scheduled,
      'pending_match' => TaxiBookingStatus.pendingMatch,
      'awaiting_acceptance' => TaxiBookingStatus.awaitingAcceptance,
      'accepted' => TaxiBookingStatus.accepted,
      'en_route_pickup' => TaxiBookingStatus.enRoutePickup,
      'arrived_at_pickup' => TaxiBookingStatus.arrivedAtPickup,
      'pickup_verification_pending' => TaxiBookingStatus.arrivedAtPickup,
      'in_progress' => TaxiBookingStatus.inProgress,
      'completed' => TaxiBookingStatus.completed,
      'cancelled' => TaxiBookingStatus.cancelled,
      'unmatched' => TaxiBookingStatus.unmatched,
      _ => TaxiBookingStatus.unknown,
    };
  }

  /// Still looking for a driver — no provider is attached yet.
  bool get isSearching =>
      this == TaxiBookingStatus.pendingMatch ||
      this == TaxiBookingStatus.awaitingAcceptance;

  /// A driver is assigned and the ride is underway.
  bool get isActive => const [
    TaxiBookingStatus.accepted,
    TaxiBookingStatus.enRoutePickup,
    TaxiBookingStatus.arrivedAtPickup,
    TaxiBookingStatus.inProgress,
  ].contains(this);

  /// Before the passenger is aboard, i.e. the driver is still approaching.
  bool get isBeforePickup => const [
    TaxiBookingStatus.pendingMatch,
    TaxiBookingStatus.awaitingAcceptance,
    TaxiBookingStatus.accepted,
    TaxiBookingStatus.enRoutePickup,
    TaxiBookingStatus.arrivedAtPickup,
  ].contains(this);

  bool get isTerminal => const [
    TaxiBookingStatus.completed,
    TaxiBookingStatus.cancelled,
    TaxiBookingStatus.unmatched,
  ].contains(this);

  String get displayLabel => switch (this) {
    TaxiBookingStatus.scheduled => 'Scheduled',
    TaxiBookingStatus.pendingMatch => 'Finding a driver...',
    TaxiBookingStatus.awaitingAcceptance => 'Waiting for a driver to accept',
    TaxiBookingStatus.accepted => 'Driver assigned',
    TaxiBookingStatus.enRoutePickup => 'Driver on the way',
    TaxiBookingStatus.arrivedAtPickup => 'Driver has arrived',
    TaxiBookingStatus.inProgress => 'On the trip',
    TaxiBookingStatus.completed => 'Completed',
    TaxiBookingStatus.cancelled => 'Cancelled',
    TaxiBookingStatus.unmatched => 'No driver available',
    TaxiBookingStatus.unknown => 'Unknown',
  };

  /// Short label used on the Trips list status pill.
  String get tripChipLabel => switch (this) {
    TaxiBookingStatus.completed => 'Completed',
    TaxiBookingStatus.cancelled => 'Cancelled',
    TaxiBookingStatus.unmatched => 'Cancelled',
    TaxiBookingStatus.scheduled => 'Upcoming',
    _ when isSearching => 'Upcoming',
    _ => 'Ongoing...',
  };

  String get activeTripHeading => switch (this) {
    TaxiBookingStatus.accepted => 'Driver assigned',
    TaxiBookingStatus.enRoutePickup => 'Your driver is on the way',
    TaxiBookingStatus.arrivedAtPickup => 'Your driver has arrived',
    TaxiBookingStatus.inProgress => 'Trip has started',
    _ => 'Ride in progress',
  };

  /// The line under [activeTripHeading]. Deliberately NOT [displayLabel],
  /// which restates the heading almost verbatim; this says what happens next.
  ///
  /// Parcel wording differs throughout: telling someone to "sit back and enjoy
  /// the ride" when they are standing at home waiting on a package reads as a
  /// bug, not a pleasantry.
  String activeTripSubtitle({bool isParcel = false}) => switch (this) {
    TaxiBookingStatus.accepted => isParcel
        ? 'Your driver is heading to collect the package.'
        : 'Your driver is heading to the pickup point.',
    TaxiBookingStatus.enRoutePickup => isParcel
        ? 'Have the package ready for collection.'
        : 'Meet them at your pickup point.',
    TaxiBookingStatus.arrivedAtPickup => isParcel
        ? 'Show your driver the code below to hand over the package.'
        : 'Show your driver the code below to start the trip.',
    TaxiBookingStatus.inProgress => isParcel
        ? 'The package is on its way to the receiver.'
        : 'Sit back and enjoy the ride.',
    _ => isParcel ? 'Your delivery is under way.' : 'Your ride is under way.',
  };
}

/// The pickup handshake, tracked independently of the booking status.
///
/// Direction is the opposite of a package delivery: the PASSENGER displays the
/// code and the DRIVER verifies it, because in a taxi the passenger is the
/// thing being picked up.
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

/// The drop-off half of a parcel's handover. A passenger ride stays
/// [notRequired] forever — there is nothing to hand over at the end of a ride.
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

/// What the customer is putting in the car: themselves, or a package.
enum TaxiRideMode {
  ride,
  parcel;

  /// The service's own `ride_purpose` vocabulary.
  String get wireValue =>
      this == TaxiRideMode.parcel ? 'parcel' : 'passenger';

  static TaxiRideMode fromString(String v) =>
      v == 'parcel' ? TaxiRideMode.parcel : TaxiRideMode.ride;

  bool get isParcel => this == TaxiRideMode.parcel;

  String get title => switch (this) {
    TaxiRideMode.ride => 'Ride',
    TaxiRideMode.parcel => 'Send a package',
  };

  String get subtitle => switch (this) {
    TaxiRideMode.ride => 'Book a car for yourself.',
    TaxiRideMode.parcel => 'Send a package by car.',
  };
}

// ─── Vehicle class ───────────────────────────────────────────────────────────

/// One bookable class from `GET /vehicle-classes`.
@immutable
class TaxiVehicleClass {
  const TaxiVehicleClass({
    required this.vehicleClass,
    required this.label,
    required this.maxPassengers,
  });

  final String vehicleClass;
  final String label;
  final int maxPassengers;

  factory TaxiVehicleClass.fromJson(Map<String, dynamic> json) {
    return TaxiVehicleClass(
      vehicleClass: (json['vehicle_class'] as String?) ?? '',
      label: (json['label'] as String?) ?? '',
      maxPassengers: (json['max_passengers'] as num?)?.toInt() ?? 1,
    );
  }
}

// ─── Fare estimate ───────────────────────────────────────────────────────────

/// One quote tile from `POST /customer/bookings/estimate`. The service returns
/// one per class the pickup's zone supports, cheapest first.
@immutable
class TaxiFareTile {
  const TaxiFareTile({
    required this.vehicleClass,
    required this.label,
    required this.fareKobo,
    required this.distanceKm,
    required this.durationMin,
    required this.quoteReference,
  });

  final String vehicleClass;
  final String label;
  final int fareKobo;
  final double distanceKm;
  final int durationMin;
  final String quoteReference;

  factory TaxiFareTile.fromJson(Map<String, dynamic> json) {
    return TaxiFareTile(
      vehicleClass: (json['vehicle_class'] as String?) ?? '',
      label: (json['label'] as String?) ?? '',
      fareKobo: (json['fare_kobo'] as num?)?.toInt() ?? 0,
      distanceKm: (json['distance_km'] as num?)?.toDouble() ?? 0,
      durationMin: (json['duration_min'] as num?)?.toInt() ?? 0,
      quoteReference: (json['quote_reference'] as String?) ?? '',
    );
  }
}

// ─── Availability ────────────────────────────────────────────────────────────

@immutable
class TaxiAvailability {
  const TaxiAvailability({required this.available, required this.count});

  final bool available;
  final int count;

  factory TaxiAvailability.fromJson(Map<String, dynamic> json) {
    return TaxiAvailability(
      available: json['available'] == true,
      count: (json['count'] as num?)?.toInt() ?? 0,
    );
  }
}

// ─── Stops ───────────────────────────────────────────────────────────────────

@immutable
class TaxiStop {
  const TaxiStop({
    required this.id,
    required this.sequence,
    required this.type,
    required this.address,
    required this.lat,
    required this.lng,
    required this.status,
  });

  final String id;
  final int sequence;
  final String type; // PICKUP | WAYPOINT | DROPOFF
  final String address;
  final double lat;
  final double lng;
  final String status;

  bool get isPickup => type == 'PICKUP';
  bool get isDropoff => type == 'DROPOFF';

  factory TaxiStop.fromJson(Map<String, dynamic> json) {
    return TaxiStop(
      id: (json['id'] as String?) ?? '',
      sequence: (json['sequence'] as num?)?.toInt() ?? 0,
      type: (json['type'] as String?) ?? '',
      address: (json['address'] as String?) ?? '',
      lat: (json['lat'] as num?)?.toDouble() ?? 0,
      lng: (json['lng'] as num?)?.toDouble() ?? 0,
      status: (json['status'] as String?) ?? '',
    );
  }
}

// ─── Booking ─────────────────────────────────────────────────────────────────

@immutable
class TaxiBooking {
  const TaxiBooking({
    required this.id,
    required this.status,
    required this.pickupVerification,
    required this.rideMode,
    required this.deliveryVerification,
    required this.vehicleClass,
    required this.passengerCount,
    required this.rideNotes,
    required this.paymentMethod,
    required this.paymentStatus,
    required this.stops,
    this.providerId,
    this.fareEstimateKobo,
    this.fareFinalKobo,
    this.distanceKm,
    this.durationMinutes,
    this.receiverName = '',
    this.receiverPhone = '',
    this.packageDesc = '',
    this.packageType = '',
    this.packageSize = '',
    this.isFragile = false,
    this.cancelReason,
    this.cancelledBy,
    this.scheduledAt,
    this.acceptedAt,
    this.arrivedAt,
    this.startedAt,
    this.completedAt,
    this.cancelledAt,
    this.createdAt,
  });

  final String id;
  final TaxiBookingStatus status;
  final PickupVerification pickupVerification;
  final TaxiRideMode rideMode;
  final DeliveryVerification deliveryVerification;
  final String vehicleClass;
  final int passengerCount;
  final String rideNotes;
  final String paymentMethod;
  final String paymentStatus;
  final List<TaxiStop> stops;
  final String? providerId;
  final int? fareEstimateKobo;
  final int? fareFinalKobo;
  final double? distanceKm;
  final int? durationMinutes;
  final String receiverName;
  final String receiverPhone;
  final String packageDesc;
  final String packageType;
  final String packageSize;
  final bool isFragile;
  final String? cancelReason;
  final String? cancelledBy;
  final DateTime? scheduledAt;
  final DateTime? acceptedAt;
  final DateTime? arrivedAt;
  final DateTime? startedAt;
  final DateTime? completedAt;
  final DateTime? cancelledAt;
  final DateTime? createdAt;

  bool get isParcel => rideMode.isParcel;

  /// The fare to show: the final settled amount once known, else the estimate.
  int get displayFareKobo => fareFinalKobo ?? fareEstimateKobo ?? 0;

  TaxiStop? get pickup {
    for (final s in stops) {
      if (s.isPickup) return s;
    }
    return stops.isNotEmpty ? stops.first : null;
  }

  TaxiStop? get dropoff {
    for (final s in stops.reversed) {
      if (s.isDropoff) return s;
    }
    return stops.isNotEmpty ? stops.last : null;
  }

  /// A future-dated ride that has not yet entered matching.
  bool get isUpcoming =>
      status == TaxiBookingStatus.scheduled &&
      scheduledAt != null &&
      scheduledAt!.isAfter(DateTime.now());

  static DateTime? _date(dynamic v) {
    if (v is! String || v.isEmpty) return null;
    return DateTime.tryParse(v)?.toLocal();
  }

  factory TaxiBooking.fromJson(Map<String, dynamic> json) {
    final rawStops = json['stops'];
    return TaxiBooking(
      id: (json['id'] as String?) ?? '',
      status: TaxiBookingStatus.fromString((json['status'] as String?) ?? ''),
      pickupVerification: PickupVerification.fromString(
        (json['pickup_verification_status'] as String?) ?? '',
      ),
      rideMode: TaxiRideMode.fromString((json['ride_purpose'] as String?) ?? ''),
      deliveryVerification: DeliveryVerification.fromString(
        (json['delivery_verification_status'] as String?) ?? '',
      ),
      vehicleClass: (json['vehicle_class'] as String?) ?? '',
      passengerCount: (json['passenger_count'] as num?)?.toInt() ?? 1,
      rideNotes: (json['ride_notes'] as String?) ?? '',
      paymentMethod: (json['payment_method'] as String?) ?? '',
      paymentStatus: (json['payment_status'] as String?) ?? '',
      stops: rawStops is List
          ? rawStops
                .map((e) => TaxiStop.fromJson(Map<String, dynamic>.from(e as Map)))
                .toList()
          : const [],
      providerId: json['provider_id'] as String?,
      fareEstimateKobo: (json['fare_estimate_kobo'] as num?)?.toInt(),
      fareFinalKobo: (json['fare_final_kobo'] as num?)?.toInt(),
      distanceKm: (json['distance_km'] as num?)?.toDouble(),
      durationMinutes: (json['duration_minutes'] as num?)?.toInt(),
      receiverName: (json['receiver_name'] as String?) ?? '',
      receiverPhone: (json['receiver_phone'] as String?) ?? '',
      packageDesc: (json['package_desc'] as String?) ?? '',
      packageType: (json['package_type'] as String?) ?? '',
      packageSize: (json['package_size'] as String?) ?? '',
      isFragile: json['is_fragile'] == true,
      cancelReason: json['cancel_reason'] as String?,
      cancelledBy: json['cancelled_by'] as String?,
      scheduledAt: _date(json['scheduled_at']),
      acceptedAt: _date(json['accepted_at']),
      arrivedAt: _date(json['arrived_at']),
      startedAt: _date(json['started_at']),
      completedAt: _date(json['completed_at']),
      cancelledAt: _date(json['cancelled_at']),
      createdAt: _date(json['created_at']),
    );
  }
}

// ─── Pickup credential ───────────────────────────────────────────────────────

/// A short-lived handover code the customer displays. Used for BOTH steps:
/// the sender's pickup code at the kerb, and the receiver's delivery code at
/// drop-off for a parcel. The wire shape is identical, so one type serves both.
@immutable
class TaxiHandoverCredential {
  const TaxiHandoverCredential({required this.code, this.expiresAt});

  final String code;
  final DateTime? expiresAt;

  bool get isExpired =>
      expiresAt != null && expiresAt!.isBefore(DateTime.now());

  factory TaxiHandoverCredential.fromJson(Map<String, dynamic> json) {
    final raw = json['expires_at'];
    return TaxiHandoverCredential(
      code: (json['code'] as String?) ?? '',
      expiresAt: raw is String ? DateTime.tryParse(raw)?.toLocal() : null,
    );
  }
}

/// The original name, kept so existing pickup call sites read unchanged.
typedef TaxiPickupCredential = TaxiHandoverCredential;

// ─── Reviews ─────────────────────────────────────────────────────────────────

/// One side of a two-way review. [visible] is false while the service is
/// withholding the other party's text until this passenger has written theirs.
@immutable
class TaxiReviewSide {
  const TaxiReviewSide({
    required this.submitted,
    required this.visible,
    this.rating,
    this.reviewText = '',
    this.recommendsDriver,
  });

  final bool submitted;
  final bool visible;
  final int? rating;
  final String reviewText;
  final bool? recommendsDriver;

  factory TaxiReviewSide.fromJson(Map<String, dynamic>? json) {
    if (json == null) {
      return const TaxiReviewSide(submitted: false, visible: false);
    }
    return TaxiReviewSide(
      submitted: json['submitted'] == true,
      visible: json['visible'] == true,
      rating: (json['rating'] as num?)?.toInt(),
      reviewText: (json['review_text'] as String?) ?? '',
      recommendsDriver: json['recommends_driver'] as bool?,
    );
  }
}

@immutable
class TaxiBookingReviews {
  const TaxiBookingReviews({required this.mine, required this.theirs});

  final TaxiReviewSide mine;
  final TaxiReviewSide theirs;

  bool get iHaveReviewed => mine.submitted;

  factory TaxiBookingReviews.fromJson(Map<String, dynamic> json) {
    Map<String, dynamic>? side(String key) {
      final v = json[key];
      return v is Map ? Map<String, dynamic>.from(v) : null;
    }

    return TaxiBookingReviews(
      mine: TaxiReviewSide.fromJson(side('mine')),
      theirs: TaxiReviewSide.fromJson(side('theirs')),
    );
  }
}
