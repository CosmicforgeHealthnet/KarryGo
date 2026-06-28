import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
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
import '../features/notifications/data/provider_notification_api.dart';
import '../features/notifications/data/provider_realtime_listener.dart';
import '../features/profile/data/provider_profile_api.dart';
import '../features/profile/state/provider_profile_controller.dart';
import '../features/trips/state/provider_trips_controller.dart';
import '../features/disputes/data/provider_support_api.dart';
import '../features/disputes/state/provider_dispute_controller.dart';
import '../features/wallet/data/provider_wallet_api.dart';
import '../features/wallet/state/provider_withdrawal_controller.dart';
import '../features/onboarding/ui/account_type_screen.dart';
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

class _TruckProviderAppState extends State<TruckProviderApp> {
  late final ProviderAuthController _authController;
  late final ProviderHomeController _homeController;
  late final ProviderProfileController _profileController;
  late final ProviderEarningsController _earningsController;
  late final ProviderWithdrawalController _withdrawalController;
  late final ProviderDisputeController _disputeController;
  late final ProviderTripsController _tripsController;
  late final ProviderAuthApi _authApi;
  late final ProviderApi _providerApi;
  late final ProviderProfileApi _profileApi;
  late final ProviderNotificationApi _notificationApi;
  late final ProviderWalletApi _walletApi;
  late final ProviderSupportApi _supportApi;
  late final MediaUploadService _mediaUploadService;

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
      api: MediaFileApi(
        config: ApiCoreConfig(baseUrl: config.mediaFileApiBaseUrl),
        serviceToken: config.mediaFileServiceToken,
        ownerService: ProviderAppConfig.mediaOwnerService,
      ),
    );

    _authController = ProviderAuthController(
      api: _authApi,
      sessionStore: SharedPrefsProviderSessionStore(),
      mediaUploadService: _mediaUploadService,
    );

    _homeController = ProviderHomeController(
      api: _providerApi,
      authController: _authController,
      realtimeListenerFactory: (accessToken, onEvent) => ProviderRealtimeListener(
        api: _notificationApi,
        wsUrl: config.notificationWsUrl,
        accessToken: accessToken,
        onEvent: onEvent,
      ),
    );

    _profileController = ProviderProfileController(
      api: _profileApi,
      authController: _authController,
      mediaUploadService: _mediaUploadService,
    );

    _earningsController = ProviderEarningsController(
      api: _providerApi,
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

    WidgetsBinding.instance.addPostFrameCallback((_) async {
      await _authController.initialize();
      if (_authController.state.status == ProviderAuthStatus.authenticated) {
        _homeController.restoreOnlineStatus();
      }
    });
  }

  @override
  void dispose() {
    _authController.dispose();
    _homeController.dispose();
    _profileController.dispose();
    _earningsController.dispose();
    _withdrawalController.dispose();
    _disputeController.dispose();
    _tripsController.dispose();
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
      title: 'Karry Go Provider',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorSchemeSeed: const Color(0xFF22A84A),
        useMaterial3: true,
        fontFamily: 'Inter',
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

            ProviderAuthStatus.accountTypeSelection => AccountTypeScreen(
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
    return const Scaffold(
      backgroundColor: Color(0xFFF7F8F7),
      body: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.local_shipping_rounded,
              size: 72,
              color: Color(0xFF22A84A),
            ),
            SizedBox(height: 16),
            Text(
              'Karry Go',
              style: TextStyle(
                color: Color(0xFF22A84A),
                fontSize: 28,
                fontWeight: FontWeight.w800,
              ),
            ),
            SizedBox(height: 8),
            Text(
              'Provider',
              style: TextStyle(color: Color(0xFF7B827C), fontSize: 14),
            ),
          ],
        ),
      ),
    );
  }
}
