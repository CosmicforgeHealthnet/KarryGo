import 'package:flutter/material.dart';

import '../features/account_setup/ui/bike_type_screen.dart';
import '../features/account_setup/ui/driver_information_screen.dart';
import '../features/account_setup/ui/operation_mode_screen.dart';
import '../features/account_setup/ui/photo_upload_screen.dart';
import '../features/account_setup/ui/profile_details_screen.dart';
import '../features/account_setup/ui/service_type_screen.dart';
import '../features/account_setup/ui/verification_pending_screen.dart';
import '../features/account_type/ui/account_type_screen.dart';
import '../features/auth/ui/otp_verification_screen.dart';
import '../features/auth/ui/phone_entry_screen.dart';
import '../features/auth/ui/splash_screen.dart';
import '../features/onboarding/ui/onboarding_screen.dart';
import '../features/home/ui/dashboard_screen.dart';

enum _DispatchScreen {
  splash,
  onboarding,
  phoneEntry,
  otpVerification,
  accountType,
  serviceType,
  operationMode,
  profileDetails,
  photoUpload,
  bikeType,
  driverInformation,
  verificationPending,
  home,
}

class DispatchProviderApp extends StatefulWidget {
  const DispatchProviderApp({super.key});

  @override
  State<DispatchProviderApp> createState() => _DispatchProviderAppState();
}

class _DispatchProviderAppState extends State<DispatchProviderApp> {
  _DispatchScreen _screen = _DispatchScreen.splash;
  String _phone = '';

  static const _totalSteps = 6;

  int get _currentStep => switch (_screen) {
        _DispatchScreen.serviceType => 1,
        _DispatchScreen.operationMode => 2,
        _DispatchScreen.profileDetails => 3,
        _DispatchScreen.photoUpload => 4,
        _DispatchScreen.bikeType => 5,
        _DispatchScreen.driverInformation => 6,
        _ => 0,
      };

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'KarryGo Driver',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorSchemeSeed: const Color(0xFF4CAF50),
        useMaterial3: true,
        fontFamily: 'Poppins',
      ),
      home: _buildScreen(),
    );
  }

  Widget _buildScreen() {
    return switch (_screen) {
      _DispatchScreen.splash => SplashScreen(
          onDone: () => _go(_DispatchScreen.onboarding),
        ),
      _DispatchScreen.onboarding => OnboardingScreen(
          onDone: () => _go(_DispatchScreen.phoneEntry),
        ),
      _DispatchScreen.phoneEntry => PhoneEntryScreen(
          onContinue: (phone) {
            _phone = phone;
            _go(_DispatchScreen.otpVerification);
          },
        ),
      _DispatchScreen.otpVerification => OtpVerificationScreen(
          phone: _phone,
          onVerify: (_) => _go(_DispatchScreen.accountType),
          onBack: () => _go(_DispatchScreen.phoneEntry),
          onResend: () {},
        ),
      _DispatchScreen.accountType => AccountTypeScreen(
          onSelected: (type) => _go(_DispatchScreen.serviceType),
        ),
      _DispatchScreen.serviceType => ServiceTypeScreen(
          onContinue: (_) => _go(_DispatchScreen.operationMode),
          onBack: () => _go(_DispatchScreen.accountType),
          currentStep: _currentStep,
          totalSteps: _totalSteps,
        ),
      _DispatchScreen.operationMode => OperationModeScreen(
          onContinue: (_) => _go(_DispatchScreen.profileDetails),
          onBack: () => _go(_DispatchScreen.serviceType),
          currentStep: _currentStep,
          totalSteps: _totalSteps,
        ),
      _DispatchScreen.profileDetails => ProfileDetailsScreen(
          onContinue: (_) => _go(_DispatchScreen.photoUpload),
          onBack: () => _go(_DispatchScreen.operationMode),
          currentStep: _currentStep,
          totalSteps: _totalSteps,
        ),
      _DispatchScreen.photoUpload => PhotoUploadScreen(
          onContinue: () => _go(_DispatchScreen.bikeType),
          onBack: () => _go(_DispatchScreen.profileDetails),
          currentStep: _currentStep,
          totalSteps: _totalSteps,
        ),
      _DispatchScreen.bikeType => BikeTypeScreen(
          onContinue: (_) => _go(_DispatchScreen.driverInformation),
          onBack: () => _go(_DispatchScreen.photoUpload),
          currentStep: _currentStep,
          totalSteps: _totalSteps,
        ),
      _DispatchScreen.driverInformation => DriverInformationScreen(
          onSubmit: (_) => _go(_DispatchScreen.verificationPending),
          onBack: () => _go(_DispatchScreen.bikeType),
          currentStep: _currentStep,
          totalSteps: _totalSteps,
        ),
      _DispatchScreen.verificationPending => VerificationPendingScreen(
          onGoToDashboard: () => _go(_DispatchScreen.home),
        ),
    _DispatchScreen.home => DashboardScreen(),
    };
  }

  void _go(_DispatchScreen screen) => setState(() => _screen = screen);
}