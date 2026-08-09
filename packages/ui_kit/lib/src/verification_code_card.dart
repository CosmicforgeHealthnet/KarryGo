import 'package:flutter/material.dart';
import 'package:qr_flutter/qr_flutter.dart';

import '../cosmicforge_logistics_ui_kit.dart';

/// Displays a short-lived handover verification code (pickup or delivery)
/// as both large digits and a QR code, with an optional resend action.
///
/// Used on whichever side of the handover is *displaying* the code: the
/// driver's app for the pickup credential, the customer's app for the
/// delivery credential.
class CosmicforgeLogisticsVerificationCodeCard extends StatelessWidget {
  const CosmicforgeLogisticsVerificationCodeCard({
    super.key,
    required this.title,
    this.subtitle,
    this.code,
    this.expiresAt,
    this.isLoading = false,
    this.errorMessage,
    this.onResend,
    this.canResend = true,
    this.resendCooldownSeconds = 0,
  });

  final String title;
  final String? subtitle;

  /// The plaintext code to display. Null while still loading.
  final String? code;

  final DateTime? expiresAt;
  final bool isLoading;
  final String? errorMessage;

  /// Called when the user taps "Resend code". Null hides the action.
  final VoidCallback? onResend;

  /// Whether the resend action is currently enabled (false while the
  /// server-side cooldown is active).
  final bool canResend;

  /// Seconds remaining before a resend is allowed, shown next to a disabled
  /// resend action.
  final int resendCooldownSeconds;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(CosmicforgeLogisticsSpacing.md),
      decoration: BoxDecoration(
        color: CosmicforgeLogisticsColors.card,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: CosmicforgeLogisticsColors.border),
        boxShadow: const [
          BoxShadow(
            color: Color(0x14000000),
            blurRadius: 16,
            offset: Offset(0, 6),
          ),
        ],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          Text(
            title,
            textAlign: TextAlign.center,
            style: const TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w800,
              color: CosmicforgeLogisticsColors.text,
            ),
          ),
          if (subtitle != null) ...[
            const SizedBox(height: CosmicforgeLogisticsSpacing.xxs),
            Text(
              subtitle!,
              textAlign: TextAlign.center,
              style: const TextStyle(
                fontSize: 13,
                color: CosmicforgeLogisticsColors.textMuted,
              ),
            ),
          ],
          const SizedBox(height: CosmicforgeLogisticsSpacing.md),
          if (isLoading || code == null || code!.isEmpty)
            const Padding(
              padding: EdgeInsets.symmetric(
                vertical: CosmicforgeLogisticsSpacing.lg,
              ),
              child: SizedBox(
                width: 28,
                height: 28,
                child: CircularProgressIndicator(strokeWidth: 3),
              ),
            )
          else ...[
            QrImageView(
              data: code!,
              size: 148,
              backgroundColor: Colors.white,
            ),
            const SizedBox(height: CosmicforgeLogisticsSpacing.md),
            CosmicforgeLogisticsOtpInput(length: code!.length, value: code!),
            if (expiresAt != null) ...[
              const SizedBox(height: CosmicforgeLogisticsSpacing.sm),
              _ExpiryLabel(expiresAt: expiresAt!),
            ],
          ],
          if (errorMessage != null) ...[
            const SizedBox(height: CosmicforgeLogisticsSpacing.sm),
            CosmicforgeLogisticsFieldError(message: errorMessage!),
          ],
          if (onResend != null) ...[
            const SizedBox(height: CosmicforgeLogisticsSpacing.sm),
            TextButton(
              onPressed: canResend ? onResend : null,
              child: Text(
                canResend
                    ? 'Resend code'
                    : 'Resend in ${resendCooldownSeconds}s',
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class _ExpiryLabel extends StatelessWidget {
  const _ExpiryLabel({required this.expiresAt});

  final DateTime expiresAt;

  @override
  Widget build(BuildContext context) {
    final remaining = expiresAt.difference(DateTime.now());
    final expired = remaining.isNegative;
    return Text(
      expired
          ? 'This code has expired'
          : 'Expires in ${_formatDuration(remaining)}',
      style: TextStyle(
        fontSize: 12,
        fontWeight: FontWeight.w600,
        color: expired
            ? CosmicforgeLogisticsColors.danger
            : CosmicforgeLogisticsColors.textMuted,
      ),
    );
  }

  static String _formatDuration(Duration d) {
    final minutes = d.inMinutes;
    final seconds = d.inSeconds % 60;
    if (minutes > 0) {
      return '${minutes}m ${seconds}s';
    }
    return '${seconds}s';
  }
}
