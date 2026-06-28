import 'package:flutter/material.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import 'widgets/provider_profile_widgets.dart';

/// Contact Support (Account option-4). "Start Chat" opens the Live Chat screen.
class ProviderSupportScreen extends StatelessWidget {
  const ProviderSupportScreen({super.key});

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
                    MaterialPageRoute(builder: (_) => const ProviderLiveChatScreen()),
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

class _ChatMessage {
  const _ChatMessage({required this.text, required this.time, required this.isAgent});
  final String text;
  final String time;
  final bool isAgent;
}

class ProviderLiveChatScreen extends StatefulWidget {
  const ProviderLiveChatScreen({super.key});

  @override
  State<ProviderLiveChatScreen> createState() => _ProviderLiveChatScreenState();
}

class _ProviderLiveChatScreenState extends State<ProviderLiveChatScreen> {
  final _inputCtrl = TextEditingController();
  final _scrollCtrl = ScrollController();

  final List<_ChatMessage> _messages = [
    const _ChatMessage(text: 'Hello, how can i help you today?', time: '12:15 PM', isAgent: true),
    const _ChatMessage(text: 'Hello, I have repeatedly tried booking a ride with add stop but its failing.', time: '12:12 PM', isAgent: false),
    const _ChatMessage(text: 'Hello, when did you book the ride?', time: '12:15 PM', isAgent: true),
  ];

  @override
  void dispose() {
    _inputCtrl.dispose();
    _scrollCtrl.dispose();
    super.dispose();
  }

  void _send() {
    final text = _inputCtrl.text.trim();
    if (text.isEmpty) return;
    setState(() {
      _messages.add(_ChatMessage(text: text, time: _now(), isAgent: false));
      _inputCtrl.clear();
    });
    _scrollToEnd();
    // Local canned agent acknowledgement (real-time chat requires the
    // support-dispute service; this mirrors the Figma conversation UI).
    Future.delayed(const Duration(seconds: 1), () {
      if (!mounted) return;
      setState(() => _messages.add(_ChatMessage(
            text: 'Thanks for the details, an agent will be with you shortly.',
            time: _now(),
            isAgent: true,
          )));
      _scrollToEnd();
    });
  }

  String _now() {
    final t = TimeOfDay.now();
    final h = t.hourOfPeriod == 0 ? 12 : t.hourOfPeriod;
    final m = t.minute.toString().padLeft(2, '0');
    return '$h:$m ${t.period == DayPeriod.am ? 'AM' : 'PM'}';
  }

  void _scrollToEnd() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scrollCtrl.hasClients) {
        _scrollCtrl.animateTo(_scrollCtrl.position.maxScrollExtent,
            duration: const Duration(milliseconds: 250), curve: Curves.easeOut);
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 16, 20, 8),
              child: ProviderProfileHeader(title: 'Live Chat'),
            ),
            const SizedBox(height: 8),
            const Text('Sunday at 4:20 PM', style: TextStyle(color: kProviderMuted, fontSize: 12)),
            const SizedBox(height: 8),
            Expanded(
              child: ListView.builder(
                controller: _scrollCtrl,
                padding: const EdgeInsets.fromLTRB(16, 8, 16, 8),
                itemCount: _messages.length,
                itemBuilder: (context, i) => _bubble(_messages[i]),
              ),
            ),
            _composer(),
          ],
        ),
      ),
    );
  }

  Widget _bubble(_ChatMessage m) {
    final bubbleColor = m.isAgent ? kProviderGreen : kProviderGreenTint;
    final textColor = m.isAgent ? Colors.white : kProviderText;
    final avatar = CircleAvatar(
      radius: 16,
      backgroundColor: kProviderGreenTint,
      child: Icon(m.isAgent ? Icons.support_agent_rounded : Icons.person_rounded, size: 18, color: kProviderGreen),
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
            bottomLeft: Radius.circular(m.isAgent ? 4 : 16),
            bottomRight: Radius.circular(m.isAgent ? 16 : 4),
          ),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(m.text, style: TextStyle(color: textColor, fontSize: 13, height: 1.4)),
            const SizedBox(height: 4),
            Align(
              alignment: Alignment.centerRight,
              child: Text(m.time, style: TextStyle(color: textColor.withValues(alpha: 0.7), fontSize: 10)),
            ),
          ],
        ),
      ),
    );

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Row(
        mainAxisAlignment: m.isAgent ? MainAxisAlignment.start : MainAxisAlignment.end,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: m.isAgent ? [avatar, bubble] : [bubble, avatar],
      ),
    );
  }

  Widget _composer() {
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
            onTap: _send,
            child: Container(
              width: 50,
              height: 50,
              decoration: const BoxDecoration(color: kProviderGreen, shape: BoxShape.circle),
              child: const Icon(Icons.send_rounded, color: Colors.white, size: 22),
            ),
          ),
        ],
      ),
    );
  }
}
