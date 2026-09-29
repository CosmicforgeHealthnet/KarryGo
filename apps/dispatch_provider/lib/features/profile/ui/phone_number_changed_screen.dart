import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 22 — Phone number updated!
// Full light screen. Green check hero · confirmation text · navy Continue.
// ═══════════════════════════════════════════════════════════════════════════

class PhoneNumberChangedScreen extends StatelessWidget {
  const PhoneNumberChangedScreen({
    super.key,
    required this.newPhoneDisplay,
  });

  final String newPhoneDisplay;

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
                width: 96,
                height: 96,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: c.successSoft,
                ),
                child: Icon(
                  Icons.check_rounded,
                  size: 54,
                  color: c.success,
                ),
              ),
            ),
            const SizedBox(height: 28),
            Text(
              'Phone number updated!',
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
              child: RichText(
                textAlign: TextAlign.center,
                text: TextSpan(
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w500,
                    color: c.textMuted,
                    height: 1.5,
                  ),
                  children: [
                    const TextSpan(
                      text:
                          'Your phone number has been successfully changed to ',
                    ),
                    TextSpan(
                      text: newPhoneDisplay,
                      style: TextStyle(
                        color: c.text,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                  ],
                ),
              ),
            ),
            const Spacer(),

            // ── Navy Continue ──
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: () => Navigator.of(context).pop(true),
                  style: FilledButton.styleFrom(
                    backgroundColor: c.brandNavy900,
                    foregroundColor: c.brandWhite,
                    shape: const StadiumBorder(),
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