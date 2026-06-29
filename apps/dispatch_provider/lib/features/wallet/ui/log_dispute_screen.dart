import 'package:flutter/material.dart';
import 'dispute_detail_screen.dart';

class _TxItem {
  const _TxItem({
    required this.title,
    required this.subtitle,
    required this.dateTime,
    required this.amount,
    required this.isCredit,
  });
  final String title;
  final String subtitle;
  final String dateTime;
  final String amount;
  final bool isCredit;
}


const _disputeTypes = [
  'successful withdrawal  not credited to bank',
  'Cancelled trip',
  'over deduction',
  'Pending transaction',
  'Failed transaction',
];

// Backend gap: no provider transaction history endpoint yet.
const Map<String, List<_TxItem>> _txGroups = {};

class LogDisputeScreen extends StatefulWidget {
  const LogDisputeScreen({super.key});

  @override
  State<LogDisputeScreen> createState() => _LogDisputeScreenState();
}

class _LogDisputeScreenState extends State<LogDisputeScreen> {
  _TxItem? _selectedTx;
  String? _selectedDisputeType;

  bool get _canConfirm => _selectedTx != null && _selectedDisputeType != null;

  void _openTxPicker() {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.white,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (_) => _TxPickerSheet(
        groups: _txGroups,
        onSelect: (tx) {
          setState(() => _selectedTx = tx);
          Navigator.of(context).pop();
        },
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final showDisputeType = _selectedTx != null;

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
          'Log Disputes',
          style: TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w700,
            color: Color(0xFF1A1A1A),
          ),
        ),
        centerTitle: false,
      ),
      body: Column(
        children: [
          Expanded(
            child: SingleChildScrollView(
              padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  // ── Select Transaction ──────────────────────────────────
                  const Text(
                    'Select a Transaction',
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 10),
                  GestureDetector(
                    onTap: _openTxPicker,
                    child: Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 16,
                        vertical: 14,
                      ),
                      decoration: BoxDecoration(
                        color: _selectedTx != null
                            ? const Color(0xFFF0FAF0)
                            : const Color(0xFFF5F5F5),
                        borderRadius: BorderRadius.circular(12),
                        border: Border.all(
                          color: _selectedTx != null
                              ? const Color(0xFF4CAF50)
                              : const Color(0xFFDDDDDD),
                        ),
                      ),
                      child: _selectedTx != null
                          ? Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Text(
                                  _selectedTx!.title,
                                  style: const TextStyle(
                                    fontSize: 14,
                                    fontWeight: FontWeight.w700,
                                    color: Color(0xFF1A1A1A),
                                  ),
                                ),
                                const SizedBox(height: 2),
                                Text(
                                  _selectedTx!.dateTime,
                                  style: const TextStyle(
                                    fontSize: 12,
                                    color: Color(0xFF888888),
                                  ),
                                ),
                                const SizedBox(height: 4),
                                Text(
                                  _selectedTx!.amount,
                                  style: TextStyle(
                                    fontSize: 14,
                                    fontWeight: FontWeight.w700,
                                    color: _selectedTx!.isCredit
                                        ? const Color(0xFF4CAF50)
                                        : const Color(0xFFE53935),
                                  ),
                                ),
                              ],
                            )
                          : const Row(
                              mainAxisAlignment: MainAxisAlignment.spaceBetween,
                              children: [
                                Text(
                                  'Select Transaction',
                                  style: TextStyle(
                                    fontSize: 14,
                                    color: Color(0xFF888888),
                                  ),
                                ),
                                Icon(
                                  Icons.chevron_right,
                                  color: Color(0xFF888888),
                                ),
                              ],
                            ),
                    ),
                  ),

                  if (showDisputeType) ...[
                    const SizedBox(height: 24),
                    // ── Select Dispute Type ─────────────────────────────
                    const Text(
                      'Select Dispute Type',
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w700,
                        color: Color(0xFF1A1A1A),
                      ),
                    ),
                    const SizedBox(height: 10),
                    Container(
                      decoration: BoxDecoration(
                        color: Colors.white,
                        borderRadius: BorderRadius.circular(12),
                        border: Border.all(color: const Color(0xFFEEEEEE)),
                      ),
                      child: Column(
                        children: _disputeTypes.asMap().entries.map((e) {
                          final i = e.key;
                          final type = e.value;
                          final isLast = i == _disputeTypes.length - 1;
                          final isSelected = _selectedDisputeType == type;
                          return GestureDetector(
                            onTap: () =>
                                setState(() => _selectedDisputeType = type),
                            child: Container(
                              padding: const EdgeInsets.symmetric(
                                horizontal: 16,
                                vertical: 14,
                              ),
                              decoration: BoxDecoration(
                                color: isSelected
                                    ? const Color(0xFFF0FAF0)
                                    : Colors.transparent,
                                border: isLast
                                    ? null
                                    : const Border(
                                        bottom: BorderSide(
                                          color: Color(0xFFF0F0F0),
                                          width: 1,
                                        ),
                                      ),
                              ),
                              child: Row(
                                children: [
                                  Expanded(
                                    child: Text(
                                      type,
                                      style: TextStyle(
                                        fontSize: 14,
                                        color: isSelected
                                            ? const Color(0xFF4CAF50)
                                            : const Color(0xFF1A1A1A),
                                      ),
                                    ),
                                  ),
                                  if (isSelected)
                                    const Icon(
                                      Icons.check,
                                      size: 16,
                                      color: Color(0xFF4CAF50),
                                    ),
                                ],
                              ),
                            ),
                          );
                        }).toList(),
                      ),
                    ),
                  ],

                ],
              ),
            ),
          ),

          // ── Confirm button ────────────────────────────────────────────
          SafeArea(
            child: Padding(
              padding: const EdgeInsets.fromLTRB(20, 12, 20, 16),
              child: SizedBox(
                width: double.infinity,
                height: 52,
                child: FilledButton(
                  onPressed: _canConfirm
                      ? () => Navigator.of(context).push(
                          MaterialPageRoute(
                            builder: (_) => DisputeDetailScreen(
                              disputeType: _selectedDisputeType!,
                              transaction: _selectedTx!,
                            ),
                          ),
                        )
                      : null,
                  style: FilledButton.styleFrom(
                    backgroundColor: const Color(0xFF4CAF50),
                    disabledBackgroundColor: const Color(
                      0xFF4CAF50,
                    ).withValues(alpha: 0.4),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(999),
                    ),
                  ),
                  child: const Text(
                    'Confirm',
                    style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700),
                  ),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _TxPickerSheet extends StatelessWidget {
  const _TxPickerSheet({required this.groups, required this.onSelect});
  final Map<String, List<_TxItem>> groups;
  final ValueChanged<_TxItem> onSelect;

  @override
  Widget build(BuildContext context) {
    return DraggableScrollableSheet(
      initialChildSize: 0.7,
      maxChildSize: 0.9,
      minChildSize: 0.5,
      expand: false,
      builder: (_, controller) => Column(
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 16, 20, 8),
            child: Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                const Text(
                  'Select the transaction',
                  style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700),
                ),
                GestureDetector(
                  onTap: () => Navigator.of(context).pop(),
                  child: const Icon(Icons.close, size: 22),
                ),
              ],
            ),
          ),
          Expanded(
            child: ListView(
              controller: controller,
              padding: const EdgeInsets.fromLTRB(0, 0, 0, 80),
              children: groups.entries.expand((entry) {
                return [
                  Padding(
                    padding: const EdgeInsets.fromLTRB(20, 12, 20, 8),
                    child: Row(
                      children: [
                        Text(
                          entry.key,
                          style: const TextStyle(
                            fontSize: 14,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                        const SizedBox(width: 4),
                        const Icon(Icons.arrow_drop_down, size: 18),
                      ],
                    ),
                  ),
                  ...entry.value.map(
                    (tx) => GestureDetector(
                      onTap: () => onSelect(tx),
                      child: Container(
                        padding: const EdgeInsets.fromLTRB(20, 12, 20, 12),
                        decoration: const BoxDecoration(
                          border: Border(
                            bottom: BorderSide(
                              color: Color(0xFFF5F5F5),
                              width: 1,
                            ),
                          ),
                        ),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              tx.title,
                              style: const TextStyle(
                                fontSize: 14,
                                fontWeight: FontWeight.w700,
                                color: Color(0xFF1A1A1A),
                              ),
                            ),
                            if (tx.subtitle.isNotEmpty) ...[
                              const SizedBox(height: 2),
                              Text(
                                tx.subtitle,
                                style: const TextStyle(
                                  fontSize: 12,
                                  color: Color(0xFF888888),
                                ),
                              ),
                            ],
                            const SizedBox(height: 2),
                            Text(
                              tx.dateTime,
                              style: const TextStyle(
                                fontSize: 12,
                                color: Color(0xFF888888),
                              ),
                            ),
                            const SizedBox(height: 4),
                            Text(
                              tx.amount,
                              style: TextStyle(
                                fontSize: 14,
                                fontWeight: FontWeight.w700,
                                color: tx.isCredit
                                    ? const Color(0xFF4CAF50)
                                    : const Color(0xFFE53935),
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                  ),
                ];
              }).toList(),
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 8, 20, 20),
            child: SizedBox(
              width: double.infinity,
              height: 52,
              child: FilledButton(
                onPressed: null,
                style: FilledButton.styleFrom(
                  backgroundColor: const Color(
                    0xFF4CAF50,
                  ).withValues(alpha: 0.4),
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(999),
                  ),
                ),
                child: const Text(
                  'Confirm',
                  style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
