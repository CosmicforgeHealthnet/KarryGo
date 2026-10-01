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
      verificationStatus: _profile.verificationStatus,
      avgRating: _profile.avgRating,
      totalTrips: _profile.totalTrips,
      isActive: _profile.isActive,
      onboardingComplete:
          onboardingComplete ?? _profile.onboardingComplete,
      hasEmergencyContact:
          hasEmergencyContact ?? _profile.hasEmergencyContact,
      hasGuarantor: hasGuarantor ?? _profile.hasGuarantor,
      createdAt: _profile.createdAt,
    );
  }
}