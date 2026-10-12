import 'pastry_configuration_screen.dart';
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
import '../../production/ui/production_operations_screen.dart';
import '../../production/ui/production_reservation_screen.dart';
import '../../production/ui/production_capacity_controls_screen.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../../settings/ui/settings_design.dart';
import '../../settings/ui/settings_verification_screen.dart';
import '../../support/ui/order_support_screen.dart' show supportDate;
import '../state/pastry_workspace_controller.dart';

enum PastryWork { quotes, changes, milestones, standard }

extension PastryWorkInfo on PastryWork {
  String get title => switch (this) {
    PastryWork.quotes => 'Quotes',
    PastryWork.changes => 'Design reviews',
    PastryWork.milestones => 'Production milestones',
    PastryWork.standard => 'Plan pastry production',
  };
  IconData get icon => switch (this) {
    PastryWork.quotes => Icons.request_quote_outlined,
    PastryWork.changes => Icons.design_services_outlined,
    PastryWork.milestones => Icons.timeline_rounded,
    PastryWork.standard => Icons.bakery_dining_outlined,
  };
}

class PastryWorkflowsScreen extends StatefulWidget {
  const PastryWorkflowsScreen({
    super.key,
    required this.runtime,
    required this.sourceId,
    required this.work,
    this.quoteId,
  });
  final VendorRuntime runtime;
  final String sourceId;
  final PastryWork work;
  final String? quoteId;
  @override
  State<PastryWorkflowsScreen> createState() => _PastryWorkflowsScreenState();
}

class _PastryWorkflowsScreenState extends State<PastryWorkflowsScreen> {
  PastryWork get work => widget.work;
  bool get standard => work == PastryWork.standard;
  bool get mock => widget.runtime.mode == VendorDataMode.mock;
  String get root => standard
      ? '$pastryBase/products/{productId}'
      : '$pastryBase/custom-requests/{requestId}';
  Map<String, String> get params => {
    standard ? 'productId' : 'requestId': widget.sourceId,
    if (widget.quoteId != null) 'quoteId': widget.quoteId!,
  };
  late final c = PastryWorkspaceController(
    widget.runtime,
    standard
        ? root
        : widget.quoteId != null
        ? '$pastryBase/quotes/{quoteId}'
        : '$root/${work.name}',
    parameters: standard
        ? {'productId': widget.sourceId}
        : widget.quoteId != null
        ? {'quoteId': widget.quoteId!}
        : {'requestId': widget.sourceId},
  );
  JsonObject subject = {}, eligibility = {};
  List<JsonObject> specs = [], changes = [], pools = [], lines = [];
  JsonObject? preview, previewBody;
  bool reading = false, preparing = false, ready = false;
  String? readError, activeAction;
  bool get writable => c.writable && ready && !reading && !preparing;
  Map<String, String> matchingPools() => {
    for (final p in pools.where(
      (p) =>
          p['branch_id'] == subject['branch_id'] &&
          p['capability'] == 'PASTRY_BAKERY' &&
          p['status'] == 'ACTIVE',
    ))
      '${p['id']}': '${p['name']}',
  };
  Map<String, String> matchingLines(String? pool) => {
    for (final l in lines.where(
      (l) =>
          l['branch_id'] == subject['branch_id'] &&
          l['capability'] == 'PASTRY_BAKERY' &&
          l['status'] == 'ACTIVE' &&
          (l['capacity_pool_id'] == null || l['capacity_pool_id'] == pool),
    ))
      '${l['id']}': '${l['name']}',
  };
  List<JsonObject> get effectiveSpecs => specs
      .where(
        (s) => !changes.any(
          (ch) =>
              ch['to_revision'] == s['revision'] && ch['status'] != 'APPROVED',
        ),
      )
      .toList();
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
      ready = false;
      readError = null;
    });
    try {
      await c.load();
      if (c.error != null) return;
      if (widget.quoteId != null &&
          c.record?['request_id'] != widget.sourceId) {
        throw const FormatException('Quote belongs to a different request.');
      }
      subject = standard
          ? c.record!
          : jsonObject(await c.fetch(root, params: params));
      if (!standard) {
        specs = c.collection(
          await c.fetch('$root/specifications', params: params),
        );
        changes = c.collection(await c.fetch('$root/changes', params: params));
        eligibility = jsonObject(
          await c.fetch('$root/change-eligibility', params: params),
        );
      }
      if (standard) {
        await capacityReferences();
      }
      if (mounted) setState(() => ready = true);
    } catch (_) {
      if (mounted) {
        setState(
          () => readError =
              'Some required details could not be loaded. Refresh before continuing.',
        );
      }
    } finally {
      if (mounted) setState(() => reading = false);
    }
  }

  Future<void> capacityReferences() async {
    pools = c.collection(
      (await c.app.productionApi.request(
        'GET',
        '/api/v1/vendor/production/pools',
      )).data,
    );
    lines = c.collection(
      (await c.app.productionApi.request(
        'GET',
        '/api/v1/vendor/production/lines',
      )).data,
    );
  }

  Future<void> feedback(String message, {bool success = false}) =>
      showOrderFeedback(
        context,
        title: success ? 'Confirmed' : 'Review required',
        message: message,
        success: success,
        warning: !success,
      );
  Future<String?> verify() => Navigator.push<String>(
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
  Future<JsonObject?> submit(
    String label,
    String route,
    JsonObject body, {
    Map<String, String>? path,
    Map<String, String> headers = const {},
  }) async {
    setState(() => activeAction = label);
    final result = await c.save(
      method: 'POST',
      route: route,
      body: body,
      params: path ?? params,
      headers: headers,
    );
    if (mounted) {
      if (result == null) {
        await feedback(c.error ?? 'The change could not be confirmed.');
      } else if (result['conversion_intent_id'] != null) {
        await conversion(result);
      } else {
        await feedback(
          label == 'Create production batch'
              ? 'Your production batch has been created.'
              : 'Your change is confirmed.',
          success: true,
        );
      }
      if (result != null) await load();
      setState(() => activeAction = null);
    }
    return result;
  }

  Future<void> conversion(JsonObject result) => settingsSheet<void>(
    context,
    Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const SettingsHeading('Quote accepted'),
        const Text(
          'Production is planned. Customer order conversion is pending.',
        ),
        const SizedBox(height: 16),
        SettingsFact(
          'Conversion reference',
          '${result['conversion_intent_id']}',
        ),
        if ('${result['reservation_id'] ?? ''}'.isNotEmpty)
          SettingsLink(
            'View reserved capacity',
            Icons.donut_large_rounded,
            onTap: () {
              Navigator.pop(context);
              Navigator.push(
                context,
                setupRoute<void>(
                  ProductionReservationScreen(
                    runtime: widget.runtime,
                    reservationId: '${result['reservation_id']}',
                  ),
                ),
              );
            },
          ),
        SettingsAction('Done', onPressed: () => Navigator.pop(context)),
      ],
    ),
  );
  Future<void> retry() async {
    String? grant;
    if ('${c.pending?['path']}'.endsWith('/approve')) {
      grant = await verify();
      if (grant == null || !mounted) return;
    }
    setState(() => activeAction = 'Retry saved change');
    final result = await c.save(headers: {'X-Step-Up-Grant': ?grant});
    if (mounted) {
      if (result?['conversion_intent_id'] != null) {
        await conversion(result!);
      } else {
        await feedback(
          result == null
              ? c.error ?? 'The outcome is unconfirmed.'
              : 'The saved change is confirmed.',
          success: result != null,
        );
      }
      await load();
      setState(() => activeAction = null);
      if (result?['production_job_id'] != null && mounted) {
        await Navigator.push(
          context,
          setupRoute<void>(
            ProductionOperationsScreen(
              runtime: widget.runtime,
              jobId: '${result!['production_job_id']}',
            ),
          ),
        );
      }
    }
  }

  PurchaseField text(
    JsonObject old,
    String key,
    String label, {
    Object? sample,
    bool required = false,
    String kind = 'text',
    int min = 0,
    int? max,
    Map<String, String> options = const {},
  }) => PurchaseField(
    key,
    label,
    kind: kind,
    required: required,
    initial: old[key] ?? (mock ? sample : null),
    min: min,
    max: max,
    options: options,
  );
  Future<void> quoteEditor({bool revise = false}) async {
    if (!writable) return;
    setState(() => preparing = true);
    try {
      if (effectiveSpecs.isEmpty) {
        await feedback('Save a design specification before creating a quote.');
        return;
      }
      final current = effectiveSpecs.last;
      final old = revise ? c.record! : <String, dynamic>{};
      final quoteFields = <PurchaseField>[
        PurchaseField(
          'amount',
          'Quote amount (NGN)',
          required: true,
          initial: old['amount_minor'] != null
              ? ((old['amount_minor'] as num) / 100).toStringAsFixed(2)
              : mock
              ? '45000.00'
              : null,
        ),
        text(
          old,
          'expires_at',
          'Quote expiry',
          kind: 'date',
          required: true,
          sample: DateTime.now()
              .toUtc()
              .add(const Duration(days: 2))
              .toIso8601String(),
        ),
        text(
          old,
          'vendor_terms',
          'Terms',
          kind: 'notes',
          sample: 'Collection after confirmation. Handle with care.',
        ),
        if (revise)
          text(
            {},
            'reason_code',
            'Revision reason',
            required: true,
            sample: 'PRICE_UPDATE',
          ),
        if (revise)
          text(
            {},
            'note',
            'Revision note',
            kind: 'notes',
            sample: 'Updated following the confirmed design.',
          ),
        if (!revise)
          const PurchaseField(
            'approval_required',
            'Require approval',
            kind: 'bool',
            initial: true,
          ),
      ];
      if (!revise) {
        await capacityReferences();
        if (!mounted) return;
        quoteFields.addAll([
          text(
            {},
            'production_start',
            'Production starts',
            kind: 'date',
            sample: DateTime.now()
                .toUtc()
                .add(const Duration(days: 1))
                .toIso8601String(),
          ),
          text(
            {},
            'production_end',
            'Production ends',
            kind: 'date',
            sample: DateTime.now()
                .toUtc()
                .add(const Duration(days: 1, hours: 2))
                .toIso8601String(),
          ),
          text(
            {},
            'hold_for_minutes',
            'Capacity hold (minutes)',
            kind: 'number',
            sample: 30,
          ),
        ]);
      }
      if (!mounted) return;
      final values = await productionForm(
        context,
        revise ? 'Revise quote' : 'Prepare quote',
        quoteFields,
        validate: (v) {
          if (purchaseDecimal('${v['amount']}') == null) {
            return 'Enter a valid amount with up to two decimal places.';
          }
          final expiry = DateTime.tryParse('${v['expires_at']}');
          if (expiry == null || !expiry.isAfter(DateTime.now())) {
            return 'Choose a future quote expiry.';
          }
          if ('${v['vendor_terms'] ?? ''}'.length > 4000) {
            return 'Keep terms within 4,000 characters.';
          }
          return null;
        },
      );
      if (values == null || !mounted) return;
      final body = <String, dynamic>{
        for (final e in values.entries)
          if (e.key != 'amount') e.key: e.value,
        'amount_minor': purchaseDecimal('${values['amount']}'),
        'specification_version_id': current['id'],
      };
      if (revise) {
        body['expected_version'] = old['version'];
        final revised = await submit(
          'Revise quote',
          '$pastryBase/quotes/{quoteId}/revise',
          body,
        );
        if (revised != null && mounted) {
          Navigator.pushReplacement(
            context,
            setupRoute<void>(
              PastryWorkflowsScreen(
                runtime: widget.runtime,
                sourceId: widget.sourceId,
                work: PastryWork.quotes,
                quoteId: '${revised['id']}',
              ),
            ),
          );
        }
        return;
      }
      final poolChoices = matchingPools();
      final chosen = await crmChoose(
        context,
        'Reserve production capacity',
        poolChoices,
        multiple: true,
        optional: true,
        initial: mock && poolChoices.isNotEmpty ? [poolChoices.keys.first] : [],
      );
      if (chosen == null || !mounted) return;
      final allocations = <JsonObject>[];
      for (final pool in chosen) {
        final choices = matchingLines(pool);
        final quantity = await productionForm(context, poolChoices[pool]!, [
          text(
            {},
            'quantity',
            'Capacity units',
            kind: 'number',
            required: true,
            sample: 1,
            min: 1,
            max: 1000000,
          ),
          text(
            {},
            'production_line_id',
            'Production line',
            kind: 'select',
            options: choices,
          ),
        ]);
        if (quantity == null || !mounted) return;
        allocations.add({'capacity_pool_id': pool, ...quantity});
      }
      if (allocations.isNotEmpty) {
        final start = DateTime.tryParse('${body['production_start']}'),
            end = DateTime.tryParse('${body['production_end']}');
        if (start == null || end == null || !end.isAfter(start)) {
          await feedback(
            'Choose a valid production window for reserved capacity.',
          );
          return;
        }
      }
      body['capacity_allocations'] = allocations;
      body['expected_request_version'] = subject['version'];
      final created = await submit('Create quote', '$root/quotes', body);
      if (created != null && mounted) await openQuote(created);
    } catch (_) {
      if (mounted) {
        await feedback(
          'Quote details could not be prepared. Refresh and try again.',
        );
      }
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  Future<void> quoteAction(String action) async {
    if (!writable) return;
    String? grant;
    if (action == 'approve') {
      grant = await verify();
      if (grant == null || !mounted) return;
    } else if (!await confirmSettings(
          context,
          action == 'accept' ? 'Accept this quote?' : 'Decline this quote?',
          action == 'accept'
              ? 'Confirm reserved production capacity and create the conversion request. Customer order conversion remains pending.'
              : 'Decline this quote and release eligible reserved capacity.',
          danger: action == 'decline',
        ) ||
        !mounted) {
      return;
    }
    await submit(
      action == 'accept'
          ? 'Accept quote'
          : action == 'approve'
          ? 'Approve quote'
          : 'Decline quote',
      '$pastryBase/quotes/{quoteId}/$action',
      {'expected_version': c.record!['version']},
      headers: {'X-Step-Up-Grant': ?grant},
    );
  }

  Future<void> openQuote(JsonObject q) async {
    await Navigator.push(
      context,
      setupRoute<void>(
        PastryWorkflowsScreen(
          runtime: widget.runtime,
          sourceId: widget.sourceId,
          work: PastryWork.quotes,
          quoteId: '${q['id']}',
        ),
      ),
    );
    if (mounted) await load();
  }

  Future<void> history(
    String title,
    String route, {
    Map<String, String>? path,
  }) async {
    if (reading || c.busy) return;
    setState(() => activeAction = title);
    try {
      final rows = c.collection(await c.fetch(route, params: path ?? params));
      if (!mounted) return;
      await Navigator.push(
        context,
        setupRoute<void>(
          Scaffold(
            appBar: AppBar(title: Text(title)),
            body: ListView(
              padding: const EdgeInsets.all(20),
              children: [
                if (rows.isEmpty)
                  const SettingsNote('No recorded changes yet.'),
                for (final h in rows)
                  SettingsCard(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          '${crmLabel(h['from_state'] ?? h['from_status'] ?? 'Created')} to ${crmLabel(h['to_state'] ?? h['to_status'])}',
                          style: const TextStyle(
                            fontWeight: FontWeight.w700,
                            fontSize: 17,
                          ),
                        ),
                        if ('${h['note'] ?? ''}'.isNotEmpty)
                          Text('${h['note']}'),
                        Text(supportDate(h['changed_at'] ?? h['occurred_at'])),
                      ],
                    ),
                  ),
              ],
            ),
          ),
        ),
      );
    } catch (_) {
      if (mounted) await feedback('History could not be loaded.');
    } finally {
      if (mounted) setState(() => activeAction = null);
    }
  }

  Future<void> milestone(JsonObject m) async {
    if (!writable) return;
    final from = '${m['state']}';
    final choices = <String, String>{
      if ((from == 'PENDING' || from == 'BLOCKED') &&
          !c.rows.any(
            (prev) =>
                prev['position'] < m['position'] &&
                !['COMPLETED', 'READY'].contains(prev['state']),
          ))
        'ACTIVE': from == 'BLOCKED' ? 'Resume' : 'Start',
      if (from == 'ACTIVE' && m['type'] == 'READY') 'READY': 'Mark ready',
      if (from == 'ACTIVE' || from == 'READY') 'COMPLETED': 'Complete',
      if (!['COMPLETED', 'BLOCKED', 'CANCELLED'].contains(from))
        'BLOCKED': 'Report block',
      if (!['COMPLETED', 'CANCELLED'].contains(from))
        'CANCELLED': 'Cancel milestone',
    };
    final choice = await crmChoose(context, crmLabel(m['type']), choices);
    if (choice == null || !mounted) return;
    final values = await productionForm(context, choices[choice.single]!, [
      text(
        {},
        'note',
        'Note',
        kind: 'notes',
        required: choice.single == 'BLOCKED' || choice.single == 'CANCELLED',
        sample: 'Production update recorded by the bakery team.',
      ),
    ]);
    if (values == null || !mounted) return;
    if (choice.single == 'CANCELLED' &&
        (!await confirmSettings(
              context,
              'Cancel milestone?',
              'This records cancellation of this milestone.',
              danger: true,
            ) ||
            !mounted)) {
      return;
    }
    await submit(
      'Update milestone',
      '$root/milestones/{milestoneType}/{state}',
      {...values, 'expected_version': m['version']},
      path: {
        ...params,
        'milestoneType': '${m['type']}',
        'state': choice.single,
      },
    );
  }

  Widget designValue(Object? value) {
    if (value is Map) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          for (final e in jsonObject(value).entries) ...[
            Text(
              crmLabel(e.key),
              style: const TextStyle(fontWeight: FontWeight.w600),
            ),
            designValue(e.value),
            const SizedBox(height: 8),
          ],
        ],
      );
    }
    if (value is List) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [for (final v in value) designValue(v)],
      );
    }
    return Text(
      value == null
          ? 'Not specified'
          : value is bool
          ? (value ? 'Yes' : 'No')
          : '$value',
    );
  }

  Future<void> compare(JsonObject change) async {
    if (reading || c.busy || preparing) return;
    setState(() => preparing = true);
    try {
      final from = specs
              .where((s) => s['revision'] == change['from_revision'])
              .firstOrNull,
          to = specs
              .where((s) => s['revision'] == change['to_revision'])
              .firstOrNull;
      if (to == null) {
        await feedback(
          'The proposed design could not be loaded. Refresh before reviewing.',
        );
        return;
      }
      final rows = from == null
          ? <JsonObject>[
              for (final e in jsonObject(to['specification']).entries)
                {'field': e.key, 'before': null, 'after': e.value},
            ]
          : c.collection(
              await c.fetch(
                '$root/specifications/diff',
                params: params,
                query: {'from': '${from['id']}', 'to': '${to['id']}'},
              ),
            );
      if (!mounted) return;
      await Navigator.push(
        context,
        setupRoute<void>(
          Scaffold(
            appBar: AppBar(title: const Text('Design changes')),
            body: SafeArea(
              child: Center(
                child: ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 680),
                  child: ListView(
                    padding: const EdgeInsets.all(20),
                    children: [
                      Text(
                        '${change['reason']}',
                        style: const TextStyle(
                          fontSize: 20,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      const SizedBox(height: 20),
                      if (rows.isEmpty)
                        const SettingsNote('No specification fields changed.'),
                      for (final row in rows)
                        SettingsCard(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              SettingsHeading(crmLabel(row['field'])),
                              const Text(
                                'Previous',
                                style: TextStyle(fontWeight: FontWeight.w600),
                              ),
                              const SizedBox(height: 6),
                              designValue(row['before']),
                              const Divider(height: 28),
                              const Text(
                                'Proposed',
                                style: TextStyle(fontWeight: FontWeight.w600),
                              ),
                              const SizedBox(height: 6),
                              designValue(row['after']),
                            ],
                          ),
                        ),
                    ],
                  ),
                ),
              ),
            ),
          ),
        ),
      );
    } catch (_) {
      if (mounted) await feedback('The design comparison could not be loaded.');
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  Future<void> propose() async {
    if (!writable || eligibility['allowed'] != true) return;
    final old = effectiveSpecs.isEmpty
        ? <String, dynamic>{}
        : jsonObject(effectiveSpecs.last['specification']);
    final reason = await productionForm(context, 'Reason for design change', [
      text(
        {},
        'reason',
        'Reason',
        kind: 'notes',
        required: true,
        sample: 'Customer requested an updated inscription.',
      ),
    ]);
    if (reason == null || !mounted) return;
    final values = await pastryDesign(
      context,
      widget.runtime,
      old,
      requestId: widget.sourceId,
    );
    if (values == null || !mounted) return;
    await submit('Propose change', '$root/changes', {
      'reason': reason['reason'],
      'specification': values,
      'expected_version': subject['version'],
    });
  }

  Future<void> decide(JsonObject ch) async {
    if (!writable) return;
    final values = await productionForm(context, 'Review design change', [
      text(
        {},
        'decision',
        'Decision',
        kind: 'select',
        required: true,
        options: const {'APPROVED': 'Approve', 'DECLINED': 'Decline'},
      ),
      text(
        {},
        'note',
        'Decision note',
        kind: 'notes',
        sample: 'Design change reviewed by the bakery team.',
      ),
    ]);
    if (values == null || !mounted) return;
    await submit(
      'Save decision',
      '$pastryBase/changes/{changeId}/decision',
      {...values, 'expected_version': ch['version']},
      path: {'changeId': '${ch['id']}'},
    );
  }

  Future<void> pool() async {
    if (!writable) return;
    final choice = await crmChoose(
      context,
      'Choose capacity pool',
      matchingPools(),
      initial: subject['capacity_pool_id'] != null
          ? ['${subject['capacity_pool_id']}']
          : [],
    );
    if (choice == null || !mounted) return;
    setState(() => activeAction = 'Set capacity pool');
    final result = await c.save(
      method: 'PATCH',
      route: root,
      params: params,
      body: {...subject, 'capacity_pool_id': choice.single},
    );
    if (mounted) {
      await feedback(
        result == null
            ? c.error ?? 'The pool could not be saved.'
            : 'Capacity pool saved.',
        success: result != null,
      );
      preview = null;
      previewBody = null;
      await load();
      setState(() => activeAction = null);
    }
  }

  Future<void> capacityPreview() async {
    if (!writable) return;
    final values = await productionForm(
      context,
      'Production window',
      [
        text(
          {},
          'quantity',
          'Items to produce',
          kind: 'number',
          required: true,
          sample: 12,
          min: 1,
          max: 1000000,
        ),
        text(
          {},
          'window_start',
          'Starts',
          kind: 'date',
          required: true,
          sample: DateTime.now()
              .toUtc()
              .add(const Duration(days: 1))
              .toIso8601String(),
        ),
        text(
          {},
          'window_end',
          'Ends',
          kind: 'date',
          required: true,
          sample: DateTime.now()
              .toUtc()
              .add(const Duration(days: 1, hours: 2))
              .toIso8601String(),
        ),
        text(
          {},
          'production_line_id',
          'Production line',
          kind: 'select',
          options: matchingLines(subject['capacity_pool_id'] as String?),
        ),
      ],
      validate: (v) =>
          DateTime.parse(
            v['window_end'],
          ).isAfter(DateTime.parse(v['window_start']))
          ? null
          : 'End time must follow start time.',
    );
    if (values == null || !mounted) return;
    setState(() => activeAction = 'Check capacity');
    final r = await c.save(
      method: 'POST',
      route: '$root/capacity-preview',
      params: params,
      body: values,
    );
    if (mounted) {
      setState(() {
        preview = r;
        previewBody = r == null ? null : values;
        activeAction = null;
      });
      if (r == null) {
        await feedback(c.error ?? 'Capacity could not be checked.');
      }
    }
  }

  Future<void> createBatch() async {
    if (!writable || preview?['available'] != true || previewBody == null) {
      return;
    }
    if (!await confirmSettings(
          context,
          'Create production batch?',
          'The server will check capacity again and reserve it for this batch.',
        ) ||
        !mounted) {
      return;
    }
    final result = await submit(
      'Create production batch',
      '$root/production-batches',
      previewBody!,
    );
    if (result != null && mounted) {
      preview = null;
      previewBody = null;
      await Navigator.push(
        context,
        setupRoute<void>(
          ProductionOperationsScreen(
            runtime: widget.runtime,
            jobId: '${result['production_job_id']}',
          ),
        ),
      );
    }
  }

  Widget badge(Object? raw) {
    final value = '${raw ?? 'UNKNOWN'}',
        color =
            (['APPROVED', 'COMPLETED', 'READY', 'ACCEPTED'].contains(value)
                    ? KarryamColors.tealInk
                    : [
                        'BLOCKED',
                        'CANCELLED',
                        'DECLINED',
                        'EXPIRED',
                      ].contains(value)
                    ? const Color(0xFFBC575B)
                    : ['PENDING_APPROVAL', 'PENDING', 'OPEN'].contains(value)
                    ? const Color(0xFFB98232)
                    : const Color(0xFF697A8D))
                .forInk(context);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
      decoration: BoxDecoration(
        color: color.withValues(alpha: .12),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Text(
        crmLabel(value),
        style: TextStyle(
          color: color,
          fontWeight: FontWeight.w700,
          fontSize: 12,
        ),
      ),
    );
  }

  Widget button(
    String label,
    VoidCallback? callback, {
    bool secondary = false,
    bool danger = false,
  }) => Padding(
    padding: const EdgeInsets.only(top: 12),
    child: SettingsAction(
      label,
      onPressed: callback,
      busy: activeAction == label && c.busy,
      secondary: secondary,
      danger: danger,
    ),
  );
  Widget empty(String message) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 32),
    child: Column(
      children: [
        Icon(
          work.icon,
          size: 64,
          color: const Color(0xFF8B65B9).forInk(context),
        ),
        const SizedBox(height: 16),
        Text(
          message,
          textAlign: TextAlign.center,
          style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w600),
        ),
      ],
    ),
  );
  List<Widget> content() {
    if (standard) {
      return [
        SettingsCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                '${subject['name']}',
                style: const TextStyle(
                  fontSize: 26,
                  fontWeight: FontWeight.w800,
                ),
              ),
              const SizedBox(height: 12),
              badge(subject['status']),
              SettingsFact(
                'Capacity pool',
                matchingPools()[subject['capacity_pool_id']] ??
                    'Not configured',
              ),
              SettingsFact(
                'Capacity per item',
                '${subject['workload_units_per_item']} units',
              ),
            ],
          ),
        ),
        button('Set capacity pool', writable ? pool : null, secondary: true),
        SettingsLink(
          'Capacity controls',
          Icons.donut_large_rounded,
          onTap: () => Navigator.push(
            context,
            setupRoute<void>(ProductionControlsHub(runtime: widget.runtime)),
          ),
        ),
        if (subject['status'] == 'ACTIVE' &&
            subject['capacity_pool_id'] != null)
          button('Check capacity', writable ? capacityPreview : null),
        if (preview != null)
          SettingsCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  preview!['available'] == true
                      ? 'Capacity available'
                      : 'Capacity unavailable',
                  style: TextStyle(
                    fontSize: 20,
                    fontWeight: FontWeight.w700,
                    color:
                        (preview!['available'] == true
                                ? KarryamColors.tealInk
                                : const Color(0xFFBC575B))
                            .forInk(context),
                  ),
                ),
                SettingsFact('Quantity', '${previewBody?['quantity']} items'),
                SettingsFact(
                  'Starts',
                  supportDate(previewBody?['window_start']),
                ),
                SettingsFact('Ends', supportDate(previewBody?['window_end'])),
                if (preview!['blocking_reason'] != null)
                  SettingsNote('${preview!['blocking_reason']}', warning: true),
              ],
            ),
          ),
        if (preview != null)
          button(
            'Create production batch',
            writable && preview!['available'] == true ? createBatch : null,
          ),
        if (subject['status'] != 'ACTIVE')
          const SettingsNote(
            'Standard production requires an active product.',
            warning: true,
          ),
      ];
    }
    if (work == PastryWork.quotes && widget.quoteId != null) {
      final q = c.record!,
          fresh =
              DateTime.tryParse(
                '${q['expires_at']}',
              )?.isAfter(DateTime.now()) ==
              true,
          current =
              effectiveSpecs.lastOrNull?['id'] == q['specification_version_id'],
          active = [
            'DRAFT',
            'PENDING_APPROVAL',
            'APPROVED',
          ].contains(q['status']);
      return [
        SettingsCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Text(
                    'Quote ${q['revision']}',
                    style: const TextStyle(
                      fontSize: 22,
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                  const Spacer(),
                  badge(q['status']),
                ],
              ),
              const SizedBox(height: 20),
              Text(
                dashboardMoney(q['amount_minor'], '${q['currency']}'),
                style: const TextStyle(
                  fontSize: 32,
                  fontWeight: FontWeight.w800,
                ),
              ),
              SettingsFact('Expires', supportDate(q['expires_at'])),
              if (q['production_start'] != null)
                SettingsFact(
                  'Production starts',
                  supportDate(q['production_start']),
                ),
              if (q['production_end'] != null)
                SettingsFact(
                  'Production ends',
                  supportDate(q['production_end']),
                ),
              if ('${q['vendor_terms'] ?? ''}'.isNotEmpty) ...[
                const SettingsHeading('Terms'),
                Text('${q['vendor_terms']}'),
              ],
              if (q['approved_at'] != null)
                SettingsFact('Approved', supportDate(q['approved_at'])),
              if (q['accepted_at'] != null)
                SettingsFact('Accepted', supportDate(q['accepted_at'])),
            ],
          ),
        ),
        if (!fresh && active)
          const SettingsNote(
            'This quote has expired. Revise it before continuing.',
            warning: true,
          ),
        if (!current && active)
          const SettingsNote(
            'The design specification has changed. Revise the quote to use the latest approved design.',
            warning: true,
          ),
        if (q['reservation_id'] != null)
          SettingsLink(
            'Reserved capacity',
            Icons.donut_large_rounded,
            onTap: () => Navigator.push(
              context,
              setupRoute<void>(
                ProductionReservationScreen(
                  runtime: widget.runtime,
                  reservationId: '${q['reservation_id']}',
                ),
              ),
            ),
          ),
        for (final a in (q['capacity_requirements'] as List? ?? []).map(
          jsonObject,
        ))
          SettingsFact(
            'Capacity allocation',
            '${a['quantity']} units / ${a['capacity_pool_id']}',
          ),
        if (active)
          button(
            'Revise quote',
            writable ? () => quoteEditor(revise: true) : null,
            secondary: true,
          ),
        if (['DRAFT', 'PENDING_APPROVAL'].contains(q['status']))
          button(
            'Approve quote',
            writable && fresh && current ? () => quoteAction('approve') : null,
          ),
        if (q['status'] == 'APPROVED')
          button(
            'Accept quote',
            writable && fresh && current ? () => quoteAction('accept') : null,
          ),
        if (active)
          button(
            'Decline quote',
            writable ? () => quoteAction('decline') : null,
            danger: true,
          ),
        if (q['status'] == 'ACCEPTED')
          const SettingsNote(
            'Production has been planned. Customer order conversion is handled by the order service.',
          ),
      ];
    }
    return [
      Text(
        '${subject['title']}',
        style: const TextStyle(fontSize: 24, fontWeight: FontWeight.w800),
      ),
      const SizedBox(height: 20),
      if (c.rows.isEmpty)
        empty(
          work == PastryWork.quotes
              ? 'Your quotes will appear here.'
              : work == PastryWork.milestones
              ? 'Milestones appear after quote acceptance.'
              : 'Design changes will appear here.',
        ),
      for (final r in c.rows)
        SettingsCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Expanded(
                    child: Text(
                      work == PastryWork.quotes
                          ? 'Quote ${r['revision']}'
                          : work == PastryWork.milestones
                          ? crmLabel(r['type'])
                          : 'Revision ${r['from_revision']} to ${r['to_revision']}',
                      style: const TextStyle(
                        fontSize: 18,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                  ),
                  badge(r['status'] ?? r['state']),
                ],
              ),
              if (work == PastryWork.quotes) ...[
                const SizedBox(height: 12),
                Text(
                  dashboardMoney(r['amount_minor'], '${r['currency']}'),
                  style: const TextStyle(
                    fontSize: 26,
                    fontWeight: FontWeight.w800,
                  ),
                ),
                button('View quote', () => openQuote(r), secondary: true),
              ],
              if (work == PastryWork.milestones) ...[
                button(
                  'Update ${crmLabel(r['type']).toLowerCase()}',
                  writable && r['state'] != 'COMPLETED'
                      ? () => milestone(r)
                      : null,
                  secondary: true,
                ),
                SettingsLink(
                  'History',
                  Icons.history_rounded,
                  onTap: () => history(
                    '${crmLabel(r['type'])} history',
                    '$root/milestones/{milestoneType}/history',
                    path: {...params, 'milestoneType': '${r['type']}'},
                  ),
                ),
              ],
              if (work == PastryWork.changes) ...[
                const SizedBox(height: 10),
                Text('${r['reason']}'),
                if ('${r['decision_note'] ?? ''}'.isNotEmpty)
                  Text('${r['decision_note']}'),
                button(
                  'View design changes',
                  !reading && !preparing && !c.busy ? () => compare(r) : null,
                  secondary: true,
                ),
                if (r['status'] == 'OPEN')
                  button(
                    'Review change',
                    writable ? () => decide(r) : null,
                    secondary: true,
                  ),
                SettingsLink(
                  'Decision history',
                  Icons.history_rounded,
                  onTap: () => history(
                    'Decision history',
                    '$pastryBase/changes/{changeId}/history',
                    path: {'changeId': '${r['id']}'},
                  ),
                ),
              ],
            ],
          ),
        ),
      if (work == PastryWork.quotes)
        button(
          'Create quote',
          writable &&
                  effectiveSpecs.isNotEmpty &&
                  !c.rows.any(
                    (q) => [
                      'DRAFT',
                      'PENDING_APPROVAL',
                      'APPROVED',
                    ].contains(q['status']),
                  ) &&
                  ![
                    'CANCELLED',
                    'CONVERTED',
                    'COMPLETED',
                    'ACCEPTED_PENDING_ORDER',
                  ].contains(subject['status'])
              ? quoteEditor
              : null,
        ),
      if (work == PastryWork.changes)
        button(
          'Propose change',
          writable &&
                  effectiveSpecs.isNotEmpty &&
                  eligibility['allowed'] == true
              ? propose
              : null,
        ),
      if (work == PastryWork.milestones)
        SettingsLink(
          'Production queue',
          Icons.precision_manufacturing_outlined,
          onTap: () => Navigator.push(
            context,
            setupRoute<void>(
              ProductionOperationsScreen(runtime: widget.runtime),
            ),
          ),
        ),
    ];
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      appBar: AppBar(
        title: Text(widget.quoteId != null ? 'Quote details' : work.title),
        actions: [
          IconButton(
            onPressed: reading || c.busy ? null : load,
            icon: const Icon(Icons.refresh_rounded),
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
                if ((reading || c.loading) && subject.isEmpty)
                  const PremiumSkeleton(detail: true)
                else ...[
                  if (readError != null || c.error != null)
                    SettingsNote(readError ?? c.error!, warning: true),
                  if (c.pending != null) ...[
                    const SettingsNote(
                      'A saved change has an unconfirmed outcome. Retry it before making another change.',
                      warning: true,
                    ),
                    button(
                      'Retry saved change',
                      c.busy || reading ? null : retry,
                    ),
                  ],
                  if (subject.isNotEmpty &&
                      (widget.quoteId == null || c.record != null))
                    ...content(),
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
