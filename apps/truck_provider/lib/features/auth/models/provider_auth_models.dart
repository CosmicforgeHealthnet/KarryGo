import 'package:flutter/foundation.dart';

@immutable
class ProviderSession {
  const ProviderSession({
    required this.accessToken,
    required this.refreshToken,
    required this.provider,
  });

  final String accessToken;
  final String refreshToken;
  final TruckProvider provider;

  factory ProviderSession.fromJson(Map<String, dynamic> j) => ProviderSession(
        accessToken: j['access_token'] as String,
        refreshToken: j['refresh_token'] as String,
        provider: TruckProvider.fromJson(j['provider'] as Map<String, dynamic>),
      );
}

@immutable
class TruckProvider {
  const TruckProvider({
    required this.id,
    required this.phone,
    required this.onboardingStatus,
    this.firstName = '',
    this.lastName = '',
    this.profilePhotoUrl,
    this.rating = 0.0,
    this.totalTrips = 0,
  });

  final String id;
  final String phone;
  final String onboardingStatus;
  final String firstName;
  final String lastName;
  final String? profilePhotoUrl;
  final double rating;
  final int totalTrips;

  String get displayName {
    final name = '$firstName $lastName'.trim();
    return name.isEmpty ? phone : name;
  }

  bool get needsProfile => onboardingStatus == 'profile_required';

  factory TruckProvider.fromJson(Map<String, dynamic> j) => TruckProvider(
        id: j['id'] as String,
        phone: j['phone'] as String? ?? '',
        onboardingStatus: j['onboarding_status'] as String? ?? 'profile_required',
        firstName: j['first_name'] as String? ?? '',
        lastName: j['last_name'] as String? ?? '',
        profilePhotoUrl: j['profile_photo_url'] as String?,
        rating: (j['rating'] as num? ?? 0).toDouble(),
        totalTrips: (j['total_trips'] as num? ?? 0).toInt(),
      );
}

@immutable
class OtpChallenge {
  const OtpChallenge({required this.challengeId, this.expiresIn = 300, this.debugOtp});
  final String challengeId;
  final int expiresIn;
  final String? debugOtp;

  factory OtpChallenge.fromJson(Map<String, dynamic> j) => OtpChallenge(
        challengeId: j['challenge_id'] as String,
        expiresIn: (j['expires_in'] as num? ?? 300).toInt(),
        debugOtp: j['debug_otp'] as String?,
      );
}

// Truck model (for assign-truck screen and listings)
@immutable
class ProviderTruck {
  const ProviderTruck({
    required this.id,
    required this.truckType,
    required this.capacityKg,
    this.plateNumber = '',
    this.status = 'active',
  });

  final String id;
  final String truckType;
  final int capacityKg;
  final String plateNumber;
  final String status;

  bool get isActive => status == 'active';

  String get displayType => switch (truckType) {
        'flatbed' => 'Flatbed',
        'container' => 'Container',
        'tipper' => 'Tipper',
        'van' => 'Van',
        'refrigerated' => 'Refrigerated',
        _ => truckType,
      };

  factory ProviderTruck.fromJson(Map<String, dynamic> j) => ProviderTruck(
        id: j['id'] as String,
        truckType: j['truck_type'] as String? ?? '',
        capacityKg: (j['capacity_kg'] as num? ?? 0).toInt(),
        plateNumber: j['plate_number'] as String? ?? '',
        status: j['status'] as String? ?? 'active',
      );
}

// Provider booking model (incoming request / active trip)
@immutable
class ProviderBooking {
  const ProviderBooking({
    required this.id,
    required this.pickupAddress,
    required this.pickupLat,
    required this.pickupLng,
    required this.dropoffAddress,
    required this.dropoffLat,
    required this.dropoffLng,
    required this.cargoType,
    required this.cargoWeightKg,
    this.cargoDescription = '',
    this.requiresHelpers = false,
    this.helperCount = 0,
    this.distanceKm,
    this.fareEstimateKobo,
    this.fareFinalKobo,
    this.receiverName = '',
    this.receiverPhone = '',
    this.packageContent = '',
    this.weightCategory = '',
    this.packageSize = '',
    this.preferredTruckType = '',
    this.isFragile = false,
    required this.status,
    required this.createdAt,
  });

  final String id;
  final String pickupAddress;
  final double pickupLat;
  final double pickupLng;
  final String dropoffAddress;
  final double dropoffLat;
  final double dropoffLng;
  final String cargoType;
  final int cargoWeightKg;
  final String cargoDescription;
  final bool requiresHelpers;
  final int helperCount;
  final double? distanceKm;
  final int? fareEstimateKobo;
  final int? fareFinalKobo;
  final String receiverName;
  final String receiverPhone;
  final String packageContent;
  final String weightCategory;
  final String packageSize;
  final String preferredTruckType;
  final bool isFragile;
  final String status;
  final DateTime createdAt;

  double get fareEstimateNaira => (fareEstimateKobo ?? 0) / 100;

  // Short display reference (first 8 chars of UUID)
  String get shortId => id.length >= 8 ? id.substring(0, 8).toUpperCase() : id.toUpperCase();

  // Headline person name — the package receiver, or a fallback.
  String get displayName => receiverName.trim().isEmpty ? 'Customer' : receiverName.trim();

  factory ProviderBooking.fromJson(Map<String, dynamic> j) => ProviderBooking(
        id: j['id'] as String,
        pickupAddress: j['pickup_address'] as String,
        pickupLat: (j['pickup_lat'] as num).toDouble(),
        pickupLng: (j['pickup_lng'] as num).toDouble(),
        dropoffAddress: j['dropoff_address'] as String,
        dropoffLat: (j['dropoff_lat'] as num).toDouble(),
        dropoffLng: (j['dropoff_lng'] as num).toDouble(),
        cargoType: j['cargo_type'] as String,
        cargoWeightKg: (j['cargo_weight_kg'] as num).toInt(),
        cargoDescription: j['cargo_description'] as String? ?? '',
        requiresHelpers: j['requires_helpers'] as bool? ?? false,
        helperCount: (j['helper_count'] as num? ?? 0).toInt(),
        distanceKm: (j['distance_km'] as num?)?.toDouble(),
        fareEstimateKobo: (j['fare_estimate_kobo'] as num?)?.toInt(),
        fareFinalKobo: (j['fare_final_kobo'] as num?)?.toInt(),
        receiverName: j['receiver_name'] as String? ?? '',
        receiverPhone: j['receiver_phone'] as String? ?? '',
        packageContent: j['package_content'] as String? ?? '',
        weightCategory: j['weight_category'] as String? ?? '',
        packageSize: j['package_size'] as String? ?? '',
        preferredTruckType: j['preferred_truck_type'] as String? ?? '',
        isFragile: j['is_fragile'] as bool? ?? false,
        status: j['status'] as String,
        createdAt: DateTime.parse(j['created_at'] as String),
      );
}
