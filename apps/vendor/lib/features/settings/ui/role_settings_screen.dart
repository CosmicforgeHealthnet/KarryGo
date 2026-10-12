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

class RoleSettingsScreen extends StatefulWidget {
  const RoleSettingsScreen({
    super.key,
    required this.runtime,
    required this.record,
  });
  final VendorRuntime runtime;
  final JsonObject record;
  @override
  State<RoleSettingsScreen> createState() => _RoleSettingsScreenState();
}

class _RoleSettingsScreenState extends State<RoleSettingsScreen> {
  late final c = SettingsController(widget.runtime.backend, SettingsKind.roles);
  JsonObject? row;
  String? error;
  bool loading = false;
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
      final next = await c.readRecord(widget.record);
      if (mounted) setState(() => row = next);
    } catch (e) {
      if (mounted) {
        setState(() {
          error = SettingsController.message(e);
          row = null;
        });
      }
    } finally {
      if (mounted) setState(() => loading = false);
    }
  }

  Future<void> edit() async {
    await Navigator.push(
      context,
      setupRoute<bool>(
        RolePermissionEditor(runtime: widget.runtime, record: row),
      ),
    );
    await load();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Role details')),
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
            if (error != null)
              SettingsAction('Refresh', secondary: true, onPressed: load),
            if (row != null) ...[
              SettingsCard(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const SettingsEmblem(SettingsKind.roles),
                    const SizedBox(height: 18),
                    SettingsHeading(
                      row!['name'].toString(),
                      subtitle: row!['is_system'] == true
                          ? 'System role'
                          : 'Custom role',
                    ),
                    SettingsFact('Code', row!['code']),
                    if (row!['description']?.toString().isNotEmpty == true)
                      SettingsFact('Description', row!['description']),
                  ],
                ),
              ),
              const SizedBox(height: 24),
              const SettingsHeading('Permissions'),
              for (final code in row!['permission_codes'] as List? ?? [])
                SettingsLink(
                  code.toString(),
                  Icons.key_outlined,
                  onTap: null,
                  color: (settingsAccent(SettingsKind.roles)).forInk(context),
                ),
              if ((row!['permission_codes'] as List? ?? []).isEmpty)
                const Text('No permissions assigned.'),
              if (row!['is_system'] != true)
                SettingsAction(
                  'Edit permissions',
                  icon: Icons.tune_rounded,
                  onPressed:
                      !loading && c.allowed('PUT', '${c.detail}/permissions')
                      ? edit
                      : null,
                ),
            ],
          ],
        ),
      ),
    ),
  );
}

class RolePermissionEditor extends StatefulWidget {
  const RolePermissionEditor({super.key, required this.runtime, this.record});
  final VendorRuntime runtime;
  final JsonObject? record;
  @override
  State<RolePermissionEditor> createState() => _RolePermissionEditorState();
}

class _RolePermissionEditorState extends State<RolePermissionEditor> {
  late final c = SettingsController(widget.runtime.backend, SettingsKind.roles);
  final form = GlobalKey<FormState>();
  final code = TextEditingController(),
      name = TextEditingController(),
      description = TextEditingController();
  final selected = <String>{};
  List<JsonObject> permissions = [];
  JsonObject? record;
  String search = '', error = '';
  bool loading = false, saving = false, dirty = false, saved = false;
  bool get editing => widget.record != null;
  String get path => editing ? '${c.detail}/permissions' : c.collection;
  @override
  void initState() {
    super.initState();
    if (!editing && widget.runtime.mode == VendorDataMode.mock) {
      code.text = 'branch_support';
      name.text = 'Branch support';
      description.text = 'Support branch operations.';
      selected.add('vendor.orders.read');
    }
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) unawaited(load());
    });
  }

  @override
  void dispose() {
    code.dispose();
    name.dispose();
    description.dispose();
    c.dispose();
    super.dispose();
  }

  Future<void> load() async {
    if (dirty &&
            !await confirmSettings(
              context,
              'Reload permissions',
              'This replaces unsaved changes.',
            ) ||
        !mounted) {
      return;
    }
    setState(() {
      loading = true;
      error = '';
    });
    await c.load();
    try {
      final catalogue = SettingsController.objects(
        (await c.read('/api/v1/vendor/permissions')).data,
        'permissions',
      );
      final next = editing ? await c.readRecord(widget.record!) : null;
      if (!mounted) return;
      setState(() {
        permissions = catalogue;
        record = next;
        if (next != null) {
          code.text = next['code'].toString();
          name.text = next['name'].toString();
          description.text = next['description']?.toString() ?? '';
          selected
            ..clear()
            ..addAll((next['permission_codes'] as List? ?? []).cast<String>());
        }
        dirty = false;
      });
    } catch (e) {
      if (mounted) setState(() => error = SettingsController.message(e));
    } finally {
      if (mounted) setState(() => loading = false);
    }
  }

  Future<void> save() async {
    if (!(form.currentState?.validate() ?? false)) return;
    final risky = permissions.any(
      (p) => p['is_high_risk'] == true && selected.contains(p['code']),
    );
    if (!await confirmSettings(
          context,
          editing ? 'Update permissions' : 'Create role',
          '${name.text.trim()} will have ${selected.length} permissions.${risky ? ' Security verification is required.' : ''}',
        ) ||
        !mounted) {
      return;
    }
    String? grant;
    if (risky) {
      grant = await Navigator.push<String>(
        context,
        setupRoute<String>(SettingsVerificationScreen(runtime: widget.runtime)),
      );
      if (grant == null || !mounted) return;
    }
    setState(() {
      saving = true;
      error = '';
    });
    final body = <String, dynamic>{
      'permission_codes': selected.toList()..sort(),
      if (editing) 'expected_version': record!['version'],
      if (!editing) ...{
        'code': code.text.trim(),
        'name': name.text.trim(),
        'description': description.text.trim(),
      },
      'step_up_grant_id': ?grant,
    };
    final ok = await c.mutate(
      editing ? 'PUT' : 'POST',
      path,
      body,
      parameters: editing ? c.params(record) : {},
    );
    if (!mounted) return;
    setState(() {
      saving = false;
      error = ok ? '' : c.feedback ?? c.error ?? 'Change not accepted.';
      saved = ok;
    });
    if (ok) {
      await showOrderFeedback(
        context,
        title: editing ? 'Permissions updated' : 'Role created',
        message: c.feedback ?? 'Your role settings are saved.',
        success: true,
      );
      if (mounted) Navigator.pop(context, true);
    }
  }

  Widget field(
    TextEditingController controller,
    String label, {
    int lines = 1,
  }) => Padding(
    padding: const EdgeInsets.only(bottom: 18),
    child: TextFormField(
      controller: controller,
      enabled: !editing && !saving,
      maxLines: lines,
      decoration: InputDecoration(labelText: label),
      onChanged: (_) => setState(() {
        dirty = true;
        if (!c.stale) {
          error = '';
        }
      }),
      validator: (v) => label != 'Description' && (v?.trim().isEmpty ?? true)
          ? 'Enter ${label.toLowerCase()}.'
          : null,
    ),
  );
  @override
  Widget build(BuildContext context) => PopScope(
    canPop: saved || !dirty && !saving,
    onPopInvokedWithResult: (didPop, result) async {
      if (!didPop &&
          !saving &&
          await confirmSettings(
            context,
            'Discard changes',
            'Your changes have not been saved.',
          ) &&
          context.mounted) {
        setState(() => dirty = false);
        Navigator.pop(context);
      }
    },
    child: Scaffold(
      appBar: AppBar(title: Text(editing ? 'Edit permissions' : 'Create role')),
      bottomNavigationBar: AnimatedBuilder(
        animation: c,
        builder: (context, _) => SafeArea(
          child: Padding(
            padding: const EdgeInsets.fromLTRB(24, 12, 24, 16),
            child: SettingsAction(
              editing ? 'Save permissions' : 'Create role',
              busy: saving,
              onPressed:
                  !loading &&
                      error.isEmpty &&
                      (!editing || dirty) &&
                      record?['is_system'] != true &&
                      c.allowed(editing ? 'PUT' : 'POST', path)
                  ? save
                  : null,
            ),
          ),
        ),
      ),
      body: Form(
        key: form,
        child: ListView(
          padding: const EdgeInsets.all(24),
          children: [
            const SettingsEmblem(SettingsKind.roles),
            const SizedBox(height: 22),
            SettingsHeading(editing ? 'Control access' : 'Create a role'),
            if (loading) const PremiumSkeleton(rows: 2),
            if (error.isNotEmpty) ...[
              SettingsNote(error, warning: c.stale),
              SettingsAction(
                'Reload permissions',
                secondary: true,
                icon: Icons.refresh_rounded,
                onPressed: loading || saving ? null : load,
              ),
            ],
            field(name, 'Name'),
            field(code, 'Code'),
            field(description, 'Description', lines: 3),
            Text(
              '${selected.length} permissions selected',
              style: const TextStyle(fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 16),
            TextField(
              decoration: const InputDecoration(
                labelText: 'Find a permission',
                prefixIcon: Icon(Icons.search_rounded),
              ),
              onChanged: (v) => setState(() => search = v.trim().toLowerCase()),
            ),
            const SizedBox(height: 18),
            for (final category
                in permissions
                    .map((p) => p['category'].toString())
                    .toSet()) ...[
              if (permissions.any(
                (p) =>
                    p['category'] == category &&
                    '${p['code']} ${p['description']}'.toLowerCase().contains(
                      search,
                    ),
              ))
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 12),
                  child: Text(
                    category.toUpperCase(),
                    style: const TextStyle(
                      fontSize: 12,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
              for (final p in permissions.where(
                (p) =>
                    p['category'] == category &&
                    '${p['code']} ${p['description']}'.toLowerCase().contains(
                      search,
                    ),
              ))
                Padding(
                  padding: const EdgeInsets.only(bottom: 10),
                  child: SettingsCard(
                    child: CheckboxListTile(
                      contentPadding: EdgeInsets.zero,
                      controlAffinity: ListTileControlAffinity.leading,
                      value: selected.contains(p['code']),
                      activeColor: (settingsAccent(
                        SettingsKind.profile,
                      )).forInk(context),
                      title: Text(
                        p['description']?.toString().isNotEmpty == true
                            ? p['description'].toString()
                            : p['code'].toString(),
                      ),
                      subtitle: Text(
                        p['code'].toString(),
                        style: const TextStyle(fontSize: 11),
                      ),
                      secondary: p['is_high_risk'] == true
                          ? Icon(
                              Icons.shield_outlined,
                              color: Color(0xFF986013).forInk(context),
                            )
                          : null,
                      onChanged:
                          loading || saving || record?['is_system'] == true
                          ? null
                          : (v) => setState(() {
                              dirty = true;
                              if (!c.stale) {
                                error = '';
                              }
                              if (v == true) {
                                selected.add(p['code'] as String);
                              } else {
                                selected.remove(p['code']);
                              }
                            }),
                    ),
                  ),
                ),
            ],
          ],
        ),
      ),
    ),
  );
}
