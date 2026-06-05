import 'package:flutter/material.dart';

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
      secondaryLabel: 'Enter location manually',
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
          FigmaPrimaryButton(label: step.primaryLabel, onPressed: _next),
          if (step.secondaryLabel != null) ...[
            const SizedBox(height: 12),
            FigmaSecondaryButton(
              label: step.secondaryLabel!,
              dark: true,
              onPressed: _next,
            ),
          ],
          const Spacer(flex: 3),
        ],
      ),
    );
  }

  void _next() {
    if (_index == _steps.length - 1) {
      widget.controller.completeOnboarding();
      return;
    }
    setState(() => _index += 1);
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
