import 'package:flutter/material.dart';

import '../../../../core/theme/theme_extensions.dart';
import '../../state/rider_dispute_controller.dart';
import 'add_evidence_screen.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 04 — Create Support Case (details step)
// Figma 05 — Add Evidence (attachments step)
// Step indicator: 1 Details · 2 Evidence · 3 Submit
// ═══════════════════════════════════════════════════════════════════════════

class CreateCaseScreen extends StatefulWidget {
  const CreateCaseScreen({
    super.key,
    required this.disputeController,
    this.initialCategory,
  });

  final RiderDisputeController disputeController;
  final String? initialCategory;

  @override
  State<CreateCaseScreen> createState() => _CreateCaseScreenState();
}

class _CreateCaseScreenState extends State<CreateCaseScreen> {
  final _subjectController = TextEditingController();
  final _detailsController = TextEditingController();
  String? _category;
  List<String> _evidencePaths = [];

  static const _categories = [
    'Delivery issue',
    'Payment enquiry',
    'Account verification',
    'App issue',
    'Safety concern',
    'Other',
  ];

  @override
  void initState() {
    super.initState();
    if (widget.initialCategory != null &&
        _categories.contains(widget.initialCategory)) {
      _category = widget.initialCategory;
    }
  }

  @override
  void dispose() {
    _subjectController.dispose();
    _detailsController.dispose();
    super.dispose();
  }

  bool get _valid =>
      _category != null &&
      _subjectController.text.trim().isNotEmpty &&
      _detailsController.text.trim().isNotEmpty;

  Future<void> _openEvidence() async {
    final r = await Navigator.of(context).push<List<String>>(
      MaterialPageRoute(
        builder: (_) => AddEvidenceScreen(
          initialPaths: _evidencePaths,
        ),
      ),
    );
    if (!mounted || r == null) return;
    setState(() => _evidencePaths = r);
    _submit();
  }

  Future<void> _submit() async {
    final trip = widget.disputeController.selectedTrip;
    if (trip == null) {
      // No trip context — build a minimal Complaint through the mock path.
      // For live, we fall through to the same API call with an empty reference.
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Please attach a trip before submitting.'),
        ),
      );
      return;
    }

    final result = await widget.disputeController.submitDispute();
    if (!mounted) return;
    if (result == null) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            widget.disputeController.submitError ?? 'Submission failed.',
          ),
        ),
      );
      return;
    }
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Support case created')),
    );
    Navigator.of(context).pop(true);
  }

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
                  const SizedBox(width: 4),
                  Expanded(
                    child: Text(
                      'Create support case',
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                        color: c.text,
                        letterSpacing: -0.2,
                      ),
                    ),
                  ),
                ],
              ),
            ),

            // ── Step indicator ──
            const Padding(
              padding: EdgeInsets.fromLTRB(20, 12, 20, 16),
              child: _StepIndicator(step: 1),
            ),

            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 4, 20, 24),
                children: [
                  Text(
                    "Tell us what's happening",
                    style: TextStyle(
                      fontSize: 22,
                      fontWeight: FontWeight.w800,
                      color: c.text,
                      letterSpacing: -0.3,
                    ),
                  ),
                  const SizedBox(height: 24),

                  const _Label('Category'),
                  _Dropdown(
                    hint: 'Select a category',
                    value: _category,
                    items: _categories,
                    onChanged: (v) => setState(() => _category = v),
                  ),
                  const SizedBox(height: 20),

                  const _Label('Subject'),
                  _Field(
                    controller: _subjectController,
                    hint: 'Briefly describe the issue',
                    textCapitalization: TextCapitalization.sentences,
                    onChanged: () => setState(() {}),
                  ),
                  const SizedBox(height: 20),

                  const _Label('Details'),
                  _Field(
                    controller: _detailsController,
                    hint: 'Provide more details',
                    maxLines: 5,
                    textCapitalization: TextCapitalization.sentences,
                    onChanged: () => setState(() {}),
                  ),
                ],
              ),
            ),

            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _valid ? _openEvidence : null,
                  style: FilledButton.styleFrom(
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('Next'),
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
// Step indicator — 3 dots
// ═══════════════════════════════════════════════════════════════════════════

class _StepIndicator extends StatelessWidget {
  const _StepIndicator({required this.step});
  final int step;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: List.generate(3, (i) {
        final n = i + 1;
        final active = n <= step;
        return Padding(
          padding: const EdgeInsets.symmetric(horizontal: 6),
          child: AnimatedContainer(
            duration: const Duration(milliseconds: 200),
            width: active ? 28 : 10,
            height: 10,
            decoration: BoxDecoration(
              color: active ? c.brandTeal500 : c.navy20,
              borderRadius: BorderRadius.circular(999),
            ),
          ),
        );
      }),
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Shared inputs
// ═══════════════════════════════════════════════════════════════════════════

class _Label extends StatelessWidget {
  const _Label(this.text);
  final String text;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: Text(
        text,
        style: TextStyle(
          fontSize: 14,
          fontWeight: FontWeight.w700,
          color: c.text,
        ),
      ),
    );
  }
}

class _Field extends StatelessWidget {
  const _Field({
    required this.controller,
    required this.hint,
    required this.onChanged,
    this.maxLines = 1,
    this.textCapitalization = TextCapitalization.none,
  });

  final TextEditingController controller;
  final String hint;
  final VoidCallback onChanged;
  final int maxLines;
  final TextCapitalization textCapitalization;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return TextField(
      controller: controller,
      maxLines: maxLines,
      textCapitalization: textCapitalization,
      onChanged: (_) => onChanged(),
      style: TextStyle(
        fontSize: 15,
        fontWeight: FontWeight.w500,
        color: c.text,
        height: 1.4,
      ),
      decoration: InputDecoration(
        hintText: hint,
        hintStyle: TextStyle(
          color: c.textDisabled,
          fontSize: 15,
          fontWeight: FontWeight.w500,
        ),
        filled: true,
        fillColor: c.inputFill,
        contentPadding:
            const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide.none,
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide.none,
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: c.brandTeal500, width: 1.6),
        ),
      ),
    );
  }
}

class _Dropdown extends StatelessWidget {
  const _Dropdown({
    required this.hint,
    required this.value,
    required this.items,
    required this.onChanged,
  });

  final String hint;
  final String? value;
  final List<String> items;
  final ValueChanged<String?> onChanged;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16),
      decoration: BoxDecoration(
        color: c.inputFill,
        borderRadius: BorderRadius.circular(12),
      ),
      child: DropdownButtonHideUnderline(
        child: DropdownButton<String>(
          value: value,
          isExpanded: true,
          hint: Text(
            hint,
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
          items: items
              .map(
                (e) => DropdownMenuItem(
                  value: e,
                  child: Text(
                    e,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w500,
                      color: c.text,
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