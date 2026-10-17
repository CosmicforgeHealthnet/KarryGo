import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 05 — Stale / expired deep link
// ═══════════════════════════════════════════════════════════════════════════

class StaleLinkScreen extends StatelessWidget {
  const StaleLinkScreen({
    super.key,
    this.onBrowse,
    this.onHome,
    this.onActivity,
    this.onContactSupport,
  });

  final VoidCallback? onBrowse;
  final VoidCallback? onHome;
  final VoidCallback? onActivity;
  final VoidCallback? onContactSupport;

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
                  const SizedBox(width: 4),
                  Expanded(
                    child: Text(
                      'This link is no longer available',
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                        color: c.text,
                        letterSpacing: -0.2,
                      ),
                    ),
                  ),
                ],
              ),
            ),

            Expanded(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(24, 40, 24, 24),
                child: Column(
                  children: [
                    Container(
                      width: 120,
                      height: 120,
                      alignment: Alignment.center,
                      decoration: BoxDecoration(
                        color: c.navy10,
                        shape: BoxShape.circle,
                      ),
                      child: Icon(
                        Icons.link_off_rounded,
                        size: 60,
                        color: c.textMuted,
                      ),
                    ),
                    const SizedBox(height: 28),
                    Text(
                      'This link is no longer available',
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        fontSize: 22,
                        fontWeight: FontWeight.w800,
                        color: c.text,
                        letterSpacing: -0.4,
                        height: 1.3,
                      ),
                    ),
                    const SizedBox(height: 12),
                    Text(
                      'This notification may be outdated or the opportunity is no longer available.',
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w500,
                        color: c.textMuted,
                        height: 1.5,
                      ),
                    ),
                  ],
                ),
              ),
            ),

            // ── CTAs ──
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 8),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: onBrowse ?? () => Navigator.of(context).pop(),
                  style: FilledButton.styleFrom(
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('Browse new opportunities'),
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 20),
              child: SizedBox(
                height: 48,
                width: double.infinity,
                child: OutlinedButton(
                  onPressed: onHome ?? () => Navigator.of(context).pop(),
                  style: OutlinedButton.styleFrom(
                    foregroundColor: c.text,
                    side: BorderSide(color: c.border, width: 1.2),
                    shape: const StadiumBorder(),
                    textStyle: const TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('Go to home'),
                ),
              ),
            ),
            const SizedBox(height: 8),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 20),
              child: SizedBox(
                height: 48,
                width: double.infinity,
                child: OutlinedButton(
                  onPressed: onActivity ?? () => Navigator.of(context).pop(),
                  style: OutlinedButton.styleFrom(
                    foregroundColor: c.text,
                    side: BorderSide(color: c.border, width: 1.2),
                    shape: const StadiumBorder(),
                    textStyle: const TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('View recent activity'),
                ),
              ),
            ),
            const SizedBox(height: 12),
            TextButton(
              onPressed: onContactSupport ?? () {},
              style: TextButton.styleFrom(
                foregroundColor: c.brandTeal600,
                textStyle: const TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w600,
                ),
              ),
              child: const Text('Need help? Contact support'),
            ),
            const SizedBox(height: 8),
          ],
        ),
      ),
    );
  }
}