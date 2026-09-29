import 'dart:io';

import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';
import 'package:path/path.dart' as p;

import '../../../core/theme/theme_extensions.dart';
import '../../verification/state/verification_controller.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 09 — Upload your licence (initial)
// Figma 13 — Resubmit your licence (rejection path)
// `resubmission: true` swaps copy + button label. Layout is shared.
// ═══════════════════════════════════════════════════════════════════════════

class LicenceUploadScreen extends StatefulWidget {
  const LicenceUploadScreen({
    super.key,
    required this.verificationController,
    this.licenceNumber,
    this.licenceType,
    this.expiry,
    this.issuingState,
    this.resubmission = false,
  });

  final VerificationController verificationController;
  final String? licenceNumber;
  final String? licenceType;
  final DateTime? expiry;
  final String? issuingState;
  final bool resubmission;

  @override
  State<LicenceUploadScreen> createState() => _LicenceUploadScreenState();
}

class _LicenceUploadScreenState extends State<LicenceUploadScreen> {
  final _picker = ImagePicker();
  String? _photoPath;
  bool _isSubmitting = false;

  // ── Effective values (fall back to draft on resubmission) ──
  String get _number =>
      widget.licenceNumber ??
      widget.verificationController.draft?.licenceNumber ??
      '';

  String? get _type =>
      widget.licenceType ?? widget.verificationController.draft?.licenceType;

  DateTime get _expiry =>
      widget.expiry ??
      DateTime.tryParse(
        widget.verificationController.draft?.expiryYear != null &&
                widget.verificationController.draft?.expiryMonth != null
            ? '${widget.verificationController.draft!.expiryYear}-'
                '${widget.verificationController.draft!.expiryMonth}-01'
            : '',
      ) ??
      DateTime.now().add(const Duration(days: 365));

  String? get _state =>
      widget.issuingState ??
      widget.verificationController.draft?.issuingState;

  // ── Pickers ───────────────────────────────────────────────────────────

  Future<void> _capture() async {
    final x = await _picker.pickImage(
      source: ImageSource.camera,
      imageQuality: 85,
      maxWidth: 1920,
      maxHeight: 1920,
    );
    if (x == null || !mounted) return;
    _apply(x.path);
  }

  Future<void> _pickFromGallery() async {
    final x = await _picker.pickImage(
      source: ImageSource.gallery,
      imageQuality: 85,
      maxWidth: 1920,
      maxHeight: 1920,
    );
    if (x == null || !mounted) return;
    _apply(x.path);
  }

  void _apply(String path) {
    final ext = p.extension(path).replaceFirst('.', '').toLowerCase();
    if (!const {'jpg', 'jpeg', 'png'}.contains(ext)) {
      _snack('Unsupported file type: .$ext');
      return;
    }
    setState(() => _photoPath = path);
  }

  void _showSourceSheet() {
    final c = context.palette;
    showModalBottomSheet<void>(
      context: context,
      backgroundColor: c.surface,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
      ),
      builder: (ctx) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 12),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Container(
                width: 36,
                height: 4,
                margin: const EdgeInsets.only(bottom: 16),
                decoration: BoxDecoration(
                  color: c.navy20,
                  borderRadius: BorderRadius.circular(2),
                ),
              ),
              _SheetRow(
                icon: Icons.photo_camera_outlined,
                label: 'Take photo',
                onTap: () {
                  Navigator.pop(ctx);
                  _capture();
                },
              ),
              Divider(height: 1, color: c.divider),
              _SheetRow(
                icon: Icons.photo_library_outlined,
                label: 'Choose from gallery',
                onTap: () {
                  Navigator.pop(ctx);
                  _pickFromGallery();
                },
              ),
            ],
          ),
        ),
      ),
    );
  }

  // ── Submit → pop true back to hub ─────────────────────────────────────

  Future<void> _submit() async {
    if (_photoPath == null || _isSubmitting) return;
    if (_number.isEmpty) {
      _snack('Licence number missing — go back and enter it.');
      return;
    }
    setState(() => _isSubmitting = true);

    final result = await widget.verificationController.submitLicence(
      licenceNumber: _number,
      expiryYear: _expiry.year.toString(),
      expiryMonth: _expiry.month.toString().padLeft(2, '0'),
      licenceFilePath: _photoPath!,
      licenceType: _type,
      issuingState: _state,
    );

    if (!mounted) return;
    setState(() => _isSubmitting = false);

    String? error;
    result.when(
      success: (_) {},
      failure: (e) =>
          error = e.fields.isNotEmpty ? e.fields.first.message : e.message,
    );

    if (error != null) {
      _snack('Licence upload failed: $error');
      return;
    }

    widget.verificationController.refreshStatus();
    // Pop true — hub refreshes; chain ends here.
    Navigator.of(context).pop(true);
  }

  void _snack(String s) {
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(s)));
  }

  // ── Build ─────────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final resub = widget.resubmission;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            // ── Header ──
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
              child: Row(
                children: [
                  GestureDetector(
                    onTap: () => Navigator.of(context).pop(false),
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
                  const Spacer(),
                  GestureDetector(
                    onTap: () {},
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Text(
                        'Help',
                        style: TextStyle(
                          fontSize: 15,
                          fontWeight: FontWeight.w600,
                          color: c.brandTeal500,
                        ),
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
                  // ── Title ──
                  Text(
                    resub ? 'Resubmit your licence' : 'Upload your licence',
                    style: TextStyle(
                      fontSize: 26,
                      fontWeight: FontWeight.w800,
                      color: c.text,
                      letterSpacing: -0.4,
                    ),
                  ),
                  const SizedBox(height: 6),
                  Text(
                    resub
                        ? "Please upload a clearer photo of your driver's licence."
                        : "Take a clear photo of your driver's licence (front and back if applicable).",
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w500,
                      color: c.textMuted,
                      height: 1.45,
                    ),
                  ),
                  const SizedBox(height: 28),

                  // ── Empty or captured card ──
                  if (_photoPath == null)
                    _DashedUpload(onTap: _showSourceSheet)
                  else ...[
                    _LicenceCardReplica(
                      photoPath: _photoPath!,
                      licenceNumber: _number,
                      fullName:
                          widget.verificationController.draft?.fullName ?? '—',
                      dateOfBirth: widget.verificationController.draft
                              ?.dateOfBirth ??
                          DateTime(1992, 3, 12),
                      expiry: _expiry,
                      issuingState: _state ?? '—',
                    ),
                    const SizedBox(height: 18),
                    Row(
                      children: [
                        Expanded(
                          child: SizedBox(
                            height: 48,
                            child: OutlinedButton(
                              onPressed: () {
                                setState(() => _photoPath = null);
                                Future.delayed(
                                  const Duration(milliseconds: 120),
                                  _showSourceSheet,
                                );
                              },
                              style: OutlinedButton.styleFrom(
                                foregroundColor: c.text,
                                side: BorderSide(
                                  color: c.border,
                                  width: 1.2,
                                ),
                                shape: RoundedRectangleBorder(
                                  borderRadius: BorderRadius.circular(999),
                                ),
                                textStyle: const TextStyle(
                                  fontSize: 15,
                                  fontWeight: FontWeight.w700,
                                ),
                              ),
                              child: const Text('Retake'),
                            ),
                          ),
                        ),
                        const SizedBox(width: 12),
                        Expanded(
                          child: SizedBox(
                            height: 48,
                            child: FilledButton(
                              onPressed: () {},
                              style: FilledButton.styleFrom(
                                textStyle: const TextStyle(
                                  fontSize: 15,
                                  fontWeight: FontWeight.w700,
                                ),
                              ),
                              child: const Text('Use this photo'),
                            ),
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 24),
                    Text(
                      'Photo tips',
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w700,
                        color: c.text,
                      ),
                    ),
                    const SizedBox(height: 14),
                    const _Tip('Ensure good lighting'),
                    const SizedBox(height: 12),
                    const _Tip('Show all four corners'),
                    const SizedBox(height: 12),
                    const _Tip('Make sure text is clear and readable'),
                  ],
                ],
              ),
            ),

            // ── CTA ──
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed:
                      _photoPath != null && !_isSubmitting ? _submit : null,
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
                            color: c.btnPrimaryFg,
                          ),
                        )
                      : Text(resub ? 'Submit for review' : 'Continue'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Licence card replica (Figma 09)
// ═══════════════════════════════════════════════════════════════════════════

class _LicenceCardReplica extends StatelessWidget {
  const _LicenceCardReplica({
    required this.photoPath,
    required this.licenceNumber,
    required this.fullName,
    required this.dateOfBirth,
    required this.expiry,
    required this.issuingState,
  });

  final String photoPath;
  final String licenceNumber;
  final String fullName;
  final DateTime dateOfBirth;
  final DateTime expiry;
  final String issuingState;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    const nigeriaGreen = Color(0xFF0B6E3F);

    return Stack(
      clipBehavior: Clip.none,
      children: [
        Container(
          decoration: BoxDecoration(
            color: c.surface,
            borderRadius: BorderRadius.circular(16),
            border: Border.all(color: c.border, width: 1.2),
            boxShadow: c.cardShadow,
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              // ── Green header band ──
              Container(
                padding:
                    const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
                decoration: const BoxDecoration(
                  color: nigeriaGreen,
                  borderRadius: BorderRadius.vertical(
                    top: Radius.circular(14.5),
                  ),
                ),
                child: Row(
                  children: [
                    Container(
                      width: 28,
                      height: 28,
                      alignment: Alignment.center,
                      decoration: const BoxDecoration(
                        shape: BoxShape.circle,
                        color: Colors.white,
                      ),
                      child: const Icon(
                        Icons.directions_car_rounded,
                        size: 16,
                        color: nigeriaGreen,
                      ),
                    ),
                    const SizedBox(width: 10),
                    const Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            'FEDERAL REPUBLIC OF NIGERIA',
                            style: TextStyle(
                              fontSize: 11,
                              fontWeight: FontWeight.w800,
                              color: Colors.white,
                              letterSpacing: 0.4,
                            ),
                          ),
                          SizedBox(height: 1),
                          Text(
                            "DRIVER'S LICENCE",
                            style: TextStyle(
                              fontSize: 9.5,
                              fontWeight: FontWeight.w500,
                              color: Colors.white,
                              letterSpacing: 0.3,
                            ),
                          ),
                        ],
                      ),
                    ),
                  ],
                ),
              ),

              // ── Body ──
              Padding(
                padding: const EdgeInsets.fromLTRB(14, 16, 14, 16),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    ClipRRect(
                      borderRadius: BorderRadius.circular(6),
                      child: Image.file(
                        File(photoPath),
                        width: 84,
                        height: 104,
                        fit: BoxFit.cover,
                      ),
                    ),
                    const SizedBox(width: 14),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          _KV(label: 'Licence No.', value: licenceNumber),
                          const SizedBox(height: 10),
                          _KV(label: 'Full Name', value: fullName),
                          const SizedBox(height: 10),
                          Row(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Expanded(
                                child: _KV(
                                  label: 'Date of Birth',
                                  value: _fmtUpper(dateOfBirth),
                                ),
                              ),
                              Expanded(
                                child: _KV(
                                  label: 'Expiry',
                                  value: _fmtUpper(expiry),
                                ),
                              ),
                            ],
                          ),
                          const SizedBox(height: 10),
                          _KV(
                            label: 'Issuing State',
                            value: issuingState.toUpperCase(),
                          ),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),

        // ── Floating "Verified" chip ──
        Positioned(
          top: -10,
          right: 12,
          child: Container(
            padding: const EdgeInsets.symmetric(
              horizontal: 10,
              vertical: 5,
            ),
            decoration: BoxDecoration(
              color: c.success,
              borderRadius: BorderRadius.circular(999),
            ),
            child: const Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(
                  Icons.check_circle_rounded,
                  size: 13,
                  color: Colors.white,
                ),
                SizedBox(width: 4),
                Text(
                  'Verified',
                  style: TextStyle(
                    fontSize: 11.5,
                    fontWeight: FontWeight.w800,
                    color: Colors.white,
                    letterSpacing: 0.2,
                  ),
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }

  static String _fmtUpper(DateTime d) {
    const months = [
      'JAN', 'FEB', 'MAR', 'APR', 'MAY', 'JUN',
      'JUL', 'AUG', 'SEP', 'OCT', 'NOV', 'DEC',
    ];
    final day = d.day.toString().padLeft(2, '0');
    return '$day ${months[d.month - 1]} ${d.year}';
  }
}

class _KV extends StatelessWidget {
  const _KV({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: TextStyle(
            fontSize: 10,
            fontWeight: FontWeight.w600,
            color: c.textMuted,
            letterSpacing: 0.2,
          ),
        ),
        const SizedBox(height: 2),
        Text(
          value,
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w700,
            color: c.text,
          ),
        ),
      ],
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Dashed upload zone
// ═══════════════════════════════════════════════════════════════════════════

class _DashedUpload extends StatelessWidget {
  const _DashedUpload({required this.onTap});
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return GestureDetector(
      onTap: onTap,
      child: CustomPaint(
        painter: _DashPainter(color: c.border),
        child: Container(
          width: double.infinity,
          padding: const EdgeInsets.symmetric(vertical: 48, horizontal: 16),
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
                  Icons.upload_rounded,
                  size: 30,
                  color: c.brandTeal600,
                ),
              ),
              const SizedBox(height: 14),
              Text(
                'Upload or take a photo',
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w700,
                  color: c.text,
                ),
              ),
              const SizedBox(height: 4),
              Text(
                'JPG, PNG or PDF · up to 500 MB',
                style: TextStyle(fontSize: 12.5, color: c.textMuted),
              ),
            ],
          ),
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

// ═══════════════════════════════════════════════════════════════════════════
// Tip row
// ═══════════════════════════════════════════════════════════════════════════

class _Tip extends StatelessWidget {
  const _Tip(this.text);
  final String text;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.only(top: 2),
          child: Icon(
            Icons.check_circle_outline_rounded,
            size: 18,
            color: c.brandTeal600,
          ),
        ),
        const SizedBox(width: 10),
        Expanded(
          child: Text(
            text,
            style: TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w500,
              color: c.text,
              height: 1.4,
            ),
          ),
        ),
      ],
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Source picker sheet row
// ═══════════════════════════════════════════════════════════════════════════

class _SheetRow extends StatelessWidget {
  const _SheetRow({
    required this.icon,
    required this.label,
    required this.onTap,
  });

  final IconData icon;
  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return InkWell(
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 16),
        child: Row(
          children: [
            Icon(icon, size: 24, color: c.brandTeal600),
            const SizedBox(width: 16),
            Text(
              label,
              style: TextStyle(
                fontSize: 15,
                fontWeight: FontWeight.w600,
                color: c.text,
              ),
            ),
          ],
        ),
      ),
    );
  }
}