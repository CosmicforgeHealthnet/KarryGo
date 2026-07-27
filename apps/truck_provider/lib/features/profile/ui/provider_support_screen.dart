import 'dart:async';

import 'package:flutter/material.dart';

import '../../disputes/models/dispute_models.dart';
import '../../disputes/state/provider_dispute_controller.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import 'widgets/provider_profile_widgets.dart';

/// Contact Support (Account option-4). "Start Chat" opens the Live Chat screen.
class ProviderSupportScreen extends StatelessWidget {
  const ProviderSupportScreen({super.key, required this.disputeController});

  final ProviderDisputeController disputeController;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 16, 20, 24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              ProviderProfileHeader(title: 'Support'),
              const Spacer(),
              Center(
                child: Container(
                  width: 150,
                  height: 150,
                  decoration: const BoxDecoration(color: kProviderGreenTint, shape: BoxShape.circle),
                  child: const Icon(Icons.support_agent_rounded, color: kProviderGreen, size: 76),
                ),
              ),
              const SizedBox(height: 28),
              const Text(
                'Contact Support!',
                textAlign: TextAlign.center,
                style: TextStyle(color: kProviderText, fontSize: 20, fontWeight: FontWeight.w800),
              ),
              const SizedBox(height: 10),
              const Text(
                'Our dedicated team is here to assist you with any questions or issues related to your experience.',
                textAlign: TextAlign.center,
                style: TextStyle(color: kProviderMuted, fontSize: 13, height: 1.5),
              ),
              const Spacer(),
              SizedBox(
                height: 54,
                child: FilledButton.icon(
                  onPressed: () => Navigator.of(context).push(
                    MaterialPageRoute(
                      builder: (_) => ProviderLiveChatScreen(controller: disputeController),
                    ),
                  ),
                  icon: const Icon(Icons.chat_bubble_outline_rounded, color: Colors.white, size: 20),
                  label: const Text('Start Chat',
                      style: TextStyle(color: Colors.white, fontWeight: FontWeight.w700, fontSize: 16)),
                  style: FilledButton.styleFrom(
                    backgroundColor: kProviderGreen,
                    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
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

// ─── Live Chat (Account option-5) ─────────────────────────────────────────────

/// Real support chat: messages live on a support-dispute-service complaint
/// (subject "Live chat support"), agent replies come from the admin surface,
/// and the thread refreshes on a short poll while the screen is open.
class ProviderLiveChatScreen extends StatefulWidget {
  const ProviderLiveChatScreen({super.key, required this.controller});

  final ProviderDisputeController controller;

  @override
  State<ProviderLiveChatScreen> createState() => _ProviderLiveChatScreenState();
}

class _ProviderLiveChatScreenState extends State<ProviderLiveChatScreen> {
  final _inputCtrl = TextEditingController();
  final _scrollCtrl = ScrollController();
  Timer? _pollTimer;
  String? _complaintId;
  int _lastCount = 0;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _openChat());
  }

  Future<void> _openChat() async {
    final complaint = await widget.controller.openSupportChat();
    if (!mounted || complaint == null) return;
    _complaintId = complaint.id;
    await widget.controller.loadMessages(complaint.id);
    if (!mounted) return;
    unawaited(widget.controller.markMessagesRead(complaint.id));
    _scrollToEnd();
    _pollTimer = Timer.periodic(const Duration(seconds: 5), (_) => _refresh());
  }

  Future<void> _refresh() async {
    final id = _complaintId;
    if (id == null) return;
    await widget.controller.loadMessages(id);
    if (!mounted) return;
    final count = widget.controller.messages.length;
    if (count != _lastCount) {
      _lastCount = count;
      unawaited(widget.controller.markMessagesRead(id));
      _scrollToEnd();
    }
  }

  @override
  void dispose() {
    _pollTimer?.cancel();
    _inputCtrl.dispose();
    _scrollCtrl.dispose();
    super.dispose();
  }

  Future<void> _send() async {
    final id = _complaintId;
    final text = _inputCtrl.text.trim();
    if (id == null || text.isEmpty || widget.controller.sending) return;
    _inputCtrl.clear();
    await widget.controller.sendMessage(id, text);
    if (!mounted) return;
    _lastCount = widget.controller.messages.length;
    _scrollToEnd();
  }

  void _scrollToEnd() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scrollCtrl.hasClients) {
        _scrollCtrl.animateTo(_scrollCtrl.position.maxScrollExtent,
            duration: const Duration(milliseconds: 250), curve: Curves.easeOut);
      }
    });
  }

  String _time(DateTime dt) {
    final t = TimeOfDay.fromDateTime(dt);
    final h = t.hourOfPeriod == 0 ? 12 : t.hourOfPeriod;
    final m = t.minute.toString().padLeft(2, '0');
    return '$h:$m ${t.period == DayPeriod.am ? 'AM' : 'PM'}';
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: AnimatedBuilder(
          animation: widget.controller,
          builder: (context, _) {
            final c = widget.controller;
            return Column(
              children: [
                Padding(
                  padding: const EdgeInsets.fromLTRB(20, 16, 20, 8),
                  child: ProviderProfileHeader(title: 'Live Chat'),
                ),
                const SizedBox(height: 8),
                Expanded(child: _body(c)),
                _composer(enabled: _complaintId != null && !c.sending),
              ],
            );
          },
        ),
      ),
    );
  }

  Widget _body(ProviderDisputeController c) {
    if (c.openingSupportChat || (c.loadingMessages && c.messages.isEmpty && _complaintId != null)) {
      return const Center(child: CircularProgressIndicator(color: kProviderGreen));
    }
    if (c.supportChatError != null && _complaintId == null) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              const Icon(Icons.cloud_off_rounded, color: kProviderMuted, size: 40),
              const SizedBox(height: 12),
              Text(
                c.supportChatError!,
                textAlign: TextAlign.center,
                style: const TextStyle(color: kProviderMuted, fontSize: 13),
              ),
              const SizedBox(height: 16),
              OutlinedButton(
                onPressed: _openChat,
                style: OutlinedButton.styleFrom(
                  foregroundColor: kProviderGreen,
                  side: const BorderSide(color: kProviderGreen),
                  shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(30)),
                ),
                child: const Text('Retry'),
              ),
            ],
          ),
        ),
      );
    }
    if (c.messages.isEmpty) {
      return const Center(
        child: Padding(
          padding: EdgeInsets.all(24),
          child: Text(
            'Start the conversation - our support team will reply here.',
            textAlign: TextAlign.center,
            style: TextStyle(color: kProviderMuted, fontSize: 13, height: 1.5),
          ),
        ),
      );
    }
    return ListView.builder(
      controller: _scrollCtrl,
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 8),
      itemCount: c.messages.length,
      itemBuilder: (context, i) => _bubble(c.messages[i]),
    );
  }

  Widget _bubble(DisputeMessage m) {
    final isAgent = !m.isMine;
    final bubbleColor = isAgent ? kProviderGreen : kProviderGreenTint;
    final textColor = isAgent ? Colors.white : kProviderText;
    final avatar = CircleAvatar(
      radius: 16,
      backgroundColor: kProviderGreenTint,
      child: Icon(isAgent ? Icons.support_agent_rounded : Icons.person_rounded, size: 18, color: kProviderGreen),
    );

    final bubble = Flexible(
      child: Container(
        margin: const EdgeInsets.symmetric(horizontal: 8),
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: bubbleColor,
          borderRadius: BorderRadius.only(
            topLeft: const Radius.circular(16),
            topRight: const Radius.circular(16),
            bottomLeft: Radius.circular(isAgent ? 4 : 16),
            bottomRight: Radius.circular(isAgent ? 16 : 4),
          ),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(m.content, style: TextStyle(color: textColor, fontSize: 13, height: 1.4)),
            const SizedBox(height: 4),
            Align(
              alignment: Alignment.centerRight,
              child: Text(_time(m.createdAt), style: TextStyle(color: textColor.withValues(alpha: 0.7), fontSize: 10)),
            ),
          ],
        ),
      ),
    );

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Row(
        mainAxisAlignment: isAgent ? MainAxisAlignment.start : MainAxisAlignment.end,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: isAgent ? [avatar, bubble] : [bubble, avatar],
      ),
    );
  }

  Widget _composer({required bool enabled}) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 12),
      child: Row(
        children: [
          Expanded(
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 12),
              decoration: BoxDecoration(color: kProviderSurface, borderRadius: BorderRadius.circular(28)),
              child: Row(
                children: [
                  const Icon(Icons.emoji_emotions_outlined, color: kProviderMuted, size: 22),
                  const SizedBox(width: 8),
                  Expanded(
                    child: TextField(
                      controller: _inputCtrl,
                      enabled: enabled,
                      onSubmitted: (_) => _send(),
                      decoration: const InputDecoration(
                        hintText: 'Write a message...',
                        hintStyle: TextStyle(color: kProviderMuted, fontSize: 14),
                        border: InputBorder.none,
                      ),
                    ),
                  ),
                  const Icon(Icons.attach_file_rounded, color: kProviderGreen, size: 20),
                  const SizedBox(width: 12),
                  const Icon(Icons.mic_none_rounded, color: kProviderGreen, size: 20),
                ],
              ),
            ),
          ),
          const SizedBox(width: 10),
          GestureDetector(
            onTap: enabled ? _send : null,
            child: Container(
              width: 50,
              height: 50,
              decoration: BoxDecoration(
                color: enabled ? kProviderGreen : kProviderMuted,
                shape: BoxShape.circle,
              ),
              child: const Icon(Icons.send_rounded, color: Colors.white, size: 22),
            ),
          ),
        ],
      ),
    );
  }
}
