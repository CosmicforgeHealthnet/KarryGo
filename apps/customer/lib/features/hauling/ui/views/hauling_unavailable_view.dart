import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/hauling_booking_controller.dart';
import '../widgets/hauling_flow_helpers.dart';

class HaulingUnavailableView extends StatelessWidget {
  const HaulingUnavailableView({
    super.key,
    required this.controller,
    required this.count,
  });

  final HaulingBookingController controller;
  final int count;

  @override
  Widget build(BuildContext context) {
    return haulingFlowScaffold(
      title: 'Truck Hauling',
      onBack: () => Navigator.of(context).pop(),
      body: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 100,
              height: 100,
              decoration: const BoxDecoration(
                color: CustomerFigmaColors.primaryPale,
                shape: BoxShape.circle,
              ),
              child: const Icon(
                Icons.local_shipping_outlined,
                size: 48,
                color: CustomerFigmaColors.primary,
              ),
            ),
            const SizedBox(height: 24),
            const Text(
              'No trucks available right now',
              textAlign: TextAlign.center,
              style: TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 18,
                fontWeight: FontWeight.w800,
              ),
            ),
            const SizedBox(height: 10),
            const Text(
              'All truck providers are currently offline.\nPlease try again later.',
              textAlign: TextAlign.center,
              style: TextStyle(
                color: CustomerFigmaColors.muted,
                fontSize: 14,
                height: 1.5,
              ),
            ),
          ],
        ),
      ),
      bottom: Column(
        children: [
          FigmaPrimaryButton(
            label: 'Check again',
            onPressed: controller.startHaulingFlow,
          ),
          const SizedBox(height: 10),
          FigmaSecondaryButton(
            label: 'Go back',
            onPressed: () => Navigator.of(context).pop(),
          ),
        ],
      ),
    );
  }
}
