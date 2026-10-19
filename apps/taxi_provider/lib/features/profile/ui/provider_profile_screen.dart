import '../../../core/theme/appearance_controller.dart';
import '../../../core/widgets/driver_state_art.dart';
import '../../../core/theme/karryam_theme.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../../../app/driver_services.dart';
import '../../auth/data/provider_auth_api.dart';
import '../../auth/models/provider_auth_models.dart';
import '../../auth/state/provider_auth_controller.dart';
import '../../home/state/provider_home_controller.dart';
import '../../notifications/ui/notifications_screen.dart';
import '../../onboarding/ui/add_vehicle_screen.dart';
import '../../onboarding/ui/onboarding_screen.dart';
import '../../onboarding/ui/verification_screen.dart';
import '../../support/ui/help_screen.dart';
import '../../wallet/ui/wallet_screen.dart';

/// Driver profile: who they are, what they drive, and whether they take
/// packages.
class ProviderProfileScreen extends StatefulWidget {
  const ProviderProfileScreen({
    super.key,
    required this.authApi,
    required this.authController,
    required this.homeController,
    required this.services,
  });

  final ProviderAuthApi authApi;
  final ProviderAuthController authController;
  final ProviderHomeController homeController;
  final DriverServices services;

  @override
  State<ProviderProfileScreen> createState() => _ProviderProfileScreenState();
}

class _ProviderProfileScreenState extends State<ProviderProfileScreen> {
  List<ProviderVehicle> _vehicles = [];
  bool _loadingVehicles = true;

  @override
  void initState() {
    super.initState();
    _loadVehicles();
  }

  Future<void> _loadVehicles() async {
    final token = widget.authController.state.accessToken;
    if (token == null) {
      setState(() => _loadingVehicles = false);
      return;
    }
    setState(() {
      _loadingVehicles = true;
      _vehicleError = null;
    });
    try {
      final vehicles = await widget.authApi.listVehicles(accessToken: token);
      if (!mounted) return;
      setState(() {
        _vehicles = vehicles;
        _loadingVehicles = false;
      });
    } on ApiException catch (error) {
      if (!mounted) return;
      setState(() {
        _loadingVehicles = false;
        _vehicleError = error.message;
      });
    }
  }

  Future<void> _confirmLogout() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: Text('Sign out?'),
        content: Text('You will stop receiving trips until you sign in again.'),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(false),
            child: Text('Cancel'),
          ),
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(true),
            child: Text(
              'Sign out',
              style: TextStyle(color: KarryamColors.of(context).danger),
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
    final colors = KarryamColors.of(context);
    return Scaffold(
      body: SafeArea(
        child: RefreshIndicator(
          onRefresh: _loadVehicles,
          child: ListView(
            padding: const EdgeInsets.fromLTRB(20, 20, 20, 32),
            children: [
              Text(
                'YOUR ACCOUNT',
                style: TextStyle(
                  color: colors.textMuted,
                  fontSize: 11,
                  letterSpacing: 2,
                ),
              ),
              const SizedBox(height: 16),
              const Text(
                'Behind every trip.\nThere’s you.',
                style: TextStyle(
                  fontSize: 30,
                  fontWeight: FontWeight.w700,
                  height: 1.2,
                ),
              ),
              const SizedBox(height: 24),
              if (provider != null) ...[
                _DriverPass(provider: provider),
                _section('Your account'),
                _panel(
                  Column(
                    children: [
                      _menu(
                        Icons.verified_user_outlined,
                        'Verification',
                        driverCleared(provider)
                            ? 'Cleared to drive'
                            : 'See what is still being checked',
                        () => _open(
                          VerificationScreen(
                            auth: widget.authController,
                            onUpdateDetails: _openEditDetails,
                          ),
                        ),
                      ),
                      _menu(
                        Icons.account_balance_wallet_outlined,
                        'Wallet',
                        'Balance, bank accounts and withdrawals',
                        () => _open(WalletScreen(api: widget.services.wallet)),
                      ),
                      _menu(
                        Icons.badge_outlined,
                        'Details and documents',
                        'Licence, ID, safety contacts and photo',
                        _openEditDetails,
                      ),
                      _menu(
                        Icons.notifications_none_rounded,
                        'Notifications',
                        'Past ride offers and updates',
                        () => _open(
                          NotificationsScreen(taxi: widget.services.taxi),
                        ),
                      ),
                      _menu(
                        Icons.support_agent_outlined,
                        'Help and support',
                        'Questions, chat and reporting a problem',
                        () => _open(HelpScreen(api: widget.services.support)),
                      ),
                    ],
                  ),
                ),
                _section('Personal details'),
                _panel(
                  Column(
                    children: [
                      _detail(
                        Icons.person_outline,
                        'Full name',
                        provider.fullName,
                      ),
                      _detail(
                        Icons.phone_outlined,
                        'Registered phone',
                        provider.phone,
                      ),
                      _detail(
                        Icons.alternate_email,
                        'Email address',
                        provider.email.isEmpty
                            ? 'Not provided'
                            : provider.email,
                      ),
                    ],
                  ),
                ),
              ],
              _section('Your vehicles'),
              if (_loadingVehicles)
                const Padding(
                  padding: EdgeInsets.all(24),
                  child: Center(child: CircularProgressIndicator()),
                )
              else if (_vehicleError != null)
                _panel(
                  Column(
                    children: [
                      Text(_vehicleError!),
                      TextButton(
                        onPressed: _loadVehicles,
                        child: const Text('Try again'),
                      ),
                    ],
                  ),
                )
              else ...[
                if (_vehicles.isEmpty)
                  _panel(
                    const Text(
                      'No vehicle registered yet. You need an active vehicle before you can go online.',
                    ),
                  ),
                for (final vehicle in _vehicles)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 16),
                    child: _VehiclePass(vehicle: vehicle),
                  ),
                OutlinedButton.icon(
                  onPressed: () async {
                    final added = await Navigator.of(context).push<bool>(
                      MaterialPageRoute(
                        builder: (_) =>
                            AddVehicleScreen(api: widget.services.onboarding),
                      ),
                    );
                    if (added == true) _loadVehicles();
                  },
                  icon: const Icon(Icons.add),
                  label: const Text('Add a car'),
                ),
              ],
              _section('App settings'),
              const AppearanceSetting(),
              _section('How you work'),
              _panel(
                Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    SwitchListTile.adaptive(
                      contentPadding: EdgeInsets.zero,
                      title: const Text(
                        'Parcel deliveries',
                        style: TextStyle(fontWeight: FontWeight.w600),
                      ),
                      subtitle: Text(
                        provider?.acceptsParcels == true
                            ? 'Open to parcel requests'
                            : 'Passenger rides only',
                      ),
                      value: provider?.acceptsParcels ?? false,
                      onChanged: _savingPreference ? null : _setParcels,
                    ),
                    const Text(
                      'Choose whether to receive package delivery requests alongside passenger rides.',
                      style: TextStyle(fontSize: 13, height: 1.5),
                    ),
                    if (widget.homeController.state.error != null)
                      Padding(
                        padding: const EdgeInsets.only(top: 12),
                        child: Text(
                          widget.homeController.state.error!,
                          style: TextStyle(color: colors.danger),
                        ),
                      ),
                    const Divider(height: 28),
                    Row(
                      children: [
                        Icon(
                          Icons.verified_user_outlined,
                          size: 18,
                          color: colors.primarySoft,
                        ),
                        const SizedBox(width: 8),
                        const Expanded(
                          child: Text(
                            'Pickup and handover codes protect each delivery.',
                            style: TextStyle(fontSize: 12),
                          ),
                        ),
                      ],
                    ),
                  ],
                ),
              ),
              if (provider != null) ...[
                _section('Account standing'),
                _panel(
                  Column(
                    children: [
                      _detail(
                        Icons.shield_outlined,
                        'Account status',
                        _status(provider.accountStatus),
                      ),
                      _detail(
                        Icons.fact_check_outlined,
                        'Onboarding',
                        _status(provider.onboardingStatus),
                      ),
                    ],
                  ),
                ),
              ],
              const SizedBox(height: 24),
              OutlinedButton(
                onPressed: _confirmLogout,
                child: Text('Sign out', style: TextStyle(color: colors.danger)),
              ),
              const SizedBox(height: 24),
              Container(
                padding: const EdgeInsets.all(24),
                decoration: BoxDecoration(
                  color: KarryamColors.navy,
                  borderRadius: BorderRadius.circular(18),
                ),
                child: Column(
                  children: [
                    Image.asset('assets/branding/karryam-logo.png', width: 116),
                    const SizedBox(height: 12),
                    const Text(
                      'Movement intelligence.\nA brighter tomorrow.',
                      textAlign: TextAlign.center,
                      style: TextStyle(color: Color(0xFFCED2DA), fontSize: 12),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  bool _savingPreference = false;
  String? _vehicleError;
  Future<void> _setParcels(bool value) async {
    setState(() => _savingPreference = true);
    try {
      await widget.homeController.setAcceptsParcels(value);
    } finally {
      if (mounted) setState(() => _savingPreference = false);
    }
  }

  void _open(Widget screen) =>
      Navigator.of(context).push(MaterialPageRoute(builder: (_) => screen));

  void _openEditDetails() => _open(
    OnboardingScreen(
      api: widget.services.onboarding,
      auth: widget.authController,
      editing: true,
    ),
  );

  Widget _menu(
    IconData icon,
    String title,
    String subtitle,
    VoidCallback onTap,
  ) => ListTile(
    contentPadding: EdgeInsets.zero,
    leading: Icon(icon),
    title: Text(title, style: const TextStyle(fontWeight: FontWeight.w600)),
    subtitle: Text(subtitle),
    trailing: const Icon(Icons.chevron_right_rounded),
    onTap: onTap,
  );

  Widget _section(String text) => Padding(
    padding: const EdgeInsets.only(top: 28, bottom: 14),
    child: Text(
      text,
      style: const TextStyle(fontSize: 17, fontWeight: FontWeight.w600),
    ),
  );
  Widget _panel(Widget child) => Card(
    child: Padding(padding: const EdgeInsets.all(18), child: child),
  );
  Widget _detail(IconData icon, String label, String value) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 12),
    child: Row(
      children: [
        Container(
          padding: const EdgeInsets.all(10),
          decoration: BoxDecoration(
            color: KarryamColors.of(context).surface,
            borderRadius: BorderRadius.circular(12),
          ),
          child: Icon(icon, size: 20),
        ),
        const SizedBox(width: 14),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                label,
                style: TextStyle(
                  color: KarryamColors.of(context).textMuted,
                  fontSize: 12,
                ),
              ),
              const SizedBox(height: 5),
              Text(value, style: const TextStyle(fontWeight: FontWeight.w500)),
            ],
          ),
        ),
      ],
    ),
  );
}

String _status(String value) {
  if (value.trim().isEmpty) return 'Not available';
  return value
      .replaceAll('_', ' ')
      .split(' ')
      .map(
        (word) => word.isEmpty
            ? ''
            : word[0].toUpperCase() + word.substring(1).toLowerCase(),
      )
      .join(' ');
}

class _DriverPass extends StatelessWidget {
  const _DriverPass({required this.provider});
  final TaxiProvider provider;
  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.all(22),
    decoration: BoxDecoration(
      color: KarryamColors.navy,
      borderRadius: BorderRadius.circular(22),
      boxShadow: [
        BoxShadow(
          color: Colors.black.withValues(alpha: .12),
          blurRadius: 24,
          offset: const Offset(0, 10),
        ),
      ],
    ),
    child: DefaultTextStyle(
      style: const TextStyle(fontFamily: 'Inter', color: Colors.white),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Image.asset('assets/branding/karryam-logo.png', width: 100),
              const SizedBox(width: 12),
              const Text(
                'DRIVER',
                style: TextStyle(
                  color: KarryamColors.teal,
                  fontSize: 10,
                  letterSpacing: 2,
                ),
              ),
            ],
          ),
          const SizedBox(height: 28),
          Row(
            children: [
              ClipRRect(
                borderRadius: BorderRadius.circular(20),
                child: SizedBox(
                  width: 64,
                  height: 64,
                  child: provider.profilePhotoUrl?.isNotEmpty == true
                      ? Image.network(
                          provider.profilePhotoUrl!,
                          fit: BoxFit.cover,
                          errorBuilder: (_, _, _) => _initial(),
                        )
                      : _initial(),
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
                        fontSize: 19,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                    const SizedBox(height: 6),
                    Text(
                      provider.acceptsParcels
                          ? 'Passenger rides & deliveries'
                          : 'Passenger rides',
                      style: const TextStyle(
                        color: Color(0xFFCED2DA),
                        fontSize: 12,
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
          const Divider(height: 40, color: Color(0xFF3C4C69)),
          Wrap(
            spacing: 28,
            runSpacing: 16,
            children: [
              _metric(
                provider.rating > 0
                    ? '${provider.rating.toStringAsFixed(2)} ★'
                    : '—',
                provider.rating > 0 ? 'Driver rating' : 'No ratings yet',
              ),
              _metric('${provider.totalTrips}', 'Completed trips'),
              _metric(_status(provider.accountStatus), 'Account'),
            ],
          ),
        ],
      ),
    ),
  );
  Widget _initial() => Container(
    color: const Color(0xFF193F54),
    alignment: Alignment.center,
    child: Text(
      provider.initial,
      style: const TextStyle(color: Color(0xFF99E7DC), fontSize: 26),
    ),
  );
  Widget _metric(String value, String label) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Text(
        value,
        style: const TextStyle(fontSize: 19, fontWeight: FontWeight.w600),
      ),
      const SizedBox(height: 6),
      Text(
        label,
        style: const TextStyle(fontSize: 10, color: Color(0xFFCED2DA)),
      ),
    ],
  );
}

class _VehiclePass extends StatelessWidget {
  const _VehiclePass({required this.vehicle});
  final ProviderVehicle vehicle;
  @override
  Widget build(BuildContext context) => Card(
    child: Padding(
      padding: const EdgeInsets.all(18),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const Icon(
                Icons.directions_car_outlined,
                color: KarryamColors.teal,
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  _status(vehicle.status),
                  style: const TextStyle(fontWeight: FontWeight.w600),
                ),
              ),
            ],
          ),
          const DriverStateArt(kind: DriverArt.offline, height: 115),
          Text(
            vehicle.make.isEmpty ? 'VEHICLE' : vehicle.make.toUpperCase(),
            style: TextStyle(
              fontSize: 11,
              letterSpacing: 2,
              color: KarryamColors.of(context).textMuted,
            ),
          ),
          const SizedBox(height: 6),
          Text(
            vehicle.model.isEmpty ? vehicle.vehicleClass : vehicle.model,
            style: const TextStyle(fontSize: 26, fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 16),
          Wrap(
            spacing: 12,
            runSpacing: 8,
            children: [
              Chip(label: Text(vehicle.plateNumber)),
              Chip(
                label: Text(
                  vehicle.color.isEmpty ? 'Color not provided' : vehicle.color,
                ),
              ),
              Chip(label: Text('${vehicle.seats} seats')),
            ],
          ),
          ExpansionTile(
            tilePadding: EdgeInsets.zero,
            title: const Text(
              'Vehicle registration',
              style: TextStyle(fontSize: 14),
            ),
            children: [
              ListTile(
                contentPadding: EdgeInsets.zero,
                title: const Text('Vehicle class'),
                subtitle: Text(vehicle.vehicleClass),
              ),
              ListTile(
                contentPadding: EdgeInsets.zero,
                title: const Text('Status'),
                subtitle: Text(_status(vehicle.status)),
              ),
            ],
          ),
        ],
      ),
    ),
  );
}
