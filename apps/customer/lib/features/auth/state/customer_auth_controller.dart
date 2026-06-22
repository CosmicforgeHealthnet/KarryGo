import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';
import 'package:image_picker/image_picker.dart';

import '../../media/data/media_upload_service.dart';
import '../data/customer_auth_api.dart';
import '../data/customer_session_store.dart';
import '../models/customer_auth_models.dart';

enum CustomerAuthStatus {
  checking,
  onboarding,
  phoneEntry,
  otpVerification,
  serviceChoice,
  profileDetails,
  photoUpload,
  allSet,
  authenticated,
}

enum CustomerAuthIdentifierType { phone, email }

class CustomerAuthState {
  const CustomerAuthState({
    required this.status,
    this.isLoading = false,
    this.identifierType = CustomerAuthIdentifierType.phone,
    this.phone = '',
    this.email = '',
    this.challengeId,
    this.otpExpiresIn = 0,
    this.debugOtp,
    this.session,
    this.customer,
    this.selectedService,
    this.profileName = '',
    this.profileEmail = '',
    this.profilePhotoUrl,
    this.profilePhotoAssetId,
    this.error,
  });

  const CustomerAuthState.checking()
    : this(status: CustomerAuthStatus.checking);

  final CustomerAuthStatus status;
  final bool isLoading;
  final CustomerAuthIdentifierType identifierType;
  final String phone;
  final String email;
  final String? challengeId;
  final int otpExpiresIn;
  final String? debugOtp;
  final CustomerSession? session;
  final CustomerProfile? customer;
  final String? selectedService;
  final String profileName;
  final String profileEmail;
  final String? profilePhotoUrl;
  final String? profilePhotoAssetId;
  final ApiException? error;

  bool get hasProfilePhoto => profilePhotoUrl != null;

  String get activeIdentifier {
    return switch (identifierType) {
      CustomerAuthIdentifierType.phone => phone,
      CustomerAuthIdentifierType.email => email,
    };
  }

  CustomerAuthState copyWith({
    CustomerAuthStatus? status,
    bool? isLoading,
    CustomerAuthIdentifierType? identifierType,
    String? phone,
    String? email,
    String? challengeId,
    int? otpExpiresIn,
    String? debugOtp,
    CustomerSession? session,
    CustomerProfile? customer,
    String? selectedService,
    String? profileName,
    String? profileEmail,
    String? profilePhotoUrl,
    String? profilePhotoAssetId,
    ApiException? error,
    bool clearChallenge = false,
    bool clearDebugOtp = false,
    bool clearSession = false,
    bool clearCustomer = false,
    bool clearSelectedService = false,
    bool clearError = false,
  }) {
    return CustomerAuthState(
      status: status ?? this.status,
      isLoading: isLoading ?? this.isLoading,
      identifierType: identifierType ?? this.identifierType,
      phone: phone ?? this.phone,
      email: email ?? this.email,
      challengeId: clearChallenge ? null : challengeId ?? this.challengeId,
      otpExpiresIn: otpExpiresIn ?? this.otpExpiresIn,
      debugOtp: clearDebugOtp ? null : debugOtp ?? this.debugOtp,
      session: clearSession ? null : session ?? this.session,
      customer: clearCustomer ? null : customer ?? this.customer,
      selectedService: clearSelectedService
          ? null
          : selectedService ?? this.selectedService,
      profileName: profileName ?? this.profileName,
      profileEmail: profileEmail ?? this.profileEmail,
      profilePhotoUrl: profilePhotoUrl ?? this.profilePhotoUrl,
      profilePhotoAssetId: profilePhotoAssetId ?? this.profilePhotoAssetId,
      error: clearError ? null : error ?? this.error,
    );
  }
}

class CustomerAuthController extends ChangeNotifier {
  CustomerAuthController({
    required CustomerAuthApi api,
    required CustomerSessionStore sessionStore,
    required MediaUploadService mediaUploadService,
  }) : _api = api,
       _sessionStore = sessionStore,
       _mediaUploadService = mediaUploadService;

  final CustomerAuthApi _api;
  final CustomerSessionStore _sessionStore;
  final MediaUploadService _mediaUploadService;

  CustomerAuthState _state = const CustomerAuthState.checking();
  bool _handlingAuthFailure = false;

  CustomerAuthState get state => _state;

  /// Forces the controller into [status] without going through the network
  /// flow. Exists only so tests can exercise state-dependent behaviour such as
  /// [handleAuthFailure].
  @visibleForTesting
  void debugSetStatus(CustomerAuthStatus status) {
    _setState(_state.copyWith(status: status));
  }

  Future<void> initialize() async {
    _setState(const CustomerAuthState.checking());

    try {
      final savedSession = await _sessionStore.readSession();
      if (savedSession == null) {
        _setState(
          const CustomerAuthState(status: CustomerAuthStatus.onboarding),
        );
        return;
      }

      final activeSession = savedSession.isAccessTokenExpired
          ? await _refreshSession(savedSession)
          : savedSession;

      final customer = await _api.me(accessToken: activeSession.accessToken);
      final session = activeSession.copyWith(customer: customer);
      await _sessionStore.saveSession(session);
      _setAuthenticatedSession(session);
    } on ApiException catch (error) {
      if (error.isAuthFailure) {
        await _sessionStore.clearSession();
      }
      _setState(
        CustomerAuthState(
          status: CustomerAuthStatus.onboarding,
          error: error.isAuthFailure ? null : error,
        ),
      );
    } catch (error) {
      _setState(
        CustomerAuthState(
          status: CustomerAuthStatus.onboarding,
          error: ApiException.network(error),
        ),
      );
    }
  }

  /// Reacts to an authentication failure (expired or revoked session) detected
  /// on any bearer-authenticated API call while the user is active in the app.
  /// Clears the local session and routes back to phone entry so the user can
  /// sign in again.
  ///
  /// No-op during the pre-authentication flow (onboarding/phone/OTP), where a
  /// 401 is part of the normal challenge handshake and handled inline.
  Future<void> handleAuthFailure() async {
    if (_handlingAuthFailure) return;

    switch (_state.status) {
      case CustomerAuthStatus.checking:
      case CustomerAuthStatus.onboarding:
      case CustomerAuthStatus.phoneEntry:
      case CustomerAuthStatus.otpVerification:
        return;
      case CustomerAuthStatus.serviceChoice:
      case CustomerAuthStatus.profileDetails:
      case CustomerAuthStatus.photoUpload:
      case CustomerAuthStatus.allSet:
      case CustomerAuthStatus.authenticated:
        break;
    }

    _handlingAuthFailure = true;
    try {
      await _sessionStore.clearSession();
      _setState(
        const CustomerAuthState(
          status: CustomerAuthStatus.phoneEntry,
          error: ApiException(
            code: ApiErrorCode.unauthorized,
            message: 'Your session has expired. Please sign in again.',
          ),
        ),
      );
    } finally {
      _handlingAuthFailure = false;
    }
  }

  void completeOnboarding() {
    _setState(
      _state.copyWith(status: CustomerAuthStatus.phoneEntry, clearError: true),
    );
  }

  void backToPhoneEntry() {
    _setState(
      _state.copyWith(
        status: CustomerAuthStatus.phoneEntry,
        clearChallenge: true,
        clearDebugOtp: true,
        clearError: true,
      ),
    );
  }

  void dismissError() {
    _setState(_state.copyWith(clearError: true));
  }

  Future<void> startAuth({
    required CustomerAuthIdentifierType type,
    required String value,
  }) async {
    final normalizedValue = value.trim();
    if (normalizedValue.isEmpty) {
      _setState(
        _state.copyWith(
          status: CustomerAuthStatus.phoneEntry,
          identifierType: type,
          error: ApiException(
            code: ApiErrorCode.validationFailed,
            message: type == CustomerAuthIdentifierType.email
                ? 'Enter your email address to continue.'
                : 'Enter your phone number to continue.',
            fields: [
              ApiFieldError(
                field: type == CustomerAuthIdentifierType.email
                    ? 'email'
                    : 'phone',
                message: type == CustomerAuthIdentifierType.email
                    ? 'Email address is required.'
                    : 'Phone number is required.',
              ),
            ],
          ),
        ),
      );
      return;
    }

    _setState(
      _state.copyWith(
        status: CustomerAuthStatus.phoneEntry,
        identifierType: type,
        phone: type == CustomerAuthIdentifierType.phone
            ? normalizedValue
            : _state.phone,
        email: type == CustomerAuthIdentifierType.email
            ? normalizedValue
            : _state.email,
        profileEmail: type == CustomerAuthIdentifierType.email
            ? normalizedValue
            : _state.profileEmail,
        isLoading: true,
        clearError: true,
        clearDebugOtp: true,
      ),
    );

    try {
      final result = await _api.startAuth(
        phone: type == CustomerAuthIdentifierType.phone ? normalizedValue : null,
        email: type == CustomerAuthIdentifierType.email ? normalizedValue : null,
      );
      _setState(
        _state.copyWith(
          status: CustomerAuthStatus.otpVerification,
          isLoading: false,
          challengeId: result.challengeId,
          otpExpiresIn: result.expiresIn,
          debugOtp: result.debugOtp,
          clearError: true,
        ),
      );
    } on ApiException catch (error) {
      _setState(_state.copyWith(isLoading: false, error: error));
    }
  }

  Future<void> resendOtp() async {
    if (_state.activeIdentifier.isEmpty) {
      backToPhoneEntry();
      return;
    }
    await startAuth(type: _state.identifierType, value: _state.activeIdentifier);
  }

  Future<void> verifyOtp(String otp) async {
    final code = otp.trim();
    final challengeId = _state.challengeId;
    if (code.length < 4 || challengeId == null || challengeId.isEmpty) {
      _setState(
        _state.copyWith(
          error: const ApiException(
            code: ApiErrorCode.validationFailed,
            message: 'Enter the verification code we sent you.',
            fields: [
              ApiFieldError(
                field: 'otp',
                message: 'Verification code is required.'
              ),
            ],
          ),
        ),
      );
      return;
    }

    _setState(_state.copyWith(isLoading: true, clearError: true));

    try {
      final deviceId = await _sessionStore.readOrCreateDeviceId();
      final result = await _api.verifyAuth(
        phone: _state.identifierType == CustomerAuthIdentifierType.phone
            ? _state.phone
            : null,
        email: _state.identifierType == CustomerAuthIdentifierType.email
            ? _state.email
            : null,
        otp: code,
        challengeId: challengeId,
        deviceId: deviceId,
      );
      final session = result.toSession(DateTime.now());
      await _sessionStore.saveSession(session);
      _setAuthenticatedSession(session);
    } on ApiException catch (error) {
      _setState(_state.copyWith(isLoading: false, error: error));
    }
  }

  void selectServiceChoice(String serviceId) {
    _setState(
      _state.copyWith(
        status: CustomerAuthStatus.profileDetails,
        selectedService: serviceId,
        clearError: true,
      ),
    );
  }

  void goToServiceChoice() {
    _setState(
      _state.copyWith(
        status: CustomerAuthStatus.serviceChoice,
        clearError: true,
      ),
    );
  }

  void completeProfileDetails({
    required String fullName,
    required String email,
  }) {
    _setState(
      _state.copyWith(
        status: CustomerAuthStatus.photoUpload,
        profileName: fullName.trim(),
        profileEmail: email.trim(),
        clearError: true,
      ),
    );
  }

  void goToProfileDetails() {
    _setState(
      _state.copyWith(
        status: CustomerAuthStatus.profileDetails,
        clearError: true,
      ),
    );
  }

  Future<void> uploadProfilePhoto(ImageSource source) async {
    final session = _state.session;
    final customer = _state.customer;
    if (session == null || customer == null) return;

    _setState(_state.copyWith(isLoading: true, clearError: true));
    try {
      final result = await _mediaUploadService.pickAndUpload(
        ownerId: customer.id,
        purpose: 'profile_photo',
        source: source,
      );
      if (result == null) {
        _setState(_state.copyWith(isLoading: false));
        return;
      }
      _setState(_state.copyWith(
        isLoading: false,
        profilePhotoUrl: result.url,
        profilePhotoAssetId: result.id,
        clearError: true
      ));
    } on ApiException catch (error) {
      _setState(_state.copyWith(isLoading: false, error: error));
    }
  }

  void completePhotoUpload() {
    _setState(
      _state.copyWith(status: CustomerAuthStatus.allSet, clearError: true),
    );
  }

  Future<void> finishProfileSetup() async {
    final session = _state.session;
    if (session == null) {
      backToPhoneEntry();
      return;
    }

    _setState(_state.copyWith(isLoading: true, clearError: true));
    try {
      final nameParts = _state.profileName.trim().split(RegExp(r'\s+'));
      final firstName = nameParts.first;
      final lastName = nameParts.length > 1 ? nameParts.skip(1).join(' ') : '';
      final updatedCustomer = await _api.updateProfile(
        accessToken: session.accessToken,
        firstName: firstName,
        lastName: lastName,
      );

      final photoUrl = _state.profilePhotoUrl;
      final assetId = _state.profilePhotoAssetId;
      if (photoUrl != null && assetId != null) {
        await _api.saveProfilePhotoUrl(
          accessToken: session.accessToken,
          photoUrl: photoUrl,
          assetId: assetId,
        );
      }

      final updatedSession = session.copyWith(customer: updatedCustomer);
      await _sessionStore.saveSession(updatedSession);
      _setState(_state.copyWith(
        status: CustomerAuthStatus.authenticated,
        session: updatedSession,
        customer: updatedCustomer,
        isLoading: false,
        clearError: true,
      ));
    } on ApiException catch (error) {
      _setState(_state.copyWith(isLoading: false, error: error));
    }
  }

  Future<void> logout() async {
    final refreshToken = _state.session?.refreshToken;
    _setState(_state.copyWith(isLoading: true, clearError: true));

    try {
      if (refreshToken != null && refreshToken.isNotEmpty) {
        await _api.logout(refreshToken: refreshToken);
      }
    } on ApiException {
      // The local session should still be cleared when server logout fails.
    } finally {
      await _sessionStore.clearSession();
      _setState(const CustomerAuthState(status: CustomerAuthStatus.phoneEntry));
    }
  }

  Future<void> clearAndRestart() async {
    await _sessionStore.clearSession();
    _setState(const CustomerAuthState(status: CustomerAuthStatus.onboarding));
  }

  Future<CustomerSession> _refreshSession(CustomerSession session) async {
    final deviceId = await _sessionStore.readOrCreateDeviceId();
    final result = await _api.refresh(
      refreshToken: session.refreshToken,
      deviceId: deviceId
    );
    final refreshed = result.toSession(DateTime.now());
    await _sessionStore.saveSession(refreshed);
    return refreshed;
  }

  void _setAuthenticatedSession(CustomerSession session) {
    _setState(
      CustomerAuthState(
        status: session.customer.requiresProfile
            ? CustomerAuthStatus.serviceChoice
            : CustomerAuthStatus.authenticated,
        session: session,
        customer: session.customer,
        phone: session.customer.phone,
        email: session.customer.email,
        profileEmail: session.customer.email
      ),
    );
  }

  void _setState(CustomerAuthState state) {
    _state = state;
    notifyListeners();
  }

  @override
  void dispose() {
    _api.close();
    _mediaUploadService.close();
    super.dispose();
  }
}