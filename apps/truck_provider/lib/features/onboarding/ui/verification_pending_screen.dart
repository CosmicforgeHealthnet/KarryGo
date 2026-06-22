import 'package:flutter/material.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../../auth/state/provider_auth_controller.dart';

class VerificationPendingScreen extends StatelessWidget {
  const VerificationPendingScreen({super.key, required this.controller});
  final ProviderAuthController controller;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: kProviderSurface,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const Spacer(),

              // ── Celebration illustration ──────────────────────────────
              Center(
                child: Container(
                  width: 160,
                  height: 160,
                  decoration: BoxDecoration(
                    color: kProviderGreenTint,
                    shape: BoxShape.circle,
                  ),
                  child: const Stack(
                    alignment: Alignment.center,
                    children: [
                      Icon(Icons.verified_rounded, size: 80, color: kProviderGreen),
                    ],
                  ),
                ),
              ),
              const SizedBox(height: 40),

              // ── Title ─────────────────────────────────────────────────
              const Text(
                'Profile submitted,\nVerification Pending!',
                textAlign: TextAlign.center,
                style: TextStyle(
                  color: kProviderText,
                  fontSize: 24,
                  fontWeight: FontWeight.w800,
                  height: 1.3,
                ),
              ),
              const SizedBox(height: 14),
              const Text(
                'Your profile is under review by our team. We\'ll notify you once verification is complete — this usually takes 1–2 business days.',
                textAlign: TextAlign.center,
                style: TextStyle(color: kProviderMuted, fontSize: 14, height: 1.6),
              ),

              const Spacer(),

              // ── Bullet points ─────────────────────────────────────────
              _BulletPoint(
                icon: Icons.schedule_rounded,
                text: 'Verification takes 1–2 business days.',
              ),
              const SizedBox(height: 10),
              _BulletPoint(
                icon: Icons.notifications_rounded,
                text: 'You\'ll receive a notification once approved.',
              ),
              const SizedBox(height: 10),
              _BulletPoint(
                icon: Icons.help_outline_rounded,
                text: 'Contact support if you have any questions.',
              ),

              const SizedBox(height: 32),

              // ── CTA button ────────────────────────────────────────────
              SizedBox(
                height: 52,
                child: FilledButton(
                  onPressed: controller.goToDashboard,
                  style: FilledButton.styleFrom(
                    backgroundColor: kProviderGreen,
                    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
                  ),
                  child: const Text(
                    'Go to dashboard',
                    style: TextStyle(color: Colors.white, fontWeight: FontWeight.w700, fontSize: 16),
                  ),
                ),
              ),
              const SizedBox(height: 16),
            ],
          ),
        ),
      ),
    );
  }
}

class _BulletPoint extends StatelessWidget {
  const _BulletPoint({required this.icon, required this.text});
  final IconData icon;
  final String text;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Container(
          width: 36,
          height: 36,
          decoration: const BoxDecoration(
            color: kProviderGreenTint,
            shape: BoxShape.circle,
          ),
          child: Icon(icon, color: kProviderGreen, size: 18),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Text(
            text,
            style: const TextStyle(color: kProviderText, fontSize: 13, height: 1.4),
          ),
        ),
      ],
    );
  }
}
