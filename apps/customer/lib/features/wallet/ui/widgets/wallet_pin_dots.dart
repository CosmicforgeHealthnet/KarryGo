import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';

/// Row of PIN entry dots (mockup #10). Filled dots reflect [length] entered of
/// [count] total. Purely visual — PIN authorization is UI-only for now.
class WalletPinDots extends StatelessWidget {
  const WalletPinDots({
    super.key,
    required this.length,
    this.count = 4,
  });

  final int length;
  final int count;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: List.generate(count, (i) {
        final filled = i < length;
        return Container(
          margin: const EdgeInsets.symmetric(horizontal: 10),
          width: 18,
          height: 18,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: filled ? CustomerFigmaColors.primary : Colors.transparent,
            border: Border.all(
              color: filled
                  ? CustomerFigmaColors.primary
                  : CustomerFigmaColors.border,
              width: 1.5,
            ),
          ),
        );
      }),
    );
  }
}
