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
import '../../settings/state/settings_controller.dart';
import '../../settings/ui/settings_design.dart';
import '../state/production_materials_controller.dart';

const materialStandardUnits = {
  'G',
  'GRAM',
  'GRAMS',
  'KG',
  'KILOGRAM',
  'KILOGRAMS',
  'ML',
  'MILLILITRE',
  'MILLILITRES',
  'MILLILITER',
  'MILLILITERS',
  'L',
  'LITRE',
  'LITRES',
  'LITER',
  'LITERS',
  'EA',
  'EACH',
  'COUNT',
  'UNIT',
  'UNITS',
};
String? validateMaterialConversion(JsonObject body, JsonObject material) {
  final from = '${body['from_unit_code'] ?? ''}'.trim().toUpperCase();
  final to = '${body['to_unit_code'] ?? ''}'.trim().toUpperCase();
  if (from.isEmpty ||
      from == to ||
      to != '${material['default_unit_code']}'.toUpperCase()) {
    return 'Use a different source unit and convert into the material default unit.';
  }
  if (body['measurement_family'] != material['measurement_family']) {
    return 'The measurement family must match this material.';
  }
  if (body['conversion_mode'] == 'EXACT_AUTHORITATIVE' &&
      (!materialStandardUnits.contains(from) ||
          !materialStandardUnits.contains(to)) &&
      '${body['source_context'] ?? ''}'.trim().isEmpty) {
    return 'Provide a material-specific source for a non-standard exact conversion.';
  }
  if ('${body['source_context'] ?? ''}'.length > 1000) {
    return 'Keep the source context within 1,000 characters.';
  }
  return null;
}

class MaterialBranchesScreen extends StatefulWidget {
  const MaterialBranchesScreen({
    super.key,
    required this.runtime,
    required this.ingredient,
    required this.material,
  });
  final VendorRuntime runtime;
  final bool ingredient;
  final JsonObject material;
  @override
  State<MaterialBranchesScreen> createState() => _MaterialBranchesScreenState();
}

class _MaterialBranchesScreenState extends State<MaterialBranchesScreen> {
  List<JsonObject> branches = [];
  bool loading = true;
  String? error;
  @override
  void initState() {
    super.initState();
    unawaited(load());
  }

  Future<void> load() async {
    setState(() {
      loading = true;
      error = null;
    });
    final app = widget.runtime.backend;
    final reader = SettingsController(app, SettingsKind.branches);
    try {
      final result = await reader.all(
        '${SettingsController.business}/branches',
        {'businessId': app.scope.vendorId!},
      );
      if (result.any(
        (r) =>
            r['business_id'] != null && r['business_id'] != app.scope.vendorId,
      )) {
        throw const FormatException('Invalid branch scope');
      }
      if (mounted) {
        setState(
          () => branches = result
              .where((r) => r['status'] != 'ARCHIVED')
              .toList(),
        );
      }
    } catch (_) {
      if (mounted) {
        setState(
          () => error = 'Branches could not be loaded. Refresh to try again.',
        );
      }
    } finally {
      reader.dispose();
      if (mounted) setState(() => loading = false);
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: const Text('Branch configuration'),
      actions: [
        IconButton(
          tooltip: 'Refresh',
          onPressed: loading ? null : load,
          icon: const Icon(Icons.refresh_rounded),
        ),
      ],
    ),
    body: SafeArea(
      child: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          if (loading)
            const PremiumSkeleton(detail: true)
          else ...[
            SettingsHeading(
              '${widget.material['name']}',
              subtitle:
                  'Choose a branch to configure production use and stock tracking.',
            ),
            if (error != null) SettingsNote(error!),
            if (error == null && branches.isEmpty)
              const SettingsCard(
                child: Column(
                  children: [
                    Icon(Icons.storefront_outlined, size: 56),
                    SizedBox(height: 16),
                    Text('No available branches'),
                  ],
                ),
              ),
            for (final branch in branches) ...[
              SettingsLink(
                '${branch['display_name'] ?? branch['name'] ?? 'Branch'}',
                Icons.storefront_outlined,
                onTap: () => Navigator.push(
                  context,
                  setupRoute<void>(
                    MaterialSetupScreen(
                      runtime: widget.runtime,
                      ingredient: widget.ingredient,
                      material: widget.material,
                      branch: branch,
                    ),
                  ),
                ),
              ),
              const SizedBox(height: 12),
            ],
          ],
        ],
      ),
    ),
  );
}

class MaterialSetupScreen extends StatefulWidget {
  const MaterialSetupScreen({
    super.key,
    required this.runtime,
    required this.ingredient,
    required this.material,
    this.branch,
  });
  final VendorRuntime runtime;
  final bool ingredient;
  final JsonObject material;
  final JsonObject? branch;
  @override
  State<MaterialSetupScreen> createState() => _MaterialSetupScreenState();
}

class _MaterialSetupScreenState extends State<MaterialSetupScreen> {
  bool get branchMode => widget.branch != null;
  String get idKey => widget.ingredient ? 'ingredientId' : 'packagingId';
  String get base =>
      '/api/v1/vendor/${widget.ingredient ? 'ingredients' : 'packaging-materials'}/{$idKey}';
  String get path =>
      branchMode ? '$base/branches/{branchId}' : '$base/conversions';
  Map<String, String> get params => {
    idKey: '${widget.material['id']}',
    if (branchMode) 'branchId': '${widget.branch!['id']}',
  };
  late final c = ProductionMaterialsController(
    widget.runtime,
    path,
    parameters: params,
    branchId: branchMode ? '${widget.branch!['id']}' : null,
    allowMissing: branchMode,
  );
  JsonObject? material;
  String? parentError, referenceNotice;
  bool preparing = true;
  @override
  void initState() {
    super.initState();
    unawaited(load());
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> load() async {
    setState(() {
      preparing = true;
      parentError = null;
    });
    try {
      final r = jsonObject(await c.fetch(base, params: params));
      c.validate(r);
      if (r['id'] != widget.material['id']) {
        throw const FormatException('Material mismatch');
      }
      if (mounted) setState(() => material = r);
      await c.load();
      if (!branchMode &&
          c.rows.any((r) => r['owner_id'] != widget.material['id'])) {
        throw const FormatException('Conversion owner mismatch');
      }
      if (branchMode &&
          c.record != null &&
          c.record!['owner_id'] != widget.material['id']) {
        throw const FormatException('Mapping owner mismatch');
      }
    } catch (_) {
      c.stale = true;
      c.rows = [];
      c.record = null;
      if (mounted) {
        setState(
          () => parentError =
              'This material setup could not be loaded. Refresh before editing.',
        );
      }
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  bool get editable =>
      !preparing &&
      parentError == null &&
      material?['status'] != 'ARCHIVED' &&
      c.writable;
  Future<void> feedback(JsonObject? result) async {
    if (!mounted) return;
    await showOrderFeedback(
      context,
      title: result == null ? 'Change needs attention' : 'Setup saved',
      message: c.error ?? 'Your production setup is up to date.',
      success: result != null,
      warning: result == null,
    );
    if (mounted && result != null) await load();
  }

  Future<void> conversion([JsonObject? row]) async {
    final r = row ?? <String, dynamic>{};
    final m = material!;
    final mock = widget.runtime.mode == VendorDataMode.mock && row == null;
    final unit = '${m['default_unit_code']}';
    final body = await purchaseForm(
      context,
      row == null ? 'Add conversion' : 'Edit conversion',
      [
        PurchaseField(
          'measurement_family',
          'Measurement family',
          kind: 'select',
          required: true,
          options: {
            '${m['measurement_family']}': dashboardLabel(
              m['measurement_family'],
            ),
          },
          initial: m['measurement_family'],
        ),
        PurchaseField(
          'from_unit_code',
          'From unit',
          required: true,
          initial:
              r['from_unit_code'] ??
              (mock
                  ? (m['measurement_family'] == 'MASS' && unit == 'G'
                        ? 'KG'
                        : 'PACK')
                  : null),
        ),
        PurchaseField(
          'to_unit_code',
          'To default unit',
          kind: 'select',
          required: true,
          options: {unit: unit},
          initial: unit,
        ),
        PurchaseField(
          'conversion_mode',
          'Conversion use',
          kind: 'select',
          required: true,
          options: purchaseEnums(['EXACT_AUTHORITATIVE', 'DISPLAY_ONLY']),
          initial: r['conversion_mode'] ?? 'DISPLAY_ONLY',
        ),
        PurchaseField(
          'numerator',
          'Ratio numerator',
          kind: 'number',
          required: true,
          min: 1,
          max: 1000000000000,
          initial: r['numerator'] ?? (mock ? (unit == 'G' ? 1000 : 10) : null),
        ),
        PurchaseField(
          'denominator',
          'Ratio denominator',
          kind: 'number',
          required: true,
          min: 1,
          max: 1000000000000,
          initial: r['denominator'] ?? (mock ? 1 : null),
        ),
        PurchaseField(
          'source_context',
          'Material-specific source',
          kind: 'notes',
          initial:
              r['source_context'] ??
              (mock ? 'Supplier specification for this material.' : null),
        ),
        PurchaseField(
          'status',
          'Status',
          kind: 'select',
          required: true,
          options: purchaseEnums(['ACTIVE', 'INACTIVE', 'ARCHIVED']),
          initial: r['status'] ?? 'ACTIVE',
        ),
      ],
      validate: (body) => validateMaterialConversion(body, m),
    );
    if (body == null || !mounted) return;
    body.removeWhere((key, value) => value == null);
    body['from_unit_code'] = '${body['from_unit_code']}'.trim().toUpperCase();
    if (body['status'] == 'ARCHIVED' &&
        !await confirmSettings(
          context,
          'Archive conversion?',
          'This conversion will no longer be active.',
          danger: true,
        )) {
      return;
    }
    if (!mounted) return;
    await feedback(
      await c.save(
        method: row == null ? 'POST' : 'PUT',
        route: row == null ? path : '$path/{conversionId}',
        body: body,
        params: {...params, if (row != null) 'conversionId': '${row['id']}'},
        headerVersion: row?['version'] as int?,
      ),
    );
  }

  Future<Map<String, String>> inventoryChoices() async {
    final app = widget.runtime.backend;
    final choices = <String, String>{};
    int offset = 0;
    while (true) {
      final response = await app.inventoryApi.request(
        'GET',
        '/api/v1/vendor/inventory/branches/{branchId}/balances',
        pathParameters: {'branchId': '${widget.branch!['id']}'},
        query: {'limit': '200', 'offset': '$offset'},
      );
      final rows = SettingsController.objects(response.data);
      for (final row in rows) {
        if (row['business_id'] != null &&
                row['business_id'] != app.scope.vendorId ||
            row['branch_id'] != null &&
                row['branch_id'] != widget.branch!['id']) {
          throw const FormatException('Inventory scope mismatch');
        }
        if (row['inventory_item_id'] is String) {
          choices[row['inventory_item_id']] =
              '${row['name'] ?? ''}'.isNotEmpty
              ? '${row['name']} (${row['on_hand'] ?? 0} on hand)'
              : 'Stock item ${choices.length + 1} (${row['on_hand'] ?? 0} on hand)';
        }
      }
      offset += rows.length;
      if (rows.length < 200) return choices;
    }
  }

  bool referencesLoading = false;
  Future<void> configure() async {
    setState(() {
      referencesLoading = true;
      referenceNotice = null;
    });
    final r = c.record ?? <String, dynamic>{};
    Map<String, String> choices = {};
    try {
      choices = await inventoryChoices();
    } catch (_) {
      referenceNotice =
          'Inventory references could not be loaded. You can keep the saved mapping or use untracked stock.';
    }
    if (!mounted) return;
    setState(() => referencesLoading = false);
    if ('${r['inventory_item_id'] ?? ''}'.isNotEmpty) {
      choices.putIfAbsent(
        '${r['inventory_item_id']}',
        () => 'Current stock item',
      );
    }
    final mock = widget.runtime.mode == VendorDataMode.mock;
    final body = await purchaseForm(
      context,
      'Configure ${widget.branch!['display_name'] ?? widget.branch!['name'] ?? 'branch'}',
      [
        PurchaseField(
          'enabled',
          'Enabled at this branch',
          kind: 'bool',
          initial: r['enabled'] ?? mock,
        ),
        PurchaseField(
          'production_eligible',
          'Eligible for production',
          kind: 'bool',
          initial: r['production_eligible'] ?? mock,
        ),
        PurchaseField(
          'inventory_control_mode_override',
          'Branch stock control',
          kind: 'select',
          options: {
            '': 'Use material setting (${dashboardLabel(material!['inventory_control_mode'])})',
            ...purchaseEnums(['UNTRACKED', 'INVENTORY_CONTROLLED']),
          },
          initial: r['inventory_control_mode_override'] ?? '',
        ),
        PurchaseField(
          'inventory_item_id',
          'Stock item (leave empty to create one automatically)',
          kind: 'select',
          options: choices,
          initial: r['inventory_item_id'],
        ),
      ],
      validate: (body) {
        final mode = (body['inventory_control_mode_override'] ?? '') == ''
            ? material!['inventory_control_mode']
            : body['inventory_control_mode_override'];
        final item = '${body['inventory_item_id'] ?? ''}';
        if (mode == 'UNTRACKED' && item.isNotEmpty) {
          return 'Remove the inventory mapping when using untracked stock.';
        }
        return null;
      },
    );
    if (body == null || !mounted) return;
    body['inventory_item_id'] ??= '';
    body['inventory_control_mode_override'] ??= '';
    await feedback(
      await c.save(method: 'PUT', route: path, body: body, params: params),
    );
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      appBar: AppBar(
        title: Text(branchMode ? 'Branch setup' : 'Unit conversions'),
        actions: [
          IconButton(
            tooltip: 'Refresh',
            onPressed: preparing || c.busy || referencesLoading ? null : load,
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
              SettingsHeading(
                '${material?['name'] ?? widget.material['name']}',
                subtitle: branchMode
                    ? '${widget.branch!['display_name'] ?? widget.branch!['name'] ?? 'Branch'}'
                    : '1 source unit = numerator / denominator default units.',
              ),
              if (parentError != null) SettingsNote(parentError!),
              if (c.error != null) SettingsNote(c.error!),
              if (referenceNotice != null)
                SettingsNote(referenceNotice!, warning: true),
              if (material?['status'] == 'ARCHIVED')
                const SettingsNote(
                  'This material is archived. Its setup is available to view.',
                  warning: true,
                ),
              if (c.pending != null) ...[
                SettingsCard(
                  child: Column(
                    children: [
                      const Text(
                        'A saved change needs confirmation before another update.',
                      ),
                      const SizedBox(height: 16),
                      SettingsAction(
                        'Retry saved change',
                        busy: c.busy,
                        onPressed: c.blocked
                            ? null
                            : () async => feedback(await c.save()),
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 20),
              ],
              if (!branchMode) ...[
                SettingsAction(
                  'Add conversion',
                  icon: Icons.add_rounded,
                  busy: c.busy,
                  onPressed: editable && c.allowed('POST', path)
                      ? () => conversion()
                      : null,
                ),
                const SizedBox(height: 20),
                if (!c.stale && c.rows.isEmpty)
                  const SettingsCard(
                    child: Column(
                      children: [
                        Icon(Icons.swap_horiz_rounded, size: 56),
                        SizedBox(height: 16),
                        Text('No unit conversions yet'),
                        SizedBox(height: 8),
                        Text(
                          'Add a source unit and its ratio to the default unit.',
                        ),
                      ],
                    ),
                  ),
                for (final r in c.rows) ...[
                  SettingsCard(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Row(
                          children: [
                            Icon(
                              Icons.swap_horiz_rounded,
                              color: KarryamColors.tealInk.forInk(context),
                            ),
                            const SizedBox(width: 12),
                            Expanded(
                              child: Text(
                                '${r['from_unit_code']} to ${r['to_unit_code']}',
                                style: Theme.of(context).textTheme.titleLarge,
                              ),
                            ),
                            SettingsStatus(r['status']),
                          ],
                        ),
                        const SizedBox(height: 16),
                        Text(
                          '1 ${r['from_unit_code']} = ${r['numerator']} / ${r['denominator']} ${r['to_unit_code']}',
                        ),
                        const SizedBox(height: 10),
                        Text(dashboardLabel(r['conversion_mode'])),
                        if ('${r['source_context'] ?? ''}'.isNotEmpty) ...[
                          const SizedBox(height: 8),
                          Text('${r['source_context']}'),
                        ],
                        const SizedBox(height: 20),
                        SettingsAction(
                          'Edit conversion',
                          secondary: true,
                          onPressed:
                              editable &&
                                  c.allowed('PUT', '$path/{conversionId}')
                              ? () => conversion(r)
                              : null,
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 16),
                ],
              ] else ...[
                SettingsCard(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Icon(
                        Icons.hub_outlined,
                        size: 48,
                        color: KarryamColors.tealInk.forInk(context),
                      ),
                      const SizedBox(height: 20),
                      if (c.record == null)
                        const Text('No saved configuration for this branch.')
                      else ...[
                        Wrap(
                          spacing: 20,
                          runSpacing: 20,
                          children: [
                            for (final entry in {
                              'Availability': c.record!['enabled'] == true
                                  ? 'Enabled'
                                  : 'Disabled',
                              'Production use':
                                  c.record!['production_eligible'] == true
                                  ? 'Eligible'
                                  : 'Not eligible',
                              'Effective stock control': dashboardLabel(
                                c.record!['effective_inventory_control_mode'],
                              ),
                              'Inventory mapping': dashboardLabel(
                                c.record!['inventory_mapping_status'],
                              ),
                            }.entries)
                              SizedBox(
                                width: 140,
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    Text(
                                      entry.key,
                                      style: Theme.of(
                                        context,
                                      ).textTheme.bodySmall,
                                    ),
                                    const SizedBox(height: 6),
                                    Text(
                                      entry.value,
                                      style: Theme.of(
                                        context,
                                      ).textTheme.titleSmall,
                                    ),
                                  ],
                                ),
                              ),
                          ],
                        ),
                        if ('${c.record!['inventory_item_id'] ?? ''}'
                            .isNotEmpty) ...[
                          const SizedBox(height: 20),
                          const Text('Inventory item'),
                          const SizedBox(height: 6),
                          SelectableText('${c.record!['inventory_item_id']}'),
                        ],
                      ],
                    ],
                  ),
                ),
                const SizedBox(height: 20),
                SettingsAction(
                  c.record == null ? 'Configure branch' : 'Edit branch setup',
                  busy: referencesLoading || c.busy,
                  onPressed:
                      editable && !referencesLoading && c.allowed('PUT', path)
                      ? configure
                      : null,
                ),
              ],
            ],
          ],
        ),
      ),
    ),
  );
}
