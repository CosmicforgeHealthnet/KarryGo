import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../auth/state/provider_auth_controller.dart';
import '../../disputes/state/provider_dispute_controller.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../../wallet/state/provider_withdrawal_controller.dart';
import '../../wallet/ui/provider_payments_screen.dart';
import '../state/provider_profile_controller.dart';
import 'provider_privacy_screen.dart';
import 'provider_security_screen.dart';
import 'provider_profile_info_screen.dart';
import 'provider_safety_emergency_screen.dart';
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
    required this.disputeController,
    required this.withdrawalController,
  });

  final ProviderAuthController authController;
  final ProviderProfileController profileController;

  /// Backs the Support live chat (a support-dispute-service complaint thread).
  final ProviderDisputeController disputeController;

  /// Backs Payments & Withdrawals (payment-wallet-service bank accounts).
  final ProviderWithdrawalController withdrawalController;

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

  /// Deleting an account is handled by support (earnings and trip records
  /// must be settled first), so this routes the driver to a support chat.
  Future<void> _confirmDelete() async {
    final confirmed = await showProviderConfirmDialog(
      context,
      icon: Icons.delete_outline_rounded,
      title: 'Delete Account?',
      message:
          'To protect your earnings and trip records, our support team closes accounts. '
          'Chat with them to delete yours.',
      confirmLabel: 'Contact support',
      cancelLabel: 'Cancel',
      confirmColor: kProviderRejectText,
    );
    if (confirmed == true && mounted) {
      _open(ProviderSupportScreen(disputeController: widget.disputeController));
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        bottom: false,
        child: AnimatedBuilder(
          animation: widget.profileController,
          builder: (context, _) {
            final provider = widget.profileController.profile;
            final loading = widget.profileController.loading && provider == null;

            return RefreshIndicator(
              onRefresh: () => widget.profileController.load(),
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 24, 20, 32),
                children: [
                  const KHeading(
                    eyebrow: 'Your account',
                    title: 'Profile',
                    subtitle: 'Manage your Personal Information and Account',
                  ),
                  const SizedBox(height: 20),
                  KCard(
                    padding: const EdgeInsets.all(18),
                    child: Column(
                      children: [
                        Row(
                          children: [
                            KAvatar(
                              name: provider?.displayName ?? '',
                              photoUrl: provider?.profilePhotoUrl,
                              size: 68,
                            ),
                            const SizedBox(width: 14),
                            Expanded(
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text(
                                    provider?.displayName ?? '—',
                                    style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700, color: kProviderText),
                                  ),
                                  const SizedBox(height: 6),
                                  if (provider != null)
                                    Wrap(
                                      spacing: 6,
                                      runSpacing: 6,
                                      children: [
                                        KStatusChip(
                                          provider.verificationLabel,
                                          tone: provider.isVerified
                                              ? KTone.success
                                              : (provider.isProcessing ? KTone.warning : KTone.danger),
                                        ),
                                        KStatusChip(provider.displayServiceType, tone: KTone.info, dot: false),
                                      ],
                                    ),
                                  if (provider != null) ...[
                                    const SizedBox(height: 6),
                                    _ProviderIdRow(providerId: provider.id),
                                  ],
                                ],
                              ),
                            ),
                          ],
                        ),
                        const SizedBox(height: 16),
                        const Divider(),
                        const SizedBox(height: 14),
                        _StatsRow(
                          totalTrips: provider?.totalTrips ?? 0,
                          rating: provider?.rating ?? 0,
                          tenure: provider?.tenureLabel ?? '—',
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 22),
                  if (loading)
                    const Padding(
                      padding: EdgeInsets.only(top: 40),
                      child: Center(child: CircularProgressIndicator()),
                    )
                  else ...[
                    const KEyebrow('Account'),
                    const SizedBox(height: 10),
                    KGroup(
                      children: [
                        KListRow(
                          icon: Icons.person_outline_rounded,
                          title: 'Profile Information',
                          subtitle: 'Name, phone, email and location',
                          onTap: () => _open(ProviderProfileInfoScreen(
                            authController: widget.authController,
                            profileController: widget.profileController,
                          )),
                        ),
                        KListRow(
                          icon: Icons.badge_outlined,
                          title: 'Verification & Documents',
                          subtitle: 'Licence, ID and vehicle papers',
                          onTap: () => _open(ProviderVerificationScreen(profileController: widget.profileController)),
                        ),
                        KListRow(
                          icon: Icons.local_shipping_outlined,
                          title: 'Vehicle Information',
                          subtitle: 'Your trucks and what they carry',
                          onTap: () => _open(ProviderTruckInfoScreen(profileController: widget.profileController)),
                        ),
                      ],
                    ),
                    const SizedBox(height: 20),
                    const KEyebrow('Money'),
                    const SizedBox(height: 10),
                    KGroup(
                      children: [
                        KListRow(
                          icon: Icons.account_balance_wallet_outlined,
                          title: 'Payment & Withdrawals',
                          subtitle: 'Bank accounts and payouts',
                          onTap: () => _open(ProviderPaymentsScreen(controller: widget.withdrawalController)),
                        ),
                      ],
                    ),
                    const SizedBox(height: 20),
                    const KEyebrow('Safety & privacy'),
                    const SizedBox(height: 10),
                    KGroup(
                      children: [
                        KListRow(
                          icon: Icons.health_and_safety_outlined,
                          title: 'Safety & Emergency',
                          subtitle: 'Emergency contact and guarantor',
                          onTap: () => _open(ProviderSafetyEmergencyScreen(profileController: widget.profileController)),
                        ),
                        KListRow(
                          icon: Icons.lock_outline_rounded,
                          title: 'Security',
                          subtitle: 'Withdrawal PIN',
                          onTap: () => _open(ProviderSecurityScreen(controller: widget.withdrawalController)),
                        ),
                        KListRow(
                          icon: Icons.shield_outlined,
                          title: 'Privacy',
                          subtitle: 'Your data and how it is used',
                          onTap: () => _open(ProviderPrivacyScreen(disputeController: widget.disputeController)),
                        ),
                      ],
                    ),
                    const SizedBox(height: 20),
                    const KEyebrow('Help'),
                    const SizedBox(height: 10),
                    KGroup(
                      children: [
                        KListRow(
                          icon: Icons.headset_mic_outlined,
                          title: 'Support',
                          subtitle: 'Chat with the Karryam team',
                          onTap: () => _open(ProviderSupportScreen(disputeController: widget.disputeController)),
                        ),
                        KListRow(
                          icon: Icons.logout_rounded,
                          iconColor: kProviderRejectText,
                          iconBackground: kProviderRejectBg,
                          titleColor: kProviderRejectText,
                          title: 'Log Out',
                          showChevron: false,
                          onTap: _confirmLogout,
                        ),
                      ],
                    ),
                    const SizedBox(height: 18),
                    Center(
                      child: TextButton(
                        onPressed: _confirmDelete,
                        style: TextButton.styleFrom(foregroundColor: kProviderMuted),
                        child: const Text('Delete account'),
                      ),
                    ),
                  ],
                ],
              ),
            );
          },
        ),
      ),
    );
  }
}

/// Formats a count with thousands separators, e.g. 1234 -> "1,234".
String _formatNumber(int value) {
  final digits = value.toString();
  final buffer = StringBuffer();
  for (var i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 == 0) buffer.write(',');
    buffer.write(digits[i]);
  }
  return buffer.toString();
}

// ─── Provider ID row ─────────────────────────────────────────────────────────

/// Displays the provider's support ID (e.g. KG-A0000000) with a copy button.
/// The ID is derived from the provider's UUID — no dedicated backend field.
class _ProviderIdRow extends StatelessWidget {
  const _ProviderIdRow({required this.providerId});
  final String providerId;

  String get _supportId {
    final hex = providerId.replaceAll('-', '');
    if (hex.isEmpty) return '';
    final take = hex.length < 8 ? hex.length : 8;
    return 'KG-${hex.substring(0, take).toUpperCase()}';
  }

  @override
  Widget build(BuildContext context) {
    final id = _supportId;
    if (id.isEmpty) return const SizedBox.shrink();
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          id,
          style: const TextStyle(
            fontSize: 12,
            fontWeight: FontWeight.w600,
            color: kProviderMuted,
            letterSpacing: 0.5,
          ),
        ),
        const SizedBox(width: 4),
        GestureDetector(
          onTap: () {
            Clipboard.setData(ClipboardData(text: id));
            ScaffoldMessenger.of(context).showSnackBar(
              const SnackBar(
                content: Text('Provider ID copied'),
                duration: Duration(seconds: 2),
                behavior: SnackBarBehavior.floating,
              ),
            );
          },
          child: const Icon(Icons.copy_outlined, size: 14, color: kProviderMuted),
        ),
      ],
    );
  }
}

// ─── Stats ───────────────────────────────────────────────────────────────────

class _StatsRow extends StatelessWidget {
  const _StatsRow({required this.totalTrips, required this.rating, required this.tenure});

  final int totalTrips;
  final double rating;
  final String tenure;

  @override
  Widget build(BuildContext context) {
    return IntrinsicHeight(
      child: Row(
        children: [
          _StatCell(label: 'Trips', value: _formatNumber(totalTrips)),
          const VerticalDivider(color: kProviderBorder, width: 1),
          _StatCell(
            label: 'Rating',
            value: rating > 0 ? rating.toStringAsFixed(2) : 'New',
            icon: rating > 0 ? Icons.star_rounded : null,
          ),
          const VerticalDivider(color: kProviderBorder, width: 1),
          _StatCell(label: 'With Karryam', value: tenure),
        ],
      ),
    );
  }
}

class _StatCell extends StatelessWidget {
  const _StatCell({required this.label, required this.value, this.icon});

  final String label;
  final String value;
  final IconData? icon;

  @override
  Widget build(BuildContext context) {
    return Expanded(
      child: Column(
        children: [
          Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              if (icon != null) ...[Icon(icon, size: 16, color: kAmber), const SizedBox(width: 3)],
              Flexible(
                child: Text(
                  value,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w700, color: kProviderText),
                ),
              ),
            ],
          ),
          const SizedBox(height: 3),
          Text(label, style: const TextStyle(fontSize: 11.5, color: kProviderMuted)),
        ],
      ),
    );
  }
}
