import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

abstract final class KarryamColors {
  static const canvas = Color(0xFFF5F7F3);
  static const navy = Color(0xFF0B1F44);
  static const teal = Color(0xFF00C2A8);
  static const indigo = Color(0xFF2D3A5B);
  static const gray = Color(0xFFA7B0BB);
  static const mint = Color(0xFFE6F9F6);
  static const border = Color(0xFFCED2DA);
  static const tealInk = Color(0xFF007D6D);
  static const error = Color(0xFFB42318);
  static const darkCanvas = Color(0xFF0E151C);
  static const darkSurface = Color(0xFF17212B);
  static const darkRaised = Color(0xFF202D39);
  static const darkBorder = Color(0xFF344452);
  static const darkText = Color(0xFFE9F1F4);
  static const darkMuted = Color(0xFFB1C1CB);
}

extension VendorAdaptiveColor on Color {
  double _darkness(BuildContext context) =>
      ((KarryamColors.canvas.r - Theme.of(context).scaffoldBackgroundColor.r) /
              (KarryamColors.canvas.r - KarryamColors.darkCanvas.r))
          .clamp(0.0, 1.0);

  Color forSurface(BuildContext context) =>
      Color.lerp(this, _darkSurface(), _darkness(context))!;

  Color _darkSurface() {
    final rgb = toARGB32() & 0xFFFFFF;
    if (rgb == 0xF5F7F3) return KarryamColors.darkCanvas.withValues(alpha: a);
    if (rgb == 0xFFFFFF) return KarryamColors.darkSurface.withValues(alpha: a);
    if (rgb == 0x00C2A8) return this;
    final hsl = HSLColor.fromColor(withValues(alpha: 1));
    if (hsl.lightness > .65) {
      return KarryamColors.darkRaised.withValues(alpha: a);
    }
    if (a < .25 && hsl.saturation > .25) return withValues(alpha: .14);
    return this;
  }

  Color forInk(BuildContext context) =>
      Color.lerp(this, _darkInk(), _darkness(context))!;

  Color _darkInk() {
    final rgb = toARGB32() & 0xFFFFFF;
    if (rgb == 0xFFFFFF || rgb == 0x00C2A8) return this;
    if (rgb == 0x0B1F44) return KarryamColors.darkText.withValues(alpha: a);
    if (rgb == 0x2D3A5B || rgb == 0xA7B0BB) {
      return KarryamColors.darkMuted.withValues(alpha: a);
    }
    if (rgb == 0x007D6D) return const Color(0xFF64DBC9).withValues(alpha: a);
    if (rgb == 0xB42318) return const Color(0xFFFF8A80).withValues(alpha: a);
    final hsl = HSLColor.fromColor(this);
    if (hsl.lightness < .68) return hsl.withLightness(.72).toColor();
    return this;
  }

  Color forOutline(BuildContext context) =>
      Color.lerp(this, _darkOutline(), _darkness(context))!;

  Color _darkOutline() {
    final hsl = HSLColor.fromColor(this);
    if (hsl.lightness > .6 && hsl.saturation < .25) {
      return KarryamColors.darkBorder.withValues(alpha: a);
    }
    return _darkInk();
  }
}

ThemeData vendorTheme({Brightness brightness = Brightness.light}) {
  final dark = brightness == Brightness.dark;
  final surface = dark ? KarryamColors.darkSurface : Colors.white;
  final ink = dark ? KarryamColors.darkText : KarryamColors.navy;
  final muted = dark ? KarryamColors.darkMuted : KarryamColors.indigo;
  final outline = dark ? KarryamColors.darkBorder : KarryamColors.border;
  final accentInk = dark ? const Color(0xFF64DBC9) : KarryamColors.tealInk;
  final error = dark ? const Color(0xFFFF8A80) : KarryamColors.error;
  final scheme =
      ColorScheme.fromSeed(
        seedColor: KarryamColors.teal,
        brightness: brightness,
      ).copyWith(
        primary: KarryamColors.teal,
        onPrimary: KarryamColors.navy,
        secondary: KarryamColors.teal,
        onSecondary: KarryamColors.navy,
        surface: surface,
        onSurface: ink,
        onSurfaceVariant: muted,
        surfaceContainerLowest: dark
            ? KarryamColors.darkCanvas
            : KarryamColors.canvas,
        surfaceContainerLow: surface,
        surfaceContainer: dark
            ? KarryamColors.darkRaised
            : const Color(0xFFEFF3EF),
        surfaceContainerHigh: dark
            ? const Color(0xFF273744)
            : const Color(0xFFE8EFEC),
        surfaceContainerHighest: dark
            ? const Color(0xFF30414F)
            : const Color(0xFFE2EAE6),
        outline: outline,
        outlineVariant: dark
            ? const Color(0xFF2A3945)
            : const Color(0xFFE1E8E3),
        error: error,
        onError: dark ? const Color(0xFF4B1713) : Colors.white,
      );
  return ThemeData(
    useMaterial3: true,
    brightness: brightness,
    fontFamily: 'Inter',
    colorScheme: scheme,
    scaffoldBackgroundColor: dark
        ? KarryamColors.darkCanvas
        : KarryamColors.canvas,
    canvasColor: surface,
    dividerColor: scheme.outlineVariant,
    textTheme: TextTheme(
      headlineLarge: TextStyle(
        fontSize: 38,
        height: 1.12,
        letterSpacing: -1.8,
        fontWeight: FontWeight.w700,
        color: ink,
      ),
      // Default text. Light mode's "muted" indigo is nearly as dark as the
      // ink, so headings that never set a colour still read strongly. Dark
      // mode's muted grey made those same headings look faded, so dark mode
      // uses a near-white here; text that is meant to be muted sets it.
      bodyLarge: TextStyle(
        fontSize: 15,
        height: 1.6,
        color: dark ? const Color(0xFFDCE6EB) : muted,
      ),
      bodyMedium: TextStyle(
        fontSize: 13,
        height: 1.5,
        color: dark ? const Color(0xFFDCE6EB) : muted,
      ),
      labelLarge: TextStyle(
        fontSize: 15,
        fontWeight: FontWeight.w700,
        color: ink,
      ),
    ),
    appBarTheme: AppBarTheme(
      backgroundColor: dark ? KarryamColors.darkCanvas : KarryamColors.canvas,
      foregroundColor: ink,
      elevation: 0,
      scrolledUnderElevation: 0,
      surfaceTintColor: Colors.transparent,
      systemOverlayStyle: dark
          ? SystemUiOverlayStyle.light
          : SystemUiOverlayStyle.dark,
    ),
    bottomSheetTheme: BottomSheetThemeData(
      backgroundColor: surface,
      surfaceTintColor: Colors.transparent,
      dragHandleColor: muted.withValues(alpha: .4),
    ),
    dialogTheme: DialogThemeData(
      backgroundColor: surface,
      surfaceTintColor: Colors.transparent,
    ),
    popupMenuTheme: PopupMenuThemeData(
      color: surface,
      surfaceTintColor: Colors.transparent,
    ),
    snackBarTheme: SnackBarThemeData(
      backgroundColor: surface,
      contentTextStyle: TextStyle(color: ink, fontFamily: 'Inter'),
      behavior: SnackBarBehavior.floating,
      elevation: 0,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(20),
        side: BorderSide(color: outline),
      ),
    ),
    cardTheme: CardThemeData(
      color: surface,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
    ),
    iconTheme: IconThemeData(color: muted),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: surface,
      contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 18),
      labelStyle: TextStyle(color: muted),
      hintStyle: TextStyle(color: muted.withValues(alpha: .7)),
      prefixIconColor: muted,
      suffixIconColor: muted,
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(15),
        borderSide: BorderSide(color: outline),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(15),
        borderSide: BorderSide(color: outline),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(15),
        borderSide: BorderSide(color: accentInk, width: 2),
      ),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        backgroundColor: KarryamColors.teal,
        foregroundColor: KarryamColors.navy,
        minimumSize: const Size.fromHeight(58),
        padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 16),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
      ),
    ),
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(
        foregroundColor: accentInk,
        minimumSize: const Size(44, 44),
        textStyle: const TextStyle(
          fontFamily: 'Inter',
          fontSize: 13,
          fontWeight: FontWeight.w700,
        ),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        foregroundColor: accentInk,
        side: BorderSide(color: outline),
      ),
    ),
    progressIndicatorTheme: const ProgressIndicatorThemeData(
      color: KarryamColors.teal,
    ),
    chipTheme: ChipThemeData(
      backgroundColor: surface,
      selectedColor: dark ? KarryamColors.darkRaised : KarryamColors.mint,
      side: BorderSide(color: outline),
      labelStyle: TextStyle(color: ink),
      checkmarkColor: accentInk,
    ),
    segmentedButtonTheme: SegmentedButtonThemeData(
      style: ButtonStyle(
        foregroundColor: WidgetStatePropertyAll(ink),
        side: WidgetStatePropertyAll(BorderSide(color: outline)),
        backgroundColor: WidgetStateProperty.resolveWith(
          (states) => states.contains(WidgetState.selected)
              ? (dark ? KarryamColors.darkRaised : KarryamColors.mint)
              : surface,
        ),
      ),
    ),
    switchTheme: SwitchThemeData(
      thumbColor: WidgetStateProperty.resolveWith(
        (states) =>
            states.contains(WidgetState.selected) ? KarryamColors.teal : null,
      ),
    ),
    textSelectionTheme: TextSelectionThemeData(
      cursorColor: accentInk,
      selectionColor: KarryamColors.teal.withValues(alpha: .25),
      selectionHandleColor: accentInk,
    ),
  );
}
