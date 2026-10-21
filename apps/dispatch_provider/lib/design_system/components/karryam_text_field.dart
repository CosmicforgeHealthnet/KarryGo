import 'package:flutter/material.dart';

import '../karryam_tokens.dart';

class KarryamTextField extends StatelessWidget {
  const KarryamTextField({
    super.key,
    required this.label,
    this.hint,
    this.controller,
    this.keyboardType,
    this.errorText,
    this.leading,
    this.trailing,
    this.enabled = true,
    this.obscureText = false,
  });

  final String label;
  final String? hint;
  final TextEditingController? controller;
  final TextInputType? keyboardType;
  final String? errorText;
  final Widget? leading;
  final Widget? trailing;
  final bool enabled;
  final bool obscureText;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: Theme.of(context).textTheme.labelLarge?.copyWith(
                color: KarryamColors.midnightNavy,
                fontWeight: FontWeight.w700,
              ),
        ),
        const SizedBox(height: KarryamSpacing.xs),
        TextField(
          controller: controller,
          keyboardType: keyboardType,
          obscureText: obscureText,
          enabled: enabled,
          decoration: InputDecoration(
            hintText: hint,
            errorText: errorText,
            prefixIcon: leading,
            suffixIcon: trailing,
            filled: true,
            fillColor: enabled ? KarryamColors.pureWhite : KarryamColors.navy05,
            contentPadding: const EdgeInsets.symmetric(
              horizontal: KarryamSpacing.md,
              vertical: KarryamSpacing.md,
            ),
            enabledBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(KarryamRadius.md),
              borderSide: const BorderSide(color: KarryamColors.border),
            ),
            focusedBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(KarryamRadius.md),
              borderSide: const BorderSide(
                color: KarryamColors.auroraTeal,
                width: 1.5,
              ),
            ),
            errorBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(KarryamRadius.md),
              borderSide: const BorderSide(color: KarryamColors.danger),
            ),
            focusedErrorBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(KarryamRadius.md),
              borderSide: const BorderSide(
                color: KarryamColors.danger,
                width: 1.5,
              ),
            ),
          ),
        ),
      ],
    );
  }
}
