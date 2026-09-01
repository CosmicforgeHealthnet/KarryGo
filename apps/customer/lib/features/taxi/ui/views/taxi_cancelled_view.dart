import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/taxi_booking_controller.dart';
import '../widgets/taxi_flow_helpers.dart';

/// Terminal screen for a cancelled or unmatched ride.
class TaxiCancelledView extends StatelessWidget {
  const TaxiCancelledView({
    super.key,
    required this.controller,
    required this.state,
  });

  final TaxiBookingController controller;
  final TaxiBookingState state;

  @override
  Widget build(BuildContext context) {
    return taxiFlowScaffold(
      body: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Container(
            width: 96,
            height: 96,
            decoration: const BoxDecoration(
              color: Color(0xFFFDECEC),
              shape: BoxShape.circle,
            ),
            child: const Icon(
              Icons.close_rounded,
              size: 42,
              color: Color(0xFFB3261E),
            ),
          ),
          const SizedBox(height: 22),
          const Text(
            'Ride cancelled',
            style: TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 19,
              fontWeight: FontWeight.w800,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            state.error ?? 'This ride was cancelled.',
            textAlign: TextAlign.center,
            style: const TextStyle(
              color: CustomerFigmaColors.muted,
              fontSize: 13,
            ),
          ),
        ],
      ),
      bottom: Column(
        children: [
          FigmaPrimaryButton(
            label: 'Book another ride',
            onPressed: controller.startTaxiFlow,
          ),
          const SizedBox(height: 8),
          FigmaSecondaryButton(
            label: 'Close',
            onPressed: () {
              controller.reset();
              Navigator.of(context).maybePop();
            },
          ),
        ],
      ),
    );
  }
}
