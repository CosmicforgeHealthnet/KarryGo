import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/journey_icon.dart';
import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../disputes/state/rider_dispute_controller.dart';
import '../../disputes/ui/support/help_centre_screen.dart';
import '../../vehicle/state/vehicle_controller.dart';
import '../../verification/state/verification_controller.dart';
import '../../wallet/state/wallet_controller.dart';
import '../state/provider_profile_controller.dart';
import 'security_screen.dart';
import '../../../core/network/provider_session.dart';
import 'app_preferences_screen.dart';
import '../../wallet/ui/earning_summary_screen.dart';
import 'verification_intro_screen.dart';

class StatsSettingsScreen extends StatelessWidget {
  const StatsSettingsScreen({
    super.key,
    required this.profileController,
    required this.verificationController,
    required this.vehicleController,
    this.walletController,
    this.disputeController,
    this.onLogout,
  });

  final ProviderProfileController profileController;
  final VerificationController verificationController;
  final VehicleController vehicleController;
  final WalletController? walletController;
  final RiderDisputeController? disputeController;
  final VoidCallback? onLogout;

  void _openPreferences(BuildContext context) {
    Navigator.of(
      context,
    ).push(MaterialPageRoute(builder: (_) => const AppPreferencesScreen()));
  }

  void _openNotifications(BuildContext context) {
    ProviderSession.of(context).openNotifications?.call();
  }

  void _openEarnings(BuildContext context) {
    if (walletController != null) {
      Navigator.of(context).push(
        MaterialPageRoute(
          builder: (_) =>
              EarningSummaryScreen(walletController: walletController!),
        ),
      );
    }
  }

  void _openDocuments(BuildContext context) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => VerificationIntroScreen(
          verificationController: verificationController,
          vehicleController: vehicleController,
        ),
      ),
    );
  }

  void _openSecurity(BuildContext context) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => SecurityScreen(walletController: walletController),
      ),
    );
  }

  void _openHelp(BuildContext context) {
    final d = disputeController;
    if (d == null) {
      _comingSoon(context, 'Help & support');
      return;
    }
    Navigator.of(context).push(
      MaterialPageRoute(builder: (_) => HelpCentreScreen(disputeController: d)),
    );
  }

  void _openAbout(BuildContext context) {
    showAboutDialog(
      context: context,
      applicationName: 'Karryam Dispatch Provider',
      applicationVersion: 'v1.0.0',
      applicationLegalese: '© Cosmicforge',
      children: [
        const SizedBox(height: 12),
        Text(
          'Move · Connect · Grow',
          style: TextStyle(
            fontSize: 14,
            fontWeight: FontWeight.w700,
            color: context.palette.brandTeal600,
            letterSpacing: 0.4,
          ),
        ),
      ],
    );
  }

  void _comingSoon(BuildContext context, String label) {
    ScaffoldMessenger.of(
      context,
    ).showSnackBar(SnackBar(content: Text('$label coming soon.')));
  }

  @override
  Widget build(BuildContext context) => AccountPage(
    title: 'Settings',
    symbol: JourneySymbol.preferences,
    description: 'Make Karryam work for you.',
    children: [
      AccountSection(
        title: 'Your experience',
        children: [
          AccountMenuTile(
            title: 'App preferences',
            subtitle: 'Appearance, data source and offline updates',
            symbol: JourneySymbol.preferences,
            onTap: () => _openPreferences(context),
          ),
          AccountMenuTile(
            title: 'Notifications',
            symbol: JourneySymbol.contact,
            onTap: ProviderSession.of(context).openNotifications == null
                ? null
                : () => _openNotifications(context),
          ),
        ],
      ),
      AccountSection(
        title: 'Account and earnings',
        children: [
          if (walletController != null)
            AccountMenuTile(
              title: 'Earnings',
              symbol: JourneySymbol.earnings,
              onTap: () => _openEarnings(context),
            ),
          AccountMenuTile(
            title: 'Documents',
            symbol: JourneySymbol.review,
            onTap: () => _openDocuments(context),
          ),
          AccountMenuTile(
            title: 'Security',
            subtitle: 'Phone number and account safety',
            symbol: JourneySymbol.key,
            onTap: () => _openSecurity(context),
          ),
        ],
      ),
      AccountSection(
        title: 'Help and information',
        children: [
          if (disputeController != null)
            AccountMenuTile(
              title: 'Help and support',
              symbol: JourneySymbol.contact,
              onTap: () => _openHelp(context),
            ),
          AccountMenuTile(
            title: 'About Karryam',
            symbol: JourneySymbol.partner,
            onTap: () => _openAbout(context),
          ),
        ],
      ),
    ],
  );
}
