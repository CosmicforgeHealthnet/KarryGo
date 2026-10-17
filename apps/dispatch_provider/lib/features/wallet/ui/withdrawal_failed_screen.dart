import 'package:flutter/material.dart';

import '../../../core/theme/dispatch_colors.dart';

class WithdrawalFailedScreen extends StatelessWidget {
  const WithdrawalFailedScreen({super.key, required this.reason});

  /// Short message shown under the heading. Pass null for the default.
  final String? reason;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: DispatchColors.brandWhite,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 24),
          child: Column(
            children: [
              const Spacer(flex: 2),

              // Big red exclamation circle
              Container(
                width: 96,
                height: 96,
                decoration: const BoxDecoration(
                  color: Color(0xFFE5484D),
                  shape: BoxShape.circle,
                ),
                child: const Icon(
                  Icons.priority_high_rounded,
                  size: 52,
                  color: Colors.white,
                ),
              ),
              const SizedBox(height: 28),

              const Text(
                'Withdrawal failed',
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFFE5484D),
                  letterSpacing: -0.4,
                  height: 1.2,
                ),
              ),
              const SizedBox(height: 12),

              Text(
                reason ??
                    "We couldn't process your withdrawal at this time. "
                        'Please try again or contact support.',
                textAlign: TextAlign.center,
                style: const TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w400,
                  color: DispatchColors.navy60,
                  height: 1.5,
                ),
              ),

              const Spacer(flex: 3),

              // Try again
              SizedBox(
                width: double.infinity,
                height: 56,
                child: FilledButton(
                  onPressed: () => Navigator.of(context).pop(),
                  style: FilledButton.styleFrom(
                    backgroundColor: const Color(0xFFE5484D),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(14),
                    ),
                  ),
                  child: const Text(
                    'Try again',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                      color: Colors.white,
                    ),
                  ),
                ),
              ),
              const SizedBox(height: 12),

              // Contact support
              SizedBox(
                width: double.infinity,
                height: 56,
                child: OutlinedButton(
                  onPressed: () {
                    // TODO: navigate to support
                  },
                  style: OutlinedButton.styleFrom(
                    side: const BorderSide(
                      color: DispatchColors.navy20,
                      width: 1.2,
                    ),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(14),
                    ),
                  ),
                  child: const Text(
                    'Contact support',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                      color: DispatchColors.brandNavy900,
                    ),
                  ),
                ),
              ),
              const SizedBox(height: 20),
            ],
          ),
        ),
      ),
    );
  }
}
