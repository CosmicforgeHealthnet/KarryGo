import 'package:flutter/material.dart';

import '../../../home/ui/widgets/provider_app_colors.dart';

/// Custom numeric keypad used by the withdrawal amount form (Figma 2281) and the
/// PIN authorization screen (Figma 2285). 3×4 grid: 1-9, then [empty, 0, delete].
class WalletKeypad extends StatelessWidget {
  const WalletKeypad({
    super.key,
    required this.onDigit,
    required this.onDelete,
    this.deleteTint = const Color(0xFFEDEFF3),
  });

  final ValueChanged<String> onDigit;
  final VoidCallback onDelete;

  /// Background tint of the delete key (green-tinted on the PIN screen).
  final Color deleteTint;

  static const _digitColor = Color(0xFF374357);

  @override
  Widget build(BuildContext context) {
    Widget digit(String d) => _KeypadButton(
          onTap: () => onDigit(d),
          child: Text(
            d,
            style: const TextStyle(
              color: _digitColor,
              fontSize: 26,
              fontWeight: FontWeight.w700,
            ),
          ),
        );

    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        _row([digit('1'), digit('2'), digit('3')]),
        _row([digit('4'), digit('5'), digit('6')]),
        _row([digit('7'), digit('8'), digit('9')]),
        _row([
          const Expanded(child: SizedBox.shrink()),
          digit('0'),
          Expanded(
            child: Center(
              child: _KeypadButton(
                onTap: onDelete,
                child: Container(
                  width: 52,
                  height: 38,
                  decoration: BoxDecoration(
                    color: deleteTint,
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: const Icon(Icons.backspace_outlined, size: 20, color: _digitColor),
                ),
              ),
            ),
          ),
        ]),
      ],
    );
  }

  Widget _row(List<Widget> children) => Padding(
        padding: const EdgeInsets.symmetric(vertical: 11),
        child: Row(
          children: [
            for (final child in children)
              child is Expanded ? child : Expanded(child: Center(child: child)),
          ],
        ),
      );
}

class _KeypadButton extends StatelessWidget {
  const _KeypadButton({required this.onTap, required this.child});
  final VoidCallback onTap;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return InkResponse(
      onTap: onTap,
      radius: 36,
      child: Padding(padding: const EdgeInsets.all(8), child: child),
    );
  }
}

/// Shared "Withdraw Now" / primary pill button used across the withdrawal flow.
class WalletPrimaryButton extends StatelessWidget {
  const WalletPrimaryButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.loading = false,
  });

  final String label;
  final VoidCallback? onPressed;
  final bool loading;

  @override
  Widget build(BuildContext context) {
    final enabled = onPressed != null && !loading;
    return SizedBox(
      width: double.infinity,
      height: 56,
      child: FilledButton(
        onPressed: enabled ? onPressed : null,
        style: FilledButton.styleFrom(
          backgroundColor: kProviderGreen,
          disabledBackgroundColor: kProviderGreenSoft,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
        ),
        child: loading
            ? const SizedBox.square(
                dimension: 22,
                child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white),
              )
            : Text(
                label,
                style: const TextStyle(
                  color: Colors.white,
                  fontSize: 16,
                  fontWeight: FontWeight.w700,
                ),
              ),
      ),
    );
  }
}
