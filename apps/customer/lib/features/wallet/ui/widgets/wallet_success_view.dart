import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';

/// Generic full-screen success view (mockup #11) reused by the funding and
/// withdrawal flows. Shows a check badge, message, and one or two actions.
class WalletSuccessView extends StatelessWidget {
  const WalletSuccessView({
    super.key,
    required this.title,
    required this.message,
    required this.primaryLabel,
    required this.onPrimary,
    this.secondaryLabel,
    this.onSecondary,
    this.icon = Icons.check_rounded,
    this.iconColor = CustomerFigmaColors.primary,
  });

  final String title;
  final String message;
  final String primaryLabel;
  final VoidCallback onPrimary;
  final String? secondaryLabel;
  final VoidCallback? onSecondary;
  final IconData icon;
  final Color iconColor;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            children: [
              const Spacer(),
              Container(
                width: 96,
                height: 96,
                decoration: BoxDecoration(
                  color: iconColor.withValues(alpha: 0.12),
                  shape: BoxShape.circle,
                ),
                child: Center(
                  child: Container(
                    width: 64,
                    height: 64,
                    decoration: BoxDecoration(
                      color: iconColor,
                      shape: BoxShape.circle,
                    ),
                    child: Icon(icon, color: Colors.white, size: 34),
                  ),
                ),
              ),
              const SizedBox(height: 24),
              Text(
                title,
                textAlign: TextAlign.center,
                style: const TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 20,
                  fontWeight: FontWeight.w900,
                ),
              ),
              const SizedBox(height: 10),
              Text(
                message,
                textAlign: TextAlign.center,
                style: const TextStyle(
                  color: CustomerFigmaColors.muted,
                  fontSize: 14,
                  height: 1.4,
                ),
              ),
              const Spacer(),
              FigmaPrimaryButton(label: primaryLabel, onPressed: onPrimary),
              if (secondaryLabel != null && onSecondary != null) ...[
                const SizedBox(height: 12),
                FigmaSecondaryButton(
                  label: secondaryLabel!,
                  onPressed: onSecondary!,
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}
