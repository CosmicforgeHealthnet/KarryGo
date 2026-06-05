import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../core/config/customer_app_config.dart';
import '../core/cosmicforge_logistics_app_theme.dart';
import '../features/auth/data/customer_auth_api.dart';
import '../features/auth/data/customer_session_store.dart';
import '../features/auth/state/customer_auth_controller.dart';
import '../features/auth/ui/otp_verification_screen.dart';
import '../features/auth/ui/phone_entry_screen.dart';
import '../features/auth/ui/profile_required_screen.dart';
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

  @override
  void initState() {
    super.initState();

    _ownsController = widget.controller == null;
    _controller = widget.controller ?? _buildController();

    if (widget.autoInitialize) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        _controller.initialize();
      });
    }
  }

  @override
  void dispose() {
    if (_ownsController) {
      _controller.dispose();
    }
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
            CustomerAuthStatus.profileRequired => ProfileRequiredScreen(
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
            ),
          };
        },
      ),
    );
  }

  CustomerAuthController _buildController() {
    final config = CustomerAppConfig.fromEnvironment();
    final api = CustomerAuthApi(
      config: ApiCoreConfig(baseUrl: config.customerApiBaseUrl),
    );
    return CustomerAuthController(
      api: api,
      sessionStore: SecureCustomerSessionStore(),
    );
  }
}
