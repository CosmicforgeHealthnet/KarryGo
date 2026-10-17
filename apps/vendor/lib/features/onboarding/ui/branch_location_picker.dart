import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:geolocator/geolocator.dart';
import 'package:latlong2/latlong.dart';
import '../../../core/widgets/vendor_widgets.dart';

class BranchLocationPicker extends StatefulWidget {
  const BranchLocationPicker({super.key, this.initial});
  final LatLng? initial;
  @override
  State<BranchLocationPicker> createState() => _BranchLocationPickerState();
}

class _BranchLocationPickerState extends State<BranchLocationPicker> {
  final _map = MapController();
  late LatLng? _selected = widget.initial;
  bool _locating = false;
  String? _error;

  Future<void> _locate() async {
    setState(() {
      _locating = true;
      _error = null;
    });
    try {
      if (!await Geolocator.isLocationServiceEnabled()) {
        throw const FormatException(
          'Turn on location, or place the pin on the map.',
        );
      }
      var permission = await Geolocator.checkPermission();
      if (permission == LocationPermission.denied) {
        permission = await Geolocator.requestPermission();
      }
      if (permission == LocationPermission.denied ||
          permission == LocationPermission.deniedForever) {
        throw const FormatException(
          'Location permission was not granted. You can still choose the branch on the map.',
        );
      }
      final point = await Geolocator.getCurrentPosition(
        locationSettings: const LocationSettings(
          accuracy: LocationAccuracy.high,
          timeLimit: Duration(seconds: 20),
        ),
      );
      if (!mounted) return;
      final selected = LatLng(point.latitude, point.longitude);
      setState(() => _selected = selected);
      _map.move(selected, 17);
    } catch (error) {
      if (!mounted) return;
      setState(
        () => _error = error is FormatException
            ? error.message
            : 'Your location could not be found. Try again or choose the branch on the map.',
      );
    } finally {
      if (mounted) setState(() => _locating = false);
    }
  }

  @override
  void dispose() {
    _map.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Branch location')),
    body: Column(
      children: [
        const Padding(
          padding: EdgeInsets.all(20),
          child: Text(
            'Tap the map to place your branch pin. Use your location if you are at the branch.',
          ),
        ),
        Expanded(
          child: FlutterMap(
            mapController: _map,
            options: MapOptions(
              initialCenter: widget.initial ?? const LatLng(9.082, 8.6753),
              initialZoom: widget.initial == null ? 6 : 16,
              onTap: (_, point) => setState(() => _selected = point),
            ),
            children: [
              TileLayer(
                urlTemplate: 'https://tile.openstreetmap.org/{z}/{x}/{y}.png',
                userAgentPackageName: 'com.karryam.vendor',
              ),
              if (_selected != null)
                MarkerLayer(
                  markers: [
                    Marker(
                      point: _selected!,
                      width: 52,
                      height: 52,
                      alignment: Alignment.topCenter,
                      child: Icon(
                        Icons.location_pin,
                        size: 48,
                        color: Theme.of(context).colorScheme.primary,
                      ),
                    ),
                  ],
                ),
              const SimpleAttributionWidget(
                source: Text('OpenStreetMap contributors'),
              ),
            ],
          ),
        ),
        SafeArea(
          top: false,
          child: Padding(
            padding: const EdgeInsets.all(20),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                if (_error != null) InlineNotice(message: _error!, error: true),
                TextButton.icon(
                  onPressed: _locating ? null : _locate,
                  icon: _locating
                      ? const SizedBox(
                          width: 18,
                          height: 18,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : const Icon(Icons.my_location_rounded),
                  label: Text(
                    _locating
                        ? 'Finding your location…'
                        : 'Use my current location',
                  ),
                ),
                PrimaryAction(
                  label: 'Confirm branch location',
                  onPressed: _selected == null || _locating
                      ? null
                      : () => Navigator.pop(context, _selected),
                ),
              ],
            ),
          ),
        ),
      ],
    ),
  );
}
