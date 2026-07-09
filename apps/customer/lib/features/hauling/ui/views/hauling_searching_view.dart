import 'package:flutter/material.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/hauling_booking_controller.dart';
import '../widgets/hauling_map_widget.dart';
import '../widgets/hauling_trip_sheet_widgets.dart';

/// "Connecting Driver…" screen (Figma 1736/1766): map + discount banner +
/// bottom sheet with the matched driver, trip stats, route and actions.
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
      backgroundColor: Colors.white,
      bottomNavigationBar: const HaulingFlowBottomNav(),
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
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                // Floating "10% off first booking" card over the map.
                const Padding(
                  padding: EdgeInsets.symmetric(horizontal: 14),
                  child: HaulingDiscountBanner(),
                ),
                const SizedBox(height: 12),

                Container(
                  width: double.infinity,
                  constraints: BoxConstraints(
                    maxHeight: MediaQuery.sizeOf(context).height * 0.72,
                  ),
                  decoration: const BoxDecoration(
                    color: Colors.white,
                    borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
                  ),
                  padding: const EdgeInsets.fromLTRB(20, 22, 20, 16),
                  child: SingleChildScrollView(
                    child: Column(
                      mainAxisSize: MainAxisSize.min,
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        const Text(
                          'Connecting Driver…',
                          style: TextStyle(
                            color: CustomerFigmaColors.text,
                            fontWeight: FontWeight.w800,
                            fontSize: 20,
                          ),
                        ),
                        const SizedBox(height: 6),
                        const Text(
                          'Connecting you with nearby driver. Driver will be on '
                          'their way as soon as they confirmed your request.',
                          style: TextStyle(
                            color: CustomerFigmaColors.muted,
                            fontSize: 13,
                            height: 1.5,
                          ),
                        ),
                        const SizedBox(height: 16),

                        // Matched driver (shimmer until a provider is known).
                        if (state.providerSnapshot != null)
                          HaulingDriverProfileBlock(provider: state.providerSnapshot!)
                        else
                          const _ConnectingDriverPlaceholder(),
                        const SizedBox(height: 18),

                        HaulingTripStatsRow(
                          distanceKm: booking?.distanceKm,
                          fareKobo: booking?.fareEstimateKobo,
                        ),
                        const SizedBox(height: 16),

                        if (booking != null)
                          _RouteSection(
                            pickupAddress: booking.pickupAddress,
                            dropoffAddress: booking.dropoffAddress,
                          ),
                        const SizedBox(height: 18),

                        // "Edit Destination" tinted pill
                        _EditDestinationButton(
                          onPressed:
                              state.isLoading ? null : () => _confirmEditDestination(context),
                        ),
                        const SizedBox(height: 14),

                        if (state.error != null) ...[
                          Text(
                            state.error!,
                            style: const TextStyle(color: Colors.red, fontSize: 12),
                            textAlign: TextAlign.center,
                          ),
                          const SizedBox(height: 8),
                        ],

                        HaulingPillButton(
                          label: 'Cancel Truck',
                          isLoading: state.isLoading,
                          onPressed: () => _confirmCancel(context),
                        ),
                      ],
                    ),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  void _confirmEditDestination(BuildContext context) {
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
              'Edit destination?',
              style: TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 16,
                fontWeight: FontWeight.w800,
              ),
            ),
            const SizedBox(height: 8),
            const Text(
              'This cancels the current search so you can change your locations.',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 24),
            FigmaPrimaryButton(
              label: 'Yes, edit destination',
              onPressed: () {
                Navigator.of(context).pop();
                controller.editDestinationDuringSearch();
              },
            ),
            const SizedBox(height: 10),
            FigmaSecondaryButton(
              label: 'Keep searching',
              onPressed: () => Navigator.of(context).pop(),
            ),
          ],
        ),
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

// ─── Driver placeholder (no provider matched yet) ─────────────────────────────

class _ConnectingDriverPlaceholder extends StatelessWidget {
  const _ConnectingDriverPlaceholder();

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Container(
          width: 64,
          height: 64,
          decoration: const BoxDecoration(
            color: kHaulingCircleGrey,
            shape: BoxShape.circle,
          ),
          child: const Center(
            child: SizedBox.square(
              dimension: 22,
              child: CircularProgressIndicator(
                strokeWidth: 2.5,
                color: CustomerFigmaColors.primary,
              ),
            ),
          ),
        ),
        const SizedBox(width: 14),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Container(
                height: 16,
                width: 160,
                decoration: BoxDecoration(
                  color: kHaulingCircleGrey,
                  borderRadius: BorderRadius.circular(4),
                ),
              ),
              const SizedBox(height: 8),
              Container(
                height: 12,
                width: 120,
                decoration: BoxDecoration(
                  color: kHaulingCircleGrey,
                  borderRadius: BorderRadius.circular(4),
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

// ─── Route section (Pick-up / Add stop / Drop off) ────────────────────────────

class _RouteSection extends StatelessWidget {
  const _RouteSection({required this.pickupAddress, required this.dropoffAddress});

  final String pickupAddress;
  final String dropoffAddress;

  @override
  Widget build(BuildContext context) {
    return IntrinsicHeight(
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const SizedBox(width: 18, child: _RouteIndicatorColumn()),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text(
                  'Pick-up',
                  style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 12),
                ),
                const SizedBox(height: 2),
                Text(
                  pickupAddress,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    color: CustomerFigmaColors.text,
                    fontSize: 13,
                    fontWeight: FontWeight.w800,
                  ),
                ),
                const SizedBox(height: 8),
                const Divider(height: 1, color: CustomerFigmaColors.border),
                const SizedBox(height: 8),
                const Text(
                  'Add stop',
                  style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
                ),
                const SizedBox(height: 8),
                const Divider(height: 1, color: CustomerFigmaColors.border),
                const SizedBox(height: 8),
                const Text(
                  'Drop off (optional)',
                  style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 12),
                ),
                const SizedBox(height: 2),
                Text(
                  dropoffAddress,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    color: CustomerFigmaColors.text,
                    fontSize: 13,
                    fontWeight: FontWeight.w800,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// Green connector column: donut → line → small dot → line → ringed dot.
class _RouteIndicatorColumn extends StatelessWidget {
  const _RouteIndicatorColumn();

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        // Pick-up donut
        Container(
          width: 16,
          height: 16,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: Colors.white,
            border: Border.all(color: CustomerFigmaColors.primary, width: 5),
          ),
        ),
        Expanded(child: Container(width: 2.5, color: CustomerFigmaColors.primary)),
        // Add-stop midpoint
        Container(
          width: 12,
          height: 12,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: Colors.white,
            border: Border.all(color: CustomerFigmaColors.primary, width: 3.5),
          ),
        ),
        Expanded(child: Container(width: 2.5, color: CustomerFigmaColors.primary)),
        // Drop-off ring with center dot
        Container(
          width: 16,
          height: 16,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: Colors.white,
            border: Border.all(color: CustomerFigmaColors.primary, width: 1.5),
          ),
          child: Center(
            child: Container(
              width: 7,
              height: 7,
              decoration: const BoxDecoration(
                shape: BoxShape.circle,
                color: CustomerFigmaColors.primary,
              ),
            ),
          ),
        ),
      ],
    );
  }
}

// ─── Edit Destination pill ────────────────────────────────────────────────────

class _EditDestinationButton extends StatelessWidget {
  const _EditDestinationButton({required this.onPressed});

  final VoidCallback? onPressed;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 48,
      child: FilledButton.icon(
        onPressed: onPressed,
        icon: const Icon(Icons.adjust, size: 18, color: CustomerFigmaColors.primary),
        label: const Text(
          'Edit Destination',
          style: TextStyle(
            color: CustomerFigmaColors.text,
            fontSize: 14,
            fontWeight: FontWeight.w800,
          ),
        ),
        style: FilledButton.styleFrom(
          backgroundColor: CustomerFigmaColors.primaryTint,
          disabledBackgroundColor: CustomerFigmaColors.primaryTint,
          elevation: 0,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
        ),
      ),
    );
  }
}
