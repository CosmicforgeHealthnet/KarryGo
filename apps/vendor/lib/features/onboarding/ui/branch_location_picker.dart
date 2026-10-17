import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;
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
  Future<bool> Function()? _openSettings;

  Future<void> _locate() async {
    setState(() {
      _locating = true;
      _error = null;
    });
    _openSettings = null;
    try {
      // Ask for permission first. Some phones (e.g. TECNO) report location
      // services as off when they are on, so a service check before this
      // would stop the permission prompt from ever appearing.
      var permission = await Geolocator.checkPermission();
      if (permission == LocationPermission.denied) {
        permission = await Geolocator.requestPermission();
      }
      if (permission == LocationPermission.deniedForever) {
        _openSettings = Geolocator.openAppSettings;
        throw const FormatException(
          'Location access is turned off for Karryam. Allow it in settings, or place the pin on the map.',
        );
      }
      if (permission == LocationPermission.denied) {
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
    } on LocationServiceDisabledException {
      if (!mounted) return;
      _openSettings = Geolocator.openLocationSettings;
      setState(() => _error = 'Turn on location, or place the pin on the map.');
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

  final _search = TextEditingController();
  bool _searching = false;

  /// Finds a typed address in Nigeria (OpenStreetMap Nominatim, one request
  /// per search press, within its usage policy) and moves the pin there.
  Future<void> _find() async {
    final query = _search.text.trim();
    if (query.length < 3 || _searching) return;
    FocusScope.of(context).unfocus();
    setState(() {
      _searching = true;
      _error = null;
      _openSettings = null;
    });
    try {
      final uri = Uri.https('nominatim.openstreetmap.org', '/search', {
        'q': query,
        'countrycodes': 'ng',
        'format': 'jsonv2',
        'limit': '1',
      });
      final res = await http
          .get(
            uri,
            headers: {'User-Agent': 'KarryamVendor/1.0 (com.karryam.vendor)'},
          )
          .timeout(const Duration(seconds: 15));
      final results = res.statusCode == 200 ? jsonDecode(res.body) : null;
      if (results is! List || results.isEmpty) {
        throw const FormatException(
          'We couldn’t find that address. Try adding the area or city, or place the pin on the map.',
        );
      }
      final first = results.first as Map<String, dynamic>;
      final lat = double.tryParse('${first['lat']}');
      final lon = double.tryParse('${first['lon']}');
      if (lat == null || lon == null) {
        throw const FormatException(
          'We couldn’t find that address. Place the pin on the map.',
        );
      }
      if (!mounted) return;
      final selected = LatLng(lat, lon);
      setState(() => _selected = selected);
      _map.move(selected, 17);
    } catch (error) {
      if (!mounted) return;
      setState(
        () => _error = error is FormatException
            ? error.message
            : 'We couldn’t search right now. Check your internet connection, or place the pin on the map.',
      );
    } finally {
      if (mounted) setState(() => _searching = false);
    }
  }

  @override
  void dispose() {
    _search.dispose();
    _map.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Branch location')),
    body: Column(
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 12),
          child: TextField(
            controller: _search,
            textInputAction: TextInputAction.search,
            onSubmitted: (_) => _find(),
            decoration: InputDecoration(
              hintText: 'Search address, e.g. 12 Allen Avenue, Ikeja',
              prefixIcon: const Icon(Icons.search_rounded),
              suffixIcon: _searching
                  ? const Padding(
                      padding: EdgeInsets.all(14),
                      child: SizedBox(
                        width: 18,
                        height: 18,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      ),
                    )
                  : IconButton(
                      tooltip: 'Search',
                      icon: const Icon(Icons.arrow_forward_rounded),
                      onPressed: _find,
                    ),
            ),
          ),
        ),
        const Padding(
          padding: EdgeInsets.fromLTRB(20, 0, 20, 12),
          child: Text(
            'Or tap the map to place your branch pin, or use your location if you are at the branch.',
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
                if (_error != null && _openSettings != null)
                  TextButton(
                    onPressed: () => _openSettings?.call(),
                    child: const Text('Open settings'),
                  ),
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
