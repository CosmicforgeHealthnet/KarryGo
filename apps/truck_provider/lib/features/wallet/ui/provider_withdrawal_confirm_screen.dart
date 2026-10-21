import 'package:flutter/material.dart';

import '../../../core/widgets/karryam_ui.dart';
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
                      const KNotice(
                        'Your withdrawal will be processed into your default account',
                        tone: KTone.success,
                        icon: Icons.account_balance_rounded,
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
                          child: Text(
                            'Change default account',
                            style: TextStyle(color: kTealText, fontSize: 14, fontWeight: FontWeight.w700),
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
    return SizedBox(
      width: double.infinity,
      child: loading
          ? Padding(
              padding: EdgeInsets.all(12),
              child: Center(child: Text('Loading your accounts…', style: TextStyle(color: kProviderMuted))),
            )
          : KEmptyState(
              icon: Icons.account_balance_outlined,
              title: 'No payout account yet.',
              message: 'Add the bank account your earnings should go to.',
              actionLabel: 'Add bank account',
              onAction: onAdd,
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
          style: TextStyle(color: kProviderText, fontSize: 13, height: 1.4),
          children: [
            const TextSpan(text: 'By completing payment, you agree to the '),
            TextSpan(
              text: 'terms and conditions',
              style: TextStyle(color: kTealText, fontWeight: FontWeight.w600),
            ),
            const TextSpan(text: ' and acknowledge the '),
            TextSpan(
              text: 'privacy policy',
              style: TextStyle(color: kTealText, fontWeight: FontWeight.w600),
            ),
          ],
        ),
      ),
    );
  }
}
