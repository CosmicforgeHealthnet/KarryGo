import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
import 'package:flutter/material.dart';

import '../../../app/app_routes.dart';
import '../../auth/state/customer_auth_controller.dart';

class CustomerHomeScreen extends StatelessWidget {
  const CustomerHomeScreen({
    super.key,
    required this.controller,
    required this.state,
  });

  final CustomerAuthController controller;
  final CustomerAuthState state;

  @override
  Widget build(BuildContext context) {
    final customer = state.customer;

    return Scaffold(
      key: const ValueKey(CustomerAppRoutes.home),
      appBar: AppBar(
        title: const Text('Home'),
        actions: [
          IconButton(
            tooltip: 'Logout',
            onPressed: state.isLoading ? null : controller.logout,
            icon: const Icon(Icons.logout_rounded),
          ),
        ],
      ),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(CosmicforgeLogisticsSpacing.lg),
          children: [
            Text(
              'Welcome${customer == null ? '' : ', ${customer.displayName}'}',
              style: Theme.of(context).textTheme.headlineSmall,
            ),
            const SizedBox(height: CosmicforgeLogisticsSpacing.xs),
            Text(
              'Choose a service to start your next request.',
              style: Theme.of(context).textTheme.bodyMedium,
            ),
            const SizedBox(height: CosmicforgeLogisticsSpacing.lg),
            _ProfileStatusCard(state: state),
            const SizedBox(height: CosmicforgeLogisticsSpacing.lg),
            CosmicforgeLogisticsServiceOptionCard(
              title: 'Book a ride',
              subtitle: 'City trips and everyday movement.',
              icon: const Icon(
                Icons.directions_car_filled_rounded,
                color: CosmicforgeLogisticsColors.primary,
              ),
              onTap: () {},
            ),
            const SizedBox(height: CosmicforgeLogisticsSpacing.md),
            CosmicforgeLogisticsServiceOptionCard(
              title: 'Send a package',
              subtitle: 'Pickup, delivery, and proof of handoff.',
              icon: const Icon(
                Icons.inventory_2_rounded,
                color: CosmicforgeLogisticsColors.primary,
              ),
              onTap: () {},
            ),
            const SizedBox(height: CosmicforgeLogisticsSpacing.md),
            CosmicforgeLogisticsServiceOptionCard(
              title: 'Request a truck',
              subtitle: 'Cargo, moving, and haulage support.',
              icon: const Icon(
                Icons.local_shipping_rounded,
                color: CosmicforgeLogisticsColors.primary,
              ),
              onTap: () {},
            ),
          ],
        ),
      ),
    );
  }
}

class _ProfileStatusCard extends StatelessWidget {
  const _ProfileStatusCard({required this.state});

  final CustomerAuthState state;

  @override
  Widget build(BuildContext context) {
    final customer = state.customer;

    return Container(
      padding: const EdgeInsets.all(CosmicforgeLogisticsSpacing.md),
      decoration: BoxDecoration(
        color: CosmicforgeLogisticsColors.primaryTint,
        borderRadius: BorderRadius.circular(20),
        border: Border.all(color: CosmicforgeLogisticsColors.primarySoft),
      ),
      child: Row(
        children: [
          const Icon(
            Icons.verified_user_rounded,
            color: CosmicforgeLogisticsColors.primary,
          ),
          const SizedBox(width: CosmicforgeLogisticsSpacing.md),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  customer?.phone ?? state.phone,
                  style: Theme.of(context).textTheme.titleMedium,
                ),
                const SizedBox(height: 2),
                Text(
                  'Profile status: ${customer?.onboardingStatus ?? 'unknown'}',
                  style: Theme.of(context).textTheme.bodyMedium,
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
