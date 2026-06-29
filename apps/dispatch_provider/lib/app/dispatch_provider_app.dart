import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../core/config.dart';
import '../core/network/api_connectivity_check.dart';
import '../core/network/authenticated_api_client.dart';
import '../core/storage/token_storage.dart';
import '../features/account_setup/ui/bike_type_screen.dart';
import '../features/account_setup/ui/driver_information_screen.dart';
import '../features/account_setup/ui/operation_mode_screen.dart';
import '../features/account_setup/ui/photo_upload_screen.dart';
import '../features/account_setup/ui/profile_details_screen.dart';
import '../features/account_setup/ui/service_type_screen.dart';
import '../features/account_setup/ui/verification_pending_screen.dart';
import '../features/account_type/ui/account_type_screen.dart';
import '../features/auth/data/dispatch_auth_api.dart';
import '../features/auth/state/dispatch_auth_controller.dart';
import '../features/auth/ui/login_screen.dart';
import '../features/auth/ui/otp_verification_screen.dart';
import '../features/auth/ui/phone_entry_screen.dart';
import '../features/auth/ui/splash_screen.dart';
import '../features/onboarding/ui/onboarding_screen.dart';
import '../features/home/ui/dashboard_screen.dart';
import '../features/profile/data/provider_profile_api.dart';
import '../features/profile/state/provider_profile_controller.dart';
import '../features/vehicle/data/vehicle_api.dart';
import '../features/vehicle/state/vehicle_controller.dart';
import '../features/verification/data/verification_api.dart';
import '../features/verification/state/verification_controller.dart';
import '../features/availability/data/availability_api.dart';
import '../features/availability/state/availability_controller.dart';
import '../features/requests/data/requests_api.dart';
import '../features/requests/state/requests_controller.dart';
import '../features/trips/data/trips_api.dart';
import '../features/trips/state/trips_controller.dart';
import '../features/wallet/data/wallet_api.dart';
import '../features/wallet/state/wallet_controller.dart';

enum _DispatchScreen {
  splash,
  login,
  onboarding,
  phoneEntry,
  otpVerification,
  accountType,
  serviceType,
  operationMode,
  profileDetails,
  photoUpload,
  bikeType,
  driverInformation,
  verificationPending,
  home,
}

class DispatchProviderApp extends StatefulWidget {
  const DispatchProviderApp({super.key});

  @override
  State<DispatchProviderApp> createState() => _DispatchProviderAppState();
}

class _DispatchProviderAppState extends State<DispatchProviderApp> {
  _DispatchScreen _screen = _DispatchScreen.splash;

  /// The identifier shown to the user on the OTP screen (phone or email).
  String _phone = '';

  String? _selectedOperationType;

  /// True when the active OTP flow was started from the Login screen.
  /// Post-verify routing uses this to decide whether to call /provider/me.
  bool _isLoginMode = false;

  late final TokenStorage _tokenStorage;
  late final DispatchAuthController _authController;
  late final ProviderProfileController _profileController;
  late final VerificationController _verificationController;
  late final VehicleController _vehicleController;
  late final AvailabilityController _availabilityController;
  late final RequestsController _requestsController;
  late final TripsController _tripsController;
  late final WalletController _walletController;

  ProfileDetailsData? _profileDetailsData;
  BikeTypeData? _bikeTypeData;

  /// Selfie file path collected on PhotoUploadScreen. Stored locally and
  /// uploaded only at the final submit step (DriverInformation.onSubmit).
  String? _selfiePath;

  static const _totalSteps = 6;

  @override
  void initState() {
    super.initState();
    final apiBaseUrl = AppConfig.apiBaseUrl;
    final walletBaseUrl = AppConfig.walletBaseUrl;
    final apiConfig = ApiCoreConfig(baseUrl: apiBaseUrl);
    final walletApiConfig = ApiCoreConfig(baseUrl: walletBaseUrl);
    assert(() {
      debugPrint('[CONFIG] Dispatch Provider API base URL: $apiBaseUrl');
      debugPrint('[CONFIG] Wallet service base URL: $walletBaseUrl');
      return true;
    }());

    _tokenStorage = TokenStorage();
    _authController = DispatchAuthController(
      api: DispatchAuthApi(apiConfig),
      tokenStorage: _tokenStorage,
    );
    final authenticatedClient = AuthenticatedApiClient(
      getAccessToken: () => _authController.accessToken,
      refreshSession: _refreshProtectedSession,
      onSessionExpired: _handleSessionExpired,
    );
    _profileController = ProviderProfileController(
      api: ProviderProfileApi(apiConfig, client: authenticatedClient),
      getAccessToken: () => _authController.accessToken,
    );
    _verificationController = VerificationController(
      api: VerificationApi(apiConfig, client: authenticatedClient),
      getAccessToken: () => _authController.accessToken,
    );
    _vehicleController = VehicleController(
      api: VehicleApi(apiConfig, client: authenticatedClient),
      getAccessToken: () => _authController.accessToken,
    );
    _availabilityController = AvailabilityController(
      api: AvailabilityApi(apiConfig, client: authenticatedClient),
      getAccessToken: () => _authController.accessToken,
    );
    _requestsController = RequestsController(
      api: RequestsApi(apiConfig, client: authenticatedClient),
      getAccessToken: () => _authController.accessToken,
    );
    _tripsController = TripsController(
      api: TripsApi(apiConfig, client: authenticatedClient),
      getAccessToken: () => _authController.accessToken,
    );
    _walletController = WalletController(
      api: WalletApi(walletApiConfig, client: authenticatedClient),
      getAccessToken: () => _authController.accessToken,
    );
    _logBackendHealth();
  }

  void _logBackendHealth() {
    assert(() {
      debugPrint('[CONFIG] Backend mode hint: ${AppConfig.backendModeHint}');
      return true;
    }());
    ApiConnectivityCheck.check().then((result) {
      assert(() {
        debugPrint(
          '[CONFIG] Backend health base=${result.baseUrl} '
          'reachable=${result.reachable} status=${result.statusCode} '
          'error=${result.errorMessage ?? 'none'}',
        );
        if (!result.reachable && AppConfig.isLocalhostUrl) {
          final isAndroid =
              !kIsWeb && defaultTargetPlatform == TargetPlatform.android;
          if (isAndroid) {
            debugPrint(
              '\n'
              '╔══════════════════════════════════════════════════════════════╗\n'
              '║  ⚠️  CONNECTION WARNING — backend not reachable on phone     ║\n'
              '╠══════════════════════════════════════════════════════════════╣\n'
              '║  The app cannot reach ${result.baseUrl} from this device.   \n'
              '║  On a physical phone, 127.0.0.1 means the phone itself.     ║\n'
              '║  The laptop backend requires an active adb reverse tunnel.  ║\n'
              '╠══════════════════════════════════════════════════════════════╣\n'
              '║  FIX — run these commands in your laptop terminal:           ║\n'
              '║                                                              ║\n'
              '║  adb reverse tcp:8103 tcp:8103                               ║\n'
              '║  adb reverse --list          ← verify tunnel is active       ║\n'
              '║                                                              ║\n'
              '║  flutter run -d RFCY51N8EJB                                  ║\n'
              '╚══════════════════════════════════════════════════════════════╝\n',
            );
          }
        }
        return true;
      }());
    });
  }

  Future<bool> _refreshProtectedSession() async {
    final result = await _authController.refreshSession();
    var refreshed = false;
    result.when(
      success: (_) {
        refreshed = true;
      },
      failure: (error) {
        debugPrint('[AUTH] Protected refresh failed: ${error.message}');
      },
    );
    return refreshed;
  }

  Future<void> _handleSessionExpired() async {
    await _authController.clearSession();
    _profileController.clearLocalState();
    if (mounted) {
      _go(_DispatchScreen.login);
    }
  }

  int get _currentStep => switch (_screen) {
    _DispatchScreen.serviceType => 1,
    _DispatchScreen.operationMode => 2,
    _DispatchScreen.profileDetails => 3,
    _DispatchScreen.photoUpload => 4,
    _DispatchScreen.bikeType => 5,
    _DispatchScreen.driverInformation => 6,
    _ => 0,
  };

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Cosmicforge Logistics Driver',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorSchemeSeed: const Color(0xFF4CAF50),
        useMaterial3: true,
        fontFamily: 'Poppins',
      ),
      home: _buildScreen(),
    );
  }

  Widget _buildScreen() {
    return switch (_screen) {
      // ── Splash ──────────────────────────────────────────────────────────────
      _DispatchScreen.splash => SplashScreen(onDone: _checkSavedSession),

      // ── Login (first screen for unauthenticated users) ───────────────────
      _DispatchScreen.login => LoginScreen(
        controller: _authController,
        onContinue: (identifier) {
          _phone = identifier;
          _isLoginMode = true;
          _go(_DispatchScreen.otpVerification);
        },
        onCreateAccountTap: () => _go(_DispatchScreen.onboarding),
      ),

      // ── Onboarding (shown before signup) ────────────────────────────────
      _DispatchScreen.onboarding => OnboardingScreen(
        onDone: () => _go(_DispatchScreen.phoneEntry),
      ),

      // ── Signup — phone + email entry ─────────────────────────────────────
      _DispatchScreen.phoneEntry => PhoneEntryScreen(
        controller: _authController,
        onContinue: (phone, email) {
          _phone = phone;
          _isLoginMode = false;
          _go(_DispatchScreen.otpVerification);
        },
        onLoginTap: () => _go(_DispatchScreen.login),
      ),

      // ── OTP verification ─────────────────────────────────────────────────
      _DispatchScreen.otpVerification => OtpVerificationScreen(
        controller: _authController,
        phone: _phone,
        onVerify: (_) => _handlePostVerify(),
        onBack: () => _isLoginMode
            ? _go(_DispatchScreen.login)
            : _go(_DispatchScreen.phoneEntry),
        onResend: () {},
      ),

      // ── Account setup ────────────────────────────────────────────────────
      _DispatchScreen.accountType => AccountTypeScreen(
        onSelected: (type) => _go(_DispatchScreen.serviceType),
      ),
      _DispatchScreen.serviceType => ServiceTypeScreen(
        onContinue: (_) => _go(_DispatchScreen.operationMode),
        onBack: () => _go(_DispatchScreen.accountType),
        currentStep: _currentStep,
        totalSteps: _totalSteps,
      ),
      _DispatchScreen.operationMode => OperationModeScreen(
        onContinue: (mode) {
          _selectedOperationType = mode;
          _go(_DispatchScreen.profileDetails);
        },
        onBack: () => _go(_DispatchScreen.serviceType),
        currentStep: _currentStep,
        totalSteps: _totalSteps,
      ),
      _DispatchScreen.profileDetails => ProfileDetailsScreen(
        initialPhone: _authController.currentPhoneNumber,
        initialEmail: _authController.currentSignupEmail,
        profileController: _profileController,
        operationType: _selectedOperationType,
        onContinue: (data) async {
          // POST /provider/onboarding was already called inside
          // ProfileDetailsScreen before this callback fires. Here we just
          // store the collected data locally and advance to the next step.
          // Backend uploads (identity, face, licence, vehicle) happen later,
          // after the user completes all remaining screens.
          _profileDetailsData = data;
          _go(_DispatchScreen.photoUpload);
        },
        onBack: () => _go(_DispatchScreen.operationMode),
        currentStep: _currentStep,
        totalSteps: _totalSteps,
      ),
      _DispatchScreen.photoUpload => PhotoUploadScreen(
        onContinue: (selfiePath) async {
          // Store the selfie path locally — no backend calls yet.
          // Identity (E) and face (F) uploads are deferred to the final
          // submit step in DriverInformation.onSubmit, after the backend
          // has had time to initialise verification steps.
          debugPrint(
            '[PHOTO_UPLOAD] Selfie selected: $selfiePath — storing locally.',
          );
          _selfiePath = selfiePath;
          _go(_DispatchScreen.bikeType);
        },
        onBack: () => _go(_DispatchScreen.profileDetails),
        currentStep: _currentStep,
        totalSteps: _totalSteps,
      ),
      _DispatchScreen.bikeType => BikeTypeScreen(
        onContinue: (data) {
          _bikeTypeData = data;
          _go(_DispatchScreen.driverInformation);
        },
        onBack: () => _go(_DispatchScreen.photoUpload),
        currentStep: _currentStep,
        totalSteps: _totalSteps,
      ),
      _DispatchScreen.driverInformation => DriverInformationScreen(
        profileController: _profileController,
        onSubmit: (data) async {
          // At this point DriverInformationScreen has already submitted:
          //   B. POST /provider/guarantor
          //   C. POST /provider/emergency-contact
          //
          // Now run the remaining upload sequence in order:
          //   D. GET /verification/status  (poll until steps exist)
          //   E. POST /verification/identity
          //   F. POST /verification/face
          //   G. POST /verification/licence
          //   H. POST /provider/vehicle
          //   I. POST /provider/vehicle/:id/documents

          // ── D: Wait for verification steps ─────────────────────────────
          debugPrint('[SUBMIT] Polling verification steps (D)...');
          final stepsReady = await _waitForVerificationSteps();
          if (!stepsReady) {
            throw 'Verification is not ready yet. Please check your connection and try again.';
          }

          // ── E: Submit identity (govt ID + profile photo) ───────────────
          if (_profileDetailsData != null && _selfiePath != null) {
            debugPrint('[SUBMIT] Submitting identity (E)...');
            final identityRes = await _verificationController.submitIdentity(
              govtIdType: _profileDetailsData!.governmentIdType,
              govtIdNumber: _profileDetailsData!.governmentIdNumber,
              govtIdFilePath: _profileDetailsData!.governmentIdFilePath,
              profilePhotoFilePath: _selfiePath!,
            );
            String? identityErr;
            identityRes.when(
              success: (_) {
                debugPrint('[SUBMIT] Identity submitted (E) ✓');
              },
              failure: (err) {
                debugPrint('[SUBMIT] Identity failed (E): ${err.message}');
                identityErr = _friendlyUploadError(err.code, err.message);
              },
            );
            if (identityErr != null) throw identityErr!;

            // ── F: Submit face (selfie only) ───────────────────────────────
            debugPrint('[SUBMIT] Submitting face (F)...');
            final faceRes = await _verificationController.submitFace(
              selfieFilePath: _selfiePath!,
            );
            String? faceErr;
            faceRes.when(
              success: (_) {
                debugPrint('[SUBMIT] Face submitted (F) ✓');
              },
              failure: (err) {
                debugPrint('[SUBMIT] Face failed (F): ${err.message}');
                faceErr = _friendlyUploadError(err.code, err.message);
              },
            );
            if (faceErr != null) throw faceErr!;
          }

          // ── G: Submit driver licence ───────────────────────────────────
          if (_bikeTypeData != null && data.licenseFilePath.isNotEmpty) {
            debugPrint('[SUBMIT] Submitting licence (G)...');
            final expiry = _bikeTypeData!.licenceExpiryDate;
            final licenceRes = await _verificationController.submitLicence(
              licenceNumber: _bikeTypeData!.licenseNo,
              expiryYear: expiry.year.toString(),
              expiryMonth: expiry.month.toString().padLeft(2, '0'),
              licenceFilePath: data.licenseFilePath,
            );
            String? licenceErr;
            licenceRes.when(
              success: (_) {
                debugPrint('[SUBMIT] Licence submitted (G) ✓');
              },
              failure: (err) {
                debugPrint('[SUBMIT] Licence failed (G): ${err.message}');
                licenceErr = err.message;
              },
            );
            if (licenceErr != null) throw licenceErr!;
          }

          // ── H: Create vehicle ──────────────────────────────────────────
          if (_bikeTypeData != null) {
            debugPrint('[SUBMIT] Creating vehicle (H)...');
            debugPrint('[SUBMIT]   bike_type=${_bikeTypeData!.bikeType}');
            debugPrint('[SUBMIT]   brand=${_bikeTypeData!.bikeBrand}');
            debugPrint('[SUBMIT]   model=${_bikeTypeData!.bikeModel}');
            debugPrint('[SUBMIT]   year=${_bikeTypeData!.bikeYear}');
            debugPrint('[SUBMIT]   color=${_bikeTypeData!.color}');
            debugPrint('[SUBMIT]   plate_number=${_bikeTypeData!.plateNumber}');
            final vehicleRes = await _vehicleController.createVehicle(
              bikeType: _bikeTypeData!.bikeType,
              brand: _bikeTypeData!.bikeBrand,
              model: _bikeTypeData!.bikeModel,
              year: _bikeTypeData!.bikeYear,
              color: _bikeTypeData!.color,
              plateNumber: _bikeTypeData!.plateNumber,
            );
            String? vehicleId;
            String? vehicleErr;
            vehicleRes.when(
              success: (id) {
                vehicleId = id;
                debugPrint('[SUBMIT] Vehicle created (H): id=$id ✓');
              },
              failure: (err) {
                debugPrint(
                  '[SUBMIT] Vehicle failed (H): ${err.message} fields=${err.fields.map((f) => f.message).join(', ')}',
                );
                // Show per-field errors when available, fall back to top-level message.
                vehicleErr = err.fields.isNotEmpty
                    ? err.fields.map((f) => f.message).join(' ')
                    : err.message;
              },
            );
            if (vehicleErr != null) throw vehicleErr!;

            // ── I: Upload vehicle registration document ──────────────────
            if (vehicleId != null &&
                vehicleId!.isNotEmpty &&
                data.vehicleRegFilePath.isNotEmpty) {
              debugPrint(
                '[SUBMIT] Uploading vehicle doc (I): vehicleId=$vehicleId',
              );
              final docRes = await _vehicleController.uploadVehicleDocument(
                vehicleId: vehicleId!,
                documentType: 'registration',
                documentFilePath: data.vehicleRegFilePath,
              );
              docRes.when(
                success: (_) {
                  debugPrint('[SUBMIT] Vehicle doc uploaded (I) ✓');
                },
                failure: (err) {
                  // Non-fatal: vehicle was created. Log and continue.
                  debugPrint(
                    '[SUBMIT] Vehicle doc upload failed (I) — non-fatal: ${err.message}',
                  );
                },
              );
            }
          }

          debugPrint(
            '[SUBMIT] All steps complete — navigating to verificationPending.',
          );
          _go(_DispatchScreen.verificationPending);
        },
        onBack: () => _go(_DispatchScreen.bikeType),
        currentStep: _currentStep,
        totalSteps: _totalSteps,
      ),
      _DispatchScreen.verificationPending => VerificationPendingScreen(
        onGoToDashboard: () => _go(_DispatchScreen.home),
        verificationController: _verificationController,
      ),
      _DispatchScreen.home => DashboardScreen(
        profileController: _profileController,
        verificationController: _verificationController,
        vehicleController: _vehicleController,
        availabilityController: _availabilityController,
        requestsController: _requestsController,
        tripsController: _tripsController,
        walletController: _walletController,
        onLogout: _handleLogout,
        onAccountDeleted: _handleAccountDeleted,
      ),
    };
  }

  // ---------------------------------------------------------------------------
  // Startup session check
  // ---------------------------------------------------------------------------
  // Reads saved tokens → tries GET /provider/me → refreshes if 401 →
  // routes to the correct screen.  Falls back to Login on any failure.
  // ---------------------------------------------------------------------------
  Future<void> _checkSavedSession() async {
    debugPrint('[AUTH] Checking saved session...');
    late final SavedTokens saved;
    try {
      saved = await _tokenStorage.readTokens();
    } catch (error, stackTrace) {
      debugPrint('[AUTH] Failed to read saved tokens: $error\n$stackTrace');
      await _handleSessionExpired();
      return;
    }

    if (!saved.hasAny ||
        (saved.accessToken == null && saved.refreshToken == null)) {
      debugPrint('[AUTH] No saved tokens → Login');
      _go(_DispatchScreen.login);
      return;
    }

    debugPrint('[AUTH] Saved tokens found — restoring in-memory state');
    _authController.restoreTokens(
      accessToken: saved.accessToken ?? '',
      refreshToken: saved.refreshToken,
      providerId: saved.providerId,
    );

    if ((saved.accessToken == null || saved.accessToken!.isEmpty) &&
        saved.refreshToken != null) {
      debugPrint('[AUTH] No saved access token — attempting refresh');
      final refreshed = await _refreshProtectedSession();
      if (!mounted) return;
      if (!refreshed) {
        debugPrint('[AUTH] Refresh-only restore failed → Login');
        await _handleSessionExpired();
        return;
      }
    }

    // Try to load profile with the existing access token.
    final profileResult = await _profileController.loadMe();
    if (!mounted) return;

    bool profileSuccess = false;
    bool isUnauthorized = false;
    bool isNetworkError = false;

    profileResult.when(
      success: (profile) {
        profileSuccess = true;
        debugPrint(
          '[AUTH] Profile loaded | onboardingComplete=${profile.onboardingComplete}',
        );
        if (profile.onboardingComplete) {
          _go(_DispatchScreen.home);
        } else {
          _go(_DispatchScreen.accountType);
        }
      },
      failure: (error) {
        debugPrint(
          '[AUTH] Profile load failed: code=${error.code} message=${error.message}',
        );
        isUnauthorized = error.code == ApiErrorCode.unauthorized;
        isNetworkError = error.code == ApiErrorCode.network;
      },
    );

    if (profileSuccess) return;

    // Network error during cold start: tokens may be valid but there is no
    // connectivity right now. Preserve tokens and route to Home — the app
    // will re-attempt profile loading when the user interacts.
    if (isNetworkError) {
      debugPrint(
        '[AUTH] Network error during session restore → Home (tokens preserved)',
      );
      _go(_DispatchScreen.home);
      return;
    }

    // If access token is expired and we have a refresh token, try to renew.
    if (isUnauthorized && saved.refreshToken != null) {
      debugPrint('[AUTH] Access token expired — attempting refresh');
      final refreshResult = await _authController.refreshSession();
      if (!mounted) return;

      bool refreshed = false;
      String? newAccess;
      String? newRefresh;

      refreshResult.when(
        success: (data) {
          refreshed = true;
          newAccess = data.accessToken;
          newRefresh = data.refreshToken;
        },
        failure: (error) {
          debugPrint('[AUTH] Refresh failed: ${error.message}');
        },
      );

      if (refreshed && newAccess != null) {
        // Persist the refreshed tokens.
        try {
          await _tokenStorage.saveTokens(
            accessToken: newAccess!,
            refreshToken: newRefresh ?? saved.refreshToken ?? '',
            providerId: saved.providerId ?? '',
          );
        } catch (e) {
          debugPrint('[AUTH] Failed to persist refreshed tokens: $e');
        }

        // Retry loading the profile with the new access token.
        final retryResult = await _profileController.loadMe();
        if (!mounted) return;

        bool retrySuccess = false;
        retryResult.when(
          success: (profile) {
            retrySuccess = true;
            debugPrint(
              '[AUTH] Profile loaded after refresh | onboardingComplete=${profile.onboardingComplete}',
            );
            if (profile.onboardingComplete) {
              _go(_DispatchScreen.home);
            } else {
              _go(_DispatchScreen.accountType);
            }
          },
          failure: (error) {
            debugPrint(
              '[AUTH] Profile load after refresh failed: ${error.message}',
            );
          },
        );

        if (retrySuccess) return;
      }
    }

    // All token-based attempts exhausted (refresh failed or was unavailable).
    // Clear stored tokens — they are no longer usable — and show Login.
    debugPrint(
      '[AUTH] Session restore failed — refresh exhausted → clearing tokens → Login',
    );
    await _handleSessionExpired();
  }

  // ---------------------------------------------------------------------------
  // Post-OTP-verify routing
  // ---------------------------------------------------------------------------
  //   • Signup  (_isLoginMode == false) → AccountType
  //   • Login   (_isLoginMode == true)  → GET /provider/me
  //       onboardingComplete → Dashboard
  //       else               → AccountType (continue setup)
  //       failure            → AccountType (safe fallback)
  // ---------------------------------------------------------------------------
  Future<void> _handlePostVerify() async {
    debugPrint('[AUTH_FLOW] post verify | isLoginMode=$_isLoginMode');

    if (!_isLoginMode) {
      debugPrint('[AUTH_FLOW] signup flow → AccountType');
      _go(_DispatchScreen.accountType);
      return;
    }

    debugPrint('[AUTH_FLOW] login flow → GET /api/v1/provider/me');
    final result = await _profileController.loadMe();
    if (!mounted) return;

    result.when(
      success: (profile) {
        debugPrint(
          '[AUTH_FLOW] /provider/me success'
          ' | provider_id=${profile.providerId}'
          ' | onboarding_complete=${profile.onboardingComplete}'
          ' | verification_status=${profile.verificationStatus}'
          ' | phone=${profile.phone.isNotEmpty ? "present" : "missing"}'
          ' | email=${profile.email != null ? "present" : "missing"}',
        );
        if (profile.onboardingComplete) {
          debugPrint('[AUTH_FLOW] onboarding_complete=true → Home');
          _go(_DispatchScreen.home);
        } else {
          // NOTE: If this fires for a user who believes they completed onboarding,
          // the backend DB has onboarding_complete=false for provider_id=${profile.providerId}.
          // Check the providers table and verify onboarding_complete was set to true.
          debugPrint(
            '[AUTH_FLOW] onboarding_complete=false → AccountType'
            ' | INVESTIGATE: provider_id=${profile.providerId} may have stale DB state',
          );
          _go(_DispatchScreen.accountType);
        }
      },
      failure: (error) {
        debugPrint(
          '[AUTH_FLOW] /provider/me failed | code=${error.code} message=${error.message}',
        );
        // OTP verified successfully — tokens are already saved. We must NOT
        // route back to Login or AccountType: Login lets the user re-enter the
        // same (already-consumed) OTP causing a 401; AccountType wipes an
        // existing user's completed onboarding. Go to Home and let the app
        // reload the profile lazily.
        debugPrint(
          '[AUTH_FLOW] profile load failed after verify → Home '
          '(tokens valid, profile loads on mount)\n'
          '  If this is a physical Android device: ensure '
          'adb reverse tcp:8103 tcp:8103 is active',
        );
        _go(_DispatchScreen.home);
      },
    );
  }

  // ---------------------------------------------------------------------------
  // Logout
  // ---------------------------------------------------------------------------
  // Calls the auth API, clears all stored tokens, resets in-memory state,
  // then routes back to the Login screen.
  // ---------------------------------------------------------------------------
  Future<void> _handleLogout() async {
    debugPrint('[AUTH] User initiated logout');
    // The controller already clears storage + in-memory state on logout().
    await _authController.logout();
    if (!mounted) return;
    // Reset profile state so a future login doesn't show stale data.
    _profileController.clearLocalState();
    _go(_DispatchScreen.login);
  }

  Future<void> _handleAccountDeleted() async {
    debugPrint('[AUTH] Account deleted — clearing local session');
    // Backend already revoked all sessions via DELETE /provider/me.
    // Clear stored tokens locally without calling the logout endpoint.
    await _authController.clearSession();
    if (!mounted) return;
    _profileController.clearLocalState();
    _go(_DispatchScreen.login);
  }

  /// Polls GET /verification/status until the steps list is non-empty (meaning
  /// the backend subscriber has initialized them) or all attempts are exhausted.
  Future<bool> _waitForVerificationSteps() async {
    const maxAttempts = 3;
    const delay = Duration(milliseconds: 800);
    for (int attempt = 0; attempt < maxAttempts; attempt++) {
      if (attempt > 0) {
        debugPrint(
          '[VERIFICATION] Steps not ready — retrying in ${delay.inMilliseconds}ms (attempt ${attempt + 1}/$maxAttempts)',
        );
        await Future.delayed(delay);
      }
      debugPrint(
        '[VERIFICATION] Polling GET /verification/status (attempt ${attempt + 1}/$maxAttempts)',
      );
      final result = await _verificationController.loadVerificationStatus();
      bool ready = false;
      result.when(
        success: (status) {
          debugPrint(
            '[VERIFICATION] Status: overall=${status.overallStatus}, steps=${status.steps.length}',
          );
          ready = status.steps.isNotEmpty;
        },
        failure: (err) {
          // 404 = steps not yet initialized — retry silently
          debugPrint(
            '[VERIFICATION] Status poll failed: ${err.message} (code=${err.code})',
          );
        },
      );
      if (ready) return true;
    }
    debugPrint(
      '[VERIFICATION] Steps still not initialized after $maxAttempts attempts',
    );
    return false;
  }

  /// Returns a user-readable error message for upload failures.
  String _friendlyUploadError(String code, String rawMessage) {
    if (code == 'precondition_failed') {
      return 'Your verification setup is not ready yet. Please go back and complete your profile details.';
    }
    return rawMessage;
  }

  void _go(_DispatchScreen screen) => setState(() => _screen = screen);
}
