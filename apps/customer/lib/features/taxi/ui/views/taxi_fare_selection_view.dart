import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/taxi_models.dart';
import '../../state/taxi_booking_controller.dart';
import '../widgets/taxi_flow_helpers.dart';

/// One quote tile per vehicle class the pickup's zone supports, cheapest
/// first (the service returns them already ordered).
class TaxiFareSelectionView extends StatelessWidget {
  const TaxiFareSelectionView({
    super.key,
    required this.controller,
    required this.state,
  });

  final TaxiBookingController controller;
  final TaxiBookingState state;

  @override
  Widget build(BuildContext context) {
    final selected = state.selectedClass;
    return taxiFlowScaffold(
      title: 'Choose a ride',
      onBack: controller.backToLocationEntry,
      body: ListView(
        children: [
          TaxiRouteSummary(
            pickupAddress: state.pickupAddress,
            dropoffAddress: state.dropoffAddress,
          ),
          const SizedBox(height: 18),
          taxiSectionLabel('Available rides'),
          const SizedBox(height: 4),
          for (final tile in state.fareTiles) ...[
            _FareTileCard(
              tile: tile,
              selected: selected?.vehicleClass == tile.vehicleClass,
              onTap: () => controller.selectClass(tile),
            ),
            const SizedBox(height: 10),
          ],
          if (state.error != null) ...[
            const SizedBox(height: 6),
            Text(
              state.error!,
              style: const TextStyle(color: Colors.red, fontSize: 12),
            ),
          ],
        ],
      ),
      bottom: FigmaPrimaryButton(
        label: selected == null
            ? 'Select a ride'
            : 'Continue with ${selected.label}',
        onPressed: selected == null ? null : controller.proceedToRideOptions,
      ),
    );
  }
}

class _FareTileCard extends StatelessWidget {
  const _FareTileCard({
    required this.tile,
    required this.selected,
    required this.onTap,
  });

  final TaxiFareTile tile;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(14),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
        decoration: BoxDecoration(
          color: selected
              ? CustomerFigmaColors.primaryTint
              : CustomerFigmaColors.field,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(
            color: selected
                ? CustomerFigmaColors.primary
                : CustomerFigmaColors.border,
            width: selected ? 1.6 : 1,
          ),
        ),
        child: Row(
          children: [
            Container(
              width: 46,
              height: 46,
              decoration: BoxDecoration(
                color: CustomerFigmaColors.fieldMuted,
                borderRadius: BorderRadius.circular(12),
              ),
              child: const Icon(
                Icons.local_taxi_rounded,
                color: CustomerFigmaColors.primary,
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    tile.label,
                    style: const TextStyle(
                      color: CustomerFigmaColors.text,
                      fontSize: 15,
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                  const SizedBox(height: 3),
                  Text(
                    '${taxiDistanceLabel(tile.distanceKm)} · ${taxiDurationLabel(tile.durationMin)}',
                    style: const TextStyle(
                      color: CustomerFigmaColors.muted,
                      fontSize: 12,
                    ),
                  ),
                ],
              ),
            ),
            Column(
              crossAxisAlignment: CrossAxisAlignment.end,
              children: [
                Text(
                  formatHaulingFare(tile.fareKobo),
                  style: const TextStyle(
                    color: CustomerFigmaColors.text,
                    fontSize: 15,
                    fontWeight: FontWeight.w900,
                  ),
                ),
                const SizedBox(height: 4),
                Icon(
                  selected
                      ? Icons.radio_button_checked
                      : Icons.radio_button_unchecked,
                  size: 18,
                  color: selected
                      ? CustomerFigmaColors.primary
                      : CustomerFigmaColors.primarySoft,
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}
