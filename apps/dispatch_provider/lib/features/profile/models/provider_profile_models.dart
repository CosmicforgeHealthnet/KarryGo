class ProviderProfile {
  final String providerId;
  final String supportId;
  final String phone;
  final String? fullName;
  final String? email;
  final String? state;
  final String? city;
  final String country;
  final String? profilePhotoUrl;
  final String? operationType;
  final String verificationStatus;
  final double avgRating;
  final int totalTrips;
  final bool isActive;
  final bool onboardingComplete;
  final bool hasEmergencyContact;
  final bool hasGuarantor;
  final DateTime? createdAt;

  ProviderProfile({
    required this.providerId,
    required this.supportId,
    required this.phone,
    this.fullName,
    this.email,
    this.state,
    this.city,
    required this.country,
    this.profilePhotoUrl,
    this.operationType,
    required this.verificationStatus,
    required this.avgRating,
    required this.totalTrips,
    required this.isActive,
    required this.onboardingComplete,
    required this.hasEmergencyContact,
    required this.hasGuarantor,
    this.createdAt,
  });

  factory ProviderProfile.fromJson(Map<String, dynamic> json) {
    return ProviderProfile(
      providerId: json['provider_id'] as String? ?? json['id'] as String? ?? '',
      supportId: json['support_id'] as String? ?? '',
      phone: json['phone'] as String? ?? '',
      fullName: json['full_name'] as String?,
      email: json['email'] as String?,
      state: json['state'] as String?,
      city: json['city'] as String?,
      country: json['country'] as String? ?? 'NG',
      profilePhotoUrl: json['profile_photo_url'] as String?,
      operationType: json['operation_type'] as String?,
      verificationStatus:
          json['verification_status'] as String? ?? 'unverified',
      avgRating: (json['avg_rating'] as num?)?.toDouble() ?? 0.0,
      totalTrips: (json['total_trips'] as num?)?.toInt() ?? 0,
      isActive: json['is_active'] as bool? ?? false,
      onboardingComplete: json['onboarding_complete'] as bool? ?? false,
      hasEmergencyContact: json['has_emergency_contact'] as bool? ?? false,
      hasGuarantor: json['has_guarantor'] as bool? ?? false,
      createdAt: json['created_at'] != null
          ? DateTime.tryParse(json['created_at'] as String)
          : null,
    );
  }
}

class EmergencyContact {
  final String id;
  final String providerId;
  final String fullName;
  final String phone;
  final String relationship;
  final DateTime? createdAt;
  final DateTime? updatedAt;

  EmergencyContact({
    required this.id,
    required this.providerId,
    required this.fullName,
    required this.phone,
    required this.relationship,
    this.createdAt,
    this.updatedAt,
  });

  factory EmergencyContact.fromJson(Map<String, dynamic> json) {
    return EmergencyContact(
      id: json['id'] as String? ?? '',
      providerId: json['provider_id'] as String? ?? '',
      fullName: json['full_name'] as String? ?? '',
      phone: json['phone'] as String? ?? '',
      relationship: json['relationship'] as String? ?? '',
      createdAt: json['created_at'] != null
          ? DateTime.tryParse(json['created_at'] as String)
          : null,
      updatedAt: json['updated_at'] != null
          ? DateTime.tryParse(json['updated_at'] as String)
          : null,
    );
  }
}

class Guarantor {
  final String id;
  final String providerId;
  final String fullName;
  final String phone;
  final DateTime? createdAt;
  final DateTime? updatedAt;

  Guarantor({
    required this.id,
    required this.providerId,
    required this.fullName,
    required this.phone,
    this.createdAt,
    this.updatedAt,
  });

  factory Guarantor.fromJson(Map<String, dynamic> json) {
    return Guarantor(
      id: json['id'] as String? ?? '',
      providerId: json['provider_id'] as String? ?? '',
      fullName: json['full_name'] as String? ?? '',
      phone: json['phone'] as String? ?? '',
      createdAt: json['created_at'] != null
          ? DateTime.tryParse(json['created_at'] as String)
          : null,
      updatedAt: json['updated_at'] != null
          ? DateTime.tryParse(json['updated_at'] as String)
          : null,
    );
  }
}

class ProviderStats {
  final int totalTrips;
  final double avgRating;
  final int ratingsCount;
  final double completionRate;
  final bool isActive;
  final String verificationStatus;

  ProviderStats({
    required this.totalTrips,
    required this.avgRating,
    required this.ratingsCount,
    required this.completionRate,
    required this.isActive,
    required this.verificationStatus,
  });

  factory ProviderStats.fromJson(Map<String, dynamic> json) {
    return ProviderStats(
      totalTrips: (json['total_trips'] as num?)?.toInt() ?? 0,
      avgRating: (json['avg_rating'] as num?)?.toDouble() ?? 0.0,
      ratingsCount: (json['ratings_count'] as num?)?.toInt() ?? 0,
      completionRate: (json['completion_rate'] as num?)?.toDouble() ?? 0.0,
      isActive: json['is_active'] as bool? ?? false,
      verificationStatus:
          json['verification_status'] as String? ?? 'unverified',
    );
  }
}

class PublicProviderProfile {
  final String providerId;
  final String? fullName;
  final String? profilePhotoUrl;
  final String verificationStatus;
  final double avgRating;
  final int totalTrips;

  PublicProviderProfile({
    required this.providerId,
    this.fullName,
    this.profilePhotoUrl,
    required this.verificationStatus,
    required this.avgRating,
    required this.totalTrips,
  });

  factory PublicProviderProfile.fromJson(Map<String, dynamic> json) {
    return PublicProviderProfile(
      providerId: json['provider_id'] as String? ?? '',
      fullName: json['full_name'] as String?,
      profilePhotoUrl: json['profile_photo_url'] as String?,
      verificationStatus:
          json['verification_status'] as String? ?? 'unverified',
      avgRating: (json['avg_rating'] as num?)?.toDouble() ?? 0.0,
      totalTrips: (json['total_trips'] as num?)?.toInt() ?? 0,
    );
  }
}
