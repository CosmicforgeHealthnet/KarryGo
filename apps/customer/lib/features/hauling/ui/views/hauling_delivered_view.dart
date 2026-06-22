import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/hauling_booking_controller.dart';

class HaulingDeliveredView extends StatelessWidget {
  const HaulingDeliveredView({super.key, required this.state});

  final HaulingBookingState state;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            children: [
              const Spacer(),
              Container(
                width: 110,
                height: 110,
                decoration: BoxDecoration(
                  color: CustomerFigmaColors.primaryTint,
                  shape: BoxShape.circle,
                  border: Border.all(color: CustomerFigmaColors.primarySoft, width: 3),
                ),
                child: const Icon(
                  Icons.done_all_rounded,
                  size: 52,
                  color: CustomerFigmaColors.primary,
                ),
              ),
              const SizedBox(height: 28),
              const Text(
                'Your cargo has been delivered!',
                textAlign: TextAlign.center,
                style: TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 20,
                  fontWeight: FontWeight.w800,
                ),
              ),
              const SizedBox(height: 12),
              const Text(
                'Please confirm receipt. The booking will auto-complete in 30 minutes.',
                textAlign: TextAlign.center,
                style: TextStyle(
                  color: CustomerFigmaColors.muted,
                  fontSize: 13,
                  height: 1.5,
                ),
              ),
              const Spacer(),
              const LinearProgressIndicator(
                value: null,
                color: CustomerFigmaColors.primary,
                backgroundColor: CustomerFigmaColors.primaryPale,
              ),
              const SizedBox(height: 8),
              const Text(
                'Auto-completing in a few minutes...',
                textAlign: TextAlign.center,
                style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 11),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
