import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../orders/ui/order_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../settings/ui/settings_design.dart';
import '../../production/ui/production_form_screen.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../state/recurring_creation_controller.dart';
import '../state/scheduling_workspace_controller.dart';
import 'recurring_schedule_editor.dart';
import 'recurring_first_setup_screen.dart';
import 'scheduling_workspace_screen.dart';

class RecurringCreationScreen extends StatefulWidget {
  const RecurringCreationScreen({super.key, required this.runtime});
  final VendorRuntime runtime;
  @override
  State<RecurringCreationScreen> createState() =>
      _RecurringCreationScreenState();
}

class _RecurringCreationScreenState extends State<RecurringCreationScreen> {
  late final plans = SchedulingWorkspaceController(
    widget.runtime,
    recurring: true,
  );
  late final creation = RecurringCreationController(widget.runtime);
  bool restoring = true, configuring = false;
  JsonObject? selected;
  @override
  void initState() {
    super.initState();
    initialize();
  }

  Future<void> initialize() async {
    await creation.restore();
    if (!mounted) return;
    setState(() => restoring = false);
    if (creation.pending == null && !creation.blocked) await plans.load();
  }

  Future<void> submit({JsonObject? changes, JsonObject? source}) async {
    final result = await creation.create(source: source, changes: changes);
    if (!mounted) return;
    await showOrderFeedback(
      context,
      title: result == null
          ? 'Schedule not confirmed'
          : 'Meal schedule created',
      message: result == null
          ? creation.error ?? 'Try again.'
          : 'Your new schedule is active. Upcoming occurrences will be prepared by the scheduling service.',
      success: result != null,
      warning: result == null,
    );
    if (!mounted) return;
    if (result != null) {
      Navigator.pop(context, result);
    } else if (creation.pending == null && !creation.blocked) {
      await plans.load();
    }
  }

  Future<void> configure() async {
    if (selected == null ||
        configuring ||
        creation.busy ||
        creation.pending != null) {
      return;
    }
    setState(() => configuring = true);
    final detail = SchedulingWorkspaceController(
      widget.runtime,
      recurring: true,
      id: selected!['ID'] as String,
    );
    try {
      await detail.load();
      if (!mounted) return;
      if (detail.record == null || detail.stale) {
        await showOrderFeedback(
          context,
          title: 'Plan unavailable',
          message: detail.error ?? 'Refresh the corporate plans and try again.',
          warning: true,
        );
        return;
      }
      final source = jsonObject(detail.record!);
      final today = DateTime.now();
      final draft = {
        ...source,
        'StartsOn': DateTime.utc(
          today.year,
          today.month,
          today.day + 1,
        ).toUtc().toIso8601String(),
        'EndsOn': null,
      };
      final changes = await orderPage<JsonObject>(
        context,
        RecurringScheduleEditor(record: draft, creating: true),
      );
      if (changes == null || !mounted) return;
      final capacity = jsonObject(source['ProductionRequirement'] ?? {});
      if (capacity['required'] == true) {
        final window = await productionForm(
          context,
          'Production capacity window',
          [
            PurchaseField(
              'window_start',
              'Production starts',
              kind: 'date',
              required: true,
              initial: capacity['window_start'],
            ),
            PurchaseField(
              'window_end',
              'Production ends',
              kind: 'date',
              required: true,
              initial: capacity['window_end'],
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
        changes['ProductionRequirement'] = {...capacity, ...window};
      }
      await submit(source: source, changes: changes);
    } finally {
      detail.dispose();
      if (mounted) setState(() => configuring = false);
    }
  }

  @override
  void dispose() {
    plans.dispose();
    creation.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: Listenable.merge([plans, creation]),
    builder: (context, _) => Scaffold(
      appBar: AppBar(title: const Text('New meal schedule')),
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 680),
            child: ListView(
              padding: const EdgeInsets.all(22),
              children: [
                if (restoring || plans.loading && plans.rows.isEmpty)
                  const PremiumSkeleton(detail: true)
                else if (creation.blocked)
                  SettingsNote(
                    creation.error ?? 'Schedule recovery is unavailable.',
                    warning: true,
                  )
                else if (creation.pending != null) ...[
                  const SettingsHeading('Finish your saved schedule'),
                  const SettingsNote(
                    'A previous submission has an unconfirmed outcome. Retry the same schedule before creating another.',
                    warning: true,
                  ),
                  SettingsFact(
                    'Meals per occurrence',
                    '${creation.pending!['body']['Headcount']}',
                  ),
                  SettingsFact(
                    'Meal time',
                    '${creation.pending!['body']['MealTime']}',
                  ),
                  SettingsAction(
                    'Retry saved schedule',
                    busy: creation.busy,
                    onPressed: creation.busy ? null : () => submit(),
                    icon: Icons.refresh_rounded,
                  ),
                ] else ...[
                  SettingsAction(
                    'First-time meal setup',
                    icon: Icons.add_business_rounded,
                    secondary: true,
                    onPressed: () async {
                      final result = await orderPage<JsonObject>(
                        context,
                        RecurringFirstSetupScreen(runtime: widget.runtime),
                      );
                      if (result != null && context.mounted) {
                        Navigator.pop(context, result);
                      }
                    },
                  ),
                  const SizedBox(height: 20),
                  const SettingsHeading(
                    'Choose a corporate plan',
                    subtitle:
                        'Use its account, destination, menu and capacity settings for a new schedule.',
                  ),
                  if (plans.error != null) ...[
                    SettingsNote(plans.error!, warning: true),
                    SettingsAction(
                      'Try again',
                      secondary: true,
                      onPressed: plans.loading ? null : () => plans.load(),
                    ),
                  ],
                  if (plans.rows.isEmpty && plans.error == null)
                    SettingsCard(
                      child: Column(
                        children: [
                          Icon(
                            Icons.event_repeat_rounded,
                            size: 64,
                            color: KarryamColors.tealInk.forInk(context),
                          ),
                          const SizedBox(height: 16),
                          const SettingsHeading('No corporate plans yet'),
                          const Text(
                            'Set up your first meal schedule using an active corporate account and an approved food menu.',
                            textAlign: TextAlign.center,
                          ),
                        ],
                      ),
                    ),
                  for (final plan in plans.rows.where(
                    (p) => '${p['CorporateRef'] ?? ''}'.isNotEmpty,
                  ))
                    Padding(
                      padding: const EdgeInsets.only(bottom: 12),
                      child: SettingsCard(
                        child: ListTile(
                          contentPadding: EdgeInsets.zero,
                          leading: Icon(
                            Icons.business_rounded,
                            color: selected?['ID'] == plan['ID']
                                ? KarryamColors.tealInk.forInk(context)
                                : const Color(0xFF3775C5).forInk(context),
                          ),
                          title: Text(
                            '${plan['Headcount']} meals at ${plan['MealTime']}',
                            style: const TextStyle(fontWeight: FontWeight.w700),
                          ),
                          subtitle: Text(
                            '${scheduleDays(plan['Weekdays'])} - ${plan['Timezone']}',
                          ),
                          trailing: Icon(
                            selected?['ID'] == plan['ID']
                                ? Icons.radio_button_checked_rounded
                                : Icons.radio_button_unchecked_rounded,
                            color: selected?['ID'] == plan['ID']
                                ? KarryamColors.tealInk.forInk(context)
                                : null,
                          ),
                          onTap: creation.busy
                              ? null
                              : () => setState(() => selected = plan),
                        ),
                      ),
                    ),
                  if (plans.hasMore)
                    SettingsAction(
                      'Load more plans',
                      secondary: true,
                      busy: plans.loading,
                      onPressed: plans.loading
                          ? null
                          : () => plans.load(more: true),
                    ),
                  const SizedBox(height: 12),
                  SettingsAction(
                    'Set up schedule',
                    icon: Icons.arrow_forward_rounded,
                    busy: creation.busy || configuring,
                    onPressed:
                        selected == null ||
                            plans.stale ||
                            creation.busy ||
                            configuring
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
