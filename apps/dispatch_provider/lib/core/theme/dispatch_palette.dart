import 'package:flutter/material.dart';

/// Theme-aware color palette. Registered on [ThemeData.extensions] and read
/// via `context.palette` (see `theme_extensions.dart`).
///
/// Light hex values come from the Karryam Foundations board.
/// Dark mode is a proper neutral dark (near-black grays) with brand teal as
/// the accent. Brand colors stay constant across themes.
///
/// Semantic button tokens (`btnPrimary*`, `btnDanger*`) live here so widgets
/// never hardcode a button color — the theme is the single source of truth.
@immutable
class DispatchPalette extends ThemeExtension<DispatchPalette> {
  final Color brandNavy900;
  final Color brandNavy800;
  final Color brandTeal500;
  final Color brandTeal600;
  final Color brandWhite;
  final Color brandGray400;
  final Color brandIndigo700;

  final Color background;
  final Color surface;
  final Color surfaceElevated;

  final Color text;
  final Color textMuted;
  final Color textDisabled;

  final Color card;
  final Color border;
  final Color divider;
  final Color inputFill;
  final Color scrim;

  final Color danger;
  final Color dangerSoft;
  final Color warning;
  final Color warningSoft;
  final Color success;
  final Color successSoft;
  final Color info;

  final Color navy80;
  final Color navy60;
  final Color navy40;
  final Color navy20;
  final Color navy10;

  final Color teal80;
  final Color teal60;
  final Color teal40;
  final Color teal20;
  final Color teal10;

  // ── Semantic button tokens ──────────────────────────────────────────────
  /// Filled primary button background (Go online, Accept, I've Arrived…).
  final Color btnPrimary;

  /// Text/icon color on the primary button.
  final Color btnPrimaryFg;

  /// Primary button background when disabled.
  final Color btnPrimaryDisabled;

  /// Text/icon on a disabled primary button.
  final Color btnPrimaryDisabledFg;

  /// Soft background for the red reject/cancel/end-trip pill.
  final Color btnDanger;

  /// Text/icon on the danger pill.
  final Color btnDangerFg;

  final LinearGradient motionGradient;
  final LinearGradient motionGradientVertical;
  final List<BoxShadow> buttonShadow;
  final List<BoxShadow> cardShadow;

  const DispatchPalette({
    required this.brandNavy900,
    required this.brandNavy800,
    required this.brandTeal500,
    required this.brandTeal600,
    required this.brandWhite,
    required this.brandGray400,
    required this.brandIndigo700,
    required this.background,
    required this.surface,
    required this.surfaceElevated,
    required this.text,
    required this.textMuted,
    required this.textDisabled,
    required this.card,
    required this.border,
    required this.divider,
    required this.inputFill,
    required this.scrim,
    required this.danger,
    required this.dangerSoft,
    required this.warning,
    required this.warningSoft,
    required this.success,
    required this.successSoft,
    required this.info,
    required this.navy80,
    required this.navy60,
    required this.navy40,
    required this.navy20,
    required this.navy10,
    required this.teal80,
    required this.teal60,
    required this.teal40,
    required this.teal20,
    required this.teal10,
    required this.btnPrimary,
    required this.btnPrimaryFg,
    required this.btnPrimaryDisabled,
    required this.btnPrimaryDisabledFg,
    required this.btnDanger,
    required this.btnDangerFg,
    required this.motionGradient,
    required this.motionGradientVertical,
    required this.buttonShadow,
    required this.cardShadow,
  });

  @override
  DispatchPalette copyWith({
    Color? brandNavy900,
    Color? brandNavy800,
    Color? brandTeal500,
    Color? brandTeal600,
    Color? brandWhite,
    Color? brandGray400,
    Color? brandIndigo700,
    Color? background,
    Color? surface,
    Color? surfaceElevated,
    Color? text,
    Color? textMuted,
    Color? textDisabled,
    Color? card,
    Color? border,
    Color? divider,
    Color? inputFill,
    Color? scrim,
    Color? danger,
    Color? dangerSoft,
    Color? warning,
    Color? warningSoft,
    Color? success,
    Color? successSoft,
    Color? info,
    Color? navy80,
    Color? navy60,
    Color? navy40,
    Color? navy20,
    Color? navy10,
    Color? teal80,
    Color? teal60,
    Color? teal40,
    Color? teal20,
    Color? teal10,
    Color? btnPrimary,
    Color? btnPrimaryFg,
    Color? btnPrimaryDisabled,
    Color? btnPrimaryDisabledFg,
    Color? btnDanger,
    Color? btnDangerFg,
    LinearGradient? motionGradient,
    LinearGradient? motionGradientVertical,
    List<BoxShadow>? buttonShadow,
    List<BoxShadow>? cardShadow,
  }) {
    return DispatchPalette(
      brandNavy900: brandNavy900 ?? this.brandNavy900,
      brandNavy800: brandNavy800 ?? this.brandNavy800,
      brandTeal500: brandTeal500 ?? this.brandTeal500,
      brandTeal600: brandTeal600 ?? this.brandTeal600,
      brandWhite: brandWhite ?? this.brandWhite,
      brandGray400: brandGray400 ?? this.brandGray400,
      brandIndigo700: brandIndigo700 ?? this.brandIndigo700,
      background: background ?? this.background,
      surface: surface ?? this.surface,
      surfaceElevated: surfaceElevated ?? this.surfaceElevated,
      text: text ?? this.text,
      textMuted: textMuted ?? this.textMuted,
      textDisabled: textDisabled ?? this.textDisabled,
      card: card ?? this.card,
      border: border ?? this.border,
      divider: divider ?? this.divider,
      inputFill: inputFill ?? this.inputFill,
      scrim: scrim ?? this.scrim,
      danger: danger ?? this.danger,
      dangerSoft: dangerSoft ?? this.dangerSoft,
      warning: warning ?? this.warning,
      warningSoft: warningSoft ?? this.warningSoft,
      success: success ?? this.success,
      successSoft: successSoft ?? this.successSoft,
      info: info ?? this.info,
      navy80: navy80 ?? this.navy80,
      navy60: navy60 ?? this.navy60,
      navy40: navy40 ?? this.navy40,
      navy20: navy20 ?? this.navy20,
      navy10: navy10 ?? this.navy10,
      teal80: teal80 ?? this.teal80,
      teal60: teal60 ?? this.teal60,
      teal40: teal40 ?? this.teal40,
      teal20: teal20 ?? this.teal20,
      teal10: teal10 ?? this.teal10,
      btnPrimary: btnPrimary ?? this.btnPrimary,
      btnPrimaryFg: btnPrimaryFg ?? this.btnPrimaryFg,
      btnPrimaryDisabled: btnPrimaryDisabled ?? this.btnPrimaryDisabled,
      btnPrimaryDisabledFg:
          btnPrimaryDisabledFg ?? this.btnPrimaryDisabledFg,
      btnDanger: btnDanger ?? this.btnDanger,
      btnDangerFg: btnDangerFg ?? this.btnDangerFg,
      motionGradient: motionGradient ?? this.motionGradient,
      motionGradientVertical:
          motionGradientVertical ?? this.motionGradientVertical,
      buttonShadow: buttonShadow ?? this.buttonShadow,
      cardShadow: cardShadow ?? this.cardShadow,
    );
  }

  @override
  DispatchPalette lerp(ThemeExtension<DispatchPalette>? other, double t) {
    if (other is! DispatchPalette) return this;
    Color c(Color a, Color b) => Color.lerp(a, b, t)!;
    return DispatchPalette(
      brandNavy900: c(brandNavy900, other.brandNavy900),
      brandNavy800: c(brandNavy800, other.brandNavy800),
      brandTeal500: c(brandTeal500, other.brandTeal500),
      brandTeal600: c(brandTeal600, other.brandTeal600),
      brandWhite: c(brandWhite, other.brandWhite),
      brandGray400: c(brandGray400, other.brandGray400),
      brandIndigo700: c(brandIndigo700, other.brandIndigo700),
      background: c(background, other.background),
      surface: c(surface, other.surface),
      surfaceElevated: c(surfaceElevated, other.surfaceElevated),
      text: c(text, other.text),
      textMuted: c(textMuted, other.textMuted),
      textDisabled: c(textDisabled, other.textDisabled),
      card: c(card, other.card),
      border: c(border, other.border),
      divider: c(divider, other.divider),
      inputFill: c(inputFill, other.inputFill),
      scrim: c(scrim, other.scrim),
      danger: c(danger, other.danger),
      dangerSoft: c(dangerSoft, other.dangerSoft),
      warning: c(warning, other.warning),
      warningSoft: c(warningSoft, other.warningSoft),
      success: c(success, other.success),
      successSoft: c(successSoft, other.successSoft),
      info: c(info, other.info),
      navy80: c(navy80, other.navy80),
      navy60: c(navy60, other.navy60),
      navy40: c(navy40, other.navy40),
      navy20: c(navy20, other.navy20),
      navy10: c(navy10, other.navy10),
      teal80: c(teal80, other.teal80),
      teal60: c(teal60, other.teal60),
      teal40: c(teal40, other.teal40),
      teal20: c(teal20, other.teal20),
      teal10: c(teal10, other.teal10),
      btnPrimary: c(btnPrimary, other.btnPrimary),
      btnPrimaryFg: c(btnPrimaryFg, other.btnPrimaryFg),
      btnPrimaryDisabled: c(btnPrimaryDisabled, other.btnPrimaryDisabled),
      btnPrimaryDisabledFg:
          c(btnPrimaryDisabledFg, other.btnPrimaryDisabledFg),
      btnDanger: c(btnDanger, other.btnDanger),
      btnDangerFg: c(btnDangerFg, other.btnDangerFg),
      motionGradient:
          LinearGradient.lerp(motionGradient, other.motionGradient, t)!,
      motionGradientVertical: LinearGradient.lerp(
          motionGradientVertical, other.motionGradientVertical, t)!,
      buttonShadow: t < 0.5 ? buttonShadow : other.buttonShadow,
      cardShadow: t < 0.5 ? cardShadow : other.cardShadow,
    );
  }

  // ───────────────────────────────────────────────────────────────────────
  // LIGHT — matches "Light surface (Default)" from the Foundations board.
  // ───────────────────────────────────────────────────────────────────────
  static const light = DispatchPalette(
    // Brand
    brandNavy900: Color(0xFF081F44),
    brandNavy800: Color(0xFF1A2F55),
    brandTeal500: Color(0xFF00C2AB),
    brandTeal600: Color(0xFF00A690),
    brandWhite: Color(0xFFFFFFFF),
    brandGray400: Color(0xFFA7B0B8),
    brandIndigo700: Color(0xFF2D3A5B),

    // Surfaces
    background: Color(0xFFFFFFFF),
    surface: Color(0xFFFFFFFF),
    surfaceElevated: Color(0xFFF6F8FC),

    // Text
    text: Color(0xFF081F44),
    textMuted: Color(0xFF6D798F),
    textDisabled: Color(0xFFA7B0B8),

    // Chrome
    card: Color(0xFFFFFFFF),
    border: Color(0xFFE1E4F0),
    divider: Color(0xFFEDE2F6),
    inputFill: Color(0xFFF6F8FC),
    scrim: Color(0x66000000),

    // Semantic
    danger: Color(0xFFE5484D),
    dangerSoft: Color(0xFFFDECEE),
    warning: Color(0xFFF57C00),
    warningSoft: Color(0xFFFFF4E5),
    success: Color(0xFF1F9D55),
    successSoft: Color(0xFFE7F5EC),
    info: Color(0xFF2F80ED),

    // Navy tints
    navy80: Color(0xFF3C4C69),
    navy60: Color(0xFF6D798F),
    navy40: Color(0xFF9DA5B4),
    navy20: Color(0xFFE1E4F0),
    navy10: Color(0xFFF1F4F8),

    // Teal tints
    teal80: Color(0xFF33CEB9),
    teal60: Color(0xFF66DACB),
    teal40: Color(0xFF99E7DC),
    teal20: Color(0xFFCCF3EE),
    teal10: Color(0xFFE6F9F6),

    // Button tokens
    btnPrimary: Color(0xFF00C2AB),
    btnPrimaryFg: Color(0xFFFFFFFF),
    btnPrimaryDisabled: Color(0x6600C2AB),
    btnPrimaryDisabledFg: Color(0xB3FFFFFF),
    btnDanger: Color(0xFFFDECEE),
    btnDangerFg: Color(0xFFE5484D),

    // Gradients
    motionGradient: LinearGradient(
      begin: Alignment.topLeft,
      end: Alignment.bottomRight,
      colors: [Color(0xFF081F44), Color(0xFF00C2AB)],
    ),
    motionGradientVertical: LinearGradient(
      begin: Alignment.topCenter,
      end: Alignment.bottomCenter,
      colors: [Color(0xFF081F44), Color(0xFF00C2AB)],
    ),

    // Shadows
    buttonShadow: [
      BoxShadow(
        color: Color(0x3300C2AB),
        blurRadius: 20,
        offset: Offset(0, 8),
      ),
    ],
    cardShadow: [
      BoxShadow(
        color: Color(0x0A081F44),
        blurRadius: 24,
        offset: Offset(0, 6),
      ),
    ],
  );

  // ───────────────────────────────────────────────────────────────────────
  // DARK — proper neutral dark mode. Near-black neutral grays for chrome,
  // brand teal stays as the accent.
  // ───────────────────────────────────────────────────────────────────────
  static const dark = DispatchPalette(
    // Brand identity — unchanged
    brandNavy900: Color(0xFF081F44),
    brandNavy800: Color(0xFF1A2F55),
    brandTeal500: Color(0xFF00C2AB),
    brandTeal600: Color(0xFF00A690),
    brandWhite: Color(0xFFFFFFFF),
    brandGray400: Color(0xFFA7B0B8),
    brandIndigo700: Color(0xFFB8C2D9),

    // Neutral near-black chrome
    background: Color(0xFF0F1115),
    surface: Color(0xFF181A1F),
    surfaceElevated: Color(0xFF21242A),

    text: Color(0xFFF5F7FA),
    textMuted: Color(0xFF9CA3AF),
    textDisabled: Color(0xFF6B7280),

    card: Color(0xFF181A1F),
    border: Color(0xFF2A2D34),
    divider: Color(0xFF22252B),
    inputFill: Color(0xFF1C1F25),
    scrim: Color(0x99000000),

    danger: Color(0xFFFF5A5F),
    dangerSoft: Color(0xFF3A1A1C),
    warning: Color(0xFFFFB020),
    warningSoft: Color(0xFF3A2A10),
    success: Color(0xFF2BD66A),
    successSoft: Color(0xFF102B1B),
    info: Color(0xFF4A9EFF),

    navy80: Color(0xFFCBD5E1),
    navy60: Color(0xFF9CA3AF),
    navy40: Color(0xFF6B7280),
    navy20: Color(0xFF2A2D34),
    navy10: Color(0xFF1C1F25),

    teal80: Color(0xFF33CEB9),
    teal60: Color(0xFF66DACB),
    teal40: Color(0xFF99E7DC),
    teal20: Color(0xFF0F3D38),
    teal10: Color(0xFF0B2A26),

    // Button tokens
    btnPrimary: Color(0xFF00C2AB),
    btnPrimaryFg: Color(0xFFFFFFFF),
    btnPrimaryDisabled: Color(0x6600C2AB),
    btnPrimaryDisabledFg: Color(0xB3FFFFFF),
    btnDanger: Color(0xFF3A1A1C),
    btnDangerFg: Color(0xFFFF5A5F),

    motionGradient: LinearGradient(
      begin: Alignment.topLeft,
      end: Alignment.bottomRight,
      colors: [Color(0xFF081F44), Color(0xFF00C2AB)],
    ),
    motionGradientVertical: LinearGradient(
      begin: Alignment.topCenter,
      end: Alignment.bottomCenter,
      colors: [Color(0xFF081F44), Color(0xFF00C2AB)],
    ),

    buttonShadow: [
      BoxShadow(
        color: Color(0x6600C2AB),
        blurRadius: 20,
        offset: Offset(0, 8),
      ),
    ],
    cardShadow: [
      BoxShadow(
        color: Color(0x80000000),
        blurRadius: 24,
        offset: Offset(0, 6),
      ),
    ],
  );
}