import 'package:flutter/material.dart';

import '../../../core/widgets/karryam_ui.dart';
import '../state/provider_withdrawal_controller.dart';
import 'provider_add_bank_account_screen.dart';
import 'widgets/wallet_keypad.dart';
import 'widgets/wallet_widgets.dart';

/// Select an existing payout account or add a new one ("Change default account").
class ProviderBankAccountsScreen extends StatelessWidget {
  const ProviderBankAccountsScreen({super.key, required this.controller});

  final ProviderWithdrawalController controller;

  void _add(BuildContext context) {
    Navigator.of(context).push(
      MaterialPageRoute(builder: (_) => ProviderAddBankAccountScreen(controller: controller)),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: AnimatedBuilder(
        animation: controller,
        builder: (context, _) {
          final accounts = controller.bankAccounts;
          final selected = controller.selectedAccount;
          return SafeArea(
            child: Column(
              children: [
                const WalletFlowAppBar(title: 'Payout Account'),
                const SizedBox(height: 12),
                Expanded(
                  child: accounts.isEmpty
                      ? _EmptyAccounts(loading: controller.loading)
                      : ListView.separated(
                          padding: const EdgeInsets.symmetric(horizontal: 20),
                          itemCount: accounts.length,
                          separatorBuilder: (_, _) => const SizedBox(height: 12),
                          itemBuilder: (context, i) {
                            final a = accounts[i];
                            return WalletBankAccountTile(
                              accountName: a.accountName,
                              bankName: a.bankName,
                              accountNumber: a.accountNumber,
                              selected: selected?.id == a.id,
                              onTap: () => controller.selectAccount(a),
                            );
                          },
                        ),
                ),
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 20),
                  child: KSecondaryButton(
                    label: 'Add new account',
                    icon: Icons.add_rounded,
                    onPressed: () => _add(context),
                  ),
                ),
                Padding(
                  padding: const EdgeInsets.fromLTRB(20, 12, 20, 16),
                  child: WalletPrimaryButton(
                    label: 'Use this account',
                    onPressed: selected != null ? () => Navigator.of(context).pop() : null,
                  ),
                ),
              ],
            ),
          );
        },
      ),
    );
  }
}

class _EmptyAccounts extends StatelessWidget {
  const _EmptyAccounts({required this.loading});
  final bool loading;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (loading)
            const CircularProgressIndicator()
          else
            const KEmptyState(
              icon: Icons.account_balance_outlined,
              title: 'No payout accounts yet.',
              message: 'Add one to withdraw your earnings.',
            ),
        ],
      ),
    );
  }
}
