import 'package:flutter/material.dart';
import 'package:latlong2/latlong.dart';
import 'branch_location_picker.dart';

/// A form field that never asks people to type coordinates. Tapping it opens
/// the map pin picker (with "use my location"); the chosen point is written
/// into [latitude]/[longitude] so callers can keep sending numbers to the API.
class LocationPickField extends StatelessWidget {
  const LocationPickField({
    super.key,
    required this.latitude,
    required this.longitude,
    this.label = 'Branch location',
    this.enabled = true,
    this.required = true,
    this.onChanged,
  });
  final TextEditingController latitude, longitude;
  final String label;
  final bool enabled, required;
  final VoidCallback? onChanged;

  LatLng? get _value {
    final lat = double.tryParse(latitude.text.trim());
    final lng = double.tryParse(longitude.text.trim());
    if (lat == null || lng == null) return null;
    if (lat.abs() > 90 || lng.abs() > 180) return null;
    return LatLng(lat, lng);
  }

  @override
  Widget build(BuildContext context) => FormField<LatLng>(
    initialValue: _value,
    validator: (_) => required && _value == null
        ? 'Choose the location on the map or use your current location.'
        : null,
    builder: (field) => Padding(
      padding: const EdgeInsets.only(bottom: 18),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          ListTile(
            contentPadding: EdgeInsets.zero,
            leading: Icon(
              field.value == null
                  ? Icons.add_location_alt_outlined
                  : Icons.location_on,
            ),
            title: Text(
              field.value == null ? 'Set $label'.trim() : '$label set',
            ),
            subtitle: Text(
              field.value == null
                  ? 'Choose on the map or use your current location'
                  : 'Tap to move the pin',
            ),
            trailing: const Icon(Icons.chevron_right_rounded),
            onTap: !enabled
                ? null
                : () async {
                    final selected = await Navigator.of(context).push<LatLng>(
                      MaterialPageRoute(
                        builder: (_) =>
                            BranchLocationPicker(initial: field.value),
                      ),
                    );
                    if (selected == null) return;
                    latitude.text = selected.latitude.toStringAsFixed(6);
                    longitude.text = selected.longitude.toStringAsFixed(6);
                    field.didChange(selected);
                    onChanged?.call();
                  },
          ),
          if (field.hasError)
            Text(
              field.errorText!,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
        ],
      ),
    ),
  );
}
