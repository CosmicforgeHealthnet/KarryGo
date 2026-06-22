import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/hauling_booking_controller.dart';
import '../widgets/hauling_flow_helpers.dart';
import '../widgets/hauling_route_point.dart';

class HaulingConfirmView extends StatelessWidget {
  const HaulingConfirmView({
    super.key,
    required this.controller,
    required this.state,
  });

  final HaulingBookingController controller;
  final HaulingBookingState state;

  @override
  Widget build(BuildContext context) {
    final est = state.fareEstimate!;
    final bd = est.breakdownKobo;
    return haulingFlowScaffold(
      title: 'Confirm booking',
      onBack: controller.backToDetails,
      body: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            // Route summary
            Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: Colors.white,
                borderRadius: BorderRadius.circular(14),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  HaulingRoutePoint(
                    label: 'Pickup',
                    address: state.pickupAddress,
                    color: CustomerFigmaColors.primary,
                  ),
                  Padding(
                    padding: const EdgeInsets.only(left: 10),
                    child: Container(width: 2, height: 24, color: CustomerFigmaColors.border),
                  ),
                  HaulingRoutePoint(
                    label: 'Dropoff',
                    address: state.dropoffAddress,
                    color: Colors.orange,
                  ),
                ],
              ),
            ),
            const SizedBox(height: 14),

            // Cargo summary
            Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: Colors.white,
                borderRadius: BorderRadius.circular(14),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text(
                    'Cargo details',
                    style: TextStyle(
                      color: CustomerFigmaColors.text,
                      fontWeight: FontWeight.w800,
                      fontSize: 13,
                    ),
                  ),
                  const SizedBox(height: 10),
                  Row(children: [
                    _ChipTag(label: state.truckTypeOption?.displayLabel ?? 'Truck'),
                    const SizedBox(width: 8),
                    _ChipTag(label: '${state.cargoWeightKg} kg'),
                    if (state.requiresHelpers) ...[
                      const SizedBox(width: 8),
                      _ChipTag(
                        label: '${state.helperCount} helper${state.helperCount > 1 ? 's' : ''}',
                      ),
                    ],
                  ]),
                  if (state.cargoDescription.isNotEmpty) ...[
                    const SizedBox(height: 8),
                    Text(
                      state.cargoDescription,
                      style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 12),
                    ),
                  ],
                ],
              ),
            ),
            const SizedBox(height: 14),

            // Fare breakdown
            Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: Colors.white,
                borderRadius: BorderRadius.circular(14),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text(
                    'Fare estimate',
                    style: TextStyle(
                      color: CustomerFigmaColors.text,
                      fontWeight: FontWeight.w800,
                      fontSize: 13,
                    ),
                  ),
                  const SizedBox(height: 10),
                  haulingFareRow('Base fare', bd.baseFareKobo),
                  haulingFareRow(
                    'Distance (${est.distanceKm.toStringAsFixed(1)} km)',
                    bd.perKmFareKobo,
                  ),
                  if (bd.weightSurchargeKobo > 0)
                    haulingFareRow('Heavy cargo surcharge', bd.weightSurchargeKobo),
                  if (bd.helperFeeKobo > 0) haulingFareRow('Helper fee', bd.helperFeeKobo),
                  const Divider(height: 18),
                  Row(
                    mainAxisAlignment: MainAxisAlignment.spaceBetween,
                    children: [
                      const Text(
                        'Total estimate',
                        style: TextStyle(
                          color: CustomerFigmaColors.text,
                          fontWeight: FontWeight.w800,
                          fontSize: 14,
                        ),
                      ),
                      Text(
                        '₦${est.fareEstimateNaira.toStringAsFixed(0)}',
                        style: const TextStyle(
                          color: CustomerFigmaColors.primary,
                          fontWeight: FontWeight.w800,
                          fontSize: 16,
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),
            const SizedBox(height: 10),
            const Text(
              'Final fare may vary based on actual route. Payment will be collected on delivery.',
              textAlign: TextAlign.center,
              style: TextStyle(
                color: CustomerFigmaColors.muted,
                fontSize: 11,
                height: 1.5,
              ),
            ),
          ],
        ),
      ),
      bottom: Column(
        children: [
          if (state.error != null) ...[
            Text(
              state.error!,
              style: const TextStyle(color: Colors.red, fontSize: 12),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 8),
          ],
          FigmaPrimaryButton(
            label: 'Confirm — ₦${est.fareEstimateNaira.toStringAsFixed(0)}',
            isLoading: state.isLoading,
            onPressed: controller.confirmPayment,
          ),
        ],
      ),
    );
  }
}

// ─── Chip tag (private — only used within confirm view) ──────────────────────

class _ChipTag extends StatelessWidget {
  const _ChipTag({required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: CustomerFigmaColors.primaryTint,
        borderRadius: BorderRadius.circular(20),
      ),
      child: Text(
        label,
        style: const TextStyle(
          color: CustomerFigmaColors.darkGreen,
          fontSize: 11,
          fontWeight: FontWeight.w700,
        ),
      ),
    );
  }
}
