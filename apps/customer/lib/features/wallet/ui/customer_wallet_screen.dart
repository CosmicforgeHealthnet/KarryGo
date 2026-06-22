import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../../../shared/widgets/figma_customer_widgets.dart';
import '../../auth/models/customer_auth_models.dart';
import '../../support/data/support_api.dart';
import '../data/wallet_api.dart';
import '../models/wallet_models.dart';
import 'disputes/wallet_dispute_flow_screen.dart';
import 'funding/wallet_funding_flow_screen.dart';
import 'screens/all_transactions_screen.dart';
import 'screens/transaction_details_screen.dart';
import 'widgets/wallet_balance_card.dart';
import 'widgets/wallet_flow_scaffold.dart';
import 'widgets/wallet_transaction_tile.dart';
import 'withdrawal/wallet_withdrawal_flow_screen.dart';

class CustomerWalletScreen extends StatefulWidget {
  const CustomerWalletScreen({
    super.key,
    required this.session,
    required this.walletApi,
    required this.supportApi,
    this.embedded = false,
  });

  final CustomerSession session;
  final WalletApi walletApi;
  final SupportApi supportApi;
  final bool embedded;

  @override
  State<CustomerWalletScreen> createState() => _CustomerWalletScreenState();
}

class _CustomerWalletScreenState extends State<CustomerWalletScreen> {
  WalletSummary? _wallet;
  List<WalletTransaction> _transactions = [];
  bool _loading = true;
  ApiException? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final wallet = await widget.walletApi
          .getWallet(accessToken: widget.session.accessToken);
      final txns = await widget.walletApi
          .getTransactions(accessToken: widget.session.accessToken);
      if (!mounted) return;
      setState(() {
        _wallet = wallet;
        _transactions = txns;
        _loading = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e;
        _loading = false;
      });
    }
  }

  Future<void> _openFunding() async {
    final funded = await Navigator.of(context).push<bool>(
      MaterialPageRoute<bool>(
        builder: (_) => WalletFundingFlowScreen(
          walletApi: widget.walletApi,
          accessToken: widget.session.accessToken,
          customerEmail: widget.session.customer.email,
        ),
      ),
    );
    if (funded == true) _load();
  }

  Future<void> _openWithdrawal() async {
    final submitted = await Navigator.of(context).push<bool>(
      MaterialPageRoute<bool>(
        builder: (_) => WalletWithdrawalFlowScreen(
          availableKobo: _wallet?.availableKobo ?? 0,
        ),
      ),
    );
    if (submitted == true && mounted) _openAllTransactions();
  }

  void _openDispute() {
    if (_transactions.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('No transactions to dispute.')),
      );
      return;
    }
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => WalletDisputeFlowScreen(
          session: widget.session,
          supportApi: widget.supportApi,
          transaction: _transactions.first,
        ),
      ),
    );
  }

  void _openAllTransactions() {
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => AllTransactionsScreen(
          transactions: _transactions,
          session: widget.session,
          supportApi: widget.supportApi,
        ),
      ),
    );
  }

  void _openTransaction(WalletTransaction txn) {
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => TransactionDetailsScreen(
          txn: txn,
          session: widget.session,
          supportApi: widget.supportApi,
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      appBar: widget.embedded
          ? null
          : AppBar(
              backgroundColor: CustomerFigmaColors.surface,
              elevation: 0,
              scrolledUnderElevation: 0,
              leading:
                  FigmaBackButton(onPressed: () => Navigator.of(context).pop()),
              title: const Text(
                'Wallet',
                style: TextStyle(
                  color: CustomerFigmaColors.text,
                  fontWeight: FontWeight.w800,
                  fontSize: 18,
                ),
              ),
            ),
      body: _loading
          ? const Center(
              child: CircularProgressIndicator(
                  color: CustomerFigmaColors.primary))
          : _error != null
              ? _ErrorView(error: _error!, onRetry: _load)
              : RefreshIndicator(
                  color: CustomerFigmaColors.primary,
                  onRefresh: _load,
                  child: ListView(
                    padding: const EdgeInsets.fromLTRB(20, 8, 20, 32),
                    children: [
                      _WalletHeader(onSpendingSummary: () {}),
                      const SizedBox(height: 16),
                      WalletBalanceCard(
                        wallet: _wallet,
                        onWithdraw: _openWithdrawal,
                      ),
                      const SizedBox(height: 24),
                      _QuickActions(
                        onFund: _openFunding,
                        onWithdraw: _openWithdrawal,
                        onDispute: _openDispute,
                      ),
                      const SizedBox(height: 28),
                      WalletSectionLabel(
                        'Transactions History',
                        trailing: _transactions.isEmpty
                            ? null
                            : GestureDetector(
                                onTap: _openAllTransactions,
                                child: const Text(
                                  'View All',
                                  style: TextStyle(
                                    color: CustomerFigmaColors.primary,
                                    fontSize: 13,
                                    fontWeight: FontWeight.w700,
                                  ),
                                ),
                              ),
                      ),
                      const SizedBox(height: 16),
                      if (_transactions.isEmpty)
                        const _EmptyTransactions()
                      else
                        _GroupedTransactionList(
                          transactions: _transactions.take(10).toList(),
                          onTap: _openTransaction,
                        ),
                    ],
                  ),
                ),
    );
  }
}

class _WalletHeader extends StatelessWidget {
  const _WalletHeader({required this.onSpendingSummary});
  final VoidCallback onSpendingSummary;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: const [
              Text(
                'Wallet',
                style: TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 22,
                  fontWeight: FontWeight.w900,
                ),
              ),
              SizedBox(height: 2),
              Text(
                'Finance your trips with our in-app wallet',
                style: TextStyle(
                  color: CustomerFigmaColors.muted,
                  fontSize: 12,
                ),
              ),
            ],
          ),
        ),
        GestureDetector(
          onTap: onSpendingSummary,
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
            decoration: BoxDecoration(
              color: CustomerFigmaColors.darkGreen,
              borderRadius: BorderRadius.circular(20),
            ),
            child: const Text(
              '\$ Spending Summary',
              style: TextStyle(
                color: Colors.white,
                fontSize: 12,
                fontWeight: FontWeight.w700,
              ),
            ),
          ),
        ),
      ],
    );
  }
}

class _QuickActions extends StatelessWidget {
  const _QuickActions({
    required this.onFund,
    required this.onWithdraw,
    required this.onDispute,
  });

  final VoidCallback onFund;
  final VoidCallback onWithdraw;
  final VoidCallback onDispute;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceEvenly,
      children: [
        _ActionIcon(
          icon: Icons.account_balance_wallet_rounded,
          iconColor: const Color(0xFF22A84A),
          bgColor: const Color(0xFFEAF8EE),
          label: 'Fund Wallet',
          onTap: onFund,
        ),
        _ActionIcon(
          icon: Icons.savings_rounded,
          iconColor: const Color(0xFF22A84A),
          bgColor: const Color(0xFFEAF8EE),
          label: 'Withdraw',
          onTap: onWithdraw,
        ),
        _ActionIcon(
          icon: Icons.receipt_long_rounded,
          iconColor: const Color(0xFFE53935),
          bgColor: const Color(0xFFFDECEC),
          label: 'Dispute',
          onTap: onDispute,
        ),
      ],
    );
  }
}

class _ActionIcon extends StatelessWidget {
  const _ActionIcon({
    required this.icon,
    required this.iconColor,
    required this.bgColor,
    required this.label,
    required this.onTap,
  });

  final IconData icon;
  final Color iconColor;
  final Color bgColor;
  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Column(
        children: [
          Container(
            width: 56,
            height: 56,
            decoration: BoxDecoration(
              color: bgColor,
              shape: BoxShape.circle,
            ),
            child: Icon(icon, color: iconColor, size: 26),
          ),
          const SizedBox(height: 8),
          Text(
            label,
            style: const TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 12,
              fontWeight: FontWeight.w600,
            ),
          ),
        ],
      ),
    );
  }
}

class _GroupedTransactionList extends StatelessWidget {
  const _GroupedTransactionList({
    required this.transactions,
    required this.onTap,
  });

  final List<WalletTransaction> transactions;
  final ValueChanged<WalletTransaction> onTap;

  @override
  Widget build(BuildContext context) {
    final groups = groupTransactionsByDate(transactions);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        for (final group in groups) ...[
          _DateGroupHeader(label: group.label),
          const SizedBox(height: 8),
          ...group.items.map(
            (txn) => Padding(
              padding: const EdgeInsets.only(bottom: 4),
              child: WalletTransactionTile(
                txn: txn,
                onTap: () => onTap(txn),
              ),
            ),
          ),
          const SizedBox(height: 16),
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

class _EmptyTransactions extends StatelessWidget {
  const _EmptyTransactions();

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(vertical: 40),
      alignment: Alignment.center,
      child: const Column(
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
          SizedBox(height: 6),
          Text(
            'Fund your wallet to get started.',
            style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
          ),
        ],
      ),
    );
  }
}

class _ErrorView extends StatelessWidget {
  const _ErrorView({required this.error, required this.onRetry});
  final ApiException error;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline_rounded,
                color: Color(0xFFE53935), size: 40),
            const SizedBox(height: 16),
            Text(error.message,
                textAlign: TextAlign.center,
                style: const TextStyle(
                    color: CustomerFigmaColors.text, fontSize: 14)),
            const SizedBox(height: 20),
            FigmaPrimaryButton(label: 'Try again', onPressed: onRetry),
          ],
        ),
      ),
    );
  }
}
