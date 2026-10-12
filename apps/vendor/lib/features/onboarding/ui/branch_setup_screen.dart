import 'dart:async';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/widgets/vendor_widgets.dart';
import '../state/business_onboarding_controller.dart';
import 'setup_design.dart';

class BranchSetupScreen extends StatefulWidget {
  const BranchSetupScreen({
    super.key,
    required this.controller,
    required this.runtime,
    required this.branch,
    required this.onSettings,
  });
  final BusinessOnboardingController controller;
  final VendorRuntime runtime;
  final JsonObject branch;
  final VoidCallback onSettings;
  @override
  State<BranchSetupScreen> createState() => _BranchSetupScreenState();
}

class _BranchSetupScreenState extends State<BranchSetupScreen> {
  final _form = GlobalKey<FormState>();
  final _lat = TextEditingController(),
      _lng = TextEditingController(),
      _radius = TextEditingController();
  JsonObject _branch = {}, _area = {}, _settings = {};
  List<JsonObject> _hours = [], _capabilities = [];
  bool _busy = true, _loaded = false, _accepting = false, _pickup = false;
  String? _error;
  static const _days = [
    'Monday',
    'Tuesday',
    'Wednesday',
    'Thursday',
    'Friday',
    'Saturday',
    'Sunday',
  ];
  BusinessOnboardingController get c => widget.controller;
  Map<String, String> get params => {
    'businessId': c.businessId,
    'branchId': widget.branch['id'] as String,
  };
  static const base =
      '/api/v1/vendor/businesses/{businessId}/branches/{branchId}';
  @override
  void initState() {
    super.initState();
    unawaited(_load());
  }

  @override
  void dispose() {
    _lat.dispose();
    _lng.dispose();
    _radius.dispose();
    super.dispose();
  }

  Future<JsonObject> _optional(String suffix) async {
    try {
      final r = await c.request('GET', '$base/$suffix', params: params);
      return r.data == null ? {} : r.object;
    } catch (e) {
      if (e.toString().contains('(404)')) return {};
      rethrow;
    }
  }

  Future<void> _load() async {
    if (mounted) setState(() => _busy = true);
    try {
      final branch = (await c.request('GET', base, params: params)).object;
      final hours = BusinessOnboardingController.objects(
        (await c.request('GET', '$base/hours', params: params)).data,
      );
      final caps = BusinessOnboardingController.objects(
        (await c.request('GET', '$base/capabilities', params: params)).data,
      );
      final area = await _optional('service-area');
      final settings = await _optional('operations-settings');
      if (!mounted) return;
      setState(() {
        _branch = branch;
        _hours = hours;
        _capabilities = caps;
        _area = area;
        _settings = settings;
        _loaded = true;
        _error = null;
        _accepting = settings['accepting_new_orders'] == true;
        _pickup = settings['pickup_enabled'] == true;
        _lat.text = (area['center_latitude'] ?? branch['latitude']).toString();
        _lng.text = (area['center_longitude'] ?? branch['longitude'])
            .toString();
        _radius.text =
            area['radius_meters']?.toString() ??
            (widget.runtime.mode == VendorDataMode.mock ? '5000' : '');
      });
    } catch (_) {
      if (mounted) {
        setState(
          () => _error =
              'Branch settings could not be loaded. Check your access and try again.',
        );
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _save(
    String suffix,
    JsonObject body, {
    Map<String, String>? extra,
  }) async {
    if (_busy) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    final success = await c.save(
      'PUT',
      '$base/$suffix',
      body,
      params: {...params, ...?extra},
    );
    if (!mounted) return;
    if (success) {
      await _load();
    } else {
      setState(() {
        _busy = false;
        _error = c.error;
      });
    }
  }

  Future<void> _window({JsonObject? existing, int? index}) async {
    int day = (existing?['weekday'] as num?)?.toInt() ?? 1;
    bool closed = existing?['is_closed'] == true;
    final opens = TextEditingController(
          text: existing?['opens_at'] as String? ?? '',
        ),
        closes = TextEditingController(
          text: existing?['closes_at'] as String? ?? '',
        );
    final form = GlobalKey<FormState>();
    final result = await showDialog<JsonObject>(
      context: context,
      builder: (context) => StatefulBuilder(
        builder: (context, setDialog) => AlertDialog(
          title: const Text('Operating hours'),
          content: SingleChildScrollView(
            child: Form(
              key: form,
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  DropdownButtonFormField<int>(
                    initialValue: day,
                    items: [
                      for (int i = 1; i <= 7; i++)
                        DropdownMenuItem(value: i, child: Text(_days[i - 1])),
                    ],
                    onChanged: (v) => setDialog(() => day = v!),
                  ),
                  SwitchListTile(
                    contentPadding: EdgeInsets.zero,
                    title: const Text('Closed all day'),
                    value: closed,
                    onChanged: (v) => setDialog(() => closed = v),
                  ),
                  if (!closed) ...[
                    TextFormField(
                      controller: opens,
                      decoration: const InputDecoration(
                        labelText: 'Opens at (HH:mm)',
                      ),
                      validator: (v) => _time(v) ? null : 'Use 24-hour time',
                    ),
                    TextFormField(
                      controller: closes,
                      decoration: const InputDecoration(
                        labelText: 'Closes at (HH:mm)',
                      ),
                      validator: (v) => _time(v) && v!.compareTo(opens.text) > 0
                          ? null
                          : 'Close after opening',
                    ),
                  ],
                ],
              ),
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context),
              child: const Text('Cancel'),
            ),
            FilledButton(
              onPressed: () {
                if (form.currentState!.validate()) {
                  Navigator.pop(context, {
                    'weekday': day,
                    'is_closed': closed,
                    if (!closed) 'opens_at': opens.text,
                    if (!closed) 'closes_at': closes.text,
                  });
                }
              },
              child: const Text('Apply'),
            ),
          ],
        ),
      ),
    );
    opens.dispose();
    closes.dispose();
    if (result == null || !mounted) return;
    setState(() {
      if (index == null) {
        _hours.add(result);
      } else {
        _hours[index] = result;
      }
    });
  }

  bool _time(String? v) =>
      v != null && RegExp(r'^(?:[01][0-9]|2[0-3]):[0-5][0-9]$').hasMatch(v);
  bool _validHours() {
    final sorted = [..._hours]
      ..sort((a, b) {
        final compare = (a['weekday'] as int).compareTo(b['weekday'] as int);
        return compare == 0
            ? (a['opens_at'] ?? '').toString().compareTo(
                (b['opens_at'] ?? '').toString(),
              )
            : compare;
      });
    for (int i = 1; i < sorted.length; i++) {
      final prev = sorted[i - 1], next = sorted[i];
      if (prev['weekday'] == next['weekday'] &&
          (prev['is_closed'] == true ||
              next['is_closed'] == true ||
              (prev['closes_at'] as String).compareTo(
                    next['opens_at'] as String,
                  ) >
                  0)) {
        return false;
      }
    }
    return true;
  }

  @override
  Widget build(BuildContext context) => VendorScreenFrame(
    modeLabel: widget.runtime.mode.name,
    onSettings: widget.onSettings,
    onBack: _busy ? null : () => Navigator.pop(context),
    leading: const Text(
      'BRANCH SETUP',
      style: TextStyle(
        fontWeight: FontWeight.w700,
        fontSize: 11,
        letterSpacing: 1.5,
      ),
    ),
    body: SetupFormSurface(
      child: ListView(
        padding: const EdgeInsets.all(24),
        children: [
          SetupChapterHeader(
            kind: 'operations',
            title: widget.branch['display_name'].toString(),
          ),
          const SizedBox(height: 28),
          if (_error != null) ...[
            InlineNotice(message: _error!, error: true),
            const SizedBox(height: 16),
            TextButton(
              onPressed: _busy ? null : _load,
              child: const Text('Refresh settings'),
            ),
          ],
          if (_busy)
            Center(
              child: CircularProgressIndicator(
                color: KarryamColors.tealInk.forInk(context),
              ),
            ),
          if (_loaded) ...[
            const Text(
              'Business types at this branch',
              style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 12),
            for (final cap in c.capabilities)
              ListTile(
                contentPadding: EdgeInsets.zero,
                title: Text(
                  (cap['capability_type'] as String).replaceAll('_', ' '),
                ),
                subtitle: Text(
                  _capabilities.any(
                        (v) => v['vendor_capability_id'] == cap['id'],
                      )
                      ? 'Linked to this branch'
                      : 'Available to link',
                ),
                trailing:
                    _capabilities.any(
                      (v) => v['vendor_capability_id'] == cap['id'],
                    )
                    ? Icon(
                        Icons.check_circle,
                        color: KarryamColors.tealInk.forInk(context),
                      )
                    : IconButton(
                        tooltip: 'Link business type',
                        onPressed: _busy
                            ? null
                            : () => _save(
                                'capabilities/{capabilityId}',
                                {'expected_version': 0},
                                extra: {'capabilityId': cap['id']},
                              ),
                        icon: const Icon(Icons.add_circle_outline),
                      ),
              ),
            const SizedBox(height: 28),
            const Text(
              'Operating hours',
              style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 12),
            for (int i = 0; i < _hours.length; i++)
              ListTile(
                contentPadding: EdgeInsets.zero,
                title: Text(_days[(_hours[i]['weekday'] as int) - 1]),
                subtitle: Text(
                  _hours[i]['is_closed'] == true
                      ? 'Closed'
                      : '${_hours[i]['opens_at']} - ${_hours[i]['closes_at']}',
                ),
                onTap: _busy
                    ? null
                    : () => _window(existing: _hours[i], index: i),
                trailing: IconButton(
                  tooltip: 'Remove hours',
                  onPressed: _busy
                      ? null
                      : () => setState(() => _hours.removeAt(i)),
                  icon: const Icon(Icons.close),
                ),
              ),
            OutlinedButton.icon(
              onPressed: _busy ? null : () => _window(),
              icon: const Icon(Icons.add),
              label: const Text('Add operating window'),
            ),
            const SizedBox(height: 12),
            PrimaryAction(
              label: 'Save hours',
              busy: _busy,
              onPressed: _busy
                  ? null
                  : () {
                      if (!_validHours()) {
                        setState(
                          () => _error = 'Operating windows must not overlap.',
                        );
                        return;
                      }
                      _save('hours', {
                        'hours': [
                          for (final w in _hours)
                            {
                              'weekday': w['weekday'],
                              'is_closed': w['is_closed'],
                              if (w['is_closed'] != true)
                                'opens_at': w['opens_at'],
                              if (w['is_closed'] != true)
                                'closes_at': w['closes_at'],
                            },
                        ],
                        'expected_version': _branch['version'],
                      });
                    },
            ),
            const SizedBox(height: 32),
            const Text(
              'Service area',
              style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 12),
            Form(
              key: _form,
              child: Column(
                children: [
                  _coordinate(_lat, 'Centre latitude', -90, 90),
                  const SizedBox(height: 16),
                  _coordinate(_lng, 'Centre longitude', -180, 180),
                  const SizedBox(height: 16),
                  TextFormField(
                    controller: _radius,
                    enabled: !_busy,
                    keyboardType: TextInputType.number,
                    decoration: const InputDecoration(
                      labelText: 'Radius in metres',
                    ),
                    validator: (v) {
                      final n = int.tryParse(v ?? '');
                      return n != null && n >= 1 && n <= 200000
                          ? null
                          : 'Use 1 to 200000 metres';
                    },
                  ),
                ],
              ),
            ),
            const SizedBox(height: 16),
            PrimaryAction(
              label: 'Save service area',
              busy: _busy,
              onPressed: _busy
                  ? null
                  : () {
                      if (_form.currentState!.validate()) {
                        _save('service-area', {
                          'mode': 'RADIUS',
                          'center_latitude': num.parse(_lat.text),
                          'center_longitude': num.parse(_lng.text),
                          'radius_meters': int.parse(_radius.text),
                          'expected_version': _area['version'] ?? 0,
                        });
                      }
                    },
            ),
            const SizedBox(height: 32),
            const Text(
              'Order settings',
              style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700),
            ),
            SwitchListTile.adaptive(
              contentPadding: EdgeInsets.zero,
              title: const Text('Accept new orders'),
              value: _accepting,
              onChanged: _busy ? null : (v) => setState(() => _accepting = v),
            ),
            SwitchListTile.adaptive(
              contentPadding: EdgeInsets.zero,
              title: const Text('Allow pickup'),
              value: _pickup,
              onChanged: _busy ? null : (v) => setState(() => _pickup = v),
            ),
            const SizedBox(height: 16),
            PrimaryAction(
              label: 'Save order settings',
              busy: _busy,
              onPressed: _busy
                  ? null
                  : () => _save('operations-settings', {
                      'schema_version': 1,
                      'accepting_new_orders': _accepting,
                      'pickup_enabled': _pickup,
                      'expected_version': _settings['version'] ?? 0,
                    }),
            ),
          ],
        ],
      ),
    ),
  );
  Widget _coordinate(
    TextEditingController field,
    String label,
    num min,
    num max,
  ) => TextFormField(
    controller: field,
    enabled: !_busy,
    keyboardType: const TextInputType.numberWithOptions(
      signed: true,
      decimal: true,
    ),
    decoration: InputDecoration(labelText: label),
    validator: (v) {
      final n = num.tryParse(v ?? '');
      return n != null && n >= min && n <= max
          ? null
          : 'Use a value from $min to $max';
    },
  );
}
