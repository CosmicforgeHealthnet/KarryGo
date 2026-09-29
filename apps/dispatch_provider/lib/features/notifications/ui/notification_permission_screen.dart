import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 03 — Notification permission education
// Bell hero · title · 4 benefit rows · Enable / Not now
// ═══════════════════════════════════════════════════════════════════════════

class NotificationPermissionScreen extends StatelessWidget {
  const NotificationPermissionScreen({
    super.key,
    this.onEnable,
    this.onSkip,
  });

  final VoidCallback? onEnable;
  final VoidCallback? onSkip;

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
                padding: const EdgeInsets.fromLTRB(24, 20, 24, 24),
                children: [
                  // ── Bell hero ──
                  Center(
                    child: Container(
                      width: 140,
                      height: 140,
                      alignment: Alignment.center,
                      decoration: BoxDecoration(
                        color: c.teal10,
                        borderRadius: BorderRadius.circular(30),
                      ),
                      child: Icon(
                        Icons.notifications_active_rounded,
                        size: 76,
                        color: c.brandTeal500,
                      ),
                    ),
                  ),
                  const SizedBox(height: 32),

                  Text(
                    'Get notified about opportunities and more',
                    textAlign: TextAlign.center,
                    style: TextStyle(
                      fontSize: 24,
                      fontWeight: FontWeight.w800,
                      color: c.text,
                      letterSpacing: -0.4,
                      height: 1.25,
                    ),
                  ),
                  const SizedBox(height: 12),
                  Text(
                    'Turn on push notifications to receive:',
                    textAlign: TextAlign.center,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w500,
                      color: c.textMuted,
                      height: 1.5,
                    ),
                  ),
                  const SizedBox(height: 36),

                  const _Benefit(
                    icon: Icons.person_add_alt_rounded,
                    text: 'New assignment offers',
                  ),
                  const SizedBox(height: 20),
                  const _Benefit(
                    icon: Icons.local_shipping_outlined,
                    text: 'Mission updates',
                  ),
                  const SizedBox(height: 20),
                  const _Benefit(
                    icon: Icons.account_balance_wallet_outlined,
                    text: 'Payout notifications',
                  ),
                  const SizedBox(height: 20),
                  const _Benefit(
                    icon: Icons.campaign_outlined,
                    text: 'Important alerts',
                  ),
                ],
              ),
            ),

            // ── Enable ──
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 8),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: onEnable ?? () => Navigator.of(context).pop(true),
                  style: FilledButton.styleFrom(
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('Enable notifications'),
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
              child: SizedBox(
                height: 44,
                width: double.infinity,
                child: TextButton(
                  onPressed: onSkip ?? () => Navigator.of(context).pop(false),
                  style: TextButton.styleFrom(
                    foregroundColor: c.textMuted,
                    textStyle: const TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                  child: const Text('Not now'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _Benefit extends StatelessWidget {
  const _Benefit({required this.icon, required this.text});
  final IconData icon;
  final String text;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Row(
      children: [
        Container(
          width: 40,
          height: 40,
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: c.navy10,
            borderRadius: BorderRadius.circular(10),
          ),
          child: Icon(icon, size: 20, color: c.brandNavy900),
        ),
        const SizedBox(width: 16),
        Expanded(
          child: Text(
            text,
            style: TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w600,
              color: c.text,
            ),
          ),
        ),
      ],
    );
  }
}