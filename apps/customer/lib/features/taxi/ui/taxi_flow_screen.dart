import 'package:flutter/material.dart';

import '../../hauling/data/places_api.dart';
import '../state/taxi_booking_controller.dart';
import 'views/taxi_active_trip_view.dart';
import 'views/taxi_cancelled_view.dart';
import 'views/taxi_completed_view.dart';
import 'views/taxi_error_view.dart';
import 'views/taxi_fare_selection_view.dart';
import 'views/taxi_location_entry_view.dart';
import 'views/taxi_mode_selection_view.dart';
import 'views/taxi_parcel_details_view.dart';
import 'views/taxi_review_view.dart';
import 'views/taxi_ride_options_view.dart';
import 'views/taxi_searching_view.dart';
import 'views/taxi_unavailable_view.dart';

/// Entry point for the car ride (taxi) booking flow.
/// Push this as a full-screen route when the user taps "Car Ride".
class TaxiFlowScreen extends StatelessWidget {
  const TaxiFlowScreen({
    super.key,
    required this.controller,
    required this.placesApi,
  });

  final TaxiBookingController controller;
  final PlacesApi placesApi;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: controller,
      builder: (context, _) {
        final state = controller.state;
        return switch (state.status) {
          TaxiFlowStatus.idle ||
          TaxiFlowStatus.modeSelection => TaxiModeSelectionView(
            controller: controller,
            state: state,
          ),

          TaxiFlowStatus.locationEntry ||
          TaxiFlowStatus.checkingAvailability => TaxiLocationEntryView(
            controller: controller,
            placesApi: placesApi,
          ),

          TaxiFlowStatus.unavailable => TaxiUnavailableView(
            controller: controller,
            message: state.error,
          ),

          TaxiFlowStatus.fareSelection => TaxiFareSelectionView(
            controller: controller,
            state: state,
          ),

          TaxiFlowStatus.parcelDetails => TaxiParcelDetailsView(
            controller: controller,
            state: state,
          ),

          TaxiFlowStatus.rideOptions => TaxiRideOptionsView(
            controller: controller,
            state: state,
          ),

          TaxiFlowStatus.searching => TaxiSearchingView(
            controller: controller,
            state: state,
          ),

          TaxiFlowStatus.activeTrip => TaxiActiveTripView(
            controller: controller,
            state: state,
          ),

          TaxiFlowStatus.review => TaxiReviewView(
            controller: controller,
            state: state,
          ),

          TaxiFlowStatus.completed => TaxiCompletedView(
            controller: controller,
            state: state,
          ),

          TaxiFlowStatus.cancelled => TaxiCancelledView(
            controller: controller,
            state: state,
          ),

          TaxiFlowStatus.error => TaxiErrorView(
            controller: controller,
            message: state.error ?? 'Something went wrong.',
          ),
        };
      },
    );
  }
}
