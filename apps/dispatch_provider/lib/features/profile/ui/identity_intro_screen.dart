import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../verification/state/verification_controller.dart';
import 'identity_details_screen.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 02 — Identity introduction
// ═══════════════════════════════════════════════════════════════════════════

class IdentityIntroScreen extends StatelessWidget {
  const IdentityIntroScreen({
    super.key,
    required this.verificationController,
  });

  final VerificationController verificationController;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            // ── Header: back · Help ──
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
                  const Spacer(),
                  GestureDetector(
                    onTap: () {},
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Text(
                        'Help',
                        style: TextStyle(
                          fontSize: 15,              // ← bumped from 14
                          fontWeight: FontWeight.w600,
                          color: c.brandTeal500,
                        ),
                      ),
                    ),
                  ),
                ],
              ),
            ),

            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(24, 24, 24, 24),
                children: [
                  // ── Hero: light rounded square + teal shield ──
                  Center(
                    child: Container(
                      width: 140,                      // ← bumped from 128
                      height: 140,
                      alignment: Alignment.center,
                      decoration: BoxDecoration(
                        color: c.surfaceElevated,
                        borderRadius: BorderRadius.circular(30),
                      ),
                      child: Icon(
                        Icons.gpp_good_rounded,
                        size: 84,                      // ← bumped from 76
                        color: c.brandTeal500,
                      ),
                    ),
                  ),
                  const SizedBox(height: 32),

                  // ── Title ──
                  Text(
                    'Verify your identity',
                    textAlign: TextAlign.center,
                    style: TextStyle(
                      fontSize: 28,                    // ← bumped from 24
                      fontWeight: FontWeight.w800,
                      color: c.text,
                      letterSpacing: -0.5,
                    ),
                  ),
                  const SizedBox(height: 12),

                  // ── Subtitle ──
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 4),
                    child: Text(
                      "We'll need your government ID (NIN or International Passport) to confirm who you are.",
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        fontSize: 15,                  // ← bumped from 14
                        color: c.textMuted,
                        height: 1.5,
                      ),
                    ),
                  ),
                  const SizedBox(height: 40),

                  // ── 3 bullets — teal icons, no background ──
                  const _Bullet(
                    icon: Icons.lock_outline_rounded,
                    text: 'Keeps your account secure',
                  ),
                  const SizedBox(height: 22),
                  const _Bullet(
                    icon: Icons.verified_outlined,
                    text: 'Helps build trust on Karryam',
                  ),
                  const SizedBox(height: 22),
                  const _Bullet(
                    icon: Icons.schedule_rounded,
                    text: 'Only takes a few minutes',
                  ),
                ],
              ),
            ),

            // ── Continue — teal pill ──
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: () async {
                    final r = await Navigator.of(context).push<bool>(
                      MaterialPageRoute(
                        builder: (_) => IdentityDetailsScreen(
                          verificationController: verificationController,
                        ),
                      ),
                    );
                    if (!context.mounted) return;
                    if (r == true) Navigator.of(context).pop(true);
                  },
                  style: FilledButton.styleFrom(
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('Continue'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Bullet — teal icon on the left, dark label on the right. No container.
// ═══════════════════════════════════════════════════════════════════════════

class _Bullet extends StatelessWidget {
  const _Bullet({required this.icon, required this.text});
  final IconData icon;
  final String text;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Row(
      children: [
        Icon(icon, size: 26, color: c.brandTeal500),   // ← bumped from 22
        const SizedBox(width: 16),
        Expanded(
          child: Text(
            text,
            style: TextStyle(
              fontSize: 16,                             // ← bumped from 15
              fontWeight: FontWeight.w600,
              color: c.text,
            ),
          ),
        ),
      ],
    );
  }
}