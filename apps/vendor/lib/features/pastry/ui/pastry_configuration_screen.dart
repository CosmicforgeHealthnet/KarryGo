import '../../media/ui/vendor_media_screen.dart';
import 'dart:convert';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../crm/ui/crm_design.dart' show crmLabel;
import '../../crm/ui/crm_editors.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../production/ui/production_form_screen.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../../settings/ui/settings_design.dart';
import '../state/pastry_workspace_controller.dart';

const pastryMilestones = [
  'DESIGN_APPROVED',
  'BASE_PREPARED',
  'DECORATION',
  'CHILLING',
  'PACKED',
  'READY',
];
const pastryRisks = [
  'LATE_START',
  'CAPACITY_AT_RISK',
  'CUSTOMER_CHANGE_PENDING',
  'MATERIAL_DEPENDENCY',
  'QUALITY_REVIEW_REQUIRED',
];
List<PurchaseField> pastryHandlingFields(JsonObject old) => [
  PurchaseField(
    'temperature_class',
    'Temperature',
    kind: 'select',
    initial: old['temperature_class'],
    options: const {
      'AMBIENT': 'Ambient',
      'CHILLED': 'Chilled',
      'FROZEN': 'Frozen',
    },
  ),
  PurchaseField(
    'spill_risk',
    'Spill risk',
    kind: 'select',
    initial: old['spill_risk'],
    options: const {'LOW': 'Low', 'MEDIUM': 'Medium', 'HIGH': 'High'},
  ),
  PurchaseField(
    'weight_grams',
    'Weight (grams)',
    kind: 'number',
    initial: old['weight_grams'],
    max: 1000000,
  ),
  PurchaseField(
    'package_size',
    'Package size',
    kind: 'select',
    initial: old['package_size'],
    options: const {
      'SMALL': 'Small',
      'MEDIUM': 'Medium',
      'LARGE': 'Large',
      'OVERSIZE': 'Oversize',
    },
  ),
  for (final f in {
    'fragile': 'Fragile',
    'upright_required': 'Keep upright',
    'cold_chain_required': 'Cold chain required',
  }.entries)
    PurchaseField(f.key, f.value, kind: 'bool', initial: old[f.key] ?? false),
];
Future<JsonObject?> pastryConfiguration(
  BuildContext context,
  VendorRuntime runtime,
) => Navigator.push<JsonObject>(
  context,
  setupRoute<JsonObject>(PastryConfigurationScreen(runtime: runtime)),
);

class PastryConfigurationScreen extends StatefulWidget {
  const PastryConfigurationScreen({super.key, required this.runtime});
  final VendorRuntime runtime;
  @override
  State<PastryConfigurationScreen> createState() =>
      _PastryConfigurationScreenState();
}

class _PastryConfigurationScreenState extends State<PastryConfigurationScreen> {
  late JsonObject change = widget.runtime.mode == VendorDataMode.mock
      ? {
          'minutes_before_required_at': 1440,
          'disallowed_after_milestone': 'DECORATION',
        }
      : {};
  late JsonObject cancellation = widget.runtime.mode == VendorDataMode.mock
      ? {
          'review_minutes_before_required_at': 1440,
          'restrict_minutes_before_required_at': 240,
          'review_after_milestone': 'BASE_PREPARED',
          'restrict_after_milestone': 'DECORATION',
        }
      : {};
  late JsonObject handling = widget.runtime.mode == VendorDataMode.mock
      ? {
          'temperature_class': 'CHILLED',
          'spill_risk': 'LOW',
          'package_size': 'MEDIUM',
          'fragile': true,
          'upright_required': true,
          'cold_chain_required': false,
        }
      : {};
  final flags = <String>[];
  Future<void> policy(bool cancel) async {
    final old = cancel ? cancellation : change;
    final keys = cancel
        ? {
            'review_minutes_before_required_at':
                'Review required before delivery (minutes)',
            'restrict_minutes_before_required_at':
                'Cancellation restricted before delivery (minutes)',
          }
        : {
            'minutes_before_required_at':
                'Changes close before delivery (minutes)',
          };
    final milestones = cancel
        ? {
            'review_after_milestone': 'Review required from milestone',
            'restrict_after_milestone':
                'Cancellation restricted from milestone',
          }
        : {'disallowed_after_milestone': 'Changes close from milestone'};
    final values = await productionForm(
      context,
      cancel ? 'Cancellation policy' : 'Design change policy',
      [
        for (final e in keys.entries)
          PurchaseField(
            e.key,
            e.value,
            kind: 'number',
            initial: old[e.key],
            max: 129600,
          ),
        for (final e in milestones.entries)
          PurchaseField(
            e.key,
            e.value,
            kind: 'select',
            initial: old[e.key],
            options: {for (final m in pastryMilestones) m: crmLabel(m)},
          ),
      ],
    );
    if (values == null || !mounted) return;
    setState(() {
      if (cancel) {
        cancellation = values;
      } else {
        change = values;
      }
    });
  }

  Future<void> delivery() async {
    final values = await productionForm(
      context,
      'Delivery handling',
      pastryHandlingFields(handling),
    );
    if (values != null && mounted) setState(() => handling = values);
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Request configuration')),
    body: SafeArea(
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 680),
          child: ListView(
            padding: const EdgeInsets.all(20),
            children: [
              const Text(
                'Protect the details',
                style: TextStyle(fontSize: 28, fontWeight: FontWeight.w800),
              ),
              const SizedBox(height: 8),
              const Text(
                'Set the change window, cancellation rules and delivery handling before creating this request.',
              ),
              const SizedBox(height: 24),
              SettingsCard(
                child: SettingsLink(
                  'Design changes',
                  Icons.edit_calendar_outlined,
                  color: const Color(0xFF8B65B9).forInk(context),
                  subtitle: change.isEmpty
                      ? 'Use server defaults'
                      : '${change['minutes_before_required_at'] ?? 0} minutes before delivery / ${crmLabel(change['disallowed_after_milestone'] ?? 'DECORATION')}',
                  onTap: () => policy(false),
                ),
              ),
              SettingsCard(
                child: SettingsLink(
                  'Cancellation policy',
                  Icons.event_busy_outlined,
                  color: const Color(0xFFB97C35).forInk(context),
                  subtitle: cancellation.isEmpty
                      ? 'Use server defaults'
                      : 'Review and restriction rules configured',
                  onTap: () => policy(true),
                ),
              ),
              SettingsCard(
                child: SettingsLink(
                  'Delivery handling',
                  Icons.inventory_2_outlined,
                  subtitle: handling.isEmpty
                      ? 'No additional handling requirements'
                      : crmLabel(
                          handling['temperature_class'] ??
                              'Handling configured',
                        ),
                  onTap: delivery,
                ),
              ),
              const SizedBox(height: 12),
              const SettingsHeading('Risk flags'),
              const Text('Select only the risks that apply to this request.'),
              const SizedBox(height: 12),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  for (final flag in pastryRisks)
                    FilterChip(
                      label: Text(crmLabel(flag)),
                      selected: flags.contains(flag),
                      selectedColor: const Color(
                        0xFFB97C35,
                      ).forInk(context).withValues(alpha: .14),
                      checkmarkColor: const Color(0xFFB97C35).forInk(context),
                      onSelected: (selected) => setState(() {
                        if (selected) {
                          flags.add(flag);
                        } else {
                          flags.remove(flag);
                        }
                      }),
                    ),
                ],
              ),
              const SizedBox(height: 28),
              SettingsAction(
                'Use configuration',
                onPressed: () => Navigator.pop(context, {
                  'change_policy': change,
                  'cancellation_policy': cancellation,
                  'logistics': handling,
                  'risk_flags': flags,
                }),
              ),
              const SizedBox(height: 24),
            ],
          ),
        ),
      ),
    ),
  );
}

Future<List<String>?> pastryListEditor(
  BuildContext context,
  String title,
  List<String> initial, {
  int max = 20,
  int itemMax = 200,
}) async {
  final values = await productionForm(
    context,
    title,
    [
      PurchaseField(
        'entries',
        'One entry per line',
        kind: 'notes',
        initial: initial.join('\n'),
      ),
    ],
    validate: (v) {
      final list = '${v['entries'] ?? ''}'
          .split('\n')
          .map((s) => s.trim())
          .where((s) => s.isNotEmpty)
          .toList();
      if (list.length > max) return 'Use at most $max entries.';
      if (list.any((s) => s.length > itemMax)) {
        return 'Each entry must be at most $itemMax characters.';
      }
      if (list.toSet().length != list.length) {
        return 'Remove duplicate entries.';
      }
      return null;
    },
  );
  return values == null
      ? null
      : '${values['entries'] ?? ''}'
            .split('\n')
            .map((s) => s.trim())
            .where((s) => s.isNotEmpty)
            .toList();
}

Future<JsonObject?> pastryDesign(
  BuildContext context,
  VendorRuntime runtime,
  JsonObject initial, {
  required String requestId,
}) => Navigator.push<JsonObject>(
  context,
  setupRoute<JsonObject>(
    PastryDesignScreen(
      runtime: runtime,
      initial: initial,
      requestId: requestId,
    ),
  ),
);

class PastryDesignScreen extends StatefulWidget {
  const PastryDesignScreen({
    super.key,
    required this.runtime,
    required this.initial,
    required this.requestId,
  });
  final VendorRuntime runtime;
  final JsonObject initial;
  final String requestId;
  @override
  State<PastryDesignScreen> createState() => _PastryDesignScreenState();
}

class _PastryDesignScreenState extends State<PastryDesignScreen> {
  late final design = <String, dynamic>{
    if (widget.runtime.mode == VendorDataMode.mock) ...{
      'occasion': 'Anniversary',
      'servings': 24,
      'theme': 'Elegant celebration',
      'inscription': 'Happy anniversary',
      'notes': 'Clean buttercream finish.',
      'colours': ['Ivory', 'Gold'],
    },
    ...widget.initial,
  };
  bool reading = false;
  bool get mock => widget.runtime.mode == VendorDataMode.mock;
  bool compatible(String key) =>
      design[key] == null ||
      design[key] is List && (design[key] as List).every((v) => v is String);
  Future<void> media() async {
    final result = await vendorMedia(
      context,
      runtime: widget.runtime,
      targetType: 'PASTRY_REQUEST',
      targetRef: widget.requestId,
      entries: [
        for (final ref in design['reference_media'] as List? ?? [])
          {'media_ref': ref},
      ],
    );
    if (result != null && mounted) {
      setState(
        () => design['reference_media'] = [
          for (final e in result) e['media_ref'],
        ],
      );
    }
  }

  Future<void> details() async {
    final values = await productionForm(
      context,
      'Design details',
      [
        for (final e in {
          'occasion': 'Occasion',
          'theme': 'Theme',
          'inscription': 'Inscription',
          'notes': 'Design notes',
        }.entries)
          PurchaseField(
            e.key,
            e.value,
            kind: e.key == 'notes' ? 'notes' : 'text',
            initial:
                design[e.key] ??
                (mock
                    ? {
                        'occasion': 'Anniversary',
                        'theme': 'Elegant celebration',
                        'inscription': 'Happy anniversary',
                        'notes': 'Clean buttercream finish.',
                      }[e.key]
                    : null),
          ),
        PurchaseField(
          'servings',
          'Servings',
          kind: 'number',
          required: true,
          min: 1,
          max: 10000,
          initial: design['servings'] ?? (mock ? 24 : null),
        ),
        PurchaseField(
          'required_at',
          'Required date & time',
          kind: 'date',
          initial: design['required_at'],
        ),
      ],
      validate: (v) => '${v['notes'] ?? ''}'.length > 4000
          ? 'Keep design notes within 4,000 characters.'
          : null,
    );
    if (values != null && mounted) setState(() => design.addAll(values));
  }

  Future<void> list(
    String key,
    String title, {
    int max = 20,
    int itemMax = 200,
  }) async {
    if (!compatible(key)) return;
    final values = await pastryListEditor(
      context,
      title,
      List<String>.from(design[key] ?? []),
      max: max,
      itemMax: itemMax,
    );
    if (values != null && mounted) setState(() => design[key] = values);
  }

  Future<void> selections() async {
    if (reading || !compatible('selected_options')) return;
    setState(() => reading = true);
    try {
      final c = PastryWorkspaceController(
        widget.runtime,
        '$pastryBase/option-groups',
      );
      final choices = <String, String>{};
      try {
        final groups = c.collection(await c.fetch('$pastryBase/option-groups'));
        for (final group in groups.where((g) => g['status'] == 'ACTIVE')) {
          final options = c.collection(
            await c.fetch(
              '$pastryBase/option-groups/{groupId}/options',
              params: {'groupId': '${group['id']}'},
            ),
          );
          for (final option in options.where((o) => o['status'] == 'ACTIVE')) {
            choices['${option['id']}'] = '${group['name']} / ${option['name']}';
          }
        }
      } finally {
        c.dispose();
      }
      if (!mounted) return;
      final saved = List<String>.from(design['selected_options'] ?? []);
      for (final id in saved) {
        choices.putIfAbsent(id, () => '$id (saved option)');
      }
      final selected = await crmChoose(
        context,
        'Design options',
        choices,
        multiple: true,
        optional: true,
        initial: saved,
      );
      if (selected != null && mounted) {
        setState(() => design['selected_options'] = selected);
      }
    } catch (_) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Options unavailable',
          message:
              'Option choices could not be loaded. Try again before changing the saved selection.',
          warning: true,
        );
      }
    } finally {
      if (mounted) setState(() => reading = false);
    }
  }

  Widget saved(Object? value) {
    if (value is Map) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          for (final e in jsonObject(value).entries) ...[
            Text(
              crmLabel(e.key),
              style: const TextStyle(fontWeight: FontWeight.w600),
            ),
            saved(e.value),
            const SizedBox(height: 8),
          ],
        ],
      );
    }
    if (value is List) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [for (final v in value) saved(v)],
      );
    }
    return Text(value == null ? 'Not specified' : '$value');
  }

  Future<void> requirements(String key) async {
    if (reading || design[key] is! Map) return;
    final updated = await Navigator.push<JsonObject>(
      context,
      setupRoute<JsonObject>(
        PastrySavedRequirementsScreen(
          title: crmLabel(key),
          initial: jsonObject(design[key]),
        ),
      ),
    );
    if (updated != null && mounted) setState(() => design[key] = updated);
  }

  Future<void> finish() async {
    if (reading) return;
    if (design['servings'] is! int ||
        design['servings'] < 1 ||
        design['servings'] > 10000) {
      await details();
      if (!mounted) return;
      if (design['servings'] is! int ||
          design['servings'] < 1 ||
          design['servings'] > 10000) {
        return;
      }
    }
    if (mounted) Navigator.pop(context, design);
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Design specification')),
    body: SafeArea(
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 680),
          child: ListView(
            padding: const EdgeInsets.all(20),
            children: [
              const Text(
                'Make it personal',
                style: TextStyle(fontSize: 28, fontWeight: FontWeight.w800),
              ),
              const SizedBox(height: 8),
              const Text(
                'Shape the design and keep the important details together.',
              ),
              const SizedBox(height: 24),
              SettingsCard(
                child: SettingsLink(
                  'Design details',
                  Icons.cake_outlined,
                  color: const Color(0xFFB97C35).forInk(context),
                  subtitle: design['servings'] == null
                      ? 'Occasion, servings, theme and inscription'
                      : '${design['servings']} servings / ${design['theme'] ?? design['occasion'] ?? 'Design configured'}',
                  onTap: reading ? null : details,
                ),
              ),
              SettingsCard(
                child: SettingsLink(
                  'Colours',
                  Icons.palette_outlined,
                  color: const Color(0xFF8B65B9).forInk(context),
                  subtitle: compatible('colours')
                      ? (design['colours'] as List? ?? []).join(', ')
                      : 'Saved colour details',
                  onTap: reading || !compatible('colours')
                      ? null
                      : () => list('colours', 'Design colours'),
                ),
              ),
              SettingsCard(
                child: SettingsLink(
                  'Design options',
                  Icons.auto_awesome_outlined,
                  subtitle: compatible('selected_options')
                      ? '${(design['selected_options'] as List? ?? []).length} selected'
                      : 'Saved selection details',
                  onTap: reading || !compatible('selected_options')
                      ? null
                      : selections,
                ),
              ),
              SettingsCard(
                child: SettingsLink(
                  'Reference media',
                  Icons.photo_library_outlined,
                  color: const Color(0xFF557FAC).forInk(context),
                  subtitle: compatible('reference_media')
                      ? '${(design['reference_media'] as List? ?? []).length} references'
                      : 'Saved media details',
                  onTap: reading || !compatible('reference_media')
                      ? null
                      : media,
                ),
              ),
              for (final key in [
                'production_requirements',
                'logistics_requirements',
              ])
                if (design[key] != null)
                  SettingsCard(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        SettingsHeading(crmLabel(key)),
                        saved(design[key]),
                        if (design[key] is Map) ...[
                          const SizedBox(height: 16),
                          SettingsAction(
                            'Edit saved requirements',
                            secondary: true,
                            icon: Icons.tune_rounded,
                            onPressed: reading ? null : () => requirements(key),
                          ),
                        ],
                      ],
                    ),
                  ),
              for (final key in [
                'colours',
                'selected_options',
                'reference_media',
              ])
                if (!compatible(key))
                  SettingsCard(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        SettingsHeading(crmLabel(key)),
                        saved(design[key]),
                        if (design[key] is Map) ...[
                          const SizedBox(height: 16),
                          SettingsAction(
                            'Edit saved requirements',
                            secondary: true,
                            icon: Icons.tune_rounded,
                            onPressed: reading ? null : () => requirements(key),
                          ),
                        ],
                        const SizedBox(height: 8),
                        const Text(
                          'These saved details will be kept with the design.',
                        ),
                      ],
                    ),
                  ),
              const SizedBox(height: 16),
              SettingsAction(
                'Use this design',
                onPressed: reading ? null : finish,
                busy: reading,
              ),
              const SizedBox(height: 24),
            ],
          ),
        ),
      ),
    ),
  );
}

class PastrySavedRequirementsScreen extends StatefulWidget {
  const PastrySavedRequirementsScreen({
    super.key,
    required this.title,
    required this.initial,
    this.description =
        'Edit the requirements already saved for this design. Other saved details are retained.',
    this.saveLabel = 'Save requirements',
  });
  final String title, description, saveLabel;
  final JsonObject initial;
  @override
  State<PastrySavedRequirementsScreen> createState() =>
      _PastrySavedRequirementsScreenState();
}

class _PastrySavedRequirementsScreenState
    extends State<PastrySavedRequirementsScreen> {
  late final values = jsonObject(jsonDecode(jsonEncode(widget.initial)));
  Future<void> edit(String key) async {
    final old = values[key];
    Object? next;
    if (old is Map) {
      next = await Navigator.push<JsonObject>(
        context,
        setupRoute<JsonObject>(
          PastrySavedRequirementsScreen(
            title: crmLabel(key),
            initial: jsonObject(old),
            description: widget.description,
            saveLabel: widget.saveLabel,
          ),
        ),
      );
    } else if (old is String || old is bool || old is num) {
      final result = await productionForm(
        context,
        crmLabel(key),
        [
          PurchaseField(
            'value',
            crmLabel(key),
            kind: old is bool ? 'bool' : 'text',
            initial: old,
            required: old is num,
          ),
        ],
        validate: (result) {
          if (old is num) {
            final number = num.tryParse('${result['value']}');
            if (number == null ||
                !number.isFinite ||
                old is int && number != number.roundToDouble()) {
              return 'Enter a valid ${old is int ? 'whole number' : 'number'}.';
            }
          }
          return null;
        },
      );
      if (result == null) return;
      next = old is num
          ? old is int
                ? num.parse('${result['value']}').toInt()
                : num.parse('${result['value']}').toDouble()
          : old is String
          ? result['value'] ?? ''
          : result['value'];
    }
    if (next != null && mounted) setState(() => values[key] = next);
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: Text(widget.title)),
    body: SafeArea(
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 680),
          child: ListView(
            padding: const EdgeInsets.all(20),
            children: [
              SettingsNote(widget.description, warning: true),
              if (values.isEmpty)
                const SettingsCard(
                  child: Text('No requirements have been supplied.'),
                ),
              for (final entry in values.entries)
                Padding(
                  padding: const EdgeInsets.only(bottom: 12),
                  child: SettingsCard(
                    child: SettingsLink(
                      crmLabel(entry.key),
                      entry.value is Map
                          ? Icons.tune_rounded
                          : entry.value is bool
                          ? Icons.toggle_on_outlined
                          : entry.value is num
                          ? Icons.numbers_rounded
                          : Icons.notes_rounded,
                      subtitle: entry.value is Map
                          ? '${(entry.value as Map).length} saved details'
                          : entry.value is List
                          ? '${(entry.value as List).length} saved entries'
                          : entry.value == null
                          ? 'Not specified'
                          : '${entry.value}',
                      onTap:
                          entry.value is Map ||
                              entry.value is String ||
                              entry.value is bool ||
                              entry.value is num
                          ? () => edit(entry.key)
                          : null,
                    ),
                  ),
                ),
              const SizedBox(height: 16),
              SettingsAction(
                widget.saveLabel,
                icon: Icons.check_rounded,
                onPressed: () => Navigator.pop(context, values),
              ),
            ],
          ),
        ),
      ),
    ),
  );
}
