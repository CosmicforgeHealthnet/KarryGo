import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../../auth/data/customer_auth_api.dart';
import '../../../auth/models/customer_auth_models.dart';
import '../../../hauling/data/places_api.dart';
import 'saved_location_map_picker_screen.dart';
import 'saved_locations_header.dart';

const _fieldGrey = Color(0xFFF3F4F3);

/// Figma 1778 (empty) / 1782+1786 (filled): add or edit a saved location.
/// Pass [existing] to edit; otherwise creates a new location.
class SavedLocationFormScreen extends StatefulWidget {
  const SavedLocationFormScreen({
    super.key,
    required this.session,
    required this.api,
    required this.placesApi,
    this.existing,
  });

  final CustomerSession session;
  final CustomerAuthApi api;
  final PlacesApi placesApi;
  final SavedLocation? existing;

  @override
  State<SavedLocationFormScreen> createState() =>
      _SavedLocationFormScreenState();
}

class _SavedLocationFormScreenState extends State<SavedLocationFormScreen> {
  late final TextEditingController _titleCtrl;
  late final TextEditingController _addressCtrl;
  late final TextEditingController _descriptionCtrl;

  String _address = '';
  double _latitude = 0;
  double _longitude = 0;
  bool _saving = false;
  bool _deleting = false;
  String? _error;

  bool get _isEdit => widget.existing != null;

  bool get _canSave =>
      !_saving &&
      !_deleting &&
      _titleCtrl.text.trim().isNotEmpty &&
      _address.isNotEmpty &&
      !(_latitude == 0 && _longitude == 0);

  @override
  void initState() {
    super.initState();
    final existing = widget.existing;
    _titleCtrl = TextEditingController(text: existing?.title ?? '');
    _addressCtrl = TextEditingController(text: existing?.address ?? '');
    _descriptionCtrl = TextEditingController(text: existing?.description ?? '');
    _address = existing?.address ?? '';
    _latitude = existing?.latitude ?? 0;
    _longitude = existing?.longitude ?? 0;
  }

  @override
  void dispose() {
    _titleCtrl.dispose();
    _addressCtrl.dispose();
    _descriptionCtrl.dispose();
    super.dispose();
  }

  Future<void> _pickLocation() async {
    final result = await Navigator.of(context).push<SavedLocationPickResult>(
      MaterialPageRoute(
        builder: (_) => SavedLocationMapPickerScreen(
          placesApi: widget.placesApi,
          initialAddress: _address.isEmpty ? null : _address,
          initialLatLng: (_latitude == 0 && _longitude == 0)
              ? null
              : LatLng(_latitude, _longitude),
        ),
      ),
    );
    if (result == null || !mounted) return;
    setState(() {
      _address = result.address;
      _addressCtrl.text = result.address;
      _latitude = result.latitude;
      _longitude = result.longitude;
      _error = null;
    });
  }

  Future<void> _save() async {
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      final existing = widget.existing;
      final SavedLocation saved;
      if (existing == null) {
        saved = await widget.api.addSavedLocation(
          accessToken: widget.session.accessToken,
          title: _titleCtrl.text.trim(),
          address: _address,
          latitude: _latitude,
          longitude: _longitude,
          description: _descriptionCtrl.text.trim(),
        );
      } else {
        saved = await widget.api.updateSavedLocation(
          accessToken: widget.session.accessToken,
          id: existing.id,
          title: _titleCtrl.text.trim(),
          address: _address,
          latitude: _latitude,
          longitude: _longitude,
          description: _descriptionCtrl.text.trim(),
        );
      }
      if (!mounted) return;
      Navigator.of(context).pop(saved);
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _saving = false;
        _error = e.message;
      });
    }
  }

  Future<void> _delete() async {
    final existing = widget.existing;
    if (existing == null) return;

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Delete Location'),
        content: Text('Remove "${existing.title}" from your saved locations?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: const Text('Cancel'),
          ),
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            child:
                const Text('Delete', style: TextStyle(color: Color(0xFFE53935))),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;

    setState(() {
      _deleting = true;
      _error = null;
    });
    try {
      await widget.api.deleteSavedLocation(
        accessToken: widget.session.accessToken,
        id: existing.id,
      );
      if (!mounted) return;
      Navigator.of(context).pop(true);
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _deleting = false;
        _error = e.message;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            SavedLocationsHeader(
              trailing: _isEdit
                  ? IconButton(
                      onPressed: _deleting ? null : _delete,
                      icon: _deleting
                          ? const SizedBox(
                              width: 18,
                              height: 18,
                              child: CircularProgressIndicator(
                                strokeWidth: 2,
                                color: Color(0xFFE53935),
                              ),
                            )
                          : const Icon(Icons.delete_outline_rounded,
                              color: Color(0xFFE53935), size: 22),
                    )
                  : null,
            ),
            const SizedBox(height: 22),
            Expanded(
              child: ListView(
                padding: const EdgeInsets.symmetric(horizontal: 20),
                children: [
                  const _FieldLabel('Title'),
                  const SizedBox(height: 10),
                  TextField(
                    controller: _titleCtrl,
                    onChanged: (_) => setState(() {}),
                    textCapitalization: TextCapitalization.words,
                    style: const TextStyle(
                      color: CustomerFigmaColors.text,
                      fontSize: 14,
                    ),
                    decoration: _greyFieldDecoration(hintText: 'Home'),
                  ),
                  const SizedBox(height: 22),
                  const _FieldLabel('Address'),
                  const SizedBox(height: 10),
                  GestureDetector(
                    onTap: _pickLocation,
                    child: AbsorbPointer(
                      child: TextField(
                        controller: _addressCtrl,
                        readOnly: true,
                        style: const TextStyle(
                          color: CustomerFigmaColors.text,
                          fontSize: 14,
                        ),
                        decoration: _greyFieldDecoration(
                          hintText: 'Set location',
                          suffixIcon: const Icon(
                            Icons.location_on_outlined,
                            color: CustomerFigmaColors.text,
                            size: 20,
                          ),
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(height: 22),
                  const _FieldLabel('Description (optional)'),
                  const SizedBox(height: 10),
                  TextField(
                    controller: _descriptionCtrl,
                    minLines: 3,
                    maxLines: 4,
                    style: const TextStyle(
                      color: CustomerFigmaColors.text,
                      fontSize: 14,
                    ),
                    decoration:
                        _greyFieldDecoration(hintText: 'Enter description'),
                  ),
                ],
              ),
            ),
            if (_error != null)
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
                child: Text(
                  _error!,
                  textAlign: TextAlign.center,
                  style: const TextStyle(color: Colors.red, fontSize: 12),
                ),
              ),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 12),
              child: FigmaPrimaryButton(
                label: 'Save',
                isLoading: _saving,
                onPressed: _canSave ? _save : null,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

InputDecoration _greyFieldDecoration({
  required String hintText,
  Widget? suffixIcon,
}) {
  return InputDecoration(
    hintText: hintText,
    hintStyle: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
    filled: true,
    fillColor: _fieldGrey,
    suffixIcon: suffixIcon,
    border: OutlineInputBorder(
      borderRadius: BorderRadius.circular(10),
      borderSide: BorderSide.none,
    ),
    enabledBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(10),
      borderSide: BorderSide.none,
    ),
    focusedBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(10),
      borderSide: BorderSide.none,
    ),
    contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
  );
}

class _FieldLabel extends StatelessWidget {
  const _FieldLabel(this.text);

  final String text;

  @override
  Widget build(BuildContext context) {
    return Text(
      text,
      style: const TextStyle(
        color: CustomerFigmaColors.text,
        fontSize: 14,
        fontWeight: FontWeight.w800,
      ),
    );
  }
}
