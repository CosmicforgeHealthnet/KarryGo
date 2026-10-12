import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../../settings/state/settings_controller.dart';
import '../../settings/ui/settings_design.dart';
import '../../support/ui/order_support_screen.dart' show supportDate;
import '../state/production_operations_controller.dart';
import 'production_form_screen.dart';
import 'production_reservation_screen.dart';

enum ProductionControl {
  pools,
  lines,
  rules,
  slots,
  templates,
  blackouts,
  alerts,
}

extension ProductionControlInfo on ProductionControl {
  String get collection =>
      this == ProductionControl.templates ? 'slot-templates' : name;
  String get title => switch (this) {
    ProductionControl.pools => 'Capacity pools',
    ProductionControl.lines => 'Production lines',
    ProductionControl.rules => 'Capacity rules',
    ProductionControl.slots => 'Production slots',
    ProductionControl.templates => 'Slot templates',
    ProductionControl.blackouts => 'Blackouts',
    ProductionControl.alerts => 'Operational alerts',
  };
  String get idKey => switch (this) {
    ProductionControl.pools => 'poolId',
    ProductionControl.lines => 'lineId',
    ProductionControl.rules => 'ruleId',
    ProductionControl.slots => 'slotId',
    ProductionControl.templates => 'templateId',
    ProductionControl.blackouts => 'blackoutId',
    ProductionControl.alerts => 'alertId',
  };
  IconData get icon => switch (this) {
    ProductionControl.pools => Icons.donut_large_rounded,
    ProductionControl.lines => Icons.precision_manufacturing_outlined,
    ProductionControl.rules => Icons.rule_rounded,
    ProductionControl.slots => Icons.calendar_month_rounded,
    ProductionControl.templates => Icons.event_repeat_rounded,
    ProductionControl.blackouts => Icons.event_busy_rounded,
    ProductionControl.alerts => Icons.notification_important_outlined,
  };
  String get purpose => switch (this) {
    ProductionControl.pools => 'Resources and production limits',
    ProductionControl.lines => 'Stations, classes and preparation buffers',
    ProductionControl.rules => 'Limits for specific days and times',
    ProductionControl.slots => 'Open, close and manage production windows',
    ProductionControl.templates => 'Reusable weekly production schedules',
    ProductionControl.blackouts =>
      'Unavailable time for closures and maintenance',
    ProductionControl.alerts => 'Production issues that need attention',
  };
}

const productionCapabilities = {
  'RESTAURANT': 'Restaurant',
  'FAST_FOOD': 'Fast food',
  'CLOUD_KITCHEN': 'Cloud kitchen',
  'PASTRY_BAKERY': 'Pastry & bakery',
};

class ProductionControlsHub extends StatelessWidget {
  const ProductionControlsHub({super.key, required this.runtime});
  final VendorRuntime runtime;
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Capacity controls')),
    body: SafeArea(
      child: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          for (final kind in ProductionControl.values)
            SettingsLink(
              kind.title,
              kind.icon,
              subtitle: kind.purpose,
              onTap: () => Navigator.push(
                context,
                setupRoute<void>(
                  ProductionControlsScreen(runtime: runtime, kind: kind),
                ),
              ),
            ),
          SettingsLink(
            'Reservations',
            Icons.bookmarks_outlined,
            onTap: () => Navigator.push(
              context,
              setupRoute<void>(ProductionReservationScreen(runtime: runtime)),
            ),
          ),
        ],
      ),
    ),
  );
}

class ProductionControlsScreen extends StatefulWidget {
  const ProductionControlsScreen({
    super.key,
    required this.runtime,
    required this.kind,
    this.id,
  });
  final VendorRuntime runtime;
  final ProductionControl kind;
  final String? id;
  @override
  State<ProductionControlsScreen> createState() =>
      _ProductionControlsScreenState();
}

class _ProductionControlsScreenState extends State<ProductionControlsScreen> {
  ProductionControl get kind => widget.kind;
  bool get collectionDetail =>
      [ProductionControl.blackouts, ProductionControl.alerts].contains(kind);
  String get base => '$productionBase/${kind.collection}';
  String get detail => '$base/{${kind.idKey}}';
  late final c = ProductionOperationsController(
    widget.runtime,
    widget.id == null || collectionDetail ? base : detail,
    parameters: {if (widget.id != null) kind.idKey: widget.id!},
  );
  JsonObject? get record {
    if (!collectionDetail) return c.record;
    for (final row in c.rows) {
      if (row['id'] == widget.id) return row;
    }
    return null;
  }

  bool preparing = false;
  String? action;
  @override
  void initState() {
    super.initState();
    c.addListener(changed);
    c.load();
  }

  void changed() {
    if (mounted) setState(() {});
  }

  @override
  void dispose() {
    c.removeListener(changed);
    c.dispose();
    super.dispose();
  }

  Future<void> save(
    String method,
    String route,
    JsonObject body, {
    JsonObject? row,
  }) async {
    setState(() => action = route);
    final result = await c.save(
      method: method,
      route: route,
      body: body,
      params: row == null ? {} : {kind.idKey: '${row['id']}'},
    );
    if (!mounted) return;
    if (result != null) {
      await showOrderFeedback(
        context,
        title: route.endsWith('/generate') ? 'Slots generated' : 'Saved',
        message: route.endsWith('/generate')
            ? '${result['created']} production slots created.'
            : 'Your production settings have been updated.',
        success: true,
      );
      await c.load();
    }
    if (mounted) setState(() => action = null);
  }

  Future<void> edit([JsonObject? row]) async {
    if (preparing) return;
    setState(() => preparing = true);
    final discovery = SettingsController(
      widget.runtime.backend,
      SettingsKind.branches,
    );
    try {
      JsonObject scope = row ?? {};
      if (row == null) {
        final branches = await discovery.all(
          '${SettingsController.business}/branches',
          discovery.scope,
        );
        if (!mounted) return;
        final chosen = await purchaseForm(context, 'Choose production scope', [
          PurchaseField(
            'branch_id',
            'Branch',
            kind: 'select',
            required: true,
            options: {
              for (final b in branches) '${b['id']}': '${b['name'] ?? b['id']}',
            },
          ),
          PurchaseField(
            'capability',
            'Business type',
            kind: 'select',
            required: kind != ProductionControl.blackouts,
            options: productionCapabilities,
          ),
        ]);
        if (chosen == null) return;
        scope = chosen;
      }
      final refs = await Future.wait([
        c.fetch('$productionBase/pools'),
        c.fetch('$productionBase/lines'),
      ]);
      if (!mounted) return;
      final pools = c
          .collection(refs[0])
          .where(
            (p) =>
                p['branch_id'] == scope['branch_id'] &&
                p['capability'] == scope['capability'] &&
                (p['status'] == 'ACTIVE' ||
                    p['id'] == row?['capacity_pool_id']),
          )
          .toList();
      final lines = c
          .collection(refs[1])
          .where(
            (p) =>
                p['branch_id'] == scope['branch_id'] &&
                p['capability'] == scope['capability'] &&
                (p['status'] == 'ACTIVE' ||
                    p['id'] == row?['production_line_id']),
          )
          .toList();
      final fields = fieldsFor(scope, row, pools, lines);
      final body = await productionForm(
        context,
        row == null
            ? 'New ${kind.title.toLowerCase()}'
            : 'Edit ${kind.title.toLowerCase()}',
        fields,
        validate: (b) => validateFields(b),
      );
      if (body == null) return;
      final payload = <String, dynamic>{
        ...body,
        'branch_id': scope['branch_id'],
        'capability': scope['capability'],
        if (row != null) 'expected_version': row['version'],
      };
      if (kind == ProductionControl.templates && row != null) {
        payload['capacity_pool_id'] = row['capacity_pool_id'];
        payload['production_line_id'] = row['production_line_id'];
      }
      if (kind == ProductionControl.rules) {
        for (final key in [
          'start_local_time',
          'end_local_time',
          'effective_start_date',
          'effective_end_date',
        ]) {
          payload[key] ??= '';
        }
      }
      if (kind == ProductionControl.lines) {
        payload['capacity_pool_id'] = body['capacity_pool_id'];
      }
      if (kind == ProductionControl.templates) {
        payload['timezone'] = row?['timezone'] ?? '';
      }
      await save(
        row == null ? 'POST' : 'PATCH',
        row == null ? base : detail,
        payload,
        row: row,
      );
    } catch (_) {
      if (mounted) {
        setState(
          () => c.error =
              'Production references could not be loaded. Refresh and try again.',
        );
      }
    } finally {
      discovery.dispose();
      if (mounted) setState(() => preparing = false);
    }
  }

  List<PurchaseField> fieldsFor(
    JsonObject scope,
    JsonObject? row,
    List<JsonObject> pools,
    List<JsonObject> lines,
  ) {
    final mock = row == null && widget.runtime.mode == VendorDataMode.mock;
    final defaults = <String, Object>{
      'name': kind == ProductionControl.pools
          ? 'Kitchen capacity'
          : 'Preparation station',
      'code': kind == ProductionControl.pools ? 'KITCHEN_2' : 'PREPARATION',
      'label': 'Daily production',
      'start_local_time': '09:00',
      'end_local_time': '17:00',
      'note': 'Scheduled maintenance',
      'start_at': DateTime.now()
          .toUtc()
          .add(const Duration(days: 1))
          .toIso8601String(),
      'end_at': DateTime.now()
          .toUtc()
          .add(const Duration(days: 1, hours: 1))
          .toIso8601String(),
    };
    PurchaseField text(
      String key,
      String label, {
      bool required = false,
      String type = 'text',
    }) => PurchaseField(
      key,
      label,
      required: required,
      kind: type,
      initial: type == 'clock' && row?[key] != null
          ? '${row![key]}'.substring(0, 5)
          : row?[key] ?? (mock ? defaults[key] : null),
    );
    PurchaseField number(
      String key,
      String label, {
      int max = 1000000,
      bool required = true,
      Object? initial = 0,
    }) => PurchaseField(
      key,
      label,
      kind: 'number',
      required: required,
      max: max,
      initial: row?[key] ?? (mock && key == 'default_capacity' ? 80 : initial),
    );
    final pool = PurchaseField(
      'capacity_pool_id',
      'Capacity pool',
      kind: 'select',
      required: ![
        ProductionControl.lines,
        ProductionControl.blackouts,
      ].contains(kind),
      initial: row?['capacity_pool_id'],
      options: {for (final p in pools) '${p['id']}': '${p['name']}'},
    );
    final line = PurchaseField(
      'production_line_id',
      'Production line',
      kind: 'select',
      initial: row?['production_line_id'],
      options: {for (final p in lines) '${p['id']}': '${p['name']}'},
    );
    final buffers = [
      number('setup_buffer_minutes', 'Setup buffer (minutes)', max: 480),
      number('cleanup_buffer_minutes', 'Cleanup buffer (minutes)', max: 480),
    ];
    final calendars = [
      text('effective_start_date', 'Effective start date', type: 'calendar'),
      text('effective_end_date', 'Effective end date', type: 'calendar'),
    ];
    switch (kind) {
      case ProductionControl.pools:
        return [
          text('name', 'Pool name', required: true),
          if (row == null) ...[
            text('code', 'Pool code', required: true),
            const PurchaseField(
              'scope_type',
              'Scope',
              kind: 'select',
              required: true,
              initial: 'CAPABILITY',
              options: {
                'BRANCH': 'Branch',
                'CAPABILITY': 'Business type',
                'PRODUCTION_LINE': 'Production line',
                'ITEM_FAMILY': 'Item family',
                'CUSTOM_CLASS': 'Custom class',
              },
            ),
            text('scope_ref', 'Scope reference'),
            const PurchaseField(
              'unit_type',
              'Unit',
              kind: 'select',
              required: true,
              initial: 'PRODUCTION_UNIT',
              options: {
                'MEAL': 'Meal',
                'ITEM': 'Item',
                'TRAY': 'Tray',
                'PACKAGE': 'Package',
                'PRODUCTION_UNIT': 'Production unit',
                'WORKLOAD_UNIT': 'Workload unit',
              },
            ),
          ],
          number('default_capacity', 'Default capacity'),
          ...buffers,
        ];
      case ProductionControl.lines:
        return [
          text('name', 'Line name', required: true),
          text('code', 'Line code', required: true),
          PurchaseField(
            'production_class',
            'Production class',
            kind: 'select',
            required: true,
            initial: row?['production_class'] ?? 'GENERAL',
            options: {
              for (final value in [
                'GENERAL',
                'HOT',
                'COLD',
                'BAKERY',
                'DECORATION',
                'PACKING',
                'CHILLING',
              ])
                value: dashboardLabel(value),
            },
          ),
          pool,
          ...buffers,
        ];
      case ProductionControl.rules:
        return [
          pool,
          line,
          text('item_family', 'Item family'),
          number('max_quantity', 'Maximum quantity'),
          PurchaseField(
            'days_of_week',
            'Days of week',
            kind: 'days',
            initial: row?['days_of_week'] ?? [],
          ),
          text('start_local_time', 'Local start time', type: 'clock'),
          text('end_local_time', 'Local end time', type: 'clock'),
          ...calendars,
          number('specificity', 'Specificity', max: 100),
        ];
      case ProductionControl.templates:
        return [
          text('label', 'Template name', required: true),
          if (row == null) ...[pool, line],
          PurchaseField(
            'days_of_week',
            'Days of week',
            kind: 'days',
            required: true,
            initial: row?['days_of_week'] ?? [1, 2, 3, 4, 5],
          ),
          text(
            'start_local_time',
            'Local start time',
            type: 'clock',
            required: true,
          ),
          text(
            'end_local_time',
            'Local end time',
            type: 'clock',
            required: true,
          ),
          number(
            'capacity_override',
            'Capacity override',
            required: false,
            initial: null,
          ),
          ...calendars,
          PurchaseField(
            'generation_horizon_days',
            'Generation horizon (days)',
            kind: 'number',
            min: 1,
            max: 366,
            required: true,
            initial: row?['generation_horizon_days'] ?? 30,
          ),
        ];
      case ProductionControl.slots:
        return [
          pool,
          line,
          text('label', 'Slot name', required: true),
          text('start_at', 'Start', type: 'date', required: true),
          text('end_at', 'End', type: 'date', required: true),
          number(
            'capacity_override',
            'Capacity override',
            required: false,
            initial: null,
          ),
        ];
      case ProductionControl.blackouts:
        return [
          pool,
          line,
          text('start_at', 'Start', type: 'date', required: true),
          text('end_at', 'End', type: 'date', required: true),
          const PurchaseField(
            'reason_code',
            'Reason',
            kind: 'select',
            required: true,
            options: {
              'MAINTENANCE': 'Maintenance',
              'STAFF_SHORTAGE': 'Staff shortage',
              'PRIVATE_PRODUCTION': 'Private production',
              'HOLIDAY': 'Holiday',
              'EQUIPMENT_FAILURE': 'Equipment failure',
              'CLOSURE': 'Closure',
              'OTHER': 'Other',
            },
          ),
          text('note', 'Note', type: 'notes'),
        ];
      case ProductionControl.alerts:
        return [];
    }
  }

  String? validateFields(JsonObject body) {
    if (body['name'] != null &&
        ('${body['name']}'.length < 2 || '${body['name']}'.length > 160)) {
      return 'Names must contain 2 to 160 characters.';
    }
    if (body['code'] != null &&
        !RegExp(r'^[A-Za-z0-9_]{2,60}$').hasMatch('${body['code']}')) {
      return 'Use 2 to 60 letters, numbers or underscores.';
    }
    if (body['start_at'] != null &&
        !DateTime.parse(
          body['end_at'],
        ).isAfter(DateTime.parse(body['start_at']))) {
      return 'End must be after start.';
    }
    final start = body['start_local_time'], end = body['end_local_time'];
    if ((start == null) != (end == null) || start != null && start == end) {
      return 'Choose distinct start and end local times.';
    }
    if (body['effective_start_date'] != null &&
        body['effective_end_date'] != null &&
        '${body['effective_end_date']}'.compareTo(
              '${body['effective_start_date']}',
            ) <
            0) {
      return 'Effective end date must not precede start date.';
    }
    return null;
  }

  Future<void> change(JsonObject row, String verb) async {
    JsonObject extra = {};
    if (kind == ProductionControl.alerts) {
      final note = await purchaseForm(
        context,
        dashboardLabel(verb),
        [const PurchaseField('note', 'Note', kind: 'notes')],
        validate: (b) => '${b['note']}'.length > 280
            ? 'Keep notes within 280 characters.'
            : null,
      );
      if (note == null) return;
      extra = note;
    } else if (!await confirmSettings(
      context,
      '${dashboardLabel(verb)}?',
      verb == 'generate'
          ? 'Create slots from this template in its configured timezone and date range.'
          : 'Update this production resource?',
      danger: ['archive', 'cancel', 'blackout', 'close'].contains(verb),
    )) {
      return;
    }
    await save('POST', '$detail/$verb', {
      ...extra,
      if (verb != 'generate') 'expected_version': row['version'],
    }, row: row);
  }

  List<String> actions(JsonObject row) => row['status'] == 'ARCHIVED'
      ? []
      : switch (kind) {
          ProductionControl.slots => [
            if (row['status'] != 'OPEN') 'open',
            if (row['status'] != 'CLOSED') 'close',
            if (row['status'] != 'BLACKOUT') 'blackout',
            'archive',
          ],
          ProductionControl.templates => ['generate', 'archive'],
          ProductionControl.blackouts =>
            row['status'] == 'ACTIVE' ? ['cancel'] : [],
          ProductionControl.alerts => [
            if (row['status'] == 'OPEN') 'acknowledge',
            if (row['status'] != 'RESOLVED') 'resolve',
          ],
          _ => ['archive'],
        };
  Future<void> open(JsonObject row) async {
    await Navigator.push(
      context,
      setupRoute<void>(
        ProductionControlsScreen(
          runtime: widget.runtime,
          kind: kind,
          id: '${row['id']}',
        ),
      ),
    );
    await c.load();
  }

  String rowTitle(JsonObject row) =>
      '${row['name'] ?? row['label'] ?? (kind == ProductionControl.alerts ? dashboardLabel('${row['alert_type']}') : dashboardLabel('${row['reason_code']}'))}';
  @override
  Widget build(BuildContext context) {
    final row = record;
    return Scaffold(
      appBar: AppBar(
        title: Text(kind.title),
        actions: [
          IconButton(
            tooltip: 'Refresh',
            onPressed: c.busy || c.loading ? null : () => c.load(),
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: SafeArea(
        child: RefreshIndicator(
          onRefresh: c.load,
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 720),
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 12, 20, 32),
                children: [
                  if (c.error != null) SettingsNote(c.error!, warning: true),
                  if (c.pending != null) ...[
                    const SettingsNote(
                      'A production change is unconfirmed. Retry it before making another change.',
                      warning: true,
                    ),
                    SettingsAction(
                      'Retry saved change',
                      busy: c.busy,
                      onPressed: c.busy || c.loading
                          ? null
                          : () async {
                              await c.save();
                              await c.load();
                            },
                    ),
                  ],
                  if (c.loading && c.data == null)
                    const PremiumSkeleton(detail: true)
                  else if (widget.id == null) ...[
                    SettingsHeading(kind.title, subtitle: kind.purpose),
                    if (kind != ProductionControl.alerts)
                      SettingsAction(
                        'Add ${kind.title.toLowerCase()}',
                        icon: Icons.add_rounded,
                        busy: preparing,
                        onPressed: c.allowed('POST', base) && !preparing
                            ? () => edit()
                            : null,
                      ),
                    const SizedBox(height: 20),
                    if (c.rows.isEmpty && !c.stale)
                      SettingsCard(
                        child: Column(
                          children: [
                            Icon(
                              kind.icon,
                              size: 58,
                              color: Theme.of(context).colorScheme.primary,
                            ),
                            const SizedBox(height: 16),
                            Text(
                              kind == ProductionControl.alerts
                                  ? 'No alerts to review'
                                  : 'No ${kind.title.toLowerCase()} yet',
                              style: const TextStyle(
                                fontSize: 20,
                                fontWeight: FontWeight.w700,
                              ),
                            ),
                            const SizedBox(height: 8),
                            Text(kind.purpose, textAlign: TextAlign.center),
                          ],
                        ),
                      ),
                    for (final r in c.rows)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 12),
                        child: SettingsCard(
                          child: InkWell(
                            onTap: () => open(r),
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Row(
                                  children: [
                                    Icon(
                                      kind.icon,
                                      color: Theme.of(
                                        context,
                                      ).colorScheme.primary,
                                    ),
                                    const SizedBox(width: 12),
                                    Expanded(
                                      child: Text(
                                        rowTitle(r),
                                        style: const TextStyle(
                                          fontSize: 18,
                                          fontWeight: FontWeight.w700,
                                        ),
                                      ),
                                    ),
                                    SettingsStatus(r['status']),
                                  ],
                                ),
                                const SizedBox(height: 12),
                                if (r['default_capacity'] != null)
                                  Text(
                                    '${r['default_capacity']} ${dashboardLabel('${r['unit_type']}')}',
                                  ),
                                if (r['max_quantity'] != null)
                                  Text('${r['max_quantity']} maximum'),
                                if (r['start_at'] != null)
                                  Text(supportDate(r['start_at'])),
                                if (r['start_local_time'] != null)
                                  Text(
                                    '${r['start_local_time']} - ${r['end_local_time']}',
                                  ),
                                if (r['capability'] != null)
                                  Text(dashboardLabel('${r['capability']}')),
                              ],
                            ),
                          ),
                        ),
                      ),
                  ] else if (row != null && !c.stale) ...[
                    SettingsCard(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Icon(
                            kind.icon,
                            size: 42,
                            color: Theme.of(context).colorScheme.primary,
                          ),
                          const SizedBox(height: 16),
                          Text(
                            rowTitle(row),
                            style: const TextStyle(
                              fontSize: 28,
                              fontWeight: FontWeight.w700,
                              letterSpacing: -.6,
                            ),
                          ),
                          const SizedBox(height: 12),
                          SettingsStatus(row['status']),
                          const SizedBox(height: 16),
                          for (final key in [
                            'code',
                            'capability',
                            'production_class',
                            'unit_type',
                            'default_capacity',
                            'setup_buffer_minutes',
                            'cleanup_buffer_minutes',
                            'max_quantity',
                            'specificity',
                            'timezone',
                            'generation_horizon_days',
                          ])
                            if (row[key] != null)
                              SettingsFact(dashboardLabel(key), row[key]),
                          for (final key in [
                            'start_at',
                            'end_at',
                            'detected_at',
                            'acknowledged_at',
                            'resolved_at',
                            'effective_start_date',
                            'effective_end_date',
                          ])
                            if (row[key] != null)
                              SettingsFact(
                                dashboardLabel(key),
                                supportDate(row[key]),
                              ),
                          if (row['days_of_week'] is List)
                            SettingsFact(
                              'Days',
                              (row['days_of_week'] as List)
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
                            ),
                          if (row['start_local_time'] != null)
                            SettingsFact(
                              'Local window',
                              '${row['start_local_time']} - ${row['end_local_time']}',
                            ),
                          if (row['note'] != null)
                            SettingsFact('Note', row['note']),
                          if (row['payload'] is Map)
                            for (final e in (row['payload'] as Map).entries)
                              SettingsFact(dashboardLabel('${e.key}'), e.value),
                        ],
                      ),
                    ),
                    const SizedBox(height: 20),
                    if (![
                          ProductionControl.alerts,
                          ProductionControl.blackouts,
                        ].contains(kind) &&
                        row['status'] != 'ARCHIVED')
                      SettingsAction(
                        'Edit details',
                        icon: Icons.edit_outlined,
                        busy: preparing,
                        onPressed: c.allowed('PATCH', detail) && !preparing
                            ? () => edit(row)
                            : null,
                      ),
                    if (kind == ProductionControl.pools &&
                        row['status'] == 'ACTIVE')
                      Padding(
                        padding: const EdgeInsets.only(top: 12),
                        child: SettingsAction(
                          'Reserve capacity',
                          icon: Icons.bookmark_add_outlined,
                          secondary: true,
                          onPressed: c.writable
                              ? () => Navigator.push(
                                  context,
                                  setupRoute<void>(
                                    ProductionReservationScreen(
                                      runtime: widget.runtime,
                                      pool: row,
                                    ),
                                  ),
                                )
                              : null,
                        ),
                      ),
                    for (final verb in actions(row))
                      Padding(
                        padding: const EdgeInsets.only(top: 12),
                        child: SettingsAction(
                          dashboardLabel(verb),
                          secondary: true,
                          danger: [
                            'archive',
                            'cancel',
                            'blackout',
                          ].contains(verb),
                          busy: c.busy && action == '$detail/$verb',
                          onPressed:
                              c.allowed('POST', '$detail/$verb') && !preparing
                              ? () => change(row, verb)
                              : null,
                        ),
                      ),
                  ] else if (!c.loading && !c.stale)
                    const SettingsNote('This record is no longer available.'),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
