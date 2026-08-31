import 'package:cosmicforge_logistics_live_tracking/cosmicforge_logistics_live_tracking.dart';
import 'package:flutter_test/flutter_test.dart';

TrackPoint pointAt(double lat, double lng, {double? heading}) =>
    TrackPoint(position: TrackLatLng(lat, lng), headingDegrees: heading);

void main() {
  group('LiveTrackController', () {
    test('the first fix is drawn immediately rather than animated from nowhere', () {
      final controller = LiveTrackController();
      addTearDown(controller.dispose);

      expect(controller.hasFix, isFalse);
      expect(controller.renderedPosition, isNull);

      controller.ingest(pointAt(6.6, 3.35, heading: 90));

      expect(controller.hasFix, isTrue);
      expect(controller.renderedPosition, const TrackLatLng(6.6, 3.35));
      expect(controller.renderedBearing, 90);
    });

    // The whole point of this class: a marker must move between reports, not
    // teleport when each one lands.
    test('interpolates between reports instead of jumping', () {
      final controller = LiveTrackController(
        minInterpolation: const Duration(seconds: 4),
        maxInterpolation: const Duration(seconds: 4),
      );
      addTearDown(controller.dispose);

      controller.ingest(pointAt(6.5, 3.0));
      controller.ingest(pointAt(6.5, 4.0));

      // Immediately after the report the marker is still at the old position.
      expect(controller.renderedPosition!.lng, closeTo(3.0, 1e-9));

      controller.debugAdvance(const Duration(seconds: 2));
      final midway = controller.renderedPosition!;
      expect(midway.lng, greaterThan(3.2));
      expect(midway.lng, lessThan(3.8));

      controller.debugAdvance(const Duration(seconds: 2));
      expect(controller.renderedPosition!.lng, closeTo(4.0, 1e-6));
    });

    test('never overshoots the reported position', () {
      final controller = LiveTrackController(
        minInterpolation: const Duration(seconds: 2),
        maxInterpolation: const Duration(seconds: 2),
      );
      addTearDown(controller.dispose);

      controller.ingest(pointAt(6.5, 3.0));
      controller.ingest(pointAt(6.5, 4.0));
      controller.debugAdvance(const Duration(seconds: 30));

      expect(controller.renderedPosition!.lng, closeTo(4.0, 1e-6));
    });

    test('derives a bearing from movement when the device reports none', () {
      final controller = LiveTrackController(
        minInterpolation: const Duration(seconds: 1),
        maxInterpolation: const Duration(seconds: 1),
      );
      addTearDown(controller.dispose);

      controller.ingest(pointAt(6.5, 3.0));
      controller.ingest(pointAt(6.5, 4.0)); // due east
      controller.debugAdvance(const Duration(seconds: 1));

      expect(controller.renderedBearing, closeTo(90, 1));
    });

    test('prefers the reported heading over the derived one', () {
      final controller = LiveTrackController(
        minInterpolation: const Duration(seconds: 1),
        maxInterpolation: const Duration(seconds: 1),
      );
      addTearDown(controller.dispose);

      controller.ingest(pointAt(6.5, 3.0, heading: 0));
      // Moving east but the device says it is pointing north-east.
      controller.ingest(pointAt(6.5, 4.0, heading: 45));
      controller.debugAdvance(const Duration(seconds: 1));

      expect(controller.renderedBearing, closeTo(45, 1));
    });

    test('a repeated position does not restart the animation', () {
      final controller = LiveTrackController(
        minInterpolation: const Duration(seconds: 4),
        maxInterpolation: const Duration(seconds: 4),
      );
      addTearDown(controller.dispose);

      controller.ingest(pointAt(6.5, 3.0));
      controller.ingest(pointAt(6.5, 4.0));
      controller.debugAdvance(const Duration(seconds: 4));
      expect(controller.renderedPosition!.lng, closeTo(4.0, 1e-6));

      // A stationary vehicle re-reporting the same spot must not make the
      // marker stutter.
      controller.ingest(pointAt(6.5, 4.0));
      controller.debugAdvance(const Duration(seconds: 1));
      expect(controller.renderedPosition!.lng, closeTo(4.0, 1e-6));
    });

    test('ignores a report with no usable fix', () {
      final controller = LiveTrackController();
      addTearDown(controller.dispose);

      controller.ingest(pointAt(0, 0)); // null island
      expect(controller.hasFix, isFalse);
    });

    test('snaps onto the route so the marker stays on the road', () {
      final controller = LiveTrackController(
        minInterpolation: const Duration(seconds: 1),
        maxInterpolation: const Duration(seconds: 1),
      );
      addTearDown(controller.dispose);

      // A route due east along the equator.
      controller.setRoute(const [TrackLatLng(0, 0), TrackLatLng(0, 1)]);

      // A fix a few tens of metres north of the road.
      controller.ingest(const TrackPoint(position: TrackLatLng(0.0003, 0.5)));

      expect(controller.renderedPosition!.lat, closeTo(0, 1e-6));
      expect(controller.renderedPosition!.lng, closeTo(0.5, 1e-6));
    });

    test('leaves a genuine detour unsnapped', () {
      final controller = LiveTrackController();
      addTearDown(controller.dispose);

      controller.setRoute(const [TrackLatLng(0, 0), TrackLatLng(0, 1)]);
      // Well off the route: pinning this to the road would misreport where
      // the vehicle actually is.
      const detour = TrackLatLng(0.05, 0.5);
      controller.ingest(const TrackPoint(position: detour));

      expect(controller.renderedPosition!.lat, closeTo(detour.lat, 1e-6));
    });

    test('reports staleness once reports dry up', () {
      final controller = LiveTrackController(staleAfter: Duration.zero);
      addTearDown(controller.dispose);

      expect(controller.isStale, isFalse, reason: 'nothing received yet is not stale');

      controller.ingest(pointAt(6.6, 3.35));
      expect(controller.isStale, isTrue);
    });

    test('reset clears everything for the next trip', () {
      final controller = LiveTrackController();
      addTearDown(controller.dispose);

      controller.setRoute(const [TrackLatLng(0, 0), TrackLatLng(0, 1)]);
      controller.ingest(pointAt(6.6, 3.35, heading: 90));
      controller.reset();

      expect(controller.hasFix, isFalse);
      expect(controller.renderedPosition, isNull);
      expect(controller.renderedBearing, 0);
      expect(controller.route, isEmpty);
    });

    test('notifies listeners as it moves', () {
      final controller = LiveTrackController(
        minInterpolation: const Duration(seconds: 2),
        maxInterpolation: const Duration(seconds: 2),
      );
      addTearDown(controller.dispose);

      var notifications = 0;
      controller.addListener(() => notifications++);

      controller.ingest(pointAt(6.5, 3.0));
      controller.ingest(pointAt(6.5, 4.0));
      controller.debugAdvance(const Duration(milliseconds: 500));

      expect(notifications, greaterThanOrEqualTo(3));
    });
  });

  group('TrackPoint.fromJson', () {
    test('parses a websocket location frame', () {
      final point = TrackPoint.fromJson({
        'type': 'location_update',
        'entity_id': 'p1',
        'provider_id': 'p1',
        'lat': 6.6018,
        'lng': 3.3515,
        'heading': 271.5,
        'speed': 12.25,
        'accuracy': 4.0,
        'updated_at': '2026-08-13T09:00:00Z',
      });

      expect(point, isNotNull);
      expect(point!.lat, 6.6018);
      expect(point.headingDegrees, 271.5);
      expect(point.speedMetersPerSecond, 12.25);
      expect(point.recordedAt, DateTime.utc(2026, 8, 13, 9));
    });

    test('parses the REST snapshot, whose updated_at is unix seconds', () {
      final point = TrackPoint.fromJson({
        'booking_id': 'b1',
        'lat': 6.6018,
        'lng': 3.3515,
        'updated_at': 1755075600,
        'available': true,
      });

      expect(point, isNotNull);
      expect(point!.recordedAt!.isUtc, isTrue);
    });

    test('an unavailable snapshot is not a position', () {
      final point = TrackPoint.fromJson({
        'lat': 0.0,
        'lng': 0.0,
        'available': false,
      });
      expect(point, isNull);
    });

    test('rejects frames with no coordinate', () {
      expect(TrackPoint.fromJson({'type': 'provider_offline'}), isNull);
      expect(TrackPoint.fromJson({'lat': 6.6}), isNull);
    });

    test('a device that reports no heading leaves it null, not zero', () {
      final point = TrackPoint.fromJson({'lat': 6.6, 'lng': 3.35});
      expect(point!.headingDegrees, isNull);
    });
  });
}
