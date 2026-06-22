import 'dart:async';

import 'package:flutter/material.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';

class HaulingMapWidget extends StatefulWidget {
  const HaulingMapWidget({
    super.key,
    this.pickupLatLng,
    this.dropoffLatLng,
    this.initialCenter,
  });

  final LatLng? pickupLatLng;
  final LatLng? dropoffLatLng;
  // Default center: Lagos island
  final LatLng? initialCenter;

  @override
  State<HaulingMapWidget> createState() => _HaulingMapWidgetState();
}

class _HaulingMapWidgetState extends State<HaulingMapWidget> {
  static const _lagosCenter = LatLng(6.5244, 3.3792);
  static const _defaultZoom = 12.0;

  final Completer<GoogleMapController> _controller = Completer();
  Set<Marker> _markers = {};

  @override
  void initState() {
    super.initState();
    _updateMarkers();
  }

  @override
  void didUpdateWidget(HaulingMapWidget old) {
    super.didUpdateWidget(old);
    if (old.pickupLatLng != widget.pickupLatLng ||
        old.dropoffLatLng != widget.dropoffLatLng) {
      _updateMarkers();
      _fitCamera();
    }
  }

  void _updateMarkers() {
    final markers = <Marker>{};
    if (widget.pickupLatLng != null) {
      markers.add(Marker(
        markerId: const MarkerId('pickup'),
        position: widget.pickupLatLng!,
        icon: BitmapDescriptor.defaultMarkerWithHue(BitmapDescriptor.hueGreen),
        infoWindow: const InfoWindow(title: 'Pickup'),
      ));
    }
    if (widget.dropoffLatLng != null) {
      markers.add(Marker(
        markerId: const MarkerId('dropoff'),
        position: widget.dropoffLatLng!,
        icon: BitmapDescriptor.defaultMarkerWithHue(BitmapDescriptor.hueRed),
        infoWindow: const InfoWindow(title: 'Dropoff'),
      ));
    }
    setState(() => _markers = markers);
  }

  Future<void> _fitCamera() async {
    if (!_controller.isCompleted) return;
    final ctrl = await _controller.future;

    if (widget.pickupLatLng != null && widget.dropoffLatLng != null) {
      final bounds = LatLngBounds(
        southwest: LatLng(
          widget.pickupLatLng!.latitude < widget.dropoffLatLng!.latitude
              ? widget.pickupLatLng!.latitude
              : widget.dropoffLatLng!.latitude,
          widget.pickupLatLng!.longitude < widget.dropoffLatLng!.longitude
              ? widget.pickupLatLng!.longitude
              : widget.dropoffLatLng!.longitude,
        ),
        northeast: LatLng(
          widget.pickupLatLng!.latitude > widget.dropoffLatLng!.latitude
              ? widget.pickupLatLng!.latitude
              : widget.dropoffLatLng!.latitude,
          widget.pickupLatLng!.longitude > widget.dropoffLatLng!.longitude
              ? widget.pickupLatLng!.longitude
              : widget.dropoffLatLng!.longitude,
        ),
      );
      ctrl.animateCamera(CameraUpdate.newLatLngBounds(bounds, 80));
    } else if (widget.pickupLatLng != null) {
      ctrl.animateCamera(CameraUpdate.newLatLngZoom(widget.pickupLatLng!, 14));
    }
  }

  @override
  Widget build(BuildContext context) {
    return GoogleMap(
      initialCameraPosition: CameraPosition(
        target: widget.pickupLatLng ??
            widget.initialCenter ??
            _lagosCenter,
        zoom: _defaultZoom,
      ),
      markers: _markers,
      polylines: _buildPolyline(),
      myLocationEnabled: false,
      zoomControlsEnabled: false,
      mapToolbarEnabled: false,
      onMapCreated: (ctrl) {
        _controller.complete(ctrl);
        _fitCamera();
      },
    );
  }

  Set<Polyline> _buildPolyline() {
    if (widget.pickupLatLng == null || widget.dropoffLatLng == null) {
      return const {};
    }
    return {
      Polyline(
        polylineId: const PolylineId('route'),
        points: [widget.pickupLatLng!, widget.dropoffLatLng!],
        color: const Color(0xFF1F7A4D),
        width: 3,
        patterns: [PatternItem.dash(20), PatternItem.gap(10)],
      ),
    };
  }
}
