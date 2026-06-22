import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../../../shared/widgets/figma_customer_widgets.dart';
import '../../auth/models/customer_auth_models.dart';
import '../data/support_api.dart';
import '../models/support_models.dart';

class NewComplaintScreen extends StatefulWidget {
  const NewComplaintScreen({
    super.key,
    required this.session,
    required this.supportApi,
  });

  final CustomerSession session;
  final SupportApi supportApi;

  @override
  State<NewComplaintScreen> createState() => _NewComplaintScreenState();
}

class _NewComplaintScreenState extends State<NewComplaintScreen> {
  final _subjectCtrl = TextEditingController();
  final _descriptionCtrl = TextEditingController();
  final _bookingRefCtrl = TextEditingController();

  String _serviceType = 'taxi';
  bool _submitting = false;
  ApiException? _error;

  @override
  void dispose() {
    _subjectCtrl.dispose();
    _descriptionCtrl.dispose();
    _bookingRefCtrl.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final subject = _subjectCtrl.text.trim();
    final description = _descriptionCtrl.text.trim();

    if (subject.isEmpty || description.isEmpty) {
      setState(() {
        _error = const ApiException(
          code: ApiErrorCode.validationFailed,
          message: 'Subject and description are required.',
          fields: [],
        );
      });
      return;
    }

    setState(() {
      _submitting = true;
      _error = null;
    });

    try {
      final complaint = await widget.supportApi.createComplaint(
        accessToken: widget.session.accessToken,
        serviceType: _serviceType,
        subject: subject,
        description: description,
        bookingReference: _bookingRefCtrl.text.trim(),
      );
      if (mounted) Navigator.of(context).pop(complaint);
    } on ApiException catch (e) {
      setState(() {
        _error = e;
        _submitting = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      appBar: AppBar(
        backgroundColor: CustomerFigmaColors.surface,
        elevation: 0,
        leading:
            FigmaBackButton(onPressed: () => Navigator.of(context).pop()),
        title: const Text(
          'New Complaint',
          style: TextStyle(
            color: CustomerFigmaColors.text,
            fontWeight: FontWeight.w800,
            fontSize: 18,
          ),
        ),
      ),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(24),
          children: [
            const _SectionLabel('Service type'),
            const SizedBox(height: 12),
            _ServiceTypeSelector(
              value: _serviceType,
              onChanged: (v) => setState(() => _serviceType = v),
            ),
            const SizedBox(height: 24),
            if (_error != null) ...[
              _ErrorBanner(message: _error!.message),
              const SizedBox(height: 16),
            ],
            FigmaTextField(
              controller: _subjectCtrl,
              label: 'Subject',
              hintText: 'Brief summary of the issue',
            ),
            const SizedBox(height: 16),
            _TextAreaField(
              controller: _descriptionCtrl,
              label: 'Description',
              hintText: 'Describe the issue in detail…',
            ),
            const SizedBox(height: 16),
            FigmaTextField(
              controller: _bookingRefCtrl,
              label: 'Booking reference (optional)',
              hintText: 'e.g. TRIP-123456',
            ),
            const SizedBox(height: 32),
            FigmaPrimaryButton(
              label: 'Submit complaint',
              isLoading: _submitting,
              onPressed: _submitting ? null : _submit,
            ),
          ],
        ),
      ),
    );
  }
}

class _ServiceTypeSelector extends StatelessWidget {
  const _ServiceTypeSelector({required this.value, required this.onChanged});

  final String value;
  final ValueChanged<String> onChanged;

  static const _options = [
    ('taxi', 'Taxi ride', Icons.directions_car_filled_rounded),
    ('dispatch', 'Dispatch delivery', Icons.inventory_2_rounded),
    ('hauling', 'Truck haulage', Icons.local_shipping_rounded),
    ('platform', 'Platform / Account', Icons.support_agent_rounded),
  ];

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: _options.map((opt) {
        final (id, label, icon) = opt;
        final selected = value == id;
        return GestureDetector(
          onTap: () => onChanged(id),
          child: AnimatedContainer(
            duration: const Duration(milliseconds: 150),
            padding:
                const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
            decoration: BoxDecoration(
              color: selected
                  ? CustomerFigmaColors.primary
                  : Colors.white,
              borderRadius: BorderRadius.circular(99),
              border: Border.all(
                color: selected
                    ? CustomerFigmaColors.primary
                    : CustomerFigmaColors.border,
              ),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(icon,
                    size: 16,
                    color: selected
                        ? Colors.white
                        : CustomerFigmaColors.muted),
                const SizedBox(width: 6),
                Text(
                  label,
                  style: TextStyle(
                    color: selected ? Colors.white : CustomerFigmaColors.text,
                    fontSize: 13,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ],
            ),
          ),
        );
      }).toList(),
    );
  }
}

class _TextAreaField extends StatelessWidget {
  const _TextAreaField({
    required this.controller,
    required this.label,
    this.hintText,
  });

  final TextEditingController controller;
  final String label;
  final String? hintText;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: const TextStyle(
            color: CustomerFigmaColors.text,
            fontSize: 13,
            fontWeight: FontWeight.w800,
          ),
        ),
        const SizedBox(height: 8),
        TextField(
          controller: controller,
          maxLines: 5,
          keyboardType: TextInputType.multiline,
          decoration: InputDecoration(
            hintText: hintText,
            filled: true,
            fillColor: CustomerFigmaColors.field,
            contentPadding: const EdgeInsets.all(16),
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(10),
              borderSide: BorderSide.none,
            ),
            enabledBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(10),
              borderSide: BorderSide.none,
            ),
            focusedBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(10),
              borderSide:
                  const BorderSide(color: CustomerFigmaColors.primary),
            ),
          ),
        ),
      ],
    );
  }
}

class _SectionLabel extends StatelessWidget {
  const _SectionLabel(this.text);
  final String text;

  @override
  Widget build(BuildContext context) {
    return Text(
      text,
      style: const TextStyle(
        color: CustomerFigmaColors.text,
        fontSize: 15,
        fontWeight: FontWeight.w800,
      ),
    );
  }
}

class _ErrorBanner extends StatelessWidget {
  const _ErrorBanner({required this.message});
  final String message;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: const Color(0xFFFFF1F0),
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: const Color(0xFFFFCDD2)),
      ),
      child: Text(
        message,
        style: const TextStyle(color: Color(0xFFC0392B), fontSize: 13),
      ),
    );
  }
}
