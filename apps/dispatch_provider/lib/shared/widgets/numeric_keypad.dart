import 'package:flutter/material.dart';

class NumericKeypad extends StatelessWidget {
  const NumericKeypad({
    super.key,
    required this.onKeyTap,
    this.textColor = const Color(0xFF1A1A1A),
  });

  final ValueChanged<String> onKeyTap;
  final Color textColor;

  static const _rows = [
    ['1', '2', '3'],
    ['4', '5', '6'],
    ['7', '8', '9'],
    ['', '0', 'backspace'],
  ];

  @override
  Widget build(BuildContext context) {
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
                              color: const Color(0xFFE8F5E9),
                              borderRadius: BorderRadius.circular(8),
                            ),
                            child: const Icon(
                              Icons.close,
                              size: 16,
                              color: Color(0xFF4CAF50),
                            ),
                          )
                        : Text(
                            key,
                            style: TextStyle(
                              fontSize: 24,
                              fontWeight: FontWeight.w600,
                              color: textColor,
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
