import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../../../core/format/money_format.dart';
import '../../../core/theme/karryam_theme.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../data/wallet_api.dart';
import 'add_bank_account_screen.dart';
import 'withdraw_screen.dart';

/// Wallet: balance, payout accounts and withdrawals.
class WalletScreen extends StatefulWidget {
  const WalletScreen({super.key, required this.api});
  final WalletApi api;

  @override
  State<WalletScreen> createState() => _WalletScreenState();
}

class _WalletScreenState extends State<WalletScreen> {
  WalletBalance? _balance;
  List<BankAccount>? _accounts;
  String? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() => _error = null);
    try {
      final results = await Future.wait([
        widget.api.balance(),
        widget.api.bankAccounts(),
      ]);
      if (!mounted) return;
      setState(() {
        _balance = results[0] as WalletBalance;
        _accounts = results[1] as List<BankAccount>;
      });
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    }
  }

  Future<void> _open(Widget screen) async {
    final changed = await Navigator.of(
      context,
    ).push<bool>(MaterialPageRoute(builder: (_) => screen));
    if (changed == true) _load();
  }

  Future<void> _remove(BankAccount account) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (c) => AlertDialog(
        title: const Text('Remove this account?'),
        content: Text(account.masked),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(c, false),
            child: const Text('Cancel'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(c, true),
            child: const Text('Remove'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await widget.api.removeBankAccount(account.id);
      _load();
    } on ApiException catch (e) {
      if (mounted) showApiError(context, e);
    }
  }

  Future<void> _makeDefault(BankAccount account) async {
    try {
      await widget.api.setDefault(account.id);
      _load();
    } on ApiException catch (e) {
      if (mounted) showApiError(context, e);
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    final balance = _balance;
    final accounts = _accounts;
    return KarryamPage(
      title: 'Wallet',
      subtitle: 'Your earnings and payouts',
      body: _error != null && balance == null
          ? KarryamPlaceholder.error(message: _error!, onRetry: _load)
          : balance == null || accounts == null
          ? const KarryamPlaceholder.loading()
          : RefreshIndicator(
              onRefresh: _load,
              child: ListView(
                padding: const EdgeInsets.all(20),
                children: [
                  Container(
                    padding: const EdgeInsets.all(20),
                    decoration: BoxDecoration(
                      color: KarryamColors.navy,
                      borderRadius: BorderRadius.circular(18),
                    ),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const Text(
                          'Available to withdraw',
                          style: TextStyle(color: Color(0xFFCED2DA)),
                        ),
                        const SizedBox(height: 6),
                        Text(
                          formatNaira(balance.availableKobo),
                          style: const TextStyle(
                            color: Colors.white,
                            fontSize: 32,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                        if (balance.pendingKobo > 0) ...[
                          const SizedBox(height: 4),
                          Text(
                            '${formatNaira(balance.pendingKobo)} pending',
                            style: const TextStyle(color: Color(0xFFCED2DA)),
                          ),
                        ],
                        const SizedBox(height: 16),
                        FilledButton(
                          onPressed: balance.availableKobo <= 0
                              ? null
                              : () => _open(
                                  WithdrawScreen(
                                    api: widget.api,
                                    availableKobo: balance.availableKobo,
                                    accounts: accounts,
                                  ),
                                ),
                          style: FilledButton.styleFrom(
                            backgroundColor: KarryamColors.teal,
                            foregroundColor: KarryamColors.navy,
                          ),
                          child: const Text('Withdraw'),
                        ),
                      ],
                    ),
                  ),
                  const KarryamSectionTitle('Payout accounts'),
                  if (accounts.isEmpty)
                    Text(
                      'Add a bank account to withdraw your earnings.',
                      style: TextStyle(color: colors.textMuted),
                    ),
                  for (final account in accounts)
                    Padding(
                      padding: const EdgeInsets.only(bottom: 10),
                      child: KarryamPanel(
                        padding: const EdgeInsets.fromLTRB(16, 8, 4, 8),
                        child: Row(
                          children: [
                            Icon(
                              Icons.account_balance_outlined,
                              color: colors.primary,
                            ),
                            const SizedBox(width: 12),
                            Expanded(
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text(
                                    account.masked,
                                    style: const TextStyle(
                                      fontWeight: FontWeight.w600,
                                    ),
                                  ),
                                  Text(
                                    account.accountName,
                                    style: TextStyle(
                                      color: colors.textMuted,
                                      fontSize: 12,
                                    ),
                                  ),
                                ],
                              ),
                            ),
                            if (account.isDefault)
                              const KarryamChip(
                                'Default',
                                tone: KarryamTone.good,
                              ),
                            PopupMenuButton<String>(
                              onSelected: (v) => v == 'default'
                                  ? _makeDefault(account)
                                  : _remove(account),
                              itemBuilder: (_) => [
                                if (!account.isDefault)
                                  const PopupMenuItem(
                                    value: 'default',
                                    child: Text('Make default'),
                                  ),
                                const PopupMenuItem(
                                  value: 'remove',
                                  child: Text('Remove'),
                                ),
                              ],
                            ),
                          ],
                        ),
                      ),
                    ),
                  const SizedBox(height: 8),
                  OutlinedButton.icon(
                    onPressed: () =>
                        _open(AddBankAccountScreen(api: widget.api)),
                    icon: const Icon(Icons.add),
                    label: const Text('Add bank account'),
                  ),
                ],
              ),
            ),
    );
  }
}
