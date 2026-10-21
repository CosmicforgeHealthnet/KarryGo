import 'package:flutter/material.dart';

import '../../../core/format/date_format.dart';
import '../../../core/format/money_format.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../../auth/models/provider_auth_models.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../../home/ui/widgets/provider_request_card.dart';
import '../models/customer_review.dart';
import '../state/provider_trips_controller.dart';
import 'widgets/trip_list_card.dart';

/// Read-only trip detail (Figma "Trip Detail" screens 4–9). Sections shown
/// adapt to the trip: completed trips show the customer review (when present),
/// receiver/package/truck-haul info, and proof of completion.
class ProviderTripDetailScreen extends StatefulWidget {
  const ProviderTripDetailScreen({
    super.key,
    required this.booking,
    required this.controller,
  });

  final ProviderBooking booking;
  final ProviderTripsController controller;

  @override
  State<ProviderTripDetailScreen> createState() => _ProviderTripDetailScreenState();
}

class _ProviderTripDetailScreenState extends State<ProviderTripDetailScreen> {
  CustomerReview? _review;
  bool _reviewLoading = false;

  ProviderBooking get _booking => widget.booking;
  bool get _isCompleted =>
      _booking.status == 'completed' || _booking.status == 'delivered';

  @override
  void initState() {
    super.initState();
    if (_isCompleted) _loadReview();
  }

  Future<void> _loadReview() async {
    setState(() => _reviewLoading = true);
    final review = await widget.controller.fetchReview(_booking.id);
    if (!mounted) return;
    setState(() {
      _review = review;
      _reviewLoading = false;
    });
  }

  @override
  Widget build(BuildContext context) {
    final b = _booking;
    final hasReceiver = b.receiverName.isNotEmpty || b.receiverPhone.isNotEmpty;

    return Scaffold(
      appBar: KTopBar(title: 'Trip details'),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(20, 8, 20, 32),
        children: [
          Row(
            children: [
              TripStatusBadge(status: b.status),
              const Spacer(),
              Text('#${b.shortId}', style: TextStyle(fontSize: 12.5, color: kProviderMuted)),
            ],
          ),
          const SizedBox(height: 14),
          Text(
            formatNairaShort(b.fareNaira),
            style: TextStyle(fontSize: 34, fontWeight: FontWeight.w700, letterSpacing: -1, color: kProviderText),
          ),
          const SizedBox(height: 4),
          Text(
            formatLongDate(b.createdAt),
            style: TextStyle(fontSize: 13.5, color: kProviderMuted),
          ),
          const SizedBox(height: 20),
          KCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                BookingRoute(
                  full: true,
                  pickupAddress: b.pickupAddress,
                  dropoffAddress: b.dropoffAddress.isNotEmpty ? b.dropoffAddress : '—',
                ),
                const SizedBox(height: 14),
                const Divider(),
                const SizedBox(height: 6),
                KInfoRow('Distance', b.distanceKm == null ? '—' : '${b.distanceKm!.toStringAsFixed(1)} km'),
                KInfoRow('Trip Fee:', formatNairaShort(b.fareNaira), bold: true),
              ],
            ),
          ),
          if (_isCompleted) ...[
            const SizedBox(height: 22),
            const KSectionHeader('Customer review'),
            const SizedBox(height: 10),
            if (_reviewLoading)
              KCard(
                child: Center(
                  child: SizedBox(width: 20, height: 20, child: CircularProgressIndicator(strokeWidth: 2)),
                ),
              )
            else if (_review != null)
              _ReviewSection(review: _review!)
            else
              KCard(
                child: Text(
                  'The customer has not left a review for this trip.',
                  style: TextStyle(fontSize: 13, color: kProviderMuted),
                ),
              ),
          ],
          const SizedBox(height: 22),
          const KSectionHeader('Load'),
          const SizedBox(height: 10),
          KCard(
            child: Column(
              children: [
                BookingCargoRow(booking: b),
                const SizedBox(height: 6),
                KInfoRow('Package', b.packageContent.isNotEmpty ? b.packageContent : '—'),
                KInfoRow('Package size', b.packageSize.isNotEmpty ? b.packageSize : '—'),
                KInfoRow('Weight', b.weightCategory.isNotEmpty ? b.weightCategory : '${b.cargoWeightKg} kg'),
                KInfoRow(
                  'Truck type',
                  b.preferredTruckType.isNotEmpty ? truckTypeLabel(b.preferredTruckType) : 'Any truck',
                ),
                KInfoRow('Loaders', b.requiresHelpers ? '${b.helperCount}' : 'None'),
                KInfoRow('Fragile', b.isFragile ? 'Yes' : 'No'),
              ],
            ),
          ),
          if (hasReceiver) ...[
            const SizedBox(height: 22),
            const KSectionHeader('Receiver'),
            const SizedBox(height: 10),
            KCard(
              child: Row(
                children: [
                  KAvatar(name: b.receiverName.isNotEmpty ? b.receiverName : 'Receiver'),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          b.receiverName.isNotEmpty ? b.receiverName : '—',
                          style: const TextStyle(fontSize: 14.5, fontWeight: FontWeight.w600),
                        ),
                        const SizedBox(height: 2),
                        Text(
                          b.receiverPhone.isNotEmpty ? b.receiverPhone : 'No phone number',
                          style: TextStyle(fontSize: 12.5, color: kProviderMuted),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),
          ],
        ],
      ),
    );
  }
}

// ─── Customer review ─────────────────────────────────────────────────────────

class _ReviewSection extends StatelessWidget {
  const _ReviewSection({required this.review});

  final CustomerReview review;

  @override
  Widget build(BuildContext context) {
    return KCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              ...List.generate(5, (i) {
                return Icon(
                  Icons.star_rounded,
                  size: 24,
                  color: i < review.rating ? kAmber : kProviderBorder,
                );
              }),
              const SizedBox(width: 8),
              Text(
                '${review.rating}.0',
                style: const TextStyle(fontSize: 14, fontWeight: FontWeight.w700),
              ),
            ],
          ),
          if (review.reviewText.trim().isNotEmpty) ...[
            const SizedBox(height: 10),
            Text(
              '"${review.reviewText.trim()}"',
              style: TextStyle(fontSize: 13.5, height: 1.45, color: kProviderText),
            ),
          ],
        ],
      ),
    );
  }
}
