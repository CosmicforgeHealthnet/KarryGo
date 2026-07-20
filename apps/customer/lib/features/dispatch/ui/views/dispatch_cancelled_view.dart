import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/dispatch_models.dart';
import '../../state/dispatch_booking_controller.dart';

class DispatchCancelledView extends StatelessWidget {
  const DispatchCancelledView({
    super.key,
    required this.controller,
    required this.state,
  });

  final DispatchBookingController controller;
  final DispatchBookingState state;

  @override
  Widget build(BuildContext context) {
    final unmatched =
        state.activeBooking?.status == DispatchBookingStatus.unmatched;
    // A cancellation reason set by the controller (e.g. the search timeout's
    // "No riders found nearby") takes precedence over the generic copy.
    final detail = state.error;
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
                unmatched ? 'No rider found' : 'Delivery cancelled',
                textAlign: TextAlign.center,
                style: const TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 20,
                  fontWeight: FontWeight.w800,
                ),
              ),
              const SizedBox(height: 10),
              Text(
                detail ??
                    (unmatched
                        ? "We couldn't find an available dispatch rider. Any hold on your wallet has been refunded."
                        : 'Your delivery has been cancelled. Any hold on your wallet has been refunded.'),
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
                  // Re-prefill from the cancelled booking (still in state) so the
                  // customer doesn't have to re-enter everything.
                  final prior = state.activeBooking;
                  if (prior != null) {
                    controller.rebookFrom(prior);
                  } else {
                    controller.reset();
                    controller.startDispatchFlow();
                  }
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
