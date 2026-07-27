import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../../hauling/ui/widgets/hauling_trip_widgets.dart' show formatTripDate;
import '../../models/dispatch_models.dart';

/// "My Trips" card for a bike delivery: mirrors the hauling TripCard layout
/// with a "Bike" service badge and rider copy.
class DispatchTripCard extends StatelessWidget {
  const DispatchTripCard({
    super.key,
    required this.booking,
    required this.onTap,
    this.rider,
  });

  final DispatchBooking booking;
  final ProviderSnapshot? rider;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final dateLabel = formatTripDate(booking.createdAt);
    return Container(
      margin: const EdgeInsets.only(bottom: 16),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(18),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.06),
            blurRadius: 16,
            offset: const Offset(0, 6),
          ),
        ],
      ),
      child: Material(
        color: Colors.transparent,
        child: InkWell(
          onTap: onTap,
          borderRadius: BorderRadius.circular(18),
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                // Header: date + service badge
                Row(
                  children: [
                    Expanded(
                      child: Text(
                        dateLabel,
                        style: const TextStyle(
                          color: CustomerFigmaColors.text,
                          fontSize: 13,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                    ),
                    const DispatchServiceBadge(),
                  ],
                ),
                const SizedBox(height: 14),

                // Rider row
                _RiderRow(rider: rider),
                const SizedBox(height: 16),

                // Route
                _RouteBlock(booking: booking),
                const SizedBox(height: 14),
                const Divider(height: 1, color: CustomerFigmaColors.border),
                const SizedBox(height: 12),

                // Footer: fare + status pill
                Row(
                  mainAxisAlignment: MainAxisAlignment.spaceBetween,
                  children: [
                    Text(
                      booking.displayFareKobo > 0
                          ? '₦${booking.displayFareNaira.toStringAsFixed(2)}'
                          : '—',
                      style: const TextStyle(
                        color: CustomerFigmaColors.text,
                        fontSize: 16,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                    DispatchTripStatusPill(status: booking.status),
                  ],
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// Small green "Bike" chip distinguishing dispatch deliveries from truck trips
/// in the merged Trips list.
class DispatchServiceBadge extends StatelessWidget {
  const DispatchServiceBadge({super.key});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: CustomerFigmaColors.primaryTint,
        borderRadius: BorderRadius.circular(20),
      ),
      child: const Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.electric_bike_rounded,
              size: 14, color: CustomerFigmaColors.primary),
          SizedBox(width: 4),
          Text(
            'Bike',
            style: TextStyle(
              color: CustomerFigmaColors.primary,
              fontSize: 11,
              fontWeight: FontWeight.w800,
            ),
          ),
        ],
      ),
    );
  }
}

class _RiderRow extends StatelessWidget {
  const _RiderRow({this.rider});

  final ProviderSnapshot? rider;

  @override
  Widget build(BuildContext context) {
    final name = rider?.displayName.trim();
    final photo = rider?.profilePhotoUrl;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        CircleAvatar(
          radius: 22,
          backgroundColor: CustomerFigmaColors.primaryPale,
          backgroundImage:
              (photo != null && photo.isNotEmpty) ? NetworkImage(photo) : null,
          child: (photo == null || photo.isEmpty)
              ? const Icon(Icons.person,
                  color: CustomerFigmaColors.primary, size: 24)
              : null,
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                (name == null || name.isEmpty) ? 'Dispatch Rider' : name,
                style: const TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 15,
                  fontWeight: FontWeight.w800,
                ),
              ),
              const SizedBox(height: 3),
              Row(
                children: [
                  const Text(
                    'Riding with KarryGo  ',
                    style: TextStyle(
                        color: CustomerFigmaColors.muted, fontSize: 12),
                  ),
                  Text(
                    '• ${_tripsLabel(rider?.totalTrips)} deliveries',
                    style: const TextStyle(
                      color: CustomerFigmaColors.primary,
                      fontSize: 12,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ],
              ),
            ],
          ),
        ),
      ],
    );
  }

  String _tripsLabel(int? total) {
    if (total == null || total <= 0) return '0';
    if (total >= 1000) return '1,000+';
    return '$total';
  }
}

class _RouteBlock extends StatelessWidget {
  const _RouteBlock({required this.booking});

  final DispatchBooking booking;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Column(
          children: [
            const SizedBox(height: 2),
            _ring(filled: false),
            SizedBox(
              height: 30,
              child: CustomPaint(
                size: const Size(2, 30),
                painter: _DottedConnectorPainter(),
              ),
            ),
            _ring(filled: true),
          ],
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              _routeText(
                'Pick-up',
                booking.pickupAddress.isEmpty
                    ? 'Pickup location'
                    : booking.pickupAddress,
              ),
              const SizedBox(height: 18),
              _routeText('Drop off', booking.dropoffAddress),
            ],
          ),
        ),
      ],
    );
  }

  Widget _ring({required bool filled}) {
    return Container(
      width: 16,
      height: 16,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: filled ? CustomerFigmaColors.primary : Colors.transparent,
        border: Border.all(color: CustomerFigmaColors.primary, width: 2.5),
      ),
      child: filled
          ? const Center(
              child: CircleAvatar(radius: 2.5, backgroundColor: Colors.white))
          : null,
    );
  }

  Widget _routeText(String label, String address) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 12),
        ),
        const SizedBox(height: 2),
        Text(
          address,
          style: const TextStyle(
            color: CustomerFigmaColors.text,
            fontSize: 14,
            fontWeight: FontWeight.w700,
          ),
          maxLines: 2,
          overflow: TextOverflow.ellipsis,
        ),
      ],
    );
  }
}

class _DottedConnectorPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = CustomerFigmaColors.primary
      ..strokeWidth = 2
      ..strokeCap = StrokeCap.round;
    const dashHeight = 3.0;
    const gap = 3.0;
    double y = 0;
    final x = size.width / 2;
    while (y < size.height) {
      canvas.drawLine(Offset(x, y), Offset(x, y + dashHeight), paint);
      y += dashHeight + gap;
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}

/// Figma status pill used on the dispatch trip card footer.
class DispatchTripStatusPill extends StatelessWidget {
  const DispatchTripStatusPill({super.key, required this.status});

  final DispatchBookingStatus status;

  @override
  Widget build(BuildContext context) {
    final label = status.tripChipLabel;
    late final Color bg;
    if (status == DispatchBookingStatus.completed ||
        status == DispatchBookingStatus.delivered ||
        status.isActive) {
      bg = CustomerFigmaColors.text; // dark pill
    } else if (status == DispatchBookingStatus.cancelled ||
        status == DispatchBookingStatus.unmatched) {
      bg = CustomerFigmaColors.primarySoft;
    } else {
      bg = CustomerFigmaColors.primary; // searching / upcoming
    }
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 9),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(22),
      ),
      child: Text(
        label,
        style: const TextStyle(
          color: Colors.white,
          fontSize: 12.5,
          fontWeight: FontWeight.w700,
        ),
      ),
    );
  }
}
