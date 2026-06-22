import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';

/// Custom on-screen numeric keypad used by the withdrawal amount screen
/// (mockup #8). Emits digit presses and backspace; the parent owns the value.
class WalletAmountKeypad extends StatelessWidget {
  const WalletAmountKeypad({
    super.key,
    required this.onDigit,
    required this.onBackspace,
    this.expand = false,
  });

  final ValueChanged<String> onDigit;
  final VoidCallback onBackspace;

  /// When true the grid fills the available height (taller keys), matching the
  /// keypad-first withdrawal screen in the mockup.
  final bool expand;

  static const _keys = [
    '1', '2', '3',
    '4', '5', '6',
    '7', '8', '9',
    '.', '0', '⌫',
  ];

  @override
  Widget build(BuildContext context) {
    return GridView.count(
      crossAxisCount: 3,
      shrinkWrap: !expand,
      physics: const NeverScrollableScrollPhysics(),
      childAspectRatio: expand ? 1.5 : 1.9,
      mainAxisSpacing: 8,
      crossAxisSpacing: 8,
      children: _keys.map((key) {
        final isBackspace = key == '⌫';
        return _KeypadButton(
          label: key,
          isBackspace: isBackspace,
          onTap: () {
            if (isBackspace) {
              onBackspace();
            } else {
              onDigit(key);
            }
          },
        );
      }).toList(),
    );
  }
}

class _KeypadButton extends StatelessWidget {
  const _KeypadButton({
    required this.label,
    required this.isBackspace,
    required this.onTap,
  });

  final String label;
  final bool isBackspace;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.transparent,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(16),
        child: Center(
          child: isBackspace
              ? const Icon(
                  Icons.backspace_outlined,
                  color: CustomerFigmaColors.text,
                  size: 24,
                )
              : Text(
                  label,
                  style: const TextStyle(
                    color: CustomerFigmaColors.text,
                    fontSize: 26,
                    fontWeight: FontWeight.w700,
                  ),
                ),
        ),
      ),
    );
  }
}

/// Applies a keypad digit/backspace edit to a raw amount string, enforcing a
/// single decimal point and at most two decimal places. Returns the new string.
String applyAmountKey(String current, String key) {
  if (key == '.') {
    if (current.contains('.')) return current;
    if (current.isEmpty) return '0.';
    return '$current.';
  }
  // Reject a third decimal digit.
  final dotIndex = current.indexOf('.');
  if (dotIndex != -1 && current.length - dotIndex > 2) return current;
  // Avoid leading zeros like "00".
  if (current == '0') return key;
  return '$current$key';
}

String backspaceAmount(String current) {
  if (current.isEmpty) return current;
  return current.substring(0, current.length - 1);
}
