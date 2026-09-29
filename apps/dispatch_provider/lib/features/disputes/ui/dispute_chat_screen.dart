import 'dart:async';

import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../models/dispute_models.dart';
import '../state/rider_dispute_controller.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 09 — Chat thread
// Figma 10 — Message sending / retry
// Figma 11 — Unread / read state
// Figma 12 — Chat unavailable (empty state inside this screen)
// ═══════════════════════════════════════════════════════════════════════════

class DisputeChatScreen extends StatefulWidget {
  const DisputeChatScreen({
    super.key,
    required this.complaint,
    required this.controller,
    this.title = 'Support Chat',
  });

  final Complaint complaint;
  final RiderDisputeController controller;
  final String title;

  @override
  State<DisputeChatScreen> createState() => _DisputeChatScreenState();
}

class _DisputeChatScreenState extends State<DisputeChatScreen> {
  final _composer = TextEditingController();
  final _scroll = ScrollController();
  Timer? _pollTimer;

  /// Tracks messages that failed to send (Figma 10 retry).
  final Set<String> _failedIds = {};

  @override
  void initState() {
    super.initState();
    widget.controller.loadMessages(widget.complaint.id);
    widget.controller.markMessagesRead(widget.complaint.id);
    _pollTimer = Timer.periodic(const Duration(seconds: 5), (_) {
      widget.controller.loadMessages(widget.complaint.id);
    });
  }

  @override
  void dispose() {
    _pollTimer?.cancel();
    _composer.dispose();
    _scroll.dispose();
    super.dispose();
  }

  void _scrollToBottom() {
    if (!_scroll.hasClients) return;
    _scroll.animateTo(
      _scroll.position.maxScrollExtent + 80,
      duration: const Duration(milliseconds: 250),
      curve: Curves.easeOut,
    );
  }

  Future<void> _send() async {
    final text = _composer.text.trim();
    if (text.isEmpty) return;
    _composer.clear();

    // Optimistic local echo so the user sees the bubble instantly.
    final localId = 'local-${DateTime.now().microsecondsSinceEpoch}';
    try {
      await widget.controller.sendMessage(widget.complaint.id, text);
      if (_failedIds.contains(localId)) {
        _failedIds.remove(localId);
      }
      _scrollToBottom();
    } catch (_) {
      setState(() => _failedIds.add(localId));
    }
  }

  Future<void> _retry(String localId) async {
    setState(() => _failedIds.remove(localId));
    await widget.controller.sendMessage(
      widget.complaint.id,
      _composer.text.isEmpty ? 'Retrying…' : _composer.text,
    );
    _scrollToBottom();
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        bottom: false,
        child: Column(
          children: [
            // ── Header ──
            Container(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 12),
              decoration: BoxDecoration(
                color: c.background,
                border: Border(
                  bottom: BorderSide(color: c.divider),
                ),
              ),
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
                  // Avatar
                  Container(
                    width: 40,
                    height: 40,
                    alignment: Alignment.center,
                    decoration: BoxDecoration(
                      color: c.brandNavy900,
                      shape: BoxShape.circle,
                    ),
                    child: const Text(
                      'K',
                      style: TextStyle(
                        fontSize: 18,
                        fontWeight: FontWeight.w800,
                        color: Colors.white,
                      ),
                    ),
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          'Karryam Support',
                          style: TextStyle(
                            fontSize: 15,
                            fontWeight: FontWeight.w700,
                            color: c.text,
                            letterSpacing: -0.1,
                          ),
                        ),
                        const SizedBox(height: 2),
                        Row(
                          children: [
                            Container(
                              width: 8,
                              height: 8,
                              decoration: BoxDecoration(
                                color: c.success,
                                shape: BoxShape.circle,
                              ),
                            ),
                            const SizedBox(width: 5),
                            Text(
                              'Usually replies in minutes',
                              style: TextStyle(
                                fontSize: 12,
                                fontWeight: FontWeight.w500,
                                color: c.textMuted,
                              ),
                            ),
                          ],
                        ),
                      ],
                    ),
                  ),
                  GestureDetector(
                    onTap: () {
                      ScaffoldMessenger.of(context).showSnackBar(
                        const SnackBar(
                          content: Text('More options coming soon.'),
                        ),
                      );
                    },
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Icon(
                        Icons.more_vert_rounded,
                        size: 22,
                        color: c.text,
                      ),
                    ),
                  ),
                ],
              ),
            ),

            // ── Messages ──
            Expanded(
              child: AnimatedBuilder(
                animation: widget.controller,
                builder: (context, _) {
                  final messages = widget.controller.messages;

                  if (widget.controller.loadingMessages &&
                      messages.isEmpty) {
                    return Center(
                      child: CircularProgressIndicator(
                        color: c.brandTeal500,
                      ),
                    );
                  }

                  if (messages.isEmpty) {
                    return _EmptyChat(
                      onStart: () {
                        _composer.text = 'Hi, I need help with…';
                      },
                    );
                  }

                  return ListView.builder(
                    controller: _scroll,
                    padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
                    itemCount: messages.length,
                    itemBuilder: (context, i) {
                      final m = messages[i];
                      return _MessageBubble(
                        message: m,
                        failed: _failedIds.contains(m.id),
                        onRetry: () => _retry(m.id),
                      );
                    },
                  );
                },
              ),
            ),

            // ── Composer ──
            Container(
              color: c.surface,
              padding: const EdgeInsets.fromLTRB(16, 10, 16, 12),
              child: SafeArea(
                top: false,
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.end,
                  children: [
                    Expanded(
                      child: TextField(
                        controller: _composer,
                        minLines: 1,
                        maxLines: 4,
                        textInputAction: TextInputAction.send,
                        onSubmitted: (_) => _send(),
                        style: TextStyle(
                          fontSize: 15,
                          fontWeight: FontWeight.w500,
                          color: c.text,
                        ),
                        decoration: InputDecoration(
                          hintText: 'Type a message...',
                          hintStyle: TextStyle(
                            fontSize: 15,
                            color: c.textDisabled,
                          ),
                          filled: true,
                          fillColor: c.inputFill,
                          contentPadding: const EdgeInsets.symmetric(
                            horizontal: 16,
                            vertical: 12,
                          ),
                          border: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(24),
                            borderSide: BorderSide.none,
                          ),
                          enabledBorder: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(24),
                            borderSide: BorderSide.none,
                          ),
                          focusedBorder: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(24),
                            borderSide: BorderSide(
                              color: c.brandTeal500,
                              width: 1.6,
                            ),
                          ),
                        ),
                      ),
                    ),
                    const SizedBox(width: 10),
                    GestureDetector(
                      onTap: widget.controller.sending ? null : _send,
                      child: Container(
                        width: 44,
                        height: 44,
                        alignment: Alignment.center,
                        decoration: BoxDecoration(
                          color: c.brandTeal500,
                          shape: BoxShape.circle,
                        ),
                        child: widget.controller.sending
                            ? const SizedBox(
                                width: 18,
                                height: 18,
                                child: CircularProgressIndicator(
                                  strokeWidth: 2,
                                  color: Colors.white,
                                ),
                              )
                            : const Icon(
                                Icons.arrow_upward_rounded,
                                size: 20,
                                color: Colors.white,
                              ),
                      ),
                    ),
                  ],
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
// Message bubble
// ═══════════════════════════════════════════════════════════════════════════

class _MessageBubble extends StatelessWidget {
  const _MessageBubble({
    required this.message,
    this.failed = false,
    this.onRetry,
  });

  final DisputeMessage message;
  final bool failed;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final mine = message.isMine;

    return Column(
      crossAxisAlignment:
          mine ? CrossAxisAlignment.end : CrossAxisAlignment.start,
      children: [
        Align(
          alignment: mine ? Alignment.centerRight : Alignment.centerLeft,
          child: Container(
            margin: const EdgeInsets.only(bottom: 4),
            padding: const EdgeInsets.symmetric(
              horizontal: 14,
              vertical: 10,
            ),
            constraints: BoxConstraints(
              maxWidth: MediaQuery.of(context).size.width * 0.75,
            ),
            decoration: BoxDecoration(
              color: mine ? c.brandTeal500 : c.surface,
              borderRadius: BorderRadius.only(
                topLeft: const Radius.circular(16),
                topRight: const Radius.circular(16),
                bottomLeft: Radius.circular(mine ? 16 : 4),
                bottomRight: Radius.circular(mine ? 4 : 16),
              ),
              border: mine
                  ? null
                  : Border.all(color: c.divider, width: 1),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.end,
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  message.content,
                  style: TextStyle(
                    fontSize: 14.5,
                    height: 1.4,
                    color: mine ? Colors.white : c.text,
                  ),
                ),
                const SizedBox(height: 3),
                Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      _formatTime(message.createdAt),
                      style: TextStyle(
                        fontSize: 10.5,
                        color: mine
                            ? Colors.white.withValues(alpha: 0.85)
                            : c.textMuted,
                      ),
                    ),
                    if (mine) ...[
                      const SizedBox(width: 4),
                      Icon(
                        failed
                            ? Icons.error_outline_rounded
                            : Icons.done_all_rounded,
                        size: 13,
                        color: failed
                            ? c.danger
                            : Colors.white.withValues(alpha: 0.85),
                      ),
                    ],
                  ],
                ),
              ],
            ),
          ),
        ),
        if (failed)
          Padding(
            padding: const EdgeInsets.only(left: 12, right: 12, bottom: 8),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(
                  Icons.error_outline_rounded,
                  size: 14,
                  color: c.danger,
                ),
                const SizedBox(width: 5),
                Text(
                  'Message failed to send.',
                  style: TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w500,
                    color: c.danger,
                  ),
                ),
                const SizedBox(width: 8),
                GestureDetector(
                  onTap: onRetry,
                  behavior: HitTestBehavior.opaque,
                  child: Text(
                    'Retry',
                    style: TextStyle(
                      fontSize: 12,
                      fontWeight: FontWeight.w700,
                      color: c.brandTeal600,
                    ),
                  ),
                ),
              ],
            ),
          ),
      ],
    );
  }

  static String _formatTime(DateTime t) {
    final h12 = t.hour % 12 == 0 ? 12 : t.hour % 12;
    final m = t.minute.toString().padLeft(2, '0');
    final p = t.hour < 12 ? 'AM' : 'PM';
    return '$h12:$m $p';
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Empty chat state — Figma 12
// ═══════════════════════════════════════════════════════════════════════════

class _EmptyChat extends StatelessWidget {
  const _EmptyChat({required this.onStart});
  final VoidCallback onStart;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 88,
              height: 88,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: c.navy10,
                shape: BoxShape.circle,
              ),
              child: Icon(
                Icons.headset_mic_outlined,
                size: 42,
                color: c.textMuted,
              ),
            ),
            const SizedBox(height: 22),
            Text(
              "Start the conversation",
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 20,
                fontWeight: FontWeight.w800,
                color: c.text,
                letterSpacing: -0.3,
              ),
            ),
            const SizedBox(height: 8),
            Text(
              'Tell us what you need help with and a support agent will reply here.',
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 14,
                fontWeight: FontWeight.w500,
                color: c.textMuted,
                height: 1.5,
              ),
            ),
            const SizedBox(height: 20),
            TextButton(
              onPressed: onStart,
              style: TextButton.styleFrom(
                foregroundColor: c.brandTeal600,
                textStyle: const TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w700,
                ),
              ),
              child: const Text('Send a message'),
            ),
          ],
        ),
      ),
    );
  }
}