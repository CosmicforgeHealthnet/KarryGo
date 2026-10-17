import 'bank_account_screen.dart';
import '../../../core/ui/premium_loading.dart';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../state/finance_workspace_controller.dart';
import 'finance_design.dart';
import 'withdrawal_history_screen.dart';
import 'withdrawal_request_screen.dart';

class FinancePanel extends StatelessWidget {
  const FinancePanel({
    super.key,
    required this.runtime,
    required this.controller,
    required this.onSetup,
    required this.onChanged,
  });
  final VendorRuntime runtime;
  final FinanceWorkspaceController controller;
  final Future<void> Function() onSetup, onChanged;
  Future<void> _withdraw(BuildContext context) async {
    final reference = await Navigator.push<String>(
      context,
      setupRoute<String>(
        WithdrawalRequestScreen(runtime: runtime, controller: controller),
      ),
    );
    await onChanged();
    if (reference != null && context.mounted) {
      await Navigator.push(
        context,
        setupRoute<void>(
          WithdrawalDetailScreen(controller: controller, reference: reference),
        ),
      );
    }
  }

  Future<void> _retry(BuildContext context) async {
    final body = controller.pending?['body'] as Map?;
    final confirmed = await financeSheet<bool>(
      context,
      Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const FinanceTitle(
            'Check the original request.',
            'This retries the same withdrawal, with the same request identity and details.',
          ),
          FinanceFact(
            'Amount',
            dashboardMoney(body?['amount_kobo'], body?['currency']),
          ),
          const SizedBox(height: 12),
          const FinanceNotice(
            'Do not create another withdrawal while this request is unresolved.',
          ),
          FinanceButton(
            'Retry original request',
            icon: Icons.refresh_rounded,
            onPressed: () => Navigator.pop(context, true),
          ),
        ],
      ),
    );
    if (confirmed != true || !context.mounted) return;
    final accepted = await controller.retryOriginal();
    await onChanged();
    if (!context.mounted) return;
    await showOrderFeedback(
      context,
      title: accepted ? 'Request confirmed' : 'Request still needs checking',
      message: accepted
          ? financeStatusCopy(controller.lastWithdrawal?['status'])
          : controller.commandError ??
                'The original request is still unresolved. New withdrawals remain paused.',
      success:
          accepted && controller.lastWithdrawal?['status'] != 'requires_otp',
      warning:
          !accepted || controller.lastWithdrawal?['status'] == 'requires_otp',
    );
    final reference = controller.lastWithdrawal?['reference'];
    if (accepted && reference is String && context.mounted) {
      await Navigator.push(
        context,
        setupRoute<void>(
          WithdrawalDetailScreen(controller: controller, reference: reference),
        ),
      );
    }
  }

  Future<void> _banks(BuildContext context) => financeSheet<void>(
    context,
    Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const FinanceTitle(
          'Payout accounts',
          'Accounts linked to your business wallet.',
        ),
        if (controller.errors['banks'] != null)
          FinanceNotice(
            controller.errors['banks']!,
            error: !controller.restricted.contains('banks'),
          ),
        for (final bank in controller.banks)
          Padding(
            padding: const EdgeInsets.only(bottom: 14),
            child: FinanceCard(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Icon(
                        Icons.account_balance_outlined,
                        color: KarryamColors.tealInk.forInk(context),
                      ),
                      const Spacer(),
                      FinanceStatus(bank['status']),
                    ],
                  ),
                  const SizedBox(height: 16),
                  Text(
                    bank['resolved_account_name'].toString(),
                    style: const TextStyle(
                      fontWeight: FontWeight.w700,
                      fontSize: 18,
                    ),
                  ),
                  const SizedBox(height: 6),
                  Text(bank['masked_account'].toString()),
                  FinanceFact('Bank', bankNameForCode(bank['bank_code'])),
                ],
              ),
            ),
          ),
        if (controller.banks.isEmpty && controller.errors['banks'] == null)
          const DashboardEmptyState(
            story: EmptyStory.finance,
            title: 'Connect your payout account.',
            message: 'Add and verify a bank account to receive withdrawals.',
          ),
        const SizedBox(height: 12),
        FinanceButton(
          controller.banks.isEmpty
              ? 'Add bank account'
              : 'Change bank account',
          icon: Icons.account_balance_outlined,
          onPressed: () async {
            Navigator.pop(context);
            await openPayoutAccount(context, runtime);
            await controller.load();
          },
        ),
      ],
    ),
  );
  Widget _resource(String key, Widget child) {
    final c = controller;
    if (c.loading.contains(key) && !c.fresh.contains(key)) {
      return const Padding(
        padding: EdgeInsets.all(24),
        child: PremiumSkeleton(detail: true, rows: 2),
      );
    }
    if (c.errors[key] != null) {
      return DashboardEmptyState(
        story: c.restricted.contains(key)
            ? EmptyStory.restricted
            : EmptyStory.unavailable,
        title: c.restricted.contains(key)
            ? 'Finance access is limited.'
            : 'This view could not load.',
        message: c.errors[key]!,
        action: 'Refresh Finance',
        onAction: c.load,
      );
    }
    return child;
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: Listenable.merge([controller, runtime]),
    builder: (context, _) {
      final c = controller;
      return RefreshIndicator(
        onRefresh: () async {
          await c.load();
          await onChanged();
        },
        child: ListView(
          key: const PageStorageKey('finance-workspace'),
          padding: const EdgeInsets.fromLTRB(24, 8, 24, 32),
          physics: const AlwaysScrollableScrollPhysics(),
          children: [
            const FinanceTitle('Finance', ''),
            if (c.errors['recovery'] != null)
              FinanceNotice(c.errors['recovery']!),
            if (c.unknown || c.journalBlocked) ...[
              FinanceCard(
                color: Color(0xFFFFF3DF).forInk(context),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Icon(
                      Icons.shield_outlined,
                      color: Color(0xFF986013).forInk(context),
                      size: 30,
                    ),
                    const SizedBox(height: 14),
                    Text(
                      c.lastWithdrawal?['reference'] is String
                          ? 'Your request is confirmed.'
                          : 'A request needs checking.',
                      style: TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 21,
                      ),
                    ),
                    const SizedBox(height: 8),
                    Text(
                      c.lastWithdrawal?['reference'] is String
                          ? 'Your withdrawal was accepted. Finish saving this confirmation before requesting another transfer.'
                          : 'New withdrawals are paused until the original request is confirmed. You can still view your withdrawal history.',
                    ),
                    if (c.pending?['body'] is Map) ...[
                      const SizedBox(height: 12),
                      Text(
                        dashboardMoney(
                          (c.pending!['body'] as Map)['amount_kobo'],
                          (c.pending!['body'] as Map)['currency'],
                        ),
                        style: const TextStyle(
                          fontWeight: FontWeight.w700,
                          fontSize: 22,
                        ),
                      ),
                    ],
                    if (c.commandError != null) ...[
                      const SizedBox(height: 12),
                      Text(c.commandError!),
                    ],
                    const SizedBox(height: 20),
                    if (c.lastWithdrawal?['reference'] is String &&
                        c.journalBlocked) ...[
                      FinanceFact(
                        'Confirmed reference',
                        c.lastWithdrawal!['reference'],
                        copy: true,
                      ),
                      FinanceButton(
                        'Finish confirmation',
                        busy: c.busy,
                        icon: Icons.check_rounded,
                        onPressed: c.clearConfirmedRecovery,
                      ),
                    ] else
                      FinanceButton(
                        'Retry original request',
                        busy: c.busy,
                        icon: Icons.refresh_rounded,
                        onPressed: c.pending != null && !c.journalBlocked
                            ? () => _retry(context)
                            : null,
                      ),
                  ],
                ),
              ),
              const SizedBox(height: 20),
            ],
            _resource(
              'summary',
              FinanceCard(
                color: Color(0xFFE2F4EE).forInk(context),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Expanded(
                          child: Text(
                            'BUSINESS WALLET',
                            style: TextStyle(
                              color: KarryamColors.tealInk.forInk(context),
                              letterSpacing: 1.4,
                              fontWeight: FontWeight.w700,
                              fontSize: 11,
                            ),
                          ),
                        ),
                        const FinanceEmblem(size: 76),
                      ],
                    ),
                    const SizedBox(height: 6),
                    Row(
                      children: [
                        const Expanded(child: Text('Available balance')),
                        IconButton(
                          tooltip: runtime.balancesVisible
                              ? 'Hide balances'
                              : 'Show balances',
                          onPressed: runtime.toggleBalances,
                          color: KarryamColors.tealInk.forInk(context),
                          icon: Icon(
                            runtime.balancesVisible
                                ? Icons.visibility_outlined
                                : Icons.visibility_off_outlined,
                            size: 21,
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 8),
                    Text(
                      runtime.balancesVisible
                          ? dashboardMoney(
                              c.summary['available_kobo'],
                              c.summary['currency'],
                            )
                          : '\u2022\u2022\u2022\u2022\u2022\u2022',
                      style: const TextStyle(
                        fontSize: 32,
                        letterSpacing: -1,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 22),
                    Container(
                      padding: const EdgeInsets.all(16),
                      decoration: BoxDecoration(
                        color: (Colors.white.withValues(
                          alpha: .7,
                        )).forSurface(context),
                        borderRadius: BorderRadius.circular(18),
                      ),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          const Text(
                            'Awaiting settlement',
                            style: TextStyle(fontSize: 12),
                          ),
                          const SizedBox(height: 5),
                          Text(
                            runtime.balancesVisible
                                ? dashboardMoney(
                                    c.summary['pending_kobo'],
                                    c.summary['currency'],
                                  )
                                : '\u2022\u2022\u2022\u2022\u2022\u2022',
                            style: const TextStyle(
                              fontSize: 19,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                        ],
                      ),
                    ),
                    const SizedBox(height: 20),
                    FinanceButton(
                      'Withdraw funds',
                      onPressed: c.canWithdraw
                          ? () => _withdraw(context)
                          : null,
                      icon: Icons.north_east_rounded,
                    ),
                    if (!c.canWithdraw && !c.loading.contains('banks'))
                      Padding(
                        padding: const EdgeInsets.only(top: 12),
                        child: Text(
                          c.unknown || c.journalBlocked
                              ? 'Resolve your earlier request to withdraw again.'
                              : c.withdrawDenied
                              ? 'Your current access does not permit withdrawals.'
                              : c.errors['banks'] != null
                              ? 'Payout accounts must load before you can withdraw.'
                              : c.activeBanks.isEmpty
                              ? 'An active payout account is required.'
                              : 'Withdrawals become available when you have an available balance.',
                        ),
                      ),
                  ],
                ),
              ),
            ),
            const SizedBox(height: 20),
            _link(
              context,
              'Withdrawals',
              'Follow every transfer',
              Icons.account_balance_outlined,
              () => Navigator.push(
                context,
                setupRoute<void>(WithdrawalHistoryScreen(controller: c)),
              ),
            ),
            const SizedBox(height: 12),
            _link(
              context,
              'Payout accounts',
              'Bank details & account status',
              Icons.credit_card_rounded,
              () => _banks(context),
            ),
            const SizedBox(height: 30),
            Row(
              children: [
                const Expanded(
                  child: Text(
                    'Wallet activity',
                    style: TextStyle(
                      fontSize: 22,
                      fontWeight: FontWeight.w700,
                      letterSpacing: -.6,
                    ),
                  ),
                ),
                TextButton(
                  onPressed: () => Navigator.push(
                    context,
                    setupRoute<void>(_TransactionHistory(controller: c)),
                  ),
                  child: const Text('View all'),
                ),
              ],
            ),
            const SizedBox(height: 12),
            _resource(
              'transactions',
              c.transactions.isEmpty
                  ? DashboardEmptyState(
                      story: EmptyStory.finance,
                      title: 'No transactions yet.',
                      message: 'Wallet activity will appear here.',
                      action: 'Refresh activity',
                      onAction: c.load,
                    )
                  : Column(
                      children: [
                        for (final record in c.transactions.take(5))
                          _transaction(context, record),
                      ],
                    ),
            ),
          ],
        ),
      );
    },
  );
  Widget _link(
    BuildContext context,
    String title,
    String subtitle,
    IconData icon,
    VoidCallback onTap,
  ) => Material(
    color: Colors.white.forSurface(context),
    borderRadius: BorderRadius.circular(22),
    clipBehavior: Clip.antiAlias,
    child: InkWell(
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Row(
          children: [
            Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: KarryamColors.mint.forSurface(context),
                borderRadius: BorderRadius.circular(16),
              ),
              child: Icon(icon, color: KarryamColors.tealInk.forInk(context)),
            ),
            const SizedBox(width: 15),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    style: const TextStyle(
                      fontWeight: FontWeight.w700,
                      fontSize: 17,
                    ),
                  ),
                  const SizedBox(height: 4),
                  Text(subtitle, style: const TextStyle(fontSize: 12)),
                ],
              ),
            ),
            const Icon(Icons.chevron_right_rounded),
          ],
        ),
      ),
    ),
  );
}

Widget _transaction(BuildContext context, Map<String, dynamic> record) {
  final credit = record['side'] == 'CREDIT';
  final color = credit ? KarryamColors.tealInk : const Color(0xFF285EB0);
  return Padding(
    padding: const EdgeInsets.only(bottom: 10),
    child: Material(
      color: Colors.white.forSurface(context),
      borderRadius: BorderRadius.circular(22),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: () => showFinanceTransaction(context, record),
        child: Padding(
          padding: const EdgeInsets.all(18),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Container(
                padding: const EdgeInsets.all(11),
                decoration: BoxDecoration(
                  color: (color.withValues(alpha: .08)).forSurface(context),
                  borderRadius: BorderRadius.circular(14),
                ),
                child: Icon(
                  credit ? Icons.south_west_rounded : Icons.north_east_rounded,
                  color: (color).forInk(context),
                  size: 20,
                ),
              ),
              const SizedBox(width: 13),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      record['memo']?.toString().isNotEmpty == true
                          ? record['memo'].toString()
                          : dashboardLabel(record['transaction_type']),
                      style: const TextStyle(fontWeight: FontWeight.w600),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      dashboardMoney(record['amount_kobo'], record['currency']),
                      style: TextStyle(
                        fontWeight: FontWeight.w700,
                        color: (color).forInk(context),
                        fontSize: 17,
                      ),
                    ),
                    const SizedBox(height: 5),
                    Text(
                      dashboardLabel(record['side']),
                      style: const TextStyle(fontSize: 11),
                    ),
                  ],
                ),
              ),
              const Icon(Icons.chevron_right_rounded, size: 18),
            ],
          ),
        ),
      ),
    ),
  );
}

class _TransactionHistory extends StatelessWidget {
  const _TransactionHistory({required this.controller});
  final FinanceWorkspaceController controller;
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: const Text('Wallet activity'),
      backgroundColor: KarryamColors.canvas.forSurface(context),
    ),
    body: AnimatedBuilder(
      animation: controller,
      builder: (context, _) => RefreshIndicator(
        onRefresh: controller.load,
        child: ListView(
          padding: const EdgeInsets.all(24),
          physics: const AlwaysScrollableScrollPhysics(),
          children: [
            const FinanceTitle('Transactions', 'Latest 100 transactions.'),
            if (controller.loading.contains('transactions'))
              const PremiumSkeleton(rows: 2),
            if (controller.errors['transactions'] != null)
              FinanceNotice(
                controller.errors['transactions']!,
                error: !controller.restricted.contains('transactions'),
              ),
            if (controller.transactions.isEmpty &&
                controller.errors['transactions'] == null &&
                !controller.loading.contains('transactions'))
              const DashboardEmptyState(
                story: EmptyStory.finance,
                title: 'No wallet activity yet.',
                message:
                    'Transactions will appear here when they are recorded.',
              ),
            for (final record in controller.transactions)
              _transaction(context, record),
          ],
        ),
      ),
    ),
  );
}
