import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/hauling_booking_controller.dart';

class HaulingCompletedView extends StatelessWidget {
  const HaulingCompletedView({
    super.key,
    required this.controller,
    required this.state,
  });

  final HaulingBookingController controller;
  final HaulingBookingState state;

  @override
  Widget build(BuildContext context) {
    final booking = state.activeBooking;
    final fare = booking?.displayFareNaira;
    return Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const Spacer(),
              const Center(
                child: Icon(
                  Icons.local_shipping_rounded,
                  size: 80,
                  color: CustomerFigmaColors.primary,
                ),
              ),
              const SizedBox(height: 24),
              const Text(
                'Booking completed!',
                textAlign: TextAlign.center,
                style: TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 22,
                  fontWeight: FontWeight.w800,
                ),
              ),
              if (fare != null && fare > 0) ...[
                const SizedBox(height: 8),
                Text(
                  '₦${fare.toStringAsFixed(0)} charged',
                  textAlign: TextAlign.center,
                  style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 15),
                ),
              ],
              const SizedBox(height: 12),
              const Text(
                'Thank you for using Karry Go truck hauling.',
                textAlign: TextAlign.center,
                style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
              ),
              const Spacer(),
              FigmaPrimaryButton(
                label: 'Done',
                onPressed: () {
                  controller.reset();
                  Navigator.of(context).pop();
                },
              ),
            ],
          ),
        ),
      ),
    );
  }
}
