import 'package:flutter/material.dart';

import '../../core/theme/theme_extensions.dart';

class NumericKeypad extends StatelessWidget {
  const NumericKeypad({
    super.key,
    required this.onKeyTap,
    this.textColor,
  });

  final ValueChanged<String> onKeyTap;

  /// Optional override for the digit color. Falls back to the theme's
  /// `text` color so the keypad adapts to light/dark automatically.
  final Color? textColor;

  static const _rows = [
    ['1', '2', '3'],
    ['4', '5', '6'],
    ['7', '8', '9'],
    ['', '0', 'backspace'],
  ];

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final digitColor = textColor ?? colors.text;

    return Column(
      children: _rows.map((row) {
        return Padding(
          padding: const EdgeInsets.symmetric(vertical: 10),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.spaceAround,
            children: row.map((key) {
              if (key.isEmpty) {
                return const SizedBox(width: 64, height: 44);
              }
              return GestureDetector(
                onTap: () => onKeyTap(key),
                behavior: HitTestBehavior.opaque,
                child: SizedBox(
                  width: 64,
                  height: 44,
                  child: Center(
                    child: key == 'backspace'
                        ? Container(
                            padding: const EdgeInsets.symmetric(
                              horizontal: 10,
                              vertical: 6,
                            ),
                            decoration: BoxDecoration(
                              color: colors.navy10,
                              borderRadius: BorderRadius.circular(8),
                            ),
                            child: Icon(
                              Icons.close,
                              size: 16,
                              color: digitColor,
                            ),
                          )
                        : Text(
                            key,
                            style: TextStyle(
                              fontSize: 24,
                              fontWeight: FontWeight.w600,
                              color: digitColor,
                            ),
                          ),
                  ),
                ),
              );
            }).toList(),
          ),
        );
      }).toList(),
    );
  }
}