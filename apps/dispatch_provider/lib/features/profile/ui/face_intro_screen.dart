import '../../../shared/widgets/journey_icon.dart';
import '../../../shared/widgets/dispatch_screen_frame.dart';
import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../verification/state/verification_controller.dart';
import 'selfie_capture_screen.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 05 — Face verification intro
// ═══════════════════════════════════════════════════════════════════════════

class FaceIntroScreen extends StatelessWidget {
  const FaceIntroScreen({super.key, required this.verificationController});

  final VerificationController verificationController;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            // ── Header: back · page title · Help ──
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
              child: Row(
                children: [
                  DispatchBackButton(
                    onPressed: () => Navigator.of(context).pop(),
                  ),
                  const SizedBox(width: 4),
                  Expanded(
                    child: Text(
                      'Face verification',
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                        color: c.text,
                        letterSpacing: -0.2,
                      ),
                    ),
                  ),
                  GestureDetector(
                    onTap: () {},
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Text(
                        'Help',
                        style: TextStyle(
                          fontSize: 15,
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
                  // ── Hero ──
                  Center(
                    child: Container(
                      width: 140,
                      height: 140,
                      alignment: Alignment.center,
                      decoration: BoxDecoration(
                        color: c.surfaceElevated,
                        borderRadius: BorderRadius.circular(30),
                      ),
                      child: JourneyIcon(
                        JourneySymbol.profile,
                        size: 76,
                        color: c.brandTeal500,
                      ),
                    ),
                  ),
                  const SizedBox(height: 32),

                  // ── Big title ──
                  Text(
                    "Let's verify it's you",
                    textAlign: TextAlign.center,
                    style: TextStyle(
                      fontSize: 28,
                      fontWeight: FontWeight.w700,
                      color: c.text,
                      letterSpacing: -0.4,
                    ),
                  ),
                  const SizedBox(height: 12),

                  // ── Subtitle ──
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 4),
                    child: Text(
                      "We'll take a quick selfie to match your face with your ID and keep your account secure.",
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w500,
                        color: c.textMuted,
                        height: 1.5,
                      ),
                    ),
                  ),
                  const SizedBox(height: 40),

                  // ── 3 bullets ──
                  const _Bullet(
                    icon: Icons.wb_sunny_outlined,
                    text: 'Be in a well-lit area',
                  ),
                  const SizedBox(height: 22),
                  const _Bullet(
                    icon: Icons.photo_camera_outlined,
                    text: 'Look directly at the camera',
                  ),
                  const SizedBox(height: 22),
                  const _Bullet(
                    icon: Icons.person_outline_rounded,
                    text: 'Remove hats or sunglasses',
                  ),
                ],
              ),
            ),

            // ── Continue ──
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: () async {
                    final r = await Navigator.of(context).push<bool>(
                      MaterialPageRoute(
                        builder: (_) => SelfieCaptureScreen(
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
// Bullet — teal icon on left, dark label on right. No container.
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
        Icon(icon, size: 26, color: c.brandTeal500),
        const SizedBox(width: 16),
        Expanded(
          child: Text(
            text,
            style: TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w600,
              color: c.text,
            ),
          ),
        ),
      ],
    );
  }
}
