import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../wallet/state/wallet_controller.dart';
import 'new_phone_number_screen.dart';
import 'session_guidance_screen.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 19 — Security & Privacy hub
// 4 rows matching Figma exactly:
//   Phone number · Login sessions · Privacy settings · Security tips
// ═══════════════════════════════════════════════════════════════════════════

class SecurityScreen extends StatelessWidget {
  const SecurityScreen({super.key, this.walletController});

  // Kept for API compatibility — not used after TransactionPin removal.
  final WalletController? walletController;

  void _openPhoneNumber(BuildContext context) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => const NewPhoneNumberScreen(
          countryFlag: '🇳🇬',
          countryCode: '+234',
        ),
      ),
    );
  }

  void _openLoginSessions(BuildContext context) {
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Login sessions coming soon.')),
    );
  }

  void _openPrivacy(BuildContext context) {
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Privacy settings coming soon.')),
    );
  }

  void _openSecurityTips(BuildContext context) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => const SessionGuidanceScreen(),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            // ── Header ──
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
              child: Row(
                children: [
                  GestureDetector(
                    onTap: () => Navigator.of(context).pop(),
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Icon(
                        Icons.arrow_back_rounded,
                        size: 22,
                        color: c.text,
                      ),
                    ),
                  ),
                ],
              ),
            ),

            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                children: [
                  Text(
                    'Security & Privacy',
                    style: TextStyle(
                      fontSize: 26,
                      fontWeight: FontWeight.w800,
                      color: c.text,
                      letterSpacing: -0.4,
                    ),
                  ),
                  const SizedBox(height: 6),
                  Text(
                    'Keep your account safe and in your control.',
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w500,
                      color: c.textMuted,
                      height: 1.45,
                    ),
                  ),
                  const SizedBox(height: 28),

                  _Row(
                    icon: Icons.phone_outlined,
                    label: 'Phone number',
                    subtitle: '+234 803 555 1234',
                    onTap: () => _openPhoneNumber(context),
                  ),
                  _Row(
                    icon: Icons.devices_outlined,
                    label: 'Login sessions',
                    subtitle: 'Manage your active sessions',
                    onTap: () => _openLoginSessions(context),
                  ),
                  _Row(
                    icon: Icons.lock_outline_rounded,
                    label: 'Privacy settings',
                    subtitle: 'Control your data',
                    onTap: () => _openPrivacy(context),
                  ),
                  _Row(
                    icon: Icons.shield_outlined,
                    label: 'Security tips',
                    subtitle: 'Learn how to stay safe',
                    onTap: () => _openSecurityTips(context),
                    isLast: true,
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _Row extends StatelessWidget {
  const _Row({
    required this.icon,
    required this.label,
    required this.subtitle,
    required this.onTap,
    this.isLast = false,
  });

  final IconData icon;
  final String label;
  final String subtitle;
  final VoidCallback onTap;
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
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        label,
                        style: TextStyle(
                          fontSize: 15,
                          fontWeight: FontWeight.w600,
                          color: c.text,
                          letterSpacing: -0.1,
                        ),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        subtitle,
                        style: TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w500,
                          color: c.textMuted,
                        ),
                      ),
                    ],
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