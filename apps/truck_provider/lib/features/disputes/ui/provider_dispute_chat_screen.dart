import 'package:flutter/material.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../models/dispute_models.dart';
import '../state/provider_dispute_controller.dart';
import 'widgets/dispute_widgets.dart';

/// Live chat on a dispute (support-dispute-service messages).
class ProviderDisputeChatScreen extends StatefulWidget {
  const ProviderDisputeChatScreen({super.key, required this.controller, required this.complaint});

  final ProviderDisputeController controller;
  final Complaint complaint;

  @override
  State<ProviderDisputeChatScreen> createState() => _ProviderDisputeChatScreenState();
}

class _ProviderDisputeChatScreenState extends State<ProviderDisputeChatScreen> {
  final _input = TextEditingController();

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => widget.controller.loadMessages(widget.complaint.id));
  }

  @override
  void dispose() {
    _input.dispose();
    super.dispose();
  }

  Future<void> _send() async {
    final text = _input.text.trim();
    if (text.isEmpty) return;
    _input.clear();
    await widget.controller.sendMessage(widget.complaint.id, text);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: AnimatedBuilder(
        animation: widget.controller,
        builder: (context, _) {
          final c = widget.controller;
          return SafeArea(
            child: Column(
              children: [
                const DisputeAppBar(title: 'Live Chat'),
                const SizedBox(height: 8),
                Expanded(
                  child: c.loadingMessages && c.messages.isEmpty
                      ? const Center(child: CircularProgressIndicator(color: kProviderGreen))
                      : c.messages.isEmpty
                          ? const _EmptyChat()
                          : ListView.builder(
                              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
                              itemCount: c.messages.length,
                              itemBuilder: (context, i) => _Bubble(message: c.messages[i]),
                            ),
                ),
                _Composer(controller: _input, sending: c.sending, onSend: _send),
              ],
            ),
          );
        },
      ),
    );
  }
}

class _EmptyChat extends StatelessWidget {
  const _EmptyChat();
  @override
  Widget build(BuildContext context) {
    return const Center(
      child: Padding(
        padding: EdgeInsets.all(40),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.chat_bubble_outline_rounded, color: kProviderMuted, size: 40),
            SizedBox(height: 12),
            Text(
              'Start the conversation with support.',
              textAlign: TextAlign.center,
              style: TextStyle(color: kProviderMuted, fontSize: 13),
            ),
          ],
        ),
      ),
    );
  }
}

class _Bubble extends StatelessWidget {
  const _Bubble({required this.message});
  final DisputeMessage message;

  @override
  Widget build(BuildContext context) {
    final mine = message.isMine;
    return Align(
      alignment: mine ? Alignment.centerRight : Alignment.centerLeft,
      child: Container(
        margin: const EdgeInsets.symmetric(vertical: 5),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
        constraints: BoxConstraints(maxWidth: MediaQuery.of(context).size.width * 0.72),
        decoration: BoxDecoration(
          color: mine ? kProviderGreen : kProviderSurface,
          borderRadius: BorderRadius.only(
            topLeft: const Radius.circular(14),
            topRight: const Radius.circular(14),
            bottomLeft: Radius.circular(mine ? 14 : 2),
            bottomRight: Radius.circular(mine ? 2 : 14),
          ),
        ),
        child: Text(
          message.content,
          style: TextStyle(color: mine ? Colors.white : kProviderText, fontSize: 14, height: 1.3),
        ),
      ),
    );
  }
}

class _Composer extends StatelessWidget {
  const _Composer({required this.controller, required this.sending, required this.onSend});
  final TextEditingController controller;
  final bool sending;
  final VoidCallback onSend;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: EdgeInsets.fromLTRB(16, 8, 16, 12 + MediaQuery.of(context).viewInsets.bottom),
      child: Row(
        children: [
          Expanded(
            child: TextField(
              controller: controller,
              minLines: 1,
              maxLines: 4,
              textInputAction: TextInputAction.send,
              onSubmitted: (_) => onSend(),
              decoration: InputDecoration(
                hintText: 'Type a message…',
                hintStyle: const TextStyle(color: kProviderMuted),
                filled: true,
                fillColor: kProviderSurface,
                contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
                border: OutlineInputBorder(borderRadius: BorderRadius.circular(24), borderSide: BorderSide.none),
              ),
            ),
          ),
          const SizedBox(width: 10),
          GestureDetector(
            onTap: sending ? null : onSend,
            child: Container(
              width: 48,
              height: 48,
              decoration: const BoxDecoration(color: kProviderGreen, shape: BoxShape.circle),
              child: sending
                  ? const Padding(
                      padding: EdgeInsets.all(14),
                      child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white),
                    )
                  : const Icon(Icons.send_rounded, color: Colors.white, size: 20),
            ),
          ),
        ],
      ),
    );
  }
}
