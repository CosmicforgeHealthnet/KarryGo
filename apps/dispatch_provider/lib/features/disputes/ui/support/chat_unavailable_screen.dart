import '../../../../shared/widgets/dispatch_screen_frame.dart';
import 'package:flutter/material.dart';

import '../../../../core/theme/theme_extensions.dart';
import '../../state/rider_dispute_controller.dart';
import 'create_case_screen.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 12 — Chat unavailable
// Headset hero · "Chat isn't available right now" · Create support case
// ═══════════════════════════════════════════════════════════════════════════

class ChatUnavailableScreen extends StatelessWidget {
  const ChatUnavailableScreen({super.key, required this.disputeController});

  final RiderDisputeController disputeController;

  Future<void> _openCreate(BuildContext context) async {
    final r = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) => CreateCaseScreen(disputeController: disputeController),
      ),
    );
    if (!context.mounted) return;
    if (r == true) Navigator.of(context).pop();
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
                  DispatchBackButton(
                    onPressed: () => Navigator.of(context).pop(),
                  ),
                  const SizedBox(width: 4),
                  Expanded(
                    child: Text(
                      'Support Chat',
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
              child: Center(
                child: Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 32),
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Container(
                        width: 120,
                        height: 120,
                        alignment: Alignment.center,
                        decoration: BoxDecoration(
                          color: c.navy10,
                          shape: BoxShape.circle,
                        ),
                        child: Stack(
                          alignment: Alignment.center,
                          children: [
                            Icon(
                              Icons.headset_mic_outlined,
                              size: 58,
                              color: c.textMuted,
                            ),
                            Positioned(
                              top: 30,
                              right: 26,
                              child: Container(
                                width: 24,
                                height: 24,
                                alignment: Alignment.center,
                                decoration: BoxDecoration(
                                  color: c.danger,
                                  shape: BoxShape.circle,
                                  border: Border.all(
                                    color: c.background,
                                    width: 2,
                                  ),
                                ),
                                child: const Icon(
                                  Icons.close_rounded,
                                  size: 12,
                                  color: Colors.white,
                                ),
                              ),
                            ),
                          ],
                        ),
                      ),
                      const SizedBox(height: 28),
                      Text(
                        "Chat isn't available right now",
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
                        'Our support team is currently offline. Please try again later or create a support case.',
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
            ),

            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: () => _openCreate(context),
                  style: FilledButton.styleFrom(
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('Create support case'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
