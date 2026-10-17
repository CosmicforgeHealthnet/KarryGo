import 'dart:async';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/ui/premium_loading.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../../settings/ui/settings_design.dart';

class RecipeRequirementsScreen extends StatefulWidget {
  const RecipeRequirementsScreen({
    super.key,
    required this.runtime,
    required this.packaging,
    required this.initial,
  });
  final VendorRuntime runtime;
  final bool packaging;
  final List<JsonObject> initial;
  @override
  State<RecipeRequirementsScreen> createState() =>
      _RecipeRequirementsScreenState();
}

class _RecipeRequirementsScreenState extends State<RecipeRequirementsScreen> {
  String get refKey =>
      widget.packaging ? 'packaging_material_id' : 'ingredient_id';
  late final rows = [
    for (final row in widget.initial)
      <String, dynamic>{
        for (final key in [
          refKey,
          'quantity_numerator',
          'quantity_denominator',
          'unit_code',
          'sequence',
          if (!widget.packaging) ...[
            'optional',
            'expected_waste_bps',
            'preparation_note',
          ],
        ])
          if (row[key] != null) key: row[key],
      },
  ];
  List<JsonObject> masters = [];
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
    try {
      final response = await widget.runtime.backend.productionMasterApi.request(
        'GET',
        '/api/v1/vendor/${widget.packaging ? 'packaging-materials' : 'ingredients'}',
        query: {'status': 'ACTIVE', 'limit': '200'},
      );
      final result = (response.data as List? ?? []).map(jsonObject).toList();
      if (result.any(
        (r) => r['business_id'] != widget.runtime.backend.scope.vendorId,
      )) {
        throw const FormatException('Invalid material scope');
      }
      if (mounted) setState(() => masters = result);
    } catch (_) {
      if (mounted) {
        setState(
          () =>
              error = 'Materials could not be loaded. Refresh before editing.',
        );
      }
    } finally {
      if (mounted) setState(() => loading = false);
    }
  }

  Future<void> edit([int? index]) async {
    final r = index == null ? <String, dynamic>{} : rows[index];
    final options = {
      for (final m in masters) '${m['id']}': '${m['name']}',
      for (final existing in rows)
        if (!masters.any((m) => m['id'] == existing[refKey]))
          '${existing[refKey]}': 'Archived material',
    };
    final mock = widget.runtime.mode == VendorDataMode.mock && index == null;
    final body = await purchaseForm(
      context,
      index == null
          ? 'Add ${widget.packaging ? 'packaging' : 'ingredient'}'
          : 'Edit requirement',
      [
        PurchaseField(
          refKey,
          widget.packaging ? 'Packaging material' : 'Ingredient',
          kind: 'select',
          required: true,
          options: options,
          initial:
              r[refKey] ??
              (mock && masters.isNotEmpty ? masters.first['id'] : null),
        ),
        PurchaseField(
          'quantity_numerator',
          'Quantity used',
          kind: 'number',
          required: true,
          min: 1,
          initial:
              r['quantity_numerator'] ??
              (mock ? (widget.packaging ? 1 : 250) : null),
        ),
        PurchaseField(
          'quantity_denominator',
          'Divided by (usually 1)',
          kind: 'number',
          required: true,
          min: 1,
          initial: r['quantity_denominator'] ?? 1,
        ),
        PurchaseField(
          'unit_code',
          "Unit (leave empty to use the material's own unit)",
          initial:
              r['unit_code'] ??
              (mock && masters.isNotEmpty
                  ? masters.first['default_unit_code']
                  : null),
        ),
        if (!widget.packaging) ...[
          PurchaseField(
            'expected_waste_bps',
            'Expected waste (%)',
            kind: 'percent',
            required: true,
            max: 9999,
            initial: r['expected_waste_bps'] ?? 0,
          ),
          PurchaseField(
            'optional',
            'Optional ingredient',
            kind: 'bool',
            initial: r['optional'],
          ),
          PurchaseField(
            'preparation_note',
            'Preparation note',
            kind: 'notes',
            initial: r['preparation_note'],
          ),
        ],
      ],
      validate: (body) {
        if (rows.indexed.any(
          (entry) => entry.$1 != index && entry.$2[refKey] == body[refKey],
        )) {
          return 'Each material can appear only once.';
        }
        if ('${body['unit_code'] ?? ''}'.length > 80 ||
            '${body['preparation_note'] ?? ''}'.length > 1000) {
          return 'Shorten the unit code or preparation note.';
        }
        return null;
      },
    );
    if (body == null || !mounted) return;
    body.removeWhere((key, value) => value == null);
    final unit = '${body['unit_code'] ?? ''}'.trim();
    body['unit_code'] = (unit.isEmpty
            ? '${masters.where((m) => m['id'] == body[refKey]).firstOrNull?['default_unit_code'] ?? ''}'
            : unit)
        .toUpperCase();
    body['sequence'] = r['sequence'] ?? rows.length;
    setState(() {
      if (index == null) {
        rows.add(body);
      } else {
        rows[index] = body;
      }
    });
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: Text(
        widget.packaging ? 'Packaging requirements' : 'Recipe ingredients',
      ),
      actions: [
        IconButton(
          tooltip: 'Refresh materials',
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
            if (error != null) SettingsNote(error!),
            if (masters.isEmpty && error == null)
              const SettingsNote(
                'Create active production materials first. Existing requirements can still be reviewed.',
                warning: true,
              ),
            for (final entry in rows.indexed) ...[
              SettingsCard(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      masters
                              .where((m) => m['id'] == entry.$2[refKey])
                              .firstOrNull?['name']
                              ?.toString() ??
                          '${entry.$2[refKey]}',
                      style: Theme.of(context).textTheme.titleLarge,
                    ),
                    const SizedBox(height: 12),
                    Text(
                      '${entry.$2['quantity_numerator']} / ${entry.$2['quantity_denominator']} ${entry.$2['unit_code']}',
                    ),
                    if (!widget.packaging) ...[
                      const SizedBox(height: 8),
                      Text(
                        entry.$2['optional'] == true
                            ? 'Optional ingredient'
                            : 'Required ingredient',
                      ),
                    ],
                    const SizedBox(height: 16),
                    Row(
                      children: [
                        Expanded(
                          child: OutlinedButton.icon(
                            onPressed: error == null
                                ? () => edit(entry.$1)
                                : null,
                            icon: const Icon(Icons.edit_outlined),
                            label: const Text('Edit'),
                          ),
                        ),
                        const SizedBox(width: 12),
                        IconButton(
                          tooltip: 'Remove requirement',
                          onPressed: () =>
                              setState(() => rows.removeAt(entry.$1)),
                          icon: const Icon(Icons.delete_outline_rounded),
                        ),
                      ],
                    ),
                  ],
                ),
              ),
              const SizedBox(height: 12),
            ],
            if (rows.isEmpty)
              SettingsCard(
                child: Column(
                  children: [
                    Icon(
                      widget.packaging
                          ? Icons.takeout_dining_outlined
                          : Icons.eco_outlined,
                      size: 56,
                    ),
                    const SizedBox(height: 16),
                    const Text('Add the materials this recipe needs.'),
                  ],
                ),
              ),
            const SizedBox(height: 20),
            SettingsAction(
              'Add ${widget.packaging ? 'packaging' : 'ingredient'}',
              icon: Icons.add_rounded,
              onPressed:
                  error == null &&
                      masters.isNotEmpty &&
                      rows.length < (widget.packaging ? 100 : 500)
                  ? () => edit()
                  : null,
            ),
          ],
        ],
      ),
    ),
    bottomNavigationBar: SafeArea(
      minimum: const EdgeInsets.all(20),
      child: SettingsAction(
        'Review requirements',
        onPressed: loading || error != null
            ? null
            : () => Navigator.pop(context, rows),
      ),
    ),
  );
}
