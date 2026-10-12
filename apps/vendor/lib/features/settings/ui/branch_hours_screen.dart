import '../../../core/ui/premium_loading.dart';
import 'dart:async';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../state/settings_controller.dart';
import 'settings_design.dart';
import 'branch_settings_screen.dart';

class BranchHoursScreen extends StatefulWidget {
  const BranchHoursScreen({
    super.key,
    required this.runtime,
    required this.branch,
  });
  final VendorRuntime runtime;
  final JsonObject branch;
  @override
  State<BranchHoursScreen> createState() => _BranchHoursScreenState();
}

class _BranchHoursScreenState extends State<BranchHoursScreen> {
  late final c = SettingsController(
    widget.runtime.backend,
    SettingsKind.branches,
  );
  final List<JsonObject> _windows = [];
  int? _version;
  bool _loading = false, _dirty = false, _saved = false, _ready = false;
  String? _error;
  static const days = [
    'Monday',
    'Tuesday',
    'Wednesday',
    'Thursday',
    'Friday',
    'Saturday',
    'Sunday',
  ];
  Map<String, String> get params => c.params(widget.branch);
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) unawaited(_load());
    });
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    if (_dirty &&
        !await confirmSettings(
          context,
          'Reload hours',
          'Replace unsaved hours with the latest server settings?',
        )) {
      return;
    }
    if (!mounted) return;
    setState(() {
      _loading = true;
      _ready = false;
      _error = null;
    });
    await c.load();
    try {
      final response = await Future.wait([
        c.read(branchSettingsBase, parameters: params),
        c.read('$branchSettingsBase/hours', parameters: params),
      ]);
      final windows = SettingsController.objects(response[1].data);
      if (mounted) {
        setState(() {
          _version = response[0].object['version'] as int;
          _ready = true;
          _windows.clear();
          _windows.addAll(windows);
          _dirty = false;
        });
      }
    } catch (e) {
      if (mounted) setState(() => _error = SettingsController.message(e));
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  String _clock(TimeOfDay time) =>
      '${time.hour.toString().padLeft(2, '0')}:${time.minute.toString().padLeft(2, '0')}';
  TimeOfDay _time(Object? value, TimeOfDay fallback) {
    final parts = value?.toString().split(':');
    return parts == null || parts.length != 2
        ? fallback
        : TimeOfDay(
            hour: int.parse(parts.first),
            minute: int.parse(parts.last),
          );
  }

  Future<void> _window(int day, {JsonObject? record}) async {
    var closed = record?['is_closed'] == true;
    var opens = _time(record?['opens_at'], const TimeOfDay(hour: 9, minute: 0)),
        closes = _time(
          record?['closes_at'],
          const TimeOfDay(hour: 18, minute: 0),
        );
    String? error;
    final result = await settingsSheet<JsonObject>(
      context,
      StatefulBuilder(
        builder: (context, setSheet) => Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            SettingsHeading(days[day - 1]),
            SwitchListTile.adaptive(
              contentPadding: EdgeInsets.zero,
              title: const Text('Closed all day'),
              value: closed,
              activeTrackColor: KarryamColors.teal.forInk(context),
              onChanged: (v) => setSheet(() => closed = v),
            ),
            if (!closed) ...[
              const SizedBox(height: 12),
              SettingsLink(
                'Opens at',
                Icons.wb_sunny_outlined,
                subtitle: _clock(opens),
                onTap: () async {
                  final time = await showTimePicker(
                    context: context,
                    initialTime: opens,
                  );
                  if (time != null) setSheet(() => opens = time);
                },
              ),
              SettingsLink(
                'Closes at',
                Icons.nightlight_outlined,
                subtitle: _clock(closes),
                onTap: () async {
                  final time = await showTimePicker(
                    context: context,
                    initialTime: closes,
                  );
                  if (time != null) setSheet(() => closes = time);
                },
              ),
            ],
            if (error != null) SettingsNote(error!),
            const SizedBox(height: 18),
            SettingsAction(
              'Save interval',
              icon: Icons.check_rounded,
              onPressed: () {
                final o = _clock(opens), e = _clock(closes);
                if (!closed && o.compareTo(e) >= 0) {
                  setSheet(
                    () => error =
                        'Closing time must be after opening time, within the same day.',
                  );
                  return;
                }
                if (!closed &&
                    _windows.any(
                      (w) =>
                          !identical(w, record) &&
                          w['weekday'] == day &&
                          w['is_closed'] != true &&
                          o.compareTo(w['closes_at'] as String) < 0 &&
                          e.compareTo(w['opens_at'] as String) > 0,
                    )) {
                  setSheet(
                    () => error =
                        'This interval overlaps another opening interval.',
                  );
                  return;
                }
                Navigator.pop(context, {
                  'weekday': day,
                  'is_closed': closed,
                  if (!closed) 'opens_at': o,
                  if (!closed) 'closes_at': e,
                });
              },
            ),
          ],
        ),
      ),
    );
    if (result != null && mounted) {
      setState(() {
        if (result['is_closed'] == true) {
          _windows.removeWhere((w) => w['weekday'] == day);
        } else {
          _windows.removeWhere(
            (w) => w['weekday'] == day && w['is_closed'] == true,
          );
        }
        if (record != null) _windows.remove(record);
        _windows.add(result);
        _windows.sort((a, b) {
          final d = (a['weekday'] as int).compareTo(b['weekday'] as int);
          return d != 0
              ? d
              : (a['opens_at']?.toString() ?? '').compareTo(
                  b['opens_at']?.toString() ?? '',
                );
        });
        _dirty = true;
      });
    }
  }

  Future<void> _save() async {
    if (_version == null ||
        !_ready ||
        !c.allowed('PUT', '$branchSettingsBase/hours')) {
      return;
    }
    final body = {
      'expected_version': _version,
      'hours': [
        for (final w in _windows)
          {
            'weekday': w['weekday'],
            'is_closed': w['is_closed'],
            if (w['is_closed'] != true) 'opens_at': w['opens_at'],
            if (w['is_closed'] != true) 'closes_at': w['closes_at'],
          },
      ],
    };
    final saved = await c.mutate(
      'PUT',
      '$branchSettingsBase/hours',
      body,
      parameters: params,
    );
    if (!mounted) return;
    setState(() {
      _saved = saved;
      _error = saved ? null : c.feedback ?? c.error;
    });
    await showOrderFeedback(
      context,
      title: saved ? 'Operating hours saved' : 'Hours not saved',
      message: saved
          ? 'Your branch hours are updated.'
          : _error ?? 'Try again.',
      success: saved,
    );
    if (saved && mounted) Navigator.pop(context, true);
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => PopScope(
      canPop: !c.busy && (!_dirty || _saved),
      onPopInvokedWithResult: (didPop, result) async {
        if (!didPop &&
            !c.busy &&
            await confirmSettings(
              context,
              'Discard changes',
              'Your unsaved opening hours will be lost.',
            ) &&
            context.mounted) {
          setState(() => _dirty = false);
          Navigator.pop(context);
        }
      },
      child: Scaffold(
        appBar: AppBar(
          title: const Text('Operating hours'),
          backgroundColor: KarryamColors.canvas.forSurface(context),
        ),
        bottomNavigationBar: SafeArea(
          child: Padding(
            padding: const EdgeInsets.fromLTRB(24, 12, 24, 16),
            child: SettingsAction(
              'Save hours',
              icon: Icons.check_rounded,
              busy: c.busy,
              onPressed:
                  _dirty &&
                      !_loading &&
                      _version != null &&
                      _ready &&
                      c.allowed('PUT', '$branchSettingsBase/hours')
                  ? _save
                  : null,
            ),
          ),
        ),
        body: ListView(
          padding: const EdgeInsets.all(24),
          children: [
            SettingsHeading(
              widget.branch['display_name'].toString(),
              subtitle: 'Times follow the branch timezone.',
            ),
            if (_loading) const PremiumSkeleton(rows: 2),
            if (_error != null) ...[
              SettingsNote(_error!),
              SettingsAction(
                'Reload hours',
                secondary: true,
                icon: Icons.refresh_rounded,
                onPressed: _load,
              ),
              const SizedBox(height: 18),
            ],
            for (var day = 1; day <= 7; day++)
              Padding(
                padding: const EdgeInsets.only(bottom: 14),
                child: SettingsCard(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          Expanded(
                            child: Text(
                              days[day - 1],
                              style: const TextStyle(
                                fontSize: 18,
                                fontWeight: FontWeight.w700,
                              ),
                            ),
                          ),
                          IconButton(
                            tooltip: 'Add ${days[day - 1]} interval',
                            onPressed: !_ready || _loading || c.busy || c.stale
                                ? null
                                : () => _window(day),
                            icon: Icon(
                              Icons.add_circle_outline_rounded,
                              color: KarryamColors.tealInk.forInk(context),
                            ),
                          ),
                        ],
                      ),
                      if (!_windows.any((w) => w['weekday'] == day))
                        Text(
                          'Not configured',
                          style: TextStyle(
                            color: Color(0xFF71818A).forInk(context),
                            fontSize: 12,
                          ),
                        ),
                      for (final window in _windows.where(
                        (w) => w['weekday'] == day,
                      ))
                        ListTile(
                          contentPadding: EdgeInsets.zero,
                          leading: Icon(
                            window['is_closed'] == true
                                ? Icons.do_not_disturb_on_outlined
                                : Icons.schedule_rounded,
                            color:
                                (window['is_closed'] == true
                                        ? const Color(0xFF986013)
                                        : KarryamColors.tealInk)
                                    .forInk(context),
                          ),
                          title: Text(
                            window['is_closed'] == true
                                ? 'Closed'
                                : '${window['opens_at']} - ${window['closes_at']}',
                          ),
                          onTap: !_ready || _loading || c.busy || c.stale
                              ? null
                              : () => _window(day, record: window),
                          trailing: IconButton(
                            tooltip: 'Remove interval',
                            icon: const Icon(Icons.close_rounded, size: 20),
                            onPressed: !_ready || _loading || c.busy || c.stale
                                ? null
                                : () => setState(() {
                                    _windows.remove(window);
                                    _dirty = true;
                                  }),
                          ),
                        ),
                    ],
                  ),
                ),
              ),
          ],
        ),
      ),
    ),
  );
}
