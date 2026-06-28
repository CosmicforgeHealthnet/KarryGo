import 'package:flutter/material.dart';

import '../../../home/ui/widgets/provider_app_colors.dart';

/// Shared building blocks for the Truck Information screens (list + detail).

/// Formats a capacity in kilograms with thousands separators, e.g. `3,000 kg`.
/// The app has no `intl` dependency, so this is a small local formatter.
String formatTruckCapacity(int kg) {
  final digits = kg.abs().toString();
  final buf = StringBuffer();
  for (var i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 == 0) buf.write(',');
    buf.write(digits[i]);
  }
  return '${kg < 0 ? '-' : ''}$buf kg';
}

/// Maps a human colour name to a swatch colour for the little colour dot.
/// Falls back to a neutral grey for unknown names.
Color truckColorSwatch(String name) {
  switch (name.trim().toLowerCase()) {
    case 'white':
      return const Color(0xFFE9EBEC);
    case 'black':
      return const Color(0xFF24292B);
    case 'silver':
    case 'grey':
    case 'gray':
      return const Color(0xFFB8BEC4);
    case 'blue':
      return const Color(0xFF2F6FED);
    case 'navy':
      return const Color(0xFF1B2A6B);
    case 'red':
      return const Color(0xFFE5484D);
    case 'green':
      return kProviderGreen;
    case 'yellow':
      return const Color(0xFFF5C518);
    case 'orange':
      return const Color(0xFFF2761B);
    case 'brown':
      return const Color(0xFF8B5E3C);
    default:
      return kProviderMuted;
  }
}

/// Rounded gradient tile holding a white truck glyph — the card/hero leading icon.
class TruckGlyphTile extends StatelessWidget {
  const TruckGlyphTile({super.key, this.size = 52, this.iconSize = 26});

  final double size;
  final double iconSize;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        gradient: kProviderBalanceGradient,
        borderRadius: BorderRadius.circular(size * 0.32),
        boxShadow: [
          BoxShadow(
            color: kProviderGreen.withValues(alpha: 0.30),
            blurRadius: 12,
            offset: const Offset(0, 6),
          ),
        ],
      ),
      child: Icon(Icons.local_shipping_rounded, color: Colors.white, size: iconSize),
    );
  }
}

/// Small "Active"/"Inactive" status pill. [onDark] renders for a dark hero card.
class TruckStatusPill extends StatelessWidget {
  const TruckStatusPill({super.key, required this.active, this.onDark = false});

  final bool active;
  final bool onDark;

  @override
  Widget build(BuildContext context) {
    final dotColor = onDark ? Colors.white : (active ? kProviderGreen : kProviderMuted);
    final bg = onDark
        ? Colors.white.withValues(alpha: 0.18)
        : (active ? kProviderGreenTint : kProviderSurface);
    final textColor = onDark ? Colors.white : (active ? kProviderGreen : kProviderMuted);

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
      decoration: BoxDecoration(color: bg, borderRadius: BorderRadius.circular(999)),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(width: 7, height: 7, decoration: BoxDecoration(color: dotColor, shape: BoxShape.circle)),
          const SizedBox(width: 6),
          Text(
            active ? 'Active' : 'Inactive',
            style: TextStyle(color: textColor, fontSize: 12, fontWeight: FontWeight.w700),
          ),
        ],
      ),
    );
  }
}

/// Compact info chip used on the truck card: a leading icon (or colour swatch)
/// plus a short label, on a soft surface background.
class TruckInfoChip extends StatelessWidget {
  const TruckInfoChip({super.key, required this.label, this.icon, this.swatch});

  final String label;
  final IconData? icon;
  final Color? swatch;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 7),
      decoration: BoxDecoration(
        color: kProviderSurface,
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: kProviderBorder),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (swatch != null) ...[
            Container(
              width: 12,
              height: 12,
              decoration: BoxDecoration(
                color: swatch,
                shape: BoxShape.circle,
                border: Border.all(color: kProviderBorder),
              ),
            ),
            const SizedBox(width: 6),
          ] else if (icon != null) ...[
            Icon(icon, size: 14, color: kProviderMuted),
            const SizedBox(width: 6),
          ],
          Text(label, style: const TextStyle(color: kProviderText, fontSize: 12.5, fontWeight: FontWeight.w600)),
        ],
      ),
    );
  }
}
