import 'dart:async';

import 'package:flutter/material.dart';
import 'package:geolocator/geolocator.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';
import 'package:permission_handler/permission_handler.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../../auth/models/customer_auth_models.dart';
import '../../data/places_api.dart';
import '../../state/hauling_booking_controller.dart';
import '../widgets/hauling_map_widget.dart';

class HaulingLocationEntryView extends StatefulWidget {
  const HaulingLocationEntryView({
    super.key,
    required this.controller,
    required this.placesApi,
  });

  final HaulingBookingController controller;
  final PlacesApi placesApi;

  @override
  State<HaulingLocationEntryView> createState() => _HaulingLocationEntryViewState();
}

class _HaulingLocationEntryViewState extends State<HaulingLocationEntryView> {
  final _pickupCtrl = TextEditingController();
  final _dropoffCtrl = TextEditingController();
  Timer? _debounce;

  List<PlaceSuggestion> _pickupSuggestions = [];
  List<PlaceSuggestion> _dropoffSuggestions = [];
  bool _pickupLoading = false;
  bool _dropoffLoading = false;
  // true while resolving the device's current location
  bool _locatingCurrent = false;
  // surfaced Places API failure (REQUEST_DENIED, billing, network, etc.)
  String? _suggestionsError;
  // tracks which field is active to show its suggestions
  _ActiveField? _activeField;
  // one-tap saved locations (Home, Office, ...) — empty when unavailable
  List<SavedLocation> _savedLocations = const [];

  HaulingBookingController get _ctrl => widget.controller;
  HaulingBookingState get _state => _ctrl.state;

  LatLng? get _pickupLatLng =>
      _state.pickupAddress.isNotEmpty ? LatLng(_state.pickupLat, _state.pickupLng) : null;
  LatLng? get _dropoffLatLng =>
      _state.dropoffAddress.isNotEmpty ? LatLng(_state.dropoffLat, _state.dropoffLng) : null;

  // Require resolved (non-null-island) coordinates for both points: a bare
  // address string isn't enough — the backend rejects (0,0) and matching needs
  // real geometry to compute distance and find nearby trucks.
  bool get _canProceed =>
      _hasRealCoords(_state.pickupLat, _state.pickupLng) &&
      _hasRealCoords(_state.dropoffLat, _state.dropoffLng);

  bool _hasRealCoords(double lat, double lng) => !(lat == 0 && lng == 0);

  @override
  void initState() {
    super.initState();
    _pickupCtrl.text = _state.pickupAddress;
    _dropoffCtrl.text = _state.dropoffAddress;
    _loadSavedLocations();
  }

  Future<void> _loadSavedLocations() async {
    final loader = _ctrl.savedLocationsLoader;
    if (loader == null) return;
    try {
      final locations = await loader();
      if (!mounted) return;
      setState(() => _savedLocations = locations);
    } catch (_) {
      // Saved locations are a convenience — booking works without them.
    }
  }

  void _selectSavedLocation(SavedLocation location) {
    FocusScope.of(context).unfocus();
    final target = _activeField ?? _ActiveField.pickup;
    if (target == _ActiveField.pickup) {
      _pickupCtrl.text = location.address;
      _ctrl.setPickupLocation(location.address, location.latitude, location.longitude);
      setState(() {
        _pickupSuggestions = [];
        _activeField = null;
      });
    } else {
      _dropoffCtrl.text = location.address;
      _ctrl.setDropoffLocation(location.address, location.latitude, location.longitude);
      setState(() {
        _dropoffSuggestions = [];
        _activeField = null;
      });
    }
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _pickupCtrl.dispose();
    _dropoffCtrl.dispose();
    super.dispose();
  }

  void _onPickupChanged(String val) {
    _ctrl.setPickupLocation('', 0, 0); // clear stored location when user types
    setState(() {
      _pickupSuggestions = [];
      _suggestionsError = null;
      _activeField = _ActiveField.pickup;
    });
    _debounce?.cancel();
    if (val.trim().length < 2) return;
    _debounce = Timer(const Duration(milliseconds: 300), () async {
      if (!mounted) return;
      setState(() => _pickupLoading = true);
      try {
        final results = await widget.placesApi.autocomplete(val.trim());
        if (!mounted) return;
        setState(() {
          _pickupSuggestions = results;
          _suggestionsError = null;
          _pickupLoading = false;
        });
      } on PlacesApiException catch (e) {
        if (!mounted) return;
        setState(() {
          _pickupSuggestions = [];
          _suggestionsError = _friendlyPlacesError(e);
          _pickupLoading = false;
        });
      }
    });
  }

  void _onDropoffChanged(String val) {
    _ctrl.setDropoffLocation('', 0, 0); // clear stored location when user types
    setState(() {
      _dropoffSuggestions = [];
      _suggestionsError = null;
      _activeField = _ActiveField.dropoff;
    });
    _debounce?.cancel();
    if (val.trim().length < 2) return;
    _debounce = Timer(const Duration(milliseconds: 300), () async {
      if (!mounted) return;
      setState(() => _dropoffLoading = true);
      try {
        final results = await widget.placesApi.autocomplete(val.trim());
        if (!mounted) return;
        setState(() {
          _dropoffSuggestions = results;
          _suggestionsError = null;
          _dropoffLoading = false;
        });
      } on PlacesApiException catch (e) {
        if (!mounted) return;
        setState(() {
          _dropoffSuggestions = [];
          _suggestionsError = _friendlyPlacesError(e);
          _dropoffLoading = false;
        });
      }
    });
  }

  String _friendlyPlacesError(PlacesApiException e) {
    switch (e.status) {
      case 'REQUEST_DENIED':
        return 'Location search is unavailable (key/Places API not enabled).';
      case 'OVER_QUERY_LIMIT':
        return 'Location search quota exceeded. Try again later.';
      case 'NETWORK_ERROR':
        return 'No internet connection for location search.';
      case 'MISSING_KEY':
        return 'Location search is not configured.';
      default:
        return 'Could not load suggestions: ${e.status}';
    }
  }

  void _showSnack(String message) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(message), duration: const Duration(seconds: 2)),
    );
  }

  /// Resolves the device's current position, reverse-geocodes it to an address,
  /// and fills the active field (defaults to pickup). Mirrors the permission /
  /// service handling used by the home-map "locate me" button.
  Future<void> _useCurrentLocation() async {
    if (_locatingCurrent) return;
    final target = _activeField ?? _ActiveField.pickup;
    setState(() {
      _locatingCurrent = true;
      if (target == _ActiveField.pickup) {
        _pickupLoading = true;
      } else {
        _dropoffLoading = true;
      }
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
        locationSettings: const LocationSettings(accuracy: LocationAccuracy.high),
      );
      if (!mounted) return;
      final address = await widget.placesApi.reverseGeocode(pos.latitude, pos.longitude) ??
          'Current location';
      if (!mounted) return;
      if (target == _ActiveField.pickup) {
        _pickupCtrl.text = address;
        _ctrl.setPickupLocation(address, pos.latitude, pos.longitude);
        setState(() => _pickupSuggestions = []);
      } else {
        _dropoffCtrl.text = address;
        _ctrl.setDropoffLocation(address, pos.latitude, pos.longitude);
        setState(() => _dropoffSuggestions = []);
      }
    } catch (_) {
      _showSnack('Could not get your location. Try again.');
    } finally {
      if (mounted) {
        setState(() {
          _locatingCurrent = false;
          _pickupLoading = false;
          _dropoffLoading = false;
          _activeField = null;
        });
      }
    }
  }

  Future<void> _selectPickup(PlaceSuggestion suggestion) async {
    _pickupCtrl.text = suggestion.description;
    setState(() {
      _pickupSuggestions = [];
      _pickupLoading = true;
    });
    final coords = await widget.placesApi.getLatLng(suggestion.placeId);
    if (!mounted) return;
    if (coords != null) {
      _ctrl.setPickupLocation(suggestion.description, coords.lat, coords.lng);
    } else {
      // fallback: store description with zero coords — user can still proceed
      _ctrl.setPickupLocation(suggestion.description, 0, 0);
    }
    setState(() {
      _pickupLoading = false;
      _activeField = null;
    });
  }

  Future<void> _selectDropoff(PlaceSuggestion suggestion) async {
    _dropoffCtrl.text = suggestion.description;
    setState(() {
      _dropoffSuggestions = [];
      _dropoffLoading = true;
    });
    final coords = await widget.placesApi.getLatLng(suggestion.placeId);
    if (!mounted) return;
    if (coords != null) {
      _ctrl.setDropoffLocation(suggestion.description, coords.lat, coords.lng);
    } else {
      _ctrl.setDropoffLocation(suggestion.description, 0, 0);
    }
    setState(() {
      _dropoffLoading = false;
      _activeField = null;
    });
  }

  @override
  Widget build(BuildContext context) {
    final suggestions = _activeField == _ActiveField.pickup
        ? _pickupSuggestions
        : _dropoffSuggestions;
    final isSuggestionsLoading = _activeField == _ActiveField.pickup
        ? _pickupLoading
        : _dropoffLoading;
    // Saved locations show as one-tap fills while the active field (pickup by
    // default) has no real query — once the user types, Places suggestions
    // take over the space.
    final activeQuery = _activeField == _ActiveField.dropoff
        ? _dropoffCtrl.text
        : _pickupCtrl.text;
    final showSavedLocations = _savedLocations.isNotEmpty &&
        suggestions.isEmpty &&
        !isSuggestionsLoading &&
        activeQuery.trim().length < 2;

    return Scaffold(
      // Keyboard handling is done manually below with a viewInsets padding on
      // the bottom sheet. The default resizeToAvoidBottomInset:true must be
      // off, or the body shrinks by the keyboard height AND the padding lifts
      // the sheet again (the padding reads viewInsets from the context above
      // this Scaffold, where the inset is not consumed) — the double offset
      // shoved the sheet to the top of the screen on focus. Keeping the body
      // full-size also spares the GoogleMap a re-layout on every keyboard
      // open/close.
      resizeToAvoidBottomInset: false,
      body: Stack(
        children: [
          // Full-screen map background
          Positioned.fill(
            child: HaulingMapWidget(
              pickupLatLng: _pickupLatLng,
              dropoffLatLng: _dropoffLatLng,
            ),
          ),

          // Back button
          Positioned(
            top: MediaQuery.of(context).padding.top + 8,
            left: 12,
            child: Material(
              color: Colors.white,
              shape: const CircleBorder(),
              elevation: 2,
              child: InkWell(
                customBorder: const CircleBorder(),
                onTap: () => Navigator.of(context).pop(),
                child: const Padding(
                  padding: EdgeInsets.all(8),
                  child: Icon(Icons.arrow_back, color: CustomerFigmaColors.text, size: 20),
                ),
              ),
            ),
          ),

          // Bottom panel. Padded on the OUTSIDE by the keyboard inset so the
          // whole sheet lifts to rest just above the keyboard. This is the only
          // keyboard compensation — resizeToAvoidBottomInset is false on the
          // Scaffold above, otherwise the sheet is lifted twice.
          Padding(
            padding: EdgeInsets.only(bottom: MediaQuery.viewInsetsOf(context).bottom),
            child: Align(
              alignment: Alignment.bottomCenter,
              child: Container(
              constraints: BoxConstraints(
                maxHeight: MediaQuery.sizeOf(context).height * 0.7,
              ),
              decoration: const BoxDecoration(
                color: Colors.white,
                borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
              ),
              padding: const EdgeInsets.only(
                left: 20,
                right: 20,
                top: 12,
                bottom: 20,
              ),
              child: SingleChildScrollView(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                  // Drag handle
                  Center(
                    child: Container(
                      width: 36, height: 4,
                      decoration: BoxDecoration(
                        color: Colors.grey[300],
                        borderRadius: BorderRadius.circular(2),
                      ),
                    ),
                  ),
                  const SizedBox(height: 16),

                  // Discount banner — matches mockup 1733
                  Container(
                    padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
                    decoration: BoxDecoration(
                      color: Colors.white,
                      borderRadius: BorderRadius.circular(12),
                      border: Border.all(color: CustomerFigmaColors.border),
                    ),
                    child: Row(
                      children: [
                        Container(
                          width: 32, height: 32,
                          decoration: const BoxDecoration(
                            color: CustomerFigmaColors.primary,
                            shape: BoxShape.circle,
                          ),
                          child: const Icon(Icons.check, color: Colors.white, size: 18),
                        ),
                        const SizedBox(width: 12),
                        const Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(
                                '10% off first booking',
                                style: TextStyle(
                                  color: CustomerFigmaColors.text,
                                  fontSize: 13,
                                  fontWeight: FontWeight.w700,
                                ),
                              ),
                              Text(
                                'View Details',
                                style: TextStyle(
                                  color: CustomerFigmaColors.muted,
                                  fontSize: 11,
                                ),
                              ),
                            ],
                          ),
                        ),
                        const Icon(Icons.chevron_right, color: CustomerFigmaColors.muted, size: 18),
                      ],
                    ),
                  ),
                  const SizedBox(height: 16),

                  const Text(
                    'Book a Truck',
                    style: TextStyle(
                      color: CustomerFigmaColors.text,
                      fontWeight: FontWeight.w800,
                      fontSize: 20,
                    ),
                  ),
                  const SizedBox(height: 2),
                  const Text(
                    'Tell us your destination',
                    style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
                  ),
                  const SizedBox(height: 16),

                  // Pickup + dropoff grouped card
                  _LocationInputCard(
                    pickupCtrl: _pickupCtrl,
                    dropoffCtrl: _dropoffCtrl,
                    pickupLoading: _pickupLoading,
                    dropoffLoading: _dropoffLoading,
                    onPickupChanged: _onPickupChanged,
                    onDropoffChanged: _onDropoffChanged,
                    onPickupTap: () => setState(() => _activeField = _ActiveField.pickup),
                    onDropoffTap: () => setState(() => _activeField = _ActiveField.dropoff),
                  ),

                  const SizedBox(height: 10),
                  // Use my current location — fills the active field (pickup by
                  // default) with the device's reverse-geocoded address.
                  InkWell(
                    onTap: _locatingCurrent ? null : _useCurrentLocation,
                    borderRadius: BorderRadius.circular(10),
                    child: Padding(
                      padding: const EdgeInsets.symmetric(vertical: 6),
                      child: Row(
                        children: [
                          _locatingCurrent
                              ? const SizedBox(
                                  width: 18, height: 18,
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2, color: CustomerFigmaColors.primary,
                                  ),
                                )
                              : const Icon(Icons.my_location,
                                  color: CustomerFigmaColors.primary, size: 18),
                          const SizedBox(width: 8),
                          Text(
                            _activeField == _ActiveField.dropoff
                                ? 'Use my current location for drop off'
                                : 'Use my current location',
                            style: const TextStyle(
                              color: CustomerFigmaColors.primary,
                              fontSize: 13,
                              fontWeight: FontWeight.w600,
                            ),
                          ),
                        ],
                      ),
                    ),
                  ),

                  // Saved locations — one tap fills the active field with the
                  // stored address + coordinates (no Places round-trip).
                  if (showSavedLocations) ...[
                    const SizedBox(height: 8),
                    Container(
                      constraints: const BoxConstraints(maxHeight: 180),
                      decoration: BoxDecoration(
                        color: Colors.white,
                        borderRadius: BorderRadius.circular(10),
                        border: Border.all(color: CustomerFigmaColors.border),
                      ),
                      child: Column(
                        mainAxisSize: MainAxisSize.min,
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          const Padding(
                            padding: EdgeInsets.fromLTRB(14, 10, 14, 4),
                            child: Text(
                              'Saved locations',
                              style: TextStyle(
                                color: CustomerFigmaColors.muted,
                                fontSize: 11,
                                fontWeight: FontWeight.w700,
                              ),
                            ),
                          ),
                          Flexible(
                            child: ListView.separated(
                              shrinkWrap: true,
                              padding: EdgeInsets.zero,
                              itemCount: _savedLocations.length,
                              separatorBuilder: (_, _) => const Divider(height: 1),
                              itemBuilder: (context, i) {
                                final location = _savedLocations[i];
                                return InkWell(
                                  onTap: () => _selectSavedLocation(location),
                                  child: Padding(
                                    padding: const EdgeInsets.symmetric(
                                        horizontal: 14, vertical: 10),
                                    child: Row(
                                      children: [
                                        const Icon(Icons.bookmark_rounded,
                                            color: CustomerFigmaColors.primary,
                                            size: 16),
                                        const SizedBox(width: 8),
                                        Expanded(
                                          child: Column(
                                            crossAxisAlignment:
                                                CrossAxisAlignment.start,
                                            children: [
                                              Text(
                                                location.title,
                                                style: const TextStyle(
                                                  color: CustomerFigmaColors.text,
                                                  fontSize: 13,
                                                  fontWeight: FontWeight.w700,
                                                ),
                                                maxLines: 1,
                                                overflow: TextOverflow.ellipsis,
                                              ),
                                              const SizedBox(height: 2),
                                              Text(
                                                location.address,
                                                style: const TextStyle(
                                                  color: CustomerFigmaColors.muted,
                                                  fontSize: 12,
                                                ),
                                                maxLines: 1,
                                                overflow: TextOverflow.ellipsis,
                                              ),
                                            ],
                                          ),
                                        ),
                                      ],
                                    ),
                                  ),
                                );
                              },
                            ),
                          ),
                        ],
                      ),
                    ),
                  ],

                  // Places API error (REQUEST_DENIED, billing, network, etc.)
                  if (_suggestionsError != null && !isSuggestionsLoading) ...[
                    const SizedBox(height: 8),
                    Container(
                      width: double.infinity,
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
                              _suggestionsError!,
                              style: const TextStyle(color: Colors.red, fontSize: 12),
                            ),
                          ),
                        ],
                      ),
                    ),
                  ],

                  // Suggestions list
                  if (suggestions.isNotEmpty || isSuggestionsLoading) ...[
                    const SizedBox(height: 8),
                    Container(
                      constraints: const BoxConstraints(maxHeight: 180),
                      decoration: BoxDecoration(
                        color: Colors.white,
                        borderRadius: BorderRadius.circular(10),
                        border: Border.all(color: CustomerFigmaColors.border),
                      ),
                      child: isSuggestionsLoading && suggestions.isEmpty
                          ? const Padding(
                              padding: EdgeInsets.all(12),
                              child: Center(child: SizedBox(
                                width: 18, height: 18,
                                child: CircularProgressIndicator(strokeWidth: 2),
                              )),
                            )
                          : ListView.separated(
                              shrinkWrap: true,
                              padding: EdgeInsets.zero,
                              itemCount: suggestions.length,
                              separatorBuilder: (_, _) => const Divider(height: 1),
                              itemBuilder: (context, i) {
                                final s = suggestions[i];
                                return InkWell(
                                  onTap: () => _activeField == _ActiveField.pickup
                                      ? _selectPickup(s)
                                      : _selectDropoff(s),
                                  child: Padding(
                                    padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
                                    child: Row(
                                      children: [
                                        const Icon(Icons.location_on_outlined,
                                            color: CustomerFigmaColors.muted, size: 16),
                                        const SizedBox(width: 8),
                                        Expanded(
                                          child: Text(
                                            s.description,
                                            style: const TextStyle(
                                              color: CustomerFigmaColors.text,
                                              fontSize: 13,
                                            ),
                                            maxLines: 2,
                                            overflow: TextOverflow.ellipsis,
                                          ),
                                        ),
                                      ],
                                    ),
                                  ),
                                );
                              },
                            ),
                    ),
                  ],

                  const SizedBox(height: 20),

                  if (_state.error != null) ...[
                    Text(
                      _state.error!,
                      style: const TextStyle(color: Colors.red, fontSize: 12),
                      textAlign: TextAlign.center,
                    ),
                    const SizedBox(height: 8),
                  ],

                  FigmaPrimaryButton(
                    label: 'Find Truck',
                    isLoading: _state.isLoading,
                    onPressed: _canProceed ? _ctrl.checkAvailabilityAndProceed : null,
                  ),
                  ],
                ),
              ),
            ),
            ),
          ),
        ],
      ),
    );
  }
}

enum _ActiveField { pickup, dropoff }

class _LocationInputCard extends StatelessWidget {
  const _LocationInputCard({
    required this.pickupCtrl,
    required this.dropoffCtrl,
    required this.pickupLoading,
    required this.dropoffLoading,
    required this.onPickupChanged,
    required this.onDropoffChanged,
    required this.onPickupTap,
    required this.onDropoffTap,
  });

  final TextEditingController pickupCtrl;
  final TextEditingController dropoffCtrl;
  final bool pickupLoading;
  final bool dropoffLoading;
  final ValueChanged<String> onPickupChanged;
  final ValueChanged<String> onDropoffChanged;
  final VoidCallback onPickupTap;
  final VoidCallback onDropoffTap;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: CustomerFigmaColors.border),
      ),
      child: IntrinsicHeight(
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            // Icon column: pin → dotted line → pin
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 14, horizontal: 14),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  // Pickup icon
                  pickupLoading
                      ? const SizedBox(
                          width: 18, height: 18,
                          child: CircularProgressIndicator(
                            strokeWidth: 2, color: CustomerFigmaColors.primary,
                          ),
                        )
                      : const Icon(
                          Icons.radio_button_checked,
                          color: CustomerFigmaColors.primary,
                          size: 18,
                        ),
                  // Dotted connecting line
                  Expanded(
                    child: Padding(
                      padding: const EdgeInsets.symmetric(vertical: 3),
                      child: CustomPaint(
                        size: const Size(2, double.infinity),
                        painter: _DottedLinePainter(),
                      ),
                    ),
                  ),
                  // Dropoff icon
                  dropoffLoading
                      ? SizedBox(
                          width: 18, height: 18,
                          child: CircularProgressIndicator(
                            strokeWidth: 2, color: Colors.orange[700],
                          ),
                        )
                      : Icon(
                          Icons.location_on,
                          color: Colors.orange[700],
                          size: 20,
                        ),
                ],
              ),
            ),

            // Text fields
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  // Pickup field
                  Padding(
                    padding: const EdgeInsets.fromLTRB(0, 10, 14, 0),
                    child: Text(
                      'Pick-up',
                      style: TextStyle(
                        color: Colors.grey[500],
                        fontSize: 11,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ),
                  TextField(
                    controller: pickupCtrl,
                    onChanged: onPickupChanged,
                    onTap: onPickupTap,
                    textInputAction: TextInputAction.next,
                    style: const TextStyle(color: CustomerFigmaColors.text, fontSize: 13, fontWeight: FontWeight.w600),
                    decoration: InputDecoration(
                      hintText: 'Enter pick up address',
                      hintStyle: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
                      filled: true,
                      fillColor: Colors.transparent,
                      border: InputBorder.none,
                      enabledBorder: InputBorder.none,
                      focusedBorder: InputBorder.none,
                      contentPadding: const EdgeInsets.fromLTRB(0, 4, 14, 10),
                    ),
                  ),
                  const Divider(height: 1, color: CustomerFigmaColors.border),
                  // Dropoff field
                  Padding(
                    padding: const EdgeInsets.fromLTRB(0, 10, 14, 0),
                    child: Text(
                      'Drop off (optional)',
                      style: TextStyle(
                        color: Colors.grey[500],
                        fontSize: 11,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ),
                  TextField(
                    controller: dropoffCtrl,
                    onChanged: onDropoffChanged,
                    onTap: onDropoffTap,
                    textInputAction: TextInputAction.done,
                    style: const TextStyle(color: CustomerFigmaColors.text, fontSize: 13, fontWeight: FontWeight.w600),
                    decoration: InputDecoration(
                      hintText: 'Enter destination address',
                      hintStyle: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
                      filled: true,
                      fillColor: Colors.transparent,
                      border: InputBorder.none,
                      enabledBorder: InputBorder.none,
                      focusedBorder: InputBorder.none,
                      contentPadding: const EdgeInsets.fromLTRB(0, 4, 14, 10),
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _DottedLinePainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = CustomerFigmaColors.border
      ..strokeWidth = 1.5
      ..style = PaintingStyle.stroke;
    const dashHeight = 4.0;
    const gapHeight = 3.0;
    double y = 0;
    while (y < size.height) {
      canvas.drawLine(
        Offset(size.width / 2, y),
        Offset(size.width / 2, (y + dashHeight).clamp(0, size.height)),
        paint,
      );
      y += dashHeight + gapHeight;
    }
  }

  @override
  bool shouldRepaint(_DottedLinePainter oldDelegate) => false;
}
