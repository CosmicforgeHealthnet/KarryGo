// profile details screen
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../profile/state/provider_profile_controller.dart';
import '../../../shared/widgets/document_upload_box.dart';

class ProfileDetailsScreen extends StatefulWidget {
  const ProfileDetailsScreen({
    super.key,
    required this.onContinue,
    required this.onBack,
    required this.currentStep,
    required this.totalSteps,
    this.initialPhone,
    this.initialEmail,
    required this.profileController,
    required this.operationType,
  });

  final Future<void> Function(ProfileDetailsData) onContinue;
  final VoidCallback onBack;
  final int currentStep;
  final int totalSteps;

  /// Phone number from the OTP-verified signup/login flow. Pre-filled and locked.
  final String? initialPhone;

  /// Email from the signup start step. Pre-filled and locked (signup flow only).
  final String? initialEmail;
  final ProviderProfileController profileController;
  final String? operationType;

  @override
  State<ProfileDetailsScreen> createState() => _ProfileDetailsScreenState();
}

class _ProfileDetailsScreenState extends State<ProfileDetailsScreen> {
  final _nameController = TextEditingController();
  final _phoneController = TextEditingController();
  final _emailController = TextEditingController();
  final _cityController = TextEditingController();
  final _idNumberController = TextEditingController();
  String? _selectedState;
  String? _selectedIdType;
  String? _govtIdFilePath;
  String? _govtIdFileName;
  bool _isLoading = false;

  /// True when the email was supplied by the signup OTP flow; the field is
  /// shown as read-only so the user does not accidentally clear a verified email.
  bool _emailReadOnly = false;

  @override
  void initState() {
    super.initState();
    _loadProfile();
  }

  Future<void> _loadProfile() async {
    // 1. Initialize phone from widget.initialPhone (strip +234 prefix for display).
    String localPhone = widget.initialPhone ?? '';
    if (localPhone.startsWith('+234')) {
      localPhone = localPhone.substring(4);
    } else if (localPhone.startsWith('234')) {
      localPhone = localPhone.substring(3);
    }
    while (localPhone.startsWith('0')) {
      localPhone = localPhone.substring(1);
    }
    _phoneController.text = localPhone;

    // 2. Prefill email from signup flow — lock it as read-only because the
    //    user verified ownership via OTP.
    if (widget.initialEmail != null && widget.initialEmail!.isNotEmpty) {
      _emailController.text = widget.initialEmail!;
      _emailReadOnly = true;
    }

    // 3. Fetch authenticated profile from backend.
    setState(() => _isLoading = true);
    final result = await widget.profileController.loadMe();
    result.when(
      success: (profile) {
        if (mounted) {
          setState(() {
            _nameController.text = profile.fullName ?? '';
            _cityController.text = profile.city ?? '';
            if (profile.state != null && profile.state!.isNotEmpty) {
              _selectedState = profile.state;
            }
            // Populate phone from profile when initialPhone was not provided
            // (e.g. the user logged in via email — currentPhoneNumber is null).
            if (_phoneController.text.isEmpty && profile.phone.isNotEmpty) {
              String p = profile.phone;
              if (p.startsWith('+234')) {
                p = p.substring(4);
              } else if (p.startsWith('234')) {
                p = p.substring(3);
              }
              while (p.startsWith('0')) {
                p = p.substring(1);
              }
              _phoneController.text = p;
            }
            // Only overwrite email with backend value if no signup email was
            // provided (login path where the user didn't just sign up).
            if (!_emailReadOnly) {
              _emailController.text = profile.email ?? '';
            }
          });
        }
      },
      failure: (error) {
        debugPrint(
          'GET /provider/me failed or profile does not exist yet: ${error.message}',
        );
      },
    );
    if (mounted) {
      setState(() => _isLoading = false);
    }
  }

  static const _states = [
    'Abia',
    'Adamawa',
    'Akwa Ibom',
    'Anambra',
    'Bauchi',
    'Bayelsa',
    'Benue',
    'Borno',
    'Cross River',
    'Delta',
    'Ebonyi',
    'Edo',
    'Ekiti',
    'Enugu',
    'FCT',
    'Gombe',
    'Imo',
    'Jigawa',
    'Kaduna',
    'Kano',
    'Katsina',
    'Kebbi',
    'Kogi',
    'Kwara',
    'Lagos',
    'Nasarawa',
    'Niger',
    'Ogun',
    'Ondo',
    'Osun',
    'Oyo',
    'Plateau',
    'Rivers',
    'Sokoto',
    'Taraba',
    'Yobe',
    'Zamfara',
  ];

  static const _idTypes = [
    'NIN',
    'Driver\'s Licence',
    'Voter\'s Card',
    'International Passport',
  ];

  @override
  void dispose() {
    _nameController.dispose();
    _phoneController.dispose();
    _emailController.dispose();
    _cityController.dispose();
    _idNumberController.dispose();
    super.dispose();
  }

  bool get _canContinue =>
      _nameController.text.trim().isNotEmpty &&
      _emailController.text.trim().isNotEmpty &&
      _selectedIdType != null &&
      _idNumberController.text.trim().isNotEmpty &&
      _govtIdFilePath != null;

  // TODO: backend upload — when submitting, send govt ID file to:
  //   POST /api/v1/provider/verification/identity
  //   multipart fields: govt_id_type, govt_id_number, govt_id_file, profile_photo
  //   Use _govtIdFilePath as the local file path.

  String _mapIdTypeToBackend(String uiType) {
    return switch (uiType) {
      'NIN' => 'nin',
      'Driver\'s Licence' => 'drivers_licence',
      'Voter\'s Card' => 'voter_card',
      'International Passport' => 'passport',
      _ => 'nin',
    };
  }

  static bool _isValidEmail(String email) =>
      RegExp(r'^[\w.+\-]+@[\w\-]+\.[\w.\-]+$').hasMatch(email);

  void _continue() async {
    // ── Validate full name: must contain at least two non-empty parts ──────
    final name = _nameController.text.trim();
    final nameParts = name
        .split(RegExp(r'\s+'))
        .where((p) => p.isNotEmpty)
        .toList();
    if (nameParts.length < 2) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Please enter your full name.'),
          backgroundColor: Colors.red,
        ),
      );
      return;
    }

    // ── Validate email format ─────────────────────────────────────────────
    final email = _emailController.text.trim();
    if (email.isNotEmpty && !_isValidEmail(email)) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Please enter a valid email address.'),
          backgroundColor: Colors.red,
        ),
      );
      return;
    }

    setState(() => _isLoading = true);
    try {
      debugPrint('[ONBOARDING] Submitting POST /provider/onboarding...');
      final result = await widget.profileController.submitOnboarding(
        fullName: _nameController.text.trim(),
        email: _emailController.text.trim().isEmpty
            ? null
            : _emailController.text.trim(),
        state: _selectedState ?? '',
        city: _cityController.text.trim(),
        operationType: widget.operationType ?? 'individual',
      );

      bool onboardingReady = false;
      String? onboardingError;
      result.when(
        success: (_) {
          debugPrint('[ONBOARDING] POST /provider/onboarding succeeded');
          onboardingReady = true;
        },
        failure: (error) {
          debugPrint(
            '[ONBOARDING] POST /provider/onboarding result: ${error.message} (code=${error.code})',
          );
          if (error.code == ApiErrorCode.conflict) {
            // Onboarding was already completed in a prior session — proceed.
            debugPrint(
              '[ONBOARDING] Already completed — proceeding to verification',
            );
            onboardingReady = true;
          } else {
            onboardingError = error.message;
          }
        },
      );

      if (!onboardingReady) {
        if (mounted && onboardingError != null) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Text('Onboarding failed: $onboardingError'),
              backgroundColor: Colors.red.shade800,
            ),
          );
        }
        return;
      }

      final data = ProfileDetailsData(
        fullName: _nameController.text.trim(),
        phone: '+234${_phoneController.text.trim()}',
        state: _selectedState ?? '',
        city: _cityController.text.trim(),
        email: _emailController.text.trim(),
        governmentIdFileName: _govtIdFileName ?? '',
        governmentIdFilePath: _govtIdFilePath ?? '',
        governmentIdType: _mapIdTypeToBackend(_selectedIdType!),
        governmentIdNumber: _idNumberController.text.trim(),
      );

      // Spinner stays on while parent polls GET /verification/status.
      debugPrint(
        '[ONBOARDING] Awaiting parent onContinue (verification step poll)...',
      );
      await widget.onContinue(data);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('$e'.replaceAll('Exception: ', '')),
            backgroundColor: Colors.red.shade800,
          ),
        );
      }
    } finally {
      if (mounted) setState(() => _isLoading = false);
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
                      hint: 'e.g. Favour Daniel',
                      keyboardType: TextInputType.name,
                      inputFormatters: [
                        // Allow letters, spaces, hyphens, and apostrophes only.
                        FilteringTextInputFormatter.allow(
                          RegExp(r"[a-zA-Z '\-]"),
                        ),
                      ],
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
                            readOnly: true,
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
                      keyboardType: TextInputType.text,
                      inputFormatters: [
                        FilteringTextInputFormatter.allow(
                          RegExp(r"[a-zA-Z '\-]"),
                        ),
                      ],
                    ),
                    const SizedBox(height: 20),

                    // Email
                    const _FieldLabel(label: 'Email'),
                    const SizedBox(height: 8),
                    _InputField(
                      controller: _emailController,
                      hint: 'you@example.com',
                      keyboardType: TextInputType.emailAddress,
                      inputFormatters: [
                        FilteringTextInputFormatter.deny(RegExp(r'\s')),
                      ],
                      onChanged: (_) => setState(() {}),
                      readOnly: _emailReadOnly,
                    ),
                    const SizedBox(height: 20),

                    // Government ID Type
                    const _FieldLabel(label: 'Government ID Type'),
                    const SizedBox(height: 8),
                    _IdTypeDropdown(
                      value: _selectedIdType,
                      items: _idTypes,
                      onChanged: (v) => setState(() => _selectedIdType = v),
                    ),
                    const SizedBox(height: 20),

                    // Government ID Number
                    const _FieldLabel(label: 'Government ID Number'),
                    const SizedBox(height: 8),
                    _InputField(
                      controller: _idNumberController,
                      hint: 'Enter ID Number',
                      keyboardType: TextInputType.text,
                      inputFormatters: [
                        // Allow alphanumeric, hyphens, and slashes (common in IDs).
                        FilteringTextInputFormatter.allow(
                          RegExp(r'[a-zA-Z0-9\-\/]'),
                        ),
                      ],
                      onChanged: (_) => setState(() {}),
                    ),
                    const SizedBox(height: 20),

                    // Upload Govt ID — real picker (camera/gallery/PDF)
                    const _FieldLabel(label: 'Upload Approved Government ID'),
                    const SizedBox(height: 8),
                    DocumentUploadBox(
                      filePath: _govtIdFilePath,
                      fileName: _govtIdFileName,
                      hint:
                          'International passport, NIN, Voter\'s card or Driver\'s License\nSupported file type: JPG, PNG, PDF\nMaximum File Size: 500 MB',
                      onFileSelected: (path, name) {
                        setState(() {
                          _govtIdFilePath = path;
                          _govtIdFileName = name;
                        });
                      },
                      onClear: () => setState(() {
                        _govtIdFilePath = null;
                        _govtIdFileName = null;
                      }),
                    ),
                    const SizedBox(height: 8),
                  ],
                ),
              ),
            ),

            Padding(
              padding: const EdgeInsets.fromLTRB(24, 8, 24, 28),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _canContinue && !_isLoading ? _continue : null,
                  style: FilledButton.styleFrom(
                    backgroundColor: const Color(0xFF4CAF50),
                    disabledBackgroundColor: const Color(
                      0xFF4CAF50,
                    ).withValues(alpha: 0.4),
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
    required this.governmentIdFilePath,
    required this.governmentIdType,
    required this.governmentIdNumber,
  });

  final String fullName;
  final String phone;
  final String state;
  final String city;
  final String email;
  final String governmentIdFileName;

  /// Local file path for the selected govt ID document.
  /// TODO: use this path when uploading to
  ///   POST /api/v1/provider/verification/identity
  final String governmentIdFilePath;
  final String governmentIdType;
  final String governmentIdNumber;
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
    this.readOnly = false,
  });

  final TextEditingController controller;
  final String hint;
  final TextInputType? keyboardType;
  final List<TextInputFormatter>? inputFormatters;
  final ValueChanged<String>? onChanged;
  final bool readOnly;

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      keyboardType: keyboardType,
      inputFormatters: inputFormatters,
      onChanged: onChanged,
      readOnly: readOnly,
      style: TextStyle(
        fontSize: 14,
        color: readOnly ? const Color(0xFF666666) : const Color(0xFF1A1A1A),
      ),
      decoration: InputDecoration(
        hintText: hint,
        hintStyle: const TextStyle(fontSize: 14, color: Color(0xFFBBBBBB)),
        filled: true,
        // Subtly different background when field is locked (verified data).
        fillColor: readOnly ? const Color(0xFFF0F0F0) : Colors.white,
        contentPadding: const EdgeInsets.symmetric(
          horizontal: 14,
          vertical: 15,
        ),
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

class _IdTypeDropdown extends StatelessWidget {
  const _IdTypeDropdown({
    required this.value,
    required this.items,
    required this.onChanged,
  });

  final String? value;
  final List<String> items;
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
            'Government ID Type',
            style: TextStyle(color: Color(0xFFBBBBBB), fontSize: 14),
          ),
          isExpanded: true,
          icon: const Icon(
            Icons.keyboard_arrow_down_rounded,
            color: Color(0xFF888888),
          ),
          style: const TextStyle(fontSize: 14, color: Color(0xFF1A1A1A)),
          items: items
              .map((s) => DropdownMenuItem(value: s, child: Text(s)))
              .toList(),
          onChanged: onChanged,
        ),
      ),
    );
  }
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
