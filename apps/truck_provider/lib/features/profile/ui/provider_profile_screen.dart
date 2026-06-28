import 'package:flutter/material.dart';

import '../../auth/state/provider_auth_controller.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_profile_controller.dart';
import 'provider_coming_soon_screen.dart';
import 'provider_profile_info_screen.dart';
import 'provider_support_screen.dart';
import 'provider_truck_info_screen.dart';
import 'provider_verification_screen.dart';
import 'widgets/provider_profile_widgets.dart';

/// Main Profile tab (Figma 2110). Rendered inside the home shell, which supplies
/// the bottom navigation bar.
class ProviderProfileScreen extends StatefulWidget {
  const ProviderProfileScreen({
    super.key,
    required this.authController,
    required this.profileController,
  });

  final ProviderAuthController authController;
  final ProviderProfileController profileController;

  @override
  State<ProviderProfileScreen> createState() => _ProviderProfileScreenState();
}

class _ProviderProfileScreenState extends State<ProviderProfileScreen> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => widget.profileController.load());
  }

  Future<void> _open(Widget screen) async {
    await Navigator.of(context).push(MaterialPageRoute(builder: (_) => screen));
    if (mounted) widget.profileController.load();
  }

  Future<void> _confirmLogout() async {
    final confirmed = await showProviderConfirmDialog(
      context,
      icon: Icons.logout_rounded,
      title: 'Log Out?',
      message: 'Are you sure you want to log out of your account?',
      confirmLabel: 'Log Out',
      cancelLabel: 'Cancel',
      confirmColor: kProviderRejectText,
    );
    if (confirmed == true) {
      await widget.authController.logout();
    }
  }

  Future<void> _confirmDelete() async {
    final confirmed = await showProviderConfirmDialog(
      context,
      icon: Icons.delete_outline_rounded,
      title: 'Delete Account?',
      message: 'When you delete your account, you loose complete access. This action cannot be undone.',
      confirmLabel: 'Delete Account',
      cancelLabel: 'Cancel',
      confirmColor: kProviderRejectText,
    );
    if (confirmed == true && mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Account deletion request submitted. Our team will reach out.'),
          behavior: SnackBarBehavior.floating,
        ),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        bottom: false,
        child: AnimatedBuilder(
          animation: widget.profileController,
          builder: (context, _) {
            final provider = widget.profileController.profile;
            final loading = widget.profileController.loading && provider == null;

            return RefreshIndicator(
              color: kProviderGreen,
              onRefresh: () => widget.profileController.load(),
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 16, 20, 24),
                children: [
                  const Text(
                    'Profile',
                    style: TextStyle(color: kProviderText, fontSize: 24, fontWeight: FontWeight.w800),
                  ),
                  const SizedBox(height: 4),
                  const Text(
                    'Manage your Personal Information and Account',
                    style: TextStyle(color: kProviderMuted, fontSize: 13),
                  ),
                  const SizedBox(height: 24),

                  // Avatar
                  Center(
                    child: Container(
                      width: 96,
                      height: 96,
                      decoration: const BoxDecoration(color: Color(0xFFE7EEF7), shape: BoxShape.circle),
                      clipBehavior: Clip.antiAlias,
                      child: provider?.profilePhotoUrl != null && provider!.profilePhotoUrl!.isNotEmpty
                          ? Image.network(
                              provider.profilePhotoUrl!,
                              fit: BoxFit.cover,
                              errorBuilder: (_, _, _) =>
                                  const Icon(Icons.person_rounded, color: kProviderGreen, size: 48),
                            )
                          : const Icon(Icons.person_rounded, color: kProviderGreen, size: 48),
                    ),
                  ),
                  const SizedBox(height: 14),
                  Text(
                    provider?.displayName ?? '—',
                    textAlign: TextAlign.center,
                    style: const TextStyle(color: kProviderText, fontSize: 24, fontWeight: FontWeight.w800),
                  ),
                  const SizedBox(height: 6),
                  if (provider != null) ...[
                    _statLine('Driving with KarryGo', provider.tenureLabel),
                    const SizedBox(height: 2),
                    _statLine('Completed Trips', provider.totalTrips >= 1000 ? '1,000+' : '${provider.totalTrips}'),
                    const SizedBox(height: 8),
                    _statusRow(provider.verificationLabel, provider.isVerified, provider.displayServiceType),
                  ],
                  const SizedBox(height: 24),

                  if (loading)
                    const Padding(
                      padding: EdgeInsets.only(top: 40),
                      child: Center(child: CircularProgressIndicator(color: kProviderGreen)),
                    )
                  else ...[
                    _ProfileMenuItem(
                      icon: Icons.person_outline_rounded,
                      label: 'Profile Information',
                      onTap: () => _open(ProviderProfileInfoScreen(
                        authController: widget.authController,
                        profileController: widget.profileController,
                      )),
                    ),
                    _ProfileMenuItem(
                      icon: Icons.badge_outlined,
                      label: 'Verification & Documents',
                      onTap: () => _open(ProviderVerificationScreen(profileController: widget.profileController)),
                    ),
                    _ProfileMenuItem(
                      icon: Icons.local_shipping_outlined,
                      label: 'Vehicle Information',
                      onTap: () => _open(ProviderTruckInfoScreen(profileController: widget.profileController)),
                    ),
                    _ProfileMenuItem(
                      icon: Icons.health_and_safety_outlined,
                      label: 'Safety & Emergency',
                      onTap: () => _open(const ProviderComingSoonScreen(title: 'Safety & Emergency')),
                    ),
                    _ProfileMenuItem(
                      icon: Icons.lock_outline_rounded,
                      label: 'Security',
                      onTap: () => _open(const ProviderComingSoonScreen(title: 'Security')),
                    ),
                    _ProfileMenuItem(
                      icon: Icons.shield_outlined,
                      label: 'Privacy',
                      onTap: () => _open(const ProviderComingSoonScreen(title: 'Privacy')),
                    ),
                    _ProfileMenuItem(
                      icon: Icons.account_balance_wallet_outlined,
                      label: 'Payment & Withdrawals',
                      onTap: () => _open(const ProviderComingSoonScreen(title: 'Payment & Withdrawals')),
                    ),
                    _ProfileMenuItem(
                      icon: Icons.headset_mic_outlined,
                      label: 'Support',
                      onTap: () => _open(const ProviderSupportScreen()),
                    ),
                    const SizedBox(height: 4),
                    _ProfileMenuItem(
                      icon: Icons.logout_rounded,
                      label: 'Log Out',
                      danger: true,
                      onTap: _confirmLogout,
                    ),
                    const SizedBox(height: 16),
                    _DeleteAccountCard(onDelete: _confirmDelete),
                  ],
                ],
              ),
            );
          },
        ),
      ),
    );
  }

  Widget _statLine(String label, String value) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        Text('$label . ', style: const TextStyle(color: kProviderMuted, fontSize: 13)),
        Text(value, style: const TextStyle(color: kProviderGreen, fontSize: 13, fontWeight: FontWeight.w700)),
      ],
    );
  }

  Widget _statusRow(String verification, bool verified, String service) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        Text(
          verification,
          style: TextStyle(
            color: verified ? kProviderGreen : (verification == 'Processing' ? kProviderAmber : kProviderRejectText),
            fontSize: 13,
            fontWeight: FontWeight.w700,
          ),
        ),
        const Padding(
          padding: EdgeInsets.symmetric(horizontal: 8),
          child: Icon(Icons.circle, size: 5, color: kProviderMuted),
        ),
        Text(service, style: const TextStyle(color: kProviderGreen, fontSize: 13, fontWeight: FontWeight.w700)),
      ],
    );
  }
}

// ─── Menu item card ─────────────────────────────────────────────────────────

class _ProfileMenuItem extends StatelessWidget {
  const _ProfileMenuItem({
    required this.icon,
    required this.label,
    required this.onTap,
    this.danger = false,
  });

  final IconData icon;
  final String label;
  final VoidCallback onTap;
  final bool danger;

  @override
  Widget build(BuildContext context) {
    final color = danger ? kProviderRejectText : kProviderText;
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Material(
        color: Colors.white,
        borderRadius: BorderRadius.circular(16),
        child: InkWell(
          onTap: onTap,
          borderRadius: BorderRadius.circular(16),
          child: Ink(
            decoration: BoxDecoration(
              color: Colors.white,
              borderRadius: BorderRadius.circular(16),
              boxShadow: const [BoxShadow(color: Color(0x0F000000), blurRadius: 16, offset: Offset(0, 4))],
            ),
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
            child: Row(
              children: [
                Icon(icon, color: danger ? kProviderRejectText : kProviderText, size: 22),
                const SizedBox(width: 14),
                Expanded(
                  child: Text(
                    label,
                    style: TextStyle(color: color, fontSize: 15, fontWeight: FontWeight.w700),
                  ),
                ),
                Icon(
                  Icons.chevron_right_rounded,
                  color: danger ? kProviderRejectText : kProviderMuted,
                  size: 22,
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

// ─── Delete account card ─────────────────────────────────────────────────────

class _DeleteAccountCard extends StatelessWidget {
  const _DeleteAccountCard({required this.onDelete});
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: kProviderRejectBg,
        borderRadius: BorderRadius.circular(16),
      ),
      child: Row(
        children: [
          const Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'Delete Account',
                  style: TextStyle(color: kProviderRejectText, fontSize: 15, fontWeight: FontWeight.w800),
                ),
                SizedBox(height: 4),
                Text(
                  'When you delete your account, you loose complete access.',
                  style: TextStyle(color: kProviderRejectText, fontSize: 12, height: 1.4),
                ),
              ],
            ),
          ),
          const SizedBox(width: 12),
          FilledButton(
            onPressed: onDelete,
            style: FilledButton.styleFrom(
              backgroundColor: kProviderRejectText,
              shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
            ),
            child: const Text(
              'Delete Account',
              style: TextStyle(color: Colors.white, fontSize: 13, fontWeight: FontWeight.w700),
            ),
          ),
        ],
      ),
    );
  }
}
