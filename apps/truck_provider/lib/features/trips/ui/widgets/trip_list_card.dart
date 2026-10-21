import 'package:flutter/material.dart';

import '../../../../core/format/date_format.dart';
import '../../../../core/format/money_format.dart';
import '../../../../core/widgets/karryam_ui.dart';
import '../../../auth/models/provider_auth_models.dart';
import '../../../home/ui/widgets/provider_app_colors.dart';
import '../../../home/ui/widgets/provider_request_card.dart';

/// One trip in the My Trips list. Tapping it opens the trip detail screen.
class TripListCard extends StatelessWidget {
  const TripListCard({super.key, required this.booking, required this.onTap});

  final ProviderBooking booking;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: KCard(
        onTap: onTap,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    formatDateTime(booking.createdAt),
                    style: TextStyle(fontSize: 12.5, color: kProviderMuted, fontWeight: FontWeight.w500),
                  ),
                ),
                TripStatusBadge(status: booking.status),
              ],
            ),
            const SizedBox(height: 12),
            BookingRoute(
              pickupAddress: booking.pickupAddress,
              dropoffAddress: booking.dropoffAddress.isNotEmpty ? booking.dropoffAddress : '—',
            ),
            const SizedBox(height: 14),
            const Divider(),
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      if (booking.receiverLine.isNotEmpty)
                        Text(
                          booking.receiverLine,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: TextStyle(fontSize: 13, fontWeight: FontWeight.w600, color: kProviderText),
                        ),
                      const SizedBox(height: 2),
                      BookingMetricsRow(
                        distanceKm: booking.distanceKm,
                        minutes: booking.estimatedMinutes,
                        fareNaira: null,
                      ),
                    ],
                  ),
                ),
                Text(
                  formatNairaShort(booking.fareNaira),
                  style: TextStyle(fontSize: 17, fontWeight: FontWeight.w700, color: kProviderText),
                ),
                const SizedBox(width: 4),
                Icon(Icons.chevron_right_rounded, color: kProviderMuted),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

/// Status pill on a trip card.
class TripStatusBadge extends StatelessWidget {
  const TripStatusBadge({super.key, required this.status});

  final String status;

  @override
  Widget build(BuildContext context) {
    return switch (status) {
      'completed' || 'delivered' => const KStatusChip('Completed'),
      'cancelled' || 'unmatched' => const KStatusChip('Cancelled', tone: KTone.danger),
      _ => const KStatusChip('Ongoing', tone: KTone.info),
    };
  }
}
