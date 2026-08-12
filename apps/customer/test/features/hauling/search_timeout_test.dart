import 'package:customer/features/hauling/models/hauling_models.dart';
import 'package:customer/features/hauling/state/hauling_booking_controller.dart';
import 'package:flutter_test/flutter_test.dart';

/// The 90s client-side search timeout cancels the booking server-side and shows
/// "No trucks found nearby". If that cancel is refused — most commonly because a
/// provider accepted while the timer was pending, or because the app restarted
/// and restarted the timer mid-trip — cancelling locally anyway strands the
/// booking: the customer sees "cancelled" while a driver is en route, loses
/// access to pickup verification, and the wallet hold is never released.
///
/// These pin which server statuses must override the local cancel.
void main() {
  group('shouldAdoptServerStatusOnTimeout', () {
    test('keeps giving up while the booking is genuinely still searching', () {
      for (final status in [
        HaulingBookingStatus.pendingMatch,
        HaulingBookingStatus.awaitingAcceptance,
      ]) {
        expect(
          HaulingBookingController.shouldAdoptServerStatusOnTimeout(status),
          isFalse,
          reason: '$status is still searching — the local cancel is correct',
        );
      }
    });

    test('adopts the server status once a provider is on the job', () {
      // These are the states where a local cancel strands real money.
      for (final status in [
        HaulingBookingStatus.accepted,
        HaulingBookingStatus.enRoutePickup,
        HaulingBookingStatus.arrivedAtPickup,
        HaulingBookingStatus.pickedUp,
        HaulingBookingStatus.enRouteDelivery,
        HaulingBookingStatus.delivered,
        HaulingBookingStatus.completed,
      ]) {
        expect(
          HaulingBookingController.shouldAdoptServerStatusOnTimeout(status),
          isTrue,
          reason: '$status means the trip is live or done — never fake-cancel',
        );
      }
    });

    test('adopts server-resolved terminal states rather than inventing one', () {
      for (final status in [
        HaulingBookingStatus.cancelled,
        HaulingBookingStatus.unmatched,
      ]) {
        expect(
          HaulingBookingController.shouldAdoptServerStatusOnTimeout(status),
          isTrue,
          reason: 'the server already resolved $status; show what it says',
        );
      }
    });

    test('covers every status, so a new one cannot be silently mishandled', () {
      for (final status in HaulingBookingStatus.values) {
        expect(
          () =>
              HaulingBookingController.shouldAdoptServerStatusOnTimeout(status),
          returnsNormally,
        );
      }
    });
  });
}
