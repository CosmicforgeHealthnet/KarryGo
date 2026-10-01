import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../core/config.dart';
import '../core/network/api_connectivity_check.dart';
import '../core/network/authenticated_api_client.dart';
import '../core/storage/token_storage.dart';
import '../core/theme/dispatch_theme.dart';
import '../core/theme/theme_controller.dart';
import '../features/account_setup/ui/bike_type_screen.dart';
import '../features/account_setup/ui/driver_information_screen.dart';
import '../features/account_setup/ui/emergency_contact_screen.dart';
import '../features/account_setup/ui/guarantor_screen.dart';
import '../features/account_setup/ui/operation_mode_screen.dart';
import '../features/account_setup/ui/photo_upload_screen.dart';
import '../features/account_setup/ui/profile_details_screen.dart';
import '../features/account_setup/ui/service_type_screen.dart';
import '../features/account_setup/ui/setup_hub_screen.dart';
import '../features/account_setup/ui/setup_review_screen.dart';
import '../features/account_setup/ui/verification_pending_screen.dart';
import '../features/account_type/ui/account_type_screen.dart';
import '../features/auth/data/dispatch_auth_api.dart';
import '../features/auth/state/dispatch_auth_controller.dart';
import '../features/auth/ui/login_screen.dart';
import '../features/auth/ui/otp_verification_screen.dart';
import '../features/auth/ui/phone_entry_screen.dart';
import '../features/auth/ui/splash_screen.dart';
import '../features/auth/ui/welcome_back_screen.dart';
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
import '../features/disputes/data/rider_support_api.dart';
import '../features/disputes/state/rider_dispute_controller.dart';
import '../features/notifications/data/firebase_push.dart';
import '../features/notifications/data/rider_notification_api.dart';
import '../features/notifications/data/rider_realtime_listener.dart';
import '../features/notifications/state/rider_notification_controller.dart';
import '../features/notifications/ui/notifications_screen.dart';
import '../features/requests/data/requests_api.dart';
import '../features/requests/state/requests_controller.dart';
import '../features/trips/data/trips_api.dart';
import '../features/trips/data/trip_model.dart';
import '../features/trips/state/trips_controller.dart';
import '../features/wallet/data/wallet_api.dart';
import '../features/wallet/state/wallet_controller.dart';
import '../shared/widgets/mock_badge.dart';

enum _DispatchScreen {
  splash,
  login,
  onboarding,
  phoneEntry,
  otpVerification,
  welcomeBack,
  setupHub,
  profileDetails,
  operationMode,
  emergencyContact,
  guarantor,
  photoUpload,
  setupReview,
  verificationPending,
  home,
  // Legacy — kept so imports don't break. Not routed in the active flow.
  accountType,
  serviceType,
  bikeType,
  driverInformation,
}

class DispatchProviderApp extends StatefulWidget {
  const DispatchProviderApp({super.key, required this.themeController});

  /// Owns the app's [ThemeMode] and persists the choice. Loaded in main()
  /// before runApp so the first frame already has the right brightness.
  final ThemeController themeController;

  @override
  State<DispatchProviderApp> createState() => _DispatchProviderAppState();
}

class _DispatchProviderAppState extends State<DispatchProviderApp> {
  _DispatchScreen _screen = _DispatchScreen.splash;

  String _phone = '';
  String? _selectedOperationType;
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
  late final RiderNotificationApi _notificationApi;
  late final RiderNotificationController _notificationController;
  late final RiderDisputeController _disputeController;
  RiderRealtimeListener? _realtimeListener;
  StreamSubscription<String>? _fcmTokenRefreshSub;
  StreamSubscription<dynamic>? _fcmForegroundSub;
  final GlobalKey<NavigatorState> _navigatorKey = GlobalKey<NavigatorState>();

  ProfileDetailsData? _profileDetailsData;
  EmergencyContactData? _emergencyContactData;
  GuarantorData? _guarantorData;
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
      shouldPoll: () => _availabilityController.isOnline,
    );
    _tripsController = TripsController(
      api: TripsApi(apiConfig, client: authenticatedClient),
      getAccessToken: () => _authController.accessToken,
    );
    _walletController = WalletController(
      api: WalletApi(walletApiConfig, client: authenticatedClient),
      getAccessToken: () => _authController.accessToken,
    );
    _notificationApi = RiderNotificationApi(
      config: apiConfig,
      client: authenticatedClient,
    );
    _notificationController = RiderNotificationController(
      api: _notificationApi,
      accessToken: () => _authController.accessToken,
    );
    _disputeController = RiderDisputeController(
      api: RiderSupportApi(
        config: ApiCoreConfig(baseUrl: AppConfig.supportBaseUrl),
        client: authenticatedClient,
      ),
      accessToken: () => _authController.accessToken,
    );
    _logBackendHealth();
  }

  void _startRealtimeAndPush() {
    _realtimeListener ??= RiderRealtimeListener(
      api: _notificationApi,
      wsUrl: AppConfig.notificationWsUrl,
      accessToken: () => _authController.accessToken,
      onEvent: _onRealtimeEvent,
    );
    _realtimeListener!.start();

    _registerPushDevice();
    _fcmTokenRefreshSub ??= FirebasePush.tokenRefresh().listen((_) {
      _registerPushDevice();
    });
    _fcmForegroundSub ??= FirebasePush.foregroundMessages().listen((_) {
      _onRealtimeEvent('fcm.foreground');
    });
  }

  void _stopRealtimeAndPush() {
    _realtimeListener?.stop();
    _notificationController.clear();
  }

  void _onRealtimeEvent(String eventType) {
    _requestsController.pokeRefresh();
    _tripsController.pokeRefresh();
    _notificationController.onRealtimeEvent(eventType);
  }

  Future<void> _registerPushDevice() async {
    final accessToken = _authController.accessToken;
    if (accessToken == null || accessToken.isEmpty) return;
    final token = await FirebasePush.deviceToken();
    if (token == null) return;
    try {
      await _notificationApi.registerDevice(
        accessToken: accessToken,
        token: token,
        platform: defaultTargetPlatform == TargetPlatform.iOS
            ? 'ios'
            : 'android',
      );
      debugPrint('[PUSH] device registered');
    } catch (error) {
      debugPrint('[PUSH] device registration failed: $error');
    }
  }

  void _openNotifications() {
    _navigatorKey.currentState?.push(
      MaterialPageRoute(
        builder: (_) => NotificationsScreen(controller: _notificationController),
      ),
    );
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
    _DispatchScreen.profileDetails => 1,
    _DispatchScreen.operationMode => 2,
    _DispatchScreen.emergencyContact => 3,
    _DispatchScreen.guarantor => 4,
    _DispatchScreen.photoUpload => 5,
    _DispatchScreen.setupReview => 6,
    _ => 0,
  };

  /// Maps a 1-based setup step index (as returned by SetupHubScreen) to the
  /// corresponding screen in the flow.
  _DispatchScreen _setupStepToScreen(int step) {
    switch (step) {
      case 1:
        return _DispatchScreen.profileDetails;
      case 2:
        return _DispatchScreen.operationMode;
      case 3:
        return _DispatchScreen.emergencyContact;
      case 4:
        return _DispatchScreen.guarantor;
      case 5:
        return _DispatchScreen.photoUpload;
      case 6:
        return _DispatchScreen.setupReview;
      default:
        return _DispatchScreen.profileDetails;
    }
  }

  @override
  Widget build(BuildContext context) {
    // Rebuild the whole MaterialApp whenever the user flips the theme so
    // themeMode + ThemeData (and therefore every context.palette lookup)
    // reflect the new brightness immediately.
    return AnimatedBuilder(
      animation: widget.themeController,
      builder: (context, _) => MaterialApp(
        title: 'Karryam Driver',
        navigatorKey: _navigatorKey,
        debugShowCheckedModeBanner: false,
        theme: DispatchTheme.light,
        darkTheme: DispatchTheme.dark,
        themeMode: widget.themeController.mode,
        builder: (context, child) {
          return Stack(
            children: [
              if (child != null) child,
              // Global MOCK / LIVE toggle — appears on every screen.
              Positioned(
                top: MediaQuery.of(context).padding.top + 8,
                right: 12,
                child: const MockBadge(),
              ),
            ],
          );
        },
        home: _buildScreen(),
      ),
    );
  }

  Widget _buildScreen() {
    return switch (_screen) {
      _DispatchScreen.splash => SplashScreen(onDone: _checkSavedSession),

      _DispatchScreen.login => LoginScreen(
        controller: _authController,
        onContinue: (identifier) {
          _phone = identifier;
          _isLoginMode = true;
          _go(_DispatchScreen.otpVerification);
        },
        onCreateAccountTap: () => _go(_DispatchScreen.onboarding),
      ),

      _DispatchScreen.onboarding => OnboardingScreen(
        onDone: () => _go(_DispatchScreen.phoneEntry),
      ),

      _DispatchScreen.phoneEntry => PhoneEntryScreen(
        controller: _authController,
        onContinue: (phone, email) {
          _phone = phone;
          _isLoginMode = false;
          _go(_DispatchScreen.otpVerification);
        },
        onLoginTap: () => _go(_DispatchScreen.login),
      ),

      _DispatchScreen.otpVerification => OtpVerificationScreen(
        controller: _authController,
        phone: _phone,
        onVerify: (_) => _handlePostVerify(),
        onBack: () => _isLoginMode
            ? _go(_DispatchScreen.login)
            : _go(_DispatchScreen.phoneEntry),
        onResend: () {},
      ),

      _DispatchScreen.welcomeBack => WelcomeBackScreen(
        profileController: _profileController,
        onContinueSetup: () => _go(_DispatchScreen.setupHub),
        onUseDifferentAccount: _handleLogout,
      ),

      _DispatchScreen.setupHub => SetupHubScreen(
        profileController: _profileController,
        selectedOperationType: _selectedOperationType,
        onContinue: (step) => _go(_setupStepToScreen(step)),
        onBack: () => _go(_DispatchScreen.login),
      ),

      _DispatchScreen.profileDetails => ProfileDetailsScreen(
        initialPhone: _authController.currentPhoneNumber,
        initialEmail: _authController.currentSignupEmail,
        profileController: _profileController,
        operationType: _selectedOperationType,
        onContinue: (data) async {
          _profileDetailsData = data;
          _go(_DispatchScreen.operationMode);
        },
        onBack: () => _go(_DispatchScreen.setupHub),
        currentStep: _currentStep,
        totalSteps: _totalSteps,
      ),

      _DispatchScreen.operationMode => OperationModeScreen(
        onContinue: (mode) {
          // Backend accepts only 'individual' or 'fleet'.
          final backendValue =
              mode == 'fleet_operator' ? 'fleet' : 'individual';
          _selectedOperationType = backendValue;
          // NOTE: We do NOT call updateMe here — the backend rejects
          // operation_type on PATCH /provider/me (read-only field). The value
          // stays in memory until submitOnboarding on step 6, where it is
          // accepted and persisted.
          _go(_DispatchScreen.emergencyContact);
        },
        onBack: () => _go(_DispatchScreen.profileDetails),
        currentStep: _currentStep,
        totalSteps: _totalSteps,
      ),

      _DispatchScreen.emergencyContact => EmergencyContactScreen(
        initialData: _emergencyContactData,
        currentStep: _currentStep,
        totalSteps: _totalSteps,
        profileController: _profileController,
        onContinue: (data) {
          _emergencyContactData = data;
          _go(_DispatchScreen.guarantor);
        },
        onBack: () => _go(_DispatchScreen.operationMode),
      ),

      _DispatchScreen.guarantor => GuarantorScreen(
        initialData: _guarantorData,
        currentStep: _currentStep,
        totalSteps: _totalSteps,
        profileController: _profileController,
        onContinue: (data) {
          _guarantorData = data;
          _go(_DispatchScreen.photoUpload);
        },
        onBack: () => _go(_DispatchScreen.emergencyContact),
      ),

      _DispatchScreen.photoUpload => PhotoUploadScreen(
        onContinue: (selfiePath) async {
          debugPrint('[PHOTO_UPLOAD] Uploading selfie: $selfiePath');

          // Upload to POST /provider/me/avatar BEFORE advancing.
          final result = await _profileController.uploadAvatar(selfiePath);
          if (!mounted) return;

          String? err;
          result.when(
            success: (_) {
              _selfiePath = selfiePath;
            },
            failure: (error) {
              err = error.message.isNotEmpty
                  ? error.message
                  : 'Photo upload failed.';
            },
          );

          if (err != null) {
            // Rethrow so PhotoUploadScreen shows the error and stays on step 5.
            throw Exception(err);
          }

          debugPrint('[PHOTO_UPLOAD] Upload succeeded → setupReview');
          _go(_DispatchScreen.setupReview);
        },
        onBack: () => _go(_DispatchScreen.guarantor),
        currentStep: _currentStep,
        totalSteps: _totalSteps,
      ),

      _DispatchScreen.setupReview => SetupReviewScreen(
        profileController: _profileController,
        profileData: _profileDetailsData,
        emergencyContact: _emergencyContactData,
        guarantorData: _guarantorData,
        operationType: _selectedOperationType,
        selfiePath: _selfiePath,
        onBack: () => _go(_DispatchScreen.photoUpload),
        onSubmitted: () => _go(_DispatchScreen.verificationPending),
        onEditPersonal: () => _go(_DispatchScreen.profileDetails),
        onEditOperation: () => _go(_DispatchScreen.operationMode),
        onEditEmergency: () => _go(_DispatchScreen.emergencyContact),
        onEditGuarantor: () => _go(_DispatchScreen.guarantor),
        onEditPhoto: () => _go(_DispatchScreen.photoUpload),
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
        disputeController: _disputeController,
        onLogout: _handleLogout,
        onAccountDeleted: _handleAccountDeleted,
        onOpenNotifications: _openNotifications,
      ),

      // ── Legacy routes (not in active flow) ────────────────────────────────
      _DispatchScreen.accountType => AccountTypeScreen(
        onSelected: (type) => _go(_DispatchScreen.setupHub),
      ),
      _DispatchScreen.serviceType => ServiceTypeScreen(
        onContinue: (_) => _go(_DispatchScreen.setupHub),
        onBack: () => _go(_DispatchScreen.setupHub),
        currentStep: _currentStep,
        totalSteps: _totalSteps,
      ),
      _DispatchScreen.bikeType => BikeTypeScreen(
        onContinue: (_) => _go(_DispatchScreen.setupReview),
        onBack: () => _go(_DispatchScreen.setupReview),
        currentStep: _currentStep,
        totalSteps: _totalSteps,
      ),
      _DispatchScreen.driverInformation => DriverInformationScreen(
        profileController: _profileController,
        onSubmit: (_) async => _go(_DispatchScreen.verificationPending),
        onBack: () => _go(_DispatchScreen.setupReview),
        currentStep: _currentStep,
        totalSteps: _totalSteps,
      ),
    };
  }

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
          _go(_DispatchScreen.welcomeBack);
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

    if (isNetworkError) {
      debugPrint(
        '[AUTH] Network error during session restore → Home (tokens preserved)',
      );
      _go(_DispatchScreen.home);
      return;
    }

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
        try {
          await _tokenStorage.saveTokens(
            accessToken: newAccess!,
            refreshToken: newRefresh ?? saved.refreshToken ?? '',
            providerId: saved.providerId ?? '',
          );
        } catch (e) {
          debugPrint('[AUTH] Failed to persist refreshed tokens: $e');
        }

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
              _go(_DispatchScreen.welcomeBack);
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

    debugPrint(
      '[AUTH] Session restore failed — refresh exhausted → clearing tokens → Login',
    );
    await _handleSessionExpired();
  }

  Future<void> _handlePostVerify() async {
    debugPrint('[AUTH_FLOW] post verify | isLoginMode=$_isLoginMode');

    if (!_isLoginMode) {
      debugPrint('[AUTH_FLOW] signup flow → SetupHub');
      _go(_DispatchScreen.setupHub);
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
          ' | verification_status=${profile.verificationStatus}',
        );
        if (profile.onboardingComplete) {
          _go(_DispatchScreen.home);
        } else {
          _go(_DispatchScreen.welcomeBack);
        }
      },
      failure: (error) {
        debugPrint(
          '[AUTH_FLOW] /provider/me failed | code=${error.code} message=${error.message}',
        );
        _go(_DispatchScreen.home);
      },
    );
  }

  Future<void> _handleLogout() async {
    debugPrint('[AUTH] User initiated logout');
    await _authController.logout();
    if (!mounted) return;
    _profileController.clearLocalState();
    _go(_DispatchScreen.login);
  }

  Future<void> _handleAccountDeleted() async {
    debugPrint('[AUTH] Account deleted — clearing local session');
    await _authController.clearSession();
    if (!mounted) return;
    _profileController.clearLocalState();
    _go(_DispatchScreen.login);
  }

  void _go(_DispatchScreen screen) {
    setState(() => _screen = screen);
    if (screen == _DispatchScreen.home) {
      _startRealtimeAndPush();
    } else if (screen == _DispatchScreen.login) {
      _stopRealtimeAndPush();
    }
  }

  @override
  void dispose() {
    _fcmTokenRefreshSub?.cancel();
    _fcmForegroundSub?.cancel();
    _realtimeListener?.stop();
    super.dispose();
  }
}