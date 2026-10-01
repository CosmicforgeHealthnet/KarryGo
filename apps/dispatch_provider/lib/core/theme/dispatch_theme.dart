import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'dispatch_palette.dart';

class DispatchTheme {
  const DispatchTheme._();

  static ThemeData get light => _build(
        colors: DispatchPalette.light,
        brightness: Brightness.light,
        statusBarStyle: SystemUiOverlayStyle.dark,
      );

  static ThemeData get dark => _build(
        colors: DispatchPalette.dark,
        brightness: Brightness.dark,
        statusBarStyle: SystemUiOverlayStyle.light,
      );

  static ThemeData _build({
    required DispatchPalette colors,
    required Brightness brightness,
    required SystemUiOverlayStyle statusBarStyle,
  }) {
    final scheme = ColorScheme(
      brightness: brightness,
      primary: colors.brandNavy900,
      onPrimary: colors.brandWhite,
      primaryContainer: colors.navy10,
      onPrimaryContainer: colors.text,
      secondary: colors.brandTeal500,
      onSecondary: Colors.white,
      secondaryContainer: colors.teal10,
      onSecondaryContainer: colors.text,
      tertiary: colors.brandTeal600,
      onTertiary: Colors.white,
      error: colors.danger,
      onError: Colors.white,
      errorContainer: colors.dangerSoft,
      onErrorContainer: colors.danger,
      surface: colors.surface,
      onSurface: colors.text,
      surfaceContainerHighest: colors.surfaceElevated,
      onSurfaceVariant: colors.textMuted,
      outline: colors.border,
      outlineVariant: colors.divider,
      shadow: Colors.black,
      scrim: colors.scrim,
      inverseSurface: colors.text,
      onInverseSurface: colors.surface,
    );

    final base = ThemeData(
      useMaterial3: true,
      brightness: brightness,
      colorScheme: scheme,
      scaffoldBackgroundColor: colors.background,
      canvasColor: colors.surface,
      dividerColor: colors.divider,
      splashColor: colors.brandTeal500.withValues(alpha: 0.08),
      highlightColor: colors.brandTeal500.withValues(alpha: 0.04),
      extensions: [colors],
    );

    return base.copyWith(
      appBarTheme: AppBarTheme(
        backgroundColor: colors.surface,
        foregroundColor: colors.text,
        elevation: 0,
        scrolledUnderElevation: 0,
        surfaceTintColor: Colors.transparent,
        centerTitle: false,
        systemOverlayStyle: statusBarStyle,
        titleTextStyle: TextStyle(
          fontSize: 17,
          fontWeight: FontWeight.w700,
          color: colors.text,
          letterSpacing: -0.3,
        ),
        iconTheme: IconThemeData(color: colors.text, size: 22),
      ),
      cardTheme: CardThemeData(
        color: colors.card,
        elevation: 0,
        surfaceTintColor: Colors.transparent,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(16),
          side: BorderSide(color: colors.divider, width: 1),
        ),
      ),
      dialogTheme: DialogThemeData(
        backgroundColor: colors.surface,
        surfaceTintColor: Colors.transparent,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(20),
        ),
        titleTextStyle: TextStyle(
          fontSize: 18,
          fontWeight: FontWeight.w800,
          color: colors.text,
        ),
        contentTextStyle: TextStyle(fontSize: 14, color: colors.textMuted),
      ),
      bottomSheetTheme: BottomSheetThemeData(
        backgroundColor: colors.surface,
        surfaceTintColor: Colors.transparent,
        modalBackgroundColor: colors.surface,
        shape: const RoundedRectangleBorder(
          borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
        ),
      ),
      filledButtonTheme: FilledButtonThemeData(
  style: FilledButton.styleFrom(
    backgroundColor: colors.btnPrimary,
    foregroundColor: colors.btnPrimaryFg,
    disabledBackgroundColor: colors.btnPrimaryDisabled,
    disabledForegroundColor: colors.btnPrimaryDisabledFg,
    shape: RoundedRectangleBorder(
      borderRadius: BorderRadius.circular(999),
    ),
    textStyle: const TextStyle(fontSize: 15, fontWeight: FontWeight.w700),
  ),
),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(
          foregroundColor: colors.text,
          side: BorderSide(color: colors.border, width: 1.2),
          textStyle: const TextStyle(
            fontSize: 15,
            fontWeight: FontWeight.w600,
          ),
        ),
      ),
      textButtonTheme: TextButtonThemeData(
        style: TextButton.styleFrom(
          foregroundColor: colors.brandTeal500,
          textStyle: const TextStyle(
            fontSize: 14,
            fontWeight: FontWeight.w700,
          ),
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: colors.inputFill,
        contentPadding:
            const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
        hintStyle: TextStyle(color: colors.textDisabled, fontSize: 14),
        labelStyle: TextStyle(color: colors.textMuted, fontSize: 14),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: colors.border, width: 1.2),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: colors.border, width: 1.2),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: colors.brandTeal500, width: 1.6),
        ),
        errorBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: colors.danger, width: 1.4),
        ),
        focusedErrorBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: colors.danger, width: 1.6),
        ),
      ),
      snackBarTheme: SnackBarThemeData(
        behavior: SnackBarBehavior.floating,
        backgroundColor: colors.text,
        contentTextStyle: TextStyle(color: colors.surface),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(12),
        ),
      ),
      dividerTheme: DividerThemeData(
        color: colors.divider,
        thickness: 1,
        space: 1,
      ),
      textTheme: base.textTheme.apply(
        bodyColor: colors.text,
        displayColor: colors.text,
      ),
      primaryTextTheme: base.primaryTextTheme.apply(
        bodyColor: colors.text,
        displayColor: colors.text,
      ),
    );
  }
}