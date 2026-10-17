import '../../orders/ui/order_handover_sheet.dart';
import '../../orchestration/ui/orchestration_workspace_screen.dart';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../settings/ui/settings_design.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../support/ui/order_support_screen.dart' show supportDate;
import '../state/fulfilment_workspace_controller.dart';
import 'parcel_manifest_screen.dart';
import 'vendor_missions_screen.dart' show missionColor;

class FulfilmentWorkspaceScreen extends StatefulWidget {
  const FulfilmentWorkspaceScreen({
    super.key,
    required this.runtime,
    required this.groupId,
    this.parcelId,
  });
  final VendorRuntime runtime;
  final String groupId;
  final String? parcelId;
  @override
  State<FulfilmentWorkspaceScreen> createState() =>
      _FulfilmentWorkspaceScreenState();
}

class _FulfilmentWorkspaceScreenState extends State<FulfilmentWorkspaceScreen> {
  late final c = FulfilmentWorkspaceController(
    widget.runtime,
    widget.groupId,
    parcelId: widget.parcelId,
  );
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

  bool counterLoading = false;
  List<JsonObject> missionsFor(JsonObject unit) {
    final parcels = c.parcels
        .where((p) => p['fulfilment_unit_id'] == unit['fulfilment_unit_id'])
        .map((p) => p['parcel_id'])
        .toSet();
    return (c.view?['vendor_missions'] as List? ?? [])
        .map(jsonObject)
        .where(
          (m) =>
              m['assigned_staff_id'] != null &&
              m['status'] == 'READY_FOR_PICKUP' &&
              (m['parcel_ids'] as List? ?? []).any(parcels.contains),
        )
        .toList();
  }

  Future<void> counterPickup(JsonObject unit, JsonObject mission) async {
    if (counterLoading) return;
    setState(() => counterLoading = true);
    try {
      final order = await c.order(unit['vendor_order_id'] as String);
      if (!mounted) return;
      if (![
        'READY',
        'READY_FOR_PICKUP',
        'HANDED_OVER',
      ].contains(order['status'])) {
        await showOrderFeedback(
          context,
          title: 'Order must be ready',
          message:
              'Finish preparation and mark this order ready before verifying pickup.',
          success: false,
          warning: true,
        );
        return;
      }
      final verified = await showModalBottomSheet<bool>(
        context: context,
        isScrollControlled: true,
        useSafeArea: true,
        showDragHandle: true,
        builder: (_) => OrderHandoverSheet(
          runtime: widget.runtime,
          orderId: unit['vendor_order_id'] as String,
          missionId: mission['vendor_mission_id'] as String,
        ),
      );
      if (verified == true && mounted) {
        await c.load();
        if (!mounted) return;
        await showOrderFeedback(
          context,
          title: 'Pickup verified',
          message: 'The assigned rider can now confirm pickup custody.',
          success: true,
        );
      }
    } catch (_) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Pickup unavailable',
          message:
              'Could not load the order for pickup verification. Refresh and try again.',
          success: false,
        );
      }
    } finally {
      if (mounted) setState(() => counterLoading = false);
    }
  }

  Future<void> feedback(bool ok) async {
    if (!mounted) return;
    await showOrderFeedback(
      context,
      title: ok ? 'Fulfilment updated' : 'Update needs attention',
      message: ok
          ? 'The latest fulfilment information has been saved.'
          : c.error ?? 'Refresh and try again.',
      success: ok,
    );
  }

  Future<void> manifest(JsonObject unit, {JsonObject? parcel}) async {
    await Navigator.push(
      context,
      setupRoute<bool>(
        ParcelManifestScreen(
          controller: c,
          unit: unit,
          parcel: parcel,
          mock: widget.runtime.mode == VendorDataMode.mock,
        ),
      ),
    );
    if (mounted) await c.load();
  }

  Future<void> ready(JsonObject unit, String state) async {
    final confirm = await confirmSettings(
      context,
      dashboardLabel(state),
      'Update this pickup obligation to ${dashboardLabel(state).toLowerCase()}?',
    );
    if (!confirm || !mounted) return;
    await feedback(
      await c.command(
        method: 'PUT',
        path: '/api/v1/vendor/fulfilment/units/{unitId}/readiness',
        params: {'unitId': unit['fulfilment_unit_id'] as String},
        version: unit['version'] as int,
        body: {'readiness': state},
      ),
    );
  }

  Future<void> initiateReturn() async {
    final reason = TextEditingController(
      text: widget.runtime.mode == VendorDataMode.mock
          ? 'Receiver requested a return'
          : '',
    );
    final input = await settingsSheet<String>(
      context,
      StatefulBuilder(
        builder: (context, set) => Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const SettingsHeading('Initiate parcel return'),
            const SizedBox(height: 16),
            TextField(
              controller: reason,
              maxLines: 3,
              decoration: const InputDecoration(labelText: 'Return reason'),
              onChanged: (_) => set(() {}),
            ),
            const SizedBox(height: 24),
            SettingsAction(
              'Review return',
              onPressed: reason.text.trim().isEmpty
                  ? null
                  : () => Navigator.pop(context, reason.text.trim()),
            ),
          ],
        ),
      ),
    );
    reason.dispose();
    if (input == null || !mounted) return;
    final confirm = await confirmSettings(context, 'Initiate return', input);
    if (!confirm || !mounted) return;
    await feedback(
      await c.command(
        method: 'POST',
        path: '/api/v1/vendor/fulfilment/parcels/{parcelId}/returns',
        params: {'parcelId': widget.parcelId!},
        body: {'reason_code': input},
      ),
    );
  }

  Widget parcelCard(JsonObject parcel) => SettingsCard(
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Icon(
              Icons.inventory_2_outlined,
              size: 32,
              color: missionColor(parcel['status']).forInk(context),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: SettingsHeading(
                parcel['parcel_ref']?.toString() ?? 'Parcel',
                subtitle: dashboardLabel(parcel['status']),
              ),
            ),
          ],
        ),
        const SizedBox(height: 16),
        if (widget.parcelId == null)
          SettingsAction(
            'Parcel details',
            secondary: true,
            onPressed: () async {
              await Navigator.push(
                context,
                setupRoute<void>(
                  FulfilmentWorkspaceScreen(
                    runtime: widget.runtime,
                    groupId: widget.groupId,
                    parcelId: parcel['parcel_id'] as String,
                  ),
                ),
              );
              if (mounted) await c.load();
            },
          )
        else ...[
          SettingsFact('Held by', dashboardLabel(parcel['custodian_type'])),
          const SizedBox(height: 14),
          SettingsFact('Weight', '${parcel['weight_grams']} g'),
          const SizedBox(height: 14),
          SettingsFact(
            'Packages inside',
            '${parcel['manifest']?['package_count'] ?? 0}',
          ),
          const SizedBox(height: 14),
          SettingsFact(
            'Temperature',
            dashboardLabel(parcel['temperature_control']),
          ),
          if (parcel['fragile'] == true) ...[
            const SizedBox(height: 14),
            const SettingsFact('Handling', 'Fragile'),
          ],
          const SizedBox(height: 24),
          const SettingsHeading('Packed contents'),
          const SizedBox(height: 12),
          for (final line in (parcel['manifest']?['lines'] as List? ?? []).map(
            jsonObject,
          ))
            Padding(
              padding: const EdgeInsets.only(bottom: 12),
              child: SettingsFact(
                line['description']?.toString() ??
                    line['order_line_ref'].toString(),
                line['quantity'].toString(),
              ),
            ),
          if (c.sealed)
            const Text('The manifest is sealed by handover evidence.')
          else
            SettingsAction(
              'Edit manifest',
              secondary: true,
              icon: Icons.edit_note_rounded,
              onPressed: c.canWrite
                  ? () => manifest({
                      'vendor_order_id': parcel['vendor_order_id'],
                      'fulfilment_unit_id': parcel['fulfilment_unit_id'],
                    }, parcel: parcel)
                  : null,
            ),
          const SizedBox(height: 12),
          SettingsAction(
            'Initiate return',
            secondary: true,
            icon: Icons.assignment_return_outlined,
            onPressed: c.canWrite ? initiateReturn : null,
          ),
        ],
      ],
    ),
  );
  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => PopScope(
      canPop: !c.busy,
      child: Scaffold(
        appBar: AppBar(
          title: Text(
            widget.parcelId == null ? 'Packing & fulfilment' : 'Parcel details',
          ),
          actions: [
            IconButton(
              tooltip: 'Refresh fulfilment',
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
              if (c.loading && c.view == null && c.parcel == null)
                const PremiumSkeleton(detail: true)
              else ...[
                if (c.error != null) ...[
                  Text(c.error!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
                  const SizedBox(height: 16),
                  SettingsAction(
                    'Refresh',
                    secondary: true,
                    onPressed: c.loading || c.busy ? null : c.load,
                  ),
                  const SizedBox(height: 16),
                ],
                if (widget.parcelId == null) ...[
                  SettingsLink(
                    'Delivery decisions',
                    Icons.route_outlined,
                    subtitle: 'Authority, pickup timing & decision history',
                    onTap: () => Navigator.push(
                      context,
                      setupRoute<void>(
                        OrchestrationWorkspaceScreen(
                          runtime: widget.runtime,
                          groupId: widget.groupId,
                        ),
                      ),
                    ),
                  ),
                  for (final unit in c.units) ...[
                    SettingsCard(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          SettingsHeading(
                            dashboardLabel(unit['readiness']),
                            subtitle: unit['pickup_address']?.toString(),
                          ),
                          const SizedBox(height: 16),
                          SettingsAction(
                            'Pack a parcel',
                            icon: Icons.inventory_2_outlined,
                            onPressed: c.canWrite ? () => manifest(unit) : null,
                          ),
                          for (final mission in missionsFor(unit))
                            Padding(
                              padding: const EdgeInsets.only(top: 12),
                              child: SettingsAction(
                                'Verify rider pickup',
                                secondary: true,
                                icon: Icons.verified_user_outlined,
                                busy: counterLoading,
                                onPressed: c.canWrite && !counterLoading
                                    ? () => counterPickup(unit, mission)
                                    : null,
                              ),
                            ),
                          if (unit['readiness'] == 'NOT_READY' ||
                              unit['readiness'] == 'PACKING') ...[
                            const SizedBox(height: 12),
                            SettingsAction(
                              unit['readiness'] == 'NOT_READY'
                                  ? 'Start packing'
                                  : 'Mark pickup ready',
                              secondary: true,
                              onPressed: c.canWrite
                                  ? () => ready(
                                      unit,
                                      unit['readiness'] == 'NOT_READY'
                                          ? 'PACKING'
                                          : 'READY',
                                    )
                                  : null,
                            ),
                          ],
                        ],
                      ),
                    ),
                    const SizedBox(height: 16),
                  ],
                  const SettingsHeading('Parcels'),
                  const SizedBox(height: 14),
                  if (c.parcels.isEmpty)
                    const Text('No parcels have been packed yet.'),
                  for (final p in c.parcels) ...[
                    parcelCard(p),
                    const SizedBox(height: 16),
                  ],
                  if ((c.view?['returns'] as List? ?? []).isNotEmpty) ...[
                    const SizedBox(height: 12),
                    const SettingsHeading('Returns'),
                    for (final r in (c.view!['returns'] as List).map(
                      jsonObject,
                    ))
                      Padding(
                        padding: const EdgeInsets.symmetric(vertical: 10),
                        child: SettingsFact(
                          r['parcel_id'].toString(),
                          dashboardLabel(r['state']),
                        ),
                      ),
                  ],
                  const SizedBox(height: 20),
                  const SettingsHeading('Plan history'),
                  const SizedBox(height: 12),
                  if (c.historyError != null)
                    Text(c.historyError!)
                  else
                    for (final plan in c.plans)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 12),
                        child: SettingsCard(
                          child: SettingsHeading(
                            'Plan ${plan['version']}',
                            subtitle:
                                '${dashboardLabel(plan['status'])} ? ${dashboardLabel(plan['reason_code'])}',
                          ),
                        ),
                      ),
                ] else if (c.parcel != null) ...[
                  parcelCard(c.parcel!),
                  const SizedBox(height: 24),
                  const SettingsHeading('Custody history'),
                  const SizedBox(height: 14),
                  if (c.historyError != null)
                    Text(c.historyError!)
                  else if (c.custody.isEmpty)
                    const Text('No custody events have been recorded.')
                  else
                    SettingsCard(
                      child: Column(
                        children: [
                          for (final event in c.custody)
                            Padding(
                              padding: const EdgeInsets.symmetric(vertical: 12),
                              child: Row(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Icon(
                                    Icons.route_outlined,
                                    color: missionColor(
                                      event['event_type'],
                                    ).forInk(context),
                                  ),
                                  const SizedBox(width: 12),
                                  Expanded(
                                    child: SettingsHeading(
                                      dashboardLabel(event['event_type']),
                                      subtitle:
                                          '${dashboardLabel(event['to_custodian_type'])} ? ${supportDate(event['occurred_at'])}',
                                    ),
                                  ),
                                ],
                              ),
                            ),
                        ],
                      ),
                    ),
                ],
                if (c.attempt.pending != null) ...[
                  const SizedBox(height: 20),
                  const Text(
                    'The previous update was not confirmed. Retry the same update.',
                  ),
                  const SizedBox(height: 12),
                  SettingsAction(
                    'Retry original update',
                    busy: c.busy,
                    onPressed: c.loading || c.attempt.blocked
                        ? null
                        : () async => feedback(await c.retry()),
                  ),
                ],
                if (c.attempt.blocked)
                  Text(c.attempt.error ?? 'Recovery unavailable.'),
              ],
            ],
          ),
        ),
      ),
    ),
  );
}
