import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../../home/ui/widgets/customer_home_bottom_nav.dart';
import '../../models/hauling_models.dart';

/// Shared building blocks for the live trip bottom sheets (Connecting Driver /
/// Driver Assigned / Trip has started / You have arrived), matching the Figma
/// mockups 1736, 1766, 1772-1776.

const kHaulingStarAmber = Color(0xFFF2B01E);
const kHaulingTripsGold = Color(0xFFE9A13B);
const kHaulingCircleGrey = Color(0xFFF2F3F2);

/// "₦7.00"-style fare: thousands separators + 2 decimals.
String formatHaulingFare(int kobo) {
  final naira = kobo / 100;
  final whole = naira.truncate();
  final cents = ((naira - whole) * 100).round().toString().padLeft(2, '0');
  final digits = whole.toString();
  final buf = StringBuffer();
  for (var i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 == 0) buf.write(',');
    buf.write(digits[i]);
  }
  return '₦$buf.$cents';
}

/// Rough city-driving ETA for a distance (used for the "8 min" stat).
int haulingEtaMinutes(double distanceKm) {
  final mins = (distanceKm / 30.0 * 60).round();
  return mins < 1 ? 1 : mins;
}

double haulingHaversineKm(double lat1, double lng1, double lat2, double lng2) {
  const r = 6371.0;
  final dLat = (lat2 - lat1) * math.pi / 180;
  final dLng = (lng2 - lng1) * math.pi / 180;
  final a = math.sin(dLat / 2) * math.sin(dLat / 2) +
      math.cos(lat1 * math.pi / 180) *
          math.cos(lat2 * math.pi / 180) *
          math.sin(dLng / 2) *
          math.sin(dLng / 2);
  return 2 * r * math.asin(math.min(1, math.sqrt(a)));
}

// ─── Discount banner (floating card over the map) ────────────────────────────

class HaulingDiscountBanner extends StatelessWidget {
  const HaulingDiscountBanner({super.key});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(14),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.08),
            blurRadius: 14,
            offset: const Offset(0, 4),
          ),
        ],
      ),
      child: const Row(
        children: [
          Icon(Icons.gpp_good, color: CustomerFigmaColors.primary, size: 32),
          SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  '10% off first booking',
                  style: TextStyle(
                    color: CustomerFigmaColors.text,
                    fontSize: 15,
                    fontWeight: FontWeight.w800,
                  ),
                ),
                SizedBox(height: 1),
                Text(
                  'View Details',
                  style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 12),
                ),
              ],
            ),
          ),
          Icon(Icons.arrow_forward, color: Color(0xFFD4D7D4), size: 20),
        ],
      ),
    );
  }
}

// ─── Driver profile block ─────────────────────────────────────────────────────

/// Avatar + name + "Driving with KarryGo . 1 year" + "Completed Trips . 1,000+"
/// + star row, as on Figma 1736/1772/1773.
class HaulingDriverProfileBlock extends StatelessWidget {
  const HaulingDriverProfileBlock({super.key, required this.provider});

  final ProviderSnapshot provider;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        HaulingDriverAvatar(provider: provider, radius: 32),
        const SizedBox(width: 14),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                provider.displayName,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 23,
                  fontWeight: FontWeight.w800,
                ),
              ),
              const SizedBox(height: 4),
              _InfoLine(
                label: 'Driving with KarryGo . ',
                value: provider.tenureLabel,
                valueColor: CustomerFigmaColors.primary,
              ),
              const SizedBox(height: 2),
              _InfoLine(
                label: 'Completed Trips . ',
                value: provider.tripsLabel,
                valueColor: kHaulingTripsGold,
              ),
              const SizedBox(height: 6),
              HaulingStarRating(rating: provider.rating),
            ],
          ),
        ),
      ],
    );
  }
}

class HaulingDriverAvatar extends StatelessWidget {
  const HaulingDriverAvatar({super.key, required this.provider, this.radius = 32});

  final ProviderSnapshot provider;
  final double radius;

  @override
  Widget build(BuildContext context) {
    final photo = provider.profilePhotoUrl;
    final initials = [
      if (provider.firstName.isNotEmpty) provider.firstName[0],
      if (provider.lastName.isNotEmpty) provider.lastName[0],
    ].join().toUpperCase();
    return CircleAvatar(
      radius: radius,
      backgroundColor: CustomerFigmaColors.primaryTint,
      backgroundImage:
          (photo != null && photo.isNotEmpty) ? NetworkImage(photo) : null,
      child: (photo == null || photo.isEmpty)
          ? Text(
              initials.isEmpty ? '?' : initials,
              style: TextStyle(
                color: CustomerFigmaColors.darkGreen,
                fontWeight: FontWeight.w800,
                fontSize: radius * 0.55,
              ),
            )
          : null,
    );
  }
}

class _InfoLine extends StatelessWidget {
  const _InfoLine({
    required this.label,
    required this.value,
    required this.valueColor,
  });

  final String label;
  final String value;
  final Color valueColor;

  @override
  Widget build(BuildContext context) {
    return Text.rich(
      TextSpan(
        text: label,
        style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
        children: [
          TextSpan(
            text: value,
            style: TextStyle(
              color: valueColor,
              fontSize: 13,
              fontWeight: FontWeight.w800,
            ),
          ),
        ],
      ),
      maxLines: 1,
      overflow: TextOverflow.ellipsis,
    );
  }
}

class HaulingStarRating extends StatelessWidget {
  const HaulingStarRating({super.key, required this.rating});

  final double rating;

  @override
  Widget build(BuildContext context) {
    final filled = rating.round().clamp(0, 5);
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        for (var i = 0; i < 5; i++)
          Padding(
            padding: const EdgeInsets.only(right: 2),
            child: Icon(
              Icons.star_rounded,
              size: 20,
              color: i < filled ? kHaulingStarAmber : const Color(0xFFD9D9D9),
            ),
          ),
        const SizedBox(width: 4),
        Text(
          '${rating.toStringAsFixed(1)} Rating',
          style: const TextStyle(
            color: CustomerFigmaColors.text,
            fontSize: 13,
            fontWeight: FontWeight.w700,
          ),
        ),
      ],
    );
  }
}

// ─── Stats row (21 km / 8 min / ₦7.00) ───────────────────────────────────────

class HaulingTripStatsRow extends StatelessWidget {
  const HaulingTripStatsRow({super.key, this.distanceKm, this.fareKobo});

  final double? distanceKm;
  final int? fareKobo;

  @override
  Widget build(BuildContext context) {
    final km = distanceKm;
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        _StatItem(
          icon: Icons.av_timer,
          label: km != null ? '${km.toStringAsFixed(0)} km' : '-- km',
        ),
        _StatItem(
          icon: Icons.access_time,
          label: km != null ? '${haulingEtaMinutes(km)} min' : '-- min',
        ),
        _StatItem(
          icon: Icons.monetization_on_outlined,
          label: fareKobo != null ? formatHaulingFare(fareKobo!) : '₦--',
        ),
      ],
    );
  }
}

class _StatItem extends StatelessWidget {
  const _StatItem({required this.icon, required this.label});

  final IconData icon;
  final String label;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(icon, size: 18, color: CustomerFigmaColors.text),
        const SizedBox(width: 6),
        Text(
          label,
          style: const TextStyle(
            color: CustomerFigmaColors.text,
            fontSize: 13,
            fontWeight: FontWeight.w800,
          ),
        ),
      ],
    );
  }
}

// ─── Vehicle line ("Blue • MAN TGX  EKY232GY") ───────────────────────────────

class HaulingVehicleLine extends StatelessWidget {
  const HaulingVehicleLine({
    super.key,
    required this.truck,
    this.centered = false,
  });

  final TruckSnapshot truck;
  final bool centered;

  @override
  Widget build(BuildContext context) {
    final desc = [
      if (truck.color.isNotEmpty) truck.color,
      [truck.make, truck.model].where((s) => s.isNotEmpty).join(' '),
    ].where((s) => s.isNotEmpty).join(' • ');
    return Text.rich(
      TextSpan(
        text: desc,
        style: const TextStyle(
          color: CustomerFigmaColors.text,
          fontSize: 13,
          fontWeight: FontWeight.w800,
        ),
        children: [
          if (truck.plateNumber.isNotEmpty)
            TextSpan(
              text: '  ${truck.plateNumber.toUpperCase()}',
              style: const TextStyle(
                color: CustomerFigmaColors.primary,
                fontSize: 13,
                fontWeight: FontWeight.w800,
              ),
            ),
        ],
      ),
      textAlign: centered ? TextAlign.center : TextAlign.start,
      maxLines: 1,
      overflow: TextOverflow.ellipsis,
    );
  }
}

// ─── Trip progress slider (Pickup ────────── Destination) ────────────────────

class HaulingTripProgressSlider extends StatelessWidget {
  const HaulingTripProgressSlider({
    super.key,
    required this.progress,
    this.showTruck = false,
  });

  /// 0..1 position between pickup and destination.
  final double progress;

  /// Draws the little truck at [progress] (Figma 1773).
  final bool showTruck;

  @override
  Widget build(BuildContext context) {
    const animDuration = Duration(milliseconds: 600);
    const animCurve = Curves.easeOut;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        SizedBox(
          height: 34,
          child: LayoutBuilder(
            builder: (context, constraints) {
              final width = constraints.maxWidth;
              final clamped = progress.clamp(0.0, 1.0);
              // Fill goes from left:7 to right:7. Total track usable width = width-14.
              // Truck (34px wide) is centered on the fill's leading edge.
              final fillWidth = (width - 14) * clamped;
              final truckLeft = (7 + fillWidth - 17).clamp(0.0, width - 34);
              return Stack(
                clipBehavior: Clip.none,
                children: [
                  // Track (full)
                  Positioned(
                    left: 7,
                    right: 7,
                    top: 18,
                    child: Container(height: 3, color: const Color(0xFFE3E6E3)),
                  ),
                  // Progress fill (animated)
                  AnimatedPositioned(
                    duration: animDuration,
                    curve: animCurve,
                    left: 7,
                    top: 18,
                    child: AnimatedContainer(
                      duration: animDuration,
                      curve: animCurve,
                      height: 3,
                      width: fillWidth,
                      color: CustomerFigmaColors.primary,
                    ),
                  ),
                  // Left endpoint: green donut
                  Positioned(
                    left: 0,
                    top: 11,
                    child: Container(
                      width: 16,
                      height: 16,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        color: Colors.white,
                        border: Border.all(
                          color: CustomerFigmaColors.primary,
                          width: 5,
                        ),
                      ),
                    ),
                  ),
                  // Right endpoint: pale green dot
                  Positioned(
                    right: 0,
                    top: 12,
                    child: Container(
                      width: 14,
                      height: 14,
                      decoration: const BoxDecoration(
                        shape: BoxShape.circle,
                        color: CustomerFigmaColors.primarySoft,
                      ),
                    ),
                  ),
                  // Truck rides the fill's leading edge (animated)
                  if (showTruck)
                    AnimatedPositioned(
                      duration: animDuration,
                      curve: animCurve,
                      left: truckLeft,
                      top: 0,
                      child: Image.asset(
                        'assets/figma/delivery truck back side view.png',
                        width: 34,
                        height: 26,
                        fit: BoxFit.contain,
                      ),
                    ),
                ],
              );
            },
          ),
        ),
        const SizedBox(height: 4),
        const Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [
            Text(
              'Pickup',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
            ),
            Text(
              'Destination',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
            ),
          ],
        ),
      ],
    );
  }
}

// ─── Big pill buttons ─────────────────────────────────────────────────────────

/// Solid green pill ("Cancel Truck" / "Call Driver" / "Submit Review").
class HaulingPillButton extends StatelessWidget {
  const HaulingPillButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.isLoading = false,
    this.background = CustomerFigmaColors.primary,
    this.foreground = Colors.white,
    this.height = 54,
  });

  final String label;
  final VoidCallback? onPressed;
  final bool isLoading;
  final Color background;
  final Color foreground;
  final double height;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: height,
      child: FilledButton(
        onPressed: (onPressed != null && !isLoading) ? onPressed : null,
        style: FilledButton.styleFrom(
          backgroundColor: background,
          foregroundColor: foreground,
          disabledBackgroundColor: CustomerFigmaColors.primarySoft,
          disabledForegroundColor: Colors.white,
          textStyle: const TextStyle(fontSize: 16, fontWeight: FontWeight.w800),
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
          elevation: 0,
        ),
        child: isLoading
            ? SizedBox.square(
                dimension: 20,
                child: CircularProgressIndicator(
                  strokeWidth: 2,
                  valueColor: AlwaysStoppedAnimation<Color>(foreground),
                ),
              )
            : Text(label),
      ),
    );
  }
}

// ─── Bottom nav (Home selected), shown under the trip sheets ─────────────────

/// The mockups show the app bottom nav under every live-trip sheet. The flow is
/// a pushed full-screen route, so tapping any item leaves the flow back to the
/// home shell — the booking keeps running and stays reachable from Trips.
class HaulingFlowBottomNav extends StatelessWidget {
  const HaulingFlowBottomNav({super.key});

  @override
  Widget build(BuildContext context) {
    return CustomerHomeBottomNav(
      selectedIndex: 0,
      onTap: (_) => Navigator.of(context).popUntil((r) => r.isFirst),
    );
  }
}
