import 'dart:async';
import 'dart:io' show Platform;

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';

import '../core/config/provider_app_config.dart';
import '../features/auth/data/provider_auth_api.dart';
import '../features/auth/data/provider_session_store.dart';
import '../features/auth/state/provider_auth_controller.dart';
import '../features/auth/ui/otp_screen.dart';
import '../features/auth/ui/auth_entry_screen.dart';
import '../features/earnings/state/provider_earnings_controller.dart';
import '../features/home/data/provider_api.dart';
import '../features/home/state/provider_home_controller.dart';
import '../features/home/ui/provider_home_screen.dart';
import '../features/media/data/media_file_api.dart';
import '../features/media/data/media_upload_service.dart';
import '../features/notifications/data/firebase_push.dart';
import '../features/notifications/data/provider_notification_api.dart';
import '../features/notifications/data/provider_realtime_listener.dart';
import '../features/notifications/state/provider_notification_controller.dart';
import '../features/profile/data/provider_profile_api.dart';
import '../features/profile/state/provider_profile_controller.dart';
import '../features/trips/state/provider_trips_controller.dart';
import '../features/disputes/data/provider_support_api.dart';
import '../features/disputes/state/provider_dispute_controller.dart';
import '../features/wallet/data/provider_wallet_api.dart';
import '../features/wallet/state/provider_withdrawal_controller.dart';
import '../features/onboarding/ui/driver_documents_screen.dart';
import '../features/onboarding/ui/operation_mode_screen.dart';
import '../features/onboarding/ui/personal_info_screen.dart';
import '../features/onboarding/ui/photo_upload_screen.dart';
import '../features/onboarding/ui/service_type_screen.dart';
import '../features/onboarding/ui/truck_info_screen.dart';
import '../features/onboarding/ui/verification_pending_screen.dart';

class TruckProviderApp extends StatefulWidget {
  const TruckProviderApp({super.key});

  @override
  State<TruckProviderApp> createState() => _TruckProviderAppState();
}

class _TruckProviderAppState extends State<TruckProviderApp>
    with WidgetsBindingObserver {
  late final ProviderAuthController _authController;
  late final ProviderHomeController _homeController;
  late final ProviderProfileController _profileController;
  late final ProviderEarningsController _earningsController;
  late final ProviderWithdrawalController _withdrawalController;
  late final ProviderDisputeController _disputeController;
  late final ProviderTripsController _tripsController;
  late final ProviderNotificationController _notificationController;
  late final ProviderAuthApi _authApi;
  late final ProviderApi _providerApi;
  late final ProviderProfileApi _profileApi;
  late final ProviderNotificationApi _notificationApi;
  late final ProviderWalletApi _walletApi;
  late final ProviderSupportApi _supportApi;
  late final MediaUploadService _mediaUploadService;

  bool _pushRegistered = false;
  StreamSubscription<String>? _fcmTokenRefreshSub;
  StreamSubscription<dynamic>? _fcmForegroundSub;

  @override
  void initState() {
    super.initState();

    final config = ProviderAppConfig.fromEnvironment();
    final coreConfig = ApiCoreConfig(baseUrl: config.haulingApiBaseUrl);

    _authApi = ProviderAuthApi(config: coreConfig);
    _providerApi = ProviderApi(config: coreConfig);
    _profileApi = ProviderProfileApi(config: coreConfig);
    _notificationApi = ProviderNotificationApi(config: coreConfig);
    _walletApi = ProviderWalletApi(
      config: ApiCoreConfig(baseUrl: config.paymentWalletApiBaseUrl),
    );
    _supportApi = ProviderSupportApi(
      config: ApiCoreConfig(baseUrl: config.supportApiBaseUrl),
    );

    _mediaUploadService = MediaUploadService(
      // Uploads go through hauling-service as the signed-in driver.
      api: MediaFileApi(
        config: ApiCoreConfig(baseUrl: config.haulingApiBaseUrl),
        accessToken: () => _authController.state.session?.accessToken,
        refreshSession: () => _authController.tryRefreshSession(),
      ),
    );

    _authController = ProviderAuthController(
      api: _authApi,
      sessionStore: SharedPrefsProviderSessionStore(),
      mediaUploadService: _mediaUploadService,
    );

    _notificationController = ProviderNotificationController(
      api: _notificationApi,
      accessToken: () => _authController.state.session?.accessToken,
    );

    _homeController = ProviderHomeController(
      api: _providerApi,
      authController: _authController,
      mediaUploadService: _mediaUploadService,
      realtimeListenerFactory: (accessToken, onEvent) => ProviderRealtimeListener(
        api: _notificationApi,
        wsUrl: config.notificationWsUrl,
        accessToken: accessToken,
        // Fan the pushed event out to the home controller (poll poke) and the
        // notification feed (live refresh) over the single shared websocket.
        onEvent: (eventType) {
          onEvent(eventType);
          _notificationController.onRealtimeEvent(eventType);
        },
      ),
    );

    _profileController = ProviderProfileController(
      api: _profileApi,
      authController: _authController,
      mediaUploadService: _mediaUploadService,
    );

    _earningsController = ProviderEarningsController(
      api: _providerApi,
      walletApi: _walletApi,
      accessToken: () => _authController.state.session?.accessToken,
    );

    _withdrawalController = ProviderWithdrawalController(
      api: _walletApi,
      accessToken: () => _authController.state.session?.accessToken,
    );

    _disputeController = ProviderDisputeController(
      api: _supportApi,
      accessToken: () => _authController.state.session?.accessToken,
    );

    _tripsController = ProviderTripsController(
      api: _providerApi,
      accessToken: () => _authController.state.session?.accessToken,
    );

    // FCM device registration: register once per login so background push
    // (new booking requests) reaches this device. Degrades to a no-op when
    // Firebase is not configured on this platform.
    _authController.addListener(_onAuthChangedForPush);
    _fcmTokenRefreshSub = FirebasePush.tokenRefresh().listen((_) {
      _pushRegistered = false;
      _onAuthChangedForPush();
    });
    // Foreground pushes show no system tray entry — poke the booking poll and
    // refresh the notification feed so the request appears immediately.
    _fcmForegroundSub = FirebasePush.foregroundMessages().listen((message) {
      _homeController.pokeRefresh();
      _notificationController.onRealtimeEvent(
        (message.data['event_type'] as String?) ?? '',
      );
    });

    WidgetsBinding.instance.addObserver(this);
    WidgetsBinding.instance.addPostFrameCallback((_) async {
      await _authController.initialize();
      if (_authController.state.status == ProviderAuthStatus.authenticated) {
        _homeController.restoreOnlineStatus();
      }
    });
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    // The online marker in Redis expires ~90s after the last heartbeat, and the
    // OS suspends our timer while backgrounded — so a provider who backgrounds
    // the app for a few minutes is dropped from the matching pool server-side
    // while the toggle still reads "Online". Re-check with the server on resume
    // so the UI reflects reality (and re-arms the heartbeat if still online).
    if (state == AppLifecycleState.resumed &&
        _authController.state.status == ProviderAuthStatus.authenticated) {
      _homeController.syncOnlineStatus();
    }
  }

  void _onAuthChangedForPush() {
    final authenticated = _authController.state.status == ProviderAuthStatus.authenticated;
    if (!authenticated) {
      _pushRegistered = false;
      return;
    }
    if (_pushRegistered) return;
    _pushRegistered = true;
    _registerPushDevice();
  }

  Future<void> _registerPushDevice() async {
    final accessToken = _authController.state.session?.accessToken;
    if (accessToken == null) return;
    try {
      final fcmToken = await FirebasePush.deviceToken();
      if (fcmToken == null || fcmToken.isEmpty) return;
      await _notificationApi.registerDevice(
        accessToken: accessToken,
        token: fcmToken,
        platform: Platform.isIOS ? 'ios' : 'android',
      );
    } catch (error) {
      // Registration failing must never break the app; the websocket + poll
      // remain the delivery path. Retried on next login/token refresh.
      _pushRegistered = false;
      if (kDebugMode) debugPrint('push registration error: $error');
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _fcmTokenRefreshSub?.cancel();
    _fcmForegroundSub?.cancel();
    _authController.removeListener(_onAuthChangedForPush);
    _authController.dispose();
    _homeController.dispose();
    _profileController.dispose();
    _earningsController.dispose();
    _withdrawalController.dispose();
    _disputeController.dispose();
    _tripsController.dispose();
    _notificationController.dispose();
    _authApi.close();
    _providerApi.close();
    _profileApi.close();
    _notificationApi.close();
    _walletApi.close();
    _supportApi.close();
    _mediaUploadService.close();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Truck Provider',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorSchemeSeed: const Color(0xFF05124D),
        useMaterial3: true,
        fontFamily: 'Poppins',
      ),
      home: AnimatedBuilder(
        animation: _authController,
        builder: (context, _) {
          final state = _authController.state;
          return switch (state.status) {
            ProviderAuthStatus.checking => const _SplashScreen(),

            ProviderAuthStatus.phoneEntry => ProviderAuthEntryScreen(
              controller: _authController,
            ),

            ProviderAuthStatus.otpVerification => ProviderOtpScreen(
              controller: _authController,
            ),

            ProviderAuthStatus.serviceTypeSelection => ServiceTypeScreen(
              controller: _authController,
            ),

            ProviderAuthStatus.operationMode => OperationModeScreen(
              controller: _authController,
            ),

            ProviderAuthStatus.personalInfo => PersonalInfoScreen(
              controller: _authController,
            ),

            ProviderAuthStatus.driverDocuments => DriverDocumentsScreen(
              controller: _authController,
            ),

            ProviderAuthStatus.truckInfo => TruckInfoScreen(
              controller: _authController,
            ),

            ProviderAuthStatus.photoUpload => PhotoUploadScreen(
              controller: _authController,
            ),

            ProviderAuthStatus.verificationPending => VerificationPendingScreen(
              controller: _authController,
            ),

            ProviderAuthStatus.authenticated => ProviderHomeScreen(
              authController: _authController,
              homeController: _homeController,
              profileController: _profileController,
              earningsController: _earningsController,
              withdrawalController: _withdrawalController,
              disputeController: _disputeController,
              tripsController: _tripsController,
              notificationController: _notificationController,
            ),
          };
        },
      ),
    );
  }
}

class _SplashScreen extends StatelessWidget {
  const _SplashScreen();

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Container(
        width: double.infinity,
        height: double.infinity,
        decoration: const BoxDecoration(
          gradient: RadialGradient(
            center: Alignment(-0.3, -0.3),
            radius: 1.2,
            colors: [Color(0xFF2761A7), Color(0xFF0A1F5C), Color(0xFF05124D)],
            stops: [0.0, 0.5, 1.0],
          ),
        ),
        child: Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Image.asset('assets/figma/splash_logo.png', width: 180),
              const SizedBox(height: 12),
              const Text(
                'Cosmicforge Logistics',
                style: TextStyle(
                  color: Colors.white,
                  fontSize: 22,
                  fontWeight: FontWeight.w700,
                  letterSpacing: 1.2,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
