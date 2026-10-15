import 'package:flutter/material.dart';

class KarryamColors {
  KarryamColors.of(BuildContext context)
    : dark = Theme.of(context).brightness == Brightness.dark;
  final bool dark;
  static const navy = Color(0xFF0B1F44);
  static const teal = Color(0xFF00C2A8);
  Color get primary => dark ? teal : navy;
  Color get onPrimary => dark ? navy : Colors.white;
  Color get surface => dark ? const Color(0xFF081832) : const Color(0xFFF5F6F8);
  Color get card => dark ? const Color(0xFF12284C) : Colors.white;
  Color get text => dark ? Colors.white : navy;
  Color get textMuted =>
      dark ? const Color(0xFFCED2DA) : const Color(0xFF3C4C69);
  Color get border => dark ? const Color(0xFF3C4C69) : const Color(0xFFE7E9EC);
  Color get primaryTint =>
      dark ? const Color(0xFF103C43) : const Color(0xFFE6F9F6);
  Color get primarySoft =>
      dark ? const Color(0xFF99E7DC) : const Color(0xFF006B5C);
  Color get info => primarySoft;
  Color get infoSoft => primaryTint;
  Color get success => primarySoft;
  Color get successSoft => primaryTint;
  Color get danger => dark ? const Color(0xFFFFB8C4) : const Color(0xFFAD263A);
  Color get dangerSoft =>
      dark ? const Color(0xFF492737) : const Color(0xFFFBECEF);
  Color get warning => dark ? const Color(0xFFFFD58A) : const Color(0xFF805500);
  Color get warningSoft =>
      dark ? const Color(0xFF493C26) : const Color(0xFFFFF4DC);
  LinearGradient get radialBlue => const LinearGradient(
    colors: [navy, Color(0xFF163D52)],
    begin: Alignment.topLeft,
    end: Alignment.bottomRight,
  );
}

ThemeData karryamTheme(Brightness brightness) {
  final dark = brightness == Brightness.dark;
  final scheme =
      ColorScheme.fromSeed(
        seedColor: KarryamColors.navy,
        brightness: brightness,
      ).copyWith(
        primary: dark ? KarryamColors.teal : KarryamColors.navy,
        onPrimary: dark ? KarryamColors.navy : Colors.white,
        secondary: KarryamColors.teal,
        surface: dark ? const Color(0xFF12284C) : Colors.white,
        onSurface: dark ? Colors.white : KarryamColors.navy,
        error: dark ? const Color(0xFFFFB8C4) : const Color(0xFFAD263A),
      );
  return ThemeData(
    useMaterial3: true,
    fontFamily: 'Inter',
    colorScheme: scheme,
    scaffoldBackgroundColor: dark
        ? const Color(0xFF081832)
        : const Color(0xFFF5F6F8),
    appBarTheme: AppBarTheme(
      backgroundColor: scheme.surface,
      foregroundColor: scheme.onSurface,
      elevation: 0,
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        minimumSize: const Size(48, 52),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
        textStyle: const TextStyle(
          fontFamily: 'Inter',
          fontWeight: FontWeight.w600,
          fontSize: 15,
        ),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        minimumSize: const Size(48, 52),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
      ),
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: scheme.surface,
      contentPadding: const EdgeInsets.all(18),
      border: OutlineInputBorder(borderRadius: BorderRadius.circular(12)),
    ),
    cardTheme: CardThemeData(
      color: scheme.surface,
      elevation: 1,
      margin: EdgeInsets.zero,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(18)),
    ),
    navigationBarTheme: NavigationBarThemeData(
      backgroundColor: scheme.surface,
      indicatorColor: dark ? const Color(0xFF103C43) : const Color(0xFFE6F9F6),
    ),
    bottomSheetTheme: BottomSheetThemeData(
      backgroundColor: scheme.surface,
      showDragHandle: true,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(28)),
      ),
    ),
  );
}
