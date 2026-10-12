import 'dart:convert';
import 'package:flutter/material.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../../settings/ui/settings_design.dart';

Future<JsonObject?> productionForm(
  BuildContext context,
  String title,
  List<PurchaseField> fields, {
  String? Function(JsonObject)? validate,
}) => Navigator.push<JsonObject>(
  context,
  setupRoute<JsonObject>(
    ProductionFormScreen(title: title, fields: fields, validate: validate),
  ),
);

class ProductionFormScreen extends StatefulWidget {
  const ProductionFormScreen({
    super.key,
    required this.title,
    required this.fields,
    this.validate,
  });
  final String title;
  final List<PurchaseField> fields;
  final String? Function(JsonObject)? validate;
  @override
  State<ProductionFormScreen> createState() => _ProductionFormScreenState();
}

class _ProductionFormScreenState extends State<ProductionFormScreen> {
  final form = GlobalKey<FormState>();
  late final text = {
    for (final f in widget.fields.where(
      (f) => !['select', 'bool', 'days', 'object'].contains(f.kind),
    ))
      f.key: TextEditingController(
        text: f.initial == null ? '' : '${f.initial}',
      ),
  };
  late final values = <String, dynamic>{
    for (final f in widget.fields)
      f.key: f.kind == 'days'
          ? List<int>.from(f.initial as List? ?? [])
          : f.initial,
  };
  String? error;
  @override
  void dispose() {
    for (final c in text.values) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> pick(PurchaseField f) async {
    final initial = DateTime.tryParse(text[f.key]!.text) ?? DateTime.now();
    if (f.kind == 'clock') {
      final parts = text[f.key]!.text.split(':');
      final time = await showTimePicker(
        context: context,
        initialTime: parts.length >= 2
            ? TimeOfDay(
                hour: int.tryParse(parts[0]) ?? 9,
                minute: int.tryParse(parts[1]) ?? 0,
              )
            : const TimeOfDay(hour: 9, minute: 0),
      );
      if (time != null) {
        text[f.key]!.text =
            '${time.hour.toString().padLeft(2, '0')}:${time.minute.toString().padLeft(2, '0')}';
      }
      return;
    }
    final date = await showDatePicker(
      context: context,
      initialDate: initial,
      firstDate: DateTime(2020),
      lastDate: DateTime(2100),
    );
    if (date == null || !mounted) return;
    if (f.kind == 'calendar') {
      text[f.key]!.text =
          '${date.year}-${date.month.toString().padLeft(2, '0')}-${date.day.toString().padLeft(2, '0')}';
      return;
    }
    final time = await showTimePicker(
      context: context,
      initialTime: TimeOfDay.fromDateTime(initial),
    );
    if (time != null) {
      text[f.key]!.text = DateTime(
        date.year,
        date.month,
        date.day,
        time.hour,
        time.minute,
      ).toUtc().toIso8601String();
    }
  }

  Future<void> editObject(PurchaseField f) async {
    final result = await settingsSheet<JsonObject>(
      context,
      ProductionRecordEditor(
        title: f.label,
        initial: values[f.key] is Map ? jsonObject(values[f.key]) : {},
      ),
    );
    if (result != null && mounted) setState(() => values[f.key] = result);
  }

  void submit() {
    if (!form.currentState!.validate()) return;
    final body = <String, dynamic>{};
    for (final f in widget.fields) {
      if (['select', 'bool', 'days', 'object'].contains(f.kind)) {
        body[f.key] = values[f.key];
      } else {
        final value = text[f.key]!.text.trim();
        body[f.key] = value.isEmpty
            ? null
            : f.kind == 'number'
            ? int.parse(value)
            : f.kind == 'date'
            ? DateTime.parse(value).toUtc().toIso8601String()
            : value;
      }
      if (f.required &&
          (body[f.key] == null ||
              body[f.key] is List && (body[f.key] as List).isEmpty ||
              body[f.key] is Map && (body[f.key] as Map).isEmpty)) {
        setState(() => error = 'Add ${f.label.toLowerCase()}.');
        return;
      }
    }
    final invalid = widget.validate?.call(body);
    if (invalid != null) {
      setState(() => error = invalid);
      return;
    }
    Navigator.pop(context, body);
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: Text(widget.title)),
    body: SafeArea(
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 640),
          child: ListView(
            padding: const EdgeInsets.fromLTRB(20, 12, 20, 28),
            children: [
              if (error != null) SettingsNote(error!, warning: true),
              Form(
                key: form,
                child: Column(
                  children: [
                    for (final f in widget.fields)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 20),
                        child: f.kind == 'days'
                            ? SettingsCard(
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    Text(
                                      f.label,
                                      style: const TextStyle(
                                        fontWeight: FontWeight.w700,
                                      ),
                                    ),
                                    const SizedBox(height: 14),
                                    Wrap(
                                      spacing: 8,
                                      runSpacing: 8,
                                      children: [
                                        for (var day = 1; day <= 7; day++)
                                          FilterChip(
                                            label: Text(
                                              [
                                                'Mon',
                                                'Tue',
                                                'Wed',
                                                'Thu',
                                                'Fri',
                                                'Sat',
                                                'Sun',
                                              ][day - 1],
                                            ),
                                            selected: (values[f.key] as List)
                                                .contains(day),
                                            onSelected: (selected) => setState(
                                              () {
                                                final days =
                                                    values[f.key] as List<int>;
                                                if (selected) {
                                                  days.add(day);
                                                } else {
                                                  days.remove(day);
                                                }
                                                days.sort();
                                              },
                                            ),
                                          ),
                                      ],
                                    ),
                                  ],
                                ),
                              )
                            : f.kind == 'object'
                            ? SettingsCard(
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    Text(
                                      f.label,
                                      style: const TextStyle(
                                        fontSize: 18,
                                        fontWeight: FontWeight.w700,
                                      ),
                                    ),
                                    const SizedBox(height: 12),
                                    if (values[f.key] is Map)
                                      for (final e
                                          in (values[f.key] as Map).entries)
                                        Padding(
                                          padding: const EdgeInsets.only(
                                            bottom: 8,
                                          ),
                                          child: Text(
                                            '${dashboardLabel('${e.key}')}: ${e.value}',
                                          ),
                                        ),
                                    SettingsAction(
                                      values[f.key] is Map &&
                                              (values[f.key] as Map).isNotEmpty
                                          ? 'Edit details'
                                          : 'Add details',
                                      secondary: true,
                                      icon: Icons.fact_check_outlined,
                                      onPressed: () => editObject(f),
                                    ),
                                  ],
                                ),
                              )
                            : f.kind == 'bool'
                            ? SwitchListTile.adaptive(
                                contentPadding: EdgeInsets.zero,
                                title: Text(f.label),
                                value: values[f.key] == true,
                                onChanged: (v) =>
                                    setState(() => values[f.key] = v),
                              )
                            : f.kind == 'select'
                            ? DropdownButtonFormField<String>(
                                initialValue:
                                    f.options.containsKey(values[f.key])
                                    ? values[f.key]
                                    : null,
                                isExpanded: true,
                                decoration: InputDecoration(labelText: f.label),
                                items: [
                                  if (!f.required)
                                    const DropdownMenuItem(
                                      value: '',
                                      child: Text('None'),
                                    ),
                                  for (final e in f.options.entries)
                                    DropdownMenuItem(
                                      value: e.key,
                                      child: Text(
                                        e.value,
                                        overflow: TextOverflow.ellipsis,
                                      ),
                                    ),
                                ],
                                onChanged: (v) =>
                                    values[f.key] = v == '' ? null : v,
                                validator: (v) =>
                                    f.required && (v == null || v.isEmpty)
                                    ? 'Choose ${f.label.toLowerCase()}.'
                                    : null,
                              )
                            : TextFormField(
                                controller: text[f.key],
                                readOnly: [
                                  'clock',
                                  'calendar',
                                  'date',
                                ].contains(f.kind),
                                onTap:
                                    [
                                      'clock',
                                      'calendar',
                                      'date',
                                    ].contains(f.kind)
                                    ? () => pick(f)
                                    : null,
                                keyboardType: f.kind == 'number'
                                    ? TextInputType.number
                                    : TextInputType.text,
                                minLines: f.kind == 'notes' ? 2 : 1,
                                maxLines: f.kind == 'notes' ? 4 : 1,
                                decoration: InputDecoration(
                                  labelText: f.label,
                                  suffixIcon:
                                      [
                                        'clock',
                                        'calendar',
                                        'date',
                                      ].contains(f.kind)
                                      ? Icon(
                                          f.kind == 'clock'
                                              ? Icons.schedule_rounded
                                              : Icons.calendar_month_rounded,
                                        )
                                      : null,
                                ),
                                validator: (value) {
                                  final v = (value ?? '').trim();
                                  if (v.isEmpty) {
                                    return f.required
                                        ? 'Enter ${f.label.toLowerCase()}.'
                                        : null;
                                  }
                                  if (f.kind == 'number') {
                                    final n = int.tryParse(v);
                                    if (n == null ||
                                        n < f.min ||
                                        f.max != null && n > f.max!) {
                                      return 'Enter a whole number in the allowed range.';
                                    }
                                  }
                                  return null;
                                },
                              ),
                      ),
                  ],
                ),
              ),
              const SizedBox(height: 8),
              SettingsAction(
                'Save details',
                icon: Icons.check_rounded,
                onPressed: submit,
              ),
            ],
          ),
        ),
      ),
    ),
  );
}

class ProductionRecordEditor extends StatefulWidget {
  const ProductionRecordEditor({
    super.key,
    required this.title,
    required this.initial,
  });
  final String title;
  final JsonObject initial;
  @override
  State<ProductionRecordEditor> createState() => _ProductionRecordEditorState();
}

class _ProductionRecordEditorState extends State<ProductionRecordEditor> {
  late final entries = [
    for (final e in widget.initial.entries)
      (
        key: TextEditingController(text: e.key),
        value: TextEditingController(
          text: e.value is String ? e.value : jsonEncode(e.value),
        ),
      ),
  ];
  String? error;
  @override
  void initState() {
    super.initState();
    if (entries.isEmpty) add();
  }

  void add() {
    entries.add((key: TextEditingController(), value: TextEditingController()));
  }

  @override
  void dispose() {
    for (final e in entries) {
      e.key.dispose();
      e.value.dispose();
    }
    super.dispose();
  }

  void submit() {
    final body = <String, dynamic>{};
    for (final e in entries) {
      final key = e.key.text.trim(), value = e.value.text.trim();
      if (key.isEmpty || value.isEmpty || body.containsKey(key)) {
        setState(() => error = 'Each detail needs a unique label and a value.');
        return;
      }
      body[key] =
          widget.initial.containsKey(key) &&
              value ==
                  (widget.initial[key] is String
                      ? widget.initial[key]
                      : jsonEncode(widget.initial[key]))
          ? widget.initial[key]
          : value;
    }
    Navigator.pop(context, body);
  }

  @override
  Widget build(BuildContext context) => Column(
    mainAxisSize: MainAxisSize.min,
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      SettingsHeading(widget.title),
      if (error != null) SettingsNote(error!, warning: true),
      for (final e in entries)
        Padding(
          padding: const EdgeInsets.only(bottom: 16),
          child: Column(
            children: [
              TextField(
                controller: e.key,
                decoration: const InputDecoration(labelText: 'Detail label'),
              ),
              const SizedBox(height: 10),
              TextField(
                controller: e.value,
                decoration: const InputDecoration(labelText: 'Recorded value'),
              ),
              Align(
                alignment: Alignment.centerRight,
                child: IconButton(
                  tooltip: 'Remove detail',
                  onPressed: entries.length == 1
                      ? null
                      : () => setState(() {
                          entries.remove(e);
                          e.key.dispose();
                          e.value.dispose();
                        }),
                  icon: const Icon(Icons.remove_circle_outline_rounded),
                ),
              ),
            ],
          ),
        ),
      TextButton.icon(
        onPressed: () => setState(add),
        icon: const Icon(Icons.add_rounded),
        label: const Text('Add another detail'),
      ),
      const SizedBox(height: 16),
      SettingsAction('Use these details', onPressed: submit),
    ],
  );
}
