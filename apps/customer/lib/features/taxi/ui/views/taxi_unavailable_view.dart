import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/taxi_booking_controller.dart';
import '../widgets/taxi_flow_helpers.dart';

/// No driver is online within range of the pickup.
class TaxiUnavailableView extends StatelessWidget {
  const TaxiUnavailableView({
    super.key,
    required this.controller,
    this.message,
  });

  final TaxiBookingController controller;
  final String? message;

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
              color: CustomerFigmaColors.fieldMuted,
              shape: BoxShape.circle,
            ),
            child: const Icon(
              Icons.no_transfer_rounded,
              size: 42,
              color: CustomerFigmaColors.muted,
            ),
          ),
          const SizedBox(height: 22),
          const Text(
            'No drivers nearby',
            style: TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 19,
              fontWeight: FontWeight.w800,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            message ??
                'There are no drivers available around your pickup right now. '
                    'Please try again in a few minutes.',
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
            label: 'Try again',
            onPressed: controller.checkAvailabilityAndQuote,
          ),
          const SizedBox(height: 8),
          FigmaSecondaryButton(
            label: 'Change location',
            onPressed: controller.backToLocationEntry,
          ),
        ],
      ),
    );
  }
}
