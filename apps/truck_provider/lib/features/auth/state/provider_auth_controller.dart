import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';
import 'package:image_picker/image_picker.dart';

import '../../media/data/media_upload_service.dart';
import '../data/provider_auth_api.dart';
import '../data/provider_session_store.dart';
import '../models/provider_auth_models.dart';

enum ProviderAuthStatus {
  checking,
  phoneEntry,
  otpVerification,
  accountTypeSelection,
  serviceTypeSelection,
  operationMode,
  personalInfo,
  driverDocuments,
  truckInfo,
  photoUpload,
  verificationPending,
  authenticated,
}

@immutable
class OnboardingFormData {
  const OnboardingFormData({
    this.accountType = '',
    this.serviceType = '',
    this.operationMode = '',
    this.firstName = '',
    this.lastName = '',
    this.locationState = '',
    this.locationCity = '',
    this.email = '',
    this.phone = '',
    this.govIdUrl = '',
    this.driverLicenseUrl = '',
    this.vehicleRegUrl = '',
    this.guarantorName = '',
    this.guarantorPhone = '',
    this.emergencyContactName = '',
    this.emergencyContactPhone = '',
    this.emergencyContactRelationship = '',
    this.profilePhotoUrl = '',
    this.photoAssetId = '',
    this.truckType = '',
    this.truckCapacityKg = '',
    this.truckPlateNumber = '',
    this.truckLicenseType = '',
    this.truckMake = '',
    this.truckModel = '',
    this.truckColor = '',
    this.truckNumberOfAxles = '',
    this.truckYearsOfExperience = '',
    this.truckGoodsTypes = const [],
    this.truckHasInsurance = false,
  });

  final String accountType;
  final String serviceType;
  final String operationMode;
  final String firstName;
  final String lastName;
  final String locationState;
  final String locationCity;
  final String email;
  final String phone;
  final String govIdUrl;
  final String driverLicenseUrl;
  final String vehicleRegUrl;
  final String guarantorName;
  final String guarantorPhone;
  final String emergencyContactName;
  final String emergencyContactPhone;
  final String emergencyContactRelationship;
  final String profilePhotoUrl;
  final String photoAssetId;
  final String truckType;
  final String truckCapacityKg;
  final String truckPlateNumber;
  final String truckLicenseType;
  final String truckMake;
  final String truckModel;
  final String truckColor;
  final String truckNumberOfAxles;
  final String truckYearsOfExperience;
  final List<String> truckGoodsTypes;
  final bool truckHasInsurance;

  OnboardingFormData copyWith({
    String? accountType,
    String? serviceType,
    String? operationMode,
    String? firstName,
    String? lastName,
    String? locationState,
    String? locationCity,
    String? email,
    String? phone,
    String? govIdUrl,
    String? driverLicenseUrl,
    String? vehicleRegUrl,
    String? guarantorName,
    String? guarantorPhone,
    String? emergencyContactName,
    String? emergencyContactPhone,
    String? emergencyContactRelationship,
    String? profilePhotoUrl,
    String? photoAssetId,
    String? truckType,
    String? truckCapacityKg,
    String? truckPlateNumber,
    String? truckLicenseType,
    String? truckMake,
    String? truckModel,
    String? truckColor,
    String? truckNumberOfAxles,
    String? truckYearsOfExperience,
    List<String>? truckGoodsTypes,
    bool? truckHasInsurance,
  }) {
    return OnboardingFormData(
      accountType: accountType ?? this.accountType,
      serviceType: serviceType ?? this.serviceType,
      operationMode: operationMode ?? this.operationMode,
      firstName: firstName ?? this.firstName,
      lastName: lastName ?? this.lastName,
      locationState: locationState ?? this.locationState,
      locationCity: locationCity ?? this.locationCity,
      email: email ?? this.email,
      phone: phone ?? this.phone,
      govIdUrl: govIdUrl ?? this.govIdUrl,
      driverLicenseUrl: driverLicenseUrl ?? this.driverLicenseUrl,
      vehicleRegUrl: vehicleRegUrl ?? this.vehicleRegUrl,
      guarantorName: guarantorName ?? this.guarantorName,
      guarantorPhone: guarantorPhone ?? this.guarantorPhone,
      emergencyContactName: emergencyContactName ?? this.emergencyContactName,
      emergencyContactPhone: emergencyContactPhone ?? this.emergencyContactPhone,
      emergencyContactRelationship: emergencyContactRelationship ?? this.emergencyContactRelationship,
      profilePhotoUrl: profilePhotoUrl ?? this.profilePhotoUrl,
      photoAssetId: photoAssetId ?? this.photoAssetId,
      truckType: truckType ?? this.truckType,
      truckCapacityKg: truckCapacityKg ?? this.truckCapacityKg,
      truckPlateNumber: truckPlateNumber ?? this.truckPlateNumber,
      truckLicenseType: truckLicenseType ?? this.truckLicenseType,
      truckMake: truckMake ?? this.truckMake,
      truckModel: truckModel ?? this.truckModel,
      truckColor: truckColor ?? this.truckColor,
      truckNumberOfAxles: truckNumberOfAxles ?? this.truckNumberOfAxles,
      truckYearsOfExperience: truckYearsOfExperience ?? this.truckYearsOfExperience,
      truckGoodsTypes: truckGoodsTypes ?? this.truckGoodsTypes,
      truckHasInsurance: truckHasInsurance ?? this.truckHasInsurance,
    );
  }
}

class ProviderAuthState {
  const ProviderAuthState({
    required this.status,
    this.isLoading = false,
    this.phone = '',
    this.email = '',
    this.identifierType = 'phone',
    this.challengeId,
    this.expiresIn = 0,
    this.debugOtp,
    this.session,
    this.error,
    this.onboarding = const OnboardingFormData(),
  });

  const ProviderAuthState.checking() : this(status: ProviderAuthStatus.checking);

  final ProviderAuthStatus status;
  final bool isLoading;
  final String phone;
  final String email;
  final String identifierType; // 'phone' or 'email'
  final String? challengeId;
  final int expiresIn;
  final String? debugOtp;
  final ProviderSession? session;
  final String? error;
  final OnboardingFormData onboarding;

  ProviderAuthState copyWith({
    ProviderAuthStatus? status,
    bool? isLoading,
    String? phone,
    String? email,
    String? identifierType,
    String? challengeId,
    int? expiresIn,
    String? debugOtp,
    ProviderSession? session,
    String? error,
    bool clearError = false,
    OnboardingFormData? onboarding,
  }) {
    return ProviderAuthState(
      status: status ?? this.status,
      isLoading: isLoading ?? this.isLoading,
      phone: phone ?? this.phone,
      email: email ?? this.email,
      identifierType: identifierType ?? this.identifierType,
      challengeId: challengeId ?? this.challengeId,
      expiresIn: expiresIn ?? this.expiresIn,
      debugOtp: debugOtp ?? this.debugOtp,
      session: session ?? this.session,
      error: clearError ? null : (error ?? this.error),
      onboarding: onboarding ?? this.onboarding,
    );
  }
}

class ProviderAuthController extends ChangeNotifier {
  ProviderAuthController({
    required ProviderAuthApi api,
    required ProviderSessionStore sessionStore,
    required MediaUploadService mediaUploadService,
  })  : _api = api,
        _store = sessionStore,
        _media = mediaUploadService;

  final ProviderAuthApi _api;
  final ProviderSessionStore _store;
  final MediaUploadService _media;

  ProviderAuthState _state = const ProviderAuthState.checking();
  ProviderAuthState get state => _state;

  void _emit(ProviderAuthState next) {
    _state = next;
    notifyListeners();
  }

  // ─── Initialization ──────────────────────────────────────────────────────────

  Future<void> initialize() async {
    final saved = await _store.loadSession();
    if (saved == null) {
      _emit(_state.copyWith(status: ProviderAuthStatus.phoneEntry, clearError: true));
      return;
    }
    try {
      final refreshed = await _api.refreshSession(refreshToken: saved.refreshToken);
      await _store.saveSession(refreshed);
      // A provider who only got as far as OTP (or quit partway through
      // onboarding) still has a valid session, but their profile is not yet
      // submitted (onboarding_status == 'profile_required'). Resume onboarding
      // from the start instead of dropping them on the dashboard.
      final needsOnboarding =
          refreshed.provider.onboardingStatus == 'profile_required';
      _emit(_state.copyWith(
        status: needsOnboarding
            ? ProviderAuthStatus.accountTypeSelection
            : ProviderAuthStatus.authenticated,
        session: refreshed,
      ));
    } catch (_) {
      await _store.clearSession();
      _emit(_state.copyWith(status: ProviderAuthStatus.phoneEntry, clearError: true));
    }
  }

  // ─── Auth ─────────────────────────────────────────────────────────────────────

  Future<void> startAuth({String phone = '', String email = ''}) async {
    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      final challenge = await _api.startAuth(phone: phone, email: email);
      _emit(_state.copyWith(
        status: ProviderAuthStatus.otpVerification,
        isLoading: false,
        phone: phone,
        email: email,
        identifierType: email.isNotEmpty ? 'email' : 'phone',
        challengeId: challenge.challengeId,
        expiresIn: challenge.expiresIn,
        debugOtp: challenge.debugOtp,
      ));
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _msg(e)));
    }
  }

  Future<void> verifyOtp(String code) async {
    final challengeId = _state.challengeId;
    if (challengeId == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      final session = await _api.verifyOtp(
        challengeId: challengeId,
        phone: _state.phone,
        email: _state.email,
        code: code,
      );
      await _store.saveSession(session);

      // Existing providers (complete or pending_verification) go straight to dashboard.
      // New providers (profile_required) go through onboarding.
      final needsOnboarding = session.provider.onboardingStatus == 'profile_required';
      _emit(_state.copyWith(
        status: needsOnboarding
            ? ProviderAuthStatus.accountTypeSelection
            : ProviderAuthStatus.authenticated,
        isLoading: false,
        session: session,
      ));
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _msg(e)));
    }
  }

  void backToPhoneEntry() {
    _emit(_state.copyWith(status: ProviderAuthStatus.phoneEntry, clearError: true));
  }

  // ─── Onboarding steps ────────────────────────────────────────────────────────

  void selectAccountType(String accountType) {
    _emit(_state.copyWith(onboarding: _state.onboarding.copyWith(accountType: accountType)));
  }

  void proceedToServiceType() {
    _emit(_state.copyWith(
      status: ProviderAuthStatus.serviceTypeSelection,
      clearError: true,
    ));
  }

  void selectServiceType(String serviceType) {
    _emit(_state.copyWith(onboarding: _state.onboarding.copyWith(serviceType: serviceType)));
  }

  void proceedToOperationMode() {
    _emit(_state.copyWith(status: ProviderAuthStatus.operationMode, clearError: true));
  }

  void selectOperationMode(String mode) {
    _emit(_state.copyWith(onboarding: _state.onboarding.copyWith(operationMode: mode)));
  }

  void proceedToPersonalInfo() {
    _emit(_state.copyWith(status: ProviderAuthStatus.personalInfo, clearError: true));
  }

  void savePersonalInfo({
    required String firstName,
    required String lastName,
    required String locationState,
    required String locationCity,
    String email = '',
    String phone = '',
    String govIdUrl = '',
  }) {
    _emit(_state.copyWith(
      status: ProviderAuthStatus.driverDocuments,
      clearError: true,
      onboarding: _state.onboarding.copyWith(
        firstName: firstName,
        lastName: lastName,
        locationState: locationState,
        locationCity: locationCity,
        email: email,
        phone: phone,
        govIdUrl: govIdUrl,
      ),
    ));
  }

  /// Checks whether the entered email/phone already belongs to another provider.
  /// Returns (false, false) if there is no active session. Throws [ApiException]
  /// on a network/validation failure so the caller can decide how to handle it.
  Future<({bool emailTaken, bool phoneTaken})> checkContactAvailability({
    String email = '',
    String phone = '',
  }) async {
    final session = _state.session;
    if (session == null) return (emailTaken: false, phoneTaken: false);
    return _api.checkContactAvailability(
      accessToken: session.accessToken,
      email: email,
      phone: phone,
    );
  }

  /// Uploads a picked document image to media-file-service and returns its
  /// stored public URL. Throws [ApiException] on failure so the caller can
  /// surface the error inline. Returns null if there is no active session.
  Future<String?> uploadDocument(XFile file, String purpose) async {
    final session = _state.session;
    if (session == null) return null;
    final result = await _media.uploadPicked(
      ownerId: session.provider.id,
      purpose: purpose,
      file: file,
    );
    return result.url;
  }

  void saveDriverDocuments({
    required String driverLicenseUrl,
    required String vehicleRegUrl,
    required String guarantorName,
    required String guarantorPhone,
    required String emergencyContactName,
    required String emergencyContactPhone,
    required String emergencyContactRelationship,
  }) {
    _emit(_state.copyWith(
      status: ProviderAuthStatus.truckInfo,
      clearError: true,
      onboarding: _state.onboarding.copyWith(
        driverLicenseUrl: driverLicenseUrl,
        vehicleRegUrl: vehicleRegUrl,
        guarantorName: guarantorName,
        guarantorPhone: guarantorPhone,
        emergencyContactName: emergencyContactName,
        emergencyContactPhone: emergencyContactPhone,
        emergencyContactRelationship: emergencyContactRelationship,
      ),
    ));
  }

  void saveTruckInfo({
    required String truckType,
    required String capacityKg,
    required String plateNumber,
    required String licenseType,
    required String make,
    required String model,
    required String color,
    required String numberOfAxles,
    required String yearsOfExperience,
    required List<String> goodsTypes,
    required bool hasInsurance,
  }) {
    _emit(_state.copyWith(
      status: ProviderAuthStatus.photoUpload,
      clearError: true,
      onboarding: _state.onboarding.copyWith(
        truckType: truckType,
        truckCapacityKg: capacityKg,
        truckPlateNumber: plateNumber,
        truckLicenseType: licenseType,
        truckMake: make,
        truckModel: model,
        truckColor: color,
        truckNumberOfAxles: numberOfAxles,
        truckYearsOfExperience: yearsOfExperience,
        truckGoodsTypes: goodsTypes,
        truckHasInsurance: hasInsurance,
      ),
    ));
  }

  /// Uploads the profile photo (if provided), then submits the full profile.
  Future<void> submitOnboarding(XFile? photo) async {
    final session = _state.session;
    if (session == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      var data = _state.onboarding;
      if (photo != null) {
        final uploaded = await _media.uploadPicked(
          ownerId: session.provider.id,
          purpose: MediaPurpose.profilePhoto,
          file: photo,
        );
        data = data.copyWith(
          profilePhotoUrl: uploaded.url,
          photoAssetId: uploaded.id,
        );
      }
      await _api.updateOnboardingProfile(
        accessToken: session.accessToken,
        firstName: data.firstName,
        lastName: data.lastName,
        email: data.email,
        phone: data.phone,
        locationState: data.locationState,
        locationCity: data.locationCity,
        operationMode: data.operationMode,
        serviceType: data.serviceType,
        govIdUrl: data.govIdUrl,
        driverLicenseUrl: data.driverLicenseUrl,
        vehicleRegUrl: data.vehicleRegUrl,
        guarantorName: data.guarantorName,
        guarantorPhone: data.guarantorPhone,
        emergencyContactName: data.emergencyContactName,
        emergencyContactPhone: data.emergencyContactPhone,
        emergencyContactRelationship: data.emergencyContactRelationship,
        profilePhotoUrl: data.profilePhotoUrl,
        photoAssetId: data.photoAssetId,
      );

      // Register the provider's truck so they have at least one active truck and
      // can go online. The backend availability gate requires this.
      if (data.truckType.isNotEmpty) {
        await _api.createTruck(
          accessToken: session.accessToken,
          body: {
            'truck_type': data.truckType,
            'capacity_kg': int.tryParse(data.truckCapacityKg) ?? 0,
            'plate_number': data.truckPlateNumber,
            'license_type': data.truckLicenseType,
            'make': data.truckMake,
            'model': data.truckModel,
            'color': data.truckColor,
            'number_of_axles': data.truckNumberOfAxles,
            'years_of_experience': data.truckYearsOfExperience,
            'goods_types': data.truckGoodsTypes,
            'has_insurance': data.truckHasInsurance,
          },
        );
      }

      _emit(_state.copyWith(
        status: ProviderAuthStatus.verificationPending,
        isLoading: false,
        onboarding: data,
      ));
    } catch (e) {
      // A duplicate email/phone is only caught here (final submit). Send the user
      // back to the personal-info step — where those fields live — so they can fix
      // it, instead of leaving them stranded on the photo step.
      final status = _identifierConflict(e)
          ? ProviderAuthStatus.personalInfo
          : _state.status;
      _emit(_state.copyWith(status: status, isLoading: false, error: _msg(e)));
    }
  }

  /// True when the error is a validation conflict on the email or phone field
  /// (i.e. the identifier is already in use), so the UI can route back to the
  /// step where that field is edited.
  bool _identifierConflict(Object e) {
    if (e is! ApiException) return false;
    return e.fields.any((f) => f.field == 'email' || f.field == 'phone');
  }

  void goToDashboard() {
    _emit(_state.copyWith(status: ProviderAuthStatus.authenticated, clearError: true));
  }

  /// Applies an updated provider (after a profile edit, photo change, phone
  /// change, or verification submit) to the active session so name/photo/phone
  /// changes propagate across the app, and persists the basics.
  void applyProviderUpdate(TruckProvider provider) {
    final session = _state.session;
    if (session == null) return;
    final updated = ProviderSession(
      accessToken: session.accessToken,
      refreshToken: session.refreshToken,
      provider: provider,
    );
    _emit(_state.copyWith(session: updated));
    _store.saveSession(updated);
  }

  // ─── Session ─────────────────────────────────────────────────────────────────

  Future<void> logout() async {
    final session = _state.session;
    if (session != null) {
      try {
        await _api.logout(accessToken: session.accessToken, refreshToken: session.refreshToken);
      } catch (_) {}
    }
    await _store.clearSession();
    _emit(const ProviderAuthState(status: ProviderAuthStatus.phoneEntry));
  }

  String _msg(Object e) {
    if (e is ApiException) {
      if (e.fields.isNotEmpty) return e.fields.map((f) => f.message).join(' ');
      return e.message;
    }
    return e.toString();
  }
}
