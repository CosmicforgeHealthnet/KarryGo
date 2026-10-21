import 'package:flutter/material.dart';

import '../karryam_tokens.dart';

enum KarryamButtonVariant { primary, accent, secondary, danger }

class KarryamButton extends StatelessWidget {
  const KarryamButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.variant = KarryamButtonVariant.primary,
    this.leading,
    this.isLoading = false,
    this.expand = true,
  });

  final String label;
  final VoidCallback? onPressed;
  final KarryamButtonVariant variant;
  final Widget? leading;
  final bool isLoading;
  final bool expand;

  @override
  Widget build(BuildContext context) {
    final style = switch (variant) {
      KarryamButtonVariant.primary => FilledButton.styleFrom(
          backgroundColor: KarryamColors.midnightNavy,
          foregroundColor: KarryamColors.pureWhite,
        ),
      KarryamButtonVariant.accent => FilledButton.styleFrom(
          backgroundColor: KarryamColors.auroraTeal,
          foregroundColor: KarryamColors.midnightNavy,
        ),
      KarryamButtonVariant.secondary => OutlinedButton.styleFrom(
          foregroundColor: KarryamColors.midnightNavy,
          side: const BorderSide(color: KarryamColors.navy20),
        ),
      KarryamButtonVariant.danger => FilledButton.styleFrom(
          backgroundColor: KarryamColors.danger,
          foregroundColor: KarryamColors.pureWhite,
        ),
    };

    final child = AnimatedSwitcher(
      duration: KarryamMotion.fast,
      child: isLoading
          ? const SizedBox.square(
              key: ValueKey('loading'),
              dimension: 18,
              child: CircularProgressIndicator(strokeWidth: 2),
            )
          : Row(
              key: const ValueKey('label'),
              mainAxisSize: MainAxisSize.min,
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                if (leading != null) ...[
                  leading!,
                  const SizedBox(width: KarryamSpacing.xs),
                ],
                Flexible(child: Text(label)),
              ],
            ),
    );

    final button = variant == KarryamButtonVariant.secondary
        ? OutlinedButton(
            onPressed: isLoading ? null : onPressed,
            style: style.copyWith(
              minimumSize: const WidgetStatePropertyAll(Size(48, 52)),
              shape: WidgetStatePropertyAll(
                RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(KarryamRadius.md),
                ),
              ),
            ),
            child: child,
          )
        : FilledButton(
            onPressed: isLoading ? null : onPressed,
            style: style.copyWith(
              minimumSize: const WidgetStatePropertyAll(Size(48, 52)),
              shape: WidgetStatePropertyAll(
                RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(KarryamRadius.md),
                ),
              ),
            ),
            child: child,
          );

    return expand ? SizedBox(width: double.infinity, child: button) : button;
  }
}
