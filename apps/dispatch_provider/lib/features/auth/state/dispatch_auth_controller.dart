import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../core/storage/token_storage.dart';
import '../data/dispatch_auth_api.dart';
import '../models/dispatch_auth_models.dart';

enum AuthStatus { unauthenticated, authenticating, authenticated, error }

class DispatchAuthController extends ChangeNotifier {
  final DispatchAuthApi api;
  final TokenStorage tokenStorage;

  AuthStatus _status = AuthStatus.unauthenticated;
  AuthStatus get status => _status;

  bool get isLoading => _status == AuthStatus.authenticating;

  String? _providerId;
  String? get providerId => _providerId;

  String? _role;
  String? get role => _role;

  String? _errorMessage;
  String? get errorMessage => _errorMessage;

  /// The identifier used to start the current OTP flow (phone or email).
  /// Sent as `identifier` in the verify request.
  String? _currentIdentifier;
  String? get currentIdentifier => _currentIdentifier;

  /// The purpose of the current OTP flow: "login" or "signup".
  String _currentPurpose = '';

  /// Stored during signup start so we can resend the signup OTP correctly.
  String? _currentSignupEmail;

  /// The phone number for OTP flows (kept for display and legacy compat).
  String? _currentPhoneNumber;
  String? get currentPhoneNumber => _currentPhoneNumber;

  /// The email collected during the most recent signup start.
  /// Available after a successful [signupStart] so it can be passed to the
  /// "Tell us about you" screen for prefill.
  String? get currentSignupEmail => _currentSignupEmail;

  /// True while a verifyOtp network call is in flight.
  /// Prevents a second concurrent request from consuming a one-time OTP.
  bool _isVerifying = false;

  String? _accessToken;
  String? get accessToken => _accessToken;

  String? _refreshToken;
  String? get refreshToken => _refreshToken;

  DispatchAuthController({required this.api, TokenStorage? tokenStorage})
    : tokenStorage = tokenStorage ?? TokenStorage();

  static void _debugLog(String message) {
    if (kDebugMode) {
      debugPrint(message);
    }
  }

  static String _presence(String? value) =>
      value != null && value.isNotEmpty ? 'present' : 'missing';

  // ── Signup flow ────────────────────────────────────────────────────────────

  /// Calls POST /api/v1/auth/signup/start.
  /// On success, stores phone + email + purpose for the subsequent [verifyOtp] call.
  Future<ApiResult<AuthStartResponse>> signupStart({
    required String phoneNumber,
    required String email,
  }) async {
    _isVerifying = false; // new OTP session — allow verify again
    _status = AuthStatus.authenticating;
    _errorMessage = null;
    notifyListeners();

    try {
      final result = await api.signupStart(
        phoneNumber: phoneNumber,
        email: email,
      );

      return result.when(
        success: (data) {
          _currentIdentifier = phoneNumber;
          _currentPhoneNumber = phoneNumber;
          _currentSignupEmail = email;
          _currentPurpose = 'signup';
          _status = AuthStatus.unauthenticated;
          _errorMessage = null;
          notifyListeners();
          return ApiSuccess(data);
        },
        failure: (error) {
          _status = AuthStatus.error;
          _errorMessage = error.message;
          notifyListeners();
          return ApiFailure(error);
        },
      );
    } catch (e) {
      _status = AuthStatus.error;
      _errorMessage = e.toString();
      notifyListeners();
      return ApiFailure(
        ApiException(code: ApiErrorCode.unknown, message: e.toString()),
      );
    }
  }

  // ── Login flow ─────────────────────────────────────────────────────────────

  /// Calls POST /api/v1/auth/login/start.
  /// [identifier] may be a phone number (E.164) or an email address.
  /// Returns not_found (404) if no account exists for the identifier.
  Future<ApiResult<AuthStartResponse>> loginStart({
    required String identifier,
  }) async {
    _isVerifying = false; // new OTP session — allow verify again
    _status = AuthStatus.authenticating;
    _errorMessage = null;
    notifyListeners();

    try {
      final result = await api.loginStart(identifier: identifier);

      return result.when(
        success: (data) {
          _currentIdentifier = identifier;
          // Only store as phone when the identifier is not an email address.
          _currentPhoneNumber = identifier.contains('@') ? null : identifier;
          _currentSignupEmail = null;
          _currentPurpose = 'login';
          _status = AuthStatus.unauthenticated;
          _errorMessage = null;
          notifyListeners();
          return ApiSuccess(data);
        },
        failure: (error) {
          _status = AuthStatus.error;
          _errorMessage = error.message;
          notifyListeners();
          return ApiFailure(error);
        },
      );
    } catch (e) {
      _status = AuthStatus.error;
      _errorMessage = e.toString();
      notifyListeners();
      return ApiFailure(
        ApiException(code: ApiErrorCode.unknown, message: e.toString()),
      );
    }
  }

  // ── OTP verification ───────────────────────────────────────────────────────

  /// Verifies the OTP using the identifier and purpose stored from the last
  /// [loginStart] or [signupStart] call.
  /// On success, tokens are saved to secure storage.
  Future<ApiResult<AuthVerifyResponse>> verifyOtp(String otpCode) async {
    if (_currentIdentifier == null) {
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.validationFailed,
          message: 'No active OTP session. Please start again.',
        ),
      );
    }

    // Controller-level guard: OTP codes are consumed on first use.
    // A concurrent second call would hit the backend with an already-used OTP
    // and return 401, then surface a false "incorrect OTP" error to the user.
    if (_isVerifying) {
      _debugLog('[AUTH_UI] verify ignored — already in flight');
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.validationFailed,
          message: 'Verification already in progress.',
        ),
      );
    }

    _isVerifying = true;
    _debugLog(
      '[AUTH_UI] verify started | identifier=$_currentIdentifier | purpose=$_currentPurpose',
    );

    _status = AuthStatus.authenticating;
    _errorMessage = null;
    notifyListeners();

    try {
      final result = await api.verify(
        identifier: _currentIdentifier!,
        otpCode: otpCode,
        purpose: _currentPurpose,
        deviceId: _deviceId,
        deviceType: _deviceType,
      );

      return result.when(
        success: (data) async {
          _isVerifying = false;
          _providerId = data.providerId;
          _role = data.role;
          _accessToken = data.accessToken;
          _refreshToken = data.refreshToken;
          _status = AuthStatus.authenticated;
          _errorMessage = null;
          _debugLog(
            '[AUTH_UI] verify success | access=${_presence(data.accessToken)} '
            'refresh=${_presence(data.refreshToken)} provider_id=${_presence(data.providerId)}',
          );
          notifyListeners();

          // Persist tokens to secure storage.
          try {
            await tokenStorage.saveTokens(
              accessToken: data.accessToken,
              refreshToken: data.refreshToken,
              providerId: data.providerId,
            );
            _debugLog(
              '[AUTH] tokens saved | access=${_presence(data.accessToken)} '
              'refresh=${_presence(data.refreshToken)} provider_id=${_presence(data.providerId)}',
            );
          } catch (e) {
            // Storage failure is non-fatal — in-memory tokens still work.
            _debugLog('[AUTH] token persistence failed: $e');
          }

          return ApiSuccess(data);
        },
        failure: (error) {
          _isVerifying = false;
          _debugLog('[AUTH_UI] verify failed: ${error.message}');
          _status = AuthStatus.error;
          _errorMessage = error.message;
          notifyListeners();
          return ApiFailure(error);
        },
      );
    } catch (e) {
      _isVerifying = false;
      _status = AuthStatus.error;
      _errorMessage = e.toString();
      notifyListeners();
      return ApiFailure(
        ApiException(code: ApiErrorCode.unknown, message: e.toString()),
      );
    }
  }

  // ── Resend OTP ─────────────────────────────────────────────────────────────

  /// Re-triggers the appropriate start endpoint for the current OTP session.
  /// Uses the stored identifier + purpose.
  Future<ApiResult<AuthStartResponse>> resendOtp() async {
    if (_currentIdentifier == null) {
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.validationFailed,
          message: 'No active OTP session. Please start again.',
        ),
      );
    }

    if (_currentPurpose == 'signup') {
      return signupStart(
        phoneNumber: _currentIdentifier!,
        email: _currentSignupEmail ?? '',
      );
    } else {
      return loginStart(identifier: _currentIdentifier!);
    }
  }

  // ── Session restoration ────────────────────────────────────────────────────

  /// Restores tokens into memory (called at startup after reading from storage).
  /// Does NOT notify listeners — called before the widget tree is ready.
  void restoreTokens({
    required String accessToken,
    String? refreshToken,
    String? providerId,
  }) {
    _accessToken = accessToken.isNotEmpty ? accessToken : null;
    _refreshToken = refreshToken != null && refreshToken.isNotEmpty
        ? refreshToken
        : null;
    _providerId = providerId != null && providerId.isNotEmpty
        ? providerId
        : null;
    _debugLog(
      '[AUTH] restored saved tokens | access=${_presence(_accessToken)} '
      'refresh=${_presence(_refreshToken)} provider_id=${_presence(_providerId)}',
    );
    if (_accessToken != null) {
      _status = AuthStatus.authenticated;
    }
  }

  // ── Refresh ────────────────────────────────────────────────────────────────

  Future<ApiResult<AuthRefreshResponse>> refreshSession() async {
    if (_refreshToken == null) {
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.unauthorized,
          message: 'No refresh token available.',
        ),
      );
    }

    final result = await api.refresh(_refreshToken!);
    if (result is ApiSuccess<AuthRefreshResponse>) {
      final data = result.data;
      _refreshToken = data.refreshToken;
      _accessToken = data.accessToken;
      _status = AuthStatus.authenticated;
      _errorMessage = null;
      _debugLog(
        '[AUTH] refresh succeeded | access=${_presence(data.accessToken)} '
        'refresh=${_presence(data.refreshToken)}',
      );
      try {
        await tokenStorage.saveTokens(
          accessToken: data.accessToken,
          refreshToken: data.refreshToken,
          providerId: _providerId ?? '',
        );
      } catch (e) {
        _debugLog('[AUTH] refreshed token persistence failed: $e');
      }
      notifyListeners();
      return ApiSuccess(data);
    }

    final error = (result as ApiFailure<AuthRefreshResponse>).error;
    if (error.code == ApiErrorCode.unauthorized || error.statusCode == 401) {
      try {
        await tokenStorage.clear();
      } catch (e) {
        _debugLog('[AUTH] token clear after refresh 401 failed: $e');
      }
    }
    _accessToken = null;
    _refreshToken = null;
    _providerId = null;
    _role = null;
    _status = AuthStatus.unauthenticated;
    _errorMessage = error.message;
    _debugLog('[AUTH] refresh failed | code=${error.code}');
    notifyListeners();
    return ApiFailure(error);
  }

  // ── Logout ─────────────────────────────────────────────────────────────────

  Future<void> clearSession() async {
    try {
      await tokenStorage.clear();
    } catch (e) {
      _debugLog('[AUTH] token clear failed: $e');
    }
    _resetLocalState();
  }

  Future<ApiResult<String>> logout() async {
    final hadSession = _accessToken != null;

    // Clear secure storage regardless of API result.
    try {
      await tokenStorage.clear();
    } catch (e) {
      _debugLog('[AUTH] token clear on logout failed: $e');
    }

    if (!hadSession) {
      _resetLocalState();
      return const ApiSuccess('Logged out successfully (no session).');
    }

    final result = await api.logout(
      accessToken: _accessToken!,
      refreshToken: _refreshToken,
    );
    _resetLocalState();
    return result.when(
      success: (data) => ApiSuccess(data),
      failure: (error) => ApiFailure(error),
    );
  }

  void _resetLocalState() {
    _accessToken = null;
    _refreshToken = null;
    _providerId = null;
    _role = null;
    _currentIdentifier = null;
    _currentPhoneNumber = null;
    _currentSignupEmail = null;
    _currentPurpose = '';
    _status = AuthStatus.unauthenticated;
    _errorMessage = null;
    notifyListeners();
  }

  // ── Platform helpers ───────────────────────────────────────────────────────

  static String get _deviceId {
    if (kIsWeb) return 'web-dev';
    switch (defaultTargetPlatform) {
      case TargetPlatform.android:
        return 'android-dev';
      case TargetPlatform.iOS:
        return 'ios-dev';
      case TargetPlatform.windows:
        return 'windows-dev';
      case TargetPlatform.macOS:
        return 'macos-dev';
      case TargetPlatform.linux:
        return 'linux-dev';
      case TargetPlatform.fuchsia:
        return 'fuchsia-dev';
    }
  }

  static String get _deviceType {
    if (kIsWeb) return 'web';
    switch (defaultTargetPlatform) {
      case TargetPlatform.android:
        return 'android';
      case TargetPlatform.iOS:
        return 'ios';
      case TargetPlatform.windows:
        return 'windows';
      case TargetPlatform.macOS:
        return 'macos';
      case TargetPlatform.linux:
        return 'linux';
      case TargetPlatform.fuchsia:
        return 'fuchsia';
    }
  }
}
