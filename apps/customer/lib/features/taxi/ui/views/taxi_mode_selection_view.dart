import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/taxi_models.dart';
import '../../state/taxi_booking_controller.dart';
import '../widgets/taxi_flow_helpers.dart';

/// The first screen of the car flow: is the customer riding, or sending a
/// package? Everything after this branches on the answer.
class TaxiModeSelectionView extends StatelessWidget {
  const TaxiModeSelectionView({
    super.key,
    required this.controller,
    required this.state,
  });

  final TaxiBookingController controller;
  final TaxiBookingState state;

  @override
  Widget build(BuildContext context) {
    return taxiFlowScaffold(
      title: 'Car',
      onBack: () {
        controller.reset();
        Navigator.of(context).maybePop();
      },
      body: ListView(
        children: [
          const Text(
            'What do you need?',
            style: TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 20,
              fontWeight: FontWeight.w800,
            ),
          ),
          const SizedBox(height: 4),
          const Text(
            'A car can take you somewhere, or take a package for you.',
            style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
          ),
          const SizedBox(height: 20),
          _ModeCard(
            icon: Icons.person_rounded,
            mode: TaxiRideMode.ride,
            selected: state.rideMode == TaxiRideMode.ride,
            onTap: () => controller.selectMode(TaxiRideMode.ride),
          ),
          const SizedBox(height: 12),
          _ModeCard(
            icon: Icons.inventory_2_rounded,
            mode: TaxiRideMode.parcel,
            selected: state.rideMode == TaxiRideMode.parcel,
            onTap: () => controller.selectMode(TaxiRideMode.parcel),
          ),
          const SizedBox(height: 16),
          const Text(
            'Packages are carried by drivers who have opted in, so a package '
            'may take a little longer to match than a ride.',
            style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 11.5),
          ),
        ],
      ),
    );
  }
}

class _ModeCard extends StatelessWidget {
  const _ModeCard({
    required this.icon,
    required this.mode,
    required this.selected,
    required this.onTap,
  });

  final IconData icon;
  final TaxiRideMode mode;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(16),
      child: Container(
        padding: const EdgeInsets.all(18),
        decoration: BoxDecoration(
          color: CustomerFigmaColors.field,
          borderRadius: BorderRadius.circular(16),
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
              width: 52,
              height: 52,
              decoration: BoxDecoration(
                color: CustomerFigmaColors.primaryTint,
                borderRadius: BorderRadius.circular(14),
              ),
              child: Icon(icon, color: CustomerFigmaColors.primary, size: 24),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    mode.title,
                    style: const TextStyle(
                      color: CustomerFigmaColors.text,
                      fontSize: 16,
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                  const SizedBox(height: 3),
                  Text(
                    mode.subtitle,
                    style: const TextStyle(
                      color: CustomerFigmaColors.muted,
                      fontSize: 12.5,
                    ),
                  ),
                ],
              ),
            ),
            const Icon(
              Icons.chevron_right_rounded,
              color: CustomerFigmaColors.muted,
            ),
          ],
        ),
      ),
    );
  }
}
