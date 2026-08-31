import 'package:cosmicforge_logistics_live_tracking/cosmicforge_logistics_live_tracking.dart';
import 'package:flutter_test/flutter_test.dart';

const driverStart = TrackLatLng(0, 0);
const pickup = TrackLatLng(0, 1);
const dropoff = TrackLatLng(0, 2);

void main() {
  group('TripProgress', () {
    // The defect this replaces: the old bar was frozen until the load was
    // aboard, so the whole approach leg showed no movement at all.
    test('advances while the driver is still on the way to collect', () {
      final progress = TripProgress();

      final atStart = progress.update(
        leg: TripLeg.toPickup,
        vehicle: driverStart,
        origin: driverStart,
        destination: pickup,
      );
      expect(atStart.legFraction, closeTo(0, 0.01));

      final halfway = progress.update(
        leg: TripLeg.toPickup,
        vehicle: const TrackLatLng(0, 0.5),
        origin: driverStart,
        destination: pickup,
      );
      expect(halfway.legFraction, closeTo(0.5, 0.01));
      // The approach leg is the first half of the whole trip.
      expect(halfway.overallFraction, closeTo(0.25, 0.01));
    });

    test('restarts at zero when the trip moves to the delivery leg', () {
      final progress = TripProgress();

      progress.update(
        leg: TripLeg.toPickup,
        vehicle: const TrackLatLng(0, 0.9),
        origin: driverStart,
        destination: pickup,
      );

      final delivery = progress.update(
        leg: TripLeg.toDropoff,
        vehicle: pickup,
        origin: pickup,
        destination: dropoff,
      );

      expect(delivery.legFraction, closeTo(0, 0.01));
      // But overall progress does not go backwards: the delivery leg starts
      // at the halfway mark.
      expect(delivery.overallFraction, closeTo(0.5, 0.01));
    });

    // GPS jitter nudges a vehicle backwards; a bar that slides back reads as
    // a fault.
    test('never regresses within a leg', () {
      final progress = TripProgress();

      final ahead = progress.update(
        leg: TripLeg.toDropoff,
        vehicle: const TrackLatLng(0, 1.6),
        origin: pickup,
        destination: dropoff,
      );
      expect(ahead.legFraction, closeTo(0.6, 0.01));

      final jittered = progress.update(
        leg: TripLeg.toDropoff,
        vehicle: const TrackLatLng(0, 1.4),
        origin: pickup,
        destination: dropoff,
      );
      expect(jittered.legFraction, closeTo(0.6, 0.01));
    });

    test('measures along the road route when one is known', () {
      final progress = TripProgress();
      // An L-shaped route: straight-line progress would read differently.
      final route = <TrackLatLng>[
        const TrackLatLng(0, 0),
        const TrackLatLng(0, 1),
        const TrackLatLng(1, 1),
      ];

      final atCorner = progress.update(
        leg: TripLeg.toDropoff,
        vehicle: const TrackLatLng(0, 1),
        origin: const TrackLatLng(0, 0),
        destination: const TrackLatLng(1, 1),
        route: route,
      );

      // Half the route travelled, even though the corner is much closer to
      // the destination as the crow flies.
      expect(atCorner.legFraction, closeTo(0.5, 0.02));
    });

    test('falls back to straight-line measurement with no route', () {
      final progress = TripProgress();

      final snapshot = progress.update(
        leg: TripLeg.toDropoff,
        vehicle: const TrackLatLng(0, 1.5),
        origin: pickup,
        destination: dropoff,
      );

      expect(snapshot.legFraction, closeTo(0.5, 0.01));
      expect(snapshot.remainingKm, isNotNull);
      expect(snapshot.etaMinutes, isNotNull);
    });

    test('holds its last value when there is no vehicle fix', () {
      final progress = TripProgress();

      progress.update(
        leg: TripLeg.toDropoff,
        vehicle: const TrackLatLng(0, 1.5),
        origin: pickup,
        destination: dropoff,
      );
      final withoutFix = progress.update(
        leg: TripLeg.toDropoff,
        vehicle: null,
        origin: pickup,
        destination: dropoff,
      );

      expect(withoutFix.legFraction, closeTo(0.5, 0.01));
    });

    test('reset clears progress for a new trip', () {
      final progress = TripProgress();

      progress.update(
        leg: TripLeg.toDropoff,
        vehicle: const TrackLatLng(0, 1.8),
        origin: pickup,
        destination: dropoff,
      );
      progress.reset();

      final fresh = progress.update(
        leg: TripLeg.toDropoff,
        vehicle: pickup,
        origin: pickup,
        destination: dropoff,
      );
      expect(fresh.legFraction, closeTo(0, 0.01));
    });

    test('a zero-length leg does not divide by zero', () {
      final progress = TripProgress();

      final snapshot = progress.update(
        leg: TripLeg.toDropoff,
        vehicle: pickup,
        origin: pickup,
        destination: pickup,
      );

      expect(snapshot.legFraction, isNotNaN);
      expect(snapshot.legFraction, inInclusiveRange(0, 1));
    });
  });
}
