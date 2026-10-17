import '../../support/ui/order_support_screen.dart' show supportDate;
import 'dart:async';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../../settings/ui/settings_design.dart';
import '../state/recipes_workspace_controller.dart';
import 'recipe_requirements_screen.dart';

class RecipesWorkspaceScreen extends StatefulWidget {
  const RecipesWorkspaceScreen({
    super.key,
    required this.runtime,
    this.recipeId,
    this.versionId,
    this.branchId,
    this.view = 'recipes',
  });
  final VendorRuntime runtime;
  final String? recipeId, versionId, branchId;
  final String view;
  @override
  State<RecipesWorkspaceScreen> createState() => _RecipesWorkspaceScreenState();
}

class _RecipesWorkspaceScreenState extends State<RecipesWorkspaceScreen> {
  String get base => '/api/v1/vendor/recipes';
  String get recipePath => '$base/{recipeId}';
  String get versionPath => '$recipePath/versions/{versionId}';
  bool get cost => widget.view.startsWith('cost');
  String get path => switch (widget.view) {
    'cost-history' => '/api/v1/vendor/recipe-cost/recipes/{recipeId}/history',
    'cost-current' => '/api/v1/vendor/recipe-cost/recipes/{recipeId}',
    'cost-latest' => '/api/v1/vendor/recipe-cost/versions/{versionId}/latest',
    'cost-preview' => '/api/v1/vendor/recipe-cost/versions/{versionId}',
    _ =>
      widget.versionId != null
          ? versionPath
          : widget.recipeId != null
          ? recipePath
          : base,
  };
  Map<String, String> get params => {
    if (widget.recipeId != null) 'recipeId': widget.recipeId!,
    if (widget.versionId != null) 'versionId': widget.versionId!,
  };
  late final c = RecipesWorkspaceController(
    widget.runtime,
    path,
    parameters: params,
    branchId: widget.branchId,
    wrapper: widget.view == 'cost-history' ? 'snapshots' : null,
  );
  List<JsonObject> versions = [];
  final materialNames = <String, String>{};
  bool preparing = false, discovering = false;
  String status = '', search = '';
  Timer? searchTimer;
  JsonObject? get v => widget.versionId == null
      ? null
      : c.record?['version'] is Map
      ? jsonObject(c.record!['version'])
      : null;
  JsonObject? recipe;
  String? loadError;
  @override
  void initState() {
    super.initState();
    unawaited(load());
  }

  @override
  void dispose() {
    searchTimer?.cancel();
    c.dispose();
    super.dispose();
  }

  Future<void> load({bool more = false}) async {
    setState(() => preparing = true);
    loadError = null;
    try {
      if (widget.recipeId != null && !cost) {
        final parent = jsonObject(await c.fetch(recipePath, params: params));
        c.validate(parent);
        if (parent['id'] != widget.recipeId) {
          throw const FormatException('Recipe mismatch');
        }
        recipe = parent;
      }
      await c.load(append: more, status: status);
      if (widget.recipeId != null &&
          widget.versionId == null &&
          !cost &&
          !c.stale) {
        versions = c.collection(
          await c.fetch('$recipePath/versions', params: params),
        );
        for (final row in versions) {
          c.validate(row);
          if (row['recipe_id'] != widget.recipeId) {
            throw const FormatException('Recipe version mismatch');
          }
        }
      }
      if (v != null) {
        for (final key in ['components', 'packaging_requirements']) {
          for (final line in c.collection(c.record![key])) {
            c.validate(line);
            if (line['recipe_version_id'] != widget.versionId) {
              throw const FormatException('Requirement version mismatch');
            }
          }
        }
      }
      if (v != null || cost) {
        await Future.wait([
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
                for (final row in c.collection(response.data)) {
                  c.validate(row);
                  materialNames['${row['id']}'] = '${row['name']}';
                }
              } catch (_) {}
            })(),
        ]);
      }
      if (v != null &&
          (v!['recipe_id'] != widget.recipeId ||
              v!['id'] != widget.versionId)) {
        throw const FormatException('Version mismatch');
      }
    } catch (_) {
      loadError =
          'Recipe details could not be loaded. Refresh before making changes.';
      c.stale = true;
      versions = [];
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  Future<void> open({
    String? recipeId,
    String? versionId,
    String? branchId,
    String view = 'recipes',
  }) async {
    await Navigator.push(
      context,
      setupRoute<void>(
        RecipesWorkspaceScreen(
          runtime: widget.runtime,
          recipeId: recipeId ?? widget.recipeId,
          versionId: versionId,
          branchId:
              branchId ?? widget.branchId ?? recipe?['branch_id'] as String?,
          view: view,
        ),
      ),
    );
    if (mounted) await load();
  }

  Future<JsonObject?> command(
    String method,
    String route,
    JsonObject body, {
    Map<String, String>? parameters,
    int? version,
    bool confirm = false,
    bool danger = false,
    Map<String, String> query = const {},
  }) async {
    if (confirm &&
        (!await confirmSettings(
              context,
              danger ? 'Confirm this action?' : 'Save this snapshot?',
              danger
                  ? 'This updates the saved recipe lifecycle.'
                  : 'This records immutable cost evidence.',
              danger: danger,
            ) ||
            !mounted)) {
      return null;
    }
    final result = await c.save(
      method: method,
      route: route,
      body: body,
      params: parameters ?? params,
      headerVersion: version,
      query: query,
    );
    if (!mounted) return result;
    await showOrderFeedback(
      context,
      title: result == null ? 'Change needs attention' : 'Changes saved',
      message: c.error ?? 'The latest recipe record is ready.',
      success: result != null,
      warning: result == null,
    );
    if (result != null && mounted) await load();
    return result;
  }

  List<JsonObject> objects(Object? data, String key) {
    if (data is Map) data = data[key] ?? data['items'];
    return c.collection(data);
  }

  Future<List<JsonObject>> catalogueList(
    VendorFeatureApi api,
    String route,
    String key, {
    Map<String, String> params = const {},
    Map<String, String> query = const {},
  }) async {
    final result = <JsonObject>[];
    final fields = api
        .operation('GET', route)
        .parametersIn('query')
        .map((p) => p['name'])
        .toSet();
    while (true) {
      final response = await api.request(
        'GET',
        route,
        pathParameters: params,
        query: {
          for (final e in query.entries)
            if (fields.contains(e.key)) e.key: e.value,
          if (fields.contains('limit')) 'limit': '200',
          if (fields.contains('offset')) 'offset': '${result.length}',
        },
      );
      final next = objects(response.data, key);
      result.addAll(next);
      if (!fields.contains('offset') || next.length < 200) return result;
    }
  }

  Future<void> createRecipe() async {
    final type = await purchaseForm(context, 'Recipe target', [
      PurchaseField(
        'target_type',
        'Catalogue type',
        kind: 'select',
        required: true,
        options: purchaseEnums([
          'FOOD_ITEM',
          'PASTRY_PRODUCT',
          'CLOUD_KITCHEN_PACKAGE',
        ]),
        initial: 'FOOD_ITEM',
      ),
    ]);
    if (type == null || !mounted) return;
    setState(() => discovering = true);
    try {
      final app = widget.runtime.backend;
      final targets = <JsonObject>[];
      if (type['target_type'] == 'FOOD_ITEM') {
        final menus = await catalogueList(
          app.foodCatalogueApi,
          '/api/v1/vendor/food/menus',
          'menus',
        );
        for (final menu in menus) {
          final sections = await catalogueList(
            app.foodCatalogueApi,
            '/api/v1/vendor/food/menus/{menuId}/sections',
            'sections',
            params: {'menuId': '${menu['id']}'},
          );
          for (final section in sections) {
            targets.addAll(
              await catalogueList(
                app.foodCatalogueApi,
                '/api/v1/vendor/food/menus/{menuId}/sections/{sectionId}/items',
                'items',
                params: {
                  'menuId': '${menu['id']}',
                  'sectionId': '${section['id']}',
                },
              ),
            );
          }
        }
      } else if (type['target_type'] == 'PASTRY_PRODUCT') {
        targets.addAll(
          await catalogueList(
            app.pastryApi,
            '/api/v1/vendor/pastry/products',
            'products',
          ),
        );
      } else {
        targets.addAll(
          await catalogueList(
            app.cloudKitchenApi,
            '/api/v1/vendor/cloud-kitchen/bulk-packages',
            'packages',
          ),
        );
      }
      final active = targets
          .where((r) => r['status'] != 'ARCHIVED' && r['status'] != 'INACTIVE')
          .toList();
      if (!mounted) return;
      if (active.isEmpty) {
        await showOrderFeedback(
          context,
          title: 'No catalogue targets',
          message:
              'Create an eligible catalogue item before adding its recipe.',
          success: false,
          warning: true,
        );
        return;
      }
      final mock = widget.runtime.mode == VendorDataMode.mock;
      final body = await purchaseForm(
        context,
        'Create recipe',
        [
          PurchaseField(
            'target_ref',
            'Catalogue item',
            kind: 'select',
            required: true,
            options: purchaseChoices(active),
            initial: mock ? active.first['id'] : null,
          ),
          PurchaseField(
            'name',
            'Recipe name',
            required: true,
            initial: mock ? 'Kitchen preparation' : null,
          ),
          PurchaseField(
            'internal_description',
            'Internal description',
            kind: 'notes',
          ),
        ],
        validate: (body) =>
            '${body['name']}'.length > 200 ||
                '${body['internal_description'] ?? ''}'.length > 4000
            ? 'Shorten the name or description.'
            : null,
      );
      if (body == null || !mounted) return;
      body['target_type'] = type['target_type'];
      if (type['target_type'] == 'FOOD_ITEM') {
        final variants = await catalogueList(
          app.foodCatalogueApi,
          '/api/v1/vendor/food/items/{itemId}/variants',
          'variants',
          params: {'itemId': '${body['target_ref']}'},
        );
        if (variants.isNotEmpty && mounted) {
          final selected = await purchaseForm(context, 'Recipe variant', [
            PurchaseField(
              'target_variant_ref',
              'Variant',
              kind: 'select',
              required: true,
              options: {'': 'Base item', ...purchaseChoices(variants)},
              initial: '',
            ),
          ]);
          if (selected == null) return;
          body['target_variant_ref'] = selected['target_variant_ref'];
        }
      }
      if (!mounted) return;
      final result = await command('POST', base, body);
      if (result != null && mounted) {
        await open(
          recipeId: '${result['id']}',
          branchId: '${result['branch_id']}',
        );
      }
    } catch (_) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Catalogue unavailable',
          message:
              'This catalogue type could not be loaded. Check access and branch capability.',
          success: false,
          warning: true,
        );
      }
    } finally {
      if (mounted) setState(() => discovering = false);
    }
  }

  Future<void> versionForm({bool edit = false}) async {
    final row = edit ? v! : <String, dynamic>{};
    final mock = widget.runtime.mode == VendorDataMode.mock && !edit;
    final body = await purchaseForm(
      context,
      edit ? 'Edit draft yield' : 'Create draft version',
      [
        PurchaseField(
          'internal_notes',
          'Internal notes',
          kind: 'notes',
          initial: row['internal_notes'],
        ),
        PurchaseField(
          'expected_output_numerator',
          'Recipe makes (quantity)',
          kind: 'number',
          required: true,
          min: 1,
          initial: row['expected_output_numerator'] ?? (mock ? 1 : null),
        ),
        PurchaseField(
          'expected_output_denominator',
          'Divided by (usually 1)',
          kind: 'number',
          required: true,
          min: 1,
          initial: row['expected_output_denominator'] ?? 1,
        ),
        PurchaseField(
          'output_unit_code',
          'Output unit',
          required: true,
          initial: row['output_unit_code'] ?? (mock ? 'SERVING' : null),
        ),
        PurchaseField(
          'serving_count',
          'Serving count',
          kind: 'number',
          initial: row['serving_count'] ?? (mock ? 1 : null),
        ),
        PurchaseField(
          'expected_waste_bps',
          'Expected waste (%)',
          kind: 'percent',
          required: true,
          max: 9999,
          initial: row['expected_waste_bps'] ?? 0,
        ),
      ],
      validate: (body) =>
          '${body['output_unit_code']}'.length > 80 ||
              '${body['internal_notes'] ?? ''}'.length > 4000
          ? 'Shorten the unit or notes.'
          : null,
    );
    if (body == null || !mounted) return;
    body.removeWhere((key, value) => value == null);
    body['output_unit_code'] = '${body['output_unit_code']}'.toUpperCase();
    final result = await command(
      edit ? 'PUT' : 'POST',
      edit ? versionPath : '$recipePath/versions',
      body,
      version: edit ? row['version'] as int : null,
    );
    if (!edit && result != null && mounted) {
      await open(versionId: '${result['id']}');
    }
  }

  Future<void> requirements(bool packaging) async {
    final rows = c.collection(
      c.record![packaging ? 'packaging_requirements' : 'components'],
    );
    final result = await Navigator.push<List<JsonObject>>(
      context,
      setupRoute<List<JsonObject>>(
        RecipeRequirementsScreen(
          runtime: widget.runtime,
          packaging: packaging,
          initial: rows,
        ),
      ),
    );
    if (result == null || !mounted) return;
    await command(
      'PUT',
      '$versionPath/${packaging ? 'packaging' : 'components'}',
      {packaging ? 'requirements' : 'components': result},
      version: v!['version'] as int,
      confirm: result.isEmpty,
      danger: result.isEmpty,
    );
  }

  Future<JsonObject?> validation() async {
    final result = await c.save(
      method: 'POST',
      route: '$versionPath/validate',
      body: {},
      params: params,
    );
    if (!mounted) return null;
    if (result == null) {
      await showOrderFeedback(
        context,
        title: 'Validation unavailable',
        message: c.error ?? 'Refresh and try again.',
        success: false,
        warning: true,
      );
      return null;
    }
    final issues = c.collection(result['issues']);
    await settingsSheet<void>(
      context,
      Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SettingsHeading(
            result['valid'] == true ? 'Ready for activation' : 'Changes needed',
          ),
          for (final issue in issues) SettingsNote('${issue['message']}'),
          if (result['valid'] == true)
            const Text('The server found no activation issues.'),
          const SizedBox(height: 20),
          SettingsAction('Done', onPressed: () => Navigator.pop(context)),
        ],
      ),
    );
    return result;
  }

  Future<void> lifecycle(String action) async {
    if (action == 'activate') {
      final result = await validation();
      if (result?['valid'] != true || !mounted) return;
    }
    await command(
      'POST',
      '$versionPath/$action',
      {},
      version: v!['version'] as int,
      confirm: true,
      danger: action == 'retire',
    );
  }

  Future<void> scale() async {
    final body = await purchaseForm(context, 'Scale requirements', [
      PurchaseField(
        'output_numerator',
        'Target quantity',
        kind: 'number',
        required: true,
        min: 1,
        initial: v!['expected_output_numerator'],
      ),
      PurchaseField(
        'output_denominator',
        'Divided by (usually 1)',
        kind: 'number',
        required: true,
        min: 1,
        initial: v!['expected_output_denominator'],
      ),
      PurchaseField(
        'output_unit_code',
        'Output unit',
        kind: 'select',
        required: true,
        options: {'${v!['output_unit_code']}': '${v!['output_unit_code']}'},
        initial: v!['output_unit_code'],
      ),
    ]);
    if (body == null || !mounted) return;
    final result = await c.save(
      method: 'POST',
      route: '$versionPath/scale',
      body: body,
      params: params,
    );
    if (!mounted) return;
    if (result == null) {
      await showOrderFeedback(
        context,
        title: 'Could not scale recipe',
        message: c.error ?? 'Try again.',
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
          const SettingsHeading('Scaled requirements'),
          const Text('This preview does not change the saved recipe.'),
          const SizedBox(height: 16),
          for (final r in c.collection(result['requirements']))
            Padding(
              padding: const EdgeInsets.only(bottom: 16),
              child: Text(
                '${dashboardLabel(r['kind'])}: ${r['reference_id']}\n${r['quantity_numerator']} / ${r['quantity_denominator']} ${r['unit_code']}',
              ),
            ),
          SettingsAction('Done', onPressed: () => Navigator.pop(context)),
        ],
      ),
    );
  }

  Widget card(JsonObject row, VoidCallback onTap, {String? label}) => Padding(
    padding: const EdgeInsets.only(bottom: 12),
    child: SettingsCard(
      child: InkWell(
        onTap: onTap,
        child: Row(
          children: [
            Icon(
              Icons.menu_book_outlined,
              color: KarryamColors.tealInk.forInk(context),
            ),
            const SizedBox(width: 16),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    label ?? '${row['name']}',
                    style: Theme.of(context).textTheme.titleMedium,
                  ),
                  const SizedBox(height: 10),
                  SettingsStatus(row['status']),
                ],
              ),
            ),
            const Icon(Icons.chevron_right_rounded),
          ],
        ),
      ),
    ),
  );
  Widget costCard(JsonObject r) => SettingsCard(
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          dashboardLabel(r['calculation_type']),
          style: Theme.of(context).textTheme.titleLarge,
        ),
        const SizedBox(height: 12),
        Text(dashboardLabel(r['completeness'])),
        const SizedBox(height: 20),
        Wrap(
          spacing: 20,
          runSpacing: 20,
          children: [
            for (final entry in {
              'Total': r['total_cost_minor'],
              'Ingredients': r['ingredient_cost_minor'],
              'Packaging': r['packaging_cost_minor'],
              'Waste': r['waste_cost_minor'],
              'Per serving': r['cost_per_serving_minor'],
              'Per unit': r['cost_per_unit_minor'],
            }.entries)
              SizedBox(
                width: 140,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(entry.key),
                    const SizedBox(height: 6),
                    Text(
                      entry.value == null
                          ? 'Unavailable'
                          : dashboardMoney(
                              entry.value,
                              '${r['currency'] ?? 'NGN'}',
                            ),
                      style: Theme.of(context).textTheme.titleMedium,
                    ),
                  ],
                ),
              ),
          ],
        ),
        if (r['calculated_at'] != null) ...[
          const SizedBox(height: 16),
          Text(supportDate(r['calculated_at'])),
        ],
        for (final line in c.collection(r['components'])) ...[
          const Divider(height: 32),
          Text(
            '${dashboardLabel(line['subject_type'])}: ${materialNames[line['subject_ref']] ?? line['subject_ref']}',
          ),
          const SizedBox(height: 6),
          Text(
            line['component_cost_minor'] == null
                ? 'Cost unavailable'
                : dashboardMoney(
                    line['component_cost_minor'],
                    '${line['currency'] ?? r['currency']}',
                  ),
          ),
          if (line['incomplete_reason'] != null)
            Text(dashboardLabel(line['incomplete_reason'])),
          Text(dashboardLabel(line['cost_source'])),
        ],
      ],
    ),
  );
  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      appBar: AppBar(
        title: Text(
          cost
              ? 'Recipe costs'
              : widget.versionId != null
              ? 'Recipe version'
              : 'Recipes',
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
                      const Text('A saved recipe change needs confirmation.'),
                      const SizedBox(height: 16),
                      SettingsAction(
                        'Retry saved change',
                        busy: c.busy,
                        onPressed: c.blocked
                            ? null
                            : () async {
                                final result = await c.save();
                                if (mounted && context.mounted) {
                                  await showOrderFeedback(
                                    context,
                                    title: result == null
                                        ? 'Change needs attention'
                                        : 'Change confirmed',
                                    message:
                                        c.error ??
                                        'The saved request was confirmed.',
                                    success: result != null,
                                    warning: result == null,
                                  );
                                  if (result != null && mounted) await load();
                                }
                              },
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 20),
              ],
              if (cost) ...[
                if (c.record != null) costCard(c.record!),
                for (final r in c.rows) ...[
                  costCard(r),
                  const SizedBox(height: 16),
                ],
                if (!c.stale && c.record == null && c.rows.isEmpty)
                  const SettingsCard(
                    child: Column(
                      children: [
                        Icon(Icons.receipt_long_outlined, size: 56),
                        SizedBox(height: 16),
                        Text('No saved cost snapshots yet'),
                      ],
                    ),
                  ),
                if (widget.view == 'cost-preview') ...[
                  const SizedBox(height: 20),
                  SettingsAction(
                    'Save cost snapshot',
                    busy: c.busy,
                    onPressed: c.writable
                        ? () => command(
                            'POST',
                            '/api/v1/vendor/recipe-cost/versions/{versionId}/snapshots',
                            {},
                            parameters: {'versionId': widget.versionId!},
                            query: {'branch_id': widget.branchId!},
                            confirm: true,
                          )
                        : null,
                  ),
                  const SizedBox(height: 12),
                  SettingsLink(
                    'Latest saved snapshot',
                    Icons.history_rounded,
                    onTap: () =>
                        open(versionId: widget.versionId, view: 'cost-latest'),
                  ),
                ],
              ] else if (widget.recipeId == null) ...[
                SettingsAction(
                  'Create recipe',
                  icon: Icons.add_rounded,
                  busy: discovering || c.busy,
                  onPressed: c.writable && !discovering ? createRecipe : null,
                ),
                const SizedBox(height: 20),
                TextField(
                  decoration: const InputDecoration(
                    hintText: 'Search recipes',
                    prefixIcon: Icon(Icons.search_rounded),
                  ),
                  onChanged: (value) {
                    setState(() => search = value.trim());
                    searchTimer?.cancel();
                    searchTimer = Timer(const Duration(milliseconds: 400), () {
                      c.searchQuery = search;
                      unawaited(load());
                    });
                  },
                ),
                const SizedBox(height: 12),
                Wrap(
                  spacing: 8,
                  children: [
                    for (final s in ['', 'ACTIVE', 'ARCHIVED'])
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
                        Icon(Icons.menu_book_outlined, size: 56),
                        SizedBox(height: 16),
                        Text('Your recipes start here'),
                        SizedBox(height: 8),
                        Text(
                          'Link a catalogue item, then build its draft version.',
                        ),
                      ],
                    ),
                  ),
                for (final r in c.rows)
                  card(
                    r,
                    () => open(
                      recipeId: '${r['id']}',
                      branchId: '${r['branch_id']}',
                    ),
                  ),
              ] else if (widget.versionId == null && c.record != null) ...[
                SettingsHeading('${c.record!['name']}'),
                SettingsStatus(c.record!['status']),
                const SizedBox(height: 20),
                if ('${c.record!['internal_description'] ?? ''}'
                    .isNotEmpty) ...[
                  Text('${c.record!['internal_description']}'),
                  const SizedBox(height: 20),
                ],
                SettingsAction(
                  'Create draft version',
                  busy: c.busy,
                  onPressed: c.writable && c.record!['status'] != 'ARCHIVED'
                      ? () => versionForm()
                      : null,
                ),
                const SizedBox(height: 20),
                for (final row in versions)
                  card(
                    row,
                    () => open(versionId: '${row['id']}'),
                    label: 'Version ${row['version_number']}',
                  ),
                if (versions.any((row) => row['status'] == 'ACTIVE'))
                  SettingsLink(
                    'Current recipe cost',
                    Icons.price_change_outlined,
                    onTap: () => open(
                      branchId: '${c.record!['branch_id']}',
                      view: 'cost-current',
                    ),
                  ),
                SettingsLink(
                  'Cost history',
                  Icons.history_rounded,
                  onTap: () => open(view: 'cost-history'),
                ),
                const SizedBox(height: 20),
                if (versions.any((row) => row['status'] == 'ACTIVE'))
                  const SettingsNote(
                    'Retire the active version before archiving this recipe.',
                    warning: true,
                  ),
                if (c.record!['status'] != 'ARCHIVED')
                  SettingsAction(
                    'Archive recipe',
                    danger: true,
                    onPressed:
                        c.writable &&
                            !versions.any((row) => row['status'] == 'ACTIVE')
                        ? () => command(
                            'POST',
                            '$recipePath/archive',
                            {},
                            version: c.record!['version'] as int,
                            confirm: true,
                            danger: true,
                          )
                        : null,
                  ),
              ] else if (v != null) ...[
                SettingsHeading(
                  'Version ${v!['version_number']}',
                  subtitle: '${recipe?['name'] ?? ''}',
                ),
                SettingsStatus(v!['status']),
                const SizedBox(height: 20),
                SettingsCard(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        'Expected output',
                        style: Theme.of(context).textTheme.titleMedium,
                      ),
                      const SizedBox(height: 12),
                      Text(
                        '${v!['expected_output_numerator']} / ${v!['expected_output_denominator']} ${v!['output_unit_code']}',
                        style: Theme.of(context).textTheme.headlineSmall,
                      ),
                      const SizedBox(height: 12),
                      Text(
                        '${v!['serving_count'] ?? 0} servings - ${((v!['expected_waste_bps'] as num) / 100).toStringAsFixed(2)}% expected waste',
                      ),
                      if ('${v!['internal_notes'] ?? ''}'.isNotEmpty) ...[
                        const SizedBox(height: 12),
                        Text('${v!['internal_notes']}'),
                      ],
                    ],
                  ),
                ),
                const SizedBox(height: 20),
                for (final packaging in [false, true]) ...[
                  SettingsCard(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          packaging ? 'Packaging' : 'Ingredients',
                          style: Theme.of(context).textTheme.titleLarge,
                        ),
                        const SizedBox(height: 16),
                        for (final line in c.collection(
                          c.record![packaging
                              ? 'packaging_requirements'
                              : 'components'],
                        ))
                          Padding(
                            padding: const EdgeInsets.only(bottom: 12),
                            child: Text(
                              '${materialNames[line[packaging ? 'packaging_material_id' : 'ingredient_id']] ?? line[packaging ? 'packaging_material_id' : 'ingredient_id']}\n${line['quantity_numerator']} / ${line['quantity_denominator']} ${line['unit_code']} - ${dashboardLabel(line['normalization_status'])}',
                            ),
                          ),
                        if (v!['status'] == 'DRAFT')
                          SettingsAction(
                            packaging ? 'Edit packaging' : 'Edit ingredients',
                            secondary: true,
                            onPressed:
                                c.writable && recipe?['status'] != 'ARCHIVED'
                                ? () => requirements(packaging)
                                : null,
                          ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 16),
                ],
                if (v!['status'] == 'DRAFT') ...[
                  SettingsAction(
                    'Edit yield and notes',
                    secondary: true,
                    onPressed: c.writable && recipe?['status'] != 'ARCHIVED'
                        ? () => versionForm(edit: true)
                        : null,
                  ),
                  const SizedBox(height: 12),
                  SettingsAction(
                    'Validate recipe',
                    secondary: true,
                    onPressed: c.writable ? validation : null,
                  ),
                  const SizedBox(height: 12),
                  SettingsAction(
                    'Activate version',
                    busy: c.busy,
                    onPressed: c.writable && recipe?['status'] != 'ARCHIVED'
                        ? () => lifecycle('activate')
                        : null,
                  ),
                  const SizedBox(height: 12),
                ],
                if (v!['status'] == 'ACTIVE') ...[
                  SettingsAction(
                    'Retire version',
                    danger: true,
                    onPressed: c.writable ? () => lifecycle('retire') : null,
                  ),
                  const SizedBox(height: 12),
                ],
                if (c.writable)
                  SettingsLink(
                    'Scale requirements',
                    Icons.balance_outlined,
                    onTap: () => scale(),
                  ),
                SettingsLink(
                  'Cost preview',
                  Icons.price_change_outlined,
                  onTap: () => open(
                    versionId: widget.versionId,
                    branchId: '${recipe!['branch_id']}',
                    view: 'cost-preview',
                  ),
                ),
              ],
              if (c.hasMore) ...[
                const SizedBox(height: 16),
                SettingsAction(
                  'Load more',
                  secondary: true,
                  onPressed: c.busy ? null : () => load(more: true),
                ),
              ],
            ],
          ],
        ),
      ),
    ),
  );
}
