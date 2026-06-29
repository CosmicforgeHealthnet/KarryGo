import 'package:flutter/material.dart';
import '../state/provider_profile_controller.dart';
import 'emergency_contact_screen.dart';
import 'guarantor_information_screen.dart';

class SafetyEmergencyScreen extends StatelessWidget {
  const SafetyEmergencyScreen({super.key, required this.profileController});

  final ProviderProfileController profileController;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
          children: [
            // ── Header ──────────────────────────────────────────
            GestureDetector(
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
            const SizedBox(height: 16),
            const Text(
              'Safety & Emergency',
              style: TextStyle(
                fontSize: 20,
                fontWeight: FontWeight.w800,
                color: Color(0xFF1A1A1A),
              ),
            ),
            const SizedBox(height: 2),
            const Text(
              'Providing this information guarantees safety and security during emergencies.',
              style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
            ),

            const SizedBox(height: 24),

            // ── Menu items ───────────────────────────────────────
            _MenuItem(
              label: 'Emergency Contact',
              onTap: () => Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => EmergencyContactScreen(
                    profileController: profileController,
                  ),
                ),
              ),
            ),
            const SizedBox(height: 10),
            _MenuItem(
              label: 'Guarantor Information',
              onTap: () => Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => GuarantorInformationScreen(
                    profileController: profileController,
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

class _MenuItem extends StatelessWidget {
  const _MenuItem({required this.label, required this.onTap});
  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.white,
      borderRadius: BorderRadius.circular(12),
      child: InkWell(
        borderRadius: BorderRadius.circular(12),
        onTap: onTap,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 18),
          decoration: BoxDecoration(
            border: Border.all(color: const Color(0xFFEEEEEE)),
            borderRadius: BorderRadius.circular(12),
          ),
          child: Row(
            children: [
              Expanded(
                child: Text(
                  label,
                  style: const TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w700,
                    color: Color(0xFF1A1A1A),
                  ),
                ),
              ),
              const Icon(
                Icons.chevron_right,
                size: 20,
                color: Color(0xFF888888),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
