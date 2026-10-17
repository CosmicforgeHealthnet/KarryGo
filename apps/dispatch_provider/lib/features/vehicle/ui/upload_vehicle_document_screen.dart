import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../../shared/widgets/async_content.dart';
import '../../home/ui/widgets/home_feedback.dart';
import 'package:flutter/material.dart';
import 'package:file_picker/file_picker.dart';
import 'package:path/path.dart' as p;

import '../state/vehicle_controller.dart';

class UploadVehicleDocumentScreen extends StatefulWidget {
  const UploadVehicleDocumentScreen({
    super.key,
    required this.vehicleController,
    required this.vehicleId,
    this.initialDocumentType,
  });

  final VehicleController vehicleController;
  final String vehicleId;
  final String? initialDocumentType;

  @override
  State<UploadVehicleDocumentScreen> createState() =>
      _UploadVehicleDocumentScreenState();
}

class _UploadVehicleDocumentScreenState
    extends State<UploadVehicleDocumentScreen> {
  String? _error;
  String? _docType;
  String? _filePath;
  DateTime? _expiryDate;
  bool _isSubmitting = false;

  static const _docTypes = [
    ('registration', 'Registration Certificate'),
    ('insurance', 'Insurance Certificate'),
  ];

  @override
  void initState() {
    super.initState();
    final initial = widget.initialDocumentType;
    if (initial != null && _docTypes.any((t) => t.$1 == initial)) {
      _docType = initial;
    }
  }

  bool get _requiresExpiry => _docType == 'insurance';

  bool get _canSubmit =>
      _docType != null &&
      _filePath != null &&
      (!_requiresExpiry || _expiryDate != null) &&
      !_isSubmitting;

  Future<void> _pick() async {
    try {
      final result = await FilePicker.platform.pickFiles(
        type: FileType.custom,
        allowedExtensions: ['jpg', 'jpeg', 'png', 'pdf'],
      );
      if (!mounted || result == null) return;
      final file = result.files.single;
      if (file.path == null) {
        setState(
          () => _error = 'This file could not be opened. Choose it again.',
        );
        return;
      }
      if (file.size > 5 * 1024 * 1024) {
        setState(() => _error = 'Choose a document smaller than 5 MB.');
        return;
      }
      setState(() {
        _filePath = file.path;
        _error = null;
      });
    } catch (_) {
      if (mounted)
        setState(
          () => _error = 'Could not open the file picker. Please try again.',
        );
    }
  }

  Future<void> _pickExpiry() async {
    final now = DateTime.now();
    final picked = await showDatePicker(
      context: context,
      initialDate: _expiryDate ?? now.add(const Duration(days: 365)),
      firstDate: now.add(const Duration(days: 1)),
      lastDate: now.add(const Duration(days: 365 * 10)),
      helpText: 'Select expiry date',
    );
    if (mounted && picked != null) setState(() => _expiryDate = picked);
  }

  String _isoDate(DateTime d) {
    final y = d.year;
    final m = d.month.toString().padLeft(2, '0');
    final day = d.day.toString().padLeft(2, '0');
    return '$y-$m-$day';
  }

  String _displayDate(DateTime d) {
    const months = [
      'Jan',
      'Feb',
      'Mar',
      'Apr',
      'May',
      'Jun',
      'Jul',
      'Aug',
      'Sep',
      'Oct',
      'Nov',
      'Dec',
    ];
    return '${d.day} ${months[d.month - 1]} ${d.year}';
  }

  Future<void> _submit() async {
    if (!_canSubmit) return;
    setState(() => _isSubmitting = true);

    final result = await widget.vehicleController.uploadVehicleDocument(
      vehicleId: widget.vehicleId,
      documentType: _docType!,
      documentFilePath: _filePath!,
      expiryDate: _requiresExpiry ? _isoDate(_expiryDate!) : null,
    );

    if (!mounted) return;
    setState(() => _isSubmitting = false);
    result.when(
      success: (_) async {
        await showHomeFeedback(
          context,
          title: 'Document uploaded',
          message:
              'Your document has been received. Its review status is shown in Documents.',
        );
        if (mounted) Navigator.of(context).pop(true);
      },
      failure: (error) => setState(() => _error = error.message),
    );
  }

  @override
  Widget build(BuildContext context) => AccountPage(
    title: 'Vehicle document',
    symbol: JourneySymbol.review,
    description: 'Upload a clear copy with all details visible.',
    bottom: FilledButton(
      onPressed: _canSubmit ? _submit : null,
      child: _isSubmitting
          ? const SizedBox.square(
              dimension: 20,
              child: CircularProgressIndicator(strokeWidth: 2),
            )
          : const Text('Upload document'),
    ),
    children: [
      DropdownButtonFormField<String>(
        initialValue: _docType,
        isExpanded: true,
        decoration: const InputDecoration(labelText: 'Document type'),
        items: _docTypes
            .map((t) => DropdownMenuItem(value: t.$1, child: Text(t.$2)))
            .toList(),
        onChanged: _isSubmitting
            ? null
            : (v) => setState(() {
                _docType = v;
                if (!_requiresExpiry) _expiryDate = null;
              }),
      ),
      const SizedBox(height: 22),
      DeliveryPanel(
        padding: EdgeInsets.zero,
        child: AccountMenuTile(
          title: _filePath == null ? 'Choose document' : p.basename(_filePath!),
          subtitle: _filePath == null
              ? 'JPG, PNG or PDF / up to 5 MB'
              : 'Tap to choose a different file',
          symbol: JourneySymbol.review,
          onTap: _isSubmitting ? null : _pick,
        ),
      ),
      if (_requiresExpiry) ...[
        const SizedBox(height: 22),
        DeliveryPanel(
          padding: EdgeInsets.zero,
          child: AccountMenuTile(
            title: 'Insurance expiry date',
            subtitle: _expiryDate == null
                ? 'Choose a future date'
                : _displayDate(_expiryDate!),
            symbol: JourneySymbol.review,
            onTap: _isSubmitting ? null : _pickExpiry,
          ),
        ),
      ],
      if (_error != null) ...[
        const SizedBox(height: 18),
        ActionNotice(message: _error!, isError: true),
      ],
    ],
  );
}
