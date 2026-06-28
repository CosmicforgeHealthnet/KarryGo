import 'dart:async';

import 'package:flutter/material.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/hauling_booking_controller.dart';
import '../widgets/hauling_driver_card.dart';
import '../widgets/hauling_map_widget.dart';

class HaulingSearchingView extends StatelessWidget {
  const HaulingSearchingView({super.key, required this.controller, required this.state});

  final HaulingBookingController controller;
  final HaulingBookingState state;

  @override
  Widget build(BuildContext context) {
    final booking = state.activeBooking;

    final pickupLatLng = booking != null
        ? LatLng(booking.pickupLat, booking.pickupLng)
        : null;
    final dropoffLatLng = booking != null
        ? LatLng(booking.dropoffLat, booking.dropoffLng)
        : null;

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
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  // Drag handle
                  Center(
                    child: Container(
                      width: 36, height: 4,
                      decoration: BoxDecoration(
                        color: Colors.grey[300],
                        borderRadius: BorderRadius.circular(2),
                      ),
                    ),
                  ),
                  const SizedBox(height: 16),

                  // Discount banner
                  Container(
                    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
                    decoration: BoxDecoration(
                      color: const Color(0xFFE8F5EE),
                      borderRadius: BorderRadius.circular(8),
                    ),
                    child: Row(
                      children: const [
                        Icon(Icons.local_offer_outlined, color: CustomerFigmaColors.primary, size: 16),
                        SizedBox(width: 8),
                        Text(
                          '10% off your first truck booking!',
                          style: TextStyle(
                            color: CustomerFigmaColors.darkGreen,
                            fontSize: 12,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 14),

                  const Text(
                    'Connecting Driver...',
                    style: TextStyle(
                      color: CustomerFigmaColors.text,
                      fontWeight: FontWeight.w800,
                      fontSize: 18,
                    ),
                  ),
                  const SizedBox(height: 6),
                  const Text(
                    'Connecting you with a nearby driver. The driver will be on their way as soon as they confirm your request.',
                    style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13, height: 1.5),
                  ),
                  const SizedBox(height: 10),
                  _SearchCountdown(deadline: controller.searchDeadline),
                  const SizedBox(height: 14),

                  // Driver card — shimmer if not yet loaded
                  if (booking?.providerId != null) ...[
                    state.providerSnapshot != null
                        ? HaulingDriverCard(
                            provider: state.providerSnapshot!,
                            truck: state.truckSnapshot,
                            fareKobo: booking?.fareEstimateKobo,
                            distanceKm: booking?.distanceKm,
                          )
                        : const HaulingDriverCardShimmer(),
                    const SizedBox(height: 14),
                  ],

                  // Route rows
                  if (booking != null) ...[
                    _RouteRow(
                      dotColor: CustomerFigmaColors.primary,
                      label: 'Pickup',
                      address: booking.pickupAddress,
                    ),
                    Padding(
                      padding: const EdgeInsets.only(left: 5),
                      child: Container(width: 2, height: 12, color: CustomerFigmaColors.border),
                    ),
                    _RouteRow(
                      dotColor: Colors.orange,
                      label: 'Drop-off',
                      address: booking.dropoffAddress,
                    ),
                    const SizedBox(height: 14),
                  ],

                  if (state.error != null) ...[
                    const SizedBox(height: 8),
                    Container(
                      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
                      decoration: BoxDecoration(
                        color: Colors.red[50],
                        borderRadius: BorderRadius.circular(8),
                        border: Border.all(color: Colors.red[200]!),
                      ),
                      child: Text(
                        state.error!,
                        style: const TextStyle(color: Colors.red, fontSize: 12),
                        textAlign: TextAlign.center,
                      ),
                    ),
                    const SizedBox(height: 8),
                  ],

                  OutlinedButton(
                    onPressed: state.isLoading ? null : () => _confirmCancel(context),
                    style: OutlinedButton.styleFrom(
                      foregroundColor: Colors.red,
                      side: const BorderSide(color: Colors.red),
                      padding: const EdgeInsets.symmetric(vertical: 14),
                      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
                    ),
                    child: state.isLoading
                        ? const SizedBox(
                            width: 18, height: 18,
                            child: CircularProgressIndicator(strokeWidth: 2, color: Colors.red),
                          )
                        : const Text('Cancel Truck', style: TextStyle(fontWeight: FontWeight.w700)),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
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
              'Are you sure you want to cancel this booking?',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 24),
            FigmaPrimaryButton(
              label: 'Yes, cancel',
              onPressed: () {
                Navigator.of(context).pop();
                controller.cancelBooking(reason: 'Customer cancelled during search');
              },
            ),
            const SizedBox(height: 10),
            FigmaSecondaryButton(
              label: 'Keep waiting',
              onPressed: () => Navigator.of(context).pop(),
            ),
          ],
        ),
      ),
    );
  }
}

class _RouteRow extends StatelessWidget {
  const _RouteRow({required this.dotColor, required this.label, required this.address});

  final Color dotColor;
  final String label;
  final String address;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        Container(
          width: 10, height: 10,
          decoration: BoxDecoration(color: dotColor, shape: BoxShape.circle),
        ),
        const SizedBox(width: 10),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                label,
                style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 10, fontWeight: FontWeight.w600),
              ),
              Text(
                address,
                style: const TextStyle(color: CustomerFigmaColors.text, fontSize: 13),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ],
          ),
        ),
      ],
    );
  }
}

/// Live countdown to the search timeout. Ticks every second off [deadline];
/// renders nothing when no search is in flight.
class _SearchCountdown extends StatefulWidget {
  const _SearchCountdown({required this.deadline});

  final DateTime? deadline;

  @override
  State<_SearchCountdown> createState() => _SearchCountdownState();
}

class _SearchCountdownState extends State<_SearchCountdown> {
  Timer? _ticker;

  @override
  void initState() {
    super.initState();
    _ticker = Timer.periodic(const Duration(seconds: 1), (_) {
      if (mounted) setState(() {});
    });
  }

  @override
  void dispose() {
    _ticker?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final deadline = widget.deadline;
    if (deadline == null) return const SizedBox.shrink();
    final remaining = deadline.difference(DateTime.now());
    final seconds = remaining.inSecondsClamp;
    return Text(
      'Searching for up to ${seconds}s…',
      style: const TextStyle(
        color: CustomerFigmaColors.primary,
        fontSize: 12,
        fontWeight: FontWeight.w600,
      ),
    );
  }
}

extension on Duration {
  int get inSecondsClamp => inSeconds < 0 ? 0 : inSeconds;
}
