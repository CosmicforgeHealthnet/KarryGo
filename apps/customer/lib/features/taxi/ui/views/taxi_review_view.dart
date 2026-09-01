import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/taxi_booking_controller.dart';
import '../widgets/taxi_flow_helpers.dart';

/// Rate the driver once the trip is complete.
///
/// The service withholds the driver's own review of this passenger until this
/// one is submitted, so nothing of theirs is shown here.
class TaxiReviewView extends StatefulWidget {
  const TaxiReviewView({
    super.key,
    required this.controller,
    required this.state,
  });

  final TaxiBookingController controller;
  final TaxiBookingState state;

  @override
  State<TaxiReviewView> createState() => _TaxiReviewViewState();
}

class _TaxiReviewViewState extends State<TaxiReviewView> {
  final _textCtrl = TextEditingController();

  @override
  void initState() {
    super.initState();
    _textCtrl.text = widget.state.reviewText;
  }

  @override
  void dispose() {
    _textCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.state;
    final controller = widget.controller;
    final booking = state.activeBooking;
    final isParcel = booking?.isParcel ?? state.rideMode.isParcel;

    return taxiFlowScaffold(
      body: ListView(
        children: [
          const SizedBox(height: 10),
          Center(
            child: Container(
              width: 84,
              height: 84,
              decoration: const BoxDecoration(
                color: CustomerFigmaColors.successSoft,
                shape: BoxShape.circle,
              ),
              child: const Icon(
                Icons.check_rounded,
                size: 40,
                color: CustomerFigmaColors.success,
              ),
            ),
          ),
          const SizedBox(height: 18),
          Center(
            child: Text(
              isParcel ? 'Package delivered' : 'You have arrived',
              style: const TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 20,
                fontWeight: FontWeight.w800,
              ),
            ),
          ),
          const SizedBox(height: 6),
          Center(
            child: Text(
              isParcel ? 'How was the delivery?' : 'How was your trip?',
              style: const TextStyle(
                color: CustomerFigmaColors.muted,
                fontSize: 13,
              ),
            ),
          ),

          if (booking != null && booking.displayFareKobo > 0) ...[
            const SizedBox(height: 14),
            Center(
              child: Text(
                formatHaulingFare(booking.displayFareKobo),
                style: const TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 24,
                  fontWeight: FontWeight.w900,
                ),
              ),
            ),
          ],

          const SizedBox(height: 24),
          Center(
            child: Row(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                for (var i = 1; i <= 5; i++)
                  IconButton(
                    onPressed: () => controller.setReviewRating(i),
                    icon: Icon(
                      i <= state.reviewRating
                          ? Icons.star_rounded
                          : Icons.star_border_rounded,
                      size: 36,
                      color: i <= state.reviewRating
                          ? CustomerFigmaColors.success
                          : CustomerFigmaColors.primarySoft,
                    ),
                  ),
              ],
            ),
          ),

          const SizedBox(height: 14),
          FigmaTextField(
            controller: _textCtrl,
            label: '',
            hintText: 'Tell us more (optional)',
            variant: FigmaTextFieldVariant.grey,
            onSubmitted: controller.setReviewText,
          ),

          const SizedBox(height: 20),
          taxiSectionLabel(
            isParcel
                ? 'Would you send with this driver again?'
                : 'Would you ride with this driver again?',
          ),
          const SizedBox(height: 6),
          Row(
            children: [
              Expanded(
                child: _ChoicePill(
                  label: 'Yes, I would',
                  selected: state.recommendsDriver == true,
                  onTap: () => controller.setRecommendsDriver(true),
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: _ChoicePill(
                  label: "No, I wouldn't",
                  selected: state.recommendsDriver == false,
                  onTap: () => controller.setRecommendsDriver(false),
                ),
              ),
            ],
          ),

          if (state.error != null) ...[
            const SizedBox(height: 14),
            Text(
              state.error!,
              style: const TextStyle(color: Color(0xFFB3261E), fontSize: 12),
            ),
          ],
          const SizedBox(height: 10),
        ],
      ),
      bottom: Column(
        children: [
          FigmaPrimaryButton(
            label: 'Submit review',
            isLoading: state.isLoading,
            onPressed: state.isLoading
                ? null
                : () {
                    controller.setReviewText(_textCtrl.text);
                    controller.submitReview();
                  },
          ),
          const SizedBox(height: 8),
          FigmaSecondaryButton(
            label: 'Skip',
            onPressed: controller.skipReview,
          ),
        ],
      ),
    );
  }
}

class _ChoicePill extends StatelessWidget {
  const _ChoicePill({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(24),
      child: Container(
        height: 44,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: selected
              ? CustomerFigmaColors.primary
              : CustomerFigmaColors.primaryTint,
          borderRadius: BorderRadius.circular(24),
        ),
        child: Text(
          label,
          style: TextStyle(
            color: selected ? Colors.white : CustomerFigmaColors.primary,
            fontSize: 13.5,
            fontWeight: FontWeight.w700,
          ),
        ),
      ),
    );
  }
}
