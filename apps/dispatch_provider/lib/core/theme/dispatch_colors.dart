import 'package:flutter/material.dart';

class DispatchColors {
  // ── Brand (from Foundations board) ──────────────────────────────────────
  static const brandNavy900 = Color(0xFF081F44); // Midnight Navy
  static const brandNavy800 = Color(0xFF1A2F55);
  static const brandTeal500 = Color(0xFF00C2AB); // Aurora Teal
  static const brandTeal600 = Color(0xFF00A690);
  static const brandWhite   = Color(0xFFFFFFFF); // Pure White
  static const brandGray400 = Color(0xFFA7B0B8); // Forge Gray
  static const brandIndigo700 = Color(0xFF2D3A5B); // Indigo Gray

  // ── Surfaces (light) ────────────────────────────────────────────────────
  static const background = Color(0xFFFFFFFF);
  static const surface    = Color(0xFFF6F8FC);
  static const surfaceElevated = Color(0xFFFFFFFF);

  // ── Legacy aliases ──────────────────────────────────────────────────────
  static const primary = brandTeal500;      // ← primary is TEAL
  static const primaryDark = Color(0xFF061634);
  static const primarySoft = Color(0xFFCED2DA);
  static const primaryTint = Color(0xFFE7E9EC);
  static const text = brandNavy900;
  static const textMuted = brandIndigo700;
  static const textDisabled = brandGray400;
  static const card = brandWhite;
  static const border = Color(0xFFE1E4F0);
  static const backgroundDark = brandNavy900;
  static const actionAccent = brandTeal500;

  // ── Semantic ────────────────────────────────────────────────────────────
  static const danger = Color(0xFFE5484D);
  static const dangerSoft = Color(0xFFFDECEE);
  static const warning = Color(0xFFF57C00);
  static const warningSoft = Color(0xFFFFF4E5);
  static const success = Color(0xFF1F9D55);
  static const successSoft = Color(0xFFE7F5EC);
  static const info = Color(0xFF2F80ED);

  // ── Gradients ───────────────────────────────────────────────────────────
  static const motionGradient = LinearGradient(
    begin: Alignment.topLeft,
    end: Alignment.bottomRight,
    colors: [brandNavy900, brandTeal500],
  );
  static const motionGradientVertical = LinearGradient(
    begin: Alignment.topCenter,
    end: Alignment.bottomCenter,
    colors: [brandNavy900, brandTeal500],
  );
  static const radialBlue = motionGradient;

  static const buttonShadow = [
    BoxShadow(color: Color(0x3300C2AB), blurRadius: 20, offset: Offset(0, 8)),
  ];
  static const cardShadow = [
    BoxShadow(color: Color(0x0A081F44), blurRadius: 24, offset: Offset(0, 6)),
  ];

  // ── Navy tints ──────────────────────────────────────────────────────────
  static const navy80 = Color(0xFF3C4C69);
  static const navy60 = Color(0xFF6D798F);
  static const navy40 = Color(0xFF9DA5B4);
  static const navy20 = Color(0xFFE1E4F0);
  static const navy10 = Color(0xFFF1F4F8);

  // ── Teal tints ──────────────────────────────────────────────────────────
  static const teal80 = Color(0xFF33CEB9);
  static const teal60 = Color(0xFF66DACB);
  static const teal40 = Color(0xFF99E7DC);
  static const teal20 = Color(0xFFCCF3EE);
  static const teal10 = Color(0xFFE6F9F6);

  const DispatchColors._();
}