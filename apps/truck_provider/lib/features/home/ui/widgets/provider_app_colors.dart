import 'package:flutter/material.dart';

const kProviderGreen = Color(0xFF22A84A);
const kProviderGreenTint = Color(0xFFEAF8EE);
const kProviderGreenPale = Color(0xFFD7EEDB);
const kProviderGreenSoft = Color(0xFFB8E0C2);
const kProviderDarkGreen = Color(0xFF2F5135);
const kProviderText = Color(0xFF121A14);
const kProviderMuted = Color(0xFF7B827C);
const kProviderBorder = Color(0xFFE5E9E5);
const kProviderSurface = Color(0xFFF7F8F7);

// Reject / decline button colors (light pink bg, red text)
const kProviderRejectBg = Color(0xFFFCE6E6);
const kProviderRejectText = Color(0xFFE5484D);

// Balance card gradient — bright green (top-left) to deep green (bottom-right).
const kProviderBalanceGradient = LinearGradient(
  begin: Alignment.topLeft,
  end: Alignment.bottomRight,
  colors: [Color(0xFF35B45A), Color(0xFF0A5626)],
);

// Page background behind the white cards so they read as elevated cards.
const kProviderPageBg = Color(0xFFF4F6F8);
