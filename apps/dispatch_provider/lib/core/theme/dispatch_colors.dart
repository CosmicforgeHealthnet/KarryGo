import 'package:flutter/material.dart';

/// Brand navy palette. Success/online-status states use [success] /
/// [successSoft] instead of [primary] so they stay visually distinct.
class DispatchColors {
  static const primary = Color(0xFF05124D);
  static const primaryDark = Color(0xFF020B2E);
  static const primarySoft = Color(0xFFC1C5D4);
  static const primaryTint = Color(0xFFE6E8EE);
  static const text = Color(0xFF000000);
  static const textMuted = Color(0xFF5B5959);
  static const textDisabled = Color(0xFF5B5959);
  static const surface = Color(0xFFF7F7F7);
  static const card = Color(0xFFFEFEFE);
  static const border = Color(0xFFE3E4E8);
  static const danger = Color(0xFFA81717);
  static const dangerSoft = Color(0xFFF7E4E4);
  static const warning = Color(0xFFE8B600);
  static const warningSoft = Color(0xFFFCF3D9);
  static const success = Color(0xFF1F9D55);
  static const successSoft = Color(0xFFE7F5EC);

  static const radialBlue = LinearGradient(
    begin: Alignment.topLeft,
    end: Alignment.bottomRight,
    colors: [Color(0xFF2761A7), Color(0xFF05124D)],
  );

  const DispatchColors._();
}
