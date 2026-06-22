import 'package:flutter/material.dart';

import '../../../app/app_routes.dart';
import '../../../shared/widgets/figma_customer_widgets.dart';
import '../../auth/state/customer_auth_controller.dart';

class AllSetScreen extends StatelessWidget {
  const AllSetScreen({
    super.key,
    required this.controller,
    required this.state,
  });

  final CustomerAuthController controller;
  final CustomerAuthState state;

  @override
  Widget build(BuildContext context) {
    return FigmaPhoneScaffold(
      key: const ValueKey(CustomerAppRoutes.allSet),
      bottom: FigmaPrimaryButton(
        label: 'Continue to dashboard',
        onPressed: state.isLoading ? null : () => controller.finishProfileSetup(),
      ),
      child: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const SizedBox(height: 92),
            Image.asset(CustomerFigmaAssets.allSetPeople, height: 132),
            const SizedBox(height: 48),
            const Text(
              "You're all set!",
              textAlign: TextAlign.center,
              style: TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 20,
                fontWeight: FontWeight.w900,
              ),
            ),
            const SizedBox(height: 14),
            const Text(
              'Your account has been created successfully. You can now book rides, send packages, and more.',
              textAlign: TextAlign.center,
              style: TextStyle(
                color: CustomerFigmaColors.muted,
                fontSize: 12,
                height: 1.45,
              ),
            ),
            const SizedBox(height: 24),
            const Text(
              "We've sent a confirmation link to your email.",
              textAlign: TextAlign.center,
              style: TextStyle(
                color: CustomerFigmaColors.primary,
                fontSize: 12,
                fontWeight: FontWeight.w600,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
