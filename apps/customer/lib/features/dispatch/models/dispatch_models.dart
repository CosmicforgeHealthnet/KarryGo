import 'package:flutter/foundation.dart';

import '../../hauling/models/hauling_models.dart' show ProviderSnapshot;

export '../../hauling/models/hauling_models.dart'
    show ProviderSnapshot, ProviderLocation, RealtimeToken;

// ─── Booking status ──────────────────────────────────────────────────────────

/// Customer-facing dispatch delivery status, derived server-side from the
/// broadcast + trip state (see dispatch-delivery-service booking feature).
enum DispatchBookingStatus {
  searching,
  accepted,
  enRoutePickup,
  arrivedAtPickup,
  inTransit,
  delivered,
  completed,
  cancelled,
  unmatched,
  unknown;

  static DispatchBookingStatus fromString(String v) {
    return switch (v) {
      'searching'         => DispatchBookingStatus.searching,
      'accepted'          => DispatchBookingStatus.accepted,
      'en_route_pickup'   => DispatchBookingStatus.enRoutePickup,
      'arrived_at_pickup' => DispatchBookingStatus.arrivedAtPickup,
      'in_transit'        => DispatchBookingStatus.inTransit,
      'delivered'         => DispatchBookingStatus.delivered,
      'completed'         => DispatchBookingStatus.completed,
      'cancelled'         => DispatchBookingStatus.cancelled,
      'unmatched'         => DispatchBookingStatus.unmatched,
      _                   => DispatchBookingStatus.unknown,
    };
  }

  bool get isActive => [
    DispatchBookingStatus.accepted,
    DispatchBookingStatus.enRoutePickup,
    DispatchBookingStatus.arrivedAtPickup,
    DispatchBookingStatus.inTransit,
  ].contains(this);

  bool get isSearching => this == DispatchBookingStatus.searching;

  bool get isTerminal => [
    DispatchBookingStatus.completed,
    DispatchBookingStatus.cancelled,
    DispatchBookingStatus.unmatched,
  ].contains(this);

  String get displayLabel => switch (this) {
    DispatchBookingStatus.searching       => 'Finding a rider...',
    DispatchBookingStatus.accepted        => 'Rider accepted',
    DispatchBookingStatus.enRoutePickup   => 'Rider heading to pickup',
    DispatchBookingStatus.arrivedAtPickup => 'Rider arrived at pickup',
    DispatchBookingStatus.inTransit       => 'Package in transit',
    DispatchBookingStatus.delivered       => 'Delivered',
    DispatchBookingStatus.completed       => 'Completed',
    DispatchBookingStatus.cancelled       => 'Cancelled',
    DispatchBookingStatus.unmatched       => 'No rider available',
    DispatchBookingStatus.unknown         => 'Unknown',
  };

  /// Short label used on the Trips list status pill.
  String get tripChipLabel => switch (this) {
    DispatchBookingStatus.completed => 'Completed',
    DispatchBookingStatus.delivered => 'Completed',
    DispatchBookingStatus.cancelled => 'Cancelled',
    DispatchBookingStatus.unmatched => 'Cancelled',
    _ when isSearching => 'Upcoming',
    _ => 'Ongoing...',
  };

  String get activeTripHeading => switch (this) {
    DispatchBookingStatus.accepted        => 'Rider assigned, arriving soon',
    DispatchBookingStatus.enRoutePickup   => 'Rider is on the way',
    DispatchBookingStatus.arrivedAtPickup => 'Rider has arrived',
    DispatchBookingStatus.inTransit       => 'Package is on its way',
    _                                     => 'Delivery in progress',
  };
}

// ─── Package size + service tier ─────────────────────────────────────────────

enum PackageSizeOption {
  small,
  medium,
  large;

  static PackageSizeOption? fromName(String name) {
    for (final o in PackageSizeOption.values) {
      if (o.name == name) return o;
    }
    return null;
  }

  String get displayLabel => switch (this) {
    PackageSizeOption.small  => 'Small',
    PackageSizeOption.medium => 'Medium',
    PackageSizeOption.large  => 'Large',
  };

  String get description => switch (this) {
    PackageSizeOption.small  => 'Fits in a backpack',
    PackageSizeOption.medium => 'Fits in a delivery box',
    PackageSizeOption.large  => 'Needs the full carrier',
  };
}

enum ServiceTierOption {
  standard,
  express;

  static ServiceTierOption? fromName(String name) {
    for (final o in ServiceTierOption.values) {
      if (o.name == name) return o;
    }
    return null;
  }

  String get displayLabel => switch (this) {
    ServiceTierOption.standard => 'Standard',
    ServiceTierOption.express  => 'Express',
  };

  String get description => switch (this) {
    ServiceTierOption.standard => 'Regular delivery speed',
    ServiceTierOption.express  => 'Priority pickup and delivery',
  };
}

// ─── Fare data ────────────────────────────────────────────────────────────────

@immutable
class DispatchFareEstimate {
  const DispatchFareEstimate({
    required this.distanceKm,
    required this.fareEstimateKobo,
    required this.eta,
    required this.breakdownKobo,
  });

  final double distanceKm;
  final int fareEstimateKobo;
  final String eta;
  final DispatchFareBreakdown breakdownKobo;

  double get fareEstimateNaira => fareEstimateKobo / 100;

  factory DispatchFareEstimate.fromJson(Map<String, dynamic> j) =>
      DispatchFareEstimate(
        distanceKm: (j['distance_km'] as num).toDouble(),
        fareEstimateKobo: (j['fare_estimate_kobo'] as num).toInt(),
        eta: j['eta'] as String? ?? '',
        breakdownKobo: DispatchFareBreakdown.fromJson(
          j['breakdown'] as Map<String, dynamic>,
        ),
      );
}

@immutable
class DispatchFareBreakdown {
  const DispatchFareBreakdown({
    required this.baseFareKobo,
    required this.distanceFareKobo,
    required this.sizeSurchargeKobo,
    required this.expressSurchargeKobo,
  });

  final int baseFareKobo;
  final int distanceFareKobo;
  final int sizeSurchargeKobo;
  final int expressSurchargeKobo;

  factory DispatchFareBreakdown.fromJson(Map<String, dynamic> j) =>
      DispatchFareBreakdown(
        baseFareKobo: (j['base_fare_kobo'] as num? ?? 0).toInt(),
        distanceFareKobo: (j['distance_fare_kobo'] as num? ?? 0).toInt(),
        sizeSurchargeKobo: (j['size_surcharge_kobo'] as num? ?? 0).toInt(),
        expressSurchargeKobo: (j['express_surcharge_kobo'] as num? ?? 0).toInt(),
      );
}

// ─── Dispatch booking ────────────────────────────────────────────────────────

@immutable
class DispatchBooking {
  const DispatchBooking({
    required this.id,
    required this.customerId,
    this.providerId,
    this.tripId,
    required this.pickupAddress,
    required this.pickupLat,
    required this.pickupLng,
    required this.dropoffAddress,
    required this.dropoffLat,
    required this.dropoffLng,
    this.distanceKm,
    required this.fareAmountKobo,
    this.currency = 'NGN',
    this.paymentMethod = 'wallet',
    this.paymentStatus = '',
    this.receiverName = '',
    this.receiverPhone = '',
    this.packageDesc = '',
    this.packageWeight,
    this.packageType = '',
    this.packageSize = '',
    this.isFragile = false,
    this.serviceTier = 'standard',
    required this.status,
    this.cancelReason,
    this.cancelledAt,
    this.completedAt,
    required this.createdAt,
  });

  final String id;
  final String customerId;
  final String? providerId;
  final String? tripId;
  final String pickupAddress;
  final double pickupLat;
  final double pickupLng;
  final String dropoffAddress;
  final double dropoffLat;
  final double dropoffLng;
  final double? distanceKm;
  final int fareAmountKobo;
  final String currency;
  final String paymentMethod;
  final String paymentStatus;
  final String receiverName;
  final String receiverPhone;
  final String packageDesc;
  final double? packageWeight;
  final String packageType;
  final String packageSize;
  final bool isFragile;
  final String serviceTier;
  final DispatchBookingStatus status;
  final String? cancelReason;
  final DateTime? cancelledAt;
  final DateTime? completedAt;
  final DateTime createdAt;

  int get displayFareKobo => fareAmountKobo;
  double get displayFareNaira => fareAmountKobo / 100;

  factory DispatchBooking.fromJson(Map<String, dynamic> j) => DispatchBooking(
    id: j['id'] as String,
    customerId: j['customer_id'] as String,
    providerId: (j['provider_id'] as String?)?.isEmpty ?? true
        ? null
        : j['provider_id'] as String,
    tripId: j['trip_id'] as String?,
    pickupAddress: j['pickup_address'] as String,
    pickupLat: (j['pickup_lat'] as num).toDouble(),
    pickupLng: (j['pickup_lng'] as num).toDouble(),
    dropoffAddress: j['dropoff_address'] as String,
    dropoffLat: (j['dropoff_lat'] as num).toDouble(),
    dropoffLng: (j['dropoff_lng'] as num).toDouble(),
    distanceKm: (j['distance_km'] as num?)?.toDouble(),
    fareAmountKobo: (j['fare_amount_kobo'] as num? ?? 0).toInt(),
    currency: j['currency'] as String? ?? 'NGN',
    paymentMethod: j['payment_method'] as String? ?? 'wallet',
    paymentStatus: j['payment_status'] as String? ?? '',
    receiverName: j['receiver_name'] as String? ?? '',
    receiverPhone: j['receiver_phone'] as String? ?? '',
    packageDesc: j['package_desc'] as String? ?? '',
    packageWeight: (j['package_weight'] as num?)?.toDouble(),
    packageType: j['package_type'] as String? ?? '',
    packageSize: j['package_size'] as String? ?? '',
    isFragile: j['is_fragile'] as bool? ?? false,
    serviceTier: j['service_tier'] as String? ?? 'standard',
    status: DispatchBookingStatus.fromString(j['status'] as String? ?? ''),
    cancelReason: (j['cancel_reason'] as String?)?.isEmpty ?? true
        ? null
        : j['cancel_reason'] as String,
    cancelledAt: j['cancelled_at'] != null
        ? DateTime.tryParse(j['cancelled_at'] as String)
        : null,
    completedAt: j['completed_at'] != null
        ? DateTime.tryParse(j['completed_at'] as String)
        : null,
    createdAt: DateTime.parse(j['created_at'] as String),
  );
}

// ─── Availability ─────────────────────────────────────────────────────────────

@immutable
class DispatchAvailabilityResult {
  const DispatchAvailabilityResult({
    required this.available,
    required this.count,
  });

  final bool available;
  final int count;

  factory DispatchAvailabilityResult.fromJson(Map<String, dynamic> j) =>
      DispatchAvailabilityResult(
        available: j['available'] as bool? ?? false,
        count: (j['count'] as num? ?? 0).toInt(),
      );
}

// ─── Booking review ─────────────────────────────────────────────────────────

@immutable
class DispatchBookingReview {
  const DispatchBookingReview({
    required this.id,
    required this.bookingId,
    required this.rating,
    this.reviewText = '',
    this.recommendsRider,
    required this.createdAt,
  });

  final String id;
  final String bookingId;
  final int rating;
  final String reviewText;
  final bool? recommendsRider;
  final DateTime createdAt;

  factory DispatchBookingReview.fromJson(Map<String, dynamic> j) =>
      DispatchBookingReview(
        id: j['id'] as String,
        bookingId: j['booking_id'] as String,
        rating: (j['rating'] as num).toInt(),
        reviewText: j['review_text'] as String? ?? '',
        recommendsRider: j['recommends_provider'] as bool?,
        createdAt: DateTime.parse(j['created_at'] as String),
      );
}

// ─── Rider public profile mapping ────────────────────────────────────────────

/// Maps the dispatch-service public rider profile
/// ({provider_id, full_name, profile_photo_url, avg_rating, total_trips})
/// onto the shared [ProviderSnapshot] used by the trip-sheet widgets.
ProviderSnapshot riderSnapshotFromPublicProfile(Map<String, dynamic> j) {
  final fullName = (j['full_name'] as String? ?? '').trim();
  final parts = fullName.split(RegExp(r'\s+'));
  final firstName = parts.isNotEmpty ? parts.first : '';
  final lastName = parts.length > 1 ? parts.sublist(1).join(' ') : '';
  return ProviderSnapshot(
    id: j['provider_id'] as String? ?? '',
    firstName: firstName.isEmpty ? 'Dispatch' : firstName,
    lastName: lastName.isEmpty && firstName.isEmpty ? 'Rider' : lastName,
    profilePhotoUrl: j['profile_photo_url'] as String?,
    phone: '',
    rating: (j['avg_rating'] as num? ?? 5.0).toDouble(),
    totalTrips: (j['total_trips'] as num? ?? 0).toInt(),
  );
}
