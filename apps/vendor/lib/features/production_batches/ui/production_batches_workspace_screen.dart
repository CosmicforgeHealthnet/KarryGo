import '../../operational_cost/ui/cost_workspace_screen.dart';
import 'dart:async';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../../settings/ui/settings_design.dart';
import '../../settings/state/settings_controller.dart';
import '../state/production_batches_workspace_controller.dart';

class ProductionBatchesScreen extends StatefulWidget {
  const ProductionBatchesScreen({
    super.key,
    required this.runtime,
    this.id,
    this.recipe,
    this.evidence,
  });
  final VendorRuntime runtime;
  final String? id, evidence;
  final JsonObject? recipe;
  @override
  State<ProductionBatchesScreen> createState() =>
      _ProductionBatchesScreenState();
}

class _ProductionBatchesScreenState extends State<ProductionBatchesScreen> {
  bool get policy => widget.recipe != null;
  String get base => '/api/v1/vendor/production-batches';
  String get detail => '$base/{batchId}';
  String get path => policy
      ? '/api/v1/vendor/production-stock-policies/branches/{branchId}'
      : widget.evidence != null
      ? '$detail/${widget.evidence}'
      : widget.id == null
      ? base
      : detail;
  Map<String, String> get params => {
    if (widget.id != null) 'batchId': widget.id!,
    if (policy) 'branchId': '${widget.recipe!['branch_id']}',
  };
  late final c = ProductionBatchesWorkspaceController(
    widget.runtime,
    path,
    parameters: params,
    branchId: policy ? '${widget.recipe!['branch_id']}' : null,
    wrapper: widget.evidence == 'inventory' ? 'reservations' : null,
  );
  JsonObject? get batch =>
      c.record?['batch'] is Map ? jsonObject(c.record!['batch']) : null;
  final recipeNames = <String, String>{};
  final materialNames = <String, String>{};
  String status = '';
  String? activeRoute;
  bool allowed(String action) => c.allowed('POST', '$detail/$action');
  bool preparing = false, discovering = false;
  String? loadError;
  @override
  void initState() {
    super.initState();
    if (policy) {
      c.readQuery = {
        for (final key in ['target_type', 'target_ref', 'target_variant_ref'])
          key: '${widget.recipe![key] ?? ''}',
      };
    }
    unawaited(load());
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> load({bool more = false}) async {
    setState(() => preparing = true);
    loadError = null;
    try {
      if (policy) {
        final r = (await widget.runtime.backend.recipesApi.request(
          'GET',
          '/api/v1/vendor/recipes/{recipeId}',
          pathParameters: {'recipeId': '${widget.recipe!['id']}'},
        )).object;
        c.validate(r);
        if (r['id'] != widget.recipe!['id'] ||
            r['branch_id'] != widget.recipe!['branch_id']) {
          throw const FormatException('Recipe scope mismatch');
        }
      }
      await c.load(append: more, status: status);
      if (policy && c.record != null) {
        for (final key in ['target_type', 'target_ref', 'target_variant_ref']) {
          if ('${c.record![key] ?? ''}' != '${widget.recipe![key] ?? ''}') {
            throw const FormatException('Policy target mismatch');
          }
        }
      }
      if (batch != null && batch!['id'] != widget.id) {
        throw const FormatException('Batch scope mismatch');
      }
      if (widget.evidence == null) {
        await Future.wait([
          (() async {
            try {
              for (final r in await recipes()) {
                recipeNames['${r['id']}'] = '${r['name']}';
              }
            } catch (_) {}
          })(),
          for (final kind in ['ingredients', 'packaging-materials'])
            (() async {
              try {
                final response = await widget
                    .runtime
                    .backend
                    .productionMasterApi
                    .request(
                      'GET',
                      '/api/v1/vendor/$kind',
                      query: {'limit': '200'},
                    );
                for (final r in c.collection(response.data)) {
                  c.validate(r);
                  materialNames['${r['id']}'] = '${r['name']}';
                }
              } catch (_) {}
            })(),
        ]);
      }
    } catch (_) {
      loadError =
          'Production details could not be loaded. Refresh before making changes.';
      c.stale = true;
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  Future<List<JsonObject>> recipes() async {
    final rows = <JsonObject>[];
    while (true) {
      final response = await widget.runtime.backend.recipesApi.request(
        'GET',
        '/api/v1/vendor/recipes',
        query: {'status': 'ACTIVE', 'limit': '200', 'offset': '${rows.length}'},
      );
      final next = c.collection(response.data);
      for (final r in next) {
        c.validate(r);
      }
      rows.addAll(next);
      if (next.length < 200) return rows;
    }
  }

  Future<void> open({String? id, JsonObject? recipe, String? evidence}) async {
    await Navigator.push(
      context,
      setupRoute<void>(
        ProductionBatchesScreen(
          runtime: widget.runtime,
          id: id,
          recipe: recipe,
          evidence: evidence,
        ),
      ),
    );
    if (mounted) await load();
  }

  Future<void> chooseRecipe({required bool stockPolicy}) async {
    setState(() => discovering = true);
    try {
      final rows = await recipes();
      if (!mounted) return;
      if (rows.isEmpty) {
        await showOrderFeedback(
          context,
          title: 'Create a recipe first',
          message: 'Production setup starts with a catalogue recipe.',
          success: false,
          warning: true,
        );
        return;
      }
      final selected = await purchaseForm(
        context,
        stockPolicy ? 'Select recipe stock policy' : 'Select batch recipe',
        [
          PurchaseField(
            'recipe_id',
            'Recipe',
            kind: 'select',
            required: true,
            options: purchaseChoices(rows),
            initial: widget.runtime.mode == VendorDataMode.mock
                ? rows.first['id']
                : null,
          ),
        ],
      );
      if (selected == null || !mounted) return;
      final r = rows.firstWhere((r) => r['id'] == selected['recipe_id']);
      if (stockPolicy) {
        await open(recipe: r);
        return;
      }
      final versions = c
          .collection(
            (await widget.runtime.backend.recipesApi.request(
              'GET',
              '/api/v1/vendor/recipes/{recipeId}/versions',
              pathParameters: {'recipeId': '${r['id']}'},
            )).data,
          )
          .where((v) => v['status'] == 'ACTIVE')
          .toList();
      if (!mounted) return;
      if (versions.isEmpty) {
        await showOrderFeedback(
          context,
          title: 'Activate a recipe version',
          message:
              'Open Recipes, finish a draft and activate it before creating a production batch.',
          success: false,
          warning: true,
        );
        return;
      }
      final version = versions.first;
      c.validate(version);
      if (version['recipe_id'] != r['id']) {
        throw const FormatException('Version scope mismatch');
      }
      final body = await purchaseForm(context, 'Create production batch', [
        PurchaseField(
          'planned_output_numerator',
          'How much to make (${version['output_unit_code'] ?? 'units'})',
          kind: 'number',
          required: true,
          min: 1,
          initial: widget.runtime.mode == VendorDataMode.mock
              ? version['expected_output_numerator']
              : null,
        ),
        PurchaseField(
          'planned_output_denominator',
          'Divided by (usually 1)',
          kind: 'number',
          required: true,
          min: 1,
          initial: 1,
        ),
      ]);
      if (body == null || !mounted) return;
      body.addAll({
        'branch_id': r['branch_id'],
        'recipe_id': r['id'],
        'recipe_version_id': version['id'],
      });
      final result = await save('POST', base, body, parameters: {});
      if (result != null && mounted) await open(id: '${result['id']}');
    } catch (_) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Recipe selection unavailable',
          message:
              'The recipe or its active version could not be read. Refresh and check access.',
          success: false,
          warning: true,
        );
      }
    } finally {
      if (mounted) setState(() => discovering = false);
    }
  }

  Future<JsonObject?> save(
    String method,
    String route,
    JsonObject body, {
    Map<String, String>? parameters,
    int? version,
    bool confirm = false,
    bool danger = false,
  }) async {
    if (confirm &&
        (!await confirmSettings(
              context,
              danger
                  ? 'Confirm production change?'
                  : 'Continue with production?',
              danger
                  ? 'This action changes the saved batch.'
                  : 'The server will apply this action to the batch and its inventory.',
              danger: danger,
            ) ||
            !mounted)) {
      return null;
    }
    activeRoute = route;
    final result = await c.save(
      method: method,
      route: route,
      body: body,
      params: parameters ?? params,
      headerVersion: version,
    );
    if (!mounted) return result;
    setState(() => activeRoute = null);
    await showOrderFeedback(
      context,
      title: result == null ? 'Change needs attention' : 'Production updated',
      message: c.error ?? 'The latest production record is ready.',
      success: result != null,
      warning: result == null,
    );
    if (result != null && mounted) await load();
    return result;
  }

  Future<void> preview() async {
    final result = await c.save(
      method: 'POST',
      route: '$detail/preview',
      body: {},
      params: params,
    );
    if (!mounted) return;
    if (result == null) {
      await showOrderFeedback(
        context,
        title: 'Preview unavailable',
        message: c.error ?? 'Refresh to try again.',
        success: false,
        warning: true,
      );
      return;
    }
    await settingsSheet<void>(
      context,
      Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const SettingsHeading('Batch requirements'),
          const Text('Planning reserves the controlled inventory shown here.'),
          const SizedBox(height: 20),
          for (final r in c.collection(result['requirements']))
            Padding(
              padding: const EdgeInsets.only(bottom: 20),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    materialNames[r['reference_id']] ?? 'Material',
                    style: Theme.of(context).textTheme.titleMedium,
                  ),
                  const SizedBox(height: 6),
                  Text(
                    '${r['quantity_numerator']} / ${r['quantity_denominator']} ${r['unit_code']}',
                  ),
                  Text(
                    r['inventory_controlled'] == true
                        ? 'Reserve ${r['reservation_quantity']} inventory units'
                        : 'Untracked material',
                  ),
                ],
              ),
            ),
          SettingsAction('Done', onPressed: () => Navigator.pop(context)),
        ],
      ),
    );
  }

  Map<String, ({String kind, JsonObject row})> references({
    bool topUp = false,
  }) => {
    for (final packaging in [false, true])
      for (final row in c.collection(
        c.record![packaging ? 'packaging' : 'components'],
      ))
        if (!topUp ||
            row['inventory_controlled'] == true &&
                (!packaging ||
                    row['production_consumption_point'] == 'PRODUCTION'))
          '${row[packaging ? 'packaging_material_id' : 'ingredient_id']}': (
            kind: packaging ? 'PACKAGING' : 'INGREDIENT',
            row: row,
          ),
  };
  Future<void> record(String action) async {
    final refs = references(topUp: action == 'inventory-adjustments');
    if (refs.isEmpty) {
      await showOrderFeedback(
        context,
        title: 'No eligible materials',
        message: 'This batch has no materials eligible for this action.',
        success: false,
        warning: true,
      );
      return;
    }
    final selected = await purchaseForm(context, 'Select material', [
      PurchaseField(
        'reference_id',
        'Material',
        kind: 'select',
        required: true,
        options: {
          for (final entry in refs.entries)
            entry.key: materialNames[entry.key] ?? entry.key,
        },
        initial: widget.runtime.mode == VendorDataMode.mock
            ? refs.keys.first
            : null,
      ),
    ]);
    if (selected == null || !mounted) return;
    final target = refs[selected['reference_id']]!;
    final row = target.row;
    final mock = widget.runtime.mode == VendorDataMode.mock;
    final body = await purchaseForm(
      context,
      action == 'waste'
          ? 'Record waste'
          : action == 'inventory-adjustments'
          ? 'Reserve additional stock'
          : 'Record actual consumption',
      [
        if (action == 'inventory-adjustments')
          PurchaseField(
            'quantity',
            'Extra stock to set aside',
            kind: 'number',
            required: true,
            min: 1,
            initial: mock ? 1 : null,
          )
        else ...[
          PurchaseField(
            'quantity_numerator',
            action == 'waste'
                ? 'Extra quantity wasted'
                : 'Quantity used',
            kind: 'number',
            required: true,
            min: action == 'waste' ? 1 : 0,
            initial: mock
                ? (action == 'waste' ? 1 : row['planned_quantity_numerator'])
                : null,
          ),
          PurchaseField(
            'quantity_denominator',
            'Divided by (usually 1)',
            kind: 'number',
            required: true,
            min: 1,
            initial: row['inventory_controlled'] == true
                ? 1
                : (mock ? row['planned_quantity_denominator'] : 1),
          ),
        ],
        if (action == 'waste')
          PurchaseField(
            'reason_code',
            'Why was it wasted?',
            kind: 'select',
            required: true,
            options: purchaseEnums([
              'PREPARATION_LOSS',
              'SPOILED',
              'SPILLED',
              'BURNT',
              'QUALITY_REJECT',
              'OTHER',
            ]),
            initial: mock ? 'PREPARATION_LOSS' : null,
          ),
      ],
      validate: (body) {
        if (row['inventory_controlled'] == true &&
            action != 'inventory-adjustments' &&
            body['quantity_denominator'] != 1) {
          return 'Enter a whole quantity for stock-tracked materials.';
        }
        return null;
      },
    );
    if (body == null || !mounted) return;
    body.addAll({
      'kind': target.kind,
      'reference_id': selected['reference_id'],
    });
    await save(
      'POST',
      '$detail/$action',
      action == 'inventory-adjustments'
          ? {
              'lines': [body],
            }
          : body,
      version: batch!['version'] as int,
      confirm: true,
    );
  }

  bool get needsActual =>
      [
        ...c.collection(c.record?['components']),
        ...c.collection(c.record?['packaging']),
      ].any(
        (r) =>
            r['inventory_controlled'] == true &&
            r['optional'] != true &&
            (r['production_consumption_point'] == null ||
                r['production_consumption_point'] == 'PRODUCTION') &&
            r['actual_recorded'] != true,
      );
  Future<void> complete() async {
    final b = batch!;
    final body = await purchaseForm(
      context,
      'Complete batch',
      [
        PurchaseField(
          'actual_output_numerator',
          'How much was made',
          kind: 'number',
          required: true,
          min: 1,
          initial: widget.runtime.mode == VendorDataMode.mock
              ? b['planned_output_numerator']
              : null,
        ),
        PurchaseField(
          'actual_output_denominator',
          'Divided by (usually 1)',
          kind: 'number',
          required: true,
          min: 1,
          initial: 1,
        ),
      ],
      validate: (body) {
        if (b['operating_model'] == 'BATCH_STOCK') {
          final n =
              BigInt.from(body['actual_output_numerator']) *
              BigInt.from(b['output_inventory_numerator']);
          final d =
              BigInt.from(body['actual_output_denominator']) *
              BigInt.from(b['output_inventory_denominator']);
          if (n % d != BigInt.zero) {
            return 'Prepared output must convert to a whole inventory quantity.';
          }
        }
        return null;
      },
    );
    if (body == null || !mounted) return;
    await save(
      'POST',
      '$detail/complete',
      body,
      version: b['version'] as int,
      confirm: true,
    );
  }

  Future<void> cancel() async {
    final body = await purchaseForm(
      context,
      'Cancel batch',
      [
        PurchaseField(
          'reason',
          'Cancellation reason',
          kind: 'notes',
          required: true,
        ),
      ],
      validate: (body) => '${body['reason']}'.length > 1000
          ? 'Keep the reason within 1,000 characters.'
          : null,
    );
    if (body == null || !mounted) return;
    await save(
      'POST',
      '$detail/cancel',
      body,
      version: batch!['version'] as int,
      confirm: true,
      danger: true,
    );
  }

  Future<void> editPolicy() async {
    final row = c.record!, r = widget.recipe!;
    Map<String, String> choices = {};
    setState(() => discovering = true);
    try {
      int offset = 0;
      while (true) {
        final response = await widget.runtime.backend.inventoryApi.request(
          'GET',
          '/api/v1/vendor/inventory/branches/{branchId}/balances',
          pathParameters: {'branchId': '${r['branch_id']}'},
          query: {'limit': '200', 'offset': '$offset'},
        );
        final rows = SettingsController.objects(response.data);
        for (final item in rows) {
          c.validate(item);
          choices['${item['inventory_item_id']}'] =
              '${item['name'] ?? ''}'.isNotEmpty
              ? '${item['name']}'
              : 'Stock item ${choices.length + 1}';
        }
        offset += rows.length;
        if (rows.length < 200) break;
      }
    } catch (_) {
    } finally {
      if (mounted) setState(() => discovering = false);
    }
    if (!mounted) return;
    if ('${row['prepared_inventory_item_id'] ?? ''}'.isNotEmpty) {
      choices.putIfAbsent(
        '${row['prepared_inventory_item_id']}',
        () => 'Current stock item',
      );
    }
    final body = await purchaseForm(
      context,
      'Production stock policy',
      [
        PurchaseField(
          'operating_model',
          'How you produce',
          kind: 'select',
          required: true,
          options: purchaseEnums(['ON_DEMAND', 'BATCH_STOCK']),
          initial: row['operating_model'],
        ),
        PurchaseField(
          'prepared_inventory_item_id',
          'Stock item for prepared output',
          kind: 'select',
          options: choices,
          initial: row['prepared_inventory_item_id'],
        ),
        PurchaseField(
          'output_inventory_numerator',
          'Stock units per output (quantity)',
          kind: 'number',
          required: true,
          min: 1,
          initial: row['output_inventory_numerator'],
        ),
        PurchaseField(
          'output_inventory_denominator',
          'Divided by (usually 1)',
          kind: 'number',
          required: true,
          min: 1,
          initial: row['output_inventory_denominator'],
        ),
      ],
      validate: (body) =>
          body['operating_model'] == 'BATCH_STOCK' &&
              body['prepared_inventory_item_id'] == null
          ? 'Choose an active production-component inventory item.'
          : null,
    );
    if (body == null || !mounted) return;
    body['prepared_inventory_item_id'] = body['operating_model'] == 'ON_DEMAND'
        ? ''
        : body['prepared_inventory_item_id'];
    body.addAll({
      for (final key in ['target_type', 'target_ref', 'target_variant_ref'])
        key: r[key] ?? '',
    });
    await save(
      'PUT',
      path,
      body,
      version: row['version'] as int,
      confirm: true,
    );
  }

  Widget metrics(Map<String, Object?> values) => Wrap(
    spacing: 20,
    runSpacing: 20,
    children: [
      for (final e in values.entries)
        SizedBox(
          width: 140,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(e.key, style: Theme.of(context).textTheme.bodySmall),
              const SizedBox(height: 6),
              Text('${e.value}', style: Theme.of(context).textTheme.titleSmall),
            ],
          ),
        ),
    ],
  );
  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      appBar: AppBar(
        title: Text(
          policy
              ? 'Production stock policy'
              : widget.evidence == 'inventory'
              ? 'Inventory reservations'
              : widget.evidence == 'output'
              ? 'Prepared output'
              : 'Production batches',
        ),
        actions: [
          IconButton(
            tooltip: 'Refresh',
            onPressed: preparing || c.busy || discovering ? null : () => load(),
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(20),
          children: [
            if (preparing || c.loading)
              const PremiumSkeleton(detail: true)
            else ...[
              if (loadError != null) SettingsNote(loadError!),
              if (c.error != null) SettingsNote(c.error!),
              if (c.pending != null) ...[
                SettingsCard(
                  child: Column(
                    children: [
                      const Text(
                        'A saved production action needs confirmation.',
                      ),
                      const SizedBox(height: 16),
                      SettingsAction(
                        'Retry saved action',
                        busy: c.busy,
                        onPressed: c.blocked
                            ? null
                            : () async {
                                final result = await c.save();
                                if (!context.mounted) return;
                                await showOrderFeedback(
                                  context,
                                  title: result == null
                                      ? 'Action needs attention'
                                      : 'Action confirmed',
                                  message:
                                      c.error ??
                                      'The original production action was confirmed.',
                                  success: result != null,
                                  warning: result == null,
                                );
                                if (result != null && mounted) await load();
                              },
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 20),
              ],
              if (policy && c.record != null) ...[
                SettingsHeading('${widget.recipe!['name']}'),
                SettingsCard(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Icon(
                        Icons.inventory_2_outlined,
                        size: 48,
                        color: KarryamColors.tealInk.forInk(context),
                      ),
                      const SizedBox(height: 20),
                      metrics({
                        'How you produce': dashboardLabel(
                          c.record!['operating_model'],
                        ),
                        'Output conversion':
                            '${c.record!['output_inventory_numerator']} / ${c.record!['output_inventory_denominator']}',
                      }),
                      const SizedBox(height: 16),
                      Text(
                        c.record!['operating_model'] == 'ON_DEMAND'
                            ? 'Completion does not post prepared stock.'
                            : 'Completion posts prepared stock through the saved output conversion.',
                      ),
                      if ('${c.record!['prepared_inventory_item_id'] ?? ''}'
                          .isNotEmpty) ...[
                        const SizedBox(height: 16),
                        SelectableText(
                          '${c.record!['prepared_inventory_item_id']}',
                        ),
                      ],
                    ],
                  ),
                ),
                const SizedBox(height: 20),
                SettingsAction(
                  'Edit stock policy',
                  busy: discovering || c.busy,
                  onPressed: c.writable && !discovering ? editPolicy : null,
                ),
              ] else if (widget.evidence != null) ...[
                if (widget.evidence == 'inventory' &&
                    !c.stale &&
                    c.rows.isEmpty)
                  const SettingsCard(
                    child: Column(
                      children: [
                        Icon(Icons.inventory_2_outlined, size: 56),
                        SizedBox(height: 16),
                        Text('No inventory reservations for this batch'),
                      ],
                    ),
                  ),
                for (final r in c.rows) ...[
                  SettingsCard(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          dashboardLabel(r['kind']),
                          style: Theme.of(context).textTheme.titleLarge,
                        ),
                        const SizedBox(height: 12),
                        SettingsStatus(r['status']),
                        const SizedBox(height: 16),
                        SelectableText('${r['inventory_reservation_id']}'),
                      ],
                    ),
                  ),
                  const SizedBox(height: 12),
                ],
                if (widget.evidence == 'output' && c.record != null)
                  SettingsCard(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          '${c.record!['quantity']} inventory units',
                          style: Theme.of(context).textTheme.headlineSmall,
                        ),
                        const SizedBox(height: 20),
                        SelectableText('${c.record!['inventory_item_id']}'),
                        if ('${c.record!['inventory_lot_id'] ?? ''}'
                            .isNotEmpty) ...[
                          const SizedBox(height: 12),
                          Text('Lot: ${c.record!['inventory_lot_id']}'),
                        ],
                      ],
                    ),
                  ),
              ] else if (widget.id == null) ...[
                SettingsAction(
                  'Create production batch',
                  icon: Icons.add_rounded,
                  busy: discovering,
                  onPressed: c.writable && !discovering
                      ? () => chooseRecipe(stockPolicy: false)
                      : null,
                ),
                const SizedBox(height: 12),
                SettingsLink(
                  'Production stock policies',
                  Icons.tune_rounded,
                  onTap: () => chooseRecipe(stockPolicy: true),
                ),
                const SizedBox(height: 20),
                Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  children: [
                    for (final s in [
                      '',
                      'DRAFT',
                      'PLANNED',
                      'IN_PROGRESS',
                      'COMPLETED',
                      'CANCELLED',
                      'FAILED',
                    ])
                      ChoiceChip(
                        label: Text(s.isEmpty ? 'All' : dashboardLabel(s)),
                        selected: status == s,
                        onSelected: c.busy
                            ? null
                            : (_) {
                                setState(() => status = s);
                                unawaited(load());
                              },
                      ),
                  ],
                ),
                const SizedBox(height: 20),
                if (!c.stale && c.rows.isEmpty)
                  const SettingsCard(
                    child: Column(
                      children: [
                        Icon(Icons.soup_kitchen_outlined, size: 64),
                        SizedBox(height: 20),
                        Text('Ready for your next batch'),
                        SizedBox(height: 8),
                        Text(
                          'Activate a recipe version, then plan its production.',
                        ),
                      ],
                    ),
                  ),
                for (final r in c.rows) ...[
                  SettingsCard(
                    child: InkWell(
                      onTap: () => open(id: '${r['id']}'),
                      child: Row(
                        children: [
                          Icon(
                            Icons.soup_kitchen_outlined,
                            color: KarryamColors.tealInk.forInk(context),
                          ),
                          const SizedBox(width: 16),
                          Expanded(
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Text(
                                  recipeNames[r['recipe_id']] ??
                                      '${r['recipe_id']}',
                                  style: Theme.of(
                                    context,
                                  ).textTheme.titleMedium,
                                ),
                                const SizedBox(height: 8),
                                Text(
                                  '${r['planned_output_numerator']} / ${r['planned_output_denominator']} ${r['output_unit_code']}',
                                ),
                                const SizedBox(height: 10),
                                SettingsStatus(r['status']),
                              ],
                            ),
                          ),
                          const Icon(Icons.chevron_right_rounded),
                        ],
                      ),
                    ),
                  ),
                  const SizedBox(height: 12),
                ],
                if (c.hasMore)
                  SettingsAction(
                    'Load more',
                    secondary: true,
                    onPressed: c.busy ? null : () => load(more: true),
                  ),
              ] else if (batch case final b?) ...[
                SettingsHeading(
                  recipeNames[b['recipe_id']] ?? 'Production batch',
                  subtitle: 'Recipe version ${b['recipe_version_number']}',
                ),
                SettingsStatus(b['status']),
                const SizedBox(height: 20),
                SettingsCard(
                  child: metrics({
                    'Planned output':
                        '${b['planned_output_numerator']} / ${b['planned_output_denominator']} ${b['output_unit_code']}',
                    'How you produce': dashboardLabel(b['operating_model']),
                    if (b['status'] == 'COMPLETED')
                      'Actual output':
                          '${b['actual_output_numerator']} / ${b['actual_output_denominator']} ${b['output_unit_code']}',
                  }),
                ),
                const SizedBox(height: 20),
                for (final packaging in [false, true])
                  if (c
                      .collection(
                        c.record![packaging ? 'packaging' : 'components'],
                      )
                      .isNotEmpty) ...[
                    SettingsCard(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            packaging ? 'Packaging' : 'Ingredients',
                            style: Theme.of(context).textTheme.titleLarge,
                          ),
                          for (final r in c.collection(
                            c.record![packaging ? 'packaging' : 'components'],
                          )) ...[
                            const Divider(height: 32),
                            Text(
                              materialNames[r[packaging
                                      ? 'packaging_material_id'
                                      : 'ingredient_id']] ??
                                  '${r[packaging ? 'packaging_material_id' : 'ingredient_id']}',
                            ),
                            const SizedBox(height: 12),
                            metrics({
                              'Planned':
                                  '${r['planned_quantity_numerator']} / ${r['planned_quantity_denominator']} ${r['unit_code']}',
                              'Reserved': r['reserved_quantity'],
                              'Used':
                                  '${r['actual_used_numerator']} / ${r['actual_used_denominator']}',
                              'Waste':
                                  '${r['actual_waste_numerator']} / ${r['actual_waste_denominator']}',
                            }),
                            const SizedBox(height: 10),
                            Text(
                              r['actual_recorded'] == true
                                  ? 'Consumption recorded'
                                  : r['inventory_controlled'] == true
                                  ? 'Consumption required'
                                  : 'Untracked material',
                            ),
                          ],
                        ],
                      ),
                    ),
                    const SizedBox(height: 16),
                  ],
                if (b['status'] == 'DRAFT') ...[
                  SettingsAction(
                    'Preview requirements',
                    secondary: true,
                    onPressed: allowed('preview') ? preview : null,
                  ),
                  const SizedBox(height: 12),
                  SettingsAction(
                    'Plan batch',
                    busy: c.busy && activeRoute == '$detail/plan',
                    onPressed: allowed('plan')
                        ? () => save(
                            'POST',
                            '$detail/plan',
                            {},
                            version: b['version'] as int,
                            confirm: true,
                          )
                        : null,
                  ),
                  const SizedBox(height: 12),
                ],
                if (b['status'] == 'PLANNED') ...[
                  SettingsAction(
                    'Start production',
                    busy: c.busy && activeRoute == '$detail/start',
                    onPressed: allowed('start')
                        ? () => save(
                            'POST',
                            '$detail/start',
                            {},
                            version: b['version'] as int,
                            confirm: true,
                          )
                        : null,
                  ),
                  const SizedBox(height: 12),
                ],
                if (b['status'] == 'IN_PROGRESS') ...[
                  SettingsAction(
                    'Record consumption',
                    secondary: true,
                    onPressed: allowed('actual-consumption')
                        ? () => record('actual-consumption')
                        : null,
                  ),
                  const SizedBox(height: 12),
                  SettingsAction(
                    'Record waste',
                    secondary: true,
                    onPressed: allowed('waste') ? () => record('waste') : null,
                  ),
                  const SizedBox(height: 12),
                  if (needsActual)
                    const SettingsNote(
                      'Record required controlled-material consumption before completing the batch.',
                      warning: true,
                    ),
                  SettingsAction(
                    'Complete batch',
                    busy: c.busy && activeRoute == '$detail/complete',
                    onPressed: allowed('complete') && !needsActual
                        ? complete
                        : null,
                  ),
                  const SizedBox(height: 12),
                ],
                if (['PLANNED', 'IN_PROGRESS'].contains(b['status'])) ...[
                  SettingsAction(
                    'Reserve additional stock',
                    secondary: true,
                    onPressed: allowed('inventory-adjustments')
                        ? () => record('inventory-adjustments')
                        : null,
                  ),
                  const SizedBox(height: 12),
                ],
                SettingsLink(
                  'Batch costs',
                  Icons.calculate_outlined,
                  onTap: () => Navigator.push(
                    context,
                    setupRoute<void>(
                      CostWorkspaceScreen(
                        runtime: widget.runtime,
                        view: 'batch',
                        id: widget.id,
                      ),
                    ),
                  ),
                ),
                SettingsLink(
                  'Inventory reservations',
                  Icons.inventory_2_outlined,
                  onTap: () => open(id: widget.id, evidence: 'inventory'),
                ),
                if (b['status'] == 'COMPLETED' &&
                    b['operating_model'] == 'BATCH_STOCK')
                  SettingsLink(
                    'Prepared output',
                    Icons.outbox_outlined,
                    onTap: () => open(id: widget.id, evidence: 'output'),
                  ),
                if ([
                  'DRAFT',
                  'PLANNED',
                  'IN_PROGRESS',
                ].contains(b['status'])) ...[
                  const SizedBox(height: 20),
                  SettingsAction(
                    'Cancel batch',
                    danger: true,
                    onPressed: allowed('cancel') ? cancel : null,
                  ),
                ],
              ],
            ],
          ],
        ),
      ),
    ),
  );
}
