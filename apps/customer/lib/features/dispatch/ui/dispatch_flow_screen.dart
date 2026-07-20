import 'package:flutter/material.dart';

import '../../hauling/data/places_api.dart';
import '../state/dispatch_booking_controller.dart';
import 'views/dispatch_active_trip_view.dart';
import 'views/dispatch_cancelled_view.dart';
import 'views/dispatch_completed_view.dart';
import 'views/dispatch_error_view.dart';
import 'views/dispatch_location_entry_view.dart';
import 'views/dispatch_package_info_view.dart';
import 'views/dispatch_payment_processing_view.dart';
import 'views/dispatch_payment_view.dart';
import 'views/dispatch_review_view.dart';
import 'views/dispatch_searching_view.dart';
import 'views/dispatch_unavailable_view.dart';

/// Entry point for the bike delivery (dispatch) booking flow.
/// Push this as a full-screen route when the user taps "Bike Delivery".
class DispatchFlowScreen extends StatelessWidget {
  const DispatchFlowScreen({
    super.key,
    required this.controller,
    required this.placesApi,
  });

  final DispatchBookingController controller;
  final PlacesApi placesApi;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: controller,
      builder: (context, _) {
        final state = controller.state;
        return switch (state.status) {
          DispatchFlowStatus.idle ||
          DispatchFlowStatus.locationEntry ||
          DispatchFlowStatus.checkingAvailability =>
            DispatchLocationEntryView(controller: controller, placesApi: placesApi),

          DispatchFlowStatus.unavailable =>
            DispatchUnavailableView(
              controller: controller,
              count: state.availability?.count ?? 0,
            ),

          DispatchFlowStatus.packageInfo =>
            DispatchPackageInfoView(controller: controller, state: state),

          DispatchFlowStatus.payment =>
            DispatchPaymentView(controller: controller),

          DispatchFlowStatus.paymentProcessing =>
            DispatchPaymentProcessingView(controller: controller),

          DispatchFlowStatus.searching =>
            DispatchSearchingView(controller: controller, state: state),

          DispatchFlowStatus.activeTrip =>
            DispatchActiveTripView(controller: controller, state: state),

          DispatchFlowStatus.review =>
            DispatchReviewView(controller: controller, state: state),

          DispatchFlowStatus.completed =>
            DispatchCompletedView(controller: controller, state: state),

          DispatchFlowStatus.cancelled =>
            DispatchCancelledView(controller: controller, state: state),

          DispatchFlowStatus.error =>
            DispatchErrorView(
              controller: controller,
              message: state.error ?? 'Something went wrong.',
            ),
        };
      },
    );
  }
}
