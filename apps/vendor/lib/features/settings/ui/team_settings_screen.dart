import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import 'dart:async';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../state/settings_controller.dart';
import 'settings_design.dart';
import 'settings_verification_screen.dart';

class StaffSettingsScreen extends StatefulWidget {
  const StaffSettingsScreen({
    super.key,
    required this.runtime,
    required this.record,
  });
  final VendorRuntime runtime;
  final JsonObject record;
  @override
  State<StaffSettingsScreen> createState() => _StaffSettingsScreenState();
}

class _StaffSettingsScreenState extends State<StaffSettingsScreen> {
  late final c = SettingsController(widget.runtime.backend, SettingsKind.staff);
  JsonObject? record;
  List<JsonObject> assignments = [], roles = [], branches = [];
  String? error;
  bool loading = false;
  bool get own => widget.record['id'] == widget.runtime.auth.session?.staffId;
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) unawaited(load());
    });
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> load() async {
    setState(() {
      loading = true;
      error = null;
    });
    await c.load();
    try {
      final row = await c.readRecord(widget.record);
      final links = SettingsController.objects(
        (await c.read('${c.detail}/roles', parameters: c.params(row))).data,
        'assignments',
      );
      List<JsonObject> roleRows = [], branchRows = [];
      try {
        roleRows = SettingsController.objects(
          (await c.read('/api/v1/vendor/roles')).data,
          'roles',
        );
      } catch (_) {}
      try {
        branchRows = await c.all(
          '${SettingsController.business}/branches',
          c.scope,
        );
      } catch (_) {}
      if (mounted) {
        setState(() {
          record = row;
          assignments = links;
          roles = roleRows;
          branches = branchRows;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          error = SettingsController.message(e);
          record = null;
        });
      }
    } finally {
      if (mounted) setState(() => loading = false);
    }
  }

  String label(List<JsonObject> source, Object? id, String key) =>
      source.where((r) => r['id'] == id).firstOrNull?[key]?.toString() ??
      id?.toString() ??
      'Business-wide';
  Future<void> feedback(bool ok, String title) async {
    if (ok) await load();
    if (mounted) {
      await showOrderFeedback(
        context,
        title: ok ? title : 'Change not accepted',
        message: ok
            ? c.feedback ?? 'Staff access has been updated.'
            : c.feedback ?? c.error ?? 'Try again.',
        success: ok,
      );
    }
  }

  Future<void> remove(JsonObject row) async {
    if (!await confirmSettings(
          context,
          'Remove role',
          'Remove this role assignment from the staff member?',
          danger: true,
        ) ||
        !mounted) {
      return;
    }
    final ok = await c.mutate(
      'DELETE',
      '/api/v1/vendor/assignments/{id}',
      null,
      parameters: {'id': row['id'] as String},
    );
    await feedback(ok, 'Role removed');
  }

  Future<void> status() async {
    final suspend = record!['status'] == 'active';
    String reason = '';
    if (suspend) {
      final input = TextEditingController();
      final form = GlobalKey<FormState>();
      final result = await settingsSheet<String>(
        context,
        Form(
          key: form,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const SettingsHeading('Suspend staff access'),
              Text(
                own
                    ? 'This signs you out and revokes your active sessions.'
                    : 'This revokes the staff member?s active sessions.',
              ),
              const SizedBox(height: 20),
              TextFormField(
                controller: input,
                maxLines: 3,
                decoration: const InputDecoration(labelText: 'Reason'),
                validator: (v) =>
                    (v?.trim().isEmpty ?? true) ? 'Enter a reason.' : null,
              ),
            ],
          ),
        ),
        footer: SettingsAction(
          'Suspend access',
          danger: true,
          onPressed: () {
            if (form.currentState!.validate()) {
              Navigator.pop(context, input.text.trim());
            }
          },
        ),
      );
      input.dispose();
      if (result == null || !mounted) return;
      reason = result;
    } else if (!await confirmSettings(
          context,
          'Reactivate access',
          'Allow this staff member to sign in again?',
        ) ||
        !mounted) {
      return;
    }
    final ok = await c.mutate(
      'PUT',
      '${c.detail}/${suspend ? 'suspend' : 'reactivate'}',
      {'expected_version': record!['version'], if (suspend) 'reason': reason},
      parameters: c.params(record),
    );
    if (ok && suspend && own) {
      await widget.runtime.signOut();
      if (mounted) Navigator.of(context).popUntil((route) => route.isFirst);
      return;
    }
    await feedback(ok, suspend ? 'Access suspended' : 'Access reactivated');
  }

  Future<void> assign() async {
    try {
      final options = SettingsController.objects(
        (await c.read('/api/v1/vendor/roles')).data,
        'roles',
      );
      final scopes = await c.all(
        '${SettingsController.business}/branches',
        c.scope,
      );
      final catalogue = SettingsController.objects(
        (await c.read('/api/v1/vendor/permissions')).data,
        'permissions',
      );
      if (!mounted) return;
      String role = '', branch = '';
      final choice = await settingsSheet<JsonObject>(
        context,
        StatefulBuilder(
          builder: (context, setLocal) => Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const SettingsHeading('Assign role'),
              DropdownButtonFormField<String>(
                initialValue: role.isEmpty ? null : role,
                isExpanded: true,
                decoration: const InputDecoration(labelText: 'Role'),
                items: options
                    .map(
                      (r) => DropdownMenuItem(
                        value: r['id'] as String,
                        child: Text(r['name'].toString()),
                      ),
                    )
                    .toList(),
                onChanged: (v) => setLocal(() => role = v ?? ''),
              ),
              const SizedBox(height: 20),
              DropdownButtonFormField<String>(
                initialValue: branch,
                isExpanded: true,
                decoration: const InputDecoration(labelText: 'Scope'),
                items: [
                  const DropdownMenuItem(
                    value: '',
                    child: Text('Business-wide'),
                  ),
                  for (final b in scopes)
                    DropdownMenuItem(
                      value: b['id'] as String,
                      child: Text(b['display_name'].toString()),
                    ),
                ],
                onChanged: (v) => setLocal(() => branch = v ?? ''),
              ),
              const SizedBox(height: 24),
              SettingsAction(
                'Continue',
                onPressed: role.isEmpty
                    ? null
                    : () => Navigator.pop(context, {
                        'role_id': role,
                        'branch_id': branch.isEmpty ? null : branch,
                      }),
              ),
            ],
          ),
        ),
      );
      if (choice == null || !mounted) return;
      final selected = (await c.read(
        '/api/v1/vendor/roles/{id}',
        parameters: {'id': choice['role_id'] as String},
      )).object;
      final codes = (selected['permission_codes'] as List? ?? [])
          .cast<String>();
      final risky = catalogue.any(
        (p) => p['is_high_risk'] == true && codes.contains(p['code']),
      );
      if (!mounted ||
          !await confirmSettings(
            context,
            'Assign role',
            'Assign ${selected['name']} with ${choice['branch_id'] == null ? 'business-wide' : label(scopes, choice['branch_id'], 'display_name')} access?',
          ) ||
          !mounted) {
        return;
      }
      String? grant;
      if (risky) {
        grant = await Navigator.push<String>(
          context,
          setupRoute<String>(
            SettingsVerificationScreen(runtime: widget.runtime),
          ),
        );
        if (grant == null || !mounted) return;
      }
      final ok = await c.mutate(
        'POST',
        '/api/v1/vendor/roles/{id}/assign',
        {'staff_id': record!['id'], ...choice, 'step_up_grant_id': ?grant},
        parameters: {'id': choice['role_id'] as String},
      );
      await feedback(ok, 'Role assigned');
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Could not assign this role',
          message: SettingsController.message(e),
        );
      }
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Staff access')),
    body: AnimatedBuilder(
      animation: c,
      builder: (context, _) => RefreshIndicator(
        onRefresh: load,
        child: ListView(
          padding: const EdgeInsets.all(24),
          physics: const AlwaysScrollableScrollPhysics(),
          children: [
            if (loading) const PremiumSkeleton(rows: 2),
            if (error != null) SettingsNote(error!),
            if (error != null || c.stale)
              SettingsAction(
                'Refresh',
                secondary: true,
                onPressed: loading || c.busy ? null : load,
              ),
            if (record != null) ...[
              SettingsCard(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        const SettingsEmblem(SettingsKind.staff),
                        const Spacer(),
                        SettingsStatus(record!['status']),
                      ],
                    ),
                    const SizedBox(height: 22),
                    SettingsHeading(own ? 'Your staff access' : 'Staff member'),
                    SettingsFact('Staff ID', record!['id']),
                    if (record!['suspended_reason']?.toString().isNotEmpty ==
                        true)
                      SettingsFact(
                        'Suspension reason',
                        record!['suspended_reason'],
                      ),
                  ],
                ),
              ),
              const SizedBox(height: 24),
              const SettingsHeading('Assigned roles'),
              for (final row in assignments)
                Padding(
                  padding: const EdgeInsets.only(bottom: 14),
                  child: SettingsCard(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Icon(
                          Icons.admin_panel_settings_outlined,
                          color: (settingsAccent(
                            SettingsKind.roles,
                          )).forInk(context),
                        ),
                        const SizedBox(height: 14),
                        Text(
                          label(roles, row['role_id'], 'name'),
                          style: const TextStyle(
                            fontSize: 20,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                        const SizedBox(height: 8),
                        Text(label(branches, row['branch_id'], 'display_name')),
                        const SizedBox(height: 18),
                        SettingsAction(
                          'Remove role',
                          secondary: true,
                          danger: true,
                          icon: Icons.remove_circle_outline,
                          onPressed:
                              !loading &&
                                  c.allowed(
                                    'DELETE',
                                    '/api/v1/vendor/assignments/{id}',
                                  )
                              ? () => remove(row)
                              : null,
                        ),
                      ],
                    ),
                  ),
                ),
              if (assignments.isEmpty)
                const Padding(
                  padding: EdgeInsets.only(bottom: 24),
                  child: Text('No roles assigned.'),
                ),
              SettingsAction(
                'Assign role',
                icon: Icons.add_rounded,
                busy: c.busy,
                onPressed:
                    !loading &&
                        c.allowed('POST', '/api/v1/vendor/roles/{id}/assign')
                    ? assign
                    : null,
              ),
              const SizedBox(height: 12),
              SettingsAction(
                record!['status'] == 'active'
                    ? 'Suspend access'
                    : 'Reactivate access',
                secondary: true,
                danger: record!['status'] == 'active',
                icon: record!['status'] == 'active'
                    ? Icons.person_off_outlined
                    : Icons.person_add_alt_1_outlined,
                onPressed:
                    !loading &&
                        c.allowed(
                          'PUT',
                          '${c.detail}/${record!['status'] == 'active' ? 'suspend' : 'reactivate'}',
                        )
                    ? status
                    : null,
              ),
            ],
          ],
        ),
      ),
    ),
  );
}
