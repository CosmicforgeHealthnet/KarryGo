import 'package:flutter/material.dart';

import 'karryam_tokens.dart';

abstract final class KarryamTheme {
  static ThemeData light() {
    final scheme = ColorScheme.fromSeed(
      seedColor: KarryamColors.midnightNavy,
      brightness: Brightness.light,
      primary: KarryamColors.midnightNavy,
      secondary: KarryamColors.auroraTeal,
      surface: KarryamColors.surface,
      error: KarryamColors.danger,
    );

    return ThemeData(
      useMaterial3: true,
      fontFamily: 'Inter',
      colorScheme: scheme,
      scaffoldBackgroundColor: KarryamColors.background,
      dividerColor: KarryamColors.border,
      textTheme: const TextTheme(
        displayLarge: TextStyle(
          fontSize: 48,
          height: 1.06,
          fontWeight: FontWeight.w700,
          color: KarryamColors.textPrimary,
        ),
        headlineLarge: TextStyle(
          fontSize: 34,
          height: 1.12,
          fontWeight: FontWeight.w700,
          color: KarryamColors.textPrimary,
        ),
        headlineMedium: TextStyle(
          fontSize: 28,
          height: 1.16,
          fontWeight: FontWeight.w600,
          color: KarryamColors.textPrimary,
        ),
        titleLarge: TextStyle(
          fontSize: 22,
          height: 1.2,
          fontWeight: FontWeight.w600,
          color: KarryamColors.textPrimary,
        ),
        titleMedium: TextStyle(
          fontSize: 17,
          height: 1.3,
          fontWeight: FontWeight.w600,
          color: KarryamColors.textPrimary,
        ),
        bodyLarge: TextStyle(
          fontSize: 16,
          height: 1.5,
          fontWeight: FontWeight.w400,
          color: KarryamColors.textPrimary,
        ),
        bodyMedium: TextStyle(
          fontSize: 14,
          height: 1.5,
          fontWeight: FontWeight.w400,
          color: KarryamColors.textSecondary,
        ),
        labelLarge: TextStyle(
          fontSize: 14,
          height: 1.2,
          fontWeight: FontWeight.w600,
        ),
        labelMedium: TextStyle(
          fontSize: 12,
          height: 1.2,
          fontWeight: FontWeight.w600,
        ),
      ),
      appBarTheme: const AppBarTheme(
        elevation: 0,
        scrolledUnderElevation: 0,
        centerTitle: false,
        backgroundColor: KarryamColors.background,
        foregroundColor: KarryamColors.textPrimary,
      ),
      cardTheme: CardThemeData(
        color: KarryamColors.surface,
        elevation: 0,
        margin: EdgeInsets.zero,
        shape: RoundedRectangleBorder(
          side: const BorderSide(color: KarryamColors.border),
          borderRadius: BorderRadius.circular(KarryamRadius.lg),
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: KarryamColors.surface,
        contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 15),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(KarryamRadius.md),
          borderSide: const BorderSide(color: KarryamColors.border),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(KarryamRadius.md),
          borderSide: const BorderSide(color: KarryamColors.border),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(KarryamRadius.md),
          borderSide: const BorderSide(color: KarryamColors.midnightNavy, width: 1.5),
        ),
        errorBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(KarryamRadius.md),
          borderSide: const BorderSide(color: KarryamColors.danger),
        ),
      ),
      bottomSheetTheme: const BottomSheetThemeData(
        backgroundColor: KarryamColors.surface,
        modalBackgroundColor: KarryamColors.surface,
        showDragHandle: true,
      ),
    );
  }

  static ThemeData dark() {
    final base = light();
    return base.copyWith(
      brightness: Brightness.dark,
      scaffoldBackgroundColor: KarryamColors.midnightNavy,
      colorScheme: base.colorScheme.copyWith(
        brightness: Brightness.dark,
        primary: KarryamColors.auroraTeal,
        secondary: KarryamColors.auroraTeal,
        surface: KarryamColors.indigoGray,
        onSurface: KarryamColors.pureWhite,
      ),
      appBarTheme: const AppBarTheme(
        elevation: 0,
        scrolledUnderElevation: 0,
        backgroundColor: KarryamColors.midnightNavy,
        foregroundColor: KarryamColors.pureWhite,
      ),
      textTheme: base.textTheme.apply(
        bodyColor: KarryamColors.pureWhite,
        displayColor: KarryamColors.pureWhite,
      ),
    );
  }
}
