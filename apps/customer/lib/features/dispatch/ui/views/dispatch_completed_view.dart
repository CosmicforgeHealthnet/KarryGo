import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/dispatch_booking_controller.dart';

class DispatchCompletedView extends StatelessWidget {
  const DispatchCompletedView({
    super.key,
    required this.controller,
    required this.state,
  });

  final DispatchBookingController controller;
  final DispatchBookingState state;

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
                  Icons.electric_bike_rounded,
                  size: 80,
                  color: CustomerFigmaColors.primary,
                ),
              ),
              const SizedBox(height: 24),
              const Text(
                'Delivery completed!',
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
                'Thank you for using Karry Go bike delivery.',
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
