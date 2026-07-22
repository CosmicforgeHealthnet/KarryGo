import 'package:flutter/material.dart';
import 'password_screen.dart';
import 'transaction_pin_screen.dart';
import 'two_step_verification_screen.dart';
import '../../wallet/state/wallet_controller.dart';

class SecurityScreen extends StatelessWidget {
  const SecurityScreen({super.key, required this.walletController});

  final WalletController walletController;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
          children: [
            GestureDetector(
              behavior: HitTestBehavior.opaque,
              onTap: () => Navigator.of(context).pop(),
              child: const Align(
                alignment: Alignment.centerLeft,
                child: Icon(Icons.arrow_back_ios_new,
                    size: 20, color: Color(0xFF1A1A1A)),
              ),
            ),
            const SizedBox(height: 16),
            const Text(
              'Security',
              style: TextStyle(
                fontSize: 20,
                fontWeight: FontWeight.w800,
                color: Color(0xFF1A1A1A),
              ),
            ),
            const SizedBox(height: 2),
            const Text(
              'We try our best to keep you safe.',
              style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
            ),
            const SizedBox(height: 24),
            _MenuItem(
              label: 'Passwords',
              subtitle: 'Manage passwords and access to your account',
              onTap: () => Navigator.of(context).push(
                MaterialPageRoute(builder: (_) => const PasswordScreen()),
              ),
            ),
            const SizedBox(height: 10),
            _MenuItem(
              label: 'Transaction Pin',
              subtitle: 'Manage Transaction Pin and access to your earnings',
              onTap: () => Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) =>
                      TransactionPinScreen(walletController: walletController),
                ),
              ),
            ),
            const SizedBox(height: 10),
            _MenuItem(
              label: 'Two-step Verification',
              subtitle: 'This adds extra security through email verification',
              onTap: () => Navigator.of(context).push(
  MaterialPageRoute(builder: (_) => const TwoStepVerificationScreen()),
),
            ),
          ],
        ),
      ),
    );
  }
}

class _MenuItem extends StatelessWidget {
  const _MenuItem({
    required this.label,
    required this.subtitle,
    required this.onTap,
  });

  final String label;
  final String subtitle;
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
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
          decoration: BoxDecoration(
            border: Border.all(color: const Color(0xFFEEEEEE)),
            borderRadius: BorderRadius.circular(12),
          ),
          child: Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      label,
                      style: const TextStyle(
                        fontSize: 14,
                        fontWeight: FontWeight.w700,
                        color: Color(0xFF1A1A1A),
                      ),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      subtitle,
                      style: const TextStyle(
                        fontSize: 12,
                        color: Color(0xFF888888),
                      ),
                    ),
                  ],
                ),
              ),
              const Icon(Icons.chevron_right, size: 20, color: Color(0xFF888888)),
            ],
          ),
        ),
      ),
    );
  }
}