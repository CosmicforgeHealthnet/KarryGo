import '../../dashboard/ui/dashboard_format.dart' show dashboardMoney;
import '../../pastry/ui/pastry_configuration_screen.dart'
    show PastrySavedRequirementsScreen;
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../crm/ui/crm_editors.dart';
import '../../crm/ui/crm_design.dart' show crmLabel;
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../production/ui/production_form_screen.dart';
import '../../production/ui/production_capacity_controls_screen.dart';
import '../../production/ui/production_operations_screen.dart';
import '../../production/ui/production_reservation_screen.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../../settings/state/settings_controller.dart';
import '../../settings/ui/settings_design.dart';
import '../../support/ui/order_support_screen.dart' show supportDate;
import '../state/cloud_kitchen_workspace_controller.dart';

enum KitchenArea { packages, plans, settings, occurrences }

extension KitchenAreaInfo on KitchenArea {
  String get title => switch (this) {
    KitchenArea.packages => 'Bulk packages',
    KitchenArea.plans => 'Corporate plans',
    KitchenArea.settings => 'Kitchen settings',
    KitchenArea.occurrences => 'Plan occurrences',
  };
  String get collection => switch (this) {
    KitchenArea.packages => 'bulk-packages',
    KitchenArea.plans => 'corporate-plans',
    KitchenArea.settings => 'settings',
    KitchenArea.occurrences => 'corporate-plans/{planId}/occurrences',
  };
  IconData get icon => switch (this) {
    KitchenArea.packages => Icons.takeout_dining_outlined,
    KitchenArea.plans => Icons.event_repeat_rounded,
    KitchenArea.settings => Icons.tune_rounded,
    KitchenArea.occurrences => Icons.calendar_month_outlined,
  };
  String get idKey => this == KitchenArea.packages
      ? 'packageId'
      : this == KitchenArea.settings
      ? 'branchId'
      : 'planId';
}

class CloudKitchenHub extends StatelessWidget {
  const CloudKitchenHub({super.key, required this.runtime});
  final VendorRuntime runtime;
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Cloud kitchen')),
    body: SafeArea(
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 680),
          child: ListView(
            padding: const EdgeInsets.all(20),
            children: [
              const Text(
                'Built for bigger orders',
                style: TextStyle(fontSize: 28, fontWeight: FontWeight.w800),
              ),
              const SizedBox(height: 8),
              const Text(
                'Packages, recurring production and kitchen capacity.',
              ),
              const SizedBox(height: 24),
              for (final area in [
                KitchenArea.packages,
                KitchenArea.plans,
                KitchenArea.settings,
              ])
                SettingsCard(
                  child: SettingsLink(
                    area.title,
                    area.icon,
                    color:
                        (area == KitchenArea.plans
                                ? const Color(0xFF7660A6)
                                : area == KitchenArea.settings
                                ? const Color(0xFFA76D25)
                                : KarryamColors.tealInk)
                            .forInk(context),
                    subtitle: switch (area) {
                      KitchenArea.packages =>
                        'Meals, quantity pricing and preparation',
                      KitchenArea.plans =>
                        'Recurring schedules and occurrence controls',
                      _ => 'Lead times and reservation defaults',
                    },
                    onTap: () => Navigator.push(
                      context,
                      setupRoute<void>(
                        CloudKitchenWorkspaceScreen(
                          runtime: runtime,
                          area: area,
                        ),
                      ),
                    ),
                  ),
                ),
              SettingsLink(
                'Capacity controls',
                Icons.donut_large_rounded,
                onTap: () => Navigator.push(
                  context,
                  setupRoute<void>(ProductionControlsHub(runtime: runtime)),
                ),
              ),
              SettingsLink(
                'Production queue',
                Icons.precision_manufacturing_outlined,
                onTap: () => Navigator.push(
                  context,
                  setupRoute<void>(
                    ProductionOperationsScreen(runtime: runtime),
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

class CloudKitchenWorkspaceScreen extends StatefulWidget {
  const CloudKitchenWorkspaceScreen({
    super.key,
    required this.runtime,
    required this.area,
    this.id,
    this.planId,
  });
  final VendorRuntime runtime;
  final KitchenArea area;
  final String? id, planId;
  @override
  State<CloudKitchenWorkspaceScreen> createState() =>
      _CloudKitchenWorkspaceScreenState();
}

class _CloudKitchenWorkspaceScreenState
    extends State<CloudKitchenWorkspaceScreen> {
  KitchenArea get area => widget.area;
  String? branchId;
  String get base => '$cloudKitchenBase/${area.collection}';
  String get detail => '$base/{${area.idKey}}';
  Map<String, String> get params => {
    if (widget.id != null) area.idKey: widget.id!,
    if (widget.planId != null) 'planId': widget.planId!,
    'branchId': ?branchId,
  };
  late final c = CloudKitchenWorkspaceController(
    widget.runtime,
    area == KitchenArea.settings
        ? '$base/{branchId}'
        : widget.id == null
        ? base
        : detail,
    parameters: {
      if (widget.id != null) area.idKey: widget.id!,
      if (widget.planId != null) 'planId': widget.planId!,
    },
  );
  List<JsonObject> branches = [], bands = [], components = [];
  final itemNames = <String, String>{};
  String? readError, currency;
  bool get decimalCurrency => const {
    'NGN',
    'USD',
    'GBP',
    'EUR',
    'GHS',
    'KES',
    'ZAR',
  }.contains(currency);
  String price(Object? value) => decimalCurrency
      ? dashboardMoney(value, currency)
      : '$currency $value minor units';
  bool preparing = false, evidenceReady = false, readingCloud = false;
  String? action;
  String filter = 'ACTIVE';
  bool get mock => widget.runtime.mode == VendorDataMode.mock;
  bool get single => widget.id != null || area == KitchenArea.settings;
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

  Future<List<JsonObject>> list(
    VendorFeatureApi api,
    String path,
    String wrapper, {
    Map<String, String> parameters = const {},
    Map<String, String> query = const {},
  }) async {
    final fields = api
        .operation('GET', path)
        .parametersIn('query')
        .map((p) => p['name'])
        .toSet();
    final rows = <JsonObject>[];
    while (true) {
      final r = await api.request(
        'GET',
        path,
        pathParameters: parameters,
        query: {
          for (final e in query.entries)
            if (fields.contains(e.key)) e.key: e.value,
          if (fields.contains('limit')) 'limit': '200',
          if (fields.contains('offset')) 'offset': '${rows.length}',
        },
      );
      final value = r.data is List ? r.data : jsonObject(r.data)[wrapper];
      final next = c.collection(value);
      rows.addAll(next);
      if (!fields.contains('offset')) return rows;
      final data = r.data is Map ? jsonObject(r.data) : <String, dynamic>{};
      if (data['has_more'] != true &&
          jsonObject(data['pagination'] ?? {})['has_more'] != true &&
          next.length < 200) {
        return rows;
      }
      if (next.isEmpty) throw const FormatException('Incomplete selection');
    }
  }

  Future<void> load() async {
    if (readingCloud || !mounted) return;
    readingCloud = true;
    if (mounted) {
      setState(() {
        readError = null;
        evidenceReady = false;
      });
    }
    try {
      final settings = SettingsController(c.app, SettingsKind.branches);
      try {
        branches = await settings.all(
          '/api/v1/vendor/businesses/{businessId}/branches',
          settings.scope,
        );
      } finally {
        settings.dispose();
      }
      final eligible = <JsonObject>[];
      for (final b in branches) {
        final result = await c.app.businessApi.request(
          'GET',
          '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/capabilities',
          pathParameters: {
            'businessId': c.app.scope.vendorId!,
            'branchId': '${b['id']}',
          },
        );
        final value = result.data is List
            ? result.data
            : jsonObject(result.data)['items'];
        if (c
            .collection(value)
            .any(
              (v) =>
                  v['capability_type'] == 'CLOUD_KITCHEN' &&
                  v['status'] == 'ACTIVE',
            )) {
          eligible.add(b);
        }
      }
      branches = eligible;
      if (area == KitchenArea.settings) {
        branchId ??= branches.firstOrNull?['id'] as String?;
        if (branchId == null) {
          if (mounted) {
            setState(
              () => readError =
                  'Enable cloud kitchen for a branch to configure its kitchen.',
            );
          }
          return;
        }
        c.parameters['branchId'] = branchId!;
      }
      await c.load();
      if (c.error != null || !mounted) return;
      if (area == KitchenArea.packages && single) {
        final results = await Future.wait([
          c.fetch('$detail/bands', params: params),
          c.fetch('$detail/components', params: params),
        ]);
        currency = await kitchenCurrency('${c.record!['branch_id']}');
        bands = c.collection(results[0]);
        components = c.collection(results[1]);
        for (final component in components) {
          try {
            final item = (await c.app.foodCatalogueApi.request(
              'GET',
              '/api/v1/vendor/food/items/{itemId}',
              pathParameters: {'itemId': '${component['item_ref']}'},
            )).object;
            itemNames['${component['item_ref']}'] = '${item['name']}';
          } catch (_) {
            itemNames['${component['item_ref']}'] = 'Item unavailable';
          }
        }
      }
      if (mounted) setState(() => evidenceReady = true);
    } catch (e) {
      if (mounted) {
        setState(
          () => readError =
              'Kitchen data could not be loaded. Refresh before making changes.',
        );
      }
    } finally {
      if (mounted) setState(() => readingCloud = false);
    }
  }

  Future<void> open(KitchenArea area, {String? id, String? planId}) async {
    await Navigator.push(
      context,
      setupRoute<void>(
        CloudKitchenWorkspaceScreen(
          runtime: widget.runtime,
          area: area,
          id: id,
          planId: planId,
        ),
      ),
    );
    if (mounted) await load();
  }

  Future<void> feedback(
    String message, {
    bool success = false,
    bool warning = false,
  }) => showOrderFeedback(
    context,
    title: success ? 'Saved' : 'Review required',
    message: message,
    success: success,
    warning: warning,
  );
  Future<JsonObject?> change(
    String method,
    String route,
    JsonObject body, {
    Map<String, String> extra = const {},
  }) async {
    setState(() => action = route);
    final saved = await c.save(
      method: method,
      route: route,
      body: body,
      params: {...params, ...extra},
    );
    if (!mounted) return saved;
    await feedback(
      saved == null
          ? c.error ?? 'The change was not confirmed.'
          : 'Your kitchen records are up to date.',
      success: saved != null,
      warning: saved == null && c.pending != null,
    );
    if (saved != null && mounted) await load();
    if (mounted) setState(() => action = null);
    return saved;
  }

  Future<Map<String, String>> poolChoices(String branch) async {
    final pools = await list(
      c.app.productionApi,
      '/api/v1/vendor/production/pools',
      'items',
    );
    return {
      for (final p in pools.where(
        (p) =>
            p['branch_id'] == branch &&
            p['capability'] == 'CLOUD_KITCHEN' &&
            p['status'] == 'ACTIVE',
      ))
        '${p['id']}': '${p['name']}',
    };
  }

  Future<void> edit() async {
    if (preparing || !c.writable || !evidenceReady) return;
    setState(() => preparing = true);
    try {
      final old = c.record ?? <String, dynamic>{};
      String? branch = old['branch_id'] as String?;
      if (area == KitchenArea.settings) branch = branchId;
      if (branch == null) {
        if (branches.isEmpty) {
          await feedback(
            'Enable cloud kitchen for a branch before creating kitchen records.',
            warning: true,
          );
          return;
        }
        if (!mounted) return;
        final choice = await crmChoose(context, 'Choose kitchen', {
          for (final b in branches) '${b['id']}': '${b['name']}',
        }, initial: mock ? ['${branches.first['id']}'] : []);
        if (choice == null || !mounted) return;
        branch = choice.single;
      }
      final pools = await poolChoices(branch);
      if (!mounted) return;
      final packages = area == KitchenArea.plans
          ? await list(
              c.app.cloudKitchenApi,
              '$cloudKitchenBase/bulk-packages',
              'items',
            )
          : <JsonObject>[];
      if (!mounted) return;
      final activePackages = {
        for (final p in packages.where(
          (p) => p['branch_id'] == branch && p['status'] == 'ACTIVE',
        ))
          '${p['id']}': '${p['name']}',
      };
      if (area == KitchenArea.plans && activePackages.isEmpty) {
        await feedback(
          'Create and activate a bulk package for this kitchen first.',
          warning: true,
        );
        return;
      }
      final corporate = area == KitchenArea.plans
          ? await corporateChoices(branch, old)
          : <String, String>{};
      if (!mounted) return;
      if (area == KitchenArea.plans && corporate.isEmpty) {
        await feedback(
          'Add an active corporate customer for this kitchen in CRM first.',
          warning: true,
        );
        return;
      }
      final fields = <PurchaseField>[
        if (area != KitchenArea.settings) ...[
          PurchaseField(
            'name',
            area == KitchenArea.packages ? 'Package name' : 'Plan name',
            required: true,
            initial:
                old['name'] ??
                (mock
                    ? area == KitchenArea.packages
                          ? 'Office lunch package'
                          : 'Weekly office lunch'
                    : null),
          ),
          if (area == KitchenArea.packages) ...[
            PurchaseField(
              'description',
              'Description',
              kind: 'notes',
              initial:
                  old['description'] ??
                  (mock ? 'Fresh meals prepared for your team.' : null),
            ),
            PurchaseField(
              'status',
              'Availability',
              kind: 'select',
              required: true,
              options: const {'DRAFT': 'Draft', 'ACTIVE': 'Active'},
              initial: old['status'] ?? 'DRAFT',
            ),
            PurchaseField(
              'minimum_quantity',
              'Minimum quantity',
              kind: 'number',
              required: true,
              min: 1,
              initial: old['minimum_quantity'] ?? (mock ? 20 : null),
            ),
            PurchaseField(
              'maximum_quantity',
              'Maximum quantity',
              kind: 'number',
              min: 1,
              initial: old['maximum_quantity'],
            ),
            PurchaseField(
              'capacity_pool_id',
              'Capacity pool',
              kind: 'select',
              options: pools,
              initial:
                  old['capacity_pool_id'] ??
                  (mock ? pools.keys.firstOrNull : null),
            ),
            PurchaseField(
              'production_class',
              'Production class',
              initial: old['production_class'],
            ),
            PurchaseField(
              'lead_time_minutes',
              'Lead time (minutes)',
              kind: 'number',
              initial: old['lead_time_minutes'] ?? (mock ? 60 : null),
            ),
            PurchaseField(
              'setup_buffer_minutes',
              'Setup buffer (minutes)',
              kind: 'number',
              required: true,
              max: 480,
              initial: old['setup_buffer_minutes'] ?? 0,
            ),
            PurchaseField(
              'cleanup_buffer_minutes',
              'Cleanup buffer (minutes)',
              kind: 'number',
              required: true,
              max: 480,
              initial: old['cleanup_buffer_minutes'] ?? 0,
            ),
          ] else ...[
            PurchaseField(
              'corporate_profile_ref',
              'Corporate customer',
              kind: 'select',
              options: corporate,
              required: true,
              initial:
                  old['corporate_profile_ref'] ??
                  (mock ? corporate.keys.firstOrNull : null),
            ),
            PurchaseField(
              'package_id',
              'Bulk package',
              kind: 'select',
              required: true,
              options: activePackages,
              initial:
                  old['package_id'] ??
                  (mock ? activePackages.keys.firstOrNull : null),
            ),
            PurchaseField(
              'recurrence_type',
              'Repeat',
              kind: 'select',
              required: true,
              options: const {
                'DAILY': 'Daily',
                'WEEKLY': 'Weekly',
                'SELECTED_WEEKDAYS': 'Selected weekdays',
              },
              initial: old['recurrence_type'] ?? 'WEEKLY',
            ),
            PurchaseField(
              'local_time',
              'Kitchen time',
              kind: 'clock',
              required: true,
              initial: old['local_time'] ?? (mock ? '12:00' : null),
            ),
            PurchaseField(
              'quantity',
              'Quantity',
              kind: 'number',
              required: true,
              min: 1,
              initial: old['quantity'] ?? (mock ? 25 : null),
            ),
            PurchaseField(
              'effective_from',
              'Starts on',
              kind: 'date',
              required: true,
              initial:
                  old['effective_from'] ??
                  (mock ? DateTime.now().toUtc().toIso8601String() : null),
            ),
            PurchaseField(
              'effective_until',
              'Ends on',
              kind: 'date',
              initial: old['effective_until'],
            ),
            PurchaseField(
              'production_minutes',
              'Production time (minutes)',
              kind: 'number',
              required: true,
              min: 1,
              max: 1440,
              initial: old['production_minutes'] ?? 60,
            ),
            PurchaseField(
              'lead_time_minutes',
              'Lead time (minutes)',
              kind: 'number',
              max: 43200,
              initial: old['lead_time_minutes'],
            ),
            PurchaseField(
              'reservation_horizon_days',
              'Reservation horizon (days)',
              kind: 'number',
              min: 1,
              max: 366,
              initial: old['reservation_horizon_days'],
            ),
          ],
        ] else ...[
          PurchaseField(
            'bulk_order_threshold',
            'Bulk order threshold',
            kind: 'number',
            required: true,
            min: 1,
            max: 1000000,
            initial: old['bulk_order_threshold'],
          ),
          PurchaseField(
            'minimum_lead_time_minutes',
            'Minimum lead time (minutes)',
            kind: 'number',
            required: true,
            max: 43200,
            initial: old['minimum_lead_time_minutes'],
          ),
          PurchaseField(
            'default_capacity_pool_ref',
            'Default capacity pool',
            kind: 'select',
            options: pools,
            initial: old['default_capacity_pool_ref'],
          ),
          PurchaseField(
            'reservation_hold_duration_minutes',
            'Reservation hold (minutes)',
            kind: 'number',
            required: true,
            min: 1,
            max: 1440,
            initial: old['reservation_hold_duration_minutes'],
          ),
          PurchaseField(
            'corporate_reservation_horizon_days',
            'Corporate horizon (days)',
            kind: 'number',
            required: true,
            min: 1,
            max: 366,
            initial: old['corporate_reservation_horizon_days'],
          ),
        ],
      ];
      final body = await productionForm(
        context,
        area == KitchenArea.settings
            ? 'Kitchen settings'
            : single
            ? 'Edit ${area == KitchenArea.packages ? 'package' : 'plan'}'
            : 'New ${area == KitchenArea.packages ? 'package' : 'plan'}',
        fields,
        validate: (b) =>
            b['maximum_quantity'] != null &&
                (b['maximum_quantity'] as int) < (b['minimum_quantity'] as int)
            ? 'Maximum quantity must be at least the minimum.'
            : b['effective_until'] != null &&
                  DateTime.parse(
                    '${b['effective_until']}',
                  ).isBefore(DateTime.parse('${b['effective_from']}'))
            ? 'The end date must follow the start date.'
            : null,
      );
      if (body == null || !mounted) return;
      body['branch_id'] = branch;
      if (area == KitchenArea.plans) {
        body['timezone'] =
            '${branches.where((b) => b['id'] == branch).firstOrNull?['timezone'] ?? old['timezone'] ?? ''}';
        final weekdays = await crmChoose(
          context,
          'Production days',
          const {
            '1': 'Monday',
            '2': 'Tuesday',
            '3': 'Wednesday',
            '4': 'Thursday',
            '5': 'Friday',
            '6': 'Saturday',
            '7': 'Sunday',
          },
          multiple: true,
          optional: body['recurrence_type'] != 'SELECTED_WEEKDAYS',
          initial: [
            for (final d in old['weekdays'] as List? ?? (mock ? [1, 3, 5] : []))
              '$d',
          ],
        );
        if (weekdays == null || !mounted) return;
        body['weekdays'] = weekdays.map(int.parse).toList();
      }
      if (area == KitchenArea.packages) {
        body['logistics'] = old['logistics'] ?? <String, dynamic>{};
      }
      if (single) body['version'] = old['version'];
      if (await confirmSettings(
        context,
        'Save ${area == KitchenArea.settings
            ? 'settings'
            : area == KitchenArea.packages
            ? 'package'
            : 'plan'}?',
        area == KitchenArea.plans
            ? 'The plan remains in its current state. Activation is a separate action.'
            : 'Your saved configuration will be used for future production.',
      )) {
        final saved = await change(
          area == KitchenArea.settings
              ? 'PUT'
              : single
              ? 'PATCH'
              : 'POST',
          area == KitchenArea.settings
              ? '$base/{branchId}'
              : single
              ? detail
              : base,
          body,
        );
        if (saved != null && !single && mounted) {
          await open(area, id: '${saved['id']}');
        }
      }
    } catch (e) {
      if (mounted) {
        await feedback(
          'The required choices could not be loaded. Refresh and try again.',
        );
      }
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  Future<String> kitchenCurrency(String branch) async {
    final result = await c.app.cloudKitchenApi.request(
      'GET',
      '$cloudKitchenBase/context/{branchId}',
      pathParameters: {'branchId': branch},
    );
    final value = '${result.object['currency'] ?? ''}';
    if (!RegExp(r'^[A-Z]{3}$').hasMatch(value)) {
      throw const FormatException('Kitchen currency unavailable');
    }
    return value;
  }

  Future<Map<String, String>> corporateChoices(
    String branch,
    JsonObject old,
  ) async {
    final choices = <String, String>{};
    var offset = 0;
    try {
      while (true) {
        final result = await c.app.cloudKitchenApi.request(
          'GET',
          '$cloudKitchenBase/corporate-references',
          query: {'branch_id': branch, 'limit': '200', 'offset': '$offset'},
        );
        final page = result.object;
        for (final row in c.collection(page['items'])) {
          choices['${row['reference']}'] = '${row['display_label']}';
        }
        if (page['has_more'] != true) break;
        final next = page['next_offset'];
        if (next is! int || next <= offset) {
          throw const FormatException('Incomplete corporate selection');
        }
        offset = next;
      }
    } catch (_) {
      if (old['corporate_profile_ref'] == null) rethrow;
    }
    final saved = old['corporate_profile_ref'];
    if (saved != null && '$saved'.isNotEmpty) {
      choices.putIfAbsent('$saved', () => 'Current saved customer (retain)');
    }
    return choices;
  }

  Future<void> editLogistics() async {
    if (preparing || !c.writable || !evidenceReady) return;
    setState(() => preparing = true);
    try {
      final row = c.record!;
      final saved = await Navigator.push<JsonObject>(
        context,
        setupRoute<JsonObject>(
          PastrySavedRequirementsScreen(
            title: 'Package logistics',
            initial: jsonObject(row['logistics']),
            description: 'Edit the saved logistics details for this package.',
            saveLabel: 'Review logistics',
          ),
        ),
      );
      if (saved == null || !mounted) return;
      if (!await confirmSettings(
        context,
        'Save package logistics?',
        'Apply these details to future production.',
      )) {
        return;
      }
      await change('PATCH', detail, {
        for (final key in [
          'name',
          'description',
          'status',
          'minimum_quantity',
          'maximum_quantity',
          'capacity_pool_id',
          'production_class',
          'lead_time_minutes',
          'setup_buffer_minutes',
          'cleanup_buffer_minutes',
          'version',
        ])
          key: row[key],
        'logistics': saved,
      });
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  Future<void> band([JsonObject? row]) async {
    if (preparing || !c.writable || !evidenceReady) return;
    setState(() => preparing = true);
    try {
      final body = await productionForm(
        context,
        row == null ? 'New quantity band' : 'Edit quantity band',
        [
          PurchaseField(
            'min_quantity',
            'From quantity',
            kind: 'number',
            required: true,
            min: 1,
            initial: row?['min_quantity'] ?? (mock ? 20 : null),
          ),
          PurchaseField(
            'max_quantity',
            'To quantity',
            kind: 'number',
            min: 1,
            initial: row?['max_quantity'],
          ),
          PurchaseField(
            'unit_price_minor',
            decimalCurrency
                ? 'Unit price ($currency)'
                : 'Unit price ($currency, minor units)',
            kind: decimalCurrency ? 'money' : 'number',
            required: true,
            initial: row?['unit_price_minor'] ?? (mock ? 250000 : null),
          ),
          PurchaseField(
            'package_price_minor',
            decimalCurrency
                ? 'Package price ($currency)'
                : 'Package price ($currency, minor units)',
            kind: decimalCurrency ? 'money' : 'number',
            required: true,
            initial: row?['package_price_minor'] ?? 0,
          ),
          PurchaseField(
            'lead_time_minutes',
            'Lead time (minutes)',
            kind: 'number',
            initial: row?['lead_time_minutes'],
          ),
          PurchaseField(
            'capacity_units_per_item',
            'Capacity units per item',
            kind: 'number',
            required: true,
            min: 1,
            max: 1000,
            initial: row?['capacity_units_per_item'] ?? 1,
          ),
        ],
        validate: (b) =>
            b['max_quantity'] != null &&
                (b['max_quantity'] as int) < (b['min_quantity'] as int)
            ? 'The upper quantity must be at least the lower quantity.'
            : null,
      );
      if (body == null || !mounted) return;
      if (row == null) {
        body['expected_version'] = c.record!['version'];
      } else {
        body['expected_package_version'] = c.record!['version'];
        body['expected_band_version'] = row['version'];
      }
      if (await confirmSettings(
        context,
        'Save quantity band?',
        'Overlapping active bands are refused by the server.',
      )) {
        await change(
          row == null ? 'POST' : 'PATCH',
          row == null ? '$detail/bands' : '$detail/bands/{bandId}',
          body,
          extra: row == null ? {} : {'bandId': '${row['id']}'},
        );
      }
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  Future<void> component() async {
    if (preparing || !c.writable || !evidenceReady) return;
    setState(() => preparing = true);
    try {
      final items = <JsonObject>[];
      final menus = await list(
        c.app.foodCatalogueApi,
        '/api/v1/vendor/food/menus',
        'menus',
        query: {'branch_id': '${c.record!['branch_id']}'},
      );
      for (final menu in menus.where(
        (m) =>
            m['status'] != 'ARCHIVED' &&
            m['branch_id'] == c.record!['branch_id'],
      )) {
        final sections = await list(
          c.app.foodCatalogueApi,
          '/api/v1/vendor/food/menus/{menuId}/sections',
          'sections',
          parameters: {'menuId': '${menu['id']}'},
        );
        for (final section in sections.where(
          (s) => s['status'] != 'ARCHIVED',
        )) {
          items.addAll(
            await list(
              c.app.foodCatalogueApi,
              '/api/v1/vendor/food/menus/{menuId}/sections/{sectionId}/items',
              'items',
              parameters: {
                'menuId': '${menu['id']}',
                'sectionId': '${section['id']}',
              },
            ),
          );
        }
      }
      if (!mounted) return;
      final selected = await crmChoose(
        context,
        'Choose food item',
        {
          for (final i in items.where((i) => i['status'] != 'ARCHIVED'))
            '${i['id']}': '${i['name']}',
        },
        initial: mock && items.isNotEmpty ? ['${items.first['id']}'] : [],
      );
      if (selected == null || !mounted) return;
      final variants = await list(
        c.app.foodCatalogueApi,
        '/api/v1/vendor/food/items/{itemId}/variants',
        'variants',
        parameters: {'itemId': selected.single},
      );
      if (!mounted) return;
      final body = await productionForm(context, 'Package component', [
        PurchaseField(
          'default_quantity',
          'Quantity per package',
          kind: 'number',
          required: true,
          min: 1,
          initial: mock ? 1 : null,
        ),
        PurchaseField(
          'position',
          'Position',
          kind: 'number',
          required: true,
          initial: components.length,
        ),
        if (variants.any((v) => v['status'] == 'ACTIVE'))
          PurchaseField(
            'variant_ref',
            'Variant',
            kind: 'select',
            options: {
              for (final v in variants.where((v) => v['status'] == 'ACTIVE'))
                '${v['id']}': '${v['name']}',
            },
          ),
      ]);
      if (body == null || !mounted) return;
      body['item_ref'] = selected.single;
      body['expected_version'] = c.record!['version'];
      if (await confirmSettings(
        context,
        'Add component?',
        'This updates the package definition.',
      )) {
        await change('POST', '$detail/components', body);
      }
    } catch (e) {
      if (mounted) {
        await feedback('Catalogue items could not be loaded for this kitchen.');
      }
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  Future<void> transition(String verb, {JsonObject? row, String? child}) async {
    final label = verb == 'remove'
        ? 'Remove component'
        : verb == 'archive'
        ? 'Archive ${child == 'bands'
              ? 'band'
              : area == KitchenArea.packages
              ? 'package'
              : 'plan'}'
        : crmLabel(verb);
    if (await confirmSettings(
      context,
      '$label?',
      verb == 'suspend'
          ? 'Future production is paused. Existing occurrences need their own controls.'
          : 'This updates the saved configuration.',
      danger: ['archive', 'remove', 'suspend'].contains(verb),
    )) {
      await change(
        verb == 'remove' ? 'DELETE' : 'POST',
        child == 'components'
            ? '$detail/components/{componentId}'
            : child == 'bands'
            ? '$detail/bands/{bandId}/archive'
            : '$detail/$verb',
        child == 'bands'
            ? {
                'expected_package_version': c.record!['version'],
                'expected_band_version': row!['version'],
              }
            : {'expected_version': c.record!['version']},
        extra: row == null
            ? {}
            : {child == 'bands' ? 'bandId' : 'componentId': '${row['id']}'},
      );
    }
  }

  Future<void> preview() async {
    if (preparing || !c.writable) return;
    setState(() => preparing = true);
    try {
      final body = await productionForm(
        context,
        'Check package capacity',
        [
          PurchaseField(
            'quantity',
            'Quantity',
            kind: 'number',
            required: true,
            min: 1,
            max: 1000000,
            initial: mock ? c.record!['minimum_quantity'] : null,
          ),
          PurchaseField(
            'start_at',
            'Production starts',
            kind: 'date',
            initial: mock
                ? DateTime.now()
                      .add(const Duration(days: 1))
                      .toUtc()
                      .toIso8601String()
                : null,
          ),
          PurchaseField(
            'end_at',
            'Production ends',
            kind: 'date',
            initial: mock
                ? DateTime.now()
                      .add(const Duration(days: 1, hours: 1))
                      .toUtc()
                      .toIso8601String()
                : null,
          ),
        ],
        validate: (b) =>
            b['start_at'] != null &&
                b['end_at'] != null &&
                !DateTime.parse(
                  '${b['end_at']}',
                ).isAfter(DateTime.parse('${b['start_at']}'))
            ? 'Production end must follow its start.'
            : null,
      );
      if (body == null || !mounted) return;
      final result = jsonObject(
        await c.fetch(
          '$detail/capacity-preview',
          params: params,
          query: {
            for (final e in body.entries)
              if (e.value != null) e.key: '${e.value}',
          },
        ),
      );
      if (!mounted) return;
      await Navigator.push(
        context,
        setupRoute<void>(
          KitchenEvidenceScreen(title: 'Capacity preview', data: result),
        ),
      );
    } catch (e) {
      if (mounted) {
        await feedback(
          'A capacity preview could not be completed. Check active quantity bands, the window and capacity pool.',
        );
      }
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  Future<void> exception() async {
    final body = await productionForm(context, 'Plan exception', [
      PurchaseField(
        'occurrence_date',
        'Occurrence date',
        kind: 'calendar',
        required: true,
        initial: mock
            ? DateTime.now()
                  .add(const Duration(days: 1))
                  .toIso8601String()
                  .substring(0, 10)
            : null,
      ),
      PurchaseField(
        'action',
        'Change',
        kind: 'select',
        required: true,
        options: const {
          'SKIP': 'Skip date',
          'RESCHEDULE': 'Move to another date',
          'QUANTITY_OVERRIDE': 'Change quantity',
          'PACKAGE_OVERRIDE': 'Change package',
        },
        initial: 'SKIP',
      ),
    ]);
    if (body == null || !mounted) return;
    final fields = <PurchaseField>[];
    if (body['action'] == 'RESCHEDULE') {
      fields.add(
        PurchaseField(
          'replacement_date',
          'Replacement date',
          kind: 'calendar',
          required: true,
          initial: mock
              ? DateTime.now()
                    .add(const Duration(days: 2))
                    .toIso8601String()
                    .substring(0, 10)
              : null,
        ),
      );
    }
    if (body['action'] == 'QUANTITY_OVERRIDE') {
      fields.add(
        PurchaseField(
          'quantity_override',
          'Replacement quantity',
          kind: 'number',
          required: true,
          min: 1,
          initial: mock ? 30 : null,
        ),
      );
    }
    if (body['action'] == 'PACKAGE_OVERRIDE') {
      final packages = await list(
        c.app.cloudKitchenApi,
        '$cloudKitchenBase/bulk-packages',
        'items',
      );
      if (!mounted) return;
      fields.add(
        PurchaseField(
          'package_override',
          'Replacement package',
          kind: 'select',
          required: true,
          options: {
            for (final p in packages.where(
              (p) =>
                  p['branch_id'] == c.record!['branch_id'] &&
                  p['status'] == 'ACTIVE',
            ))
              '${p['id']}': '${p['name']}',
          },
        ),
      );
    }
    if (fields.isNotEmpty) {
      final details = await productionForm(
        context,
        'Exception details',
        fields,
      );
      if (details == null || !mounted) return;
      body.addAll(details);
    }
    if (await confirmSettings(
      context,
      'Save date exception?',
      'This changes future materialization for that date. Use occurrence controls for an occurrence already created.',
    )) {
      await change('POST', '$detail/exceptions', body);
    }
  }

  Future<void> occurrence(JsonObject row, String verb) async {
    final body = await productionForm(
      context,
      verb == 'reschedule' ? 'Reschedule occurrence' : crmLabel(verb),
      [
        if (verb == 'reschedule') ...[
          PurchaseField(
            'starts_at',
            'Production starts',
            kind: 'date',
            required: true,
            initial: row['starts_at'],
          ),
          PurchaseField(
            'ends_at',
            'Production ends',
            kind: 'date',
            required: true,
            initial: row['ends_at'],
          ),
        ] else
          PurchaseField(
            'reason',
            'Reason',
            kind: 'notes',
            required: verb == 'cancel',
            initial: mock ? 'Kitchen schedule updated' : null,
          ),
      ],
      validate: (b) =>
          verb == 'reschedule' &&
              !DateTime.parse(
                '${b['ends_at']}',
              ).isAfter(DateTime.parse('${b['starts_at']}'))
          ? 'The end must follow the start.'
          : '${b['reason'] ?? ''}'.length > 280
          ? 'Keep the reason within 280 characters.'
          : null,
    );
    if (body == null || !mounted) return;
    body['expected_version'] = row['version'];
    if (await confirmSettings(
      context,
      '${crmLabel(verb)} occurrence?',
      verb == 'cancel'
          ? 'Linked reservations and unstarted production are handled by the server.'
          : 'The server controls recovery and reservation state.',
      danger: verb == 'cancel',
    )) {
      await change(
        'POST',
        '$base/{occurrenceId}/$verb',
        body,
        extra: {'occurrenceId': '${row['id']}'},
      );
    }
  }

  Widget stateLabel(Object? value) => Text(
    crmLabel(value),
    style: TextStyle(
      fontWeight: FontWeight.w700,
      color:
          (['CAPACITY_CONFLICT', 'FAILED'].contains(value)
                  ? KarryamColors.error
                  : ['SUSPENDED', 'PLANNED', 'DRAFT'].contains(value)
                  ? const Color(0xFFA66E19)
                  : ['ACTIVE', 'RESERVED', 'COMPLETED'].contains(value)
                  ? KarryamColors.tealInk
                  : const Color(0xFF738292))
              .forInk(context),
    ),
  );
  Widget button(
    String label,
    VoidCallback onTap, {
    bool secondary = false,
    bool danger = false,
    IconData icon = Icons.arrow_forward_rounded,
    String? route,
  }) => SettingsAction(
    label,
    onPressed: c.writable && evidenceReady && !preparing ? onTap : null,
    busy: route != null && action == route,
    secondary: secondary,
    danger: danger,
    icon: icon,
  );
  List<Widget> detailWidgets() {
    final row = c.record!;
    if (area == KitchenArea.settings) {
      return [
        SettingsCard(child: Column(children: kitchenFacts(row))),
        button('Edit settings', edit, icon: Icons.tune_rounded),
      ];
    }
    if (area == KitchenArea.plans) {
      return [
        SettingsCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                '${row['name']}',
                style: const TextStyle(
                  fontSize: 25,
                  fontWeight: FontWeight.w800,
                ),
              ),
              const SizedBox(height: 8),
              stateLabel(row['status']),
              const SizedBox(height: 16),
              ...kitchenFacts(row),
            ],
          ),
        ),
        if (['DRAFT', 'SUSPENDED'].contains(row['status']))
          button('Edit plan', edit, secondary: true, icon: Icons.edit_outlined),
        if (row['status'] == 'DRAFT')
          button(
            'Activate plan',
            () => transition('activate'),
            route: '$detail/activate',
            icon: Icons.play_arrow_rounded,
          ),
        if (row['status'] == 'ACTIVE')
          button(
            'Suspend plan',
            () => transition('suspend'),
            route: '$detail/suspend',
            secondary: true,
            icon: Icons.pause_rounded,
          ),
        if (row['status'] == 'SUSPENDED')
          button(
            'Resume plan',
            () => transition('resume'),
            route: '$detail/resume',
            icon: Icons.play_arrow_rounded,
          ),
        SettingsLink(
          'Occurrences',
          Icons.calendar_month_outlined,
          onTap: () => open(KitchenArea.occurrences, planId: widget.id),
        ),
        if (row['status'] != 'ARCHIVED')
          button(
            'Add date exception',
            exception,
            secondary: true,
            icon: Icons.event_note_outlined,
          ),
        if (row['status'] != 'ARCHIVED')
          button(
            'Archive plan',
            () => transition('archive'),
            danger: true,
            route: '$detail/archive',
            icon: Icons.archive_outlined,
          ),
      ];
    }
    return [
      SettingsCard(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(
              area.icon,
              size: 40,
              color: KarryamColors.tealInk.forInk(context),
            ),
            const SizedBox(height: 14),
            Text(
              '${row['name']}',
              style: const TextStyle(fontSize: 25, fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 8),
            stateLabel(row['status']),
            const SizedBox(height: 16),
            ...kitchenFacts(row),
          ],
        ),
      ),
      if (row['status'] != 'ARCHIVED')
        button(
          'Edit package',
          edit,
          secondary: true,
          icon: Icons.edit_outlined,
        ),
      if (jsonObject(row['logistics'] ?? {}).isNotEmpty &&
          row['status'] != 'ARCHIVED')
        button(
          'Edit logistics',
          editLogistics,
          secondary: true,
          icon: Icons.local_shipping_outlined,
        ),
      const SettingsHeading('What is included'),
      if (components.isEmpty)
        const SettingsNote('Add catalogue items to define the package.'),
      for (final item in components)
        SettingsCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                itemNames['${item['item_ref']}'] ?? 'Catalogue item',
                style: const TextStyle(
                  fontSize: 17,
                  fontWeight: FontWeight.w700,
                ),
              ),
              ...kitchenFacts(item),
              if (row['status'] != 'ARCHIVED')
                button(
                  'Remove component',
                  () => transition('remove', row: item, child: 'components'),
                  secondary: true,
                  danger: true,
                  icon: Icons.remove_circle_outline,
                ),
            ],
          ),
        ),
      if (row['status'] != 'ARCHIVED')
        button(
          'Add food item',
          component,
          secondary: true,
          icon: Icons.add_rounded,
        ),
      const SettingsHeading('Quantity pricing'),
      if (bands.isEmpty)
        const SettingsNote(
          'Add a quantity band before checking package capacity.',
        ),
      for (final bandRow in bands.where((b) => b['status'] != 'ARCHIVED'))
        SettingsCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                '${bandRow['min_quantity']}?${bandRow['max_quantity'] ?? 'Any'} items',
                style: const TextStyle(
                  fontSize: 18,
                  fontWeight: FontWeight.w700,
                ),
              ),
              ...kitchenFacts({
                for (final e in bandRow.entries)
                  if (![
                    'unit_price_minor',
                    'package_price_minor',
                  ].contains(e.key))
                    e.key: e.value,
              }),
              SettingsFact('Unit price', price(bandRow['unit_price_minor'])),
              SettingsFact(
                'Package price',
                price(bandRow['package_price_minor']),
              ),
              if (row['status'] != 'ARCHIVED') ...[
                button(
                  'Edit band',
                  () => band(bandRow),
                  secondary: true,
                  icon: Icons.edit_outlined,
                ),
                button(
                  'Archive band',
                  () => transition('archive', row: bandRow, child: 'bands'),
                  secondary: true,
                  danger: true,
                  icon: Icons.archive_outlined,
                ),
              ],
            ],
          ),
        ),
      if (row['status'] != 'ARCHIVED')
        button(
          'Add quantity band',
          band,
          secondary: true,
          icon: Icons.add_rounded,
        ),
      if (row['status'] == 'ACTIVE')
        button('Check capacity', preview, icon: Icons.donut_large_rounded),
      if (row['status'] != 'ARCHIVED')
        button(
          'Archive package',
          () => transition('archive'),
          danger: true,
          route: '$detail/archive',
          icon: Icons.archive_outlined,
        ),
    ];
  }

  Widget occurrenceCard(JsonObject row) => SettingsCard(
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Icon(
              Icons.event_outlined,
              color: KarryamColors.tealInk.forInk(context),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                supportDate(row['occurrence_date']),
                style: const TextStyle(
                  fontSize: 19,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ),
          ],
        ),
        const SizedBox(height: 12),
        stateLabel(row['state']),
        ...kitchenFacts(row),
        if (row['production_job_id'] != null)
          SettingsLink(
            'Production job',
            Icons.precision_manufacturing_outlined,
            onTap: () => Navigator.push(
              context,
              setupRoute<void>(
                ProductionOperationsScreen(
                  runtime: widget.runtime,
                  jobId: '${row['production_job_id']}',
                ),
              ),
            ),
          ),
        if (row['reservation_id'] != null)
          SettingsLink(
            'Capacity reservation',
            Icons.bookmarks_outlined,
            onTap: () => Navigator.push(
              context,
              setupRoute<void>(
                ProductionReservationScreen(
                  runtime: widget.runtime,
                  reservationId: '${row['reservation_id']}',
                ),
              ),
            ),
          ),
        if (row['dead_lettered_at'] != null)
          button(
            'Replay occurrence',
            () => occurrence(row, 'replay'),
            secondary: true,
            icon: Icons.replay_rounded,
          )
        else if (row['state'] == 'CAPACITY_CONFLICT')
          button(
            'Retry reservation',
            () => occurrence(row, 'retry'),
            secondary: true,
            icon: Icons.refresh_rounded,
          ),
        if (['PLANNED', 'CAPACITY_CONFLICT'].contains(row['state']))
          button(
            'Reschedule',
            () => occurrence(row, 'reschedule'),
            secondary: true,
            icon: Icons.edit_calendar_outlined,
          ),
        if (!['COMPLETED', 'CANCELLED'].contains(row['state']))
          button(
            'Cancel occurrence',
            () => occurrence(row, 'cancel'),
            danger: true,
            icon: Icons.close_rounded,
          ),
      ],
    ),
  );
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
                physics: const AlwaysScrollableScrollPhysics(),
                padding: const EdgeInsets.fromLTRB(20, 12, 20, 32),
                children: [
                  if (area == KitchenArea.settings && branches.isNotEmpty)
                    Padding(
                      padding: const EdgeInsets.only(bottom: 16),
                      child: DropdownButtonFormField<String>(
                        initialValue: branchId,
                        isExpanded: true,
                        items: [
                          for (final b in branches)
                            DropdownMenuItem(
                              value: '${b['id']}',
                              child: Text('${b['name']}'),
                            ),
                        ],
                        onChanged: c.loading || preparing || readingCloud
                            ? null
                            : (value) {
                                setState(() {
                                  branchId = value;
                                  c.data = null;
                                  c.record = null;
                                });
                                load();
                              },
                        decoration: const InputDecoration(
                          labelText: 'Kitchen branch',
                        ),
                      ),
                    ),
                  if (!single && area != KitchenArea.occurrences) ...[
                    Text(
                      area == KitchenArea.packages
                          ? 'Prepared for scale'
                          : 'A dependable schedule',
                      style: const TextStyle(
                        fontSize: 26,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                    const SizedBox(height: 8),
                    Text(
                      area == KitchenArea.packages
                          ? 'Build packages with catalogue items and quantity pricing.'
                          : 'Plan recurring production for your corporate customers.',
                    ),
                    const SizedBox(height: 20),
                    button(
                      area == KitchenArea.packages ? 'New package' : 'New plan',
                      edit,
                      icon: Icons.add_rounded,
                    ),
                    const SizedBox(height: 16),
                    SingleChildScrollView(
                      scrollDirection: Axis.horizontal,
                      child: Row(
                        children: [
                          for (final value in [
                            'ACTIVE',
                            'DRAFT',
                            'SUSPENDED',
                            'ARCHIVED',
                            'ALL',
                          ])
                            if (area == KitchenArea.plans ||
                                value != 'SUSPENDED')
                              Padding(
                                padding: const EdgeInsets.only(right: 8),
                                child: ChoiceChip(
                                  label: Text(crmLabel(value)),
                                  selected: filter == value,
                                  onSelected: (_) =>
                                      setState(() => filter = value),
                                ),
                              ),
                        ],
                      ),
                    ),
                    const SizedBox(height: 16),
                  ],
                  if (readingCloud &&
                      c.data == null &&
                      readError == null &&
                      c.error == null)
                    const PremiumSkeleton(detail: true),
                  if (readError != null || c.error != null) ...[
                    SettingsNote(readError ?? c.error!),
                    SettingsAction(
                      'Refresh',
                      secondary: true,
                      onPressed: c.loading ? null : load,
                    ),
                  ],
                  if (c.pending != null) ...[
                    const SettingsNote(
                      'A saved change needs confirmation before another edit.',
                      warning: true,
                    ),
                    SettingsAction(
                      'Retry saved change',
                      busy: c.busy,
                      onPressed: c.busy
                          ? null
                          : () async {
                              final saved = await c.save();
                              if (mounted) {
                                await feedback(
                                  saved == null
                                      ? c.error ?? 'Still unconfirmed.'
                                      : 'Your change is confirmed.',
                                  success: saved != null,
                                );
                                if (mounted) await load();
                              }
                            },
                    ),
                  ],
                  if (c.data != null && !single) ...[
                    if (c.rows
                        .where(
                          (r) =>
                              filter == 'ALL' ||
                              area == KitchenArea.occurrences ||
                              r['status'] == filter,
                        )
                        .isEmpty)
                      SettingsCard(
                        child: Padding(
                          padding: const EdgeInsets.symmetric(vertical: 28),
                          child: Column(
                            children: [
                              Icon(
                                area.icon,
                                size: 60,
                                color: const Color(0xFFA76D25).forInk(context),
                              ),
                              const SizedBox(height: 16),
                              Text(
                                area == KitchenArea.occurrences
                                    ? 'Your production calendar'
                                    : 'Nothing here yet',
                                style: const TextStyle(
                                  fontSize: 21,
                                  fontWeight: FontWeight.w700,
                                ),
                              ),
                              const SizedBox(height: 8),
                              Text(
                                area == KitchenArea.occurrences
                                    ? 'Activated plans are materialized by the server. Refresh to follow their progress.'
                                    : 'Create a record or choose another status.',
                                textAlign: TextAlign.center,
                              ),
                            ],
                          ),
                        ),
                      ),
                    for (final row in c.rows.where(
                      (r) =>
                          filter == 'ALL' ||
                          area == KitchenArea.occurrences ||
                          r['status'] == filter,
                    ))
                      Padding(
                        padding: const EdgeInsets.only(bottom: 12),
                        child: area == KitchenArea.occurrences
                            ? occurrenceCard(row)
                            : SettingsCard(
                                child: SettingsLink(
                                  '${row['name']}',
                                  area.icon,
                                  subtitle:
                                      '${crmLabel(row['status'])} ? ${branches.where((b) => b['id'] == row['branch_id']).firstOrNull?['name'] ?? 'Kitchen'}',
                                  onTap: () => open(area, id: '${row['id']}'),
                                ),
                              ),
                      ),
                  ],
                  if (single && c.record != null)
                    for (final child in detailWidgets())
                      Padding(
                        padding: const EdgeInsets.only(bottom: 14),
                        child: child,
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

List<Widget> kitchenFacts(JsonObject data, {int depth = 0}) => [
  for (final e in data.entries.where(
    (e) => ![
      'id',
      'branch_id',
      'version',
      'name',
      'status',
      'state',
      'item_ref',
      'variant_ref',
      'package_id',
      'capacity_pool_id',
      'production_job_id',
      'reservation_id',
      'occurrence_key',
      'lease_owner',
      'lease_expires_at',
      'corporate_profile_ref',
      'default_capacity_pool_ref',
    ].contains(e.key),
  ))
    if (e.value is Map && depth < 2) ...[
      SettingsHeading(crmLabel(e.key)),
      ...kitchenFacts(jsonObject(e.value), depth: depth + 1),
    ] else if (e.value is List) ...[
      SettingsFact(
        crmLabel(e.key),
        (e.value as List)
            .map((v) => v is Map ? crmLabel(v['name'] ?? v['quantity']) : '$v')
            .join(', '),
      ),
    ] else
      SettingsFact(
        crmLabel(e.key),
        e.value == null
            ? 'Not set'
            : e.value is bool
            ? e.value == true
                  ? 'Yes'
                  : 'No'
            : e.key.endsWith('_at') ||
                  e.key.endsWith('_from') ||
                  e.key.endsWith('_until') ||
                  e.key == 'occurrence_date'
            ? supportDate(e.value)
            : e.value is String &&
                  RegExp(r'^[A-Z][A-Z0-9_]*$').hasMatch(e.value)
            ? crmLabel(e.value)
            : '${e.value}',
      ),
];

class KitchenEvidenceScreen extends StatelessWidget {
  const KitchenEvidenceScreen({
    super.key,
    required this.title,
    required this.data,
  });
  final String title;
  final JsonObject data;
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: Text(title)),
    body: SafeArea(
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 680),
          child: ListView(
            padding: const EdgeInsets.all(20),
            children: [
              const SettingsNote(
                'This preview does not reserve capacity or create an order.',
              ),
              SettingsCard(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: kitchenFacts(data),
                ),
              ),
            ],
          ),
        ),
      ),
    ),
  );
}
