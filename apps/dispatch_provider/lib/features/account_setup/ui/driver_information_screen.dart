import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

// ── Data model ──────────────────────────────────────────────────────────────

class DriverInformationData {
  const DriverInformationData({
    required this.licenseFileName,
    required this.vehicleRegFileName,
    required this.guarantorName,
    required this.guarantorPhone,
    required this.emergencyName,
    required this.emergencyPhone,
    required this.relationshipType,
  });

  final String licenseFileName;
  final String vehicleRegFileName;
  final String guarantorName;
  final String guarantorPhone;
  final String emergencyName;
  final String emergencyPhone;
  final String relationshipType;
}

// ── Screen ───────────────────────────────────────────────────────────────────

class DriverInformationScreen extends StatefulWidget {
  const DriverInformationScreen({
    super.key,
    required this.onSubmit,
    required this.onBack,
    required this.currentStep,
    required this.totalSteps,
  });

  final ValueChanged<DriverInformationData> onSubmit;
  final VoidCallback onBack;
  final int currentStep;
  final int totalSteps;

  @override
  State<DriverInformationScreen> createState() =>
      _DriverInformationScreenState();
}

class _DriverInformationScreenState extends State<DriverInformationScreen> {
  String? _licenseFileName;
  bool _isUploadingLicense = false;
  double _licenseProgress = 0;

  String? _vehicleRegFileName;
  bool _isUploadingVehicleReg = false;
  double _vehicleRegProgress = 0;

  final _guarantorNameController = TextEditingController();
  final _guarantorPhoneController = TextEditingController();
  final _emergencyNameController = TextEditingController();
  final _emergencyPhoneController = TextEditingController();
  String? _relationshipType;

  static const _relationships = [
    'Parent', 'Sibling', 'Spouse', 'Friend', 'Child', 'Other',
  ];

  @override
  void dispose() {
    _guarantorNameController.dispose();
    _guarantorPhoneController.dispose();
    _emergencyNameController.dispose();
    _emergencyPhoneController.dispose();
    super.dispose();
  }

  bool get _canSubmit =>
      _licenseFileName != null &&
      _vehicleRegFileName != null &&
      _guarantorNameController.text.trim().isNotEmpty &&
      _guarantorPhoneController.text.trim().isNotEmpty &&
      _emergencyNameController.text.trim().isNotEmpty &&
      _emergencyPhoneController.text.trim().isNotEmpty &&
      _relationshipType != null;

  void _uploadLicense() async {
    setState(() { _isUploadingLicense = true; _licenseProgress = 0; });
    for (int i = 1; i <= 10; i++) {
      await Future.delayed(const Duration(milliseconds: 120));
      if (!mounted) return;
      setState(() => _licenseProgress = i / 10);
    }
    setState(() { _isUploadingLicense = false; _licenseFileName = 'drivers_license.jpg'; });
  }

  void _uploadVehicleReg() async {
    setState(() { _isUploadingVehicleReg = true; _vehicleRegProgress = 0; });
    for (int i = 1; i <= 10; i++) {
      await Future.delayed(const Duration(milliseconds: 120));
      if (!mounted) return;
      setState(() => _vehicleRegProgress = i / 10);
    }
    setState(() { _isUploadingVehicleReg = false; _vehicleRegFileName = 'vehicle_registration.jpg'; });
  }

  void _submit() {
    widget.onSubmit(DriverInformationData(
      licenseFileName: _licenseFileName!,
      vehicleRegFileName: _vehicleRegFileName!,
      guarantorName: _guarantorNameController.text.trim(),
      guarantorPhone: _guarantorPhoneController.text.trim(),
      emergencyName: _emergencyNameController.text.trim(),
      emergencyPhone: _emergencyPhoneController.text.trim(),
      relationshipType: _relationshipType!,
    ));
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFFF5F5F5),
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.fromLTRB(24, 20, 24, 24),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    // Standard back arrow
                    GestureDetector(
                      onTap: widget.onBack,
                      behavior: HitTestBehavior.opaque,
                      child: const SizedBox(
                        height: 36,
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
                    _ProgressBar(
                      current: widget.currentStep,
                      total: widget.totalSteps,
                    ),
                    const SizedBox(height: 28),
                    const Text(
                      'Driver Information!',
                      style: TextStyle(
                        fontSize: 22,
                        fontWeight: FontWeight.w800,
                        color: Color(0xFF1A1A1A),
                        letterSpacing: -0.3,
                      ),
                    ),
                    const SizedBox(height: 8),
                    const Text(
                      'You need to provide the required information to complete the last step of your account registration.',
                      style: TextStyle(
                        fontSize: 13,
                        color: Color(0xFF888888),
                        height: 1.5,
                      ),
                    ),
                    const SizedBox(height: 28),

                    // ── Driver's License upload
                    const _FieldLabel(label: "Upload Driver's License"),
                    const SizedBox(height: 8),
                    _UploadBox(
                      fileName: _licenseFileName,
                      isUploading: _isUploadingLicense,
                      progress: _licenseProgress,
                      onTap: _uploadLicense,
                      onRemove: () => setState(() {
                        _licenseFileName = null;
                        _licenseProgress = 0;
                      }),
                    ),
                    const SizedBox(height: 8),
                    const _UploadHint(),
                    const SizedBox(height: 20),

                    // ── Vehicle Registration upload
                    const _FieldLabel(label: 'Upload Vehicle Registration Sticker'),
                    const SizedBox(height: 8),
                    _UploadBox(
                      fileName: _vehicleRegFileName,
                      isUploading: _isUploadingVehicleReg,
                      progress: _vehicleRegProgress,
                      onTap: _uploadVehicleReg,
                      onRemove: () => setState(() {
                        _vehicleRegFileName = null;
                        _vehicleRegProgress = 0;
                      }),
                    ),
                    const SizedBox(height: 8),
                    const _UploadHint(),
                    const SizedBox(height: 28),

                    // ── Guarantor Information section
                    const _InfoBanner(
                      text: 'We need you to provide guarantor information. This could be a family member or close relative.',
                    ),
                    const SizedBox(height: 20),
                    const Text(
                      'Guarantor Information',
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w800,
                        color: Color(0xFF1A1A1A),
                      ),
                    ),
                    const SizedBox(height: 16),
                    const _FieldLabel(label: 'Full Name'),
                    const SizedBox(height: 8),
                    _InputField(
                      controller: _guarantorNameController,
                      hint: 'Enter name',
                      onChanged: (_) => setState(() {}),
                    ),
                    const SizedBox(height: 16),
                    const _FieldLabel(label: 'Mobile Number'),
                    const SizedBox(height: 8),
                    _PhoneRow(controller: _guarantorPhoneController, onChanged: (_) => setState(() {})),
                    const SizedBox(height: 28),

                    // ── Emergency Contact section
                    const _InfoBanner(
                      text: 'Provide emergency contact information. This could be a family member or close individual to guarantor.',
                    ),
                    const SizedBox(height: 20),
                    const Text(
                      'Emergency Contact',
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w800,
                        color: Color(0xFF1A1A1A),
                      ),
                    ),
                    const SizedBox(height: 16),
                    const _FieldLabel(label: 'Full Name'),
                    const SizedBox(height: 8),
                    _InputField(
                      controller: _emergencyNameController,
                      hint: 'Enter name',
                      onChanged: (_) => setState(() {}),
                    ),
                    const SizedBox(height: 16),
                    const _FieldLabel(label: 'Mobile Number'),
                    const SizedBox(height: 8),
                    _PhoneRow(controller: _emergencyPhoneController, onChanged: (_) => setState(() {})),
                    const SizedBox(height: 16),
                    const _FieldLabel(label: 'Relationship Type'),
                    const SizedBox(height: 8),
                    _RelationshipDropdown(
                      value: _relationshipType,
                      relationships: _relationships,
                      onChanged: (v) => setState(() => _relationshipType = v),
                    ),
                    const SizedBox(height: 8),
                  ],
                ),
              ),
            ),

            // Pinned bottom button
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 8, 24, 28),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _canSubmit ? _submit : null,
                  style: FilledButton.styleFrom(
                    backgroundColor: const Color(0xFF4CAF50),
                    disabledBackgroundColor:
                        const Color(0xFF4CAF50).withValues(alpha: 0.4),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(999),
                    ),
                  ),
                  child: const Text(
                    'Final Step',
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

// ── Shared widgets ────────────────────────────────────────────────────────────

class _ProgressBar extends StatelessWidget {
  const _ProgressBar({required this.current, required this.total});
  final int current;
  final int total;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: List.generate(total, (i) {
        return Expanded(
          child: Container(
            margin: EdgeInsets.only(right: i < total - 1 ? 4 : 0),
            height: 4,
            decoration: BoxDecoration(
              color: i < current
                  ? const Color(0xFF4CAF50)
                  : const Color(0xFFDDDDDD),
              borderRadius: BorderRadius.circular(2),
            ),
          ),
        );
      }),
    );
  }
}

class _FieldLabel extends StatelessWidget {
  const _FieldLabel({required this.label});
  final String label;

  @override
  Widget build(BuildContext context) {
    return Text(
      label,
      style: const TextStyle(
        fontSize: 13,
        fontWeight: FontWeight.w600,
        color: Color(0xFF1A1A1A),
      ),
    );
  }
}

class _InfoBanner extends StatelessWidget {
  const _InfoBanner({required this.text});
  final String text;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: const Color(0xFFFFF8E1),
        borderRadius: BorderRadius.circular(10),
      ),
      child: Text(
        text,
        style: const TextStyle(
          fontSize: 12,
          color: Color(0xFF888888),
          height: 1.5,
        ),
      ),
    );
  }
}

class _UploadHint extends StatelessWidget {
  const _UploadHint();

  @override
  Widget build(BuildContext context) {
    return const Text(
      'International passport, NIN, Voter\'s card or Driver\'s License\nSupported file type: JPG, PNG\nMaximum File Size: 500MB',
      style: TextStyle(
        fontSize: 11,
        color: Color(0xFFAAAAAA),
        height: 1.6,
      ),
    );
  }
}

class _InputField extends StatelessWidget {
  const _InputField({
    required this.controller,
    required this.hint,
    this.keyboardType,
    this.inputFormatters,
    this.onChanged,
  });

  final TextEditingController controller;
  final String hint;
  final TextInputType? keyboardType;
  final List<TextInputFormatter>? inputFormatters;
  final ValueChanged<String>? onChanged;

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      keyboardType: keyboardType,
      inputFormatters: inputFormatters,
      onChanged: onChanged,
      style: const TextStyle(fontSize: 14, color: Color(0xFF1A1A1A)),
      decoration: InputDecoration(
        hintText: hint,
        hintStyle: const TextStyle(color: Color(0xFFBBBBBB), fontSize: 14),
        filled: true,
        fillColor: Colors.white,
        contentPadding:
            const EdgeInsets.symmetric(horizontal: 14, vertical: 15),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(10),
          borderSide: const BorderSide(color: Color(0xFFE0E0E0)),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(10),
          borderSide: const BorderSide(color: Color(0xFFE0E0E0)),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(10),
          borderSide: const BorderSide(color: Color(0xFF4CAF50), width: 1.5),
        ),
      ),
    );
  }
}

class _PhoneRow extends StatelessWidget {
  const _PhoneRow({required this.controller, this.onChanged});
  final TextEditingController controller;
  final ValueChanged<String>? onChanged;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Container(
          height: 50,
          padding: const EdgeInsets.symmetric(horizontal: 12),
          decoration: BoxDecoration(
            color: Colors.white,
            borderRadius: BorderRadius.circular(10),
            border: Border.all(color: const Color(0xFFE0E0E0)),
          ),
          child: const Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              _NigeriaFlag(),
              SizedBox(width: 6),
              Text(
                '+234',
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              SizedBox(width: 2),
              Icon(
                Icons.keyboard_arrow_down_rounded,
                size: 18,
                color: Color(0xFF888888),
              ),
            ],
          ),
        ),
        const SizedBox(width: 10),
        Expanded(
          child: _InputField(
            controller: controller,
            hint: '8067735987',
            keyboardType: TextInputType.phone,
            inputFormatters: [FilteringTextInputFormatter.digitsOnly],
            onChanged: onChanged,
          ),
        ),
      ],
    );
  }
}

class _RelationshipDropdown extends StatelessWidget {
  const _RelationshipDropdown({
    required this.value,
    required this.relationships,
    required this.onChanged,
  });

  final String? value;
  final List<String> relationships;
  final ValueChanged<String?> onChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: const Color(0xFFE0E0E0)),
      ),
      child: DropdownButtonHideUnderline(
        child: DropdownButton<String>(
          value: value,
          hint: const Text(
            'Select Relationship Type',
            style: TextStyle(color: Color(0xFFBBBBBB), fontSize: 14),
          ),
          isExpanded: true,
          icon: const Icon(
            Icons.keyboard_arrow_down_rounded,
            color: Color(0xFF888888),
          ),
          style: const TextStyle(fontSize: 14, color: Color(0xFF1A1A1A)),
          items: relationships
              .map((r) => DropdownMenuItem(value: r, child: Text(r)))
              .toList(),
          onChanged: onChanged,
        ),
      ),
    );
  }
}

class _UploadBox extends StatelessWidget {
  const _UploadBox({
    required this.fileName,
    required this.isUploading,
    required this.progress,
    required this.onTap,
    required this.onRemove,
  });

  final String? fileName;
  final bool isUploading;
  final double progress;
  final VoidCallback onTap;
  final VoidCallback onRemove;

  @override
  Widget build(BuildContext context) {
    // Uploaded state
    if (fileName != null) {
      return Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(10),
          border: Border.all(color: const Color(0xFFE0E0E0)),
        ),
        child: Row(
          children: [
            Container(
              width: 36,
              height: 36,
              decoration: BoxDecoration(
                color: const Color(0xFFE8F5E9),
                borderRadius: BorderRadius.circular(8),
              ),
              child: const Icon(
                Icons.image_outlined,
                size: 18,
                color: Color(0xFF4CAF50),
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                fileName!,
                style: const TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w600,
                  color: Color(0xFF1A1A1A),
                ),
                overflow: TextOverflow.ellipsis,
              ),
            ),
            GestureDetector(
              onTap: onRemove,
              child: const Icon(
                Icons.cancel_outlined,
                size: 20,
                color: Color(0xFF888888),
              ),
            ),
          ],
        ),
      );
    }

    // Uploading state
    if (isUploading) {
      return Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 18),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(10),
          border: Border.all(color: const Color(0xFFE0E0E0)),
        ),
        child: ClipRRect(
          borderRadius: BorderRadius.circular(4),
          child: LinearProgressIndicator(
            value: progress,
            backgroundColor: const Color(0xFFE0E0E0),
            color: const Color(0xFF4CAF50),
            minHeight: 6,
          ),
        ),
      );
    }

    // Default — dashed border
    return GestureDetector(
      onTap: onTap,
      child: CustomPaint(
        painter: _DashedBorderPainter(
          color: const Color(0xFFCCCCCC),
          borderRadius: 10,
          dashWidth: 6,
          dashSpace: 4,
        ),
        child: Container(
          width: double.infinity,
          padding: const EdgeInsets.symmetric(vertical: 24),
          decoration: BoxDecoration(
            color: Colors.white,
            borderRadius: BorderRadius.circular(10),
          ),
          child: const Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(Icons.upload_rounded, size: 28, color: Color(0xFF4CAF50)),
              SizedBox(height: 8),
              Text(
                'Upload ID',
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF1A1A1A),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

// ── Dashed border painter ─────────────────────────────────────────────────────

class _DashedBorderPainter extends CustomPainter {
  const _DashedBorderPainter({
    required this.color,
    required this.borderRadius,
    required this.dashWidth,
    required this.dashSpace,
  });

  final Color color;
  final double borderRadius;
  final double dashWidth;
  final double dashSpace;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 1.5
      ..style = PaintingStyle.stroke;

    final path = Path()
      ..addRRect(RRect.fromRectAndRadius(
        Rect.fromLTWH(0, 0, size.width, size.height),
        Radius.circular(borderRadius),
      ));

    final dashPath = Path();
    for (final metric in path.computeMetrics()) {
      double distance = 0;
      while (distance < metric.length) {
        dashPath.addPath(
          metric.extractPath(distance, distance + dashWidth),
          Offset.zero,
        );
        distance += dashWidth + dashSpace;
      }
    }
    canvas.drawPath(dashPath, paint);
  }

  @override
  bool shouldRepaint(_DashedBorderPainter old) =>
      old.color != color || old.dashWidth != dashWidth || old.dashSpace != dashSpace;
}

// ── Nigeria flag ──────────────────────────────────────────────────────────────

class _NigeriaFlag extends StatelessWidget {
  const _NigeriaFlag();

  @override
  Widget build(BuildContext context) {
    return ClipRRect(
      borderRadius: BorderRadius.circular(2),
      child: const Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          _Stripe(color: Color(0xFF008751)),
          _Stripe(color: Colors.white),
          _Stripe(color: Color(0xFF008751)),
        ],
      ),
    );
  }
}

class _Stripe extends StatelessWidget {
  const _Stripe({required this.color});
  final Color color;

  @override
  Widget build(BuildContext context) =>
      Container(width: 7, height: 16, color: color);
}