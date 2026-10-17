import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'pastry_configuration_screen.dart';
import '../../settings/ui/settings_verification_screen.dart';
import 'pastry_workflows_screen.dart';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../crm/ui/crm_design.dart' show crmLabel;
import '../../crm/ui/crm_editors.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../production/ui/production_form_screen.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../../settings/state/settings_controller.dart';
import '../../settings/ui/settings_design.dart';
import '../../support/ui/order_support_screen.dart' show supportDate;
import '../state/pastry_workspace_controller.dart';

enum PastryArea { products, groups, requests }

extension PastryAreaInfo on PastryArea {
  String get title => switch (this) {
    PastryArea.products => 'Pastry products',
    PastryArea.groups => 'Customisation options',
    PastryArea.requests => 'Custom requests',
  };
  String get collection => switch (this) {
    PastryArea.products => 'products',
    PastryArea.groups => 'option-groups',
    PastryArea.requests => 'custom-requests',
  };
  String get idKey => switch (this) {
    PastryArea.products => 'productId',
    PastryArea.groups => 'groupId',
    PastryArea.requests => 'requestId',
  };
  IconData get icon => switch (this) {
    PastryArea.products => Icons.bakery_dining_outlined,
    PastryArea.groups => Icons.palette_outlined,
    PastryArea.requests => Icons.cake_outlined,
  };
}

class PastryHub extends StatelessWidget {
  const PastryHub({super.key, required this.runtime});
  final VendorRuntime runtime;
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Pastry & bakery')),
    body: SafeArea(
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 680),
          child: ListView(
            padding: const EdgeInsets.all(20),
            children: [
              const Text(
                'Made for the occasion',
                style: TextStyle(fontSize: 28, fontWeight: FontWeight.w800),
              ),
              const SizedBox(height: 8),
              const Text(
                'Your bakery range, custom designs and customer requests.',
              ),
              const SizedBox(height: 24),
              for (final area in PastryArea.values)
                SettingsCard(
                  child: SettingsLink(
                    area.title,
                    area.icon,
                    color:
                        (area == PastryArea.products
                                ? const Color(0xFFB97C35)
                                : area == PastryArea.groups
                                ? const Color(0xFF8B65B9)
                                : KarryamColors.tealInk)
                            .forInk(context),
                    subtitle: switch (area) {
                      PastryArea.products =>
                        'Prices, preparation and delivery handling',
                      PastryArea.groups =>
                        'Reusable flavours, finishes and extras',
                      PastryArea.requests =>
                        'Specifications and request history',
                    },
                    onTap: () => Navigator.push(
                      context,
                      setupRoute<void>(
                        PastryWorkspaceScreen(runtime: runtime, area: area),
                      ),
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

class PastryWorkspaceScreen extends StatefulWidget {
  const PastryWorkspaceScreen({
    super.key,
    required this.runtime,
    required this.area,
    this.id,
  });
  final VendorRuntime runtime;
  final PastryArea area;
  final String? id;
  @override
  State<PastryWorkspaceScreen> createState() => _PastryWorkspaceScreenState();
}

class _PastryWorkspaceScreenState extends State<PastryWorkspaceScreen> {
  PastryArea get area => widget.area;
  String get base => '$pastryBase/${area.collection}';
  String get detail => '$base/{${area.idKey}}';
  Map<String, String> get params => {
    if (widget.id != null) area.idKey: widget.id!,
  };
  late final c = PastryWorkspaceController(
    widget.runtime,
    widget.id == null ? base : detail,
    parameters: params,
  );
  List<JsonObject> branches = [], children = [], attached = [];
  JsonObject? changeEligibility, cancelEligibility;
  bool reading = false, preparing = false, evidenceReady = false;
  String? error, action, buttonAction;
  String filter = 'CURRENT';
  bool get mock => widget.runtime.mode == VendorDataMode.mock;
  bool get writable => c.writable && evidenceReady && !reading && !preparing;
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

  Future<void> load() async {
    if (reading || c.busy) return;
    setState(() {
      reading = true;
      evidenceReady = false;
      error = null;
    });
    try {
      final settings = SettingsController(c.app, SettingsKind.branches);
      List<JsonObject> all;
      try {
        all = await settings.all(
          '/api/v1/vendor/businesses/{businessId}/branches',
          settings.scope,
        );
      } finally {
        settings.dispose();
      }
      final eligible = <JsonObject>[];
      for (final b in all) {
        final r = await c.app.businessApi.request(
          'GET',
          '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/capabilities',
          pathParameters: {
            'businessId': c.app.scope.vendorId!,
            'branchId': '${b['id']}',
          },
        );
        if (c
            .collection(r.data is List ? r.data : jsonObject(r.data)['items'])
            .any(
              (v) =>
                  v['capability_type'] == 'PASTRY_BAKERY' &&
                  v['status'] == 'ACTIVE',
            )) {
          eligible.add(b);
        }
      }
      branches = eligible;
      await c.load();
      if (c.error != null) return;
      if (widget.id != null && area == PastryArea.products) {
        attached = c.collection(
          await c.fetch('$detail/option-groups', params: params),
        );
      }
      if (widget.id != null && area == PastryArea.groups) {
        children = c.collection(
          await c.fetch('$detail/options', params: params),
        );
      }
      if (widget.id != null && area == PastryArea.requests) {
        children = c.collection(
          await c.fetch('$detail/specifications', params: params),
        );
        changeEligibility = jsonObject(
          await c.fetch('$detail/change-eligibility', params: params),
        );
        cancelEligibility = jsonObject(
          await c.fetch('$detail/cancellation-eligibility', params: params),
        );
      }
      if (mounted) setState(() => evidenceReady = true);
    } catch (_) {
      if (mounted) {
        setState(
          () => error =
              'Pastry data could not be loaded. Refresh before making changes.',
        );
      }
    } finally {
      if (mounted) setState(() => reading = false);
    }
  }

  Future<void> feedback(String message, {bool success = false}) =>
      showOrderFeedback(
        context,
        title: success ? 'Saved' : 'Review required',
        message: message,
        success: success,
        warning: !success,
      );
  Future<JsonObject?> save(
    String method,
    String route,
    JsonObject body, {
    Map<String, String>? parameters,
  }) async {
    setState(() => action = route);
    final result = await c.save(
      method: method,
      route: route,
      body: body,
      params: parameters ?? params,
    );
    if (mounted) {
      await feedback(
        result == null
            ? c.error ?? 'The change could not be confirmed.'
            : 'Your changes have been saved.',
        success: result != null,
      );
      if (result != null) await load();
      setState(() => action = null);
    }
    return result;
  }

  Future<void> recover() async {
    String? grant;
    if ('${c.pending?['path']}'.endsWith('/approve')) {
      grant = await Navigator.push<String>(
        context,
        setupRoute<String>(
          SettingsVerificationScreen(
            runtime: widget.runtime,
            purpose: 'pastry.high_value_quote_approval',
            title: 'Verify quote approval',
            actionLabel: 'Approve quote',
          ),
        ),
      );
      if (grant == null || !mounted) return;
    }
    setState(() => action = 'recovery');
    final result = await c.save(headers: {'X-Step-Up-Grant': ?grant});
    if (mounted) {
      await feedback(
        result == null
            ? c.error ?? 'The outcome is still unconfirmed.'
            : 'The saved change is confirmed.',
        success: result != null,
      );
      await load();
      setState(() => action = null);
    }
  }

  Object? initial(JsonObject old, String key, Object? sample) =>
      old.containsKey(key)
      ? old[key]
      : mock
      ? sample
      : null;
  PurchaseField field(
    JsonObject old,
    String key,
    String label, {
    String kind = 'text',
    Object? sample,
    bool required = false,
    int min = 0,
    int? max,
    Map<String, String> options = const {},
  }) => PurchaseField(
    key,
    label,
    kind: kind == 'money' ? 'text' : kind,
    initial: kind == 'money' && initial(old, key, sample) != null
        ? ((initial(old, key, sample) as num) / 100).toStringAsFixed(2)
        : initial(old, key, sample),
    required: required,
    min: min,
    max: max,
    options: options,
  );
  Future<String?> branchChoice(JsonObject old) async {
    if (old['branch_id'] != null) return '${old['branch_id']}';
    if (branches.isEmpty) {
      await feedback(
        'Enable pastry & bakery for a branch before creating records.',
      );
      return null;
    }
    if (!mounted) return null;
    final choice = await crmChoose(context, 'Choose bakery', {
      for (final b in branches)
        '${b['id']}': '${b['display_name'] ?? b['name'] ?? b['id']}',
    }, initial: mock ? ['${branches.first['id']}'] : []);
    return choice?.firstOrNull;
  }

  Future<List<PurchaseField>> relationshipFields(String branch) async {
    final fields = <PurchaseField>[];
    for (final entry in {
      'customer_ref': 'CUSTOMER',
      'corporate_profile_ref': 'CORPORATE',
    }.entries) {
      final choices = <String, String>{};
      var offset = 0;
      String? unavailable;
      try {
        while (true) {
          final response = jsonObject(
            await c.fetch(
              '$pastryBase/request-references',
              query: {
                'branch_id': branch,
                'kind': entry.value,
                'limit': '100',
                'offset': '$offset',
              },
            ),
          );
          if (response['branch_id'] != branch ||
              response['kind'] != entry.value ||
              response['items'] is! List) {
            throw const FormatException('Invalid reference choices');
          }
          for (final item in response['items']) {
            final row = jsonObject(item);
            if (row['reference'] is! String ||
                (row['reference'] as String).isEmpty) {
              throw const FormatException('Invalid relationship reference');
            }
            choices[row['reference']] = '${row['display_label'] ?? 'Customer'}';
          }
          if (response['has_more'] != true) break;
          final next = response['next_offset'];
          if (next is! int || next <= offset) {
            throw const FormatException('Invalid reference pagination');
          }
          offset = next;
        }
      } on ApiException catch (e) {
        if (![403, 404, 503].contains(e.statusCode)) rethrow;
        choices.clear();
        unavailable = 'unavailable';
      }
      fields.add(
        PurchaseField(
          entry.key,
          '${entry.value == 'CUSTOMER' ? 'Customer' : 'Corporate account'} (${unavailable ?? 'optional'})',
          kind: 'select',
          options: {
            '': unavailable == null
                ? 'No linked relationship'
                : 'Unavailable - continue without linking',
            ...choices,
          },
          initial: mock && choices.isNotEmpty && entry.value == 'CUSTOMER'
              ? choices.keys.first
              : '',
        ),
      );
    }
    return fields;
  }

  Future<void> edit() async {
    if (!writable) return;
    setState(() => preparing = true);
    try {
      final old = c.record ?? <String, dynamic>{}, creating = widget.id == null;
      String? branch;
      if (area != PastryArea.groups) {
        branch = await branchChoice(old);
        if (branch == null || !mounted) return;
      }
      final fields = <PurchaseField>[];
      switch (area) {
        case PastryArea.products:
          fields.addAll([
            field(
              old,
              'name',
              'Product name',
              required: true,
              sample: 'Chocolate celebration cake',
            ),
            field(
              old,
              'description',
              'Description',
              kind: 'notes',
              sample: 'Rich chocolate sponge with buttercream frosting.',
            ),
            if (creating)
              field(
                old,
                'product_type',
                'Product type',
                kind: 'select',
                required: true,
                sample: 'CAKE',
                options: {
                  for (final t in [
                    'CAKE',
                    'CUPCAKE',
                    'BREAD',
                    'DOUGHNUT',
                    'PIE',
                    'COOKIE',
                    'SMALL_CHOPS',
                    'SNACK',
                    'BOX',
                    'TRAY',
                    'PARTY_PACK',
                    'OTHER',
                  ])
                    t: crmLabel(t),
                },
              ),
            field(
              old,
              'base_price_minor',
              'Base price',
              kind: 'money',
              required: true,
              sample: 4200000,
            ),
            if (creating)
              field(old, 'currency', 'Currency', required: true, sample: 'NGN'),
            if (creating)
              field(
                old,
                'status',
                'Status',
                kind: 'select',
                required: true,
                sample: 'DRAFT',
                options: const {'DRAFT': 'Draft', 'ACTIVE': 'Active'},
              ),
            field(
              old,
              'customizable',
              'Accept customisation',
              kind: 'bool',
              sample: true,
            ),
            field(old, 'production_class', 'Production class'),
            field(
              old,
              'setup_buffer_minutes',
              'Setup time (minutes)',
              kind: 'number',
              required: true,
              sample: 15,
              max: 480,
            ),
            field(
              old,
              'cleanup_buffer_minutes',
              'Cleanup time (minutes)',
              kind: 'number',
              required: true,
              sample: 10,
              max: 480,
            ),
            field(
              old,
              'workload_units_per_item',
              'Capacity units per item',
              kind: 'number',
              required: true,
              sample: 1,
              min: 1,
              max: 1000,
            ),
            field(
              old,
              'position',
              'Display position',
              kind: 'number',
              required: true,
              sample: 0,
            ),
          ]);
        case PastryArea.groups:
          fields.addAll([
            field(
              old,
              'name',
              'Group name',
              required: true,
              sample: 'Cake finishes',
            ),
            if (creating)
              field(
                old,
                'group_type',
                'Group type',
                kind: 'select',
                required: true,
                sample: 'ICING',
                options: {
                  for (final t in [
                    'FLAVOUR',
                    'SIZE',
                    'WEIGHT',
                    'LAYERS',
                    'FILLING',
                    'ICING',
                    'SERVING_COUNT',
                    'PACKAGING',
                    'SHAPE',
                    'COLOUR_SCHEME',
                    'DECORATION_STYLE',
                  ])
                    t: crmLabel(t),
                },
              ),
            field(
              old,
              'min_select',
              'Minimum selections',
              kind: 'number',
              required: true,
              sample: 1,
            ),
            field(
              old,
              'max_select',
              'Maximum selections',
              kind: 'number',
              required: true,
              sample: 1,
            ),
          ]);
        case PastryArea.requests:
          fields.addAll([
            field(
              old,
              'title',
              'Request title',
              required: true,
              sample: 'Anniversary celebration cake',
            ),
            field(
              old,
              'notes',
              'Customer notes',
              kind: 'notes',
              sample: 'Elegant finish with a personalised inscription.',
            ),
            PurchaseField(
              'event_at',
              'Required date & time',
              kind: 'date',
              initial:
                  old['required_at'] ??
                  (mock
                      ? DateTime.now()
                            .toUtc()
                            .add(const Duration(days: 7))
                            .toIso8601String()
                      : null),
            ),
            if (creating) ...await relationshipFields(branch!),
            if (creating)
              field(old, 'change_cutoff_at', 'Change cutoff', kind: 'date'),
          ]);
      }
      if (!mounted) return;
      final values = await productionForm(
        context,
        creating
            ? 'Create ${area == PastryArea.products
                  ? 'product'
                  : area == PastryArea.groups
                  ? 'option group'
                  : 'custom request'}'
            : 'Edit details',
        fields,
        validate: (v) {
          if (area == PastryArea.products &&
              purchaseDecimal('${v['base_price_minor']}') == null) {
            return 'Enter a valid price with up to two decimal places.';
          }
          if (area == PastryArea.groups && v['max_select'] < v['min_select']) {
            return 'Maximum selections must cover the minimum.';
          }
          if (area == PastryArea.requests && '${v['title']}'.length < 2) {
            return 'Enter a request title with at least two characters.';
          }
          return null;
        },
      );
      if (values == null || !mounted) return;
      final body = <String, dynamic>{...values, 'branch_id': ?branch};
      if (area == PastryArea.products) {
        body['base_price_minor'] = purchaseDecimal(
          '${values['base_price_minor']}',
        );
        body['logistics'] = old['logistics'] ?? {};
        body['capacity_pool_id'] = old['capacity_pool_id'];
        body['production_class'] = values['production_class'];
      }
      if (creating && area == PastryArea.requests) {
        final config = await pastryConfiguration(context, widget.runtime);
        if (config == null || !mounted) return;
        body.addAll(config);
      }
      if (!creating) {
        body[area == PastryArea.requests ? 'expected_version' : 'version'] =
            old['version'];
      }
      await save(creating ? 'POST' : 'PATCH', creating ? base : detail, body);
    } catch (_) {
      if (mounted) {
        await feedback(
          'The editor could not be opened. Refresh and try again.',
        );
      }
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  Future<void> editOption([JsonObject? old]) async {
    if (!writable) return;
    final r = old ?? <String, dynamic>{};
    final values = await productionForm(
      context,
      old == null ? 'Add option' : 'Edit option',
      [
        field(r, 'name', 'Option name', required: true, sample: 'Buttercream'),
        field(r, 'code', 'Code', sample: 'BUTTERCREAM'),
        field(
          r,
          'price_delta_minor',
          'Additional price',
          kind: 'money',
          required: true,
          sample: 0,
        ),
        field(
          r,
          'workload_weight',
          'Workload weight',
          kind: 'number',
          required: true,
          sample: 1,
          min: 1,
        ),
        field(
          r,
          'position',
          'Display position',
          kind: 'number',
          required: true,
          sample: 0,
          max: 999,
        ),
        field(
          r,
          'capacity_class',
          'Capacity class',
          kind: 'select',
          options: {
            for (final t in [
              'GENERAL',
              'HOT',
              'COLD',
              'BAKERY',
              'DECORATION',
              'PACKING',
              'CHILLING',
            ])
              t: crmLabel(t),
          },
        ),
      ],
      validate: (v) => purchaseDecimal('${v['price_delta_minor']}') == null
          ? 'Enter a valid additional price with up to two decimal places.'
          : null,
    );
    if (values == null || !mounted) return;
    await save(
      old == null ? 'POST' : 'PATCH',
      old == null ? '$detail/options' : '$pastryBase/options/{optionId}',
      {
        ...values,
        'price_delta_minor': purchaseDecimal('${values['price_delta_minor']}'),
        'capacity_class': values['capacity_class'],
        if (old != null) 'version': old['version'],
      },
      parameters: old == null ? params : {'optionId': '${old['id']}'},
    );
  }

  Future<void> optionActions(JsonObject r) async {
    final selection = await settingsSheet<String>(
      context,
      Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          SettingsHeading('${r['name']}'),
          SettingsAction(
            'Edit option',
            onPressed: () => Navigator.pop(context, 'edit'),
          ),
          const SizedBox(height: 12),
          if (r['status'] != 'ARCHIVED')
            SettingsAction(
              'Archive option',
              danger: true,
              onPressed: () => Navigator.pop(context, 'archive'),
            ),
        ],
      ),
    );
    if (!mounted) return;
    if (selection == 'edit') await editOption(r);
    if (selection == 'archive' &&
        mounted &&
        await confirmSettings(
          context,
          'Archive option?',
          'This option will be archived.',
          danger: true,
        ) &&
        mounted) {
      await save(
        'POST',
        '$pastryBase/options/{optionId}/archive',
        {'expected_version': r['version']},
        parameters: {'optionId': '${r['id']}'},
      );
    }
  }

  Future<void> specification() async {
    if (!writable || changeEligibility?['allowed'] != true) return;
    final old = children.isEmpty
        ? <String, dynamic>{}
        : jsonObject(children.last['specification']);
    final values = await pastryDesign(
      context,
      widget.runtime,
      old,
      requestId: widget.id!,
    );
    if (values == null || !mounted) return;
    await save('POST', '$detail/specifications', {
      'expected_version': c.record!['version'],
      'specification': {...old, ...values},
    });
  }

  Future<void> detachGroup(JsonObject group) async {
    if (!writable) return;
    if (!await confirmSettings(
          context,
          'Remove option group?',
          'Remove ${group['name']} from this product. The reusable group and its options remain available.',
          danger: true,
        ) ||
        !mounted) {
      return;
    }
    await save(
      'DELETE',
      '$detail/option-groups/{groupId}',
      {'expected_version': c.record!['version']},
      parameters: {...params, 'groupId': '${group['group_id']}'},
    );
  }

  Future<void> attachGroup() async {
    if (!writable) return;
    setState(() => preparing = true);
    try {
      final groups = c
          .collection(await c.fetch('$pastryBase/option-groups'))
          .where((g) => g['status'] == 'ACTIVE')
          .toList();
      if (!mounted) return;
      if (groups.isEmpty) {
        await feedback('Create an active option group before attaching it.');
        return;
      }
      final choice = await crmChoose(context, 'Choose option group', {
        for (final g in groups) '${g['id']}': '${g['name']}',
      });
      if (choice == null || !mounted) return;
      final values = await productionForm(context, 'Attach option group', [
        PurchaseField(
          'position',
          'Display position',
          kind: 'number',
          required: true,
          initial: mock ? 0 : null,
        ),
        const PurchaseField('required', 'Selection required', kind: 'bool'),
      ]);
      if (values == null || !mounted) return;
      await save('POST', '$detail/option-groups', {
        'group_id': choice.single,
        ...values,
        'expected_version': c.record!['version'],
      });
    } catch (_) {
      if (mounted) {
        await feedback(
          'Option groups could not be loaded. Refresh and try again.',
        );
      }
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  Future<void> workflow(PastryWork work) async {
    await Navigator.push(
      context,
      setupRoute<void>(
        PastryWorkflowsScreen(
          runtime: widget.runtime,
          sourceId: widget.id!,
          work: work,
        ),
      ),
    );
    if (mounted) await load();
  }

  Future<void> logistics() async {
    if (!writable || area != PastryArea.products) return;
    final old = c.record!, logistics = jsonObject(old['logistics'] ?? {});
    final values = await productionForm(context, 'Delivery handling', [
      field(
        logistics,
        'temperature_class',
        'Temperature',
        kind: 'select',
        options: const {
          'AMBIENT': 'Ambient',
          'CHILLED': 'Chilled',
          'FROZEN': 'Frozen',
        },
      ),
      field(
        logistics,
        'spill_risk',
        'Spill risk',
        kind: 'select',
        options: const {'LOW': 'Low', 'MEDIUM': 'Medium', 'HIGH': 'High'},
      ),
      field(
        logistics,
        'weight_grams',
        'Weight (grams)',
        kind: 'number',
        max: 1000000,
      ),
      field(
        logistics,
        'package_size',
        'Package size',
        kind: 'select',
        options: const {
          'SMALL': 'Small',
          'MEDIUM': 'Medium',
          'LARGE': 'Large',
          'OVERSIZE': 'Oversize',
        },
      ),
      field(logistics, 'fragile', 'Fragile', kind: 'bool'),
      field(logistics, 'upright_required', 'Keep upright', kind: 'bool'),
      field(
        logistics,
        'cold_chain_required',
        'Cold chain required',
        kind: 'bool',
      ),
    ]);
    if (values == null || !mounted) return;
    await save('PATCH', detail, {
      ...old,
      'logistics': {...logistics, ...values},
    });
  }

  Future<void> lifecycle(String state) async {
    if (!writable) return;
    final r = c.record!;
    if (state == 'cancel') {
      if (cancelEligibility?['state'] == 'RESTRICTED') return;
      final needsReview = cancelEligibility?['state'] == 'REQUIRES_REVIEW';
      final values = await productionForm(
        context,
        'Cancel request',
        [
          PurchaseField(
            'reason',
            'Cancellation reason',
            kind: 'notes',
            required: true,
            initial: mock ? 'Customer no longer needs this request.' : null,
          ),
          if (needsReview)
            const PurchaseField(
              'review_approved',
              'Cancellation review has been approved',
              kind: 'bool',
            ),
        ],
        validate: (v) => needsReview && v['review_approved'] != true
            ? 'Confirm review approval before cancelling.'
            : null,
      );
      if (values == null || !mounted) return;
      if (!await confirmSettings(
            context,
            'Cancel request?',
            'This cancels the request and releases any eligible held capacity.',
            danger: true,
          ) ||
          !mounted) {
        return;
      }
      await save('POST', '$detail/cancel', {
        ...values,
        'review_approved': values['review_approved'] ?? false,
        'expected_version': r['version'],
      });
      return;
    }
    if (!await confirmSettings(
          context,
          state == 'submit'
              ? 'Submit request?'
              : 'Archive ${area == PastryArea.products ? 'product' : 'option group'}?',
          state == 'submit'
              ? 'Move this draft into the submitted state.'
              : 'Archive this record.',
          danger: state == 'archive',
        ) ||
        !mounted) {
      return;
    }
    await save('POST', '$detail/$state', {'expected_version': r['version']});
  }

  Future<void> open(JsonObject r) async {
    await Navigator.push(
      context,
      setupRoute<void>(
        PastryWorkspaceScreen(
          runtime: widget.runtime,
          area: area,
          id: '${r['id']}',
        ),
      ),
    );
    if (mounted) await load();
  }

  Widget status(Object? value) {
    final state = '${value ?? 'UNKNOWN'}';
    final color =
        (['ACTIVE', 'APPROVED', 'COMPLETED'].contains(state)
                ? KarryamColors.tealInk
                : [
                    'CANCELLED',
                    'ARCHIVED',
                    'DECLINED',
                    'BLOCKED',
                    'RESTRICTED',
                  ].contains(state)
                ? const Color(0xFFB34F52)
                : [
                    'SUBMITTED',
                    'PENDING_APPROVAL',
                    'QUOTE_PENDING',
                    'REQUIRES_REVIEW',
                  ].contains(state)
                ? const Color(0xFFAC792D)
                : const Color(0xFF667587))
            .forInk(context);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
      decoration: BoxDecoration(
        color: color.withValues(alpha: .1),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Text(
        crmLabel(state),
        style: TextStyle(
          color: color,
          fontSize: 12,
          fontWeight: FontWeight.w700,
        ),
      ),
    );
  }

  Widget empty(String title, String message, IconData icon) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 32),
    child: Column(
      children: [
        Container(
          width: 88,
          height: 88,
          decoration: BoxDecoration(
            color: Theme.of(context).colorScheme.surfaceContainerHighest,
            borderRadius: BorderRadius.circular(28),
          ),
          child: Icon(
            icon,
            size: 42,
            color: const Color(0xFFB97C35).forInk(context),
          ),
        ),
        const SizedBox(height: 20),
        Text(
          title,
          style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w700),
        ),
        const SizedBox(height: 8),
        Text(message, textAlign: TextAlign.center),
      ],
    ),
  );
  Widget button(
    String label,
    VoidCallback? callback, {
    bool secondary = false,
    bool danger = false,
  }) => Padding(
    padding: const EdgeInsets.only(top: 12),
    child: SettingsAction(
      label,
      onPressed: callback == null
          ? null
          : () {
              setState(() => buttonAction = label);
              callback();
            },
      busy: action != null && c.busy && buttonAction == label,
      secondary: secondary,
      danger: danger,
    ),
  );
  List<Widget> detailWidgets() {
    final r = c.record!;
    final editable =
        area != PastryArea.requests ||
        ['DRAFT', 'SUBMITTED'].contains(r['status']);
    return [
      SettingsCard(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  area.icon,
                  size: 34,
                  color: const Color(0xFFB97C35).forInk(context),
                ),
                const Spacer(),
                status(r['status']),
              ],
            ),
            const SizedBox(height: 20),
            Text(
              '${r['name'] ?? r['title']}',
              style: const TextStyle(fontSize: 26, fontWeight: FontWeight.w800),
            ),
            if ('${r['description'] ?? r['notes'] ?? ''}'.isNotEmpty) ...[
              const SizedBox(height: 10),
              Text('${r['description'] ?? r['notes']}'),
            ],
            if (r['branch_id'] != null)
              SettingsFact(
                'Branch',
                branches
                        .where((b) => b['id'] == r['branch_id'])
                        .map(
                          (b) => '${b['display_name'] ?? b['name'] ?? b['id']}',
                        )
                        .firstOrNull ??
                    '${r['branch_id']}',
              ),
            if (area == PastryArea.products) ...[
              SettingsFact(
                'Base price',
                dashboardMoney(r['base_price_minor'], '${r['currency']}'),
              ),
              SettingsFact(
                'Customisation',
                r['customizable'] == true ? 'Available' : 'Unavailable',
              ),
              SettingsFact(
                'Preparation',
                '${r['setup_buffer_minutes']} min setup /  ${r['cleanup_buffer_minutes']} min cleanup',
              ),
            ],
            if (area == PastryArea.groups)
              SettingsFact(
                'Selections',
                '${r['min_select']} to ${r['max_select']}',
              ),
            if (area == PastryArea.requests && r['required_at'] != null)
              SettingsFact('Required', supportDate(r['required_at'])),
          ],
        ),
      ),
      if (editable && r['status'] != 'ARCHIVED')
        button('Edit details', writable ? edit : null, secondary: true),
      if (area == PastryArea.products) ...[
        if (r['status'] != 'ARCHIVED')
          button(
            'Plan production',
            writable ? () => workflow(PastryWork.standard) : null,
            secondary: true,
          ),
        button(
          'Delivery handling',
          writable ? logistics : null,
          secondary: true,
        ),
        if (r['status'] != 'ARCHIVED')
          button(
            'Attach option group',
            writable ? attachGroup : null,
            secondary: true,
          ),
        const SizedBox(height: 20),
        const SettingsHeading('Attached option groups'),
        if (attached.isEmpty)
          const SettingsNote('No option groups are attached yet.'),
        for (final group in attached)
          SettingsCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  '${group['name']}',
                  style: const TextStyle(
                    fontSize: 18,
                    fontWeight: FontWeight.w700,
                  ),
                ),
                SettingsFact(
                  'Selections',
                  '${group['min_select']} to ${group['max_select']}',
                ),
                SettingsFact(
                  'Selection required',
                  group['required'] == true ? 'Yes' : 'No',
                ),
                if (group['status'] == 'ARCHIVED')
                  SettingsNote('This option group is archived.', warning: true),
                if (r['status'] != 'ARCHIVED')
                  button(
                    'Remove ${group['name']}',
                    writable ? () => detachGroup(group) : null,
                    danger: true,
                  ),
              ],
            ),
          ),
        if (r['capacity_pool_id'] != null)
          SettingsFact('Capacity pool', '${r['capacity_pool_id']}'),
        if (r['status'] != 'ARCHIVED')
          button(
            'Archive product',
            writable ? () => lifecycle('archive') : null,
            danger: true,
          ),
      ],
      if (area == PastryArea.groups) ...[
        const SizedBox(height: 24),
        const SettingsHeading('Options'),
        if (children.isEmpty)
          empty(
            'Make it their own',
            'Add flavours, finishes or extras to this group.',
            Icons.palette_outlined,
          ),
        for (final option in children)
          SettingsCard(
            child: SettingsLink(
              '${option['name']}',
              Icons.auto_awesome_outlined,
              subtitle:
                  '${((option['price_delta_minor'] as num) / 100).toStringAsFixed(2)} extra /  ${crmLabel(option['status'])}',
              color: const Color(0xFF8B65B9).forInk(context),
              onTap: writable ? () => optionActions(option) : null,
            ),
          ),
        if (r['status'] != 'ARCHIVED')
          button('Add option', writable ? () => editOption() : null),
        if (r['status'] != 'ARCHIVED')
          button(
            'Archive group',
            writable ? () => lifecycle('archive') : null,
            danger: true,
          ),
      ],
      if (area == PastryArea.requests) ...[
        const SizedBox(height: 24),
        for (final work in [
          PastryWork.quotes,
          PastryWork.changes,
          PastryWork.milestones,
        ])
          SettingsCard(
            child: SettingsLink(
              work.title,
              work.icon,
              color: const Color(0xFF8B65B9).forInk(context),
              onTap: () => workflow(work),
            ),
          ),
        SettingsCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const SettingsHeading('Request configuration'),
              for (final key in [
                'change_policy',
                'cancellation_policy',
                'logistics',
              ])
                if (r[key] is Map)
                  for (final e in jsonObject(r[key]).entries)
                    SettingsFact(
                      crmLabel(e.key),
                      e.value is bool ? (e.value ? 'Yes' : 'No') : '${e.value}',
                    ),
              if (r['risk_flags'] is List &&
                  (r['risk_flags'] as List).isNotEmpty) ...[
                const SettingsHeading('Risk flags'),
                for (final flag in r['risk_flags'])
                  SettingsNote(crmLabel(flag), warning: true),
              ],
            ],
          ),
        ),
        const SettingsHeading('Design history'),
        if (children.isEmpty)
          empty(
            'Start with the details',
            'Save the design specification before preparing a quote.',
            Icons.draw_outlined,
          ),
        for (final spec in children.reversed)
          SettingsCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'Revision ${spec['revision']}',
                  style: const TextStyle(
                    fontWeight: FontWeight.w700,
                    fontSize: 18,
                  ),
                ),
                const SizedBox(height: 8),
                for (final entry
                    in jsonObject(spec['specification']).entries.where(
                      (e) => e.value != null && '${e.value}'.isNotEmpty,
                    ))
                  SettingsFact(
                    crmLabel(entry.key),
                    entry.value is Map || entry.value is List
                        ? 'Saved in specification'
                        : '${entry.value}',
                  ),
              ],
            ),
          ),
        if (changeEligibility?['allowed'] == true &&
            ![
              'ACCEPTED',
              'ACCEPTED_PENDING_ORDER',
              'CONVERTED',
              'CANCELLED',
            ].contains(r['status']))
          button('Save design revision', writable ? specification : null),
        if (changeEligibility?['allowed'] == false)
          SettingsNote(
            'Changes unavailable: ${crmLabel(changeEligibility?['reason_code'])}',
            warning: true,
          ),
        if (r['status'] == 'DRAFT')
          button('Submit request', writable ? () => lifecycle('submit') : null),
        if (cancelEligibility?['state'] != 'RESTRICTED' &&
            cancelEligibility != null)
          button(
            'Cancel request',
            writable ? () => lifecycle('cancel') : null,
            danger: true,
          ),
        if (cancelEligibility?['state'] == 'RESTRICTED')
          SettingsNote(
            'Cancellation unavailable: ${crmLabel(cancelEligibility?['reason_code'])}',
            warning: true,
          ),
      ],
    ];
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      appBar: AppBar(
        title: Text(
          widget.id == null
              ? area.title
              : area == PastryArea.products
              ? 'Product details'
              : area == PastryArea.groups
              ? 'Option group'
              : 'Request details',
        ),
        actions: [
          IconButton(
            onPressed: reading || c.busy ? null : load,
            icon: const Icon(Icons.refresh_rounded),
            tooltip: 'Refresh',
          ),
        ],
      ),
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 680),
            child: ListView(
              padding: const EdgeInsets.all(20),
              children: [
                if ((reading || c.loading) &&
                    c.record == null &&
                    c.rows.isEmpty)
                  const PremiumSkeleton(detail: true)
                else ...[
                  if (error != null || c.error != null)
                    SettingsNote(error ?? c.error!),
                  if (c.pending != null) ...[
                    const SettingsNote(
                      'A previous change has an unconfirmed outcome. Retry it before making another change.',
                      warning: true,
                    ),
                    button(
                      'Retry saved change',
                      c.busy || reading ? null : recover,
                    ),
                  ],
                  if (widget.id == null) ...[
                    Wrap(
                      spacing: 8,
                      children: [
                        for (final state in ['CURRENT', 'ARCHIVED'])
                          ChoiceChip(
                            label: Text(
                              state == 'CURRENT' ? 'Current' : 'Archived',
                            ),
                            selected: filter == state,
                            onSelected: (_) => setState(() => filter = state),
                          ),
                      ],
                    ),
                    const SizedBox(height: 20),
                    if (c.rows
                        .where(
                          (r) => filter == 'ARCHIVED'
                              ? r['status'] == 'ARCHIVED'
                              : r['status'] != 'ARCHIVED',
                        )
                        .isEmpty)
                      empty(
                        'No ${area == PastryArea.requests
                            ? 'requests'
                            : area == PastryArea.groups
                            ? 'option groups'
                            : 'products'} here',
                        filter == 'ARCHIVED'
                            ? 'Archived records will appear here.'
                            : 'Create your first ${area == PastryArea.requests
                                  ? 'custom request'
                                  : area == PastryArea.groups
                                  ? 'option group'
                                  : 'pastry product'}.',
                        area.icon,
                      ),
                    for (final r in c.rows.where(
                      (r) => filter == 'ARCHIVED'
                          ? r['status'] == 'ARCHIVED'
                          : r['status'] != 'ARCHIVED',
                    ))
                      SettingsCard(
                        child: InkWell(
                          borderRadius: BorderRadius.circular(20),
                          onTap: () => open(r),
                          child: Padding(
                            padding: const EdgeInsets.symmetric(vertical: 6),
                            child: Row(
                              children: [
                                Container(
                                  width: 56,
                                  height: 56,
                                  decoration: BoxDecoration(
                                    color: Theme.of(
                                      context,
                                    ).colorScheme.surfaceContainerHighest,
                                    borderRadius: BorderRadius.circular(18),
                                  ),
                                  child: Icon(
                                    area.icon,
                                    color: const Color(
                                      0xFFB97C35,
                                    ).forInk(context),
                                    size: 28,
                                  ),
                                ),
                                const SizedBox(width: 14),
                                Expanded(
                                  child: Column(
                                    crossAxisAlignment:
                                        CrossAxisAlignment.start,
                                    children: [
                                      Text(
                                        '${r['name'] ?? r['title']}',
                                        style: const TextStyle(
                                          fontSize: 17,
                                          fontWeight: FontWeight.w700,
                                        ),
                                      ),
                                      const SizedBox(height: 8),
                                      status(r['status']),
                                    ],
                                  ),
                                ),
                                const Icon(Icons.chevron_right_rounded),
                              ],
                            ),
                          ),
                        ),
                      ),
                    button(
                      area == PastryArea.products
                          ? 'Add product'
                          : area == PastryArea.groups
                          ? 'Create option group'
                          : 'New custom request',
                      writable ? edit : null,
                    ),
                  ] else if (c.record != null)
                    ...detailWidgets(),
                ],
                const SizedBox(height: 24),
              ],
            ),
          ),
        ),
      ),
    ),
  );
}
