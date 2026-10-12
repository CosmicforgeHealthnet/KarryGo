import '../../../core/ui/premium_loading.dart';
import 'dart:async';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../settings/ui/settings_design.dart';
import '../state/advanced_controller.dart';
import '../state/advanced_resource.dart';
import '../state/advanced_vocabulary.dart';
import '../state/catalogue_resource.dart';
import 'advanced_fields.dart';
import 'catalogue_editor.dart';

IconData advancedIcon(AdvancedKind kind) => switch (kind) {
  AdvancedKind.schedules => Icons.calendar_month_outlined,
  AdvancedKind.availability => Icons.storefront_outlined,
  AdvancedKind.source => Icons.account_tree_outlined,
  AdvancedKind.override => Icons.tune_rounded,
  AdvancedKind.prep => Icons.timer_outlined,
  AdvancedKind.logistics => Icons.takeout_dining_outlined,
  AdvancedKind.effective => Icons.fact_check_outlined,
  AdvancedKind.measures => Icons.straighten_rounded,
  AdvancedKind.aliases => Icons.translate_rounded,
  AdvancedKind.calibrations => Icons.balance_rounded,
  AdvancedKind.sellUnits => Icons.pie_chart_outline_rounded,
  AdvancedKind.packaging => Icons.all_inbox_outlined,
  AdvancedKind.fresh => Icons.eco_outlined,
  AdvancedKind.identifiers => Icons.barcode_reader,
  AdvancedKind.quantityBreaks => Icons.stacked_line_chart_rounded,
  AdvancedKind.listingUnits => Icons.local_offer_outlined,
  AdvancedKind.models => Icons.store_mall_directory_outlined,
  _ => Icons.history_rounded,
};
Color advancedAccent(AdvancedKind kind) => switch (kind) {
  AdvancedKind.schedules || AdvancedKind.prep => const Color(0xFF326AA2),
  AdvancedKind.packaging || AdvancedKind.logistics => const Color(0xFFA66028),
  AdvancedKind.calibrations || AdvancedKind.measures => const Color(0xFF7660A6),
  _ => KarryamColors.tealInk,
};

class AdvancedCatalogueHub extends StatelessWidget {
  const AdvancedCatalogueHub({
    super.key,
    required this.runtime,
    required this.kind,
    required this.record,
    required this.currency,
    this.branchId,
    this.writable = true,
  });
  final VendorRuntime runtime;
  final CatalogueKind kind;
  final JsonObject record;
  final String currency;
  final String? branchId;
  final bool writable;
  List<AdvancedResource> get resources {
    final id = record['id'].toString();
    final kinds = switch (kind) {
      CatalogueKind.menus => [AdvancedKind.schedules],
      CatalogueKind.items => [
        AdvancedKind.availability,
        AdvancedKind.effective,
        AdvancedKind.prep,
        AdvancedKind.logistics,
        AdvancedKind.override,
        AdvancedKind.source,
        AdvancedKind.foodHistory,
      ],
      CatalogueKind.products => [
        AdvancedKind.sellUnits,
        AdvancedKind.calibrations,
        AdvancedKind.packaging,
        AdvancedKind.fresh,
        AdvancedKind.measures,
      ],
      CatalogueKind.shopVariants => [
        AdvancedKind.identifiers,
        AdvancedKind.shopHistory,
      ],
      CatalogueKind.listings => [
        AdvancedKind.listingUnits,
        AdvancedKind.quantityBreaks,
        AdvancedKind.shopHistory,
      ],
      _ => [],
    };
    return [
      for (final k in kinds)
        AdvancedResource(
          k,
          parent: record,
          writable: writable,
          parameters: switch (k) {
            AdvancedKind.schedules => {'menuId': id},
            AdvancedKind.availability ||
            AdvancedKind.effective ||
            AdvancedKind.prep ||
            AdvancedKind.logistics ||
            AdvancedKind.override ||
            AdvancedKind.source ||
            AdvancedKind.foodHistory => {'itemId': id},
            AdvancedKind.sellUnits ||
            AdvancedKind.calibrations ||
            AdvancedKind.packaging ||
            AdvancedKind.fresh => {'productId': id},
            AdvancedKind.identifiers => {'variantId': id},
            AdvancedKind.listingUnits ||
            AdvancedKind.quantityBreaks => {'listingId': id},
            _ => {},
          },
        ),
    ];
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Catalogue setup')),
    body: ListView(
      padding: const EdgeInsets.all(24),
      children: [
        SettingsHeading(record['name']?.toString() ?? 'Branch listing'),
        for (final r in resources)
          SettingsLink(
            r.title,
            advancedIcon(r.kind),
            color: (advancedAccent(r.kind)).forInk(context),
            subtitle: r.description,
            onTap: () => Navigator.push(
              context,
              setupRoute<void>(
                AdvancedCatalogueScreen(
                  runtime: runtime,
                  resource: r,
                  currency: currency,
                ),
              ),
            ),
          ),
      ],
    ),
  );
}

class AdvancedCatalogueScreen extends StatefulWidget {
  const AdvancedCatalogueScreen({
    super.key,
    required this.runtime,
    required this.resource,
    required this.currency,
  });
  final VendorRuntime runtime;
  final AdvancedResource resource;
  final String currency;
  @override
  State<AdvancedCatalogueScreen> createState() =>
      _AdvancedCatalogueScreenState();
}

class _AdvancedCatalogueScreenState extends State<AdvancedCatalogueScreen> {
  late final c = AdvancedController(widget.runtime.backend, widget.resource);
  AdvancedResource get r => widget.resource;
  bool opening = false;
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) unawaited(c.load());
    });
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<Map<String, Map<String, String>>> choices() async {
    final result = <String, Map<String, String>>{};
    final api = widget.runtime.backend.shopCatalogueApi;
    final product =
        r.parameters['productId'] ?? r.parent['product_ref']?.toString();
    if ({
      AdvancedKind.sellUnits,
      AdvancedKind.calibrations,
      AdvancedKind.packaging,
    }.contains(r.kind)) {
      final variants = await c.all(
        api,
        '/api/v1/vendor/shop/products/{productId}/variants',
        parameters: {'productId': product!},
      );
      final names = {
        for (final v in variants.where((v) => v['status'] != 'ARCHIVED'))
          v['id'].toString(): v['name'].toString(),
      };
      for (final field in [
        'variant_id',
        'parent_variant_id',
        'child_variant_id',
      ]) {
        result[field] = names;
      }
    }
    if ({AdvancedKind.sellUnits, AdvancedKind.calibrations}.contains(r.kind)) {
      final measures = await c.all(api, '/api/v1/vendor/shop/measurements');
      result['measure_id'] = {
        for (final m in measures.where((m) => m['status'] != 'ARCHIVED'))
          m['id'].toString(): m['display_name'].toString(),
      };
    }
    if (r.kind == AdvancedKind.calibrations) {
      final branches = await c.all(
        widget.runtime.backend.businessApi,
        '/api/v1/vendor/businesses/{businessId}/branches',
        parameters: {'businessId': widget.runtime.backend.scope.vendorId!},
      );
      result['branch_id'] = {
        for (final b in branches.where((b) => b['status'] != 'CLOSED'))
          b['id'].toString(): b['display_name'].toString(),
      };
    }
    if ({AdvancedKind.sellUnits, AdvancedKind.listingUnits}.contains(r.kind)) {
      final calibrations = await c.all(
        api,
        '/api/v1/vendor/shop/products/{productId}/measure-calibrations',
        parameters: {'productId': product!},
      );
      final measures = await c.all(api, '/api/v1/vendor/shop/measurements');
      final names = {
        for (final m in measures)
          m['id'].toString(): m['display_name'].toString(),
      };
      result['calibration_id'] = {
        for (final row in calibrations.where(
          (v) =>
              v['status'] != 'ARCHIVED' &&
              (r.kind == AdvancedKind.sellUnits
                  ? v['branch_ref'] == null
                  : v['branch_ref'] == null ||
                        v['branch_ref'] == r.parent['branch_ref']),
        ))
          row['id'].toString():
              '${names[row['measure_ref']] ?? row['measure_ref']} / ${row['approximate_weight_grams'] != null ? '${row['approximate_weight_grams']} g' : '${row['approximate_volume_ml']} ml'}',
      };
    }
    if (r.kind == AdvancedKind.quantityBreaks) {
      final detail = (await api.request(
        'GET',
        '/api/v1/vendor/shop/listings/{listingId}',
        pathParameters: r.parameters,
      )).object;
      final enabled = AdvancedController.objects(detail, 'sell_units')
          .where((u) => u['enabled'] == true)
          .map((u) => u['sell_unit_ref'])
          .toSet();
      final units = await c.all(
        api,
        '/api/v1/vendor/shop/products/{productId}/sell-units',
        parameters: {'productId': product!},
      );
      result['sell_unit_id'] = {
        for (final u in units.where(
          (u) => enabled.contains(u['id']) && u['status'] == 'ACTIVE',
        ))
          u['id'].toString(): u['display_name'].toString(),
      };
    }
    return result;
  }

  Future<void> editor([JsonObject? row]) async {
    if (opening || c.busy || c.loading || c.stale) return;
    setState(() => opening = true);
    try {
      final current = row == null ? null : await c.fresh(row);
      final configured =
          current != null &&
          (r.kind != AdvancedKind.prep && r.kind != AdvancedKind.logistics ||
              (c.metadata['configured'] == true));
      final record = configured
          ? normalizeAdvancedRecord(r.kind, current)
          : null;
      String? freshType = record?['attribute_type']?.toString();
      if (r.kind == AdvancedKind.fresh && freshType == null) {
        final types =
            catalogueVocabularyMap(
              'classAttributes',
            )[r.parent['product_class']] ??
            [];
        final available = types
            .where((t) => !c.rows.any((row) => row['attribute_type'] == t))
            .toList();
        if (!mounted) return;
        freshType = await settingsSheet<String>(
          context,
          Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const SettingsHeading('Choose an attribute'),
              for (final t in available)
                SettingsLink(
                  dashboardLabel(t),
                  Icons.eco_outlined,
                  onTap: () => Navigator.pop(context, t),
                ),
              if (available.isEmpty)
                const Text('All supported attributes are configured.'),
            ],
          ),
        );
        if (freshType == null) return;
      }
      final options = await choices();
      final eligible = r.kind == AdvancedKind.models
          ? (c.metadata['eligible_capabilities'] as List? ?? []).cast<String>()
          : null;
      final presets = widget.runtime.mode == VendorDataMode.mock
          ? advancedPresets(r, freshType: freshType)
          : <String, dynamic>{};
      if (r.kind == AdvancedKind.fresh) presets['attribute_type'] = freshType;
      if (r.kind == AdvancedKind.models &&
          eligible?.isNotEmpty == true &&
          record == null &&
          widget.runtime.mode == VendorDataMode.mock) {
        presets['models'] = [eligible!.first];
        presets['primary_model'] = eligible.first;
      }
      if (!mounted) return;
      final method = row == null && !r.singleton ? r.saveMethod : r.editMethod,
          path = row == null && !r.singleton ? r.path : r.editPath;
      final fields = advancedFields(
        r,
        record: record,
        freshType: freshType,
        eligibleModels: eligible,
      );
      final form = CatalogueEditor(
        runtime: widget.runtime,
        resource: CatalogueResource(
          r.food ? CatalogueKind.menus : CatalogueKind.products,
        ),
        currency: widget.currency,
        record: record,
        entityLabel: r.singular,
        helper: r.description,
        fieldsOverride: fields,
        presets: presets,
        choices: options,
        sheet: r.smallEditor,
        partialUpdate: !r.singleton || r.kind == AdvancedKind.override
            ? r.kind != AdvancedKind.fresh
            : false,
        transform: (body) => advancedPayload(r, body, record),
        validate: (body) => validateAdvanced(r, body, record),
        canSave: () => c.allowed(method, path),
        errorMessage: () => c.feedback ?? c.error,
        onSave: (body) => c.mutate(method, path, {
          ...body,
          if (current != null || r.singleton || r.kind == AdvancedKind.fresh)
            'expected_version': current?['version'] ?? 0,
        }, parameters: r.params(current)),
      );
      final saved = r.smallEditor
          ? await showModalBottomSheet<bool>(
              context: context,
              isScrollControlled: true,
              useSafeArea: true,
              showDragHandle: true,
              backgroundColor: KarryamColors.canvas.forSurface(context),
              builder: (_) => form,
            )
          : await Navigator.push<bool>(context, setupRoute<bool>(form));
      if (saved == true && mounted) {
        await showOrderFeedback(
          context,
          title: '${r.title} saved',
          message: c.feedback ?? 'Your catalogue settings are updated.',
          success: true,
        );
      }
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Could not open this editor',
          message: AdvancedController.message(e),
        );
      }
    } finally {
      if (mounted) setState(() => opening = false);
    }
  }

  Future<void> remove(JsonObject row) async {
    if (opening) return;
    setState(() => opening = true);
    try {
      final current = await c.fresh(row);
      if (!mounted) return;
      final title = r.singleton
          ? 'Clear ${r.title.toLowerCase()}'
          : r.kind == AdvancedKind.aliases
          ? 'Remove measure name'
          : 'Archive ${r.singular}';
      if (!await confirmSettings(
            context,
            title,
            r.kind == AdvancedKind.models
                ? 'This deactivates the branch listings governed by this configuration.'
                : r.singleton
                ? 'Remove this configuration and restore the default settings.'
                : 'This retires the record from active use.',
            danger: true,
          ) ||
          !mounted) {
        return;
      }
      final ok = await c.mutate(
        r.removeMethod,
        r.removePath,
        r.kind == AdvancedKind.aliases
            ? {}
            : {'expected_version': current['version']},
        parameters: r.params(current),
      );
      if (mounted) {
        await showOrderFeedback(
          context,
          title: ok
              ? r.singleton
                    ? 'Configuration cleared'
                    : r.kind == AdvancedKind.aliases
                    ? 'Name removed'
                    : 'Record archived'
              : 'Change not accepted',
          message: ok
              ? c.feedback ?? 'Your catalogue is updated.'
              : c.feedback ?? c.error ?? 'Try again.',
          success: ok,
          warning: ok,
        );
      }
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Could not update this record',
          message: AdvancedController.message(e),
        );
      }
    } finally {
      if (mounted) setState(() => opening = false);
    }
  }

  Future<void> selectUnits() async {
    if (opening || !c.allowed('PATCH', r.path)) return;
    setState(() => opening = true);
    try {
      final data = (await c.get(r.path, parameters: r.parameters)).object;
      final listing = jsonObject(data['listing']);
      final all = await c.all(
        widget.runtime.backend.shopCatalogueApi,
        '/api/v1/vendor/shop/products/{productId}/sell-units',
        parameters: {'productId': listing['product_ref'].toString()},
      );
      final options = all
          .where(
            (u) =>
                u['variant_ref'] == listing['variant_ref'] &&
                u['status'] == 'ACTIVE' &&
                (u['retail_enabled'] == true &&
                        listing['retail_enabled'] == true ||
                    u['wholesale_enabled'] == true &&
                        listing['wholesale_enabled'] == true),
          )
          .toList();
      final selected = AdvancedController.objects(data, 'sell_units')
          .where((u) => u['enabled'] == true)
          .map((u) => u['sell_unit_ref'].toString())
          .toSet();
      if (!mounted) return;
      final values = await settingsSheet<List<String>>(
        context,
        StatefulBuilder(
          builder: (context, setLocal) => Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const SettingsHeading('Choose sell units'),
              if (options.isEmpty)
                const Text(
                  'Create an active sell unit for this variant and sales channel first.',
                ),
              for (final u in options)
                Padding(
                  padding: const EdgeInsets.only(bottom: 12),
                  child: SettingsCard(
                    child: CheckboxListTile(
                      contentPadding: EdgeInsets.zero,
                      title: Text(u['display_name'].toString()),
                      subtitle: Text(dashboardLabel(u['measurement_mode'])),
                      activeColor: KarryamColors.tealInk.forInk(context),
                      value: selected.contains(u['id']),
                      onChanged: (v) => setLocal(() {
                        v == true
                            ? selected.add(u['id'].toString())
                            : selected.remove(u['id']);
                      }),
                    ),
                  ),
                ),
            ],
          ),
        ),
        footer: SettingsAction(
          'Update sell units',
          onPressed: () => Navigator.pop(context, selected.toList()),
        ),
      );
      if (values == null || !mounted) return;
      final ok = await c.mutate('PATCH', r.path, {
        'sell_unit_ids': values,
        'expected_version': listing['version'],
      }, parameters: r.parameters);
      if (mounted) {
        await showOrderFeedback(
          context,
          title: ok ? 'Branch sell units updated' : 'Change not accepted',
          message: ok
              ? c.feedback ?? 'The branch selection is saved.'
              : c.feedback ?? c.error ?? 'Try again.',
          success: ok,
        );
      }
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Could not load selling units',
          message: AdvancedController.message(e),
        );
      }
    } finally {
      if (mounted) setState(() => opening = false);
    }
  }

  Future<void> aliases(JsonObject row) async {
    await Navigator.push(
      context,
      setupRoute<void>(
        AdvancedCatalogueScreen(
          runtime: widget.runtime,
          currency: widget.currency,
          resource: AdvancedResource(
            AdvancedKind.aliases,
            parameters: {'measureId': row['id'].toString()},
            parent: row,
            writable: r.writable,
          ),
        ),
      ),
    );
    await c.load();
  }

  String name(JsonObject row) => switch (r.kind) {
    AdvancedKind.schedules => dashboardLabel(row['label']),
    AdvancedKind.measures ||
    AdvancedKind.sellUnits => row['display_name'].toString(),
    AdvancedKind.aliases => row['alias'].toString(),
    AdvancedKind.fresh => dashboardLabel(row['attribute_type']),
    AdvancedKind.identifiers => row['identifier_value'].toString(),
    AdvancedKind.quantityBreaks =>
      '${row['min_quantity']} ${row['max_quantity'] == null ? 'and above' : '- ${row['max_quantity']}'}',
    AdvancedKind.calibrations =>
      row['approximate_weight_grams'] != null
          ? '${row['approximate_weight_grams']} g'
          : '${row['approximate_volume_ml']} ml',
    AdvancedKind.packaging => '${row['units_per_parent']} inner units',
    AdvancedKind.listingUnits => c.label(
      row['sell_unit_ref'],
      'Branch sell unit',
    ),
    AdvancedKind.foodHistory || AdvancedKind.shopHistory => dashboardLabel(
      row['change_reason'] ?? row['reason'] ?? row['subject_type'],
    ),
    _ => r.title,
  };
  Widget facts(JsonObject row) {
    final entries = <String, Object?>{};
    switch (r.kind) {
      case AdvancedKind.schedules:
        entries.addAll({
          'Days': (row['days_of_week'] as List? ?? [])
              .map(
                (d) => [
                  'Mon',
                  'Tue',
                  'Wed',
                  'Thu',
                  'Fri',
                  'Sat',
                  'Sun',
                ][(d as int) - 1],
              )
              .join(', '),
          'Time':
              '${row['start_local_time']} - ${row['end_local_time']}${row['overnight'] == true ? ' / next day' : ''}',
          'First date': row['effective_start_date'],
          'Last date': row['effective_end_date'],
        });
      case AdvancedKind.calibrations:
        entries.addAll({
          'Measure': c.label(row['measure_ref'], 'Not set'),
          'Branch': row['branch_ref'] ?? 'Business-wide',
          'Variant': c.label(row['variant_ref'], 'All product variants'),
          'Tolerance': '${row['tolerance_bps']} basis points',
          'Source': dashboardLabel(row['calibration_source']),
        });
      case AdvancedKind.listingUnits:
        entries.addAll({
          'Sell unit': c.label(row['sell_unit_ref'], 'Not set'),
          'Enabled': row['enabled'] == true ? 'Yes' : 'No',
          'Retail override': row['price_override_minor'] == null
              ? 'Inherited'
              : dashboardMoney(row['price_override_minor'], widget.currency),
          'Wholesale override': row['wholesale_price_override_minor'] == null
              ? 'Inherited'
              : dashboardMoney(
                  row['wholesale_price_override_minor'],
                  widget.currency,
                ),
          'Calibration': row['calibration_ref'] ?? 'Inherited',
        });
      case AdvancedKind.packaging:
        entries.addAll({
          'Outer pack': c.label(row['parent_variant_ref'], 'Not set'),
          'Inner pack': c.label(row['child_variant_ref'], 'Not set'),
        });
      case AdvancedKind.fresh:
        entries.addAll({
          'Selection': row['selection_required'] == true
              ? 'Required'
              : 'Optional',
          'Choices': (row['options'] as List? ?? [])
              .map((o) => jsonObject(o)['display_name'])
              .join(', '),
        });
      case AdvancedKind.sellUnits:
        entries.addAll({
          'Measurement': dashboardLabel(row['measurement_mode']),
          'Conversion': dashboardLabel(row['conversion_type']),
          'Retail': row['price_minor'] == null
              ? 'Not set'
              : dashboardMoney(row['price_minor'], widget.currency),
          'Wholesale': row['wholesale_price_minor'] == null
              ? 'Not set'
              : dashboardMoney(row['wholesale_price_minor'], widget.currency),
        });
      case AdvancedKind.identifiers:
        entries.addAll({
          'Type': dashboardLabel(row['identifier_type']),
          'Package': row['package_level'],
          'External verification': row['externally_verified'] == true
              ? 'Verified'
              : 'Not verified',
        });
      case AdvancedKind.quantityBreaks:
        entries.addAll({
          'Unit price': dashboardMoney(
            row['unit_price_minor'],
            widget.currency,
          ),
          'Wholesale only': row['wholesale_only'] == true ? 'Yes' : 'No',
          'Lead time': row['lead_time_days'] == null
              ? 'Not set'
              : '${row['lead_time_days']} days',
        });
      case AdvancedKind.measures:
        entries.addAll({
          'Family': dashboardLabel(row['measurement_family']),
          'Scope': dashboardLabel(row['scope']),
          'Conversion': row['converts_exactly'] == true
              ? 'Exact / ${row['base_quantity']} ${row['base_unit']}'
              : 'Product calibration required',
        });
      case AdvancedKind.foodHistory:
      case AdvancedKind.shopHistory:
        for (final e in row.entries.where(
          (e) => !{
            'id',
            'business_id',
            'version',
            'subject_ref',
            'product_ref',
            'variant_ref',
            'sell_unit_ref',
            'listing_ref',
          }.contains(e.key),
        )) {
          entries[dashboardLabel(e.key)] = e.key.contains('minor')
              ? dashboardMoney(e.value, widget.currency)
              : e.value;
        }
      default:
        for (final f in advancedFields(
          r,
          record: normalizeAdvancedRecord(r.kind, row),
          freshType: row['attribute_type']?.toString(),
        )) {
          final v = row[f.source ?? f.key];
          entries[f.label] = f.key.contains('price') && v != null
              ? dashboardMoney(v, widget.currency)
              : v is bool
              ? (v ? 'Yes' : 'No')
              : v is List
              ? v
                    .map(
                      (x) => x is Map
                          ? x['code'] ?? x.toString()
                          : dashboardLabel(x),
                    )
                    .join(', ')
              : v;
        }
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        for (final e in entries.entries)
          if (e.value != null && e.value.toString().isNotEmpty)
            SettingsFact(e.key, e.value),
      ],
    );
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: Text(r.title)),
    bottomNavigationBar:
        r.singleton || r.readOnly || r.kind == AdvancedKind.listingUnits
        ? null
        : AnimatedBuilder(
            animation: c,
            builder: (context, _) => SafeArea(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(24, 12, 24, 16),
                child: SettingsAction(
                  'Add ${r.singular}',
                  icon: Icons.add_rounded,
                  busy: c.busy,
                  onPressed: !opening && c.allowed(r.saveMethod, r.path)
                      ? () => editor()
                      : null,
                ),
              ),
            ),
          ),
    body: AnimatedBuilder(
      animation: c,
      builder: (context, _) => RefreshIndicator(
        onRefresh: c.load,
        child: ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: const EdgeInsets.all(24),
          children: [
            SettingsCard(
              color: (advancedAccent(
                r.kind,
              ).withValues(alpha: .07)).forInk(context),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  _CatalogueGlyph(kind: r.kind),
                  const SizedBox(height: 22),
                  SettingsHeading(r.title, subtitle: r.description),
                  if (r.parent['name'] != null)
                    Text(
                      r.parent['name'].toString(),
                      style: const TextStyle(fontWeight: FontWeight.w700),
                    ),
                  if (r.kind == AdvancedKind.schedules &&
                      c.metadata['timezone'] != null)
                    SettingsFact('Menu timezone', c.metadata['timezone']),
                ],
              ),
            ),
            const SizedBox(height: 20),
            if (r.kind == AdvancedKind.listingUnits)
              SettingsLink(
                'Choose sell units',
                Icons.checklist_rounded,
                onTap: !opening && c.allowed('PATCH', r.path)
                    ? selectUnits
                    : null,
              ),
            if (c.loading)
              c.rows.isEmpty
                  ? const PremiumSkeleton(rows: 2)
                  : const ContentRefresh(),
            if (c.error != null) ...[
              SettingsNote(
                c.feedback ?? c.error!,
                warning: c.restricted || c.stale,
              ),
              SettingsAction(
                'Refresh',
                secondary: true,
                icon: Icons.refresh_rounded,
                onPressed: c.loading || c.busy || opening ? null : c.load,
              ),
              const SizedBox(height: 18),
            ],
            if (c.api
                .operation('GET', r.path)
                .parametersIn('query')
                .any((q) => q['name'] == 'include_archived'))
              Padding(
                padding: const EdgeInsets.only(bottom: 18),
                child: Align(
                  alignment: Alignment.centerRight,
                  child: ArchiveFilter(
                    selected: c.archived,
                    onChanged: opening || c.loading || c.busy
                        ? null
                        : (v) {
                            c.archived = v;
                            c.load();
                          },
                  ),
                ),
              ),
            if (!c.loading && c.error == null && c.rows.isEmpty)
              DashboardEmptyState(
                story: EmptyStory.setup,
                title: r.readOnly
                    ? 'No recorded changes.'
                    : 'No ${r.title.toLowerCase()} yet.',
                message: r.kind == AdvancedKind.listingUnits
                    ? 'Create product sell units to make them available on this listing.'
                    : r.readOnly
                    ? 'Your price changes will appear here.'
                    : 'Use the action below to configure your catalogue.',
              ),
            if (r.singleton && c.rows.isNotEmpty) ...[
              if ((r.kind == AdvancedKind.prep ||
                      r.kind == AdvancedKind.logistics) &&
                  c.metadata['configured'] != true)
                const SettingsNote('Not configured.', warning: true),
              if (r.kind == AdvancedKind.source &&
                  c.rows.first['mode'] == 'INVENTORY_LINKED')
                const SettingsNote(
                  'Inventory-linked items rely on the inventory authority. Check Selling status for the actual result.',
                  warning: true,
                ),
              SettingsCard(
                child: r.kind == AdvancedKind.effective
                    ? _EffectiveFacts(c.rows.first, currency: widget.currency)
                    : facts(c.rows.first),
              ),
              const SizedBox(height: 18),
              if (r.canEdit)
                SettingsAction(
                  c.metadata['configured'] == false ||
                          c.rows.first['version'] == 0
                      ? 'Configure'
                      : 'Edit settings',
                  icon: Icons.tune_rounded,
                  busy: c.busy,
                  onPressed: !opening && c.allowed(r.editMethod, r.editPath)
                      ? () => editor(c.rows.first)
                      : null,
                ),
              if (r.canRemove &&
                  (c.rows.first['version'] as int? ?? 0) > 0) ...[
                const SizedBox(height: 12),
                SettingsAction(
                  r.kind == AdvancedKind.models
                      ? 'Archive selling models'
                      : 'Clear configuration',
                  danger: true,
                  secondary: true,
                  onPressed: !opening && c.allowed(r.removeMethod, r.removePath)
                      ? () => remove(c.rows.first)
                      : null,
                ),
              ],
              if (r.kind == AdvancedKind.prep && c.metadata['effective'] is Map)
                Padding(
                  padding: const EdgeInsets.only(top: 18),
                  child: _EffectiveFacts(
                    jsonObject(c.metadata['effective']),
                    currency: widget.currency,
                  ),
                ),
            ] else
              for (final row in c.rows)
                Padding(
                  padding: const EdgeInsets.only(bottom: 16),
                  child: SettingsCard(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Row(
                          children: [
                            Icon(
                              advancedIcon(r.kind),
                              color: (advancedAccent(r.kind)).forInk(context),
                              size: 28,
                            ),
                            const Spacer(),
                            if (row['status']?.toString().isNotEmpty == true)
                              Flexible(child: SettingsStatus(row['status'])),
                          ],
                        ),
                        const SizedBox(height: 18),
                        Text(
                          name(row),
                          style: const TextStyle(
                            fontSize: 23,
                            fontWeight: FontWeight.w700,
                            letterSpacing: -.6,
                          ),
                        ),
                        const SizedBox(height: 8),
                        facts(row),
                        if (r.kind == AdvancedKind.measures)
                          SettingsLink(
                            'Manage names',
                            Icons.translate_rounded,
                            onTap: !opening && !c.loading
                                ? () => aliases(row)
                                : null,
                          ),
                        if (r.canEdit &&
                            (row['status'] != 'ARCHIVED' ||
                                r.kind == AdvancedKind.fresh) &&
                            !(r.kind == AdvancedKind.measures &&
                                row['scope'] == 'PLATFORM')) ...[
                          const SizedBox(height: 12),
                          SettingsAction(
                            row['status'] == 'ARCHIVED'
                                ? 'Reconfigure'
                                : 'Edit ${r.singular}',
                            secondary: true,
                            icon: Icons.edit_outlined,
                            onPressed:
                                !opening && c.allowed(r.editMethod, r.editPath)
                                ? () => editor(row)
                                : null,
                          ),
                        ],
                        if (r.canRemove &&
                            row['status'] != 'ARCHIVED' &&
                            !(r.kind == AdvancedKind.measures &&
                                row['scope'] == 'PLATFORM') &&
                            !(r.kind == AdvancedKind.aliases &&
                                row['scope'] == 'PLATFORM')) ...[
                          const SizedBox(height: 10),
                          SettingsAction(
                            r.kind == AdvancedKind.aliases
                                ? 'Remove name'
                                : 'Archive ${r.singular}',
                            secondary: true,
                            danger: true,
                            icon: Icons.archive_outlined,
                            onPressed:
                                !opening &&
                                    c.allowed(r.removeMethod, r.removePath)
                                ? () => remove(row)
                                : null,
                          ),
                        ],
                      ],
                    ),
                  ),
                ),
            if (c.hasMore)
              SettingsAction(
                'Load more',
                secondary: true,
                busy: c.loading,
                onPressed: opening || c.loading
                    ? null
                    : () => c.load(more: true),
              ),
          ],
        ),
      ),
    ),
  );
}

class _EffectiveFacts extends StatelessWidget {
  const _EffectiveFacts(this.data, {required this.currency});
  final JsonObject data;
  final String currency;
  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      for (final e in data.entries.where(
        (e) => !{
          'id',
          'item_id',
          'branch_id',
          'business_id',
          'version',
        }.contains(e.key),
      ))
        if (e.value is Map)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 12),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  dashboardLabel(e.key),
                  style: const TextStyle(fontWeight: FontWeight.w700),
                ),
                _EffectiveFacts(jsonObject(e.value), currency: currency),
              ],
            ),
          )
        else
          SettingsFact(
            dashboardLabel(e.key),
            e.value is bool
                ? (e.value == true ? 'Yes' : 'No')
                : e.key.contains('minor')
                ? dashboardMoney(e.value, currency)
                : e.value is List
                ? (e.value as List).map(dashboardLabel).join(', ')
                : e.value == null
                ? 'Not configured'
                : dashboardLabel(e.value),
          ),
    ],
  );
}

class _CatalogueGlyph extends StatelessWidget {
  const _CatalogueGlyph({required this.kind});
  final AdvancedKind kind;
  @override
  Widget build(BuildContext context) => ExcludeSemantics(
    child: SizedBox(
      width: 94,
      height: 82,
      child: Stack(
        children: [
          Positioned(
            left: 2,
            top: 4,
            child: Container(
              width: 72,
              height: 72,
              decoration: BoxDecoration(
                color: Colors.white.forSurface(context),
                borderRadius: BorderRadius.circular(24),
              ),
              child: Icon(
                advancedIcon(kind),
                size: 36,
                color: (advancedAccent(kind)).forInk(context),
              ),
            ),
          ),
          Positioned(
            right: 0,
            top: 0,
            child: Container(
              padding: const EdgeInsets.all(8),
              decoration: BoxDecoration(
                color: (advancedAccent(
                  kind,
                ).withValues(alpha: .13)).forSurface(context),
                shape: BoxShape.circle,
              ),
              child: Icon(
                kind == AdvancedKind.packaging
                    ? Icons.inventory_2_outlined
                    : kind == AdvancedKind.schedules
                    ? Icons.schedule_rounded
                    : kind == AdvancedKind.fresh
                    ? Icons.check_rounded
                    : Icons.add_rounded,
                color: (advancedAccent(kind)).forInk(context),
                size: 16,
              ),
            ),
          ),
        ],
      ),
    ),
  );
}
