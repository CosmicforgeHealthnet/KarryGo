import 'package:flutter/material.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../settings/ui/settings_design.dart';

class PurchaseField {
  const PurchaseField(
    this.key,
    this.label, {
    this.kind = 'text',
    this.required = false,
    this.options = const {},
    this.initial,
    this.min = 0,
    this.max,
  });
  final String key, label, kind;
  final bool required;
  final Map<String, String> options;
  final Object? initial;
  final int min;
  final int? max;
}

int? purchaseDecimal(String text) {
  if (!RegExp(r'^\d+(?:\.\d{1,2})?$').hasMatch(text.trim())) return null;
  final parts = text.trim().split('.');
  return int.parse(parts[0]) * 100 +
      int.parse(parts.length == 1 ? '0' : parts[1].padRight(2, '0'));
}

class PurchaseForm extends StatefulWidget {
  const PurchaseForm({
    super.key,
    required this.title,
    required this.fields,
    this.validate,
    this.submitLabel = 'Review details',
  });
  final String title, submitLabel;
  final List<PurchaseField> fields;
  final String? Function(JsonObject)? validate;
  @override
  State<PurchaseForm> createState() => _PurchaseFormState();
}

class _PurchaseFormState extends State<PurchaseForm> {
  final form = GlobalKey<FormState>();
  late final text = {
    for (final f in widget.fields.where(
      (f) => !['bool', 'select'].contains(f.kind),
    ))
      f.key: TextEditingController(
        text: f.initial == null
            ? ''
            : ['money', 'percent'].contains(f.kind)
            ? ((f.initial as num) / 100).toStringAsFixed(2)
            : '${f.initial}',
      ),
  };
  late final values = <String, dynamic>{
    for (final f in widget.fields.where(
      (f) => ['bool', 'select'].contains(f.kind),
    ))
      f.key: f.kind == 'bool'
          ? f.initial == true
          : f.options.containsKey(f.initial)
          ? f.initial
          : null,
  };
  String? error;
  @override
  void dispose() {
    for (final c in text.values) {
      c.dispose();
    }
    super.dispose();
  }

  void submit() {
    if (!form.currentState!.validate()) return;
    final body = <String, dynamic>{};
    for (final f in widget.fields) {
      if (['bool', 'select'].contains(f.kind)) {
        if (values[f.key] != null) {
          body[f.key] = values[f.key];
        } else {
          body[f.key] = null;
        }
      } else {
        final s = text[f.key]!.text.trim();
        if (s.isEmpty &&
            ['number', 'money', 'percent', 'date'].contains(f.kind)) {
          body[f.key] = null;
        } else {
          body[f.key] = switch (f.kind) {
            'number' => int.parse(s),
            'money' || 'percent' => purchaseDecimal(s),
            'date' => DateTime.parse(s).toUtc().toIso8601String(),
            _ => s,
          };
        }
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
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    mainAxisSize: MainAxisSize.min,
    children: [
      SettingsHeading(widget.title),
      if (error != null) ...[
        Text(
          error!,
          style: TextStyle(color: Theme.of(context).colorScheme.error),
        ),
        const SizedBox(height: 16),
      ],
      Form(
        key: form,
        child: Column(
          children: [
            for (final f in widget.fields)
              Padding(
                padding: const EdgeInsets.only(bottom: 18),
                child: f.kind == 'bool'
                    ? SwitchListTile.adaptive(
                        contentPadding: EdgeInsets.zero,
                        title: Text(f.label),
                        value: values[f.key] == true,
                        onChanged: (v) => setState(() => values[f.key] = v),
                      )
                    : f.kind == 'select'
                    ? DropdownButtonFormField<String>(
                        initialValue: values[f.key],
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
                        onChanged: (v) => values[f.key] = v == '' ? null : v,
                        validator: (v) => f.required && (v == null || v.isEmpty)
                            ? 'Choose ${f.label.toLowerCase()}.'
                            : null,
                      )
                    : TextFormField(
                        controller: text[f.key],
                        keyboardType:
                            ['number', 'money', 'percent'].contains(f.kind)
                            ? const TextInputType.numberWithOptions(
                                decimal: true,
                              )
                            : f.kind == 'date'
                            ? TextInputType.datetime
                            : TextInputType.text,
                        minLines: f.kind == 'notes' ? 2 : 1,
                        maxLines: f.kind == 'notes' ? 4 : 1,
                        decoration: InputDecoration(
                          labelText: f.label,
                          hintText: f.kind == 'date'
                              ? 'YYYY-MM-DD or date and time with timezone'
                              : null,
                        ),
                        validator: (v) {
                          final s = (v ?? '').trim();
                          if (s.isEmpty) {
                            return f.required
                                ? 'Enter ${f.label.toLowerCase()}.'
                                : null;
                          }
                          if (f.kind == 'date' &&
                              DateTime.tryParse(s) == null) {
                            return 'Enter a valid date.';
                          }
                          if (['number', 'money', 'percent'].contains(f.kind)) {
                            final n = f.kind == 'number'
                                ? int.tryParse(s)
                                : purchaseDecimal(s);
                            if (n == null ||
                                n < f.min ||
                                f.max != null && n > f.max!) {
                              return 'Enter a valid ${f.kind == 'number' ? 'whole number' : 'amount'}.';
                            }
                          }
                          return null;
                        },
                      ),
              ),
          ],
        ),
      ),
      SettingsAction(widget.submitLabel, onPressed: submit),
    ],
  );
}

Future<JsonObject?> purchaseForm(
  BuildContext context,
  String title,
  List<PurchaseField> fields, {
  String? Function(JsonObject)? validate,
}) => settingsSheet<JsonObject>(
  context,
  PurchaseForm(title: title, fields: fields, validate: validate),
);
Map<String, String> purchaseChoices(
  List<JsonObject> rows, {
  String key = 'id',
  String label = 'name',
}) => {
  for (final r in rows)
    '${r[key]}':
        '${r[label] ?? r['display_name'] ?? r['supplier_sku'] ?? r[key]}',
};
Map<String, String> purchaseEnums(List<String> values) => {
  for (final v in values) v: dashboardLabel(v),
};
