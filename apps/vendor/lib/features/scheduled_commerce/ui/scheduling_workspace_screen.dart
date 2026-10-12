import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../orders/ui/order_design.dart';
import '../../orders/ui/order_detail_screen.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../orders/ui/order_form_sheet.dart';
import '../../settings/ui/settings_design.dart';
import '../state/scheduling_workspace_controller.dart';
import 'recurring_schedule_editor.dart';
import 'recurring_creation_screen.dart';
import 'rfq_workspace_screen.dart';
import 'scheduled_creation_screen.dart';

Color scheduleColor(String value) => switch (value) {
  'CANCELLED' => KarryamColors.error,
  'PAUSED' => const Color(0xFFB27318),
  'ACCEPTED' || 'ACTIVE' || 'READY' => KarryamColors.tealInk,
  _ => const Color(0xFF64768A),
};
String scheduleDate(Object? value, {bool time = true}) {
  final parsed = DateTime.tryParse(value?.toString() ?? '')?.toLocal();
  if (parsed == null) return 'Not provided';
  const months = [
    'Jan',
    'Feb',
    'Mar',
    'Apr',
    'May',
    'Jun',
    'Jul',
    'Aug',
    'Sep',
    'Oct',
    'Nov',
    'Dec',
  ];
  final date = '${parsed.day} ${months[parsed.month - 1]} ${parsed.year}';
  return time
      ? '$date ? ${parsed.hour.toString().padLeft(2, '0')}:${parsed.minute.toString().padLeft(2, '0')}'
      : date;
}

String scheduleDays(Object? value) {
  const days = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
  return value is List
      ? value
            .whereType<int>()
            .where((d) => d >= 1 && d <= 7)
            .map((d) => days[d - 1])
            .join(' ? ')
      : 'Not provided';
}

class SchedulingWorkspaceScreen extends StatefulWidget {
  const SchedulingWorkspaceScreen({super.key, required this.runtime});
  final VendorRuntime runtime;
  @override
  State<SchedulingWorkspaceScreen> createState() =>
      _SchedulingWorkspaceScreenState();
}

class _SchedulingWorkspaceScreenState extends State<SchedulingWorkspaceScreen> {
  late final scheduled = SchedulingWorkspaceController(widget.runtime);
  late final recurring = SchedulingWorkspaceController(
    widget.runtime,
    recurring: true,
  );
  bool showRecurring = false;
  String filter = 'Upcoming';
  SchedulingWorkspaceController get c => showRecurring ? recurring : scheduled;
  @override
  void initState() {
    super.initState();
    scheduled.load();
  }

  @override
  void dispose() {
    scheduled.dispose();
    recurring.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: Listenable.merge([scheduled, recurring]),
    builder: (context, _) => Scaffold(
      backgroundColor: KarryamColors.canvas.forSurface(context),
      appBar: AppBar(
        title: const Text('Scheduled orders'),
        actions: [
          IconButton(
            tooltip: 'Refresh schedules',
            onPressed: c.loading ? null : () => c.load(),
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () => c.load(),
        child: ListView(
          padding: const EdgeInsets.fromLTRB(22, 10, 22, 30),
          children: [
            SettingsCard(
              child: Row(
                children: [
                  Container(
                    padding: const EdgeInsets.all(16),
                    decoration: BoxDecoration(
                      color: const Color(
                        0xFF3775C5,
                      ).withValues(alpha: .10).forSurface(context),
                      borderRadius: BorderRadius.circular(20),
                    ),
                    child: Icon(
                      Icons.calendar_month_rounded,
                      size: 34,
                      color: const Color(0xFF3775C5).forInk(context),
                    ),
                  ),
                  const SizedBox(width: 16),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          showRecurring ? 'Recurring meals' : 'Plan ahead',
                          style: const TextStyle(
                            fontSize: 24,
                            fontWeight: FontWeight.w700,
                            letterSpacing: -.6,
                          ),
                        ),
                        const SizedBox(height: 6),
                        Text(
                          showRecurring
                              ? 'Meal schedules and their occurrences.'
                              : 'Delivery windows and preparation deadlines.',
                          style: TextStyle(
                            height: 1.4,
                            color: KarryamColors.indigo.forInk(context),
                          ),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 14),
            SettingsLink(
              'Quotation requests',
              Icons.request_quote_outlined,
              subtitle: 'Requests inviting your business to quote',
              onTap: () => orderPage<void>(
                context,
                RfqWorkspaceScreen(runtime: widget.runtime),
              ),
            ),
            const SizedBox(height: 20),
            SegmentedButton<bool>(
              segments: const [
                ButtonSegment(
                  value: false,
                  label: Text('Scheduled'),
                  icon: Icon(Icons.event_outlined),
                ),
                ButtonSegment(
                  value: true,
                  label: Text('Recurring'),
                  icon: Icon(Icons.repeat_rounded),
                ),
              ],
              selected: {showRecurring},
              showSelectedIcon: false,
              onSelectionChanged: (values) {
                setState(() {
                  showRecurring = values.single;
                  filter = showRecurring ? 'Active' : 'Upcoming';
                });
                if (c.rows.isEmpty && !c.loading) c.load();
              },
            ),
            if (!showRecurring) ...[
              const SizedBox(height: 16),
              SettingsAction(
                'Schedule delivery',
                icon: Icons.add_rounded,
                onPressed: () async {
                  final result = await orderPage<JsonObject>(
                    context,
                    ScheduledCreationScreen(runtime: widget.runtime),
                  );
                  if (!mounted || !context.mounted) return;
                  if (result != null) {
                    await orderPage<void>(
                      context,
                      ScheduleDetailScreen(
                        runtime: widget.runtime,
                        id: result['ID'] as String,
                      ),
                    );
                  }
                  if (mounted) await scheduled.load();
                },
              ),
            ],
            if (showRecurring) ...[
              const SizedBox(height: 16),
              SettingsAction(
                'New meal schedule',
                icon: Icons.add_rounded,
                onPressed: () async {
                  final result = await orderPage<JsonObject>(
                    context,
                    RecurringCreationScreen(runtime: widget.runtime),
                  );
                  if (!mounted || !context.mounted) return;
                  if (result != null) {
                    await orderPage<void>(
                      context,
                      ScheduleDetailScreen(
                        runtime: widget.runtime,
                        id: result['ID'] as String,
                        recurring: true,
                      ),
                    );
                  }
                  if (mounted) await recurring.load();
                },
              ),
            ],
            const SizedBox(height: 14),
            SingleChildScrollView(
              scrollDirection: Axis.horizontal,
              child: Row(
                children: [
                  for (final name
                      in showRecurring
                          ? ['Active', 'Paused', 'All']
                          : ['Upcoming', 'Past', 'Cancelled', 'All'])
                    Padding(
                      padding: const EdgeInsets.only(right: 8),
                      child: ChoiceChip(
                        label: Text(name),
                        selected: filter == name,
                        showCheckmark: false,
                        onSelected: (_) => setState(() => filter = name),
                        selectedColor: KarryamColors.mint.forSurface(context),
                        backgroundColor: Colors.white.forSurface(context),
                        side: BorderSide(
                          color: const Color(0xFFE4ECE7).forOutline(context),
                        ),
                      ),
                    ),
                ],
              ),
            ),
            const SizedBox(height: 16),
            if (c.loading && c.rows.isEmpty) const PremiumSkeleton(),
            if (c.error != null) SettingsNote(c.error!),
            if (!c.loading && c.error == null && filtered.isEmpty)
              DashboardEmptyState(
                story: EmptyStory.orders,
                iconOverride: Icons.event_available_rounded,
                title: showRecurring
                    ? 'No ${filter.toLowerCase()} schedules'
                    : 'No ${filter.toLowerCase()} orders',
                message: showRecurring
                    ? 'Corporate meal schedules will appear here when available.'
                    : 'Orders with an accepted delivery schedule will appear here.',
                action: 'Refresh',
                onAction: () => c.load(),
              ),
            for (final row in filtered)
              Padding(
                padding: const EdgeInsets.only(bottom: 12),
                child: _card(row),
              ),
            if (c.hasMore)
              SettingsAction(
                'Load more',
                secondary: true,
                busy: c.loading,
                onPressed: () => c.load(more: true),
              ),
          ],
        ),
      ),
    ),
  );
  List<JsonObject> get filtered => c.rows.where((row) {
    final status = row[showRecurring ? 'Status' : 'SchedulingStatus'];
    if (filter == 'All') return true;
    if (showRecurring) {
      return status == (filter == 'Active' ? 'ACTIVE' : 'PAUSED');
    }
    if (filter == 'Cancelled') return status == 'CANCELLED';
    if (status == 'CANCELLED') return false;
    final at = DateTime.tryParse(row['DesiredDeliveryAt']?.toString() ?? '');
    return at != null &&
        (filter == 'Upcoming'
            ? !at.isBefore(DateTime.now())
            : at.isBefore(DateTime.now()));
  }).toList();
  Widget _card(JsonObject row) {
    final status = row[showRecurring ? 'Status' : 'SchedulingStatus']
        .toString();
    return Material(
      color: Colors.white.forSurface(context),
      borderRadius: BorderRadius.circular(24),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: () async {
          await orderPage<void>(
            context,
            ScheduleDetailScreen(
              runtime: widget.runtime,
              id: row['ID'] as String,
              recurring: showRecurring,
            ),
          );
          if (mounted) await c.load();
        },
        child: Padding(
          padding: const EdgeInsets.all(18),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Expanded(
                    child: Text(
                      showRecurring
                          ? '${row['Headcount']} meals ? ${row['MealTime']}'
                          : scheduleDate(row['DesiredDeliveryAt']),
                      style: const TextStyle(
                        fontSize: 17,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                  ),
                  const SizedBox(width: 8),
                  Icon(
                    Icons.arrow_forward_rounded,
                    size: 20,
                    color: KarryamColors.tealInk.forInk(context),
                  ),
                ],
              ),
              const SizedBox(height: 10),
              Text(
                showRecurring
                    ? scheduleDays(row['Weekdays'])
                    : 'Ready by ${scheduleDate(row['ProductionReadyBy'])}',
                style: TextStyle(
                  fontSize: 12,
                  color: KarryamColors.indigo.forInk(context),
                ),
              ),
              const SizedBox(height: 14),
              Row(
                children: [
                  Icon(
                    showRecurring
                        ? Icons.repeat_rounded
                        : Icons.schedule_rounded,
                    size: 16,
                    color: scheduleColor(status).forInk(context),
                  ),
                  const SizedBox(width: 6),
                  Text(
                    dashboardLabel(status),
                    style: TextStyle(
                      fontSize: 12,
                      fontWeight: FontWeight.w600,
                      color: scheduleColor(status).forInk(context),
                    ),
                  ),
                  const Spacer(),
                  if (showRecurring)
                    Flexible(
                      child: Text(
                        row['Timezone']?.toString() ?? '',
                        overflow: TextOverflow.ellipsis,
                        style: TextStyle(
                          fontSize: 11,
                          color: KarryamColors.indigo.forInk(context),
                        ),
                      ),
                    ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class ScheduleDetailScreen extends StatefulWidget {
  const ScheduleDetailScreen({
    super.key,
    required this.runtime,
    required this.id,
    this.recurring = false,
  });
  final VendorRuntime runtime;
  final String id;
  final bool recurring;
  @override
  State<ScheduleDetailScreen> createState() => _ScheduleDetailScreenState();
}

class _ScheduleDetailScreenState extends State<ScheduleDetailScreen> {
  late final c = SchedulingWorkspaceController(
    widget.runtime,
    id: widget.id,
    recurring: widget.recurring,
  );
  bool presenting = false;
  @override
  void initState() {
    super.initState();
    c.load();
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> change(String target) async {
    if (presenting || !c.writable) return;
    presenting = true;
    try {
      final destructive = ['CANCELLED', 'ENDED'].contains(target);
      final confirmed = await showModalBottomSheet<JsonObject>(
        context: context,
        isScrollControlled: true,
        useSafeArea: true,
        showDragHandle: true,
        builder: (_) => OrderFormSheet(
          title:
              '${target == 'ACTIVE'
                  ? 'Resume'
                  : target == 'PAUSED'
                  ? 'Pause'
                  : target == 'ENDED'
                  ? 'End'
                  : 'Cancel'} ${widget.recurring ? 'schedule' : 'scheduled order'}?',
          message: widget.recurring
              ? 'This changes future scheduling. Existing occurrences remain separate records.'
              : 'This cancels the schedule and releases its reserved capacity where applicable. It does not confirm an order refund.',
          fields: const [],
          destructive: destructive,
          icon: destructive
              ? Icons.event_busy_rounded
              : Icons.pause_circle_outline,
        ),
      );
      if (confirmed == null || !mounted) return;
      final saved = await c.changeStatus(target);
      if (mounted) {
        await showOrderFeedback(
          context,
          title: saved
              ? '${widget.recurring ? 'Schedule' : 'Scheduled order'} updated'
              : 'Could not update',
          message: saved
              ? dashboardLabel(c.status)
              : c.error ?? 'Refresh to check the current state.',
          success: saved,
          warning: !saved,
        );
      }
    } finally {
      presenting = false;
    }
  }

  Future<void> edit() async {
    if (presenting || !c.writable) return;
    presenting = true;
    try {
      final changes = await orderPage<JsonObject>(
        context,
        RecurringScheduleEditor(record: c.record!),
      );
      if (changes == null || !mounted) return;
      final saved = await c.edit(changes);
      if (mounted) {
        await showOrderFeedback(
          context,
          title: saved ? 'Schedule updated' : 'Could not update schedule',
          message: saved
              ? 'Your meal schedule has been saved.'
              : c.error ?? 'Refresh before trying again.',
          success: saved,
          warning: !saved,
        );
      }
    } finally {
      presenting = false;
    }
  }

  Widget fact(String label, Object? value) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 9),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: TextStyle(
            fontSize: 12,
            color: KarryamColors.indigo.forInk(context),
          ),
        ),
        const SizedBox(height: 5),
        SelectableText(
          value?.toString().isNotEmpty == true
              ? value.toString()
              : 'Not provided',
          style: const TextStyle(fontSize: 15, fontWeight: FontWeight.w600),
        ),
      ],
    ),
  );
  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => PopScope(
      canPop: !c.busy,
      child: Scaffold(
        backgroundColor: KarryamColors.canvas.forSurface(context),
        appBar: AppBar(
          title: Text(
            widget.recurring ? 'Recurring schedule' : 'Scheduled order',
          ),
          actions: [
            IconButton(
              tooltip: 'Refresh schedule',
              onPressed: c.busy || c.loading ? null : () => c.load(),
              icon: const Icon(Icons.refresh_rounded),
            ),
          ],
        ),
        body: RefreshIndicator(
          onRefresh: () => c.load(),
          child: ListView(
            padding: const EdgeInsets.fromLTRB(22, 10, 22, 30),
            children: [
              if (c.loading && c.record == null) const PremiumSkeleton(),
              if (c.error != null) SettingsNote(c.error!),
              if (c.record != null) ...[
                SettingsCard(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Icon(
                        widget.recurring
                            ? Icons.repeat_rounded
                            : Icons.event_available_rounded,
                        size: 40,
                        color: const Color(0xFF3775C5).forInk(context),
                      ),
                      const SizedBox(height: 16),
                      Text(
                        widget.recurring
                            ? '${c.record!['Headcount']} meals'
                            : scheduleDate(c.record!['DesiredDeliveryAt']),
                        style: const TextStyle(
                          fontSize: 27,
                          fontWeight: FontWeight.w700,
                          letterSpacing: -.7,
                        ),
                      ),
                      const SizedBox(height: 10),
                      Text(
                        dashboardLabel(c.status),
                        style: TextStyle(
                          color: scheduleColor(c.status).forInk(context),
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                      if (widget.recurring) ...[
                        fact(
                          'Meal time',
                          '${c.record!['MealTime']} ? ${c.record!['Timezone']}',
                        ),
                        fact('Days', scheduleDays(c.record!['Weekdays'])),
                        fact(
                          'Starts',
                          scheduleDate(c.record!['StartsOn'], time: false),
                        ),
                        if (c.record!['EndsOn'] != null)
                          fact(
                            'Ends',
                            scheduleDate(c.record!['EndsOn'], time: false),
                          ),
                        fact(
                          'Advance planning',
                          '${c.record!['AdvanceHorizonDays']} days',
                        ),
                      ] else ...[
                        fact(
                          'Delivery window',
                          '${scheduleDate(c.record!['DeliveryWindowStart'])} ? ${scheduleDate(c.record!['DeliveryWindowEnd'])}',
                        ),
                        fact(
                          'Production ready by',
                          scheduleDate(c.record!['ProductionReadyBy']),
                        ),
                        fact(
                          'Expected ready time',
                          scheduleDate(c.record!['ExpectedReadyAt']),
                        ),
                        fact(
                          'Capacity',
                          dashboardLabel(c.record!['CapacityStatus']),
                        ),
                      ],
                    ],
                  ),
                ),
                const SizedBox(height: 16),
                if (widget.recurring) ...[
                  const SettingsHeading(
                    'Occurrences',
                    subtitle:
                        'Proposals and capacity records; these do not confirm payment.',
                  ),
                  if (c.occurrenceError != null)
                    SettingsNote(c.occurrenceError!),
                  if (c.occurrences.isEmpty && c.occurrenceError == null)
                    const SettingsCard(child: Text('No occurrences yet.')),
                  for (final occurrence in c.occurrences)
                    Padding(
                      padding: const EdgeInsets.only(bottom: 10),
                      child: SettingsCard(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              scheduleDate(occurrence['OccurrenceAt']),
                              style: const TextStyle(
                                fontWeight: FontWeight.w700,
                              ),
                            ),
                            const SizedBox(height: 7),
                            Text(
                              '${occurrence['Headcount']} meals ? ${dashboardLabel(occurrence['Status'])}',
                            ),
                            const SizedBox(height: 5),
                            Text(
                              'Capacity: ${dashboardLabel(occurrence['CapacityStatus'])}',
                              style: TextStyle(
                                fontSize: 12,
                                color: KarryamColors.indigo.forInk(context),
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                ] else if (c.record!['VendorOrderID']?.toString().isNotEmpty ==
                    true)
                  SettingsLink(
                    'Open order',
                    Icons.receipt_long_outlined,
                    onTap: () => orderPage<void>(
                      context,
                      OrderDetailScreen(
                        runtime: widget.runtime,
                        id: c.record!['VendorOrderID'] as String,
                        onChanged: () {},
                      ),
                    ),
                  ),
                if (c.busy) ...[
                  const SizedBox(height: 16),
                  const SettingsAction('Saving changes', busy: true),
                ],
                if (widget.recurring && c.writable) ...[
                  const SizedBox(height: 12),
                  SettingsAction(
                    'Edit schedule',
                    icon: Icons.edit_outlined,
                    onPressed: edit,
                    busy: c.busy,
                  ),
                  const SizedBox(height: 10),
                  SettingsAction(
                    c.status == 'ACTIVE' ? 'Pause schedule' : 'Resume schedule',
                    secondary: true,
                    icon: c.status == 'ACTIVE'
                        ? Icons.pause_circle_outline
                        : Icons.play_circle_outline,
                    onPressed: () =>
                        change(c.status == 'ACTIVE' ? 'PAUSED' : 'ACTIVE'),
                  ),
                  const SizedBox(height: 10),
                  SettingsAction(
                    'End schedule',
                    secondary: true,
                    icon: Icons.event_available_rounded,
                    onPressed: () => change('ENDED'),
                  ),
                ],
                if (c.writable) ...[
                  const SizedBox(height: 10),
                  SettingsAction(
                    widget.recurring
                        ? 'Cancel schedule'
                        : 'Cancel scheduled order',
                    danger: true,
                    icon: Icons.event_busy_outlined,
                    onPressed: () => change('CANCELLED'),
                    busy: c.busy,
                  ),
                ],
                const SizedBox(height: 20),
                ExpansionTile(
                  tilePadding: EdgeInsets.zero,
                  title: const Text('References'),
                  children: [
                    fact('Schedule', c.record!['ID']),
                    fact('Branch', c.record!['BranchID']),
                    if (widget.recurring)
                      fact('Corporate account', c.record!['CorporateRef']),
                  ],
                ),
              ],
            ],
          ),
        ),
      ),
    ),
  );
}
