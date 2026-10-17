import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../state/settings_controller.dart';
import 'settings_design.dart';
import '../../../core/widgets/nigerian_phone_prefix.dart';
import '../../onboarding/ui/location_pick_field.dart';

class SettingsEditor extends StatefulWidget {
  const SettingsEditor({
    super.key,
    required this.runtime,
    required this.kind,
    required this.onSave,
    required this.errorMessage,
    required this.canSave,
    this.record,
    this.onReload,
    this.brands = const [],
    this.branches = const [],
    this.roles = const [],
  });
  final VendorRuntime runtime;
  final SettingsKind kind;
  final JsonObject? record;
  final List<JsonObject> brands, branches, roles;
  final Future<bool> Function(JsonObject) onSave;
  final String? Function() errorMessage;
  final bool Function() canSave;
  final Future<JsonObject?> Function()? onReload;
  @override
  State<SettingsEditor> createState() => _SettingsEditorState();
}

class _SettingsEditorState extends State<SettingsEditor> {
  final _form = GlobalKey<FormState>();
  final Map<String, TextEditingController> _fields = {};
  List<JsonObject> _media = [];
  String _brand = '', _role = '', _branch = '';
  bool _email = true, _saving = false, _dirty = false, _saved = false;
  String? _error;
  Object? _expectedVersion;
  SettingsKind get kind => widget.kind;
  bool get mock => widget.runtime.mode == VendorDataMode.mock;
  String get title => kind == SettingsKind.profile
      ? 'Edit business profile'
      : kind == SettingsKind.staff
      ? 'Add staff member'
      : '${widget.record == null ? 'Create' : 'Edit'} ${kind == SettingsKind.brands ? 'brand' : 'branch'}';
  @override
  void initState() {
    super.initState();
    final r = widget.record ?? <String, dynamic>{};
    _expectedVersion = r['version'];
    _brand = r['brand_id']?.toString() ?? '';
    if (widget.roles.isNotEmpty && mock) {
      _role =
          (widget.roles
                      .where((r) => r['id'] == 'role_branch_manager')
                      .firstOrNull ??
                  widget.roles.first)['id']
              as String;
    }
    _media = (r['media_refs'] as List? ?? []).map(jsonObject).toList();
    final address = jsonObject(r['address'] ?? {});
    final values = <String, Object?>{...r, ...address};
    final defaults = {
      'display_name': kind == SettingsKind.brands
          ? 'Karryam Kitchen'
          : 'Victoria Island branch',
      'description': 'Fresh meals, prepared with care.',
      'line1': '12 Admiralty Way',
      'city': 'Lagos',
      'region': 'Lagos',
      'country_code': 'NG',
      'latitude': 6.4281,
      'longitude': 3.4219,
      'timezone': 'Africa/Lagos',
      'email': 'team@example.com',
      'phone': '8012345678',
    };
    final keys = switch (kind) {
      SettingsKind.profile => [
        'legal_name',
        'trading_name',
        'support_email',
        'support_phone',
        'currency',
        'timezone',
      ],
      SettingsKind.brands => ['display_name', 'description'],
      SettingsKind.branches => [
        'display_name',
        'line1',
        'line2',
        'city',
        'region',
        'postal_code',
        'country_code',
        'latitude',
        'longitude',
        'timezone',
      ],
      _ => ['email', 'phone'],
    };
    for (final key in keys) {
      final initial =
          (values[key] ??
                  (mock && widget.record == null ? defaults[key] : null))
              ?.toString() ??
          '';
      _fields[key] = TextEditingController(
        text: isPhoneFieldKey(key) ? nigerianPhoneLocal(initial) : initial,
      );
    }
  }

  Future<void> _reload() async {
    if (widget.record != null &&
            _dirty &&
            !await confirmSettings(
              context,
              'Reload details',
              'This replaces your unsaved changes.',
            ) ||
        !mounted) {
      return;
    }
    setState(() => _saving = true);
    try {
      final r = await widget.onReload!();
      if (!mounted) return;
      setState(() {
        if (r != null) {
          final values = <String, dynamic>{
            ...r,
            ...jsonObject(r['address'] ?? {}),
          };
          for (final field in _fields.entries) {
            final text = values[field.key]?.toString() ?? '';
            field.value.text = isPhoneFieldKey(field.key)
                ? nigerianPhoneLocal(text)
                : text;
          }
          _brand = r['brand_id']?.toString() ?? '';
          _media = (r['media_refs'] as List? ?? []).map(jsonObject).toList();
          _expectedVersion = r['version'];
          _dirty = false;
        }
        _error = widget.canSave() ? null : widget.errorMessage();
      });
    } catch (e) {
      if (mounted) setState(() => _error = SettingsController.message(e));
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  void dispose() {
    for (final field in _fields.values) {
      field.dispose();
    }
    super.dispose();
  }

  String value(String key) {
    final text = _fields[key]!.text.trim();
    return isPhoneFieldKey(key) && text.isNotEmpty
        ? nigerianPhoneForSubmit(text)
        : text;
  }

  Widget _field(
    String key,
    String label, {
    bool required = false,
    bool numeric = false,
    int? maxLength,
    int lines = 1,
  }) => Padding(
    padding: const EdgeInsets.only(bottom: 18),
    child: TextFormField(
      controller: _fields[key],
      enabled: !_saving,
      onChanged: (_) => setState(() => _dirty = true),
      maxLines: lines,
      keyboardType: numeric
          ? const TextInputType.numberWithOptions(decimal: true, signed: true)
          : key.contains('email')
          ? TextInputType.emailAddress
          : key.contains('phone')
          ? TextInputType.phone
          : TextInputType.text,
      decoration: InputDecoration(
        labelText: label,
        prefixIcon: isPhoneFieldKey(key) ? nigerianPhonePrefix : null,
        hintText: isPhoneFieldKey(key) ? nigerianPhoneHint : null,
      ),
      validator: (input) {
        final text = input?.trim() ?? '';
        if (required && text.isEmpty) return 'Enter ${label.toLowerCase()}.';
        if (text.isEmpty) return null;
        if (maxLength != null && text.length > maxLength) {
          return 'Use up to $maxLength characters.';
        }
        if ((key == 'legal_name' || key == 'display_name') && text.length < 2) {
          return 'Use at least 2 characters.';
        }
        if (key.contains('email') &&
            !RegExp(r'^[^\s@]+@[^\s@]+\.[^\s@]+$').hasMatch(text)) {
          return 'Enter a valid email address.';
        }
        if (isPhoneFieldKey(key)) {
          return validateNigerianPhone(text, required: required);
        }
        if (key == 'currency' && !RegExp(r'^[A-Za-z]{3}$').hasMatch(text)) {
          return 'Use a three-letter currency code.';
        }
        if (key == 'country_code' && !RegExp(r'^[A-Za-z]{2}$').hasMatch(text)) {
          return 'Use a two-letter country code.';
        }
        if (numeric) {
          final n = double.tryParse(text);
          if (n == null ||
              !n.isFinite ||
              n.abs() > (key == 'latitude' ? 90 : 180)) {
            return 'Enter a valid ${label.toLowerCase()}.';
          }
        }
        return null;
      },
    ),
  );
  Widget _select(
    String label,
    String selected,
    List<JsonObject> options,
    String name,
    void Function(String) change, {
    String? empty,
  }) => Padding(
    padding: const EdgeInsets.only(bottom: 18),
    child: DropdownButtonFormField<String>(
      initialValue: selected,
      isExpanded: true,
      decoration: InputDecoration(labelText: label),
      items: [
        DropdownMenuItem(
          value: '',
          child: Text(empty ?? 'Select ${label.toLowerCase()}'),
        ),
        if (selected.isNotEmpty && !options.any((o) => o['id'] == selected))
          DropdownMenuItem(value: selected, child: Text(selected)),
        for (final option in options)
          DropdownMenuItem(
            value: option['id'] as String,
            child: Text(
              option[name].toString(),
              overflow: TextOverflow.ellipsis,
            ),
          ),
      ],
      onChanged: _saving
          ? null
          : (v) => setState(() {
              _dirty = true;
              change(v!);
            }),
      validator: (v) => empty == null && (v == null || v.isEmpty)
          ? 'Select ${label.toLowerCase()}.'
          : null,
    ),
  );
  Future<void> _mediaReference() async {
    final field = TextEditingController();
    var kind = 'LOGO';
    final form = GlobalKey<FormState>();
    final result = await settingsSheet<JsonObject>(
      context,
      StatefulBuilder(
        builder: (context, setSheet) => Form(
          key: form,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const SettingsHeading(
                'Image reference',
                subtitle: 'Use an existing media-service reference.',
              ),
              DropdownButtonFormField<String>(
                initialValue: kind,
                decoration: const InputDecoration(labelText: 'Image type'),
                items: [
                  for (final v in ['LOGO', 'COVER', 'GALLERY'])
                    DropdownMenuItem(value: v, child: Text(v)),
                ],
                onChanged: (v) => setSheet(() => kind = v!),
              ),
              const SizedBox(height: 18),
              TextFormField(
                controller: field,
                decoration: const InputDecoration(labelText: 'Media reference'),
                validator: (v) =>
                    v == null ||
                        !RegExp(
                          r'^[A-Za-z0-9][A-Za-z0-9._:/-]{1,255}$',
                        ).hasMatch(v.trim()) ||
                        (v.startsWith('http://') || v.startsWith('https://'))
                    ? 'Enter an opaque media reference, not a URL.'
                    : null,
              ),
              const SizedBox(height: 22),
              SettingsAction(
                'Add reference',
                icon: Icons.add_rounded,
                onPressed: () {
                  if (form.currentState!.validate()) {
                    Navigator.pop(context, {
                      'kind': kind,
                      'media_ref': field.text.trim(),
                    });
                  }
                },
              ),
            ],
          ),
        ),
      ),
    );
    field.dispose();
    if (mounted && result != null) {
      setState(() {
        if (!_media.any(
          (m) =>
              m['kind'] == result['kind'] &&
              m['media_ref'] == result['media_ref'],
        )) {
          _media.add(result);
        }
        _dirty = true;
      });
    }
  }

  Future<void> _save() async {
    if (_saving || !widget.canSave() || !_form.currentState!.validate()) return;
    FocusScope.of(context).unfocus();
    setState(() {
      _saving = true;
      _error = null;
    });
    final body = switch (kind) {
      SettingsKind.profile => {
        for (final key in _fields.keys)
          key: key == 'currency' ? value(key).toUpperCase() : value(key),
      },
      SettingsKind.brands => {
        'display_name': value('display_name'),
        'description': value('description'),
        'media_refs': _media,
      },
      SettingsKind.branches => {
        'brand_id': _brand,
        'display_name': value('display_name'),
        'address': {
          for (final key in [
            'line1',
            'line2',
            'city',
            'region',
            'postal_code',
            'country_code',
          ])
            key: key == 'country_code' ? value(key).toUpperCase() : value(key),
        },
        'latitude': double.parse(value('latitude')),
        'longitude': double.parse(value('longitude')),
        'timezone': value('timezone'),
      },
      _ => {
        _email ? 'email' : 'phone': value(_email ? 'email' : 'phone'),
        'role_id': _role,
        'branch_id': _branch.isEmpty ? null : _branch,
      },
    };
    if (widget.record != null) {
      body['expected_version'] = _expectedVersion;
    }
    final saved = await widget.onSave(body);
    if (!mounted) return;
    setState(() {
      _saving = false;
      _saved = saved;
      _error = saved
          ? null
          : widget.errorMessage() ?? 'This change could not be saved.';
    });
    if (saved) Navigator.pop(context, true);
  }

  @override
  Widget build(BuildContext context) => PopScope(
    canPop: !_saving && (!_dirty || _saved),
    onPopInvokedWithResult: (didPop, result) async {
      if (!didPop &&
          !_saving &&
          await confirmSettings(
            context,
            'Discard changes',
            'Your unsaved changes will be lost.',
          ) &&
          context.mounted) {
        setState(() => _dirty = false);
        Navigator.pop(context);
      }
    },
    child: Scaffold(
      appBar: AppBar(
        title: Text(title),
        backgroundColor: KarryamColors.canvas.forSurface(context),
      ),
      bottomNavigationBar: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 12, 24, 16),
          child: SettingsAction(
            kind == SettingsKind.staff ? 'Add staff member' : 'Save changes',
            icon: Icons.check_rounded,
            busy: _saving,
            onPressed:
                !_saving &&
                    (widget.record == null || _dirty) &&
                    widget.canSave()
                ? _save
                : null,
          ),
        ),
      ),
      body: SingleChildScrollView(
        padding: const EdgeInsets.all(24),
        child: Form(
          key: _form,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  SettingsEmblem(kind, size: 72),
                  const SizedBox(width: 16),
                  Expanded(
                    child: Text(
                      kind == SettingsKind.profile
                          ? 'Business details'
                          : kind == SettingsKind.brands
                          ? 'Brand identity'
                          : kind == SettingsKind.branches
                          ? 'Branch details'
                          : 'Staff access',
                      style: const TextStyle(
                        fontSize: 25,
                        fontWeight: FontWeight.w700,
                        letterSpacing: -.8,
                      ),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 26),
              if (_error != null) ...[
                SettingsNote(_error!),
                if (widget.onReload != null)
                  SettingsAction(
                    'Refresh details',
                    secondary: true,
                    onPressed: _saving ? null : _reload,
                  ),
              ],
              if (kind == SettingsKind.profile) ...[
                _field(
                  'legal_name',
                  'Legal name',
                  required: true,
                  maxLength: 200,
                ),
                _field('trading_name', 'Trading name', maxLength: 200),
                _field('support_email', 'Support email'),
                _field('support_phone', 'Support phone'),
                // Nigeria only: naira and Lagos time are kept as they are.
                Offstage(
                  child: Column(
                    children: [
                      _field('currency', 'Currency', required: true),
                      _field('timezone', 'Timezone', required: true),
                    ],
                  ),
                ),
                const Text(
                  'Prices are in Nigerian Naira (₦) and times use Lagos time (WAT).',
                ),
                const SizedBox(height: 12),
                const Text(
                  'Profile changes may require updated business verification.',
                ),
              ],
              if (kind == SettingsKind.brands) ...[
                _field(
                  'display_name',
                  'Brand name',
                  required: true,
                  maxLength: 160,
                ),
                _field('description', 'Description', maxLength: 2000, lines: 3),
                ExpansionTile(
                  tilePadding: EdgeInsets.zero,
                  title: const Text('Image references'),
                  children: [
                    for (final media in _media)
                      ListTile(
                        contentPadding: EdgeInsets.zero,
                        title: Text(media['kind'].toString()),
                        subtitle: Text(media['media_ref'].toString()),
                        trailing: IconButton(
                          tooltip: 'Remove reference',
                          icon: const Icon(Icons.close_rounded),
                          onPressed: _saving
                              ? null
                              : () {
                                  setState(() {
                                    _media.remove(media);
                                    _dirty = true;
                                  });
                                },
                        ),
                      ),
                    TextButton.icon(
                      onPressed: _saving ? null : _mediaReference,
                      icon: const Icon(Icons.add_rounded),
                      label: const Text('Add existing reference'),
                    ),
                  ],
                ),
              ],
              if (kind == SettingsKind.branches) ...[
                _field(
                  'display_name',
                  'Branch name',
                  required: true,
                  maxLength: 160,
                ),
                _select(
                  'Brand',
                  _brand,
                  widget.brands,
                  'display_name',
                  (v) => _brand = v,
                  empty: 'No brand',
                ),
                _field('line1', 'Address', required: true),
                _field('line2', 'Address line 2'),
                _field('city', 'City', required: true),
                _field('region', 'Region', required: true),
                _field('postal_code', 'Postal code'),
                _field('country_code', 'Country code', required: true),
                LocationPickField(
                  latitude: _fields['latitude']!,
                  longitude: _fields['longitude']!,
                  enabled: !_saving,
                  onChanged: () => setState(() => _dirty = true),
                ),
                Offstage(
                  child: _field('timezone', 'Timezone', required: true),
                ),
              ],
              if (kind == SettingsKind.staff) ...[
                Row(
                  children: [
                    for (final email in [true, false])
                      Expanded(
                        child: Padding(
                          padding: const EdgeInsets.only(right: 8),
                          child: ChoiceChip(
                            label: Text(email ? 'Email' : 'Phone'),
                            selected: _email == email,
                            showCheckmark: false,
                            selectedColor: KarryamColors.mint.forSurface(
                              context,
                            ),
                            backgroundColor: Colors.white.forSurface(context),
                            side: BorderSide(
                              color:
                                  (_email == email
                                          ? KarryamColors.teal.withValues(
                                              alpha: .3,
                                            )
                                          : const Color(0xFFE1E9E4))
                                      .forOutline(context),
                            ),
                            labelStyle: TextStyle(
                              color:
                                  (_email == email
                                          ? KarryamColors.tealInk
                                          : KarryamColors.indigo)
                                      .forInk(context),
                            ),
                            onSelected: _saving
                                ? null
                                : (_) => setState(() {
                                    _email = email;
                                    _dirty = true;
                                  }),
                          ),
                        ),
                      ),
                  ],
                ),
                const SizedBox(height: 22),
                _field(
                  _email ? 'email' : 'phone',
                  _email ? 'Email address' : 'Phone number',
                  required: true,
                ),
                _select('Role', _role, widget.roles, 'name', (v) => _role = v),
                _select(
                  'Access scope',
                  _branch,
                  widget.branches,
                  'display_name',
                  (v) => _branch = v,
                  empty: 'Business-wide',
                ),
                const Text(
                  'Staff can sign in with this contact once access is added.',
                ),
              ],
              const SizedBox(height: 24),
            ],
          ),
        ),
      ),
    ),
  );
}
