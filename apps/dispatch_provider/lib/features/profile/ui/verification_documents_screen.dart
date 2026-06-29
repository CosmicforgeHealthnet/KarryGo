import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import '../../verification/state/verification_controller.dart';
import 'face_verification_screen.dart';

class VerificationDocumentsScreen extends StatefulWidget {
  const VerificationDocumentsScreen({
    super.key,
    this.verificationStatus = 'unverified',
    required this.verificationController,
  });

  final String verificationStatus;
  final VerificationController verificationController;

  @override
  State<VerificationDocumentsScreen> createState() =>
      _VerificationDocumentsScreenState();
}

class _VerificationDocumentsScreenState
    extends State<VerificationDocumentsScreen> {
  // Licence fields
  final _licenseController = TextEditingController();
  String? _selectedYear;
  String? _selectedMonth;

  // Identity fields
  String? _govtIdType;
  final _govtIdNumberController = TextEditingController();

  static const _govtIdTypes = [
    'nin',
    'voter_card',
    'passport',
    'drivers_licence',
  ];

  static const _govtIdTypeLabels = {
    'nin': 'National ID (NIN)',
    'voter_card': "Voter's Card",
    'passport': 'International Passport',
    'drivers_licence': "Driver's License",
  };

  // Uploaded file paths
  String? _govIdFilePath;
  String? _profilePhotoFilePath;
  String? _licenceFilePath;

  // Displayed file names (basename only)
  String? _govIdFileName;
  String? _profilePhotoFileName;
  String? _licenceFileName;

  bool _isSubmitting = false;
  bool _hasSubmitted = false;

  bool get _canProceed =>
      _licenseController.text.trim().length >= 5 &&
      _selectedYear != null &&
      _selectedMonth != null &&
      _govtIdType != null &&
      _govtIdNumberController.text.trim().length >= 5 &&
      _govIdFilePath != null &&
      _profilePhotoFilePath != null &&
      _licenceFilePath != null &&
      !_isSubmitting &&
      !_hasSubmitted;

  @override
  void dispose() {
    _licenseController.dispose();
    _govtIdNumberController.dispose();
    super.dispose();
  }

  Future<void> _pickFile(_UploadSlot slot) async {
    final result = await FilePicker.platform.pickFiles(
      type: FileType.custom,
      allowedExtensions: ['jpg', 'jpeg', 'png', 'pdf'],
      allowMultiple: false,
    );
    if (result == null || result.files.isEmpty) return;
    final file = result.files.first;
    if (file.path == null) return;
    setState(() {
      switch (slot) {
        case _UploadSlot.govId:
          _govIdFilePath = file.path;
          _govIdFileName = file.name;
          break;
        case _UploadSlot.profilePhoto:
          _profilePhotoFilePath = file.path;
          _profilePhotoFileName = file.name;
          break;
        case _UploadSlot.driverLicence:
          _licenceFilePath = file.path;
          _licenceFileName = file.name;
          break;
      }
    });
  }

  void _removeFile(_UploadSlot slot) {
    setState(() {
      switch (slot) {
        case _UploadSlot.govId:
          _govIdFilePath = null;
          _govIdFileName = null;
          break;
        case _UploadSlot.profilePhoto:
          _profilePhotoFilePath = null;
          _profilePhotoFileName = null;
          break;
        case _UploadSlot.driverLicence:
          _licenceFilePath = null;
          _licenceFileName = null;
          break;
      }
    });
  }

  Future<void> _onNext() async {
    if (_isSubmitting || _hasSubmitted) return;

    // Frontend validation
    final idNum = _govtIdNumberController.text.trim();
    final licNum = _licenseController.text.trim();
    if (idNum.length < 5) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text(
            'Government ID number must be at least 5 characters.',
          ),
        ),
      );
      return;
    }
    if (licNum.length < 5) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text(
            "Driver's licence number must be at least 5 characters.",
          ),
        ),
      );
      return;
    }

    setState(() => _isSubmitting = true);

    // Submit identity (govt ID + profile photo)
    final identityResult = await widget.verificationController.submitIdentity(
      govtIdType: _govtIdType!,
      govtIdNumber: idNum,
      govtIdFilePath: _govIdFilePath!,
      profilePhotoFilePath: _profilePhotoFilePath!,
    );

    if (!mounted) return;

    String? identityError;
    identityResult.when(
      success: (_) {},
      failure: (error) => identityError = error.fields.isNotEmpty
          ? error.fields.first.message
          : error.message,
    );

    if (identityError != null) {
      setState(() => _isSubmitting = false);
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Identity upload failed: $identityError')),
      );
      return;
    }

    // Submit driver's licence
    final licenceResult = await widget.verificationController.submitLicence(
      licenceNumber: licNum,
      expiryYear: _selectedYear!,
      expiryMonth: _selectedMonth!,
      licenceFilePath: _licenceFilePath!,
    );

    if (!mounted) return;

    String? licenceError;
    licenceResult.when(
      success: (_) {},
      failure: (error) => licenceError = error.fields.isNotEmpty
          ? error.fields.first.message
          : error.message,
    );

    if (licenceError != null) {
      setState(() => _isSubmitting = false);
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text("Licence upload failed: $licenceError")),
      );
      return;
    }

    // Both uploads succeeded — lock the form so back-navigation cannot resubmit.
    setState(() {
      _isSubmitting = false;
      _hasSubmitted = true;
    });

    // Refresh status so the hub screen shows "Pending Review" immediately.
    widget.verificationController.refreshStatus();

    if (!mounted) return;

    final faceResult = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) => FaceVerificationScreen(
          verificationController: widget.verificationController,
        ),
      ),
    );

    if (!mounted) return;
    // Propagate success up to VerificationIntroScreen → ProfileScreen.
    if (faceResult == true) {
      widget.verificationController.refreshStatus();
      Navigator.of(context).pop(true);
    } else {
      // Face screen was dismissed (back button or retry) after documents were
      // already submitted. Pop back to the hub so the user can tap "Start Face
      // Verification" there, rather than being stuck on a locked form.
      Navigator.of(context).pop(false);
    }
  }

  @override
  Widget build(BuildContext context) {
    // After both uploads succeed the form is locked. Show a plain redirect
    // screen instead of the filled form — this prevents the form from flashing
    // during the pop-back animation when returning from face verification.
    if (_hasSubmitted) {
      return const Scaffold(
        backgroundColor: Colors.white,
        body: Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              CircularProgressIndicator(color: Color(0xFF4CAF50)),
              SizedBox(height: 16),
              Text(
                'Documents submitted.',
                style: TextStyle(fontSize: 14, color: Color(0xFF888888)),
              ),
            ],
          ),
        ),
      );
    }
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
                children: [
                  // ── Header ─────────────────────────────────────
                  GestureDetector(
                    behavior: HitTestBehavior.opaque,
                    onTap: () => Navigator.of(context).pop(),
                    child: const Padding(
                      padding: EdgeInsets.only(bottom: 4),
                      child: Align(
                        alignment: Alignment.centerLeft,
                        child: Icon(
                          Icons.arrow_back_ios_new,
                          size: 20,
                          color: Color(0xFF1A1A1A),
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(height: 16),
                  const Text(
                    'Verification & Documents',
                    style: TextStyle(
                      fontSize: 20,
                      fontWeight: FontWeight.w800,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 2),
                  const Text(
                    'Verify your identity',
                    style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
                  ),
                  const SizedBox(height: 20),

                  // ── Verification status ─────────────────────────
                  _VerificationStatusBadge(status: widget.verificationStatus),

                  const SizedBox(height: 24),

                  // ── Government ID type ──────────────────────────
                  const _FieldLabel('Government ID Type'),
                  const SizedBox(height: 8),
                  _DropdownField(
                    hint: 'Select ID Type',
                    value: _govtIdType,
                    items: _govtIdTypes,
                    labelOf: (v) => _govtIdTypeLabels[v] ?? v,
                    onChanged: (v) => setState(() {
                      _govtIdType = v;
                    }),
                    suffixIcon: Icons.keyboard_arrow_down,
                  ),

                  const SizedBox(height: 16),

                  // ── Government ID number ────────────────────────
                  const _FieldLabel('Government ID Number'),
                  const SizedBox(height: 8),
                  TextField(
                    controller: _govtIdNumberController,
                    onChanged: (_) => setState(() {}),
                    style: const TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w500,
                      color: Color(0xFF1A1A1A),
                    ),
                    decoration: InputDecoration(
                      hintText: 'Enter ID number',
                      hintStyle: const TextStyle(
                        color: Color(0xFFAAAAAA),
                        fontSize: 14,
                      ),
                      filled: true,
                      fillColor: const Color(0xFFF5F6F8),
                      contentPadding: const EdgeInsets.symmetric(
                        horizontal: 16,
                        vertical: 14,
                      ),
                      border: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(12),
                        borderSide: BorderSide.none,
                      ),
                    ),
                  ),

                  const SizedBox(height: 16),

                  // ── Driver's license number ─────────────────────
                  const _FieldLabel("Driver's License Number"),
                  const SizedBox(height: 8),
                  TextField(
                    controller: _licenseController,
                    onChanged: (_) => setState(() {}),
                    style: const TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w500,
                      color: Color(0xFF1A1A1A),
                    ),
                    decoration: InputDecoration(
                      hintText: 'e.g. ABC1234567890',
                      hintStyle: const TextStyle(
                        color: Color(0xFFAAAAAA),
                        fontSize: 14,
                      ),
                      filled: true,
                      fillColor: const Color(0xFFF5F6F8),
                      contentPadding: const EdgeInsets.symmetric(
                        horizontal: 16,
                        vertical: 14,
                      ),
                      border: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(12),
                        borderSide: BorderSide.none,
                      ),
                    ),
                  ),

                  const SizedBox(height: 16),

                  // ── Expiry year + month ─────────────────────────
                  Row(
                    children: [
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            const _FieldLabel('Expiry Year'),
                            const SizedBox(height: 8),
                            _DropdownField(
                              hint: 'Select Year',
                              value: _selectedYear,
                              items: List.generate(
                                10,
                                (i) => (DateTime.now().year + i).toString(),
                              ),
                              onChanged: (v) =>
                                  setState(() => _selectedYear = v),
                              suffixIcon: Icons.keyboard_arrow_down,
                            ),
                          ],
                        ),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            const _FieldLabel('Expiry Month'),
                            const SizedBox(height: 8),
                            _DropdownField(
                              hint: 'Select Month',
                              value: _selectedMonth,
                              items: List.generate(
                                12,
                                (i) => (i + 1).toString().padLeft(2, '0'),
                              ),
                              onChanged: (v) =>
                                  setState(() => _selectedMonth = v),
                              suffixIcon: Icons.calendar_today_outlined,
                            ),
                          ],
                        ),
                      ),
                    ],
                  ),

                  const SizedBox(height: 28),

                  // ── Government ID upload ────────────────────────
                  _UploadSection(
                    title: 'Upload Government ID',
                    description:
                        'Upload a clear photo of your government-issued ID (passport, NIN slip, voter card).',
                    uploadLabel: 'Upload ID',
                    uploadHint:
                        '(International passport, NIN, Voter\'s card or Driver\'s License)',
                    filePath: _govIdFilePath,
                    fileName: _govIdFileName,
                    onPick: () => _pickFile(_UploadSlot.govId),
                    onRemove: () => _removeFile(_UploadSlot.govId),
                  ),

                  const SizedBox(height: 24),

                  // ── Profile photo upload ────────────────────────
                  _UploadSection(
                    title: 'Upload Profile Photo',
                    description:
                        'Upload a clear face photo of yourself. This is used to verify your identity matches your ID.',
                    uploadLabel: 'Upload Photo',
                    uploadHint: '(Clear front-facing photo, JPEG or PNG)',
                    filePath: _profilePhotoFilePath,
                    fileName: _profilePhotoFileName,
                    onPick: () => _pickFile(_UploadSlot.profilePhoto),
                    onRemove: () => _removeFile(_UploadSlot.profilePhoto),
                  ),

                  const SizedBox(height: 24),

                  // ── Driver's licence upload ─────────────────────
                  _UploadSection(
                    title: "Upload Driver's Licence",
                    description:
                        "Upload a clear photo of your driver's licence (front page).",
                    uploadLabel: 'Upload Licence',
                    uploadHint: '(Clear photo of front of licence)',
                    filePath: _licenceFilePath,
                    fileName: _licenceFileName,
                    onPick: () => _pickFile(_UploadSlot.driverLicence),
                    onRemove: () => _removeFile(_UploadSlot.driverLicence),
                  ),
                ],
              ),
            ),

            // ── Next button ─────────────────────────────────────────
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _canProceed ? _onNext : null,
                  style: FilledButton.styleFrom(
                    backgroundColor: const Color(0xFF4CAF50),
                    disabledBackgroundColor: const Color(
                      0xFF4CAF50,
                    ).withValues(alpha: 0.35),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(999),
                    ),
                  ),
                  child: _isSubmitting
                      ? const SizedBox(
                          width: 20,
                          height: 20,
                          child: CircularProgressIndicator(
                            strokeWidth: 2,
                            color: Colors.white,
                          ),
                        )
                      : const Text(
                          'Next',
                          style: TextStyle(
                            fontSize: 16,
                            fontWeight: FontWeight.w700,
                            color: Colors.white,
                          ),
                        ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ── Upload slot enum ──────────────────────────────────────────────────────────

enum _UploadSlot { govId, profilePhoto, driverLicence }

// ── Verification status badge ─────────────────────────────────────────────────

class _VerificationStatusBadge extends StatelessWidget {
  const _VerificationStatusBadge({required this.status});
  final String status;

  @override
  Widget build(BuildContext context) {
    final Color color;
    final String label;

    switch (status) {
      case 'verified':
        color = const Color(0xFF2E7D32);
        label = 'Verified';
        break;
      case 'pending_review':
        color = const Color(0xFFF57F17);
        label = 'Submitted — Pending review';
        break;
      case 'in_progress':
        color = const Color(0xFF1565C0);
        label = 'In Progress';
        break;
      case 'rejected':
        color = const Color(0xFFC62828);
        label = 'Rejected';
        break;
      case 'suspended':
        color = const Color(0xFFC62828);
        label = 'Suspended';
        break;
      case 'not_started':
      default:
        color = const Color(0xFFE53935);
        label = 'Unverified';
    }

    return Row(
      children: [
        const Text(
          'Verification Status: ',
          style: TextStyle(
            fontSize: 15,
            fontWeight: FontWeight.w700,
            color: Color(0xFF1A1A1A),
          ),
        ),
        Text(
          label,
          style: TextStyle(
            fontSize: 15,
            fontWeight: FontWeight.w700,
            color: color,
          ),
        ),
      ],
    );
  }
}

// ── Upload section ────────────────────────────────────────────────────────────

class _UploadSection extends StatelessWidget {
  const _UploadSection({
    required this.title,
    required this.description,
    required this.uploadLabel,
    required this.uploadHint,
    required this.filePath,
    required this.fileName,
    required this.onPick,
    required this.onRemove,
  });

  final String title;
  final String description;
  final String uploadLabel;
  final String uploadHint;
  final String? filePath;
  final String? fileName;
  final VoidCallback onPick;
  final VoidCallback onRemove;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          title,
          style: const TextStyle(
            fontSize: 15,
            fontWeight: FontWeight.w700,
            color: Color(0xFF1A1A1A),
          ),
        ),
        const SizedBox(height: 4),
        Text(
          description,
          style: const TextStyle(
            fontSize: 12,
            color: Color(0xFF4CAF50),
            height: 1.4,
          ),
        ),
        const SizedBox(height: 10),
        if (filePath == null)
          _UploadBox(label: uploadLabel, hint: uploadHint, onTap: onPick)
        else
          _UploadedFileRow(name: fileName ?? filePath!, onRemove: onRemove),
      ],
    );
  }
}

// ── Empty upload box ──────────────────────────────────────────────────────────

class _UploadBox extends StatelessWidget {
  const _UploadBox({
    required this.label,
    required this.hint,
    required this.onTap,
  });

  final String label;
  final String hint;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: CustomPaint(
        painter: _DashedBorderPainter(),
        child: Container(
          width: double.infinity,
          padding: const EdgeInsets.symmetric(vertical: 20, horizontal: 16),
          child: Column(
            children: [
              const Icon(
                Icons.download_outlined,
                size: 28,
                color: Color(0xFF1A1A1A),
              ),
              const SizedBox(height: 8),
              Text(
                label,
                style: const TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 4),
              Text(
                hint,
                style: const TextStyle(fontSize: 11, color: Color(0xFF888888)),
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: 2),
              const Text(
                'Supported File type: JPEG, PNG, PDF',
                style: TextStyle(fontSize: 11, color: Color(0xFF888888)),
              ),
              const Text(
                'Maximum File Size: 500MB',
                style: TextStyle(fontSize: 11, color: Color(0xFF888888)),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

// ── Uploaded file row ─────────────────────────────────────────────────────────

class _UploadedFileRow extends StatelessWidget {
  const _UploadedFileRow({required this.name, required this.onRemove});
  final String name;
  final VoidCallback onRemove;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        ClipRRect(
          borderRadius: BorderRadius.circular(6),
          child: Container(
            width: 40,
            height: 40,
            color: const Color(0xFFE0E0E0),
            child: const Icon(
              Icons.image_outlined,
              size: 22,
              color: Color(0xFF888888),
            ),
          ),
        ),
        const SizedBox(width: 10),
        Expanded(
          child: Text(
            name,
            style: const TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w500,
              color: Color(0xFF1A1A1A),
            ),
            overflow: TextOverflow.ellipsis,
          ),
        ),
        GestureDetector(
          onTap: onRemove,
          child: Container(
            width: 26,
            height: 26,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              border: Border.all(color: const Color(0xFFCCCCCC)),
            ),
            child: const Icon(Icons.close, size: 14, color: Color(0xFF888888)),
          ),
        ),
      ],
    );
  }
}

// ── Dashed border painter ─────────────────────────────────────────────────────

class _DashedBorderPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    const dashWidth = 6.0;
    const dashSpace = 4.0;
    const radius = 12.0;
    final paint = Paint()
      ..color = const Color(0xFFCCCCCC)
      ..strokeWidth = 1.2
      ..style = PaintingStyle.stroke;

    final rect = RRect.fromRectAndRadius(
      Offset.zero & size,
      const Radius.circular(radius),
    );
    final path = Path()..addRRect(rect);
    final metrics = path.computeMetrics();

    for (final metric in metrics) {
      double distance = 0;
      while (distance < metric.length) {
        final start = distance;
        final end = (distance + dashWidth).clamp(0, metric.length);
        canvas.drawPath(metric.extractPath(start, end.toDouble()), paint);
        distance += dashWidth + dashSpace;
      }
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}

// ── Dropdown field ────────────────────────────────────────────────────────────

class _DropdownField extends StatelessWidget {
  const _DropdownField({
    required this.hint,
    required this.value,
    required this.items,
    required this.onChanged,
    required this.suffixIcon,
    this.labelOf,
  });

  final String hint;
  final String? value;
  final List<String> items;
  final ValueChanged<String?> onChanged;
  final IconData suffixIcon;
  final String Function(String)? labelOf;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12),
      decoration: BoxDecoration(
        color: const Color(0xFFF5F6F8),
        borderRadius: BorderRadius.circular(12),
      ),
      child: DropdownButtonHideUnderline(
        child: DropdownButton<String>(
          value: value,
          isExpanded: true,
          hint: Text(
            hint,
            style: const TextStyle(color: Color(0xFFAAAAAA), fontSize: 13),
          ),
          icon: Icon(suffixIcon, size: 18, color: const Color(0xFF888888)),
          items: items
              .map(
                (e) => DropdownMenuItem(
                  value: e,
                  child: Text(
                    labelOf != null ? labelOf!(e) : e,
                    style: const TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w500,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                ),
              )
              .toList(),
          onChanged: onChanged,
        ),
      ),
    );
  }
}

// ── Field label ───────────────────────────────────────────────────────────────

class _FieldLabel extends StatelessWidget {
  const _FieldLabel(this.text);
  final String text;

  @override
  Widget build(BuildContext context) {
    return Text(
      text,
      style: const TextStyle(
        fontSize: 13,
        fontWeight: FontWeight.w700,
        color: Color(0xFF1A1A1A),
      ),
    );
  }
}
