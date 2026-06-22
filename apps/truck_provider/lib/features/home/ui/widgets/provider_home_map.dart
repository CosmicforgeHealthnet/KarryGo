import 'dart:async';
import 'dart:math';

import 'package:flutter/material.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';
import 'package:permission_handler/permission_handler.dart';

/// Full-screen live map for the provider home and active-trip screens.
class ProviderHomeMap extends StatefulWidget {
  const ProviderHomeMap({
    super.key,
    this.extraMarkers = const {},
    this.polylinePoints = const [],
  });

  final Set<Marker> extraMarkers;
  final List<LatLng> polylinePoints;

  @override
  State<ProviderHomeMap> createState() => _ProviderHomeMapState();
}

class _ProviderHomeMapState extends State<ProviderHomeMap> {
  static const _lagosCenter = LatLng(6.5244, 3.3792);

  final Completer<GoogleMapController> _controller = Completer();
  Set<Marker> _vehicleMarkers = {};
  bool _myLocationEnabled = false;

  @override
  void initState() {
    super.initState();
    _buildVehicleMarkers();
    _resolveLocation();
  }

  Future<void> _resolveLocation() async {
    final status = await Permission.locationWhenInUse.status;
    if (!mounted) return;
    if (status.isGranted || status.isLimited) {
      setState(() => _myLocationEnabled = true);
    }
  }

  void _buildVehicleMarkers() {
    final rng = Random(99);
    final markers = <Marker>{};
    for (var i = 0; i < 8; i++) {
      final dLat = (rng.nextDouble() - 0.5) * 0.05;
      final dLng = (rng.nextDouble() - 0.5) * 0.05;
      markers.add(
        Marker(
          markerId: MarkerId('truck_$i'),
          position: LatLng(
            _lagosCenter.latitude + dLat,
            _lagosCenter.longitude + dLng,
          ),
          icon: BitmapDescriptor.defaultMarkerWithHue(BitmapDescriptor.hueGreen),
          anchor: const Offset(0.5, 0.5),
          flat: true,
        ),
      );
    }
    setState(() => _vehicleMarkers = markers);
  }

  @override
  Widget build(BuildContext context) {
    final allMarkers = {..._vehicleMarkers, ...widget.extraMarkers};
    Set<Polyline> polylines = {};
    if (widget.polylinePoints.length >= 2) {
      polylines = {
        Polyline(
          polylineId: const PolylineId('route'),
          points: widget.polylinePoints,
          color: const Color(0xFF22A84A),
          width: 5,
        ),
      };
    }
    return GoogleMap(
      initialCameraPosition: const CameraPosition(target: _lagosCenter, zoom: 13.5),
      markers: allMarkers,
      polylines: polylines,
      myLocationEnabled: _myLocationEnabled,
      myLocationButtonEnabled: false,
      zoomControlsEnabled: false,
      mapToolbarEnabled: false,
      compassEnabled: false,
      onMapCreated: (ctrl) {
        if (!_controller.isCompleted) _controller.complete(ctrl);
      },
    );
  }
}
