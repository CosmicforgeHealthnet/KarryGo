import 'package:flutter/material.dart';

import '../../../../core/theme/theme_extensions.dart';

class LocateButton extends StatelessWidget {
  const LocateButton({super.key, this.onPressed});
  final VoidCallback? onPressed;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return GestureDetector(
      onTap: onPressed,
      child: Container(
        width: 44,
        height: 44,
        decoration: BoxDecoration(
          color: colors.surface,
          shape: BoxShape.circle,
          boxShadow: colors.cardShadow,
        ),
        child: Icon(Icons.my_location, size: 20, color: colors.text),
      ),
    );
  }
}