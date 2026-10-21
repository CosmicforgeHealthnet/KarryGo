import 'package:flutter/material.dart';

import '../../disputes/state/provider_dispute_controller.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import 'provider_support_screen.dart';
import 'widgets/provider_profile_widgets.dart';

/// What the app collects about the driver, why, and who to ask about it.
class ProviderPrivacyScreen extends StatelessWidget {
  const ProviderPrivacyScreen({super.key, required this.disputeController});

  final ProviderDisputeController disputeController;

  static const _items = <(IconData, String, String)>[
    (
      Icons.location_on_outlined,
      'Your location',
      'Only while you are online or on a job: to offer you nearby requests and show customers where their cargo is. Going offline stops it.',
    ),
    (
      Icons.badge_outlined,
      'Your documents',
      'Your ID, licence and truck papers are stored privately and seen only by the team that verifies drivers.',
    ),
    (
      Icons.photo_camera_outlined,
      'Photos and signatures',
      'Proof-of-delivery photos and signatures are kept with the job, in case of a dispute.',
    ),
    (
      Icons.notifications_none_rounded,
      'Notifications',
      'Your phone receives job offers and updates. You can turn notifications off in your phone settings.',
    ),
    (
      Icons.account_balance_outlined,
      'Bank details',
      'Used only to pay your withdrawals. Account names are checked with your bank.',
    ),
  ];

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.fromLTRB(20, 16, 20, 24),
          children: [
            const ProviderProfileHeader(title: 'Privacy', subtitle: 'How your information is used'),
            const SizedBox(height: 20),
            for (final (icon, title, body) in _items)
              Padding(
                padding: const EdgeInsets.only(bottom: 16),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Container(
                      width: 40,
                      height: 40,
                      decoration: const BoxDecoration(color: kProviderGreenTint, shape: BoxShape.circle),
                      child: Icon(icon, color: kProviderGreen, size: 20),
                    ),
                    const SizedBox(width: 12),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(title, style: const TextStyle(color: kProviderText, fontWeight: FontWeight.w800, fontSize: 15)),
                          const SizedBox(height: 4),
                          Text(body, style: const TextStyle(color: kProviderMuted, fontSize: 13, height: 1.4)),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
            const SizedBox(height: 8),
            ProviderPrimaryButton(
              label: 'Ask about my data',
              onPressed: () => Navigator.of(context).push(
                MaterialPageRoute(builder: (_) => ProviderLiveChatScreen(controller: disputeController)),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
