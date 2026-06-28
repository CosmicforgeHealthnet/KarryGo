import 'package:flutter/material.dart';

import '../../../home/ui/widgets/provider_app_colors.dart';

// Status colours used across the profile (verification badges, etc.)
const kProviderAmber = Color(0xFFEAA300);

// ─── Sub-screen header (circular back button + title + subtitle) ───────────────

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
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        GestureDetector(
          onTap: onBack ?? () => Navigator.of(context).maybePop(),
          child: Container(
            width: 44,
            height: 44,
            decoration: const BoxDecoration(
              color: Colors.white,
              shape: BoxShape.circle,
              boxShadow: [BoxShadow(color: Color(0x14000000), blurRadius: 12, offset: Offset(0, 4))],
            ),
            child: const Icon(Icons.arrow_back_rounded, color: kProviderText, size: 20),
          ),
        ),
        const SizedBox(width: 14),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                title,
                style: const TextStyle(color: kProviderText, fontSize: 20, fontWeight: FontWeight.w800),
              ),
              if (subtitle != null) ...[
                const SizedBox(height: 2),
                Text(
                  subtitle!,
                  style: const TextStyle(color: kProviderMuted, fontSize: 12),
                ),
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
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Text(
        text,
        style: const TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w700),
      ),
    );
  }
}

// Gray-filled input (Profile Info style — Figma 2111/2112).
InputDecoration providerGrayField(String hint) => InputDecoration(
      hintText: hint,
      hintStyle: const TextStyle(color: kProviderMuted, fontSize: 14),
      filled: true,
      fillColor: kProviderSurface,
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: BorderSide.none,
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: BorderSide.none,
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: kProviderGreen, width: 1.5),
      ),
      contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
    );

// White card input with a soft shadow (Verification / Truck edit style — 2133/2164).
InputDecoration providerWhiteField(String hint, {Widget? suffixIcon}) => InputDecoration(
      hintText: hint,
      hintStyle: const TextStyle(color: kProviderMuted, fontSize: 14),
      filled: true,
      fillColor: Colors.white,
      suffixIcon: suffixIcon,
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: kProviderBorder),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: kProviderBorder),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: kProviderGreen, width: 1.5),
      ),
      contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
    );

// ─── Primary pill button (green; disabled = pale green) ────────────────────────

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
  Widget build(BuildContext context) {
    final enabled = onPressed != null && !isLoading;
    return SizedBox(
      height: 54,
      width: double.infinity,
      child: FilledButton(
        onPressed: enabled ? onPressed : null,
        style: FilledButton.styleFrom(
          backgroundColor: kProviderGreen,
          disabledBackgroundColor: kProviderGreenSoft,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
        ),
        child: isLoading
            ? const SizedBox.square(
                dimension: 22,
                child: CircularProgressIndicator(strokeWidth: 2.4, color: Colors.white),
              )
            : Text(
                label,
                style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w700, fontSize: 16),
              ),
      ),
    );
  }
}

// ─── Inline error banner ───────────────────────────────────────────────────────

class ProviderErrorText extends StatelessWidget {
  const ProviderErrorText(this.message, {super.key});
  final String? message;

  @override
  Widget build(BuildContext context) {
    if (message == null) return const SizedBox.shrink();
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Text(
        message!,
        textAlign: TextAlign.center,
        style: const TextStyle(color: kProviderRejectText, fontSize: 13),
      ),
    );
  }
}

// ─── Confirmation dialog (illustration + title + message + actions) ────────────
// Figma 2114 (Change Phone Number?), 2115 (success), 2163 (Edit Vehicle Details?).

Future<bool?> showProviderConfirmDialog(
  BuildContext context, {
  required IconData icon,
  required String title,
  required String message,
  required String confirmLabel,
  String? cancelLabel,
  Color confirmColor = kProviderGreen,
}) {
  return showDialog<bool>(
    context: context,
    barrierColor: const Color(0x66000000),
    builder: (ctx) => Dialog(
      backgroundColor: Colors.white,
      insetPadding: const EdgeInsets.symmetric(horizontal: 20),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(24)),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(24, 28, 24, 24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 96,
              height: 96,
              decoration: const BoxDecoration(color: kProviderGreenTint, shape: BoxShape.circle),
              child: Icon(icon, color: kProviderGreen, size: 44),
            ),
            const SizedBox(height: 24),
            Text(
              title,
              textAlign: TextAlign.center,
              style: const TextStyle(color: kProviderText, fontSize: 20, fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 10),
            Text(
              message,
              textAlign: TextAlign.center,
              style: const TextStyle(color: kProviderMuted, fontSize: 13, height: 1.5),
            ),
            const SizedBox(height: 24),
            SizedBox(
              height: 54,
              width: double.infinity,
              child: FilledButton(
                onPressed: () => Navigator.of(ctx).pop(true),
                style: FilledButton.styleFrom(
                  backgroundColor: confirmColor,
                  shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
                ),
                child: Text(
                  confirmLabel,
                  style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w700, fontSize: 16),
                ),
              ),
            ),
            if (cancelLabel != null) ...[
              const SizedBox(height: 12),
              SizedBox(
                height: 54,
                width: double.infinity,
                child: FilledButton(
                  onPressed: () => Navigator.of(ctx).pop(false),
                  style: FilledButton.styleFrom(
                    backgroundColor: kProviderGreenSoft,
                    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
                  ),
                  child: Text(
                    cancelLabel,
                    style: const TextStyle(color: kProviderGreen, fontWeight: FontWeight.w700, fontSize: 16),
                  ),
                ),
              ),
            ],
          ],
        ),
      ),
    ),
  );
}
