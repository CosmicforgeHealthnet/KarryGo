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
  other;

  String get displayLabel => switch (this) {
    HaulingTruckTypeOption.pickupTruck       => 'Pickup Truck',
    HaulingTruckTypeOption.boxTruck          => 'Box Truck',
    HaulingTruckTypeOption.flatbedTruck      => 'Flatbed Truck',
    HaulingTruckTypeOption.refrigeratedTruck => 'Refrigerated Truck',
    HaulingTruckTypeOption.tanker            => 'Tanker',
    HaulingTruckTypeOption.trailer           => 'Trailer',
    HaulingTruckTypeOption.dumpTruck         => 'Dump Truck',
    HaulingTruckTypeOption.lowbedTruck       => 'Lowbed Truck',
    HaulingTruckTypeOption.craneTruck        => 'Crane Truck',
    HaulingTruckTypeOption.other             => 'Other',
  };

  String get apiValue => switch (this) {
    HaulingTruckTypeOption.pickupTruck       => 'van',
    HaulingTruckTypeOption.boxTruck          => 'van',
    HaulingTruckTypeOption.flatbedTruck      => 'flatbed',
    HaulingTruckTypeOption.refrigeratedTruck => 'refrigerated',
    HaulingTruckTypeOption.tanker            => 'container',
    HaulingTruckTypeOption.trailer           => 'container',
    HaulingTruckTypeOption.dumpTruck         => 'tipper',
    HaulingTruckTypeOption.lowbedTruck       => 'flatbed',
    HaulingTruckTypeOption.craneTruck        => 'flatbed',
    HaulingTruckTypeOption.other             => '',
  };

  /// Best-effort reverse of `apiValue` (the backend stores the collapsed value,
  /// e.g. 'flatbed'/'van'/'container'). The mapping is many-to-one, so this picks
  /// the first matching option — good enough to prefill a re-book; returns null
  /// for empty/unknown values so the user re-selects.
  static HaulingTruckTypeOption? fromApiValue(String apiValue) {
    if (apiValue.isEmpty) return null;
    for (final o in HaulingTruckTypeOption.values) {
      if (o.apiValue == apiValue) return o;
    }
    return null;
  }
}

// ─── Truck tier ───────────────────────────────────────────────────────────────

enum TruckTier {
  normal,
  economy,
  comfort;

  String get displayLabel => switch (this) {
    TruckTier.normal  => 'Normal',
    TruckTier.economy => 'Economy',
    TruckTier.comfort => 'Comfort',
  };

  String get description => switch (this) {
    TruckTier.normal  => 'Standard truck for everyday cargo',
    TruckTier.economy => 'Budget-friendly option for light loads',
    TruckTier.comfort => 'Premium service with extra care for fragile items',
  };
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
  });

  final String id;
  final String firstName;
  final String lastName;
  final String? profilePhotoUrl;
  final String phone;
  final double rating;
  final int totalTrips;

  String get displayName => '$firstName $lastName';

  factory ProviderSnapshot.fromJson(Map<String, dynamic> j) => ProviderSnapshot(
    id: j['id'] as String,
    firstName: j['first_name'] as String? ?? '',
    lastName: j['last_name'] as String? ?? '',
    profilePhotoUrl: j['profile_photo_url'] as String?,
    phone: j['phone'] as String? ?? '',
    rating: (j['rating'] as num? ?? 5.0).toDouble(),
    totalTrips: (j['total_trips'] as num? ?? 0).toInt(),
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
    createdAt: DateTime.parse(j['created_at'] as String),
  );
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

// ─── Card payment init ───────────────────────────────────────────────────────

/// Result of starting an up-front card (Paystack) payment for a booking.
class CardPaymentInit {
  const CardPaymentInit({
    required this.authorizationUrl,
    required this.paymentIntentId,
  });

  final String authorizationUrl;
  final String paymentIntentId;
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
  });

  final double lat;
  final double lng;
  final bool available;
  final int updatedAt;

  factory ProviderLocation.fromJson(Map<String, dynamic> json) {
    return ProviderLocation(
      lat: (json['lat'] as num?)?.toDouble() ?? 0,
      lng: (json['lng'] as num?)?.toDouble() ?? 0,
      available: json['available'] as bool? ?? false,
      updatedAt: (json['updated_at'] as num?)?.toInt() ?? 0,
    );
  }

  bool get hasFix => available && !(lat == 0 && lng == 0);
}
