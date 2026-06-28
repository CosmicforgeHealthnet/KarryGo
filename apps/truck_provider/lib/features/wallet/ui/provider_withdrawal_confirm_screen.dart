import 'package:flutter/material.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_withdrawal_controller.dart';
import 'provider_bank_accounts_screen.dart';
import 'provider_transaction_auth_screen.dart';
import 'widgets/wallet_keypad.dart';
import 'widgets/wallet_widgets.dart';

/// Step 2 — confirm amount + payout account (Figma 2282).
class ProviderWithdrawalConfirmScreen extends StatelessWidget {
  const ProviderWithdrawalConfirmScreen({super.key, required this.controller});

  final ProviderWithdrawalController controller;

  void _changeAccount(BuildContext context) {
    Navigator.of(context).push(
      MaterialPageRoute(builder: (_) => ProviderBankAccountsScreen(controller: controller)),
    );
  }

  void _confirm(BuildContext context) {
    Navigator.of(context).push(
      MaterialPageRoute(builder: (_) => ProviderTransactionAuthScreen(controller: controller)),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: AnimatedBuilder(
        animation: controller,
        builder: (context, _) {
          final account = controller.selectedAccount;
          return SafeArea(
            child: Column(
              children: [
                const WalletFlowAppBar(title: 'Withdrawal'),
                const SizedBox(height: 12),
                Expanded(
                  child: ListView(
                    padding: const EdgeInsets.symmetric(horizontal: 20),
                    children: [
                      WalletAmountToPayCard(naira: controller.amountKobo / 100),
                      const SizedBox(height: 16),
                      const Text(
                        'Your withdrawal will be processed into your default account',
                        textAlign: TextAlign.center,
                        style: TextStyle(color: kProviderGreen, fontSize: 13, fontWeight: FontWeight.w500),
                      ),
                      const SizedBox(height: 16),
                      if (account != null)
                        WalletBankAccountTile(
                          accountName: account.accountName,
                          bankName: account.bankName,
                          accountNumber: account.accountNumber,
                          selected: true,
                        )
                      else
                        _NoAccountCard(
                          loading: controller.loading,
                          onAdd: () => _changeAccount(context),
                        ),
                      const SizedBox(height: 16),
                      Center(
                        child: GestureDetector(
                          onTap: () => _changeAccount(context),
                          behavior: HitTestBehavior.opaque,
                          child: const Text(
                            'Change default account',
                            style: TextStyle(color: kProviderGreen, fontSize: 14, fontWeight: FontWeight.w700),
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
                const _TermsText(),
                Padding(
                  padding: const EdgeInsets.fromLTRB(20, 16, 20, 16),
                  child: WalletPrimaryButton(
                    label: 'Confirm',
                    onPressed: account != null ? () => _confirm(context) : null,
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

class _NoAccountCard extends StatelessWidget {
  const _NoAccountCard({required this.loading, required this.onAdd});
  final bool loading;
  final VoidCallback onAdd;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(18),
      decoration: BoxDecoration(
        color: kProviderSurface,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: kProviderBorder),
      ),
      child: Column(
        children: [
          const Icon(Icons.account_balance_outlined, color: kProviderMuted, size: 32),
          const SizedBox(height: 10),
          Text(
            loading ? 'Loading your accounts…' : 'No payout account yet.',
            style: const TextStyle(color: kProviderMuted, fontSize: 13.5),
          ),
          if (!loading) ...[
            const SizedBox(height: 12),
            OutlinedButton(
              onPressed: onAdd,
              style: OutlinedButton.styleFrom(
                foregroundColor: kProviderGreen,
                side: const BorderSide(color: kProviderGreen),
                shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(30)),
              ),
              child: const Text('Add bank account'),
            ),
          ],
        ],
      ),
    );
  }
}

class _TermsText extends StatelessWidget {
  const _TermsText();

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 20),
      child: Text.rich(
        TextSpan(
          style: const TextStyle(color: kProviderText, fontSize: 13, height: 1.4),
          children: [
            const TextSpan(text: 'By completing payment, you agree to the '),
            TextSpan(
              text: 'terms and conditions',
              style: const TextStyle(color: kProviderGreen, fontWeight: FontWeight.w600),
            ),
            const TextSpan(text: ' and acknowledge the '),
            TextSpan(
              text: 'privacy policy',
              style: const TextStyle(color: kProviderGreen, fontWeight: FontWeight.w600),
            ),
          ],
        ),
      ),
    );
  }
}
