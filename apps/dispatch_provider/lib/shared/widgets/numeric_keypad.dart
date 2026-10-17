import 'package:flutter/material.dart';
import '../../core/theme/theme_extensions.dart';

class NumericKeypad extends StatelessWidget {
  const NumericKeypad({
    super.key,
    required this.onKeyTap,
    this.textColor,
    this.enabled = true,
  });
  final ValueChanged<String> onKeyTap;
  final Color? textColor;
  final bool enabled;
  @override
  Widget build(BuildContext context) => Column(
    mainAxisSize: MainAxisSize.min,
    children: [
      for (final row in const [
        ['1', '2', '3'],
        ['4', '5', '6'],
        ['7', '8', '9'],
        ['', '0', 'backspace'],
      ])
        Padding(
          padding: const EdgeInsets.symmetric(vertical: 5),
          child: Row(
            children: [
              for (final key in row)
                Expanded(
                  child: key.isEmpty
                      ? const SizedBox(height: 50)
                      : Semantics(
                          label: key == 'backspace' ? 'Delete last digit' : key,
                          button: true,
                          child: Material(
                            color: Colors.transparent,
                            child: InkWell(
                              borderRadius: BorderRadius.circular(16),
                              onTap: enabled ? () => onKeyTap(key) : null,
                              child: SizedBox(
                                height: 50,
                                child: Center(
                                  child: key == 'backspace'
                                      ? Icon(
                                          Icons.backspace_outlined,
                                          size: 22,
                                          color:
                                              textColor ??
                                              context.palette.textMuted,
                                        )
                                      : Text(
                                          key,
                                          style: Theme.of(context)
                                              .textTheme
                                              .headlineSmall
                                              ?.copyWith(
                                                color:
                                                    textColor ??
                                                    context.palette.text,
                                              ),
                                        ),
                                ),
                              ),
                            ),
                          ),
                        ),
                ),
            ],
          ),
        ),
    ],
  );
}
