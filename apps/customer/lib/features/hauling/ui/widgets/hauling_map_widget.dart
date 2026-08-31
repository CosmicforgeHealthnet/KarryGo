import 'dart:async';

import 'package:cosmicforge_logistics_live_tracking/cosmicforge_logistics_live_tracking.dart';
import 'package:cosmicforge_logistics_live_tracking_google_maps/cosmicforge_logistics_live_tracking_google_maps.dart';
import 'package:flutter/material.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';

/// The trip map, shared by the truck-hauling and bike-dispatch flows.
///
/// When a [liveTrack] controller is supplied the vehicle marker glides between
/// GPS reports and rotates to its heading, rather than being redrawn at each
/// poll result. The camera follows only when the vehicle nears the edge of the
/// viewport, so the map no longer lurches on every update, and it stands down
/// as soon as the user pans.
class HaulingMapWidget extends StatefulWidget {
  const HaulingMapWidget({
    super.key,
    this.pickupLatLng,
    this.dropoffLatLng,
    this.driverLatLng,
    this.initialCenter,
    this.liveTrack,
    this.routePolyline,
  });

  final LatLng? pickupLatLng;
  final LatLng? dropoffLatLng;

  /// Last known driver position. Used when no [liveTrack] controller is given,
  /// and as the initial framing hint when one is.
  final LatLng? driverLatLng;

  /// Default center: Lagos island
  final LatLng? initialCenter;

  /// Drives the smoothly-moving vehicle marker. Null outside an active trip.
  final LiveTrackController? liveTrack;

  /// Encoded road route for the active leg. Empty falls back to the dashed
  /// straight line between pickup and drop-off.
  final String? routePolyline;

  @override
  State<HaulingMapWidget> createState() => _HaulingMapWidgetState();
}

class _HaulingMapWidgetState extends State<HaulingMapWidget> {
  static const _lagosCenter = LatLng(6.5244, 3.3792);
  static const _defaultZoom = 12.0;

  final Completer<GoogleMapController> _controller = Completer();
  final LiveCameraController _camera = LiveCameraController();

  List<TrackLatLng> _route = const [];
  bool _framedOnce = false;

  @override
  void initState() {
    super.initState();
    _decodeRoute();
    widget.liveTrack?.addListener(_onVehicleMoved);
  }

  @override
  void didUpdateWidget(HaulingMapWidget old) {
    super.didUpdateWidget(old);

    if (old.liveTrack != widget.liveTrack) {
      old.liveTrack?.removeListener(_onVehicleMoved);
      widget.liveTrack?.addListener(_onVehicleMoved);
    }
    if (old.routePolyline != widget.routePolyline) {
      _decodeRoute();
    }
    // Frame the trip once the endpoints are known. Re-framing on every driver
    // update is what used to make the map jump every few seconds.
    if (old.pickupLatLng != widget.pickupLatLng ||
        old.dropoffLatLng != widget.dropoffLatLng) {
      _framedOnce = false;
      _frameTrip();
    }
  }

  @override
  void dispose() {
    widget.liveTrack?.removeListener(_onVehicleMoved);
    super.dispose();
  }

  void _decodeRoute() {
    final encoded = widget.routePolyline;
    final decoded =
        encoded == null || encoded.isEmpty ? const <TrackLatLng>[] : TrackGeo.decodePolyline(encoded);
    // Feeding the route to the controller keeps the marker on the road
    // between fixes instead of cutting corners.
    widget.liveTrack?.setRoute(decoded);
    if (mounted) {
      setState(() => _route = decoded);
    } else {
      _route = decoded;
    }
  }

  void _onVehicleMoved() {
    if (!mounted) return;
    // Repaint for the interpolated position, then let the camera decide
    // whether it needs to move at all.
    setState(() {});
    unawaited(_followVehicle());
  }

  Future<void> _followVehicle() async {
    if (!_controller.isCompleted) return;
    final map = await _controller.future;
    await _camera.onVehicleMoved(map, widget.liveTrack?.renderedPosition);
  }

  Future<void> _frameTrip() async {
    if (_framedOnce || !_controller.isCompleted) return;
    if (widget.pickupLatLng == null && widget.dropoffLatLng == null) return;
    _framedOnce = true;

    final map = await _controller.future;
    await fitTripBounds(
      map,
      vehicle: _vehiclePosition(),
      pickup: widget.pickupLatLng?.toTrackLatLng(),
      dropoff: widget.dropoffLatLng?.toTrackLatLng(),
    );
  }

  /// The position to draw the vehicle at: the interpolated one when tracking
  /// is live, otherwise whatever the caller last passed in.
  TrackLatLng? _vehiclePosition() {
    final live = widget.liveTrack?.renderedPosition;
    if (live != null) return live;
    return widget.driverLatLng?.toTrackLatLng();
  }

  Set<Marker> _buildMarkers() {
    final markers = buildEndpointMarkers(
      pickup: widget.pickupLatLng?.toTrackLatLng(),
      dropoff: widget.dropoffLatLng?.toTrackLatLng(),
      dropoffLabel: 'Dropoff',
    );

    final track = widget.liveTrack;
    if (track != null && track.hasFix) {
      return {...markers, buildVehicleMarker(controller: track)};
    }

    // No live controller: fall back to a plain driver pin.
    final driver = widget.driverLatLng;
    if (driver != null) {
      return {
        ...markers,
        Marker(
          markerId: const MarkerId('driver'),
          position: driver,
          icon: BitmapDescriptor.defaultMarkerWithHue(BitmapDescriptor.hueAzure),
          infoWindow: const InfoWindow(title: 'Driver'),
        ),
      };
    }
    return markers;
  }

  Set<Polyline> _buildPolylines() {
    // A real road route, split into covered and remaining at the vehicle.
    if (_route.length >= 2) {
      return buildRoutePolylines(route: _route, vehicle: _vehiclePosition());
    }

    // No route geometry (no routing key configured, or not fetched yet):
    // the original dashed straight line.
    if (widget.pickupLatLng == null || widget.dropoffLatLng == null) {
      return const {};
    }
    return {
      Polyline(
        polylineId: const PolylineId('route'),
        points: [widget.pickupLatLng!, widget.dropoffLatLng!],
        color: const Color(0xFF05124D),
        width: 3,
        patterns: [PatternItem.dash(20), PatternItem.gap(10)],
      ),
    };
  }

  @override
  Widget build(BuildContext context) {
    return GoogleMap(
      initialCameraPosition: CameraPosition(
        target: widget.pickupLatLng ?? widget.initialCenter ?? _lagosCenter,
        zoom: _defaultZoom,
      ),
      markers: _buildMarkers(),
      polylines: _buildPolylines(),
      myLocationEnabled: false,
      zoomControlsEnabled: false,
      mapToolbarEnabled: false,
      // The map belongs to the user while they are touching it.
      onCameraMoveStarted: _camera.handleUserGesture,
      onMapCreated: (ctrl) {
        _controller.complete(ctrl);
        _frameTrip();
      },
    );
  }
}
