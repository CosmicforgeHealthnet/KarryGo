import 'dart:async';

import 'package:flutter/material.dart';
import 'package:geolocator/geolocator.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';
import 'package:permission_handler/permission_handler.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../../hauling/data/places_api.dart';

/// Result returned by [SavedLocationMapPickerScreen] when the user confirms.
class SavedLocationPickResult {
  const SavedLocationPickResult({
    required this.address,
    required this.latitude,
    required this.longitude,
  });

  final String address;
  final double latitude;
  final double longitude;
}

/// Figma 1779-1781: full-screen map with a search pill, autocomplete dropdown,
/// green marker on the chosen spot, and a bottom "Confirm Location" pill.
class SavedLocationMapPickerScreen extends StatefulWidget {
  const SavedLocationMapPickerScreen({
    super.key,
    required this.placesApi,
    this.initialAddress,
    this.initialLatLng,
  });

  final PlacesApi placesApi;
  final String? initialAddress;
  final LatLng? initialLatLng;

  @override
  State<SavedLocationMapPickerScreen> createState() =>
      _SavedLocationMapPickerScreenState();
}

class _SavedLocationMapPickerScreenState
    extends State<SavedLocationMapPickerScreen> {
  static const _lagosCenter = LatLng(6.5244, 3.3792);

  final _searchCtrl = TextEditingController();
  final _mapController = Completer<GoogleMapController>();
  Timer? _debounce;

  List<PlaceSuggestion> _suggestions = [];
  bool _suggestionsLoading = false;
  String? _suggestionsError;
  // resolving the selected suggestion's coordinates (or a map tap's address)
  bool _resolving = false;
  // resolving the device's current location
  bool _locating = false;

  LatLng? _selected;
  String _selectedAddress = '';

  @override
  void initState() {
    super.initState();
    final initial = widget.initialLatLng;
    if (initial != null && !(initial.latitude == 0 && initial.longitude == 0)) {
      _selected = initial;
      _selectedAddress = widget.initialAddress ?? '';
      _searchCtrl.text = _selectedAddress;
    }
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _searchCtrl.dispose();
    super.dispose();
  }

  void _onQueryChanged(String val) {
    setState(() {
      _suggestions = [];
      _suggestionsError = null;
    });
    _debounce?.cancel();
    if (val.trim().length < 2) return;
    _debounce = Timer(const Duration(milliseconds: 300), () async {
      if (!mounted) return;
      setState(() => _suggestionsLoading = true);
      try {
        final results = await widget.placesApi.autocomplete(val.trim());
        if (!mounted) return;
        setState(() {
          _suggestions = results;
          _suggestionsLoading = false;
        });
      } on PlacesApiException catch (e) {
        if (!mounted) return;
        setState(() {
          _suggestions = [];
          _suggestionsError = e.status == 'NETWORK_ERROR'
              ? 'No internet connection for location search.'
              : 'Could not load suggestions.';
          _suggestionsLoading = false;
        });
      }
    });
  }

  Future<void> _selectSuggestion(PlaceSuggestion suggestion) async {
    FocusScope.of(context).unfocus();
    _searchCtrl.text = suggestion.description;
    setState(() {
      _suggestions = [];
      _resolving = true;
    });
    final coords = await widget.placesApi.getLatLng(suggestion.placeId);
    if (!mounted) return;
    if (coords == null) {
      setState(() {
        _resolving = false;
        _suggestionsError = 'Could not locate that address. Try another.';
      });
      return;
    }
    final latLng = LatLng(coords.lat, coords.lng);
    setState(() {
      _selected = latLng;
      _selectedAddress = suggestion.description;
      _resolving = false;
    });
    await _animateTo(latLng);
  }

  Future<void> _onMapTap(LatLng latLng) async {
    FocusScope.of(context).unfocus();
    setState(() {
      _selected = latLng;
      _suggestions = [];
      _suggestionsError = null;
      _resolving = true;
    });
    final address =
        await widget.placesApi.reverseGeocode(latLng.latitude, latLng.longitude);
    if (!mounted) return;
    setState(() {
      _selectedAddress = address ??
          '${latLng.latitude.toStringAsFixed(5)}, ${latLng.longitude.toStringAsFixed(5)}';
      _searchCtrl.text = _selectedAddress;
      _resolving = false;
    });
    await _animateTo(latLng);
  }

  /// Resolves the device's current position, reverse-geocodes it, drops the
  /// marker there, and fills the search field. Mirrors the permission /
  /// service handling used by the hauling location entry.
  Future<void> _useCurrentLocation() async {
    if (_locating) return;
    FocusScope.of(context).unfocus();
    setState(() {
      _locating = true;
      _suggestions = [];
      _suggestionsError = null;
    });
    try {
      var permission = await Permission.locationWhenInUse.status;
      if (!permission.isGranted && !permission.isLimited) {
        permission = await Permission.locationWhenInUse.request();
      }
      if (!permission.isGranted && !permission.isLimited) {
        _showSnack('Enable location access to use your current location.');
        return;
      }
      if (!await Geolocator.isLocationServiceEnabled()) {
        _showSnack('Turn on device location to use your current location.');
        return;
      }

      final pos = await Geolocator.getCurrentPosition(
        locationSettings:
            const LocationSettings(accuracy: LocationAccuracy.high),
      );
      if (!mounted) return;
      final latLng = LatLng(pos.latitude, pos.longitude);
      final address =
          await widget.placesApi.reverseGeocode(pos.latitude, pos.longitude) ??
              'Current location';
      if (!mounted) return;
      setState(() {
        _selected = latLng;
        _selectedAddress = address;
        _searchCtrl.text = address;
      });
      await _animateTo(latLng);
    } catch (_) {
      _showSnack('Could not get your location. Try again.');
    } finally {
      if (mounted) setState(() => _locating = false);
    }
  }

  void _showSnack(String message) {
    if (!mounted) return;
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(content: Text(message)));
  }

  Future<void> _animateTo(LatLng latLng) async {
    final ctrl = await _mapController.future;
    await ctrl.animateCamera(CameraUpdate.newLatLngZoom(latLng, 16));
  }

  void _confirm() {
    final selected = _selected;
    if (selected == null) return;
    Navigator.of(context).pop(SavedLocationPickResult(
      address: _selectedAddress,
      latitude: selected.latitude,
      longitude: selected.longitude,
    ));
  }

  @override
  Widget build(BuildContext context) {
    final canConfirm = _selected != null && !_resolving;

    return Scaffold(
      resizeToAvoidBottomInset: false,
      body: Stack(
        children: [
          // Full-screen map
          Positioned.fill(
            child: GoogleMap(
              initialCameraPosition: CameraPosition(
                target: _selected ?? _lagosCenter,
                zoom: _selected != null ? 16 : 13,
              ),
              markers: {
                if (_selected != null)
                  Marker(
                    markerId: const MarkerId('picked'),
                    position: _selected!,
                    icon: BitmapDescriptor.defaultMarkerWithHue(
                      BitmapDescriptor.hueGreen,
                    ),
                  ),
              },
              onTap: _onMapTap,
              myLocationEnabled: false,
              myLocationButtonEnabled: false,
              zoomControlsEnabled: false,
              mapToolbarEnabled: false,
              compassEnabled: false,
              onMapCreated: (ctrl) {
                if (!_mapController.isCompleted) _mapController.complete(ctrl);
              },
            ),
          ),

          // Top overlay: back circle + search pill + suggestions dropdown
          Positioned(
            top: 0,
            left: 0,
            right: 0,
            child: SafeArea(
              bottom: false,
              child: Padding(
                padding: const EdgeInsets.fromLTRB(12, 8, 16, 0),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Material(
                      color: Colors.white,
                      shape: const CircleBorder(),
                      elevation: 3,
                      shadowColor: const Color(0x33000000),
                      child: InkWell(
                        customBorder: const CircleBorder(),
                        onTap: () => Navigator.of(context).pop(),
                        child: const Padding(
                          padding: EdgeInsets.all(10),
                          child: Icon(Icons.arrow_back,
                              color: CustomerFigmaColors.text, size: 20),
                        ),
                      ),
                    ),
                    const SizedBox(width: 10),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          _SearchPill(
                            controller: _searchCtrl,
                            loading: _suggestionsLoading || _resolving,
                            onChanged: _onQueryChanged,
                          ),
                          if (_suggestionsError != null) ...[
                            const SizedBox(height: 6),
                            _SuggestionsErrorCard(message: _suggestionsError!),
                          ],
                          if (_suggestions.isNotEmpty) ...[
                            const SizedBox(height: 6),
                            _SuggestionsDropdown(
                              suggestions: _suggestions,
                              onSelect: _selectSuggestion,
                            ),
                          ],
                        ],
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),

          // Bottom actions: "Use current location" + confirm pill
          Positioned(
            left: 16,
            right: 16,
            bottom: 0,
            child: SafeArea(
              top: false,
              child: Padding(
                padding: const EdgeInsets.only(bottom: 12),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    _UseCurrentLocationButton(
                      loading: _locating,
                      onTap: _locating ? null : _useCurrentLocation,
                    ),
                    const SizedBox(height: 12),
                    FigmaPrimaryButton(
                      label: 'Confirm Location',
                      isLoading: _resolving,
                      onPressed: canConfirm ? _confirm : null,
                    ),
                  ],
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

// ─── Search pill ──────────────────────────────────────────────────────────────

class _SearchPill extends StatelessWidget {
  const _SearchPill({
    required this.controller,
    required this.loading,
    required this.onChanged,
  });

  final TextEditingController controller;
  final bool loading;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 44,
      padding: const EdgeInsets.symmetric(horizontal: 14),
      decoration: const BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.all(Radius.circular(999)),
        boxShadow: [
          BoxShadow(
            color: Color(0x1F000000),
            blurRadius: 10,
            offset: Offset(0, 3),
          ),
        ],
      ),
      child: Row(
        children: [
          loading
              ? const SizedBox(
                  width: 16,
                  height: 16,
                  child: CircularProgressIndicator(
                    strokeWidth: 2,
                    color: CustomerFigmaColors.primary,
                  ),
                )
              : const Icon(Icons.location_on,
                  color: CustomerFigmaColors.primary, size: 18),
          const SizedBox(width: 8),
          Expanded(
            child: TextField(
              controller: controller,
              onChanged: onChanged,
              style: const TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 13,
                fontWeight: FontWeight.w600,
              ),
              decoration: const InputDecoration(
                hintText: 'Enter location',
                hintStyle:
                    TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
                border: InputBorder.none,
                enabledBorder: InputBorder.none,
                focusedBorder: InputBorder.none,
                isDense: true,
                contentPadding: EdgeInsets.zero,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

// ─── Use current location button ──────────────────────────────────────────────

class _UseCurrentLocationButton extends StatelessWidget {
  const _UseCurrentLocationButton({required this.loading, required this.onTap});

  final bool loading;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.white,
      borderRadius: BorderRadius.circular(999),
      elevation: 3,
      shadowColor: const Color(0x22000000),
      child: InkWell(
        borderRadius: BorderRadius.circular(999),
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 11),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              loading
                  ? const SizedBox(
                      width: 18,
                      height: 18,
                      child: CircularProgressIndicator(
                        strokeWidth: 2,
                        color: CustomerFigmaColors.primary,
                      ),
                    )
                  : const Icon(Icons.my_location,
                      color: CustomerFigmaColors.primary, size: 18),
              const SizedBox(width: 8),
              const Text(
                'Use my current location',
                style: TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 13,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

// ─── Suggestions dropdown ─────────────────────────────────────────────────────

class _SuggestionsDropdown extends StatelessWidget {
  const _SuggestionsDropdown({
    required this.suggestions,
    required this.onSelect,
  });

  final List<PlaceSuggestion> suggestions;
  final ValueChanged<PlaceSuggestion> onSelect;

  @override
  Widget build(BuildContext context) {
    return Container(
      constraints: const BoxConstraints(maxHeight: 220),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(12),
        boxShadow: const [
          BoxShadow(
            color: Color(0x1F000000),
            blurRadius: 10,
            offset: Offset(0, 3),
          ),
        ],
      ),
      child: ListView.separated(
        shrinkWrap: true,
        padding: EdgeInsets.zero,
        itemCount: suggestions.length,
        separatorBuilder: (_, _) =>
            const Divider(height: 1, color: CustomerFigmaColors.border),
        itemBuilder: (context, i) {
          final s = suggestions[i];
          return InkWell(
            onTap: () => onSelect(s),
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 11),
              child: Text(
                s.description,
                style: const TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 12.5,
                ),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ),
          );
        },
      ),
    );
  }
}

// ─── Error card ───────────────────────────────────────────────────────────────

class _SuggestionsErrorCard extends StatelessWidget {
  const _SuggestionsErrorCard({required this.message});

  final String message;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: const Color(0xFFFDECEC),
        borderRadius: BorderRadius.circular(10),
      ),
      child: Row(
        children: [
          const Icon(Icons.error_outline, color: Colors.red, size: 16),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              message,
              style: const TextStyle(color: Colors.red, fontSize: 12),
            ),
          ),
        ],
      ),
    );
  }
}
