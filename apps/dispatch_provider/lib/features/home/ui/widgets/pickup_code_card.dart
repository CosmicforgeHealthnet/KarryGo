import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:qr_flutter/qr_flutter.dart';

import '../../../../core/theme/theme_extensions.dart';

/// Theme-aware pickup handover code card.
///
/// Shows a QR code (scannable by the customer's camera) plus a 6-digit code
/// in rounded boxes (for manual entry). Reads from `context.palette` so it
/// tints correctly per theme.
class PickupCodeCard extends StatelessWidget {
  const PickupCodeCard({
    super.key,
    required this.title,
    required this.subtitle,
    required this.code,
    this.expiresAt,
    this.onResend,
    this.canResend = true,
    this.isResending = false,
  });

  final String title;
  final String subtitle;
  final String? code;
  final DateTime? expiresAt;
  final VoidCallback? onResend;
  final bool canResend;
  final bool isResending;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final digits = (code ?? '').split('');
    final expiresIn = _expiresInText(expiresAt);
    final hasCode = code != null && code!.isNotEmpty;

    return Container(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 14),
      decoration: BoxDecoration(
        color: colors.surfaceElevated, // ← was colors.navy10
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: colors.divider),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // ── Label + copy ────────────────────────────────────────
          Row(
            children: [
              Icon(Icons.qr_code_rounded, size: 15, color: colors.brandTeal600),
              const SizedBox(width: 6),
              Expanded(
                child: Text(
                  title,
                  style: TextStyle(
                    fontSize: 12.5,
                    fontWeight: FontWeight.w700,
                    color: colors.text,
                    letterSpacing: 0.2,
                  ),
                ),
              ),
              if (hasCode)
                GestureDetector(
                  onTap: () {
                    Clipboard.setData(ClipboardData(text: code!));
                    ScaffoldMessenger.of(context).showSnackBar(
                      const SnackBar(
                        content: Text('Code copied'),
                        duration: Duration(seconds: 2),
                      ),
                    );
                  },
                  behavior: HitTestBehavior.opaque,
                  child: Container(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 8,
                      vertical: 5,
                    ),
                    decoration: BoxDecoration(
                      color: colors.surface,
                      borderRadius: BorderRadius.circular(6),
                      border: Border.all(color: colors.divider),
                    ),
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Icon(
                          Icons.copy_rounded,
                          size: 12,
                          color: colors.textMuted,
                        ),
                        const SizedBox(width: 4),
                        Text(
                          'Copy',
                          style: TextStyle(
                            fontSize: 11,
                            fontWeight: FontWeight.w700,
                            color: colors.text,
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
            ],
          ),

          const SizedBox(height: 4),

          Text(
            subtitle,
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w400,
              color: colors.textMuted,
              height: 1.4,
            ),
          ),

          const SizedBox(height: 16),

          // ── QR code ─────────────────────────────────────────────
          Center(
            child: Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: Colors.white,
                borderRadius: BorderRadius.circular(14),
                border: Border.all(color: colors.divider),
              ),
              child: hasCode
                  ? QrImageView(
                      data: code!,
                      version: QrVersions.auto,
                      size: 160,
                      backgroundColor: Colors.white,
                      eyeStyle: const QrEyeStyle(
                        eyeShape: QrEyeShape.square,
                        color: Color(0xFF081F44),
                      ),
                      dataModuleStyle: const QrDataModuleStyle(
                        dataModuleShape: QrDataModuleShape.square,
                        color: Color(0xFF081F44),
                      ),
                    )
                  : SizedBox(
                      width: 160,
                      height: 160,
                      child: Center(
                        child: Text(
                          'Generating...',
                          style: TextStyle(
                            fontSize: 13,
                            fontWeight: FontWeight.w600,
                            color: colors.textMuted,
                          ),
                        ),
                      ),
                    ),
            ),
          ),

          const SizedBox(height: 16),

          // ── Digit boxes ─────────────────────────────────────────
          if (digits.isEmpty)
            _PlaceholderRow(colors: colors)
          else
            Row(
              children: [
                for (int i = 0; i < digits.length; i++) ...[
                  Expanded(child: _DigitBox(digit: digits[i])),
                  if (i != digits.length - 1) const SizedBox(width: 6),
                ],
              ],
            ),

          const SizedBox(height: 12),

          // ── Expiry + resend ─────────────────────────────────────
          Row(
            children: [
              if (expiresIn != null) ...[
                Icon(
                  Icons.access_time_rounded,
                  size: 13,
                  color: colors.textMuted,
                ),
                const SizedBox(width: 4),
                Text(
                  expiresIn,
                  style: TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w500,
                    color: colors.textMuted,
                  ),
                ),
              ],
              const Spacer(),
              if (onResend != null)
                GestureDetector(
                  onTap: canResend && !isResending ? onResend : null,
                  behavior: HitTestBehavior.opaque,
                  child: Padding(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 6,
                      vertical: 4,
                    ),
                    child: isResending
                        ? SizedBox(
                            width: 14,
                            height: 14,
                            child: CircularProgressIndicator(
                              strokeWidth: 2,
                              color: colors.brandTeal600,
                            ),
                          )
                        : Text(
                            'Resend code',
                            style: TextStyle(
                              fontSize: 12.5,
                              fontWeight: FontWeight.w700,
                              color: canResend
                                  ? colors.brandTeal600
                                  : colors.textDisabled,
                            ),
                          ),
                  ),
                ),
            ],
          ),
        ],
      ),
    );
  }

  String? _expiresInText(DateTime? expiresAt) {
    if (expiresAt == null) return null;
    final remaining = expiresAt.difference(DateTime.now());
    if (remaining.isNegative) return 'Expired';
    final m = remaining.inMinutes;
    final s = remaining.inSeconds % 60;
    return 'Expires in $m:${s.toString().padLeft(2, '0')}';
  }
}

// ── Digit box ────────────────────────────────────────────────────────────

class _DigitBox extends StatelessWidget {
  const _DigitBox({required this.digit});
  final String digit;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Container(
      height: 52,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: colors.divider, width: 1.2),
      ),
      child: Text(
        digit,
        style: TextStyle(
          fontSize: 22,
          fontWeight: FontWeight.w700,
          color: colors.text,
          height: 1.0,
          fontFeatures: const [FontFeature.tabularFigures()],
        ),
      ),
    );
  }
}

class _PlaceholderRow extends StatelessWidget {
  const _PlaceholderRow({required this.colors});
  final dynamic colors;

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 52,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: colors.divider, width: 1.2),
      ),
      child: Text(
        '— — — — — —',
        style: TextStyle(
          fontSize: 18,
          fontWeight: FontWeight.w700,
          color: colors.textDisabled,
          letterSpacing: 2,
        ),
      ),
    );
  }
}
