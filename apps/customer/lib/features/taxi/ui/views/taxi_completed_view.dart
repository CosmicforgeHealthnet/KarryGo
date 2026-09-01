import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/taxi_booking_controller.dart';
import '../widgets/taxi_flow_helpers.dart';

/// Terminal screen once the ride is done and the review step is behind us.
class TaxiCompletedView extends StatelessWidget {
  const TaxiCompletedView({
    super.key,
    required this.controller,
    required this.state,
  });

  final TaxiBookingController controller;
  final TaxiBookingState state;

  @override
  Widget build(BuildContext context) {
    final booking = state.activeBooking;
    final isParcel = booking?.isParcel ?? state.rideMode.isParcel;
    return taxiFlowScaffold(
      body: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Container(
            width: 96,
            height: 96,
            decoration: const BoxDecoration(
              color: CustomerFigmaColors.successSoft,
              shape: BoxShape.circle,
            ),
            child: const Icon(
              Icons.check_rounded,
              size: 44,
              color: CustomerFigmaColors.success,
            ),
          ),
          const SizedBox(height: 22),
          Text(
            isParcel ? 'Delivery complete' : 'Trip complete',
            style: const TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 20,
              fontWeight: FontWeight.w800,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            isParcel
                ? 'Thanks for sending with Karry Go.'
                : 'Thanks for riding with Karry Go.',
            style: const TextStyle(
              color: CustomerFigmaColors.muted,
              fontSize: 13,
            ),
          ),
          if (booking != null && booking.displayFareKobo > 0) ...[
            const SizedBox(height: 20),
            Text(
              formatHaulingFare(booking.displayFareKobo),
              style: const TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 26,
                fontWeight: FontWeight.w900,
              ),
            ),
            const SizedBox(height: 4),
            Text(
              'Charged to your wallet',
              style: const TextStyle(
                color: CustomerFigmaColors.muted,
                fontSize: 12,
              ),
            ),
          ],
        ],
      ),
      bottom: FigmaPrimaryButton(
        label: 'Done',
        onPressed: () {
          controller.reset();
          Navigator.of(context).maybePop();
        },
      ),
    );
  }
}
