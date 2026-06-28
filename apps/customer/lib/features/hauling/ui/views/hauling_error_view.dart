import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/hauling_booking_controller.dart';
import '../widgets/hauling_flow_helpers.dart';

class HaulingErrorView extends StatelessWidget {
  const HaulingErrorView({
    super.key,
    required this.controller,
    required this.message,
  });

  final HaulingBookingController controller;
  final String message;

  @override
  Widget build(BuildContext context) {
    return haulingFlowScaffold(
      title: 'Error',
      onBack: () => Navigator.of(context).pop(),
      body: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline_rounded, size: 56, color: Colors.red),
            const SizedBox(height: 16),
            Text(
              message,
              textAlign: TextAlign.center,
              style: const TextStyle(
                color: CustomerFigmaColors.muted,
                fontSize: 14,
                height: 1.5,
              ),
            ),
          ],
        ),
      ),
      bottom: FigmaPrimaryButton(
        label: 'Try again',
        onPressed: () {
          // Re-prefill from the prior booking if one exists, otherwise start fresh.
          final prior = controller.state.activeBooking;
          if (prior != null) {
            controller.rebookFrom(prior);
          } else {
            controller.reset();
            controller.startHaulingFlow();
          }
        },
      ),
    );
  }
}
