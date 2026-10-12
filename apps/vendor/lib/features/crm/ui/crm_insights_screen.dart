import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../orders/ui/order_form_sheet.dart';
import '../../settings/state/settings_controller.dart';
import '../../settings/ui/settings_design.dart';
import '../../support/ui/order_support_screen.dart' show supportDate;
import '../state/crm_workspace_controller.dart';
import '../state/crm_insights_controller.dart';
import 'crm_editors.dart';

class CrmInsightsScreen extends StatefulWidget {
  const CrmInsightsScreen({
    super.key,
    required this.runtime,
    this.model,
    this.branch,
  });
  final VendorRuntime runtime;
  final String? model, branch;
  @override
  State<CrmInsightsScreen> createState() => _CrmInsightsScreenState();
}

class _CrmInsightsScreenState extends State<CrmInsightsScreen> {
  late final c = CrmInsightsController(widget.runtime, model: widget.model);
  late String? branch = widget.branch ?? widget.runtime.backend.scope.branchId;
  String scopeLabel = 'Current scope';
  bool selecting = false;
  static const models = {'RFM_V1': 'Customer value', 'CHURN_V1': 'Churn risk'};
  @override
  void initState() {
    super.initState();
    load();
  }

  Future<void> load() async {
    c.readQuery = {'branch_ref': ?branch};
    await c.load();
  }

  Future<void> selectScope() async {
    if (selecting || c.loading || c.busy || c.pending != null) return;
    setState(() => selecting = true);
    final settings = SettingsController(
      widget.runtime.backend,
      SettingsKind.branches,
    );
    try {
      await settings.load();
      while (settings.hasMore && settings.error == null) {
        await settings.load(more: true);
      }
      if (!mounted) return;
      if (settings.error != null) {
        await showOrderFeedback(
          context,
          title: 'Could not load branches',
          message: settings.error!,
          warning: true,
        );
        return;
      }
      final current = widget.runtime.backend.scope.branchId;
      final options = <String, String>{
        if (current == null) 'business': 'Whole business',
        for (final row in settings.rows)
          if (current == null || row['id'] == current)
            '${row['id']}': '${row['name'] ?? 'Branch'}',
      };
      final selected = await crmChoose(context, 'View insights for', options);
      if (selected == null || selected.isEmpty || !mounted) return;
      branch = selected.single == 'business' ? null : selected.single;
      scopeLabel = options[selected.single]!;
      await load();
    } finally {
      settings.dispose();
      if (mounted) setState(() => selecting = false);
    }
  }

  Future<void> rebuild(String model, {bool retry = false}) async {
    if (c.busy || c.loading) return;
    if (!retry) {
      final confirmed = await showModalBottomSheet<JsonObject>(
        context: context,
        isScrollControlled: true,
        useSafeArea: true,
        showDragHandle: true,
        builder: (_) => OrderFormSheet(
          title: 'Refresh ${models[model]?.toLowerCase()}?',
          message:
              'Queue a rebuild for ${branch == null ? 'your business' : 'this branch'}. Existing results remain visible while the server processes it.',
          fields: const [],
          icon: Icons.refresh_rounded,
        ),
      );
      if (confirmed == null || !mounted) return;
    }
    final result = await c.save(
      method: retry ? null : 'POST',
      route: retry ? null : '$crmBase/intelligence/rebuild',
      body: retry
          ? null
          : {
              'model_key': model,
              'scope_type': branch == null ? 'BUSINESS' : 'BRANCH',
              if (branch != null) 'branch_ref': branch,
            },
    );
    if (!mounted) return;
    if (result != null) {
      await showOrderFeedback(
        context,
        title: 'Rebuild ${pretty(result['status']).toLowerCase()}',
        message:
            '${models[result['model_key']] ?? 'Insights'} refresh was accepted. Refresh this screen to check progress.',
        success: true,
      );
      await load();
    }
  }

  String pretty(Object? value) => value
      .toString()
      .replaceAll('_', ' ')
      .toLowerCase()
      .split(' ')
      .map(
        (word) => word.isEmpty
            ? word
            : '${word[0].toUpperCase()}${word.substring(1)}',
      )
      .join(' ');
  Color color(Object? state) => switch (state) {
    'CURRENT' => KarryamColors.tealInk.forInk(context),
    'FAILED' || 'HIGH' || 'CRITICAL' => KarryamColors.error.forInk(context),
    'STALE' || 'MEDIUM' => const Color(0xFFBC812E).forInk(context),
    'QUEUED' || 'RUNNING' => const Color(0xFF397FC1).forInk(context),
    _ => Theme.of(context).colorScheme.onSurfaceVariant,
  };
  Widget statePill(Object? state) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
    decoration: BoxDecoration(
      color: color(state).withValues(alpha: .12),
      borderRadius: BorderRadius.circular(12),
    ),
    child: Text(
      pretty(state),
      style: TextStyle(
        color: color(state),
        fontSize: 12,
        fontWeight: FontWeight.w700,
      ),
    ),
  );
  Widget metric(String label, Object? value) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Text(
        '$value',
        style: Theme.of(
          context,
        ).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w800),
      ),
      const SizedBox(height: 4),
      Text(label, style: Theme.of(context).textTheme.bodySmall),
    ],
  );
  Widget availability(JsonObject row, {bool link = false}) => Padding(
    padding: const EdgeInsets.only(bottom: 16),
    child: SettingsCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                padding: const EdgeInsets.all(12),
                decoration: BoxDecoration(
                  color:
                      (row['model_key'] == 'RFM_V1'
                              ? const Color(0xFF7660A6)
                              : const Color(0xFF397FC1))
                          .withValues(alpha: .12),
                  borderRadius: BorderRadius.circular(16),
                ),
                child: Icon(
                  row['model_key'] == 'RFM_V1'
                      ? Icons.stacked_line_chart_rounded
                      : Icons.monitor_heart_outlined,
                  color:
                      (row['model_key'] == 'RFM_V1'
                              ? const Color(0xFF7660A6)
                              : const Color(0xFF397FC1))
                          .forInk(context),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Text(
                  models[row['model_key']] ?? 'Insights',
                  style: Theme.of(
                    context,
                  ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w800),
                ),
              ),
            ],
          ),
          const SizedBox(height: 16),
          statePill(row['status']),
          const SizedBox(height: 20),
          Wrap(
            spacing: 26,
            runSpacing: 16,
            children: [
              metric('Relationships', row['profile_count'] ?? 0),
              metric('Scored', row['available_count'] ?? 0),
              metric('Unavailable', row['unavailable_count'] ?? 0),
            ],
          ),
          const SizedBox(height: 16),
          Text(switch (row['status']) {
            'NOT_AVAILABLE' => 'No evaluation is available for this scope.',
            'QUEUED' => 'Waiting for the server to process this scope.',
            'RUNNING' => 'The server is updating these insights.',
            'STALE' => 'Results need refreshing before you act on them.',
            'FAILED' => 'The latest evaluation failed.',
            'CURRENT' =>
              row['available_count'] == 0
                  ? 'Evaluation complete. No relationships have enough data to score.'
                  : 'Insights are up to date.',
            _ => 'Refresh to check availability.',
          }),
          if (row['source_as_of'] != null) ...[
            const SizedBox(height: 12),
            Text(
              'Source as of ${supportDate(row['source_as_of'])}',
              style: Theme.of(context).textTheme.bodySmall,
            ),
          ],
          if (row['computed_at'] != null)
            Text(
              'Computed ${supportDate(row['computed_at'])}',
              style: Theme.of(context).textTheme.bodySmall,
            ),
          if (row['stale_since'] != null)
            Text(
              'Stale since ${supportDate(row['stale_since'])}',
              style: TextStyle(color: color('STALE')),
            ),
          if (row['error_code'] != null) ...[
            const SizedBox(height: 12),
            SettingsNote(pretty(row['error_code'])),
          ],
          if (link) ...[
            const SizedBox(height: 16),
            SettingsLink(
              'View distribution',
              Icons.bar_chart_rounded,
              onTap: () async {
                await Navigator.push(
                  context,
                  setupRoute<void>(
                    CrmInsightsScreen(
                      runtime: widget.runtime,
                      model: row['model_key'],
                      branch: branch,
                    ),
                  ),
                );
                if (mounted) await load();
              },
            ),
          ],
          const SizedBox(height: 14),
          SettingsAction(
            'Refresh insights',
            secondary: true,
            icon: Icons.refresh_rounded,
            busy: c.busy,
            onPressed:
                c.allowed('POST', '$crmBase/intelligence/rebuild') &&
                    !['QUEUED', 'RUNNING'].contains(row['status'])
                ? () => rebuild(row['model_key'])
                : null,
          ),
        ],
      ),
    ),
  );
  Widget distribution(String title, Object? raw) => Padding(
    padding: const EdgeInsets.only(bottom: 16),
    child: SettingsCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            title,
            style: Theme.of(
              context,
            ).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w800),
          ),
          const SizedBox(height: 18),
          if (raw == null || (raw is List && raw.isEmpty))
            Row(
              children: [
                Icon(
                  Icons.bar_chart_rounded,
                  size: 32,
                  color: Theme.of(context).colorScheme.onSurfaceVariant,
                ),
                const SizedBox(width: 12),
                const Expanded(
                  child: Text('No scored relationships in this distribution.'),
                ),
              ],
            )
          else
            ...buckets(raw),
        ],
      ),
    ),
  );
  List<Widget> buckets(Object? raw) {
    final rows = (raw as List).map(jsonObject).toList();
    final total = rows.fold<int>(0, (sum, r) => sum + (r['count'] as int));
    return [
      for (final row in rows)
        Padding(
          padding: const EdgeInsets.only(bottom: 16),
          child: Column(
            children: [
              Row(
                children: [
                  Expanded(child: Text(pretty(row['key']))),
                  Text(
                    '${row['count']}',
                    style: const TextStyle(fontWeight: FontWeight.w800),
                  ),
                ],
              ),
              const SizedBox(height: 8),
              TweenAnimationBuilder<double>(
                tween: Tween(
                  begin: 0,
                  end: total == 0 ? 0 : (row['count'] as int) / total,
                ),
                duration: const Duration(milliseconds: 450),
                curve: Curves.easeOutCubic,
                builder: (context, value, _) => ClipRRect(
                  borderRadius: BorderRadius.circular(8),
                  child: LinearProgressIndicator(
                    value: value,
                    minHeight: 7,
                    color: color(row['key']),
                    backgroundColor: Theme.of(
                      context,
                    ).colorScheme.surfaceContainerHighest,
                  ),
                ),
              ),
            ],
          ),
        ),
    ];
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      appBar: AppBar(
        title: Text(
          widget.model == null ? 'Customer insights' : models[widget.model]!,
        ),
        actions: [
          IconButton(
            tooltip: 'Refresh',
            onPressed: c.loading || c.busy || selecting ? null : load,
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 680),
            child: RefreshIndicator(
              onRefresh: load,
              child: ListView(
                padding: const EdgeInsets.all(20),
                physics: const AlwaysScrollableScrollPhysics(),
                children: [
                  if (widget.model == null) ...[
                    SettingsLink(
                      scopeLabel,
                      Icons.storefront_outlined,
                      subtitle: branch == null
                          ? 'Business-wide results'
                          : 'Branch results',
                      onTap:
                          c.loading || c.busy || selecting || c.pending != null
                          ? null
                          : selectScope,
                    ),
                    const SizedBox(height: 20),
                  ],
                  if (c.loading) const PremiumSkeleton(rows: 2, detail: true),
                  if (c.error != null) SettingsNote(c.error!),
                  if (c.pending != null) ...[
                    const SettingsNote(
                      'A previous change has an unconfirmed outcome.',
                      warning: true,
                    ),
                    if (c.pending!['path'] == '$crmBase/intelligence/rebuild')
                      SettingsAction(
                        'Retry saved rebuild',
                        icon: Icons.refresh_rounded,
                        busy: c.busy,
                        onPressed: c.loading || c.busy || c.blocked
                            ? null
                            : () => rebuild('', retry: true),
                      )
                    else
                      const SettingsNote(
                        'Finish the saved change in its original CRM screen.',
                        warning: true,
                      ),
                  ],
                  if (c.record != null && !c.loading && !c.stale) ...[
                    if (widget.model == null)
                      for (final row in (c.record!['models'] as List))
                        availability(jsonObject(row), link: true)
                    else ...[
                      availability(jsonObject(c.record!['availability'])),
                      if (widget.model == 'RFM_V1') ...[
                        distribution(
                          'Combined RFM score',
                          c.record!['composite_distribution'],
                        ),
                        distribution(
                          'Recency',
                          c.record!['recency_distribution'],
                        ),
                        distribution(
                          'Frequency',
                          c.record!['frequency_distribution'],
                        ),
                        distribution(
                          'Monetary value',
                          c.record!['monetary_distribution'],
                        ),
                      ] else
                        distribution(
                          'Risk bands',
                          c.record!['band_distribution'],
                        ),
                      if ((c.record!['unavailable_reasons'] as List?)
                              ?.isNotEmpty ==
                          true)
                        distribution(
                          'Why scores are unavailable',
                          c.record!['unavailable_reasons'],
                        ),
                    ],
                  ],
                ],
              ),
            ),
          ),
        ),
      ),
    ),
  );
}
