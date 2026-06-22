import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/hauling_models.dart';
import '../../state/hauling_booking_controller.dart';

class HaulingCancelledView extends StatelessWidget {
  const HaulingCancelledView({
    super.key,
    required this.controller,
    required this.state,
  });

  final HaulingBookingController controller;
  final HaulingBookingState state;

  @override
  Widget build(BuildContext context) {
    final unmatched = state.activeBooking?.status == HaulingBookingStatus.unmatched;
    return Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const Spacer(),
              Center(
                child: Container(
                  width: 100,
                  height: 100,
                  decoration: const BoxDecoration(
                    color: Color(0xFFFFF0F0),
                    shape: BoxShape.circle,
                  ),
                  child: const Icon(Icons.cancel_outlined, size: 48, color: Colors.red),
                ),
              ),
              const SizedBox(height: 24),
              Text(
                unmatched ? 'No provider found' : 'Booking cancelled',
                textAlign: TextAlign.center,
                style: const TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 20,
                  fontWeight: FontWeight.w800,
                ),
              ),
              const SizedBox(height: 10),
              Text(
                unmatched
                    ? "We couldn't find an available truck provider. Please try again later."
                    : 'Your booking has been cancelled.',
                textAlign: TextAlign.center,
                style: const TextStyle(
                  color: CustomerFigmaColors.muted,
                  fontSize: 13,
                  height: 1.5,
                ),
              ),
              const Spacer(),
              FigmaPrimaryButton(
                label: 'Try again',
                onPressed: () {
                  controller.reset();
                  controller.startHaulingFlow();
                },
              ),
              const SizedBox(height: 10),
              FigmaSecondaryButton(
                label: 'Go home',
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
