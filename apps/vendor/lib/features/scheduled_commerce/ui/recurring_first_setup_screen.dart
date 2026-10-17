import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../orders/ui/order_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../orders/ui/order_form_sheet.dart';
import '../../production/ui/production_form_screen.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../../settings/ui/settings_design.dart';
import '../state/recurring_creation_controller.dart';
import '../state/scheduling_workspace_controller.dart';
import 'recurring_schedule_editor.dart';
import 'scheduled_capacity_editor.dart';

class RecurringFirstSetupScreen extends StatefulWidget {
  const RecurringFirstSetupScreen({super.key, required this.runtime});
  final VendorRuntime runtime;
  @override
  State<RecurringFirstSetupScreen> createState() =>
      _RecurringFirstSetupScreenState();
}

class _RecurringFirstSetupScreenState extends State<RecurringFirstSetupScreen> {
  late final c = RecurringCreationController(widget.runtime);
  final rows = <JsonObject>[];
  JsonObject? selected;
  bool reading = true, preparing = false, more = false;
  int offset = 0;
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
        '$schedulingBase/recurring-setup-contexts',
        query: {
          'limit': '50',
          'offset': append ? '$offset' : '0',
          if (c.app.scope.branchId != null) 'branch_id': c.app.scope.branchId!,
        },
      )).object;
      final items = (result['items'] as List).map(jsonObject).toList();
      for (final item in items) {
        if (item['BusinessID'] != c.app.scope.vendorId ||
            item['CorporateRef'] is! String ||
            item['RequesterID'] is! String ||
            item['BranchID'] is! String ||
            c.app.scope.branchId != null &&
                item['BranchID'] != c.app.scope.branchId) {
          throw const FormatException('Invalid corporate setup');
        }
      }
      if (!append) {
        rows.clear();
        selected = null;
      }
      rows.addAll(items);
      more = result['pagination']['has_more'] == true;
      offset = result['pagination']['next_offset'] as int;
    } catch (_) {
      error =
          'Corporate meal options could not be loaded. Check your account access and try again.';
    } finally {
      if (mounted) setState(() => reading = false);
    }
  }

  Future<void> submit({JsonObject? source, JsonObject? changes}) async {
    final result = await c.create(source: source, changes: changes);
    if (!mounted) return;
    await showOrderFeedback(
      context,
      title: result == null
          ? 'Schedule not confirmed'
          : 'Meal schedule created',
      message: result == null
          ? c.error ?? 'Try again.'
          : 'Your schedule is active. Upcoming meals are proposals; customer checkout and payment remain separate.',
      success: result != null,
      warning: result == null,
    );
    if (!mounted) return;
    if (result != null) {
      Navigator.pop(context, result);
    } else if (c.pending == null && !c.blocked) {
      await load();
    }
  }

  Future<void> configure() async {
    if (selected == null ||
        preparing ||
        reading ||
        c.busy ||
        c.pending != null) {
      return;
    }
    setState(() => preparing = true);
    try {
      final mock = widget.runtime.mode == VendorDataMode.mock;
      final settings = await productionForm(context, 'Meal plan settings', [
        PurchaseField(
          'Timezone',
          'Meal timezone',
          required: true,
          initial: mock ? 'Africa/Lagos' : null,
        ),
        PurchaseField(
          'AllowanceMinor',
          'Allowance per person (${selected!['MenuRotation']['currency']})',
          kind: 'money',
          required: true,
          initial: 0,
        ),
        const PurchaseField(
          'TentativeVehicleClass',
          'Suggested vehicle',
          kind: 'select',
          options: {'MOTORCYCLE': 'Motorcycle', 'VAN': 'Van', 'TRUCK': 'Truck'},
        ),
        const PurchaseField(
          'required',
          'Plan production capacity',
          kind: 'bool',
          initial: false,
        ),
      ]);
      if (settings == null || !mounted) return;
      final now = DateTime.now();
      final source = <String, dynamic>{
        ...selected!,
        ...settings,
        'Weekdays': [1, 2, 3, 4, 5],
        'MealTime': '12:30',
        'Headcount': mock ? 20 : null,
        'AdvanceHorizonDays': 14,
        'StartsOn': DateTime.utc(
          now.year,
          now.month,
          now.day + 1,
        ).toUtc().toIso8601String(),
        'EndsOn': null,
        'ProductionRequirement': {'required': false},
      };
      source.remove('required');
      final changes = await orderPage<JsonObject>(
        context,
        RecurringScheduleEditor(
          record: source,
          creating: true,
          reviewBeforeReturning: false,
        ),
      );
      if (changes == null || !mounted) return;
      if (settings['required'] == true) {
        final cap = await scheduledCapacity(
          context,
          widget.runtime,
          source['BranchID'],
        );
        if (cap == null || !mounted) return;
        changes['ProductionRequirement'] = cap;
      }
      final confirmation = await showModalBottomSheet<JsonObject>(
        context: context,
        isScrollControlled: true,
        useSafeArea: true,
        showDragHandle: true,
        builder: (_) => OrderFormSheet(
          title: 'Activate meal schedule?',
          message:
              '${selected!['OrganizationName']} - ${changes['Headcount']} meals at ${changes['MealTime']} (${source['Timezone']}). The fixed quoted menu and delivery address will be preserved. ${settings['required'] == true ? 'Capacity is planned per occurrence.' : 'No production capacity is requested.'}',
          fields: const [],
          icon: Icons.event_available_rounded,
        ),
      );
      if (confirmation != null && mounted) {
        await submit(source: source, changes: changes);
      }
    } catch (_) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Setup unavailable',
          message: 'The meal settings could not be prepared. Please try again.',
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
      appBar: AppBar(title: const Text('Corporate meal setup')),
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 680),
            child: ListView(
              padding: const EdgeInsets.all(22),
              children: [
                if (reading && rows.isEmpty)
                  const PremiumSkeleton(detail: true)
                else if (c.blocked)
                  SettingsNote(
                    c.error ?? 'Resolve the saved schedule before continuing.',
                    warning: true,
                  )
                else if (c.pending != null) ...[
                  const SettingsHeading('Finish your saved schedule'),
                  const SettingsNote(
                    'Retry the saved submission before creating another.',
                    warning: true,
                  ),
                  SettingsAction(
                    'Retry saved schedule',
                    busy: c.busy,
                    onPressed: c.busy ? null : () => submit(),
                    icon: Icons.refresh_rounded,
                  ),
                ] else ...[
                  const SettingsHeading(
                    'Meals for your corporate customers',
                    subtitle:
                        'Choose an approved food menu linked to an active corporate account.',
                  ),
                  if (error != null) ...[
                    SettingsNote(error!),
                    SettingsAction(
                      'Try again',
                      secondary: true,
                      onPressed: reading ? null : () => load(),
                    ),
                  ],
                  if (rows.isEmpty && error == null)
                    const SettingsNote(
                      'No eligible meal plans yet. The account needs an active customer contact and a current approved food quote with a delivery address.',
                    ),
                  for (final row in rows)
                    Padding(
                      padding: const EdgeInsets.only(bottom: 12),
                      child: SettingsCard(
                        child: ListTile(
                          contentPadding: EdgeInsets.zero,
                          leading: Icon(
                            Icons.domain_rounded,
                            color: KarryamColors.tealInk.forInk(context),
                            size: 32,
                          ),
                          title: Text(
                            '${row['OrganizationName']}',
                            style: const TextStyle(fontWeight: FontWeight.w700),
                          ),
                          subtitle: Padding(
                            padding: const EdgeInsets.only(top: 6),
                            child: Text(
                              '${row['DestinationSnapshot']['address']}\n${(row['MenuRotation']['lines'] as List).length} menu items - ${row['MenuRotation']['currency']}',
                            ),
                          ),
                          isThreeLine: true,
                          trailing: Icon(
                            identical(selected, row)
                                ? Icons.radio_button_checked_rounded
                                : Icons.radio_button_unchecked_rounded,
                            color: identical(selected, row)
                                ? KarryamColors.tealInk.forInk(context)
                                : null,
                          ),
                          onTap: reading || preparing
                              ? null
                              : () => setState(() => selected = row),
                        ),
                      ),
                    ),
                  if (selected != null)
                    SettingsCard(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          const SettingsHeading('Fixed menu'),
                          for (final line
                              in selected!['MenuRotation']['lines'] as List)
                            SettingsFact(
                              '${line['item_ref']}',
                              '${line['quantity']} units',
                            ),
                          const SettingsNote(
                            'Quoted item quantities stay fixed. Changing headcount does not automatically scale the menu.',
                          ),
                        ],
                      ),
                    ),
                  if (more)
                    SettingsAction(
                      'Load more options',
                      secondary: true,
                      busy: reading,
                      onPressed: reading ? null : () => load(append: true),
                    ),
                  const SizedBox(height: 12),
                  SettingsAction(
                    'Configure meal schedule',
                    busy: preparing || c.busy,
                    icon: Icons.arrow_forward_rounded,
                    onPressed:
                        selected == null || reading || preparing || c.busy
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
