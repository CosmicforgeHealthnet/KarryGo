// profile details screen
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

class ProfileDetailsScreen extends StatefulWidget {
  const ProfileDetailsScreen({
    super.key,
    required this.onContinue,
    required this.onBack,
    required this.currentStep,
    required this.totalSteps,
  });

  final ValueChanged<ProfileDetailsData> onContinue;
  final VoidCallback onBack;
  final int currentStep;
  final int totalSteps;

  @override
  State<ProfileDetailsScreen> createState() => _ProfileDetailsScreenState();
}

class _ProfileDetailsScreenState extends State<ProfileDetailsScreen> {
  final _nameController = TextEditingController();
  final _phoneController = TextEditingController();
  final _emailController = TextEditingController();
  final _cityController = TextEditingController();
  String? _selectedState;
  String? _uploadedFileName;
  bool _isUploading = false;
  double _uploadProgress = 0;

  static const _states = [
    'Abia', 'Adamawa', 'Akwa Ibom', 'Anambra', 'Bauchi', 'Bayelsa',
    'Benue', 'Borno', 'Cross River', 'Delta', 'Ebonyi', 'Edo',
    'Ekiti', 'Enugu', 'FCT', 'Gombe', 'Imo', 'Jigawa', 'Kaduna',
    'Kano', 'Katsina', 'Kebbi', 'Kogi', 'Kwara', 'Lagos', 'Nasarawa',
    'Niger', 'Ogun', 'Ondo', 'Osun', 'Oyo', 'Plateau', 'Rivers',
    'Sokoto', 'Taraba', 'Yobe', 'Zamfara',
  ];

  @override
  void dispose() {
    _nameController.dispose();
    _phoneController.dispose();
    _emailController.dispose();
    _cityController.dispose();
    super.dispose();
  }

  bool get _canContinue =>
      _nameController.text.trim().isNotEmpty &&
      _emailController.text.trim().isNotEmpty &&
      _uploadedFileName != null;

  void _simulateUpload() async {
    setState(() {
      _isUploading = true;
      _uploadProgress = 0;
      _uploadedFileName = null;
    });
    for (int i = 1; i <= 10; i++) {
      await Future.delayed(const Duration(milliseconds: 120));
      if (!mounted) return;
      setState(() => _uploadProgress = i / 10);
    }
    setState(() {
      _isUploading = false;
      _uploadedFileName = 'IMG_20250720_174356.jpg';
    });
  }

  void _continue() {
    widget.onContinue(ProfileDetailsData(
      fullName: _nameController.text.trim(),
      phone: '+234${_phoneController.text.trim()}',
      state: _selectedState ?? '',
      city: _cityController.text.trim(),
      email: _emailController.text.trim(),
      governmentIdFileName: _uploadedFileName ?? '',
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
                      'Tell us about you',
                      style: TextStyle(
                        fontSize: 22,
                        fontWeight: FontWeight.w800,
                        color: Color(0xFF1A1A1A),
                        letterSpacing: -0.3,
                      ),
                    ),
                    const SizedBox(height: 8),
                    const Text(
                      "Let's setup your account so we can connect you with the right opportunities.",
                      style: TextStyle(
                        fontSize: 13,
                        color: Color(0xFF888888),
                        height: 1.5,
                      ),
                    ),
                    const SizedBox(height: 32),

                    // Full Name
                    const _FieldLabel(label: 'Your Full Name'),
                    const SizedBox(height: 8),
                    _InputField(
                      controller: _nameController,
                      hint: 'Demaulux_1',
                      onChanged: (_) => setState(() {}),
                    ),
                    const SizedBox(height: 20),

                    // Phone
                    const _FieldLabel(label: 'Your Phone Number'),
                    const SizedBox(height: 8),
                    Row(
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
                            controller: _phoneController,
                            hint: '8067735987',
                            keyboardType: TextInputType.phone,
                            inputFormatters: [
                              FilteringTextInputFormatter.digitsOnly,
                            ],
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 20),

                    // Location
                    const _FieldLabel(label: 'Location'),
                    const SizedBox(height: 8),
                    _StateDropdown(
                      value: _selectedState,
                      states: _states,
                      onChanged: (v) => setState(() => _selectedState = v),
                    ),
                    const SizedBox(height: 10),
                    _InputField(
                      controller: _cityController,
                      hint: 'City',
                    ),
                    const SizedBox(height: 20),

                    // Email
                    const _FieldLabel(label: 'Email'),
                    const SizedBox(height: 8),
                    _InputField(
                      controller: _emailController,
                      hint: 'uiuxwithdema@gmail.com',
                      keyboardType: TextInputType.emailAddress,
                      onChanged: (_) => setState(() {}),
                    ),
                    const SizedBox(height: 20),

                    // Upload
                    const _FieldLabel(label: 'Upload Approved Government ID'),
                    const SizedBox(height: 8),
                    _UploadBox(
                      fileName: _uploadedFileName,
                      isUploading: _isUploading,
                      progress: _uploadProgress,
                      onTap: _simulateUpload,
                      onRemove: () => setState(() {
                        _uploadedFileName = null;
                        _uploadProgress = 0;
                      }),
                    ),
                    const SizedBox(height: 10),
                    const Text(
                      'International passport, NIN, Voter\'s card or Driver\'s License\nSupported file type: JPG, PNG\nMaximum File Size: 500MB',
                      style: TextStyle(
                        fontSize: 11,
                        color: Color(0xFFAAAAAA),
                        height: 1.6,
                      ),
                    ),
                    const SizedBox(height: 8),
                  ],
                ),
              ),
            ),

            // Continue button — pinned to bottom
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 8, 24, 28),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _canContinue ? _continue : null,
                  style: FilledButton.styleFrom(
                    backgroundColor: const Color(0xFF4CAF50),
                    disabledBackgroundColor:
                        const Color(0xFF4CAF50).withValues(alpha: 0.4),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(999),
                    ),
                  ),
                  child: const Text(
                    'Continue',
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

// ─── Data Model ──────────────────────────────────────────────────────────────

class ProfileDetailsData {
  const ProfileDetailsData({
    required this.fullName,
    required this.phone,
    required this.state,
    required this.city,
    required this.email,
    required this.governmentIdFileName,
  });

  final String fullName;
  final String phone;
  final String state;
  final String city;
  final String email;
  final String governmentIdFileName;
}

// ─── Field Label ─────────────────────────────────────────────────────────────

class _FieldLabel extends StatelessWidget {
  const _FieldLabel({required this.label});
  final String label;

  @override
  Widget build(BuildContext context) {
    return Text(
      label,
      style: const TextStyle(
        fontSize: 14,
        fontWeight: FontWeight.w600,
        color: Color(0xFF1A1A1A),
      ),
    );
  }
}

// ─── Input Field ─────────────────────────────────────────────────────────────

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
        hintStyle: const TextStyle(
          fontSize: 14,
          color: Color(0xFFBBBBBB),
        ),
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

// ─── State Dropdown ───────────────────────────────────────────────────────────

class _StateDropdown extends StatelessWidget {
  const _StateDropdown({
    required this.value,
    required this.states,
    required this.onChanged,
  });

  final String? value;
  final List<String> states;
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
            'State',
            style: TextStyle(color: Color(0xFFBBBBBB), fontSize: 14),
          ),
          isExpanded: true,
          icon: const Icon(
            Icons.keyboard_arrow_down_rounded,
            color: Color(0xFF888888),
          ),
          style: const TextStyle(fontSize: 14, color: Color(0xFF1A1A1A)),
          items: states
              .map((s) => DropdownMenuItem(value: s, child: Text(s)))
              .toList(),
          onChanged: onChanged,
        ),
      ),
    );
  }
}

// ─── Upload Box ───────────────────────────────────────────────────────────────

class _UploadBox extends StatelessWidget {
  const _UploadBox({
    required this.onTap,
    required this.onRemove,
    this.fileName,
    this.isUploading = false,
    this.progress = 0,
  });

  final VoidCallback onTap;
  final VoidCallback onRemove;
  final String? fileName;
  final bool isUploading;
  final double progress;

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

    // Uploading / progress state
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

    // Default — dashed border upload prompt (matches Figma)
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
              Icon(
                Icons.upload_rounded,
                size: 30,
                color: Color(0xFF4CAF50),
              ),
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

// ─── Dashed Border Painter ────────────────────────────────────────────────────

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
    final metrics = path.computeMetrics();
    for (final metric in metrics) {
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
  bool shouldRepaint(_DashedBorderPainter oldDelegate) =>
      oldDelegate.color != color ||
      oldDelegate.dashWidth != dashWidth ||
      oldDelegate.dashSpace != dashSpace;
}

// ─── Nigeria Flag ─────────────────────────────────────────────────────────────

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

// ─── Progress Bar ─────────────────────────────────────────────────────────────

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