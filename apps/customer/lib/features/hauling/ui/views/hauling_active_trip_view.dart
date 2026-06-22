import 'package:flutter/material.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/hauling_models.dart';
import '../../state/hauling_booking_controller.dart';
import '../widgets/hauling_driver_card.dart';
import '../widgets/hauling_map_widget.dart';

class HaulingActiveTripView extends StatelessWidget {
  const HaulingActiveTripView({super.key, required this.controller, required this.state});

  final HaulingBookingController controller;
  final HaulingBookingState state;

  @override
  Widget build(BuildContext context) {
    final booking = state.activeBooking!;
    final pickupLatLng = LatLng(booking.pickupLat, booking.pickupLng);
    final dropoffLatLng = LatLng(booking.dropoffLat, booking.dropoffLng);
    final canCancel = [
      HaulingBookingStatus.accepted,
      HaulingBookingStatus.enRoutePickup,
    ].contains(booking.status);

    return Scaffold(
      body: Stack(
        children: [
          Positioned.fill(
            child: HaulingMapWidget(
              pickupLatLng: pickupLatLng,
              dropoffLatLng: dropoffLatLng,
            ),
          ),

          Align(
            alignment: Alignment.bottomCenter,
            child: Container(
              decoration: const BoxDecoration(
                color: Colors.white,
                borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
              ),
              padding: EdgeInsets.fromLTRB(20, 12, 20, MediaQuery.of(context).padding.bottom + 16),
              child: SingleChildScrollView(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    // Drag handle
                    Center(
                      child: Container(
                        width: 36, height: 4,
                        decoration: BoxDecoration(color: Colors.grey[300], borderRadius: BorderRadius.circular(2)),
                      ),
                    ),
                    const SizedBox(height: 14),

                    // Status heading
                    Text(
                      booking.status.activeTripHeading,
                      style: const TextStyle(
                        color: CustomerFigmaColors.text,
                        fontWeight: FontWeight.w800,
                        fontSize: 17,
                      ),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      _statusSubtitle(booking.status),
                      style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 13, height: 1.4),
                    ),
                    const SizedBox(height: 14),

                    // Driver card
                    if (state.providerSnapshot != null)
                      HaulingDriverCard(
                        provider: state.providerSnapshot!,
                        truck: state.truckSnapshot,
                        fareKobo: booking.displayFareKobo,
                        distanceKm: booking.distanceKm,
                      )
                    else
                      const HaulingDriverCardShimmer(),
                    const SizedBox(height: 14),

                    // Truck type + weight info row
                    Row(
                      children: [
                        const Icon(Icons.local_shipping_outlined, color: CustomerFigmaColors.muted, size: 14),
                        const SizedBox(width: 6),
                        Text(
                          booking.preferredTruckType.isNotEmpty
                              ? '${booking.preferredTruckType} · ${booking.cargoWeightKg} kg'
                              : '${booking.cargoWeightKg} kg cargo',
                          style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 12),
                        ),
                      ],
                    ),
                    const SizedBox(height: 14),

                    // Progress bar
                    _TripProgressBar(status: booking.status),
                    const SizedBox(height: 16),

                    // Action buttons row
                    Row(
                      children: [
                        Expanded(
                          child: OutlinedButton.icon(
                            onPressed: () => _callDriver(state.providerSnapshot?.phone),
                            icon: const Icon(Icons.phone_outlined, size: 16),
                            label: const Text('Call Driver'),
                            style: OutlinedButton.styleFrom(
                              foregroundColor: CustomerFigmaColors.primary,
                              side: const BorderSide(color: CustomerFigmaColors.primary),
                              padding: const EdgeInsets.symmetric(vertical: 14),
                              shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
                            ),
                          ),
                        ),
                        if (canCancel) ...[
                          const SizedBox(width: 10),
                          Expanded(
                            child: OutlinedButton(
                              onPressed: () => _confirmCancel(context),
                              style: OutlinedButton.styleFrom(
                                foregroundColor: Colors.red,
                                side: const BorderSide(color: Colors.red),
                                padding: const EdgeInsets.symmetric(vertical: 14),
                                shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
                              ),
                              child: const Text('Cancel Truck', style: TextStyle(fontWeight: FontWeight.w700)),
                            ),
                          ),
                        ],
                      ],
                    ),
                  ],
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }

  String _statusSubtitle(HaulingBookingStatus status) => switch (status) {
    HaulingBookingStatus.accepted        => 'The driver has accepted your request and is preparing to depart.',
    HaulingBookingStatus.enRoutePickup   => 'The driver is on the way to your pickup location.',
    HaulingBookingStatus.arrivedAtPickup => 'The driver has arrived at your pickup location.',
    HaulingBookingStatus.pickedUp        => 'Your cargo has been loaded and is en route.',
    HaulingBookingStatus.enRouteDelivery => 'Your cargo is on its way to the destination.',
    _                                    => 'Your trip is in progress.',
  };

  Future<void> _callDriver(String? phone) async {
    if (phone == null || phone.isEmpty) return;
    final uri = Uri.parse('tel:$phone');
    if (await canLaunchUrl(uri)) await launchUrl(uri);
  }

  void _confirmCancel(BuildContext context) {
    showModalBottomSheet(
      context: context,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (_) => Padding(
        padding: const EdgeInsets.fromLTRB(24, 20, 24, 32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Text(
              'Cancel booking?',
              style: TextStyle(color: CustomerFigmaColors.text, fontSize: 16, fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 8),
            const Text(
              'Cancelling after assignment may incur a fee.',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 24),
            FigmaPrimaryButton(
              label: 'Yes, cancel',
              onPressed: () {
                Navigator.of(context).pop();
                controller.cancelBooking(reason: 'Customer cancelled active booking');
              },
            ),
            const SizedBox(height: 10),
            FigmaSecondaryButton(
              label: 'Keep booking',
              onPressed: () => Navigator.of(context).pop(),
            ),
          ],
        ),
      ),
    );
  }
}

// ─── Progress bar ─────────────────────────────────────────────────────────────

class _TripProgressBar extends StatelessWidget {
  const _TripProgressBar({required this.status});

  final HaulingBookingStatus status;

  static const _orderedStatuses = [
    HaulingBookingStatus.accepted,
    HaulingBookingStatus.enRoutePickup,
    HaulingBookingStatus.arrivedAtPickup,
    HaulingBookingStatus.pickedUp,
    HaulingBookingStatus.enRouteDelivery,
    HaulingBookingStatus.delivered,
  ];

  double get _progress {
    final idx = _orderedStatuses.indexOf(status);
    if (idx < 0) return 0;
    return (idx + 1) / _orderedStatuses.length;
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        ClipRRect(
          borderRadius: BorderRadius.circular(4),
          child: LinearProgressIndicator(
            value: _progress,
            backgroundColor: CustomerFigmaColors.border,
            color: CustomerFigmaColors.primary,
            minHeight: 6,
          ),
        ),
        const SizedBox(height: 6),
        const Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [
            Text('Pickup', style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 11)),
            Text('Destination', style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 11)),
          ],
        ),
      ],
    );
  }
}
