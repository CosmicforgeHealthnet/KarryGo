import 'package:flutter/foundation.dart';
import 'package:flutter/scheduler.dart';

import 'track_geo.dart';
import 'track_point.dart';

/// Turns discrete position reports into continuous movement.
///
/// A vehicle reports every few seconds, but a marker redrawn only on each
/// report teleports. This drives a [Ticker] and exposes an interpolated
/// position that advances every frame, so the vehicle glides between fixes the
/// way it does in Uber or Bolt.
///
/// The interpolation window adapts to the cadence actually observed rather
/// than being hardcoded, so the same controller behaves correctly whether the
/// vehicle is reporting every 4 seconds on a trip or every 30 seconds idle.
class LiveTrackController extends ChangeNotifier {
  LiveTrackController({
    TickerProvider? vsync,
    this.minInterpolation = const Duration(seconds: 1),
    this.maxInterpolation = const Duration(seconds: 6),
    this.staleAfter = const Duration(seconds: 45),
  }) {
    // A vsync is optional so the controller can be unit-tested, and driven
    // manually via [debugAdvance], without a widget tree.
    if (vsync != null) {
      _ticker = vsync.createTicker(_onTick);
    }
  }

  /// Shortest window a move is animated over. Below this, movement reads as a
  /// jump anyway.
  final Duration minInterpolation;

  /// Longest window a move is animated over. Beyond this the vehicle would
  /// lag so far behind its real position that the map would mislead.
  final Duration maxInterpolation;

  /// How long without a report before the position is considered stale. The
  /// controller stops moving rather than drifting on guesses.
  final Duration staleAfter;

  Ticker? _ticker;

  TrackPoint? _target;
  TrackPoint? _previous;

  TrackLatLng? _rendered;
  double _renderedBearing = 0;
  double? _targetBearing;
  double? _previousBearing;

  Duration _window = const Duration(seconds: 4);
  Duration _elapsed = Duration.zero;
  Duration _lastTick = Duration.zero;
  DateTime? _lastIngestAt;

  List<TrackLatLng> _route = const [];

  /// The most recent position reported by the server, un-interpolated.
  TrackPoint? get latest => _target;

  /// The position to draw right now: somewhere between the previous report
  /// and the latest one.
  TrackLatLng? get renderedPosition => _rendered;

  /// The bearing to rotate the vehicle marker by, in degrees.
  double get renderedBearing => _renderedBearing;

  /// Whether nothing has been reported for longer than [staleAfter]. A stale
  /// controller holds its last position instead of extrapolating.
  bool get isStale {
    final last = _lastIngestAt;
    if (last == null) return false;
    return DateTime.now().difference(last) > staleAfter;
  }

  /// Whether any position has been received yet.
  bool get hasFix => _rendered != null;

  /// The route the vehicle is following, if known.
  List<TrackLatLng> get route => _route;

  /// Sets the road geometry the vehicle is following.
  ///
  /// When set, reported positions are snapped onto it, which keeps the marker
  /// on the road rather than cutting corners across buildings between fixes.
  /// Positions far off the route (a real detour) are left unsnapped so the map
  /// stays truthful.
  void setRoute(List<TrackLatLng> route) {
    _route = route;
  }

  /// Feeds a new server report.
  void ingest(TrackPoint point) {
    if (!point.hasFix) return;

    final now = DateTime.now();
    final snapped = _snap(point.position);

    // First fix: jump straight there, there is nothing to interpolate from.
    if (_rendered == null) {
      _previous = point;
      _target = point;
      _rendered = snapped;
      _renderedBearing = point.headingDegrees ?? 0;
      _targetBearing = _renderedBearing;
      _previousBearing = _renderedBearing;
      _lastIngestAt = now;
      _elapsed = Duration.zero;
      _startTicker();
      notifyListeners();
      return;
    }

    // Ignore a repeat of the same position: animating a zero-length move
    // would only reset the clock and stutter the marker.
    if (_target != null && _sameSpot(_target!.position, point.position)) {
      _lastIngestAt = now;
      return;
    }

    _window = _windowFor(now);
    _previous = TrackPoint(position: _rendered!, headingDegrees: _renderedBearing);
    _previousBearing = _renderedBearing;
    _target = TrackPoint(
      position: snapped,
      headingDegrees: point.headingDegrees,
      speedMetersPerSecond: point.speedMetersPerSecond,
      accuracyMeters: point.accuracyMeters,
      recordedAt: point.recordedAt,
    );
    // Prefer the device's own heading; fall back to the direction of travel,
    // which is all a device that reports no heading can give us.
    _targetBearing = point.headingDegrees ??
        TrackGeo.bearingDegrees(_previous!.position, snapped);

    _lastIngestAt = now;
    _elapsed = Duration.zero;
    _startTicker();
    notifyListeners();
  }

  /// Clears all state, for when a trip ends or a different booking is opened.
  void reset() {
    _ticker?.stop();
    _target = null;
    _previous = null;
    _rendered = null;
    _renderedBearing = 0;
    _targetBearing = null;
    _previousBearing = null;
    _elapsed = Duration.zero;
    _lastTick = Duration.zero;
    _lastIngestAt = null;
    _route = const [];
    notifyListeners();
  }

  /// Advances the animation by [delta] without a ticker. Exposed for tests,
  /// which cannot pump real frames.
  @visibleForTesting
  void debugAdvance(Duration delta) => _advance(delta);

  @override
  void dispose() {
    _ticker?.dispose();
    _ticker = null;
    super.dispose();
  }

  void _startTicker() {
    _lastTick = Duration.zero;
    final ticker = _ticker;
    if (ticker != null && !ticker.isActive) {
      ticker.start();
    }
  }

  void _onTick(Duration elapsed) {
    final delta = elapsed - _lastTick;
    _lastTick = elapsed;
    _advance(delta);
  }

  void _advance(Duration delta) {
    final previous = _previous;
    final target = _target;
    if (previous == null || target == null) return;

    if (_elapsed >= _window) {
      // Settled on the target: stop ticking until the next report rather than
      // burning a frame callback forever.
      _ticker?.stop();
      return;
    }

    _elapsed += delta;
    final t = _window.inMicroseconds == 0
        ? 1.0
        : (_elapsed.inMicroseconds / _window.inMicroseconds).clamp(0.0, 1.0);

    _rendered = TrackGeo.lerp(previous.position, target.position, t);
    if (_targetBearing != null && _previousBearing != null) {
      _renderedBearing = TrackGeo.lerpBearing(_previousBearing!, _targetBearing!, t);
    }
    notifyListeners();
  }

  /// Picks how long to animate the move over, from the gap since the last
  /// report. Matching the observed cadence means the marker arrives just as
  /// the next fix lands, so movement looks continuous rather than
  /// stop-and-go.
  Duration _windowFor(DateTime now) {
    final last = _lastIngestAt;
    if (last == null) return _window;

    final gap = now.difference(last);
    if (gap < minInterpolation) return minInterpolation;
    if (gap > maxInterpolation) return maxInterpolation;
    return gap;
  }

  /// Snaps a reported position onto the route, unless it is far enough off it
  /// to be a genuine deviation.
  TrackLatLng _snap(TrackLatLng position) {
    if (_route.length < 2) return position;
    final projection = TrackGeo.snapToPath(position, _route);
    if (projection == null) return position;
    // Roughly a city block: beyond this the vehicle is genuinely off-route and
    // pinning it to the road would misreport where it is.
    if (projection.offPathKm > 0.1) return position;
    return projection.point;
  }

  static bool _sameSpot(TrackLatLng a, TrackLatLng b) =>
      (a.lat - b.lat).abs() < 1e-7 && (a.lng - b.lng).abs() < 1e-7;
}
