import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../settings/ui/settings_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../support/ui/order_support_screen.dart' show supportDate;
import '../state/order_readiness_controller.dart';

String readinessActionLabel(String action) => switch (action) {
  'start' => 'Start preparation',
  'estimate' => 'Update preparation',
  'ready-soon' => 'Ready soon',
  'ready' => 'Mark ready',
  'delay' => 'Report delay',
  _ => 'Report a blocker',
};
Color readinessColor(Object? state) => switch (state) {
  'READY' => KarryamColors.tealInk,
  'BLOCKED' || 'CANCELLED' => KarryamColors.error,
  'DELAYED' => const Color(0xFF986013),
  _ => const Color(0xFF326AA2),
};
IconData readinessIcon(String action) => switch (action) {
  'ready' => Icons.task_alt_rounded,
  'delay' => Icons.more_time_rounded,
  'block' => Icons.front_hand_outlined,
  'ready-soon' => Icons.timelapse_rounded,
  _ => Icons.restaurant_outlined,
};

class OrderReadinessScreen extends StatefulWidget {
  const OrderReadinessScreen({
    super.key,
    required this.runtime,
    required this.order,
  });
  final VendorRuntime runtime;
  final JsonObject order;
  @override
  State<OrderReadinessScreen> createState() => _OrderReadinessScreenState();
}

class _OrderReadinessScreenState extends State<OrderReadinessScreen> {
  late final c = OrderReadinessController(widget.runtime.backend, widget.order);
  @override
  void initState() {
    super.initState();
    c.load();
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> edit(String action) async {
    final reason = TextEditingController();
    String? type = action == 'start' && c.types.length == 1
        ? c.types.single
        : null;
    String? substate = action == 'ready'
        ? null
        : c.record?['substate'] as String?;
    DateTime? estimate;
    final needsReason = action == 'delay' || action == 'block';
    final result = await settingsSheet<JsonObject>(
      context,
      StatefulBuilder(
        builder: (context, update) => Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            SettingsHeading(
              readinessActionLabel(action),
              subtitle: action == 'estimate'
                  ? 'This update puts preparation in progress.'
                  : action == 'ready'
                  ? 'Confirm every item is prepared and ready for collection.'
                  : null,
            ),
            const SizedBox(height: 20),
            if (action == 'start')
              DropdownButtonFormField<String>(
                initialValue: type,
                isExpanded: true,
                decoration: const InputDecoration(labelText: 'Business type'),
                items: c.types
                    .map(
                      (t) => DropdownMenuItem(
                        value: t,
                        child: Text(dashboardLabel(t)),
                      ),
                    )
                    .toList(),
                onChanged: (v) => update(() => type = v),
              ),
            if (action != 'start' && action != 'ready') ...[
              DropdownButtonFormField<String>(
                initialValue: substate,
                isExpanded: true,
                decoration: const InputDecoration(
                  labelText: 'Preparation activity (optional)',
                ),
                items: [
                  const DropdownMenuItem<String>(
                    value: null,
                    child: Text('No activity detail'),
                  ),
                  for (final s
                      in readinessSubstates[c.record?['vendor_type']] ??
                          <String>[])
                    DropdownMenuItem(value: s, child: Text(dashboardLabel(s))),
                ],
                onChanged: (v) => update(() => substate = v),
              ),
              const SizedBox(height: 16),
            ],
            if (needsReason) ...[
              TextField(
                controller: reason,
                onChanged: (_) => update(() {}),
                decoration: const InputDecoration(labelText: 'Reason'),
                textCapitalization: TextCapitalization.sentences,
                maxLines: 3,
              ),
              const SizedBox(height: 16),
            ],
            if (action != 'ready')
              SettingsLink(
                estimate == null
                    ? 'Set ready time (optional)'
                    : supportDate(estimate!.toIso8601String()),
                Icons.schedule_rounded,
                onTap: () async {
                  final now = DateTime.now();
                  final date = await showDatePicker(
                    context: context,
                    initialDate: now,
                    firstDate: now,
                    lastDate: now.add(const Duration(days: 365)),
                  );
                  if (date == null || !context.mounted) return;
                  final time = await showTimePicker(
                    context: context,
                    initialTime: TimeOfDay.fromDateTime(
                      now.add(const Duration(minutes: 30)),
                    ),
                  );
                  if (time == null || !context.mounted) return;
                  update(
                    () => estimate = DateTime(
                      date.year,
                      date.month,
                      date.day,
                      time.hour,
                      time.minute,
                    ),
                  );
                },
              ),
            const SizedBox(height: 24),
            SettingsAction(
              'Review update',
              onPressed:
                  (action == 'start' && type == null ||
                      needsReason && reason.text.trim().isEmpty)
                  ? null
                  : () => Navigator.pop(context, <String, dynamic>{
                      'type': type,
                      'substate': substate,
                      'reason': reason.text.trim(),
                      'estimate': estimate,
                    }),
            ),
          ],
        ),
      ),
    );
    reason.dispose();
    if (result == null || !mounted) return;
    final confirmed = await confirmSettings(
      context,
      'Confirm preparation update',
      '${readinessActionLabel(action)}${result['substate'] != null ? '\nActivity: ${dashboardLabel(result['substate'])}' : ''}${result['reason'].toString().isNotEmpty ? '\nReason: ${result['reason']}' : ''}${result['estimate'] != null ? '\nReady time: ${supportDate((result['estimate'] as DateTime).toIso8601String())}' : ''}',
      danger: action == 'block',
    );
    if (confirmed != true || !mounted) return;
    final success = await c.save(
      action,
      type: result['type'] as String?,
      substate: result['substate'] as String?,
      reason: result['reason'] as String?,
      estimate: result['estimate'] as DateTime?,
    );
    if (!mounted) return;
    await showOrderFeedback(
      context,
      title: success ? 'Preparation updated' : 'Update needs attention',
      message: success
          ? 'The latest preparation status has been saved.'
          : c.error ?? 'Refresh and try again.',
      success: success,
    );
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => PopScope(
      canPop: !c.busy,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('Preparation'),
          actions: [
            IconButton(
              tooltip: 'Refresh preparation',
              onPressed: c.loading || c.busy ? null : c.load,
              icon: const Icon(Icons.refresh_rounded),
            ),
          ],
        ),
        body: RefreshIndicator(
          onRefresh: c.load,
          child: ListView(
            padding: const EdgeInsets.fromLTRB(20, 12, 20, 32),
            children: [
              if (c.loading && c.record == null)
                const PremiumSkeleton(detail: true)
              else ...[
                if (c.error != null) ...[
                  SettingsCard(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const SettingsHeading('Preparation unavailable'),
                        Text(c.error!),
                        const SizedBox(height: 16),
                        SettingsAction(
                          'Refresh',
                          onPressed: c.loading || c.busy ? null : c.load,
                          secondary: true,
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 16),
                ],
                SettingsCard(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          Container(
                            padding: const EdgeInsets.all(14),
                            decoration: BoxDecoration(
                              color: readinessColor(
                                c.record?['state'],
                              ).withValues(alpha: .10).forSurface(context),
                              borderRadius: BorderRadius.circular(18),
                            ),
                            child: Icon(
                              Icons.soup_kitchen_outlined,
                              size: 30,
                              color: readinessColor(
                                c.record?['state'],
                              ).forInk(context),
                            ),
                          ),
                          const SizedBox(width: 14),
                          Expanded(
                            child: SettingsHeading(
                              c.record == null
                                  ? c.missing
                                        ? 'Not started'
                                        : 'Preparation status'
                                  : dashboardLabel(c.record!['state']),
                              subtitle: c.record?['substate'] == null
                                  ? null
                                  : dashboardLabel(c.record!['substate']),
                            ),
                          ),
                        ],
                      ),
                      if (c.missing) ...[
                        const SizedBox(height: 18),
                        const Text(
                          'Start preparation to share a ready-time estimate and track progress.',
                        ),
                      ],
                      if (c.typeError != null) ...[
                        const SizedBox(height: 12),
                        Text(c.typeError!),
                      ],
                      if (c.record != null) ...[
                        const SizedBox(height: 20),
                        for (final entry in {
                          'vendor_estimated_ready_at': 'Your ready time',
                          'predicted_ready_start': 'Predicted window starts',
                          'predicted_ready_end': 'Predicted window ends',
                          'actual_ready_at': 'Ready at',
                          'block_reason': 'Block reason',
                          'delay_reason': 'Delay reason',
                        }.entries)
                          if (c.record![entry.key] != null)
                            Padding(
                              padding: const EdgeInsets.only(bottom: 12),
                              child: SettingsFact(
                                entry.value,
                                entry.key.endsWith('_at') ||
                                        entry.key.startsWith('predicted_')
                                    ? supportDate(c.record![entry.key])
                                    : c.record![entry.key].toString(),
                              ),
                            ),
                      ],
                    ],
                  ),
                ),
                if (c.loading || c.busy)
                  const Padding(
                    padding: EdgeInsets.symmetric(vertical: 16),
                    child: Center(
                      child: SizedBox(
                        width: 22,
                        height: 22,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      ),
                    ),
                  ),
                for (final action in c.actions)
                  Padding(
                    padding: const EdgeInsets.only(top: 12),
                    child: SettingsAction(
                      readinessActionLabel(action),
                      icon: readinessIcon(action),
                      secondary:
                          action != 'start' &&
                          action != 'ready' &&
                          action != 'estimate',
                      danger: action == 'block',
                      onPressed: () => edit(action),
                    ),
                  ),
                const SizedBox(height: 28),
                if (c.record != null) ...[
                  const SettingsHeading('Preparation history'),
                  const SizedBox(height: 14),
                  if (c.historyError != null)
                    Text(c.historyError!)
                  else if (c.history.isEmpty)
                    const Text('No preparation updates recorded yet.')
                  else
                    SettingsCard(
                      child: Column(
                        children: [
                          for (final h in c.history)
                            Padding(
                              padding: const EdgeInsets.symmetric(vertical: 12),
                              child: Row(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Icon(
                                    h['to_state'] == 'READY'
                                        ? Icons.task_alt_rounded
                                        : Icons.update_rounded,
                                    color: readinessColor(
                                      h['to_state'],
                                    ).forInk(context),
                                  ),
                                  const SizedBox(width: 12),
                                  Expanded(
                                    child: Column(
                                      crossAxisAlignment:
                                          CrossAxisAlignment.start,
                                      children: [
                                        Text(
                                          dashboardLabel(h['to_state']),
                                          style: Theme.of(
                                            context,
                                          ).textTheme.titleSmall,
                                        ),
                                        if (h['to_substate'] != null)
                                          Text(
                                            dashboardLabel(h['to_substate']),
                                          ),
                                        if (h['reason_code'] != null)
                                          Text(h['reason_code'].toString()),
                                        Text(
                                          supportDate(h['created_at']),
                                          style: Theme.of(
                                            context,
                                          ).textTheme.bodySmall,
                                        ),
                                      ],
                                    ),
                                  ),
                                ],
                              ),
                            ),
                        ],
                      ),
                    ),
                ],
              ],
            ],
          ),
        ),
      ),
    ),
  );
}
