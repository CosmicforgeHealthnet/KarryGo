import 'material_setup_screen.dart';
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
import '../state/production_materials_controller.dart';

class ProductionMaterialsHub extends StatelessWidget {
  const ProductionMaterialsHub({super.key, required this.runtime});
  final VendorRuntime runtime;
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Production materials')),
    body: SafeArea(
      child: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          const SettingsHeading('Your production essentials'),
          for (final ingredient in [true, false]) ...[
            SettingsCard(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Icon(
                    ingredient
                        ? Icons.eco_outlined
                        : Icons.takeout_dining_outlined,
                    size: 42,
                    color:
                        (ingredient
                                ? const Color(0xFF3B8B70)
                                : const Color(0xFFB17D43))
                            .forInk(context),
                  ),
                  const SizedBox(height: 20),
                  Text(
                    ingredient ? 'Ingredients' : 'Packaging',
                    style: Theme.of(context).textTheme.headlineSmall,
                  ),
                  const SizedBox(height: 8),
                  Text(
                    ingredient
                        ? 'Organise what goes into every recipe.'
                        : 'Manage the materials that complete each order.',
                  ),
                  const SizedBox(height: 24),
                  SettingsAction(
                    ingredient ? 'Manage ingredients' : 'Manage packaging',
                    onPressed: () => Navigator.push(
                      context,
                      setupRoute<void>(
                        ProductionMaterialsScreen(
                          runtime: runtime,
                          ingredient: ingredient,
                        ),
                      ),
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 16),
          ],
        ],
      ),
    ),
  );
}

class ProductionMaterialsScreen extends StatefulWidget {
  const ProductionMaterialsScreen({
    super.key,
    required this.runtime,
    required this.ingredient,
    this.id,
  });
  final VendorRuntime runtime;
  final bool ingredient;
  final String? id;
  @override
  State<ProductionMaterialsScreen> createState() =>
      _ProductionMaterialsScreenState();
}

class _ProductionMaterialsScreenState extends State<ProductionMaterialsScreen> {
  String get kind => widget.ingredient ? 'ingredients' : 'packaging-materials';
  String get noun => widget.ingredient ? 'ingredient' : 'packaging material';
  String get idKey => widget.ingredient ? 'ingredientId' : 'packagingId';
  String get base => '/api/v1/vendor/$kind';
  Map<String, String> get params => {if (widget.id != null) idKey: widget.id!};
  late final c = ProductionMaterialsController(
    widget.runtime,
    widget.id == null ? base : '$base/{$idKey}',
    parameters: params,
  );
  String status = '', search = '';
  Timer? searchTimer;
  @override
  void initState() {
    super.initState();
    unawaited(c.load());
  }

  @override
  void dispose() {
    searchTimer?.cancel();
    c.dispose();
    super.dispose();
  }

  Future<void> open(JsonObject r) async {
    await Navigator.push(
      context,
      setupRoute<void>(
        ProductionMaterialsScreen(
          runtime: widget.runtime,
          ingredient: widget.ingredient,
          id: '${r['id']}',
        ),
      ),
    );
    if (mounted) await c.load(status: status);
  }

  Future<void> feedback(JsonObject? result) async {
    if (!mounted) return;
    await showOrderFeedback(
      context,
      title: result == null ? 'Change needs attention' : 'Changes saved',
      message: c.error ?? 'Your production material is up to date.',
      success: result != null,
      warning: result == null,
    );
    if (mounted && result != null) await c.load(status: status);
  }

  Future<void> edit() async {
    final r = c.record ?? <String, dynamic>{};
    final mock =
        widget.runtime.mode == VendorDataMode.mock && widget.id == null;
    PurchaseField field(
      String key,
      String label, {
      String type = 'text',
      bool required = false,
      Map<String, String> options = const {},
      Object? fallback,
    }) => PurchaseField(
      key,
      label,
      kind: type,
      required: required,
      options: options,
      initial: r[key] ?? fallback,
    );
    final body = await purchaseForm(
      context,
      widget.id == null ? 'Add $noun' : 'Edit $noun',
      [
        field(
          'code',
          'Code',
          required: true,
          fallback: mock
              ? 'MATERIAL-${DateTime.now().millisecondsSinceEpoch}'
              : null,
        ),
        field(
          'name',
          'Name',
          required: true,
          fallback: mock
              ? (widget.ingredient ? 'Brown rice' : 'Food container')
              : null,
        ),
        field('description', 'Description', type: 'notes'),
        field(
          'measurement_family',
          'Measurement family',
          type: 'select',
          required: true,
          options: purchaseEnums(['MASS', 'VOLUME', 'COUNT']),
          fallback: mock ? (widget.ingredient ? 'MASS' : 'COUNT') : null,
        ),
        field(
          'default_unit_code',
          'Default unit code',
          required: true,
          fallback: mock ? (widget.ingredient ? 'G' : 'UNIT') : null,
        ),
        field(
          'inventory_control_mode',
          'Stock control',
          type: 'select',
          required: true,
          options: purchaseEnums(['UNTRACKED', 'INVENTORY_CONTROLLED']),
          fallback: mock ? 'UNTRACKED' : null,
        ),
        if (widget.ingredient) ...[
          field('perishable', 'Perishable', type: 'bool'),
          field('requires_expiry', 'Requires expiry tracking', type: 'bool'),
        ],
        field(
          'status',
          'Status',
          type: 'select',
          required: true,
          options: purchaseEnums(['DRAFT', 'ACTIVE', 'INACTIVE']),
          fallback: 'ACTIVE',
        ),
      ],
      validate: (body) {
        if (body['requires_expiry'] == true && body['perishable'] != true) {
          return 'Enable perishable before requiring expiry tracking.';
        }
        if ('${body['code']}'.length > 80 ||
            '${body['name']}'.length > 200 ||
            '${body['default_unit_code']}'.length > 80 ||
            '${body['description'] ?? ''}'.length > 4000) {
          return 'Shorten the code, name, unit or description to fit the supported limits.';
        }
        return null;
      },
    );
    if (body == null || !mounted) return;
    body.removeWhere((key, value) => value == null);
    final result = await c.save(
      method: widget.id == null ? 'POST' : 'PUT',
      route: widget.id == null ? base : '$base/{$idKey}',
      body: body,
      params: params,
      headerVersion: widget.id == null ? null : r['version'] as int,
    );
    await feedback(result);
    if (mounted && result != null && widget.id == null) await open(result);
  }

  Future<void> archive() async {
    if (!await confirmSettings(
          context,
          'Archive $noun?',
          'Archived materials cannot be used for new production setup.',
          danger: true,
        ) ||
        !mounted) {
      return;
    }
    await feedback(
      await c.save(
        method: 'POST',
        route: '$base/{$idKey}/archive',
        body: {},
        params: params,
        headerVersion: c.record!['version'] as int,
      ),
    );
  }

  Widget icon(BuildContext context, {double size = 28}) => Icon(
    widget.ingredient ? Icons.eco_outlined : Icons.takeout_dining_outlined,
    size: size,
    color:
        (widget.ingredient ? const Color(0xFF3B8B70) : const Color(0xFFB17D43))
            .forInk(context),
  );
  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      appBar: AppBar(
        title: Text(
          widget.id == null
              ? (widget.ingredient ? 'Ingredients' : 'Packaging')
              : 'Material details',
        ),
        actions: [
          IconButton(
            tooltip: 'Refresh',
            onPressed: c.loading || c.busy
                ? null
                : () => c.load(status: status),
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(20),
          children: [
            if (c.loading)
              const PremiumSkeleton(detail: true)
            else ...[
              if (c.error != null) SettingsNote(c.error!),
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
                const SizedBox(height: 16),
              ],
              if (widget.id == null) ...[
                SettingsAction(
                  'Add $noun',
                  icon: Icons.add_rounded,
                  busy: c.busy,
                  onPressed: c.allowed('POST', base) ? edit : null,
                ),
                const SizedBox(height: 20),
                TextField(
                  decoration: InputDecoration(
                    hintText:
                        'Search ${widget.ingredient ? 'ingredients' : 'packaging'}',
                    prefixIcon: const Icon(Icons.search_rounded),
                  ),
                  onChanged: (v) {
                    setState(() => search = v.toLowerCase().trim());
                    searchTimer?.cancel();
                    searchTimer = Timer(const Duration(milliseconds: 400), () {
                      c.searchQuery = search;
                      unawaited(c.load(status: status));
                    });
                  },
                ),
                const SizedBox(height: 12),
                Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  children: [
                    for (final s in [
                      '',
                      'ACTIVE',
                      'DRAFT',
                      'INACTIVE',
                      'ARCHIVED',
                    ])
                      ChoiceChip(
                        label: Text(s.isEmpty ? 'All' : dashboardLabel(s)),
                        selected: status == s,
                        onSelected: c.busy || c.pending != null
                            ? null
                            : (_) {
                                setState(() => status = s);
                                unawaited(c.load(status: s));
                              },
                      ),
                  ],
                ),
                const SizedBox(height: 20),
                if (!c.stale &&
                    c.rows
                        .where(
                          (r) => '${r['name']} ${r['code']}'
                              .toLowerCase()
                              .contains(search),
                        )
                        .isEmpty)
                  SettingsCard(
                    child: Column(
                      children: [
                        icon(context, size: 64),
                        const SizedBox(height: 20),
                        Text(
                          search.isEmpty
                              ? 'No ${widget.ingredient ? 'ingredients' : 'packaging'} here yet'
                              : 'No matching materials',
                          style: Theme.of(context).textTheme.titleLarge,
                        ),
                        const SizedBox(height: 8),
                        const Text('Add a material or adjust your filters.'),
                      ],
                    ),
                  ),
                for (final r in c.rows.where(
                  (r) => '${r['name']} ${r['code']}'.toLowerCase().contains(
                    search,
                  ),
                )) ...[
                  SettingsCard(
                    child: InkWell(
                      borderRadius: BorderRadius.circular(20),
                      onTap: () => open(r),
                      child: Row(
                        children: [
                          icon(context),
                          const SizedBox(width: 16),
                          Expanded(
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Text(
                                  '${r['name']}',
                                  style: Theme.of(
                                    context,
                                  ).textTheme.titleMedium,
                                ),
                                const SizedBox(height: 6),
                                Text(
                                  '${r['code']} / ${r['default_unit_code']}',
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
                if (c.rows.length == 200)
                  const SettingsNote(
                    'Showing the first 200 materials. Search or use a status filter to narrow the list.',
                    warning: true,
                  ),
              ] else if (c.record case final r?) ...[
                SettingsCard(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      icon(context, size: 48),
                      const SizedBox(height: 18),
                      Text(
                        '${r['name']}',
                        style: Theme.of(context).textTheme.headlineMedium,
                      ),
                      const SizedBox(height: 10),
                      SettingsStatus(r['status']),
                      if ('${r['description'] ?? ''}'.isNotEmpty) ...[
                        const SizedBox(height: 16),
                        Text('${r['description']}'),
                      ],
                      const SizedBox(height: 24),
                      Wrap(
                        spacing: 24,
                        runSpacing: 20,
                        children: [
                          for (final entry in {
                            'Code': r['code'],
                            'Measurement': dashboardLabel(
                              r['measurement_family'],
                            ),
                            'Unit': r['default_unit_code'],
                            'Stock control': dashboardLabel(
                              r['inventory_control_mode'],
                            ),
                            if (widget.ingredient) ...{
                              'Perishable': r['perishable'] == true
                                  ? 'Yes'
                                  : 'No',
                              'Expiry tracking': r['requires_expiry'] == true
                                  ? 'Required'
                                  : 'Not required',
                            },
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
                                    '${entry.value}',
                                    style: Theme.of(
                                      context,
                                    ).textTheme.titleSmall,
                                  ),
                                ],
                              ),
                            ),
                        ],
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 20),
                SettingsLink(
                  'Unit conversions',
                  Icons.swap_horiz_rounded,
                  subtitle:
                      'Define how purchase units convert to the default unit.',
                  onTap: () => Navigator.push(
                    context,
                    setupRoute<void>(
                      MaterialSetupScreen(
                        runtime: widget.runtime,
                        ingredient: widget.ingredient,
                        material: r,
                      ),
                    ),
                  ),
                ),
                const SizedBox(height: 12),
                SettingsLink(
                  'Branch configuration',
                  Icons.hub_outlined,
                  subtitle: 'Set availability and production stock mapping.',
                  onTap: () => Navigator.push(
                    context,
                    setupRoute<void>(
                      MaterialBranchesScreen(
                        runtime: widget.runtime,
                        ingredient: widget.ingredient,
                        material: r,
                      ),
                    ),
                  ),
                ),
                const SizedBox(height: 20),
                if (r['status'] != 'ARCHIVED') ...[
                  SettingsAction(
                    'Edit details',
                    busy: c.busy,
                    onPressed: c.allowed('PUT', '$base/{$idKey}') ? edit : null,
                  ),
                  const SizedBox(height: 12),
                  SettingsAction(
                    'Archive $noun',
                    danger: true,
                    onPressed: c.allowed('POST', '$base/{$idKey}/archive')
                        ? archive
                        : null,
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
