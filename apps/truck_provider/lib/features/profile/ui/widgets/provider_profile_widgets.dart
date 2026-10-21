import 'package:flutter/material.dart';

import '../../../../core/widgets/karryam_ui.dart';
import '../../../home/ui/widgets/provider_app_colors.dart';

// Status colour used across the profile (verification badges, ratings).
const kProviderAmber = kAmber;

// ─── Sub-screen header (back circle + title + subtitle) ──────────────────────

class ProviderProfileHeader extends StatelessWidget {
  const ProviderProfileHeader({
    super.key,
    required this.title,
    this.subtitle,
    this.onBack,
  });

  final String title;
  final String? subtitle;
  final VoidCallback? onBack;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        KCircleButton(
          icon: Icons.arrow_back_rounded,
          tooltip: 'Back',
          onTap: onBack ?? () => Navigator.of(context).maybePop(),
        ),
        const SizedBox(width: 14),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                title,
                style: TextStyle(color: kProviderText, fontSize: 18, fontWeight: FontWeight.w700),
              ),
              if (subtitle != null) ...[
                const SizedBox(height: 2),
                Text(subtitle!, style: TextStyle(color: kProviderMuted, fontSize: 12.5)),
              ],
            ],
          ),
        ),
      ],
    );
  }
}

// ─── Field label ──────────────────────────────────────────────────────────────

class ProviderFieldLabel extends StatelessWidget {
  const ProviderFieldLabel(this.text, {super.key});
  final String text;

  @override
  Widget build(BuildContext context) => KFieldLabel(text);
}

OutlineInputBorder _outline(Color color, [double width = 1]) => OutlineInputBorder(
      borderRadius: BorderRadius.circular(12),
      borderSide: BorderSide(color: color, width: width),
    );

/// Standard text field decoration. Kept under its old name so every form
/// shares one field style.
InputDecoration providerGrayField(String hint) => providerWhiteField(hint);

/// White field with a hairline border and a teal focus ring.
InputDecoration providerWhiteField(String hint, {Widget? suffixIcon}) => InputDecoration(
      hintText: hint,
      hintStyle: TextStyle(color: kProviderMuted, fontSize: 14),
      filled: true,
      fillColor: kCard,
      suffixIcon: suffixIcon,
      border: _outline(kProviderBorder),
      enabledBorder: _outline(kProviderBorder),
      disabledBorder: _outline(kProviderBorder),
      focusedBorder: _outline(kTeal, 1.6),
      errorBorder: _outline(kProviderRejectText),
      focusedErrorBorder: _outline(kProviderRejectText, 1.6),
      contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
    );

// ─── Primary button ───────────────────────────────────────────────────────────

class ProviderPrimaryButton extends StatelessWidget {
  const ProviderPrimaryButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.isLoading = false,
  });

  final String label;
  final VoidCallback? onPressed;
  final bool isLoading;

  @override
  Widget build(BuildContext context) =>
      KPrimaryButton(label: label, onPressed: onPressed, loading: isLoading);
}

// ─── Inline error ─────────────────────────────────────────────────────────────

class ProviderErrorText extends StatelessWidget {
  const ProviderErrorText(this.message, {super.key});
  final String? message;

  @override
  Widget build(BuildContext context) {
    if (message == null) return const SizedBox.shrink();
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: KNotice(message!, tone: KTone.danger),
    );
  }
}

// ─── Confirmation dialog ──────────────────────────────────────────────────────

Future<bool?> showProviderConfirmDialog(
  BuildContext context, {
  required IconData icon,
  required String title,
  required String message,
  required String confirmLabel,
  String? cancelLabel,
  Color? confirmColor,
}) {
  confirmColor ??= kPrimary;
  final destructive = confirmColor == kProviderRejectText;
  return showDialog<bool>(
    context: context,
    builder: (ctx) => Dialog(
      insetPadding: const EdgeInsets.symmetric(horizontal: 24),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(22, 26, 22, 20),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            KIconTile(
              icon,
              size: 64,
              color: destructive ? kProviderRejectText : kTealText,
              background: destructive ? kProviderRejectBg : kTealTint,
            ),
            const SizedBox(height: 18),
            Text(
              title,
              textAlign: TextAlign.center,
              style: TextStyle(color: kProviderText, fontSize: 19, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 8),
            Text(
              message,
              textAlign: TextAlign.center,
              style: TextStyle(color: kProviderMuted, fontSize: 13.5, height: 1.5),
            ),
            const SizedBox(height: 22),
            KPrimaryButton(
              label: confirmLabel,
              color: confirmColor,
              onPressed: () => Navigator.of(ctx).pop(true),
            ),
            if (cancelLabel != null) ...[
              const SizedBox(height: 10),
              KSecondaryButton(label: cancelLabel, onPressed: () => Navigator.of(ctx).pop(false)),
            ],
          ],
        ),
      ),
    ),
  );
}
