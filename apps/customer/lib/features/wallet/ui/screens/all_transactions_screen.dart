import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../../auth/models/customer_auth_models.dart';
import '../../../support/data/support_api.dart';
import '../../models/wallet_models.dart';
import '../widgets/wallet_flow_scaffold.dart';
import '../widgets/wallet_transaction_tile.dart';
import 'transaction_details_screen.dart';

class AllTransactionsScreen extends StatelessWidget {
  const AllTransactionsScreen({
    super.key,
    required this.transactions,
    required this.session,
    required this.supportApi,
  });

  final List<WalletTransaction> transactions;
  final CustomerSession session;
  final SupportApi supportApi;

  @override
  Widget build(BuildContext context) {
    return WalletFlowScaffold(
      title: 'All Transactions',
      body: transactions.isEmpty
          ? const _Empty()
          : _GroupedList(
              transactions: transactions,
              onTap: (txn) => Navigator.of(context).push(
                MaterialPageRoute<void>(
                  builder: (_) => TransactionDetailsScreen(
                    txn: txn,
                    session: session,
                    supportApi: supportApi,
                  ),
                ),
              ),
            ),
    );
  }
}

class _GroupedList extends StatelessWidget {
  const _GroupedList({required this.transactions, required this.onTap});
  final List<WalletTransaction> transactions;
  final ValueChanged<WalletTransaction> onTap;

  @override
  Widget build(BuildContext context) {
    final groups = groupTransactionsByDate(transactions);
    return ListView(
      padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
      children: [
        for (final group in groups) ...[
          _DateGroupHeader(label: group.label),
          const SizedBox(height: 10),
          ...group.items.map(
            (txn) => Padding(
              padding: const EdgeInsets.only(bottom: 4),
              child: WalletTransactionTile(
                txn: txn,
                onTap: () => onTap(txn),
              ),
            ),
          ),
          const SizedBox(height: 20),
        ],
      ],
    );
  }
}

class _DateGroupHeader extends StatelessWidget {
  const _DateGroupHeader({required this.label});
  final String label;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Text(
          label,
          style: const TextStyle(
            color: CustomerFigmaColors.text,
            fontSize: 14,
            fontWeight: FontWeight.w800,
          ),
        ),
        const SizedBox(width: 4),
        const Icon(Icons.arrow_drop_down_rounded,
            size: 20, color: CustomerFigmaColors.text),
      ],
    );
  }
}

class _Empty extends StatelessWidget {
  const _Empty();

  @override
  Widget build(BuildContext context) {
    return const Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.receipt_long_outlined,
              size: 42, color: CustomerFigmaColors.muted),
          SizedBox(height: 12),
          Text(
            'No transactions yet',
            style: TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 15,
              fontWeight: FontWeight.w700,
            ),
          ),
        ],
      ),
    );
  }
}
