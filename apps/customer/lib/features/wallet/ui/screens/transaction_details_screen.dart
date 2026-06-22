import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../../auth/models/customer_auth_models.dart';
import '../../../support/data/support_api.dart';
import '../../../support/ui/support_chat_screen.dart';
import '../../models/wallet_models.dart';
import '../widgets/wallet_flow_scaffold.dart';
import '../widgets/wallet_status_chip.dart';
class TransactionDetailsScreen extends StatelessWidget {
  const TransactionDetailsScreen({
    super.key,
    required this.txn,
    required this.session,
    required this.supportApi,
  });

  final WalletTransaction txn;
  final CustomerSession session;
  final SupportApi supportApi;

  @override
  Widget build(BuildContext context) {
    final color =
        txn.isCredit ? CustomerFigmaColors.primary : const Color(0xFFE53935);

    return WalletFlowScaffold(
      title: 'Transaction Details',
      body: ListView(
        padding: const EdgeInsets.fromLTRB(20, 24, 20, 24),
        children: [
          Center(
            child: Column(
              children: [
                const Text(
                  'Total Amount',
                  style: TextStyle(
                    color: CustomerFigmaColors.muted,
                    fontSize: 14,
                  ),
                ),
                const SizedBox(height: 8),
                Text(
                  txn.formattedAmount,
                  style: TextStyle(
                    color: color,
                    fontSize: 36,
                    fontWeight: FontWeight.w900,
                    letterSpacing: -1,
                  ),
                ),
                const SizedBox(height: 12),
                WalletStatusChip(status: txn.status),
              ],
            ),
          ),
          const SizedBox(height: 32),
          const Text(
            'Transaction Details',
            style: TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 16,
              fontWeight: FontWeight.w800,
            ),
          ),
          const SizedBox(height: 16),
          _DetailRow(
            label: 'Transaction ID',
            value: txn.reference.isNotEmpty
                ? '#${txn.reference.length > 10 ? txn.reference.substring(0, 10) : txn.reference}'
                : '-',
          ),
          const Divider(height: 1, color: CustomerFigmaColors.border),
          _DetailRow(
            label: 'Transaction Amount',
            value: txn.formattedAmount,
            valueColor: color,
          ),
          const Divider(height: 1, color: CustomerFigmaColors.border),
          _DetailRow(
            label: 'Commission',
            value: '-10%',
            valueColor: CustomerFigmaColors.muted,
          ),
          const Divider(height: 1, color: CustomerFigmaColors.border),
          _DetailRow(
            label: 'Total',
            value: txn.formattedAmount,
            bold: true,
          ),
          const SizedBox(height: 32),
          Container(
            padding: const EdgeInsets.all(16),
            decoration: BoxDecoration(
              color: CustomerFigmaColors.primaryTint,
              borderRadius: BorderRadius.circular(16),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text(
                  'Any Question about this transaction?',
                  style: TextStyle(
                    color: CustomerFigmaColors.muted,
                    fontSize: 13,
                  ),
                ),
                const SizedBox(height: 8),
                GestureDetector(
                  onTap: () => Navigator.of(context).push(
                    MaterialPageRoute<void>(
                      builder: (_) => SupportChatScreen(
                        session: session,
                        supportApi: supportApi,
                      ),
                    ),
                  ),
                  child: Row(
                    children: const [
                      Icon(Icons.headset_mic_rounded,
                          color: CustomerFigmaColors.primary, size: 20),
                      SizedBox(width: 8),
                      Text(
                        'Contact Support',
                        style: TextStyle(
                          color: CustomerFigmaColors.primary,
                          fontSize: 14,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                    ],
                  ),
                ),
              ],
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
    this.bold = false,
  });

  final String label;
  final String value;
  final Color? valueColor;
  final bool bold;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 14),
      child: Row(
        children: [
          Text(
            label,
            style: TextStyle(
              color: CustomerFigmaColors.muted,
              fontSize: 13,
              fontWeight: bold ? FontWeight.w700 : FontWeight.w400,
            ),
          ),
          const Spacer(),
          Text(
            value,
            style: TextStyle(
              color: valueColor ?? CustomerFigmaColors.text,
              fontSize: 13,
              fontWeight: FontWeight.w700,
            ),
          ),
        ],
      ),
    );
  }
}
