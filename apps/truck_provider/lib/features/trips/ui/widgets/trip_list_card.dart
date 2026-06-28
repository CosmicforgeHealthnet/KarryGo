import 'package:flutter/material.dart';

import '../../../../core/format/money_format.dart';
import '../../../auth/models/provider_auth_models.dart';
import '../../../home/ui/widgets/provider_app_colors.dart';
import '../../../home/ui/widgets/provider_request_card.dart';

/// One trip row in the My Trips list (Figma "Completed" / "Ongoing" /
/// "Cancelled"). Tapping it opens the trip detail screen.
class TripListCard extends StatelessWidget {
  const TripListCard({super.key, required this.booking, required this.onTap});

  final ProviderBooking booking;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final distanceKm = booking.distanceKm;
    final estMinutes = distanceKm != null ? (distanceKm / 30 * 60).round() : null;

    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        margin: const EdgeInsets.only(bottom: 16),
        padding: const EdgeInsets.fromLTRB(16, 16, 16, 16),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(20),
          boxShadow: const [
            BoxShadow(color: Color(0x0F000000), blurRadius: 18, offset: Offset(0, 6)),
          ],
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // ─── Date + overflow menu ───────────────────────────────
            Row(
              children: [
                Text(
                  _formatDateTime(booking.createdAt),
                  style: const TextStyle(
                    color: kProviderText,
                    fontSize: 13,
                    fontWeight: FontWeight.w600,
                  ),
                ),
                const Spacer(),
                const Icon(Icons.more_vert_rounded, color: kProviderMuted, size: 20),
              ],
            ),
            const SizedBox(height: 12),

            // ─── Avatar + name + metrics ────────────────────────────
            Row(
              children: [
                ProviderAvatar(name: booking.displayName, size: 48),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        booking.displayName,
                        style: const TextStyle(
                          color: kProviderText,
                          fontWeight: FontWeight.w800,
                          fontSize: 18,
                        ),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                      ),
                      const SizedBox(height: 4),
                      BookingMetricsRow(
                        distanceKm: distanceKm,
                        minutes: estMinutes,
                        fareNaira: booking.fareEstimateNaira,
                      ),
                    ],
                  ),
                ),
              ],
            ),
            const SizedBox(height: 14),

            // ─── Route ──────────────────────────────────────────────
            BookingRoute(
              pickupAddress: booking.pickupAddress,
              dropoffAddress:
                  booking.dropoffAddress.isNotEmpty ? booking.dropoffAddress : '—',
            ),
            const SizedBox(height: 16),

            // ─── Fare + status badge ────────────────────────────────
            Row(
              children: [
                Text(
                  '₦${formatNaira(booking.fareEstimateNaira)}',
                  style: const TextStyle(
                    color: kProviderText,
                    fontWeight: FontWeight.w800,
                    fontSize: 15,
                  ),
                ),
                const Spacer(),
                TripStatusBadge(status: booking.status),
              ],
            ),
          ],
        ),
      ),
    );
  }

  String _formatDateTime(DateTime dt) {
    const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
    final hh = dt.hour.toString().padLeft(2, '0');
    final mm = dt.minute.toString().padLeft(2, '0');
    return '${dt.day.toString().padLeft(2, '0')} ${months[dt.month - 1]} ${dt.year}, $hh:$mm';
  }
}

/// Pill badge in the bottom-right of a trip card. Colour + label vary by status:
/// completed = dark, ongoing = dark/green split look, cancelled = pale green.
class TripStatusBadge extends StatelessWidget {
  const TripStatusBadge({super.key, required this.status});

  final String status;

  @override
  Widget build(BuildContext context) {
    final (label, bg, fg) = _style(status);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 9),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(20),
      ),
      child: Text(
        label,
        style: TextStyle(color: fg, fontSize: 13, fontWeight: FontWeight.w700),
      ),
    );
  }

  (String, Color, Color) _style(String status) {
    switch (status) {
      case 'completed':
      case 'delivered':
        return ('Completed', kProviderText, Colors.white);
      case 'cancelled':
      case 'unmatched':
        return ('Cancelled', kProviderGreenSoft, Colors.white);
      default:
        return ('Ongoing...', kProviderText, Colors.white);
    }
  }
}
