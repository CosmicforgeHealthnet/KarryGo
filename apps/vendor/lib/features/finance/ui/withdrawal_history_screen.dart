import '../../../core/ui/premium_loading.dart';
import 'dart:async';
import 'package:flutter/material.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../onboarding/ui/setup_design.dart';
import '../state/finance_workspace_controller.dart';
import 'finance_design.dart';

class WithdrawalHistoryScreen extends StatefulWidget {
  const WithdrawalHistoryScreen({super.key, required this.controller});
  final FinanceWorkspaceController controller;
  @override
  State<WithdrawalHistoryScreen> createState() =>
      _WithdrawalHistoryScreenState();
}

class _WithdrawalHistoryScreenState extends State<WithdrawalHistoryScreen> {
  String _filter = 'All';
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) unawaited(widget.controller.load());
    });
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: const Text('Withdrawals'),
      backgroundColor: KarryamColors.canvas.forSurface(context),
    ),
    body: AnimatedBuilder(
      animation: widget.controller,
      builder: (context, _) {
        final c = widget.controller;
        final records = c.withdrawals
            .where(
              (value) => switch (_filter) {
                'In progress' => {
                  'pending',
                  'processing',
                  'requires_otp',
                }.contains(value['status']),
                'Paid' => value['status'] == 'paid',
                'Needs attention' => {
                  'failed',
                  'reversed',
                  'requires_otp',
                }.contains(value['status']),
                _ => true,
              },
            )
            .toList();
        return RefreshIndicator(
          onRefresh: c.load,
          child: ListView(
            physics: const AlwaysScrollableScrollPhysics(),
            padding: const EdgeInsets.all(24),
            children: [
              const FinanceTitle('Withdrawals', ''),
              SingleChildScrollView(
                scrollDirection: Axis.horizontal,
                child: Row(
                  children: [
                    for (final filter in [
                      'All',
                      'In progress',
                      'Paid',
                      'Needs attention',
                    ])
                      Padding(
                        padding: const EdgeInsets.only(right: 8),
                        child: ChoiceChip(
                          label: Text(filter),
                          selected: _filter == filter,
                          showCheckmark: false,
                          selectedColor: KarryamColors.mint.forSurface(context),
                          backgroundColor: Colors.white.forSurface(context),
                          side: BorderSide(
                            color:
                                (_filter == filter
                                        ? KarryamColors.teal.withValues(
                                            alpha: .35,
                                          )
                                        : const Color(0xFFE1E9E4))
                                    .forOutline(context),
                          ),
                          labelStyle: TextStyle(
                            color:
                                (_filter == filter
                                        ? KarryamColors.tealInk
                                        : KarryamColors.indigo)
                                    .forInk(context),
                          ),
                          onSelected: (_) => setState(() => _filter = filter),
                        ),
                      ),
                  ],
                ),
              ),
              const SizedBox(height: 24),
              if (c.loading.contains('withdrawals'))
                const PremiumSkeleton(rows: 2),
              if (c.errors['withdrawals'] != null)
                FinanceNotice(
                  c.errors['withdrawals']!,
                  error: !c.restricted.contains('withdrawals'),
                ),
              if (!c.loading.contains('withdrawals') &&
                  c.errors['withdrawals'] == null &&
                  records.isEmpty)
                DashboardEmptyState(
                  story: _filter == 'All'
                      ? EmptyStory.finance
                      : EmptyStory.filtered,
                  title: _filter == 'All'
                      ? 'No withdrawals yet.'
                      : 'Nothing in this view.',
                  message: _filter == 'All'
                      ? 'Your withdrawal requests will appear here with their latest status.'
                      : 'Switch to All to see the other withdrawal records.',
                  action: _filter == 'All' ? 'Refresh' : 'Show all',
                  onAction: _filter == 'All'
                      ? c.load
                      : () => setState(() => _filter = 'All'),
                ),
              for (final value in records)
                WithdrawalTile(
                  value: value,
                  onTap: () => Navigator.push(
                    context,
                    setupRoute<void>(
                      WithdrawalDetailScreen(
                        controller: c,
                        reference: value['reference'] as String,
                      ),
                    ),
                  ),
                ),
            ],
          ),
        );
      },
    ),
  );
}

class WithdrawalTile extends StatelessWidget {
  const WithdrawalTile({super.key, required this.value, required this.onTap});
  final JsonObject value;
  final VoidCallback onTap;
  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.only(bottom: 12),
    child: Material(
      color: Colors.white.forSurface(context),
      borderRadius: BorderRadius.circular(24),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.all(20),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Container(
                padding: const EdgeInsets.all(12),
                decoration: BoxDecoration(
                  color: (financeStatusColor(
                    value['status'],
                  ).withValues(alpha: .09)).forSurface(context),
                  borderRadius: BorderRadius.circular(16),
                ),
                child: Icon(
                  value['status'] == 'paid'
                      ? Icons.task_alt_rounded
                      : value['status'] == 'failed' ||
                            value['status'] == 'reversed'
                      ? Icons.undo_rounded
                      : Icons.account_balance_outlined,
                  color: (financeStatusColor(value['status'])).forInk(context),
                  size: 23,
                ),
              ),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      dashboardMoney(value['amount_kobo'], value['currency']),
                      style: const TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 17,
                      ),
                    ),
                    const SizedBox(height: 5),
                    Text(
                      value['reference'].toString(),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(fontSize: 11),
                    ),
                    const SizedBox(height: 10),
                    FinanceStatus(value['status']),
                  ],
                ),
              ),
              const Icon(Icons.chevron_right_rounded, size: 20),
            ],
          ),
        ),
      ),
    ),
  );
}

class WithdrawalDetailScreen extends StatefulWidget {
  const WithdrawalDetailScreen({
    super.key,
    required this.controller,
    required this.reference,
  });
  final FinanceWorkspaceController controller;
  final String reference;
  @override
  State<WithdrawalDetailScreen> createState() => _WithdrawalDetailScreenState();
}

class _WithdrawalDetailScreenState extends State<WithdrawalDetailScreen> {
  JsonObject? _value;
  String? _error;
  bool _loading = false;
  @override
  void initState() {
    super.initState();
    unawaited(_load());
  }

  Future<void> _load() async {
    if (_loading) return;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final value = await widget.controller.readWithdrawal(widget.reference);
      if (mounted) setState(() => _value = value);
    } catch (e) {
      if (mounted) {
        setState(() => _error = FinanceWorkspaceController.message(e));
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final value = _value;
    final bank = widget.controller.banks
        .where((b) => b['external_ref'] == value?['bank_account_id'])
        .firstOrNull;
    return Scaffold(
      appBar: AppBar(
        title: const Text('Withdrawal details'),
        backgroundColor: KarryamColors.canvas.forSurface(context),
      ),
      body: RefreshIndicator(
        onRefresh: _load,
        child: ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: const EdgeInsets.all(24),
          children: [
            if (_error != null)
              FinanceNotice(
                'Latest status could not be loaded. $_error',
                error: true,
              ),
            if (_loading && value == null)
              const PremiumSkeleton(detail: true, rows: 2),
            if (value != null) ...[
              FinanceCard(
                color: Color(0xFFEAF5F1).forInk(context),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        FinanceEmblem(
                          color: (financeStatusColor(
                            value['status'],
                          )).forInk(context),
                          size: 78,
                        ),
                        const Spacer(),
                        Flexible(child: FinanceStatus(value['status'])),
                      ],
                    ),
                    const SizedBox(height: 22),
                    const Text('Withdrawal amount'),
                    const SizedBox(height: 8),
                    Text(
                      dashboardMoney(value['amount_kobo'], value['currency']),
                      style: const TextStyle(
                        fontSize: 29,
                        letterSpacing: -.8,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 18),
                    Text(financeStatusCopy(value['status'])),
                  ],
                ),
              ),
              const SizedBox(height: 18),
              FinanceCard(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      'TRANSFER RECORD',
                      style: TextStyle(
                        fontSize: 11,
                        letterSpacing: 1.4,
                        fontWeight: FontWeight.w700,
                        color: KarryamColors.tealInk.forInk(context),
                      ),
                    ),
                    FinanceFact('Reference', value['reference'], copy: true),
                    if (bank != null) ...[
                      FinanceFact(
                        'Account name',
                        bank['resolved_account_name'],
                      ),
                      FinanceFact('Account', bank['masked_account']),
                    ] else
                      FinanceFact(
                        'Bank account reference',
                        value['bank_account_id'],
                      ),
                    FinanceFact('Currency', value['currency']),
                  ],
                ),
              ),
              const SizedBox(height: 22),
            ],
            FinanceButton(
              'Refresh status',
              icon: Icons.refresh_rounded,
              busy: _loading,
              onPressed: _load,
            ),
          ],
        ),
      ),
    );
  }
}
