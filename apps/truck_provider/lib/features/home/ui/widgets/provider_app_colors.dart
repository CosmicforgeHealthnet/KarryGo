import 'package:flutter/material.dart';

// Karryam truck-partner palette, shared with the taxi driver app so both
// partner apps read as one product. Light ("Daylight") and dark ("After
// hours") values; the app root picks the active one with [KPalette.apply]
// before every build, so the tokens below always match the current theme.
//
// The historical names (kProviderGreen etc.) are kept so the existing call
// sites pick up the palette without churn: "Green" is the brand primary
// (navy in light, teal in dark), not a green.

/// Which palette the token getters return.
class KPalette {
  KPalette._();
  static bool dark = false;

  /// Selects the palette for [brightness]; returns true when it changed.
  static bool apply(Brightness brightness) {
    final next = brightness == Brightness.dark;
    if (next == dark) return false;
    dark = next;
    return true;
  }
}

Color _p(int light, int darkValue) => Color(KPalette.dark ? darkValue : light);

// ─── Brand (fixed in both themes) ─────────────────────────────────────────────
const kNavy = Color(0xFF0B1F44);
const kNavyDeep = Color(0xFF071530);
const kTeal = Color(0xFF00C2A8);

/// Star ratings.
const kAmber = Color(0xFFF2A900);

// ─── Theme-aware tokens ───────────────────────────────────────────────────────

/// Primary action fill: navy by day, teal after hours.
Color get kPrimary => KPalette.dark ? kTeal : kNavy;

/// Text and icons on [kPrimary].
Color get kOnPrimary => KPalette.dark ? kNavy : Colors.white;

/// Teal that is readable as text on the canvas and cards.
Color get kTealText => _p(0xFF006B5C, 0xFF99E7DC);
Color get kTealTint => _p(0xFFE3F7F4, 0xFF103C43);

Color get kCanvas => _p(0xFFF5F6F8, 0xFF081832);
Color get kCard => _p(0xFFFFFFFF, 0xFF12284C);

/// A selected card's wash.
Color get kSelectedWash => _p(0xFFF4FCFB, 0xFF123A48);

// ─── Legacy names, mapped onto the palette ────────────────────────────────────
Color get kProviderGreen => kPrimary;
Color get kProviderGreenTint => _p(0xFFE8EBF1, 0xFF1B3358);
Color get kProviderGreenPale => _p(0xFFDDE2EA, 0xFF22385E);
Color get kProviderGreenSoft => _p(0xFFB9C2D2, 0xFF3C4C69);
Color get kProviderDarkGreen => kNavyDeep;
Color get kProviderText => _p(0xFF0B1F44, 0xFFFFFFFF);
Color get kProviderMuted => _p(0xFF5E6A7D, 0xFFB6BECC);
Color get kProviderBorder => _p(0xFFE4E7EC, 0xFF2C4167);
Color get kProviderSurface => kCanvas;

// Reject / decline (soft red background, red text).
Color get kProviderRejectBg => _p(0xFFFBEAEC, 0xFF492737);
Color get kProviderRejectText => _p(0xFFAD263A, 0xFFFFB8C4);

// Genuine success / online / completed: Karryam teal (text-safe shade).
Color get kProviderSuccess => kTealText;
Color get kProviderSuccessTint => kTealTint;

// Money cards: deep navy with a soft lift towards the top-left.
LinearGradient get kProviderBalanceGradient => LinearGradient(
      begin: Alignment.topLeft,
      end: Alignment.bottomRight,
      colors: KPalette.dark
          ? const [Color(0xFF1D3F78), Color(0xFF12284C)]
          : const [Color(0xFF173468), kNavy],
    );

// Page background behind cards.
Color get kProviderPageBg => kCanvas;

// Waiting / in-review states.
Color get kWarning => _p(0xFF805500, 0xFFFFD58A);
Color get kWarningTint => _p(0xFFFFF4DC, 0xFF493C26);
