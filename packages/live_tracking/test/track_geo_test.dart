import 'package:cosmicforge_logistics_live_tracking/cosmicforge_logistics_live_tracking.dart';
import 'package:flutter_test/flutter_test.dart';

const ikeja = TrackLatLng(6.6018, 3.3515);
const victoriaIsland = TrackLatLng(6.4281, 3.4219);

void main() {
  group('haversineKm', () {
    test('matches the value the services compute', () {
      // The Go side computes 20.8217 km for the same pair; client and server
      // must agree or the progress bar and the fare would disagree.
      expect(TrackGeo.haversineKm(ikeja, victoriaIsland), closeTo(20.8217, 0.01));
      expect(TrackGeo.haversineKm(ikeja, ikeja), 0);
    });

    test('one degree of latitude is the documented constant', () {
      expect(
        TrackGeo.haversineKm(const TrackLatLng(0, 0), const TrackLatLng(1, 0)),
        closeTo(TrackGeo.kmPerDegreeLat, 0.001),
      );
    });
  });

  group('bearingDegrees', () {
    const origin = TrackLatLng(0, 0);

    test('cardinal directions', () {
      expect(TrackGeo.bearingDegrees(origin, const TrackLatLng(1, 0)), closeTo(0, 0.01));
      expect(TrackGeo.bearingDegrees(origin, const TrackLatLng(0, 1)), closeTo(90, 0.01));
      expect(TrackGeo.bearingDegrees(origin, const TrackLatLng(-1, 0)), closeTo(180, 0.01));
      expect(TrackGeo.bearingDegrees(origin, const TrackLatLng(0, -1)), closeTo(270, 0.01));
    });
  });

  group('lerpBearing', () {
    // A marker crossing north must turn a couple of degrees, not spin all the
    // way round the compass.
    test('takes the short way across the 0/360 wrap', () {
      expect(TrackGeo.lerpBearing(350, 10, 0.5), closeTo(0, 0.001));
      expect(TrackGeo.lerpBearing(10, 350, 0.5), closeTo(0, 0.001));
    });

    test('interpolates normally away from the wrap', () {
      expect(TrackGeo.lerpBearing(90, 180, 0.5), closeTo(135, 0.001));
    });

    test('clamps t and normalises the result', () {
      expect(TrackGeo.lerpBearing(90, 180, -1), closeTo(90, 0.001));
      expect(TrackGeo.lerpBearing(90, 180, 2), closeTo(180, 0.001));
      expect(TrackGeo.normalizeBearing(-90), closeTo(270, 0.001));
      expect(TrackGeo.normalizeBearing(720), closeTo(0, 0.001));
    });
  });

  group('decodePolyline', () {
    test('decodes the Google reference vector', () {
      // The same string the Go tests pin, so both ends decode identically.
      final points = TrackGeo.decodePolyline('_p~iF~ps|U_ulLnnqC_mqNvxq`@');

      expect(points, hasLength(3));
      expect(points[0].lat, closeTo(38.5, 1e-6));
      expect(points[0].lng, closeTo(-120.2, 1e-6));
      expect(points[2].lat, closeTo(43.252, 1e-6));
      expect(points[2].lng, closeTo(-126.453, 1e-6));
    });

    test('an unset route decodes to nothing rather than throwing', () {
      expect(TrackGeo.decodePolyline(''), isEmpty);
    });

    test('a truncated polyline yields what it can instead of throwing', () {
      expect(() => TrackGeo.decodePolyline('_p~iF'), returnsNormally);
    });
  });

  group('progressAlongPath', () {
    // An L-shaped route, where along-route progress and straight-line
    // progress genuinely differ.
    final route = <TrackLatLng>[
      const TrackLatLng(0, 0),
      const TrackLatLng(0, 1),
      const TrackLatLng(1, 1),
    ];

    test('measures along the route, not as the crow flies', () {
      expect(TrackGeo.progressAlongPath(const TrackLatLng(0, 0), route), closeTo(0, 0.01));
      expect(TrackGeo.progressAlongPath(const TrackLatLng(0, 0.5), route), closeTo(0.25, 0.01));
      expect(TrackGeo.progressAlongPath(const TrackLatLng(0, 1), route), closeTo(0.5, 0.01));
      expect(TrackGeo.progressAlongPath(const TrackLatLng(0.5, 1), route), closeTo(0.75, 0.01));
      expect(TrackGeo.progressAlongPath(const TrackLatLng(1, 1), route), closeTo(1, 0.01));
    });

    test('reports nothing for a path it cannot project onto', () {
      expect(TrackGeo.progressAlongPath(ikeja, const []), isNull);
      expect(TrackGeo.progressAlongPath(ikeja, [ikeja]), isNull);
    });
  });

  group('snapToPath', () {
    test('reports how far off the route a point is', () {
      final route = <TrackLatLng>[const TrackLatLng(0, 0), const TrackLatLng(0, 1)];

      final onRoute = TrackGeo.snapToPath(const TrackLatLng(0, 0.5), route)!;
      expect(onRoute.offPathKm, closeTo(0, 0.01));
      expect(onRoute.point.lat, closeTo(0, 1e-6));

      final offRoute = TrackGeo.snapToPath(const TrackLatLng(0.01, 0.5), route)!;
      expect(offRoute.offPathKm, closeTo(0.01 * TrackGeo.kmPerDegreeLat, 0.02));
      // The snapped point sits back on the line.
      expect(offRoute.point.lat, closeTo(0, 1e-6));
    });
  });

  group('splitAt', () {
    test('divides the route into travelled and remaining', () {
      final route = <TrackLatLng>[
        const TrackLatLng(0, 0),
        const TrackLatLng(0, 1),
        const TrackLatLng(0, 2),
      ];

      final (travelled, remaining) = TrackGeo.splitAt(const TrackLatLng(0, 0.5), route);

      expect(travelled.first, const TrackLatLng(0, 0));
      expect(travelled.last.lng, closeTo(0.5, 1e-6));
      expect(remaining.first.lng, closeTo(0.5, 1e-6));
      expect(remaining.last, const TrackLatLng(0, 2));
    });

    test('with no route everything is still ahead', () {
      final (travelled, remaining) = TrackGeo.splitAt(ikeja, const []);
      expect(travelled, isEmpty);
      expect(remaining, isEmpty);
    });
  });

  group('TrackLatLng.hasFix', () {
    test('null island is not a fix', () {
      expect(const TrackLatLng(0, 0).hasFix, isFalse);
      expect(ikeja.hasFix, isTrue);
      expect(const TrackLatLng(91, 3).hasFix, isFalse);
      expect(const TrackLatLng(6, 181).hasFix, isFalse);
    });
  });

  group('etaMinutes', () {
    test('never reports less than a minute', () {
      expect(TrackGeo.etaMinutes(0), 1);
      expect(TrackGeo.etaMinutes(0.1), 1);
      expect(TrackGeo.etaMinutes(30), 60);
      expect(TrackGeo.etaMinutes(18, speedKmh: 18), 60);
    });
  });
}
