import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
import 'package:flutter/material.dart';

import '../../../app/app_routes.dart';
import '../../../shared/widgets/customer_screen_frame.dart';
import '../state/customer_auth_controller.dart';

class ProfileRequiredScreen extends StatelessWidget {
  const ProfileRequiredScreen({
    super.key,
    required this.controller,
    required this.state,
  });

  final CustomerAuthController controller;
  final CustomerAuthState state;

  @override
  Widget build(BuildContext context) {
    return CustomerScreenFrame(
      key: const ValueKey(CustomerAppRoutes.profileRequired),
      header: const AuthHeader(
        title: 'Profile setup is next',
        subtitle:
            'Your number is verified. Profile details will connect when the profile API is ready.',
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Container(
            padding: const EdgeInsets.all(CosmicforgeLogisticsSpacing.lg),
            decoration: BoxDecoration(
              color: Colors.white,
              borderRadius: BorderRadius.circular(20),
              border: Border.all(color: CosmicforgeLogisticsColors.border),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  state.customer?.phone ?? state.phone,
                  style: Theme.of(context).textTheme.titleLarge,
                ),
                const SizedBox(height: CosmicforgeLogisticsSpacing.xs),
                Text(
                  'Status: profile required',
                  style: Theme.of(context).textTheme.bodyMedium,
                ),
              ],
            ),
          ),
          const SizedBox(height: CosmicforgeLogisticsSpacing.lg),
          CosmicforgeLogisticsButton(
            label: 'Continue to app',
            onPressed: controller.continueFromProfileRequired,
          ),
        ],
      ),
    );
  }
}
