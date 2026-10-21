import 'package:flutter/material.dart';

import '../../features/home/ui/widgets/provider_app_colors.dart';

/// The truck partner app theme: Karryam navy and teal, Inter, 8px rhythm,
/// 12–20px corners and 52px primary actions — the same system as the taxi
/// driver app. Material components pick it up, so screens built from plain
/// buttons, fields, dialogs and sheets match without per-screen styling.
ThemeData karryamTruckTheme() {
  const radius12 = BorderRadius.all(Radius.circular(12));
  const radius14 = BorderRadius.all(Radius.circular(14));
  final scheme = ColorScheme.fromSeed(
    seedColor: kNavy,
    primary: kNavy,
    onPrimary: Colors.white,
    secondary: kTeal,
    onSecondary: kNavy,
    surface: kCard,
    onSurface: kProviderText,
    error: kProviderRejectText,
  );
  final base = ThemeData(useMaterial3: true, colorScheme: scheme, fontFamily: 'Inter');
  final text = base.textTheme.apply(bodyColor: kProviderText, displayColor: kProviderText);

  OutlineInputBorder outline(Color color, [double width = 1]) =>
      OutlineInputBorder(borderRadius: radius12, borderSide: BorderSide(color: color, width: width));

  return base.copyWith(
    scaffoldBackgroundColor: kCanvas,
    textTheme: text.copyWith(
      headlineMedium: text.headlineMedium?.copyWith(fontWeight: FontWeight.w700, letterSpacing: -0.5),
      headlineSmall: text.headlineSmall?.copyWith(fontWeight: FontWeight.w700, letterSpacing: -0.4),
      titleLarge: text.titleLarge?.copyWith(fontWeight: FontWeight.w700, letterSpacing: -0.2),
      titleMedium: text.titleMedium?.copyWith(fontWeight: FontWeight.w600),
    ),
    appBarTheme: const AppBarTheme(
      backgroundColor: kCanvas,
      foregroundColor: kProviderText,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      centerTitle: true,
      titleTextStyle: TextStyle(fontFamily: 'Inter', fontSize: 15, fontWeight: FontWeight.w600, color: kProviderText),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        backgroundColor: kNavy,
        foregroundColor: Colors.white,
        disabledBackgroundColor: kProviderGreenSoft,
        disabledForegroundColor: Colors.white,
        minimumSize: const Size.fromHeight(52),
        shape: const RoundedRectangleBorder(borderRadius: radius14),
        textStyle: const TextStyle(fontFamily: 'Inter', fontSize: 15, fontWeight: FontWeight.w700),
      ),
    ),
    elevatedButtonTheme: ElevatedButtonThemeData(
      style: ElevatedButton.styleFrom(
        backgroundColor: kNavy,
        foregroundColor: Colors.white,
        elevation: 0,
        minimumSize: const Size.fromHeight(52),
        shape: const RoundedRectangleBorder(borderRadius: radius14),
        textStyle: const TextStyle(fontFamily: 'Inter', fontSize: 15, fontWeight: FontWeight.w700),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        foregroundColor: kProviderText,
        backgroundColor: kCard,
        minimumSize: const Size.fromHeight(52),
        side: const BorderSide(color: kProviderBorder),
        shape: const RoundedRectangleBorder(borderRadius: radius14),
        textStyle: const TextStyle(fontFamily: 'Inter', fontSize: 15, fontWeight: FontWeight.w600),
      ),
    ),
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(
        foregroundColor: kTealText,
        textStyle: const TextStyle(fontFamily: 'Inter', fontWeight: FontWeight.w600),
      ),
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: kCard,
      contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
      hintStyle: const TextStyle(color: kProviderMuted, fontWeight: FontWeight.w400),
      labelStyle: const TextStyle(color: kProviderMuted),
      border: outline(kProviderBorder),
      enabledBorder: outline(kProviderBorder),
      focusedBorder: outline(kTeal, 1.6),
      errorBorder: outline(kProviderRejectText),
      focusedErrorBorder: outline(kProviderRejectText, 1.6),
    ),
    cardTheme: const CardThemeData(
      color: kCard,
      elevation: 0,
      margin: EdgeInsets.zero,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.all(Radius.circular(18)),
        side: BorderSide(color: kProviderBorder),
      ),
    ),
    dividerTheme: const DividerThemeData(color: kProviderBorder, thickness: 1, space: 1),
    bottomSheetTheme: const BottomSheetThemeData(
      backgroundColor: kCard,
      surfaceTintColor: Colors.transparent,
      showDragHandle: true,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.vertical(top: Radius.circular(24))),
    ),
    dialogTheme: const DialogThemeData(
      backgroundColor: kCard,
      surfaceTintColor: Colors.transparent,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.all(Radius.circular(20))),
      titleTextStyle: TextStyle(fontFamily: 'Inter', fontSize: 18, fontWeight: FontWeight.w700, color: kProviderText),
    ),
    snackBarTheme: const SnackBarThemeData(
      behavior: SnackBarBehavior.floating,
      backgroundColor: kNavy,
      contentTextStyle: TextStyle(fontFamily: 'Inter', color: Colors.white),
      shape: RoundedRectangleBorder(borderRadius: radius12),
    ),
    progressIndicatorTheme: const ProgressIndicatorThemeData(color: kTeal),
    switchTheme: SwitchThemeData(
      thumbColor: WidgetStateProperty.resolveWith((s) => s.contains(WidgetState.selected) ? Colors.white : kProviderGreenSoft),
      trackColor: WidgetStateProperty.resolveWith((s) => s.contains(WidgetState.selected) ? kTeal : kProviderGreenTint),
      trackOutlineColor: WidgetStateProperty.all(Colors.transparent),
    ),
    radioTheme: RadioThemeData(
      fillColor: WidgetStateProperty.resolveWith((s) => s.contains(WidgetState.selected) ? kTealText : kProviderMuted),
    ),
    checkboxTheme: CheckboxThemeData(
      fillColor: WidgetStateProperty.resolveWith((s) => s.contains(WidgetState.selected) ? kTealText : Colors.transparent),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(5)),
    ),
    chipTheme: ChipThemeData(
      backgroundColor: kCard,
      selectedColor: kTealTint,
      side: const BorderSide(color: kProviderBorder),
      labelStyle: const TextStyle(fontFamily: 'Inter', color: kProviderText, fontWeight: FontWeight.w600),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
    ),
    tabBarTheme: const TabBarThemeData(
      labelColor: kProviderText,
      unselectedLabelColor: kProviderMuted,
      indicatorColor: kTeal,
      dividerColor: Colors.transparent,
      labelStyle: TextStyle(fontFamily: 'Inter', fontWeight: FontWeight.w700),
    ),
  );
}
