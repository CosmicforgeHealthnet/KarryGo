import 'package:flutter/material.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_profile_controller.dart';
import 'provider_emergency_contact_screen.dart';
import 'provider_guarantor_screen.dart';
import 'widgets/provider_profile_widgets.dart';

/// Safety & Emergency landing screen: two rows leading to the Emergency
/// Contact and Guarantor Information screens.
class ProviderSafetyEmergencyScreen extends StatelessWidget {
  const ProviderSafetyEmergencyScreen({super.key, required this.profileController});

  final ProviderProfileController profileController;

  void _push(BuildContext context, Widget screen) {
    Navigator.of(context).push(MaterialPageRoute(builder: (_) => screen));
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
          children: [
            const ProviderProfileHeader(
              title: 'Safety & Emergency',
              subtitle: 'Providing this information guarantees safety and security during emergencies.',
            ),
            const SizedBox(height: 24),
            _SafetyMenuItem(
              label: 'Emergency Contact',
              onTap: () => _push(
                context,
                ProviderEmergencyContactScreen(profileController: profileController),
              ),
            ),
            _SafetyMenuItem(
              label: 'Guarantor Information',
              onTap: () => _push(
                context,
                ProviderGuarantorScreen(profileController: profileController),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _SafetyMenuItem extends StatelessWidget {
  const _SafetyMenuItem({required this.label, required this.onTap});

  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 16),
      child: Material(
        color: Colors.white,
        borderRadius: BorderRadius.circular(12),
        child: InkWell(
          onTap: onTap,
          borderRadius: BorderRadius.circular(12),
          child: Ink(
            decoration: BoxDecoration(
              color: Colors.white,
              borderRadius: BorderRadius.circular(12),
              boxShadow: const [BoxShadow(color: Color(0x0F000000), blurRadius: 16, offset: Offset(0, 4))],
            ),
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 18),
            child: Row(
              children: [
                Expanded(
                  child: Text(
                    label,
                    style: const TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w700),
                  ),
                ),
                const Icon(Icons.chevron_right_rounded, color: kProviderMuted, size: 22),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
