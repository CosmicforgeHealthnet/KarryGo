import 'dart:async';

import 'package:flutter/material.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
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
  // surfaced Places API failure (REQUEST_DENIED, billing, network, etc.)
  String? _suggestionsError;
  // tracks which field is active to show its suggestions
  _ActiveField? _activeField;

  HaulingBookingController get _ctrl => widget.controller;
  HaulingBookingState get _state => _ctrl.state;

  LatLng? get _pickupLatLng =>
      _state.pickupAddress.isNotEmpty ? LatLng(_state.pickupLat, _state.pickupLng) : null;
  LatLng? get _dropoffLatLng =>
      _state.dropoffAddress.isNotEmpty ? LatLng(_state.dropoffLat, _state.dropoffLng) : null;

  bool get _canProceed =>
      _state.pickupAddress.isNotEmpty && _state.dropoffAddress.isNotEmpty;

  @override
  void initState() {
    super.initState();
    _pickupCtrl.text = _state.pickupAddress;
    _dropoffCtrl.text = _state.dropoffAddress;
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

    return Scaffold(
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
                onTap: () => widget.controller.backToPackageInfo(),
                child: const Padding(
                  padding: EdgeInsets.all(8),
                  child: Icon(Icons.arrow_back, color: CustomerFigmaColors.text, size: 20),
                ),
              ),
            ),
          ),

          // Bottom panel
          Align(
            alignment: Alignment.bottomCenter,
            child: Container(
              decoration: const BoxDecoration(
                color: Colors.white,
                borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
              ),
              padding: const EdgeInsets.fromLTRB(20, 12, 20, 20),
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

                  // Discount banner
                  Container(
                    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
                    decoration: BoxDecoration(
                      color: const Color(0xFFE8F5EE),
                      borderRadius: BorderRadius.circular(8),
                    ),
                    child: const Row(
                      children: [
                        Icon(Icons.local_offer_outlined, color: CustomerFigmaColors.primary, size: 16),
                        SizedBox(width: 8),
                        Text(
                          '10% off your first truck booking!',
                          style: TextStyle(
                            color: CustomerFigmaColors.darkGreen,
                            fontSize: 12,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                        Spacer(),
                        Icon(Icons.chevron_right, color: CustomerFigmaColors.primary, size: 16),
                      ],
                    ),
                  ),
                  const SizedBox(height: 16),

                  const Text(
                    'Book a Truck',
                    style: TextStyle(
                      color: CustomerFigmaColors.text,
                      fontWeight: FontWeight.w800,
                      fontSize: 18,
                    ),
                  ),
                  const SizedBox(height: 2),
                  const Text(
                    'Tell us your pickup and destination',
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
                  SizedBox(height: MediaQuery.of(context).padding.bottom),
                ],
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
                  TextField(
                    controller: pickupCtrl,
                    onChanged: onPickupChanged,
                    onTap: onPickupTap,
                    textInputAction: TextInputAction.next,
                    decoration: InputDecoration(
                      hintText: 'Choose pick up point',
                      hintStyle: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 14),
                      filled: true,
                      fillColor: Colors.transparent,
                      border: InputBorder.none,
                      enabledBorder: InputBorder.none,
                      focusedBorder: InputBorder.none,
                      contentPadding: const EdgeInsets.fromLTRB(0, 14, 14, 14),
                    ),
                  ),
                  const Divider(height: 1, color: CustomerFigmaColors.border),
                  // Dropoff field
                  TextField(
                    controller: dropoffCtrl,
                    onChanged: onDropoffChanged,
                    onTap: onDropoffTap,
                    textInputAction: TextInputAction.done,
                    decoration: InputDecoration(
                      hintText: 'Choose your destination',
                      hintStyle: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 14),
                      filled: true,
                      fillColor: Colors.transparent,
                      border: InputBorder.none,
                      enabledBorder: InputBorder.none,
                      focusedBorder: InputBorder.none,
                      contentPadding: const EdgeInsets.fromLTRB(0, 14, 14, 14),
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
