import 'package:flutter/material.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../production/ui/production_form_screen.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../../settings/ui/settings_design.dart';
import 'crm_design.dart';

Future<List<String>?> crmChoose(
  BuildContext context,
  String title,
  Map<String, String> options, {
  bool multiple = false,
  List<String> initial = const [],
  bool optional = false,
}) => Navigator.push<List<String>>(
  context,
  setupRoute<List<String>>(
    CrmChoiceScreen(
      title: title,
      options: options,
      multiple: multiple,
      initial: initial,
      optional: optional,
    ),
  ),
);

class CrmChoiceScreen extends StatefulWidget {
  const CrmChoiceScreen({
    super.key,
    required this.title,
    required this.options,
    this.multiple = false,
    this.initial = const [],
    this.optional = false,
  });
  final String title;
  final Map<String, String> options;
  final bool multiple, optional;
  final List<String> initial;
  @override
  State<CrmChoiceScreen> createState() => _CrmChoiceScreenState();
}

class _CrmChoiceScreenState extends State<CrmChoiceScreen> {
  late final selected = widget.initial
      .where(widget.options.containsKey)
      .toSet();
  String search = '';
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: Text(widget.title)),
    body: SafeArea(
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 640),
          child: Column(
            children: [
              Padding(
                padding: const EdgeInsets.all(20),
                child: TextField(
                  decoration: const InputDecoration(
                    prefixIcon: Icon(Icons.search_rounded),
                    hintText: 'Search',
                  ),
                  onChanged: (v) => setState(() => search = v.toLowerCase()),
                ),
              ),
              Expanded(
                child: ListView(
                  children: [
                    for (final e in widget.options.entries.where(
                      (e) => e.value.toLowerCase().contains(search),
                    ))
                      CheckboxListTile(
                        value: selected.contains(e.key),
                        activeColor: KarryamColors.tealInk.forInk(context),
                        title: Text(e.value),
                        onChanged: (v) => setState(() {
                          if (!widget.multiple) selected.clear();
                          if (v == true) {
                            selected.add(e.key);
                          } else {
                            selected.remove(e.key);
                          }
                        }),
                      ),
                    if (widget.options.isEmpty)
                      const Padding(
                        padding: EdgeInsets.all(24),
                        child: Text('No available records.'),
                      ),
                  ],
                ),
              ),
              Padding(
                padding: const EdgeInsets.all(20),
                child: SettingsAction(
                  'Continue',
                  onPressed: selected.isNotEmpty || widget.optional
                      ? () => Navigator.pop(context, selected.toList())
                      : null,
                ),
              ),
            ],
          ),
        ),
      ),
    ),
  );
}

class CrmRuleScreen extends StatefulWidget {
  const CrmRuleScreen({
    super.key,
    required this.registry,
    required this.tags,
    required this.scope,
  });
  final JsonObject registry;
  final Map<String, String> tags;
  final String scope;
  @override
  State<CrmRuleScreen> createState() => _CrmRuleScreenState();
}

class _CrmRuleScreenState extends State<CrmRuleScreen> {
  final conditions = <JsonObject>[];
  String group = 'AND';
  Future<void> add() async {
    final fields = (widget.registry['fields'] as List)
        .map(jsonObject)
        .where(
          (f) =>
              f['available'] == true &&
              (f['scopes'] as List).contains(widget.scope) &&
              !(widget.scope == 'BUSINESS' && f['branch_only'] == true),
        )
        .toList();
    final selected = await crmChoose(context, 'Choose a customer attribute', {
      for (final f in fields) '${f['key']}': '${f['label']}',
    });
    if (selected == null || !mounted) return;
    final f = fields.firstWhere((f) => f['key'] == selected.single);
    final op = await productionForm(context, 'Choose condition', [
      PurchaseField(
        'operator',
        'Comparison',
        kind: 'select',
        required: true,
        options: crmOptions(List<String>.from(f['operators'] as List)),
      ),
    ]);
    if (op == null || !mounted) return;
    final operator = op['operator'];
    Object? value;
    if (!['EXISTS', 'NOT_EXISTS'].contains(operator)) {
      if (f['type'] == 'TAG_REFERENCE') {
        final chosen = await crmChoose(context, 'Choose tag', widget.tags);
        if (chosen == null) return;
        value = chosen.single;
      } else if (operator == 'WITHIN_LAST') {
        final period = await productionForm(context, 'Recent period', [
          const PurchaseField(
            'amount',
            'Amount',
            kind: 'number',
            required: true,
            min: 1,
          ),
          PurchaseField(
            'unit',
            'Unit',
            kind: 'select',
            required: true,
            options: crmOptions(['DAY', 'WEEK', 'MONTH', 'YEAR']),
          ),
        ]);
        if (period == null) return;
        value = period;
      } else {
        final multi = ['IN', 'NOT_IN', 'BETWEEN'].contains(operator);
        final enums = List<String>.from(
          f['enum_values'] as List? ?? f['values'] as List? ?? [],
        );
        if (enums.isNotEmpty && multi && operator != 'BETWEEN') {
          final chosen = await crmChoose(
            context,
            'Choose values',
            crmOptions(enums),
            multiple: true,
          );
          if (chosen == null) return;
          value = chosen;
        } else {
          final kind = enums.isNotEmpty
              ? 'select'
              : f['type'] == 'BOOLEAN'
              ? 'select'
              : f['type'] == 'INTEGER' || f['type'] == 'MONEY_MINOR'
              ? 'number'
              : f['type'] == 'TIMESTAMP'
              ? 'date'
              : 'text';
          final result = await productionForm(context, 'Condition value', [
            for (final key
                in operator == 'BETWEEN' ? ['from', 'to'] : ['value'])
              PurchaseField(
                key,
                operator == 'BETWEEN'
                    ? (key == 'from' ? 'From' : 'To')
                    : 'Value',
                kind: kind,
                required: true,
                options: enums.isNotEmpty
                    ? crmOptions(enums)
                    : f['type'] == 'BOOLEAN'
                    ? const {'true': 'Yes', 'false': 'No'}
                    : const {},
              ),
          ]);
          if (result == null) return;
          value = operator == 'BETWEEN'
              ? [result['from'], result['to']]
              : result['value'];
          if (f['type'] == 'BOOLEAN') value = value == 'true';
          if (multi && operator != 'BETWEEN') value = [value];
        }
      }
    }
    if (mounted) {
      setState(
        () => conditions.add({
          'field': f['key'],
          'operator': operator,
          'value': ?value,
        }),
      );
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Segment rules')),
    body: SafeArea(
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 640),
          child: ListView(
            padding: const EdgeInsets.all(20),
            children: [
              const SettingsNote(
                'Choose from the available customer attributes. Preview checks the rule against current server data.',
              ),
              Wrap(
                spacing: 10,
                children: [
                  for (final op in ['AND', 'OR'])
                    ChoiceChip(
                      label: Text(op == 'AND' ? 'Match all' : 'Match any'),
                      selected: group == op,
                      onSelected: (_) => setState(() => group = op),
                    ),
                ],
              ),
              const SizedBox(height: 20),
              for (final entry in conditions.indexed)
                Padding(
                  padding: const EdgeInsets.only(bottom: 14),
                  child: SettingsCard(
                    child: Row(
                      children: [
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(crmLabel(entry.$2['field'])),
                              Text(
                                '${crmLabel(entry.$2['operator'])} ${entry.$2['value'] ?? ''}',
                              ),
                            ],
                          ),
                        ),
                        IconButton(
                          tooltip: 'Remove condition',
                          onPressed: () =>
                              setState(() => conditions.removeAt(entry.$1)),
                          icon: const Icon(Icons.close_rounded),
                        ),
                      ],
                    ),
                  ),
                ),
              SettingsAction(
                'Add condition',
                secondary: true,
                icon: Icons.add_rounded,
                onPressed:
                    conditions.length <
                        ((widget.registry['limits'] as Map?)?['max_nodes']
                                    as int? ??
                                50) -
                            1
                    ? add
                    : null,
              ),
              const SizedBox(height: 16),
              SettingsAction(
                'Use these rules',
                onPressed: conditions.isEmpty
                    ? null
                    : () => Navigator.pop(context, {
                        'schema_version': widget.registry['schema_version'],
                        'root': conditions.length == 1
                            ? conditions.single
                            : {'op': group, 'children': conditions},
                      }),
              ),
            ],
          ),
        ),
      ),
    ),
  );
}

class CrmTiersScreen extends StatefulWidget {
  const CrmTiersScreen({super.key, required this.initial});
  final List<JsonObject> initial;
  @override
  State<CrmTiersScreen> createState() => _CrmTiersScreenState();
}

class _CrmTiersScreenState extends State<CrmTiersScreen> {
  late final tiers = widget.initial.map((r) => JsonObject.from(r)).toList();
  Future<void> edit([int? index]) async {
    final r = index == null ? null : tiers[index];
    final result = await productionForm(
      context,
      index == null ? 'New loyalty tier' : 'Edit tier',
      [
        PurchaseField(
          'tier_key',
          'Tier key',
          required: true,
          initial: r?['tier_key'],
        ),
        PurchaseField(
          'display_name',
          'Display name',
          required: true,
          initial: r?['display_name'],
        ),
        PurchaseField(
          'minimum_lifetime_points',
          'Lifetime points threshold',
          kind: 'number',
          required: true,
          initial: r?['minimum_lifetime_points'],
        ),
      ],
    );
    if (result != null && mounted) {
      setState(() {
        if (index == null) {
          tiers.add(result);
        } else {
          tiers[index] = result;
        }
      });
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Loyalty tier ladder')),
    body: SafeArea(
      child: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          for (final e in tiers.indexed)
            SettingsCard(
              child: Row(
                children: [
                  Expanded(
                    child: ListTile(
                      contentPadding: EdgeInsets.zero,
                      title: Text('${e.$2['display_name']}'),
                      subtitle: Text(
                        '${e.$2['minimum_lifetime_points']} lifetime points',
                      ),
                      onTap: () => edit(e.$1),
                    ),
                  ),
                  IconButton(
                    tooltip: 'Remove tier',
                    onPressed: () => setState(() => tiers.removeAt(e.$1)),
                    icon: const Icon(Icons.close_rounded),
                  ),
                ],
              ),
            ),
          const SizedBox(height: 16),
          SettingsAction(
            'Add tier',
            secondary: true,
            icon: Icons.add_rounded,
            onPressed: () => edit(),
          ),
          const SizedBox(height: 16),
          SettingsAction(
            'Review ladder',
            onPressed: () => Navigator.pop(context, tiers),
          ),
        ],
      ),
    ),
  );
}
