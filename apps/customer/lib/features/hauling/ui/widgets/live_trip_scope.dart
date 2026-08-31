import 'package:cosmicforge_logistics_live_tracking/cosmicforge_logistics_live_tracking.dart';
import 'package:flutter/material.dart';

/// What the builder gets: the animation controller to hand to the map, and the
/// progress to hand to the bar.
class LiveTripTracking {
  const LiveTripTracking({required this.controller, required this.progress});

  final LiveTrackController controller;
  final TripProgressSnapshot progress;
}

/// Owns the live-tracking animation for one trip.
///
/// This exists as a wrapper so the trip screens stay declarative: a `Ticker`
/// needs a `TickerProvider`, which needs a `State`, and neither the hauling nor
/// the dispatch trip view should have to become stateful just to animate a
/// marker. Both flows wrap their map and progress bar in one of these.
///
/// It converts each new server fix into smooth movement and recomputes trip
/// progress, restarting cleanly when the trip crosses from the approach leg to
/// the delivery leg.
class LiveTripScope extends StatefulWidget {
  const LiveTripScope({
    super.key,
    required this.vehicle,
    required this.leg,
    required this.origin,
    required this.destination,
    required this.builder,
    this.routePolyline = '',
    this.speedKmh = 30,
  });

  /// The latest reported position, or null when none has arrived yet.
  final TrackPoint? vehicle;

  /// Which leg the trip is on. A change resets progress to zero for the new leg.
  final TripLeg leg;

  /// Start and end of the current leg, used for straight-line fallback when no
  /// road route is available.
  final TrackLatLng origin;
  final TrackLatLng destination;

  /// Encoded road geometry for the current leg. Empty falls back to
  /// straight-line measurement.
  final String routePolyline;

  /// Assumed travel speed for the ETA. Trucks and bikes differ.
  final double speedKmh;

  final Widget Function(BuildContext context, LiveTripTracking tracking) builder;

  @override
  State<LiveTripScope> createState() => _LiveTripScopeState();
}

class _LiveTripScopeState extends State<LiveTripScope> with TickerProviderStateMixin {
  late final LiveTrackController _controller;
  late TripProgress _progress;
  TripProgressSnapshot _snapshot = TripProgressSnapshot.empty;

  @override
  void initState() {
    super.initState();
    _controller = LiveTrackController(vsync: this);
    _progress = TripProgress(speedKmh: widget.speedKmh);
    _applyRoute();
    _ingest();
    _recompute();
  }

  @override
  void didUpdateWidget(LiveTripScope old) {
    super.didUpdateWidget(old);

    if (old.speedKmh != widget.speedKmh) {
      _progress = TripProgress(speedKmh: widget.speedKmh);
    }
    if (old.routePolyline != widget.routePolyline || old.leg != widget.leg) {
      _applyRoute();
    }
    if (!identical(old.vehicle, widget.vehicle)) {
      _ingest();
    }
    _recompute();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _applyRoute() {
    _controller.setRoute(
      widget.routePolyline.isEmpty
          ? const <TrackLatLng>[]
          : TrackGeo.decodePolyline(widget.routePolyline),
    );
  }

  void _ingest() {
    final vehicle = widget.vehicle;
    if (vehicle != null) _controller.ingest(vehicle);
  }

  void _recompute() {
    _snapshot = _progress.update(
      leg: widget.leg,
      // Measure from where the marker is drawn, so the bar and the map agree.
      vehicle: _controller.renderedPosition,
      origin: widget.origin,
      destination: widget.destination,
      route: _controller.route,
    );
  }

  @override
  Widget build(BuildContext context) {
    // Rebuild on every interpolated frame so the progress bar advances with
    // the marker rather than only on each server report.
    return AnimatedBuilder(
      animation: _controller,
      builder: (context, _) {
        _recompute();
        return widget.builder(
          context,
          LiveTripTracking(controller: _controller, progress: _snapshot),
        );
      },
    );
  }
}
