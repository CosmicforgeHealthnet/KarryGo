import 'package:flutter/material.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../../profile/ui/widgets/provider_profile_widgets.dart';
import '../models/wallet_models.dart';
import '../state/provider_withdrawal_controller.dart';
import 'provider_add_bank_account_screen.dart';
import 'widgets/wallet_widgets.dart';

/// Payments & Withdrawals (profile section): linked payout accounts with
/// delete, an "Add Bank Account" entry, and the default withdrawal account
/// selector.
class ProviderPaymentsScreen extends StatefulWidget {
  const ProviderPaymentsScreen({super.key, required this.controller});

  final ProviderWithdrawalController controller;

  @override
  State<ProviderPaymentsScreen> createState() => _ProviderPaymentsScreenState();
}

class _ProviderPaymentsScreenState extends State<ProviderPaymentsScreen> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => widget.controller.loadAccounts());
  }

  Future<void> _addAccount() async {
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProviderAddBankAccountScreen(controller: widget.controller),
      ),
    );
    if (mounted) widget.controller.loadAccounts();
  }

  Future<void> _confirmDelete(ProviderBankAccount account) async {
    final confirmed = await showProviderConfirmDialog(
      context,
      icon: Icons.delete_outline_rounded,
      title: 'Remove Bank Account?',
      message:
          '${account.bankName} • ${account.accountNumber} will no longer be available for withdrawals.',
      confirmLabel: 'Remove',
      cancelLabel: 'Cancel',
      confirmColor: kProviderRejectText,
    );
    if (confirmed != true || !mounted) return;
    final ok = await widget.controller.deleteAccount(account);
    if (ok && mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Bank account removed.'), behavior: SnackBarBehavior.floating),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = widget.controller;
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: AnimatedBuilder(
          animation: c,
          builder: (context, _) {
            final accounts = c.bankAccounts;
            return Column(
              children: [
                const WalletFlowAppBar(
                  title: 'Payments & Withdrawals',
                  subtitle: 'Manage your payment Information here.',
                ),
                const SizedBox(height: 18),
                Expanded(
                  child: RefreshIndicator(
                    color: kProviderGreen,
                    onRefresh: c.loadAccounts,
                    child: ListView(
                      physics: const AlwaysScrollableScrollPhysics(),
                      padding: const EdgeInsets.fromLTRB(20, 0, 20, 24),
                      children: [
                        Row(
                          children: [
                            const Expanded(
                              child: Text(
                                'Linked Accounts',
                                style: TextStyle(
                                  color: kProviderText,
                                  fontSize: 18,
                                  fontWeight: FontWeight.w800,
                                ),
                              ),
                            ),
                            FilledButton(
                              onPressed: _addAccount,
                              style: FilledButton.styleFrom(
                                backgroundColor: kProviderGreen,
                                shape: RoundedRectangleBorder(
                                  borderRadius: BorderRadius.circular(999),
                                ),
                                padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 10),
                              ),
                              child: const Text(
                                'Add Bank Account',
                                style: TextStyle(
                                  color: Colors.white,
                                  fontSize: 13,
                                  fontWeight: FontWeight.w700,
                                ),
                              ),
                            ),
                          ],
                        ),
                        const SizedBox(height: 14),
                        if (c.bankError != null) ...[
                          _ErrorBox(message: c.bankError!),
                          const SizedBox(height: 12),
                        ],
                        if (c.loading && accounts.isEmpty)
                          const Padding(
                            padding: EdgeInsets.only(top: 48),
                            child: Center(child: CircularProgressIndicator(color: kProviderGreen)),
                          )
                        else if (c.loadError != null && accounts.isEmpty)
                          _LoadErrorState(message: c.loadError!, onRetry: c.loadAccounts)
                        else if (accounts.isEmpty)
                          const _EmptyState()
                        else ...[
                          for (final account in accounts) ...[
                            _LinkedAccountCard(
                              account: account,
                              onDelete: () => _confirmDelete(account),
                            ),
                            const SizedBox(height: 12),
                          ],
                          const SizedBox(height: 16),
                          const Text(
                            'Set Default withdrawal Account',
                            style: TextStyle(
                              color: kProviderText,
                              fontSize: 18,
                              fontWeight: FontWeight.w800,
                            ),
                          ),
                          const SizedBox(height: 14),
                          for (final account in accounts) ...[
                            _DefaultAccountCard(
                              account: account,
                              onTap: account.isDefault ? null : () => c.makeDefault(account),
                            ),
                            const SizedBox(height: 12),
                          ],
                        ],
                      ],
                    ),
                  ),
                ),
              ],
            );
          },
        ),
      ),
    );
  }
}

// ─── Cards ───────────────────────────────────────────────────────────────────

class _AccountDetails extends StatelessWidget {
  const _AccountDetails({required this.account});
  final ProviderBankAccount account;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          account.accountName,
          style: const TextStyle(color: kProviderText, fontSize: 15, fontWeight: FontWeight.w800),
        ),
        const SizedBox(height: 3),
        Text(
          account.bankName,
          style: const TextStyle(color: kProviderText, fontSize: 13.5, fontWeight: FontWeight.w600),
        ),
        const SizedBox(height: 2),
        Text(
          account.accountNumber,
          style: const TextStyle(color: kProviderMuted, fontSize: 13),
        ),
      ],
    );
  }
}

class _LinkedAccountCard extends StatelessWidget {
  const _LinkedAccountCard({required this.account, required this.onDelete});

  final ProviderBankAccount account;
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.fromLTRB(16, 14, 8, 14),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(14),
        boxShadow: const [BoxShadow(color: Color(0x14000000), blurRadius: 12, offset: Offset(0, 3))],
      ),
      child: Row(
        children: [
          Expanded(child: _AccountDetails(account: account)),
          IconButton(
            onPressed: onDelete,
            icon: const Icon(Icons.delete_outline_rounded, color: kProviderRejectText, size: 24),
            tooltip: 'Remove account',
          ),
        ],
      ),
    );
  }
}

class _DefaultAccountCard extends StatelessWidget {
  const _DefaultAccountCard({required this.account, required this.onTap});

  final ProviderBankAccount account;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: kProviderBorder),
        ),
        child: Row(
          children: [
            Expanded(child: _AccountDetails(account: account)),
            _RadioDot(selected: account.isDefault),
          ],
        ),
      ),
    );
  }
}

class _RadioDot extends StatelessWidget {
  const _RadioDot({required this.selected});
  final bool selected;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 20,
      height: 20,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        border: Border.all(color: kProviderGreen, width: 2),
      ),
      child: selected
          ? Center(
              child: Container(
                width: 10,
                height: 10,
                decoration: const BoxDecoration(color: kProviderGreen, shape: BoxShape.circle),
              ),
            )
          : null,
    );
  }
}

// ─── States ──────────────────────────────────────────────────────────────────

class _EmptyState extends StatelessWidget {
  const _EmptyState();

  @override
  Widget build(BuildContext context) {
    return const Padding(
      padding: EdgeInsets.only(top: 48),
      child: Column(
        children: [
          Icon(Icons.account_balance_rounded, color: kProviderGreenSoft, size: 56),
          SizedBox(height: 14),
          Text(
            'No bank accounts yet',
            style: TextStyle(color: kProviderText, fontSize: 16, fontWeight: FontWeight.w800),
          ),
          SizedBox(height: 6),
          Text(
            'Add a bank account to receive your withdrawals.',
            textAlign: TextAlign.center,
            style: TextStyle(color: kProviderMuted, fontSize: 13.5),
          ),
        ],
      ),
    );
  }
}

class _LoadErrorState extends StatelessWidget {
  const _LoadErrorState({required this.message, required this.onRetry});
  final String message;
  final Future<void> Function() onRetry;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(top: 48),
      child: Column(
        children: [
          Text(
            message,
            textAlign: TextAlign.center,
            style: const TextStyle(color: kProviderMuted, fontSize: 13.5),
          ),
          const SizedBox(height: 12),
          OutlinedButton(
            onPressed: onRetry,
            style: OutlinedButton.styleFrom(
              foregroundColor: kProviderGreen,
              side: const BorderSide(color: kProviderGreen),
              shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
            ),
            child: const Text('Retry'),
          ),
        ],
      ),
    );
  }
}

class _ErrorBox extends StatelessWidget {
  const _ErrorBox({required this.message});
  final String message;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      decoration: BoxDecoration(
        color: kProviderRejectBg,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Text(
        message,
        style: const TextStyle(color: kProviderRejectText, fontSize: 13),
      ),
    );
  }
}
