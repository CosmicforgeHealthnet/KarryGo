import 'mission_action_panel.dart';
import '../../../shared/widgets/journey_icon.dart';
import 'dart:async';
import 'dart:convert';
import 'mission_photo_field.dart';
import 'commerce_exception_screen.dart';
import 'package:flutter/material.dart';
import 'package:geolocator/geolocator.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../core/app_mode.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/async_content.dart';
import '../data/provider_operations_api.dart';

class CommerceRecoveryScreen extends StatefulWidget {
  const CommerceRecoveryScreen({
    super.key,
    required this.api,
    required this.tripId,
    required this.initial,
  });
  final ProviderOperationsApi api;
  final String tripId;
  final Map<String, dynamic> initial;
  @override
  State<CommerceRecoveryScreen> createState() => _CommerceRecoveryScreenState();
}

class _CommerceRecoveryScreenState extends State<CommerceRecoveryScreen> {
  late Map<String, dynamic> _mission;
  Map<String, dynamic>? _credential;
  Timer? _poll;
  String? _busy, _error, _notice;
  bool _refreshing = false;
  bool _transferConfirmed = false;
  bool _accessLost = false;
  final _code = TextEditingController();
  final _transferPhoto = MissionPhotoDraft();
  final _returnPhotos = <String, MissionPhotoDraft>{};
  final _timestamps = <String, String>{};
  String get _path =>
      '/api/v1/provider/commerce-missions/${Uri.encodeComponent(_mission['mission_ref'] as String)}';
  List<Map<String, dynamic>> get _returns =>
      (_mission['returns'] as List? ?? [])
          .whereType<Map>()
          .map((v) => Map<String, dynamic>.from(v))
          .toList();
  Map<String, dynamic> get _transfer =>
      Map<String, dynamic>.from(_mission['transfer'] as Map? ?? {});
  List<String> refs(dynamic value) =>
      (value as List? ?? []).map((v) => v.toString()).toList();
  @override
  void initState() {
    super.initState();
    _mission = widget.initial;
    _poll = Timer.periodic(const Duration(seconds: 5), (_) {
      if (_busy == null && !_accessLost) _refresh(silent: true);
    });
  }

  @override
  void dispose() {
    _poll?.cancel();
    _code.dispose();
    super.dispose();
  }

  Future<void> _refresh({bool silent = false}) async {
    if (_refreshing || _accessLost) return;
    _refreshing = true;
    try {
      final data = await widget.api.read(
        '/api/v1/provider/commerce-missions/by-trip/${Uri.encodeComponent(widget.tripId)}',
      );
      if (!mounted) return;
      setState(() {
        if (_mission['version'] != data['version']) _transferConfirmed = false;
        final transfer = data['transfer'] as Map?;
        if (jsonEncode([
              _transfer['transfer_ref'],
              _transfer['parcel_refs'],
              _transfer['package_count'],
            ]) !=
            jsonEncode([
              transfer?['transfer_ref'],
              transfer?['parcel_refs'],
              transfer?['package_count'],
            ]))
          _transferPhoto.clear();
        if (_credential?['transfer_ref'] != transfer?['transfer_ref'] ||
            transfer?['state'] != 'CHALLENGE_ISSUED')
          _credential = null;
        _mission = data;
        if (!silent) _error = null;
      });
    } on ApiException catch (error) {
      if (mounted)
        setState(() {
          if (error.statusCode == 404)
            _accessLost = true;
          else
            _error = error.message;
        });
    } catch (_) {
      if (mounted)
        setState(() => _error = 'Could not refresh this delivery. Try again.');
    } finally {
      if (mounted) setState(() => _refreshing = false);
    }
  }

  Future<void> _run(
    String label,
    Future<void> Function() operation, {
    String? action,
  }) async {
    if (_busy != null || _refreshing) return;
    setState(() {
      _busy = action ?? label;
      _error = null;
      _notice = null;
    });
    try {
      await operation();
      if (mounted) {
        setState(() => _notice = label);
        _timestamps.clear();
        await _refresh(silent: true);
      }
    } catch (error) {
      if (mounted) {
        setState(
          () => _error = error is ApiException
              ? error.message
              : 'The result could not be confirmed. Refresh before retrying.',
        );
        await _refresh(silent: true);
      }
    } finally {
      if (mounted) setState(() => _busy = null);
    }
  }

  Map<String, dynamic> _evidence(String operation) => {
    'captured_at': _timestamps.putIfAbsent(
      operation,
      () => DateTime.now().toUtc().toIso8601String(),
    ),
  };
  Future<Map<String, double>> _location() async {
    if (AppModeConfig.instance.isMock)
      return {'latitude': 6.45, 'longitude': 3.4};
    if (!await Geolocator.isLocationServiceEnabled())
      throw const ApiException(
        code: ApiErrorCode.validationFailed,
        message: 'Enable location services to confirm this handover.',
      );
    var permission = await Geolocator.checkPermission();
    if (permission == LocationPermission.denied)
      permission = await Geolocator.requestPermission();
    if (permission == LocationPermission.denied ||
        permission == LocationPermission.deniedForever)
      throw const ApiException(
        code: ApiErrorCode.forbidden,
        message:
            'Location permission is needed to confirm that both riders are at the handover location.',
      );
    final fix = await Geolocator.getCurrentPosition(
      locationSettings: const LocationSettings(
        accuracy: LocationAccuracy.high,
        timeLimit: Duration(seconds: 20),
      ),
    );
    return {'latitude': fix.latitude, 'longitude': fix.longitude};
  }

  Future<bool> _confirm(String title, String message, String button) async =>
      await showModalBottomSheet<bool>(
        context: context,
        showDragHandle: true,
        useSafeArea: true,
        builder: (sheet) => Padding(
          padding: const EdgeInsets.fromLTRB(24, 0, 24, 24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(title, style: Theme.of(sheet).textTheme.titleLarge),
              const SizedBox(height: 12),
              Text(message),
              const SizedBox(height: 24),
              FilledButton(
                onPressed: () => Navigator.pop(sheet, true),
                child: Text(button),
              ),
            ],
          ),
        ),
      ) ??
      false;
  Future<void> _report({bool evidenceOnly = false}) async {
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => CommerceExceptionScreen(
          api: widget.api,
          mission: _mission,
          evidenceOnly: evidenceOnly,
        ),
      ),
    );
    if (mounted) await _refresh();
  }

  Future<void> _startReturn() async {
    final selected = await Navigator.of(context).push<Map<String, dynamic>>(
      MaterialPageRoute(
        builder: (_) =>
            _ReturnForm(parcels: refs(_mission['return_parcel_refs'])),
      ),
    );
    if (selected == null || !mounted) return;
    final description = selected.remove('description');
    await _run('Return started', () async {
      await widget.api.command('$_path/returns', {
        ...selected,
        'expected_version': _mission['version'],
        'evidence': {..._evidence('start-return'), 'description': description},
      }, operationRef: true);
    }, action: 'Arrange a return');
  }

  Future<void> _completeReturn(Map<String, dynamic> item) async {
    final isVendor = item['disposition'] == 'RETURN_TO_VENDOR';
    if (!await _confirm(
          isVendor ? 'Confirm return handover' : 'Confirm return handling',
          isVendor
              ? 'Confirm only after every parcel in this return has been handed back to the vendor.'
              : 'Confirm only after every parcel has been handled under the recorded return policy.',
          'Confirm completed handling',
        ) ||
        !mounted)
      return;
    await _run('Return completed', () async {
      await widget.api.command(
        '$_path/returns/${Uri.encodeComponent(item['return_id'] as String)}/complete',
        {
          'return_id': item['return_id'],
          'expected_version': _mission['version'],
          'parcel_refs': item['parcel_refs'],
          'evidence': await _returnPhotos
              .putIfAbsent(
                item['return_id'] as String,
                () => MissionPhotoDraft(),
              )
              .attach(
                widget.api,
                '$_path/evidence-content',
                refs(item['parcel_refs']),
                _evidence('complete:${item['return_id']}'),
              ),
        },
        operationRef: true,
      );
      _returnPhotos.remove(item['return_id']);
    }, action: 'return:${item['return_id']}');
  }

  List<String> _returnLocations(Map<String, dynamic> item) {
    final parcelRefs = refs(item['parcel_refs']);
    final stopRefs = (_mission['parcels'] as List? ?? [])
        .whereType<Map>()
        .where((p) => parcelRefs.contains(p['parcel_ref']))
        .map((p) => p['pickup_stop_ref'])
        .toSet();
    return (_mission['stops'] as List? ?? [])
        .whereType<Map>()
        .where((stop) => stopRefs.contains(stop['stop_ref']))
        .map((stop) => (stop['location'] as Map?)?['address']?.toString() ?? '')
        .where((address) => address.trim().isNotEmpty)
        .toSet()
        .toList();
  }

  int _packages(List<String> parcels) => (_mission['parcels'] as List? ?? [])
      .whereType<Map>()
      .where((p) => parcels.contains(p['parcel_ref']))
      .fold<int>(0, (sum, p) => sum + (p['package_count'] as num).toInt());
  Future<void> _startTransfer() => _run('Transfer code ready', () async {
    final parcels = refs(_mission['transfer_parcel_refs']);
    final fix = await _location();
    final data = await widget.api.command('$_path/provider-transfers/start', {
      'expected_version': _mission['version'],
      'parcel_refs': parcels,
      'package_count': _packages(parcels),
      ...fix,
    }, operationRef: true);
    if (mounted) setState(() => _credential = data);
  }, action: 'Prepare handover');
  Future<void> _readCredential() => _run('Transfer code refreshed', () async {
    final data = await widget.api.read(
      '$_path/provider-transfers/${Uri.encodeComponent(_transfer['transfer_ref'] as String)}/credential',
    );
    if (mounted) setState(() => _credential = data);
  }, action: 'Show transfer code');
  Future<void> _acceptTransfer() async {
    if (!_transferConfirmed || _code.text.trim().isEmpty) return;
    if (!await _confirm(
          'Accept parcel custody?',
          'Confirm every package is present and has been handed to you by the outgoing rider.',
          'Accept custody',
        ) ||
        !mounted)
      return;
    await _run('Parcel custody accepted', () async {
      final fix = await _location();
      await widget.api.command(
        '$_path/provider-transfers/${Uri.encodeComponent(_transfer['transfer_ref'] as String)}/complete',
        {
          'expected_version': _mission['version'],
          'code': _code.text.trim(),
          'parcel_refs': _transfer['parcel_refs'],
          'package_count': _transfer['package_count'],
          ...fix,
          'evidence': await _transferPhoto.attach(
            widget.api,
            '$_path/evidence-content',
            refs(_transfer['parcel_refs']),
            {..._evidence('accept-transfer'), ...fix},
          ),
        },
        operationRef: true,
      );
      _code.clear();
      _transferPhoto.clear();
    }, action: 'Accept custody');
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final pending = _returns.any((r) => r['state'] != 'RETURNED');
    final transferOpen = _transfer['state'] == 'CHALLENGE_ISSUED';
    final expires = DateTime.tryParse(
      _transfer['expires_at']?.toString() ?? '',
    );
    final expired = expires != null && !expires.isAfter(DateTime.now());
    Widget button(
      String label,
      IconData icon,
      VoidCallback? onTap, {
      String? busyKey,
    }) => Padding(
      padding: const EdgeInsets.only(top: 12),
      child: FilledButton.icon(
        onPressed: _busy == null && !_refreshing ? onTap : null,
        icon: _busy == (busyKey ?? label)
            ? const SizedBox(
                width: 18,
                height: 18,
                child: CircularProgressIndicator(strokeWidth: 2),
              )
            : Icon(icon),
        label: Text(label),
      ),
    );
    return Scaffold(
      appBar: AppBar(title: const Text('Delivery options')),
      body: RefreshIndicator(
        onRefresh: _refresh,
        child: ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: const EdgeInsets.all(20),
          children: [
            if (_error != null) ...[
              ActionNotice(message: _error!, isError: true, onRetry: _refresh),
              const SizedBox(height: 16),
            ],
            if (_notice != null) ...[
              ActionNotice(message: _notice!, success: true),
              const SizedBox(height: 16),
            ],
            if (_accessLost)
              const ActionNotice(
                message:
                    'This delivery is no longer assigned to you. Return to Home for your current work.',
              )
            else ...[
              Text(
                'Parcel custody',
                style: Theme.of(context).textTheme.headlineSmall,
              ),
              const SizedBox(height: 8),
              const Text(
                'Track returns and confirm a handover with the assigned replacement rider.',
              ),
              const SizedBox(height: 24),
              if (_mission['can_start_transfer'] == true) ...[
                Card(
                  child: Padding(
                    padding: const EdgeInsets.all(20),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        Icon(
                          Icons.swap_horiz_rounded,
                          size: 36,
                          color: c.brandTeal600,
                        ),
                        const SizedBox(height: 12),
                        Text(
                          'Replacement rider assigned',
                          style: Theme.of(context).textTheme.titleLarge,
                        ),
                        const SizedBox(height: 8),
                        Text(
                          '${refs(_mission['transfer_parcel_refs']).length} parcels must be handed over together.',
                        ),
                        CheckboxListTile(
                          contentPadding: EdgeInsets.zero,
                          value: _transferConfirmed,
                          onChanged: _busy == null
                              ? (v) => setState(
                                  () => _transferConfirmed = v == true,
                                )
                              : null,
                          title: const Text(
                            'I have checked every package with the replacement rider',
                          ),
                        ),
                        button(
                          'Prepare handover',
                          Icons.swap_horiz_rounded,
                          _transferConfirmed ? _startTransfer : null,
                        ),
                      ],
                    ),
                  ),
                ),
                const SizedBox(height: 16),
              ],
              if (transferOpen) ...[
                Card(
                  child: Padding(
                    padding: const EdgeInsets.all(20),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        Text(
                          _transfer['role'] == 'OUTGOING'
                              ? 'Hand over to your replacement'
                              : 'Receive the delivery',
                          style: Theme.of(context).textTheme.titleLarge,
                        ),
                        const SizedBox(height: 12),
                        Text(
                          '${_transfer['package_count']} packages across ${refs(_transfer['parcel_refs']).length} parcels',
                        ),
                        const SizedBox(height: 12),
                        if (expired)
                          const ActionNotice(
                            message:
                                'The transfer code expired. Contact support to arrange a new handover.',
                            isError: true,
                          )
                        else if (_transfer['role'] == 'OUTGOING') ...[
                          const Text(
                            'Show your code to the assigned replacement rider. They confirm receipt in their app.',
                          ),
                          if (_credential != null)
                            Padding(
                              padding: const EdgeInsets.symmetric(vertical: 20),
                              child: SelectableText(
                                _credential!['challenge_code']?.toString() ??
                                    '',
                                textAlign: TextAlign.center,
                                style: Theme.of(context).textTheme.headlineLarge
                                    ?.copyWith(letterSpacing: 6),
                              ),
                            ),
                          button(
                            'Show transfer code',
                            Icons.key_outlined,
                            _readCredential,
                          ),
                        ] else ...[
                          TextField(
                            controller: _code,
                            onChanged: (_) => setState(() {}),
                            decoration: const InputDecoration(
                              labelText: 'Code from outgoing rider',
                            ),
                            autocorrect: false,
                            enableSuggestions: false,
                          ),
                          CheckboxListTile(
                            contentPadding: EdgeInsets.zero,
                            value: _transferConfirmed,
                            onChanged: _busy == null
                                ? (v) => setState(
                                    () => _transferConfirmed = v == true,
                                  )
                                : null,
                            title: const Text(
                              'Every package is present and handed over to me',
                            ),
                          ),
                          MissionPhotoField(
                            draft: _transferPhoto,
                            enabled: _busy == null,
                            onChanged: () => setState(() {}),
                          ),
                          button(
                            'Accept custody',
                            Icons.inventory_2_outlined,
                            _transferConfirmed &&
                                    _code.text.trim().isNotEmpty &&
                                    !_transferPhoto.picking
                                ? _acceptTransfer
                                : null,
                          ),
                        ],
                      ],
                    ),
                  ),
                ),
                const SizedBox(height: 20),
              ],
              if (AppModeConfig.instance.isMock &&
                  _mission['state'] == 'IN_TRANSIT' &&
                  _mission['can_start_transfer'] != true &&
                  _transfer.isEmpty)
                OutlinedButton.icon(
                  onPressed: _busy == null
                      ? () => _run('Demo replacement assigned', () async {
                          await widget.api.command('$_path/mock-replacement', {
                            'expected_version': _mission['version'],
                          }, operationRef: true);
                        })
                      : null,
                  icon: const Icon(Icons.science_outlined),
                  label: const Text('Simulate replacement assignment'),
                ),
              if (AppModeConfig.instance.isMock &&
                  transferOpen &&
                  _transfer['role'] == 'OUTGOING')
                OutlinedButton.icon(
                  onPressed: _busy == null
                      ? () => _run('Demo incoming rider selected', () async {
                          await widget.api.command(
                            '$_path/mock-incoming-provider',
                            {'expected_version': _mission['version']},
                            operationRef: true,
                          );
                          _credential = null;
                        })
                      : null,
                  icon: const Icon(Icons.science_outlined),
                  label: const Text('Switch to incoming rider'),
                ),
              if (_transfer['state'] == 'COMPLETED') ...[
                const ActionNotice(
                  message:
                      'Rider handover completed. Parcel custody has been updated.',
                  success: true,
                ),
                const SizedBox(height: 20),
              ],

              if ((_mission['exception_options'] as List? ?? [])
                  .isNotEmpty) ...[
                MissionActionPanel(
                  title: 'Delivery support',
                  busy: _busy != null,
                  actions: [
                    MissionTaskAction(
                      label: 'Report a problem',
                      description: 'Record an issue with this delivery',
                      symbol: JourneySymbol.shield,
                      color: c.warning,
                      onTap: () => _report(),
                    ),
                    MissionTaskAction(
                      label: 'Add delivery evidence',
                      symbol: JourneySymbol.photo,
                      onTap: () => _report(evidenceOnly: true),
                    ),
                  ],
                ),
                const SizedBox(height: 24),
              ],
              if ((_mission['exceptions'] as List? ?? []).isNotEmpty) ...[
                Text(
                  'Reported issues',
                  style: Theme.of(context).textTheme.titleLarge,
                ),
                const SizedBox(height: 12),
                for (final issue
                    in (_mission['exceptions'] as List).whereType<Map>())
                  Card(
                    child: ListTile(
                      leading: Icon(
                        issue['state'] == 'RESOLVED'
                            ? Icons.task_alt_rounded
                            : Icons.report_problem_outlined,
                        color: issue['state'] == 'RESOLVED'
                            ? c.success
                            : c.warning,
                      ),
                      title: Text(
                        commerceReasonLabels[issue['reason_code']] ??
                            'Delivery issue',
                      ),
                      subtitle: Text(
                        '${commerceRecoveryLabels[issue['recovery_action']] ?? 'Recovery recorded'} \u00b7 ${issue['state'] == 'RESOLVED'
                            ? 'Resolved'
                            : issue['state'] == 'RECOVERY_SELECTED'
                            ? 'Recovery selected'
                            : 'Open'}',
                      ),
                    ),
                  ),
                const SizedBox(height: 24),
              ],
              if ((_mission['evidence'] as List? ?? []).isNotEmpty) ...[
                Text(
                  'Saved evidence',
                  style: Theme.of(context).textTheme.titleLarge,
                ),
                const SizedBox(height: 12),
                for (final item
                    in (_mission['evidence'] as List).whereType<Map>())
                  Card(
                    child: ListTile(
                      leading: Icon(
                        Icons.fact_check_outlined,
                        color: c.brandTeal600,
                      ),
                      title: Text(switch (item['evidence_type']) {
                        'DAMAGE' => 'Damage evidence',
                        'DISCREPANCY' => 'Package mismatch',
                        'FAILED_ATTEMPT' => 'Attempt record',
                        'PICKUP' => 'Pickup evidence',
                        'RETURN' => 'Return evidence',
                        'PROVIDER_TRANSFER' => 'Rider handover evidence',
                        _ => 'Delivery evidence',
                      }),
                      subtitle: Text(
                        '${(item['parcel_refs'] as List? ?? []).length} parcels \u00b7 ${item['attachment_count'] ?? 0} attachments${(item['description']?.toString() ?? '').isEmpty ? '' : '\n${item['description']}'}',
                      ),
                    ),
                  ),
                const SizedBox(height: 24),
              ],
              Text('Returns', style: Theme.of(context).textTheme.titleLarge),
              const SizedBox(height: 12),
              if (_returns.isEmpty)
                const ActionNotice(
                  message: 'No returns recorded for this delivery.',
                ),
              for (final item in _returns)
                Card(
                  child: Padding(
                    padding: const EdgeInsets.all(20),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        Row(
                          children: [
                            Icon(
                              item['state'] == 'RETURNED'
                                  ? Icons.task_alt_rounded
                                  : Icons.assignment_return_outlined,
                              color: item['state'] == 'RETURNED'
                                  ? c.success
                                  : c.warning,
                            ),
                            const SizedBox(width: 12),
                            Expanded(
                              child: Text(
                                _returnStateLabel(
                                  item['state']?.toString() ?? '',
                                ),
                                style: Theme.of(context).textTheme.titleMedium,
                              ),
                            ),
                          ],
                        ),
                        const SizedBox(height: 12),
                        Text('${refs(item['parcel_refs']).length} parcels'),
                        const SizedBox(height: 8),
                        Text(_dispositionLabel(item['disposition'].toString())),

                        if (item['disposition'] == 'RETURN_TO_VENDOR')
                          for (final address in _returnLocations(item))
                            ListTile(
                              contentPadding: EdgeInsets.zero,
                              leading: const Icon(Icons.storefront_outlined),
                              title: Text(address),
                            ),

                        if (item['state'] == 'IN_RETURN' &&
                            refs(_mission['return_parcel_refs']).isNotEmpty)
                          MissionPhotoField(
                            draft: _returnPhotos.putIfAbsent(
                              item['return_id'] as String,
                              () => MissionPhotoDraft(),
                            ),
                            enabled: _busy == null,
                            onChanged: () => setState(() {}),
                          ),
                        if (item['state'] == 'IN_RETURN' &&
                            refs(_mission['return_parcel_refs']).isNotEmpty)
                          button(
                            'Complete return',
                            Icons.check_rounded,
                            _returnPhotos[item['return_id']]?.picking == true
                                ? null
                                : () => _completeReturn(item),
                            busyKey: 'return:${item['return_id']}',
                          ),
                      ],
                    ),
                  ),
                ),
              if (!pending &&
                  refs(_mission['return_parcel_refs']).isNotEmpty &&
                  !transferOpen)
                button(
                  'Arrange a return',
                  Icons.assignment_return_outlined,
                  _startReturn,
                ),
            ],
            const SizedBox(height: 24),
          ],
        ),
      ),
    );
  }
}

String _dispositionLabel(String value) => switch (value) {
  'RETURN_TO_VENDOR' => 'Return to vendor',
  'FOOD_DISPOSAL_POLICY' => 'Authorised food disposal policy',
  'SHOP_INSPECTION' => 'Retail inspection',
  'SUPPORT_REVIEW' => 'Support review',
  _ => 'Return handling',
};

class _ReturnForm extends StatefulWidget {
  const _ReturnForm({required this.parcels});
  final List<String> parcels;
  @override
  State<_ReturnForm> createState() => _ReturnFormState();
}

class _ReturnFormState extends State<_ReturnForm> {
  String? _class, _disposition, _reason;
  final _notes = TextEditingController();
  final _selected = <String>{};
  @override
  void dispose() {
    _notes.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final options = [
      'RETURN_TO_VENDOR',
      if (_class == 'PREPARED_FOOD') 'FOOD_DISPOSAL_POLICY',
      if (_class == 'SHOP_RETAIL') 'SHOP_INSPECTION',
      'SUPPORT_REVIEW',
    ];
    return Scaffold(
      appBar: AppBar(title: const Text('Arrange a return')),
      body: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          Text(
            'What needs to come back?',
            style: Theme.of(context).textTheme.headlineSmall,
          ),
          const SizedBox(height: 20),
          DropdownButtonFormField<String>(
            isExpanded: true,
            initialValue: _class,
            decoration: const InputDecoration(labelText: 'Product type'),
            items: const [
              DropdownMenuItem(
                value: 'PREPARED_FOOD',
                child: Text('Prepared food'),
              ),
              DropdownMenuItem(
                value: 'SHOP_RETAIL',
                child: Text('Shop / retail'),
              ),
            ],
            onChanged: (v) => setState(() {
              _class = v;
              _disposition = null;
            }),
          ),
          const SizedBox(height: 16),
          DropdownButtonFormField<String>(
            isExpanded: true,
            key: ValueKey(_class),
            initialValue: _disposition,
            decoration: const InputDecoration(labelText: 'Return handling'),
            items: [
              for (final value in options)
                DropdownMenuItem(
                  value: value,
                  child: Text(_dispositionLabel(value)),
                ),
            ],
            onChanged: (v) => setState(() => _disposition = v),
          ),
          const SizedBox(height: 16),
          DropdownButtonFormField<String>(
            isExpanded: true,
            initialValue: _reason,
            decoration: const InputDecoration(labelText: 'Reason'),
            items: const [
              DropdownMenuItem(
                value: 'RECEIVER_REFUSED',
                child: Text('Recipient refused'),
              ),
              DropdownMenuItem(
                value: 'RECEIVER_UNAVAILABLE',
                child: Text('Recipient unavailable'),
              ),
              DropdownMenuItem(
                value: 'DAMAGED_PACKAGE',
                child: Text('Package damaged'),
              ),
              DropdownMenuItem(
                value: 'PACKAGE_MISMATCH',
                child: Text('Package mismatch'),
              ),
            ],
            onChanged: (v) => setState(() => _reason = v),
          ),
          const SizedBox(height: 20),
          for (final parcel in widget.parcels)
            Card(
              child: CheckboxListTile(
                value: _selected.contains(parcel),
                title: Text(parcel),
                onChanged: (v) => setState(() {
                  v == true ? _selected.add(parcel) : _selected.remove(parcel);
                }),
              ),
            ),
          const SizedBox(height: 20),
          TextField(
            controller: _notes,
            maxLength: 2000,
            minLines: 2,
            maxLines: 4,
            decoration: const InputDecoration(
              labelText: 'Handling notes (optional)',
            ),
          ),
          const SizedBox(height: 20),
          FilledButton(
            onPressed:
                _class != null &&
                    _disposition != null &&
                    _reason != null &&
                    _selected.isNotEmpty
                ? () => Navigator.pop(context, <String, dynamic>{
                    'product_class': _class,
                    'disposition': _disposition,
                    'reason_code': _reason,
                    'parcel_refs': _selected.toList(),
                    'description': _notes.text.trim(),
                  })
                : null,
            child: const Text('Start return'),
          ),
        ],
      ),
    );
  }
}

String _returnStateLabel(String state) => switch (state) {
  'RETURNED' => 'Return completed',
  'IN_RETURN' => 'Return in progress',
  'REQUIRED' => 'Return required',
  'SUPPORT_REQUIRED' => 'Support review required',
  _ => 'Return status unavailable',
};
