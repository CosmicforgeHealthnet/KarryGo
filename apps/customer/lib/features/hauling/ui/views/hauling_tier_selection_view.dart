import 'package:flutter/material.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/hauling_models.dart';
import '../../state/hauling_booking_controller.dart';
import '../widgets/hauling_map_widget.dart';

class HaulingTierSelectionView extends StatelessWidget {
  const HaulingTierSelectionView({
    super.key,
    required this.controller,
    required this.state,
  });

  final HaulingBookingController controller;
  final HaulingBookingState state;

  @override
  Widget build(BuildContext context) {
    final pickupLatLng = state.pickupAddress.isNotEmpty
        ? LatLng(state.pickupLat, state.pickupLng)
        : null;
    final dropoffLatLng = state.dropoffAddress.isNotEmpty
        ? LatLng(state.dropoffLat, state.dropoffLng)
        : null;

    // Indicative price only: this preview estimate uses a default cargo weight
    // until the customer enters details, and the tier itself has no backend
    // effect on the fare. The final fare is computed at booking from the real
    // weight/helpers, so label it "from ..." rather than an exact amount.
    final fareKobo = state.fareEstimate?.fareEstimateKobo;
    final fareLabel = fareKobo != null
        ? 'from ₦${(fareKobo / 100).toStringAsFixed(0)}'
        : '—';

    return Scaffold(
      body: Stack(
        children: [
          Positioned.fill(
            child: HaulingMapWidget(
              pickupLatLng: pickupLatLng,
              dropoffLatLng: dropoffLatLng,
            ),
          ),

          // Back button
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
                  // Drag handle
                  Center(
                    child: Container(
                      width: 36, height: 4,
                      decoration: BoxDecoration(
                        color: Colors.grey[300],
                        borderRadius: BorderRadius.circular(2),
                      ),
                    ),
                  ),
                  const SizedBox(height: 14),

                  // Discount banner
                  Container(
                    padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
                    decoration: BoxDecoration(
                      color: Colors.white,
                      borderRadius: BorderRadius.circular(12),
                      border: Border.all(color: CustomerFigmaColors.border),
                    ),
                    child: const Row(
                      children: [
                        CircleAvatar(
                          radius: 14,
                          backgroundColor: CustomerFigmaColors.primary,
                          child: Icon(Icons.check, color: Colors.white, size: 16),
                        ),
                        SizedBox(width: 12),
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(
                                '10% off first booking',
                                style: TextStyle(
                                  color: CustomerFigmaColors.text,
                                  fontSize: 13,
                                  fontWeight: FontWeight.w700,
                                ),
                              ),
                              Text(
                                'View Details',
                                style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 11),
                              ),
                            ],
                          ),
                        ),
                        Icon(Icons.chevron_right, color: CustomerFigmaColors.muted, size: 18),
                      ],
                    ),
                  ),
                  const SizedBox(height: 14),

                  // Route summary
                  if (state.pickupAddress.isNotEmpty) ...[
                    _RouteRow(
                      dot: const _GreenDot(),
                      label: 'Pick-up',
                      address: state.pickupAddress,
                    ),
                    Padding(
                      padding: const EdgeInsets.only(left: 9),
                      child: SizedBox(
                        width: 2, height: 12,
                        child: CustomPaint(painter: _DashedLine()),
                      ),
                    ),
                    _RouteRow(
                      dot: const _OrangeDot(),
                      label: 'Drop off (optional)',
                      address: state.dropoffAddress.isNotEmpty
                          ? state.dropoffAddress
                          : 'Not set',
                    ),
                    const SizedBox(height: 14),
                  ],

                  // Heading
                  const Text(
                    'Choose a Truck',
                    style: TextStyle(
                      color: CustomerFigmaColors.text,
                      fontWeight: FontWeight.w800,
                      fontSize: 20,
                    ),
                  ),
                  const SizedBox(height: 2),
                  const Text(
                    'Tell us your destination',
                    style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
                  ),
                  const SizedBox(height: 12),

                  ...TruckTier.values.toList().asMap().entries.map((entry) {
                    final i = entry.key;
                    final tier = entry.value;
                    return _TierCard(
                      tier: tier,
                      selected: state.selectedTier == tier,
                      fareLabel: fareLabel,
                      isPopular: i == 0,
                      onTap: () => controller.selectTier(tier),
                    );
                  }),

                  const SizedBox(height: 16),
                  FigmaPrimaryButton(
                    label: 'Select Truck',
                    onPressed: state.selectedTier != null
                        ? controller.proceedFromTierToDetails
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

// ─── Tier card ────────────────────────────────────────────────────────────────

class _TierCard extends StatelessWidget {
  const _TierCard({
    required this.tier,
    required this.selected,
    required this.fareLabel,
    required this.isPopular,
    required this.onTap,
  });

  final TruckTier tier;
  final bool selected;
  final String fareLabel;
  final bool isPopular;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 180),
        margin: const EdgeInsets.only(bottom: 10),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        decoration: BoxDecoration(
          color: selected ? CustomerFigmaColors.primaryTint : Colors.white,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(
            color: selected ? CustomerFigmaColors.primary : CustomerFigmaColors.border,
            width: selected ? 1.5 : 1,
          ),
        ),
        child: Row(
          children: [
            // Truck image
            ClipRRect(
              borderRadius: BorderRadius.circular(8),
              child: Image.asset(
                'assets/figma/delivery truck back side view.png',
                width: 56,
                height: 44,
                fit: BoxFit.contain,
                errorBuilder: (_, _, _) => const Icon(
                  Icons.local_shipping_rounded,
                  color: CustomerFigmaColors.primary,
                  size: 40,
                ),
              ),
            ),
            const SizedBox(width: 12),

            // Tier info
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    tier.displayLabel,
                    style: TextStyle(
                      color: selected ? CustomerFigmaColors.primary : CustomerFigmaColors.text,
                      fontWeight: FontWeight.w700,
                      fontSize: 15,
                    ),
                  ),
                  const SizedBox(height: 3),
                  Row(
                    children: [
                      Text(
                        fareLabel,
                        style: TextStyle(
                          color: selected ? CustomerFigmaColors.primary : CustomerFigmaColors.muted,
                          fontSize: 13,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                      const SizedBox(width: 10),
                      Icon(
                        Icons.access_time_rounded,
                        size: 12,
                        color: selected ? CustomerFigmaColors.primary : CustomerFigmaColors.muted,
                      ),
                      const SizedBox(width: 3),
                      Text(
                        '2 min',
                        style: TextStyle(
                          color: selected ? CustomerFigmaColors.primary : CustomerFigmaColors.muted,
                          fontSize: 12,
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),

            // Popular badge + radio
            Column(
              crossAxisAlignment: CrossAxisAlignment.end,
              children: [
                if (isPopular)
                  Container(
                    margin: const EdgeInsets.only(bottom: 4),
                    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                    decoration: BoxDecoration(
                      color: const Color(0xFF1A1A1A),
                      borderRadius: BorderRadius.circular(6),
                    ),
                    child: const Text(
                      'Popular',
                      style: TextStyle(color: Colors.white, fontSize: 10, fontWeight: FontWeight.w700),
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
          ],
        ),
      ),
    );
  }
}

// ─── Route summary widgets ────────────────────────────────────────────────────

class _RouteRow extends StatelessWidget {
  const _RouteRow({required this.dot, required this.label, required this.address});

  final Widget dot;
  final String label;
  final String address;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        dot,
        const SizedBox(width: 10),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                label,
                style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 11),
              ),
              Text(
                address,
                style: const TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 13,
                  fontWeight: FontWeight.w600,
                ),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ],
          ),
        ),
      ],
    );
  }
}

class _GreenDot extends StatelessWidget {
  const _GreenDot();
  @override
  Widget build(BuildContext context) => const Icon(
        Icons.radio_button_checked,
        color: CustomerFigmaColors.primary,
        size: 18,
      );
}

class _OrangeDot extends StatelessWidget {
  const _OrangeDot();
  @override
  Widget build(BuildContext context) => const Icon(
        Icons.location_on,
        color: Colors.orange,
        size: 18,
      );
}

class _DashedLine extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = CustomerFigmaColors.border
      ..strokeWidth = 1.5;
    const dash = 3.0, gap = 2.5;
    double y = 0;
    while (y < size.height) {
      canvas.drawLine(Offset(size.width / 2, y),
          Offset(size.width / 2, (y + dash).clamp(0, size.height)), paint);
      y += dash + gap;
    }
  }

  @override
  bool shouldRepaint(_DashedLine _) => false;
}
