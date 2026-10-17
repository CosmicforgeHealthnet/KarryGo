import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../../shared/widgets/dispatch_empty_state.dart';
import '../../home/ui/widgets/home_feedback.dart';
import 'package:flutter/material.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/async_content.dart';
import '../data/wallet_models.dart';
import '../state/wallet_controller.dart';
import 'add_bank_account_screen.dart';

class BankAccountsScreen extends StatefulWidget {
  const BankAccountsScreen({super.key, required this.walletController});
  final WalletController walletController;
  @override
  State<BankAccountsScreen> createState() => _BankAccountsScreenState();
}

class _BankAccountsScreenState extends State<BankAccountsScreen> {
  bool _loading = true;
  bool _busy = false;
  String? _error;
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _load();
    });
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    final result = await widget.walletController.loadBankAccounts();
    if (!mounted) return;
    result.when(
      success: (_) => setState(() => _loading = false),
      failure: (e) => setState(() {
        _loading = false;
        _error = e.message;
      }),
    );
  }

  Future<void> _change(BankAccount bank, bool remove) async {
    if (_busy) return;
    if (remove) {
      final confirmed = await confirmAccountAction(
        context,
        title: 'Remove bank account?',
        message:
            '${bank.bankName} will no longer be available for withdrawals.',
        action: 'Remove account',
        symbol: JourneySymbol.bank,
        destructive: true,
      );
      if (confirmed != true || !mounted) return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    final result = await widget.walletController.updateBankAccount(
      bank.id,
      remove: remove,
    );
    if (!mounted) return;
    result.when(
      success: (_) async {
        await showHomeFeedback(
          context,
          title: remove ? 'Bank account removed' : 'Default account updated',
          message: remove
              ? 'This account is no longer available for withdrawals.'
              : 'This account will be selected for future withdrawals.',
        );
      },
      failure: (e) => setState(() => _error = e.message),
    );
    setState(() => _busy = false);
  }

  void _manage(BankAccount bank) {
    showModalBottomSheet<void>(
      context: context,
      useSafeArea: true,
      showDragHandle: true,
      isScrollControlled: true,
      barrierColor: Colors.black.withValues(alpha: .5),
      builder: (sheet) => SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(24, 0, 24, 24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(bank.bankName, style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 8),
            Text(bank.accountName),
            const SizedBox(height: 20),
            if (!bank.isDefault)
              FilledButton.icon(
                onPressed: () {
                  Navigator.pop(sheet);
                  _change(bank, false);
                },
                icon: const Icon(Icons.check_circle_outline),
                label: const Text('Use for withdrawals'),
              ),
            const SizedBox(height: 10),
            TextButton.icon(
              onPressed: () {
                Navigator.pop(sheet);
                _change(bank, true);
              },
              style: TextButton.styleFrom(
                foregroundColor: context.palette.danger,
              ),
              icon: const Icon(Icons.delete_outline_rounded),
              label: const Text('Remove account'),
            ),
          ],
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final accounts = widget.walletController.accounts;
    return AccountPage(
      title: 'Bank accounts',
      symbol: JourneySymbol.bank,
      description: 'Choose where your withdrawals are sent.',
      onRefresh: _load,
      bottom: FilledButton.icon(
        onPressed: _busy || _loading
            ? null
            : () async {
                await Navigator.of(context).push(
                  MaterialPageRoute(
                    builder: (_) => AddBankAccountScreen(
                      walletController: widget.walletController,
                    ),
                  ),
                );
                if (mounted) _load();
              },
        icon: const Icon(Icons.add_rounded),
        label: const Text('Add bank account'),
      ),
      children: [
        if (_loading)
          const SizedBox(height: 230, child: ContentSkeleton(rows: 2))
        else ...[
          if (_error != null) ...[
            ActionNotice(message: _error!, isError: true, onRetry: _load),
            const SizedBox(height: 16),
          ],
          if (_busy)
            const Padding(
              padding: EdgeInsets.all(12),
              child: Center(
                child: SizedBox.square(
                  dimension: 20,
                  child: CircularProgressIndicator(strokeWidth: 2),
                ),
              ),
            ),
          if (accounts.isEmpty && _error == null)
            const DispatchEmptyState(
              story: EmptyStory.wallet,
              title: 'Your earnings need a destination',
              message: 'Add your bank account to request withdrawals.',
            ),
          for (final bank in accounts)
            Padding(
              padding: const EdgeInsets.only(bottom: 14),
              child: DeliveryPanel(
                onTap: _busy ? null : () => _manage(bank),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Row(
                      children: [
                        const AccountEmblem(symbol: JourneySymbol.bank),
                        const SizedBox(width: 14),
                        Expanded(
                          child: Text(
                            bank.bankName,
                            style: Theme.of(context).textTheme.titleMedium,
                          ),
                        ),
                        const Icon(Icons.chevron_right_rounded),
                      ],
                    ),
                    const SizedBox(height: 16),
                    Text(
                      bank.accountName,
                      style: Theme.of(context).textTheme.titleSmall,
                    ),
                    const SizedBox(height: 6),
                    Text(
                      bank.accountNumber,
                      style: Theme.of(context).textTheme.bodyMedium,
                    ),
                    if (bank.isDefault) ...[
                      const SizedBox(height: 14),
                      Text(
                        'Default for withdrawals',
                        style: Theme.of(context).textTheme.bodySmall?.copyWith(
                          color: context.palette.brandTeal600,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                    ],
                  ],
                ),
              ),
            ),
        ],
      ],
    );
  }
}
