import 'package:flutter/material.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/hauling_models.dart';
import '../../state/hauling_booking_controller.dart';
import '../widgets/hauling_map_widget.dart';

class HaulingTierSelectionView extends StatelessWidget {
  const HaulingTierSelectionView({super.key, required this.controller});

  final HaulingBookingController controller;

  HaulingBookingState get _state => controller.state;

  @override
  Widget build(BuildContext context) {
    final pickupLatLng = _state.pickupAddress.isNotEmpty
        ? LatLng(_state.pickupLat, _state.pickupLng)
        : null;
    final dropoffLatLng = _state.dropoffAddress.isNotEmpty
        ? LatLng(_state.dropoffLat, _state.dropoffLng)
        : null;

    return Scaffold(
      body: Stack(
        children: [
          Positioned.fill(
            child: HaulingMapWidget(
              pickupLatLng: pickupLatLng,
              dropoffLatLng: dropoffLatLng,
            ),
          ),

          Positioned(
            top: MediaQuery.of(context).padding.top + 8,
            left: 12,
            child: Material(
              color: Colors.white,
              shape: const CircleBorder(),
              elevation: 2,
              child: InkWell(
                customBorder: const CircleBorder(),
                onTap: controller.backToLocationEntry,
                child: const Padding(
                  padding: EdgeInsets.all(8),
                  child: Icon(Icons.arrow_back, color: CustomerFigmaColors.text, size: 20),
                ),
              ),
            ),
          ),

          Align(
            alignment: Alignment.bottomCenter,
            child: Container(
              decoration: const BoxDecoration(
                color: Colors.white,
                borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
              ),
              padding: const EdgeInsets.fromLTRB(20, 12, 20, 20),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Center(
                    child: Container(
                      width: 36, height: 4,
                      decoration: BoxDecoration(
                        color: Colors.grey[300],
                        borderRadius: BorderRadius.circular(2),
                      ),
                    ),
                  ),
                  const SizedBox(height: 16),

                  // Route summary
                  if (_state.pickupAddress.isNotEmpty) ...[
                    _RouteRow(
                      icon: Icons.circle, color: CustomerFigmaColors.primary, size: 10,
                      address: _state.pickupAddress,
                    ),
                    const Padding(
                      padding: EdgeInsets.only(left: 4),
                      child: SizedBox(width: 2, height: 8,
                        child: DecoratedBox(decoration: BoxDecoration(color: CustomerFigmaColors.border))),
                    ),
                    _RouteRow(
                      icon: Icons.circle, color: Colors.orange, size: 10,
                      address: _state.dropoffAddress,
                    ),
                    const SizedBox(height: 16),
                  ],

                  const Text(
                    'Choose a Truck',
                    style: TextStyle(
                      color: CustomerFigmaColors.text,
                      fontWeight: FontWeight.w800,
                      fontSize: 18,
                    ),
                  ),
                  const SizedBox(height: 4),
                  const Text(
                    'Select the service tier that fits your needs',
                    style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
                  ),
                  const SizedBox(height: 12),

                  ...TruckTier.values.map((tier) => _TierRow(
                    tier: tier,
                    selected: _state.selectedTier == tier,
                    onTap: () => controller.selectTier(tier),
                  )),

                  const SizedBox(height: 16),
                  FigmaPrimaryButton(
                    label: 'Select Truck',
                    onPressed: _state.selectedTier != null
                        ? controller.proceedFromTierToPayment
                        : null,
                  ),
                  SizedBox(height: MediaQuery.of(context).padding.bottom),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _RouteRow extends StatelessWidget {
  const _RouteRow({
    required this.icon,
    required this.color,
    required this.size,
    required this.address,
  });

  final IconData icon;
  final Color color;
  final double size;
  final String address;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Icon(icon, color: color, size: size),
        const SizedBox(width: 8),
        Expanded(
          child: Text(
            address,
            style: const TextStyle(color: CustomerFigmaColors.text, fontSize: 12),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
        ),
      ],
    );
  }
}

class _TierRow extends StatelessWidget {
  const _TierRow({
    required this.tier,
    required this.selected,
    required this.onTap,
  });

  final TruckTier tier;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 180),
        margin: const EdgeInsets.only(bottom: 8),
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: selected ? CustomerFigmaColors.primaryTint : Colors.white,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: selected ? CustomerFigmaColors.primary : CustomerFigmaColors.border,
            width: selected ? 1.5 : 1,
          ),
        ),
        child: Row(
          children: [
            Icon(
              Icons.local_shipping_outlined,
              color: selected ? CustomerFigmaColors.primary : CustomerFigmaColors.muted,
              size: 24,
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    tier.displayLabel,
                    style: TextStyle(
                      color: selected ? CustomerFigmaColors.primary : CustomerFigmaColors.text,
                      fontWeight: FontWeight.w700,
                      fontSize: 14,
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    tier.description,
                    style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 12),
                  ),
                ],
              ),
            ),
            Radio<TruckTier>(
              value: tier,
              groupValue: selected ? tier : null,
              onChanged: (_) => onTap(),
              activeColor: CustomerFigmaColors.primary,
              materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
            ),
          ],
        ),
      ),
    );
  }
}
