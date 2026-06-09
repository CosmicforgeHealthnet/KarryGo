import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../profile/state/provider_profile_controller.dart';
import '../../../shared/widgets/document_upload_box.dart';

// ── Data model ──────────────────────────────────────────────────────────────

class DriverInformationData {
  const DriverInformationData({
    required this.licenseFileName,
    required this.licenseFilePath,
    required this.vehicleRegFileName,
    required this.vehicleRegFilePath,
    required this.guarantorName,
    required this.guarantorPhone,
    required this.emergencyName,
    required this.emergencyPhone,
    required this.relationshipType,
  });

  final String licenseFileName;
  /// Local file path.
  /// TODO: POST /api/v1/provider/verification/licence
  ///   fields: licence_number, expiry_year, expiry_month, licence_file
  final String licenseFilePath;
  final String vehicleRegFileName;
  /// Local file path.
  /// TODO: POST /api/v1/provider/vehicle/:id/documents
  ///   fields: document_type, expiry_date (optional), document_file
  final String vehicleRegFilePath;
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
    required this.profileController,
  });

  final Future<void> Function(DriverInformationData) onSubmit;
  final VoidCallback onBack;
  final int currentStep;
  final int totalSteps;
  final ProviderProfileController profileController;

  @override
  State<DriverInformationScreen> createState() =>
      _DriverInformationScreenState();
}

class _DriverInformationScreenState extends State<DriverInformationScreen> {
  String? _licenseFileName;
  String? _licenseFilePath;

  String? _vehicleRegFileName;
  String? _vehicleRegFilePath;

  final _guarantorNameController = TextEditingController();
  final _guarantorPhoneController = TextEditingController();
  final _emergencyNameController = TextEditingController();
  final _emergencyPhoneController = TextEditingController();
  String? _relationshipType;
  bool _isLoading = false;

  static const _relationships = [
    'Parent', 'Sibling', 'Spouse', 'Friend', 'Child', 'Other',
  ];

  @override
  void initState() {
    super.initState();
    _loadExistingData();
  }

  void _loadExistingData() async {
    setState(() => _isLoading = true);

    // 1. Fetch guarantor
    final guarantorRes = await widget.profileController.loadGuarantor();
    guarantorRes.when(
      success: (data) {
        if (mounted) {
          setState(() {
            _guarantorNameController.text = data.fullName;
            String phone = data.phone;
            if (phone.startsWith('+234')) {
              phone = phone.substring(4);
            }
            _guarantorPhoneController.text = phone;
          });
        }
      },
      failure: (_) {}, // Ignore not found / error
    );

    // 2. Fetch emergency contact
    final contactRes = await widget.profileController.loadEmergencyContact();
    contactRes.when(
      success: (data) {
        if (mounted) {
          setState(() {
            _emergencyNameController.text = data.fullName;
            String phone = data.phone;
            if (phone.startsWith('+234')) {
              phone = phone.substring(4);
            }
            _emergencyPhoneController.text = phone;
            if (_relationships.contains(data.relationship)) {
              _relationshipType = data.relationship;
            } else {
              _relationshipType = 'Other';
            }
          });
        }
      },
      failure: (_) {}, // Ignore not found / error
    );

    if (mounted) {
      setState(() => _isLoading = false);
    }
  }

  @override
  void dispose() {
    _guarantorNameController.dispose();
    _guarantorPhoneController.dispose();
    _emergencyNameController.dispose();
    _emergencyPhoneController.dispose();
    super.dispose();
  }

  bool get _canSubmit =>
      _licenseFilePath != null &&
      _vehicleRegFilePath != null &&
      _guarantorNameController.text.trim().isNotEmpty &&
      _guarantorPhoneController.text.trim().isNotEmpty &&
      _emergencyNameController.text.trim().isNotEmpty &&
      _emergencyPhoneController.text.trim().isNotEmpty &&
      _relationshipType != null;

  // TODO: backend upload — when submitting, send licence file to:
  //   POST /api/v1/provider/verification/licence
  //   fields: licence_number, expiry_year, expiry_month, licence_file
  //   Use _licenseFilePath as the local file path.

  // TODO: backend upload — when submitting, send vehicle reg to:
  //   POST /api/v1/provider/vehicle/:id/documents
  //   fields: document_type, expiry_date (optional), document_file
  //   Use _vehicleRegFilePath as the local file path.

  void _submit() async {
    // ── Validate guarantor full name (at least two parts) ─────────────────
    final gName = _guarantorNameController.text.trim();
    if (gName.split(RegExp(r'\s+')).where((p) => p.isNotEmpty).length < 2) {
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
        content: Text("Please enter your guarantor's full name."),
        backgroundColor: Colors.red,
      ));
      return;
    }
    // ── Validate emergency contact full name ──────────────────────────────
    final eName = _emergencyNameController.text.trim();
    if (eName.split(RegExp(r'\s+')).where((p) => p.isNotEmpty).length < 2) {
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
        content: Text("Please enter your emergency contact's full name."),
        backgroundColor: Colors.red,
      ));
      return;
    }

    String gPhone = _guarantorPhoneController.text.trim();
    while (gPhone.startsWith('0')) {
      gPhone = gPhone.substring(1);
    }
    if (gPhone.startsWith('234')) {
      gPhone = gPhone.substring(3);
    }
    if (gPhone.startsWith('+234')) {
      gPhone = gPhone.substring(4);
    }
    final guarantorPhoneE164 = '+234$gPhone';

    String ePhone = _emergencyPhoneController.text.trim();
    while (ePhone.startsWith('0')) {
      ePhone = ePhone.substring(1);
    }
    if (ePhone.startsWith('234')) {
      ePhone = ePhone.substring(3);
    }
    if (ePhone.startsWith('+234')) {
      ePhone = ePhone.substring(4);
    }
    final emergencyPhoneE164 = '+234$ePhone';

    setState(() => _isLoading = true);

    // 1. Submit Guarantor
    final guarantorRes = await widget.profileController.saveGuarantor(
      fullName: _guarantorNameController.text.trim(),
      phone: guarantorPhoneE164,
    );

    bool hasError = false;
    String errMsg = '';

    await guarantorRes.when(
      success: (_) async {
        // 2. Submit Emergency Contact
        final emergencyRes = await widget.profileController.saveEmergencyContact(
          fullName: _emergencyNameController.text.trim(),
          phone: emergencyPhoneE164,
          relationship: _relationshipType!,
        );
        emergencyRes.when(
          success: (_) {},
          failure: (error) {
            hasError = true;
            errMsg = 'Emergency Contact failed: ${error.message}';
          },
        );
      },
      failure: (error) {
        hasError = true;
        errMsg = 'Guarantor failed: ${error.message}';
      },
    );

    if (!mounted) return;

    if (hasError) {
      setState(() => _isLoading = false);
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(errMsg),
          backgroundColor: Colors.red.shade800,
        ),
      );
    } else {
      try {
        await widget.onSubmit(DriverInformationData(
          licenseFileName: _licenseFileName ?? '',
          licenseFilePath: _licenseFilePath ?? '',
          vehicleRegFileName: _vehicleRegFileName ?? '',
          vehicleRegFilePath: _vehicleRegFilePath ?? '',
          guarantorName: _guarantorNameController.text.trim(),
          guarantorPhone: guarantorPhoneE164,
          emergencyName: _emergencyNameController.text.trim(),
          emergencyPhone: emergencyPhoneE164,
          relationshipType: _relationshipType!,
        ));
      } catch (e) {
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Text('Submission failed: $e'),
              backgroundColor: Colors.red.shade800,
            ),
          );
        }
      } finally {
        if (mounted) {
          setState(() => _isLoading = false);
        }
      }
    }
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

                    // ── Driver's License upload (real picker)
                    const _FieldLabel(label: "Upload Driver's License"),
                    const SizedBox(height: 8),
                    DocumentUploadBox(
                      filePath: _licenseFilePath,
                      fileName: _licenseFileName,
                      hint: 'Supported file type: JPG, PNG, PDF\nMaximum File Size: 500 MB',
                      onFileSelected: (path, name) {
                        setState(() {
                          _licenseFilePath = path;
                          _licenseFileName = name;
                        });
                      },
                      onClear: () => setState(() {
                        _licenseFilePath = null;
                        _licenseFileName = null;
                      }),
                    ),
                    const SizedBox(height: 20),

                    // ── Vehicle Registration upload (real picker)
                    const _FieldLabel(label: 'Upload Vehicle Registration Sticker'),
                    const SizedBox(height: 8),
                    DocumentUploadBox(
                      filePath: _vehicleRegFilePath,
                      fileName: _vehicleRegFileName,
                      hint: 'Supported file type: JPG, PNG, PDF\nMaximum File Size: 500 MB',
                      onFileSelected: (path, name) {
                        setState(() {
                          _vehicleRegFilePath = path;
                          _vehicleRegFileName = name;
                        });
                      },
                      onClear: () => setState(() {
                        _vehicleRegFilePath = null;
                        _vehicleRegFileName = null;
                      }),
                    ),
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
                      hint: 'Enter full name',
                      keyboardType: TextInputType.name,
                      inputFormatters: [
                        FilteringTextInputFormatter.allow(
                          RegExp(r"[a-zA-Z '\-]"),
                        ),
                      ],
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
                      hint: 'Enter full name',
                      keyboardType: TextInputType.name,
                      inputFormatters: [
                        FilteringTextInputFormatter.allow(
                          RegExp(r"[a-zA-Z '\-]"),
                        ),
                      ],
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
                  onPressed: _canSubmit && !_isLoading ? _submit : null,
                  style: FilledButton.styleFrom(
                    backgroundColor: const Color(0xFF4CAF50),
                    disabledBackgroundColor:
                        const Color(0xFF4CAF50).withValues(alpha: 0.4),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(999),
                    ),
                  ),
                  child: _isLoading
                      ? const SizedBox(
                          width: 20,
                          height: 20,
                          child: CircularProgressIndicator(
                            color: Colors.white,
                            strokeWidth: 2.5,
                          ),
                        )
                      : const Text(
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