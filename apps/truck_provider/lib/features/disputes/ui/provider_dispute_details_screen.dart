import 'package:flutter/material.dart';

import '../../../core/format/money_format.dart';
import '../../earnings/models/earnings_models.dart';
import '../../earnings/ui/widgets/earnings_transaction_list.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../models/dispute_models.dart';
import '../state/provider_dispute_controller.dart';
import 'provider_dispute_chat_screen.dart';
import 'widgets/dispute_widgets.dart';

/// Dispute Details + status timeline (Figma 2269 / 2270).
class ProviderDisputeDetailsScreen extends StatelessWidget {
  const ProviderDisputeDetailsScreen({
    super.key,
    required this.controller,
    required this.complaint,
  });

  final ProviderDisputeController controller;
  final Complaint complaint;

  void _openChat(BuildContext context) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProviderDisputeChatScreen(controller: controller, complaint: complaint),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final txn = controller.selectedTransaction;
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Column(
          children: [
            DisputeAppBar(
              title: 'Disputes Details',
              trailing: GestureDetector(
                onTap: () => _openChat(context),
                behavior: HitTestBehavior.opaque,
                child: const Padding(
                  padding: EdgeInsets.only(right: 4),
                  child: Text(
                    'Live Chat',
                    style: TextStyle(color: kProviderGreen, fontSize: 14, fontWeight: FontWeight.w700),
                  ),
                ),
              ),
            ),
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 16, 20, 16),
                children: [
                  const Text('Dispute Type',
                      style: TextStyle(color: kProviderText, fontSize: 16, fontWeight: FontWeight.w800)),
                  const SizedBox(height: 10),
                  _TypePill(label: complaint.subject),
                  const SizedBox(height: 22),
                  if (txn != null) _TransactionSummary(txn: txn),
                  const SizedBox(height: 24),
                  _StatusStepper(stage: complaint.stage, date: complaint.createdAt, resolvedAt: complaint.resolvedAt),
                  const SizedBox(height: 24),
                  const Text('Processing Record',
                      style: TextStyle(color: kProviderText, fontSize: 16, fontWeight: FontWeight.w800)),
                  const SizedBox(height: 14),
                  _ProcessingRecord(complaint: complaint),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: DisputePrimaryButton(label: 'Contact Support', onPressed: () => _openChat(context)),
            ),
          ],
        ),
      ),
    );
  }
}

class _TypePill extends StatelessWidget {
  const _TypePill({required this.label});
  final String label;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
      decoration: BoxDecoration(
        color: kProviderGreenPale,
        borderRadius: BorderRadius.circular(30),
      ),
      child: Text(
        label.isEmpty ? 'Dispute' : label,
        style: const TextStyle(color: kProviderDarkGreen, fontSize: 14, fontWeight: FontWeight.w600),
      ),
    );
  }
}

class _TransactionSummary extends StatelessWidget {
  const _TransactionSummary({required this.txn});
  final EarningsTransaction txn;

  static const _commissionRate = 0.10;

  ({String label, Color color}) get _status {
    switch (txn.status) {
      case EarningsTransaction.statusPending:
        return (label: 'Pending', color: const Color(0xFFE8A21B));
      case EarningsTransaction.statusFailed:
        return (label: 'Failed', color: kProviderRejectText);
      default:
        return (label: 'Successful', color: kProviderGreen);
    }
  }

  @override
  Widget build(BuildContext context) {
    final status = _status;
    final gross = txn.amountNaira.abs();
    final net = gross - gross * _commissionRate;
    final sign = txn.isCredit ? '+' : '-';

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Center(
          child: Column(
            children: [
              const Text('Total Amount', style: TextStyle(color: kProviderMuted, fontSize: 13)),
              const SizedBox(height: 8),
              Text('$sign₦${formatNaira(gross)}',
                  style: TextStyle(color: status.color, fontSize: 26, fontWeight: FontWeight.w800)),
              const SizedBox(height: 10),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 6),
                decoration: BoxDecoration(
                  color: status.color.withValues(alpha: 0.14),
                  borderRadius: BorderRadius.circular(20),
                ),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Container(width: 7, height: 7, decoration: BoxDecoration(color: status.color, shape: BoxShape.circle)),
                    const SizedBox(width: 8),
                    Text(status.label, style: TextStyle(color: status.color, fontSize: 13, fontWeight: FontWeight.w700)),
                  ],
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 20),
        const Text('Transaction Details',
            style: TextStyle(color: kProviderText, fontSize: 16, fontWeight: FontWeight.w800)),
        const SizedBox(height: 12),
        _row('Transaction ID', '#${_ref(txn.id)}'),
        _row('Transaction Amount', '$sign₦${formatNaira(gross)}', color: status.color),
        _row('Commission', '-10%', color: kProviderRejectText),
        _row('Total', '₦${formatNaira(net)}', bold: true),
      ],
    );
  }

  static String _ref(String id) {
    final clean = id.replaceAll('-', '');
    return clean.length >= 9 ? clean.substring(0, 9) : clean;
  }

  Widget _row(String label, String value, {Color? color, bool bold = false}) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Expanded(child: Text(label, style: const TextStyle(color: kProviderMuted, fontSize: 14))),
          const SizedBox(width: 12),
          Text(value,
              style: TextStyle(
                  color: color ?? kProviderText, fontSize: 14, fontWeight: bold ? FontWeight.w800 : FontWeight.w600)),
        ],
      ),
    );
  }
}

/// Horizontal Submitted — Processing — Completed stepper (Figma 2270).
class _StatusStepper extends StatelessWidget {
  const _StatusStepper({required this.stage, required this.date, this.resolvedAt});
  final int stage;
  final DateTime date;
  final DateTime? resolvedAt;

  @override
  Widget build(BuildContext context) {
    final steps = ['Submitted', 'Processing', 'Completed'];
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 16),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: kProviderBorder),
        boxShadow: const [BoxShadow(color: Color(0x0A000000), blurRadius: 12, offset: Offset(0, 4))],
      ),
      child: Column(
        children: [
          const Align(
            alignment: Alignment.centerLeft,
            child: Text('Status', style: TextStyle(color: kProviderText, fontSize: 15, fontWeight: FontWeight.w800)),
          ),
          const SizedBox(height: 14),
          Row(
            children: [
              for (var i = 0; i < steps.length; i++) ...[
                Text(
                  steps[i],
                  style: TextStyle(
                    color: i <= stage ? kProviderGreen : kProviderMuted,
                    fontSize: 12.5,
                    fontWeight: FontWeight.w700,
                  ),
                ),
                if (i < steps.length - 1)
                  Expanded(
                    child: Container(
                      height: 2,
                      margin: const EdgeInsets.symmetric(horizontal: 6),
                      color: i < stage ? kProviderGreen : kProviderBorder,
                    ),
                  ),
              ],
            ],
          ),
        ],
      ),
    );
  }
}

/// Vertical processing record timeline (Figma 2270).
class _ProcessingRecord extends StatelessWidget {
  const _ProcessingRecord({required this.complaint});
  final Complaint complaint;

  @override
  Widget build(BuildContext context) {
    final entries = <({String title, DateTime? when, bool done})>[
      (title: 'Submitted', when: complaint.createdAt, done: true),
      (title: 'Processing', when: complaint.stage >= 1 ? complaint.createdAt : null, done: complaint.stage >= 1),
      (title: 'Completed', when: complaint.resolvedAt, done: complaint.stage >= 2),
    ];

    return Column(
      children: [
        for (var i = 0; i < entries.length; i++)
          _RecordRow(
            title: entries[i].title,
            when: entries[i].when,
            done: entries[i].done,
            isLast: i == entries.length - 1,
          ),
      ],
    );
  }
}

class _RecordRow extends StatelessWidget {
  const _RecordRow({required this.title, required this.when, required this.done, required this.isLast});
  final String title;
  final DateTime? when;
  final bool done;
  final bool isLast;

  @override
  Widget build(BuildContext context) {
    return IntrinsicHeight(
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Column(
            children: [
              Container(
                width: 14,
                height: 14,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: done ? kProviderGreen : Colors.white,
                  border: Border.all(color: done ? kProviderGreen : kProviderBorder, width: 2),
                ),
              ),
              if (!isLast)
                Expanded(child: Container(width: 2, color: done ? kProviderGreen : kProviderBorder)),
            ],
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Padding(
              padding: const EdgeInsets.only(bottom: 18),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(title, style: const TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w800)),
                  const SizedBox(height: 2),
                  Text(
                    when != null ? formatTransactionDate(when!) : 'Pending',
                    style: const TextStyle(color: kProviderMuted, fontSize: 12.5),
                  ),
                  const SizedBox(height: 6),
                  const Text(
                    'Record note here…',
                    style: TextStyle(color: kProviderMuted, fontSize: 12.5),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}
