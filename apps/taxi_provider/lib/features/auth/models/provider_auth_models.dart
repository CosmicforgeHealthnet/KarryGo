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
    this.identityVerificationStatus = '',
    this.vehicleVerificationStatus = '',
    this.vehicleOperationalStatus = '',
    this.documentComplianceStatus = '',
    this.locationState = '',
    this.locationCity = '',
    this.driverLicenseNumber = '',
    this.licenseExpiryDate = '',
    this.govIdUrl = '',
    this.driverLicenseUrl = '',
    this.vehicleRegUrl = '',
    this.guarantorName = '',
    this.guarantorPhone = '',
    this.emergencyContactName = '',
    this.emergencyContactPhone = '',
    this.emergencyContactRelationship = '',
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

  /// Review statuses. A driver can go online only once these pass.
  final String identityVerificationStatus;
  final String vehicleVerificationStatus;
  final String vehicleOperationalStatus;
  final String documentComplianceStatus;

  final String locationState;
  final String locationCity;
  final String driverLicenseNumber;
  final String licenseExpiryDate;
  final String govIdUrl;
  final String driverLicenseUrl;
  final String vehicleRegUrl;
  final String guarantorName;
  final String guarantorPhone;
  final String emergencyContactName;
  final String emergencyContactPhone;
  final String emergencyContactRelationship;

  String get fullName {
    final name = '$firstName $lastName'.trim();
    return name.isEmpty ? 'Driver' : name;
  }

  String get initial =>
      fullName.isEmpty ? 'D' : fullName.substring(0, 1).toUpperCase();

  bool get isActive => accountStatus == 'active';

  /// A new driver who has not finished setting up their account.
  bool get needsOnboarding => onboardingStatus == 'profile_required';

  factory TaxiProvider.fromJson(Map<String, dynamic> json) {
    String str(String key) => (json[key] as String?) ?? '';
    return TaxiProvider(
      id: str('id'),
      phone: str('phone'),
      firstName: str('first_name'),
      lastName: str('last_name'),
      accountStatus: str('account_status'),
      onboardingStatus: str('onboarding_status'),
      acceptsParcels: json['accepts_parcels'] == true,
      email: str('email'),
      rating: (json['rating'] as num?)?.toDouble() ?? 0,
      totalTrips: (json['total_trips'] as num?)?.toInt() ?? 0,
      profilePhotoUrl: json['profile_photo_url'] as String?,
      identityVerificationStatus: str('identity_verification_status'),
      vehicleVerificationStatus: str('vehicle_verification_status'),
      vehicleOperationalStatus: str('vehicle_operational_status'),
      documentComplianceStatus: str('document_compliance_status'),
      locationState: str('location_state'),
      locationCity: str('location_city'),
      driverLicenseNumber: str('driver_license_number'),
      licenseExpiryDate: str('license_expiry_date'),
      govIdUrl: str('gov_id_url'),
      driverLicenseUrl: str('driver_license_url'),
      vehicleRegUrl: str('vehicle_reg_url'),
      guarantorName: str('guarantor_name'),
      guarantorPhone: str('guarantor_phone'),
      emergencyContactName: str('emergency_contact_name'),
      emergencyContactPhone: str('emergency_contact_phone'),
      emergencyContactRelationship: str('emergency_contact_relationship'),
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
      identityVerificationStatus: identityVerificationStatus,
      vehicleVerificationStatus: vehicleVerificationStatus,
      vehicleOperationalStatus: vehicleOperationalStatus,
      documentComplianceStatus: documentComplianceStatus,
      locationState: locationState,
      locationCity: locationCity,
      driverLicenseNumber: driverLicenseNumber,
      licenseExpiryDate: licenseExpiryDate,
      govIdUrl: govIdUrl,
      driverLicenseUrl: driverLicenseUrl,
      vehicleRegUrl: vehicleRegUrl,
      guarantorName: guarantorName,
      guarantorPhone: guarantorPhone,
      emergencyContactName: emergencyContactName,
      emergencyContactPhone: emergencyContactPhone,
      emergencyContactRelationship: emergencyContactRelationship,
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
