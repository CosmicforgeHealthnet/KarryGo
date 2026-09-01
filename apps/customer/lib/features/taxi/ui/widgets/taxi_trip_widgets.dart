import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../../hauling/ui/widgets/hauling_trip_widgets.dart' show formatTripDate;
import '../../models/taxi_models.dart';
import 'taxi_flow_helpers.dart';

/// One ride in the merged Trips list.
class TaxiTripCard extends StatelessWidget {
  const TaxiTripCard({super.key, required this.booking, required this.onTap});

  final TaxiBooking booking;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
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
                Row(
                  children: [
                    Expanded(
                      child: Text(
                        formatTripDate(booking.createdAt ?? DateTime.now()),
                        style: const TextStyle(
                          color: CustomerFigmaColors.text,
                          fontSize: 13,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                    ),
                    TaxiServiceBadge(isParcel: booking.isParcel),
                  ],
                ),
                const SizedBox(height: 14),
                TaxiRouteSummary(
                  pickupAddress: booking.pickup?.address ?? '',
                  dropoffAddress: booking.dropoff?.address ?? '',
                ),
                const SizedBox(height: 14),
                const Divider(height: 1, color: CustomerFigmaColors.border),
                const SizedBox(height: 12),
                Row(
                  mainAxisAlignment: MainAxisAlignment.spaceBetween,
                  children: [
                    Text(
                      booking.displayFareKobo > 0
                          ? formatHaulingFare(booking.displayFareKobo)
                          : '—',
                      style: const TextStyle(
                        color: CustomerFigmaColors.text,
                        fontSize: 16,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                    TaxiTripStatusPill(status: booking.status),
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

/// Small chip distinguishing car trips from bike deliveries and truck trips in
/// the merged Trips list — and, within car trips, a passenger ride from a
/// package sent by car. Without the second distinction the two are
/// indistinguishable in the list.
class TaxiServiceBadge extends StatelessWidget {
  const TaxiServiceBadge({super.key, this.isParcel = false});

  final bool isParcel;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: CustomerFigmaColors.primaryTint,
        borderRadius: BorderRadius.circular(20),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(
            isParcel ? Icons.inventory_2_rounded : Icons.local_taxi_rounded,
            size: 14,
            color: CustomerFigmaColors.primary,
          ),
          const SizedBox(width: 4),
          Text(
            isParcel ? 'Package' : 'Ride',
            style: const TextStyle(
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

class TaxiTripStatusPill extends StatelessWidget {
  const TaxiTripStatusPill({super.key, required this.status});

  final TaxiBookingStatus status;

  @override
  Widget build(BuildContext context) {
    final (bg, fg) = switch (status) {
      TaxiBookingStatus.completed => (
        CustomerFigmaColors.darkGreen,
        Colors.white,
      ),
      TaxiBookingStatus.cancelled ||
      TaxiBookingStatus.unmatched => (
        CustomerFigmaColors.successSoft,
        CustomerFigmaColors.success,
      ),
      TaxiBookingStatus.scheduled => (
        CustomerFigmaColors.success,
        Colors.white,
      ),
      _ => (CustomerFigmaColors.darkGreen, Colors.white),
    };

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 7),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(20),
      ),
      child: Text(
        status.tripChipLabel,
        style: TextStyle(
          color: fg,
          fontSize: 11.5,
          fontWeight: FontWeight.w700,
        ),
      ),
    );
  }
}
