import 'dart:convert';
import 'dart:io';
import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/async_content.dart';
import '../data/provider_operations_api.dart';

const commerceReasonLabels = <String, String>{
  'VENDOR_NOT_READY': 'Vendor is not ready',
  'PACKAGE_MISMATCH': 'Packages do not match',
  'PROHIBITED_ITEM': 'Prohibited item',
  'RESTRICTED_ITEM': 'Restricted item',
  'DAMAGED_PACKAGE': 'Package damaged',
  'RECEIVER_UNAVAILABLE': 'Recipient unavailable',
  'RECEIVER_REFUSED': 'Recipient refused delivery',
  'WRONG_ADDRESS': 'Address is incorrect',
  'ADDRESS_INACCESSIBLE': 'Cannot access the address',
  'VEHICLE_BREAKDOWN': 'Vehicle breakdown',
  'PROVIDER_ILLNESS': 'Rider illness',
  'WEATHER_ROAD': 'Weather or road conditions',
  'SECURITY_RISK': 'Security concern',
  'SAFETY_INCIDENT': 'Safety incident',
};
const commerceRecoveryLabels = <String, String>{
  'WAIT': 'Wait at this location',
  'RETRY_PICKUP': 'Retry pickup',
  'RETRY_DELIVERY': 'Retry delivery',
  'RESCHEDULE': 'Reschedule',
  'REPLAN': 'Replan delivery',
  'RETURN_TO_SENDER': 'Return to sender',
  'RETURN_TO_VENDOR': 'Return to vendor',
  'PROVIDER_REPLACEMENT': 'Request a replacement rider',
  'SUPPORT': 'Support review',
};

class CommerceExceptionScreen extends StatefulWidget {
  const CommerceExceptionScreen({
    super.key,
    required this.api,
    required this.mission,
    this.evidenceOnly = false,
  });
  final ProviderOperationsApi api;
  final Map<String, dynamic> mission;
  final bool evidenceOnly;
  @override
  State<CommerceExceptionScreen> createState() =>
      _CommerceExceptionScreenState();
}

class _CommerceExceptionScreenState extends State<CommerceExceptionScreen> {
  String? _reason, _recovery, _photo, _media, _error, _phase;
  String _uploadKey = ProviderOperationsApi.newOperationKey();
  DateTime? _capturedAt;
  bool _saved = false;
  final _selected = <String>{};
  final _notes = TextEditingController();
  Map<String, dynamic>? _pending;
  List<Map<String, dynamic>> get _options =>
      (widget.mission['exception_options'] as List? ?? [])
          .whereType<Map>()
          .map((v) => Map<String, dynamic>.from(v))
          .toList();
  Map<String, dynamic>? get _option =>
      _options.where((v) => v['reason_code'] == _reason).firstOrNull;
  bool get _photoRequired => _option?['photo_required'] == true;
  List<String> get _actions => (_option?['recovery_actions'] as List? ?? [])
      .map((v) => v.toString())
      .toList();
  bool get _valid =>
      _reason != null &&
      (widget.evidenceOnly || _recovery != null) &&
      _selected.isNotEmpty &&
      (!_photoRequired || _photo != null);
  @override
  void dispose() {
    _notes.dispose();
    super.dispose();
  }

  void _changed() {
    _pending = null;
    _media = null;
    _uploadKey = ProviderOperationsApi.newOperationKey();
    if (_photo == null) _capturedAt = null;
  }

  Future<void> _choosePhoto() async {
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
    try {
      final file = await ImagePicker().pickImage(
        source: source,
        maxWidth: 1600,
        maxHeight: 1600,
        imageQuality: 85,
      );
      if (file == null || !mounted) return;
      if (await file.length() > 5 * 1024 * 1024) {
        if (mounted)
          setState(() => _error = 'Choose a photo smaller than 5 MB.');
        return;
      }
      if (mounted)
        setState(() {
          _changed();
          _photo = file.path;
          _capturedAt = DateTime.now().toUtc();
          _error = null;
        });
    } catch (_) {
      if (mounted)
        setState(
          () => _error =
              'The photo could not be opened. Check photo or camera access and try again.',
        );
    }
  }

  Future<void> _submit() async {
    if (!_valid || _phase != null) return;
    setState(() {
      _phase = _photo == null ? 'Saving report' : 'Uploading photo';
      _error = null;
    });
    try {
      final path =
          '/api/v1/provider/commerce-missions/${Uri.encodeComponent(widget.mission['mission_ref'] as String)}';
      _capturedAt ??= DateTime.now().toUtc();
      if (_photo != null && _media == null)
        _media = await widget.api.upload(
          '$path/evidence-content',
          '',
          _photo!,
          fields: {
            'operation_ref': _uploadKey,
            'parcel_refs': jsonEncode(_selected.toList()..sort()),
          },
        );
      if (!mounted) return;
      setState(
        () =>
            _phase = widget.evidenceOnly ? 'Saving evidence' : 'Saving report',
      );
      final evidence = <String, dynamic>{
        'evidence_type': _reason == 'DAMAGED_PACKAGE'
            ? 'DAMAGE'
            : _reason == 'PACKAGE_MISMATCH'
            ? 'DISCREPANCY'
            : 'FAILED_ATTEMPT',
        'parcel_refs': _selected.toList()..sort(),
        'captured_at': _capturedAt!.toIso8601String(),
        'description': _notes.text.trim(),
        'reason_code': _reason,
        if (_media != null) 'media_refs': [_media],
      };
      _pending ??= widget.evidenceOnly
          ? evidence
          : {
              'expected_version': widget.mission['version'],
              'reason_code': _reason,
              'recovery_action': _recovery,
              'parcel_refs': evidence['parcel_refs'],
              'evidence': evidence,
            };
      await widget.api.command(
        '$path/${widget.evidenceOnly ? 'evidence' : 'exceptions'}',
        _pending!,
        operationRef: true,
      );
      if (mounted) setState(() => _saved = true);
    } catch (error) {
      if (mounted)
        setState(
          () => _error = error is ApiException
              ? error.message
              : 'The result could not be confirmed. Retry without changing the report to reuse the same request.',
        );
    } finally {
      if (mounted) setState(() => _phase = null);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final parcels = (widget.mission['parcels'] as List? ?? [])
        .whereType<Map>()
        .toList();
    return PopScope(
      canPop: _phase == null,
      child: Scaffold(
        appBar: AppBar(
          title: Text(
            widget.evidenceOnly ? 'Add delivery evidence' : 'Report a problem',
          ),
        ),
        body: _saved
            ? ConfirmedResult(
                title: widget.evidenceOnly
                    ? 'Evidence saved'
                    : 'Report recorded',
                message: widget.evidenceOnly
                    ? 'Your evidence is attached to this delivery.'
                    : 'Your report and recovery choice have been saved. Check the delivery for its latest status.',
                onDone: () => Navigator.pop(context, true),
              )
            : ListView(
                padding: const EdgeInsets.all(20),
                children: [
                  Text(
                    widget.evidenceOnly
                        ? 'Keep the delivery record clear'
                        : 'What happened?',
                    style: Theme.of(context).textTheme.headlineSmall,
                  ),
                  const SizedBox(height: 20),
                  if (_error != null) ...[
                    ActionNotice(message: _error!, isError: true),
                    const SizedBox(height: 16),
                  ],
                  DropdownButtonFormField<String>(
                    isExpanded: true,
                    initialValue: _reason,
                    decoration: const InputDecoration(labelText: 'Problem'),
                    items: [
                      for (final option in _options)
                        DropdownMenuItem(
                          value: option['reason_code'] as String,
                          child: Text(
                            commerceReasonLabels[option['reason_code']] ??
                                option['reason_code'].toString(),
                          ),
                        ),
                    ],
                    onChanged: _phase != null
                        ? null
                        : (value) => setState(() {
                            _reason = value;
                            _recovery = null;
                            _changed();
                          }),
                  ),
                  if (!widget.evidenceOnly) ...[
                    const SizedBox(height: 16),
                    DropdownButtonFormField<String>(
                      key: ValueKey(_reason),
                      isExpanded: true,
                      initialValue: _recovery,
                      decoration: const InputDecoration(
                        labelText: 'Recovery action',
                      ),
                      items: [
                        for (final action in _actions)
                          DropdownMenuItem(
                            value: action,
                            child: Text(
                              commerceRecoveryLabels[action] ?? action,
                            ),
                          ),
                      ],
                      onChanged: _phase != null
                          ? null
                          : (value) => setState(() {
                              _recovery = value;
                              _pending = null;
                            }),
                    ),
                  ],
                  const SizedBox(height: 24),
                  Text(
                    'Affected parcels',
                    style: Theme.of(context).textTheme.titleMedium,
                  ),
                  const SizedBox(height: 8),
                  for (final parcel in parcels)
                    Card(
                      child: CheckboxListTile(
                        value: _selected.contains(parcel['parcel_ref']),
                        title: Text(parcel['parcel_ref'].toString()),
                        subtitle: Text('${parcel['package_count']} packages'),
                        onChanged: _phase != null
                            ? null
                            : (value) => setState(() {
                                value == true
                                    ? _selected.add(
                                        parcel['parcel_ref'] as String,
                                      )
                                    : _selected.remove(parcel['parcel_ref']);
                                _changed();
                              }),
                      ),
                    ),
                  const SizedBox(height: 20),
                  TextField(
                    controller: _notes,
                    enabled: _phase == null,
                    onChanged: (_) => _pending = null,
                    maxLength: 2000,
                    minLines: 2,
                    maxLines: 4,
                    decoration: const InputDecoration(
                      labelText: 'Details (optional)',
                    ),
                  ),
                  const SizedBox(height: 12),
                  if (_photo != null) ...[
                    ClipRRect(
                      borderRadius: BorderRadius.circular(20),
                      child: Image.file(
                        File(_photo!),
                        height: 200,
                        width: double.infinity,
                        fit: BoxFit.cover,
                      ),
                    ),
                    const SizedBox(height: 12),
                  ],
                  OutlinedButton.icon(
                    onPressed: _phase == null ? _choosePhoto : null,
                    icon: Icon(
                      _photo == null
                          ? Icons.add_a_photo_outlined
                          : Icons.photo_camera_outlined,
                      color: c.brandTeal600,
                    ),
                    label: Text(
                      _photo != null
                          ? 'Change photo'
                          : _photoRequired
                          ? 'Add required photo'
                          : 'Add photo (optional)',
                    ),
                  ),
                  if (_photoRequired && _photo == null)
                    Padding(
                      padding: const EdgeInsets.only(top: 8),
                      child: Text(
                        'A clear photo is required for damage or a package mismatch.',
                        style: TextStyle(color: c.textMuted),
                      ),
                    ),
                  const SizedBox(height: 24),
                  FilledButton.icon(
                    onPressed: _valid && _phase == null ? _submit : null,
                    icon: _phase == null
                        ? const Icon(Icons.check_rounded)
                        : const SizedBox(
                            width: 18,
                            height: 18,
                            child: CircularProgressIndicator(strokeWidth: 2),
                          ),
                    label: Text(
                      _phase ??
                          (widget.evidenceOnly
                              ? 'Save evidence'
                              : 'Submit report'),
                    ),
                  ),
                ],
              ),
      ),
    );
  }
}
