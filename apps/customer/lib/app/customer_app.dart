import 'dart:async';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../core/config/customer_app_config.dart';
import '../core/cosmicforge_logistics_app_theme.dart';
import '../features/auth/data/customer_auth_api.dart';
import '../features/auth/data/customer_session_store.dart';
import '../features/auth/models/customer_auth_models.dart';
import '../features/auth/state/customer_auth_controller.dart';
import '../features/dispatch/data/dispatch_api.dart';
import '../features/dispatch/state/dispatch_booking_controller.dart';
import '../features/taxi/data/taxi_api.dart';
import '../features/taxi/state/taxi_booking_controller.dart';
import '../features/hauling/data/customer_realtime_listener.dart';
import '../features/hauling/data/hauling_api.dart';
import '../features/hauling/data/places_api.dart';
import '../features/hauling/state/hauling_booking_controller.dart';
import '../features/media/data/media_file_api.dart';
import '../features/media/data/media_upload_service.dart';
import '../features/notifications/data/firebase_push.dart';
import '../features/notifications/data/notification_api.dart';
import '../features/notifications/data/push_registration_service.dart';
import '../features/notifications/state/notification_controller.dart';
import '../features/support/data/support_api.dart';
import '../features/wallet/data/wallet_api.dart';
import '../features/auth/ui/otp_verification_screen.dart';
import '../features/auth/ui/phone_entry_screen.dart';
import '../features/auth/ui/splash_screen.dart';
import '../features/home/ui/customer_home_screen.dart';
import '../features/onboarding/ui/onboarding_screen.dart';
import '../features/profile_setup/ui/all_set_screen.dart';
import '../features/profile_setup/ui/photo_upload_screen.dart';
import '../features/profile_setup/ui/profile_details_screen.dart';

class CustomerApp extends StatefulWidget {
  const CustomerApp({super.key, this.controller, this.autoInitialize = true});

  final CustomerAuthController? controller;
  final bool autoInitialize;

  @override
  State<CustomerApp> createState() => _CustomerAppState();
}

class _CustomerAppState extends State<CustomerApp> {
  // Root navigator key so a session-end (logout / 401 expiry) can pop any
  // pushed screens (booking flow, profile edit, …) and reveal the login screen
  // underneath, instead of leaving the cleared session hidden behind them.
  final GlobalKey<NavigatorState> _navigatorKey = GlobalKey<NavigatorState>();

  late final CustomerAuthController _controller;
  late final bool _ownsController;
  late final SupportApi _supportApi;
  late final WalletApi _walletApi;
  late final CustomerAuthApi _authApi;
  late final MediaUploadService _mediaUploadService;
  late final HaulingBookingController _haulingController;
  late final DispatchBookingController _dispatchController;
  late final TaxiBookingController _taxiController;
  late final PlacesApi _placesApi;
  late final NotificationApi _notificationApi;
  late final NotificationController _notificationController;
  StreamSubscription<String>? _fcmTokenRefreshSub;
  StreamSubscription<dynamic>? _fcmForegroundSub;

  @override
  void initState() {
    super.initState();


    _ownsController = widget.controller == null;
    if (_ownsController) {
      _controller = _buildController();
    } else {
      _initApis();
      _controller = widget.controller!;
    }

    
    _initHaulingController();

    if (widget.autoInitialize) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        _controller.initialize();
      });
    }
  }

  @override
  void dispose() {
    _fcmTokenRefreshSub?.cancel();
    _fcmForegroundSub?.cancel();
    if (_ownsController) _controller.dispose();
    _haulingController.dispose();
    _dispatchController.dispose();
    _taxiController.dispose();
    _notificationController.dispose();
    _notificationApi.close();
    _supportApi.close();
    _walletApi.close();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Cosmicforge Logistics',
      debugShowCheckedModeBanner: false,
      theme: buildCosmicforgeLogisticsAppTheme(),
      navigatorKey: _navigatorKey,
      home: AnimatedBuilder(
        animation: _controller,
        builder: (context, _) {
          final state = _controller.state;
          // Open the realtime feed once authenticated; tear it down otherwise.
          if (state.status == CustomerAuthStatus.authenticated) {
            _notificationController.start();
            // Resume a booking left in progress if the app was closed mid-trip
            // (both calls are idempotent — see restoreActiveBookingIfAny).
            _haulingController.restoreActiveBookingIfAny();
            _dispatchController.restoreActiveBookingIfAny();
          } else {
            _notificationController.stop();
            // Session ended (logout or expired-token 401): drop any pushed
            // routes so the pre-auth screen this builder now returns is
            // actually visible rather than hidden behind a stale screen.
            WidgetsBinding.instance.addPostFrameCallback((_) {
              _navigatorKey.currentState?.popUntil((route) => route.isFirst);
            });
          }
          return switch (state.status) {
            CustomerAuthStatus.checking => const SplashScreen(),
            CustomerAuthStatus.onboarding => OnboardingScreen(
              controller: _controller,
            ),
            CustomerAuthStatus.phoneEntry => PhoneEntryScreen(
              controller: _controller,
              state: state,
            ),
            CustomerAuthStatus.otpVerification => OtpVerificationScreen(
              controller: _controller,
              state: state,
            ),
            CustomerAuthStatus.profileDetails => ProfileDetailsScreen(
              controller: _controller,
              state: state,
            ),
            CustomerAuthStatus.photoUpload => PhotoUploadScreen(
              controller: _controller,
              state: state,
            ),
            CustomerAuthStatus.allSet => AllSetScreen(
              controller: _controller,
              state: state,
            ),
            CustomerAuthStatus.authenticated => CustomerHomeScreen(
              controller: _controller,
              state: state,
              authApi: _authApi,
              supportApi: _supportApi,
              walletApi: _walletApi,
              mediaUploadService: _mediaUploadService,
              haulingController: _haulingController,
              dispatchController: _dispatchController,
              taxiController: _taxiController,
              placesApi: _placesApi,
              notificationController: _notificationController,
            ),
          };
        },
      ),
    );
  }

  // Lazily reaches the controller so a 401 on any authenticated request drives
  // a global logout. Safe even though APIs are built before `_controller` is
  // assigned in the owns-controller path — it is only ever called in response
  // to a network call, long after construction.
  void _onAuthFailure() {
    _controller.handleAuthFailure();
  }

  void _initApis() {
    final config = CustomerAppConfig.fromEnvironment();
    _authApi = CustomerAuthApi(
      config: ApiCoreConfig(baseUrl: config.customerApiBaseUrl),
      onAuthFailure: _onAuthFailure,
    );
    _supportApi = SupportApi(
      config: ApiCoreConfig(baseUrl: config.supportApiBaseUrl),
      onAuthFailure: _onAuthFailure,
    );
    _walletApi = WalletApi(
      config: ApiCoreConfig(baseUrl: config.walletApiBaseUrl),
      onAuthFailure: _onAuthFailure,
    );
    final mediaApi = MediaFileApi(
      config: ApiCoreConfig(baseUrl: config.mediaFileApiBaseUrl),
      serviceToken: config.mediaFileServiceToken,
    );
    _mediaUploadService = MediaUploadService(api: mediaApi);

  }

  void _initHaulingController() {
    final config = CustomerAppConfig.fromEnvironment();
    _placesApi = PlacesApi(apiKey: config.googleMapsApiKey);
    final haulingApi = HaulingApi(
      config: ApiCoreConfig(baseUrl: config.haulingApiBaseUrl),
      onAuthFailure: _onAuthFailure,
    );
    _haulingController = HaulingBookingController(
      api: haulingApi,
      authController: _controller,
      walletApi: _walletApi,
      realtimeListenerFactory: (accessToken, onEvent) => CustomerRealtimeListener(
        fetchToken: (token) => haulingApi.fetchRealtimeToken(accessToken: token),
        wsUrl: config.notificationWsUrl,
        accessToken: accessToken,
        onEvent: onEvent,
      ),
      savedLocationsLoader: () async {
        final session = _controller.state.session;
        if (session == null) return const <SavedLocation>[];
        return _authApi.getSavedLocations(accessToken: session.accessToken);
      },
    );

    // Bike delivery (dispatch) flow. Realtime mirrors hauling: the token is
    // minted by the dispatch-service notification proxy, the websocket connects
    // directly to notification-service; polling remains the fallback.
    final dispatchApi = DispatchApi(
      config: ApiCoreConfig(baseUrl: config.dispatchApiBaseUrl),
      onAuthFailure: _onAuthFailure,
    );
    _dispatchController = DispatchBookingController(
      api: dispatchApi,
      authController: _controller,
      walletApi: _walletApi,
      realtimeListenerFactory: (accessToken, onEvent) => CustomerRealtimeListener(
        fetchToken: (token) => dispatchApi.fetchRealtimeToken(accessToken: token),
        wsUrl: config.notificationWsUrl,
        accessToken: accessToken,
        onEvent: onEvent,
      ),
      savedLocationsLoader: () async {
        final session = _controller.state.session;
        if (session == null) return const <SavedLocation>[];
        return _authApi.getSavedLocations(accessToken: session.accessToken);
      },
    );

    // Car ride (taxi) flow. No realtime listener: taxi-service exposes
    // neither a customer realtime channel nor a customer-facing
    // driver-location endpoint, so this controller polls only. It does take
    // the wallet client -- rides are wallet-only, like hauling and dispatch.
    final taxiApi = TaxiApi(
      config: ApiCoreConfig(baseUrl: config.taxiApiBaseUrl),
      onAuthFailure: _onAuthFailure,
    );
    _taxiController = TaxiBookingController(
      api: taxiApi,
      authController: _controller,
      walletApi: _walletApi,
      savedLocationsLoader: () async {
        final session = _controller.state.session;
        if (session == null) return const <SavedLocation>[];
        return _authApi.getSavedLocations(accessToken: session.accessToken);
      },
    );

    // Notifications proxy lives on the customer API base; the websocket connects
    // directly to notification-service.
    _notificationApi = NotificationApi(
      config: ApiCoreConfig(baseUrl: config.customerApiBaseUrl),
      onAuthFailure: _onAuthFailure,
    );
    // FCM device registration: NotificationController.start() registers the
    // token once authenticated. Degrades to a no-op when Firebase is not
    // configured on this platform (e.g. iOS until it is registered).
    final pushRegistration = PushRegistrationService(
      api: _notificationApi,
      tokenProvider: FirebasePush.deviceToken,
    );
    _notificationController = NotificationController(
      api: _notificationApi,
      authController: _controller,
      wsUrl: config.notificationWsUrl,
      pushRegistration: pushRegistration,
    );
    // Rotated FCM tokens must be re-registered or background push goes dark.
    _fcmTokenRefreshSub = FirebasePush.tokenRefresh().listen((_) {
      final accessToken = _controller.state.session?.accessToken;
      if (accessToken != null) pushRegistration.register(accessToken: accessToken);
    });
    // Foreground pushes show no system tray entry — refresh the in-app feed and
    // poke the booking poll so the update is visible immediately.
    _fcmForegroundSub = FirebasePush.foregroundMessages().listen((_) {
      _notificationController.loadFeed();
      _haulingController.pokeRefresh();
      _dispatchController.pokeRefresh();
      _taxiController.pokeRefresh();
    });
  }

  CustomerAuthController _buildController() {
    _initApis();
    return CustomerAuthController(
      api: _authApi,
      sessionStore: SecureCustomerSessionStore(),
      mediaUploadService: _mediaUploadService,
    );
  }
}
