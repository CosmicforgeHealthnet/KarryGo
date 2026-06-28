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
    this.email = '',
    this.profilePhotoUrl,
    this.rating = 0.0,
    this.totalTrips = 0,
    this.locationState = '',
    this.locationCity = '',
    this.language = '',
    this.serviceType = '',
    this.operationMode = '',
    this.driverLicenseNumber = '',
    this.licenseExpiryYear = '',
    this.licenseExpiryDate = '',
    this.govIdUrl = '',
    this.driverLicenseUrl = '',
    this.vehicleRegUrl = '',
    this.createdAt,
  });

  final String id;
  final String phone;
  final String onboardingStatus;
  final String firstName;
  final String lastName;
  final String email;
  final String? profilePhotoUrl;
  final double rating;
  final int totalTrips;
  final String locationState;
  final String locationCity;
  final String language;
  final String serviceType;
  final String operationMode;
  final String driverLicenseNumber;
  final String licenseExpiryYear;
  final String licenseExpiryDate;
  final String govIdUrl;
  final String driverLicenseUrl;
  final String vehicleRegUrl;
  final DateTime? createdAt;

  String get displayName {
    final name = '$firstName $lastName'.trim();
    return name.isEmpty ? phone : name;
  }

  bool get needsProfile => onboardingStatus == 'profile_required';

  bool get isVerified => onboardingStatus == 'complete';
  bool get isProcessing => onboardingStatus == 'pending_verification';

  /// Verification badge text shown on the profile header + verification screen.
  String get verificationLabel {
    if (isVerified) return 'Verified';
    if (isProcessing) return 'Processing';
    return 'Unverified';
  }

  /// Service line shown under the name (e.g. "Truck"). Falls back to "Truck"
  /// since this is the truck-provider app.
  String get displayServiceType {
    final s = serviceType.trim();
    if (s.isEmpty) return 'Truck';
    return s[0].toUpperCase() + s.substring(1);
  }

  /// "Driving with KarryGo . 1 year" tenure label derived from createdAt.
  String get tenureLabel {
    final created = createdAt;
    if (created == null) return 'New';
    final days = DateTime.now().difference(created).inDays;
    if (days < 30) return '$days ${days == 1 ? 'day' : 'days'}';
    if (days < 365) {
      final months = (days / 30).floor();
      return '$months ${months == 1 ? 'month' : 'months'}';
    }
    final years = (days / 365).floor();
    return '$years ${years == 1 ? 'year' : 'years'}';
  }

  TruckProvider copyWith({
    String? phone,
    String? onboardingStatus,
    String? firstName,
    String? lastName,
    String? email,
    String? profilePhotoUrl,
    double? rating,
    int? totalTrips,
    String? locationState,
    String? locationCity,
    String? language,
    String? serviceType,
    String? operationMode,
    String? driverLicenseNumber,
    String? licenseExpiryYear,
    String? licenseExpiryDate,
    String? govIdUrl,
    String? driverLicenseUrl,
    String? vehicleRegUrl,
    DateTime? createdAt,
  }) {
    return TruckProvider(
      id: id,
      phone: phone ?? this.phone,
      onboardingStatus: onboardingStatus ?? this.onboardingStatus,
      firstName: firstName ?? this.firstName,
      lastName: lastName ?? this.lastName,
      email: email ?? this.email,
      profilePhotoUrl: profilePhotoUrl ?? this.profilePhotoUrl,
      rating: rating ?? this.rating,
      totalTrips: totalTrips ?? this.totalTrips,
      locationState: locationState ?? this.locationState,
      locationCity: locationCity ?? this.locationCity,
      language: language ?? this.language,
      serviceType: serviceType ?? this.serviceType,
      operationMode: operationMode ?? this.operationMode,
      driverLicenseNumber: driverLicenseNumber ?? this.driverLicenseNumber,
      licenseExpiryYear: licenseExpiryYear ?? this.licenseExpiryYear,
      licenseExpiryDate: licenseExpiryDate ?? this.licenseExpiryDate,
      govIdUrl: govIdUrl ?? this.govIdUrl,
      driverLicenseUrl: driverLicenseUrl ?? this.driverLicenseUrl,
      vehicleRegUrl: vehicleRegUrl ?? this.vehicleRegUrl,
      createdAt: createdAt ?? this.createdAt,
    );
  }

  factory TruckProvider.fromJson(Map<String, dynamic> j) => TruckProvider(
        id: j['id'] as String,
        phone: j['phone'] as String? ?? '',
        onboardingStatus: j['onboarding_status'] as String? ?? 'profile_required',
        firstName: j['first_name'] as String? ?? '',
        lastName: j['last_name'] as String? ?? '',
        email: j['email'] as String? ?? '',
        profilePhotoUrl: j['profile_photo_url'] as String?,
        rating: (j['rating'] as num? ?? 0).toDouble(),
        totalTrips: (j['total_trips'] as num? ?? 0).toInt(),
        locationState: j['location_state'] as String? ?? '',
        locationCity: j['location_city'] as String? ?? '',
        language: j['language'] as String? ?? '',
        serviceType: j['service_type'] as String? ?? '',
        operationMode: j['operation_mode'] as String? ?? '',
        driverLicenseNumber: j['driver_license_number'] as String? ?? '',
        licenseExpiryYear: j['license_expiry_year'] as String? ?? '',
        licenseExpiryDate: j['license_expiry_date'] as String? ?? '',
        govIdUrl: j['gov_id_url'] as String? ?? '',
        driverLicenseUrl: j['driver_license_url'] as String? ?? '',
        vehicleRegUrl: j['vehicle_reg_url'] as String? ?? '',
        createdAt: j['created_at'] != null ? DateTime.tryParse(j['created_at'] as String) : null,
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

// Truck model (for assign-truck screen, listings, and the Truck Information screens)
@immutable
class ProviderTruck {
  const ProviderTruck({
    required this.id,
    required this.truckType,
    required this.capacityKg,
    this.plateNumber = '',
    this.status = 'active',
    this.year,
    this.make = '',
    this.model = '',
    this.color = '',
    this.licenseType = '',
    this.numberOfAxles = '',
    this.yearsOfExperience = '',
    this.goodsTypes = const [],
    this.hasInsurance = false,
  });

  final String id;
  final String truckType;
  final int capacityKg;
  final String plateNumber;
  final String status;
  final int? year;
  final String make;
  final String model;
  final String color;
  final String licenseType;
  final String numberOfAxles;
  final String yearsOfExperience;
  final List<String> goodsTypes;
  final bool hasInsurance;

  bool get isActive => status == 'active';

  String get displayType => truckTypeLabel(truckType);

  factory ProviderTruck.fromJson(Map<String, dynamic> j) => ProviderTruck(
        id: j['id'] as String,
        truckType: j['truck_type'] as String? ?? '',
        capacityKg: (j['capacity_kg'] as num? ?? 0).toInt(),
        plateNumber: j['plate_number'] as String? ?? '',
        status: j['status'] as String? ?? 'active',
        year: (j['year'] as num?)?.toInt(),
        make: j['make'] as String? ?? '',
        model: j['model'] as String? ?? '',
        color: j['color'] as String? ?? '',
        licenseType: j['license_type'] as String? ?? '',
        numberOfAxles: j['number_of_axles'] as String? ?? '',
        yearsOfExperience: j['years_of_experience'] as String? ?? '',
        goodsTypes: (j['goods_types'] as List?)?.map((e) => e.toString()).toList() ?? const [],
        hasInsurance: j['has_insurance'] as bool? ?? false,
      );
}

/// Truck-type slug → display label. Covers the original seeded slugs plus the
/// options in the Truck Information form ("Select Truck Type").
String truckTypeLabel(String slug) => switch (slug) {
      'flatbed' => 'Flatbed Truck',
      'container' => 'Container',
      'tipper' => 'Tipper',
      'van' => 'Van',
      'refrigerated' => 'Refrigerated Truck',
      'pickup' => 'Pickup Truck',
      'box' => 'Box Truck',
      'tanker' => 'Tanker',
      'trailer' => 'Trailer (Articulated Truck)',
      'dump' => 'Dump Truck',
      'lowbed' => 'Lowbed Truck',
      'crane' => 'Crane Truck',
      'other' => 'Other',
      _ => slug.isEmpty ? '' : slug,
    };

/// Truck-type options for the Truck Information form, matching the Figma
/// "Select Truck Type" dropdown. Value is the slug stored by the backend.
const providerTruckTypeOptions = <({String slug, String label})>[
  (slug: 'pickup', label: 'Pickup Truck'),
  (slug: 'box', label: 'Box Truck'),
  (slug: 'flatbed', label: 'Flatbed Truck'),
  (slug: 'refrigerated', label: 'Refrigerated Truck'),
  (slug: 'tanker', label: 'Tanker'),
  (slug: 'trailer', label: 'Trailer (Articulated Truck)'),
  (slug: 'dump', label: 'Dump Truck'),
  (slug: 'lowbed', label: 'Lowbed Truck'),
  (slug: 'crane', label: 'Crane Truck'),
  (slug: 'container', label: 'Container'),
  (slug: 'van', label: 'Van'),
  (slug: 'tipper', label: 'Tipper'),
  (slug: 'other', label: 'Other'),
];

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
