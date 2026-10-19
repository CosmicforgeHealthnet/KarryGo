import '../core/theme/appearance_controller.dart';
import '../core/theme/karryam_theme.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'dart:async';
import 'dart:io' show Platform;

import 'package:flutter/foundation.dart';
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
import '../features/notifications/data/firebase_push.dart';
import '../features/onboarding/ui/onboarding_screen.dart';
import 'driver_services.dart';

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
  late final DriverServices _services;
  late final bool _ownsAuth;
  final _appearance = AppearanceController();

  // Push: registered once per signed-in session, re-registered on token
  // rotation. Any push (foreground, or a tapped tray notification) pulls the
  // latest offers immediately instead of waiting for the next poll.
  bool _pushRegistered = false;
  final List<StreamSubscription<Object?>> _pushSubs = [];

  @override
  void initState() {
    super.initState();
    _appearance.load();
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
    _services = DriverServices(config: config, auth: _auth);

    if (_ownsAuth) _auth.initialize();

    // Tests inject their own auth controller and have no Firebase plugin.
    if (_ownsAuth) {
      _auth.addListener(_onAuthChangedForPush);
      _pushSubs
        ..add(FirebasePush.tokenRefresh().listen((_) => _registerPushDevice()))
        ..add(FirebasePush.foregroundMessages().listen((_) => _pokeHome()))
        ..add(FirebasePush.openedMessages().listen((_) => _pokeHome()));
    }
  }

  void _onAuthChangedForPush() {
    if (_auth.state.status != ProviderAuthStatus.authenticated) {
      _pushRegistered = false;
      return;
    }
    if (_pushRegistered) return;
    _pushRegistered = true;
    _registerPushDevice();
  }

  Future<void> _registerPushDevice() async {
    final accessToken = _auth.state.accessToken;
    if (accessToken == null) return;
    try {
      final fcmToken = await FirebasePush.deviceToken();
      if (fcmToken == null || fcmToken.isEmpty) return;
      await _api.registerDevice(
        accessToken: accessToken,
        token: fcmToken,
        platform: Platform.isIOS ? 'ios' : 'android',
      );
    } catch (error) {
      // Registration failing must never break the app; foreground polling
      // stays the delivery path. Retried on next sign-in or token rotation.
      _pushRegistered = false;
      if (kDebugMode) debugPrint('push registration error: $error');
    }
  }

  void _pokeHome() {
    if (_auth.state.status == ProviderAuthStatus.authenticated) {
      unawaited(_home.refreshBookings());
    }
  }

  @override
  void dispose() {
    for (final sub in _pushSubs) {
      sub.cancel();
    }
    if (_ownsAuth) _auth.removeListener(_onAuthChangedForPush);
    _appearance.dispose();
    _home.dispose();
    if (_ownsAuth) _auth.dispose();
    _api.close();
    _authApi.close();
    _services.close();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AppearanceScope(
      controller: _appearance,
      child: AnimatedBuilder(
        animation: _appearance,
        builder: (context, _) => MaterialApp(
          title: 'Karryam Driver',
          debugShowCheckedModeBanner: false,
          theme: karryamTheme(Brightness.light),
          darkTheme: karryamTheme(Brightness.dark),
          themeMode: _appearance.mode,
          home: AnimatedBuilder(
            animation: _auth,
            builder: (context, _) {
              final state = _auth.state;
              return switch (state.status) {
                ProviderAuthStatus.checking => _Splash(),
                ProviderAuthStatus.phoneEntry => PhoneEntryScreen(
                  controller: _auth,
                ),
                ProviderAuthStatus.otpVerification => OtpScreen(
                  controller: _auth,
                ),
                // A new driver sets up their account before anything else.
                ProviderAuthStatus.authenticated
                    when state.provider?.needsOnboarding ?? false =>
                  OnboardingScreen(api: _services.onboarding, auth: _auth),
                ProviderAuthStatus.authenticated => ProviderHomeScreen(
                  controller: _home,
                  authController: _auth,
                  api: _api,
                  authApi: _authApi,
                  services: _services,
                ),
              };
            },
          ),
        ),
      ),
    );
  }
}

class _Splash extends StatelessWidget {
  const _Splash();

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Theme.of(context).scaffoldBackgroundColor,
      body: Center(child: CircularProgressIndicator()),
    );
  }
}
