import 'package:flutter/material.dart';

import 'dispatch_palette.dart';

/// Sugar for reading the app's [DispatchPalette] off any [BuildContext].
///
///   final colors = context.palette;
///   Container(color: colors.surface);
///   Text('Hi', style: TextStyle(color: colors.text));
extension DispatchThemeX on BuildContext {
  /// The active palette (light or dark).
  DispatchPalette get palette =>
      Theme.of(this).extension<DispatchPalette>() ?? DispatchPalette.light;

  /// True when the active theme is dark.
  bool get isDark => Theme.of(this).brightness == Brightness.dark;

  ColorScheme get scheme => Theme.of(this).colorScheme;
  TextTheme get textStyles => Theme.of(this).textTheme;
}