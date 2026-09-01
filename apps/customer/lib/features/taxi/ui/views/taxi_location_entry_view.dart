import 'dart:async';

import 'package:flutter/material.dart';
import 'package:geolocator/geolocator.dart';
import 'package:permission_handler/permission_handler.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../../auth/models/customer_auth_models.dart';
import '../../../hauling/data/places_api.dart';
import '../../state/taxi_booking_controller.dart';
import '../widgets/taxi_flow_helpers.dart';

enum _ActiveField { pickup, dropoff }

/// Where from, where to — the first screen of the ride flow.
class TaxiLocationEntryView extends StatefulWidget {
  const TaxiLocationEntryView({
    super.key,
    required this.controller,
    required this.placesApi,
  });

  final TaxiBookingController controller;
  final PlacesApi placesApi;

  @override
  State<TaxiLocationEntryView> createState() => _TaxiLocationEntryViewState();
}

class _TaxiLocationEntryViewState extends State<TaxiLocationEntryView> {
  final _pickupCtrl = TextEditingController();
  final _dropoffCtrl = TextEditingController();
  Timer? _debounce;

  List<PlaceSuggestion> _suggestions = const [];
  bool _suggestionsLoading = false;
  bool _locatingCurrent = false;
  String? _suggestionsError;
  _ActiveField? _activeField;
  List<SavedLocation> _savedLocations = const [];

  static const _savedQuickPickLimit = 3;

  TaxiBookingController get _ctrl => widget.controller;
  TaxiBookingState get _state => _ctrl.state;

  /// The API returns saved locations oldest-first; the quick-pick list shows
  /// the most recent few.
  List<SavedLocation> get _recentSaved =>
      _savedLocations.reversed.take(_savedQuickPickLimit).toList();

  /// Both ends need real resolved coordinates: the service rejects (0,0) and
  /// matching needs real geometry to find nearby drivers.
  bool get _canProceed =>
      _hasFix(_state.pickupLat, _state.pickupLng) &&
      _hasFix(_state.dropoffLat, _state.dropoffLng);

  bool _hasFix(double lat, double lng) => !(lat == 0 && lng == 0);

  @override
  void initState() {
    super.initState();
    _pickupCtrl.text = _state.pickupAddress;
    _dropoffCtrl.text = _state.dropoffAddress;
    _loadSavedLocations();
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _pickupCtrl.dispose();
    _dropoffCtrl.dispose();
    super.dispose();
  }

  Future<void> _loadSavedLocations() async {
    final loader = _ctrl.savedLocationsLoader;
    if (loader == null) return;
    try {
      final locations = await loader();
      if (!mounted) return;
      setState(() => _savedLocations = locations);
    } catch (_) {
      // A convenience only — booking works fine without saved locations.
    }
  }

  void _onFieldChanged(String value, _ActiveField field) {
    // Typing invalidates any previously resolved coordinates for that field,
    // so a stale fix can never be submitted with a newly typed address.
    if (field == _ActiveField.pickup) {
      _ctrl.setPickup(address: value, lat: 0, lng: 0);
    } else {
      _ctrl.setDropoff(address: value, lat: 0, lng: 0);
    }
    setState(() {
      _suggestions = const [];
      _suggestionsError = null;
      _activeField = field;
    });

    _debounce?.cancel();
    if (value.trim().length < 2) return;
    _debounce = Timer(const Duration(milliseconds: 300), () async {
      if (!mounted) return;
      setState(() => _suggestionsLoading = true);
      try {
        final results = await widget.placesApi.autocomplete(value.trim());
        if (!mounted) return;
        setState(() {
          _suggestions = results;
          _suggestionsError = null;
          _suggestionsLoading = false;
        });
      } on PlacesApiException catch (e) {
        if (!mounted) return;
        setState(() {
          _suggestions = const [];
          _suggestionsError = _friendlyPlacesError(e);
          _suggestionsLoading = false;
        });
      }
    });
  }

  Future<void> _selectSuggestion(PlaceSuggestion suggestion) async {
    final field = _activeField ?? _ActiveField.pickup;
    FocusScope.of(context).unfocus();
    setState(() {
      _suggestions = const [];
      _suggestionsLoading = true;
    });

    final coords = await widget.placesApi.getLatLng(suggestion.placeId);
    if (!mounted) return;
    setState(() {
      _suggestionsLoading = false;
      _activeField = null;
    });

    if (coords == null) {
      setState(() => _suggestionsError =
          'Could not resolve that place. Try another one.');
      return;
    }
    _applyLocation(
      field,
      suggestion.description,
      coords.lat,
      coords.lng,
    );
  }

  void _applyLocation(
    _ActiveField field,
    String address,
    double lat,
    double lng,
  ) {
    if (field == _ActiveField.pickup) {
      _pickupCtrl.text = address;
      _ctrl.setPickup(address: address, lat: lat, lng: lng);
    } else {
      _dropoffCtrl.text = address;
      _ctrl.setDropoff(address: address, lat: lat, lng: lng);
    }
    setState(() {});
  }

  void _selectSaved(SavedLocation location) {
    FocusScope.of(context).unfocus();
    _applyLocation(
      _activeField ?? _ActiveField.pickup,
      location.address,
      location.latitude,
      location.longitude,
    );
    setState(() {
      _suggestions = const [];
      _activeField = null;
    });
  }

  String _friendlyPlacesError(PlacesApiException e) => switch (e.status) {
    'REQUEST_DENIED' =>
      'Location search is unavailable (key/Places API not enabled).',
    'OVER_QUERY_LIMIT' => 'Location search quota exceeded. Try again later.',
    'NETWORK_ERROR' => 'No internet connection for location search.',
    'MISSING_KEY' => 'Location search is not configured.',
    _ => 'Could not load suggestions: ${e.status}',
  };

  void _showSnack(String message) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(message), duration: const Duration(seconds: 2)),
    );
  }

  /// Resolves the device position and fills the active field (pickup by
  /// default). Mirrors the permission handling of the home-map locate button.
  Future<void> _useCurrentLocation() async {
    if (_locatingCurrent) return;
    final field = _activeField ?? _ActiveField.pickup;
    setState(() => _locatingCurrent = true);
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
        locationSettings: const LocationSettings(
          accuracy: LocationAccuracy.high,
        ),
      );
      if (!mounted) return;
      final address =
          await widget.placesApi.reverseGeocode(pos.latitude, pos.longitude) ??
              'Current location';
      if (!mounted) return;
      _applyLocation(field, address, pos.latitude, pos.longitude);
    } catch (_) {
      _showSnack('Could not get your current location.');
    } finally {
      if (mounted) setState(() => _locatingCurrent = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final busy = _state.status == TaxiFlowStatus.checkingAvailability;
    final showSaved = _recentSaved.isNotEmpty &&
        (_activeField == null || _currentFieldText.trim().length < 2);

    return taxiFlowScaffold(
      title: _state.rideMode.isParcel ? 'Send a package' : 'Book a ride',
      onBack: _ctrl.backToModeSelection,
      body: ListView(
        children: [
          Text(
            _state.rideMode.isParcel
                ? 'Where is it going?'
                : 'Where are you going?',
            style: const TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 20,
              fontWeight: FontWeight.w800,
            ),
          ),
          const SizedBox(height: 4),
          Text(
            _state.rideMode.isParcel
                ? 'Where should the package be collected and dropped off?'
                : 'Enter your pick-up point and destination.',
            style: const TextStyle(
              color: CustomerFigmaColors.muted,
              fontSize: 13,
            ),
          ),
          const SizedBox(height: 16),

          _LocationInputCard(
            pickupController: _pickupCtrl,
            dropoffController: _dropoffCtrl,
            onPickupChanged: (v) => _onFieldChanged(v, _ActiveField.pickup),
            onDropoffChanged: (v) => _onFieldChanged(v, _ActiveField.dropoff),
            onPickupFocus: () => setState(() => _activeField = _ActiveField.pickup),
            onDropoffFocus: () => setState(() => _activeField = _ActiveField.dropoff),
          ),

          const SizedBox(height: 10),
          InkWell(
            onTap: _locatingCurrent ? null : _useCurrentLocation,
            borderRadius: BorderRadius.circular(10),
            child: Padding(
              padding: const EdgeInsets.symmetric(vertical: 8),
              child: Row(
                children: [
                  if (_locatingCurrent)
                    const SizedBox(
                      width: 18,
                      height: 18,
                      child: CircularProgressIndicator(
                        strokeWidth: 2,
                        color: CustomerFigmaColors.primary,
                      ),
                    )
                  else
                    const Icon(
                      Icons.my_location_rounded,
                      size: 18,
                      color: CustomerFigmaColors.primary,
                    ),
                  const SizedBox(width: 8),
                  Text(
                    _locatingCurrent
                        ? 'Getting your location...'
                        : 'Use my current location',
                    style: const TextStyle(
                      color: CustomerFigmaColors.primary,
                      fontSize: 13,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ],
              ),
            ),
          ),

          if (_suggestionsError != null) ...[
            const SizedBox(height: 8),
            Container(
              padding: const EdgeInsets.all(10),
              decoration: BoxDecoration(
                color: const Color(0xFFFDECEC),
                borderRadius: BorderRadius.circular(8),
                border: Border.all(color: const Color(0xFFF5B5B5)),
              ),
              child: Text(
                _suggestionsError!,
                style: const TextStyle(color: Color(0xFFB3261E), fontSize: 12),
              ),
            ),
          ],

          if (_suggestionsLoading) ...[
            const SizedBox(height: 12),
            const Center(
              child: SizedBox(
                width: 20,
                height: 20,
                child: CircularProgressIndicator(strokeWidth: 2),
              ),
            ),
          ],

          if (_suggestions.isNotEmpty) ...[
            const SizedBox(height: 10),
            Container(
              decoration: BoxDecoration(
                color: CustomerFigmaColors.field,
                borderRadius: BorderRadius.circular(12),
                border: Border.all(color: CustomerFigmaColors.border),
              ),
              child: Column(
                children: [
                  for (final s in _suggestions)
                    InkWell(
                      onTap: () => _selectSuggestion(s),
                      child: Padding(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 14,
                          vertical: 12,
                        ),
                        child: Row(
                          children: [
                            const Icon(
                              Icons.place_outlined,
                              size: 16,
                              color: CustomerFigmaColors.muted,
                            ),
                            const SizedBox(width: 8),
                            Expanded(
                              child: Text(
                                s.description,
                                maxLines: 2,
                                overflow: TextOverflow.ellipsis,
                                style: const TextStyle(
                                  color: CustomerFigmaColors.text,
                                  fontSize: 13,
                                ),
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                ],
              ),
            ),
          ],

          if (showSaved) ...[
            const SizedBox(height: 16),
            taxiSectionLabel('Saved locations'),
            Container(
              decoration: BoxDecoration(
                color: CustomerFigmaColors.field,
                borderRadius: BorderRadius.circular(12),
                border: Border.all(color: CustomerFigmaColors.border),
              ),
              child: Column(
                children: [
                  for (final loc in _recentSaved)
                    InkWell(
                      onTap: () => _selectSaved(loc),
                      child: Padding(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 14,
                          vertical: 10,
                        ),
                        child: Row(
                          children: [
                            const Icon(
                              Icons.bookmark_rounded,
                              size: 16,
                              color: CustomerFigmaColors.primary,
                            ),
                            const SizedBox(width: 8),
                            Expanded(
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text(
                                    loc.title,
                                    maxLines: 1,
                                    overflow: TextOverflow.ellipsis,
                                    style: const TextStyle(
                                      color: CustomerFigmaColors.text,
                                      fontSize: 13,
                                      fontWeight: FontWeight.w700,
                                    ),
                                  ),
                                  const SizedBox(height: 2),
                                  Text(
                                    loc.address,
                                    maxLines: 1,
                                    overflow: TextOverflow.ellipsis,
                                    style: const TextStyle(
                                      color: CustomerFigmaColors.muted,
                                      fontSize: 12,
                                    ),
                                  ),
                                ],
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                ],
              ),
            ),
          ],

          if (_state.error != null) ...[
            const SizedBox(height: 14),
            Text(
              _state.error!,
              style: const TextStyle(color: Color(0xFFB3261E), fontSize: 12),
            ),
          ],
          const SizedBox(height: 10),
        ],
      ),
      bottom: FigmaPrimaryButton(
        label: 'See ride options',
        isLoading: busy,
        onPressed: (!_canProceed || busy)
            ? null
            : _ctrl.checkAvailabilityAndQuote,
      ),
    );
  }

  String get _currentFieldText => switch (_activeField) {
    _ActiveField.dropoff => _dropoffCtrl.text,
    _ => _pickupCtrl.text,
  };
}

/// The grouped pickup/destination card with the dotted connector between the
/// two indicators.
class _LocationInputCard extends StatelessWidget {
  const _LocationInputCard({
    required this.pickupController,
    required this.dropoffController,
    required this.onPickupChanged,
    required this.onDropoffChanged,
    required this.onPickupFocus,
    required this.onDropoffFocus,
  });

  final TextEditingController pickupController;
  final TextEditingController dropoffController;
  final ValueChanged<String> onPickupChanged;
  final ValueChanged<String> onDropoffChanged;
  final VoidCallback onPickupFocus;
  final VoidCallback onDropoffFocus;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 6),
      decoration: BoxDecoration(
        color: CustomerFigmaColors.field,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: CustomerFigmaColors.border),
      ),
      child: Row(
        children: [
          Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(
                Icons.radio_button_checked,
                size: 16,
                color: CustomerFigmaColors.success,
              ),
              SizedBox(
                height: 34,
                child: CustomPaint(
                  size: const Size(2, 34),
                  painter: _DottedLinePainter(),
                ),
              ),
              const Icon(
                Icons.location_on,
                size: 16,
                color: Color(0xFFE9A13B),
              ),
            ],
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              children: [
                _BareField(
                  controller: pickupController,
                  hint: 'Pick-up',
                  onChanged: onPickupChanged,
                  onTap: onPickupFocus,
                ),
                const Divider(height: 1, color: CustomerFigmaColors.border),
                _BareField(
                  controller: dropoffController,
                  hint: 'Where to?',
                  onChanged: onDropoffChanged,
                  onTap: onDropoffFocus,
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _BareField extends StatelessWidget {
  const _BareField({
    required this.controller,
    required this.hint,
    required this.onChanged,
    required this.onTap,
  });

  final TextEditingController controller;
  final String hint;
  final ValueChanged<String> onChanged;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      onChanged: onChanged,
      onTap: onTap,
      style: const TextStyle(
        color: CustomerFigmaColors.text,
        fontSize: 14,
        fontWeight: FontWeight.w600,
      ),
      decoration: InputDecoration(
        hintText: hint,
        hintStyle: const TextStyle(
          color: CustomerFigmaColors.muted,
          fontSize: 14,
          fontWeight: FontWeight.w500,
        ),
        border: InputBorder.none,
        focusedBorder: InputBorder.none,
        enabledBorder: InputBorder.none,
        contentPadding: const EdgeInsets.symmetric(vertical: 14),
        isDense: true,
      ),
    );
  }
}

class _DottedLinePainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = CustomerFigmaColors.primarySoft
      ..strokeWidth = 2
      ..strokeCap = StrokeCap.round;
    const dash = 3.0;
    const gap = 3.0;
    var y = 0.0;
    while (y < size.height) {
      canvas.drawLine(
        Offset(size.width / 2, y),
        Offset(size.width / 2, (y + dash).clamp(0, size.height)),
        paint,
      );
      y += dash + gap;
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
