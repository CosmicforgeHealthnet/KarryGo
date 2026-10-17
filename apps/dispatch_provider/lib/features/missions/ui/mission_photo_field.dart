import 'dart:convert';
import 'dart:io';
import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../data/provider_operations_api.dart';

class MissionPhotoDraft {
  String? path;
  DateTime? capturedAt;
  String? _media, _scope;
  String _key = ProviderOperationsApi.newOperationKey();
  bool picking = false;
  void select(String value, DateTime time) {
    path = value;
    capturedAt = time;
    _media = null;
    _scope = null;
    _key = ProviderOperationsApi.newOperationKey();
  }

  void clear() {
    path = null;
    capturedAt = null;
    _media = null;
    _scope = null;
    _key = ProviderOperationsApi.newOperationKey();
  }

  Future<Map<String, dynamic>> attach(
    ProviderOperationsApi api,
    String uploadPath,
    List<String> parcels,
    Map<String, dynamic> evidence, {
    Map<String, String> fields = const {},
  }) async {
    if (path == null) return evidence;
    final refs = parcels.toSet().toList()..sort();
    final scope = '$uploadPath:${jsonEncode(refs)}:${jsonEncode(fields)}';
    if (_scope != scope) {
      _scope = scope;
      _media = null;
      _key = ProviderOperationsApi.newOperationKey();
    }
    _media ??= await api.upload(
      uploadPath,
      '',
      path!,
      fields: {
        ...fields,
        'operation_ref': _key,
        'parcel_refs': jsonEncode(refs),
      },
    );
    return {
      ...evidence,
      'captured_at': capturedAt!.toUtc().toIso8601String(),
      'media_refs': [_media],
    };
  }
}

class MissionPhotoField extends StatefulWidget {
  const MissionPhotoField({
    super.key,
    required this.draft,
    this.enabled = true,
    this.required = false,
    this.onChanged,
  });
  final MissionPhotoDraft draft;
  final bool enabled;
  final bool required;
  final VoidCallback? onChanged;
  @override
  State<MissionPhotoField> createState() => _MissionPhotoFieldState();
}

class _MissionPhotoFieldState extends State<MissionPhotoField> {
  String? _error;
  Future<void> _pick() async {
    final source = await showModalBottomSheet<ImageSource>(
      context: context,
      showDragHandle: true,
      useSafeArea: true,
      builder: (sheet) => Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          ListTile(
            leading: const Icon(Icons.photo_camera_outlined),
            title: const Text('Take a photo'),
            onTap: () => Navigator.pop(sheet, ImageSource.camera),
          ),
          ListTile(
            leading: const Icon(Icons.photo_library_outlined),
            title: const Text('Choose from photos'),
            onTap: () => Navigator.pop(sheet, ImageSource.gallery),
          ),
          const SizedBox(height: 16),
        ],
      ),
    );
    if (source == null || !mounted) return;
    setState(() {
      widget.draft.picking = true;
      _error = null;
    });
    widget.onChanged?.call();
    try {
      final photo = await ImagePicker().pickImage(
        source: source,
        maxWidth: 1600,
        maxHeight: 1600,
        imageQuality: 85,
      );
      if (photo == null) return;
      if (await photo.length() > 5 * 1024 * 1024)
        throw const ApiException(
          code: ApiErrorCode.validationFailed,
          message: 'Choose a photo smaller than 5 MB.',
        );
      widget.draft.select(photo.path, DateTime.now().toUtc());
    } catch (error) {
      if (mounted)
        setState(
          () => _error = error is ApiException
              ? error.message
              : 'The photo could not be opened. Check photo or camera access and try again.',
        );
    } finally {
      widget.draft.picking = false;
      if (mounted) {
        setState(() {});
        widget.onChanged?.call();
      }
    }
  }

  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.stretch,
    children: [
      if (widget.draft.path != null) ...[
        ClipRRect(
          borderRadius: BorderRadius.circular(20),
          child: Image.file(
            File(widget.draft.path!),
            height: 180,
            fit: BoxFit.cover,
          ),
        ),
        const SizedBox(height: 12),
      ],
      OutlinedButton.icon(
        onPressed: widget.enabled && !widget.draft.picking ? _pick : null,
        icon: widget.draft.picking
            ? const SizedBox(
                width: 18,
                height: 18,
                child: CircularProgressIndicator(strokeWidth: 2),
              )
            : const Icon(Icons.add_a_photo_outlined),
        label: Text(
          widget.draft.path == null
              ? (widget.required
                    ? 'Add required hub photo'
                    : 'Add handover photo (optional)')
              : 'Change handover photo',
        ),
      ),
      if (_error != null)
        Padding(
          padding: const EdgeInsets.only(top: 8),
          child: Text(
            _error!,
            style: TextStyle(color: Theme.of(context).colorScheme.error),
          ),
        ),
    ],
  );
}
