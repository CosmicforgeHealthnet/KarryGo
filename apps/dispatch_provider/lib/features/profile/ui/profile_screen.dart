import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../disputes/state/rider_dispute_controller.dart';
import '../../vehicle/state/vehicle_controller.dart';
import '../../vehicle/ui/bike_list_screen.dart';
import '../../verification/state/verification_controller.dart';
import '../../wallet/state/wallet_controller.dart';
import '../state/provider_profile_controller.dart';
import 'profile_info_screen.dart';
import 'emergency_contact_screen.dart';
import 'guarantor_information_screen.dart';
import 'stats_settings_screen.dart';
import 'deactivate_account_screen.dart';
import 'update_profile_photo_screen.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 11 — Account Home
// Navy hero (avatar · "Good morning," · "Karryam Provider" · gear) +
// white sheet with 7 rows (Figma 5 + Bikes/Vehicles + Log out).
// ═══════════════════════════════════════════════════════════════════════════

class ProfileScreen extends StatefulWidget {
  final ProviderProfileController profileController;
  final VerificationController verificationController;
  final VehicleController vehicleController;
  final RiderDisputeController? disputeController;
  final WalletController? walletController;
  final VoidCallback? onLogout;
  final VoidCallback? onAccountDeleted;

  const ProfileScreen({
    super.key,
    required this.profileController,
    required this.verificationController,
    required this.vehicleController,
    this.disputeController,
    this.walletController,
    this.onLogout,
    this.onAccountDeleted,
  });

  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> {
  @override
  void initState() {
    super.initState();
    _fetch();
  }

  Future<void> _fetch() async {
    await Future.wait([
      widget.profileController.loadMe(),
      widget.profileController.loadStats(),
      widget.verificationController.loadVerificationStatus(),
    ]);
    if (mounted) setState(() {});
  }

  String get _greeting {
    final h = DateTime.now().hour;
    if (h < 12) return 'Good morning,';
    if (h < 17) return 'Good afternoon,';
    return 'Good evening,';
  }

  // ── Navigation ────────────────────────────────────────────────────────

  void _openProfileDetails() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProfileInfoScreen(
          profileController: widget.profileController,
        ),
      ),
    );
  }

  void _openEmergencyContact() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => EmergencyContactScreen(
          profileController: widget.profileController,
        ),
      ),
    );
  }

  void _openGuarantor() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => GuarantorInformationScreen(
          profileController: widget.profileController,
        ),
      ),
    );
  }

  void _openBikes() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => BikeListScreen(
          vehicleController: widget.vehicleController,
        ),
      ),
    );
  }

  void _openStatsSettings() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => StatsSettingsScreen(
          profileController: widget.profileController,
          verificationController: widget.verificationController,
          vehicleController: widget.vehicleController,
          walletController: widget.walletController,
          disputeController: widget.disputeController,
          onLogout: widget.onLogout,
        ),
      ),
    );
  }

  void _openAvatar() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => UpdateProfilePhotoScreen(
          profileController: widget.profileController,
        ),
      ),
    );
  }

  void _openDeactivate() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => DeactivateAccountScreen(
          profileController: widget.profileController,
          onDeleted: widget.onAccountDeleted,
        ),
      ),
    );
  }

  void _confirmLogout() {
    showDialog<void>(
      context: context,
      builder: (ctx) {
        final c = ctx.palette;
        return AlertDialog(
          title: const Text('Log out?'),
          content: Text(
            'You will need to sign in again to use Karryam.',
            style: TextStyle(color: c.textMuted, fontSize: 14, height: 1.5),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx),
              child: const Text('Cancel'),
            ),
            TextButton(
              onPressed: () {
                Navigator.pop(ctx);
                widget.onLogout?.call();
              },
              style: TextButton.styleFrom(foregroundColor: c.danger),
              child: const Text(
                'Log out',
                style: TextStyle(fontWeight: FontWeight.w700),
              ),
            ),
          ],
        );
      },
    );
  }

  // ── Build ─────────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final profile = widget.profileController.profile;
    final initial = (profile?.fullName?.isNotEmpty == true)
        ? profile!.fullName![0].toUpperCase()
        : 'K';

    return Scaffold(
      backgroundColor: c.brandNavy900,
      body: Column(
        children: [
          // ── Navy hero ──
          SafeArea(
            bottom: false,
            child: Padding(
              padding: const EdgeInsets.fromLTRB(20, 4, 20, 28),
              child: Column(
                children: [
                  Row(
                    children: [
                      const Spacer(),
                      GestureDetector(
                        onTap: _openStatsSettings,
                        behavior: HitTestBehavior.opaque,
                        child: Padding(
                          padding: const EdgeInsets.all(8),
                          child: Icon(
                            Icons.settings_outlined,
                            size: 22,
                            color: Colors.white,
                          ),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 4),

                  // Avatar with camera badge
                  GestureDetector(
                    onTap: _openAvatar,
                    behavior: HitTestBehavior.opaque,
                    child: Stack(
                      children: [
                        Container(
                          width: 96,
                          height: 96,
                          decoration: BoxDecoration(
                            shape: BoxShape.circle,
                            border: Border.all(
                              color: Colors.white.withValues(alpha: 0.22),
                              width: 2.5,
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
                        Positioned(
                          bottom: 0,
                          right: 0,
                          child: Container(
                            width: 30,
                            height: 30,
                            alignment: Alignment.center,
                            decoration: BoxDecoration(
                              color: c.brandTeal500,
                              shape: BoxShape.circle,
                              border: Border.all(
                                color: c.brandNavy900,
                                width: 2,
                              ),
                            ),
                            child: const Icon(
                              Icons.photo_camera_rounded,
                              size: 15,
                              color: Colors.white,
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 14),

                  Text(
                    _greeting,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w500,
                      color: Colors.white.withValues(alpha: 0.72),
                    ),
                  ),
                  const SizedBox(height: 2),
                  const Text(
                    'Karryam Provider',
                    style: TextStyle(
                      fontSize: 20,
                      fontWeight: FontWeight.w800,
                      color: Colors.white,
                      letterSpacing: -0.3,
                    ),
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
                  _MenuRow(
                    icon: Icons.person_outline_rounded,
                    label: 'Profile details',
                    onTap: _openProfileDetails,
                  ),
                  _MenuRow(
                    icon: Icons.emergency_outlined,
                    label: 'Emergency contact',
                    onTap: _openEmergencyContact,
                  ),
                  _MenuRow(
                    icon: Icons.people_outline_rounded,
                    label: 'Guarantor',
                    onTap: _openGuarantor,
                  ),
                  _MenuRow(
                    icon: Icons.two_wheeler_outlined,
                    label: 'Bikes / Vehicles',
                    onTap: _openBikes,
                  ),
                  _MenuRow(
                    icon: Icons.bar_chart_rounded,
                    label: 'Stats and settings',
                    onTap: _openStatsSettings,
                  ),
                  _MenuRow(
                    icon: Icons.delete_outline_rounded,
                    label: 'Deactivate account',
                    onTap: _openDeactivate,
                    destructive: true,
                  ),
                  _MenuRow(
                    icon: Icons.logout_rounded,
                    label: 'Log out',
                    onTap: _confirmLogout,
                    destructive: true,
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
          fontSize: 36,
          fontWeight: FontWeight.w800,
          color: Colors.white,
        ),
      ),
    );
  }
}

class _MenuRow extends StatelessWidget {
  const _MenuRow({
    required this.icon,
    required this.label,
    required this.onTap,
    this.destructive = false,
    this.isLast = false,
  });

  final IconData icon;
  final String label;
  final VoidCallback? onTap;
  final bool destructive;
  final bool isLast;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final fg = destructive ? c.danger : c.text;

    return Column(
      children: [
        GestureDetector(
          onTap: onTap,
          behavior: HitTestBehavior.opaque,
          child: Padding(
            padding: const EdgeInsets.symmetric(vertical: 16),
            child: Row(
              children: [
                Icon(icon, size: 22, color: fg),
                const SizedBox(width: 16),
                Expanded(
                  child: Text(
                    label,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w600,
                      color: fg,
                      letterSpacing: -0.1,
                    ),
                  ),
                ),
                Icon(
                  Icons.chevron_right_rounded,
                  size: 22,
                  color: destructive
                      ? c.danger.withValues(alpha: 0.6)
                      : c.textMuted,
                ),
              ],
            ),
          ),
        ),
        if (!isLast)
          Divider(
            height: 1,
            color: destructive
                ? c.danger.withValues(alpha: 0.15)
                : c.divider,
          ),
      ],
    );
  }
}