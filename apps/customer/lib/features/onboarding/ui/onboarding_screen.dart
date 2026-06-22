import 'package:flutter/material.dart';
import 'package:geolocator/geolocator.dart';
import 'package:permission_handler/permission_handler.dart';

import '../../../app/app_routes.dart';
import '../../../features/auth/state/customer_auth_controller.dart';
import '../../../shared/widgets/figma_customer_widgets.dart';

class OnboardingScreen extends StatefulWidget {
  const OnboardingScreen({super.key, required this.controller});

  final CustomerAuthController controller;

  @override
  State<OnboardingScreen> createState() => _OnboardingScreenState();
}

class _OnboardingScreenState extends State<OnboardingScreen> {
  int _index = 0;

  // Step indices
  static const _stepLocation = 0;
  static const _stepLocationDenied = 1;
  static const _stepLocationPermanent = 2;
  static const _stepNotification = 3;
  static const _stepUpdates = 4;

  static const _steps = [
    _PermissionStep(
      title: 'Enable Location',
      message:
          'We use your location to find nearby drivers, estimate prices, and make pickup faster.',
      imageAsset: CustomerFigmaAssets.locationMap,
      primaryLabel: 'Allow Location',
      secondaryLabel: 'Enter location manually',
    ),
    _PermissionStep(
      title: 'Location Disabled',
      message:
          'Location access is off. Turn it on to find nearby drivers faster.',
      imageAsset: CustomerFigmaAssets.locationMap,
      primaryLabel: 'Enable Location',
      secondaryLabel: 'Continue manually',
    ),
    _PermissionStep(
      title: 'Turn on location for better experience.',
      message:
          'Enable location in your settings to see nearby drivers and faster pickups.',
      imageAsset: CustomerFigmaAssets.locationMap,
      primaryLabel: 'Open Settings',
    ),
    _PermissionStep(
      title: "Don't miss important updates",
      message:
          'Get real-time alerts and updates about your rides, deliveries, and activity.',
      imageAsset: CustomerFigmaAssets.notificationBell,
      primaryLabel: 'Enable Notification',
      secondaryLabel: 'Skip for now',
    ),
    _PermissionStep(
      title: 'Get Updates and Reminders',
      message:
          'Get updates on discounts, reminders, and important trip alerts.',
      imageAsset: CustomerFigmaAssets.updatesTag,
      primaryLabel: 'I want to receive updates',
      secondaryLabel: "No, I don't want to receive updates",
    ),
  ];

  @override
  Widget build(BuildContext context) {
    final step = _steps[_index];

    return FigmaPhoneScaffold(
      key: const ValueKey(CustomerAppRoutes.onboarding),
      backgroundColor: CustomerFigmaColors.darkGreen,
      padding: const EdgeInsets.fromLTRB(28, 28, 28, 32),
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Spacer(flex: 2),
          Image.asset(step.imageAsset, height: 118),
          const SizedBox(height: 28),
          Text(
            step.title,
            textAlign: TextAlign.center,
            style: const TextStyle(
              color: Colors.white,
              fontSize: 18,
              fontWeight: FontWeight.w800,
            ),
          ),
          const SizedBox(height: 10),
          Text(
            step.message,
            textAlign: TextAlign.center,
            style: TextStyle(
              color: Colors.white.withValues(alpha: 0.42),
              fontSize: 12,
              height: 1.45,
            ),
          ),
          const SizedBox(height: 28),
          FigmaPrimaryButton(
            label: step.primaryLabel,
            isLoading: _busy,
            onPressed: _onPrimary,
          ),
          if (step.secondaryLabel != null) ...[
            const SizedBox(height: 12),
            FigmaSecondaryButton(
              label: step.secondaryLabel!,
              dark: true,
              onPressed: _onSecondary,
            ),
          ],
          const Spacer(flex: 3),
        ],
      ),
    );
  }

  bool _busy = false;

  Future<void> _onPrimary() async {
    if (_busy) return;
    switch (_index) {
      case _stepLocation:
        await _requestLocation();
      case _stepLocationDenied:
        await _openLocationSettings();
      case _stepLocationPermanent:
        await openAppSettings();
        // Re-check after returning from settings; advance if now granted.
        final status = await Permission.locationWhenInUse.status;
        if (status.isGranted) {
          _goTo(_stepNotification);
        }
      case _stepNotification:
        await _requestNotification();
      case _stepUpdates:
        widget.controller.completeOnboarding();
    }
  }

  void _onSecondary() {
    switch (_index) {
      case _stepLocation:
      case _stepLocationDenied:
        _goTo(_stepNotification);
      case _stepNotification:
        _goTo(_stepUpdates);
      case _stepUpdates:
        widget.controller.completeOnboarding();
    }
  }

  Future<void> _requestLocation() async {
    setState(() => _busy = true);
    try {
      final status = await Permission.locationWhenInUse.request();
      if (status.isGranted || status.isLimited) {
        // On Android the app permission can be granted while the device
        // Location Services (GPS) switch is still off. Check both.
        final serviceEnabled = await Geolocator.isLocationServiceEnabled();
        if (serviceEnabled) {
          _goTo(_stepNotification);
        } else {
          _goTo(_stepLocationDenied);
        }
      } else if (status.isPermanentlyDenied) {
        _goTo(_stepLocationPermanent);
      } else {
        _goTo(_stepLocationDenied);
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _openLocationSettings() async {
    setState(() => _busy = true);
    try {
      // App permission may already be granted; the device GPS switch is off.
      // Open the device location settings so the user can turn it on.
      await Geolocator.openLocationSettings();
      // Re-check after returning; advance if the service is now enabled.
      final enabled = await Geolocator.isLocationServiceEnabled();
      final status = await Permission.locationWhenInUse.request();
      if (enabled && (status.isGranted || status.isLimited)) {
        _goTo(_stepNotification);
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _requestNotification() async {
    setState(() => _busy = true);
    try {
      await Permission.notification.request();
      _goTo(_stepUpdates);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  void _goTo(int step) {
    if (mounted) setState(() => _index = step);
  }
}

class _PermissionStep {
  const _PermissionStep({
    required this.title,
    required this.message,
    required this.imageAsset,
    required this.primaryLabel,
    this.secondaryLabel,
  });

  final String title;
  final String message;
  final String imageAsset;
  final String primaryLabel;
  final String? secondaryLabel;
}
