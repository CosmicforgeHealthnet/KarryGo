import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../dashboard/ui/dashboard_format.dart';

class OrderInput {
  const OrderInput(
    this.key,
    this.label, {
    this.required = false,
    this.number = false,
    this.max,
    this.initialText,
  });
  final String key, label;
  final bool required, number;
  final int? max;
  final String? initialText;
}

class OrderFormSheet extends StatefulWidget {
  const OrderFormSheet({
    super.key,
    required this.title,
    required this.message,
    required this.fields,
    this.choices = const {},
    this.destructive = false,
    this.icon = Icons.fact_check_outlined,
  });
  final String title, message;
  final List<OrderInput> fields;
  final Map<String, List<String>> choices;
  final bool destructive;
  final IconData icon;
  @override
  State<OrderFormSheet> createState() => _OrderFormSheetState();
}

class _OrderFormSheetState extends State<OrderFormSheet> {
  final _form = GlobalKey<FormState>();
  final JsonObject _values = {};
  late final _controllers = {
    for (final field in widget.fields)
      field.key: TextEditingController(
        text: field.initialText ?? (field.number ? '0' : ''),
      ),
  };
  @override
  void dispose() {
    for (final controller in _controllers.values) {
      controller.dispose();
    }
    super.dispose();
  }

  String? _validate(OrderInput field, String? value) {
    if (field.required && (value == null || value.trim().isEmpty)) {
      return 'Enter ${field.label.toLowerCase()}';
    }
    if (field.number) {
      final number = int.tryParse(value ?? '');
      if (number == null ||
          number < 0 ||
          (field.max != null && number > field.max!)) {
        return field.max == null
            ? 'Enter a whole number'
            : 'Enter a number from 0 to ${field.max}';
      }
    }
    return null;
  }

  bool get _ready =>
      widget.fields.every(
        (f) => _validate(f, _controllers[f.key]!.text) == null,
      ) &&
      widget.choices.keys.every((key) => _values[key] != null);
  void _count(OrderInput field, int delta) {
    final controller = _controllers[field.key]!;
    final value = ((int.tryParse(controller.text) ?? 0) + delta).clamp(
      0,
      field.max ?? 999999999,
    );
    setState(() => controller.text = '$value');
  }

  @override
  Widget build(BuildContext context) {
    final color = widget.destructive
        ? KarryamColors.error
        : KarryamColors.tealInk;
    return AnimatedPadding(
      duration: MediaQuery.disableAnimationsOf(context)
          ? Duration.zero
          : const Duration(milliseconds: 180),
      curve: Curves.easeOut,
      padding: EdgeInsets.only(bottom: MediaQuery.viewInsetsOf(context).bottom),
      child: ConstrainedBox(
        constraints: BoxConstraints(
          maxHeight:
              MediaQuery.sizeOf(context).height * .86 -
              MediaQuery.viewInsetsOf(context).bottom,
        ),
        child: SingleChildScrollView(
          padding: const EdgeInsets.fromLTRB(24, 0, 24, 24),
          child: Form(
            key: _form,
            autovalidateMode: AutovalidateMode.onUserInteraction,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Container(
                      width: 58,
                      height: 58,
                      decoration: BoxDecoration(
                        color: (color.withValues(
                          alpha: .09,
                        )).forSurface(context),
                        borderRadius: BorderRadius.circular(20),
                        border: Border.all(
                          color: (color.withValues(
                            alpha: .12,
                          )).forOutline(context),
                        ),
                      ),
                      child: Icon(
                        widget.icon,
                        color: (color).forInk(context),
                        size: 29,
                      ),
                    ),
                    const Spacer(),
                    IconButton(
                      tooltip: 'Close',
                      onPressed: () => Navigator.pop(context),
                      icon: const Icon(Icons.close_rounded, size: 21),
                    ),
                  ],
                ),
                const SizedBox(height: 18),
                Text(
                  widget.title,
                  style: const TextStyle(
                    fontSize: 27,
                    fontWeight: FontWeight.w800,
                    letterSpacing: -.7,
                  ),
                ),
                const SizedBox(height: 10),
                Text(widget.message, style: const TextStyle(height: 1.6)),
                const SizedBox(height: 24),
                for (final entry in widget.choices.entries) ...[
                  Text(
                    dashboardLabel(entry.key),
                    style: const TextStyle(fontWeight: FontWeight.w600),
                  ),
                  const SizedBox(height: 10),
                  for (final value in entry.value)
                    Padding(
                      padding: const EdgeInsets.only(bottom: 8),
                      child: Material(
                        color:
                            (_values[entry.key] == value
                                    ? KarryamColors.mint
                                    : const Color(0xFFF6F8FA))
                                .forSurface(context),
                        borderRadius: BorderRadius.circular(16),
                        child: InkWell(
                          borderRadius: BorderRadius.circular(16),
                          onTap: () =>
                              setState(() => _values[entry.key] = value),
                          child: Padding(
                            padding: const EdgeInsets.all(14),
                            child: Row(
                              children: [
                                Icon(
                                  value == 'MET'
                                      ? Icons.task_alt_rounded
                                      : value == 'NOT_AVAILABLE'
                                      ? Icons.block_rounded
                                      : value == 'PARTIAL'
                                      ? Icons.adjust_rounded
                                      : Icons.swap_horiz_rounded,
                                  color:
                                      (value == 'MET'
                                              ? KarryamColors.tealInk
                                              : value == 'NOT_AVAILABLE'
                                              ? KarryamColors.error
                                              : const Color(0xFF986013))
                                          .forInk(context),
                                  size: 23,
                                ),
                                const SizedBox(width: 12),
                                Expanded(
                                  child: Text(
                                    dashboardLabel(value),
                                    style: const TextStyle(
                                      fontWeight: FontWeight.w600,
                                    ),
                                  ),
                                ),
                                Icon(
                                  _values[entry.key] == value
                                      ? Icons.radio_button_checked_rounded
                                      : Icons.radio_button_off_rounded,
                                  color:
                                      (_values[entry.key] == value
                                              ? KarryamColors.tealInk
                                              : KarryamColors.gray)
                                          .forInk(context),
                                  size: 20,
                                ),
                              ],
                            ),
                          ),
                        ),
                      ),
                    ),
                  const SizedBox(height: 12),
                ],
                for (final field in widget.fields)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 18),
                    child: field.number
                        ? Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(
                                field.label,
                                style: const TextStyle(
                                  fontWeight: FontWeight.w600,
                                ),
                              ),
                              const SizedBox(height: 10),
                              Row(
                                children: [
                                  IconButton.filledTonal(
                                    tooltip:
                                        'Decrease ${field.label.toLowerCase()}',
                                    onPressed:
                                        (int.tryParse(
                                                  _controllers[field.key]!.text,
                                                ) ??
                                                0) >
                                            0
                                        ? () => _count(field, -1)
                                        : null,
                                    icon: const Icon(Icons.remove_rounded),
                                  ),
                                  const SizedBox(width: 12),
                                  Expanded(child: _input(field)),
                                  const SizedBox(width: 12),
                                  IconButton.filledTonal(
                                    tooltip:
                                        'Increase ${field.label.toLowerCase()}',
                                    onPressed:
                                        field.max == null ||
                                            (int.tryParse(
                                                      _controllers[field.key]!
                                                          .text,
                                                    ) ??
                                                    0) <
                                                field.max!
                                        ? () => _count(field, 1)
                                        : null,
                                    icon: const Icon(Icons.add_rounded),
                                  ),
                                ],
                              ),
                            ],
                          )
                        : _input(field),
                  ),
                const SizedBox(height: 4),
                SizedBox(
                  width: double.infinity,
                  child: FilledButton(
                    style: FilledButton.styleFrom(
                      backgroundColor:
                          (widget.destructive
                                  ? KarryamColors.error
                                  : KarryamColors.teal)
                              .forSurface(context),
                      foregroundColor: widget.destructive
                          ? Colors.white
                          : KarryamColors.navy,
                      shape: RoundedRectangleBorder(
                        borderRadius: BorderRadius.circular(16),
                      ),
                      padding: const EdgeInsets.symmetric(vertical: 18),
                    ),
                    onPressed: !_ready
                        ? null
                        : () {
                            if (!_form.currentState!.validate()) return;
                            _form.currentState!.save();
                            Navigator.pop(context, _values);
                          },
                    child: Text(
                      widget.title,
                      style: const TextStyle(fontWeight: FontWeight.w700),
                    ),
                  ),
                ),
                Center(
                  child: TextButton(
                    onPressed: () => Navigator.pop(context),
                    child: const Text('Go back'),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _input(OrderInput field) => TextFormField(
    controller: _controllers[field.key],
    decoration: InputDecoration(labelText: field.number ? null : field.label),
    textAlign: field.number ? TextAlign.center : TextAlign.start,
    keyboardType: field.number ? TextInputType.number : TextInputType.text,
    inputFormatters: field.number
        ? [FilteringTextInputFormatter.digitsOnly]
        : null,
    onChanged: (_) => setState(() {}),
    validator: (value) => _validate(field, value),
    onSaved: (value) {
      if (value?.trim().isNotEmpty == true) {
        _values[field.key] = field.number ? int.parse(value!) : value!.trim();
      }
    },
  );
}
