import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../core/config/customer_app_config.dart';
import '../core/cosmicforge_logistics_app_theme.dart';
import '../features/auth/data/customer_auth_api.dart';
import '../features/auth/data/customer_session_store.dart';
import '../features/auth/state/customer_auth_controller.dart';
import '../features/hauling/data/customer_realtime_listener.dart';
import '../features/hauling/data/hauling_api.dart';
import '../features/hauling/data/places_api.dart';
import '../features/hauling/state/hauling_booking_controller.dart';
import '../features/media/data/media_file_api.dart';
import '../features/media/data/media_upload_service.dart';
import '../features/notifications/data/notification_api.dart';
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
import '../features/profile_setup/ui/service_choice_screen.dart';

class CustomerApp extends StatefulWidget {
  const CustomerApp({super.key, this.controller, this.autoInitialize = true});

  final CustomerAuthController? controller;
  final bool autoInitialize;

  @override
  State<CustomerApp> createState() => _CustomerAppState();
}

class _CustomerAppState extends State<CustomerApp> {
  late final CustomerAuthController _controller;
  late final bool _ownsController;
  late final SupportApi _supportApi;
  late final WalletApi _walletApi;
  late final CustomerAuthApi _authApi;
  late final MediaUploadService _mediaUploadService;
  late final HaulingBookingController _haulingController;
  late final PlacesApi _placesApi;
  late final NotificationApi _notificationApi;
  late final NotificationController _notificationController;

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
    if (_ownsController) _controller.dispose();
    _haulingController.dispose();
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
      home: AnimatedBuilder(
        animation: _controller,
        builder: (context, _) {
          final state = _controller.state;
          // Open the realtime feed once authenticated; tear it down otherwise.
          if (state.status == CustomerAuthStatus.authenticated) {
            _notificationController.start();
          } else {
            _notificationController.stop();
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
            CustomerAuthStatus.serviceChoice => ServiceChoiceScreen(
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
    );

    // Notifications proxy lives on the customer API base; the websocket connects
    // directly to notification-service.
    _notificationApi = NotificationApi(
      config: ApiCoreConfig(baseUrl: config.customerApiBaseUrl),
      onAuthFailure: _onAuthFailure,
    );
    _notificationController = NotificationController(
      api: _notificationApi,
      authController: _controller,
      wsUrl: config.notificationWsUrl,
    );
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
