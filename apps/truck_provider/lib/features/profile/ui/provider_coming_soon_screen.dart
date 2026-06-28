import 'package:flutter/material.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import 'widgets/provider_profile_widgets.dart';

/// Placeholder for profile menu entries that exist in the Figma main-profile
/// screen but have no dedicated mockup yet (Safety & Emergency, Security,
/// Privacy, Payment & Withdrawals).
class ProviderComingSoonScreen extends StatelessWidget {
  const ProviderComingSoonScreen({super.key, required this.title});

  final String title;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 16, 20, 24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              ProviderProfileHeader(title: title),
              const Spacer(),
              Center(
                child: Column(
                  children: [
                    Container(
                      width: 96,
                      height: 96,
                      decoration: const BoxDecoration(color: kProviderGreenTint, shape: BoxShape.circle),
                      child: const Icon(Icons.construction_rounded, color: kProviderGreen, size: 44),
                    ),
                    const SizedBox(height: 20),
                    const Text(
                      'Coming soon',
                      style: TextStyle(color: kProviderText, fontSize: 18, fontWeight: FontWeight.w800),
                    ),
                    const SizedBox(height: 8),
                    Text(
                      "$title settings will be available in an upcoming update.",
                      textAlign: TextAlign.center,
                      style: const TextStyle(color: kProviderMuted, fontSize: 13, height: 1.5),
                    ),
                  ],
                ),
              ),
              const Spacer(),
            ],
          ),
        ),
      ),
    );
  }
}
