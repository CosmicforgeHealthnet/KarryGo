import 'package:flutter/material.dart';

import '../../../../core/format/money_format.dart';
import '../../../../core/widgets/karryam_ui.dart';
import '../../../auth/models/provider_auth_models.dart';

/// Shown right after the driver confirms a delivery: what they finished and
/// what it paid, before going back to new requests.
class ProviderDeliveryCompleteScreen extends StatelessWidget {
  const ProviderDeliveryCompleteScreen({
    super.key,
    required this.booking,
    required this.onDone,
    this.onViewEarnings,
  });

  final ProviderBooking booking;
  final VoidCallback onDone;
  final VoidCallback? onViewEarnings;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 40, 20, 24),
                children: [
                  Center(
                    child: const KAnimArt(KArt.success, size: 170),
                  ),
                  const SizedBox(height: 24),
                  const KHeading(
                    center: true,
                    eyebrow: 'Job done',
                    title: 'Delivery complete',
                    subtitle: 'Nice work. The customer has been notified.',
                  ),
                  const SizedBox(height: 28),
                  KMoneyCard(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        KEyebrow('Trip fare', color: Color(0xB3FFFFFF)),
                        const SizedBox(height: 6),
                        Text(
                          formatNairaShort(booking.fareNaira),
                          style: const TextStyle(fontSize: 32, fontWeight: FontWeight.w700, letterSpacing: -0.8),
                        ),
                        const SizedBox(height: 6),
                        const Text(
                          'Your share is added to your earnings once the job is settled.',
                          style: TextStyle(fontSize: 12.5, color: Color(0xCCFFFFFF), height: 1.4),
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 14),
                  KCard(
                    child: Column(
                      children: [
                        KRouteStops(
                          pickup: booking.pickupAddress,
                          dropoff: booking.dropoffAddress,
                          pickupLabel: 'Picked up from',
                          dropoffLabel: 'Delivered to',
                        ),
                        const SizedBox(height: 10),
                        KInfoRow('Trip', '#${booking.shortId}'),
                        KInfoRow('Load', booking.cargoSummary),
                      ],
                    ),
                  ),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 12),
              child: Column(
                children: [
                  KPrimaryButton(label: 'Back to requests', arrow: true, onPressed: onDone),
                  if (onViewEarnings != null) ...[
                    const SizedBox(height: 10),
                    KSecondaryButton(label: 'View earnings', onPressed: onViewEarnings),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}
