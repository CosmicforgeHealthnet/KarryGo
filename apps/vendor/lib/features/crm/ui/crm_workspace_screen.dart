import 'crm_insights_screen.dart';
import 'crm_exports_screen.dart';
import 'campaign_workspace_screen.dart';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../production/ui/production_form_screen.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../../settings/state/settings_controller.dart';
import '../../settings/ui/settings_design.dart';
import '../../support/ui/order_support_screen.dart' show supportDate;
import '../state/crm_workspace_controller.dart' hide crmBase;
import 'crm_design.dart';
import 'crm_editors.dart';

class CrmHub extends StatelessWidget {
  const CrmHub({super.key, required this.runtime});
  final VendorRuntime runtime;
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Customers & relationships')),
    body: SafeArea(
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 680),
          child: ListView(
            padding: const EdgeInsets.all(20),
            children: [
              const SettingsHeading('Relationships'),
              for (final area in [
                CrmArea.customers,
                CrmArea.tasks,
                CrmArea.interactions,
                CrmArea.corporates,
              ])
                SettingsLink(
                  area.title,
                  area.icon,
                  color:
                      (area == CrmArea.tasks
                              ? const Color(0xFFB47A25)
                              : area == CrmArea.corporates
                              ? const Color(0xFF7660A6)
                              : KarryamColors.tealInk)
                          .forInk(context),
                  onTap: () => Navigator.push(
                    context,
                    setupRoute<void>(
                      CrmWorkspaceScreen(runtime: runtime, area: area),
                    ),
                  ),
                ),
              const SizedBox(height: 20),
              SettingsLink(
                'Campaigns',
                Icons.campaign_outlined,
                color: const Color(0xFF7660A6).forInk(context),
                subtitle: 'Audiences, messages and delivery',
                onTap: () => Navigator.push(
                  context,
                  setupRoute<void>(CampaignWorkspaceScreen(runtime: runtime)),
                ),
              ),
              const SizedBox(height: 20),
              SettingsLink(
                'Customer insights',
                Icons.insights_outlined,
                subtitle: 'Customer value, risk and score availability',
                onTap: () => Navigator.push(
                  context,
                  setupRoute<void>(CrmInsightsScreen(runtime: runtime)),
                ),
              ),
              const SizedBox(height: 20),
              SettingsLink(
                'Exports',
                Icons.file_download_outlined,
                subtitle: 'Create and download CRM files',
                onTap: () => Navigator.push(
                  context,
                  setupRoute<void>(CrmExportsScreen(runtime: runtime)),
                ),
              ),
              const SizedBox(height: 20),
              const SettingsHeading('Organise & reward'),
              for (final area in [
                CrmArea.tags,
                CrmArea.segments,
                CrmArea.program,
                CrmArea.tiers,
              ])
                SettingsLink(
                  area.title,
                  area.icon,
                  onTap: () => Navigator.push(
                    context,
                    setupRoute<void>(
                      CrmWorkspaceScreen(runtime: runtime, area: area),
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    ),
  );
}

class CrmWorkspaceScreen extends StatefulWidget {
  const CrmWorkspaceScreen({
    super.key,
    required this.runtime,
    required this.area,
    this.id,
    this.profileId,
    this.corporateId,
    this.segmentId,
    this.activityId,
  });
  final VendorRuntime runtime;
  final CrmArea area;
  final String? id, profileId, corporateId, segmentId, activityId;
  @override
  State<CrmWorkspaceScreen> createState() => _CrmWorkspaceScreenState();
}

class _CrmWorkspaceScreenState extends State<CrmWorkspaceScreen> {
  CrmArea get area => widget.area;
  String get collection => area.corporateChild
      ? '$crmBase/corporate-customers/{corporateId}/${area.collection}'
      : area.profileChild
      ? '$crmBase/customers/{profileId}/${area.collection}'
      : area == CrmArea.members
      ? '$crmBase/segments/{segmentId}/members'
      : area == CrmArea.revisions
      ? '$crmBase/interactions/{interactionId}/revisions'
      : area == CrmArea.history
      ? '$crmBase/tasks/{taskId}/history'
      : widget.profileId != null &&
            [CrmArea.interactions, CrmArea.tasks].contains(area)
      ? '$crmBase/customers/{profileId}/${area.collection}'
      : '$crmBase/${area.collection}';
  String get detail =>
      widget.profileId != null &&
          [CrmArea.interactions, CrmArea.tasks].contains(area)
      ? '$crmBase/${area.collection}/{${area.idKey}}'
      : '$collection/{${area.idKey}}';
  String get path =>
      widget.id != null && area != CrmArea.memberships ? detail : collection;
  Map<String, String> get params => {
    if (widget.id != null) area.idKey: widget.id!,
    if (widget.profileId != null) 'profileId': widget.profileId!,
    if (widget.corporateId != null) 'corporateId': widget.corporateId!,
    if (widget.segmentId != null) 'segmentId': widget.segmentId!,
    if (widget.activityId != null)
      (area == CrmArea.history ? 'taskId' : 'interactionId'):
          widget.activityId!,
  };
  late final c = CrmWorkspaceController(
    widget.runtime,
    path,
    parameters: params,
  );
  JsonObject? get r => area == CrmArea.memberships && widget.id != null
      ? c.rows.where((r) => r['membership_ref'] == widget.id).firstOrNull
      : area == CrmArea.corporates && c.record?['corporate_customer'] is Map
      ? jsonObject(c.record!['corporate_customer'])
      : c.record;
  bool preparing = false;
  String? activeRoute;
  int offset = 0;
  String status = '';
  bool get mock => widget.runtime.mode == VendorDataMode.mock;
  bool get isDetail => widget.id != null || area.singleton;
  JsonObject? get profile => area == CrmArea.customers && r?['profile'] is Map
      ? jsonObject(r!['profile'])
      : null;
  bool get actionable =>
      profile == null ||
      profile!['privacy_state'] == 'ACTIVE' &&
          profile!['status'] == 'ACTIVE' &&
          profile!['merge_state'] != 'MERGED_SOURCE';
  @override
  void initState() {
    super.initState();
    load();
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> load({bool more = false}) async {
    if (!more) offset = 0;
    final operation = c.api(path).operation('GET', path);
    final queryNames = operation.parameters
        .where((p) => p['in'] == 'query')
        .map((p) => p['name'])
        .toSet();
    c.readQuery = {
      if (queryNames.contains('limit')) 'limit': '50',
      if (queryNames.contains('offset')) 'offset': '$offset',
      if (status.isNotEmpty && queryNames.contains('status')) 'status': status,
    };
    await c.load(append: more);
  }

  Future<void> open(
    CrmArea next, {
    String? id,
    String? profileId,
    String? corporateId,
    String? segmentId,
    String? activityId,
  }) async {
    await Navigator.push(
      context,
      setupRoute<void>(
        CrmWorkspaceScreen(
          runtime: widget.runtime,
          area: next,
          id: id,
          profileId: profileId ?? widget.profileId,
          corporateId: corporateId ?? widget.corporateId,
          segmentId: segmentId ?? widget.segmentId,
          activityId: activityId,
        ),
      ),
    );
    if (mounted) await load();
  }

  Future<List<JsonObject>> discover(
    String route, {
    Map<String, String> parameters = const {},
  }) async {
    final rows = <JsonObject>[];
    var offset = 0;
    while (true) {
      final op = c.api(route).operation('GET', route);
      final queryNames = op.parameters
          .where((p) => p['in'] == 'query')
          .map((p) => p['name'])
          .toSet();
      final response = await c.fetch(
        route,
        params: parameters,
        query: {
          if (queryNames.contains('limit')) 'limit': '100',
          if (queryNames.contains('offset')) 'offset': '$offset',
        },
      );
      final object = response is Map ? jsonObject(response) : null;
      final key = object == null
          ? null
          : [
              'items',
              'corporate_customers',
              'contacts',
              'locations',
              'cost_centers',
              'interactions',
              'tasks',
              'memberships',
            ].where((key) => object[key] is List).firstOrNull;
      final next = c.collection(key == null ? response : object![key]);
      for (final row in next) {
        c.validate(row);
      }
      rows.addAll(next);
      final more =
          object?['has_more'] == true ||
          (object?['pagination'] is Map &&
              (object!['pagination'] as Map)['has_more'] == true);
      if (!more || next.isEmpty || !queryNames.contains('offset')) break;
      offset += next.length;
    }
    return rows;
  }

  Future<Map<String, String>> choices(String key) async {
    if (['branch_ref', 'branch_refs'].contains(key)) {
      final dc = SettingsController(
        widget.runtime.backend,
        SettingsKind.branches,
      );
      try {
        final rows = await dc.all(
          '${SettingsController.business}/branches',
          dc.scope,
        );
        return {for (final b in rows) '${b['id']}': '${b['name']}'};
      } finally {
        dc.dispose();
      }
    }
    if (['assigned_staff_ref', 'account_manager_staff_ref'].contains(key)) {
      final dc = SettingsController(widget.runtime.backend, SettingsKind.staff);
      try {
        final rows = await dc.all('/api/v1/vendor/staff', {});
        return {
          for (final s in rows.where(
            (s) => s['status'] == 'ACTIVE' || s['status'] == 'active',
          ))
            '${s['id']}':
                '${s['display_name'] ?? s['full_name'] ?? s['email'] ?? s['id']}',
        };
      } finally {
        dc.dispose();
      }
    }
    if (key == 'customer_profile_ref' || key == 'profile_refs') {
      final rows = await discover('$crmBase/customers');
      return {
        for (final row in rows)
          if (row['profile'] is Map)
            ...(() {
              final p = jsonObject(row['profile']);
              return p['privacy_state'] == 'ACTIVE' &&
                      p['status'] == 'ACTIVE' &&
                      p['merge_state'] != 'MERGED_SOURCE'
                  ? {
                      '${p['profile_ref']}':
                          '${p['display_label'] ?? 'Customer relationship'}',
                    }
                  : <String, String>{};
            })(),
      };
    }
    final kind = switch (key) {
      'contact_ref' => 'contacts',
      'location_ref' => 'locations',
      'cost_center_ref' => 'cost-centers',
      'source_interaction_ref' => 'interactions',
      'tag_ref' => 'tags',
      _ => null,
    };
    if (kind == null) return {};
    final corporate = widget.corporateId != null;
    final rows = await discover(
      corporate
          ? '$crmBase/corporate-customers/{corporateId}/$kind'
          : '$crmBase/$kind',
      parameters: params,
    );
    final refKey = switch (key) {
      'source_interaction_ref' => 'interaction_ref',
      _ => key,
    };
    return {
      for (final row in rows.where(
        (r) =>
            r['status'] != 'ARCHIVED' &&
            r['privacy_suppressed'] != true &&
            r['actionable'] != false,
      ))
        '${row[refKey]}':
            '${row['display_label'] ?? row['name'] ?? row['subject'] ?? row[refKey]}',
    };
  }

  Future<void> change(
    String method,
    String route,
    JsonObject body, {
    int? headerVersion,
  }) async {
    setState(() => activeRoute = route);
    final saved = await c.save(
      method: method,
      route: route,
      body: body,
      params: params,
      headerVersion: headerVersion,
    );
    if (!mounted) return;
    await showOrderFeedback(
      context,
      title: saved == null ? 'Change not confirmed' : 'Saved',
      message: saved == null
          ? c.error ?? 'Try again.'
          : 'Your relationship records are up to date.',
      success: saved != null,
      warning: saved == null && c.pending != null,
    );
    if (saved != null && mounted) {
      if (saved.containsKey('applied') ||
          saved.containsKey('results') ||
          saved.containsKey('skipped')) {
        await settingsSheet<void>(
          context,
          Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const SettingsHeading('Customer update results'),
              ...facts(saved),
            ],
          ),
        );
      }
      await load();
    }
  }

  Future<void> edit({String? action}) async {
    if (preparing || !c.writable) return;
    setState(() => preparing = true);
    try {
      final create = widget.id == null && !area.singleton;
      final route = action != null
          ? '$detail/$action'
          : create
          ? (area == CrmArea.tasks || area == CrmArea.interactions
                ? '$crmBase/${area.collection}'
                : collection)
          : area.singleton
          ? collection
          : detail;
      final method = create || action != null ? 'POST' : 'PATCH';
      if (area == CrmArea.tiers) {
        await editTiers();
        return;
      }
      final schema = c.api(route).operation(method, route).requestSchema;
      if (schema == null) return;
      final properties = Map<String, dynamic>.from(
        schema['properties'] as Map? ?? {},
      );
      final body = <String, dynamic>{};
      final fields = <PurchaseField>[];
      final initial = create
          ? (mock ? crmDefaults(area) : <String, Object?>{})
          : r ?? {};
      if (!create && properties.containsKey('expected_version')) {
        body['expected_version'] = r?['version'];
      }
      final correct = action == 'correct';
      for (final e in properties.entries) {
        final key = e.key;
        if (key == 'expected_version') continue;
        if (action == 'reassign' && key != 'assigned_staff_ref') continue;
        if (key.startsWith('related_') || key == 'step_up_grant_id') continue;
        if (key.startsWith('clear_') && create) continue;
        if (!create &&
            [
              'scope_mode',
              'branch_refs',
              'customer_profile_ref',
              'contact_ref',
              'source_interaction_ref',
            ].contains(key) &&
            action != 'reassign') {
          continue;
        }
        if (key == 'rule') {
          if (create && area == CrmArea.segments) {
            continue;
          } else if (!create) {
            continue;
          }
        }
        if (key == 'scope') {
          continue;
        }
        if (key == 'customer_profile_ref' && widget.profileId != null) {
          body[key] = widget.profileId;
          continue;
        }
        if ([
          'branch_ref',
          'branch_refs',
          'assigned_staff_ref',
          'account_manager_staff_ref',
          'customer_profile_ref',
          'contact_ref',
          'source_interaction_ref',
          'location_ref',
          'cost_center_ref',
        ].contains(key)) {
          final options = await choices(key);
          if (!mounted) return;
          final multiple = key == 'branch_refs';
          final required =
              key == 'customer_profile_ref' ||
              key == 'contact_ref' && area == CrmArea.memberships;
          if (multiple) {
            final selected = await crmChoose(
              context,
              'Choose branches',
              options,
              multiple: true,
              initial: List<String>.from(initial[key] as List? ?? []),
              optional: true,
            );
            if (selected == null) return;
            body[key] = selected;
            continue;
          }
          if (initial[key] != null &&
              initial[key] != '' &&
              !options.containsKey(initial[key])) {
            options['${initial[key]}'] = 'Current selection';
          }
          fields.add(
            PurchaseField(
              key,
              switch (key) {
                'customer_profile_ref' => 'Customer',
                'assigned_staff_ref' => 'Assigned to',
                'account_manager_staff_ref' => 'Account manager',
                'source_interaction_ref' => 'Source interaction',
                _ => crmLabel(key.replaceAll('_ref', '')),
              },
              kind: 'select',
              required: required,
              options: options,
              initial:
                  initial[key] ??
                  (mock && required ? options.keys.firstOrNull : null),
            ),
          );
          continue;
        }
        final spec = jsonObject(e.value);
        final enums = crmEnums(key, area);
        final required =
            create &&
                [
                  'name',
                  'organization_name',
                  'organization_type',
                  'title',
                  'type',
                  'direction',
                  'outcome',
                  'visibility',
                  'occurred_at',
                  'relationship_role',
                  'location_code',
                  'location_type',
                  'code',
                  'membership_role',
                ].contains(key) ||
            ['reason', 'correction_reason'].contains(key);
        final kind = enums.isNotEmpty
            ? 'select'
            : spec['type'] == 'boolean'
            ? 'bool'
            : spec['type'] == 'integer'
            ? 'number'
            : key.endsWith('_at')
            ? 'date'
            : ['note', 'description', 'correction_reason'].contains(key)
            ? 'notes'
            : 'text';
        fields.add(
          PurchaseField(
            key,
            crmLabel(key),
            kind: kind,
            required: required,
            options: enums,
            initial:
                initial[key] ??
                (create && mock ? crmDefaults(area)[key] : null),
            min: ['redemption_increment_points', 'expiry_days'].contains(key)
                ? 1
                : 0,
          ),
        );
      }
      if (create && [CrmArea.tags, CrmArea.segments].contains(area)) {
        final branches = await choices('branch_ref');
        if (!mounted) return;
        final scope = await productionForm(
          context,
          'Relationship scope',
          [
            PurchaseField(
              'type',
              'Scope',
              kind: 'select',
              required: true,
              options: const {
                'BUSINESS': 'Whole business',
                'BRANCH': 'One branch',
              },
              initial: 'BUSINESS',
            ),
            PurchaseField(
              'branch_ref',
              'Branch',
              kind: 'select',
              options: branches,
            ),
          ],
          validate: (b) => b['type'] == 'BRANCH' && b['branch_ref'] == null
              ? 'Choose a branch.'
              : null,
        );
        if (scope == null) return;
        body['scope'] = {
          'type': scope['type'],
          if (scope['type'] == 'BRANCH') 'branch_ref': scope['branch_ref'],
        };
      }
      if (!mounted) return;
      final submitted = await productionForm(
        context,
        correct
            ? 'Correct interaction'
            : create
            ? 'New ${area.title.toLowerCase()}'
            : 'Edit ${area.title.toLowerCase()}',
        fields,
      );
      if (submitted == null || !mounted) return;
      body.addAll(submitted);
      body.removeWhere((key, value) => value == null);
      if (area == CrmArea.corporates &&
          create &&
          body['scope_mode'] == 'BRANCH_SET' &&
          (body['branch_refs'] as List? ?? []).isEmpty) {
        await showOrderFeedback(
          context,
          title: 'Choose branches',
          message:
              'A branch-scoped corporate account needs at least one branch.',
          success: false,
          warning: true,
        );
        return;
      }
      if (create &&
          area == CrmArea.segments &&
          body['segment_type'] == 'DYNAMIC') {
        final rule = await buildRule('${(body['scope'] as Map)['type']}');
        if (rule == null) return;
        body['rule'] = rule;
        if (!await previewRule(body['scope'], rule)) return;
      }
      if (area == CrmArea.program && body['expiry_policy'] == 'NEVER') {
        body.remove('expiry_days');
        body['clear_expiry_days'] = true;
      }
      if (!mounted ||
          !await confirmSettings(
            context,
            correct ? 'Save correction?' : 'Save these details?',
            correct
                ? 'The original interaction remains in its revision history.'
                : 'The server will validate your scope and references before saving.',
          )) {
        return;
      }
      await change(method, route, body);
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Details unavailable',
          message: e is FormatException
              ? e.message
              : 'Check your connection and access, then try again. Nothing was submitted.',
          success: false,
        );
      }
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  Future<JsonObject?> buildRule(String scope) async {
    final registry = jsonObject(await c.fetch('$crmBase/segment-fields'));
    final tags = await choices('tag_ref');
    if (!mounted) return null;
    return Navigator.push<JsonObject>(
      context,
      setupRoute<JsonObject>(
        CrmRuleScreen(registry: registry, tags: tags, scope: scope),
      ),
    );
  }

  Future<bool> previewRule(Object? scope, JsonObject rule) async {
    final result = await c.save(
      method: 'POST',
      route: '$crmBase/segments/preview',
      body: {'scope': scope, 'rule': rule, 'sample_limit': 10},
    );
    if (!mounted) return false;
    if (result == null) {
      await showOrderFeedback(
        context,
        title: 'Preview unavailable',
        message: c.error ?? 'Review the rule and try again.',
        success: false,
        warning: true,
      );
      return false;
    }
    return await settingsSheet<bool>(
          context,
          Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const SettingsHeading('Audience preview'),
              ...facts(result),
              const SizedBox(height: 16),
              SettingsAction(
                'Use this audience',
                onPressed: () => Navigator.pop(context, true),
              ),
            ],
          ),
        ) ==
        true;
  }

  Future<void> editBranchScope() async {
    if (preparing || !c.writable) return;
    setState(() => preparing = true);
    try {
      final actual = jsonObject(
        await c.fetch('$detail/branches', params: params),
      );
      if (!mounted) return;
      final scope = await productionForm(context, 'Corporate branch scope', [
        PurchaseField(
          'scope_mode',
          'Scope',
          kind: 'select',
          required: true,
          options: const {
            'BUSINESS': 'Whole business',
            'BRANCH_SET': 'Selected branches',
          },
          initial: actual['scope_mode'],
        ),
      ]);
      if (scope == null || !mounted) return;
      final options = await choices('branch_refs');
      if (!mounted) return;
      final refs = scope['scope_mode'] == 'BUSINESS'
          ? <String>[]
          : await crmChoose(
              context,
              'Choose branches',
              options,
              multiple: true,
              initial: List<String>.from(actual['branch_refs'] as List? ?? []),
            );
      if (refs == null || !mounted) return;
      if (await confirmSettings(
        context,
        'Change account scope?',
        'Only the selected scope will be eligible for this account.',
      )) {
        await change('PUT', '$detail/branches', {
          'scope_mode': scope['scope_mode'],
          'branch_refs': refs,
          'expected_version': actual['version'],
        });
      }
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  Future<void> editTiers() async {
    final tiers = c.collection(r?['tiers']);
    final result = await Navigator.push<List<JsonObject>>(
      context,
      setupRoute<List<JsonObject>>(CrmTiersScreen(initial: tiers)),
    );
    if (result == null || !mounted) return;
    for (final tier in result) {
      tier['tier_key'] = '${tier['tier_key']}'.trim().toUpperCase();
    }
    final thresholds = result.map((t) => t['minimum_lifetime_points']).toSet(),
        keys = result.map((t) => t['tier_key']).toSet();
    if (result.length > 8 ||
        thresholds.length != result.length ||
        keys.length != result.length) {
      await showOrderFeedback(
        context,
        title: 'Review the ladder',
        message:
            'Use up to eight tiers, each with its own key and points threshold.',
        success: false,
        warning: true,
      );
      return;
    }
    if (await confirmSettings(
      context,
      'Replace loyalty tiers?',
      'Customer standing will be derived from the saved ladder and loyalty ledger.',
    )) {
      await change('PUT', collection, {
        'tiers': result,
        if ((r?['version'] as int? ?? 0) > 0) 'expected_version': r!['version'],
      });
    }
  }

  Future<void> transition(String action) async {
    final version = (profile ?? r)?['version'];
    if (!await confirmSettings(
          context,
          '${crmLabel(action)} this ${area == CrmArea.customers
              ? 'relationship'
              : area == CrmArea.tasks || area == CrmArea.corporateTasks
              ? 'follow-up'
              : 'record'}?',
          'This changes its working status.',
          danger: ['archive', 'cancel', 'deactivate'].contains(action),
        ) ||
        !mounted) {
      return;
    }
    await change(
      'POST',
      '$detail/$action',
      area == CrmArea.customers ? {} : {'expected_version': version},
      headerVersion: area == CrmArea.customers ? version : null,
    );
  }

  Future<void> bulk(String action) async {
    if (preparing || !c.writable) return;
    setState(() => preparing = true);
    try {
      final options = await choices('profile_refs');
      if (!mounted) return;
      final selected = await crmChoose(
        context,
        'Choose customers',
        options,
        multiple: true,
      );
      if (selected == null || !mounted) return;
      final route = area == CrmArea.tags
          ? '$crmBase/tags/bulk-$action'
          : area == CrmArea.segments
          ? '$detail/${r!['segment_type'] == 'DYNAMIC' ? 'exclusions' : 'members'}/bulk-$action'
          : '$crmBase/segments/{segmentId}/members/bulk-$action';
      if (await confirmSettings(
        context,
        '${crmLabel(action)} selected customers?',
        'Apply this change to ${selected.length} customer relationships.',
      )) {
        await change('POST', route, {
          'profile_refs': selected,
          if (area == CrmArea.tags) 'tag_ref': widget.id,
        });
      }
    } catch (_) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Customer list unavailable',
          message: 'Refresh and try again.',
          success: false,
        );
      }
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  Future<void> adjust() async {
    final result = await productionForm(context, 'Adjust loyalty points', [
      PurchaseField(
        'direction',
        'Adjustment',
        kind: 'select',
        required: true,
        options: const {'CREDIT': 'Add points', 'DEBIT': 'Remove points'},
      ),
      const PurchaseField(
        'points',
        'Points',
        kind: 'number',
        required: true,
        min: 1,
      ),
      const PurchaseField('reason', 'Reason', kind: 'notes', required: true),
    ]);
    if (result == null || !mounted) return;
    if (await confirmSettings(
      context,
      'Confirm points adjustment?',
      'The server validates available points, adjustment limits and programme status.',
    )) {
      await change('POST', '$collection/adjustments', result);
    }
  }

  Future<void> reverseEntry(JsonObject entry) async {
    final b = await productionForm(context, 'Reverse points entry', [
      const PurchaseField('reason', 'Reason', required: true, kind: 'notes'),
    ]);
    if (b == null || !mounted) return;
    if (await confirmSettings(
      context,
      'Reverse this entry?',
      'This creates a separate reversal entry and preserves the ledger.',
      danger: true,
    )) {
      final route = '$crmBase/loyalty/entries/{entryId}/reverse';
      final result = await c.save(
        method: 'POST',
        route: route,
        body: b,
        params: {'entryId': entry['entry_ref']},
      );
      if (mounted) {
        await showOrderFeedback(
          context,
          title: result == null ? 'Reversal not confirmed' : 'Entry reversed',
          message: result == null
              ? c.error ?? 'Try again.'
              : 'The loyalty ledger is updated.',
          success: result != null,
        );
        if (result != null) await load();
      }
    }
  }

  List<Widget> facts(JsonObject object, {int depth = 0}) => [
    for (final e in object.entries.where(
      (e) =>
          ![
            'business_id',
            'profile',
            'version',
            'modules',
            'definition_hash',
            'rule',
            'opaque_ref',
          ].contains(e.key) &&
          !e.key.endsWith('_ref') &&
          !e.key.endsWith('_refs'),
    ))
      if (e.value is Map && depth < 2) ...[
        const SizedBox(height: 14),
        SettingsHeading(crmLabel(e.key)),
        ...facts(jsonObject(e.value), depth: depth + 1),
      ] else if (e.value is List && depth < 2) ...[
        const SizedBox(height: 14),
        SettingsHeading(crmLabel(e.key)),
        for (final v in e.value as List)
          if (v is Map)
            ...facts(jsonObject(v), depth: depth + 1)
          else
            SettingsFact('', '${v ?? 'Unavailable'}'),
      ] else if (e.value is! List && e.value is! Map)
        SettingsFact(
          crmLabel(e.key),
          e.value == null
              ? 'Unavailable'
              : e.key.endsWith('_at')
              ? supportDate(e.value)
              : e.value is bool
              ? e.value == true
                    ? 'Yes'
                    : 'No'
              : e.key.endsWith('_minor') && object['currency'] != null
              ? dashboardMoney(e.value, '${object['currency']}')
              : '${e.value}',
        ),
  ];
  Widget card(JsonObject row) => SettingsCard(
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (row['status'] != null) SettingsStatus(row['status']),
        ...facts(row),
      ],
    ),
  );
  Widget item(JsonObject row) {
    final customer = area == CrmArea.customers
        ? jsonObject(row['profile'])
        : row;
    final label = crmRecordTitle(area, customer);
    final blocked =
        customer['privacy_state'] != null &&
            customer['privacy_state'] != 'ACTIVE' ||
        customer['privacy_suppressed'] == true;
    return SettingsCard(
      child: InkWell(
        borderRadius: BorderRadius.circular(20),
        onTap:
            area == CrmArea.entries ||
                [
                  CrmArea.members,
                  CrmArea.history,
                  CrmArea.revisions,
                ].contains(area)
            ? null
            : () => open(area, id: '${customer[area.refKey]}'),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Container(
                  padding: const EdgeInsets.all(12),
                  decoration: BoxDecoration(
                    color:
                        (blocked
                                ? KarryamColors.error.withValues(alpha: .08)
                                : KarryamColors.teal.withValues(alpha: .08))
                            .forSurface(context),
                    borderRadius: BorderRadius.circular(16),
                  ),
                  child: Icon(
                    blocked ? Icons.shield_outlined : area.icon,
                    color:
                        (blocked ? KarryamColors.error : KarryamColors.tealInk)
                            .forInk(context),
                  ),
                ),
                const SizedBox(width: 14),
                Expanded(
                  child: Text(
                    label,
                    style: Theme.of(context).textTheme.titleMedium,
                  ),
                ),
                if (area != CrmArea.entries)
                  const Icon(Icons.chevron_right_rounded),
              ],
            ),
            const SizedBox(height: 12),
            if (customer['status'] != null) SettingsStatus(customer['status']),
            if (blocked)
              SettingsNote(
                crmLabel(customer['privacy_state'] ?? 'Privacy suppressed'),
                warning: true,
              ),
            for (final key in [
              'priority',
              'due_at',
              'outcome',
              'relationship_role',
              'membership_role',
              'evaluation_status',
            ])
              if (customer[key] != null)
                SettingsFact(
                  crmLabel(key),
                  key.endsWith('_at')
                      ? supportDate(customer[key])
                      : crmLabel(customer[key]),
                ),
            if (area == CrmArea.entries) ...[
              ...facts(row),
              if ([
                    'ADJUSTMENT_CREDIT',
                    'ADJUSTMENT_DEBIT',
                  ].contains(row['entry_type']) &&
                  row['reversed_by_entry_ref'] == null)
                SettingsAction(
                  'Reverse entry',
                  danger: true,
                  onPressed: c.writable ? () => reverseEntry(row) : null,
                ),
            ],
            if ([
              CrmArea.members,
              CrmArea.history,
              CrmArea.revisions,
            ].contains(area))
              ...facts(row),
          ],
        ),
      ),
    );
  }

  List<Widget> detailWidgets() {
    final row = r;
    if (row == null) {
      return [
        const SettingsNote(
          'This record is unavailable. Refresh to load it.',
          warning: true,
        ),
      ];
    }
    if (area == CrmArea.customers) {
      final p = profile!;
      return [
        SettingsCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                crmRecordTitle(area, p),
                style: Theme.of(context).textTheme.headlineSmall,
              ),
              const SizedBox(height: 14),
              SettingsStatus(p['status']),
              ...facts(p),
            ],
          ),
        ),
        if (row['commercial'] is Map) card(jsonObject(row['commercial'])),
        for (final section in [
          'tags',
          'segments',
          'modules',
          'loyalty_tier',
          'customer_value',
          'relationship_signals',
        ])
          if (row[section] != null) card({section: row[section]}),
        for (final next in [
          CrmArea.interactions,
          CrmArea.tasks,
          CrmArea.loyalty,
          CrmArea.consent,
          CrmArea.privacy,
          CrmArea.intelligence,
        ])
          SettingsLink(
            next.title,
            next.icon,
            onTap: () => open(next, profileId: widget.id),
          ),
        SettingsAction(
          p['status'] == 'ARCHIVED'
              ? 'Restore relationship'
              : 'Archive relationship',
          danger: p['status'] != 'ARCHIVED',
          onPressed:
              c.writable &&
                  p['privacy_state'] == 'ACTIVE' &&
                  p['merge_state'] != 'MERGED_SOURCE'
              ? () => transition(
                  p['status'] == 'ARCHIVED' ? 'restore' : 'archive',
                )
              : null,
        ),
      ];
    }
    final widgets = <Widget>[card(row)];
    if (area == CrmArea.corporates) {
      if (c.record?['summary'] is Map) {
        widgets.add(card(jsonObject(c.record!['summary'])));
      }
      widgets.add(
        SettingsAction(
          'Branch scope',
          secondary: true,
          onPressed: c.writable ? editBranchScope : null,
        ),
      );
      for (final next in [
        CrmArea.contacts,
        CrmArea.locations,
        CrmArea.costCenters,
        CrmArea.memberships,
        CrmArea.corporateInteractions,
        CrmArea.corporateTasks,
      ]) {
        widgets.add(
          SettingsLink(
            next.title,
            next.icon,
            onTap: () => open(next, corporateId: widget.id),
          ),
        );
      }
      final actions = switch (row['status']) {
        'PROSPECT' => ['activate', 'archive'],
        'ACTIVE' => ['hold', 'deactivate', 'archive'],
        'ON_HOLD' => ['activate', 'deactivate', 'archive'],
        'INACTIVE' => ['activate', 'archive'],
        _ => <String>[],
      };
      for (final action in actions) {
        widgets.add(
          SettingsAction(
            crmLabel(action),
            secondary: action == 'activate',
            danger: action != 'activate',
            onPressed: c.writable ? () => transition(action) : null,
          ),
        );
      }
    }
    if (area.creatable &&
            row['status'] != 'ARCHIVED' &&
            !([CrmArea.tasks, CrmArea.corporateTasks].contains(area) &&
                ['COMPLETED', 'CANCELLED'].contains(row['status'])) &&
            row['privacy_suppressed'] != true &&
            row['actionable'] != false ||
        area == CrmArea.program ||
        area == CrmArea.tiers) {
      widgets.add(
        SettingsAction(
          'Edit details',
          icon: Icons.edit_outlined,
          busy: preparing,
          onPressed: c.writable ? () => edit() : null,
        ),
      );
    }
    if ([CrmArea.tasks, CrmArea.corporateTasks].contains(area)) {
      if (['OPEN', 'IN_PROGRESS'].contains(row['status'])) {
        for (final action in [
          if (row['status'] == 'OPEN') 'start',
          'complete',
          'cancel',
        ]) {
          widgets.add(
            SettingsAction(
              crmLabel(action),
              danger: action == 'cancel',
              busy: c.busy && activeRoute == '$detail/$action',
              onPressed: c.writable ? () => transition(action) : null,
            ),
          );
        }
        widgets.add(
          SettingsAction(
            'Reassign',
            secondary: true,
            onPressed: c.writable ? () => edit(action: 'reassign') : null,
          ),
        );
      }
      if (area == CrmArea.tasks) {
        widgets.add(
          SettingsLink(
            'Task history',
            Icons.history_rounded,
            onTap: () => open(CrmArea.history, activityId: widget.id),
          ),
        );
      }
    }
    if ([CrmArea.interactions, CrmArea.corporateInteractions].contains(area)) {
      widgets.removeWhere((w) => w is SettingsAction);
      widgets.add(
        SettingsAction(
          'Correct interaction',
          icon: Icons.edit_note_rounded,
          onPressed: c.writable ? () => edit(action: 'correct') : null,
        ),
      );
      if (area == CrmArea.interactions) {
        widgets.add(
          SettingsLink(
            'Revision history',
            Icons.history_rounded,
            onTap: () => open(CrmArea.revisions, activityId: widget.id),
          ),
        );
      }
    }
    if ([CrmArea.tags, CrmArea.segments].contains(area) &&
        row['status'] != 'ARCHIVED') {
      for (final action in ['assign', 'remove']) {
        widgets.add(
          SettingsAction(
            area == CrmArea.tags
                ? (action == 'assign' ? 'Assign customers' : 'Remove customers')
                : row['segment_type'] == 'DYNAMIC'
                ? (action == 'assign'
                      ? 'Exclude customers'
                      : 'Remove exclusions')
                : (action == 'assign' ? 'Add members' : 'Remove members'),
            secondary: true,
            onPressed: c.writable
                ? () => bulk(
                    action == 'assign' && area != CrmArea.tags ? 'add' : action,
                  )
                : null,
          ),
        );
      }
      widgets.add(
        SettingsAction(
          'Archive',
          danger: true,
          onPressed: c.writable ? () => transition('archive') : null,
        ),
      );
    }
    if (area == CrmArea.segments) {
      widgets.add(
        SettingsLink(
          'Members',
          Icons.people_outline_rounded,
          onTap: () => open(CrmArea.members, segmentId: widget.id),
        ),
      );
      if (row['status'] == 'DRAFT') {
        widgets.add(
          SettingsAction(
            'Activate segment',
            onPressed: c.writable ? () => transition('activate') : null,
          ),
        );
      }
      if (row['segment_type'] == 'DYNAMIC' && row['status'] != 'ARCHIVED') {
        widgets.add(
          SettingsAction(
            'Replace rules',
            secondary: true,
            onPressed: c.writable
                ? () async {
                    final rule = await buildRule('${row['scope_type']}');
                    if (rule == null || !mounted) return;
                    if (await previewRule({
                          'type': row['scope_type'],
                          if (row['branch_ref'] != null)
                            'branch_ref': row['branch_ref'],
                        }, rule) &&
                        mounted &&
                        await confirmSettings(
                          context,
                          'Replace segment rules?',
                          'The saved membership will need reevaluation.',
                        )) {
                      await change('PATCH', detail, {
                        'rule': rule,
                        'expected_version': row['version'],
                        'change_reason': 'Vendor updated the segment rule.',
                      });
                    }
                  }
                : null,
          ),
        );
        widgets.add(
          SettingsAction(
            'Refresh membership',
            secondary: true,
            onPressed:
                c.writable &&
                    !['QUEUED', 'RUNNING'].contains(row['evaluation_status'])
                ? () => change('POST', '$detail/materialize', {})
                : null,
          ),
        );
      }
    }
    if (area.corporateChild &&
        ![
          CrmArea.corporateInteractions,
          CrmArea.corporateTasks,
        ].contains(area) &&
        row['status'] != 'ARCHIVED') {
      widgets.add(
        SettingsAction(
          'Archive',
          danger: true,
          onPressed: c.writable && row['privacy_suppressed'] != true
              ? () => transition('archive')
              : null,
        ),
      );
    }
    if (area == CrmArea.loyalty) {
      widgets.add(
        SettingsLink(
          'Points history',
          Icons.history_rounded,
          onTap: () => open(CrmArea.entries),
        ),
      );
      widgets.add(
        SettingsAction(
          'Adjust points',
          icon: Icons.swap_vert_rounded,
          onPressed:
              c.writable &&
                  row['program_status'] == 'ACTIVE' &&
                  !['MERGED', 'REDACTED'].contains(row['account_status'])
              ? adjust
              : null,
        ),
      );
    }
    return widgets;
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      appBar: AppBar(title: Text(area.title)),
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 680),
            child: RefreshIndicator(
              onRefresh: load,
              child: ListView(
                padding: const EdgeInsets.all(20),
                children: [
                  if (c.pending != null) ...[
                    const SettingsNote(
                      'A saved change is awaiting confirmation. Retry it before making another change.',
                      warning: true,
                    ),
                    SettingsAction(
                      'Retry saved change',
                      busy: c.busy,
                      onPressed: () async {
                        final saved = await c.save();
                        if (saved != null) await load();
                      },
                    ),
                  ],
                  if (c.error != null) ...[
                    SettingsNote(c.error!, warning: true),
                    SettingsAction(
                      'Refresh',
                      secondary: true,
                      onPressed: c.loading || c.busy ? null : load,
                    ),
                  ],
                  if (c.loading)
                    const PremiumSkeleton(detail: true)
                  else if (!c.stale) ...[
                    if (!isDetail && area.creatable)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 20),
                        child: SettingsAction(
                          'New ${area.title.toLowerCase()}',
                          icon: Icons.add_rounded,
                          busy: preparing,
                          onPressed: c.writable ? () => edit() : null,
                        ),
                      ),
                    if (!isDetail &&
                        [
                          CrmArea.customers,
                          CrmArea.tasks,
                          CrmArea.corporates,
                          CrmArea.tags,
                          CrmArea.segments,
                        ].contains(area))
                      Padding(
                        padding: const EdgeInsets.only(bottom: 20),
                        child: Wrap(
                          spacing: 8,
                          runSpacing: 8,
                          children: [
                            for (final state in [
                              '',
                              if (area != CrmArea.tasks) 'ACTIVE',
                              if (area == CrmArea.tasks) ...[
                                'OPEN',
                                'IN_PROGRESS',
                                'COMPLETED',
                                'CANCELLED',
                              ] else
                                'ARCHIVED',
                            ])
                              ChoiceChip(
                                label: Text(
                                  state.isEmpty ? 'All' : crmLabel(state),
                                ),
                                selected: status == state,
                                onSelected: (_) {
                                  status = state;
                                  load();
                                },
                              ),
                          ],
                        ),
                      ),
                    if (isDetail)
                      for (final w in detailWidgets())
                        Padding(
                          padding: const EdgeInsets.only(bottom: 16),
                          child: w,
                        )
                    else ...[
                      if (c.rows.isEmpty)
                        SettingsCard(
                          child: Padding(
                            padding: const EdgeInsets.symmetric(vertical: 26),
                            child: Column(
                              children: [
                                Icon(
                                  area.icon,
                                  size: 52,
                                  color: KarryamColors.tealInk.forInk(context),
                                ),
                                const SizedBox(height: 18),
                                Text(
                                  'No ${area.title.toLowerCase()} yet',
                                  style: Theme.of(context).textTheme.titleLarge,
                                  textAlign: TextAlign.center,
                                ),
                                const SizedBox(height: 8),
                                Text(
                                  area == CrmArea.customers
                                      ? 'Customer relationships appear from supported commerce activity.'
                                      : 'Your saved records will appear here.',
                                  textAlign: TextAlign.center,
                                ),
                              ],
                            ),
                          ),
                        ),
                      for (final row in c.rows)
                        Padding(
                          padding: const EdgeInsets.only(bottom: 16),
                          child: item(row),
                        ),
                      if (c.hasMore)
                        SettingsAction(
                          'Load more',
                          secondary: true,
                          onPressed: () {
                            offset += 50;
                            load(more: true);
                          },
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
