import 'dart:async';
import 'dart:io' show Platform;

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';

import '../core/provider_appearance.dart';
import '../core/theme/karryam_theme.dart';
import '../core/widgets/karryam_ui.dart';
import '../features/referrals/data/referral_api.dart';
import '../features/referrals/state/referral_controller.dart';
import '../features/home/ui/widgets/provider_app_colors.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter/services.dart';

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
  late final ProviderReferralController _referralController;
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
  StreamSubscription<dynamic>? _fcmOpenedSub;
  final _navigatorKey = GlobalKey<NavigatorState>();

  /// Tab the home shell should show next; set when a tapped push asks for one.
  final _homeTabRequest = ValueNotifier<int?>(null);

  @override
  void initState() {
    super.initState();
    providerAppearance.addListener(_onAppearanceChanged);
    providerAppearance.restore();

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

    _referralController = ProviderReferralController(
      api: ReferralApi(config: ApiCoreConfig(baseUrl: config.referralApiBaseUrl)),
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

    // A tapped tray notification opens the screen it is about.
    _fcmOpenedSub = FirebasePush.openedMessages().listen(_openFromPush);

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
    _fcmOpenedSub?.cancel();
    _homeTabRequest.dispose();
    _authController.removeListener(_onAuthChangedForPush);
    _authController.dispose();
    _homeController.dispose();
    _profileController.dispose();
    _earningsController.dispose();
    _withdrawalController.dispose();
    _disputeController.dispose();
    _referralController.dispose();
    _tripsController.dispose();
    _notificationController.dispose();
    providerAppearance.removeListener(_onAppearanceChanged);
    _authApi.close();
    _providerApi.close();
    _profileApi.close();
    _notificationApi.close();
    _walletApi.close();
    _supportApi.close();
    _mediaUploadService.close();
    super.dispose();
  }

  /// Refreshes, closes anything open on top of the home shell, and shows the
  /// tab the notification is about. An active trip takes over Home by itself.
  void _openFromPush(RemoteMessage message) {
    _homeController.pokeRefresh();
    final type = ((message.data['event_type'] as String?) ?? '').toLowerCase();
    _notificationController.onRealtimeEvent(type);
    if (_authController.state.status != ProviderAuthStatus.authenticated) return;
    _navigatorKey.currentState?.popUntil((route) => route.isFirst);
    _homeTabRequest.value = switch (type) {
      _ when type.contains('matched') || type.contains('request') => 1,
      _ when type.contains('withdraw') || type.contains('payout') || type.contains('payment') => 3,
      _ => 0,
    };
  }

  void _onAppearanceChanged() => _repaintAll();

  @override
  void didChangePlatformBrightness() => _repaintAll();

  /// The colour tokens are read at build time, so a theme switch rebuilds
  /// every element (navigation stack included) instead of just the theme
  /// dependants.
  void _repaintAll() {
    if (!mounted) return;
    setState(() {});
    void mark(Element e) {
      e.markNeedsBuild();
      e.visitChildren(mark);
    }

    (context as Element).visitChildren(mark);
  }

  Brightness get _brightness => switch (providerAppearance.value) {
        ThemeMode.light => Brightness.light,
        ThemeMode.dark => Brightness.dark,
        ThemeMode.system => WidgetsBinding.instance.platformDispatcher.platformBrightness,
      };

  @override
  Widget build(BuildContext context) {
    final brightness = _brightness;
    KPalette.apply(brightness);
    final dark = brightness == Brightness.dark;
    return MaterialApp(
      title: 'Karryam Trucks',
      debugShowCheckedModeBanner: false,
      navigatorKey: _navigatorKey,
      theme: karryamTruckTheme(brightness),
      builder: (context, child) => AnnotatedRegion<SystemUiOverlayStyle>(
        value: (dark ? SystemUiOverlayStyle.light : SystemUiOverlayStyle.dark)
            .copyWith(statusBarColor: Colors.transparent),
        child: child!,
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
              referralController: _referralController,
              tripsController: _tripsController,
              notificationController: _notificationController,
              tabRequests: _homeTabRequest,
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
          gradient: LinearGradient(
            begin: Alignment.topLeft,
            end: Alignment.bottomRight,
            colors: [Color(0xFF173468), kNavy, kNavyDeep],
          ),
        ),
        child: SafeArea(
          child: Column(
            children: [
              const Spacer(flex: 3),
              Image.asset('assets/brand/karryam-logo.png', width: 190, semanticLabel: 'Karryam'),
              const SizedBox(height: 36),
              const KTruckIllustration(width: 210, onDark: true),
              const SizedBox(height: 28),
              const Text(
                'Karryam Trucks',
                style: TextStyle(color: Colors.white, fontSize: 24, fontWeight: FontWeight.w700, letterSpacing: -0.4),
              ),
              const SizedBox(height: 6),
              const Text(
                'Carry more. Go further.',
                style: TextStyle(color: Color(0xFF99E7DC), fontSize: 14, fontWeight: FontWeight.w500),
              ),
              const Spacer(flex: 3),
              const SizedBox(
                width: 22,
                height: 22,
                child: CircularProgressIndicator(strokeWidth: 2.4, color: kTeal),
              ),
              const SizedBox(height: 40),
            ],
          ),
        ),
      ),
    );
  }
}
