import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../data/places_api.dart';
import '../../models/hauling_models.dart';
import '../../state/hauling_booking_controller.dart';
import '../hauling_flow_screen.dart';
import '../widgets/hauling_trip_widgets.dart';

/// Read-only details for a terminal booking (completed / cancelled / unmatched),
/// matching the Figma "Trip Detail" design: driver header, completed reference +
/// date, route card, fee, receiver/package info, proof of completion, and an
/// inline review section for completed trips. Finished trips can be re-ordered
/// with "Book again".
class HaulingTripDetailScreen extends StatefulWidget {
  const HaulingTripDetailScreen({
    super.key,
    required this.booking,
    required this.controller,
    required this.placesApi,
  });

  final HaulageBooking booking;
  final HaulingBookingController controller;
  final PlacesApi placesApi;

  @override
  State<HaulingTripDetailScreen> createState() =>
      _HaulingTripDetailScreenState();
}

class _HaulingTripDetailScreenState extends State<HaulingTripDetailScreen> {
  ProviderSnapshot? _provider;
  TruckSnapshot? _truck;

  // Review form state.
  int _rating = 0;
  final _reviewCtrl = TextEditingController();
  bool? _recommends;
  bool _submitting = false;
  bool _submitted = false;
  String? _error;

  HaulageBooking get booking => widget.booking;

  bool get _canRebook =>
      booking.status == HaulingBookingStatus.completed ||
      booking.status == HaulingBookingStatus.cancelled ||
      booking.status == HaulingBookingStatus.unmatched;

  // A delivered trip can still be reviewed — the backend accepts a review for a
  // delivered or completed booking and promotes it to completed on submit.
  bool get _canReview =>
      booking.status == HaulingBookingStatus.completed ||
      booking.status == HaulingBookingStatus.delivered;

  @override
  void initState() {
    super.initState();
    _fetchProvider();
  }

  @override
  void dispose() {
    _reviewCtrl.dispose();
    super.dispose();
  }

  Future<void> _fetchProvider() async {
    final token = widget.controller.accessTokenForWallet;
    final providerId = booking.providerId;
    if (token == null || providerId == null || providerId.isEmpty) return;
    try {
      final p = await widget.controller.api
          .getProvider(accessToken: token, providerId: providerId);
      if (mounted) setState(() => _provider = p);
    } catch (_) {}
    final truckId = booking.truckId;
    if (truckId != null && truckId.isNotEmpty) {
      try {
        final t = await widget.controller.api
            .getTruck(accessToken: token, truckId: truckId);
        if (mounted) setState(() => _truck = t);
      } catch (_) {}
    }
  }

  void _bookAgain() {
    widget.controller.rebookFrom(booking);
    Navigator.of(context).pushReplacement(
      MaterialPageRoute(
        fullscreenDialog: true,
        builder: (_) => HaulingFlowScreen(
          controller: widget.controller,
          placesApi: widget.placesApi,
        ),
      ),
    );
  }

  Future<void> _submitReview() async {
    if (_rating <= 0) return;
    setState(() {
      _submitting = true;
      _error = null;
    });
    try {
      await widget.controller.submitReviewForBooking(
        bookingId: booking.id,
        rating: _rating,
        reviewText: _reviewCtrl.text.trim(),
        recommendsDriver: _recommends,
      );
      if (mounted) {
        setState(() {
          _submitting = false;
          _submitted = true;
        });
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Thanks for your review!')),
        );
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _submitting = false;
          _error = e.toString();
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final weightLabel =
        WeightCategory.fromName(booking.weightCategory)?.displayLabel ?? '';
    final truckLabel = HaulingTruckTypeOption.fromApiValue(
                booking.preferredTruckType)
            ?.displayLabel ??
        '';

    final truckSubtitle = _truck != null
        ? [_truck!.color, _truck!.make, _truck!.model]
            .where((s) => s.trim().isNotEmpty)
            .join(' · ')
        : '';

    return Scaffold(
      backgroundColor: Colors.white,
      appBar: AppBar(
        backgroundColor: Colors.white,
        elevation: 0,
        leading: Padding(
          padding: const EdgeInsets.only(left: 12),
          child: Container(
            decoration: const BoxDecoration(
              color: CustomerFigmaColors.surface,
              shape: BoxShape.circle,
            ),
            child: IconButton(
              icon:
                  const Icon(Icons.arrow_back, color: CustomerFigmaColors.text),
              onPressed: () => Navigator.of(context).pop(),
            ),
          ),
        ),
        title: const Text(
          'Trip Detail',
          style: TextStyle(
            color: CustomerFigmaColors.text,
            fontWeight: FontWeight.w800,
            fontSize: 18,
          ),
        ),
        centerTitle: true,
      ),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(20, 8, 20, 32),
        children: [
          // Driver header
          Center(
            child: Column(
              children: [
                CircleAvatar(
                  radius: 38,
                  backgroundColor: CustomerFigmaColors.primaryPale,
                  backgroundImage: (_provider?.profilePhotoUrl != null &&
                          _provider!.profilePhotoUrl!.isNotEmpty)
                      ? NetworkImage(_provider!.profilePhotoUrl!)
                      : null,
                  child: (_provider?.profilePhotoUrl == null ||
                          _provider!.profilePhotoUrl!.isEmpty)
                      ? const Icon(Icons.person,
                          size: 40, color: CustomerFigmaColors.primary)
                      : null,
                ),
                const SizedBox(height: 12),
                Text(
                  _provider?.displayName.trim().isNotEmpty == true
                      ? _provider!.displayName
                      : 'Driver',
                  style: const TextStyle(
                    color: CustomerFigmaColors.text,
                    fontSize: 24,
                    fontWeight: FontWeight.w800,
                  ),
                ),
                if (truckSubtitle.isNotEmpty || (_truck?.plateNumber.isNotEmpty ?? false)) ...[
                  const SizedBox(height: 4),
                  Row(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      Text(
                        truckSubtitle,
                        style: const TextStyle(
                          color: CustomerFigmaColors.text,
                          fontSize: 13,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      if (_truck?.plateNumber.isNotEmpty ?? false) ...[
                        const SizedBox(width: 8),
                        Text(
                          _truck!.plateNumber,
                          style: const TextStyle(
                            color: CustomerFigmaColors.primary,
                            fontSize: 13,
                            fontWeight: FontWeight.w800,
                          ),
                        ),
                      ],
                    ],
                  ),
                ],
              ],
            ),
          ),
          const SizedBox(height: 24),

          // Trip Completed reference + Date
          _LabelValueRow(
            label: (booking.status == HaulingBookingStatus.completed ||
                    booking.status == HaulingBookingStatus.delivered)
                ? 'Trip Completed:'
                : 'Trip Reference:',
            value: booking.id,
          ),
          const SizedBox(height: 12),
          _LabelValueRow(
            label: 'Date:',
            value: formatTripDate(
                booking.completedAt ?? booking.scheduledAt ?? booking.createdAt),
          ),
          const SizedBox(height: 16),

          // Route + fee card
          _Card(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                _RouteMini(booking: booking),
                const SizedBox(height: 12),
                const Divider(height: 1, color: CustomerFigmaColors.border),
                const SizedBox(height: 12),
                Row(
                  mainAxisAlignment: MainAxisAlignment.spaceBetween,
                  children: [
                    const Text(
                      'Trip Fee:',
                      style: TextStyle(
                        color: CustomerFigmaColors.text,
                        fontSize: 13,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                    Text(
                      booking.displayFareKobo > 0
                          ? '₦${booking.displayFareNaira.toStringAsFixed(2)}'
                          : '—',
                      style: const TextStyle(
                        color: CustomerFigmaColors.text,
                        fontSize: 14,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),

          // Receiver information
          if (booking.receiverName.isNotEmpty ||
              booking.receiverPhone.isNotEmpty) ...[
            const SizedBox(height: 24),
            const _SectionHeading('Receiver Information'),
            if (booking.receiverName.isNotEmpty)
              _StackedField('Full Name', booking.receiverName),
            if (booking.receiverPhone.isNotEmpty)
              _StackedField('Phone Number', booking.receiverPhone),
          ],

          // Package information
          if (booking.packageContent.isNotEmpty ||
              booking.packageSize.isNotEmpty ||
              booking.isFragile ||
              truckLabel.isNotEmpty ||
              weightLabel.isNotEmpty) ...[
            const SizedBox(height: 24),
            const _SectionHeading('Package Information'),
            if (booking.packageContent.isNotEmpty)
              _StackedField('Package Content', booking.packageContent),
            if (booking.packageSize.isNotEmpty)
              _StackedField('Package Size', booking.packageSize),
            if (truckLabel.isNotEmpty)
              _StackedField('Truck Type', truckLabel),
            if (weightLabel.isNotEmpty) _StackedField('Weight', weightLabel),
            if (booking.isFragile) ...[
              const SizedBox(height: 12),
              const Text(
                'Is the item fragile?',
                style: TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 14,
                  fontWeight: FontWeight.w800,
                ),
              ),
              const SizedBox(height: 2),
              const Text(
                'Let us know if the item needs to be handled with extra care.',
                style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 12),
              ),
              const SizedBox(height: 4),
              const Text(
                'Yes',
                style: TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 14,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ],
          ],

          // Cancellation reason
          if (booking.status == HaulingBookingStatus.cancelled &&
              (booking.cancelReason?.isNotEmpty ?? false)) ...[
            const SizedBox(height: 16),
            _Card(
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Icon(Icons.info_outline,
                      color: Color(0xFFD7493B), size: 18),
                  const SizedBox(width: 8),
                  Expanded(
                    child: Text(
                      'Cancelled: ${booking.cancelReason}',
                      style: const TextStyle(
                          color: Color(0xFFB7362A), fontSize: 13),
                    ),
                  ),
                ],
              ),
            ),
          ],

          // Review section (completed trips only)
          if (_canReview) ...[
            const SizedBox(height: 28),
            _ReviewSection(
              rating: _rating,
              onRate: _submitted ? null : (v) => setState(() => _rating = v),
              reviewController: _reviewCtrl,
              recommends: _recommends,
              onRecommend: _submitted
                  ? null
                  : (v) => setState(() => _recommends = v),
              readOnly: _submitted,
              error: _error,
            ),
            const SizedBox(height: 20),
            FigmaPrimaryButton(
              label: _submitted ? 'Review Submitted' : 'Submit Review',
              isLoading: _submitting,
              onPressed:
                  (_rating > 0 && !_submitting && !_submitted) ? _submitReview : null,
            ),
          ] else if (_canRebook) ...[
            const SizedBox(height: 28),
            FigmaPrimaryButton(label: 'Book again', onPressed: _bookAgain),
          ],
          const SizedBox(height: 8),
        ],
      ),
    );
  }
}

// ─── Sub-widgets ───────────────────────────────────────────────────────────

class _Card extends StatelessWidget {
  const _Card({required this.child});
  final Widget child;
  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(14),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.05),
            blurRadius: 14,
            offset: const Offset(0, 6),
          ),
        ],
      ),
      child: child,
    );
  }
}

class _LabelValueRow extends StatelessWidget {
  const _LabelValueRow({required this.label, required this.value});
  final String label;
  final String value;
  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: const TextStyle(
            color: CustomerFigmaColors.text,
            fontSize: 13,
            fontWeight: FontWeight.w800,
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Text(
            value,
            textAlign: TextAlign.right,
            style: const TextStyle(
              color: CustomerFigmaColors.muted,
              fontSize: 13,
            ),
            overflow: TextOverflow.ellipsis,
          ),
        ),
      ],
    );
  }
}

class _SectionHeading extends StatelessWidget {
  const _SectionHeading(this.title);
  final String title;
  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Text(
        title,
        style: const TextStyle(
          color: CustomerFigmaColors.text,
          fontSize: 17,
          fontWeight: FontWeight.w800,
        ),
      ),
    );
  }
}

class _StackedField extends StatelessWidget {
  const _StackedField(this.label, this.value);
  final String label;
  final String value;
  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: const TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 14,
              fontWeight: FontWeight.w800,
            ),
          ),
          const SizedBox(height: 2),
          Text(
            value,
            style: const TextStyle(
              color: CustomerFigmaColors.muted,
              fontSize: 14,
            ),
          ),
        ],
      ),
    );
  }
}

class _RouteMini extends StatelessWidget {
  const _RouteMini({required this.booking});
  final HaulageBooking booking;
  @override
  Widget build(BuildContext context) {
    final hasDropoff = booking.dropoffAddress.isNotEmpty;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Column(
          children: [
            const SizedBox(height: 2),
            _ring(filled: false),
            if (hasDropoff) ...[
              Container(
                width: 2,
                height: 30,
                color: CustomerFigmaColors.primary,
              ),
              _ring(filled: true),
            ],
          ],
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              _text('Pick-up',
                  booking.pickupAddress.isEmpty ? '—' : booking.pickupAddress),
              if (hasDropoff) ...[
                const SizedBox(height: 18),
                _text('Drop off (optional)', booking.dropoffAddress),
              ],
            ],
          ),
        ),
      ],
    );
  }

  Widget _ring({required bool filled}) {
    return Container(
      width: 16,
      height: 16,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: filled ? CustomerFigmaColors.primary : Colors.transparent,
        border: Border.all(color: CustomerFigmaColors.primary, width: 2.5),
      ),
      child: filled
          ? const Center(
              child: CircleAvatar(radius: 2.5, backgroundColor: Colors.white))
          : null,
    );
  }

  Widget _text(String label, String address) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label,
            style:
                const TextStyle(color: CustomerFigmaColors.muted, fontSize: 12)),
        const SizedBox(height: 2),
        Text(
          address,
          style: const TextStyle(
            color: CustomerFigmaColors.text,
            fontSize: 13,
            fontWeight: FontWeight.w700,
          ),
        ),
      ],
    );
  }
}

class _ReviewSection extends StatelessWidget {
  const _ReviewSection({
    required this.rating,
    required this.onRate,
    required this.reviewController,
    required this.recommends,
    required this.onRecommend,
    required this.readOnly,
    this.error,
  });

  final int rating;
  final ValueChanged<int>? onRate;
  final TextEditingController reviewController;
  final bool? recommends;
  final ValueChanged<bool>? onRecommend;
  final bool readOnly;
  final String? error;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Center(
          child: Column(
            children: [
              if (readOnly)
                const Align(
                  alignment: Alignment.centerLeft,
                  child: Text(
                    'Rating',
                    style: TextStyle(
                      color: CustomerFigmaColors.text,
                      fontSize: 18,
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                )
              else ...[
                const Text(
                  'How was your trip?',
                  style: TextStyle(
                    color: CustomerFigmaColors.text,
                    fontSize: 20,
                    fontWeight: FontWeight.w800,
                  ),
                ),
                const SizedBox(height: 4),
                const Text(
                  '(Describe your experience with 1 to 5 stars)',
                  style:
                      TextStyle(color: CustomerFigmaColors.muted, fontSize: 12),
                ),
              ],
            ],
          ),
        ),
        const SizedBox(height: 10),
        Align(
          alignment: readOnly ? Alignment.centerLeft : Alignment.center,
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              for (var star = 1; star <= 5; star++)
                GestureDetector(
                  onTap: onRate == null ? null : () => onRate!(star),
                  child: Icon(
                    star <= rating
                        ? Icons.star_rounded
                        : Icons.star_outline_rounded,
                    color: star <= rating
                        ? CustomerFigmaColors.primary
                        : CustomerFigmaColors.primarySoft,
                    size: 34,
                  ),
                ),
            ],
          ),
        ),
        const SizedBox(height: 16),
        const Text(
          'Describe your experience or Review',
          style: TextStyle(
            color: CustomerFigmaColors.text,
            fontSize: 14,
            fontWeight: FontWeight.w800,
          ),
        ),
        const SizedBox(height: 8),
        TextField(
          controller: reviewController,
          readOnly: readOnly,
          maxLines: 3,
          decoration: InputDecoration(
            hintText: 'Enter Description here',
            hintStyle: const TextStyle(color: CustomerFigmaColors.muted),
            filled: true,
            fillColor: CustomerFigmaColors.surface,
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide: BorderSide.none,
            ),
          ),
        ),
        const SizedBox(height: 16),
        const Text(
          'Do you Recommend this Driver?',
          style: TextStyle(
            color: CustomerFigmaColors.text,
            fontSize: 14,
            fontWeight: FontWeight.w800,
          ),
        ),
        const SizedBox(height: 10),
        Row(
          children: [
            _RecommendButton(
              label: 'Yes, I do!',
              selected: recommends == true,
              onTap: onRecommend == null ? null : () => onRecommend!(true),
            ),
            const SizedBox(width: 12),
            _RecommendButton(
              label: "No, I don't!",
              selected: recommends == false,
              onTap: onRecommend == null ? null : () => onRecommend!(false),
            ),
          ],
        ),
        if (error != null) ...[
          const SizedBox(height: 12),
          Text(error!,
              style: const TextStyle(color: Color(0xFFB7362A), fontSize: 13)),
        ],
      ],
    );
  }
}

class _RecommendButton extends StatelessWidget {
  const _RecommendButton({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final String label;
  final bool selected;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 22, vertical: 12),
        decoration: BoxDecoration(
          color: selected
              ? CustomerFigmaColors.primary
              : CustomerFigmaColors.primarySoft,
          borderRadius: BorderRadius.circular(24),
        ),
        child: Text(
          label,
          style: const TextStyle(
            color: Colors.white,
            fontSize: 13,
            fontWeight: FontWeight.w700,
          ),
        ),
      ),
    );
  }
}
