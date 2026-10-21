import 'package:flutter/material.dart';

import '../../../core/widgets/karryam_ui.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../../auth/state/provider_auth_controller.dart';

class VerificationPendingScreen extends StatelessWidget {
  const VerificationPendingScreen({super.key, required this.controller});
  final ProviderAuthController controller;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(24, 40, 24, 24),
                children: [
                  Center(
                    child: const KAnimArt(KArt.pending, size: 160),
                  ),
                  const SizedBox(height: 20),
                  const Center(child: KStatusChip('Verification in review', tone: KTone.warning)),
                  const SizedBox(height: 14),
                  const KHeading(
                    center: true,
                    title: "You're nearly there.",
                    subtitle:
                        "Your details are submitted. Our team reviews them within 1–2 business days, and we'll notify you once your account is ready.",
                  ),
                  const SizedBox(height: 28),
                  const KGroup(
                    children: [
                      _Item(icon: Icons.person_outline_rounded, title: 'Personal details', detail: 'Received'),
                      _Item(icon: Icons.badge_outlined, title: 'Driver documents', detail: 'Received'),
                      _Item(icon: Icons.local_shipping_outlined, title: 'Truck information', detail: 'Received'),
                      _Item(icon: Icons.notifications_none_rounded, title: 'Approval', detail: "We'll notify you", done: false),
                    ],
                  ),
                  const SizedBox(height: 16),
                  const KNotice(
                    'Questions? Reach support from Profile → Support once you are in.',
                    tone: KTone.info,
                  ),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 8, 24, 16),
              child: KPrimaryButton(label: 'Go to dashboard', arrow: true, onPressed: controller.goToDashboard),
            ),
          ],
        ),
      ),
    );
  }
}

class _Item extends StatelessWidget {
  const _Item({required this.icon, required this.title, required this.detail, this.done = true});
  final IconData icon;
  final String title;
  final String detail;
  final bool done;

  @override
  Widget build(BuildContext context) {
    return KListRow(
      icon: icon,
      title: title,
      subtitle: detail,
      trailing: done
          ? Icon(Icons.check_circle_rounded, color: kTealText, size: 20)
          : Icon(Icons.schedule_rounded, color: kWarning, size: 20),
    );
  }
}
