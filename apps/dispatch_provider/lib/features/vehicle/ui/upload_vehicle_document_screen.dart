import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';
import 'package:path/path.dart' as p;

import '../../../core/theme/theme_extensions.dart';
import '../state/vehicle_controller.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Upload / replace a bike document.
//
// Backend accepts exactly two document types:
//   registration  → Registration Certificate (no expiry required)
//   insurance     → Insurance Certificate (expiry_date REQUIRED, future date)
//
// The controller and API already forward expiry_date when provided; this
// screen is the only place that needs to collect it.
// ═══════════════════════════════════════════════════════════════════════════

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
  final _picker = ImagePicker();
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
    final x = await _picker.pickImage(
      source: ImageSource.gallery,
      imageQuality: 85,
      maxWidth: 1920,
      maxHeight: 1920,
    );
    if (x == null || !mounted) return;
    final ext = p.extension(x.path).replaceFirst('.', '').toLowerCase();
    if (!const {'jpg', 'jpeg', 'png', 'pdf'}.contains(ext)) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Unsupported file type: .$ext')),
      );
      return;
    }
    setState(() => _filePath = x.path);
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
    if (picked != null) setState(() => _expiryDate = picked);
  }

  /// Backend expects YYYY-MM-DD.
  String _isoDate(DateTime d) {
    final y = d.year;
    final m = d.month.toString().padLeft(2, '0');
    final day = d.day.toString().padLeft(2, '0');
    return '$y-$m-$day';
  }

  String _displayDate(DateTime d) {
    const months = [
      'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
      'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
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
      success: (_) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Document uploaded')),
        );
        Navigator.of(context).pop(true);
      },
      failure: (error) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(error.message)),
        );
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
              child: Row(
                children: [
                  GestureDetector(
                    onTap: () => Navigator.of(context).pop(),
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Icon(
                        Icons.arrow_back_rounded,
                        size: 22,
                        color: c.text,
                      ),
                    ),
                  ),
                ],
              ),
            ),

            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                children: [
                  Text(
                    'Upload / Replace document',
                    style: TextStyle(
                      fontSize: 26,
                      fontWeight: FontWeight.w800,
                      color: c.text,
                      letterSpacing: -0.4,
                    ),
                  ),
                  const SizedBox(height: 6),
                  Text(
                    'Upload a clear photo or PDF of your document.',
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w500,
                      color: c.textMuted,
                      height: 1.45,
                    ),
                  ),
                  const SizedBox(height: 28),

                  // ── Upload zone ──
                  GestureDetector(
                    onTap: _pick,
                    behavior: HitTestBehavior.opaque,
                    child: CustomPaint(
                      painter: _DashPainter(color: c.border),
                      child: Container(
                        width: double.infinity,
                        padding: const EdgeInsets.symmetric(
                          vertical: 44,
                          horizontal: 16,
                        ),
                        decoration: BoxDecoration(
                          color: c.surface,
                          borderRadius: BorderRadius.circular(16),
                        ),
                        child: Column(
                          children: [
                            Container(
                              width: 64,
                              height: 64,
                              alignment: Alignment.center,
                              decoration: BoxDecoration(
                                color: c.teal10,
                                shape: BoxShape.circle,
                              ),
                              child: Icon(
                                _filePath == null
                                    ? Icons.upload_rounded
                                    : Icons.check_rounded,
                                size: 30,
                                color: c.brandTeal600,
                              ),
                            ),
                            const SizedBox(height: 14),
                            Text(
                              _filePath == null
                                  ? 'Upload document'
                                  : p.basename(_filePath!),
                              textAlign: TextAlign.center,
                              style: TextStyle(
                                fontSize: 15,
                                fontWeight: FontWeight.w700,
                                color: c.text,
                              ),
                            ),
                            const SizedBox(height: 4),
                            Text(
                              'JPG, PNG or PDF · Max 5 MB',
                              style: TextStyle(
                                fontSize: 12.5,
                                color: c.textMuted,
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(height: 24),

                  // ── Document type ──
                  Text(
                    'Document type',
                    style: TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w700,
                      color: c.text,
                    ),
                  ),
                  const SizedBox(height: 10),
                  Container(
                    padding: const EdgeInsets.symmetric(horizontal: 16),
                    decoration: BoxDecoration(
                      color: c.inputFill,
                      borderRadius: BorderRadius.circular(12),
                    ),
                    child: DropdownButtonHideUnderline(
                      child: DropdownButton<String>(
                        value: _docType,
                        isExpanded: true,
                        hint: Text(
                          'Select document type',
                          style: TextStyle(
                            color: c.textDisabled,
                            fontSize: 15,
                            fontWeight: FontWeight.w500,
                          ),
                        ),
                        icon: Icon(
                          Icons.keyboard_arrow_down_rounded,
                          size: 22,
                          color: c.textMuted,
                        ),
                        dropdownColor: c.surface,
                        borderRadius: BorderRadius.circular(12),
                        padding: const EdgeInsets.symmetric(vertical: 6),
                        items: _docTypes
                            .map(
                              (t) => DropdownMenuItem(
                                value: t.$1,
                                child: Text(
                                  t.$2,
                                  style: TextStyle(
                                    fontSize: 15,
                                    fontWeight: FontWeight.w500,
                                    color: c.text,
                                  ),
                                ),
                              ),
                            )
                            .toList(),
                        onChanged: (v) => setState(() {
                          _docType = v;
                          if (v != 'insurance') _expiryDate = null;
                        }),
                      ),
                    ),
                  ),

                  // ── Expiry date (insurance only) ──
                  if (_requiresExpiry) ...[
                    const SizedBox(height: 20),
                    Text(
                      'Expiry date',
                      style: TextStyle(
                        fontSize: 14,
                        fontWeight: FontWeight.w700,
                        color: c.text,
                      ),
                    ),
                    const SizedBox(height: 10),
                    GestureDetector(
                      onTap: _pickExpiry,
                      behavior: HitTestBehavior.opaque,
                      child: Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 16,
                          vertical: 18,
                        ),
                        decoration: BoxDecoration(
                          color: c.inputFill,
                          borderRadius: BorderRadius.circular(12),
                        ),
                        child: Row(
                          children: [
                            Expanded(
                              child: Text(
                                _expiryDate == null
                                    ? 'Select expiry date'
                                    : _displayDate(_expiryDate!),
                                style: TextStyle(
                                  fontSize: 15,
                                  fontWeight: FontWeight.w500,
                                  color: _expiryDate == null
                                      ? c.textDisabled
                                      : c.text,
                                ),
                              ),
                            ),
                            Icon(
                              Icons.calendar_today_outlined,
                              size: 20,
                              color: c.textMuted,
                            ),
                          ],
                        ),
                      ),
                    ),
                  ],
                ],
              ),
            ),

            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _canSubmit ? _submit : null,
                  style: FilledButton.styleFrom(
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: _isSubmitting
                      ? SizedBox(
                          width: 20,
                          height: 20,
                          child: CircularProgressIndicator(
                            strokeWidth: 2,
                            color: c.brandWhite,
                          ),
                        )
                      : const Text('Submit document'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _DashPainter extends CustomPainter {
  const _DashPainter({required this.color});
  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 1.4
      ..style = PaintingStyle.stroke;

    final path = Path()
      ..addRRect(
        RRect.fromRectAndRadius(
          Rect.fromLTWH(0, 0, size.width, size.height),
          const Radius.circular(16),
        ),
      );

    for (final metric in path.computeMetrics()) {
      double d = 0;
      while (d < metric.length) {
        canvas.drawPath(metric.extractPath(d, d + 6), paint);
        d += 10;
      }
    }
  }

  @override
  bool shouldRepaint(_DashPainter old) => old.color != color;
}