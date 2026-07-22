import 'package:flutter/material.dart';

import '../../disputes/models/dispute_models.dart';
import '../../disputes/state/rider_dispute_controller.dart';
import '../../disputes/ui/dispute_chat_screen.dart';

/// Dispute detail: status timeline (Submitted → Processing → Completed),
/// dispute info, and the live chat with support.
class DisputeDetailScreen extends StatelessWidget {
  const DisputeDetailScreen({
    super.key,
    required this.complaint,
    required this.disputeController,
  });

  final Complaint complaint;
  final RiderDisputeController disputeController;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      appBar: AppBar(
        backgroundColor: Colors.white,
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
          'Dispute Details',
          style: TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w700,
            color: Color(0xFF1A1A1A),
          ),
        ),
        centerTitle: false,
      ),
      body: SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _StatusStepper(stage: complaint.stage),
            const SizedBox(height: 24),
            const Text(
              'Dispute Information',
              style: TextStyle(
                fontSize: 15,
                fontWeight: FontWeight.w700,
                color: Color(0xFF1A1A1A),
              ),
            ),
            const SizedBox(height: 10),
            Container(
              width: double.infinity,
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: const Color(0xFFF9F9F9),
                borderRadius: BorderRadius.circular(12),
                border: Border.all(color: const Color(0xFFEEEEEE)),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  _InfoRow(label: 'Dispute Type', value: complaint.subject),
                  if (complaint.bookingReference.isNotEmpty)
                    _InfoRow(
                      label: 'Trip Reference',
                      value: complaint.bookingReference,
                    ),
                  _InfoRow(
                    label: 'Date Submitted',
                    value: _formatDate(complaint.createdAt),
                  ),
                  _InfoRow(label: 'Status', value: complaint.statusLabel),
                  if (complaint.resolvedAt != null)
                    _InfoRow(
                      label: 'Resolved',
                      value: _formatDate(complaint.resolvedAt!),
                    ),
                ],
              ),
            ),
            const SizedBox(height: 24),
            SizedBox(
              width: double.infinity,
              height: 52,
              child: FilledButton.icon(
                onPressed: () => Navigator.of(context).push(
                  MaterialPageRoute(
                    builder: (_) => DisputeChatScreen(
                      complaint: complaint,
                      controller: disputeController,
                      title: 'Dispute Chat',
                    ),
                  ),
                ),
                style: FilledButton.styleFrom(
                  backgroundColor: const Color(0xFF4CAF50),
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(999),
                  ),
                ),
                icon: const Icon(
                  Icons.chat_bubble_outline,
                  size: 18,
                  color: Colors.white,
                ),
                label: const Text(
                  'Chat with Support',
                  style: TextStyle(fontSize: 15, fontWeight: FontWeight.w700),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  String _formatDate(DateTime time) {
    const months = [
      'Jan',
      'Feb',
      'Mar',
      'Apr',
      'May',
      'Jun',
      'Jul',
      'Aug',
      'Sep',
      'Oct',
      'Nov',
      'Dec',
    ];
    return '${time.day} ${months[time.month - 1]} ${time.year}';
  }
}

class _StatusStepper extends StatelessWidget {
  const _StatusStepper({required this.stage});

  final int stage;

  static const _labels = ['Submitted', 'Processing', 'Completed'];

  @override
  Widget build(BuildContext context) {
    return Row(
      children: List.generate(_labels.length * 2 - 1, (i) {
        if (i.isOdd) {
          final done = stage >= (i + 1) ~/ 2;
          return Expanded(
            child: Container(
              height: 3,
              margin: const EdgeInsets.only(bottom: 22),
              color: done ? const Color(0xFF4CAF50) : const Color(0xFFE0E0E0),
            ),
          );
        }
        final index = i ~/ 2;
        final reached = stage >= index;
        return Column(
          children: [
            Container(
              width: 28,
              height: 28,
              decoration: BoxDecoration(
                color: reached ? const Color(0xFF4CAF50) : Colors.white,
                shape: BoxShape.circle,
                border: Border.all(
                  color: reached
                      ? const Color(0xFF4CAF50)
                      : const Color(0xFFE0E0E0),
                  width: 2,
                ),
              ),
              child: reached
                  ? const Icon(Icons.check, size: 16, color: Colors.white)
                  : null,
            ),
            const SizedBox(height: 6),
            Text(
              _labels[index],
              style: TextStyle(
                fontSize: 11,
                fontWeight: reached ? FontWeight.w700 : FontWeight.w400,
                color: reached
                    ? const Color(0xFF1A1A1A)
                    : const Color(0xFF888888),
              ),
            ),
          ],
        );
      }),
    );
  }
}

class _InfoRow extends StatelessWidget {
  const _InfoRow({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 120,
            child: Text(
              label,
              style: const TextStyle(fontSize: 13, color: Color(0xFF888888)),
            ),
          ),
          Expanded(
            child: Text(
              value,
              style: const TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w600,
                color: Color(0xFF1A1A1A),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
