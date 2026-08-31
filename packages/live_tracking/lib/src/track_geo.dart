import 'dart:math' as math;

/// A WGS84 coordinate.
///
/// Deliberately its own type rather than `google_maps_flutter`'s `LatLng` or
/// `latlong2`'s: the apps use two different map engines, and the core of this
/// package must not depend on either. The adapter packages convert at the
/// boundary.
class TrackLatLng {
  const TrackLatLng(this.lat, this.lng);

  final double lat;
  final double lng;

  /// Whether this is a usable position. A genuine (0, 0) reading is in the
  /// Gulf of Guinea, not serviceable territory, so it reads as "no fix".
  bool get hasFix =>
      lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180 && !(lat == 0 && lng == 0);

  @override
  bool operator ==(Object other) =>
      other is TrackLatLng && other.lat == lat && other.lng == lng;

  @override
  int get hashCode => Object.hash(lat, lng);

  @override
  String toString() => 'TrackLatLng($lat, $lng)';
}

/// Where a point sits once projected onto a path.
class TrackProjection {
  const TrackProjection({
    required this.segmentIndex,
    required this.t,
    required this.point,
    required this.alongKm,
    required this.offPathKm,
  });

  /// Index of the path vertex that starts the closest segment.
  final int segmentIndex;

  /// Position along that segment, 0 at its start and 1 at its end.
  final double t;

  /// The snapped position on the path.
  final TrackLatLng point;

  /// Distance from the start of the path to [point].
  final double alongKm;

  /// How far the input point sits from the path. A large value means the
  /// vehicle has deviated and the projection should be distrusted.
  final double offPathKm;
}

/// Pure geographic helpers, mirroring `shared/go/geo` so the client and the
/// services measure distance and progress the same way.
class TrackGeo {
  TrackGeo._();

  /// Mean Earth radius, matching the value every service uses.
  static const double earthRadiusKm = 6371.0;

  /// Length of one degree of latitude, in kilometres.
  static const double kmPerDegreeLat = math.pi * earthRadiusKm / 180;

  /// Great-circle distance in kilometres.
  static double haversineKm(TrackLatLng a, TrackLatLng b) {
    final dLat = _radians(b.lat - a.lat);
    final dLng = _radians(b.lng - a.lng);
    final h = math.sin(dLat / 2) * math.sin(dLat / 2) +
        math.cos(_radians(a.lat)) *
            math.cos(_radians(b.lat)) *
            math.sin(dLng / 2) *
            math.sin(dLng / 2);
    return earthRadiusKm * 2 * math.atan2(math.sqrt(h), math.sqrt(1 - h));
  }

  /// Initial compass bearing from [from] to [to], normalised to [0, 360).
  ///
  /// This is what a vehicle marker is rotated by so it points where it is
  /// travelling.
  static double bearingDegrees(TrackLatLng from, TrackLatLng to) {
    final lat1 = _radians(from.lat);
    final lat2 = _radians(to.lat);
    final dLng = _radians(to.lng - from.lng);

    final y = math.sin(dLng) * math.cos(lat2);
    final x = math.cos(lat1) * math.sin(lat2) -
        math.sin(lat1) * math.cos(lat2) * math.cos(dLng);
    return normalizeBearing(_degrees(math.atan2(y, x)));
  }

  /// Folds any angle into [0, 360).
  static double normalizeBearing(double degrees) {
    final d = degrees % 360;
    return d < 0 ? d + 360 : d;
  }

  /// Interpolates between two bearings the short way round, so a marker
  /// crossing north turns 2 degrees rather than spinning 358.
  static double lerpBearing(double from, double to, double t) {
    final a = normalizeBearing(from);
    final b = normalizeBearing(to);
    var delta = b - a;
    if (delta > 180) {
      delta -= 360;
    } else if (delta < -180) {
      delta += 360;
    }
    return normalizeBearing(a + delta * t.clamp(0.0, 1.0));
  }

  /// Linear interpolation between two points. Over the distances one GPS
  /// interval covers, the error against a great-circle path is far below GPS
  /// accuracy.
  static TrackLatLng lerp(TrackLatLng a, TrackLatLng b, double t) {
    if (t <= 0) return a;
    if (t >= 1) return b;
    return TrackLatLng(a.lat + (b.lat - a.lat) * t, a.lng + (b.lng - a.lng) * t);
  }

  /// Summed great-circle length of a path.
  static double pathLengthKm(List<TrackLatLng> path) {
    if (path.length < 2) return 0;
    var total = 0.0;
    for (var i = 1; i < path.length; i++) {
      total += haversineKm(path[i - 1], path[i]);
    }
    return total;
  }

  /// Projects [p] onto the closest segment of [path], or null when the path
  /// has fewer than two points.
  static TrackProjection? snapToPath(TrackLatLng p, List<TrackLatLng> path) {
    if (path.length < 2) return null;

    TrackProjection? best;
    var cumulative = 0.0;

    for (var i = 0; i < path.length - 1; i++) {
      final a = path[i];
      final b = path[i + 1];
      final segmentKm = haversineKm(a, b);

      final projected = _projectOntoSegment(p, a, b);
      final t = projected.$1;
      final offKm = projected.$2;

      if (best == null || offKm < best.offPathKm) {
        best = TrackProjection(
          segmentIndex: i,
          t: t,
          point: lerp(a, b, t),
          alongKm: cumulative + segmentKm * t,
          offPathKm: offKm,
        );
      }
      cumulative += segmentKm;
    }
    return best;
  }

  /// How far along [path] the point sits, as a fraction in [0, 1]. Null when
  /// the path cannot be projected onto.
  static double? progressAlongPath(TrackLatLng p, List<TrackLatLng> path) {
    final projection = snapToPath(p, path);
    if (projection == null) return null;
    final total = pathLengthKm(path);
    if (total <= 0) return 0;
    return (projection.alongKm / total).clamp(0.0, 1.0);
  }

  /// Distance left along [path] from [p] to its end.
  static double? remainingKm(TrackLatLng p, List<TrackLatLng> path) {
    final projection = snapToPath(p, path);
    if (projection == null) return null;
    final remaining = pathLengthKm(path) - projection.alongKm;
    return remaining < 0 ? 0 : remaining;
  }

  /// Splits a route into the part already travelled and the part remaining, so
  /// a map can draw them in different colours.
  static (List<TrackLatLng> travelled, List<TrackLatLng> remaining) splitAt(
    TrackLatLng p,
    List<TrackLatLng> path,
  ) {
    final projection = snapToPath(p, path);
    if (projection == null) return (const [], path);

    final travelled = <TrackLatLng>[
      ...path.sublist(0, projection.segmentIndex + 1),
      projection.point,
    ];
    final remaining = <TrackLatLng>[
      projection.point,
      ...path.sublist(projection.segmentIndex + 1),
    ];
    return (travelled, remaining);
  }

  /// Decodes a Google encoded polyline, the format the Routes API returns and
  /// the services store. An empty string decodes to an empty list.
  static List<TrackLatLng> decodePolyline(String encoded) {
    if (encoded.isEmpty) return const [];

    final points = <TrackLatLng>[];
    var index = 0;
    var lat = 0;
    var lng = 0;

    while (index < encoded.length) {
      final latResult = _decodeValue(encoded, index);
      if (latResult == null) return points;
      lat += latResult.$1;
      index = latResult.$2;

      final lngResult = _decodeValue(encoded, index);
      if (lngResult == null) return points;
      lng += lngResult.$1;
      index = lngResult.$2;

      points.add(TrackLatLng(lat / 1e5, lng / 1e5));
    }
    return points;
  }

  /// Estimated minutes to cover [distanceKm] at [speedKmh], never less than 1.
  static int etaMinutes(double distanceKm, {double speedKmh = 30}) {
    if (speedKmh <= 0) return 1;
    final minutes = (distanceKm / speedKmh * 60).round();
    return minutes < 1 ? 1 : minutes;
  }

  /// Projects [p] onto segment a-b in a local flat frame anchored at a,
  /// returning the clamped position along the segment and the distance to it.
  static (double, double) _projectOntoSegment(
    TrackLatLng p,
    TrackLatLng a,
    TrackLatLng b,
  ) {
    final kmPerDegreeLng = kmPerDegreeLat * math.max(math.cos(_radians(a.lat)), 1e-6);

    final bx = (b.lng - a.lng) * kmPerDegreeLng;
    final by = (b.lat - a.lat) * kmPerDegreeLat;
    final px = (p.lng - a.lng) * kmPerDegreeLng;
    final py = (p.lat - a.lat) * kmPerDegreeLat;

    final lengthSq = bx * bx + by * by;
    if (lengthSq == 0) {
      return (0, _hypot(px, py));
    }

    final t = ((px * bx + py * by) / lengthSq).clamp(0.0, 1.0);
    return (t, _hypot(px - bx * t, py - by * t));
  }

  static (int, int)? _decodeValue(String encoded, int index) {
    var shift = 0;
    var result = 0;
    var i = index;

    while (true) {
      if (i >= encoded.length) return null;
      final b = encoded.codeUnitAt(i) - 63;
      if (b < 0 || b > 63) return null;
      i++;
      result |= (b & 0x1f) << shift;
      shift += 5;
      if (b < 0x20) break;
      if (shift > 32) return null;
    }

    final value = (result & 1) != 0 ? ~(result >> 1) : result >> 1;
    return (value, i);
  }

  static double _hypot(double x, double y) => math.sqrt(x * x + y * y);
  static double _radians(double degrees) => degrees * math.pi / 180;
  static double _degrees(double radians) => radians * 180 / math.pi;
}
