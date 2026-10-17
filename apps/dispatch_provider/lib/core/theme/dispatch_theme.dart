import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'dispatch_palette.dart';

class DispatchTheme {
  const DispatchTheme._();

  static SystemUiOverlayStyle systemUiStyle(Brightness brightness) {
    final dark = brightness == Brightness.dark;
    return SystemUiOverlayStyle(
      statusBarColor: Colors.transparent,
      statusBarIconBrightness: dark ? Brightness.light : Brightness.dark,
      statusBarBrightness: brightness,
      systemNavigationBarColor: dark
          ? DispatchPalette.dark.background
          : DispatchPalette.light.background,
      systemNavigationBarIconBrightness: dark
          ? Brightness.light
          : Brightness.dark,
      systemStatusBarContrastEnforced: false,
      systemNavigationBarContrastEnforced: false,
    );
  }

  static ThemeData get light => _build(
    colors: DispatchPalette.light,
    brightness: Brightness.light,
    statusBarStyle: systemUiStyle(Brightness.light),
  );

  static ThemeData get dark => _build(
    colors: DispatchPalette.dark,
    brightness: Brightness.dark,
    statusBarStyle: systemUiStyle(Brightness.dark),
  );

  static ThemeData _build({
    required DispatchPalette colors,
    required Brightness brightness,
    required SystemUiOverlayStyle statusBarStyle,
  }) {
    final scheme = ColorScheme(
      brightness: brightness,
      primary: colors.brandTeal600,
      onPrimary: colors.btnPrimaryFg,
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
      fontFamily: 'Inter',
      brightness: brightness,
      colorScheme: scheme,
      scaffoldBackgroundColor: colors.background,
      textTheme: TextTheme(
        headlineLarge: TextStyle(
          fontSize: 38,
          height: 1.12,
          letterSpacing: -1.8,
          fontWeight: FontWeight.w700,
          color: colors.text,
        ),
        headlineMedium: TextStyle(
          fontSize: 28,
          height: 1.15,
          letterSpacing: -.8,
          fontWeight: FontWeight.w700,
          color: colors.text,
        ),
        headlineSmall: TextStyle(
          fontSize: 24,
          height: 1.2,
          letterSpacing: -.5,
          fontWeight: FontWeight.w700,
          color: colors.text,
        ),
        titleLarge: TextStyle(
          fontSize: 20,
          height: 1.3,
          fontWeight: FontWeight.w700,
          color: colors.text,
        ),
        titleMedium: TextStyle(
          fontSize: 17,
          height: 1.35,
          fontWeight: FontWeight.w700,
          color: colors.text,
        ),
        titleSmall: TextStyle(
          fontSize: 15,
          height: 1.4,
          fontWeight: FontWeight.w700,
          color: colors.text,
        ),
        bodyLarge: TextStyle(
          fontSize: 15,
          height: 1.6,
          color: colors.textMuted,
        ),
        bodyMedium: TextStyle(
          fontSize: 13,
          height: 1.5,
          color: colors.textMuted,
        ),
        bodySmall: TextStyle(
          fontSize: 12,
          height: 1.5,
          color: colors.textMuted,
        ),
        labelLarge: TextStyle(
          fontSize: 15,
          fontWeight: FontWeight.w700,
          color: colors.text,
        ),
      ),
      canvasColor: colors.surface,
      dividerColor: colors.divider,
      splashColor: colors.brandTeal500.withValues(alpha: 0.08),
      highlightColor: colors.brandTeal500.withValues(alpha: 0.04),
      extensions: [colors],
    );

    return base.copyWith(
      actionIconTheme: ActionIconThemeData(
        backButtonIconBuilder: (_) =>
            const Icon(Icons.chevron_left_rounded, size: 26),
      ),
      iconButtonTheme: IconButtonThemeData(
        style: IconButton.styleFrom(
          minimumSize: const Size(48, 48),
          foregroundColor: colors.text,
        ),
      ),
      chipTheme: ChipThemeData(
        backgroundColor: colors.surface,
        selectedColor: colors.teal10,
        checkmarkColor: colors.brandTeal600,
        labelStyle: TextStyle(
          fontFamily: 'Inter',
          fontSize: 13,
          color: colors.text,
        ),
        side: BorderSide(color: colors.divider),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(99)),
      ),
      segmentedButtonTheme: SegmentedButtonThemeData(
        style: ButtonStyle(
          backgroundColor: WidgetStateProperty.resolveWith(
            (states) => states.contains(WidgetState.selected)
                ? colors.teal10
                : colors.surface,
          ),
          foregroundColor: WidgetStateProperty.resolveWith(
            (states) => states.contains(WidgetState.selected)
                ? colors.brandTeal600
                : colors.textMuted,
          ),
          side: WidgetStatePropertyAll(BorderSide(color: colors.divider)),
        ),
      ),
      pageTransitionsTheme: PageTransitionsTheme(
        builders: {
          ...base.pageTransitionsTheme.builders,
          TargetPlatform.android: const FadeForwardsPageTransitionsBuilder(),
        },
      ),
      appBarTheme: AppBarTheme(
        backgroundColor: colors.background,
        foregroundColor: colors.text,
        elevation: 0,
        scrolledUnderElevation: 0,
        surfaceTintColor: Colors.transparent,
        centerTitle: false,
        systemOverlayStyle: statusBarStyle,
        titleTextStyle: TextStyle(
          fontFamily: 'Inter',
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
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
        titleTextStyle: TextStyle(
          fontFamily: 'Inter',
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
          minimumSize: const Size(48, 56),
          backgroundColor: colors.btnPrimary,
          foregroundColor: colors.btnPrimaryFg,
          disabledBackgroundColor: colors.btnPrimaryDisabled,
          disabledForegroundColor: colors.btnPrimaryDisabledFg,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(20),
          ),
          textStyle: const TextStyle(
            fontFamily: 'Inter',
            fontSize: 15,
            fontWeight: FontWeight.w700,
          ),
        ),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(
          minimumSize: const Size(48, 56),
          foregroundColor: colors.text,
          side: BorderSide(color: colors.border, width: 1.2),
          textStyle: const TextStyle(
            fontFamily: 'Inter',
            fontSize: 15,
            fontWeight: FontWeight.w600,
          ),
        ),
      ),
      textButtonTheme: TextButtonThemeData(
        style: TextButton.styleFrom(
          foregroundColor: colors.brandTeal500,
          textStyle: const TextStyle(
            fontFamily: 'Inter',
            fontSize: 14,
            fontWeight: FontWeight.w700,
          ),
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: colors.inputFill,
        contentPadding: const EdgeInsets.symmetric(
          horizontal: 16,
          vertical: 14,
        ),
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
        backgroundColor: colors.surface,
        contentTextStyle: TextStyle(color: colors.text, fontFamily: 'Inter'),
        elevation: 0,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(20),
          side: BorderSide(color: colors.divider),
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
