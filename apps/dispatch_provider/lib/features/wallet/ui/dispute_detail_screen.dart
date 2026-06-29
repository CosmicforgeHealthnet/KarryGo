import 'package:flutter/material.dart';

class DisputeDetailScreen extends StatelessWidget {
  const DisputeDetailScreen({
    super.key,
    required this.disputeType,
    required this.transaction,
  });

  final String disputeType;
  final dynamic transaction; // _TxItem from log_dispute_screen

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
          'Disputes Details',
          style: TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w700,
            color: Color(0xFF1A1A1A),
          ),
        ),
        centerTitle: false,
        actions: [
          TextButton(
            onPressed: () {},
            child: Text(
              'Live Chat',
              style: TextStyle(
                fontSize: 14,
                fontWeight: FontWeight.w600,
                color: Colors.green.shade600,
              ),
            ),
          ),
        ],
      ),
      body: Column(
        children: [
          Expanded(
            child: SingleChildScrollView(
              padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  // ── Dispute Type ────────────────────────────────────────
                  const Text(
                    'Dispute Type',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 10),
                  Container(
                    width: double.infinity,
                    padding: const EdgeInsets.symmetric(
                      horizontal: 16,
                      vertical: 14,
                    ),
                    decoration: BoxDecoration(
                      color: const Color(0xFFE8F5E9),
                      borderRadius: BorderRadius.circular(12),
                    ),
                    child: Text(
                      disputeType,
                      style: const TextStyle(
                        fontSize: 14,
                        color: Color(0xFF2E7D32),
                      ),
                    ),
                  ),

                  const SizedBox(height: 24),

                  // ── Transaction Details (amount) ─────────────────────────
                  const Text(
                    'Transaction Details',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 16),
                  const Center(
                    child: Text(
                      'Total Amount',
                      style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
                    ),
                  ),
                  const SizedBox(height: 6),
                  const Center(
                    child: Text(
                      '-₦2,400.00',
                      style: TextStyle(
                        fontSize: 28,
                        fontWeight: FontWeight.w800,
                        color: Color(0xFFE53935),
                      ),
                    ),
                  ),
                  const SizedBox(height: 10),
                  Center(
                    child: Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 14,
                        vertical: 6,
                      ),
                      decoration: BoxDecoration(
                        color: const Color(0xFF4CAF50),
                        borderRadius: BorderRadius.circular(999),
                      ),
                      child: const Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          Icon(Icons.circle, size: 8, color: Colors.white),
                          SizedBox(width: 6),
                          Text(
                            'Successful',
                            style: TextStyle(
                              fontSize: 13,
                              fontWeight: FontWeight.w600,
                              color: Colors.white,
                            ),
                          ),
                        ],
                      ),
                    ),
                  ),

                  const SizedBox(height: 24),

                  // ── Transaction Details table ─────────────────────────────
                  const Text(
                    'Transaction Details',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 12),
                  _DetailRow(label: 'Transaction ID', value: '#123569876'),
                  _DetailRow(
                    label: 'Transaction Amount',
                    value: '-₦2,400.00',
                    valueColor: const Color(0xFFE53935),
                  ),
                  _DetailRow(
                    label: 'Commission',
                    value: '-10%',
                    valueColor: const Color(0xFFE53935),
                  ),
                  _DetailRow(label: 'Total', value: '-₦2,400.00', isBold: true),

                  const SizedBox(height: 24),

                  // ── Status timeline ───────────────────────────────────────
                  Container(
                    padding: const EdgeInsets.all(16),
                    decoration: BoxDecoration(
                      color: const Color(0xFFF9F9F9),
                      borderRadius: BorderRadius.circular(12),
                      border: Border.all(color: const Color(0xFFEEEEEE)),
                    ),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const Text(
                          'Status',
                          style: TextStyle(
                            fontSize: 14,
                            fontWeight: FontWeight.w700,
                            color: Color(0xFF1A1A1A),
                          ),
                        ),
                        const SizedBox(height: 16),
                        Row(
                          children: [
                            _StatusStep(
                              label: 'Submitted',
                              date: '27th July 2023,',
                              time: '12:09:39',
                              isActive: true,
                            ),
                            _StatusConnector(isActive: true),
                            _StatusStep(
                              label: 'Processing',
                              date: '27th July 2023,',
                              time: '12:09:39',
                              isActive: false,
                            ),
                            _StatusConnector(isActive: false),
                            _StatusStep(
                              label: 'Completed',
                              date: '27th July 2023,',
                              time: '12:09:39',
                              isActive: false,
                            ),
                          ],
                        ),
                      ],
                    ),
                  ),

                  const SizedBox(height: 24),

                  // ── Processing Record ─────────────────────────────────────
                  const Text(
                    'Processing Record',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 16),
                  _ProcessingRecord(
                    items: const [
                      _RecordItem(
                        label: 'Submitted',
                        date: '27th July 2023, 12:09:39',
                        note: 'Record note here.......Lorem Ipsum',
                        isActive: true,
                      ),
                      _RecordItem(
                        label: 'Processing',
                        date: '27th July 2023, 12:09:39',
                        note: 'Record note here.......Lorem Ipsum',
                        isActive: false,
                      ),
                      _RecordItem(
                        label: 'Completed',
                        date: '27th July 2023, 12:09:39',
                        note: 'Record note here.......Lorem Ipsum',
                        isActive: false,
                        isLast: true,
                      ),
                    ],
                  ),
                ],
              ),
            ),
          ),

          // ── Contact Support button ─────────────────────────────────────
          SafeArea(
            child: Padding(
              padding: const EdgeInsets.fromLTRB(20, 12, 20, 16),
              child: SizedBox(
                width: double.infinity,
                height: 52,
                child: FilledButton(
                  onPressed: () {},
                  style: FilledButton.styleFrom(
                    backgroundColor: const Color(0xFF4CAF50),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(999),
                    ),
                  ),
                  child: const Text(
                    'Contact Support',
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

class _DetailRow extends StatelessWidget {
  const _DetailRow({
    required this.label,
    required this.value,
    this.valueColor,
    this.isBold = false,
  });
  final String label;
  final String value;
  final Color? valueColor;
  final bool isBold;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 10),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Text(
            label,
            style: const TextStyle(fontSize: 14, color: Color(0xFF555555)),
          ),
          Text(
            value,
            style: TextStyle(
              fontSize: 14,
              fontWeight: isBold ? FontWeight.w700 : FontWeight.w500,
              color: valueColor ?? const Color(0xFF1A1A1A),
            ),
          ),
        ],
      ),
    );
  }
}

class _StatusStep extends StatelessWidget {
  const _StatusStep({
    required this.label,
    required this.date,
    required this.time,
    required this.isActive,
  });
  final String label;
  final String date;
  final String time;
  final bool isActive;

  @override
  Widget build(BuildContext context) {
    return Expanded(
      child: Column(
        children: [
          Text(
            label,
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w600,
              color: isActive
                  ? const Color(0xFF4CAF50)
                  : const Color(0xFF888888),
            ),
          ),
          const SizedBox(height: 4),
          Text(
            date,
            textAlign: TextAlign.center,
            style: const TextStyle(fontSize: 10, color: Color(0xFF888888)),
          ),
          Text(
            time,
            textAlign: TextAlign.center,
            style: const TextStyle(fontSize: 10, color: Color(0xFF888888)),
          ),
        ],
      ),
    );
  }
}

class _StatusConnector extends StatelessWidget {
  const _StatusConnector({required this.isActive});
  final bool isActive;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 24,
      height: 1,
      color: isActive ? const Color(0xFF4CAF50) : const Color(0xFFDDDDDD),
    );
  }
}

class _RecordItem {
  const _RecordItem({
    required this.label,
    required this.date,
    required this.note,
    required this.isActive,
    this.isLast = false,
  });
  final String label;
  final String date;
  final String note;
  final bool isActive;
  final bool isLast;
}

class _ProcessingRecord extends StatelessWidget {
  const _ProcessingRecord({required this.items});
  final List<_RecordItem> items;

  @override
  Widget build(BuildContext context) {
    return Column(
      children: items.map((item) {
        return Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            SizedBox(
              width: 20,
              child: Column(
                children: [
                  Container(
                    width: 10,
                    height: 10,
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      color: item.isActive
                          ? const Color(0xFF4CAF50)
                          : Colors.white,
                      border: Border.all(
                        color: item.isActive
                            ? const Color(0xFF4CAF50)
                            : const Color(0xFFDDDDDD),
                        width: 2,
                      ),
                    ),
                  ),
                  if (!item.isLast)
                    Container(
                      width: 1.5,
                      height: 60,
                      color: const Color(0xFFDDDDDD),
                    ),
                ],
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Padding(
                padding: const EdgeInsets.only(bottom: 24),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      item.label,
                      style: const TextStyle(
                        fontSize: 14,
                        fontWeight: FontWeight.w700,
                        color: Color(0xFF1A1A1A),
                      ),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      item.date,
                      style: const TextStyle(
                        fontSize: 12,
                        color: Color(0xFF888888),
                      ),
                    ),
                    const SizedBox(height: 8),
                    Text(
                      item.note,
                      style: const TextStyle(
                        fontSize: 13,
                        color: Color(0xFF555555),
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ],
        );
      }).toList(),
    );
  }
}
