import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../../../core/theme/karryam_theme.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../data/support_api.dart';

/// Report a problem to support. Pops `true` once filed.
class ReportProblemScreen extends StatefulWidget {
  const ReportProblemScreen({super.key, required this.api, this.tripId});
  final SupportApi api;

  /// The ride this is about, when reported from a trip.
  final String? tripId;

  @override
  State<ReportProblemScreen> createState() => _ReportProblemScreenState();
}

class _ReportProblemScreenState extends State<ReportProblemScreen> {
  static const _topics = [
    'Payment or earnings',
    'Passenger behaviour',
    'Trip or route problem',
    'App not working',
    'Account or documents',
    'Something else',
  ];
  String? _topic;
  final _details = TextEditingController();
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _details.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await widget.api.report(
        subject: _topic!,
        description: _details.text.trim(),
        tripId: widget.tripId,
      );
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Thanks. Support will reply in the app.')),
      );
      Navigator.of(context).pop(true);
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    final ready = _topic != null && _details.text.trim().length >= 10;
    return KarryamPage(
      title: 'Report a problem',
      subtitle: widget.tripId == null ? null : 'About this trip',
      bottom: KarryamButton(
        label: 'Send report',
        busy: _busy,
        onPressed: ready ? _submit : null,
      ),
      body: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          const Text(
            'What is it about?',
            style: TextStyle(fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 10),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: [
              for (final topic in _topics)
                ChoiceChip(
                  label: Text(topic),
                  selected: _topic == topic,
                  onSelected: (_) => setState(() => _topic = topic),
                ),
            ],
          ),
          const SizedBox(height: 20),
          KarryamField(
            controller: _details,
            label: 'What happened?',
            hint: 'Tell us what happened (at least 10 characters).',
            maxLines: 6,
            textCapitalization: TextCapitalization.sentences,
          ),
          if (_error != null)
            Text(_error!, style: TextStyle(color: colors.danger)),
        ],
      ),
    );
  }

  @override
  void initState() {
    super.initState();
    _details.addListener(() => setState(() {}));
  }
}
