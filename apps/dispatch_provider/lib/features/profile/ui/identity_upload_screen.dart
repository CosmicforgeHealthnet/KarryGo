import '../../../shared/widgets/dispatch_screen_frame.dart';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';
import 'package:path/path.dart' as p;

import '../../../core/theme/theme_extensions.dart';
import '../../verification/state/verification_controller.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 04 — Upload your ID
//
// Two required uploads on one screen:
//   • Government ID document  → backend field `govt_id_file`
//   • Profile photo           → backend field `profile_photo`
//
// The backend rejects submissions that omit either file, so this screen
// collects both before enabling Continue.
// ═══════════════════════════════════════════════════════════════════════════

class IdentityUploadScreen extends StatefulWidget {
  const IdentityUploadScreen({
    super.key,
    required this.verificationController,
    required this.idType,
    required this.idNumber,
    required this.fullName,
    required this.dateOfBirth,
    required this.gender,
  });

  final VerificationController verificationController;
  final String idType;
  final String idNumber;
  final String fullName;
  final DateTime dateOfBirth;
  final String gender;

  @override
  State<IdentityUploadScreen> createState() => _IdentityUploadScreenState();
}

class _IdentityUploadScreenState extends State<IdentityUploadScreen> {
  final _picker = ImagePicker();
  String? _idPhotoPath;
  String? _profilePhotoPath;
  bool _isSubmitting = false;

  bool get _canContinue =>
      _idPhotoPath != null && _profilePhotoPath != null && !_isSubmitting;

  // ── Capture handlers ──────────────────────────────────────────────────

  Future<void> _captureId() async {
    final x = await _picker.pickImage(
      source: ImageSource.camera,
      imageQuality: 85,
      maxWidth: 1920,
      maxHeight: 1920,
    );
    if (x == null || !mounted) return;
    final ext = p.extension(x.path).replaceFirst('.', '').toLowerCase();
    if (!const {'jpg', 'jpeg', 'png'}.contains(ext)) {
      _snack('Unsupported file type: .$ext');
      return;
    }
    setState(() => _idPhotoPath = x.path);
  }

  Future<void> _captureProfile() async {
    final x = await _picker.pickImage(
      source: ImageSource.camera,
      imageQuality: 85,
      maxWidth: 1080,
      maxHeight: 1080,
      preferredCameraDevice: CameraDevice.front,
    );
    if (x == null || !mounted) return;
    final ext = p.extension(x.path).replaceFirst('.', '').toLowerCase();
    if (!const {'jpg', 'jpeg', 'png'}.contains(ext)) {
      _snack('Unsupported file type: .$ext');
      return;
    }
    setState(() => _profilePhotoPath = x.path);
  }

  void _retakeId() {
    setState(() => _idPhotoPath = null);
    Future.delayed(const Duration(milliseconds: 120), _captureId);
  }

  void _retakeProfile() {
    setState(() => _profilePhotoPath = null);
    Future.delayed(const Duration(milliseconds: 120), _captureProfile);
  }

  // ── Continue → submit identity, pop true ──────────────────────────────

  Future<void> _continue() async {
    if (!_canContinue) return;
    setState(() => _isSubmitting = true);

    final result = await widget.verificationController.submitIdentity(
      govtIdType: widget.idType,
      govtIdNumber: widget.idNumber,
      govtIdFilePath: _idPhotoPath!,
      profilePhotoFilePath: _profilePhotoPath!,
      fullName: widget.fullName,
      dateOfBirth: widget.dateOfBirth,
      gender: widget.gender,
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
      _snack(error!);
      return;
    }

    // Pop true — hub refreshes and marks this row submitted.
    Navigator.of(context).pop(true);
  }

  void _snack(String s) {
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(s)));
  }

  // ── Build ─────────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

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
                  DispatchBackButton(
                    onPressed: () => Navigator.of(context).pop(),
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
                    'Upload your ID',
                    style: TextStyle(
                      fontSize: 28,
                      fontWeight: FontWeight.w700,
                      color: c.text,
                      letterSpacing: -0.4,
                    ),
                  ),
                  const SizedBox(height: 6),
                  Text(
                    'We need two photos to verify your identity.',
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w500,
                      color: c.textMuted,
                      height: 1.45,
                    ),
                  ),
                  const SizedBox(height: 28),

                  // ── Government ID section ──
                  _SectionHeader(
                    index: 1,
                    title: 'Government ID',
                    subtitle:
                        'Take a clear photo of your National ID. Make sure all details are visible.',
                    completed: _idPhotoPath != null,
                  ),
                  const SizedBox(height: 12),
                  if (_idPhotoPath == null)
                    _DashedUpload(
                      icon: Icons.badge_outlined,
                      label: 'Upload or take a photo',
                      hint: 'JPG, PNG or PDF · up to 500 MB',
                      onTap: _captureId,
                    )
                  else ...[
                    _NinCardReplica(
                      photoPath: _idPhotoPath!,
                      fullName: widget.fullName,
                      idNumber: widget.idNumber,
                      dateOfBirth: widget.dateOfBirth,
                      gender: widget.gender,
                    ),
                    const SizedBox(height: 14),
                    _RetakeUseRow(
                      onRetake: _retakeId,
                      onUse: () => setState(() {}),
                    ),
                    const SizedBox(height: 20),
                  ],

                  // ── Profile photo section ──
                  _SectionHeader(
                    index: 2,
                    title: 'Profile photo',
                    subtitle:
                        'Clear, front-facing photo of yourself. This will appear on your profile.',
                    completed: _profilePhotoPath != null,
                  ),
                  const SizedBox(height: 12),
                  if (_profilePhotoPath == null)
                    _DashedUpload(
                      icon: Icons.person_outline_rounded,
                      label: 'Take a selfie',
                      hint: 'JPG or PNG · up to 3 MB',
                      onTap: _captureProfile,
                    )
                  else ...[
                    _ProfilePhotoPreview(photoPath: _profilePhotoPath!),
                    const SizedBox(height: 14),
                    _RetakeUseRow(
                      onRetake: _retakeProfile,
                      onUse: () => setState(() {}),
                    ),
                    const SizedBox(height: 20),
                  ],

                  // ── Info chip ──
                  _InfoChip(
                    text:
                        'Both photos are required before you can continue. Make sure they are clear, well-lit and not cropped.',
                  ),
                ],
              ),
            ),

            // ── Continue ──
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _canContinue ? _continue : null,
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
                      : const Text('Continue'),
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
// Section header with step counter + completion tick
// ═══════════════════════════════════════════════════════════════════════════

class _SectionHeader extends StatelessWidget {
  const _SectionHeader({
    required this.index,
    required this.title,
    required this.subtitle,
    required this.completed,
  });

  final int index;
  final String title;
  final String subtitle;
  final bool completed;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Container(
              width: 22,
              height: 22,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: completed ? c.success : c.brandTeal500,
                shape: BoxShape.circle,
              ),
              child: completed
                  ? const Icon(
                      Icons.check_rounded,
                      size: 14,
                      color: Colors.white,
                    )
                  : Text(
                      '$index',
                      style: const TextStyle(
                        fontSize: 12,
                        fontWeight: FontWeight.w700,
                        color: Colors.white,
                      ),
                    ),
            ),
            const SizedBox(width: 10),
            Text(
              title,
              style: TextStyle(
                fontSize: 15,
                fontWeight: FontWeight.w700,
                color: c.text,
                letterSpacing: -0.2,
              ),
            ),
          ],
        ),
        const SizedBox(height: 6),
        Padding(
          padding: const EdgeInsets.only(left: 32),
          child: Text(
            subtitle,
            style: TextStyle(fontSize: 13, color: c.textMuted, height: 1.4),
          ),
        ),
      ],
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// NIN card replica
// ═══════════════════════════════════════════════════════════════════════════

class _NinCardReplica extends StatelessWidget {
  const _NinCardReplica({
    required this.photoPath,
    required this.fullName,
    required this.idNumber,
    required this.dateOfBirth,
    required this.gender,
  });

  final String photoPath;
  final String fullName;
  final String idNumber;
  final DateTime dateOfBirth;
  final String gender;

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
              Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: 14,
                  vertical: 12,
                ),
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
                        Icons.star_rounded,
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
                              fontWeight: FontWeight.w700,
                              color: Colors.white,
                              letterSpacing: 0.4,
                            ),
                          ),
                          SizedBox(height: 1),
                          Text(
                            'National Identification Number',
                            style: TextStyle(
                              fontSize: 9.5,
                              fontWeight: FontWeight.w500,
                              color: Colors.white,
                            ),
                          ),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
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
                          _KV(label: 'NIN', value: _spaced(idNumber)),
                          const SizedBox(height: 10),
                          _KV(label: 'Name', value: fullName),
                          const SizedBox(height: 10),
                          Row(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Expanded(
                                child: _KV(
                                  label: 'Date of Birth',
                                  value: _fmtDob(dateOfBirth),
                                ),
                              ),
                              Expanded(
                                child: _KV(
                                  label: 'Gender',
                                  value:
                                      gender[0].toUpperCase() +
                                      gender.substring(1),
                                ),
                              ),
                            ],
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
        Positioned(
          top: -10,
          right: 12,
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
            decoration: BoxDecoration(
              color: c.success,
              borderRadius: BorderRadius.circular(999),
            ),
            child: const Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(Icons.check_circle_rounded, size: 13, color: Colors.white),
                SizedBox(width: 4),
                Text(
                  'Captured',
                  style: TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w700,
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

  static String _spaced(String digits) {
    final buf = StringBuffer();
    for (var i = 0; i < digits.length; i++) {
      if (i > 0 && i % 4 == 0) buf.write(' ');
      buf.write(digits[i]);
    }
    return buf.toString();
  }

  static String _fmtDob(DateTime d) {
    const m = [
      'January',
      'February',
      'March',
      'April',
      'May',
      'June',
      'July',
      'August',
      'September',
      'October',
      'November',
      'December',
    ];
    return '${d.day} ${m[d.month - 1]} ${d.year}';
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
// Profile photo preview — circular avatar
// ═══════════════════════════════════════════════════════════════════════════

class _ProfilePhotoPreview extends StatelessWidget {
  const _ProfilePhotoPreview({required this.photoPath});
  final String photoPath;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Stack(
      clipBehavior: Clip.none,
      alignment: Alignment.center,
      children: [
        Container(
          padding: const EdgeInsets.all(4),
          decoration: BoxDecoration(
            color: c.surface,
            shape: BoxShape.circle,
            boxShadow: c.cardShadow,
          ),
          child: ClipOval(
            child: Image.file(
              File(photoPath),
              width: 176,
              height: 176,
              fit: BoxFit.cover,
            ),
          ),
        ),
        Positioned(
          top: 4,
          right: 4,
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
            decoration: BoxDecoration(
              color: c.success,
              borderRadius: BorderRadius.circular(999),
            ),
            child: const Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(Icons.check_circle_rounded, size: 13, color: Colors.white),
                SizedBox(width: 4),
                Text(
                  'Captured',
                  style: TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w700,
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
}

// ═══════════════════════════════════════════════════════════════════════════
// Retake / Use this photo row
// ═══════════════════════════════════════════════════════════════════════════

class _RetakeUseRow extends StatelessWidget {
  const _RetakeUseRow({required this.onRetake, required this.onUse});
  final VoidCallback onRetake;
  final VoidCallback onUse;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Row(
      children: [
        Expanded(
          child: SizedBox(
            height: 48,
            child: OutlinedButton(
              onPressed: onRetake,
              style: OutlinedButton.styleFrom(
                foregroundColor: c.text,
                side: BorderSide(color: c.border, width: 1.2),
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
              onPressed: onUse,
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
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Dashed upload zone
// ═══════════════════════════════════════════════════════════════════════════

class _DashedUpload extends StatelessWidget {
  const _DashedUpload({
    required this.icon,
    required this.label,
    required this.hint,
    required this.onTap,
  });

  final IconData icon;
  final String label;
  final String hint;
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
          padding: const EdgeInsets.symmetric(vertical: 40, horizontal: 16),
          decoration: BoxDecoration(
            color: c.surface,
            borderRadius: BorderRadius.circular(16),
          ),
          child: Column(
            children: [
              Container(
                width: 60,
                height: 60,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  color: c.teal10,
                  shape: BoxShape.circle,
                ),
                child: Icon(icon, size: 28, color: c.brandTeal600),
              ),
              const SizedBox(height: 14),
              Text(
                label,
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w700,
                  color: c.text,
                ),
              ),
              const SizedBox(height: 4),
              Text(hint, style: TextStyle(fontSize: 13, color: c.textMuted)),
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
// Info chip
// ═══════════════════════════════════════════════════════════════════════════

class _InfoChip extends StatelessWidget {
  const _InfoChip({required this.text});
  final String text;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
      decoration: BoxDecoration(
        color: c.info.withValues(alpha: 0.10),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(Icons.info_outline_rounded, size: 18, color: c.info),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              text,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w500,
                color: c.info,
                height: 1.4,
              ),
            ),
          ),
        ],
      ),
    );
  }
}
