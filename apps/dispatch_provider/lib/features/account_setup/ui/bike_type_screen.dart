import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

class BikeTypeData {
  const BikeTypeData({
    required this.licenseNo,
    required this.expiryYear,
    required this.expiryDate,
    required this.bikeType,
    required this.bikeBrand,
    required this.color,
    required this.plateNumber,
  });

  final String licenseNo;
  final String expiryYear;
  final String expiryDate;
  final String bikeType;
  final String bikeBrand;
  final String color;
  final String plateNumber;
}

class BikeTypeScreen extends StatefulWidget {
  const BikeTypeScreen({
    super.key,
    required this.onContinue,
    required this.onBack,
    required this.currentStep,
    required this.totalSteps,
  });

  final ValueChanged<String> onContinue;
  final VoidCallback onBack;
  final int currentStep;
  final int totalSteps;

  @override
  State<BikeTypeScreen> createState() => _BikeTypeScreenState();
}

class _BikeTypeScreenState extends State<BikeTypeScreen> {
  final _licenseNoController = TextEditingController();
  final _bikeBrandController = TextEditingController();
  final _colorController = TextEditingController();
  final _plateNumberController = TextEditingController();

  String? _expiryYear;
  DateTime? _expiryDate;
  String? _bikeType;

  static const _bikeTypes = [
    'Motorcycle',
    'Scooter',
    'Tricycle',
    'Bicycle',
    'Electric Bike',
  ];

  static final _years = List.generate(
    20,
    (i) => (DateTime.now().year + i).toString(),
  );

  bool get _canContinue =>
      _licenseNoController.text.trim().isNotEmpty &&
      _expiryYear != null &&
      _expiryDate != null &&
      _bikeType != null &&
      _bikeBrandController.text.trim().isNotEmpty &&
      _colorController.text.trim().isNotEmpty &&
      _plateNumberController.text.trim().isNotEmpty;

  @override
  void dispose() {
    _licenseNoController.dispose();
    _bikeBrandController.dispose();
    _colorController.dispose();
    _plateNumberController.dispose();
    super.dispose();
  }

  Future<void> _pickDate() async {
    final now = DateTime.now();
    final picked = await showDatePicker(
      context: context,
      initialDate: now,
      firstDate: now,
      lastDate: DateTime(now.year + 20),
      builder: (context, child) => Theme(
        data: Theme.of(context).copyWith(
          colorScheme: const ColorScheme.light(
            primary: Color(0xFF4CAF50),
          ),
        ),
        child: child!,
      ),
    );
    if (picked != null) setState(() => _expiryDate = picked);
  }

  String get _formattedDate {
    if (_expiryDate == null) return '';
    final d = _expiryDate!;
    return '${d.day.toString().padLeft(2, '0')}/${d.month.toString().padLeft(2, '0')}/${d.year}';
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
                      'Add your Bike details',
                      style: TextStyle(
                        fontSize: 22,
                        fontWeight: FontWeight.w800,
                        color: Color(0xFF1A1A1A),
                        letterSpacing: -0.3,
                      ),
                    ),
                    const SizedBox(height: 6),
                    const Text(
                      'Provide your bike details to start delivering packages.',
                      style: TextStyle(fontSize: 12, color: Color(0xFF888888)),
                    ),
                    const SizedBox(height: 28),

                    // License No
                    const _FieldLabel(label: "Your Driver's license no"),
                    const SizedBox(height: 8),
                    _InputField(
                      controller: _licenseNoController,
                      hint: '1234567890',
                      keyboardType: TextInputType.text,
                      onChanged: (_) => setState(() {}),
                    ),
                    const SizedBox(height: 16),

                    // Expiry Year + Expiry Date
                    Row(
                      children: [
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              const _FieldLabel(label: 'Expiry Year'),
                              const SizedBox(height: 8),
                              _DropdownField(
                                value: _expiryYear,
                                hint: 'Select Year',
                                items: _years,
                                onChanged: (v) =>
                                    setState(() => _expiryYear = v),
                              ),
                            ],
                          ),
                        ),
                        const SizedBox(width: 12),
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              const _FieldLabel(label: 'Expiry Date'),
                              const SizedBox(height: 8),
                              GestureDetector(
                                onTap: _pickDate,
                                child: Container(
                                  height: 50,
                                  padding: const EdgeInsets.symmetric(
                                      horizontal: 14),
                                  decoration: BoxDecoration(
                                    color: Colors.white,
                                    borderRadius: BorderRadius.circular(10),
                                    border: Border.all(
                                        color: const Color(0xFFE0E0E0)),
                                  ),
                                  child: Row(
                                    children: [
                                      Expanded(
                                        child: Text(
                                          _expiryDate == null
                                              ? 'Select Expiry Date'
                                              : _formattedDate,
                                          style: TextStyle(
                                            fontSize: 13,
                                            color: _expiryDate == null
                                                ? const Color(0xFFBBBBBB)
                                                : const Color(0xFF1A1A1A),
                                          ),
                                          overflow: TextOverflow.ellipsis,
                                        ),
                                      ),
                                      const Icon(
                                        Icons.calendar_month_outlined,
                                        size: 18,
                                        color: Color(0xFF888888),
                                      ),
                                    ],
                                  ),
                                ),
                              ),
                            ],
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 16),

                    // Bike Type
                    const _FieldLabel(label: 'Bike Type'),
                    const SizedBox(height: 8),
                    _DropdownField(
                      value: _bikeType,
                      hint: 'Motorcycle',
                      items: _bikeTypes,
                      onChanged: (v) => setState(() => _bikeType = v),
                    ),
                    const SizedBox(height: 16),

                    // Bike Brand
                    const _FieldLabel(label: 'Bike Brand'),
                    const SizedBox(height: 8),
                    _InputField(
                      controller: _bikeBrandController,
                      hint: 'Sedan',
                      onChanged: (_) => setState(() {}),
                    ),
                    const SizedBox(height: 16),

                    // Color
                    const _FieldLabel(label: 'Color'),
                    const SizedBox(height: 8),
                    _InputField(
                      controller: _colorController,
                      hint: 'Enter color',
                      onChanged: (_) => setState(() {}),
                    ),
                    const SizedBox(height: 16),

                    // Plate Number
                    const _FieldLabel(label: 'Plate Number'),
                    const SizedBox(height: 8),
                    _InputField(
                      controller: _plateNumberController,
                      hint: 'AS347654',
                      inputFormatters: [
                        FilteringTextInputFormatter.allow(
                            RegExp(r'[a-zA-Z0-9]')),
                      ],
                      onChanged: (_) => setState(() {}),
                    ),
                    const SizedBox(height: 8),
                  ],
                ),
              ),
            ),

            // Pinned button
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 8, 24, 28),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _canContinue
                      ? () => widget.onContinue(_bikeType!)
                      : null,
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
        fontSize: 14,
        fontWeight: FontWeight.w600,
        color: Color(0xFF1A1A1A),
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
        hintStyle:
            const TextStyle(color: Color(0xFFBBBBBB), fontSize: 14),
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
          borderSide:
              const BorderSide(color: Color(0xFF4CAF50), width: 1.5),
        ),
      ),
    );
  }
}

class _DropdownField extends StatelessWidget {
  const _DropdownField({
    required this.value,
    required this.hint,
    required this.items,
    required this.onChanged,
  });

  final String? value;
  final String hint;
  final List<String> items;
  final ValueChanged<String?> onChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 50,
      padding: const EdgeInsets.symmetric(horizontal: 14),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: const Color(0xFFE0E0E0)),
      ),
      child: DropdownButtonHideUnderline(
        child: DropdownButton<String>(
          value: value,
          hint: Text(
            hint,
            style: const TextStyle(
                color: Color(0xFFBBBBBB), fontSize: 14),
          ),
          isExpanded: true,
          icon: const Icon(
            Icons.keyboard_arrow_down_rounded,
            color: Color(0xFF888888),
          ),
          style: const TextStyle(fontSize: 14, color: Color(0xFF1A1A1A)),
          items: items
              .map((r) => DropdownMenuItem(value: r, child: Text(r)))
              .toList(),
          onChanged: onChanged,
        ),
      ),
    );
  }
}