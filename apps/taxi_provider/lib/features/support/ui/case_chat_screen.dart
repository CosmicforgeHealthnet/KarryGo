import 'dart:async';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../../../core/theme/karryam_theme.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../data/support_api.dart';

/// Conversation with support about one case. Polls while open.
class CaseChatScreen extends StatefulWidget {
  const CaseChatScreen({
    super.key,
    required this.api,
    required this.supportCase,
    required this.title,
  });
  final SupportApi api;
  final SupportCase supportCase;
  final String title;

  @override
  State<CaseChatScreen> createState() => _CaseChatScreenState();
}

class _CaseChatScreenState extends State<CaseChatScreen> {
  final _input = TextEditingController();
  List<SupportMessage>? _messages;
  String? _error;
  bool _sending = false;
  Timer? _poll;

  @override
  void initState() {
    super.initState();
    _load();
    _poll = Timer.periodic(
      const Duration(seconds: 5),
      (_) => _load(quiet: true),
    );
  }

  @override
  void dispose() {
    _poll?.cancel();
    _input.dispose();
    super.dispose();
  }

  Future<void> _load({bool quiet = false}) async {
    try {
      final messages = await widget.api.messages(widget.supportCase.id);
      if (!mounted) return;
      setState(() {
        _messages = messages;
        _error = null;
      });
      unawaited(widget.api.markRead(widget.supportCase.id).catchError((_) {}));
    } on ApiException catch (e) {
      if (mounted && !quiet) setState(() => _error = e.message);
    }
  }

  Future<void> _send() async {
    final text = _input.text.trim();
    if (text.isEmpty) return;
    setState(() => _sending = true);
    try {
      await widget.api.send(widget.supportCase.id, text);
      _input.clear();
      await _load();
    } on ApiException catch (e) {
      if (mounted) showApiError(context, e);
    } finally {
      if (mounted) setState(() => _sending = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    final messages = _messages;
    final c = widget.supportCase;
    return KarryamPage(
      title: widget.title,
      subtitle: humanize(c.status),
      body: Column(
        children: [
          Expanded(
            child: _error != null && messages == null
                ? KarryamPlaceholder.error(message: _error!, onRetry: _load)
                : messages == null
                ? const KarryamPlaceholder.loading()
                : ListView(
                    reverse: true,
                    padding: const EdgeInsets.all(16),
                    children: [
                      for (final m in messages.reversed) _bubble(context, m),
                      if (c.description.isNotEmpty &&
                          c.subject != 'Support Chat')
                        _bubble(
                          context,
                          SupportMessage(
                            content: c.description,
                            fromMe: true,
                            createdAt: c.createdAt,
                          ),
                        ),
                      if (messages.isEmpty && c.subject == 'Support Chat')
                        Padding(
                          padding: const EdgeInsets.all(24),
                          child: Text(
                            'Send a message and a support agent will reply here.',
                            textAlign: TextAlign.center,
                            style: TextStyle(color: colors.textMuted),
                          ),
                        ),
                    ],
                  ),
          ),
          SafeArea(
            top: false,
            child: Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 12),
              child: Row(
                children: [
                  Expanded(
                    child: TextField(
                      controller: _input,
                      minLines: 1,
                      maxLines: 4,
                      textCapitalization: TextCapitalization.sentences,
                      decoration: InputDecoration(
                        hintText: 'Type a message',
                        filled: true,
                        fillColor: colors.card,
                        border: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(24),
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(width: 8),
                  IconButton.filled(
                    onPressed: _sending ? null : _send,
                    icon: const Icon(Icons.send_rounded),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _bubble(BuildContext context, SupportMessage m) {
    final colors = KarryamColors.of(context);
    return Align(
      alignment: m.fromMe ? Alignment.centerRight : Alignment.centerLeft,
      child: Container(
        margin: const EdgeInsets.symmetric(vertical: 4),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
        constraints: BoxConstraints(
          maxWidth: MediaQuery.sizeOf(context).width * .75,
        ),
        decoration: BoxDecoration(
          color: m.fromMe ? colors.primary : colors.card,
          borderRadius: BorderRadius.circular(16),
          border: m.fromMe ? null : Border.all(color: colors.border),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            if (!m.fromMe)
              Text(
                'Support',
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w700,
                  color: colors.textMuted,
                ),
              ),
            Text(
              m.content,
              style: TextStyle(
                color: m.fromMe ? colors.onPrimary : colors.text,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
