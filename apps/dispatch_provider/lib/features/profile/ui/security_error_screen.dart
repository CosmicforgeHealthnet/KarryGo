import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 24 — Access restricted (security error)
// ═══════════════════════════════════════════════════════════════════════════

class SecurityErrorScreen extends StatelessWidget {
  const SecurityErrorScreen({super.key, this.onContactSupport});

  final VoidCallback? onContactSupport;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            const Spacer(),
            Center(
              child: Container(
                width: 88,
                height: 88,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: c.dangerSoft,
                ),
                child: Icon(
                  Icons.warning_amber_rounded,
                  size: 46,
                  color: c.danger,
                ),
              ),
            ),
            const SizedBox(height: 24),
            Text(
              'Access restricted',
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 24,
                fontWeight: FontWeight.w800,
                color: c.text,
                letterSpacing: -0.4,
              ),
            ),
            const SizedBox(height: 12),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 32),
              child: Text(
                'We detected unusual activity on your account. '
                'For your safety, access has been temporarily limited. '
                'Please contact support to regain access.',
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w500,
                  color: c.textMuted,
                  height: 1.5,
                ),
              ),
            ),
            const Spacer(),

            // ── Navy Contact support ──
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: onContactSupport ?? () {},
                  style: FilledButton.styleFrom(
                    backgroundColor: c.brandNavy900,
                    foregroundColor: c.brandWhite,
                    shape: const StadiumBorder(),
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('Contact support'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}