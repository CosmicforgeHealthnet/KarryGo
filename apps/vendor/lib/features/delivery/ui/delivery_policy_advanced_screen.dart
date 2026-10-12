import 'dart:convert';
import 'package:flutter/material.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../settings/ui/settings_design.dart';
import '../state/delivery_policy_rules.dart';

class DeliveryPolicyAdvancedScreen extends StatefulWidget {
  const DeliveryPolicyAdvancedScreen({
    super.key,
    required this.values,
    required this.currency,
    required this.hybrid,
    this.mock = false,
    this.readOnly = false,
  });
  final JsonObject values;
  final String currency;
  final bool hybrid, mock, readOnly;
  @override
  State<DeliveryPolicyAdvancedScreen> createState() =>
      _DeliveryPolicyAdvancedScreenState();
}

class _DeliveryPolicyAdvancedScreenState
    extends State<DeliveryPolicyAdvancedScreen> {
  late final values = jsonObject(jsonDecode(jsonEncode(widget.values)));
  late final timezone = TextEditingController(
    text: values['operating_timezone']?.toString() ?? 'UTC',
  );
  List<JsonObject> rows(String key) =>
      (values[key] as List? ?? []).map(jsonObject).toList();
  List<String> choices(String key) =>
      (values[key] as List? ?? []).cast<String>();
  @override
  void dispose() {
    timezone.dispose();
    super.dispose();
  }

  void toggle(String key, String choice, bool selected) => setState(() {
    final list = [...choices(key)]..remove(choice);
    if (selected) list.add(choice);
    values[key] = list;
  });
  Future<void> rule(String key, [int? index]) async {
    final current = rows(key);
    final result = await showModalBottomSheet<JsonObject>(
      context: context,
      showDragHandle: true,
      isScrollControlled: true,
      useSafeArea: true,
      builder: (_) => _PolicyRuleSheet(
        kind: key,
        currency: widget.currency,
        mock: widget.mock,
        record: index == null ? null : current[index],
        nextMinimum: current.isEmpty
            ? 0
            : current.last['max_distance_meters'] as int? ?? 0,
      ),
    );
    if (result == null || !mounted) return;
    setState(() {
      if (index == null) {
        current.add(result);
      } else {
        current[index] = result;
      }
      if (key == 'distance_tiers') {
        current.sort(
          (a, b) => (a['min_distance_meters'] as int).compareTo(
            b['min_distance_meters'] as int,
          ),
        );
      }
      values[key] = current;
    });
  }

  Future<void> remove(String key, int index) async {
    final confirm = await confirmSettings(
      context,
      'Remove this rule?',
      'The rule will be removed from this draft. Save the policy to apply it.',
    );
    if (confirm && mounted) {
      setState(() {
        values[key] = rows(key)..removeAt(index);
      });
    }
  }

  Widget group(
    String title,
    IconData icon,
    Color accent,
    List<Widget> children,
  ) => Padding(
    padding: const EdgeInsets.only(bottom: 18),
    child: SettingsCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                width: 44,
                height: 44,
                decoration: BoxDecoration(
                  color: accent.forInk(context).withValues(alpha: .1),
                  borderRadius: BorderRadius.circular(15),
                ),
                child: Icon(icon, color: accent.forInk(context), size: 24),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Text(
                  title,
                  style: const TextStyle(
                    fontSize: 18,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 16),
          ...children,
        ],
      ),
    ),
  );
  Widget collection(
    String key,
    String title,
    IconData icon,
    Color accent,
    String empty,
  ) {
    final list = rows(key);
    return group(title, icon, accent, [
      if (list.isEmpty) Text(empty),
      for (var i = 0; i < list.length; i++)
        ListTile(
          contentPadding: EdgeInsets.zero,
          title: Text(
            key == 'service_zones'
                ? (list[i]['name']?.toString().isNotEmpty == true
                          ? list[i]['name']
                          : list[i]['id'])
                      .toString()
                : key == 'distance_tiers'
                ? '${list[i]['min_distance_meters']}–${list[i]['max_distance_meters'] == 0 ? '∞' : list[i]['max_distance_meters']} metres'
                : deliveryWeekdays[(list[i]['weekday'] as int) - 1],
          ),
          subtitle: Text(
            key == 'operating_periods'
                ? '${policyClock(list[i]['start_minute'])}–${policyClock(list[i]['end_minute'])}${list[i]['end_minute'] <= list[i]['start_minute'] ? ' · Overnight' : ''}'
                : '${dashboardMoney(list[i]['fee_minor'], widget.currency)}${key == 'service_zones' ? ' · ${list[i]['enabled'] == true ? 'Enabled' : 'Disabled'}' : ''}',
          ),
          onTap: widget.readOnly ? null : () => rule(key, i),
          trailing: widget.readOnly
              ? null
              : IconButton(
                  tooltip: 'Remove rule',
                  icon: const Icon(Icons.delete_outline_rounded),
                  color: Theme.of(context).colorScheme.error,
                  onPressed: () => remove(key, i),
                ),
        ),
      if (!widget.readOnly)
        TextButton.icon(
          onPressed: () => rule(key),
          icon: const Icon(Icons.add_rounded),
          label: Text(
            key == 'service_zones'
                ? 'Add zone'
                : key == 'distance_tiers'
                ? 'Add distance tier'
                : 'Add delivery window',
          ),
        ),
    ]);
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Advanced delivery settings')),
    body: ListView(
      padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
      children: [
        const SettingsHeading(
          'Reach, hours & fallback',
          subtitle: 'Changes are applied when you save the branch policy.',
        ),
        collection(
          'service_zones',
          'Service zones',
          Icons.location_on_outlined,
          const Color(0xFF346BA6),
          'No zones configured. Zone pricing needs an eligible enabled zone.',
        ),
        collection(
          'distance_tiers',
          'Distance pricing',
          Icons.route_outlined,
          const Color(0xFF8D65B5),
          'No tiers: distance pricing uses your base fee and fee per kilometre.',
        ),
        group(
          'Delivery timezone',
          Icons.public_outlined,
          const Color(0xFF346BA6),
          [
            TextField(
              controller: timezone,
              readOnly: widget.readOnly,
              decoration: const InputDecoration(
                labelText: 'Operating timezone',
                hintText: 'Africa/Lagos',
              ),
            ),
            const SizedBox(height: 10),
            const Text(
              'Use an IANA timezone. The server validates it; delivery windows use its local clock.',
            ),
          ],
        ),
        collection(
          'operating_periods',
          'Delivery windows',
          Icons.schedule_outlined,
          const Color(0xFFB17A27),
          'No windows: delivery hours are unrestricted by this policy.',
        ),
        group(
          'Vehicle classes',
          Icons.local_shipping_outlined,
          const Color(0xFF8D65B5),
          [
            const Text(
              'No selection means this policy imposes no vehicle-class restriction.',
            ),
            const SizedBox(height: 12),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                for (final value in deliveryVehicleClasses)
                  FilterChip(
                    label: Text(dashboardLabel(value)),
                    selected: choices(
                      'supported_vehicle_classes',
                    ).contains(value),
                    onSelected: widget.readOnly
                        ? null
                        : (selected) => toggle(
                            'supported_vehicle_classes',
                            value,
                            selected,
                          ),
                  ),
              ],
            ),
          ],
        ),
        group(
          'Rider availability',
          Icons.delivery_dining_rounded,
          const Color(0xFF346BA6),
          [
            DropdownButtonFormField<String>(
              initialValue: values['rider_availability'] ?? 'UNKNOWN',
              isExpanded: true,
              decoration: const InputDecoration(
                labelText: 'Policy availability',
              ),
              items: [
                for (final value in ['UNKNOWN', 'AVAILABLE', 'UNAVAILABLE'])
                  DropdownMenuItem(
                    value: value,
                    child: Text(dashboardLabel(value)),
                  ),
              ],
              onChanged: widget.readOnly
                  ? null
                  : (value) =>
                        setState(() => values['rider_availability'] = value),
            ),
            const SizedBox(height: 12),
            const Text('Unknown does not count as an available vendor rider.'),
          ],
        ),
        group('Hybrid fallback', Icons.alt_route_rounded, const Color(0xFFB17A27), [
          Text(
            widget.hybrid
                ? 'No selection allows all supported fallback triggers.'
                : 'These rules take effect when delivery mode is Hybrid.',
          ),
          const SizedBox(height: 10),
          for (final entry in deliveryFallbackTriggers.entries)
            CheckboxListTile(
              contentPadding: EdgeInsets.zero,
              controlAffinity: ListTileControlAffinity.leading,
              title: Text(entry.value),
              value: choices('fallback_triggers').contains(entry.key),
              onChanged: widget.readOnly
                  ? null
                  : (selected) => toggle(
                      'fallback_triggers',
                      entry.key,
                      selected == true,
                    ),
            ),
          const Divider(height: 28),
          Text(
            '${dashboardLabel(values['hybrid_fallback_funding_sponsor'] ?? 'VENDOR')}-funded fallback',
            style: const TextStyle(fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 8),
          const Text(
            'Platform spending approval and delivery dispatch remain controlled by the server.',
          ),
        ]),
      ],
    ),
    bottomNavigationBar: widget.readOnly
        ? null
        : SafeArea(
            top: false,
            minimum: const EdgeInsets.fromLTRB(20, 10, 20, 14),
            child: FilledButton(
              onPressed: () async {
                final body = {
                  ...values,
                  'operating_timezone': timezone.text.trim().isEmpty
                      ? 'UTC'
                      : timezone.text.trim(),
                };
                final error = validateAdvancedPolicy(body);
                if (error != null) {
                  await showOrderFeedback(
                    context,
                    title: 'Check delivery rules',
                    message: error,
                    warning: true,
                  );
                  return;
                }
                if (context.mounted) Navigator.pop(context, body);
              },
              child: const Text('Use these settings'),
            ),
          ),
  );
}

class _PolicyRuleSheet extends StatefulWidget {
  const _PolicyRuleSheet({
    required this.kind,
    required this.currency,
    required this.mock,
    this.record,
    this.nextMinimum = 0,
  });
  final String kind, currency;
  final bool mock;
  final JsonObject? record;
  final int nextMinimum;
  @override
  State<_PolicyRuleSheet> createState() => _PolicyRuleSheetState();
}

class _PolicyRuleSheetState extends State<_PolicyRuleSheet> {
  final form = GlobalKey<FormState>();
  final fields = <String, TextEditingController>{};
  late bool enabled = widget.record?['enabled'] ?? true;
  late int weekday = widget.record?['weekday'] ?? 1;
  late int start = widget.record?['start_minute'] ?? 540,
      end = widget.record?['end_minute'] ?? 1080;
  bool get zone => widget.kind == 'service_zones';
  bool get period => widget.kind == 'operating_periods';
  @override
  void initState() {
    super.initState();
    for (final key in [
      'id',
      'name',
      'min_distance_meters',
      'max_distance_meters',
      'fee_minor',
      'priority',
    ]) {
      final raw = widget.record?[key];
      final value = raw != null
          ? key == 'fee_minor'
                ? ((raw as int) / 100).toStringAsFixed(2)
                : '$raw'
          : switch (key) {
              'id' =>
                widget.mock
                    ? 'mock-zone-${DateTime.now().microsecondsSinceEpoch}'
                    : '',
              'name' => widget.mock ? 'Central delivery zone' : '',
              'min_distance_meters' => '${widget.nextMinimum}',
              'max_distance_meters' => '0',
              'fee_minor' => widget.mock ? '500.00' : '',
              'priority' => '100',
              _ => '',
            };
      fields[key] = TextEditingController(text: value);
    }
  }

  @override
  void dispose() {
    for (final c in fields.values) {
      c.dispose();
    }
    super.dispose();
  }

  Widget field(String key, String label, {bool text = false}) => Padding(
    padding: const EdgeInsets.only(bottom: 14),
    child: TextFormField(
      controller: fields[key],
      keyboardType: text
          ? TextInputType.text
          : TextInputType.numberWithOptions(decimal: key == 'fee_minor'),
      decoration: InputDecoration(labelText: label),
      validator: (value) {
        if (text) {
          return key == 'id' && (value?.trim().isEmpty ?? true)
              ? 'Enter an existing zone reference'
              : null;
        }
        return (key == 'fee_minor'
                    ? policyMoney(value ?? '')
                    : int.tryParse(value ?? '')) ==
                null
            ? 'Enter a valid number'
            : null;
      },
    ),
  );
  Future<void> time(bool first) async {
    final minute = first ? start : end;
    final result = await showTimePicker(
      context: context,
      initialTime: TimeOfDay(hour: minute ~/ 60, minute: minute % 60),
    );
    if (result != null && mounted) {
      setState(() {
        if (first) {
          start = result.hour * 60 + result.minute;
        } else {
          end = result.hour * 60 + result.minute;
        }
      });
    }
  }

  Future<void> submit() async {
    if (!form.currentState!.validate()) return;
    final value = period
        ? <String, dynamic>{
            'weekday': weekday,
            'start_minute': start,
            'end_minute': end,
          }
        : <String, dynamic>{
            if (zone) ...{
              'id': fields['id']!.text.trim(),
              'name': fields['name']!.text.trim(),
              'enabled': enabled,
              'priority': int.parse(fields['priority']!.text),
            },
            'min_distance_meters': int.parse(
              fields['min_distance_meters']!.text,
            ),
            'max_distance_meters': int.parse(
              fields['max_distance_meters']!.text,
            ),
            'fee_minor': policyMoney(fields['fee_minor']!.text),
          };
    final error = validateAdvancedPolicy({
      widget.kind: [value],
    });
    if (error != null) {
      await showOrderFeedback(
        context,
        title: 'Check this rule',
        message: error,
        warning: true,
      );
      return;
    }
    if (mounted) Navigator.pop(context, value);
  }

  @override
  Widget build(BuildContext context) => SingleChildScrollView(
    padding: EdgeInsets.fromLTRB(
      20,
      0,
      20,
      MediaQuery.viewInsetsOf(context).bottom + 24,
    ),
    child: Form(
      key: form,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            period
                ? 'Delivery window'
                : zone
                ? 'Service zone'
                : 'Distance tier',
            style: const TextStyle(fontSize: 23, fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 20),
          if (period) ...[
            DropdownButtonFormField<int>(
              initialValue: weekday,
              decoration: const InputDecoration(labelText: 'Weekday'),
              items: [
                for (var day = 1; day <= 7; day++)
                  DropdownMenuItem(
                    value: day,
                    child: Text(deliveryWeekdays[day - 1]),
                  ),
              ],
              onChanged: (value) => setState(() => weekday = value!),
            ),
            const SizedBox(height: 12),
            ListTile(
              contentPadding: EdgeInsets.zero,
              title: const Text('Starts'),
              subtitle: Text(policyClock(start)),
              trailing: const Icon(Icons.schedule_rounded),
              onTap: () => time(true),
            ),
            ListTile(
              contentPadding: EdgeInsets.zero,
              title: const Text('Ends'),
              subtitle: Text(policyClock(end)),
              trailing: const Icon(Icons.schedule_rounded),
              onTap: () => time(false),
            ),
            Text(
              end <= start
                  ? 'Ends the following day. Times follow the policy timezone.'
                  : 'Times follow the policy timezone.',
            ),
          ] else ...[
            if (zone) ...[
              field('id', 'Zone reference', text: true),
              field('name', 'Zone name', text: true),
              const Padding(
                padding: EdgeInsets.only(bottom: 14),
                child: Text(
                  'Use a zone reference recognized by your location authority. This does not draw a new map boundary.',
                ),
              ),
              SwitchListTile.adaptive(
                contentPadding: EdgeInsets.zero,
                title: const Text('Enable zone'),
                value: enabled,
                onChanged: (value) => setState(() => enabled = value),
              ),
            ],
            field('min_distance_meters', 'From (metres)'),
            field('max_distance_meters', 'Up to (metres, 0 = no upper limit)'),
            field('fee_minor', 'Delivery fee (${widget.currency})'),
            if (zone) field('priority', 'Priority'),
            const Text(
              'Ranges include the starting distance and exclude the upper limit. The branch service area still applies.',
            ),
          ],
          const SizedBox(height: 20),
          FilledButton(onPressed: submit, child: const Text('Save rule')),
        ],
      ),
    ),
  );
}
