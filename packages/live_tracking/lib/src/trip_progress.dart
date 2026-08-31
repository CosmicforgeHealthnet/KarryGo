import 'track_geo.dart';

/// Which leg of a trip is being measured.
enum TripLeg {
  /// The vehicle is on its way to collect. Progress runs from where the
  /// driver was when assigned to the pickup point.
  toPickup,

  /// The load is aboard and moving to its destination.
  toDropoff,
}

/// How far along a trip the vehicle is.
class TripProgressSnapshot {
  const TripProgressSnapshot({
    required this.leg,
    required this.legFraction,
    required this.overallFraction,
    this.remainingKm,
    this.etaMinutes,
  });

  /// The leg currently in progress.
  final TripLeg leg;

  /// Progress along the current leg, 0 to 1. This is what the progress bar
  /// fills to.
  final double legFraction;

  /// Progress across the whole trip, 0 to 1, treating each leg as half.
  final double overallFraction;

  /// Distance left on the current leg, when it can be computed.
  final double? remainingKm;

  /// Rough minutes left on the current leg.
  final int? etaMinutes;

  static const TripProgressSnapshot empty = TripProgressSnapshot(
    leg: TripLeg.toPickup,
    legFraction: 0,
    overallFraction: 0,
  );
}

/// Computes trip progress from the vehicle's position.
///
/// Two things make this different from the straight-line ratio it replaces:
///
///  * it covers the approach leg as well as the delivery leg, so the bar moves
///    while the driver is still on their way to collect; and
///  * when a road route is known it measures along that route, so a detour
///    reads as progress rather than as the vehicle standing still.
///
/// Progress is monotonic within a leg: GPS jitter can nudge a vehicle
/// backwards, and a bar that slides back reads as a fault.
class TripProgress {
  TripProgress({double speedKmh = 30}) : _speedKmh = speedKmh;

  final double _speedKmh;

  double _highWater = 0;
  TripLeg? _leg;

  /// Resets progress, for a new trip or when the leg changes.
  void reset() {
    _highWater = 0;
    _leg = null;
  }

  /// Recomputes progress for the current vehicle position.
  ///
  /// [route] is the road geometry for the active leg when known; pass an
  /// empty list to fall back to straight-line measurement between [origin]
  /// and [destination].
  TripProgressSnapshot update({
    required TripLeg leg,
    required TrackLatLng? vehicle,
    required TrackLatLng origin,
    required TrackLatLng destination,
    List<TrackLatLng> route = const [],
  }) {
    // A new leg starts from zero: the bar restarts as the trip moves from
    // approach to delivery.
    if (_leg != leg) {
      _leg = leg;
      _highWater = 0;
    }

    final measured = _measure(
      vehicle: vehicle,
      origin: origin,
      destination: destination,
      route: route,
    );

    final fraction = measured.$1;
    final remainingKm = measured.$2;

    if (fraction > _highWater) {
      _highWater = fraction;
    }

    return TripProgressSnapshot(
      leg: leg,
      legFraction: _highWater,
      overallFraction: leg == TripLeg.toPickup ? _highWater / 2 : 0.5 + _highWater / 2,
      remainingKm: remainingKm,
      etaMinutes:
          remainingKm == null ? null : TrackGeo.etaMinutes(remainingKm, speedKmh: _speedKmh),
    );
  }

  /// Returns the raw fraction and remaining distance for a position.
  (double, double?) _measure({
    required TrackLatLng? vehicle,
    required TrackLatLng origin,
    required TrackLatLng destination,
    required List<TrackLatLng> route,
  }) {
    if (vehicle == null || !vehicle.hasFix) return (_highWater, null);

    if (route.length >= 2) {
      final fraction = TrackGeo.progressAlongPath(vehicle, route);
      final remaining = TrackGeo.remainingKm(vehicle, route);
      if (fraction != null) return (fraction, remaining);
    }

    // Straight-line fallback: how much of the origin-to-destination gap has
    // been closed.
    final total = TrackGeo.haversineKm(origin, destination);
    if (total < 0.05) return (_highWater, 0);

    final remaining = TrackGeo.haversineKm(vehicle, destination);
    final fraction = (1 - remaining / total).clamp(0.0, 1.0);
    return (fraction, remaining);
  }
}
