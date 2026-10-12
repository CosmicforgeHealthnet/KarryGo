import '../../onboarding/ui/business_onboarding_screen.dart';
import '../../delivery/ui/delivery_policy_screen.dart';
import '../../../core/ui/premium_loading.dart';
import 'dart:async';
import 'package:flutter/material.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../state/settings_controller.dart';
import 'settings_design.dart';
import 'settings_editor.dart';
import 'branch_hours_screen.dart';

const branchSettingsBase = '${SettingsController.business}/branches/{branchId}';

class BranchSettingsScreen extends StatefulWidget {
  const BranchSettingsScreen({
    super.key,
    required this.runtime,
    required this.record,
  });
  final VendorRuntime runtime;
  final JsonObject record;
  @override
  State<BranchSettingsScreen> createState() => _BranchSettingsScreenState();
}

class _BranchSettingsScreenState extends State<BranchSettingsScreen> {
  late final c = SettingsController(
    widget.runtime.backend,
    SettingsKind.branches,
  );
  JsonObject? _row;
  String? _error;
  bool _loading = false;
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) unawaited(_load());
    });
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    await c.load();
    try {
      final row = await c.readRecord(widget.record);
      if (mounted) setState(() => _row = row);
    } catch (e) {
      if (mounted) setState(() => _error = SettingsController.message(e));
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _edit() async {
    try {
      final brands = await c.all(
        '${SettingsController.business}/brands',
        c.scope,
      );
      if (!mounted) return;
      final result = await Navigator.push<bool>(
        context,
        setupRoute<bool>(
          SettingsEditor(
            runtime: widget.runtime,
            kind: SettingsKind.branches,
            record: _row!,
            brands: brands,
            onSave: (body) =>
                c.mutate('PATCH', c.detail, body, parameters: c.params(_row)),
            onReload: () async {
              await c.load();
              return c.readRecord(_row!);
            },
            canSave: () => c.allowed('PATCH', c.detail),
            errorMessage: () => c.feedback ?? c.error,
          ),
        ),
      );
      if (result == true) {
        await _load();
        if (mounted) {
          await showOrderFeedback(
            context,
            title: 'Branch updated',
            message: 'Your branch details are saved.',
            success: true,
          );
        }
      }
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Could not open branch details',
          message: SettingsController.message(e),
        );
      }
    }
  }

  Future<void> _activation() async {
    if (_loading || c.busy) return;
    setState(() => _loading = true);
    try {
      final readiness = (await c.app.onboardingApi.request(
        'GET',
        '/api/v1/vendor/onboarding/activation-readiness',
      )).object;
      if (!mounted) return;
      final ready = readiness['ready'] == true;
      final choice = await showModalBottomSheet<String>(
        context: context,
        isScrollControlled: true,
        useSafeArea: true,
        showDragHandle: true,
        builder: (context) => SingleChildScrollView(
          child: Padding(
            padding: const EdgeInsets.fromLTRB(24, 8, 24, 24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const SettingsHeading(
                  'Activation review',
                  subtitle: 'Business setup and verification',
                ),
                const SizedBox(height: 16),
                SettingsNote(
                  ready
                      ? 'Your business meets the activation requirements. The server will activate eligible draft branches and business types.'
                      : 'Complete the requirements below before requesting activation.',
                  warning: !ready,
                ),
                for (final raw in readiness['blockers'] as List? ?? [])
                  SettingsCard(
                    child: SettingsLink(
                      dashboardLabel((raw as Map)['code']),
                      Icons.pending_actions_rounded,
                      onTap: null,
                      color: const Color(0xFF986013).forInk(context),
                      subtitle: dashboardLabel(raw['blocker'] ?? raw['status']),
                    ),
                  ),
                const SizedBox(height: 16),
                SettingsAction(
                  'Request activation',
                  icon: Icons.verified_outlined,
                  onPressed: ready
                      ? () => Navigator.pop(context, 'activate')
                      : null,
                ),
                if (!ready) ...[
                  const SizedBox(height: 12),
                  SettingsAction(
                    'Open business setup',
                    secondary: true,
                    icon: Icons.tune_rounded,
                    onPressed: () => Navigator.pop(context, 'setup'),
                  ),
                ],
              ],
            ),
          ),
        ),
      );
      if (!mounted) return;
      if (choice == 'setup') {
        await Navigator.push(
          context,
          setupRoute<void>(
            BusinessOnboardingScreen(
              runtime: widget.runtime,
              onSettings: () => Navigator.pop(context),
              onActivated: () => Navigator.pop(context),
            ),
          ),
        );
        if (mounted) await _load();
        return;
      }
      if (choice != 'activate') return;
      final accepted = await c.mutate(
        'POST',
        '/api/v1/vendor/onboarding/request-activation',
        null,
      );
      if (!mounted) return;
      await _load();
      if (!mounted) return;
      final activated = accepted && _row?['status'] == 'ACTIVE';
      await showOrderFeedback(
        context,
        title: activated ? 'Branch activated' : 'Activation pending',
        message: activated
            ? 'Your branch is active.'
            : c.error ??
                  'Refresh to check the current activation requirements.',
        success: activated,
        warning: !activated,
      );
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Could not check activation',
          message: SettingsController.message(e),
        );
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _status() async {
    final pause = _row!['status'] == 'ACTIVE',
        action = pause ? 'pause' : 'resume',
        path = '$branchSettingsBase/$action';
    if (!await confirmSettings(
          context,
          pause ? 'Pause branch' : 'Resume branch',
          pause
              ? 'This pauses availability for ${_row!['display_name']}.'
              : 'Resume ${_row!['display_name']}?',
          danger: pause,
        ) ||
        !mounted) {
      return;
    }
    final result = await c.mutate('POST', path, {
      'expected_version': _row!['version'],
    }, parameters: c.params(_row));
    if (result) await _load();
    if (mounted) {
      await showOrderFeedback(
        context,
        title: result
            ? pause
                  ? 'Branch paused'
                  : 'Branch resumed'
            : 'Status not changed',
        message: result
            ? 'The branch status is updated.'
            : c.feedback ?? c.error ?? 'Try again.',
        success: result && !pause,
        warning: result && pause,
      );
    }
  }

  Future<void> _hours() async {
    final saved = await Navigator.push<bool>(
      context,
      setupRoute<bool>(
        BranchHoursScreen(runtime: widget.runtime, branch: _row!),
      ),
    );
    if (saved == true) await _load();
  }

  Future<void> _configuration(String suffix) async {
    final saved = await settingsSheet<bool>(
      context,
      BranchConfigSheet(runtime: widget.runtime, branch: _row!, suffix: suffix),
    );
    if (saved == true) {
      await _load();
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Settings saved',
          message: suffix == 'service-area'
              ? 'The branch delivery radius is updated.'
              : 'Order preferences are updated.',
          success: true,
        );
      }
    }
  }

  Future<void> _types() async {
    await Navigator.push(
      context,
      setupRoute<void>(
        BranchTypesScreen(runtime: widget.runtime, branch: _row!),
      ),
    );
    await _load();
  }

  @override
  Widget build(BuildContext context) {
    final row = _row;
    return Scaffold(
      appBar: AppBar(
        title: const Text('Branch details'),
        backgroundColor: KarryamColors.canvas.forSurface(context),
      ),
      body: AnimatedBuilder(
        animation: c,
        builder: (context, _) => RefreshIndicator(
          onRefresh: _load,
          child: ListView(
            padding: const EdgeInsets.all(24),
            physics: const AlwaysScrollableScrollPhysics(),
            children: [
              if (_loading) const PremiumSkeleton(rows: 2),
              if (_error != null) SettingsNote(_error!),
              if (row != null) ...[
                SettingsCard(
                  color: Color(0xFFE8F4EF).forInk(context),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          const SettingsEmblem(SettingsKind.branches, size: 78),
                          const Spacer(),
                          SettingsStatus(row['status']),
                        ],
                      ),
                      const SizedBox(height: 18),
                      Text(
                        row['display_name'].toString(),
                        style: const TextStyle(
                          fontSize: 28,
                          fontWeight: FontWeight.w700,
                          letterSpacing: -1,
                        ),
                      ),
                      const SizedBox(height: 10),
                      Text(
                        [
                          for (final k in [
                            'line1',
                            'line2',
                            'city',
                            'region',
                            'postal_code',
                            'country_code',
                          ])
                            if (jsonObject(
                                  row['address'] ?? {},
                                )[k]?.toString().isNotEmpty ==
                                true)
                              jsonObject(row['address'] ?? {})[k].toString(),
                        ].join(', '),
                      ),
                      const SizedBox(height: 10),
                      Text(row['timezone']?.toString() ?? ''),
                    ],
                  ),
                ),
                const SizedBox(height: 20),
                SettingsAction(
                  'Edit details',
                  icon: Icons.edit_outlined,
                  onPressed:
                      !_loading &&
                          _error == null &&
                          c.allowed('PATCH', c.detail)
                      ? _edit
                      : null,
                ),
                const SizedBox(height: 12),
                if ({'ACTIVE', 'PAUSED'}.contains(row['status'])) ...[
                  SettingsAction(
                    row['status'] == 'ACTIVE'
                        ? 'Pause branch'
                        : 'Resume branch',
                    danger: row['status'] == 'ACTIVE',
                    secondary: row['status'] == 'PAUSED',
                    icon: row['status'] == 'ACTIVE'
                        ? Icons.pause_rounded
                        : Icons.play_arrow_rounded,
                    busy: c.busy,
                    onPressed:
                        !_loading &&
                            _error == null &&
                            c.allowed(
                              'POST',
                              '$branchSettingsBase/${row['status'] == 'ACTIVE' ? 'pause' : 'resume'}',
                            )
                        ? _status
                        : null,
                  ),
                  const SizedBox(height: 24),
                ],
                SettingsLink(
                  'Operating hours',
                  Icons.schedule_rounded,
                  onTap: !_loading ? _hours : null,
                ),
                SettingsLink(
                  'Delivery policy',
                  Icons.local_shipping_outlined,
                  onTap: !_loading
                      ? () => Navigator.push(
                          context,
                          setupRoute<void>(
                            DeliveryPolicyScreen(
                              runtime: widget.runtime,
                              branch: row,
                            ),
                          ),
                        )
                      : null,
                ),
                SettingsLink(
                  'Delivery area',
                  Icons.radar_rounded,
                  onTap: !_loading
                      ? () => _configuration('service-area')
                      : null,
                ),
                SettingsLink(
                  'Order preferences',
                  Icons.tune_rounded,
                  onTap: !_loading
                      ? () => _configuration('operations-settings')
                      : null,
                ),
                SettingsLink(
                  'Business types',
                  Icons.widgets_outlined,
                  onTap: !_loading ? _types : null,
                ),
                if (row['status'] == 'DRAFT')
                  SettingsAction(
                    'Review activation',
                    icon: Icons.verified_outlined,
                    onPressed:
                        !_loading &&
                            _error == null &&
                            c.allowed(
                              'POST',
                              '/api/v1/vendor/onboarding/request-activation',
                            )
                        ? _activation
                        : null,
                  ),
              ],
              if (_error != null)
                SettingsAction(
                  'Refresh',
                  secondary: true,
                  icon: Icons.refresh_rounded,
                  onPressed: _load,
                ),
            ],
          ),
        ),
      ),
    );
  }
}

class BranchConfigSheet extends StatefulWidget {
  const BranchConfigSheet({
    super.key,
    required this.runtime,
    required this.branch,
    required this.suffix,
  });
  final VendorRuntime runtime;
  final JsonObject branch;
  final String suffix;
  @override
  State<BranchConfigSheet> createState() => _BranchConfigSheetState();
}

class _BranchConfigSheetState extends State<BranchConfigSheet> {
  late final c = SettingsController(
    widget.runtime.backend,
    SettingsKind.branches,
  );
  final _form = GlobalKey<FormState>();
  final _lat = TextEditingController(),
      _lng = TextEditingController(),
      _radius = TextEditingController();
  JsonObject _record = {};
  bool _loading = true,
      _saving = false,
      _accepting = false,
      _pickup = false,
      _loaded = false;
  String? _error;
  bool get area => widget.suffix == 'service-area';
  String get path => '$branchSettingsBase/${widget.suffix}';
  Map<String, String> get params => c.params(widget.branch);
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) unawaited(_load());
    });
  }

  @override
  void dispose() {
    c.dispose();
    _lat.dispose();
    _lng.dispose();
    _radius.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _loaded = false;
      _error = null;
    });
    await c.load();
    try {
      JsonObject record = {};
      try {
        final response = await c.read(path, parameters: params);
        record = response.data == null ? {} : response.object;
      } on ApiException catch (e) {
        if (e.statusCode != 404) rethrow;
      }
      if (!mounted) return;
      setState(() {
        _record = record;
        _loaded = true;
        _accepting = record['accepting_new_orders'] == true;
        _pickup = record['pickup_enabled'] == true;
        _lat.text = (record['center_latitude'] ?? widget.branch['latitude'])
            .toString();
        _lng.text = (record['center_longitude'] ?? widget.branch['longitude'])
            .toString();
        _radius.text =
            record['radius_meters']?.toString() ??
            (widget.runtime.mode == VendorDataMode.mock ? '5000' : '');
      });
    } catch (e) {
      if (mounted) setState(() => _error = SettingsController.message(e));
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _save() async {
    if (_saving ||
        !_loaded ||
        !c.allowed('PUT', path) ||
        !_form.currentState!.validate()) {
      return;
    }
    setState(() {
      _saving = true;
      _error = null;
    });
    final body = area
        ? {
            'mode': 'RADIUS',
            'center_latitude': double.parse(_lat.text),
            'center_longitude': double.parse(_lng.text),
            'radius_meters': int.parse(_radius.text),
            'expected_version': _record['version'] ?? 0,
          }
        : {
            'schema_version': 1,
            'accepting_new_orders': _accepting,
            'pickup_enabled': _pickup,
            'expected_version': _record['version'] ?? 0,
          };
    final result = await c.mutate('PUT', path, body, parameters: params);
    if (!mounted) return;
    setState(() {
      _saving = false;
      _error = result ? null : c.feedback ?? c.error;
    });
    if (result) Navigator.pop(context, true);
  }

  Widget _number(
    String label,
    TextEditingController controller,
    int min,
    int max, {
    bool integer = false,
  }) => Padding(
    padding: const EdgeInsets.only(bottom: 18),
    child: TextFormField(
      controller: controller,
      enabled: !_saving,
      keyboardType: TextInputType.numberWithOptions(
        decimal: !integer,
        signed: !integer,
      ),
      decoration: InputDecoration(labelText: label),
      validator: (s) {
        final n = integer ? int.tryParse(s ?? '') : double.tryParse(s ?? '');
        return n == null || !n.isFinite || n < min || n > max
            ? 'Use a value from $min to $max.'
            : null;
      },
    ),
  );
  @override
  Widget build(BuildContext context) => PopScope(
    canPop: !_saving,
    child: Form(
      key: _form,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SettingsHeading(
            area ? 'Delivery area' : 'Order preferences',
            subtitle: widget.branch['display_name'].toString(),
          ),
          if (_loading) const PremiumSkeleton(rows: 2),
          if (_error != null) SettingsNote(_error!),
          if (_loaded) ...[
            if (area) ...[
              Center(
                child: Container(
                  width: 100,
                  height: 100,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: KarryamColors.mint.forSurface(context),
                    border: Border.all(
                      color: (KarryamColors.teal.withValues(
                        alpha: .3,
                      )).forOutline(context),
                      width: 12,
                    ),
                  ),
                  child: Icon(
                    Icons.radar_rounded,
                    size: 43,
                    color: KarryamColors.tealInk.forInk(context),
                  ),
                ),
              ),
              const SizedBox(height: 22),
              _number('Centre latitude', _lat, -90, 90),
              _number('Centre longitude', _lng, -180, 180),
              _number('Radius (metres)', _radius, 1, 200000, integer: true),
            ] else ...[
              SwitchListTile.adaptive(
                contentPadding: EdgeInsets.zero,
                title: const Text('Accept new orders'),
                value: _accepting,
                activeTrackColor: KarryamColors.teal.forInk(context),
                onChanged: _saving
                    ? null
                    : (v) => setState(() => _accepting = v),
              ),
              Divider(color: Color(0xFFE1E9E4).forOutline(context)),
              SwitchListTile.adaptive(
                contentPadding: EdgeInsets.zero,
                title: const Text('Allow pickup'),
                value: _pickup,
                activeTrackColor: KarryamColors.teal.forInk(context),
                onChanged: _saving ? null : (v) => setState(() => _pickup = v),
              ),
              const SizedBox(height: 24),
            ],
            SettingsAction(
              'Save settings',
              icon: Icons.check_rounded,
              busy: _saving,
              onPressed: !_loading && _loaded && c.allowed('PUT', path)
                  ? _save
                  : null,
            ),
          ],
          if (_error != null && !_saving)
            Padding(
              padding: const EdgeInsets.only(top: 12),
              child: SettingsAction(
                'Reload settings',
                secondary: true,
                icon: Icons.refresh_rounded,
                onPressed: _load,
              ),
            ),
        ],
      ),
    ),
  );
}

class BranchTypesScreen extends StatefulWidget {
  const BranchTypesScreen({
    super.key,
    required this.runtime,
    required this.branch,
  });
  final VendorRuntime runtime;
  final JsonObject branch;
  @override
  State<BranchTypesScreen> createState() => _BranchTypesScreenState();
}

class _BranchTypesScreenState extends State<BranchTypesScreen> {
  late final c = SettingsController(
    widget.runtime.backend,
    SettingsKind.branches,
  );
  List<JsonObject> _rows = [], _available = [];
  String? _error;
  bool _loading = false;
  Map<String, String> get params => c.params(widget.branch);
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) unawaited(_load());
    });
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    await c.load();
    try {
      final rows = SettingsController.objects(
            (await c.read(
              '$branchSettingsBase/capabilities',
              parameters: params,
            )).data,
          ),
          available = await c.all(
            '${SettingsController.business}/capabilities',
            c.scope,
          );
      if (mounted) {
        setState(() {
          _rows = rows;
          _available = available;
        });
      }
    } catch (e) {
      if (mounted) setState(() => _error = SettingsController.message(e));
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _link() async {
    final options = _available
        .where((a) => !_rows.any((r) => r['vendor_capability_id'] == a['id']))
        .toList();
    final selected = await settingsSheet<JsonObject>(
      context,
      Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const SettingsHeading('Link business type'),
          for (final option in options)
            SettingsLink(
              dashboardLabel(option['capability_type']),
              Icons.widgets_outlined,
              subtitle: dashboardLabel(option['status']),
              onTap: () => Navigator.pop(context, option),
            ),
          if (options.isEmpty)
            const Text('All available business types are already linked.'),
        ],
      ),
    );
    if (selected == null || !mounted) return;
    final saved = await c.mutate(
      'PUT',
      '$branchSettingsBase/capabilities/{capabilityId}',
      {'expected_version': 0},
      parameters: {...params, 'capabilityId': selected['id'] as String},
    );
    if (saved) await _load();
    if (mounted) {
      await showOrderFeedback(
        context,
        title: saved ? 'Business type linked' : 'Type not linked',
        message: saved
            ? 'The branch link starts as Draft.'
            : c.feedback ?? c.error ?? 'Try again.',
        success: saved,
      );
    }
  }

  Future<void> _status(JsonObject row) async {
    final pause = row['status'] == 'ACTIVE',
        path =
            '$branchSettingsBase/capabilities/{capabilityId}/${pause ? 'pause' : 'resume'}';
    if (!await confirmSettings(
          context,
          pause ? 'Pause branch type' : 'Resume branch type',
          dashboardLabel(row['capability_type']),
          danger: pause,
        ) ||
        !mounted) {
      return;
    }
    final saved = await c.mutate(
      'POST',
      path,
      {'expected_version': row['version']},
      parameters: {
        ...params,
        'capabilityId': row['vendor_capability_id'] as String,
      },
    );
    if (saved) await _load();
    if (mounted) {
      await showOrderFeedback(
        context,
        title: saved ? 'Branch type updated' : 'Change not accepted',
        message: saved
            ? 'The status is updated.'
            : c.feedback ?? c.error ?? 'Try again.',
        success: saved && !pause,
        warning: saved && pause,
      );
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: const Text('Branch business types'),
      backgroundColor: KarryamColors.canvas.forSurface(context),
    ),
    bottomNavigationBar: AnimatedBuilder(
      animation: c,
      builder: (context, _) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 12, 24, 16),
          child: SettingsAction(
            'Link business type',
            icon: Icons.add_link_rounded,
            onPressed:
                !_loading &&
                    _error == null &&
                    c.allowed(
                      'PUT',
                      '$branchSettingsBase/capabilities/{capabilityId}',
                    )
                ? _link
                : null,
          ),
        ),
      ),
    ),
    body: AnimatedBuilder(
      animation: c,
      builder: (context, _) => RefreshIndicator(
        onRefresh: _load,
        child: ListView(
          padding: const EdgeInsets.all(24),
          physics: const AlwaysScrollableScrollPhysics(),
          children: [
            SettingsHeading(widget.branch['display_name'].toString()),
            if (_loading) const PremiumSkeleton(rows: 2),
            if (_error != null) SettingsNote(_error!),
            for (final row in _rows)
              Padding(
                padding: const EdgeInsets.only(bottom: 16),
                child: SettingsCard(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          Icon(
                            Icons.widgets_outlined,
                            color: KarryamColors.tealInk.forInk(context),
                          ),
                          const Spacer(),
                          SettingsStatus(row['status']),
                        ],
                      ),
                      const SizedBox(height: 16),
                      Text(
                        dashboardLabel(row['capability_type']),
                        style: const TextStyle(
                          fontSize: 22,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      const SizedBox(height: 8),
                      Text(
                        row['effective'] == true
                            ? 'Effective'
                            : 'Not effective',
                        style: TextStyle(
                          color:
                              (row['effective'] == true
                                      ? KarryamColors.tealInk
                                      : const Color(0xFF986013))
                                  .forInk(context),
                        ),
                      ),
                      if (row['effective'] != true &&
                          row['effective_reason'] != null)
                        Text(dashboardLabel(row['effective_reason'])),
                      if ({'ACTIVE', 'PAUSED'}.contains(row['status'])) ...[
                        const SizedBox(height: 18),
                        SettingsAction(
                          row['status'] == 'ACTIVE' ? 'Pause' : 'Resume',
                          danger: row['status'] == 'ACTIVE',
                          secondary: row['status'] == 'PAUSED',
                          onPressed:
                              _error == null &&
                                  !_loading &&
                                  c.allowed(
                                    'POST',
                                    '$branchSettingsBase/capabilities/{capabilityId}/${row['status'] == 'ACTIVE' ? 'pause' : 'resume'}',
                                  )
                              ? () => _status(row)
                              : null,
                        ),
                      ],
                      if (row['status'] == 'DRAFT')
                        const Padding(
                          padding: EdgeInsets.only(top: 12),
                          child: Text(
                            'Activation is not available through public settings.',
                          ),
                        ),
                    ],
                  ),
                ),
              ),
            if (!_loading && _rows.isEmpty && _error == null)
              const Text('No business types are linked to this branch.'),
            if (_error != null)
              SettingsAction(
                'Refresh',
                secondary: true,
                icon: Icons.refresh_rounded,
                onPressed: _load,
              ),
          ],
        ),
      ),
    ),
  );
}
