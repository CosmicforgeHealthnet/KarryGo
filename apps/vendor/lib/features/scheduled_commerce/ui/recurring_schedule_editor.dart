import 'package:flutter/material.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../settings/ui/settings_design.dart';
import '../../orders/ui/order_form_sheet.dart';
import 'scheduling_workspace_screen.dart';
import '../../crm/ui/crm_design.dart' show crmLabel;

class RecurringScheduleEditor extends StatefulWidget {
  const RecurringScheduleEditor({
    super.key,
    required this.record,
    this.creating = false,
    this.reviewBeforeReturning = true,
  });
  final JsonObject record;
  final bool creating;
  final bool reviewBeforeReturning;
  @override
  State<RecurringScheduleEditor> createState() =>
      _RecurringScheduleEditorState();
}

class _RecurringScheduleEditorState extends State<RecurringScheduleEditor> {
  final form = GlobalKey<FormState>();
  late final headcount = TextEditingController(
    text: widget.record['Headcount']?.toString() ?? '',
  );
  late final meal = TextEditingController(
    text: widget.record['MealTime'] as String,
  );
  late final horizon = TextEditingController(
    text: '${widget.record['AdvanceHorizonDays']}',
  );
  late Set<int> weekdays = (widget.record['Weekdays'] as List)
      .cast<int>()
      .toSet();
  late DateTime start = DateTime.parse(widget.record['StartsOn'] as String);
  late DateTime? end = DateTime.tryParse(
    widget.record['EndsOn']?.toString() ?? '',
  );
  bool reviewing = false;
  @override
  void dispose() {
    headcount.dispose();
    meal.dispose();
    horizon.dispose();
    super.dispose();
  }

  Future<void> date(bool ending) async {
    final now = DateTime.now();
    final first = DateTime(now.year, now.month, now.day);
    final current = (ending ? end : start) ?? first;
    final selected = await showDatePicker(
      context: context,
      initialDate: current.isBefore(first) ? first : current,
      firstDate: first,
      lastDate: DateTime(now.year + 10),
    );
    if (selected != null && mounted) {
      setState(() {
        if (ending) {
          end = selected;
        } else {
          start = selected;
        }
      });
    }
  }

  Future<void> save() async {
    if (reviewing || !form.currentState!.validate() || weekdays.isEmpty) return;
    final now = DateTime.now();
    if (DateTime(
          start.year,
          start.month,
          start.day,
        ).isBefore(DateTime(now.year, now.month, now.day)) ||
        end != null && end!.isBefore(start)) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text(
            'Choose a current start date and an end date after it.',
          ),
        ),
      );
      return;
    }
    setState(() => reviewing = true);
    try {
      final confirmation = widget.reviewBeforeReturning
          ? await showModalBottomSheet<JsonObject>(
              context: context,
              isScrollControlled: true,
              useSafeArea: true,
              showDragHandle: true,
              builder: (_) => OrderFormSheet(
                title: widget.creating
                    ? 'Create meal schedule?'
                    : 'Save meal schedule?',
                message:
                    '${headcount.text} meals at ${meal.text} (${widget.record['Timezone']}). ${widget.creating ? 'The selected corporate account, destination, menu and capacity configuration will be used for this new schedule.' : 'Existing occurrence records stay unchanged.'}',
                fields: const [],
                icon: Icons.event_available_rounded,
              ),
            )
          : <String, dynamic>{};
      if (confirmation != null && mounted) {
        Navigator.pop(context, {
          'Headcount': int.parse(headcount.text),
          'MealTime': meal.text.trim(),
          'AdvanceHorizonDays': int.parse(horizon.text),
          'Weekdays': weekdays.toList()..sort(),
          'StartsOn': DateTime.utc(
            start.year,
            start.month,
            start.day,
          ).toIso8601String(),
          'EndsOn': end == null
              ? null
              : DateTime.utc(end!.year, end!.month, end!.day).toIso8601String(),
        });
      }
    } finally {
      if (mounted) setState(() => reviewing = false);
    }
  }

  Widget contextValue(Object? value) {
    if (value is Map) {
      if (value.isEmpty) return const Text('No additional settings');
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          for (final entry in value.entries)
            Padding(
              padding: const EdgeInsets.only(top: 6),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(crmLabel('${entry.key}')),
                  contextValue(entry.value),
                ],
              ),
            ),
        ],
      );
    }
    if (value is List) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [for (final item in value) contextValue(item)],
      );
    }
    return Text(
      value is bool ? (value ? 'Yes' : 'No') : '${value ?? 'Not provided'}',
    );
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    backgroundColor: KarryamColors.canvas.forSurface(context),
    appBar: AppBar(
      title: Text(
        widget.creating ? 'New meal schedule' : 'Edit recurring schedule',
      ),
    ),
    body: Form(
      key: form,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(22, 12, 22, 30),
        children: [
          SettingsHeading(
            'Meal schedule',
            subtitle: widget.record['Timezone'] as String,
          ),
          if (widget.creating)
            SettingsCard(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const SettingsHeading('Corporate plan settings'),
                  SettingsFact(
                    'Corporate account',
                    '${widget.record['OrganizationName'] ?? widget.record['CorporateRef']}',
                  ),
                  SettingsFact('Timezone', '${widget.record['Timezone']}'),
                  if (widget.record['MenuRotation'] is Map &&
                      widget.record['MenuRotation']['mode'] == 'FIXED') ...[
                    SettingsFact(
                      'Delivery address',
                      '${widget.record['DestinationSnapshot']['address']}',
                    ),
                    SettingsFact(
                      'Fixed menu',
                      '${(widget.record['MenuRotation']['lines'] as List).length} items - ${widget.record['MenuRotation']['currency']}',
                    ),
                    const SettingsNote(
                      'The approved menu and address are preserved. Capacity is reviewed before activation.',
                    ),
                  ],
                  for (final key
                      in [
                        'DestinationSnapshot',
                        'MenuRotation',
                        'ProductionRequirement',
                      ].where(
                        (_) =>
                            widget.record['MenuRotation'] is! Map ||
                            widget.record['MenuRotation']['mode'] != 'FIXED',
                      )) ...[
                    const SizedBox(height: 12),
                    Text(
                      crmLabel(key),
                      style: const TextStyle(fontWeight: FontWeight.w700),
                    ),
                    contextValue(widget.record[key]),
                  ],
                  const SizedBox(height: 8),
                  const Text(
                    'These settings are carried into the new schedule. Required production windows are reviewed next.',
                  ),
                ],
              ),
            ),
          SettingsCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                TextFormField(
                  controller: headcount,
                  keyboardType: TextInputType.number,
                  decoration: const InputDecoration(
                    labelText: 'Meals per occurrence',
                  ),
                  validator: (v) {
                    final n = int.tryParse(v ?? '');
                    return n == null || n < 1 || n > 1000000
                        ? 'Enter a quantity from 1 to 1,000,000'
                        : null;
                  },
                ),
                const SizedBox(height: 16),
                TextFormField(
                  controller: meal,
                  decoration: const InputDecoration(
                    labelText: 'Meal time',
                    hintText: '12:30',
                  ),
                  validator: (v) =>
                      RegExp(
                        r'^(?:[01]\d|2[0-3]):[0-5]\d$',
                      ).hasMatch(v?.trim() ?? '')
                      ? null
                      : 'Use a 24-hour time, such as 12:30',
                ),
                const SizedBox(height: 16),
                TextFormField(
                  controller: horizon,
                  keyboardType: TextInputType.number,
                  decoration: const InputDecoration(
                    labelText: 'Advance planning days',
                  ),
                  validator: (v) {
                    final n = int.tryParse(v ?? '');
                    return n == null || n < 1 || n > 90
                        ? 'Enter 1 to 90 days'
                        : null;
                  },
                ),
                const SizedBox(height: 20),
                const Text(
                  'Days',
                  style: TextStyle(fontWeight: FontWeight.w700),
                ),
                const SizedBox(height: 10),
                Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  children: [
                    for (var i = 1; i <= 7; i++)
                      FilterChip(
                        label: Text(
                          const [
                            'Mon',
                            'Tue',
                            'Wed',
                            'Thu',
                            'Fri',
                            'Sat',
                            'Sun',
                          ][i - 1],
                        ),
                        selected: weekdays.contains(i),
                        showCheckmark: true,
                        selectedColor: KarryamColors.mint.forSurface(context),
                        onSelected: (selected) => setState(() {
                          if (selected) {
                            weekdays.add(i);
                          } else {
                            weekdays.remove(i);
                          }
                        }),
                      ),
                  ],
                ),
                const SizedBox(height: 18),
                ListTile(
                  contentPadding: EdgeInsets.zero,
                  title: const Text('Starts'),
                  subtitle: Text(
                    scheduleDate(start.toIso8601String(), time: false),
                  ),
                  trailing: const Icon(Icons.calendar_month_outlined),
                  onTap: () => date(false),
                ),
                ListTile(
                  contentPadding: EdgeInsets.zero,
                  title: const Text('Ends'),
                  subtitle: Text(
                    end == null
                        ? 'No end date'
                        : scheduleDate(end!.toIso8601String(), time: false),
                  ),
                  trailing: const Icon(Icons.calendar_month_outlined),
                  onTap: () => date(true),
                ),
                if (end != null)
                  TextButton(
                    onPressed: () => setState(() => end = null),
                    child: const Text('Remove end date'),
                  ),
              ],
            ),
          ),
          const SizedBox(height: 20),
          SettingsAction(
            'Review changes',
            busy: reviewing,
            icon: Icons.check_rounded,
            onPressed: weekdays.isEmpty ? null : save,
          ),
        ],
      ),
    ),
  );
}
