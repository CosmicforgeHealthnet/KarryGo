import 'package:flutter/material.dart';

import '../../../core/widgets/karryam_ui.dart';
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
                    onRefresh: c.loadAccounts,
                    child: ListView(
                      physics: const AlwaysScrollableScrollPhysics(),
                      padding: const EdgeInsets.fromLTRB(20, 0, 20, 24),
                      children: [
                        Row(
                          children: [
                            const Expanded(child: KSectionHeader('Linked Accounts')),
                            FilledButton.icon(
                              onPressed: _addAccount,
                              style: FilledButton.styleFrom(
                                minimumSize: const Size(0, 40),
                                shape: const StadiumBorder(),
                                padding: const EdgeInsets.symmetric(horizontal: 16),
                                textStyle: const TextStyle(fontFamily: 'Inter', fontSize: 13, fontWeight: FontWeight.w700),
                              ),
                              icon: const Icon(Icons.add_rounded, size: 18),
                              label: const Text('Add Bank Account'),
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
                            child: Center(child: CircularProgressIndicator()),
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
                          Text(
                            'Set Default withdrawal Account',
                            style: TextStyle(color: kProviderText, fontSize: 15, fontWeight: FontWeight.w700),
                          ),
                          const SizedBox(height: 4),
                          Text(
                            'Withdrawals go to this account unless you choose another.',
                            style: TextStyle(color: kProviderMuted, fontSize: 12.5),
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
    return Row(
      children: [
        KIconTile(Icons.account_balance_rounded),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                account.accountName,
                style: TextStyle(color: kProviderText, fontSize: 14.5, fontWeight: FontWeight.w700),
              ),
              const SizedBox(height: 2),
              Text(account.bankName, style: TextStyle(color: kProviderText, fontSize: 13)),
              const SizedBox(height: 1),
              Text(account.accountNumber, style: TextStyle(color: kProviderMuted, fontSize: 12.5)),
            ],
          ),
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
    return KCard(
      padding: const EdgeInsets.fromLTRB(16, 14, 6, 14),
      child: Row(
        children: [
          Expanded(child: _AccountDetails(account: account)),
          if (account.isDefault) const KStatusChip('Default'),
          IconButton(
            onPressed: onDelete,
            icon: Icon(Icons.delete_outline_rounded, color: kProviderRejectText, size: 22),
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
    return KCard(
      onTap: onTap,
      borderColor: account.isDefault ? kTeal : kProviderBorder,
      child: Row(
        children: [
          Expanded(child: _AccountDetails(account: account)),
          WalletRadioDot(selected: account.isDefault),
        ],
      ),
    );
  }
}

// ─── States ──────────────────────────────────────────────────────────────────

class _EmptyState extends StatelessWidget {
  const _EmptyState();

  @override
  Widget build(BuildContext context) {
    return const KEmptyState(
      icon: Icons.account_balance_rounded,
      title: 'No bank accounts yet',
      message: 'Add a bank account to receive your withdrawals.',
    );
  }
}

class _LoadErrorState extends StatelessWidget {
  const _LoadErrorState({required this.message, required this.onRetry});
  final String message;
  final Future<void> Function() onRetry;

  @override
  Widget build(BuildContext context) {
    return KEmptyState(
      icon: Icons.cloud_off_rounded,
      title: "Couldn't load your accounts",
      message: message,
      actionLabel: 'Retry',
      onAction: onRetry,
    );
  }
}

class _ErrorBox extends StatelessWidget {
  const _ErrorBox({required this.message});
  final String message;

  @override
  Widget build(BuildContext context) => KNotice(message, tone: KTone.danger);
}
