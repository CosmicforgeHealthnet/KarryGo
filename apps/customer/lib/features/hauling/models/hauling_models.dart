import 'package:cosmicforge_logistics_live_tracking/cosmicforge_logistics_live_tracking.dart';
import 'package:flutter/foundation.dart';

// ─── Booking status ──────────────────────────────────────────────────────────

enum HaulingBookingStatus {
  pendingMatch,
  awaitingAcceptance,
  accepted,
  enRoutePickup,
  arrivedAtPickup,
  pickedUp,
  enRouteDelivery,
  delivered,
  completed,
  cancelled,
  unmatched,
  unknown;

  static HaulingBookingStatus fromString(String v) {
    return switch (v) {
      'pending_match'        => HaulingBookingStatus.pendingMatch,
      'awaiting_acceptance'  => HaulingBookingStatus.awaitingAcceptance,
      'accepted'             => HaulingBookingStatus.accepted,
      'en_route_pickup'      => HaulingBookingStatus.enRoutePickup,
      'arrived_at_pickup'    => HaulingBookingStatus.arrivedAtPickup,
      'picked_up'            => HaulingBookingStatus.pickedUp,
      'en_route_delivery'    => HaulingBookingStatus.enRouteDelivery,
      'delivered'            => HaulingBookingStatus.delivered,
      'completed'            => HaulingBookingStatus.completed,
      'cancelled'            => HaulingBookingStatus.cancelled,
      'unmatched'            => HaulingBookingStatus.unmatched,
      _                      => HaulingBookingStatus.unknown,
    };
  }

  bool get isActive => [
    HaulingBookingStatus.accepted,
    HaulingBookingStatus.enRoutePickup,
    HaulingBookingStatus.arrivedAtPickup,
    HaulingBookingStatus.pickedUp,
    HaulingBookingStatus.enRouteDelivery,
  ].contains(this);

  bool get isSearching => [
    HaulingBookingStatus.pendingMatch,
    HaulingBookingStatus.awaitingAcceptance,
  ].contains(this);

  /// Whether the cargo is not yet aboard, i.e. the driver is still on their
  /// approach leg. Selects which route and which progress leg to show.
  bool get isBeforePickup => [
    HaulingBookingStatus.pendingMatch,
    HaulingBookingStatus.awaitingAcceptance,
    HaulingBookingStatus.accepted,
    HaulingBookingStatus.enRoutePickup,
    HaulingBookingStatus.arrivedAtPickup,
  ].contains(this);

  bool get isTerminal => [
    HaulingBookingStatus.completed,
    HaulingBookingStatus.cancelled,
    HaulingBookingStatus.unmatched,
  ].contains(this);

  String get displayLabel => switch (this) {
    HaulingBookingStatus.pendingMatch       => 'Finding a truck...',
    HaulingBookingStatus.awaitingAcceptance => 'Waiting for provider to accept...',
    HaulingBookingStatus.accepted           => 'Driver accepted',
    HaulingBookingStatus.enRoutePickup      => 'Heading to pickup',
    HaulingBookingStatus.arrivedAtPickup    => 'Driver arrived at pickup',
    HaulingBookingStatus.pickedUp           => 'Cargo picked up',
    HaulingBookingStatus.enRouteDelivery    => 'En route to destination',
    HaulingBookingStatus.delivered          => 'Delivered',
    HaulingBookingStatus.completed          => 'Completed',
    HaulingBookingStatus.cancelled          => 'Cancelled',
    HaulingBookingStatus.unmatched          => 'No provider available',
    HaulingBookingStatus.unknown            => 'Unknown',
  };

  /// Short label used on the Trips list status pill (Figma:
  /// Completed / Ongoing… / Upcoming / Cancelled).
  String get tripChipLabel => switch (this) {
    HaulingBookingStatus.completed => 'Completed',
    HaulingBookingStatus.cancelled => 'Cancelled',
    HaulingBookingStatus.unmatched => 'Cancelled',
    HaulingBookingStatus.delivered => 'Completed',
    _ when isSearching => 'Upcoming',
    _ when isActive => 'Ongoing...',
    _ => 'Ongoing...',
  };

  String get activeTripHeading => switch (this) {
    HaulingBookingStatus.accepted        => 'Driver assigned, arriving soon',
    HaulingBookingStatus.enRoutePickup   => 'Driver is on the way',
    HaulingBookingStatus.arrivedAtPickup => 'Driver has arrived',
    HaulingBookingStatus.pickedUp        => 'Cargo picked up',
    HaulingBookingStatus.enRouteDelivery => 'En route to destination',
    _                                    => 'Trip in progress',
  };
}

// ─── Weight category ─────────────────────────────────────────────────────────

enum WeightCategory {
  light,
  moderate,
  heavy,
  veryHeavy;

  /// Reverse of `.name` (stored on the booking) — null when it doesn't match.
  static WeightCategory? fromName(String name) {
    for (final c in WeightCategory.values) {
      if (c.name == name) return c;
    }
    return null;
  }

  int get kg => switch (this) {
    WeightCategory.light    => 100,
    WeightCategory.moderate => 350,
    WeightCategory.heavy    => 750,
    WeightCategory.veryHeavy => 1500,
  };

  String get displayLabel => switch (this) {
    WeightCategory.light     => 'Light (up to 200 kg)',
    WeightCategory.moderate  => 'Moderate (200 – 500 kg)',
    WeightCategory.heavy     => 'Heavy (500 kg – 1 ton)',
    WeightCategory.veryHeavy => 'Very Heavy (1 ton+)',
  };
}

// ─── Truck type option ───────────────────────────────────────────────────────

/// Truck types a customer can request.
///
/// [apiValue] must stay an identity mapping onto the backend's `ValidTruckTypes`
/// (`provider_profile/models/models.go`) and mirror `providerTruckTypeOptions` in
/// the truck-provider app: the matcher compares the booking's
/// `preferred_truck_type` to the provider's `trucks.truck_type` with exact string
/// equality, so any label that collapses onto a different slug silently makes
/// those providers unmatchable.
enum HaulingTruckTypeOption {
  pickupTruck,
  boxTruck,
  flatbedTruck,
  refrigeratedTruck,
  tanker,
  trailer,
  dumpTruck,
  lowbedTruck,
  craneTruck,
  container,
  van,
  tipper,
  other;

  String get displayLabel => switch (this) {
    HaulingTruckTypeOption.pickupTruck       => 'Pickup Truck',
    HaulingTruckTypeOption.boxTruck          => 'Box Truck',
    HaulingTruckTypeOption.flatbedTruck      => 'Flatbed Truck',
    HaulingTruckTypeOption.refrigeratedTruck => 'Refrigerated Truck',
    HaulingTruckTypeOption.tanker            => 'Tanker',
    HaulingTruckTypeOption.trailer           => 'Trailer (Articulated Truck)',
    HaulingTruckTypeOption.dumpTruck         => 'Dump Truck',
    HaulingTruckTypeOption.lowbedTruck       => 'Lowbed Truck',
    HaulingTruckTypeOption.craneTruck        => 'Crane Truck',
    HaulingTruckTypeOption.container         => 'Container',
    HaulingTruckTypeOption.van               => 'Van',
    HaulingTruckTypeOption.tipper            => 'Tipper',
    // Sends an empty preferred_truck_type, which disables the matcher's type
    // filter — i.e. "any truck". Labelled to say so.
    HaulingTruckTypeOption.other             => 'Any / Not sure',
  };

  String get apiValue => switch (this) {
    HaulingTruckTypeOption.pickupTruck       => 'pickup',
    HaulingTruckTypeOption.boxTruck          => 'box',
    HaulingTruckTypeOption.flatbedTruck      => 'flatbed',
    HaulingTruckTypeOption.refrigeratedTruck => 'refrigerated',
    HaulingTruckTypeOption.tanker            => 'tanker',
    HaulingTruckTypeOption.trailer           => 'trailer',
    HaulingTruckTypeOption.dumpTruck         => 'dump',
    HaulingTruckTypeOption.lowbedTruck       => 'lowbed',
    HaulingTruckTypeOption.craneTruck        => 'crane',
    HaulingTruckTypeOption.container         => 'container',
    HaulingTruckTypeOption.van               => 'van',
    HaulingTruckTypeOption.tipper            => 'tipper',
    // Deliberately empty rather than 'other': no provider registers a truck as
    // `other`, so sending it would match nobody. Empty skips the type filter.
    HaulingTruckTypeOption.other             => '',
  };

  /// Reverse of [apiValue]. The mapping is 1:1, so this round-trips exactly;
  /// returns null for empty/unknown values so the user re-selects.
  static HaulingTruckTypeOption? fromApiValue(String apiValue) {
    if (apiValue.isEmpty) return null;
    for (final o in HaulingTruckTypeOption.values) {
      if (o.apiValue == apiValue) return o;
    }
    return null;
  }
}

// ─── Provider + truck snapshots ───────────────────────────────────────────────

@immutable
class ProviderSnapshot {
  const ProviderSnapshot({
    required this.id,
    required this.firstName,
    required this.lastName,
    this.profilePhotoUrl,
    required this.phone,
    required this.rating,
    required this.totalTrips,
    this.createdAt,
  });

  final String id;
  final String firstName;
  final String lastName;
  final String? profilePhotoUrl;
  final String phone;
  final double rating;
  final int totalTrips;
  final DateTime? createdAt;

  String get displayName => '$firstName $lastName';

  /// Tenure on the platform ("1 year", "3 months") for the Figma driver card.
  String get tenureLabel {
    final since = createdAt;
    if (since == null) return 'New';
    final days = DateTime.now().difference(since).inDays;
    if (days >= 365) {
      final years = days ~/ 365;
      return years == 1 ? '1 year' : '$years years';
    }
    if (days >= 30) {
      final months = days ~/ 30;
      return months == 1 ? '1 month' : '$months months';
    }
    return 'New';
  }

  /// Completed-trips bucket ("1,000+", "100+") for the Figma driver card.
  String get tripsLabel {
    final n = totalTrips;
    if (n >= 1000) return '${n ~/ 1000},000+';
    if (n >= 100) return '${(n ~/ 100) * 100}+';
    if (n >= 10) return '${(n ~/ 10) * 10}+';
    return '$n';
  }

  factory ProviderSnapshot.fromJson(Map<String, dynamic> j) => ProviderSnapshot(
    id: j['id'] as String,
    firstName: j['first_name'] as String? ?? '',
    lastName: j['last_name'] as String? ?? '',
    profilePhotoUrl: j['profile_photo_url'] as String?,
    phone: j['phone'] as String? ?? '',
    rating: (j['rating'] as num? ?? 5.0).toDouble(),
    totalTrips: (j['total_trips'] as num? ?? 0).toInt(),
    createdAt: j['created_at'] != null
        ? DateTime.tryParse(j['created_at'] as String)
        : null,
  );
}

@immutable
class TruckSnapshot {
  const TruckSnapshot({
    required this.id,
    required this.make,
    required this.model,
    required this.color,
    required this.plateNumber,
    required this.truckType,
  });

  final String id;
  final String make;
  final String model;
  final String color;
  final String plateNumber;
  final String truckType;

  String get displayInfo => '$color $make $model';

  factory TruckSnapshot.fromJson(Map<String, dynamic> j) => TruckSnapshot(
    id: j['id'] as String,
    make: j['make'] as String? ?? '',
    model: j['model'] as String? ?? '',
    color: j['color'] as String? ?? '',
    plateNumber: j['plate_number'] as String? ?? '',
    truckType: j['truck_type'] as String? ?? '',
  );
}

// ─── Fare data ────────────────────────────────────────────────────────────────

@immutable
class FareEstimate {
  const FareEstimate({
    required this.distanceKm,
    required this.fareEstimateKobo,
    required this.breakdownKobo,
  });

  final double distanceKm;
  final int fareEstimateKobo;
  final FareBreakdown breakdownKobo;

  double get fareEstimateNaira => fareEstimateKobo / 100;

  factory FareEstimate.fromJson(Map<String, dynamic> j) => FareEstimate(
    distanceKm: (j['distance_km'] as num).toDouble(),
    fareEstimateKobo: (j['fare_estimate_kobo'] as num).toInt(),
    breakdownKobo: FareBreakdown.fromJson(j['breakdown'] as Map<String, dynamic>),
  );
}

@immutable
class FareBreakdown {
  const FareBreakdown({
    required this.baseFareKobo,
    required this.perKmFareKobo,
    required this.weightSurchargeKobo,
    required this.helperFeeKobo,
  });

  final int baseFareKobo;
  final int perKmFareKobo;
  final int weightSurchargeKobo;
  final int helperFeeKobo;

  factory FareBreakdown.fromJson(Map<String, dynamic> j) => FareBreakdown(
    baseFareKobo: (j['base_fare_kobo'] as num? ?? 0).toInt(),
    perKmFareKobo: (j['per_km_fare_kobo'] as num? ?? 0).toInt(),
    weightSurchargeKobo: (j['weight_surcharge_kobo'] as num? ?? 0).toInt(),
    helperFeeKobo: (j['helper_fee_kobo'] as num? ?? 0).toInt(),
  );
}

// ─── Haulage booking ─────────────────────────────────────────────────────────

@immutable
class HaulageBooking {
  const HaulageBooking({
    required this.id,
    required this.customerId,
    this.providerId,
    this.truckId,
    required this.pickupAddress,
    required this.pickupLat,
    required this.pickupLng,
    required this.dropoffAddress,
    required this.dropoffLat,
    required this.dropoffLng,
    required this.cargoWeightKg,
    this.preferredTruckType = '',
    this.cargoDescription = '',
    this.requiresHelpers = false,
    this.helperCount = 0,
    this.weightCategory = '',
    this.receiverName = '',
    this.receiverPhone = '',
    this.packageContent = '',
    this.packageSize = '',
    this.isFragile = false,
    this.distanceKm,
    this.fareEstimateKobo,
    this.fareFinalKobo,
    required this.status,
    this.cancelReason,
    this.scheduledAt,
    this.completedAt,
    this.routePickupPolyline = '',
    this.routeDropoffPolyline = '',
    required this.createdAt,
  });

  final String id;
  final String customerId;
  final String? providerId;
  final String? truckId;
  final String pickupAddress;
  final double pickupLat;
  final double pickupLng;
  final String dropoffAddress;
  final double dropoffLat;
  final double dropoffLng;
  final int cargoWeightKg;
  final String preferredTruckType;
  final String cargoDescription;
  final bool requiresHelpers;
  final int helperCount;
  final String weightCategory;
  final String receiverName;
  final String receiverPhone;
  final String packageContent;
  final String packageSize;
  final bool isFragile;
  final double? distanceKm;
  final int? fareEstimateKobo;
  final int? fareFinalKobo;
  final HaulingBookingStatus status;
  final String? cancelReason;
  final DateTime? scheduledAt;
  final DateTime? completedAt;

  /// Encoded road geometry for the two legs the customer watches: the driver
  /// approaching the pickup, then the cargo moving to the drop-off. Empty when
  /// the service has no routing key configured, in which case the map draws a
  /// straight line and progress is measured as the crow flies.
  final String routePickupPolyline;
  final String routeDropoffPolyline;

  final DateTime createdAt;

  int get displayFareKobo => fareFinalKobo ?? fareEstimateKobo ?? 0;
  double get displayFareNaira => displayFareKobo / 100;

  /// A booking scheduled for a future time that hasn't started yet.
  bool get isUpcoming =>
      scheduledAt != null &&
      scheduledAt!.isAfter(DateTime.now()) &&
      (status.isSearching || status == HaulingBookingStatus.accepted);

  factory HaulageBooking.fromJson(Map<String, dynamic> j) => HaulageBooking(
    id: j['id'] as String,
    customerId: j['customer_id'] as String,
    providerId: j['provider_id'] as String?,
    truckId: j['truck_id'] as String?,
    pickupAddress: j['pickup_address'] as String,
    pickupLat: (j['pickup_lat'] as num).toDouble(),
    pickupLng: (j['pickup_lng'] as num).toDouble(),
    dropoffAddress: j['dropoff_address'] as String,
    dropoffLat: (j['dropoff_lat'] as num).toDouble(),
    dropoffLng: (j['dropoff_lng'] as num).toDouble(),
    cargoWeightKg: (j['cargo_weight_kg'] as num).toInt(),
    preferredTruckType: j['preferred_truck_type'] as String? ?? '',
    cargoDescription: j['cargo_description'] as String? ?? '',
    requiresHelpers: j['requires_helpers'] as bool? ?? false,
    helperCount: (j['helper_count'] as num? ?? 0).toInt(),
    weightCategory: j['weight_category'] as String? ?? '',
    receiverName: j['receiver_name'] as String? ?? '',
    receiverPhone: j['receiver_phone'] as String? ?? '',
    packageContent: j['package_content'] as String? ?? '',
    packageSize: j['package_size'] as String? ?? '',
    isFragile: j['is_fragile'] as bool? ?? false,
    distanceKm: (j['distance_km'] as num?)?.toDouble(),
    fareEstimateKobo: (j['fare_estimate_kobo'] as num?)?.toInt(),
    fareFinalKobo: (j['fare_final_kobo'] as num?)?.toInt(),
    status: HaulingBookingStatus.fromString(j['status'] as String),
    cancelReason: j['cancel_reason'] as String?,
    scheduledAt: j['scheduled_at'] != null
        ? DateTime.tryParse(j['scheduled_at'] as String)
        : null,
    completedAt: j['completed_at'] != null
        ? DateTime.tryParse(j['completed_at'] as String)
        : null,
    routePickupPolyline: j['route_pickup_polyline'] as String? ?? '',
    routeDropoffPolyline: j['route_dropoff_polyline'] as String? ?? '',
    createdAt: DateTime.parse(j['created_at'] as String),
  );

  /// The road geometry for whichever leg the customer is watching: the
  /// driver's approach before pickup, the cargo run after it. Empty means
  /// there is no route to draw and consumers fall back to a straight line.
  String get activeRoutePolyline =>
      status.isBeforePickup ? routePickupPolyline : routeDropoffPolyline;
}

// ─── Availability ─────────────────────────────────────────────────────────────

@immutable
class AvailabilityResult {
  const AvailabilityResult({required this.available, required this.count});

  final bool available;
  final int count;

  factory AvailabilityResult.fromJson(Map<String, dynamic> j) =>
      AvailabilityResult(
        available: j['available'] as bool,
        count: (j['count'] as num? ?? 0).toInt(),
      );
}

// ─── Booking review ─────────────────────────────────────────────────────────

@immutable
class BookingReview {
  const BookingReview({
    required this.id,
    required this.bookingId,
    required this.rating,
    this.reviewText = '',
    this.recommendsDriver,
    required this.createdAt,
  });

  final String id;
  final String bookingId;
  final int rating;
  final String reviewText;
  final bool? recommendsDriver;
  final DateTime createdAt;

  factory BookingReview.fromJson(Map<String, dynamic> j) => BookingReview(
    id: j['id'] as String,
    bookingId: j['booking_id'] as String,
    rating: (j['rating'] as num).toInt(),
    reviewText: j['review_text'] as String? ?? '',
    recommendsDriver: j['recommends_driver'] as bool?,
    createdAt: DateTime.parse(j['created_at'] as String),
  );
}

// ─── Legacy CargoType (kept for backward compat with seeded data display) ────

enum CargoType {
  furniture,
  equipment,
  construction,
  food,
  general;

  String get value => name;

  String get displayLabel => switch (this) {
    CargoType.furniture    => 'Furniture',
    CargoType.equipment    => 'Equipment',
    CargoType.construction => 'Construction materials',
    CargoType.food         => 'Food / perishables',
    CargoType.general      => 'General cargo',
  };
}

// ─── Realtime + live location ────────────────────────────────────────────────

/// Short-lived token for the customer realtime websocket.
class RealtimeToken {
  const RealtimeToken({required this.token, required this.expiresAt});

  final String token;
  final String expiresAt;

  bool get isValid => token.isNotEmpty;
}

/// Live location of the provider assigned to a booking.
class ProviderLocation {
  const ProviderLocation({
    required this.lat,
    required this.lng,
    required this.available,
    this.updatedAt = 0,
    this.heading,
    this.speed,
    this.accuracy,
    this.pickupEta,
    this.dropoffEta,
  });

  final double lat;
  final double lng;
  final bool available;
  final int updatedAt;

  /// When the driver should reach the pickup and the drop-off, from the
  /// server's estimate. Null when it can't be estimated right now.
  final DateTime? pickupEta;
  final DateTime? dropoffEta;

  /// Reads `pickup_eta_seconds` / `dropoff_eta_seconds` into local times.
  /// Seconds-from-now are used rather than the server's clock times so a
  /// phone with the wrong clock still shows the right countdown.
  static (DateTime?, DateTime?) etasFromJson(Map<String, dynamic> json) {
    DateTime? at(String key) {
      final secs = (json[key] as num?)?.toInt();
      return secs == null ? null : DateTime.now().add(Duration(seconds: secs));
    }

    return (at('pickup_eta_seconds'), at('dropoff_eta_seconds'));
  }

  /// Compass bearing in degrees, when the driver's device reported one. Null
  /// means the marker's rotation has to be derived from movement instead.
  final double? heading;

  /// Ground speed in metres per second.
  final double? speed;

  /// Reported horizontal accuracy in metres.
  final double? accuracy;

  factory ProviderLocation.fromJson(Map<String, dynamic> json) {
    final (pickupEta, dropoffEta) = etasFromJson(json);
    return ProviderLocation(
      pickupEta: pickupEta,
      dropoffEta: dropoffEta,
      lat: (json['lat'] as num?)?.toDouble() ?? 0,
      lng: (json['lng'] as num?)?.toDouble() ?? 0,
      available: json['available'] as bool? ?? false,
      updatedAt: (json['updated_at'] as num?)?.toInt() ?? 0,
      heading: (json['heading'] as num?)?.toDouble(),
      speed: (json['speed'] as num?)?.toDouble(),
      accuracy: (json['accuracy'] as num?)?.toDouble(),
    );
  }

  bool get hasFix => available && !(lat == 0 && lng == 0);

  /// The tracking-package view of this fix, for the live map controller.
  TrackPoint? get asTrackPoint {
    if (!hasFix) return null;
    return TrackPoint(
      position: TrackLatLng(lat, lng),
      headingDegrees: heading,
      speedMetersPerSecond: speed,
      accuracyMeters: accuracy,
      recordedAt: updatedAt > 0
          ? DateTime.fromMillisecondsSinceEpoch(updatedAt * 1000, isUtc: true)
          : null,
    );
  }
}

/// Customer-facing view of the delivery verification code (the driver's
/// pickup credential is never exposed to the customer through the API).
class VerificationCodeResult {
  const VerificationCodeResult({
    required this.code,
    this.expiresAt,
    this.attemptsRemaining = 0,
  });

  final String code;
  final DateTime? expiresAt;
  final int attemptsRemaining;

  factory VerificationCodeResult.fromJson(Map<String, dynamic> json) {
    final expiresRaw = json['expires_at'] as String?;
    return VerificationCodeResult(
      code: (json['code'] as String?) ?? '',
      expiresAt: expiresRaw != null ? DateTime.tryParse(expiresRaw) : null,
      attemptsRemaining: (json['attempts_remaining'] as num?)?.toInt() ?? 0,
    );
  }
}

/// The driver's waiting at one stop, as the hauling service computes it.
class HaulingWaitingStop {
  const HaulingWaitingStop({
    required this.stop,
    this.arrivedAt,
    this.freeUntil,
    required this.feeKobo,
    required this.running,
  });

  final String stop;
  final DateTime? arrivedAt;
  final DateTime? freeUntil;
  final int feeKobo;
  final bool running;

  bool get reached => arrivedAt != null;

  factory HaulingWaitingStop.fromJson(Map<String, dynamic> j) => HaulingWaitingStop(
        stop: j['stop'] as String? ?? '',
        arrivedAt: DateTime.tryParse(j['arrived_at'] as String? ?? '')?.toLocal(),
        freeUntil: DateTime.tryParse(j['free_until'] as String? ?? '')?.toLocal(),
        feeKobo: (j['fee_kobo'] as num?)?.toInt() ?? 0,
        running: j['running'] as bool? ?? false,
      );
}

/// Waiting time at pickup and drop-off. Past the free time, the driver's wait
/// is charged per started block.
class HaulingWaiting {
  const HaulingWaiting({
    required this.freeMinutes,
    required this.blockMinutes,
    required this.blockFeeKobo,
    required this.stops,
    required this.totalFeeKobo,
    required this.feeStatus,
  });

  final int freeMinutes;
  final int blockMinutes;
  final int blockFeeKobo;
  final List<HaulingWaitingStop> stops;
  final int totalFeeKobo;
  final String feeStatus;

  HaulingWaitingStop? get running {
    for (final s in stops) {
      if (s.running) return s;
    }
    return null;
  }

  factory HaulingWaiting.fromJson(Map<String, dynamic> j) => HaulingWaiting(
        freeMinutes: (j['free_minutes'] as num?)?.toInt() ?? 0,
        blockMinutes: (j['block_minutes'] as num?)?.toInt() ?? 0,
        blockFeeKobo: (j['block_fee_kobo'] as num?)?.toInt() ?? 0,
        stops: (j['stops'] as List? ?? const [])
            .whereType<Map>()
            .map((m) => HaulingWaitingStop.fromJson(Map<String, dynamic>.from(m)))
            .toList(),
        totalFeeKobo: (j['total_fee_kobo'] as num?)?.toInt() ?? 0,
        feeStatus: j['fee_status'] as String? ?? '',
      );

  /// Same rule as the server: free time, then every started block.
  int feeFor(Duration waited) {
    if (blockMinutes <= 0) return 0;
    final over = waited - Duration(minutes: freeMinutes);
    if (over <= Duration.zero) return 0;
    final block = blockMinutes * 60;
    return ((over.inSeconds + block - 1) ~/ block) * blockFeeKobo;
  }
}
