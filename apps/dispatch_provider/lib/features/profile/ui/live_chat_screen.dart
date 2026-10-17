import '../../../core/theme/theme_extensions.dart';
import 'package:flutter/material.dart';

import '../../disputes/state/rider_dispute_controller.dart';
import '../../disputes/ui/dispute_chat_screen.dart';

/// Live chat with support. Finds (or creates) the rider's open support-chat
/// complaint on support-dispute-service, then opens the shared real chat
/// screen — agent replies arrive from the admin surface.
class LiveChatScreen extends StatefulWidget {
  const LiveChatScreen({super.key, required this.disputeController});

  final RiderDisputeController disputeController;

  @override
  State<LiveChatScreen> createState() => _LiveChatScreenState();
}

class _LiveChatScreenState extends State<LiveChatScreen> {
  @override
  void initState() {
    super.initState();
    _open();
  }

  Future<void> _open() async {
    final complaint = await widget.disputeController.openSupportChat();
    if (!mounted || complaint == null) return;
    Navigator.of(context).pushReplacement(
      MaterialPageRoute(
        builder: (_) => DisputeChatScreen(
          complaint: complaint,
          controller: widget.disputeController,
          title: 'Live Chat',
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: context.palette.surface,
      appBar: AppBar(
        backgroundColor: context.palette.surface,
        elevation: 0,
        leading: GestureDetector(
          onTap: () => Navigator.of(context).pop(),
          behavior: HitTestBehavior.opaque,
          child: const Padding(
            padding: EdgeInsets.all(12),
            child: Icon(
              Icons.arrow_back_ios_new,
              size: 18,
              color: Color(0xFF1A1A1A),
            ),
          ),
        ),
        title: const Text(
          'Live Chat',
          style: TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w700,
            color: Color(0xFF1A1A1A),
          ),
        ),
        centerTitle: false,
      ),
      body: AnimatedBuilder(
        animation: widget.disputeController,
        builder: (context, _) {
          final error = widget.disputeController.supportChatError;
          if (error != null) {
            return Center(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  const Icon(
                    Icons.cloud_off_rounded,
                    size: 48,
                    color: Color(0xFF888888),
                  ),
                  const SizedBox(height: 12),
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 32),
                    child: Text(
                      error,
                      textAlign: TextAlign.center,
                      style: const TextStyle(
                        fontSize: 13,
                        color: Color(0xFF888888),
                        height: 1.5,
                      ),
                    ),
                  ),
                  const SizedBox(height: 16),
                  TextButton(
                    onPressed: _open,
                    child: const Text(
                      'Try again',
                      style: TextStyle(
                        color: Color(0xFF05124D),
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                  ),
                ],
              ),
            );
          }
          return const Center(
            child: CircularProgressIndicator(color: Color(0xFF05124D)),
          );
        },
      ),
    );
  }
}
