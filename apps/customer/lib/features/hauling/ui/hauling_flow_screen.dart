import 'package:flutter/material.dart';

import '../data/places_api.dart';
import '../state/hauling_booking_controller.dart';
import 'views/hauling_active_trip_view.dart';
import 'views/hauling_cancelled_view.dart';
import 'views/hauling_completed_view.dart';
import 'views/hauling_details_view.dart';
import 'views/hauling_error_view.dart';
import 'views/hauling_location_entry_view.dart';
import 'views/hauling_package_info_view.dart';
import 'views/hauling_payment_processing_view.dart';
import 'views/hauling_payment_view.dart';
import 'views/hauling_review_view.dart';
import 'views/hauling_searching_view.dart';
import 'views/hauling_tier_selection_view.dart';
import 'views/hauling_unavailable_view.dart';

/// Entry point for the truck hauling booking flow.
/// Push this as a full-screen route when the user taps "Find a Truck".
class HaulingFlowScreen extends StatelessWidget {
  const HaulingFlowScreen({
    super.key,
    required this.controller,
    required this.placesApi,
  });

  final HaulingBookingController controller;
  final PlacesApi placesApi;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: controller,
      builder: (context, _) {
        final state = controller.state;
        return switch (state.status) {
          HaulingFlowStatus.idle ||
          HaulingFlowStatus.locationEntry ||
          HaulingFlowStatus.checkingAvailability =>
            HaulingLocationEntryView(controller: controller, placesApi: placesApi),

          HaulingFlowStatus.unavailable =>
            HaulingUnavailableView(
              controller: controller,
              count: state.availability?.count ?? 0,
            ),

          HaulingFlowStatus.tierSelection =>
            HaulingTierSelectionView(controller: controller, state: state),

          HaulingFlowStatus.details =>
            HaulingDetailsView(controller: controller, state: state),

          HaulingFlowStatus.packageInfo =>
            HaulingPackageInfoView(controller: controller, state: state),

          HaulingFlowStatus.payment =>
            HaulingPaymentView(controller: controller),

          HaulingFlowStatus.paymentProcessing =>
            HaulingPaymentProcessingView(controller: controller),

          HaulingFlowStatus.searching =>
            HaulingSearchingView(controller: controller, state: state),

          HaulingFlowStatus.activeTrip =>
            HaulingActiveTripView(controller: controller, state: state),

          HaulingFlowStatus.delivered ||
          HaulingFlowStatus.review =>
            HaulingReviewView(controller: controller, state: state),

          HaulingFlowStatus.completed =>
            HaulingCompletedView(controller: controller, state: state),

          HaulingFlowStatus.cancelled =>
            HaulingCancelledView(controller: controller, state: state),

          HaulingFlowStatus.error =>
            HaulingErrorView(
              controller: controller,
              message: state.error ?? 'Something went wrong.',
            ),
        };
      },
    );
  }
}
