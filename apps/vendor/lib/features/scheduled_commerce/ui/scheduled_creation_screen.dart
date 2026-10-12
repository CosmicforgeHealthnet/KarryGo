import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../orders/ui/order_form_sheet.dart';
import '../../production/ui/production_form_screen.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../../settings/ui/settings_design.dart';
import '../state/scheduled_creation_controller.dart';
import '../state/scheduling_workspace_controller.dart';
import 'scheduled_capacity_editor.dart';
import 'scheduling_workspace_screen.dart';

class ScheduledCreationScreen extends StatefulWidget {
  const ScheduledCreationScreen({super.key, required this.runtime});
  final VendorRuntime runtime;
  @override
  State<ScheduledCreationScreen> createState() =>
      _ScheduledCreationScreenState();
}

class _ScheduledCreationScreenState extends State<ScheduledCreationScreen> {
  late final c = ScheduledCreationController(widget.runtime);
  List<JsonObject> quotes = [];
  JsonObject? selected;
  bool reading = true, preparing = false, more = false;
  String? error;
  @override
  void initState() {
    super.initState();
    initialize();
  }

  Future<void> initialize() async {
    await c.restore();
    if (!mounted) return;
    if (c.pending == null && !c.blocked) {
      await load();
    } else {
      setState(() => reading = false);
    }
  }

  Future<void> load({bool append = false}) async {
    setState(() => reading = true);
    error = null;
    try {
      final result = (await c.app.scheduledCommerceApi.request(
        'GET',
        '$schedulingBase/scheduling-quotes',
        query: {
          'limit': '50',
          'offset': append ? '${quotes.length}' : '0',
          if (c.app.scope.branchId != null) 'branch_id': c.app.scope.branchId!,
        },
      )).object;
      final rows = (result['items'] as List).map(jsonObject).toList();
      for (final q in rows) {
        if (q['BusinessID'] != c.app.scope.vendorId ||
            q['QuoteID'] is! String ||
            q['CustomerID'] is! String ||
            q['BranchID'] is! String ||
            q['ApprovedAt'] == null ||
            c.app.scope.branchId != null &&
                q['BranchID'] != c.app.scope.branchId) {
          throw const FormatException('Invalid scheduling quote');
        }
      }
      quotes = append ? [...quotes, ...rows] : rows;
      more = result['pagination']['has_more'] == true;
    } catch (_) {
      error = 'Approved quotes could not be loaded. Refresh to try again.';
    } finally {
      if (mounted) setState(() => reading = false);
    }
  }

  Future<void> submit({JsonObject? source, JsonObject? values}) async {
    final result = await c.create(source: source, changes: values);
    if (!mounted) return;
    await showOrderFeedback(
      context,
      title: result == null ? 'Schedule not confirmed' : 'Delivery scheduled',
      message: result == null
          ? c.error ?? 'Try again.'
          : 'The delivery schedule is accepted. ${result['CapacityStatus'] == 'RESERVED' ? 'Production capacity is reserved.' : 'Production capacity was not required.'}',
      success: result != null,
      warning: result == null,
    );
    if (!mounted) return;
    if (result != null) {
      Navigator.pop(context, result);
    } else if (c.pending == null) {
      await load();
    }
  }

  Future<void> configure() async {
    if (selected == null ||
        reading ||
        preparing ||
        c.busy ||
        c.pending != null) {
      return;
    }
    setState(() => preparing = true);
    try {
      final source = (await c.app.scheduledCommerceApi.request(
        'GET',
        '$schedulingBase/scheduling-quotes/{quoteId}',
        pathParameters: {'quoteId': selected!['QuoteID']},
        query: {'branch_id': selected!['BranchID']},
      )).object;
      if (!mounted) return;
      final mock = widget.runtime.mode == VendorDataMode.mock,
          at = DateTime.now().toUtc().add(const Duration(days: 2));
      final values = await productionForm(
        context,
        'Schedule delivery',
        [
          PurchaseField(
            'DesiredDeliveryAt',
            'Delivery time',
            kind: 'date',
            required: true,
            initial: mock ? at.toIso8601String() : null,
          ),
          PurchaseField(
            'DeliveryWindowStart',
            'Delivery window starts',
            kind: 'date',
            initial: mock ? at.toIso8601String() : null,
          ),
          PurchaseField(
            'DeliveryWindowEnd',
            'Delivery window ends',
            kind: 'date',
            initial: mock
                ? at.add(const Duration(hours: 1)).toIso8601String()
                : null,
          ),
          PurchaseField(
            'ProductionReadyBy',
            'Production ready by',
            kind: 'date',
            initial: mock
                ? at.subtract(const Duration(hours: 2)).toIso8601String()
                : null,
          ),
          const PurchaseField(
            'ExpectedReadyAt',
            'Expected ready time',
            kind: 'date',
          ),
          const PurchaseField(
            'TentativeVehicleClass',
            'Suggested vehicle',
            kind: 'select',
            options: {
              'MOTORCYCLE': 'Motorcycle',
              'VAN': 'Van',
              'TRUCK': 'Truck',
            },
          ),
          const PurchaseField(
            'required',
            'Reserve production capacity',
            kind: 'bool',
            initial: false,
          ),
        ],
        validate: (v) {
          final delivery = DateTime.tryParse('${v['DesiredDeliveryAt']}'),
              start = DateTime.tryParse('${v['DeliveryWindowStart'] ?? ''}'),
              end = DateTime.tryParse('${v['DeliveryWindowEnd'] ?? ''}'),
              ready = DateTime.tryParse('${v['ProductionReadyBy'] ?? ''}');
          if (delivery == null || !delivery.isAfter(DateTime.now())) {
            return 'Choose a future delivery time.';
          }
          if (start != null && start.isBefore(DateTime.now()) ||
              start != null && end != null && !end.isAfter(start)) {
            return 'Check the delivery window.';
          }
          if (ready != null && !ready.isBefore(delivery)) {
            return 'Production must be ready before delivery.';
          }
          return null;
        },
      );
      if (values == null || !mounted) return;
      final reserve = values.remove('required') == true;
      final capacity = reserve
          ? await scheduledCapacity(context, widget.runtime, source['BranchID'])
          : <String, dynamic>{'required': false};
      if (capacity == null || !mounted) return;
      values['Capacity'] = capacity;
      final reviewed = await showModalBottomSheet<JsonObject>(
        context: context,
        isScrollControlled: true,
        useSafeArea: true,
        showDragHandle: true,
        builder: (_) => OrderFormSheet(
          title: 'Confirm delivery schedule?',
          message:
              'Delivery ${scheduleDate(values['DesiredDeliveryAt'])}. ${source['Address']['address']}. ${reserve ? 'The server will check and reserve your selected production capacity.' : 'No production capacity reservation.'} This does not complete payment or dispatch.',
          fields: const [],
          icon: Icons.event_available_rounded,
        ),
      );
      if (reviewed != null && mounted) {
        await submit(source: source, values: values);
      }
    } catch (_) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Quote unavailable',
          message:
              'The quote could not be prepared for scheduling. Refresh and try again.',
          warning: true,
        );
      }
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      appBar: AppBar(title: const Text('Schedule delivery')),
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 680),
            child: ListView(
              padding: const EdgeInsets.all(22),
              children: [
                if (reading && quotes.isEmpty)
                  const PremiumSkeleton(detail: true)
                else if (c.blocked)
                  SettingsNote(
                    c.error ?? 'Saved schedule recovery is unavailable.',
                    warning: true,
                  )
                else if (c.pending != null) ...[
                  const SettingsHeading('Finish your saved schedule'),
                  const SettingsNote(
                    'The last submission has an unconfirmed outcome. Retry the saved schedule before creating another.',
                    warning: true,
                  ),
                  SettingsFact(
                    'Delivery',
                    scheduleDate(c.pending!['body']['DesiredDeliveryAt']),
                  ),
                  SettingsAction(
                    'Retry saved schedule',
                    busy: c.busy,
                    onPressed: c.busy ? null : () => submit(),
                    icon: Icons.refresh_rounded,
                  ),
                ] else ...[
                  const SettingsHeading(
                    'Choose an approved quote',
                    subtitle:
                        'Only current customer-approved quotes for your business are shown.',
                  ),
                  if (error != null) ...[
                    SettingsNote(error!, warning: true),
                    SettingsAction(
                      'Refresh quotes',
                      secondary: true,
                      onPressed: reading ? null : () => load(),
                    ),
                  ],
                  if (quotes.isEmpty && error == null)
                    SettingsCard(
                      child: Column(
                        children: [
                          Icon(
                            Icons.event_note_outlined,
                            size: 64,
                            color: KarryamColors.tealInk.forInk(context),
                          ),
                          const SizedBox(height: 16),
                          const SettingsHeading('No approved quotes'),
                          const Text(
                            'Quotes appear after customer approval, while they are still valid.',
                            textAlign: TextAlign.center,
                          ),
                        ],
                      ),
                    ),
                  for (final q in quotes)
                    Padding(
                      padding: const EdgeInsets.only(bottom: 12),
                      child: SettingsCard(
                        child: ListTile(
                          contentPadding: EdgeInsets.zero,
                          leading: Icon(
                            Icons.fact_check_outlined,
                            color: KarryamColors.tealInk.forInk(context),
                          ),
                          title: Text(
                            '${(q['Lines'] as List).length} approved item(s)',
                            style: const TextStyle(fontWeight: FontWeight.w700),
                          ),
                          subtitle: Text(
                            '${q['Address']['address']}\nValid until ${scheduleDate(q['ExpiresAt'])}',
                          ),
                          trailing: Icon(
                            selected?['QuoteID'] == q['QuoteID'] &&
                                    selected?['BranchID'] == q['BranchID']
                                ? Icons.radio_button_checked_rounded
                                : Icons.radio_button_unchecked_rounded,
                          ),
                          onTap: preparing
                              ? null
                              : () => setState(() => selected = q),
                        ),
                      ),
                    ),
                  if (more)
                    SettingsAction(
                      'Load more quotes',
                      secondary: true,
                      busy: reading,
                      onPressed: reading ? null : () => load(append: true),
                    ),
                  const SizedBox(height: 12),
                  SettingsAction(
                    'Set delivery time',
                    icon: Icons.arrow_forward_rounded,
                    busy: preparing || c.busy,
                    onPressed:
                        selected == null ||
                            reading ||
                            preparing ||
                            error != null
                        ? null
                        : configure,
                  ),
                ],
              ],
            ),
          ),
        ),
      ),
    ),
  );
}
