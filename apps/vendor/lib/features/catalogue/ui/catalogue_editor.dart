import 'dart:convert';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../state/catalogue_resource.dart';
import 'catalogue_fields.dart';

class CatalogueEditor extends StatefulWidget {
  const CatalogueEditor({
    super.key,
    required this.runtime,
    required this.resource,
    required this.currency,
    required this.onSave,
    this.record,
    this.choices = const {},
    this.sheet = false,
    this.errorMessage,
    this.canSave,
    this.fieldsOverride,
    this.entityLabel,
    this.helper,
    this.presets = const {},
    this.partialUpdate = true,
    this.transform,
    this.validate,
  });
  final List<CatalogueField>? fieldsOverride;
  final String? entityLabel, helper;
  final JsonObject presets;
  final bool partialUpdate;
  final JsonObject Function(JsonObject)? transform;
  final String? Function(JsonObject)? validate;
  final VendorRuntime runtime;
  final CatalogueResource resource;
  final String currency;
  final JsonObject? record;
  final Map<String, Map<String, String>> choices;
  final bool sheet;
  final Future<bool> Function(JsonObject) onSave;
  final String? Function()? errorMessage;
  final bool Function()? canSave;
  @override
  State<CatalogueEditor> createState() => _CatalogueEditorState();
}

class _CatalogueEditorState extends State<CatalogueEditor> {
  final _form = GlobalKey<FormState>();
  late final fields =
      widget.fieldsOverride ??
      catalogueFields(widget.resource.kind, creating: widget.record == null);
  final Map<String, TextEditingController> _text = {};
  final JsonObject _values = {}, _initial = {};
  bool _saving = false;
  String? _error;
  @override
  void initState() {
    super.initState();
    for (final f in fields) {
      var value = widget.record?[f.source ?? f.key];
      if (widget.record == null) {
        value = switch (f.key) {
          'pack_quantity' || 'minimum_order_qty' || 'order_multiple' => 1,
          'package_level' => 'EACH',
          'temperature_zone' => 'AMBIENT',
          'stackable' ||
          'retail_enabled' ||
          'auto_hide_unavailable' ||
          'logistics_available' => true,
          'wholesale_enabled' || 'large_load' => false,
          'min_select' => 0,
          'max_select' => 1,
          _ => null,
        };
        if (widget.runtime.mode == VendorDataMode.mock) {
          value ??= switch (f.key) {
            'name' => switch (widget.resource.kind) {
              CatalogueKind.menus => 'Fresh from our kitchen',
              CatalogueKind.sections => 'Customer favourites',
              CatalogueKind.items => 'Smoky jollof rice',
              CatalogueKind.products => 'Premium long-grain rice',
              CatalogueKind.groups => 'Choose your side',
              CatalogueKind.modifiers => 'Plantain',
              CatalogueKind.categories => 'Pantry essentials',
              _ => 'Regular',
            },
            'description' => 'Carefully selected for everyday enjoyment.',
            'slug' => 'pantry-essentials',
            'base_price_minor' => 350000,
            'price_delta_minor' => 50000,
            'sku' => 'RICE-REG-01',
            'product_class' => 'DRY_BULK',
            _ => null,
          };
          final options = widget.choices[f.key];
          if (value == null &&
              f.required &&
              options != null &&
              options.isNotEmpty) {
            value = options.keys.first;
          }
        }
      }
      value = widget.presets.containsKey(f.key) && widget.record == null
          ? widget.presets[f.key]
          : value;
      if ([
        CatalogueInput.custom,
        CatalogueInput.choice,
        CatalogueInput.multi,
        CatalogueInput.toggle,
      ].contains(f.type)) {
        _values[f.key] =
            value ??
            (f.type == CatalogueInput.multi
                ? <String>[]
                : f.type == CatalogueInput.toggle
                ? false
                : null);
      } else {
        final text = f.type == CatalogueInput.money && value is int
            ? '${value < 0 ? '-' : ''}${value.abs() ~/ 100}.${(value.abs() % 100).toString().padLeft(2, '0')}'
            : value is List
            ? value.map((v) => v.toString().replaceAll('_', ' ')).join(', ')
            : value?.toString() ?? '';
        _text[f.key] = TextEditingController(text: text);
      }
    }
    _initial.addAll(_body());
  }

  @override
  void dispose() {
    for (final t in _text.values) {
      t.dispose();
    }
    super.dispose();
  }

  JsonObject _body() {
    final out = <String, dynamic>{};
    for (final f in fields) {
      final text = _text[f.key]?.text.trim();
      final value = switch (f.type) {
        CatalogueInput.money =>
          text == null || text.isEmpty ? null : catalogueMinor(text),
        CatalogueInput.integer => int.tryParse(text ?? ''),
        CatalogueInput.codes =>
          (text ?? '')
              .split(',')
              .map(
                (s) => s
                    .trim()
                    .toUpperCase()
                    .replaceAll(RegExp(r'[^A-Z0-9]+'), '_')
                    .replaceAll(RegExp(r'^_+|_+$'), ''),
              )
              .where((s) => s.isNotEmpty)
              .toSet()
              .toList(),
        CatalogueInput.custom ||
        CatalogueInput.choice ||
        CatalogueInput.multi ||
        CatalogueInput.toggle => _values[f.key],
        _ => text ?? '',
      };
      if (value != null) out[f.key] = value;
      if (value == null &&
          widget.record != null &&
          ['parent_id', 'consolidation_class'].contains(f.key) &&
          widget.record?[f.source ?? f.key] != null) {
        out[f.key] = '';
      }
      if (value == null &&
          widget.record != null &&
          [
            'retail_price_minor',
            'wholesale_price_minor',
            'retail_override',
            'wholesale_override',
            'maximum_order_qty',
          ].contains(f.key) &&
          widget.record?[f.source ?? f.key] != null) {
        out[f.key] = null;
      }
      if (f.key == 'price_override_minor' &&
          value == null &&
          widget.record?['price_override_minor'] != null) {
        out['clear_price_override'] = true;
      }
    }
    return widget.transform?.call(out) ?? out;
  }

  String? _validate(CatalogueField f) {
    final text = _text[f.key]?.text.trim() ?? '';
    if (f.type == CatalogueInput.custom) {
      return f.required &&
              (_values[f.key] == null ||
                  _values[f.key] is List && (_values[f.key] as List).isEmpty)
          ? 'Choose ${f.label.toLowerCase()}.'
          : null;
    }
    if (f.type == CatalogueInput.choice) {
      final value = _values[f.key];
      if (f.required && value == null) return 'Choose ${f.label.toLowerCase()}';
      if (value != null &&
          !(widget.choices[f.key]?.containsKey(value) ??
              f.options.contains(value))) {
        return 'Choose an available option';
      }
      return null;
    }
    if (f.type == CatalogueInput.multi || f.type == CatalogueInput.toggle) {
      return null;
    }
    if (f.required && text.isEmpty) return 'Enter ${f.label.toLowerCase()}';
    if (text.isEmpty &&
        f.type == CatalogueInput.integer &&
        widget.record?[f.source ?? f.key] != null &&
        f.key != 'maximum_order_qty' &&
        !f.allowClear) {
      return 'Enter a replacement value';
    }
    if (text.isEmpty) return null;
    if (f.maxLength != null && text.length > f.maxLength!) {
      return 'Use at most ${f.maxLength} characters';
    }
    if (f.key == 'name' &&
        text.length <
            (widget.resource.kind == CatalogueKind.shopVariants ? 1 : 2)) {
      return 'Enter at least two characters';
    }
    if (f.type == CatalogueInput.money || f.type == CatalogueInput.integer) {
      final number = f.type == CatalogueInput.money
          ? catalogueMinor(text)
          : int.tryParse(text);
      if (number == null ||
          number < f.min ||
          number.abs() >
              (f.type == CatalogueInput.money ? 99999999999 : 999999999)) {
        return f.type == CatalogueInput.money
            ? 'Enter a valid amount with up to two decimal places'
            : 'Enter a whole number of at least ${f.min}';
      }
    }
    return null;
  }

  bool get _ready =>
      !_saving &&
      (widget.canSave?.call() ?? true) &&
      fields.every((f) => _validate(f) == null) &&
      (widget.record == null || jsonEncode(_body()) != jsonEncode(_initial));
  Future<void> _save() async {
    if (!_ready || !_form.currentState!.validate()) return;
    final body = _body();
    final validation = widget.validate?.call(body);
    if (validation != null) {
      setState(() => _error = validation);
      return;
    }
    if (widget.record != null && widget.partialUpdate) {
      body.removeWhere((k, v) => jsonEncode(v) == jsonEncode(_initial[k]));
    }
    if (widget.resource.kind == CatalogueKind.groups) {
      final min = body['min_select'] ?? widget.record?['min_select'] ?? 0;
      final max = body['max_select'] ?? widget.record?['max_select'] ?? 1;
      if (min > max) {
        setState(
          () => _error = 'Minimum selections cannot exceed the maximum.',
        );
        return;
      }
    }
    if (widget.resource.kind == CatalogueKind.listings) {
      if ((body['retail_enabled'] ?? widget.record?['retail_enabled']) !=
              true &&
          (body['wholesale_enabled'] ?? widget.record?['wholesale_enabled']) !=
              true) {
        setState(() => _error = 'Enable retail, wholesale, or both.');
        return;
      }
    }
    setState(() {
      _saving = true;
      _error = null;
    });
    final saved = await widget.onSave(body);
    if (!mounted) return;
    if (saved) {
      Navigator.pop(context, true);
    } else {
      setState(() {
        _saving = false;
        _error =
            widget.errorMessage?.call() ?? 'Could not save. Please try again.';
      });
    }
  }

  Future<void> _close() async {
    if (_saving) return;
    if (jsonEncode(_body()) != jsonEncode(_initial)) {
      final discard = await showDialog<bool>(
        context: context,
        builder: (context) => AlertDialog(
          title: const Text('Discard changes?'),
          content: const Text('Your unsaved edits will be lost.'),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('Keep editing'),
            ),
            TextButton(
              onPressed: () => Navigator.pop(context, true),
              child: Text(
                'Discard',
                style: TextStyle(color: KarryamColors.error.forInk(context)),
              ),
            ),
          ],
        ),
      );
      if (discard != true || !mounted) return;
    }
    if (mounted) Navigator.pop(context);
  }

  @override
  Widget build(BuildContext context) {
    final title =
        '${widget.record == null
            ? widget.entityLabel == null
                  ? 'Create'
                  : 'Add'
            : 'Edit'} ${widget.entityLabel ?? widget.resource.singular}';
    final content = Form(
      key: _form,
      child: ListView(
        shrinkWrap: widget.sheet,
        padding: const EdgeInsets.fromLTRB(22, 12, 22, 28),
        children: [
          if (widget.sheet)
            Row(
              children: [
                Expanded(
                  child: Text(
                    title,
                    style: const TextStyle(
                      fontSize: 26,
                      fontWeight: FontWeight.w700,
                      letterSpacing: -.8,
                    ),
                  ),
                ),
                IconButton(
                  onPressed: _saving ? null : _close,
                  icon: const Icon(Icons.close_rounded),
                ),
              ],
            ),
          if (!widget.sheet) ...[
            Text(
              widget.entityLabel != null
                  ? title
                  : widget.record == null
                  ? 'Make it yours.'
                  : 'Every detail matters.',
              style: const TextStyle(
                fontSize: 30,
                fontWeight: FontWeight.w700,
                letterSpacing: -1,
              ),
            ),
            const SizedBox(height: 8),
            Text(
              widget.helper ??
                  (widget.resource.kind == CatalogueKind.products
                      ? 'Create the product first, then add its variants and branch listings.'
                      : widget.resource.kind == CatalogueKind.items
                      ? 'Keep names, prices and food information clear for your customers.'
                      : 'Your changes are saved to your catalogue.'),
              style: TextStyle(color: KarryamColors.indigo.forInk(context)),
            ),
            const SizedBox(height: 12),
          ],
          for (final section in fields.map((f) => f.section).toSet()) ...[
            Padding(
              padding: const EdgeInsets.only(top: 18, bottom: 14),
              child: Row(
                children: [
                  Container(
                    width: 4,
                    height: 17,
                    decoration: BoxDecoration(
                      color: KarryamColors.teal.forSurface(context),
                      borderRadius: BorderRadius.circular(8),
                    ),
                  ),
                  const SizedBox(width: 10),
                  Text(
                    section,
                    style: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ],
              ),
            ),
            Material(
              color: Colors.white.forSurface(context),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(24),
                side: BorderSide(color: Color(0xFFE3EAE8).forOutline(context)),
              ),
              child: Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                  children: [
                    for (final f in fields.where((f) => f.section == section))
                      Padding(
                        padding: const EdgeInsets.only(bottom: 14),
                        child: _field(f),
                      ),
                  ],
                ),
              ),
            ),
          ],
          if (_error != null)
            Container(
              margin: const EdgeInsets.only(top: 18),
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: Color(0xFFFFEDE9).forSurface(context),
                borderRadius: BorderRadius.circular(18),
              ),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Icon(
                    Icons.error_outline_rounded,
                    color: KarryamColors.error.forInk(context),
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Text(
                      _error!,
                      style: TextStyle(
                        color: KarryamColors.error.forInk(context),
                      ),
                    ),
                  ),
                ],
              ),
            ),
          if (widget.sheet)
            Padding(padding: const EdgeInsets.only(top: 20), child: _button()),
        ],
      ),
    );
    return PopScope(
      canPop: false,
      onPopInvokedWithResult: (didPop, result) {
        if (!didPop) _close();
      },
      child: widget.sheet
          ? Padding(
              padding: EdgeInsets.only(
                bottom: MediaQuery.viewInsetsOf(context).bottom,
              ),
              child: ConstrainedBox(
                constraints: BoxConstraints(
                  maxHeight:
                      MediaQuery.sizeOf(context).height * .86 -
                      MediaQuery.viewInsetsOf(context).bottom,
                ),
                child: content,
              ),
            )
          : Scaffold(
              backgroundColor: KarryamColors.canvas.forSurface(context),
              appBar: AppBar(
                title: Text(title),
                leading: IconButton(
                  onPressed: _saving ? null : _close,
                  icon: const Icon(Icons.arrow_back_rounded),
                ),
              ),
              body: content,
              bottomNavigationBar: SafeArea(
                top: false,
                child: Padding(
                  padding: const EdgeInsets.fromLTRB(22, 12, 22, 16),
                  child: _button(),
                ),
              ),
            ),
    );
  }

  Widget _button() => FilledButton(
    onPressed: _ready ? _save : null,
    child: _saving
        ? SizedBox(
            width: 22,
            height: 22,
            child: CircularProgressIndicator(
              strokeWidth: 2,
              color: KarryamColors.navy.forInk(context),
            ),
          )
        : Text(
            widget.record == null
                ? widget.entityLabel == null
                      ? 'Create ${widget.resource.singular}'
                      : 'Add ${widget.entityLabel}'
                : 'Save changes',
          ),
  );
  Widget _field(CatalogueField f) {
    if (f.type == CatalogueInput.custom) {
      return f.builder!(
        context,
        _values[f.key],
        _saving || f.readOnly
            ? null
            : (value) => setState(() => _values[f.key] = value),
      );
    }
    if (f.type == CatalogueInput.toggle) {
      return SwitchListTile.adaptive(
        contentPadding: EdgeInsets.zero,
        title: Text(
          f.label,
          style: const TextStyle(fontSize: 14, fontWeight: FontWeight.w600),
        ),
        value: _values[f.key] == true,
        onChanged: _saving ? null : (v) => setState(() => _values[f.key] = v),
        activeThumbColor: KarryamColors.tealInk.forInk(context),
        activeTrackColor: KarryamColors.mint.forInk(context),
      );
    }
    if (f.type == CatalogueInput.multi) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(f.label, style: const TextStyle(fontWeight: FontWeight.w600)),
          const SizedBox(height: 10),
          Wrap(
            spacing: 8,
            runSpacing: 6,
            children: [
              for (final option in f.options)
                FilterChip(
                  label: Text(dashboardLabel(option)),
                  selected: (_values[f.key] as List).contains(option),
                  selectedColor: KarryamColors.mint.forSurface(context),
                  checkmarkColor: KarryamColors.tealInk.forInk(context),
                  onSelected: _saving
                      ? null
                      : (selected) => setState(() {
                          final values = List<String>.from(_values[f.key]);
                          selected ? values.add(option) : values.remove(option);
                          _values[f.key] = values;
                        }),
                ),
            ],
          ),
        ],
      );
    }
    if (f.type == CatalogueInput.choice) {
      final options =
          widget.choices[f.key] ??
          {for (final option in f.options) option: dashboardLabel(option)};
      return DropdownButtonFormField<String>(
        initialValue: options.containsKey(_values[f.key])
            ? _values[f.key]
            : null,
        isExpanded: true,
        decoration: InputDecoration(labelText: f.label),
        items: [
          if (!f.required)
            const DropdownMenuItem(value: '', child: Text('None')),
          for (final e in options.entries)
            DropdownMenuItem(
              value: e.key,
              child: Text(
                e.value,
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
              ),
            ),
        ],
        validator: (_) => _validate(f),
        onChanged: _saving || f.readOnly
            ? null
            : (v) => setState(() => _values[f.key] = v == '' ? null : v),
      );
    }
    return TextFormField(
      controller: _text[f.key],
      enabled: !_saving && !f.readOnly,
      minLines: f.type == CatalogueInput.paragraph ? 3 : 1,
      maxLines: f.type == CatalogueInput.paragraph ? 5 : 1,
      keyboardType: f.type == CatalogueInput.money
          ? const TextInputType.numberWithOptions(decimal: true, signed: true)
          : f.type == CatalogueInput.integer
          ? TextInputType.number
          : f.type == CatalogueInput.paragraph
          ? TextInputType.multiline
          : TextInputType.text,
      textInputAction: f.type == CatalogueInput.paragraph
          ? TextInputAction.newline
          : TextInputAction.next,
      decoration: InputDecoration(
        labelText: f.label,
        prefixText: f.type == CatalogueInput.money
            ? '${widget.currency} '
            : null,
        helperText: f.type == CatalogueInput.codes
            ? 'Separate each one with a comma'
            : f.type == CatalogueInput.money && !f.required
            ? f.key == 'price_override_minor' || f.key.endsWith('_override')
                  ? 'Leave blank to use the inherited price'
                  : 'Leave blank if no price is set'
            : null,
      ),
      validator: (_) => _validate(f),
      onChanged: (_) => setState(() {}),
      autovalidateMode: AutovalidateMode.onUserInteraction,
    );
  }
}
