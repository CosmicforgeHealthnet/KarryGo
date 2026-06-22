import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/hauling_booking_controller.dart';
import '../widgets/hauling_flow_helpers.dart';

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
    final provider = _state.providerSnapshot;

    return haulingFlowScaffold(
      title: 'Rate your trip',
      onBack: null,
      body: SingleChildScrollView(
        child: Column(
          children: [
            const SizedBox(height: 20),
            const Icon(Icons.check_circle, color: CustomerFigmaColors.primary, size: 60),
            const SizedBox(height: 12),
            const Text(
              'You have arrived!',
              style: TextStyle(
                fontSize: 22,
                fontWeight: FontWeight.w700,
                color: CustomerFigmaColors.text,
              ),
            ),
            const SizedBox(height: 4),
            Text(
              'Your cargo has been delivered to ${_state.activeBooking?.dropoffAddress ?? "destination"}',
              textAlign: TextAlign.center,
              style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
            ),
            const SizedBox(height: 24),

            if (provider != null) ...[
              CircleAvatar(
                radius: 30,
                backgroundImage: provider.profilePhotoUrl != null
                    ? NetworkImage(provider.profilePhotoUrl!)
                    : null,
                backgroundColor: CustomerFigmaColors.primaryTint,
                child: provider.profilePhotoUrl == null
                    ? Text(
                        provider.firstName.isNotEmpty ? provider.firstName[0] : '?',
                        style: const TextStyle(fontSize: 24, color: CustomerFigmaColors.primary),
                      )
                    : null,
              ),
              const SizedBox(height: 8),
              Text(
                provider.displayName,
                style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600, color: CustomerFigmaColors.text),
              ),
              const SizedBox(height: 20),
            ],

            haulingSectionLabel('How was your experience?'),
            const SizedBox(height: 12),
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
                    padding: const EdgeInsets.symmetric(horizontal: 6),
                    child: Icon(
                      star <= _state.reviewRating ? Icons.star_rounded : Icons.star_outline_rounded,
                      size: 40,
                      color: star <= _state.reviewRating
                          ? Colors.amber
                          : CustomerFigmaColors.border,
                    ),
                  ),
                );
              }),
            ),
            const SizedBox(height: 24),

            haulingSectionLabel('Leave a review (optional)'),
            const SizedBox(height: 8),
            TextField(
              controller: _reviewCtrl,
              maxLines: 3,
              onChanged: (v) => _ctrl.setReviewText(v),
              decoration: InputDecoration(
                hintText: 'Tell us about your experience...',
                hintStyle: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
                filled: true,
                fillColor: Colors.white,
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(10),
                  borderSide: const BorderSide(color: CustomerFigmaColors.border),
                ),
                enabledBorder: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(10),
                  borderSide: const BorderSide(color: CustomerFigmaColors.border),
                ),
                focusedBorder: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(10),
                  borderSide: const BorderSide(color: CustomerFigmaColors.primary),
                ),
                contentPadding: const EdgeInsets.all(14),
              ),
            ),
            const SizedBox(height: 20),

            haulingSectionLabel('Would you recommend this driver?'),
            const SizedBox(height: 8),
            Row(
              children: [
                _RecommendPill(
                  label: 'Yes',
                  icon: Icons.thumb_up_outlined,
                  selected: _state.recommendsDriver == true,
                  onTap: () {
                    setState(() {});
                    _ctrl.setRecommendsDriver(
                      _state.recommendsDriver == true ? null : true,
                    );
                  },
                ),
                const SizedBox(width: 12),
                _RecommendPill(
                  label: 'No',
                  icon: Icons.thumb_down_outlined,
                  selected: _state.recommendsDriver == false,
                  onTap: () {
                    setState(() {});
                    _ctrl.setRecommendsDriver(
                      _state.recommendsDriver == false ? null : false,
                    );
                  },
                ),
              ],
            ),
            const SizedBox(height: 8),
          ],
        ),
      ),
      bottom: Column(
        children: [
          if (_state.error != null) ...[
            Text(
              _state.error!,
              style: const TextStyle(color: Colors.red, fontSize: 12),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 8),
          ],
          FigmaPrimaryButton(
            label: 'Submit Review',
            isLoading: _state.isLoading,
            onPressed: _state.reviewRating > 0 ? _ctrl.submitReview : null,
          ),
          const SizedBox(height: 8),
          TextButton(
            onPressed: _ctrl.skipReview,
            child: const Text(
              'Skip',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 14),
            ),
          ),
        ],
      ),
    );
  }
}

class _RecommendPill extends StatelessWidget {
  const _RecommendPill({required this.label, required this.icon, required this.selected, required this.onTap});

  final String label;
  final IconData icon;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 10),
        decoration: BoxDecoration(
          color: selected ? CustomerFigmaColors.primaryTint : Colors.white,
          borderRadius: BorderRadius.circular(20),
          border: Border.all(
            color: selected ? CustomerFigmaColors.primary : CustomerFigmaColors.border,
            width: 1.5,
          ),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 18, color: selected ? CustomerFigmaColors.primary : CustomerFigmaColors.muted),
            const SizedBox(width: 6),
            Text(
              label,
              style: TextStyle(
                color: selected ? CustomerFigmaColors.primary : CustomerFigmaColors.text,
                fontWeight: selected ? FontWeight.w600 : FontWeight.w400,
                fontSize: 14,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
