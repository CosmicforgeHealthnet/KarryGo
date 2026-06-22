import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../core/config/provider_app_config.dart';
import '../features/auth/data/provider_auth_api.dart';
import '../features/auth/data/provider_session_store.dart';
import '../features/auth/state/provider_auth_controller.dart';
import '../features/auth/ui/otp_screen.dart';
import '../features/auth/ui/phone_entry_screen.dart';
import '../features/home/data/provider_api.dart';
import '../features/home/state/provider_home_controller.dart';
import '../features/home/ui/provider_home_screen.dart';
import '../features/media/data/media_file_api.dart';
import '../features/media/data/media_upload_service.dart';
import '../features/notifications/data/provider_notification_api.dart';
import '../features/notifications/data/provider_realtime_listener.dart';
import '../features/onboarding/ui/account_type_screen.dart';
import '../features/onboarding/ui/driver_documents_screen.dart';
import '../features/onboarding/ui/operation_mode_screen.dart';
import '../features/onboarding/ui/personal_info_screen.dart';
import '../features/onboarding/ui/photo_upload_screen.dart';
import '../features/onboarding/ui/service_type_screen.dart';
import '../features/onboarding/ui/verification_pending_screen.dart';

class TruckProviderApp extends StatefulWidget {
  const TruckProviderApp({super.key});

  @override
  State<TruckProviderApp> createState() => _TruckProviderAppState();
}

class _TruckProviderAppState extends State<TruckProviderApp> {
  late final ProviderAuthController _authController;
  late final ProviderHomeController _homeController;
  late final ProviderAuthApi _authApi;
  late final ProviderApi _providerApi;
  late final ProviderNotificationApi _notificationApi;
  late final MediaUploadService _mediaUploadService;

  @override
  void initState() {
    super.initState();

    final config = ProviderAppConfig.fromEnvironment();
    final coreConfig = ApiCoreConfig(baseUrl: config.haulingApiBaseUrl);

    _authApi = ProviderAuthApi(config: coreConfig);
    _providerApi = ProviderApi(config: coreConfig);
    _notificationApi = ProviderNotificationApi(config: coreConfig);

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

    WidgetsBinding.instance.addPostFrameCallback((_) {
      _authController.initialize();
    });
  }

  @override
  void dispose() {
    _authController.dispose();
    _homeController.dispose();
    _authApi.close();
    _providerApi.close();
    _notificationApi.close();
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

            ProviderAuthStatus.phoneEntry => ProviderPhoneEntryScreen(
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

            ProviderAuthStatus.photoUpload => PhotoUploadScreen(
              controller: _authController,
            ),

            ProviderAuthStatus.verificationPending => VerificationPendingScreen(
              controller: _authController,
            ),

            ProviderAuthStatus.authenticated => ProviderHomeScreen(
              authController: _authController,
              homeController: _homeController,
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
