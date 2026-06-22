import 'dart:async';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../../../shared/widgets/figma_customer_widgets.dart';
import '../../auth/models/customer_auth_models.dart';
import '../data/support_api.dart';
import '../models/support_models.dart';

class SupportChatScreen extends StatefulWidget {
  const SupportChatScreen({
    super.key,
    required this.session,
    required this.supportApi,
  });

  final CustomerSession session;
  final SupportApi supportApi;

  @override
  State<SupportChatScreen> createState() => _SupportChatScreenState();
}

class _SupportChatScreenState extends State<SupportChatScreen> {
  Complaint? _complaint;
  List<ChatMessage> _messages = [];
  bool _starting = false;
  bool _sending = false;
  ApiException? _error;

  final _inputCtrl = TextEditingController();
  Timer? _pollTimer;

  @override
  void dispose() {
    _pollTimer?.cancel();
    _inputCtrl.dispose();
    super.dispose();
  }

  Future<void> _startChat() async {
    setState(() {
      _starting = true;
      _error = null;
    });
    try {
      final complaint = await widget.supportApi.startSupportChat(
        accessToken: widget.session.accessToken,
      );
      final messages = await widget.supportApi.listMessages(
        accessToken: widget.session.accessToken,
        complaintId: complaint.id,
      );
      setState(() {
        _complaint = complaint;
        _messages = messages;
        _starting = false;
      });
      _startPolling();
    } on ApiException catch (e) {
      setState(() {
        _error = e;
        _starting = false;
      });
    }
  }

  void _startPolling() {
    _pollTimer?.cancel();
    _pollTimer = Timer.periodic(const Duration(seconds: 6), (_) => _poll());
  }

  Future<void> _poll() async {
    final complaint = _complaint;
    if (complaint == null) return;
    try {
      final messages = await widget.supportApi.listMessages(
        accessToken: widget.session.accessToken,
        complaintId: complaint.id,
      );
      if (mounted && messages.length != _messages.length) {
        setState(() => _messages = messages);
      }
    } catch (_) {
      // silent — next poll will retry
    }
  }

  Future<void> _send() async {
    final content = _inputCtrl.text.trim();
    if (content.isEmpty || _complaint == null) return;

    _inputCtrl.clear();
    setState(() => _sending = true);

    try {
      final msg = await widget.supportApi.sendMessage(
        accessToken: widget.session.accessToken,
        complaintId: _complaint!.id,
        content: content,
      );
      setState(() {
        _messages = [..._messages, msg];
        _sending = false;
      });
    } on ApiException catch (e) {
      setState(() => _sending = false);
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(e.message)));
      }
      _inputCtrl.text = content;
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_complaint == null) {
      return _EmptyState(
        starting: _starting,
        error: _error,
        onStart: _startChat,
      );
    }

    return Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      appBar: AppBar(
        backgroundColor: Colors.white,
        elevation: 0,
        leading: IconButton(
          icon: const Icon(Icons.arrow_back_rounded, color: CustomerFigmaColors.text),
          onPressed: () => Navigator.of(context).pop(),
        ),
        title: const Text(
          'Support',
          style: TextStyle(
            color: CustomerFigmaColors.text,
            fontSize: 17,
            fontWeight: FontWeight.w800,
          ),
        ),
      ),
      body: Column(
        children: [
          Expanded(
            child: _messages.isEmpty
                ? const Center(
                    child: Text(
                      'No messages yet.\nSay hello to get started.',
                      textAlign: TextAlign.center,
                      style: TextStyle(
                          color: CustomerFigmaColors.muted, fontSize: 14),
                    ),
                  )
                : ListView.builder(
                    padding: const EdgeInsets.symmetric(
                        horizontal: 16, vertical: 12),
                    itemCount: _messages.length,
                    itemBuilder: (_, index) {
                      final msg = _messages[index];
                      final showDate = index == 0 ||
                          !_sameDay(
                              _messages[index - 1].createdAt, msg.createdAt);
                      return Column(
                        children: [
                          if (showDate) _DateDivider(date: msg.createdAt),
                          _MessageBubble(
                            message: msg,
                            customerId: widget.session.customer.id,
                          ),
                        ],
                      );
                    },
                  ),
          ),
          _InputBar(
            controller: _inputCtrl,
            sending: _sending,
            onSend: _send,
          ),
        ],
      ),
    );
  }

  bool _sameDay(DateTime a, DateTime b) =>
      a.year == b.year && a.month == b.month && a.day == b.day;
}

// ─── Empty state ──────────────────────────────────────────────────────────────

class _EmptyState extends StatelessWidget {
  const _EmptyState({
    required this.starting,
    required this.error,
    required this.onStart,
  });

  final bool starting;
  final ApiException? error;
  final VoidCallback onStart;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      appBar: AppBar(
        backgroundColor: CustomerFigmaColors.surface,
        elevation: 0,
        leading: IconButton(
          icon: const Icon(Icons.arrow_back_rounded, color: CustomerFigmaColors.text),
          onPressed: () => Navigator.of(context).pop(),
        ),
        title: const Text(
          'Support',
          style: TextStyle(
            color: CustomerFigmaColors.text,
            fontSize: 17,
            fontWeight: FontWeight.w800,
          ),
        ),
      ),
      body: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          children: [
            Expanded(
              child: Center(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Image.asset(
                      CustomerFigmaAssets.allSetPeople,
                      height: 180,
                      fit: BoxFit.contain,
                    ),
                    const SizedBox(height: 28),
                    const Text(
                      'Contact Support!',
                      style: TextStyle(
                        color: CustomerFigmaColors.text,
                        fontSize: 22,
                        fontWeight: FontWeight.w900,
                      ),
                    ),
                    const SizedBox(height: 12),
                    const Text(
                      'Our dedicated team is here to assist you\nwith any questions or issues related\nto your experience.',
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        color: CustomerFigmaColors.muted,
                        fontSize: 14,
                        height: 1.5,
                      ),
                    ),
                    if (error != null) ...[
                      const SizedBox(height: 16),
                      Text(
                        error!.message,
                        textAlign: TextAlign.center,
                        style: const TextStyle(
                            color: Color(0xFFC0392B), fontSize: 13),
                      ),
                    ],
                  ],
                ),
              ),
            ),
            FigmaPrimaryButton(
              label: 'Start Chat',
              isLoading: starting,
              onPressed: starting ? null : onStart,
            ),
          ],
        ),
      ),
    );
  }
}

// ─── Message bubble ───────────────────────────────────────────────────────────

class _MessageBubble extends StatelessWidget {
  const _MessageBubble({required this.message, required this.customerId});

  final ChatMessage message;
  final String customerId;

  @override
  Widget build(BuildContext context) {
    final isOwn = !message.isAdmin;

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        mainAxisAlignment:
            isOwn ? MainAxisAlignment.end : MainAxisAlignment.start,
        crossAxisAlignment: CrossAxisAlignment.end,
        children: [
          if (!isOwn) ...[
            _AgentAvatar(),
            const SizedBox(width: 8),
          ],
          Column(
            crossAxisAlignment:
                isOwn ? CrossAxisAlignment.end : CrossAxisAlignment.start,
            children: [
              if (!isOwn)
                Padding(
                  padding: const EdgeInsets.only(bottom: 4, left: 4),
                  child: Text(
                    'Support Agent',
                    style: const TextStyle(
                      color: CustomerFigmaColors.muted,
                      fontSize: 11,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                ),
              Container(
                constraints: BoxConstraints(
                  maxWidth: MediaQuery.of(context).size.width * 0.68,
                ),
                padding:
                    const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
                decoration: BoxDecoration(
                  color: isOwn
                      ? CustomerFigmaColors.primaryPale
                      : CustomerFigmaColors.primary,
                  borderRadius: BorderRadius.only(
                    topLeft: const Radius.circular(16),
                    topRight: const Radius.circular(16),
                    bottomLeft: Radius.circular(isOwn ? 16 : 4),
                    bottomRight: Radius.circular(isOwn ? 4 : 16),
                  ),
                ),
                child: Text(
                  message.content,
                  style: TextStyle(
                    color: isOwn ? CustomerFigmaColors.text : Colors.white,
                    fontSize: 14,
                    height: 1.4,
                  ),
                ),
              ),
              Padding(
                padding: const EdgeInsets.only(top: 4, left: 4, right: 4),
                child: Text(
                  _formatTime(message.createdAt),
                  style: const TextStyle(
                    color: CustomerFigmaColors.muted,
                    fontSize: 10,
                  ),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }

  String _formatTime(DateTime dt) {
    final h = dt.hour % 12 == 0 ? 12 : dt.hour % 12;
    final m = dt.minute.toString().padLeft(2, '0');
    final period = dt.hour < 12 ? 'AM' : 'PM';
    return '$h:$m $period';
  }
}

class _AgentAvatar extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Container(
      width: 32,
      height: 32,
      decoration: const BoxDecoration(
        color: CustomerFigmaColors.primary,
        shape: BoxShape.circle,
      ),
      child: const Center(
        child: Text(
          'A',
          style: TextStyle(
            color: Colors.white,
            fontWeight: FontWeight.w700,
            fontSize: 14,
          ),
        ),
      ),
    );
  }
}

// ─── Date divider ─────────────────────────────────────────────────────────────

class _DateDivider extends StatelessWidget {
  const _DateDivider({required this.date});
  final DateTime date;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 12),
      child: Row(
        children: [
          const Expanded(child: Divider(color: CustomerFigmaColors.border)),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 12),
            child: Text(
              _label(date),
              style: const TextStyle(
                color: CustomerFigmaColors.muted,
                fontSize: 11,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
          const Expanded(child: Divider(color: CustomerFigmaColors.border)),
        ],
      ),
    );
  }

  String _label(DateTime d) {
    final now = DateTime.now();
    if (d.year == now.year && d.month == now.month && d.day == now.day) {
      return 'Today';
    }
    const days = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
    return '${days[d.weekday % 7]} at ${d.hour % 12 == 0 ? 12 : d.hour % 12}:${d.minute.toString().padLeft(2, '0')} ${d.hour < 12 ? 'AM' : 'PM'}';
  }
}

// ─── Input bar ────────────────────────────────────────────────────────────────

class _InputBar extends StatelessWidget {
  const _InputBar({
    required this.controller,
    required this.sending,
    required this.onSend,
  });

  final TextEditingController controller;
  final bool sending;
  final VoidCallback onSend;

  @override
  Widget build(BuildContext context) {
    return Container(
      color: Colors.white,
      padding: EdgeInsets.only(
        left: 16,
        right: 16,
        top: 10,
        bottom: MediaQuery.of(context).viewInsets.bottom + 12,
      ),
      child: Row(
        children: [
          Expanded(
            child: TextField(
              controller: controller,
              maxLines: 4,
              minLines: 1,
              textCapitalization: TextCapitalization.sentences,
              decoration: InputDecoration(
                hintText: 'Write a message...',
                hintStyle:
                    const TextStyle(color: CustomerFigmaColors.muted, fontSize: 14),
                filled: true,
                fillColor: CustomerFigmaColors.surface,
                contentPadding: const EdgeInsets.symmetric(
                    horizontal: 16, vertical: 10),
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(24),
                  borderSide: BorderSide.none,
                ),
              ),
            ),
          ),
          const SizedBox(width: 10),
          GestureDetector(
            onTap: sending ? null : onSend,
            child: Container(
              width: 44,
              height: 44,
              decoration: BoxDecoration(
                color: sending
                    ? CustomerFigmaColors.primarySoft
                    : CustomerFigmaColors.primary,
                shape: BoxShape.circle,
              ),
              child: sending
                  ? const Padding(
                      padding: EdgeInsets.all(12),
                      child: CircularProgressIndicator(
                        strokeWidth: 2,
                        valueColor:
                            AlwaysStoppedAnimation<Color>(Colors.white),
                      ),
                    )
                  : const Icon(Icons.send_rounded,
                      color: Colors.white, size: 20),
            ),
          ),
        ],
      ),
    );
  }
}
