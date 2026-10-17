import '../../../../shared/widgets/dispatch_screen_frame.dart';
import 'package:flutter/material.dart';

import '../../../../core/theme/theme_extensions.dart';
import '../../state/rider_dispute_controller.dart';
import 'create_case_screen.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 02 — FAQ / Category detail
// Category header · FAQ rows · "Still need help?" → Create case
// ═══════════════════════════════════════════════════════════════════════════

class FaqCategoryScreen extends StatelessWidget {
  const FaqCategoryScreen({
    super.key,
    required this.categoryTitle,
    required this.disputeController,
  });

  final String categoryTitle;
  final RiderDisputeController disputeController;

  static const _faqs = <String, List<String>>{
    'Deliveries & Pickups': [
      'Where do I pick up a package?',
      'What if the recipient is unavailable?',
      'Can I change the delivery location?',
      'What if the package is damaged?',
      'How do I mark a delivery complete?',
      'Can I get a new assignment?',
      'What if I\'m running late?',
      'Other delivery questions',
    ],
    'Earnings & Payments': [
      'When do I get paid for completed trips?',
      'How is my earnings calculated?',
      'Why is a payout pending?',
      'What fees does Karryam charge?',
      'How do I see my earnings history?',
      'Can I withdraw before the cycle ends?',
    ],
    'Account & Verification': [
      'How do I verify my identity?',
      'Why was my verification rejected?',
      'How do I update my phone number?',
      'How do I change my profile photo?',
      'How do I deactivate my account?',
    ],
    'App & Technical Issues': [
      'The app is slow or crashing',
      'Notifications are delayed',
      'I can\'t log in',
      'Location is not working',
      'The map is not loading',
    ],
    'Safety & Conduct': [
      'How do I report a customer?',
      'What is the safety policy?',
      'What do I do in an emergency?',
      'How is rider conduct evaluated?',
    ],
  };

  void _openCreateCase(BuildContext context) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => CreateCaseScreen(
          disputeController: disputeController,
          initialCategory: categoryTitle,
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final faqs = _faqs[categoryTitle] ?? const <String>[];

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
                      categoryTitle,
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
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                children: [
                  const SizedBox(height: 4),
                  Text(
                    'What do you need help with?',
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w500,
                      color: c.textMuted,
                      height: 1.45,
                    ),
                  ),
                  const SizedBox(height: 20),

                  for (final q in faqs) ...[
                    _FaqRow(
                      question: q,
                      onTap: () {
                        Navigator.of(context).push(
                          MaterialPageRoute(
                            builder: (_) => CreateCaseScreen(
                              disputeController: disputeController,
                              initialCategory: categoryTitle,
                              initialSubject: q,
                            ),
                          ),
                        );
                      },
                    ),
                    const SizedBox(height: 10),
                  ],

                  const SizedBox(height: 20),
                  Text(
                    'Still need help?',
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                      color: c.text,
                    ),
                  ),
                  const SizedBox(height: 6),
                  Text(
                    "Create a support case and we'll get back to you.",
                    style: TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w500,
                      color: c.textMuted,
                      height: 1.4,
                    ),
                  ),
                ],
              ),
            ),

            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: () => _openCreateCase(context),
                  style: FilledButton.styleFrom(
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('Create a case'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _FaqRow extends StatelessWidget {
  const _FaqRow({required this.question, required this.onTap});
  final String question;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: c.surface,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: c.divider),
        ),
        child: Row(
          children: [
            Expanded(
              child: Text(
                question,
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w600,
                  color: c.text,
                  letterSpacing: -0.1,
                ),
              ),
            ),
            const SizedBox(width: 12),
            Icon(Icons.chevron_right_rounded, size: 22, color: c.textMuted),
          ],
        ),
      ),
    );
  }
}
