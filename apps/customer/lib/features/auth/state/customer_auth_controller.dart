import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';

import '../data/customer_auth_api.dart';
import '../data/customer_session_store.dart';
import '../models/customer_auth_models.dart';

enum CustomerAuthStatus {
  checking,
  onboarding,
  phoneEntry,
  otpVerification,
  profileRequired,
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
    this.hasProfilePhoto = false,
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
  final bool hasProfilePhoto;
  final ApiException? error;

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
    bool? hasProfilePhoto,
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
      hasProfilePhoto: hasProfilePhoto ?? this.hasProfilePhoto,
      error: clearError ? null : error ?? this.error,
    );
  }
}

class CustomerAuthController extends ChangeNotifier {
  CustomerAuthController({
    required CustomerAuthApi api,
    required CustomerSessionStore sessionStore,
  }) : _api = api,
       _sessionStore = sessionStore;

  final CustomerAuthApi _api;
  final CustomerSessionStore _sessionStore;

  CustomerAuthState _state = const CustomerAuthState.checking();

  CustomerAuthState get state => _state;

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
                message: 'Verification code is required.',
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

  void continueFromProfileRequired() {
    if (_state.session == null) {
      backToPhoneEntry();
      return;
    }

    _setState(
      _state.copyWith(
        status: CustomerAuthStatus.serviceChoice,
        clearError: true,
      ),
    );
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

  void markProfilePhotoSelected() {
    _setState(_state.copyWith(hasProfilePhoto: true, clearError: true));
  }

  void completePhotoUpload() {
    _setState(
      _state.copyWith(status: CustomerAuthStatus.allSet, clearError: true),
    );
  }

  void finishProfileSetup() {
    if (_state.session == null) {
      backToPhoneEntry();
      return;
    }

    _setState(
      _state.copyWith(
        status: CustomerAuthStatus.authenticated,
        clearError: true,
      ),
    );
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

  Future<CustomerSession> _refreshSession(CustomerSession session) async {
    final deviceId = await _sessionStore.readOrCreateDeviceId();
    final result = await _api.refresh(
      refreshToken: session.refreshToken,
      deviceId: deviceId,
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
        profileEmail: session.customer.email,
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
    super.dispose();
  }
}
