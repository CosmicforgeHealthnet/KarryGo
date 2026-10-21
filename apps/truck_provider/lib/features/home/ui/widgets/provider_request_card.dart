import 'package:flutter/material.dart';

import '../../../../core/format/money_format.dart';
import '../../../../core/widgets/karryam_ui.dart';
import '../../../auth/models/provider_auth_models.dart';
import 'provider_app_colors.dart';

/// A haulage request as a card: fare up top, the route, the load, then
/// Decline / Accept. Used on the home sheet and the Requests tab.
class ProviderRequestCard extends StatelessWidget {
  const ProviderRequestCard({
    super.key,
    required this.booking,
    required this.onReject,
    required this.onAccept,
    this.isLoading = false,
    this.onOpen,
  });

  final ProviderBooking booking;
  final VoidCallback onReject;
  final VoidCallback onAccept;
  final bool isLoading;
  final VoidCallback? onOpen;

  @override
  Widget build(BuildContext context) {
    return KCard(
      onTap: onOpen,
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Row(
            children: [
              const KStatusChip('New request'),
              const Spacer(),
              Text(
                '#${booking.shortId}',
                style: const TextStyle(fontSize: 12, color: kProviderMuted, fontWeight: FontWeight.w500),
              ),
            ],
          ),
          const SizedBox(height: 12),
          Row(
            crossAxisAlignment: CrossAxisAlignment.end,
            children: [
              Expanded(
                child: FittedBox(
                  fit: BoxFit.scaleDown,
                  alignment: Alignment.centerLeft,
                  child: Text(
                    formatNairaShort(booking.fareNaira),
                    style: const TextStyle(
                      fontSize: 28,
                      fontWeight: FontWeight.w700,
                      letterSpacing: -0.8,
                      color: kProviderText,
                    ),
                  ),
                ),
              ),
              BookingMetricsRow(
                distanceKm: booking.distanceKm,
                minutes: booking.estimatedMinutes,
                fareNaira: null,
              ),
            ],
          ),
          const SizedBox(height: 14),
          BookingRoute(pickupAddress: booking.pickupAddress, dropoffAddress: booking.dropoffAddress),
          const SizedBox(height: 14),
          BookingCargoRow(booking: booking),
          const SizedBox(height: 14),
          Row(
            children: [
              Expanded(child: RejectRequestButton(onPressed: isLoading ? null : onReject)),
              const SizedBox(width: 10),
              Expanded(child: AcceptRequestButton(onPressed: isLoading ? null : onAccept)),
            ],
          ),
        ],
      ),
    );
  }
}

// ─── Shared building blocks (reused on detail, trip and history screens) ─────

/// Circular avatar: photo when available, otherwise initials on teal tint.
class ProviderAvatar extends StatelessWidget {
  const ProviderAvatar({super.key, required this.name, this.size = 48, this.photoUrl});

  final String name;
  final double size;
  final String? photoUrl;

  @override
  Widget build(BuildContext context) => KAvatar(name: name, photoUrl: photoUrl, size: size);
}

/// "24 km · 48 min · ₦32,500" — compact trip metrics. Pass a null fare to
/// leave it out (when the fare is already the headline).
class BookingMetricsRow extends StatelessWidget {
  const BookingMetricsRow({
    super.key,
    required this.distanceKm,
    required this.minutes,
    required this.fareNaira,
  });

  final double? distanceKm;
  final int? minutes;
  final double? fareNaira;

  @override
  Widget build(BuildContext context) {
    final parts = <Widget>[
      if (distanceKm != null) _metric(Icons.route_rounded, '${distanceKm!.toStringAsFixed(distanceKm! < 10 ? 1 : 0)} km'),
      if (minutes != null) _metric(Icons.schedule_rounded, '$minutes min'),
      if (fareNaira != null) _metric(Icons.payments_outlined, formatNairaShort(fareNaira!), strong: true),
    ];
    return Wrap(spacing: 12, runSpacing: 4, children: parts);
  }

  Widget _metric(IconData icon, String label, {bool strong = false}) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(icon, size: 14, color: kProviderMuted),
        const SizedBox(width: 4),
        Text(
          label,
          style: TextStyle(
            color: strong ? kTealText : kProviderMuted,
            fontSize: 12.5,
            fontWeight: strong ? FontWeight.w700 : FontWeight.w500,
          ),
        ),
      ],
    );
  }
}

/// Pickup → delivery stops.
class BookingRoute extends StatelessWidget {
  const BookingRoute({super.key, required this.pickupAddress, required this.dropoffAddress, this.full = false});

  final String pickupAddress;
  final String dropoffAddress;

  /// Show addresses in full instead of two lines each.
  final bool full;

  @override
  Widget build(BuildContext context) =>
      KRouteStops(pickup: pickupAddress, dropoff: dropoffAddress, maxLines: full ? null : 2);
}

/// Tinted tile with what is being moved and its weight / truck / handling.
class BookingCargoRow extends StatelessWidget {
  const BookingCargoRow({super.key, required this.booking});
  final ProviderBooking booking;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(color: kCanvas, borderRadius: BorderRadius.circular(14)),
      child: Row(
        children: [
          const KIconTile(Icons.inventory_2_outlined, size: 38),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  booking.cargoTitle,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(fontSize: 13.5, fontWeight: FontWeight.w600, color: kProviderText),
                ),
                const SizedBox(height: 2),
                Text(
                  booking.cargoSummary,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(fontSize: 12, color: kProviderMuted),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// White outlined "Decline" button.
class RejectRequestButton extends StatelessWidget {
  const RejectRequestButton({super.key, required this.onPressed, this.label = 'Decline'});

  final VoidCallback? onPressed;
  final String label;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 48,
      child: OutlinedButton(
        onPressed: onPressed,
        style: OutlinedButton.styleFrom(
          minimumSize: const Size(0, 48),
          foregroundColor: kProviderRejectText,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
          textStyle: const TextStyle(fontFamily: 'Inter', fontWeight: FontWeight.w600, fontSize: 14),
        ),
        child: Text(label),
      ),
    );
  }
}

/// Navy "Accept" button.
class AcceptRequestButton extends StatelessWidget {
  const AcceptRequestButton({super.key, required this.onPressed, this.label = 'Accept'});

  final VoidCallback? onPressed;
  final String label;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 48,
      child: FilledButton(
        onPressed: onPressed,
        style: FilledButton.styleFrom(
          minimumSize: const Size(0, 48),
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
          textStyle: const TextStyle(fontFamily: 'Inter', fontWeight: FontWeight.w700, fontSize: 14),
        ),
        child: Text(label),
      ),
    );
  }
}
