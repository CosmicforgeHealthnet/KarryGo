import 'package:flutter/material.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/hauling_booking_controller.dart';
import '../widgets/hauling_map_widget.dart';
import '../widgets/hauling_trip_sheet_widgets.dart';

/// "You have arrived" rating screen (Figma 1776): map behind, sheet with the
/// belongings banner, centered driver, trip reference/date, 1-5 stars, review
/// text, recommend pills and Submit Review.
class HaulingReviewView extends StatefulWidget {
  const HaulingReviewView({super.key, required this.controller, required this.state});

  final HaulingBookingController controller;
  final HaulingBookingState state;

  @override
  State<HaulingReviewView> createState() => _HaulingReviewViewState();
}

class _HaulingReviewViewState extends State<HaulingReviewView> {
  final _reviewCtrl = TextEditingController();

  HaulingBookingController get _ctrl => widget.controller;
  HaulingBookingState get _state => widget.state;

  @override
  void initState() {
    super.initState();
    _reviewCtrl.text = _state.reviewText;
  }

  @override
  void dispose() {
    _reviewCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final booking = _state.activeBooking;
    final provider = _state.providerSnapshot;
    final truck = _state.truckSnapshot;
    final topInset = MediaQuery.paddingOf(context).top;

    return Scaffold(
      backgroundColor: Colors.white,
      bottomNavigationBar: const HaulingFlowBottomNav(),
      body: Stack(
        children: [
          Positioned.fill(
            child: HaulingMapWidget(
              pickupLatLng: booking != null
                  ? LatLng(booking.pickupLat, booking.pickupLng)
                  : null,
              dropoffLatLng: booking != null
                  ? LatLng(booking.dropoffLat, booking.dropoffLng)
                  : null,
            ),
          ),

          // Back arrow over the map (leaves the review — trip completes).
          Positioned(
            top: topInset + 8,
            left: 16,
            child: GestureDetector(
              onTap: _ctrl.skipReview,
              child: Container(
                width: 40,
                height: 40,
                decoration: BoxDecoration(
                  color: Colors.white,
                  shape: BoxShape.circle,
                  boxShadow: [
                    BoxShadow(
                      color: Colors.black.withValues(alpha: 0.12),
                      blurRadius: 8,
                    ),
                  ],
                ),
                child: const Icon(Icons.arrow_back,
                    color: CustomerFigmaColors.text, size: 22),
              ),
            ),
          ),

          // Rating sheet
          Positioned.fill(
            top: topInset + 56,
            child: Container(
              decoration: const BoxDecoration(
                color: Colors.white,
                borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
              ),
              clipBehavior: Clip.antiAlias,
              child: SingleChildScrollView(
                padding: const EdgeInsets.only(top: 20, bottom: 20),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    const Padding(
                      padding: EdgeInsets.symmetric(horizontal: 20),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            'You have arrived',
                            style: TextStyle(
                              color: CustomerFigmaColors.text,
                              fontSize: 20,
                              fontWeight: FontWeight.w800,
                            ),
                          ),
                          SizedBox(height: 4),
                          Text(
                            'You have reached your destination.',
                            style: TextStyle(
                              color: CustomerFigmaColors.muted,
                              fontSize: 13,
                            ),
                          ),
                        ],
                      ),
                    ),
                    const SizedBox(height: 14),

                    // Full-bleed green belongings banner
                    Container(
                      color: CustomerFigmaColors.primary,
                      padding: const EdgeInsets.symmetric(
                          horizontal: 16, vertical: 10),
                      child: const Text(
                        "Make sure you don't leave any of your belongings behind.",
                        textAlign: TextAlign.center,
                        style: TextStyle(
                          color: Colors.white,
                          fontSize: 13,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    ),
                    const SizedBox(height: 22),

                    if (provider != null) ...[
                      Center(
                        child:
                            HaulingDriverAvatar(provider: provider, radius: 40),
                      ),
                      const SizedBox(height: 14),
                      Center(
                        child: Text(
                          provider.displayName,
                          style: const TextStyle(
                            color: CustomerFigmaColors.text,
                            fontSize: 26,
                            fontWeight: FontWeight.w800,
                          ),
                        ),
                      ),
                      const SizedBox(height: 6),
                      if (truck != null)
                        Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 20),
                          child: HaulingVehicleLine(truck: truck, centered: true),
                        ),
                      const SizedBox(height: 22),
                    ],

                    Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 20),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          if (booking != null) ...[
                            _LabelValueRow(
                              label: 'Trip Completed:',
                              value: booking.id,
                            ),
                            const SizedBox(height: 14),
                            _LabelValueRow(
                              label: 'Date:',
                              value: _formatArrivalDate(
                                  booking.completedAt ?? DateTime.now()),
                            ),
                            const SizedBox(height: 22),
                          ],

                          const Center(
                            child: Text(
                              'How was your trip?',
                              style: TextStyle(
                                color: CustomerFigmaColors.text,
                                fontSize: 18,
                                fontWeight: FontWeight.w800,
                              ),
                            ),
                          ),
                          const SizedBox(height: 6),
                          const Center(
                            child: Text(
                              '(Describe your experience with 1 to 5 stars)',
                              style: TextStyle(
                                color: CustomerFigmaColors.text,
                                fontSize: 12,
                              ),
                            ),
                          ),
                          const SizedBox(height: 12),

                          // Green rating stars
                          Row(
                            mainAxisAlignment: MainAxisAlignment.center,
                            children: List.generate(5, (i) {
                              final star = i + 1;
                              return GestureDetector(
                                onTap: () {
                                  setState(() {});
                                  _ctrl.setReviewRating(star);
                                },
                                child: Padding(
                                  padding:
                                      const EdgeInsets.symmetric(horizontal: 7),
                                  child: Icon(
                                    Icons.star_rounded,
                                    size: 42,
                                    color: star <= _state.reviewRating
                                        ? CustomerFigmaColors.primary
                                        : const Color(0xFFD9D9D9),
                                  ),
                                ),
                              );
                            }),
                          ),
                          const SizedBox(height: 18),

                          const Text(
                            'Describe your experience or Review',
                            style: TextStyle(
                              color: CustomerFigmaColors.text,
                              fontSize: 13,
                              fontWeight: FontWeight.w800,
                            ),
                          ),
                          const SizedBox(height: 10),
                          TextField(
                            controller: _reviewCtrl,
                            minLines: 4,
                            maxLines: 5,
                            onChanged: (v) => _ctrl.setReviewText(v),
                            decoration: InputDecoration(
                              hintText: 'Enter Description here',
                              hintStyle: const TextStyle(
                                color: CustomerFigmaColors.muted,
                                fontSize: 13,
                              ),
                              filled: true,
                              fillColor: const Color(0xFFECEDEC),
                              border: OutlineInputBorder(
                                borderRadius: BorderRadius.circular(10),
                                borderSide: BorderSide.none,
                              ),
                              contentPadding: const EdgeInsets.all(14),
                            ),
                          ),
                          const SizedBox(height: 18),

                          const Text(
                            'Do you Recommend this Driver?',
                            style: TextStyle(
                              color: CustomerFigmaColors.text,
                              fontSize: 13,
                              fontWeight: FontWeight.w800,
                            ),
                          ),
                          const SizedBox(height: 12),
                          Row(
                            children: [
                              _RecommendPill(
                                label: 'Yes, I do!',
                                selected: _state.recommendsDriver == true,
                                onTap: () {
                                  setState(() {});
                                  _ctrl.setRecommendsDriver(
                                    _state.recommendsDriver == true ? null : true,
                                  );
                                },
                              ),
                              const SizedBox(width: 14),
                              _RecommendPill(
                                label: "No, I don't!",
                                selected: _state.recommendsDriver == false,
                                onTap: () {
                                  setState(() {});
                                  _ctrl.setRecommendsDriver(
                                    _state.recommendsDriver == false
                                        ? null
                                        : false,
                                  );
                                },
                              ),
                            ],
                          ),
                          const SizedBox(height: 24),

                          if (_state.error != null) ...[
                            Text(
                              _state.error!,
                              style: const TextStyle(
                                  color: Colors.red, fontSize: 12),
                              textAlign: TextAlign.center,
                            ),
                            const SizedBox(height: 8),
                          ],
                          HaulingPillButton(
                            label: 'Submit Review',
                            isLoading: _state.isLoading,
                            onPressed: _state.reviewRating > 0
                                ? _ctrl.submitReview
                                : null,
                          ),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// "Monday, 22 May 2026" (no intl dependency in the app).
String _formatArrivalDate(DateTime dt) {
  final d = dt.toLocal();
  const weekdays = [
    'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday',
  ];
  const months = [
    'January', 'February', 'March', 'April', 'May', 'June',
    'July', 'August', 'September', 'October', 'November', 'December',
  ];
  return '${weekdays[d.weekday - 1]}, ${d.day} ${months[d.month - 1]} ${d.year}';
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
            fontSize: 14,
            fontWeight: FontWeight.w800,
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Text(
            value,
            textAlign: TextAlign.right,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
              color: CustomerFigmaColors.muted,
              fontSize: 13,
            ),
          ),
        ),
      ],
    );
  }
}

class _RecommendPill extends StatelessWidget {
  const _RecommendPill({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 22, vertical: 11),
        decoration: BoxDecoration(
          color: selected
              ? CustomerFigmaColors.primary
              : CustomerFigmaColors.primaryPale,
          borderRadius: BorderRadius.circular(999),
        ),
        child: Text(
          label,
          style: const TextStyle(
            color: Colors.white,
            fontSize: 14,
            fontWeight: FontWeight.w800,
          ),
        ),
      ),
    );
  }
}
