import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../crm/ui/crm_design.dart' show crmLabel;
import '../../crm/ui/crm_editors.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../orders/ui/order_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../orders/ui/order_form_sheet.dart';
import '../../production/ui/production_form_screen.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../../settings/state/settings_controller.dart';
import '../../settings/ui/settings_design.dart';
import '../state/rfq_workspace_controller.dart';
import 'scheduling_workspace_screen.dart';

class RfqWorkspaceScreen extends StatefulWidget {
  const RfqWorkspaceScreen({super.key, required this.runtime, this.id});
  final VendorRuntime runtime;
  final String? id;
  @override
  State<RfqWorkspaceScreen> createState() => _RfqWorkspaceScreenState();
}

class _RfqWorkspaceScreenState extends State<RfqWorkspaceScreen> {
  late final c = RfqWorkspaceController(
    widget.runtime,
    widget.id == null ? rfqBase : '$rfqBase/{rfqId}',
    parameters: params,
  );
  Map<String, String> get params =>
      widget.id == null ? {} : {'rfqId': widget.id!};
  List<JsonObject> quotes = [];
  bool reading = true, preparing = false;
  String filter = 'Open';
  String? readError, activeRevision;
  String label(Object? value) => crmLabel(
    '$value'.replaceAllMapped(
      RegExp(r'([a-z])([A-Z])'),
      (m) => '${m[1]} ${m[2]}',
    ),
  );
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

  Future<void> load({bool more = false}) async {
    if (c.busy || preparing) return;
    setState(() {
      reading = true;
      readError = null;
    });
    try {
      c.readQuery = widget.id == null
          ? {'limit': '50', 'offset': more ? '${c.rows.length}' : '0'}
          : {};
      await c.load(append: more);
      if (c.error != null) throw StateError(c.error!);
      if (widget.id != null) {
        quotes = c.collection(
          await c.fetch('$rfqBase/{rfqId}/quotes', params: params),
        );
        for (final quote in quotes) {
          if (quote['RFQID'] != widget.id ||
              quote['VendorID'] != c.app.scope.vendorId) {
            throw const FormatException('Invalid quote scope');
          }
        }
        if (c.app.scope.branchId != null) {
          quotes = quotes
              .where((q) => q['BranchID'] == c.app.scope.branchId)
              .toList();
        }
      }
    } catch (_) {
      c.stale = true;
      readError =
          c.error ??
          'Quote details could not be loaded. Refresh before continuing.';
    } finally {
      if (mounted) setState(() => reading = false);
    }
  }

  bool open(JsonObject r) =>
      ['OPEN', 'QUOTED'].contains(r['Status']) &&
      (DateTime.tryParse('${r['DeadlineAt']}')?.isAfter(DateTime.now()) ??
          false);
  Future<void> feedback(String message, {bool success = false}) =>
      showOrderFeedback(
        context,
        title: success ? 'Quote submitted' : 'Review required',
        message: message,
        success: success,
        warning: !success,
      );
  Future<void> recover() async {
    final result = await c.save();
    if (!mounted) return;
    await feedback(
      result == null
          ? c.error ?? 'The quote could not be confirmed.'
          : 'Your quote is confirmed. Customer acceptance is still required.',
      success: result != null,
    );
    if (mounted) await load();
  }

  Future<void> compose([JsonObject? previous]) async {
    if (!c.writable || preparing || c.record == null || !open(c.record!)) {
      return;
    }
    setState(() {
      preparing = true;
      activeRevision = previous?['ID'];
    });
    try {
      final settings = SettingsController(c.app, SettingsKind.branches);
      late final List<JsonObject> branches;
      try {
        branches = await settings.all(
          '/api/v1/vendor/businesses/{businessId}/branches',
          settings.scope,
        );
      } finally {
        settings.dispose();
      }
      final branchChoices = {
        for (final b in branches.where(
          (b) =>
              b['status'] == 'ACTIVE' &&
              (c.app.scope.branchId == null ||
                  b['id'] == c.app.scope.branchId) &&
              (previous == null
                  ? !quotes.any((q) => q['BranchID'] == b['id'])
                  : b['id'] == previous['BranchID']),
        ))
          '${b['id']}': '${b['display_name']}',
      };
      if (!mounted) return;
      if (branchChoices.isEmpty) {
        await feedback(
          quotes.isEmpty
              ? 'No available branch was found for this quote.'
              : 'Use Revise quotation to update a branch that already has a quote.',
        );
        return;
      }
      final r = c.record!, mock = widget.runtime.mode == VendorDataMode.mock;
      final requested = jsonObject(r['Capacity'] ?? {});
      final values = await productionForm(
        context,
        previous == null ? 'Create quotation' : 'Revise quotation',
        [
          PurchaseField(
            'branch_id',
            'Quoting branch',
            kind: 'select',
            required: true,
            options: branchChoices,
            initial:
                previous?['BranchID'] ??
                (mock || branchChoices.length == 1
                    ? branchChoices.keys.first
                    : null),
          ),
          PurchaseField(
            'price_minor',
            'Total quote (${r['Currency']})',
            kind: 'money',
            required: true,
            initial:
                previous?['PriceMinor'] ??
                (mock ? r['BudgetMinor'] ?? 12000000 : null),
          ),
          PurchaseField(
            'expires_at',
            'Quote expires',
            kind: 'date',
            required: true,
            initial: mock
                ? DateTime.now()
                      .toUtc()
                      .add(const Duration(days: 2))
                      .toIso8601String()
                : null,
          ),
          if (requested['required'] != true)
            PurchaseField(
              'required',
              'Include production capacity',
              kind: 'bool',
              initial: previous == null
                  ? false
                  : jsonObject(previous['Capacity'] ?? {})['required'] ?? false,
            ),
        ],
        validate: (v) =>
            DateTime.tryParse('${v['expires_at']}')?.isAfter(DateTime.now()) ==
                true
            ? null
            : 'Choose a future expiry.',
      );
      if (values == null || !mounted) return;
      JsonObject capacity = {'required': false};
      if (requested['required'] == true || values['required'] == true) {
        final pools = c
            .collection(
              (await c.app.productionApi.request(
                'GET',
                '/api/v1/vendor/production/pools',
              )).data,
            )
            .where(
              (p) =>
                  p['status'] == 'ACTIVE' &&
                  p['branch_id'] == values['branch_id'] &&
                  ('${requested['capability'] ?? ''}'.isEmpty ||
                      p['capability'] == requested['capability']),
            )
            .toList();
        if (!mounted) return;
        if (pools.isEmpty) {
          await feedback(
            'No matching production capacity is available for this branch.',
          );
          return;
        }
        final poolNames = {for (final p in pools) '${p['id']}': '${p['name']}'};
        final chosen = await crmChoose(
          context,
          'Production capacity',
          poolNames,
          multiple: true,
          initial: mock ? [poolNames.keys.first] : [],
        );
        if (chosen == null || chosen.isEmpty || !mounted) return;
        final capabilities = pools
            .where((p) => chosen.contains(p['id']))
            .map((p) => p['capability'])
            .toSet();
        if (capabilities.length != 1) {
          await feedback('Choose pools with the same production capability.');
          return;
        }
        final allocations = <JsonObject>[];
        for (final id in chosen) {
          final quantity = await productionForm(context, poolNames[id]!, [
            PurchaseField(
              'quantity',
              'Capacity units',
              kind: 'number',
              required: true,
              min: 1,
              max: 1000000,
              initial: mock ? 1 : null,
            ),
          ]);
          if (quantity == null || !mounted) return;
          allocations.add({
            'capacity_pool_id': id,
            'quantity': quantity['quantity'],
          });
        }
        final window = await productionForm(
          context,
          'Production window',
          [
            PurchaseField(
              'window_start',
              'Production starts',
              kind: 'date',
              required: true,
              initial: mock
                  ? DateTime.now()
                        .toUtc()
                        .add(const Duration(days: 1))
                        .toIso8601String()
                  : null,
            ),
            PurchaseField(
              'window_end',
              'Production ends',
              kind: 'date',
              required: true,
              initial: mock
                  ? DateTime.now()
                        .toUtc()
                        .add(const Duration(days: 1, hours: 2))
                        .toIso8601String()
                  : null,
            ),
          ],
          validate: (v) {
            final start = DateTime.tryParse('${v['window_start']}'),
                end = DateTime.tryParse('${v['window_end']}');
            return start == null || end == null || !end.isAfter(start)
                ? 'Choose an end after the start.'
                : null;
          },
        );
        if (window == null || !mounted) return;
        capacity = {
          'required': true,
          'capability': capabilities.single,
          'branch_id': values['branch_id'],
          ...window,
          'allocations': allocations,
        };
      }
      if (!mounted) return;
      final reviewed = await showModalBottomSheet<JsonObject>(
        context: context,
        isScrollControlled: true,
        useSafeArea: true,
        showDragHandle: true,
        builder: (_) => OrderFormSheet(
          title: previous == null
              ? 'Submit quotation?'
              : 'Submit new revision?',
          message:
              '${branchChoices[values['branch_id']]} - ${dashboardMoney(values['price_minor'], r['Currency'])}. Expires ${scheduleDate(values['expires_at'])}. ${capacity['required'] == true ? 'Includes ${(capacity['allocations'] as List).length} capacity allocation(s).' : 'No production capacity included.'} Customer acceptance and checkout are separate.',
          fields: const [],
          icon: Icons.request_quote_outlined,
        ),
      );
      if (reviewed == null || !mounted) return;
      final result = await c.save(
        method: 'POST',
        route: '$rfqBase/{rfqId}/quotes',
        params: params,
        body: {
          'branch_id': values['branch_id'],
          'price_minor': values['price_minor'],
          'currency': r['Currency'],
          'expires_at': values['expires_at'],
          'capacity': capacity,
          'snapshot': previous?['Snapshot'] ?? {},
        },
      );
      if (!mounted) return;
      await feedback(
        result == null
            ? c.error ?? 'The quote could not be confirmed.'
            : 'Your quotation is submitted. It is awaiting the customer decision.',
        success: result != null,
      );
    } catch (_) {
      if (mounted) {
        await feedback(
          'The quote could not be prepared. Refresh and try again.',
        );
      }
    } finally {
      if (mounted) {
        setState(() {
          preparing = false;
          activeRevision = null;
        });
        await load();
      }
    }
  }

  Widget evidence(Object? value) {
    if (value is Map) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          for (final e in value.entries)
            if (e.value is Map || e.value is List)
              Padding(
                padding: const EdgeInsets.only(top: 8),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      label(e.key),
                      style: const TextStyle(fontWeight: FontWeight.w600),
                    ),
                    evidence(e.value),
                  ],
                ),
              )
            else
              SettingsFact(
                label(e.key),
                e.value is bool ? (e.value ? 'Yes' : 'No') : '${e.value}',
              ),
        ],
      );
    }
    if (value is List) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [for (final item in value) evidence(item)],
      );
    }
    return Text(
      value is bool ? (value ? 'Yes' : 'No') : '${value ?? 'Not provided'}',
    );
  }

  List<Widget> detail() {
    final r = c.record!;
    final latest = <String, JsonObject>{};
    for (final q in quotes) {
      final id = '${q['QuoteID']}';
      if (latest[id] == null || q['Revision'] > latest[id]!['Revision']) {
        latest[id] = q;
      }
    }
    return [
      SettingsCard(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            SettingsHeading(
              '${r['Servings']} servings',
              subtitle: crmLabel('${r['Status']}'),
            ),
            SettingsFact('Requested for', scheduleDate(r['RequestedAt'])),
            SettingsFact('Quotation deadline', scheduleDate(r['DeadlineAt'])),
            if (r['BudgetMinor'] != null)
              SettingsFact(
                'Budget',
                dashboardMoney(r['BudgetMinor'], r['Currency']),
              ),
          ],
        ),
      ),
      for (final key in [
        'DestinationSnapshot',
        'MenuRequirements',
        'PackagingRequirements',
        'DeliveryTerms',
        'Capacity',
      ])
        if (r[key] is Map && (r[key] as Map).isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(top: 12),
            child: SettingsCard(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [SettingsHeading(label(key)), evidence(r[key])],
              ),
            ),
          ),
      const SizedBox(height: 18),
      SettingsAction(
        quotes.isEmpty ? 'Create quotation' : 'Quote from another branch',
        icon: Icons.add_rounded,
        busy: preparing && activeRevision == null,
        onPressed: c.writable && open(r) && !reading && !preparing
            ? compose
            : null,
      ),
      if (!open(r))
        const SettingsNote(
          'This request is closed for quotations.',
          warning: true,
        ),
      const SizedBox(height: 22),
      const SettingsHeading('Your quotations'),
      if (quotes.isEmpty) const SettingsNote('No quotation submitted yet.'),
      for (final q in quotes)
        Padding(
          padding: const EdgeInsets.only(top: 12),
          child: SettingsCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                SettingsHeading(
                  dashboardMoney(q['PriceMinor'], q['Currency']),
                  subtitle: 'Revision ${q['Revision']}',
                ),
                SettingsFact('Status', crmLabel('${q['Status']}')),
                SettingsFact('Expires', scheduleDate(q['ExpiresAt'])),
                for (final key in [
                  'Snapshot',
                  'Capacity',
                  'VendorTerms',
                  'PaymentPolicy',
                  'DepositPolicy',
                  'CancellationPolicy',
                ])
                  if (q[key] is Map && (q[key] as Map).isNotEmpty)
                    ExpansionTile(
                      tilePadding: EdgeInsets.zero,
                      title: Text(label(key)),
                      children: [evidence(q[key])],
                    ),
                if (latest['${q['QuoteID']}']?['ID'] == q['ID'])
                  SettingsAction(
                    'Revise quotation',
                    busy: preparing && activeRevision == q['ID'],
                    secondary: true,
                    icon: Icons.edit_outlined,
                    onPressed: c.writable && open(r) && !reading && !preparing
                        ? () => compose(q)
                        : null,
                  ),
              ],
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
        title: Text(
          widget.id == null ? 'Quotation requests' : 'Request details',
        ),
        actions: [
          IconButton(
            tooltip: 'Refresh',
            onPressed: reading || preparing || c.busy ? null : () => load(),
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 680),
            child: ListView(
              padding: const EdgeInsets.all(22),
              children: [
                if (reading && c.data == null)
                  const PremiumSkeleton(detail: true)
                else ...[
                  if (c.blocked)
                    SettingsNote(
                      c.error ?? 'Saved quotation recovery is unavailable.',
                      warning: true,
                    ),
                  if (c.pending != null) ...[
                    const SettingsNote(
                      'We could not confirm your last quotation was sent, possibly because the connection dropped. Tap retry to send it again. It will not be sent twice.',
                      warning: true,
                    ),
                    SettingsAction(
                      'Retry saved quotation',
                      busy: c.busy,
                      onPressed: c.busy || preparing ? null : recover,
                      icon: Icons.refresh_rounded,
                    ),
                  ],
                  if (readError != null)
                    SettingsNote(readError!),
                  if (widget.id != null && c.record != null) ...detail(),
                  if (widget.id == null) ...[
                    const SettingsHeading(
                      'Requests for your business',
                      subtitle: 'Review requirements and send your quotation.',
                    ),
                    Wrap(
                      spacing: 8,
                      children: [
                        for (final name in ['Open', 'All'])
                          ChoiceChip(
                            label: Text(name),
                            selected: filter == name,
                            showCheckmark: false,
                            onSelected: (_) => setState(() => filter = name),
                          ),
                      ],
                    ),
                    const SizedBox(height: 18),
                    if (c.rows
                            .where((r) => filter == 'All' || open(r))
                            .isEmpty &&
                        readError == null)
                      SettingsCard(
                        child: Column(
                          children: [
                            Icon(
                              Icons.mark_email_read_outlined,
                              size: 64,
                              color: KarryamColors.tealInk.forInk(context),
                            ),
                            const SizedBox(height: 16),
                            const SettingsHeading('No requests to quote'),
                            const Text(
                              'Eligible quotation requests will appear here.',
                              textAlign: TextAlign.center,
                            ),
                          ],
                        ),
                      ),
                    for (final r in c.rows.where(
                      (r) => filter == 'All' || open(r),
                    ))
                      Padding(
                        padding: const EdgeInsets.only(bottom: 12),
                        child: SettingsCard(
                          child: SettingsLink(
                            '${r['Servings']} servings',
                            Icons.request_quote_outlined,
                            subtitle:
                                '${scheduleDate(r['RequestedAt'])} - ${crmLabel('${r['Status']}')}',
                            onTap: () async {
                              await orderPage<void>(
                                context,
                                RfqWorkspaceScreen(
                                  runtime: widget.runtime,
                                  id: r['ID'] as String,
                                ),
                              );
                              if (mounted) await load();
                            },
                          ),
                        ),
                      ),
                    if (c.hasMore)
                      SettingsAction(
                        'Load more requests',
                        secondary: true,
                        busy: reading,
                        onPressed: reading ? null : () => load(more: true),
                      ),
                  ],
                ],
              ],
            ),
          ),
        ),
      ),
    ),
  );
}
