import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/hauling_models.dart';

/// Status → accent color used by the Trips list and the trip detail screen.
Color tripStatusColor(HaulingBookingStatus status) {
  if (status == HaulingBookingStatus.completed) return CustomerFigmaColors.primary;
  if (status == HaulingBookingStatus.cancelled ||
      status == HaulingBookingStatus.unmatched) {
    return const Color(0xFFD7493B);
  }
  if (status.isSearching) return const Color(0xFFE08A00);
  // active states
  return const Color(0xFF2E7DD1);
}

/// Pill showing the booking status with its accent color.
class TripStatusChip extends StatelessWidget {
  const TripStatusChip({super.key, required this.status});

  final HaulingBookingStatus status;

  @override
  Widget build(BuildContext context) {
    final color = tripStatusColor(status);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(20),
      ),
      child: Text(
        status.displayLabel,
        style: TextStyle(
          color: color,
          fontSize: 11,
          fontWeight: FontWeight.w700,
        ),
      ),
    );
  }
}

/// Figma "My Trips" card: header date, driver row (avatar + name + tenure +
/// trip count), pickup → dropoff route with a dotted connector, and a fare +
/// status pill footer. [provider] is optional and fills the driver row once the
/// snapshot has loaded.
class TripCard extends StatelessWidget {
  const TripCard({
    super.key,
    required this.booking,
    required this.onTap,
    this.provider,
  });

  final HaulageBooking booking;
  final ProviderSnapshot? provider;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final dateLabel = formatTripDate(booking.scheduledAt ?? booking.createdAt);
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
                // Header: date + 3-dot
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
                    const Icon(Icons.more_vert,
                        color: CustomerFigmaColors.muted, size: 20),
                  ],
                ),
                const SizedBox(height: 14),

                // Driver row
                _DriverRow(provider: provider),
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
                    TripStatusPill(status: booking.status),
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

class _DriverRow extends StatelessWidget {
  const _DriverRow({this.provider});

  final ProviderSnapshot? provider;

  @override
  Widget build(BuildContext context) {
    final name = provider?.displayName.trim();
    final photo = provider?.profilePhotoUrl;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        CircleAvatar(
          radius: 22,
          backgroundColor: CustomerFigmaColors.primaryPale,
          backgroundImage: (photo != null && photo.isNotEmpty)
              ? NetworkImage(photo)
              : null,
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
                (name == null || name.isEmpty) ? 'Driver' : name,
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
                    'Driving with KarryGo  ',
                    style: TextStyle(
                        color: CustomerFigmaColors.muted, fontSize: 12),
                  ),
                  Text(
                    '• 1 year',
                    style: const TextStyle(
                      color: CustomerFigmaColors.primary,
                      fontSize: 12,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 2),
              Row(
                children: [
                  const Text(
                    'Completed Trips  ',
                    style: TextStyle(
                        color: CustomerFigmaColors.muted, fontSize: 12),
                  ),
                  Text(
                    '• ${_tripsLabel(provider?.totalTrips)}',
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

  final HaulageBooking booking;

  @override
  Widget build(BuildContext context) {
    final hasDropoff = booking.dropoffAddress.isNotEmpty;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Indicator column with dotted connector
        Column(
          children: [
            const SizedBox(height: 2),
            _ring(filled: false),
            if (hasDropoff) ...[
              SizedBox(
                height: 30,
                child: CustomPaint(
                  size: const Size(2, 30),
                  painter: _DottedConnectorPainter(),
                ),
              ),
              _ring(filled: true),
            ],
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
              if (hasDropoff) ...[
                const SizedBox(height: 18),
                _routeText('Drop off (optional)', booking.dropoffAddress),
              ],
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

/// Figma status pill used on the trip card footer. Completed = dark, Upcoming /
/// Ongoing = green, Cancelled = muted green.
class TripStatusPill extends StatelessWidget {
  const TripStatusPill({super.key, required this.status});

  final HaulingBookingStatus status;

  @override
  Widget build(BuildContext context) {
    final label = status.tripChipLabel;
    late final Color bg;
    late final Color fg;
    if (status == HaulingBookingStatus.completed) {
      bg = CustomerFigmaColors.text; // dark pill
      fg = Colors.white;
    } else if (status == HaulingBookingStatus.cancelled ||
        status == HaulingBookingStatus.unmatched) {
      bg = CustomerFigmaColors.primarySoft;
      fg = Colors.white;
    } else if (status == HaulingBookingStatus.delivered || status.isActive) {
      // Ongoing — split dark/green look approximated with a dark pill.
      bg = CustomerFigmaColors.text;
      fg = Colors.white;
    } else {
      bg = CustomerFigmaColors.primary; // upcoming / searching
      fg = Colors.white;
    }
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 9),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(22),
      ),
      child: Text(
        label,
        style: TextStyle(
          color: fg,
          fontSize: 12.5,
          fontWeight: FontWeight.w700,
        ),
      ),
    );
  }
}

/// Lightweight date formatter (the app has no `intl` dependency).
/// e.g. "26 Jun 2026, 3:05 PM".
String formatTripDate(DateTime dt) {
  final d = dt.toLocal();
  const months = [
    'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
    'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
  ];
  final hour = d.hour % 12 == 0 ? 12 : d.hour % 12;
  final minute = d.minute.toString().padLeft(2, '0');
  final ampm = d.hour < 12 ? 'AM' : 'PM';
  return '${d.day} ${months[d.month - 1]} ${d.year}, $hour:$minute $ampm';
}
