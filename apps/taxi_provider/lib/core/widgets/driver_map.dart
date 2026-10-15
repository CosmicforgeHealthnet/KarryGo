import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:geolocator/geolocator.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';
import '../../features/home/models/provider_booking.dart';
import '../theme/karryam_theme.dart';

/// Real map tiles and booking coordinates. No invented road route or ETA.
/// Enable after providing restricted native Maps SDK keys (see README).
class DriverMap extends StatefulWidget {
  const DriverMap({super.key, this.booking, this.online = false});
  final ProviderBooking? booking;
  final bool online;
  @override
  State<DriverMap> createState() => _DriverMapState();
}

class _DriverMapState extends State<DriverMap> {
  GoogleMapController? _map;
  LatLng? _position;
  bool get _enabled =>
      const bool.fromEnvironment('GOOGLE_MAPS_ENABLED') &&
      !kIsWeb &&
      (defaultTargetPlatform == TargetPlatform.android ||
          defaultTargetPlatform == TargetPlatform.iOS);
  @override
  void initState() {
    super.initState();
    if (_enabled) _locate();
  }

  Future<void> _locate() async {
    try {
      final permission = await Geolocator.checkPermission();
      if (permission != LocationPermission.always &&
          permission != LocationPermission.whileInUse) {
        return;
      }
      final position = await Geolocator.getCurrentPosition(
        locationSettings: const LocationSettings(
          timeLimit: Duration(seconds: 10),
        ),
      );
      if (!mounted) return;
      setState(() => _position = LatLng(position.latitude, position.longitude));
      if (widget.booking == null) {
        await _map?.animateCamera(CameraUpdate.newLatLng(_position!));
      }
    } catch (_) {
      /* Booking markers remain usable without a device fix. */
    }
  }

  @override
  void didUpdateWidget(covariant DriverMap oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (_enabled && widget.online && !oldWidget.online) _locate();
    if (oldWidget.booking?.id != widget.booking?.id) {
      final pickup = widget.booking?.pickup;
      if (pickup != null) {
        _map?.animateCamera(
          CameraUpdate.newLatLng(LatLng(pickup.lat, pickup.lng)),
        );
      }
    }
  }

  @override
  void dispose() {
    _map?.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    final pickup = widget.booking?.pickup;
    final dropoff = widget.booking?.dropoff;
    if (!_enabled) {
      return Stack(
        fit: StackFit.expand,
        children: [
          CustomPaint(painter: _MapPlaceholder(colors.border, colors.surface)),
          Center(
            child: Container(
              padding: const EdgeInsets.all(14),
              decoration: BoxDecoration(
                color: colors.card,
                borderRadius: BorderRadius.circular(14),
              ),
              child: const Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(Icons.map_outlined),
                  SizedBox(height: 8),
                  Text('Map unavailable'),
                  SizedBox(height: 4),
                  Text(
                    'Trip addresses remain available.',
                    style: TextStyle(fontSize: 12),
                  ),
                ],
              ),
            ),
          ),
        ],
      );
    }
    return GoogleMap(
      initialCameraPosition: CameraPosition(
        target: pickup == null
            ? (_position ?? const LatLng(6.5244, 3.3792))
            : LatLng(pickup.lat, pickup.lng),
        zoom: 13,
      ),
      onMapCreated: (controller) => _map = controller,
      myLocationEnabled: _position != null,
      myLocationButtonEnabled: false,
      zoomControlsEnabled: false,
      mapToolbarEnabled: false,
      style: colors.dark
          ? '[{"elementType":"geometry","stylers":[{"color":"#12284c"}]},{"elementType":"labels.text.fill","stylers":[{"color":"#ced2da"}]},{"elementType":"labels.text.stroke","stylers":[{"color":"#0b1f44"}]},{"featureType":"water","elementType":"geometry","stylers":[{"color":"#103c43"}]}]'
          : null,
      markers: {
        if (pickup != null)
          Marker(
            markerId: const MarkerId('pickup'),
            position: LatLng(pickup.lat, pickup.lng),
            infoWindow: InfoWindow(title: 'Pickup', snippet: pickup.address),
            icon: BitmapDescriptor.defaultMarkerWithHue(
              BitmapDescriptor.hueCyan,
            ),
          ),
        if (dropoff != null)
          Marker(
            markerId: const MarkerId('dropoff'),
            position: LatLng(dropoff.lat, dropoff.lng),
            infoWindow: InfoWindow(title: 'Drop-off', snippet: dropoff.address),
          ),
      },
    );
  }
}

class _MapPlaceholder extends CustomPainter {
  const _MapPlaceholder(this.line, this.background);
  final Color line, background;
  @override
  void paint(Canvas canvas, Size size) {
    canvas.drawRect(Offset.zero & size, Paint()..color = background);
    final paint = Paint()
      ..color = line
      ..strokeWidth = 10;
    for (double x = -size.height; x < size.width; x += 70) {
      canvas.drawLine(
        Offset(x, 0),
        Offset(x + size.height * .4, size.height),
        paint,
      );
    }
    for (double y = 0; y < size.height; y += 85) {
      canvas.drawLine(Offset(0, y), Offset(size.width, y), paint);
    }
  }

  @override
  bool shouldRepaint(covariant _MapPlaceholder old) =>
      old.line != line || old.background != background;
}
