import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../../../core/theme/karryam_theme.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../data/support_api.dart';
import 'case_chat_screen.dart';
import 'report_problem_screen.dart';

/// Help & support: answers first, then a person.
class HelpScreen extends StatefulWidget {
  const HelpScreen({super.key, required this.api});
  final SupportApi api;

  @override
  State<HelpScreen> createState() => _HelpScreenState();
}

class _HelpScreenState extends State<HelpScreen> {
  List<Faq>? _faqs;
  List<SupportCase>? _cases;
  String? _error;
  bool _openingChat = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() => _error = null);
    try {
      final results = await Future.wait([
        widget.api.faqs(),
        widget.api.cases(),
      ]);
      if (!mounted) return;
      setState(() {
        _faqs = results[0] as List<Faq>;
        // The live chat has its own button; list only reported problems.
        _cases = (results[1] as List<SupportCase>)
            .where((c) => c.subject != 'Support Chat')
            .toList();
      });
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    }
  }

  Future<void> _chat() async {
    setState(() => _openingChat = true);
    try {
      final chat = await widget.api.startChat();
      if (!mounted) return;
      await Navigator.of(context).push(
        MaterialPageRoute(
          builder: (_) => CaseChatScreen(
            api: widget.api,
            supportCase: chat,
            title: 'Chat with support',
          ),
        ),
      );
    } on ApiException catch (e) {
      if (mounted) showApiError(context, e);
    } finally {
      if (mounted) setState(() => _openingChat = false);
    }
  }

  Future<void> _report() async {
    final created = await Navigator.of(context).push<bool>(
      MaterialPageRoute(builder: (_) => ReportProblemScreen(api: widget.api)),
    );
    if (created == true) _load();
  }

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    final faqs = _faqs;
    final cases = _cases;
    return KarryamPage(
      title: 'Help & support',
      body: RefreshIndicator(
        onRefresh: _load,
        child: ListView(
          padding: const EdgeInsets.all(20),
          children: [
            KarryamButton(
              label: 'Chat with support',
              busy: _openingChat,
              onPressed: _chat,
            ),
            const SizedBox(height: 10),
            OutlinedButton.icon(
              onPressed: _report,
              icon: const Icon(Icons.report_outlined),
              label: const Text('Report a problem'),
            ),
            if (_error != null && faqs == null)
              KarryamPlaceholder.error(message: _error!, onRetry: _load)
            else if (faqs == null || cases == null)
              const KarryamPlaceholder.loading()
            else ...[
              if (cases.isNotEmpty) ...[
                const KarryamSectionTitle('Your reports'),
                for (final c in cases)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 10),
                    child: KarryamPanel(
                      onTap: () async {
                        await Navigator.of(context).push(
                          MaterialPageRoute(
                            builder: (_) => CaseChatScreen(
                              api: widget.api,
                              supportCase: c,
                              title: c.subject,
                            ),
                          ),
                        );
                        _load();
                      },
                      child: Row(
                        children: [
                          Expanded(
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Text(
                                  c.subject,
                                  style: const TextStyle(
                                    fontWeight: FontWeight.w600,
                                  ),
                                ),
                                Text(
                                  formatDateTime(c.createdAt),
                                  style: TextStyle(
                                    color: colors.textMuted,
                                    fontSize: 12,
                                  ),
                                ),
                              ],
                            ),
                          ),
                          if (c.unread > 0) ...[
                            KarryamChip(
                              '${c.unread} new',
                              tone: KarryamTone.warning,
                            ),
                            const SizedBox(width: 6),
                          ],
                          KarryamChip(
                            humanize(c.status),
                            tone: c.isOpen
                                ? KarryamTone.neutral
                                : KarryamTone.good,
                          ),
                        ],
                      ),
                    ),
                  ),
              ],
              const KarryamSectionTitle('Common questions'),
              if (faqs.isEmpty)
                Text(
                  'No articles yet.',
                  style: TextStyle(color: colors.textMuted),
                ),
              for (final faq in faqs)
                Padding(
                  padding: const EdgeInsets.only(bottom: 10),
                  child: KarryamPanel(
                    padding: EdgeInsets.zero,
                    child: ExpansionTile(
                      shape: const Border(),
                      title: Text(
                        faq.title,
                        style: const TextStyle(fontWeight: FontWeight.w600),
                      ),
                      childrenPadding: const EdgeInsets.fromLTRB(16, 0, 16, 16),
                      expandedCrossAxisAlignment: CrossAxisAlignment.start,
                      children: [Text(faq.body)],
                    ),
                  ),
                ),
            ],
          ],
        ),
      ),
    );
  }
}
