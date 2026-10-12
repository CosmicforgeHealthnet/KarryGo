import 'dart:async';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../settings/ui/settings_design.dart';
import '../../support/ui/order_support_screen.dart' show supportDate;
import '../state/orchestration_workspace_controller.dart';

Color orchestrationColor(Object? state) => switch (state) {
  'FAILED' || 'CANCELLED' => KarryamColors.error,
  'RECOVERING' || 'OPS_REVIEW' => const Color(0xFF986013),
  'COMMITTED' || 'IN_EXECUTION' || 'COMPLETED' => KarryamColors.tealInk,
  _ => const Color(0xFF346BA6),
};

class OrchestrationWorkspaceScreen extends StatefulWidget {
  const OrchestrationWorkspaceScreen({
    super.key,
    required this.runtime,
    required this.groupId,
  });
  final VendorRuntime runtime;
  final String groupId;
  @override
  State<OrchestrationWorkspaceScreen> createState() =>
      _OrchestrationWorkspaceScreenState();
}

class _OrchestrationWorkspaceScreenState
    extends State<OrchestrationWorkspaceScreen> {
  late final c = OrchestrationWorkspaceController(
    widget.runtime,
    widget.groupId,
  );
  @override
  void initState() {
    super.initState();
    unawaited(c.load());
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> reviewOverride() async {
    final version = c.record!['version'] as int;
    final selected = await showModalBottomSheet<JsonObject>(
      context: context,
      showDragHandle: true,
      isScrollControlled: true,
      useSafeArea: true,
      builder: (_) => _OverrideSheet(
        current: c.record!,
        decisions: c.decisions,
        mock: widget.runtime.mode == VendorDataMode.mock,
      ),
    );
    if (selected == null || !mounted) return;
    final confirmed = await confirmSettings(
      context,
      'Override execution authority?',
      '${dashboardLabel(selected['authority'])}${selected['product_code'] == '' ? '' : ' · ${selected['product_code']}'}\n${selected['reason']}\n\nThis is an audited authority choice. The server still controls eligibility and dispatch.',
    );
    if (!confirmed || !mounted) return;
    final ok = await c.submitOverride(selected, version);
    if (!mounted) return;
    await showOrderFeedback(
      context,
      title: ok ? 'Authority updated' : 'Override needs attention',
      message: ok
          ? 'Your reason is recorded in the decision history.'
          : c.error ?? 'Refresh before continuing.',
      success: ok,
      warning: c.pending != null,
    );
  }

  Widget section(String title, IconData icon, List<Widget> children) => Padding(
    padding: const EdgeInsets.only(bottom: 18),
    child: SettingsCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(icon, color: const Color(0xFF346BA6).forInk(context)),
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  title,
                  style: const TextStyle(
                    fontSize: 18,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 16),
          ...children,
        ],
      ),
    ),
  );
  Widget decision(JsonObject event) {
    final failed = event['serviceable'] == false;
    final selected = event['selected'] == true;
    final color =
        (failed
                ? KarryamColors.error
                : selected
                ? KarryamColors.tealInk
                : const Color(0xFF346BA6))
            .forInk(context);
    return Padding(
      padding: const EdgeInsets.only(bottom: 14),
      child: SettingsCard(
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Container(
              width: 40,
              height: 40,
              decoration: BoxDecoration(
                color: color.withValues(alpha: .1),
                borderRadius: BorderRadius.circular(14),
              ),
              child: Icon(
                event['event_type'] == 'OVERRIDDEN'
                    ? Icons.tune_rounded
                    : failed
                    ? Icons.block_outlined
                    : selected
                    ? Icons.task_alt_rounded
                    : Icons.hub_outlined,
                color: color,
                size: 23,
              ),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    dashboardLabel(event['event_type']),
                    style: const TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  const SizedBox(height: 6),
                  Text(
                    supportDate(event['created_at']),
                    style: const TextStyle(fontSize: 11),
                  ),
                  if ((event['candidate_authority'] ?? '') != '') ...[
                    const SizedBox(height: 10),
                    Text(
                      '${dashboardLabel(event['candidate_authority'])}${(event['candidate_product_code'] ?? '') == '' ? '' : ' · ${event['candidate_product_code']}'}',
                      style: TextStyle(
                        color: color,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ],
                  if ((event['reason_code'] ?? '') != '') ...[
                    const SizedBox(height: 8),
                    Text(dashboardLabel(event['reason_code'])),
                  ],
                  if ((event['override_reason'] ?? '') != '') ...[
                    const SizedBox(height: 8),
                    Text(event['override_reason'].toString()),
                  ],
                  if ((event['actor_ref'] ?? '') != '') ...[
                    const SizedBox(height: 8),
                    Text(
                      'Recorded by ${event['actor_ref']}',
                      style: const TextStyle(fontSize: 11),
                    ),
                  ],
                  if (event['serviceable'] is bool)
                    Text(
                      event['serviceable'] == true
                          ? 'Serviceable'
                          : 'Not serviceable',
                      style: TextStyle(color: color, fontSize: 12),
                    ),
                  if ((event['outcome_class'] ?? '') != '')
                    Text(dashboardLabel(event['outcome_class'])),
                  if ((event['supply_status'] ?? '') != '')
                    Text('Supply: ${dashboardLabel(event['supply_status'])}'),
                  if (event['estimated_pickup_eta_seconds'] is int &&
                      event['estimated_pickup_eta_seconds'] > 0)
                    Text(
                      'Pickup estimate: ${event['estimated_pickup_eta_seconds']} seconds',
                    ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  List<Widget> content() {
    final row = c.record!;
    final color = orchestrationColor(row['state']).forInk(context);
    final requirement = row['requirements_snapshot'] is Map
        ? jsonObject(row['requirements_snapshot'])
        : <String, dynamic>{};
    return [
      SettingsCard(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Container(
                  width: 60,
                  height: 60,
                  decoration: BoxDecoration(
                    color: color.withValues(alpha: .1),
                    borderRadius: BorderRadius.circular(20),
                  ),
                  child: Icon(Icons.route_rounded, size: 32, color: color),
                ),
                const SizedBox(width: 16),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        dashboardLabel(row['state']),
                        style: TextStyle(
                          fontSize: 21,
                          fontWeight: FontWeight.w700,
                          color: color,
                        ),
                      ),
                      const SizedBox(height: 6),
                      Text(
                        'Plan ${row['plan_version']} · Version ${row['version']}',
                        style: const TextStyle(fontSize: 12),
                      ),
                    ],
                  ),
                ),
              ],
            ),
            const SizedBox(height: 24),
            Text(
              (row['selected_authority'] ?? '') == ''
                  ? 'Awaiting selection'
                  : dashboardLabel(row['selected_authority']),
              style: const TextStyle(
                fontSize: 29,
                fontWeight: FontWeight.w800,
                letterSpacing: -1,
              ),
            ),
            if ((row['selected_product_code'] ?? '') != '')
              Text(row['selected_product_code'].toString()),
            const SizedBox(height: 10),
            Text('${dashboardLabel(row['selection_source'])} selection'),
            if ((row['last_reason_code'] ?? '') != '') ...[
              const SizedBox(height: 14),
              Text(dashboardLabel(row['last_reason_code'])),
            ],
          ],
        ),
      ),
      const SizedBox(height: 18),
      if (c.feedback != null)
        Padding(
          padding: const EdgeInsets.only(bottom: 18),
          child: Text(
            c.feedback!,
            style: TextStyle(color: KarryamColors.tealInk.forInk(context)),
          ),
        ),
      section('Pickup & execution', Icons.schedule_outlined, [
        if (row['desired_pickup_start'] != null)
          SettingsFact('Pickup from', supportDate(row['desired_pickup_start'])),
        if (row['desired_pickup_end'] != null)
          SettingsFact('Pickup until', supportDate(row['desired_pickup_end'])),
        if (row['commit_not_before'] != null)
          SettingsFact(
            'Earliest commitment',
            supportDate(row['commit_not_before']),
          ),
        if (row['next_evaluation_at'] != null)
          SettingsFact(
            'Next evaluation',
            supportDate(row['next_evaluation_at']),
          ),
        SettingsFact('Recovery', dashboardLabel(row['recovery_state'])),
        if ((row['current_mission_ref'] ?? '') != '')
          SettingsFact('Execution reference', row['current_mission_ref']),
      ]),
      section('Transport requirements', Icons.inventory_2_outlined, [
        for (final entry in {
          'package_count': 'Packages',
          'pickup_count': 'Pickups',
          'dropoff_count': 'Dropoffs',
          'weight_grams': 'Weight (grams)',
          'volume_ml': 'Volume (ml)',
          'longest_dimension_mm': 'Longest dimension (mm)',
        }.entries)
          if (requirement[entry.key] != null)
            SettingsFact(entry.value, requirement[entry.key]),
        if (requirement['secure_pickup'] is bool)
          SettingsFact(
            'Protected pickup',
            requirement['secure_pickup'] == true ? 'Required' : 'Not required',
          ),
        if (requirement['secure_delivery'] is bool)
          SettingsFact(
            'Protected delivery',
            requirement['secure_delivery'] == true
                ? 'Required'
                : 'Not required',
          ),
        if ((requirement['temperature_band'] ?? '') != '')
          SettingsFact(
            'Temperature',
            dashboardLabel(requirement['temperature_band']),
          ),
        if ((requirement['handling'] as List? ?? []).isNotEmpty)
          SettingsFact(
            'Handling',
            (requirement['handling'] as List).map(dashboardLabel).join(', '),
          ),
      ]),
      const Text(
        'Decision history',
        style: TextStyle(fontSize: 23, fontWeight: FontWeight.w700),
      ),
      const SizedBox(height: 16),
      if (c.historyError != null)
        DashboardEmptyState(
          story: EmptyStory.unavailable,
          title: 'History unavailable',
          message: c.historyError!,
          action: 'Refresh',
          onAction: c.load,
        )
      else if (c.decisions.isEmpty)
        const DashboardEmptyState(
          story: EmptyStory.activity,
          iconOverride: Icons.hub_outlined,
          title: 'No decisions recorded yet',
          message: 'Decisions appear as the server evaluates this group.',
        )
      else
        ...c.decisions.map(decision),
      if (c.canOverride)
        SettingsAction(
          'Override authority',
          icon: Icons.tune_rounded,
          onPressed: reviewOverride,
        ),
      if (!c.canOverride && c.pending == null && !c.denied)
        const Text(
          'Authority changes are unavailable while this plan is executing, finished or awaiting a fresh read.',
        ),
      if (c.denied)
        Text(
          'Your account does not have authority-override access.',
          style: TextStyle(color: const Color(0xFF986013).forInk(context)),
        ),
    ];
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => PopScope(
      canPop: !c.busy,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('Delivery decisions'),
          actions: [
            IconButton(
              tooltip: 'Refresh decisions',
              onPressed: c.loading || c.busy ? null : c.load,
              icon: const Icon(Icons.refresh_rounded),
            ),
          ],
        ),
        body: RefreshIndicator(
          onRefresh: c.load,
          child: ListView(
            padding: const EdgeInsets.all(20),
            children: [
              if (c.pending != null || c.blocked) ...[
                section(
                  'Override awaiting confirmation',
                  Icons.sync_problem_outlined,
                  [
                    Text(
                      c.blocked
                          ? 'Recovery could not be restored. Reopen this screen.'
                          : 'Check decision history to confirm the previous update. It will not be submitted again.',
                    ),
                    const SizedBox(height: 16),
                    SettingsAction(
                      'Check decision history',
                      icon: Icons.refresh_rounded,
                      secondary: true,
                      onPressed: c.loading || c.busy ? null : c.load,
                    ),
                  ],
                ),
              ],
              if (c.loading || c.busy)
                const PremiumSkeleton(detail: true)
              else if (c.error != null && c.stale)
                DashboardEmptyState(
                  story: c.errorCode == 'ORCHESTRATION_FORBIDDEN'
                      ? EmptyStory.restricted
                      : c.errorCode == 'ORCHESTRATION_NOT_FOUND'
                      ? EmptyStory.activity
                      : EmptyStory.unavailable,
                  title: c.errorCode == 'ORCHESTRATION_NOT_FOUND'
                      ? 'No delivery plan yet'
                      : c.errorCode == 'ORCHESTRATION_FORBIDDEN'
                      ? 'Access required'
                      : 'Delivery planning unavailable',
                  message: c.error!,
                  action: 'Refresh',
                  onAction: c.load,
                )
              else if (c.record != null)
                ...content(),
            ],
          ),
        ),
      ),
    ),
  );
}

class _OverrideSheet extends StatefulWidget {
  const _OverrideSheet({
    required this.current,
    required this.decisions,
    required this.mock,
  });
  final JsonObject current;
  final List<JsonObject> decisions;
  final bool mock;
  @override
  State<_OverrideSheet> createState() => _OverrideSheetState();
}

class _OverrideSheetState extends State<_OverrideSheet> {
  late String authority =
      executionAuthorities.contains(widget.current['selected_authority'])
      ? widget.current['selected_authority']
      : executionAuthorities.first;
  late final product = TextEditingController(
    text: widget.current['selected_product_code'],
  );
  late final reason = TextEditingController(
    text: widget.mock
        ? 'Operational requirement reviewed before commitment.'
        : '',
  );
  final form = GlobalKey<FormState>();
  @override
  void dispose() {
    product.dispose();
    reason.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => SingleChildScrollView(
    padding: EdgeInsets.fromLTRB(
      20,
      0,
      20,
      MediaQuery.viewInsetsOf(context).bottom + 24,
    ),
    child: Form(
      key: form,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Text(
            'Override authority',
            style: TextStyle(fontSize: 24, fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 10),
          const Text(
            'Requires override permission. Physical eligibility remains controlled by the server.',
          ),
          const SizedBox(height: 20),
          DropdownButtonFormField<String>(
            initialValue: authority,
            decoration: const InputDecoration(labelText: 'Execution authority'),
            items: [
              for (final value in executionAuthorities)
                DropdownMenuItem(
                  value: value,
                  child: Text(dashboardLabel(value)),
                ),
            ],
            onChanged: (value) => setState(() {
              authority = value!;
              product.clear();
            }),
          ),
          const SizedBox(height: 16),
          TextFormField(
            controller: product,
            decoration: const InputDecoration(
              labelText: 'Product code (optional)',
              hintText: 'Use an existing product code',
            ),
          ),
          Wrap(
            spacing: 8,
            children: [
              for (final code
                  in widget.decisions
                      .where((e) => e['candidate_authority'] == authority)
                      .map((e) => e['candidate_product_code'])
                      .whereType<String>()
                      .where((code) => code.isNotEmpty)
                      .toSet())
                ActionChip(
                  label: Text(code),
                  onPressed: () => product.text = code,
                ),
            ],
          ),
          const SizedBox(height: 16),
          TextFormField(
            controller: reason,
            maxLines: 3,
            decoration: const InputDecoration(labelText: 'Override reason'),
            validator: (value) => value == null || value.trim().isEmpty
                ? 'Explain why this authority should be used'
                : null,
          ),
          const SizedBox(height: 22),
          FilledButton(
            onPressed: () {
              if (form.currentState!.validate()) {
                Navigator.pop(context, <String, dynamic>{
                  'authority': authority,
                  'product_code': product.text.trim(),
                  'reason': reason.text.trim(),
                });
              }
            },
            child: const Text('Review override'),
          ),
        ],
      ),
    ),
  );
}
