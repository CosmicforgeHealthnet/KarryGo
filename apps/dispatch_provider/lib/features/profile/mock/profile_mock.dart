import '../models/provider_profile_models.dart';

/// Stateful mock for the profile feature. Starts EMPTY — mirrors a fresh
/// signup so onboarding flows behave exactly like LIVE mode.
///
/// Every mutation (updateMe, saveGuarantor, saveEmergencyContact, avatar,
/// completeOnboarding) fills in the profile the same way the backend would.
class ProfileMock {
  ProfileMock._();

  static ProviderProfile _profile = _empty();
  static EmergencyContact? _emergency;
  static Guarantor? _guarantor;
  static ProviderStats _stats = ProviderStats(
    totalTrips: 0,
    avgRating: 0,
    ratingsCount: 0,
    completionRate: 0,
    isActive: false,
    verificationStatus: 'unverified',
  );

  // ── Getters ──
  static ProviderProfile get profile => _profile;
  static EmergencyContact? get emergencyContact => _emergency;
  static Guarantor? get guarantor => _guarantor;
  static ProviderStats get stats => _stats;

  static Map<String, dynamic> snapshot() => {
    'profile': {
      'provider_id': _profile.providerId,
      'support_id': _profile.supportId,
      'phone': _profile.phone,
      'full_name': _profile.fullName,
      'email': _profile.email,
      'state': _profile.state,
      'city': _profile.city,
      'country': _profile.country,
      'profile_photo_url': _profile.profilePhotoUrl,
      'operation_type': _profile.operationType,
      'verification_status': _profile.verificationStatus,
      'onboarding_complete': _profile.onboardingComplete,
      'has_emergency_contact': _profile.hasEmergencyContact,
      'has_guarantor': _profile.hasGuarantor,
    },
    if (_emergency != null)
      'emergency': {
        'id': _emergency!.id,
        'provider_id': _emergency!.providerId,
        'full_name': _emergency!.fullName,
        'phone': _emergency!.phone,
        'relationship': _emergency!.relationship,
      },
    if (_guarantor != null)
      'guarantor': {
        'id': _guarantor!.id,
        'provider_id': _guarantor!.providerId,
        'full_name': _guarantor!.fullName,
        'phone': _guarantor!.phone,
      },
  };

  static void restore(Map<String, dynamic> data) {
    reset();
    if (data['profile'] is Map) {
      _profile = ProviderProfile.fromJson(
        Map<String, dynamic>.from(data['profile'] as Map),
      );
    }
    if (data['emergency'] is Map) {
      _emergency = EmergencyContact.fromJson(
        Map<String, dynamic>.from(data['emergency'] as Map),
      );
    }
    if (data['guarantor'] is Map) {
      _guarantor = Guarantor.fromJson(
        Map<String, dynamic>.from(data['guarantor'] as Map),
      );
    }
  }

  // ── Mutations ──

  static ProviderProfile updateMe({
    String? fullName,
    String? email,
    String? state,
    String? city,
    String? profilePhotoUrl,
    String? operationType,
  }) {
    _profile = _copy(
      fullName: fullName,
      email: email,
      state: state,
      city: city,
      profilePhotoUrl: profilePhotoUrl,
      operationType: operationType,
    );
    return _profile;
  }

  static ProviderProfile setAvatar(String filePath) {
    _profile = _copy(profilePhotoUrl: filePath);
    return _profile;
  }

  static EmergencyContact saveEmergencyContact({
    required String fullName,
    required String phone,
    required String relationship,
  }) {
    _emergency = EmergencyContact(
      id: _emergency?.id ?? 'ec-1',
      providerId: _profile.providerId,
      fullName: fullName,
      phone: phone,
      relationship: relationship,
      createdAt: _emergency?.createdAt ?? DateTime.now(),
      updatedAt: DateTime.now(),
    );
    _profile = _copy(hasEmergencyContact: true);
    return _emergency!;
  }

  static Guarantor saveGuarantor({
    required String fullName,
    required String phone,
  }) {
    _guarantor = Guarantor(
      id: _guarantor?.id ?? 'g-1',
      providerId: _profile.providerId,
      fullName: fullName,
      phone: phone,
      createdAt: _guarantor?.createdAt ?? DateTime.now(),
      updatedAt: DateTime.now(),
    );
    _profile = _copy(hasGuarantor: true);
    return _guarantor!;
  }

  /// Marks onboarding complete — called by submitOnboarding in the controller.
  static ProviderProfile completeOnboarding() {
    _profile = _copy(onboardingComplete: true);
    return _profile;
  }

  static void seedDashboard() {
    updateMe(
      fullName: 'Tobi Adeyemi',
      email: 'rider@example.com',
      state: 'Lagos',
      city: 'Ikeja',
      operationType: 'own_vehicle',
    );
    saveEmergencyContact(
      fullName: 'Ada Adeyemi',
      phone: '+2348000000001',
      relationship: 'Sibling',
    );
    saveGuarantor(fullName: 'Kunle Adeyemi', phone: '+2348000000002');
    completeOnboarding();
    _profile = _copy(verificationStatus: 'verified');
  }

  static void deleteAccount() {
    // No-op in mock — controller clears local state after.
  }

  static void reset() {
    _profile = _empty();
    _emergency = null;
    _guarantor = null;
    _stats = ProviderStats(
      totalTrips: 0,
      avgRating: 0,
      ratingsCount: 0,
      completionRate: 0,
      isActive: false,
      verificationStatus: 'unverified',
    );
  }

  // ── Helpers ──

  static ProviderProfile _empty() => ProviderProfile(
    providerId: 'mock-provider-1',
    supportId: 'SUP-001',
    phone: '+234 803 555 1234',
    fullName: null,
    email: null,
    state: null,
    city: null,
    country: 'NG',
    profilePhotoUrl: null,
    operationType: null,
    verificationStatus: 'unverified',
    avgRating: 0,
    totalTrips: 0,
    isActive: false,
    onboardingComplete: false,
    hasEmergencyContact: false,
    hasGuarantor: false,
    createdAt: DateTime.now(),
  );

  static ProviderProfile _copy({
    String? fullName,
    String? email,
    String? state,
    String? city,
    String? profilePhotoUrl,
    String? operationType,
    bool? hasEmergencyContact,
    bool? hasGuarantor,
    bool? onboardingComplete,
    String? verificationStatus,
  }) {
    return ProviderProfile(
      providerId: _profile.providerId,
      supportId: _profile.supportId,
      phone: _profile.phone,
      fullName: fullName ?? _profile.fullName,
      email: email ?? _profile.email,
      state: state ?? _profile.state,
      city: city ?? _profile.city,
      country: _profile.country,
      profilePhotoUrl: profilePhotoUrl ?? _profile.profilePhotoUrl,
      operationType: operationType ?? _profile.operationType,
      verificationStatus: verificationStatus ?? _profile.verificationStatus,
      avgRating: _profile.avgRating,
      totalTrips: _profile.totalTrips,
      isActive: _profile.isActive,
      onboardingComplete: onboardingComplete ?? _profile.onboardingComplete,
      hasEmergencyContact: hasEmergencyContact ?? _profile.hasEmergencyContact,
      hasGuarantor: hasGuarantor ?? _profile.hasGuarantor,
      createdAt: _profile.createdAt,
    );
  }
}
