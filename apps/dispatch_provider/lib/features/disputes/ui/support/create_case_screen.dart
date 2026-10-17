import 'package:flutter/material.dart';
import '../../../../core/theme/theme_extensions.dart';
import '../../../../shared/widgets/async_content.dart';
import '../../state/rider_dispute_controller.dart';

class CreateCaseScreen extends StatefulWidget {
  const CreateCaseScreen({
    super.key,
    required this.disputeController,
    this.initialCategory,
    this.initialSubject,
  });
  final RiderDisputeController disputeController;
  final String? initialCategory;
  final String? initialSubject;
  @override
  State<CreateCaseScreen> createState() => _CreateCaseScreenState();
}

class _CreateCaseScreenState extends State<CreateCaseScreen> {
  final _subject = TextEditingController();
  final _details = TextEditingController();
  String? _category;
  bool _busy = false;
  String? _error;
  bool _created = false;
  static const _categories = {
    'incorrect_delivery': 'Incorrect delivery',
    'delayed_arrival': 'Delayed arrival',
    'payment': 'Payment',
    'damaged_goods': 'Damaged goods',
    'provider_misconduct': 'Misconduct',
    'fraud': 'Fraud',
    'emergency': 'Emergency',
    'other': 'Other',
  };
  @override
  void initState() {
    super.initState();
    _subject.text = widget.initialSubject ?? '';
    final initial = widget.initialCategory;
    if (initial != null)
      _category = _categories.containsKey(initial)
          ? initial
          : initial == 'Earnings & Payments'
          ? 'payment'
          : 'other';
  }

  @override
  void dispose() {
    _subject.dispose();
    _details.dispose();
    super.dispose();
  }

  bool get _valid =>
      !_busy &&
      _category != null &&
      _subject.text.trim().isNotEmpty &&
      _subject.text.trim().length <= 200 &&
      _details.text.trim().isNotEmpty;
  Future<void> _submit() async {
    if (!_valid) return;
    FocusScope.of(context).unfocus();
    setState(() {
      _busy = true;
      _error = null;
    });
    final result = await widget.disputeController.createCase(
      subject: _subject.text.trim(),
      description: _details.text.trim(),
      category: _category,
    );
    if (!mounted) return;
    setState(() {
      _busy = false;
      _created = result != null;
      _error = result == null
          ? widget.disputeController.submitError ??
                'The case was not confirmed. Check My cases before retrying.'
          : null;
    });
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return PopScope(
      canPop: !_busy,
      child: Scaffold(
        appBar: AppBar(title: const Text('New support case')),
        body: SafeArea(
          child: _created
              ? ConfirmedResult(
                  title: 'Case submitted',
                  message:
                      'Your details have been saved. Follow replies in My cases.',
                  onDone: () => Navigator.pop(context, true),
                )
              : Column(
                  children: [
                    Expanded(
                      child: ListView(
                        padding: const EdgeInsets.all(24),
                        children: [
                          Align(
                            alignment: Alignment.centerLeft,
                            child: Container(
                              padding: const EdgeInsets.all(16),
                              decoration: BoxDecoration(
                                color: c.surfaceElevated,
                                borderRadius: BorderRadius.circular(20),
                              ),
                              child: Icon(
                                Icons.support_agent_rounded,
                                color: c.brandTeal600,
                                size: 32,
                              ),
                            ),
                          ),
                          const SizedBox(height: 24),
                          Text(
                            "Tell us what happened",
                            style: Theme.of(context).textTheme.headlineSmall
                                ?.copyWith(fontWeight: FontWeight.w700),
                          ),
                          const SizedBox(height: 8),
                          Text(
                            'Include the details that will help us understand your issue.',
                            style: TextStyle(color: c.textMuted, height: 1.5),
                          ),
                          const SizedBox(height: 28),
                          DropdownButtonFormField<String>(
                            initialValue: _category,
                            isExpanded: true,
                            decoration: const InputDecoration(
                              labelText: 'Category',
                            ),
                            items: _categories.entries
                                .map(
                                  (e) => DropdownMenuItem(
                                    value: e.key,
                                    child: Text(e.value),
                                  ),
                                )
                                .toList(),
                            onChanged: _busy
                                ? null
                                : (value) => setState(() => _category = value),
                          ),
                          const SizedBox(height: 20),
                          TextField(
                            controller: _subject,
                            enabled: !_busy,
                            maxLength: 200,
                            textCapitalization: TextCapitalization.sentences,
                            decoration: const InputDecoration(
                              labelText: 'Subject',
                              hintText: 'Briefly describe the issue',
                            ),
                            onChanged: (_) => setState(() {}),
                          ),
                          const SizedBox(height: 12),
                          TextField(
                            controller: _details,
                            enabled: !_busy,
                            minLines: 5,
                            maxLines: 10,
                            textCapitalization: TextCapitalization.sentences,
                            decoration: const InputDecoration(
                              labelText: 'Details',
                              alignLabelWithHint: true,
                            ),
                            onChanged: (_) => setState(() {}),
                          ),
                          if (_error != null)
                            Padding(
                              padding: const EdgeInsets.only(top: 20),
                              child: ActionNotice(
                                message: _error!,
                                isError: true,
                              ),
                            ),
                        ],
                      ),
                    ),
                    Padding(
                      padding: const EdgeInsets.fromLTRB(24, 8, 24, 20),
                      child: SizedBox(
                        width: double.infinity,
                        child: FilledButton(
                          onPressed: _valid ? _submit : null,
                          child: _busy
                              ? const SizedBox(
                                  width: 22,
                                  height: 22,
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2,
                                  ),
                                )
                              : const Text('Submit case'),
                        ),
                      ),
                    ),
                  ],
                ),
        ),
      ),
    );
  }
}
