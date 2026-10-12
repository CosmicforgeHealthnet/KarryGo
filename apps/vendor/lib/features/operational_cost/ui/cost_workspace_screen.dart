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
import '../state/cost_workspace_controller.dart';

const policiesPath = '/api/v1/vendor/cost/policies';
const batchCostPath = '/api/v1/vendor/production-cost/batches/{batchId}';
const costStrategies = {
  'LATEST_ACCEPTED_PURCHASE_COST': 'Latest accepted purchase',
  'WEIGHTED_AVERAGE_PURCHASE_COST': 'Weighted average purchase',
  'MANUAL_CONFIGURED_COST': 'Manual unit cost',
};

class CostWorkspaceScreen extends StatefulWidget {
  const CostWorkspaceScreen({
    super.key,
    required this.runtime,
    this.view = 'hub',
    this.id,
    this.snapshot,
  });
  final VendorRuntime runtime;
  final String view;
  final String? id;
  final JsonObject? snapshot;
  @override
  State<CostWorkspaceScreen> createState() => _CostWorkspaceScreenState();
}

class _CostWorkspaceScreenState extends State<CostWorkspaceScreen> {
  String get path => switch (widget.view) {
    'policies' => widget.id == null ? policiesPath : '$policiesPath/{policyId}',
    'batch' => batchCostPath,
    'variance' => '$batchCostPath/variance',
    'components' => '/api/v1/vendor/cost/snapshots/{snapshotId}/components',
    'insights' => '/api/v1/vendor/cost/highest-cost-subjects',
    _ => '/api/v1/vendor/production-cost/history',
  };
  String get title => switch (widget.view) {
    'policies' => widget.id == null ? 'Cost policies' : 'Material cost policy',
    'batch' => 'Batch costs',
    'variance' => 'Cost variance',
    'components' => 'Cost breakdown',
    'insights' => 'Highest-cost materials',
    'history' => 'Cost history',
    _ => 'Production costs',
  };
  Map<String, String> get params => {
    if (widget.id != null)
      switch (widget.view) {
        'policies' => 'policyId',
        'components' => 'snapshotId',
        _ => 'batchId',
      }: widget.id!,
  };
  late final c = CostWorkspaceController(
    widget.runtime,
    path,
    parameters: params,
    wrapper: switch (widget.view) {
      'policies' => widget.id == null ? 'policies' : null,
      'components' => 'components',
      'insights' => 'subjects',
      'history' => 'snapshots',
      _ => null,
    },
    allowMissing: widget.view == 'batch',
  );
  final names = <String, String>{};
  bool preparing = false;
  int offset = 0;
  String? activeRoute;
  @override
  void initState() {
    super.initState();
    if (widget.view != 'hub') load();
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> load({bool more = false}) async {
    if (widget.view == 'hub') return;
    if (!more) offset = 0;
    c.readQuery = {
      if (['history', 'policies'].contains(widget.view) &&
          widget.id == null) ...{
        'limit': '50',
        'offset': '$offset',
      },
    };
    await c.load(append: more);
    if (!mounted) return;
    try {
      for (final kind in ['ingredients', 'packaging-materials']) {
        final result = await widget.runtime.backend.productionMasterApi.request(
          'GET',
          '/api/v1/vendor/$kind',
          query: {'limit': '200'},
        );
        for (final r in c.collection(result.data)) {
          c.validate(r);
          names['${r['id']}'] = '${r['name']}';
        }
      }
    } catch (_) {}
    if (mounted) setState(() {});
  }

  Future<void> open(String view, {String? id, JsonObject? snapshot}) async {
    await Navigator.push(
      context,
      setupRoute<void>(
        CostWorkspaceScreen(
          runtime: widget.runtime,
          view: view,
          id: id,
          snapshot: snapshot,
        ),
      ),
    );
    if (mounted && widget.view != 'hub') await load();
  }

  Future<void> change(
    String method,
    String route,
    JsonObject body, {
    int? version,
  }) async {
    setState(() => activeRoute = route);
    final result = await c.save(
      method: method,
      route: route,
      body: body,
      params: params,
      headerVersion: version,
    );
    if (!mounted) return;
    await showOrderFeedback(
      context,
      title: result == null ? 'Change not confirmed' : 'Saved',
      message: result == null
          ? c.error ?? 'Try again.'
          : 'Your cost records are up to date.',
      success: result != null,
      warning: result == null && c.pending != null,
    );
    if (result != null) await load();
  }

  Future<void> editPolicy() async {
    if (preparing || !c.writable) return;
    setState(() => preparing = true);
    try {
      final r = c.record;
      final edit = r != null;
      JsonObject scope = {
        'branch_id': r?['branch_id'] ?? '',
        'subject_type': r?['subject_type'] ?? 'INGREDIENT',
      };
      JsonObject? material;
      if (!edit) {
        final discovery = SettingsController(
          widget.runtime.backend,
          SettingsKind.branches,
        );
        final branches = await discovery.all(
          '${SettingsController.business}/branches',
          discovery.scope,
        );
        discovery.dispose();
        if (!mounted) return;
        final selected = await productionForm(context, 'Policy scope', [
          PurchaseField(
            'branch_id',
            'Applies to',
            kind: 'select',
            required: true,
            options: {
              'BUSINESS': 'Whole business',
              for (final b in branches) '${b['id']}': '${b['name']}',
            },
            initial: widget.runtime.mode == VendorDataMode.mock
                ? 'BUSINESS'
                : null,
          ),
          PurchaseField(
            'subject_type',
            'Material type',
            kind: 'select',
            required: true,
            options: const {
              'INGREDIENT': 'Ingredient',
              'PACKAGING': 'Packaging',
            },
            initial: 'INGREDIENT',
          ),
        ]);
        if (selected == null) return;
        scope = selected;
        if (scope['branch_id'] == 'BUSINESS') scope['branch_id'] = '';
        final kind = scope['subject_type'] == 'INGREDIENT'
            ? 'ingredients'
            : 'packaging-materials';
        final response = await widget.runtime.backend.productionMasterApi
            .request(
              'GET',
              '/api/v1/vendor/$kind',
              query: {'limit': '200', 'status': 'ACTIVE'},
            );
        final materials = c.collection(response.data);
        for (final row in materials) {
          c.validate(row);
        }
        if (!mounted) return;
        if (materials.isEmpty) {
          await showOrderFeedback(
            context,
            title: 'No materials yet',
            message:
                'Add a production material before creating its cost policy.',
            success: false,
            warning: true,
          );
          return;
        }
        final picked = await productionForm(context, 'Choose material', [
          PurchaseField(
            'subject_ref',
            'Material',
            kind: 'select',
            required: true,
            options: {for (final m in materials) '${m['id']}': '${m['name']}'},
            initial: widget.runtime.mode == VendorDataMode.mock
                ? materials.first['id']
                : null,
          ),
        ]);
        if (picked == null) return;
        scope['subject_ref'] = picked['subject_ref'];
        material = materials.firstWhere(
          (m) => m['id'] == picked['subject_ref'],
        );
      } else {
        scope['subject_ref'] = r['subject_ref'];
      }
      if (!mounted) return;
      final strategy = await productionForm(context, 'Cost method', [
        PurchaseField(
          'strategy',
          'Valuation method',
          kind: 'select',
          required: true,
          options: costStrategies,
          initial: r?['strategy'] ?? 'LATEST_ACCEPTED_PURCHASE_COST',
        ),
      ]);
      if (strategy == null || !mounted) return;
      final manual = strategy['strategy'] == 'MANUAL_CONFIGURED_COST';
      final weighted = strategy['strategy'] == 'WEIGHTED_AVERAGE_PURCHASE_COST';
      final body = await Navigator.push<JsonObject>(
        context,
        setupRoute<JsonObject>(
          PurchaseForm(
            title: edit ? 'Edit cost policy' : 'New cost policy',
            fields: [
              PurchaseField(
                'currency',
                'Currency code',
                required: true,
                initial:
                    r?['currency'] ??
                    (widget.runtime.mode == VendorDataMode.mock ? 'NGN' : null),
              ),
              if (weighted)
                PurchaseField(
                  'weighted_average_receipt_limit',
                  'Accepted receipt limit',
                  kind: 'number',
                  required: true,
                  max: 200,
                  initial: r?['weighted_average_receipt_limit'] ?? 10,
                ),
              if (manual) ...[
                PurchaseField(
                  'manual_unit_cost_minor',
                  'Cost per unit',
                  kind: 'money',
                  required: true,
                  initial:
                      r?['manual_unit_cost_minor'] ??
                      (widget.runtime.mode == VendorDataMode.mock ? 180 : null),
                ),
                PurchaseField(
                  'manual_unit_code',
                  'Unit code',
                  required: true,
                  initial:
                      r?['manual_unit_code'] ?? material?['default_unit_code'],
                ),
                PurchaseField(
                  'manual_cost_reason',
                  'Manual cost note',
                  kind: 'notes',
                  initial: r?['manual_cost_reason'] ?? '',
                ),
              ],
              const PurchaseField('reason', 'Change note', kind: 'notes'),
            ],
            validate: (b) => RegExp(r'^[A-Z]{3}$').hasMatch('${b['currency']}')
                ? null
                : 'Use a three-letter uppercase currency code.',
          ),
        ),
      );
      if (body == null || !mounted) return;
      body.addAll({
        ...scope,
        ...strategy,
        'weighted_average_receipt_limit':
            body['weighted_average_receipt_limit'] ?? 10,
        'manual_unit_cost_minor': body['manual_unit_cost_minor'],
        'manual_unit_code': body['manual_unit_code'] ?? '',
        'manual_cost_reason': body['manual_cost_reason'] ?? '',
      });
      if (!await confirmSettings(
        context,
        'Save cost policy?',
        'This method will apply to future calculations. Saved snapshots stay unchanged.',
      )) {
        return;
      }
      await change(
        edit ? 'PUT' : 'POST',
        edit ? '$policiesPath/{policyId}' : policiesPath,
        body,
        version: edit ? r['version'] as int : null,
      );
    } catch (_) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Details unavailable',
          message: 'Refresh and try again. No cost change was submitted.',
          success: false,
        );
      }
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  Future<void> pickBatch() async {
    if (preparing) return;
    setState(() => preparing = true);
    try {
      final result = await widget.runtime.backend.productionBatchesApi.request(
        'GET',
        '/api/v1/vendor/production-batches',
        query: {'limit': '100'},
      );
      final rows = c.collection(result.data);
      for (final row in rows) {
        c.validate(row);
      }
      if (!mounted) return;
      if (rows.isEmpty) {
        await showOrderFeedback(
          context,
          title: 'No production batches',
          message:
              'Create a batch from an active recipe to start tracking its costs.',
          success: false,
          warning: true,
        );
        return;
      }
      final selected = await productionForm(
        context,
        'Choose production batch',
        [
          PurchaseField(
            'batchId',
            'Batch',
            kind: 'select',
            required: true,
            options: {
              for (final b in rows)
                '${b['id']}': '${b['id']} · ${dashboardLabel(b['status'])}',
            },
            initial: widget.runtime.mode == VendorDataMode.mock
                ? rows.first['id']
                : null,
          ),
        ],
      );
      if (selected != null && mounted) {
        await open('batch', id: selected['batchId']);
      }
    } catch (_) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Batches unavailable',
          message: 'Check your connection and branch access, then try again.',
          success: false,
        );
      }
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  Widget metric(String label, Object? value, Object? currency) => SettingsFact(
    label,
    value == null ? 'Unavailable' : dashboardMoney(value, '${currency ?? ''}'),
  );
  Widget snapshotCard(JsonObject r) => SettingsCard(
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          dashboardLabel(r['calculation_type']),
          style: Theme.of(context).textTheme.titleLarge,
        ),
        const SizedBox(height: 12),
        completeness(r['completeness']),
        const SizedBox(height: 16),
        metric('Total', r['total_cost_minor'], r['currency']),
        for (final e in {
          'Ingredients': 'ingredient_cost_minor',
          'Packaging': 'packaging_cost_minor',
          'Waste': 'waste_cost_minor',
          'Per unit': 'cost_per_unit_minor',
        }.entries)
          metric(e.key, r[e.value], r['currency']),
        SettingsFact('Calculated', supportDate(r['calculated_at'])),
        if (r['id'] != null && r['id'] != '')
          SettingsLink(
            'Component breakdown',
            Icons.receipt_long_outlined,
            onTap: () => open('components', id: '${r['id']}', snapshot: r),
          ),
      ],
    ),
  );
  Widget completeness(Object? value) => Row(
    children: [
      Icon(
        value == 'COMPLETE'
            ? Icons.verified_outlined
            : Icons.info_outline_rounded,
        size: 19,
        color:
            (value == 'COMPLETE'
                    ? KarryamColors.tealInk
                    : const Color(0xFF986013))
                .forInk(context),
      ),
      const SizedBox(width: 8),
      Expanded(child: Text(dashboardLabel(value))),
    ],
  );
  Widget componentCard(JsonObject r) => SettingsCard(
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          names[r['subject_ref']] ?? '${r['subject_ref']}',
          style: Theme.of(context).textTheme.titleMedium,
        ),
        SettingsFact('Material', dashboardLabel(r['subject_type'])),
        SettingsFact(
          'Quantity',
          '${r['quantity_numerator']}/${r['quantity_denominator']} ${r['quantity_unit_code'] ?? ''}',
        ),
        metric('Unit cost', r['unit_cost_minor'], r['currency']),
        metric('Component cost', r['component_cost_minor'], r['currency']),
        metric('Waste cost', r['waste_cost_minor'], r['currency']),
        SettingsFact('Source', dashboardLabel(r['cost_source'])),
        if (r['source_evidence_ref'] != null)
          SettingsFact('Evidence', '${r['source_evidence_ref']}'),
        if (r['incomplete_reason'] != null)
          SettingsNote(dashboardLabel(r['incomplete_reason']), warning: true),
      ],
    ),
  );
  List<Widget> contents() {
    if (widget.view == 'hub') {
      return [
        SettingsLink(
          'Batch costs',
          Icons.calculate_outlined,
          subtitle: 'Expected, actual and variance',
          onTap: preparing ? null : pickBatch,
        ),
        SettingsLink(
          'Cost policies',
          Icons.tune_rounded,
          subtitle: 'Ingredient and packaging valuation',
          onTap: () => open('policies'),
        ),
        SettingsLink(
          'Saved history',
          Icons.history_rounded,
          onTap: () => open('history'),
        ),
        SettingsLink(
          'Highest-cost materials',
          Icons.bar_chart_rounded,
          onTap: () => open('insights'),
        ),
        if (preparing)
          const Padding(
            padding: EdgeInsets.all(16),
            child: Center(child: CircularProgressIndicator()),
          ),
      ];
    }
    if (widget.view == 'policies' && widget.id != null && c.record != null) {
      final r = c.record!;
      return [
        SettingsCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                names[r['subject_ref']] ?? '${r['subject_ref']}',
                style: Theme.of(context).textTheme.headlineSmall,
              ),
              const SizedBox(height: 12),
              SettingsStatus(r['status']),
              SettingsFact(
                'Scope',
                r['branch_id'] == null || r['branch_id'] == ''
                    ? 'Whole business'
                    : '${r['branch_id']}',
              ),
              SettingsFact(
                'Method',
                costStrategies[r['strategy']] ?? dashboardLabel(r['strategy']),
              ),
              SettingsFact('Currency', '${r['currency']}'),
              SettingsFact(
                'Receipt limit',
                '${r['weighted_average_receipt_limit']}',
              ),
              if (r['strategy'] == 'MANUAL_CONFIGURED_COST') ...[
                metric(
                  'Manual unit cost',
                  r['manual_unit_cost_minor'],
                  r['currency'],
                ),
                SettingsFact('Unit', '${r['manual_unit_code']}'),
                if (r['manual_cost_reason'] != null)
                  SettingsFact('Note', '${r['manual_cost_reason']}'),
              ],
            ],
          ),
        ),
        SettingsAction(
          'Edit policy',
          icon: Icons.edit_outlined,
          busy: preparing,
          onPressed: c.allowed('PUT', '$policiesPath/{policyId}')
              ? editPolicy
              : null,
        ),
        SettingsAction(
          'Archive policy',
          danger: true,
          icon: Icons.archive_outlined,
          onPressed:
              r['status'] == 'ACTIVE' &&
                  c.allowed('POST', '$policiesPath/{policyId}/archive')
              ? () async {
                  final b = await productionForm(context, 'Archive policy', [
                    const PurchaseField('reason', 'Reason', kind: 'notes'),
                  ]);
                  if (b != null &&
                      mounted &&
                      await confirmSettings(
                        context,
                        'Archive this policy?',
                        'Future calculations will use the next applicable policy. Existing evidence is preserved.',
                        danger: true,
                      )) {
                    await change(
                      'POST',
                      '$policiesPath/{policyId}/archive',
                      b,
                      version: r['version'],
                    );
                  }
                }
              : null,
        ),
      ];
    }
    if (widget.view == 'batch') {
      return [
        if (c.record == null)
          empty(
            'No saved costs yet',
            'Save an expected or actual snapshot for this batch.',
            Icons.calculate_outlined,
          ),
        for (final key in ['expected', 'actual'])
          if (c.record?[key] is Map) snapshotCard(jsonObject(c.record![key])),
        for (final action in ['expected', 'actual'])
          SettingsAction(
            'Save $action snapshot',
            busy: c.busy && activeRoute == '$batchCostPath/$action',
            icon: Icons.add_chart_rounded,
            onPressed: c.allowed('POST', '$batchCostPath/$action')
                ? () async {
                    if (await confirmSettings(
                      context,
                      'Save $action costs?',
                      'The server will freeze costs from this batch\'s ${action == 'expected' ? 'planned requirements' : 'recorded consumption and waste'}.',
                    )) {
                      await change('POST', '$batchCostPath/$action', {});
                    }
                  }
                : null,
          ),
        SettingsLink(
          'Compare expected and actual',
          Icons.compare_arrows_rounded,
          onTap: c.stale ? null : () => open('variance', id: widget.id),
        ),
      ];
    }
    if (widget.view == 'variance' && c.record != null) {
      final r = c.record!;
      return [
        SettingsCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              completeness(r['completeness']),
              metric('Expected', r['expected_total_minor'], r['currency']),
              metric('Actual', r['actual_total_minor'], r['currency']),
              metric('Difference', r['variance_minor'], r['currency']),
              if (r['completeness'] != 'COMPLETE')
                const SettingsNote(
                  'A complete comparison needs both snapshots and resolved costs.',
                  warning: true,
                ),
            ],
          ),
        ),
        for (final line in c.collection(r['components']))
          SettingsCard(
            child: Column(
              children: [
                SettingsFact(
                  'Material',
                  names[line['subject_ref']] ?? '${line['subject_ref']}',
                ),
                SettingsFact(
                  'Planned quantity',
                  '${line['expected_quantity_numerator']}/${line['expected_quantity_denominator']}',
                ),
                SettingsFact(
                  'Actual quantity',
                  '${line['actual_quantity_numerator']}/${line['actual_quantity_denominator']}',
                ),
                metric('Expected', line['expected_cost_minor'], r['currency']),
                metric('Actual', line['actual_cost_minor'], r['currency']),
                metric('Difference', line['variance_minor'], r['currency']),
              ],
            ),
          ),
      ];
    }
    return [
      if (widget.view == 'policies')
        SettingsAction(
          'New cost policy',
          icon: Icons.add_rounded,
          busy: preparing,
          onPressed: c.allowed('POST', policiesPath) ? editPolicy : null,
        ),
      if (c.rows.isEmpty)
        empty(
          widget.view == 'policies'
              ? 'No cost policies yet'
              : widget.view == 'components'
              ? 'No component evidence'
              : 'No saved cost evidence',
          widget.view == 'policies'
              ? 'Choose how your materials are valued.'
              : 'Evidence will appear here after costs are calculated.',
          widget.view == 'policies'
              ? Icons.tune_rounded
              : Icons.receipt_long_outlined,
        ),
      for (final r in c.rows)
        switch (widget.view) {
          'policies' => SettingsLink(
            names[r['subject_ref']] ?? '${r['subject_ref']}',
            r['subject_type'] == 'INGREDIENT'
                ? Icons.grass_outlined
                : Icons.takeout_dining_outlined,
            subtitle:
                '${costStrategies[r['strategy']] ?? dashboardLabel(r['strategy'])} · ${dashboardLabel(r['status'])}',
            onTap: () => open('policies', id: '${r['id']}'),
          ),
          'components' => componentCard(r),
          'insights' => SettingsCard(
            child: Column(
              children: [
                SettingsFact(
                  'Material',
                  names[r['subject_ref']] ?? '${r['subject_ref']}',
                ),
                metric(
                  'Contributed cost',
                  r['total_cost_minor'],
                  r['currency'],
                ),
                SettingsFact('Observations', '${r['observations']}'),
              ],
            ),
          ),
          _ => snapshotCard(r),
        },
      if (c.hasMore &&
          ['history', 'policies'].contains(widget.view) &&
          widget.id == null)
        SettingsAction(
          'Load more',
          secondary: true,
          onPressed: () {
            offset += 50;
            load(more: true);
          },
        ),
    ];
  }

  Widget empty(String title, String message, IconData icon) => SettingsCard(
    child: Padding(
      padding: const EdgeInsets.symmetric(vertical: 24),
      child: Column(
        children: [
          Icon(icon, size: 52, color: KarryamColors.tealInk.forInk(context)),
          const SizedBox(height: 18),
          Text(
            title,
            style: Theme.of(context).textTheme.titleLarge,
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 8),
          Text(message, textAlign: TextAlign.center),
        ],
      ),
    ),
  );
  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      appBar: AppBar(title: Text(title)),
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
                      'A saved cost change is awaiting confirmation. Retry it before making another change.',
                      warning: true,
                    ),
                    SettingsAction(
                      'Retry saved change',
                      busy: c.busy,
                      onPressed: () async {
                        final result = await c.save();
                        if (result != null) await load();
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
                  else if (widget.view == 'hub' || !c.stale)
                    for (final w in contents())
                      Padding(
                        padding: const EdgeInsets.only(bottom: 16),
                        child: w,
                      ),
                ],
              ),
            ),
          ),
        ),
      ),
    ),
  );
}
