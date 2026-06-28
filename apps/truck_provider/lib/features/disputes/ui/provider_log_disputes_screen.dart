import 'package:flutter/material.dart';

import '../../earnings/models/earnings_models.dart';
import '../../earnings/state/provider_earnings_controller.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_dispute_controller.dart';
import 'provider_select_dispute_type_screen.dart';
import 'widgets/dispute_widgets.dart';

/// Log Disputes entry (Figma 2266): pick a transaction to dispute + a feed of
/// past complaints.
class ProviderLogDisputesScreen extends StatefulWidget {
  const ProviderLogDisputesScreen({
    super.key,
    required this.disputeController,
    required this.earningsController,
  });

  final ProviderDisputeController disputeController;
  final ProviderEarningsController earningsController;

  @override
  State<ProviderLogDisputesScreen> createState() => _ProviderLogDisputesScreenState();
}

class _ProviderLogDisputesScreenState extends State<ProviderLogDisputesScreen> {
  @override
  void initState() {
    super.initState();
    widget.disputeController.startNewDispute();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      widget.disputeController.loadComplaints();
      widget.earningsController.load();
    });
  }

  Future<void> _pickTransaction() async {
    final txns = widget.earningsController.earnings.transactions;
    final selected = await showModalBottomSheet<EarningsTransaction>(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.white,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (_) => _TransactionPickerSheet(transactions: txns),
    );
    if (selected != null && mounted) {
      widget.disputeController.selectTransaction(selected);
      Navigator.of(context).push(
        MaterialPageRoute(
          builder: (_) => ProviderSelectDisputeTypeScreen(controller: widget.disputeController),
        ),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: AnimatedBuilder(
        animation: widget.disputeController,
        builder: (context, _) {
          final c = widget.disputeController;
          return SafeArea(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const DisputeAppBar(title: 'Log Disputes'),
                const SizedBox(height: 16),
                const Padding(
                  padding: EdgeInsets.symmetric(horizontal: 20),
                  child: Text(
                    'Select a Transaction',
                    style: TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w700),
                  ),
                ),
                const SizedBox(height: 10),
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 20),
                  child: GestureDetector(
                    onTap: _pickTransaction,
                    child: Container(
                      height: 56,
                      padding: const EdgeInsets.symmetric(horizontal: 16),
                      decoration: BoxDecoration(
                        color: const Color(0xFFE9EBEC),
                        borderRadius: BorderRadius.circular(12),
                      ),
                      child: const Row(
                        children: [
                          Expanded(
                            child: Text(
                              'Select Transaction',
                              style: TextStyle(color: kProviderMuted, fontSize: 15),
                            ),
                          ),
                          Icon(Icons.chevron_right_rounded, color: kProviderMuted),
                        ],
                      ),
                    ),
                  ),
                ),
                const SizedBox(height: 28),
                Expanded(child: _FeedbackSection(controller: c)),
              ],
            ),
          );
        },
      ),
    );
  }
}

class _FeedbackSection extends StatelessWidget {
  const _FeedbackSection({required this.controller});
  final ProviderDisputeController controller;

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: const EdgeInsets.symmetric(horizontal: 20),
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: kProviderBorder),
        boxShadow: const [BoxShadow(color: Color(0x0F000000), blurRadius: 16, offset: Offset(0, 6))],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              const Text(
                'Feedbacks',
                style: TextStyle(color: kProviderText, fontSize: 16, fontWeight: FontWeight.w800),
              ),
              const Text(
                'More',
                style: TextStyle(color: kProviderGreen, fontSize: 13, fontWeight: FontWeight.w600),
              ),
            ],
          ),
          const SizedBox(height: 12),
          Expanded(
            child: controller.loading && controller.complaints.isEmpty
                ? const Center(child: CircularProgressIndicator(color: kProviderGreen))
                : controller.complaints.isEmpty
                    ? const Center(
                        child: Text(
                          'No disputes yet.',
                          style: TextStyle(color: kProviderMuted, fontSize: 13),
                        ),
                      )
                    : ListView.separated(
                        padding: EdgeInsets.zero,
                        itemCount: controller.complaints.length,
                        separatorBuilder: (_, _) => const SizedBox(height: 10),
                        itemBuilder: (context, i) => DisputeFeedbackRow(complaint: controller.complaints[i]),
                      ),
          ),
        ],
      ),
    );
  }
}

// ─── Transaction picker sheet (Frame 1984079940/941) ──────────────────────────

class _TransactionPickerSheet extends StatefulWidget {
  const _TransactionPickerSheet({required this.transactions});
  final List<EarningsTransaction> transactions;

  @override
  State<_TransactionPickerSheet> createState() => _TransactionPickerSheetState();
}

class _TransactionPickerSheetState extends State<_TransactionPickerSheet> {
  EarningsTransaction? _selected;

  @override
  Widget build(BuildContext context) {
    final groups = EarningsTransactionGroup.group(widget.transactions);
    return SafeArea(
      child: Padding(
        padding: EdgeInsets.only(bottom: MediaQuery.of(context).viewInsets.bottom),
        child: ConstrainedBox(
          constraints: BoxConstraints(maxHeight: MediaQuery.of(context).size.height * 0.8),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Padding(
                padding: EdgeInsets.fromLTRB(20, 18, 20, 8),
                child: Align(
                  alignment: Alignment.centerLeft,
                  child: Text(
                    'Select the transaction',
                    style: TextStyle(color: kProviderText, fontSize: 18, fontWeight: FontWeight.w800),
                  ),
                ),
              ),
              Flexible(
                child: widget.transactions.isEmpty
                    ? const Padding(
                        padding: EdgeInsets.all(40),
                        child: Text('No transactions to dispute.',
                            style: TextStyle(color: kProviderMuted, fontSize: 13)),
                      )
                    : ListView(
                        shrinkWrap: true,
                        padding: const EdgeInsets.symmetric(horizontal: 20),
                        children: [
                          for (final group in groups) ...[
                            Padding(
                              padding: const EdgeInsets.symmetric(vertical: 8),
                              child: Text(group.label,
                                  style: const TextStyle(
                                      color: kProviderText, fontSize: 14, fontWeight: FontWeight.w800)),
                            ),
                            for (final txn in group.items)
                              Padding(
                                padding: const EdgeInsets.only(bottom: 10),
                                child: DisputeTransactionCard(
                                  txn: txn,
                                  selected: _selected?.id == txn.id,
                                  onTap: () => setState(() => _selected = txn),
                                ),
                              ),
                          ],
                        ],
                      ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
                child: DisputePrimaryButton(
                  label: 'Confirm',
                  onPressed: _selected == null ? null : () => Navigator.of(context).pop(_selected),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
