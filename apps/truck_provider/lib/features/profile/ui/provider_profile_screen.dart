import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../auth/state/provider_auth_controller.dart';
import '../../disputes/state/provider_dispute_controller.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../../wallet/state/provider_withdrawal_controller.dart';
import '../../wallet/ui/provider_payments_screen.dart';
import '../state/provider_profile_controller.dart';
import 'provider_coming_soon_screen.dart';
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
      backgroundColor: kProviderSurface,
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

                  // ── Header: avatar, name, stat lines, status, provider id ──
                  Center(
                    child: Column(
                      children: [
                        _ProfileAvatar(
                          photoUrl: provider?.profilePhotoUrl,
                          fallbackInitial: (provider?.displayName.isNotEmpty == true)
                              ? provider!.displayName[0].toUpperCase()
                              : '?',
                        ),
                        const SizedBox(height: 12),
                        Text(
                          provider?.displayName ?? '—',
                          textAlign: TextAlign.center,
                          style: const TextStyle(color: kProviderText, fontSize: 20, fontWeight: FontWeight.w800),
                        ),
                        if (provider != null) ...[
                          const SizedBox(height: 6),
                          _statLine('Driving with KarryGo', provider.tenureLabel),
                          const SizedBox(height: 2),
                          _statLine('Completed Trips', _formatNumber(provider.totalTrips)),
                          const SizedBox(height: 8),
                          _statusRow(provider.verificationLabel, provider.isVerified, provider.displayServiceType),
                          const SizedBox(height: 10),
                          _ProviderIdRow(providerId: provider.id),
                        ],
                      ],
                    ),
                  ),

                  const SizedBox(height: 24),

                  // ── Stats card ─────────────────────────────────────────────
                  _StatsCard(
                    totalTrips: provider?.totalTrips ?? 0,
                    rating: provider?.rating ?? 0,
                  ),

                  const SizedBox(height: 24),

                  if (loading)
                    const Padding(
                      padding: EdgeInsets.only(top: 40),
                      child: Center(child: CircularProgressIndicator(color: kProviderGreen)),
                    )
                  else ...[
                    _ProfileMenuItem(
                      icon: Icons.person_outline,
                      label: 'Profile Information',
                      onTap: () => _open(ProviderProfileInfoScreen(
                        authController: widget.authController,
                        profileController: widget.profileController,
                      )),
                    ),
                    const SizedBox(height: 10),
                    _ProfileMenuItem(
                      icon: Icons.badge_outlined,
                      label: 'Verification & Documents',
                      onTap: () => _open(ProviderVerificationScreen(profileController: widget.profileController)),
                    ),
                    const SizedBox(height: 10),
                    _ProfileMenuItem(
                      icon: Icons.local_shipping_outlined,
                      label: 'Vehicle Information',
                      onTap: () => _open(ProviderTruckInfoScreen(profileController: widget.profileController)),
                    ),
                    const SizedBox(height: 10),
                    _ProfileMenuItem(
                      icon: Icons.health_and_safety_outlined,
                      label: 'Safety & Emergency',
                      onTap: () => _open(ProviderSafetyEmergencyScreen(profileController: widget.profileController)),
                    ),
                    const SizedBox(height: 10),
                    _ProfileMenuItem(
                      icon: Icons.lock_outline_rounded,
                      label: 'Security',
                      onTap: () => _open(const ProviderComingSoonScreen(title: 'Security')),
                    ),
                    const SizedBox(height: 10),
                    _ProfileMenuItem(
                      icon: Icons.shield_outlined,
                      label: 'Privacy',
                      onTap: () => _open(const ProviderComingSoonScreen(title: 'Privacy')),
                    ),
                    const SizedBox(height: 10),
                    _ProfileMenuItem(
                      icon: Icons.account_balance_wallet_outlined,
                      label: 'Payment & Withdrawals',
                      onTap: () => _open(
                        ProviderPaymentsScreen(controller: widget.withdrawalController),
                      ),
                    ),
                    const SizedBox(height: 10),
                    _ProfileMenuItem(
                      icon: Icons.headset_mic_outlined,
                      label: 'Support',
                      onTap: () => _open(
                        ProviderSupportScreen(disputeController: widget.disputeController),
                      ),
                    ),
                    const SizedBox(height: 14),
                    _ProfileMenuItem(
                      icon: Icons.logout_rounded,
                      label: 'Log Out',
                      danger: true,
                      onTap: _confirmLogout,
                    ),
                    const SizedBox(height: 24),
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

// ─── Avatar ──────────────────────────────────────────────────────────────────

/// Avatar that shows the provider's uploaded photo, falling back to an
/// initial-letter circle while loading or if no photo is set / it fails to load.
class _ProfileAvatar extends StatelessWidget {
  const _ProfileAvatar({required this.photoUrl, required this.fallbackInitial});

  final String? photoUrl;
  final String fallbackInitial;

  static const double _size = 100;

  Widget _fallback() {
    return Container(
      width: _size,
      height: _size,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: kProviderGreen.withValues(alpha: 0.1),
      ),
      child: Text(
        fallbackInitial,
        style: const TextStyle(fontSize: 36, fontWeight: FontWeight.bold, color: kProviderGreen),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final url = photoUrl;
    if (url == null || url.isEmpty) return _fallback();

    return ClipOval(
      child: Image.network(
        url,
        width: _size,
        height: _size,
        fit: BoxFit.cover,
        errorBuilder: (_, _, _) => _fallback(),
        loadingBuilder: (context, child, progress) {
          if (progress == null) return child;
          return Container(
            width: _size,
            height: _size,
            alignment: Alignment.center,
            color: kProviderGreen.withValues(alpha: 0.1),
            child: const SizedBox(
              width: 24,
              height: 24,
              child: CircularProgressIndicator(strokeWidth: 2, color: kProviderGreen),
            ),
          );
        },
      ),
    );
  }
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

// ─── Stats card ──────────────────────────────────────────────────────────────

class _StatsCard extends StatelessWidget {
  const _StatsCard({required this.totalTrips, required this.rating});

  final int totalTrips;
  final double rating;

  @override
  Widget build(BuildContext context) {
    final hasRating = rating > 0;
    final ratingLabel = hasRating ? rating.toStringAsFixed(2) : 'No ratings yet';

    return Container(
      padding: const EdgeInsets.symmetric(vertical: 18, horizontal: 16),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(16),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.04),
            blurRadius: 8,
            offset: const Offset(0, 2),
          ),
        ],
      ),
      child: Row(
        children: [
          _StatCell(label: 'Trips', value: _formatNumber(totalTrips)),
          const _StatDivider(),
          _StatCell(
            label: 'Rating',
            value: ratingLabel,
            icon: hasRating ? Icons.star_rounded : null,
            iconColor: kProviderAmber,
          ),
        ],
      ),
    );
  }
}

class _StatCell extends StatelessWidget {
  const _StatCell({
    required this.label,
    required this.value,
    this.icon,
    this.iconColor,
  });

  final String label;
  final String value;
  final IconData? icon;
  final Color? iconColor;

  @override
  Widget build(BuildContext context) {
    return Expanded(
      child: Column(
        children: [
          if (icon != null)
            Row(
              mainAxisAlignment: MainAxisAlignment.center,
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(icon, size: 14, color: iconColor),
                const SizedBox(width: 2),
                Text(
                  value,
                  style: const TextStyle(fontSize: 15, fontWeight: FontWeight.w800, color: kProviderText),
                ),
              ],
            )
          else
            Text(
              value,
              textAlign: TextAlign.center,
              style: const TextStyle(fontSize: 15, fontWeight: FontWeight.w800, color: kProviderText),
            ),
          const SizedBox(height: 4),
          Text(
            label,
            textAlign: TextAlign.center,
            style: const TextStyle(fontSize: 11, color: kProviderMuted),
          ),
        ],
      ),
    );
  }
}

class _StatDivider extends StatelessWidget {
  const _StatDivider();

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 1,
      height: 36,
      color: kProviderBorder,
      margin: const EdgeInsets.symmetric(horizontal: 4),
    );
  }
}

// ─── Menu item ───────────────────────────────────────────────────────────────

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
    return Material(
      color: Colors.white,
      borderRadius: BorderRadius.circular(14),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(14),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(14),
            border: Border.all(color: kProviderBorder),
          ),
          child: Row(
            children: [
              Icon(icon, color: color, size: 20),
              const SizedBox(width: 12),
              Expanded(
                child: Text(
                  label,
                  style: TextStyle(color: color, fontSize: 14, fontWeight: FontWeight.w600),
                ),
              ),
              Icon(Icons.chevron_right, color: color, size: 20),
            ],
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
