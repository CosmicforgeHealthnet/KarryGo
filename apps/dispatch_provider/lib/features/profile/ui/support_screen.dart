import 'package:flutter/material.dart';

import '../../disputes/state/rider_dispute_controller.dart';
import 'live_chat_screen.dart';

class SupportScreen extends StatelessWidget {
  const SupportScreen({super.key, required this.disputeController});

  final RiderDisputeController disputeController;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFFF7F7F7),
      body: SafeArea(
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 12, 20, 0),
              child: GestureDetector(
                behavior: HitTestBehavior.opaque,
                onTap: () => Navigator.of(context).pop(),
                child: const Align(
                  alignment: Alignment.centerLeft,
                  child: Icon(
                    Icons.arrow_back_ios_new,
                    size: 20,
                    color: Color(0xFF1A1A1A),
                  ),
                ),
              ),
            ),
            const Spacer(),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 32),
              child: Column(
                children: [
                  Image.asset(
                    'assets/figma/profile_submitted.png',
                    height: 200,
                    errorBuilder: (_, __, ___) => const SizedBox(height: 200),
                  ),
                  const SizedBox(height: 24),
                  const Text(
                    'Contact Support!',
                    style: TextStyle(
                      fontSize: 20,
                      fontWeight: FontWeight.w700,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 8),
                  const Text(
                    'Our dedicated team is here to assist you with any questions or issues related to your experience.',
                    textAlign: TextAlign.center,
                    style: TextStyle(
                      fontSize: 13,
                      color: Color(0xFF888888),
                      height: 1.5,
                    ),
                  ),
                ],
              ),
            ),
            const Spacer(),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton.icon(
                  onPressed: () => Navigator.of(context).push(
                    MaterialPageRoute(
                      builder: (_) =>
                          LiveChatScreen(disputeController: disputeController),
                    ),
                  ),
                  icon: const Icon(
                    Icons.chat_bubble_outline,
                    size: 18,
                    color: Color(0xFF081F44),
                  ),
                  label: const Text(
                    'Start Chat',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                      color: Color(0xFF081F44),
                    ),
                  ),
                  style: FilledButton.styleFrom(
                    backgroundColor: const Color(0xFF05124D),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(999),
                    ),
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
