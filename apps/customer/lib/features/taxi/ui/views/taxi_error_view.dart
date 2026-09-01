import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/taxi_booking_controller.dart';
import '../widgets/taxi_flow_helpers.dart';

class TaxiErrorView extends StatelessWidget {
  const TaxiErrorView({
    super.key,
    required this.controller,
    required this.message,
  });

  final TaxiBookingController controller;
  final String message;

  @override
  Widget build(BuildContext context) {
    return taxiFlowScaffold(
      body: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          const Icon(
            Icons.error_outline_rounded,
            size: 56,
            color: Color(0xFFB3261E),
          ),
          const SizedBox(height: 18),
          const Text(
            'Something went wrong',
            style: TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 18,
              fontWeight: FontWeight.w800,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            message,
            textAlign: TextAlign.center,
            style: const TextStyle(
              color: CustomerFigmaColors.muted,
              fontSize: 13,
            ),
          ),
        ],
      ),
      bottom: FigmaPrimaryButton(
        label: 'Start over',
        onPressed: controller.startTaxiFlow,
      ),
    );
  }
}
