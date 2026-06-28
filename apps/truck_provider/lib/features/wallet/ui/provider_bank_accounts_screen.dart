import 'package:flutter/material.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
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
      backgroundColor: Colors.white,
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
                  child: OutlinedButton.icon(
                    onPressed: () => _add(context),
                    icon: const Icon(Icons.add, color: kProviderGreen),
                    label: const Text('Add new account'),
                    style: OutlinedButton.styleFrom(
                      foregroundColor: kProviderGreen,
                      minimumSize: const Size.fromHeight(52),
                      side: const BorderSide(color: kProviderGreen),
                      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(30)),
                    ),
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
            const CircularProgressIndicator(color: kProviderGreen)
          else ...[
            const Icon(Icons.account_balance_outlined, color: kProviderMuted, size: 44),
            const SizedBox(height: 12),
            const Text(
              'No payout accounts yet.\nAdd one to withdraw your earnings.',
              textAlign: TextAlign.center,
              style: TextStyle(color: kProviderMuted, fontSize: 13.5, height: 1.4),
            ),
          ],
        ],
      ),
    );
  }
}
