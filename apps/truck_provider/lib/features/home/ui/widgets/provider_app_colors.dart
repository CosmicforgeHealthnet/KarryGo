import 'package:flutter/material.dart';

// NOTE: these constants keep their historical "Green" names for backward
// compatibility with ~750 call sites, but the values are the navy brand
// palette (#05124D) — not green. See kProviderSuccess* for genuine
// success/online-status green.
const kProviderGreen = Color(0xFF05124D);
const kProviderGreenTint = Color(0xFFE6E8EE);
const kProviderGreenPale = Color(0xFFDADCE4);
const kProviderGreenSoft = Color(0xFFC1C5D4);
const kProviderDarkGreen = Color(0xFF020B2E);
const kProviderText = Color(0xFF000000);
const kProviderMuted = Color(0xFF5B5959);
const kProviderBorder = Color(0xFFE3E4E8);
const kProviderSurface = Color(0xFFF7F7F7);

// Reject / decline button colors (light pink bg, red text)
const kProviderRejectBg = Color(0xFFF7E4E4);
const kProviderRejectText = Color(0xFFA81717);

// Genuine success / online-status / completed-trip green (kept distinct from
// the brand navy above so status pills stay legible at a glance).
const kProviderSuccess = Color(0xFF1F9D55);
const kProviderSuccessTint = Color(0xFFE7F5EC);

// Balance card gradient — Radial Blue (light navy top-left) to deep navy
// (bottom-right).
const kProviderBalanceGradient = LinearGradient(
  begin: Alignment.topLeft,
  end: Alignment.bottomRight,
  colors: [Color(0xFF2761A7), Color(0xFF05124D)],
);

// Page background behind the white cards so they read as elevated cards.
const kProviderPageBg = Color(0xFFF7F7F7);
