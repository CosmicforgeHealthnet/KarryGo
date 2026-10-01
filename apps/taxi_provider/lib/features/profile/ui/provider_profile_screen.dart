import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
import 'package:flutter/material.dart';

import '../../auth/data/provider_auth_api.dart';
import '../../auth/models/provider_auth_models.dart';
import '../../auth/state/provider_auth_controller.dart';
import '../../home/state/provider_home_controller.dart';

/// Driver profile: who they are, what they drive, and whether they take
/// packages.
class ProviderProfileScreen extends StatefulWidget {
  const ProviderProfileScreen({
    super.key,
    required this.authApi,
    required this.authController,
    required this.homeController,
  });

  final ProviderAuthApi authApi;
  final ProviderAuthController authController;
  final ProviderHomeController homeController;

  @override
  State<ProviderProfileScreen> createState() => _ProviderProfileScreenState();
}

class _ProviderProfileScreenState extends State<ProviderProfileScreen> {
  List<ProviderVehicle> _vehicles = const [];
  bool _loadingVehicles = true;

  @override
  void initState() {
    super.initState();
    _loadVehicles();
  }

  Future<void> _loadVehicles() async {
    final token = widget.authController.state.accessToken;
    if (token == null) return;
    try {
      final vehicles = await widget.authApi.listVehicles(accessToken: token);
      if (!mounted) return;
      setState(() {
        _vehicles = vehicles;
        _loadingVehicles = false;
      });
    } on ApiException {
      if (!mounted) return;
      setState(() => _loadingVehicles = false);
    }
  }

  Future<void> _confirmLogout() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: const Text('Sign out?'),
        content: const Text(
          'You will stop receiving trips until you sign in again.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(false),
            child: const Text('Cancel'),
          ),
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(true),
            child: const Text(
              'Sign out',
              style: TextStyle(color: CosmicforgeLogisticsColors.danger),
            ),
          ),
        ],
      ),
    );
    if (confirmed == true) {
      widget.homeController.stop();
      await widget.authController.logout();
    }
  }

  @override
  Widget build(BuildContext context) {
    final provider = widget.authController.state.provider;

    return Scaffold(
      backgroundColor: CosmicforgeLogisticsColors.surface,
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.fromLTRB(20, 16, 20, 32),
          children: [
            const Text(
              'Profile',
              style: TextStyle(
                fontSize: 22,
                fontWeight: FontWeight.w800,
                color: CosmicforgeLogisticsColors.text,
              ),
            ),
            const SizedBox(height: 18),

            if (provider != null) _IdentityCard(provider: provider),

            const SizedBox(height: 18),
            const _SectionLabel('Work preferences'),
            const SizedBox(height: 8),
            _ParcelToggle(
              value: provider?.acceptsParcels ?? false,
              onChanged: widget.homeController.setAcceptsParcels,
            ),

            const SizedBox(height: 22),
            const _SectionLabel('Your vehicles'),
            const SizedBox(height: 8),
            if (_loadingVehicles)
              const Padding(
                padding: EdgeInsets.symmetric(vertical: 20),
                child: Center(child: CircularProgressIndicator()),
              )
            else if (_vehicles.isEmpty)
              const _Card(
                child: Text(
                  'No vehicle registered yet. You need an active vehicle '
                  'before you can go online.',
                  style: TextStyle(
                    color: CosmicforgeLogisticsColors.textMuted,
                    fontSize: 13,
                  ),
                ),
              )
            else
              for (final vehicle in _vehicles)
                Padding(
                  padding: const EdgeInsets.only(bottom: 10),
                  child: _VehicleCard(vehicle: vehicle),
                ),

            const SizedBox(height: 28),
            OutlinedButton(
              onPressed: _confirmLogout,
              style: OutlinedButton.styleFrom(
                minimumSize: const Size(0, 48),
                side: const BorderSide(color: CosmicforgeLogisticsColors.border),
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(24),
                ),
              ),
              child: const Text(
                'Sign out',
                style: TextStyle(
                  color: CosmicforgeLogisticsColors.danger,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _IdentityCard extends StatelessWidget {
  const _IdentityCard({required this.provider});

  final TaxiProvider provider;

  @override
  Widget build(BuildContext context) {
    return _Card(
      child: Row(
        children: [
          CircleAvatar(
            radius: 28,
            backgroundColor: CosmicforgeLogisticsColors.primaryTint,
            child: Text(
              provider.initial,
              style: const TextStyle(
                color: CosmicforgeLogisticsColors.primary,
                fontSize: 20,
                fontWeight: FontWeight.w800,
              ),
            ),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  provider.fullName,
                  style: const TextStyle(
                    fontSize: 16,
                    fontWeight: FontWeight.w800,
                    color: CosmicforgeLogisticsColors.text,
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  provider.phone,
                  style: const TextStyle(
                    fontSize: 13,
                    color: CosmicforgeLogisticsColors.textMuted,
                  ),
                ),
                const SizedBox(height: 6),
                Row(
                  children: [
                    const Icon(
                      Icons.star_rounded,
                      size: 15,
                      color: CosmicforgeLogisticsColors.warning,
                    ),
                    const SizedBox(width: 3),
                    Text(
                      provider.rating > 0
                          ? provider.rating.toStringAsFixed(2)
                          : 'No ratings yet',
                      style: const TextStyle(
                        fontSize: 12.5,
                        fontWeight: FontWeight.w700,
                        color: CosmicforgeLogisticsColors.text,
                      ),
                    ),
                    const SizedBox(width: 12),
                    Text(
                      '${provider.totalTrips} trips',
                      style: const TextStyle(
                        fontSize: 12.5,
                        color: CosmicforgeLogisticsColors.textMuted,
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _ParcelToggle extends StatelessWidget {
  const _ParcelToggle({required this.value, required this.onChanged});

  final bool value;
  final ValueChanged<bool> onChanged;

  @override
  Widget build(BuildContext context) {
    return _Card(
      child: Row(
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text(
                  'Accept packages',
                  style: TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w800,
                    color: CosmicforgeLogisticsColors.text,
                  ),
                ),
                const SizedBox(height: 3),
                Text(
                  value
                      ? 'You can be matched to package deliveries as well as rides.'
                      : 'You will only be matched to passenger rides.',
                  style: const TextStyle(
                    fontSize: 12,
                    color: CosmicforgeLogisticsColors.textMuted,
                  ),
                ),
              ],
            ),
          ),
          Switch.adaptive(value: value, onChanged: onChanged),
        ],
      ),
    );
  }
}

class _VehicleCard extends StatelessWidget {
  const _VehicleCard({required this.vehicle});

  final ProviderVehicle vehicle;

  @override
  Widget build(BuildContext context) {
    return _Card(
      child: Row(
        children: [
          const Icon(
            Icons.directions_car_filled_rounded,
            color: CosmicforgeLogisticsColors.primary,
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  vehicle.descriptor,
                  style: const TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w700,
                    color: CosmicforgeLogisticsColors.text,
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  '${vehicle.plateNumber} · ${vehicle.vehicleClass}',
                  style: const TextStyle(
                    fontSize: 12.5,
                    color: CosmicforgeLogisticsColors.textMuted,
                  ),
                ),
              ],
            ),
          ),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
            decoration: BoxDecoration(
              color: vehicle.isActive
                  ? CosmicforgeLogisticsColors.successSoft
                  : CosmicforgeLogisticsColors.warningSoft,
              borderRadius: BorderRadius.circular(20),
            ),
            child: Text(
              vehicle.isActive ? 'Active' : vehicle.status,
              style: TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w800,
                color: vehicle.isActive
                    ? CosmicforgeLogisticsColors.success
                    : CosmicforgeLogisticsColors.warning,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _SectionLabel extends StatelessWidget {
  const _SectionLabel(this.text);

  final String text;

  @override
  Widget build(BuildContext context) => Text(
    text,
    style: const TextStyle(
      fontSize: 13,
      fontWeight: FontWeight.w800,
      color: CosmicforgeLogisticsColors.text,
    ),
  );
}

class _Card extends StatelessWidget {
  const _Card({required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: CosmicforgeLogisticsColors.card,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: CosmicforgeLogisticsColors.border),
      ),
      child: child,
    );
  }
}
