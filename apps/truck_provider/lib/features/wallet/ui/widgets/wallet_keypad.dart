import 'package:flutter/material.dart';

import '../../../../core/widgets/karryam_ui.dart';
import '../../../home/ui/widgets/provider_app_colors.dart';

/// Custom numeric keypad used by the withdrawal amount form (Figma 2281) and the
/// PIN authorization screen (Figma 2285). 3×4 grid: 1-9, then [empty, 0, delete].
class WalletKeypad extends StatelessWidget {
  const WalletKeypad({
    super.key,
    required this.onDigit,
    required this.onDelete,
    this.deleteTint = const Color(0xFFE9ECF1),
  });

  final ValueChanged<String> onDigit;
  final VoidCallback onDelete;

  /// Background tint of the delete key (green-tinted on the PIN screen).
  final Color deleteTint;

  static Color get _digitColor => kProviderText;

  @override
  Widget build(BuildContext context) {
    Widget digit(String d) => _KeypadButton(
          onTap: () => onDigit(d),
          child: Text(
            d,
            style: TextStyle(
              color: _digitColor,
              fontSize: 26,
              fontWeight: FontWeight.w600,
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
                  child: Icon(Icons.backspace_outlined, size: 20, color: _digitColor),
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

/// Primary action used across the withdrawal flow.
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
  Widget build(BuildContext context) =>
      KPrimaryButton(label: label, onPressed: onPressed, loading: loading);
}
