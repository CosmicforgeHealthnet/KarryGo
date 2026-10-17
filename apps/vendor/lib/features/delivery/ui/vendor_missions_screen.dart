import 'hybrid_fallback_screen.dart';
import 'fulfilment_workspace_screen.dart';
import 'rider_assignment_screen.dart';
import 'rider_workspace_screen.dart';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../settings/ui/settings_design.dart';
import '../../support/ui/order_support_screen.dart' show supportDate;
import '../state/vendor_missions_controller.dart';

String missionActionLabel(String status) => switch (status) {
  'READY_FOR_PICKUP' => 'Ready for pickup',
  'OUT_FOR_DELIVERY' => 'Out for delivery',
  'RETURN_REQUIRED' => 'Require return',
  _ => 'Report delivery failure',
};
Color missionColor(Object? status) => switch (status) {
  'DELIVERED' || 'RETURNED' => KarryamColors.tealInk,
  'FAILED' => KarryamColors.error,
  'RETURN_REQUIRED' => const Color(0xFF986013),
  _ => const Color(0xFF326AA2),
};

class VendorMissionsScreen extends StatefulWidget {
  const VendorMissionsScreen({super.key, required this.runtime, this.id});
  final VendorRuntime runtime;
  final String? id;
  @override
  State<VendorMissionsScreen> createState() => _VendorMissionsScreenState();
}

class _VendorMissionsScreenState extends State<VendorMissionsScreen> {
  late final c = VendorMissionsController(
    widget.runtime.backend,
    id: widget.id,
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

  Future<void> update(String status) async {
    String? reason;
    if (status == 'FAILED') {
      final text = TextEditingController();
      reason = await settingsSheet<String>(
        context,
        StatefulBuilder(
          builder: (context, set) => Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const SettingsHeading('Report delivery failure'),
              const SizedBox(height: 18),
              TextField(
                controller: text,
                maxLines: 3,
                decoration: const InputDecoration(labelText: 'What happened?'),
                onChanged: (_) => set(() {}),
              ),
              const SizedBox(height: 24),
              SettingsAction(
                'Review',
                onPressed: text.text.trim().isEmpty
                    ? null
                    : () => Navigator.pop(context, text.text.trim()),
              ),
            ],
          ),
        ),
      );
      text.dispose();
      if (reason == null || !mounted) return;
    }
    if (!mounted) return;
    final confirm = await confirmSettings(
      context,
      missionActionLabel(status),
      reason ??
          'Update this vendor delivery to ${dashboardLabel(status).toLowerCase()}?',
      danger: status == 'FAILED',
    );
    if (!confirm || !mounted) return;
    final success = await c.update(status, reason: reason);
    if (!mounted) return;
    await showOrderFeedback(
      context,
      title: success ? 'Delivery updated' : 'Update needs attention',
      message: success
          ? 'The current delivery status has been saved.'
          : c.error ?? 'Refresh and try again.',
      success: success,
    );
  }

  Widget mission(JsonObject value) => SettingsCard(
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Container(
              padding: const EdgeInsets.all(14),
              decoration: BoxDecoration(
                color: missionColor(
                  value['status'],
                ).withValues(alpha: .10).forSurface(context),
                borderRadius: BorderRadius.circular(18),
              ),
              child: Icon(
                Icons.local_shipping_outlined,
                size: 30,
                color: missionColor(value['status']).forInk(context),
              ),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: SettingsHeading(
                dashboardLabel(value['status']),
                subtitle: 'Vendor delivery',
              ),
            ),
          ],
        ),
        const SizedBox(height: 20),
        if (widget.id != null) ...[
          SettingsFact('Mission', value['vendor_mission_id'].toString()),
          const SizedBox(height: 14),
          if (value['assigned_staff_id'] != null) ...[
            SettingsFact(
              'Assigned rider',
              value['assigned_staff_id'].toString(),
            ),
            const SizedBox(height: 14),
          ],
          if (value['failure_reason'] != null) ...[
            SettingsFact('Failure reason', value['failure_reason'].toString()),
            const SizedBox(height: 14),
          ],
          if (value['updated_at'] != null)
            SettingsFact('Last updated', supportDate(value['updated_at'])),
          if (value['stops'] is List &&
              (value['stops'] as List).isNotEmpty) ...[
            const SizedBox(height: 24),
            const SettingsHeading('Delivery stops'),
            const SizedBox(height: 12),
            for (final stop in (value['stops'] as List).map(jsonObject))
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 10),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Icon(
                      stop['stop_type'] == 'PICKUP'
                          ? Icons.storefront_outlined
                          : Icons.location_on_outlined,
                      color: KarryamColors.tealInk.forInk(context),
                    ),
                    const SizedBox(width: 12),
                    Expanded(
                      child: SettingsHeading(
                        dashboardLabel(stop['stop_type']),
                        subtitle: stop['address']?.toString(),
                      ),
                    ),
                  ],
                ),
              ),
          ],
          if (value['parcel_ids'] is List) ...[
            const SizedBox(height: 16),
            SettingsFact(
              'Packages',
              (value['parcel_ids'] as List).length.toString(),
            ),
          ],
        ] else ...[
          Text(
            value['vendor_mission_id'].toString(),
            style: Theme.of(context).textTheme.bodySmall,
          ),
          const SizedBox(height: 16),
          SettingsAction(
            'View delivery',
            secondary: true,
            onPressed: () async {
              await Navigator.push(
                context,
                setupRoute<void>(
                  VendorMissionsScreen(
                    runtime: widget.runtime,
                    id: value['vendor_mission_id'] as String,
                  ),
                ),
              );
              if (mounted) await c.load();
            },
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
            widget.id == null ? 'Vendor deliveries' : 'Delivery details',
          ),
          actions: [
            IconButton(
              tooltip: 'Refresh deliveries',
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
              if (c.loading && c.record == null && c.missions.isEmpty)
                const PremiumSkeleton(detail: true)
              else ...[
                if (c.error != null) ...[
                  SettingsCard(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const SettingsHeading('Delivery unavailable'),
                        Text(c.error!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
                        const SizedBox(height: 16),
                        SettingsAction(
                          'Refresh',
                          secondary: true,
                          onPressed: c.loading || c.busy ? null : c.load,
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 16),
                ],
                if (widget.id == null && c.missions.isEmpty && c.error == null)
                  const DashboardEmptyState(
                    story: EmptyStory.orders,
                    title: 'No vendor deliveries',
                    message: 'Vendor-owned delivery missions will appear here.',
                    iconOverride: Icons.local_shipping_outlined,
                    detailIconOverride: Icons.inventory_2_outlined,
                  ),
                if (widget.id == null)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 16),
                    child: SettingsAction(
                      'My rider workspace',
                      icon: Icons.route_outlined,
                      onPressed: () => Navigator.push(
                        context,
                        setupRoute<void>(
                          RiderWorkspaceScreen(runtime: widget.runtime),
                        ),
                      ),
                    ),
                  ),
                if (c.record != null) mission(c.record!),
                for (final row in c.missions)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 16),
                    child: mission(row),
                  ),
                if (c.loading || c.busy)
                  const Padding(
                    padding: EdgeInsets.all(16),
                    child: Center(
                      child: SizedBox(
                        width: 22,
                        height: 22,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      ),
                    ),
                  ),
                if (c.record != null &&
                    !(c.record!['failure_reason']?.toString().startsWith(
                          'HYBRID_FALLBACK:',
                        ) ??
                        false) &&
                    !c.stale &&
                    !c.loading &&
                    ![
                      'CUSTODY_TRANSFERRED',
                      'OUT_FOR_DELIVERY',
                      'RETURN_REQUIRED',
                      'DELIVERED',
                      'RETURNED',
                    ].contains(c.record!['status']))
                  Padding(
                    padding: const EdgeInsets.only(top: 12),
                    child: SettingsAction(
                      c.record!['assigned_staff_id'] == null
                          ? 'Assign rider'
                          : 'Change rider',
                      secondary: true,
                      icon: Icons.delivery_dining_rounded,
                      onPressed: c.busy
                          ? null
                          : () async {
                              await Navigator.push(
                                context,
                                setupRoute<bool>(
                                  RiderAssignmentScreen(
                                    runtime: widget.runtime,
                                    mission: c.record!,
                                  ),
                                ),
                              );
                              if (mounted) await c.load();
                            },
                    ),
                  ),
                if (c.record?['fulfilment_group_id'] is String)
                  Padding(
                    padding: const EdgeInsets.only(top: 12),
                    child: SettingsAction(
                      'Packing & parcels',
                      secondary: true,
                      icon: Icons.inventory_2_outlined,
                      onPressed: () => Navigator.push(
                        context,
                        setupRoute<void>(
                          FulfilmentWorkspaceScreen(
                            runtime: widget.runtime,
                            groupId: c.record!['fulfilment_group_id'] as String,
                          ),
                        ),
                      ),
                    ),
                  ),
                if (c.record != null &&
                    !(c.record!['failure_reason']?.toString().startsWith(
                          'HYBRID_FALLBACK:',
                        ) ??
                        false) &&
                    !c.stale &&
                    ![
                      'CUSTODY_TRANSFERRED',
                      'OUT_FOR_DELIVERY',
                      'RETURN_REQUIRED',
                      'DELIVERED',
                      'RETURNED',
                    ].contains(c.record!['status']))
                  Padding(
                    padding: const EdgeInsets.only(top: 12),
                    child: SettingsAction(
                      'Platform fallback',
                      secondary: true,
                      icon: Icons.alt_route_rounded,
                      onPressed: c.busy || c.loading
                          ? null
                          : () async {
                              await Navigator.push(
                                context,
                                setupRoute<void>(
                                  HybridFallbackScreen(
                                    runtime: widget.runtime,
                                    mission: c.record!,
                                  ),
                                ),
                              );
                              if (mounted) await c.load();
                            },
                    ),
                  ),
                for (final status in c.actions)
                  Padding(
                    padding: const EdgeInsets.only(top: 12),
                    child: SettingsAction(
                      missionActionLabel(status),
                      danger: status == 'FAILED',
                      secondary: status == 'RETURN_REQUIRED',
                      icon: status == 'FAILED'
                          ? Icons.report_outlined
                          : status == 'RETURN_REQUIRED'
                          ? Icons.assignment_return_outlined
                          : Icons.local_shipping_outlined,
                      onPressed: () => update(status),
                    ),
                  ),
                if (c.record != null &&
                    [
                      'READY_FOR_PICKUP',
                      'OUT_FOR_DELIVERY',
                      'RETURN_REQUIRED',
                    ].contains(c.record!['status']))
                  const Padding(
                    padding: EdgeInsets.only(top: 20),
                    child: Text(
                      'The assigned rider confirms pickup, delivery and returns through secure handover.',
                    ),
                  ),
                if (widget.id == null && c.missions.length == 200)
                  const Padding(
                    padding: EdgeInsets.only(top: 12),
                    child: Text('Showing the latest 200 vendor deliveries.'),
                  ),
              ],
            ],
          ),
        ),
      ),
    ),
  );
}
