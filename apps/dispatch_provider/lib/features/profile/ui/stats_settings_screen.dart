import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../disputes/state/rider_dispute_controller.dart';
import '../../disputes/ui/support/help_centre_screen.dart';
import '../../vehicle/state/vehicle_controller.dart';
import '../../verification/state/verification_controller.dart';
import '../../wallet/state/wallet_controller.dart';
import '../state/provider_profile_controller.dart';
import 'security_screen.dart';
import 'verification_intro_screen.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 17 — Account settings
// Navy header (back + title + avatar row) + 7 rows matching Figma exactly.
// ═══════════════════════════════════════════════════════════════════════════

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
    _comingSoon(context, 'App preferences');
  }

  void _openNotifications(BuildContext context) {
    _comingSoon(context, 'Notifications');
  }

  void _openEarnings(BuildContext context) {
    _comingSoon(context, 'Earnings summary');
  }

  void _openDocuments(BuildContext context) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => VerificationIntroScreen(
          verificationController: verificationController,
          vehicleController: vehicleController,
          // profileController removed — VerificationIntroScreen no longer
          // takes it. Guarantor and emergency are collected during
          // onboarding and are not surfaced on the verification hub.
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
      MaterialPageRoute(
        builder: (_) => HelpCentreScreen(disputeController: d),
      ),
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
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text('$label coming soon.')),
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final profile = profileController.profile;
    final initial = (profile?.fullName?.isNotEmpty == true)
        ? profile!.fullName![0].toUpperCase()
        : 'K';

    return Scaffold(
      backgroundColor: c.brandNavy900,
      body: Column(
        children: [
          // ── Navy header with back + title + avatar row ──
          SafeArea(
            bottom: false,
            child: Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 20),
              child: Column(
                children: [
                  Row(
                    children: [
                      GestureDetector(
                        onTap: () => Navigator.of(context).pop(),
                        behavior: HitTestBehavior.opaque,
                        child: Padding(
                          padding: const EdgeInsets.all(8),
                          child: Icon(
                            Icons.arrow_back_rounded,
                            size: 22,
                            color: Colors.white,
                          ),
                        ),
                      ),
                      const SizedBox(width: 4),
                      const Expanded(
                        child: Text(
                          'Account settings',
                          style: TextStyle(
                            fontSize: 16,
                            fontWeight: FontWeight.w700,
                            color: Colors.white,
                            letterSpacing: -0.2,
                          ),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 16),
                  Row(
                    children: [
                      Container(
                        width: 56,
                        height: 56,
                        decoration: BoxDecoration(
                          shape: BoxShape.circle,
                          border: Border.all(
                            color: Colors.white.withValues(alpha: 0.22),
                            width: 2,
                          ),
                        ),
                        child: ClipOval(
                          child:
                              profile?.profilePhotoUrl?.isNotEmpty == true
                                  ? Image.network(
                                      profile!.profilePhotoUrl!,
                                      fit: BoxFit.cover,
                                      errorBuilder: (_, _, _) =>
                                          _Initial(letter: initial),
                                    )
                                  : _Initial(letter: initial),
                        ),
                      ),
                      const SizedBox(width: 14),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            const Text(
                              'Karryam Provider',
                              style: TextStyle(
                                fontSize: 16,
                                fontWeight: FontWeight.w800,
                                color: Colors.white,
                                letterSpacing: -0.2,
                              ),
                            ),
                            const SizedBox(height: 2),
                            Text(
                              profile?.email ?? '—',
                              style: TextStyle(
                                fontSize: 13,
                                fontWeight: FontWeight.w500,
                                color:
                                    Colors.white.withValues(alpha: 0.65),
                              ),
                            ),
                          ],
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),
          ),

          // ── White sheet ──
          Expanded(
            child: Container(
              decoration: BoxDecoration(
                color: c.background,
                borderRadius: const BorderRadius.vertical(
                  top: Radius.circular(28),
                ),
              ),
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 20, 20, 24),
                children: [
                  _SettingsRow(
                    icon: Icons.tune_rounded,
                    label: 'App preferences',
                    onTap: () => _openPreferences(context),
                  ),
                  _SettingsRow(
                    icon: Icons.notifications_outlined,
                    label: 'Notifications',
                    onTap: () => _openNotifications(context),
                  ),
                  _SettingsRow(
                    icon: Icons.account_balance_wallet_outlined,
                    label: 'Earnings summary',
                    onTap: () => _openEarnings(context),
                  ),
                  _SettingsRow(
                    icon: Icons.folder_outlined,
                    label: 'Documents',
                    onTap: () => _openDocuments(context),
                  ),
                  _SettingsRow(
                    icon: Icons.lock_outline_rounded,
                    label: 'Security & privacy',
                    onTap: () => _openSecurity(context),
                  ),
                  _SettingsRow(
                    icon: Icons.support_agent_outlined,
                    label: 'Help & support',
                    onTap: () => _openHelp(context),
                  ),
                  _SettingsRow(
                    icon: Icons.info_outline_rounded,
                    label: 'About Karryam',
                    onTap: () => _openAbout(context),
                    isLast: true,
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _Initial extends StatelessWidget {
  const _Initial({required this.letter});
  final String letter;

  @override
  Widget build(BuildContext context) {
    return Container(
      alignment: Alignment.center,
      color: Colors.white.withValues(alpha: 0.14),
      child: Text(
        letter,
        style: const TextStyle(
          fontSize: 22,
          fontWeight: FontWeight.w800,
          color: Colors.white,
        ),
      ),
    );
  }
}

class _SettingsRow extends StatelessWidget {
  const _SettingsRow({
    required this.icon,
    required this.label,
    required this.onTap,
    this.isLast = false,
  });

  final IconData icon;
  final String label;
  final VoidCallback? onTap;
  final bool isLast;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Column(
      children: [
        GestureDetector(
          onTap: onTap,
          behavior: HitTestBehavior.opaque,
          child: Padding(
            padding: const EdgeInsets.symmetric(vertical: 16),
            child: Row(
              children: [
                Icon(icon, size: 22, color: c.text),
                const SizedBox(width: 16),
                Expanded(
                  child: Text(
                    label,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w600,
                      color: c.text,
                      letterSpacing: -0.1,
                    ),
                  ),
                ),
                Icon(
                  Icons.chevron_right_rounded,
                  size: 22,
                  color: c.textMuted,
                ),
              ],
            ),
          ),
        ),
        if (!isLast) Divider(height: 1, color: c.divider),
      ],
    );
  }
}