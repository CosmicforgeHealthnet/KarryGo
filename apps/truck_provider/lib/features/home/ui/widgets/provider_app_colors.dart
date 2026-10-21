import 'package:flutter/material.dart';

// Karryam truck-partner palette (light / "Daylight"), shared with the taxi
// driver app so both partner apps read as one product.
//
// The historical names (kProviderGreen etc.) are kept so the ~750 existing
// call sites pick up the new palette without churn: "Green" is the brand
// primary (Karryam navy), not a green.

// ─── Brand ────────────────────────────────────────────────────────────────────
const kNavy = Color(0xFF0B1F44);
const kNavyDeep = Color(0xFF071530);
const kTeal = Color(0xFF00C2A8);
// Teal is too light for text on white; this is the accessible teal for text.
const kTealText = Color(0xFF006B5C);
const kTealTint = Color(0xFFE3F7F4);

// ─── Surfaces ─────────────────────────────────────────────────────────────────
const kCanvas = Color(0xFFF5F6F8);
const kCard = Color(0xFFFFFFFF);

// ─── Legacy names, mapped onto the palette ────────────────────────────────────
const kProviderGreen = kNavy;
const kProviderGreenTint = Color(0xFFE8EBF1);
const kProviderGreenPale = Color(0xFFDDE2EA);
const kProviderGreenSoft = Color(0xFFB9C2D2);
const kProviderDarkGreen = kNavyDeep;
const kProviderText = Color(0xFF0B1F44);
const kProviderMuted = Color(0xFF5E6A7D);
const kProviderBorder = Color(0xFFE4E7EC);
const kProviderSurface = kCanvas;

// Reject / decline (soft red background, red text).
const kProviderRejectBg = Color(0xFFFBEAEC);
const kProviderRejectText = Color(0xFFAD263A);

// Genuine success / online / completed: Karryam teal (text-safe shade).
const kProviderSuccess = kTealText;
const kProviderSuccessTint = kTealTint;

// Money cards: deep navy with a soft lift towards the top-left.
const kProviderBalanceGradient = LinearGradient(
  begin: Alignment.topLeft,
  end: Alignment.bottomRight,
  colors: [Color(0xFF173468), kNavy],
);

// Page background behind white cards.
const kProviderPageBg = kCanvas;

// Waiting / in-review states.
const kWarning = Color(0xFF805500);
const kWarningTint = Color(0xFFFFF4DC);

/// Star ratings.
const kAmber = Color(0xFFF2A900);
