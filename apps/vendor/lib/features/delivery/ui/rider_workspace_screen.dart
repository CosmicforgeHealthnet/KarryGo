import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import '../../settings/ui/settings_design.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../state/rider_workspace_controller.dart';
import 'vendor_missions_screen.dart' show missionColor;

String riderActionLabel(String action) => switch (action) {
  'acknowledge' => 'Accept assignment',
  'start-pickup' => 'Head to pickup',
  'arrive-pickup' => 'I?m at pickup',
  'complete-pickup' => 'Confirm verified pickup',
  'start-receiver' => 'Head to receiver',
  'arrive-receiver' => 'I?ve arrived',
  'complete-delivery' => 'Verify delivery',
  'receiver-unavailable' => 'Receiver unavailable',
  _ => 'Verify return',
};

class RiderWorkspaceScreen extends StatefulWidget {
  const RiderWorkspaceScreen({super.key, required this.runtime, this.id});
  final VendorRuntime runtime;
  final String? id;
  @override
  State<RiderWorkspaceScreen> createState() => _RiderWorkspaceScreenState();
}

class _RiderWorkspaceScreenState extends State<RiderWorkspaceScreen> {
  late final c = RiderWorkspaceController(widget.runtime, id: widget.id);
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

  Future<void> feedback(bool ok) async {
    if (!mounted) return;
    await showOrderFeedback(
      context,
      title: ok ? 'Delivery updated' : 'Update needs attention',
      message: ok
          ? 'Your delivery progress has been saved.'
          : c.error ?? 'Refresh and try again.',
      success: ok,
    );
  }

  Future<void> action(String action) async {
    final secure = ['complete-delivery', 'return'].contains(action);
    final pending = c.attempt.pending;
    final saved = pending == null
        ? <String, dynamic>{}
        : jsonObject(pending['body']);
    final mock = widget.runtime.mode == VendorDataMode.mock;
    final evidence = TextEditingController(
      text:
          saved['evidence_ref']?.toString() ??
          (mock
              ? 'demo-handover-${widget.id}-${action == 'return' ? 'return' : 'delivery'}'
              : ''),
    );
    final credential = TextEditingController(text: mock ? '654321' : '');
    final reason = TextEditingController(
      text:
          saved['reason_code']?.toString() ??
          (mock ? 'Receiver could not be reached' : ''),
    );
    String method = saved['method']?.toString() ?? 'OTP';
    JsonObject? input = {};
    if (secure || action == 'receiver-unavailable') {
      input = await settingsSheet<JsonObject>(
        context,
        StatefulBuilder(
          builder: (context, set) => Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              SettingsHeading(
                riderActionLabel(action),
                subtitle: secure
                    ? 'Use the reference and credential issued for this handover.'
                    : null,
              ),
              const SizedBox(height: 20),
              if (secure) ...[
                TextField(
                  controller: evidence,
                  readOnly: pending != null,
                  decoration: const InputDecoration(
                    labelText: 'Handover reference',
                  ),
                  onChanged: (_) => set(() {}),
                ),
                const SizedBox(height: 16),
                SegmentedButton<String>(
                  segments: const [
                    ButtonSegment(
                      value: 'OTP',
                      label: Text('OTP'),
                      icon: Icon(Icons.password_rounded),
                    ),
                    ButtonSegment(
                      value: 'QR',
                      label: Text('QR value'),
                      icon: Icon(Icons.qr_code_rounded),
                    ),
                  ],
                  selected: {method},
                  showSelectedIcon: false,
                  onSelectionChanged: pending != null
                      ? null
                      : (v) => set(() => method = v.single),
                ),
                const SizedBox(height: 16),
                TextField(
                  controller: credential,
                  obscureText: true,
                  autocorrect: false,
                  enableSuggestions: false,
                  keyboardType: method == 'OTP'
                      ? TextInputType.number
                      : TextInputType.text,
                  decoration: InputDecoration(
                    labelText: method == 'OTP'
                        ? 'Verification code'
                        : 'QR credential',
                  ),
                  onChanged: (_) => set(() {}),
                ),
              ] else
                TextField(
                  controller: reason,
                  maxLines: 3,
                  readOnly: pending != null,
                  decoration: const InputDecoration(labelText: 'Reason'),
                  onChanged: (_) => set(() {}),
                ),
              const SizedBox(height: 24),
              SettingsAction(
                'Review verification',
                onPressed: secure
                    ? (evidence.text.trim().isEmpty || credential.text.isEmpty
                          ? null
                          : () => Navigator.pop(context, {
                              'evidence': evidence.text.trim(),
                              'method': method,
                              'credential': credential.text,
                            }))
                    : reason.text.trim().isEmpty
                    ? null
                    : () => Navigator.pop(context, {
                        'reason': reason.text.trim(),
                      }),
              ),
            ],
          ),
        ),
      );
    }
    evidence.dispose();
    credential.dispose();
    reason.dispose();
    if (input == null || !mounted) return;
    if (pending == null) {
      final confirm = await confirmSettings(
        context,
        riderActionLabel(action),
        action == 'complete-pickup'
            ? 'Confirm the branch has verified the pickup handover before taking custody.'
            : secure
            ? 'Verify this handover with the issued credential?'
            : input['reason']?.toString() ??
                  'Confirm this delivery progress update.',
      );
      if (!confirm || !mounted) return;
    }
    final ok = await c.execute(
      action,
      evidence: input['evidence'] as String?,
      method: input['method'] as String?,
      credential: input['credential'] as String?,
      reason: input['reason'] as String?,
    );
    await feedback(ok);
  }

  Future<void> availability() async {
    final pending = c.attempt.pending;
    if (pending != null) {
      final body = jsonObject(pending['body']);
      await feedback(
        await c.setAvailability(
          body['branch_id'] as String,
          body['state'] as String,
        ),
      );
      return;
    }
    String? branch = c.availability?['branch_id'] as String?;
    if (!c.branches.any((b) => b['id'] == branch)) {
      branch = c.branches.length == 1
          ? c.branches.single['id'] as String
          : null;
    }
    String state = c.availability?['state'] == 'AVAILABLE'
        ? 'OFFLINE'
        : 'AVAILABLE';
    final input = await settingsSheet<JsonObject>(
      context,
      StatefulBuilder(
        builder: (context, set) => Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const SettingsHeading('Rider availability'),
            const SizedBox(height: 20),
            DropdownButtonFormField<String>(
              initialValue: branch,
              isExpanded: true,
              decoration: const InputDecoration(labelText: 'Branch'),
              items: [
                for (final b in c.branches)
                  DropdownMenuItem(
                    value: b['id'] as String,
                    child: Text(
                      b['display_name']?.toString() ?? b['id'].toString(),
                    ),
                  ),
              ],
              onChanged: (v) => set(() => branch = v),
            ),
            const SizedBox(height: 16),
            SegmentedButton<String>(
              segments: const [
                ButtonSegment(value: 'AVAILABLE', label: Text('Available')),
                ButtonSegment(value: 'OFFLINE', label: Text('Offline')),
              ],
              selected: {state},
              showSelectedIcon: false,
              onSelectionChanged: (v) => set(() => state = v.single),
            ),
            const SizedBox(height: 24),
            SettingsAction(
              'Save availability',
              onPressed: branch == null
                  ? null
                  : () => Navigator.pop(context, {
                      'branch': branch,
                      'state': state,
                    }),
            ),
          ],
        ),
      ),
    );
    if (input == null || !mounted) return;
    await feedback(
      await c.setAvailability(
        input['branch'] as String,
        input['state'] as String,
      ),
    );
  }

  Widget card(JsonObject value) {
    final mission = jsonObject(value['mission']);
    final stage = value['stage'];
    return SettingsCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                padding: const EdgeInsets.all(14),
                decoration: BoxDecoration(
                  color: missionColor(
                    mission['status'],
                  ).withValues(alpha: .10).forSurface(context),
                  borderRadius: BorderRadius.circular(18),
                ),
                child: Icon(
                  Icons.delivery_dining_rounded,
                  size: 32,
                  color: missionColor(mission['status']).forInk(context),
                ),
              ),
              const SizedBox(width: 14),
              Expanded(
                child: SettingsHeading(
                  dashboardLabel(stage),
                  subtitle: dashboardLabel(mission['status']),
                ),
              ),
            ],
          ),
          const SizedBox(height: 20),
          if (widget.id != null) ...[
            for (final stop in (mission['stops'] as List? ?? []).map(
              jsonObject,
            ))
              Padding(
                padding: const EdgeInsets.only(bottom: 16),
                child: SettingsFact(
                  dashboardLabel(stop['stop_type']),
                  stop['address']?.toString() ?? 'Address not supplied',
                ),
              ),
            if (value['receiver_unavailable_reason'] != null)
              SettingsFact(
                'Receiver unavailable',
                value['receiver_unavailable_reason'].toString(),
              ),
            if (stage == 'COMPLETED' || stage == 'RETURNED')
              const SettingsHeading('Handover complete'),
          ] else
            SettingsAction(
              'Open assigned delivery',
              secondary: true,
              onPressed: () async {
                await Navigator.push(
                  context,
                  setupRoute<void>(
                    RiderWorkspaceScreen(
                      runtime: widget.runtime,
                      id: mission['vendor_mission_id'] as String,
                    ),
                  ),
                );
                if (mounted) await c.load();
              },
            ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => PopScope(
      canPop: !c.busy,
      child: Scaffold(
        appBar: AppBar(
          title: Text(
            widget.id == null ? 'My rider workspace' : 'Assigned delivery',
          ),
          actions: [
            IconButton(
              tooltip: 'Refresh rider workspace',
              onPressed: c.busy || c.loading ? null : c.load,
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
                        const SettingsHeading('Update needs attention'),
                        Text(c.error!),
                        const SizedBox(height: 16),
                        SettingsAction(
                          'Refresh',
                          secondary: true,
                          onPressed: c.busy || c.loading ? null : c.load,
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 16),
                ],
                if (widget.id == null) ...[
                  SettingsCard(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        SettingsHeading(
                          c.availability == null
                              ? 'Set your availability'
                              : dashboardLabel(c.availability!['state']),
                          subtitle: 'Your rider status',
                        ),
                        const SizedBox(height: 16),
                        SettingsAction(
                          c.attempt.pending != null
                              ? 'Retry availability update'
                              : 'Update availability',
                          secondary: true,
                          icon: Icons.toggle_on_outlined,
                          onPressed:
                              c.busy ||
                                  c.loading ||
                                  c.attempt.blocked ||
                                  c.branchError != null &&
                                      c.attempt.pending == null ||
                                  c.attempt.pending == null &&
                                      (c.availability?['state'] == 'BUSY' ||
                                          c.availability?['current_vendor_mission_id']
                                                  ?.toString()
                                                  .isNotEmpty ==
                                              true)
                              ? null
                              : availability,
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 20),
                  if (c.branchError != null) Text(c.branchError!),
                  if (c.missions.isEmpty && c.error == null)
                    const DashboardEmptyState(
                      story: EmptyStory.orders,
                      title: 'No assigned deliveries',
                      message: 'Your assigned deliveries will appear here.',
                      iconOverride: Icons.delivery_dining_rounded,
                      detailIconOverride: Icons.route_outlined,
                    ),
                ],
                if (c.record != null) card(c.record!),
                for (final value in c.missions)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 16),
                    child: card(value),
                  ),
                if (c.busy)
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
                for (final a in c.actions)
                  Padding(
                    padding: const EdgeInsets.only(top: 12),
                    child: SettingsAction(
                      riderActionLabel(a),
                      secondary: a == 'receiver-unavailable',
                      icon: a == 'receiver-unavailable'
                          ? Icons.person_off_outlined
                          : [
                              'complete-pickup',
                              'complete-delivery',
                              'return',
                            ].contains(a)
                          ? Icons.verified_user_outlined
                          : Icons.route_outlined,
                      onPressed: () => action(a),
                    ),
                  ),
                if (c.attempt.pending != null && widget.id != null) ...[
                  const SizedBox(height: 20),
                  const Text(
                    'The previous action could not be confirmed. Retry it with the same details.',
                  ),
                  const SizedBox(height: 12),
                  SettingsAction(
                    'Retry original action',
                    icon: Icons.refresh_rounded,
                    onPressed: c.busy || c.loading || c.attempt.blocked
                        ? null
                        : () => action(
                            jsonObject(c.attempt.pending!['params'])['action']
                                as String,
                          ),
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
