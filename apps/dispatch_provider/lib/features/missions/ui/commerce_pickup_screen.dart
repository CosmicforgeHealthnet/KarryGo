import 'dart:async';
import 'commerce_recovery_screen.dart';
import 'dart:convert';
import '../../../core/app_mode.dart';
import 'package:flutter/material.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/async_content.dart';
import '../data/provider_operations_api.dart';

class CommercePickupScreen extends StatefulWidget {
  const CommercePickupScreen({
    super.key,
    required this.api,
    required this.tripId,
    required this.initial,
  });
  final ProviderOperationsApi api;
  final String tripId;
  final Map<String, dynamic> initial;
  @override
  State<CommercePickupScreen> createState() => _CommercePickupScreenState();
}

class _CommercePickupScreenState extends State<CommercePickupScreen> {
  late Map<String, dynamic> _mission;
  Map<String, dynamic>? _credential;
  final _confirmed = <String>{};
  Timer? _poll;
  Timer? _clock;
  bool _refreshing = false;
  bool _assignmentLost = false;
  bool _busy = false;
  String? _error;
  String? _notice;
  String? _selectedRef;
  List<Map<String, dynamic>> get _stops => (_mission['stops'] as List? ?? [])
      .whereType<Map>()
      .map((v) => Map<String, dynamic>.from(v))
      .toList();
  List<Map<String, dynamic>> get _parcels =>
      (_mission['parcels'] as List? ?? [])
          .whereType<Map>()
          .map((v) => Map<String, dynamic>.from(v))
          .toList();
  Map<String, dynamic>? get _next => _stops
      .where(
        (stop) =>
            !['PICKED_UP', 'SKIPPED', 'RETURNED'].contains(stop['status']),
      )
      .firstOrNull;
  String get _path =>
      '/api/v1/provider/commerce-missions/${Uri.encodeComponent(_mission['mission_ref'] as String)}';
  bool get _expired =>
      !(DateTime.tryParse(
            _credential?['expires_at']?.toString() ?? '',
          )?.isAfter(DateTime.now()) ??
          false);

  @override
  void initState() {
    super.initState();
    _mission = widget.initial;
    _poll = Timer.periodic(const Duration(seconds: 5), (_) {
      if (!_busy && !_assignmentLost) _refresh(silent: true);
    });
    _clock = Timer.periodic(const Duration(seconds: 1), (_) {
      if (mounted && _credential != null) setState(() {});
    });
  }

  @override
  void dispose() {
    _poll?.cancel();
    _clock?.cancel();
    super.dispose();
  }

  Future<void> _refresh({bool silent = false}) async {
    if (_refreshing) return;
    _refreshing = true;
    try {
      final result = await widget.api.read(
        '/api/v1/provider/commerce-missions/by-trip/${Uri.encodeComponent(widget.tripId)}',
      );
      if (!mounted) return;
      setState(() {
        _mission = result;
        if (!silent) _error = null;
      });
    } catch (error) {
      if (error is ApiException && error.statusCode == 404) {
        if (mounted) setState(() => _assignmentLost = true);
        return;
      }
      if (mounted)
        setState(
          () => _error = error is ApiException
              ? error.message
              : 'Delivery details could not be refreshed. Try again.',
        );
    } finally {
      if (mounted) {
        setState(() => _refreshing = false);
      } else {
        _refreshing = false;
      }
    }
  }

  Future<void> _arrive() async {
    final stop = _next;
    if (stop == null ||
        _busy ||
        _refreshing ||
        _mission['pickup_actions_allowed'] != true)
      return;
    setState(() {
      _busy = true;
      _error = null;
      _notice = null;
    });
    try {
      final result = await widget.api.command(
        '$_path/stops/${Uri.encodeComponent(stop['stop_ref'] as String)}/arrive',
        {'expected_version': _mission['version']},
        operationRef: true,
      );
      if (!mounted) return;
      setState(() => _credential = result);
      await _refresh(silent: true);
    } catch (error) {
      if (mounted)
        setState(
          () => _error = error is ApiException
              ? error.message
              : 'Arrival could not be confirmed. Refresh before retrying.',
        );
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _demoSenderConfirmation() async {
    if (_busy || _refreshing || _next == null) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await widget.api.command(
        '$_path/stops/${Uri.encodeComponent(_next!['stop_ref'] as String)}/mock-sender-confirmation',
        {'expected_version': _mission['version']},
        operationRef: true,
      );
      await _refresh(silent: true);
    } catch (_) {
      if (mounted)
        setState(
          () => _error = 'Demo confirmation could not be completed. Try again.',
        );
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _complete() async {
    final stop = _next;
    if (stop == null ||
        _busy ||
        _refreshing ||
        _mission['pickup_actions_allowed'] != true)
      return;
    final parcels = _parcels
        .where((p) => p['pickup_stop_ref'] == stop['stop_ref'])
        .toList();
    if (parcels.isEmpty ||
        parcels.any((p) => !_confirmed.contains(p['parcel_ref'])))
      return;
    final approved = await showModalBottomSheet<bool>(
      context: context,
      showDragHandle: true,
      useSafeArea: true,
      builder: (sheet) => Padding(
        padding: const EdgeInsets.fromLTRB(24, 0, 24, 24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              'Confirm collection',
              style: Theme.of(sheet).textTheme.titleLarge,
            ),
            const SizedBox(height: 12),
            const Text(
              'Only confirm once the sender has handed over every selected parcel.',
            ),
            const SizedBox(height: 24),
            FilledButton(
              onPressed: () => Navigator.pop(sheet, true),
              child: const Text('I have collected these parcels'),
            ),
          ],
        ),
      ),
    );
    if (approved != true || !mounted) return;
    setState(() {
      _busy = true;
      _error = null;
      _notice = null;
    });
    try {
      await widget.api.command(
        '$_path/stops/${Uri.encodeComponent(stop['stop_ref'] as String)}/complete',
        {
          'expected_version': _mission['version'],
          'manifest_ref': stop['manifest_ref'],
          'manifest_version': stop['manifest_version'],
          'parcel_refs': parcels.map((p) => p['parcel_ref']).toList(),
          'package_count': parcels.fold<int>(
            0,
            (sum, p) => sum + (p['package_count'] as num).toInt(),
          ),
        },
        operationRef: true,
      );
      if (!mounted) return;
      setState(() {
        _credential = null;
        _confirmed.clear();
        _notice = 'Pickup confirmed';
      });
      await _refresh(silent: true);
    } catch (error) {
      if (mounted)
        setState(
          () => _error = error is ApiException
              ? error.message
              : 'Collection could not be confirmed. Refresh this delivery before retrying.',
        );
      await _refresh(silent: true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_assignmentLost)
      return Scaffold(
        appBar: AppBar(title: const Text('Delivery updated')),
        body: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const ActionNotice(
                message:
                    'This delivery is no longer assigned to you. Return to Home for your current work.',
              ),
              const SizedBox(height: 24),
              FilledButton(
                onPressed: () =>
                    Navigator.of(context).popUntil((route) => route.isFirst),
                child: const Text('Back to Home'),
              ),
            ],
          ),
        ),
      );
    final c = context.palette;
    final next = _next;
    final parcels = _parcels
        .where((p) => p['pickup_stop_ref'] == next?['stop_ref'])
        .toList();
    final selectionScope = jsonEncode([
      next?['stop_ref'],
      next?['manifest_ref'],
      next?['manifest_version'],
      parcels.map((p) => [p['parcel_ref'], p['package_count']]).toList(),
    ]);
    if (_selectedRef != selectionScope) {
      _selectedRef = selectionScope;
      _confirmed.clear();
    }
    final verified = next?['handover_status'] == 'VERIFIED';
    final showRecovery =
        (_mission['return_parcel_refs'] as List? ?? []).isNotEmpty ||
        (_mission['returns'] as List? ?? []).isNotEmpty ||
        _mission['can_start_transfer'] == true ||
        _mission['transfer'] != null ||
        (_mission['exception_options'] as List? ?? []).isNotEmpty ||
        (_mission['exceptions'] as List? ?? []).isNotEmpty ||
        (_mission['evidence'] as List? ?? []).isNotEmpty;
    return Scaffold(
      appBar: AppBar(
        title: const Text('Pickup route'),
        bottom: showRecovery
            ? PreferredSize(
                preferredSize: const Size.fromHeight(52),
                child: Padding(
                  padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
                  child: SizedBox(
                    width: double.infinity,
                    child: TextButton.icon(
                      onPressed: _busy
                          ? null
                          : () async {
                              await Navigator.of(context).push(
                                MaterialPageRoute(
                                  builder: (_) => CommerceRecoveryScreen(
                                    api: widget.api,
                                    tripId: widget.tripId,
                                    initial: _mission,
                                  ),
                                ),
                              );
                              if (mounted) await _refresh();
                            },
                      icon: const Icon(Icons.swap_horiz_rounded),
                      label: const Text('Delivery options'),
                    ),
                  ),
                ),
              )
            : null,
        actions: [
          IconButton(
            tooltip: 'Refresh delivery',
            onPressed: _busy ? null : _refresh,
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: _refresh,
        child: ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: const EdgeInsets.all(20),
          children: [
            Text(
              'Every pickup, in order',
              style: Theme.of(context).textTheme.headlineSmall,
            ),
            const SizedBox(height: 8),
            const Text(
              'Confirm each collection before moving to the next location.',
            ),
            const SizedBox(height: 24),
            if (_error != null) ...[
              ActionNotice(message: _error!, isError: true, onRetry: _refresh),
              const SizedBox(height: 16),
            ],
            if (_notice != null) ...[
              ActionNotice(message: _notice!, success: true),
              const SizedBox(height: 16),
            ],
            for (final stop in _stops)
              Card(
                child: ListTile(
                  leading: Icon(
                    stop['status'] == 'PICKED_UP'
                        ? Icons.check_circle_rounded
                        : stop['stop_ref'] == next?['stop_ref']
                        ? Icons.storefront_outlined
                        : Icons.location_on_outlined,
                    color: stop['status'] == 'PICKED_UP'
                        ? c.success
                        : c.textMuted,
                  ),
                  title: Text(
                    (stop['location'] as Map?)?['address']?.toString() ??
                        'Pickup ${stop['sequence']}',
                  ),
                  subtitle: Text(
                    stop['status'] == 'PICKED_UP'
                        ? 'Collected'
                        : stop['stop_ref'] == next?['stop_ref']
                        ? 'Current pickup'
                        : 'Upcoming',
                  ),
                  trailing: Text('${stop['sequence']}'),
                ),
              ),
            const SizedBox(height: 24),
            if (_mission['pickup_actions_allowed'] != true) ...[
              const ActionNotice(
                message:
                    'Pickup actions are currently unavailable. Refresh for the latest delivery status.',
              ),
            ] else if (_stops.isEmpty) ...[
              ActionNotice(
                message:
                    'No pickup stops are available. Refresh this delivery.',
                onRetry: _refresh,
              ),
            ] else if (next == null) ...[
              ActionNotice(
                message: widget.tripId == 'demo-commerce-trip'
                    ? 'Demo complete. Both pickups are confirmed.'
                    : 'All pickups are confirmed. Continue to the recipient.',
                success: true,
              ),
              const SizedBox(height: 20),
              FilledButton.icon(
                onPressed: () => Navigator.pop(context),
                icon: const Icon(Icons.arrow_forward_rounded),
                label: Text(
                  widget.tripId == 'demo-commerce-trip'
                      ? 'Back to preferences'
                      : 'Continue to delivery',
                ),
              ),
            ] else ...[
              if (_credential != null &&
                  _credential!['stop_ref'] == next['stop_ref'] &&
                  !verified) ...[
                Card(
                  child: Padding(
                    padding: const EdgeInsets.all(24),
                    child: Column(
                      children: [
                        Text(
                          _expired
                              ? 'Pickup code expired'
                              : 'Show this code to the sender',
                          style: Theme.of(context).textTheme.titleMedium,
                          textAlign: TextAlign.center,
                        ),
                        const SizedBox(height: 16),
                        if (!_expired)
                          SelectableText(
                            _credential!['code']?.toString() ?? '',
                            style: Theme.of(context).textTheme.headlineLarge
                                ?.copyWith(letterSpacing: 6),
                          ),
                        const SizedBox(height: 16),
                        Text(
                          _expired
                              ? 'Request a new code when you are ready.'
                              : 'The sender verifies this code in their app. This screen updates automatically.',
                          textAlign: TextAlign.center,
                        ),
                      ],
                    ),
                  ),
                ),
                const SizedBox(height: 16),
              ],
              if (!verified && (_credential == null || _expired))
                FilledButton.icon(
                  onPressed: _busy || _refreshing ? null : _arrive,
                  icon: _busy
                      ? const SizedBox(
                          width: 18,
                          height: 18,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : const Icon(Icons.storefront_outlined),
                  label: Text(
                    next['status'] == 'ARRIVED'
                        ? 'Request pickup code'
                        : 'I am at this pickup',
                  ),
                ),
              if (AppModeConfig.instance.isMock &&
                  _credential != null &&
                  !verified &&
                  !_expired)
                OutlinedButton.icon(
                  onPressed: _busy ? null : _demoSenderConfirmation,
                  icon: const Icon(Icons.science_outlined),
                  label: const Text('Simulate sender confirmation'),
                ),
              if (verified) ...[
                const ActionNotice(
                  message:
                      'Sender confirmed. Check each parcel before collecting.',
                  success: true,
                ),
                const SizedBox(height: 16),
                for (final parcel in parcels)
                  Card(
                    child: CheckboxListTile(
                      value: _confirmed.contains(parcel['parcel_ref']),
                      onChanged: _busy
                          ? null
                          : (checked) => setState(() {
                              checked == true
                                  ? _confirmed.add(
                                      parcel['parcel_ref'] as String,
                                    )
                                  : _confirmed.remove(parcel['parcel_ref']);
                            }),
                      title: Text(
                        '${parcel['package_count']} package${parcel['package_count'] == 1 ? '' : 's'}',
                      ),
                      subtitle: Text(parcel['parcel_ref'] as String),
                    ),
                  ),
                const SizedBox(height: 16),
                FilledButton.icon(
                  onPressed:
                      !_busy &&
                          !_refreshing &&
                          parcels.isNotEmpty &&
                          parcels.every(
                            (p) => _confirmed.contains(p['parcel_ref']),
                          )
                      ? _complete
                      : null,
                  icon: _busy
                      ? const SizedBox(
                          width: 18,
                          height: 18,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : const Icon(Icons.inventory_2_outlined),
                  label: const Text('Confirm collection'),
                ),
              ],
            ],
          ],
        ),
      ),
    );
  }
}
