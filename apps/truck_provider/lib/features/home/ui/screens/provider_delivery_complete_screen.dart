import 'package:flutter/material.dart';

import '../../../../core/format/money_format.dart';
import '../../../auth/models/provider_auth_models.dart';
import '../widgets/provider_app_colors.dart';

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
    final fareKobo = booking.fareFinalKobo ?? booking.fareEstimateKobo ?? 0;
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 24, 24, 16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const Spacer(),
              Center(
                child: Container(
                  width: 96,
                  height: 96,
                  decoration: const BoxDecoration(color: kProviderGreenTint, shape: BoxShape.circle),
                  child: const Icon(Icons.check_rounded, color: kProviderGreen, size: 56),
                ),
              ),
              const SizedBox(height: 20),
              const Text(
                'Delivery complete',
                textAlign: TextAlign.center,
                style: TextStyle(color: kProviderText, fontSize: 24, fontWeight: FontWeight.w800),
              ),
              const SizedBox(height: 6),
              Text(
                'Trip ${booking.shortId}',
                textAlign: TextAlign.center,
                style: const TextStyle(color: kProviderMuted, fontSize: 14),
              ),
              const SizedBox(height: 28),
              Container(
                padding: const EdgeInsets.all(18),
                decoration: BoxDecoration(
                  color: kProviderPageBg,
                  borderRadius: BorderRadius.circular(18),
                  border: Border.all(color: kProviderBorder),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    _stop(Icons.radio_button_checked, 'Picked up from', booking.pickupAddress),
                    const SizedBox(height: 14),
                    _stop(Icons.location_on, 'Delivered to', booking.dropoffAddress),
                    const Divider(height: 32, color: kProviderBorder),
                    Row(
                      children: [
                        const Expanded(
                          child: Text('Trip fare', style: TextStyle(color: kProviderMuted, fontSize: 14)),
                        ),
                        Text(
                          '₦${formatNaira(fareKobo / 100)}',
                          style: const TextStyle(color: kProviderText, fontSize: 20, fontWeight: FontWeight.w800),
                        ),
                      ],
                    ),
                    const SizedBox(height: 6),
                    const Text(
                      'Your share is added to your earnings once the job is settled.',
                      style: TextStyle(color: kProviderMuted, fontSize: 12),
                    ),
                  ],
                ),
              ),
              const Spacer(flex: 2),
              SizedBox(
                height: 54,
                child: FilledButton(
                  onPressed: onDone,
                  style: FilledButton.styleFrom(
                    backgroundColor: kProviderGreen,
                    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(30)),
                  ),
                  child: const Text('Back to requests', style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700)),
                ),
              ),
              if (onViewEarnings != null)
                TextButton(
                  onPressed: onViewEarnings,
                  child: const Text('View earnings', style: TextStyle(color: kProviderGreen)),
                ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _stop(IconData icon, String label, String address) => Row(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Icon(icon, color: kProviderGreen, size: 18),
      const SizedBox(width: 10),
      Expanded(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(label, style: const TextStyle(color: kProviderMuted, fontSize: 12)),
            Text(
              address.isEmpty ? 'Address not recorded' : address,
              style: const TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w600),
            ),
          ],
        ),
      ),
    ],
  );
}
