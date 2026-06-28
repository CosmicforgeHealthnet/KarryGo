import 'package:flutter/material.dart';

import '../../../core/format/money_format.dart';
import '../../auth/models/provider_auth_models.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../../home/ui/widgets/provider_request_card.dart';
import '../models/customer_review.dart';
import '../state/provider_trips_controller.dart';

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
    final distanceKm = b.distanceKm;
    final estMinutes = distanceKm != null ? (distanceKm / 30 * 60).round() : null;

    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.fromLTRB(20, 8, 20, 32),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // ─── Top bar ─────────────────────────────────────────
              Row(
                children: [
                  _CircleBackButton(onTap: () => Navigator.of(context).pop()),
                  const SizedBox(width: 12),
                  const Text(
                    'Trip Detail',
                    style: TextStyle(
                      color: kProviderText,
                      fontSize: 18,
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 20),

              // ─── Customer header ─────────────────────────────────
              Center(
                child: Column(
                  children: [
                    ProviderAvatar(name: b.displayName, size: 64),
                    const SizedBox(height: 10),
                    Text(
                      b.displayName,
                      style: const TextStyle(
                        color: kProviderText,
                        fontWeight: FontWeight.w800,
                        fontSize: 22,
                      ),
                    ),
                    const SizedBox(height: 8),
                    BookingMetricsRow(
                      distanceKm: distanceKm,
                      minutes: estMinutes,
                      fareNaira: b.fareEstimateNaira,
                    ),
                  ],
                ),
              ),
              const SizedBox(height: 24),

              // ─── Customer review (completed only) ─────────────────
              if (_isCompleted) ...[
                if (_reviewLoading)
                  const Center(
                    child: Padding(
                      padding: EdgeInsets.symmetric(vertical: 8),
                      child: SizedBox(
                        width: 20, height: 20,
                        child: CircularProgressIndicator(strokeWidth: 2, color: kProviderGreen),
                      ),
                    ),
                  )
                else if (_review != null) ...[
                  _ReviewSection(review: _review!),
                  const SizedBox(height: 20),
                ],
              ],

              // ─── Trip ref + date ─────────────────────────────────
              _LabelValueRow(label: 'Trip Completed:', value: b.shortId),
              const SizedBox(height: 12),
              _LabelValueRow(label: 'Date:', value: _formatDate(b.createdAt)),
              const SizedBox(height: 16),

              // ─── Route card ───────────────────────────────────────
              Container(
                padding: const EdgeInsets.all(16),
                decoration: BoxDecoration(
                  color: Colors.white,
                  borderRadius: BorderRadius.circular(16),
                  border: Border.all(color: kProviderBorder),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    BookingRoute(
                      pickupAddress: b.pickupAddress,
                      dropoffAddress: b.dropoffAddress.isNotEmpty ? b.dropoffAddress : '—',
                    ),
                    const Divider(height: 28, color: kProviderBorder),
                    Row(
                      children: [
                        const Text(
                          'Trip Fee:',
                          style: TextStyle(
                            color: kProviderText,
                            fontWeight: FontWeight.w700,
                            fontSize: 14,
                          ),
                        ),
                        const Spacer(),
                        Text(
                          '₦${formatNaira(b.fareEstimateNaira)}',
                          style: const TextStyle(
                            color: kProviderText,
                            fontWeight: FontWeight.w800,
                            fontSize: 15,
                          ),
                        ),
                      ],
                    ),
                  ],
                ),
              ),
              const SizedBox(height: 24),

              // ─── Receiver Information ─────────────────────────────
              if (b.receiverName.isNotEmpty || b.receiverPhone.isNotEmpty) ...[
                const _SectionTitle('Receiver Information'),
                const SizedBox(height: 14),
                _FieldBlock(label: 'Full Name', value: b.receiverName.isNotEmpty ? b.receiverName : '—'),
                const SizedBox(height: 14),
                _FieldBlock(label: 'Phone Number', value: b.receiverPhone.isNotEmpty ? b.receiverPhone : '—'),
                const SizedBox(height: 24),
              ],

              // ─── Package Information ───────────────────────────────
              const _SectionTitle('Package Information'),
              const SizedBox(height: 14),
              _FieldBlock(
                label: 'Package Content',
                value: b.packageContent.isNotEmpty ? b.packageContent : '—',
              ),
              const SizedBox(height: 14),
              _FieldBlock(
                label: 'Package Size',
                value: b.packageSize.isNotEmpty ? b.packageSize : '—',
              ),
              const SizedBox(height: 14),
              _FieldBlock(
                label: 'Is the item fragile?',
                helper: 'Let us know if the item needs to be handled with extra care.',
                value: b.isFragile ? 'Yes' : 'No',
              ),
              const SizedBox(height: 24),

              // ─── Truck Haul Information ────────────────────────────
              const _SectionTitle('Truck Haul Information'),
              const SizedBox(height: 14),
              _FieldBlock(
                label: 'What are you moving?',
                value: b.cargoDescription.isNotEmpty
                    ? b.cargoDescription
                    : (b.packageContent.isNotEmpty ? b.packageContent : '—'),
              ),
              const SizedBox(height: 14),
              _FieldBlock(
                label: 'Load weight category',
                helper: 'Let us know how heavy the item is.',
                value: b.weightCategory.isNotEmpty ? b.weightCategory : '${b.cargoWeightKg} kg',
              ),
              const SizedBox(height: 14),
              _FieldBlock(
                label: 'Truck Type',
                value: b.preferredTruckType.isNotEmpty
                    ? truckTypeLabel(b.preferredTruckType)
                    : 'Select truck type',
              ),
              const SizedBox(height: 14),
              _FieldBlock(
                label: 'Do you need loaders?',
                helper: 'Let us know if you need extra hands to help you load the truck.',
                value: b.requiresHelpers ? 'Yes ( ${b.helperCount} )' : 'No',
              ),
            ],
          ),
        ),
      ),
    );
  }

  String _formatDate(DateTime dt) {
    const days = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'];
    const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
    return '${days[dt.weekday - 1]}, ${dt.day} ${months[dt.month - 1]} ${dt.year}';
  }
}

// ─── Customer review section (star rating + text) ─────────────────────────────

class _ReviewSection extends StatelessWidget {
  const _ReviewSection({required this.review});

  final CustomerReview review;

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        const Text(
          'Customer Review',
          style: TextStyle(color: kProviderText, fontWeight: FontWeight.w800, fontSize: 15),
        ),
        const SizedBox(height: 8),
        Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: List.generate(5, (i) {
            final filled = i < review.rating;
            return Icon(
              Icons.star_rounded,
              size: 28,
              color: filled ? kProviderGreen : const Color(0xFFD7DBD7),
            );
          }),
        ),
        if (review.reviewText.trim().isNotEmpty) ...[
          const SizedBox(height: 8),
          Text(
            review.reviewText.trim(),
            style: const TextStyle(color: kProviderMuted, fontSize: 13),
            textAlign: TextAlign.center,
          ),
        ],
      ],
    );
  }
}

// ─── Small building blocks ────────────────────────────────────────────────────

class _CircleBackButton extends StatelessWidget {
  const _CircleBackButton({required this.onTap});
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.white,
      shape: const CircleBorder(),
      elevation: 1.5,
      child: InkWell(
        customBorder: const CircleBorder(),
        onTap: onTap,
        child: const Padding(
          padding: EdgeInsets.all(8),
          child: Icon(Icons.arrow_back_rounded, color: kProviderText, size: 22),
        ),
      ),
    );
  }
}

class _SectionTitle extends StatelessWidget {
  const _SectionTitle(this.title);
  final String title;

  @override
  Widget build(BuildContext context) {
    return Text(
      title,
      style: const TextStyle(color: kProviderText, fontWeight: FontWeight.w800, fontSize: 18),
    );
  }
}

/// "Label:" on the left, value right-aligned (trip ref / date rows).
class _LabelValueRow extends StatelessWidget {
  const _LabelValueRow({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label, style: const TextStyle(color: kProviderText, fontWeight: FontWeight.w700, fontSize: 13)),
        const SizedBox(width: 12),
        Expanded(
          child: Text(
            value,
            style: const TextStyle(color: kProviderMuted, fontSize: 13),
            textAlign: TextAlign.end,
          ),
        ),
      ],
    );
  }
}

/// Bold question/label with an optional helper line and the answer below.
class _FieldBlock extends StatelessWidget {
  const _FieldBlock({required this.label, required this.value, this.helper});
  final String label;
  final String value;
  final String? helper;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label, style: const TextStyle(color: kProviderText, fontWeight: FontWeight.w700, fontSize: 14)),
        if (helper != null) ...[
          const SizedBox(height: 2),
          Text(helper!, style: const TextStyle(color: kProviderMuted, fontSize: 12, height: 1.3)),
        ],
        const SizedBox(height: 4),
        Text(value, style: const TextStyle(color: kProviderText, fontSize: 13, fontWeight: FontWeight.w600)),
      ],
    );
  }
}
