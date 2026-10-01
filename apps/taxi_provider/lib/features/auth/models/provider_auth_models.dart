import 'package:flutter/foundation.dart';

/// One issued OTP challenge. [debugOtp] is only ever populated when the
/// service runs with TAXI_OTP_DEBUG=true, and is shown only in debug builds.
@immutable
class OtpChallenge {
  const OtpChallenge({
    required this.challengeId,
    required this.expiresIn,
    this.debugOtp,
  });

  final String challengeId;
  final int expiresIn;
  final String? debugOtp;

  factory OtpChallenge.fromJson(Map<String, dynamic> json) {
    final debug = (json['debug_otp'] as String?) ?? '';
    return OtpChallenge(
      challengeId: (json['challenge_id'] as String?) ?? '',
      expiresIn: (json['expires_in'] as num?)?.toInt() ?? 0,
      debugOtp: debug.isEmpty ? null : debug,
    );
  }
}

/// The driver's own profile, as returned by verify/refresh and
/// `GET /provider/profile`.
@immutable
class TaxiProvider {
  const TaxiProvider({
    required this.id,
    required this.phone,
    required this.firstName,
    required this.lastName,
    required this.accountStatus,
    required this.onboardingStatus,
    required this.acceptsParcels,
    this.email = '',
    this.rating = 0,
    this.totalTrips = 0,
    this.profilePhotoUrl,
  });

  final String id;
  final String phone;
  final String firstName;
  final String lastName;
  final String accountStatus;
  final String onboardingStatus;

  /// Whether this driver may be matched to package deliveries. Off by
  /// default; toggled from the profile screen.
  final bool acceptsParcels;

  final String email;
  final double rating;
  final int totalTrips;
  final String? profilePhotoUrl;

  String get fullName {
    final name = '$firstName $lastName'.trim();
    return name.isEmpty ? 'Driver' : name;
  }

  String get initial =>
      fullName.isEmpty ? 'D' : fullName.substring(0, 1).toUpperCase();

  bool get isActive => accountStatus == 'active';

  factory TaxiProvider.fromJson(Map<String, dynamic> json) {
    return TaxiProvider(
      id: (json['id'] as String?) ?? '',
      phone: (json['phone'] as String?) ?? '',
      firstName: (json['first_name'] as String?) ?? '',
      lastName: (json['last_name'] as String?) ?? '',
      accountStatus: (json['account_status'] as String?) ?? '',
      onboardingStatus: (json['onboarding_status'] as String?) ?? '',
      acceptsParcels: json['accepts_parcels'] == true,
      email: (json['email'] as String?) ?? '',
      rating: (json['rating'] as num?)?.toDouble() ?? 0,
      totalTrips: (json['total_trips'] as num?)?.toInt() ?? 0,
      profilePhotoUrl: json['profile_photo_url'] as String?,
    );
  }

  TaxiProvider copyWith({bool? acceptsParcels}) {
    return TaxiProvider(
      id: id,
      phone: phone,
      firstName: firstName,
      lastName: lastName,
      accountStatus: accountStatus,
      onboardingStatus: onboardingStatus,
      acceptsParcels: acceptsParcels ?? this.acceptsParcels,
      email: email,
      rating: rating,
      totalTrips: totalTrips,
      profilePhotoUrl: profilePhotoUrl,
    );
  }
}

/// A signed-in driver session.
@immutable
class ProviderSession {
  const ProviderSession({
    required this.accessToken,
    required this.refreshToken,
    required this.provider,
  });

  final String accessToken;
  final String refreshToken;
  final TaxiProvider provider;

  factory ProviderSession.fromJson(Map<String, dynamic> json) {
    final rawProvider = json['provider'];
    return ProviderSession(
      accessToken: (json['access_token'] as String?) ?? '',
      refreshToken: (json['refresh_token'] as String?) ?? '',
      provider: TaxiProvider.fromJson(
        rawProvider is Map ? Map<String, dynamic>.from(rawProvider) : const {},
      ),
    );
  }

  ProviderSession withProvider(TaxiProvider next) => ProviderSession(
    accessToken: accessToken,
    refreshToken: refreshToken,
    provider: next,
  );
}

/// One of the driver's registered cars.
@immutable
class ProviderVehicle {
  const ProviderVehicle({
    required this.id,
    required this.vehicleClass,
    required this.plateNumber,
    required this.seats,
    required this.status,
    this.make = '',
    this.model = '',
    this.color = '',
  });

  final String id;
  final String vehicleClass;
  final String plateNumber;
  final int seats;
  final String status;
  final String make;
  final String model;
  final String color;

  bool get isActive => status == 'active';

  String get descriptor {
    final parts = [color, make, model].where((p) => p.trim().isNotEmpty);
    return parts.isEmpty ? vehicleClass : parts.join(' ');
  }

  factory ProviderVehicle.fromJson(Map<String, dynamic> json) {
    return ProviderVehicle(
      id: (json['id'] as String?) ?? '',
      vehicleClass: (json['vehicle_class'] as String?) ?? '',
      plateNumber: (json['plate_number'] as String?) ?? '',
      seats: (json['seats'] as num?)?.toInt() ?? 0,
      status: (json['status'] as String?) ?? '',
      make: (json['make'] as String?) ?? '',
      model: (json['model'] as String?) ?? '',
      color: (json['color'] as String?) ?? '',
    );
  }
}
