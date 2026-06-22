import 'dart:async';
import 'dart:math';

import 'package:flutter/material.dart';
import 'package:geolocator/geolocator.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';
import 'package:permission_handler/permission_handler.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';

/// Live Google Map background for the home screen, matching the Figma mockup:
/// a real map centered on Lagos (or the user's location once granted), a
/// scattering of nearby-vehicle markers, and a green "locate me" button.
class CustomerHomeMap extends StatefulWidget {
  const CustomerHomeMap({super.key, this.locateButtonBottomInset = 0});

  /// Distance from the bottom of the screen to place the locate-me button,
  /// so it floats just above the service panel like in the mockup.
  final double locateButtonBottomInset;

  @override
  State<CustomerHomeMap> createState() => _CustomerHomeMapState();
}

class _CustomerHomeMapState extends State<CustomerHomeMap> {
  static const _lagosCenter = LatLng(6.5244, 3.3792);
  static const _defaultZoom = 14.0;

  final Completer<GoogleMapController> _controller = Completer();
  Set<Marker> _vehicleMarkers = {};
  bool _myLocationEnabled = false;
  bool _locating = false;

  @override
  void initState() {
    super.initState();
    _buildVehicleMarkers();
    _resolveLocationPermission();
  }

  Future<void> _resolveLocationPermission() async {
    final status = await Permission.locationWhenInUse.status;
    if (!mounted) return;
    if (status.isGranted || status.isLimited) {
      setState(() => _myLocationEnabled = true);
    }
  }

  void _buildVehicleMarkers() {
    // Deterministic scatter of vehicle pins around the center, like the mockup.
    final rng = Random(42);
    final markers = <Marker>{};
    final hues = [
      BitmapDescriptor.hueGreen,
      BitmapDescriptor.hueOrange,
      BitmapDescriptor.hueYellow,
    ];
    for (var i = 0; i < 12; i++) {
      final dLat = (rng.nextDouble() - 0.5) * 0.06;
      final dLng = (rng.nextDouble() - 0.5) * 0.06;
      markers.add(
        Marker(
          markerId: MarkerId('vehicle_$i'),
          position: LatLng(
            _lagosCenter.latitude + dLat,
            _lagosCenter.longitude + dLng,
          ),
          icon: BitmapDescriptor.defaultMarkerWithHue(hues[i % hues.length]),
          anchor: const Offset(0.5, 0.5),
          flat: true,
        ),
      );
    }
    setState(() => _vehicleMarkers = markers);
  }

  Future<void> _goToMyLocation() async {
    if (_locating) return;
    setState(() => _locating = true);
    try {
      var permission = await Permission.locationWhenInUse.status;
      if (!permission.isGranted && !permission.isLimited) {
        permission = await Permission.locationWhenInUse.request();
      }
      if (!permission.isGranted && !permission.isLimited) {
        _showSnack('Enable location access to find your position.');
        return;
      }
      if (!await Geolocator.isLocationServiceEnabled()) {
        _showSnack('Turn on device location to find your position.');
        return;
      }
      if (!_myLocationEnabled && mounted) {
        setState(() => _myLocationEnabled = true);
      }

      final pos = await Geolocator.getCurrentPosition(
        locationSettings: const LocationSettings(accuracy: LocationAccuracy.high),
      );
      if (!_controller.isCompleted) return;
      final ctrl = await _controller.future;
      await ctrl.animateCamera(
        CameraUpdate.newLatLngZoom(LatLng(pos.latitude, pos.longitude), 15.5),
      );
    } catch (_) {
      _showSnack('Could not get your location. Try again.');
    } finally {
      if (mounted) setState(() => _locating = false);
    }
  }

  void _showSnack(String message) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(message), duration: const Duration(seconds: 2)),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Stack(
      children: [
        GoogleMap(
          initialCameraPosition: const CameraPosition(
            target: _lagosCenter,
            zoom: _defaultZoom,
          ),
          markers: _vehicleMarkers,
          myLocationEnabled: _myLocationEnabled,
          myLocationButtonEnabled: false,
          zoomControlsEnabled: false,
          mapToolbarEnabled: false,
          compassEnabled: false,
          onMapCreated: (ctrl) {
            if (!_controller.isCompleted) _controller.complete(ctrl);
          },
        ),

        // Green "locate me" FAB (matches mockup).
        Positioned(
          right: 16,
          bottom: widget.locateButtonBottomInset,
          child: _LocateButton(loading: _locating, onTap: _goToMyLocation),
        ),
      ],
    );
  }
}

class _LocateButton extends StatelessWidget {
  const _LocateButton({required this.loading, required this.onTap});

  final bool loading;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: CustomerFigmaColors.primary,
      shape: const CircleBorder(),
      elevation: 4,
      child: InkWell(
        customBorder: const CircleBorder(),
        onTap: loading ? null : onTap,
        child: SizedBox(
          width: 48,
          height: 48,
          child: loading
              ? const Padding(
                  padding: EdgeInsets.all(14),
                  child: CircularProgressIndicator(
                    strokeWidth: 2,
                    color: Colors.white,
                  ),
                )
              : const Icon(Icons.my_location, color: Colors.white, size: 22),
        ),
      ),
    );
  }
}
