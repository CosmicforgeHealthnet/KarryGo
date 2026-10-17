import 'dart:convert';
import 'production_form_screen.dart';
import 'production_capacity_controls_screen.dart';
import 'production_reservation_screen.dart';
import '../state/production_transitions.dart';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../../settings/state/settings_controller.dart';
import '../../settings/ui/settings_design.dart';
import '../../support/ui/order_support_screen.dart' show supportDate;
import '../state/production_operations_controller.dart';

class ProductionOperationsScreen extends StatefulWidget {
  const ProductionOperationsScreen({
    super.key,
    required this.runtime,
    this.jobId,
  });
  final VendorRuntime runtime;
  final String? jobId;
  @override
  State<ProductionOperationsScreen> createState() =>
      _ProductionOperationsScreenState();
}

class _ProductionOperationsScreenState
    extends State<ProductionOperationsScreen> {
  late final c = ProductionOperationsController(
    widget.runtime,
    widget.jobId == null ? batchesBase : '$batchesBase/{jobId}',
    parameters: {if (widget.jobId != null) 'jobId': widget.jobId!},
  );
  List<JsonObject> steps = [], events = [];
  String status = 'ACTIVE';
  String? evidenceError;
  bool evidenceLoading = false, preparingJob = false;
  @override
  void initState() {
    super.initState();
    c.addListener(changed);
    load();
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

  Future<void> load({bool append = false}) async {
    await c.load(append: append, status: status);
    if (widget.jobId == null || c.stale || !mounted) return;
    setState(() {
      evidenceLoading = true;
      evidenceError = null;
    });
    try {
      final results = await Future.wait([
        c.fetch('$batchesBase/{jobId}/steps', params: c.parameters),
        c.fetch('$batchesBase/{jobId}/events', params: c.parameters),
      ]);
      final nextSteps = c.collection(results[0]);
      for (final step in nextSteps) {
        c.validate(step);
      }
      if (mounted) {
        setState(() {
          steps = nextSteps;
          events = c.collection(results[1]);
        });
      }
    } catch (_) {
      if (mounted) {
        setState(
          () => evidenceError =
              'Station steps and history could not be loaded. Refresh before editing.',
        );
      }
    } finally {
      if (mounted) setState(() => evidenceLoading = false);
    }
  }

  Future<void> submit(
    String route,
    JsonObject body, {
    Map<String, String> params = const {},
    int? version,
  }) async {
    final result = await c.save(
      method: 'POST',
      route: route,
      body: body,
      params: params,
      headerVersion: version,
    );
    if (!mounted) return;
    if (result != null) {
      await showOrderFeedback(
        context,
        title: 'Saved',
        message: 'Production has been updated.',
        success: true,
      );
      await load();
    }
  }

  Future<void> createJob() async {
    if (preparingJob) return;
    setState(() => preparingJob = true);
    final discovery = SettingsController(
      widget.runtime.backend,
      SettingsKind.branches,
    );
    try {
      final branches = await discovery.all(
        '${SettingsController.business}/branches',
        discovery.scope,
      );
      if (!mounted) return;
      final scope = await purchaseForm(context, 'Choose production scope', [
        PurchaseField(
          'branch_id',
          'Branch',
          kind: 'select',
          required: true,
          options: {
            for (final b in branches) '${b['id']}': '${b['display_name'] ?? b['name'] ?? 'Branch'}',
          },
          initial:
              widget.runtime.mode == VendorDataMode.mock && branches.isNotEmpty
              ? branches.first['id']
              : null,
        ),
        PurchaseField(
          'capability',
          'Business type',
          kind: 'select',
          required: true,
          options: productionCapabilities,
          initial: widget.runtime.mode == VendorDataMode.mock
              ? 'RESTAURANT'
              : null,
        ),
      ]);
      if (scope == null) return;
      final lines = c
          .collection(await c.fetch('$productionBase/lines'))
          .where(
            (r) =>
                r['branch_id'] == scope['branch_id'] &&
                r['capability'] == scope['capability'] &&
                r['status'] == 'ACTIVE',
          );
      final reservationChoices = <String, String>{};
      try {
        final raw = await c.storage.read(
          '${c.recoveryKey}.reservation-references',
        );
        final refs = raw == null
            ? <String>[]
            : List<String>.from(jsonDecode(raw));
        for (final ref in refs) {
          try {
            final r = jsonObject(
              await c.fetch(
                '$productionBase/reservations/{reservationId}',
                params: {'reservationId': ref},
              ),
            );
            if (r['branch_id'] == scope['branch_id'] &&
                r['capability'] == scope['capability'] &&
                r['consuming_capacity'] == true) {
              reservationChoices[ref] =
                  '${supportDate(r['window_start'])} / ${dashboardLabel('${r['status']}')}';
            }
          } catch (_) {}
        }
      } catch (_) {}
      if (!mounted) return;
      final mock = widget.runtime.mode == VendorDataMode.mock;
      final body = await productionForm(
        context,
        'New production job',
        [
          PurchaseField(
            'title',
            'Job name',
            required: true,
            initial: mock ? 'Lunch preparation' : null,
          ),
          PurchaseField(
            'planned_quantity',
            'Planned quantity',
            kind: 'number',
            required: true,
            min: 1,
            initial: mock ? 24 : null,
          ),
          const PurchaseField(
            'priority',
            'Priority',
            kind: 'select',
            required: true,
            initial: 'NORMAL',
            options: {'NORMAL': 'Normal', 'HIGH': 'High', 'URGENT': 'Urgent'},
          ),
          PurchaseField(
            'production_line_id',
            'Production line',
            kind: 'select',
            options: {
              for (final line in lines) '${line['id']}': '${line['name']}',
            },
          ),
          PurchaseField(
            'reservation_id',
            'Saved capacity reservation',
            kind: 'select',
            options: reservationChoices,
          ),
          PurchaseField(
            'scheduled_start',
            'Scheduled start',
            kind: 'date',
            initial: mock
                ? DateTime.now()
                      .toUtc()
                      .add(const Duration(hours: 2))
                      .toIso8601String()
                : null,
          ),
          PurchaseField(
            'scheduled_end',
            'Scheduled end',
            kind: 'date',
            initial: mock
                ? DateTime.now()
                      .toUtc()
                      .add(const Duration(hours: 3))
                      .toIso8601String()
                : null,
          ),
          const PurchaseField(
            'expected_ready_window_start',
            'Expected ready window start',
            kind: 'date',
          ),
          const PurchaseField(
            'expected_ready_window_end',
            'Expected ready window end',
            kind: 'date',
          ),
        ],
        validate: (b) =>
            windowError(b) ??
            ((b['expected_ready_window_start'] == null) !=
                    (b['expected_ready_window_end'] == null)
                ? 'Choose both expected ready times.'
                : b['expected_ready_window_start'] != null &&
                      DateTime.parse(b['expected_ready_window_end']).isBefore(
                        DateTime.parse(b['expected_ready_window_start']),
                      )
                ? 'Expected ready end must not precede start.'
                : null),
      );
      if (body != null) {
        await submit(batchesBase, {...body, ...scope, 'source_type': 'MANUAL'});
      }
    } catch (_) {
      if (mounted) {
        setState(
          () =>
              c.error = 'Production references could not be loaded. Try again.',
        );
      }
    } finally {
      discovery.dispose();
      if (mounted) setState(() => preparingJob = false);
    }
  }

  String? windowError(JsonObject b) {
    final start = b['scheduled_start'], end = b['scheduled_end'];
    if ((start == null) != (end == null)) return 'Select both scheduled times.';
    if (start != null &&
        !DateTime.parse('$end').isAfter(DateTime.parse('$start'))) {
      return 'End must be after start.';
    }
    return null;
  }

  String jobActionRoute(String target) =>
      '$batchesBase/{jobId}/${switch (target) {
        'IN_PROGRESS' => 'start',
        'READY' => 'ready',
        'COMPLETED' => 'complete',
        _ => '{status}',
      }}';

  Future<void> transition(String target, {JsonObject? step}) async {
    final job = c.record!;
    final body = await productionForm(
      context,
      dashboardLabel(target),
      [
        PurchaseField('note', 'Note', kind: 'notes'),
        if (target == 'BLOCKED')
          const PurchaseField(
            'blocked_reason',
            'What is blocking production?',
            required: true,
            kind: 'notes',
          ),
        if (['READY', 'COMPLETED'].contains(target) && step == null) ...[
          PurchaseField(
            'produced_quantity',
            'Produced quantity',
            kind: 'number',
            required: true,
            max: job['planned_quantity'] as int,
          ),
          PurchaseField(
            'waste_quantity',
            'Waste quantity',
            kind: 'number',
            required: true,
            initial: 0,
            max: job['planned_quantity'] as int,
          ),
        ],
        if (target == 'READY' && step == null) ...[
          PurchaseField(
            'packaging_profile',
            'Packaging profile',
            kind: 'object',
            required: true,
            initial: widget.runtime.mode == VendorDataMode.mock
                ? {'note': 'Food-safe sealed containers'}
                : null,
          ),
          PurchaseField(
            'packaging_evidence',
            'Recorded packaging evidence',
            kind: 'object',
            required: true,
            initial: widget.runtime.mode == VendorDataMode.mock
                ? {'note': 'Packaging checked before handoff'}
                : null,
          ),
        ],
      ],
      validate: (b) =>
          step == null &&
              ['READY', 'COMPLETED'].contains(target) &&
              (b['produced_quantity'] as int) + (b['waste_quantity'] as int) >
                  (job['planned_quantity'] as int)
          ? 'Produced and waste cannot exceed the planned quantity.'
          : null,
    );
    if (body == null) return;
    if (!mounted) return;
    if (['CANCELLED', 'SKIPPED'].contains(target) &&
        !await confirmSettings(
          context,
          '${dashboardLabel(target)}?',
          step == null
              ? 'This changes the production job only.'
              : 'This changes this station step.',
          danger: true,
        )) {
      return;
    }
    await submit(
      step == null
          ? jobActionRoute(target)
          : '$batchesBase/{jobId}/steps/{stepId}/{status}',
      {...body, if (step != null) 'expected_version': step['version']},
      params: {
        ...c.parameters,
        'status': target,
        if (step != null) 'stepId': '${step['id']}',
      },
      version: job['version'] as int,
    );
  }

  Future<void> reportDelay() async {
    final body = await purchaseForm(
      context,
      'Report delay',
      [
        const PurchaseField(
          'reason_code',
          'Reason',
          kind: 'select',
          required: true,
          options: {
            'STAFF_SHORTAGE': 'Staff shortage',
            'EQUIPMENT_FAILURE': 'Equipment failure',
            'INGREDIENT_SHORTAGE': 'Ingredient shortage',
            'QUALITY_REWORK': 'Quality rework',
            'CAPACITY_CONFLICT': 'Capacity conflict',
            'LOGISTICS_CONSTRAINT': 'Logistics constraint',
            'OTHER': 'Other',
          },
        ),
        const PurchaseField('note', 'Note', kind: 'notes'),
        const PurchaseField(
          'new_window_start',
          'New ready window start',
          kind: 'date',
          required: true,
        ),
        const PurchaseField(
          'new_window_end',
          'New ready window end',
          kind: 'date',
          required: true,
        ),
      ],
      validate: (b) => '${b['note']}'.length > 280
          ? 'Keep the note within 280 characters.'
          : !DateTime.parse(
              b['new_window_end'],
            ).isAfter(DateTime.parse(b['new_window_start']))
          ? 'End must be after start.'
          : null,
    );
    if (body != null) {
      await submit(
        '$batchesBase/{jobId}/delay',
        {...body, 'expected_version': c.record!['version']},
        params: c.parameters,
        version: c.record!['version'] as int,
      );
    }
  }

  Future<void> addStep() async {
    final body = await purchaseForm(context, 'Add station step', [
      const PurchaseField('name', 'Step name', required: true),
      PurchaseField(
        'sequence_no',
        'Sequence',
        kind: 'number',
        required: true,
        initial: steps.length,
      ),
      const PurchaseField(
        'phase',
        'Phase',
        kind: 'number',
        required: true,
        initial: 0,
      ),
      const PurchaseField(
        'required',
        'Required step',
        kind: 'bool',
        initial: true,
      ),
    ]);
    if (body != null) {
      await submit(
        '$batchesBase/{jobId}/steps',
        {...body, 'expected_job_version': c.record!['version']},
        params: c.parameters,
        version: c.record!['version'] as int,
      );
    }
  }

  void openJob(JsonObject row) async {
    await Navigator.push(
      context,
      setupRoute<void>(
        ProductionOperationsScreen(
          runtime: widget.runtime,
          jobId: '${row['id']}',
        ),
      ),
    );
    await load();
  }

  Widget quantity(String label, Object? value, IconData icon) => Expanded(
    child: Column(
      children: [
        Icon(icon, size: 24, color: Theme.of(context).colorScheme.primary),
        const SizedBox(height: 10),
        Text(
          '$value',
          style: const TextStyle(fontSize: 24, fontWeight: FontWeight.w700),
        ),
        const SizedBox(height: 4),
        Text(
          label,
          style: Theme.of(context).textTheme.bodySmall,
          textAlign: TextAlign.center,
        ),
      ],
    ),
  );
  @override
  Widget build(BuildContext context) {
    final job = c.record;
    final detail = widget.jobId != null;
    return Scaffold(
      appBar: AppBar(
        title: Text(detail ? 'Production job' : 'Production'),
        actions: [
          IconButton(
            tooltip: 'Refresh',
            onPressed: c.busy || c.loading ? null : () => load(),
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: SafeArea(
        child: RefreshIndicator(
          onRefresh: load,
          child: ListView(
            padding: const EdgeInsets.fromLTRB(20, 12, 20, 32),
            children: [
              if (c.error != null) SettingsNote(c.error!, warning: true),
              if (c.pending != null) ...[
                const SettingsNote(
                  'A change is unconfirmed. Retry the saved request before making another change.',
                  warning: true,
                ),
                SettingsAction(
                  'Retry saved change',
                  busy: c.busy,
                  onPressed: c.loading || c.busy
                      ? null
                      : () async {
                          await c.save();
                          await load();
                        },
                ),
              ],
              if (c.loading && (detail ? job == null : c.rows.isEmpty))
                const PremiumSkeleton(detail: true)
              else if (!detail) ...[
                const SettingsHeading(
                  'Production queue',
                  subtitle: 'Jobs in deadline and priority order.',
                ),
                SettingsAction(
                  'New job',
                  icon: Icons.add_rounded,
                  onPressed: c.allowed('POST', batchesBase) && !preparingJob
                      ? createJob
                      : null,
                  busy: c.busy || preparingJob,
                ),
                const SizedBox(height: 12),
                SettingsLink(
                  'Capacity',
                  Icons.donut_large_rounded,
                  subtitle: 'Check workload and available capacity',
                  onTap: () => Navigator.push(
                    context,
                    setupRoute<void>(
                      ProductionCapacityScreen(runtime: widget.runtime),
                    ),
                  ),
                ),
                const SizedBox(height: 16),
                SingleChildScrollView(
                  scrollDirection: Axis.horizontal,
                  child: Row(
                    children: [
                      for (final value in [
                        'ACTIVE',
                        'READY',
                        'BLOCKED',
                        'COMPLETED',
                        'ALL',
                      ])
                        Padding(
                          padding: const EdgeInsets.only(right: 8),
                          child: ChoiceChip(
                            label: Text(dashboardLabel(value)),
                            selected: status == value,
                            onSelected: c.loading
                                ? null
                                : (_) {
                                    setState(() => status = value);
                                    load();
                                  },
                          ),
                        ),
                    ],
                  ),
                ),
                const SizedBox(height: 20),
                if (c.rows.isEmpty && !c.stale)
                  SettingsCard(
                    child: Column(
                      children: [
                        Icon(
                          Icons.precision_manufacturing_outlined,
                          size: 56,
                          color: Theme.of(context).colorScheme.primary,
                        ),
                        const SizedBox(height: 16),
                        const Text(
                          'Your production queue is clear',
                          style: TextStyle(
                            fontSize: 19,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                        const SizedBox(height: 8),
                        const Text(
                          'Create a job to plan your next production run.',
                          textAlign: TextAlign.center,
                        ),
                      ],
                    ),
                  ),
                for (final r in c.rows)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 12),
                    child: SettingsCard(
                      child: InkWell(
                        onTap: () => openJob(r),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Row(
                              children: [
                                Expanded(
                                  child: Text(
                                    '${r['title']}',
                                    style: const TextStyle(
                                      fontSize: 19,
                                      fontWeight: FontWeight.w700,
                                    ),
                                  ),
                                ),
                                SettingsStatus(r['status']),
                              ],
                            ),
                            const SizedBox(height: 12),
                            Text(
                              '${r['planned_quantity']} planned  /  ${dashboardLabel('${r['priority']}')} priority',
                            ),
                            if (r['promised_ready_at'] != null)
                              Padding(
                                padding: const EdgeInsets.only(top: 8),
                                child: Text(
                                  'Due ${supportDate(r['promised_ready_at'])}',
                                ),
                              ),
                            if (r['step_count'] != null && r['step_count'] != 0)
                              Padding(
                                padding: const EdgeInsets.only(top: 12),
                                child: LinearProgressIndicator(
                                  value:
                                      (r['completed_step_count'] as num) /
                                      (r['step_count'] as num),
                                  borderRadius: BorderRadius.circular(4),
                                ),
                              ),
                          ],
                        ),
                      ),
                    ),
                  ),
                if (c.hasMore)
                  SettingsAction(
                    'Load more',
                    secondary: true,
                    onPressed: c.loading ? null : () => load(append: true),
                  ),
              ] else if (job != null && !c.stale) ...[
                SettingsCard(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      SettingsStatus(job['status']),
                      const SizedBox(height: 16),
                      Text(
                        '${job['title']}',
                        style: const TextStyle(
                          fontSize: 28,
                          fontWeight: FontWeight.w700,
                          letterSpacing: -.7,
                        ),
                      ),
                      const SizedBox(height: 8),
                      Text(
                        '${dashboardLabel('${job['capability']}')} / ${dashboardLabel('${job['priority']}')} priority',
                      ),
                      const SizedBox(height: 24),
                      Row(
                        children: [
                          quantity(
                            'Planned',
                            job['planned_quantity'],
                            Icons.assignment_outlined,
                          ),
                          quantity(
                            'Produced',
                            job['produced_quantity'],
                            Icons.inventory_2_outlined,
                          ),
                          quantity(
                            'Waste',
                            job['waste_quantity'],
                            Icons.delete_outline_rounded,
                          ),
                        ],
                      ),
                      if ('${job['blocked_reason'] ?? ''}'.isNotEmpty)
                        Padding(
                          padding: const EdgeInsets.only(top: 16),
                          child: SettingsNote(
                            '${job['blocked_reason']}',
                            warning: true,
                          ),
                        ),
                    ],
                  ),
                ),
                const SizedBox(height: 20),
                if (job['scheduled_start'] != null)
                  SettingsFact(
                    'Scheduled start',
                    supportDate(job['scheduled_start']),
                  ),
                if (job['expected_ready_window_end'] != null)
                  SettingsFact(
                    'Expected ready',
                    supportDate(job['expected_ready_window_end']),
                  ),
                for (final target
                    in jobTransitions[job['status']] ?? <String>[])
                  Padding(
                    padding: const EdgeInsets.only(bottom: 10),
                    child: SettingsAction(
                      dashboardLabel(target),
                      danger: target == 'CANCELLED',
                      secondary: target == 'BLOCKED',
                      onPressed:
                          c.allowed('POST', jobActionRoute(target)) &&
                              (target != 'READY' ||
                                  !evidenceLoading &&
                                      evidenceError == null &&
                                      steps.every(
                                        (step) =>
                                            step['required'] != true ||
                                            [
                                              'COMPLETED',
                                              'SKIPPED',
                                            ].contains(step['status']),
                                      ))
                          ? () => transition(target)
                          : null,
                    ),
                  ),
                if (job['status'] == 'IN_PROGRESS')
                  const SettingsNote(
                    'Complete required station steps and record packaging details to mark this job ready.',
                    warning: true,
                  ),
                if (!['COMPLETED', 'CANCELLED'].contains(job['status']))
                  SettingsAction(
                    'Report delay',
                    secondary: true,
                    icon: Icons.schedule_rounded,
                    onPressed: c.allowed('POST', '$batchesBase/{jobId}/delay')
                        ? reportDelay
                        : null,
                  ),
                if (job['reservation_id'] != null)
                  SettingsLink(
                    'Capacity reservation',
                    Icons.bookmark_added_outlined,
                    onTap: () => Navigator.push(
                      context,
                      setupRoute<void>(
                        ProductionReservationScreen(
                          runtime: widget.runtime,
                          reservationId: '${job['reservation_id']}',
                        ),
                      ),
                    ),
                  ),
                const SizedBox(height: 12),
                const SettingsHeading('Station steps'),
                if (evidenceLoading) const PremiumSkeleton(detail: true),
                if (evidenceError != null)
                  SettingsNote(evidenceError!, warning: true),
                for (final step in steps)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 12),
                    child: SettingsCard(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Row(
                            children: [
                              Expanded(
                                child: Text(
                                  '${step['name']}',
                                  style: const TextStyle(
                                    fontSize: 18,
                                    fontWeight: FontWeight.w700,
                                  ),
                                ),
                              ),
                              SettingsStatus(step['status']),
                            ],
                          ),
                          if (step['required'] == true)
                            const Padding(
                              padding: EdgeInsets.only(top: 8),
                              child: Text('Required'),
                            ),
                          if ('${step['blocked_reason'] ?? ''}'.isNotEmpty)
                            SettingsNote(
                              '${step['blocked_reason']}',
                              warning: true,
                            ),
                          const SizedBox(height: 12),
                          Wrap(
                            spacing: 8,
                            runSpacing: 8,
                            children: [
                              for (final target
                                  in stationTransitions[step['status']] ??
                                      <String>[])
                                OutlinedButton(
                                  onPressed:
                                      c.allowed(
                                            'POST',
                                            '$batchesBase/{jobId}/steps/{stepId}/{status}',
                                          ) &&
                                          !evidenceLoading &&
                                          evidenceError == null
                                      ? () => transition(target, step: step)
                                      : null,
                                  child: Text(dashboardLabel(target)),
                                ),
                            ],
                          ),
                        ],
                      ),
                    ),
                  ),
                if (!['COMPLETED', 'CANCELLED'].contains(job['status']))
                  SettingsAction(
                    'Add station step',
                    icon: Icons.add_task_rounded,
                    secondary: true,
                    onPressed:
                        c.allowed('POST', '$batchesBase/{jobId}/steps') &&
                            !evidenceLoading &&
                            evidenceError == null
                        ? addStep
                        : null,
                  ),
                const SizedBox(height: 24),
                const SettingsHeading('History'),
                for (final event in events)
                  ListTile(
                    contentPadding: EdgeInsets.zero,
                    leading: const Icon(Icons.history_rounded),
                    title: Text(dashboardLabel('${event['event_type']}')),
                    subtitle: Text('${event['note'] ?? ''}'),
                    trailing: event['to_status'] == null
                        ? null
                        : SettingsStatus(event['to_status']),
                  ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

class ProductionCapacityScreen extends StatefulWidget {
  const ProductionCapacityScreen({super.key, required this.runtime});
  final VendorRuntime runtime;
  @override
  State<ProductionCapacityScreen> createState() =>
      _ProductionCapacityScreenState();
}

class _ProductionCapacityScreenState extends State<ProductionCapacityScreen> {
  late final c = ProductionOperationsController(
    widget.runtime,
    '$productionBase/pools',
  );
  Object? utilization;
  String? error;
  bool checking = false;
  String? activePool;
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

  Future<void> inspect(JsonObject pool) async {
    final body = await purchaseForm(
      context,
      'Check capacity',
      [
        const PurchaseField(
          'window_start',
          'Window start',
          kind: 'date',
          required: true,
        ),
        const PurchaseField(
          'window_end',
          'Window end',
          kind: 'date',
          required: true,
        ),
        const PurchaseField(
          'quantity',
          'Required quantity',
          kind: 'number',
          required: true,
          min: 1,
        ),
      ],
      validate: (b) =>
          DateTime.parse(
            b['window_end'],
          ).isAfter(DateTime.parse(b['window_start']))
          ? null
          : 'End must be after start.',
    );
    if (body == null || !mounted) return;
    setState(() {
      checking = true;
      activePool = pool['id'];
      error = null;
      utilization = null;
    });
    try {
      final result = await widget.runtime.backend.productionApi.request(
        'POST',
        '$productionBase/capacity/preview',
        body: VendorPayload({
          'branch_id': pool['branch_id'],
          'capability': pool['capability'],
          'window_start': body['window_start'],
          'window_end': body['window_end'],
          'source_type': 'MANUAL',
          'allocations': [
            {'capacity_pool_id': pool['id'], 'quantity': body['quantity']},
          ],
        }),
        command: VendorCommand.create(),
      );
      final report = await c.fetch(
        '$productionBase/capacity/utilization',
        query: {
          'branch_id': '${pool['branch_id']}',
          'capability': '${pool['capability']}',
          'start': '${body['window_start']}',
          'end': '${body['window_end']}',
        },
      );
      if (!mounted) return;
      setState(() => utilization = report);
      final preview = result.object;
      await showOrderFeedback(
        context,
        title: preview['available'] == true
            ? 'Capacity available'
            : 'Capacity unavailable',
        message: preview['available'] == true
            ? 'Capacity is available for this window. A reservation rechecks availability.'
            : '${preview['blocking_reason'] ?? 'This request exceeds available capacity.'}',
        success: preview['available'] == true,
        warning: preview['available'] != true,
      );
    } catch (_) {
      if (mounted) {
        setState(() => error = 'Capacity could not be checked. Try again.');
      }
    } finally {
      if (mounted) setState(() => checking = false);
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Production capacity')),
    body: SafeArea(
      child: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          const SettingsHeading(
            'Capacity pools',
            subtitle: 'Check available production resources for a time window.',
          ),
          SettingsLink(
            'Manage capacity',
            Icons.tune_rounded,
            subtitle: 'Resources, schedules and operational alerts',
            onTap: () async {
              await Navigator.push(
                context,
                setupRoute<void>(
                  ProductionControlsHub(runtime: widget.runtime),
                ),
              );
              await c.load();
            },
          ),
          SettingsLink(
            'Reservations',
            Icons.bookmarks_outlined,
            subtitle: 'Find saved capacity reservations',
            onTap: () => Navigator.push(
              context,
              setupRoute<void>(
                ProductionReservationScreen(runtime: widget.runtime),
              ),
            ),
          ),
          if (c.loading) const PremiumSkeleton(detail: true),
          if (c.error != null) SettingsNote(c.error!, warning: true),
          if (error != null) SettingsNote(error!, warning: true),
          if (!c.loading && c.rows.isEmpty && !c.stale)
            const SettingsNote(
              'No capacity pools have been configured for your accessible branches.',
            ),
          for (final pool in c.rows)
            Padding(
              padding: const EdgeInsets.only(bottom: 16),
              child: SettingsCard(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Icon(
                          Icons.donut_large_rounded,
                          color: Theme.of(context).colorScheme.primary,
                        ),
                        const SizedBox(width: 12),
                        Expanded(
                          child: Text(
                            '${pool['name']}',
                            style: const TextStyle(
                              fontSize: 20,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                        ),
                        SettingsStatus(pool['status']),
                      ],
                    ),
                    const SizedBox(height: 12),
                    Text(
                      '${pool['default_capacity']} ${dashboardLabel('${pool['unit_type']}')} / ${dashboardLabel('${pool['capability']}')}',
                    ),
                    const SizedBox(height: 16),
                    SettingsAction(
                      'Check availability',
                      secondary: true,
                      busy: checking && activePool == pool['id'],
                      onPressed:
                          checking || c.stale || pool['status'] != 'ACTIVE'
                          ? null
                          : () => inspect(pool),
                    ),
                  ],
                ),
              ),
            ),
          if (utilization is List) ...[
            const SettingsHeading('Window utilization'),
            for (final row in c.collection(utilization))
              SettingsCard(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      '${row['code']}',
                      style: const TextStyle(fontWeight: FontWeight.w700),
                    ),
                    const SizedBox(height: 12),
                    Text(
                      '${row['confirmed']} confirmed / ${row['held']} held / ${row['remaining']} remaining',
                    ),
                    const SizedBox(height: 12),
                    LinearProgressIndicator(
                      value: ((row['utilization_basis_points'] as num) / 10000)
                          .clamp(0, 1),
                      borderRadius: BorderRadius.circular(4),
                    ),
                  ],
                ),
              ),
          ],
        ],
      ),
    ),
  );
}
