import 'package:flutter/material.dart';

/// Karryam product design tokens.
///
/// Brand values are taken from Karryam Brand Guidelines v1.0 (September 2026).
/// Semantic success/warning/error colors are product-state colors, not brand colors.
abstract final class KarryamColors {
  static const midnightNavy = Color(0xFF0B1F44);
  static const auroraTeal = Color(0xFF00C2A8);
  static const pureWhite = Color(0xFFFFFFFF);
  static const forgeGray = Color(0xFFA7B0BB);
  static const indigoGray = Color(0xFF2D3A5B);

  static const navy80 = Color(0xFF3C4C69);
  static const navy60 = Color(0xFF6D798F);
  static const navy40 = Color(0xFF9DA5B4);
  static const navy20 = Color(0xFFCED2DA);
  static const navy10 = Color(0xFFE7E9EC);

  static const teal80 = Color(0xFF33CEB9);
  static const teal60 = Color(0xFF66DACB);
  static const teal40 = Color(0xFF99E7DC);
  static const teal20 = Color(0xFFCCF3EE);
  static const teal10 = Color(0xFFE6F9F6);

  static const background = pureWhite;
  static const backgroundSubtle = Color(0xFFF7F8FA);
  static const surface = pureWhite;
  static const textPrimary = midnightNavy;
  static const textSecondary = indigoGray;
  static const border = navy20;
  static const actionPrimary = midnightNavy;
  static const actionAccent = auroraTeal;

  static const success = Color(0xFF16865C);
  static const successSurface = Color(0xFFE8F6F0);
  static const warning = Color(0xFF9A6700);
  static const warningSurface = Color(0xFFFFF4D6);
  static const danger = Color(0xFFB42318);
  static const dangerSurface = Color(0xFFFFEDEC);
  static const info = Color(0xFF2457C5);
  static const infoSurface = Color(0xFFEAF0FF);

  static const motionGradient = LinearGradient(
    begin: Alignment.centerLeft,
    end: Alignment.centerRight,
    colors: [midnightNavy, auroraTeal],
  );
}

abstract final class KarryamSpacing {
  static const xxs = 4.0;
  static const xs = 8.0;
  static const sm = 12.0;
  static const md = 16.0;
  static const lg = 24.0;
  static const xl = 32.0;
  static const xxl = 48.0;
  static const xxxl = 64.0;
}

abstract final class KarryamRadius {
  static const sm = 10.0;
  static const md = 14.0;
  static const lg = 20.0;
  static const xl = 28.0;
  static const pill = 999.0;
}

abstract final class KarryamElevation {
  static const subtle = [
    BoxShadow(
      color: Color(0x120B1F44),
      blurRadius: 18,
      offset: Offset(0, 6),
    ),
  ];

  static const floating = [
    BoxShadow(
      color: Color(0x220B1F44),
      blurRadius: 28,
      offset: Offset(0, 12),
    ),
  ];
}

abstract final class KarryamMotion {
  static const fast = Duration(milliseconds: 140);
  static const standard = Duration(milliseconds: 220);
  static const emphasized = Duration(milliseconds: 320);
}
