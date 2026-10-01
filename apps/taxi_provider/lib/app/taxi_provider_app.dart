import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
import 'package:flutter/material.dart';

import '../core/config/taxi_provider_config.dart';
import '../features/auth/data/provider_auth_api.dart';
import '../features/auth/data/provider_session_store.dart';
import '../features/auth/state/provider_auth_controller.dart';
import '../features/auth/ui/otp_screen.dart';
import '../features/auth/ui/phone_entry_screen.dart';
import '../features/home/data/provider_api.dart';
import '../features/home/state/provider_home_controller.dart';
import '../features/home/ui/provider_home_screen.dart';

/// Root of the taxi driver app. Routes purely on auth status; everything
/// below the home screen is driven by ProviderHomeController.
class TaxiProviderApp extends StatefulWidget {
  const TaxiProviderApp({super.key, this.authController});

  /// Injected by tests; production builds construct their own.
  final ProviderAuthController? authController;

  @override
  State<TaxiProviderApp> createState() => _TaxiProviderAppState();
}

class _TaxiProviderAppState extends State<TaxiProviderApp> {
  late final ProviderAuthApi _authApi;
  late final ProviderApi _api;
  late final ProviderAuthController _auth;
  late final ProviderHomeController _home;
  late final bool _ownsAuth;

  @override
  void initState() {
    super.initState();
    final config = TaxiProviderConfig.fromEnvironment();
    final apiConfig = ApiCoreConfig(baseUrl: config.taxiApiBaseUrl);

    _ownsAuth = widget.authController == null;

    // The auth-failure hook is wired after construction because it needs the
    // controller it drops the session on.
    _authApi = ProviderAuthApi(
      config: apiConfig,
      onAuthFailure: () => _auth.handleAuthFailure(),
    );
    _api = ProviderApi(
      config: apiConfig,
      onAuthFailure: () => _auth.handleAuthFailure(),
    );
    _auth =
        widget.authController ??
        ProviderAuthController(
          api: _authApi,
          sessionStore: SharedPrefsProviderSessionStore(),
        );
    _home = ProviderHomeController(
      api: _api,
      authApi: _authApi,
      authController: _auth,
    );

    if (_ownsAuth) _auth.initialize();
  }

  @override
  void dispose() {
    _home.dispose();
    if (_ownsAuth) _auth.dispose();
    _api.close();
    _authApi.close();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Karry Go Driver',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        useMaterial3: true,
        colorScheme: ColorScheme.fromSeed(
          seedColor: CosmicforgeLogisticsColors.primary,
        ),
        scaffoldBackgroundColor: CosmicforgeLogisticsColors.surface,
      ),
      home: AnimatedBuilder(
        animation: _auth,
        builder: (context, _) {
          final state = _auth.state;
          return switch (state.status) {
            ProviderAuthStatus.checking => const _Splash(),
            ProviderAuthStatus.phoneEntry => PhoneEntryScreen(
              controller: _auth,
            ),
            ProviderAuthStatus.otpVerification => OtpScreen(controller: _auth),
            ProviderAuthStatus.authenticated => ProviderHomeScreen(
              controller: _home,
              authController: _auth,
              api: _api,
              authApi: _authApi,
            ),
          };
        },
      ),
    );
  }
}

class _Splash extends StatelessWidget {
  const _Splash();

  @override
  Widget build(BuildContext context) {
    return const Scaffold(
      backgroundColor: CosmicforgeLogisticsColors.surface,
      body: Center(child: CircularProgressIndicator()),
    );
  }
}
