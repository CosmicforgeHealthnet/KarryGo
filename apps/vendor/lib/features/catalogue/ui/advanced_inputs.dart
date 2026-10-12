import 'package:flutter/material.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../settings/ui/settings_design.dart';

class CatalogueWeekdays extends StatelessWidget {
  const CatalogueWeekdays({
    super.key,
    required this.value,
    required this.onChanged,
  });
  final Object? value;
  final ValueChanged<Object?>? onChanged;
  @override
  Widget build(BuildContext context) {
    final days = (value as List? ?? []).cast<int>();
    return FormField<List<int>>(
      initialValue: days,
      validator: (_) => days.isEmpty ? 'Choose at least one day.' : null,
      builder: (field) => Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text('Days', style: TextStyle(fontWeight: FontWeight.w700)),
          const SizedBox(height: 12),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: [
              for (var day = 1; day <= 7; day++)
                FilterChip(
                  label: Text(
                    ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'][day - 1],
                  ),
                  selected: days.contains(day),
                  selectedColor: KarryamColors.mint.forSurface(context),
                  checkmarkColor: KarryamColors.tealInk.forInk(context),
                  side: BorderSide(
                    color:
                        (days.contains(day)
                                ? const Color(0xFF91DCCE)
                                : const Color(0xFFDDE7E3))
                            .forOutline(context),
                  ),
                  onSelected: onChanged == null
                      ? null
                      : (selected) {
                          final next = List<int>.from(days);
                          selected ? next.add(day) : next.remove(day);
                          next.sort();
                          field.didChange(next);
                          onChanged!(next);
                        },
                ),
            ],
          ),
          if (field.hasError)
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Text(
                field.errorText!,
                style: TextStyle(color: KarryamColors.error.forInk(context)),
              ),
            ),
        ],
      ),
    );
  }
}

class CatalogueTemporalInput extends StatelessWidget {
  const CatalogueTemporalInput({
    super.key,
    required this.label,
    required this.value,
    required this.onChanged,
    this.clock = false,
    this.timestamp = false,
    this.required = false,
  });
  final String label;
  final Object? value;
  final ValueChanged<Object?>? onChanged;
  final bool clock, timestamp, required;
  Future<void> pick(BuildContext context) async {
    if (clock) {
      final parts = value?.toString().split(':');
      final time = await showTimePicker(
        context: context,
        initialTime: parts?.length == 2
            ? TimeOfDay(hour: int.parse(parts![0]), minute: int.parse(parts[1]))
            : const TimeOfDay(hour: 9, minute: 0),
      );
      if (time != null) {
        onChanged!(
          '${time.hour.toString().padLeft(2, '0')}:${time.minute.toString().padLeft(2, '0')}',
        );
      }
      return;
    }
    final now = DateTime.now(),
        parsed = DateTime.tryParse(value?.toString() ?? '')?.toLocal();
    final date = await showDatePicker(
      context: context,
      initialDate: parsed ?? now,
      firstDate: DateTime(1),
      lastDate: DateTime(9999),
    );
    if (date == null || !context.mounted) return;
    if (!timestamp) {
      onChanged!(date.toIso8601String().substring(0, 10));
      return;
    }
    final time = await showTimePicker(
      context: context,
      initialTime: TimeOfDay.fromDateTime(
        parsed ?? now.add(const Duration(hours: 1)),
      ),
    );
    if (time != null) {
      onChanged!(
        DateTime(
          date.year,
          date.month,
          date.day,
          time.hour,
          time.minute,
        ).toUtc().toIso8601String(),
      );
    }
  }

  @override
  Widget build(BuildContext context) => FormField<Object?>(
    initialValue: value,
    validator: (_) =>
        required && value == null ? 'Choose ${label.toLowerCase()}.' : null,
    builder: (field) => Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Expanded(
              child: SettingsLink(
                label,
                clock ? Icons.schedule_rounded : Icons.calendar_month_outlined,
                subtitle: value == null
                    ? (required ? 'Choose time' : 'No limit')
                    : timestamp
                    ? DateTime.parse(
                        value.toString(),
                      ).toLocal().toString().substring(0, 16)
                    : value.toString(),
                onTap: onChanged == null ? null : () => pick(context),
              ),
            ),
            if (!required && value != null)
              IconButton(
                tooltip: 'Clear ${label.toLowerCase()}',
                onPressed: onChanged == null ? null : () => onChanged!(null),
                icon: Icon(
                  Icons.close_rounded,
                  size: 20,
                  color: KarryamColors.indigo.forInk(context),
                ),
              ),
          ],
        ),
        if (timestamp)
          Text(
            'Uses your phone timezone.',
            style: TextStyle(
              fontSize: 12,
              color: KarryamColors.indigo.forInk(context),
            ),
          ),
        if (field.hasError)
          Text(
            field.errorText!,
            style: TextStyle(color: KarryamColors.error.forInk(context)),
          ),
      ],
    ),
  );
}

class CatalogueFreshOptions extends StatelessWidget {
  const CatalogueFreshOptions({
    super.key,
    required this.value,
    required this.allowed,
    required this.onChanged,
  });
  final Object? value;
  final List<String> allowed;
  final ValueChanged<Object?>? onChanged;
  List<JsonObject> get rows => (value as List? ?? []).map(jsonObject).toList();
  Future<void> add(BuildContext context) async {
    final choices = allowed
        .where((v) => !rows.any((r) => r['value'] == v))
        .toList();
    final selected = await settingsSheet<String>(
      context,
      Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const SettingsHeading('Add a choice'),
          for (final v in choices)
            SettingsLink(
              dashboardLabel(v),
              Icons.add_circle_outline_rounded,
              onTap: () => Navigator.pop(context, v),
            ),
          if (choices.isEmpty) const Text('All supported choices are added.'),
        ],
      ),
    );
    if (selected != null) {
      onChanged!([
        ...rows,
        {
          'value': selected,
          'display_name': dashboardLabel(selected),
          'is_default': false,
        },
      ]);
    }
  }

  Future<void> rename(BuildContext context, int index) async {
    final input = TextEditingController(
      text: rows[index]['display_name'].toString(),
    );
    final form = GlobalKey<FormState>();
    final name = await settingsSheet<String>(
      context,
      Form(
        key: form,
        child: Column(
          children: [
            const SettingsHeading('Choice name'),
            TextFormField(
              controller: input,
              maxLength: 80,
              decoration: const InputDecoration(labelText: 'Display name'),
              validator: (v) =>
                  (v?.trim().isEmpty ?? true) ? 'Enter a name.' : null,
            ),
          ],
        ),
      ),
      footer: SettingsAction(
        'Save name',
        onPressed: () {
          if (form.currentState!.validate()) {
            Navigator.pop(context, input.text.trim());
          }
        },
      ),
    );
    input.dispose();
    if (name != null) {
      final next = rows.map((r) => Map<String, dynamic>.of(r)).toList();
      next[index]['display_name'] = name;
      onChanged!(next);
    }
  }

  @override
  Widget build(BuildContext context) => FormField<Object?>(
    initialValue: value,
    validator: (_) => rows.isEmpty ? 'Add at least one choice.' : null,
    builder: (field) => Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text(
          'Customer choices',
          style: TextStyle(fontWeight: FontWeight.w700),
        ),
        const SizedBox(height: 10),
        for (var index = 0; index < rows.length; index++)
          Padding(
            padding: const EdgeInsets.only(bottom: 12),
            child: Material(
              color: Color(0xFFF1F8F5).forSurface(context),
              borderRadius: BorderRadius.circular(18),
              child: Padding(
                padding: const EdgeInsets.all(12),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Expanded(
                          child: Text(
                            rows[index]['display_name'].toString(),
                            style: const TextStyle(fontWeight: FontWeight.w700),
                          ),
                        ),
                        IconButton(
                          tooltip: 'Edit choice name',
                          onPressed: onChanged == null
                              ? null
                              : () => rename(context, index),
                          icon: Icon(
                            Icons.edit_outlined,
                            size: 20,
                            color: KarryamColors.tealInk.forInk(context),
                          ),
                        ),
                        IconButton(
                          tooltip: 'Remove choice',
                          onPressed: onChanged == null
                              ? null
                              : () {
                                  final next = rows..removeAt(index);
                                  onChanged!(next);
                                },
                          icon: Icon(
                            Icons.close_rounded,
                            size: 20,
                            color: KarryamColors.error.forInk(context),
                          ),
                        ),
                      ],
                    ),
                    Text(
                      dashboardLabel(rows[index]['value']),
                      style: TextStyle(
                        fontSize: 12,
                        color: KarryamColors.indigo.forInk(context),
                      ),
                    ),
                    const SizedBox(height: 8),
                    FilterChip(
                      label: const Text('Default choice'),
                      selected: rows[index]['is_default'] == true,
                      selectedColor: KarryamColors.mint.forSurface(context),
                      checkmarkColor: KarryamColors.tealInk.forInk(context),
                      onSelected: onChanged == null
                          ? null
                          : (selected) {
                              final next = rows
                                  .map((r) => Map<String, dynamic>.of(r))
                                  .toList();
                              for (var i = 0; i < next.length; i++) {
                                next[i]['is_default'] = i == index && selected;
                              }
                              onChanged!(next);
                            },
                    ),
                  ],
                ),
              ),
            ),
          ),
        SettingsAction(
          'Add choice',
          secondary: true,
          icon: Icons.add_rounded,
          onPressed: onChanged == null ? null : () => add(context),
        ),
        if (field.hasError)
          Text(
            field.errorText!,
            style: TextStyle(color: KarryamColors.error.forInk(context)),
          ),
      ],
    ),
  );
}
