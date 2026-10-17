import 'mission_action_panel.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/dispatch_screen_frame.dart';
import 'dart:async';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:image_picker/image_picker.dart';
import 'package:qr_flutter/qr_flutter.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../../../core/app_mode.dart';
import '../../../core/config.dart';
import '../../../core/network/provider_session.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/async_content.dart';
import '../data/provider_operations_api.dart';
import 'parcel_scanner_screen.dart';
import 'signature_capture_screen.dart';
import 'commerce_pickup_screen.dart';
import 'hub_custody_screen.dart';

class DeliveryOperationsScreen extends StatefulWidget {
  const DeliveryOperationsScreen({super.key, required this.tripId});
  final String tripId;
  @override
  State<DeliveryOperationsScreen> createState() =>
      _DeliveryOperationsScreenState();
}

class _DeliveryOperationsScreenState extends State<DeliveryOperationsScreen> {
  ProviderOperationsApi? _api;
  Map<String, dynamic>? _trip;
  Map<String, dynamic>? _commerce;
  String? _error;
  String? _notice;
  String? _busy;
  String? _actionLabel;
  int _readTicket = 0;
  bool _refreshing = false;
  bool _loading = true;
  int _stopIndex = 0;
  String? _handoverRef;
  Timer? _poll;
  Timer? _clock;
  Map<String, dynamic>? _capture;
  String? _photoPath;
  String _captureKind = 'PHOTO';
  String? _mediaRef;
  DateTime? _capturedAt;

  Map<String, dynamic> get _mission =>
      Map<String, dynamic>.from(_trip?['mission'] as Map? ?? {});
  Map<String, dynamic> get _handover =>
      Map<String, dynamic>.from(_mission['handover'] as Map? ?? {});
  Map<String, dynamic> get _recovery =>
      Map<String, dynamic>.from(_mission['recovery'] as Map? ?? {});
  Map<String, dynamic> get _manifest =>
      Map<String, dynamic>.from(_mission['manifest'] as Map? ?? {});
  List<Map<String, dynamic>> get _stops => (_mission['stops'] as List? ?? [])
      .whereType<Map<String, dynamic>>()
      .toList();
  List<Map<String, dynamic>> get _parcels =>
      (_mission['parcels'] as List? ?? [])
          .whereType<Map<String, dynamic>>()
          .toList();
  Set<String> get _actions => (_mission['allowed_actions'] as List? ?? [])
      .map((v) => v.toString())
      .toSet();
  Map<String, dynamic>? get _stop =>
      _stops.isEmpty ? null : _stops[_stopIndex.clamp(0, _stops.length - 1)];
  String get _tripPath =>
      '/api/v1/provider/trips/${Uri.encodeComponent(widget.tripId)}';
  String get _stopPath =>
      '$_tripPath/stops/${Uri.encodeComponent(_stop?['id']?.toString() ?? '')}';
  String get _stage => _stop?['type'] == 'RETURN'
      ? 'RETURN'
      : _stop?['type'] == 'DELIVERY'
      ? 'DELIVERY'
      : 'PICKUP';
  bool get _canCapture =>
      _handover['status'] == 'CONSUMED' &&
      _handover['stage'] == _stage &&
      (_handover['stop_ref'] == _stop?['stop_ref']) &&
      _manifest.isNotEmpty;

  Set<String> get _acceptedTypes =>
      (_handover['accepted_evidence_types'] as List? ?? [])
          .map((v) => v.toString())
          .toSet();
  bool get _needsPhoto =>
      (_handover['photo_required'] == true &&
          !_acceptedTypes.contains('PHOTO')) ||
      ((_handover['required_evidence_count'] as num? ?? 0) >
              (_handover['accepted_evidence_count'] as num? ?? 0) &&
          _handover['signature_required'] != true);
  bool get _needsSignature =>
      _handover['signature_required'] == true &&
      !_acceptedTypes.contains('SIGNATURE');

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (_api != null) return;
    final session = ProviderSession.of(context);
    _api = ProviderOperationsApi(
      origin: AppConfig.apiBaseUrl,
      client: session.client,
      token: session.accessToken,
      onUncertainEvidence: session.offlineUpdates?.saveEvidence,
    );
    _load();
    _poll = Timer.periodic(const Duration(seconds: 6), (_) {
      if (_busy == null && !_loading) _load(silent: true);
    });
    _clock = Timer.periodic(const Duration(seconds: 1), (_) {
      if (mounted && _recovery.isNotEmpty) setState(() {});
    });
  }

  Future<void> _load({bool silent = false}) async {
    if (_refreshing) return;
    _refreshing = true;
    final ticket = ++_readTicket;
    if (!silent)
      setState(() {
        _loading = true;
        _error = null;
      });
    try {
      final trip = await _api!.readTrip(widget.tripId);
      Map<String, dynamic>? commerce;
      if (AppModeConfig.instance.isLive) {
        try {
          commerce = await _api!.read(
            '/api/v1/provider/commerce-missions/by-trip/${Uri.encodeComponent(widget.tripId)}',
          );
        } on ApiException catch (error) {
          if (error.statusCode != 404) rethrow;
        }
      }
      if (!mounted || ticket != _readTicket) return;
      if (commerce != null) _poll?.cancel();
      setState(() {
        _commerce = commerce;
        _trip = trip;
        if (_capture == null && _handoverRef != _handover['challenge_ref']) {
          _handoverRef = _handover['challenge_ref']?.toString();
          final index = _stops.indexWhere(
            (stop) => stop['stop_ref'] == _handover['stop_ref'],
          );
          if (index >= 0) _stopIndex = index;
        }
        if (_stopIndex >= _stops.length) _stopIndex = 0;
      });
    } catch (error) {
      if (mounted && ticket == _readTicket)
        setState(
          () => _error = error is ApiException
              ? error.message
              : 'We could not refresh this delivery. Check your connection.',
        );
    } finally {
      if (ticket == _readTicket) _refreshing = false;
      if (mounted && ticket == _readTicket) setState(() => _loading = false);
    }
  }

  Future<void> _run(
    String label,
    Future<Map<String, dynamic>> Function() operation,
  ) async {
    if (_busy != null) return;
    _readTicket++;
    _refreshing = false;
    setState(() {
      _busy = label;
      _error = null;
      _notice = null;
    });
    try {
      await operation();
      if (!mounted) return;
      setState(() => _notice = label);
      await _load(silent: true);
    } catch (error) {
      if (!mounted) return;
      setState(
        () => _error = error is ApiException
            ? error.message
            : 'We could not confirm the result. Refresh the delivery before retrying.',
      );
      await _load(silent: true);
    } finally {
      if (mounted)
        setState(() {
          _busy = null;
          _actionLabel = null;
        });
    }
  }

  Future<void> _credential() async {
    if (_stop == null) return;
    await _run('Pickup code refreshed', () async {
      final credential = await _api!.read('$_stopPath/pickup-credential');
      if (!mounted) return credential;
      final expires = DateTime.tryParse(
        credential['expires_at']?.toString() ?? '',
      );
      final expired = expires != null && !expires.isAfter(DateTime.now());
      final otp = credential['otp']?.toString() ?? '';
      final qr = credential['qr_token']?.toString() ?? '';
      await showModalBottomSheet<void>(
        context: context,
        showDragHandle: true,
        useSafeArea: true,
        isScrollControlled: true,
        builder: (sheet) => SingleChildScrollView(
          padding: const EdgeInsets.fromLTRB(24, 0, 24, 30),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                expired ? 'This code has expired' : 'Show this to the sender',
                style: Theme.of(sheet).textTheme.titleLarge,
              ),
              const SizedBox(height: 12),
              Text(
                expired
                    ? 'Refresh the delivery and request a current code.'
                    : 'The sender verifies this code in their app before handing over the parcels.',
                textAlign: TextAlign.center,
              ),
              if (!expired && qr.isNotEmpty)
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 20),
                  child: ColoredBox(
                    color: Colors.white,
                    child: QrImageView(data: qr, size: 210),
                  ),
                ),
              if (!expired && otp.isNotEmpty)
                SelectableText(
                  otp,
                  style: Theme.of(
                    sheet,
                  ).textTheme.headlineLarge?.copyWith(letterSpacing: 6),
                ),
              if (AppModeConfig.instance.isMock) ...[
                const SizedBox(height: 24),
                OutlinedButton(
                  onPressed: () async {
                    Navigator.pop(sheet);
                    await _api!.command(
                      '$_tripPath/mock-sender-confirmation',
                      {},
                    );
                  },
                  child: const Text('Simulate sender confirmation'),
                ),
              ],
            ],
          ),
        ),
      );
      return credential;
    });
  }

  Future<void> _scan() async {
    final code = await Navigator.of(context).push<String>(
      MaterialPageRoute(builder: (_) => const ParcelScannerScreen()),
    );
    if (code == null || !mounted) return;
    await _run(
      'Parcel scan recorded',
      () => _api!.command('$_stopPath/parcels/scan', {
        'parcel_code': code,
        'stage': _stage,
        'manifest_ref': _manifest['reference'],
        'manifest_version': _manifest['version'],
      }),
    );
  }

  Future<void> _capturePhoto() async {
    if (_busy != null || !_canCapture) return;
    if (_photoPath != null && _captureKind != 'PHOTO') {
      setState(
        () => _error =
            'Finish submitting the current signature before adding a photo.',
      );
      return;
    }
    if (_photoPath == null) {
      try {
        final photo = await ImagePicker().pickImage(
          source: ImageSource.camera,
          maxWidth: 2000,
          maxHeight: 2000,
          imageQuality: 85,
        );
        if (photo == null || !mounted) return;
        _photoPath = photo.path;
        _capturedAt = DateTime.now().toUtc();
      } catch (_) {
        if (mounted)
          setState(
            () => _error = 'Allow camera access to capture a handover photo.',
          );
        return;
      }
    }
    _captureKind = 'PHOTO';
    await _submitCapture();
  }

  Future<void> _captureSignature() async {
    if (_busy != null || !_canCapture) return;
    if (_photoPath != null && _captureKind != 'SIGNATURE') {
      setState(
        () => _error =
            'Finish submitting the current photo before adding a signature.',
      );
      return;
    }
    if (_photoPath == null) {
      final path = await Navigator.of(context).push<String>(
        MaterialPageRoute(builder: (_) => const SignatureCaptureScreen()),
      );
      if (path == null || !mounted) return;
      _photoPath = path;
      _captureKind = 'SIGNATURE';
      _capturedAt = DateTime.now().toUtc();
    }
    await _submitCapture();
  }

  Future<void> _submitCapture() async {
    final expires = DateTime.tryParse(
      _capture?['expires_at']?.toString() ?? '',
    );
    if (expires != null && !expires.isAfter(DateTime.now())) {
      _capture = null;
      _mediaRef = null;
    }
    await _run(
      _captureKind == 'SIGNATURE' ? 'Signature saved' : 'Photo evidence saved',
      () async {
        _capture ??= await _api!.command('$_stopPath/evidence-session', {
          'stage': _stage,
          'evidence_type': _captureKind,
          'manifest_ref': _manifest['reference'],
          'manifest_version': _manifest['version'],
        });
        _mediaRef ??= await _api!.upload(
          '$_stopPath/evidence-content',
          _capture!['capture_session_ref'] as String,
          _photoPath!,
        );
        final result = await _api!.command('$_stopPath/evidence', {
          'capture_session_ref': _capture!['capture_session_ref'],
          'media_ref': _mediaRef,
          'captured_at': _capturedAt!.toIso8601String(),
        });
        final savedSignature = _captureKind == 'SIGNATURE' ? _photoPath : null;
        if (savedSignature != null) {
          try {
            await File(savedSignature).delete();
          } on FileSystemException {
            /* Capture already removed. */
          }
        }
        _capture = null;
        _photoPath = null;
        _mediaRef = null;
        _capturedAt = null;
        return result;
      },
    );
  }

  Widget _handoverProgress(BuildContext context) {
    final c = context.palette;
    final scanned = (_handover['scanned_parcel_count'] as num? ?? 0).toInt();
    final expected = (_handover['expected_parcel_count'] as num? ?? 0).toInt();
    final accepted = (_handover['accepted_evidence_count'] as num? ?? 0)
        .toInt();
    final required = (_handover['required_evidence_count'] as num? ?? 0)
        .toInt();
    Widget item(String label, bool complete) => Padding(
      padding: const EdgeInsets.symmetric(vertical: 7),
      child: Row(
        children: [
          AnimatedSwitcher(
            duration: const Duration(milliseconds: 220),
            child: Icon(
              complete
                  ? Icons.check_circle_rounded
                  : Icons.radio_button_unchecked_rounded,
              key: ValueKey('$label:$complete'),
              size: 22,
              color: complete ? c.brandTeal600 : c.textMuted,
            ),
          ),
          const SizedBox(width: 12),
          Expanded(child: Text(label)),
        ],
      ),
    );
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(18),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              'Handover progress',
              style: Theme.of(context).textTheme.titleMedium,
            ),
            const SizedBox(height: 8),
            item(
              _stage == 'PICKUP'
                  ? 'Sender confirmation'
                  : 'Recipient confirmation',
              _handover['status'] == 'CONSUMED',
            ),
            if (_handover['parcel_scans_required'] == true)
              item(
                '$scanned of $expected parcels scanned',
                expected > 0 && scanned >= expected,
              ),
            if (_handover['photo_required'] == true)
              item('Handover photo', _acceptedTypes.contains('PHOTO')),
            if (_handover['signature_required'] == true)
              item('Recipient signature', _acceptedTypes.contains('SIGNATURE')),
            if (required > 0 &&
                _handover['photo_required'] != true &&
                _handover['signature_required'] != true)
              item(
                '$accepted of $required evidence items saved',
                accepted >= required,
              ),
          ],
        ),
      ),
    );
  }

  Future<String?> _choose(String title, Map<String, String> options) =>
      showModalBottomSheet<String>(
        context: context,
        showDragHandle: true,
        useSafeArea: true,
        isScrollControlled: true,
        builder: (sheet) => SingleChildScrollView(
          padding: const EdgeInsets.fromLTRB(20, 0, 20, 24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(title, style: Theme.of(sheet).textTheme.titleLarge),
              const SizedBox(height: 16),
              for (final item in options.entries)
                ListTile(
                  title: Text(item.value),
                  trailing: const Icon(Icons.chevron_right_rounded),
                  onTap: () => Navigator.pop(sheet, item.key),
                ),
            ],
          ),
        ),
      );

  Future<void> _verify() async {
    final code = TextEditingController();
    final entered = await showModalBottomSheet<String>(
      context: context,
      useSafeArea: true,
      showDragHandle: true,
      isScrollControlled: true,
      builder: (sheet) => Padding(
        padding: EdgeInsets.fromLTRB(
          24,
          0,
          24,
          MediaQuery.viewInsetsOf(sheet).bottom + 24,
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              _stage == 'RETURN'
                  ? 'Confirm return handover'
                  : 'Confirm delivery',
              style: Theme.of(sheet).textTheme.titleLarge,
            ),
            const SizedBox(height: 12),
            const Text('Ask the recipient for their handover code.'),
            const SizedBox(height: 20),
            TextField(
              controller: code,
              autofocus: true,
              keyboardType: TextInputType.number,
              inputFormatters: [FilteringTextInputFormatter.digitsOnly],
              decoration: const InputDecoration(labelText: 'Handover code'),
            ),
            const SizedBox(height: 20),
            FilledButton(
              onPressed: () {
                if (code.text.trim().isNotEmpty)
                  Navigator.pop(sheet, code.text.trim());
              },
              child: const Text('Verify handover'),
            ),
          ],
        ),
      ),
    );
    code.dispose();
    if (entered == null || !mounted) return;
    await _run(
      'Handover verified',
      () => _api!.command(
        '$_tripPath/${_stage == 'RETURN' ? 'return' : 'delivery'}/verify',
        {
          'challenge_ref': _handover['challenge_ref'],
          'method': 'OTP',
          'credential': entered,
          'manifest_ref': _handover['manifest_ref'],
          'manifest_version': _handover['manifest_version'],
        },
      ),
    );
  }

  Future<void> _contact() async {
    final outcome = await _choose('Record a phone contact attempt', const {
      'UNREACHABLE': 'No answer or unreachable',
      'DECLINED': 'Call declined',
      'NUMBER_INVALID': 'Number is invalid',
      'CONTACTED_WAITING': 'Recipient is coming',
      'CONTACTED_CANNOT_RECEIVE': 'Recipient cannot receive the parcel',
    });
    if (outcome == null || !mounted) return;
    await _run(
      'Contact attempt recorded',
      () => _api!.command(
        '$_tripPath/delivery-attempts/${Uri.encodeComponent(_recovery['attempt_id'] as String)}/contact',
        {'method': 'PHONE', 'outcome': outcome},
      ),
    );
  }

  Future<void> _failure() async {
    final reason = await _choose('Why could delivery not be completed?', const {
      'RECEIVER_NO_ANSWER': 'Recipient is not answering',
      'RECEIVER_UNREACHABLE': 'Recipient is unreachable',
      'RECEIVER_REFUSED': 'Recipient refused the parcel',
      'INVALID_CONTACT': 'Contact details are invalid',
      'ACCESS_RESTRICTED': 'Access is restricted',
      'LOCATION_UNSAFE': 'Location is unsafe',
      'DELIVERY_WINDOW_MISSED': 'Delivery window was missed',
      'OTHER_REVIEW_REQUIRED': 'Another issue needs review',
    });
    if (reason == null || !mounted) return;
    await _run(
      'Delivery attempt recorded',
      () => _api!.command(
        '$_tripPath/delivery-attempts/${Uri.encodeComponent(_recovery['attempt_id'] as String)}/fail',
        {'reason_code': reason},
      ),
    );
  }

  Future<void> _incident() async {
    final reason = await _choose('Get help with this delivery', const {
      'BREAKDOWN': 'Vehicle breakdown',
      'VEHICLE_FAILURE': 'Vehicle cannot continue',
      'ILLNESS': 'I am unwell',
      'SAFETY_CONCERN': 'Safety concern',
      'ACCIDENT': 'Accident',
      'OTHER_REVIEW_REQUIRED': 'Another issue needs review',
    });
    if (reason == null || !mounted) return;
    await _run(
      'Help request received',
      () => _api!.command('$_tripPath/breakdown', {'reason_code': reason}),
    );
  }

  Future<void> _reroute() async {
    final reason = await _choose('Update the route', const {
      'ROAD_CLOSURE': 'Road closure',
      'SAFETY_REROUTE': 'A safer route is needed',
      'ROUTE_PROVIDER_UPDATE': 'Route conditions changed',
    });
    if (reason == null || !mounted) return;
    await _run(
      'Route updated',
      () => _api!.command('$_tripPath/reroute', {'reason_code': reason}),
    );
  }

  Widget _taskActions({required bool activeAttempt, required int remaining}) {
    final tasks = <String, MissionTaskAction>{};
    final tools = <MissionTaskAction>[];
    void add(
      String key,
      String label,
      JourneySymbol symbol,
      VoidCallback tap, {
      String? description,
    }) {
      tasks[key] = MissionTaskAction(
        label: label,
        symbol: symbol,
        description: description,
        onTap: () {
          setState(() => _actionLabel = label);
          tap();
        },
      );
    }

    if (_actions.contains('ARRIVE_AT_PICKUP'))
      add(
        'arrive',
        'Arrive at pickup',
        JourneySymbol.route,
        () => _run(
          'Arrival recorded',
          () => _api!.command('$_tripPath/arrived', {}),
        ),
      );
    if (_actions.contains('READ_PICKUP_CREDENTIAL') && _stage == 'PICKUP')
      add(
        'code',
        'Show pickup code',
        JourneySymbol.key,
        _credential,
        description:
            'The sender confirms this code before handing over the parcels.',
      );
    if (_actions.contains('SCAN_PARCELS') &&
        _stage != 'DELIVERY' &&
        _manifest.isNotEmpty)
      add('scan', 'Scan parcel', JourneySymbol.parcel, _scan);
    if (_canCapture &&
        (_needsPhoto || (_photoPath != null && _captureKind == 'PHOTO')))
      add(
        'photo',
        _capture == null ? 'Capture handover photo' : 'Retry photo submission',
        JourneySymbol.photo,
        _capturePhoto,
      );
    if (_canCapture &&
        (_needsSignature ||
            (_photoPath != null && _captureKind == 'SIGNATURE')))
      add(
        'signature',
        'Add recipient signature',
        JourneySymbol.review,
        _captureSignature,
      );
    if (_actions.contains('START_DELIVERY'))
      add(
        'start',
        'Start delivery',
        JourneySymbol.route,
        () => _run(
          'Delivery started',
          () => _api!.command('$_tripPath/start', {}),
        ),
      );
    if (_actions.contains('VERIFY_DELIVERY') &&
        _handover['stage'] == 'DELIVERY' &&
        _handover['status'] == 'ACTIVE')
      add('verify', 'Verify delivery code', JourneySymbol.key, _verify);
    if (_stage == 'RETURN' &&
        _handover['stage'] == 'RETURN' &&
        _handover['status'] == 'ACTIVE')
      add('verify', 'Verify return code', JourneySymbol.key, _verify);
    if (activeAttempt) {
      add('contact', 'Record contact attempt', JourneySymbol.contact, _contact);
      if (remaining <= 0)
        add('failed', 'Record failed attempt', JourneySymbol.review, _failure);
    }
    if (_actions.contains('COMPLETE'))
      add(
        'complete',
        'Complete delivery',
        JourneySymbol.shield,
        () => _run(
          'Delivery completed',
          () => _api!.command('$_tripPath/complete', {}),
        ),
      );
    if (_recovery['action'] == 'HUB_HOLD')
      add('hub', 'Hub custody', JourneySymbol.partner, () async {
        await Navigator.of(context).push(
          MaterialPageRoute(
            builder: (_) => HubCustodyScreen(api: _api!, tripId: widget.tripId),
          ),
        );
        if (mounted) await _load(silent: true);
      });
    if (_actions.contains('START_DELIVERY_ATTEMPT') && !activeAttempt)
      tools.add(
        MissionTaskAction(
          label: 'Recipient unavailable',
          symbol: JourneySymbol.contact,
          description: 'Start the recipient contact and waiting flow',
          color: context.palette.warning,
          onTap: () async {
            final confirmed = await confirmAccountAction(
              context,
              title: 'Recipient unavailable?',
              message:
                  'Start a delivery attempt to record contact attempts and follow the required waiting period.',
              action: 'Start contact attempt',
              symbol: JourneySymbol.contact,
            );
            if (confirmed && mounted)
              await _run(
                'Delivery attempt started',
                () => _api!.command('$_tripPath/delivery-attempts', {}),
              );
          },
        ),
      );
    if (_actions.contains('REPORT_BREAKDOWN'))
      tools.add(
        MissionTaskAction(
          label: 'Get delivery help',
          description: 'Vehicle, health or safety issues',
          symbol: JourneySymbol.shield,
          color: context.palette.warning,
          onTap: _incident,
        ),
      );
    if (_trip?['status'] == 'in_progress')
      tools.add(
        MissionTaskAction(
          label: 'Route conditions changed',
          symbol: JourneySymbol.route,
          onTap: _reroute,
        ),
      );

    final scansMissing =
        _handover['parcel_scans_required'] == true &&
        (_handover['scanned_parcel_count'] as num? ?? 0) <
            (_handover['expected_parcel_count'] as num? ?? 0);
    final preferred = [
      'complete',
      'hub',
      'arrive',
      'verify',
      if (_handover['status'] != 'CONSUMED') 'code',
      if (scansMissing) 'scan',
      'photo',
      'signature',
      if (_canCapture) 'start',
      if (activeAttempt && remaining <= 0) 'failed',
      if (activeAttempt) 'contact',
    ].where(tasks.containsKey).firstOrNull;
    final primary = preferred == null ? null : tasks.remove(preferred);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        MissionActionPanel(
          primary: primary,
          actions: tasks.values.toList(),
          title: 'Other delivery tasks',
          busy: _busy != null,
          busyLabel: _actionLabel,
        ),
        if (tools.isNotEmpty) ...[
          const SizedBox(height: 24),
          MissionActionPanel(
            actions: tools,
            title: 'Help with this delivery',
            busy: _busy != null,
          ),
        ],
        if (AppModeConfig.instance.isMock &&
            _actions.contains('START_DELIVERY_ATTEMPT') &&
            _recovery['action'] != 'HUB_HOLD') ...[
          const SizedBox(height: 20),
          ExpansionTile(
            title: const Text('Demo controls'),
            children: [
              ListTile(
                leading: const Icon(Icons.science_outlined),
                title: const Text('Simulate approved hub hold'),
                onTap: _busy == null
                    ? () => _run(
                        'Demo hub approved',
                        () => _api!.command('$_tripPath/mock-hub-hold', {}),
                      )
                    : null,
              ),
            ],
          ),
        ],
      ],
    );
  }

  @override
  void dispose() {
    _poll?.cancel();
    _clock?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (_commerce != null)
      return CommercePickupScreen(
        api: _api!,
        tripId: widget.tripId,
        initial: _commerce!,
      );
    final c = context.palette;
    final expiry = DateTime.tryParse(
      _recovery['grace_expires_at']?.toString() ?? '',
    );
    final remaining = expiry?.difference(DateTime.now()).inSeconds ?? 0;
    final activeAttempt = _recovery['attempt_state'] == 'GRACE_PERIOD';
    return Scaffold(
      appBar: AppBar(
        leading: const DispatchBackButton(),
        title: const Text('Delivery tasks'),
        actions: [
          IconButton(
            tooltip: 'Refresh delivery',
            onPressed: _busy == null ? _load : null,
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: _loading && _trip == null
          ? const ContentSkeleton()
          : RefreshIndicator(
              onRefresh: _load,
              child: ListView(
                padding: const EdgeInsets.all(20),
                children: [
                  if (_error != null) ...[
                    ActionNotice(
                      message: _error!,
                      isError: true,
                      onRetry: _load,
                    ),
                    const SizedBox(height: 16),
                  ],
                  if (_notice != null) ...[
                    ActionNotice(message: _notice!, success: true),
                    const SizedBox(height: 16),
                  ],
                  if (_trip != null) ...[
                    Container(
                      padding: const EdgeInsets.all(22),
                      decoration: BoxDecoration(
                        color: c.surface,
                        border: Border.all(color: c.divider),
                        borderRadius: BorderRadius.circular(24),
                      ),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Icon(
                            Icons.route_outlined,
                            size: 32,
                            color: c.brandTeal600,
                          ),
                          const SizedBox(height: 18),
                          Text(
                            (_mission['state'] ?? _trip!['status'] ?? '')
                                .toString()
                                .replaceAll('_', ' ')
                                .toLowerCase(),
                            style: Theme.of(context).textTheme.headlineSmall
                                ?.copyWith(fontWeight: FontWeight.w700),
                          ),
                          const SizedBox(height: 12),
                          Text(_trip!['pickup_address']?.toString() ?? ''),
                          const Padding(
                            padding: EdgeInsets.symmetric(vertical: 8),
                            child: Icon(Icons.south_rounded, size: 18),
                          ),
                          Text(_trip!['dropoff_address']?.toString() ?? ''),
                        ],
                      ),
                    ),
                    const SizedBox(height: 24),
                    if (_stops.isNotEmpty) ...[
                      Text(
                        'Stops',
                        style: Theme.of(context).textTheme.titleLarge,
                      ),
                      const SizedBox(height: 12),
                      for (final entry in _stops.asMap().entries)
                        Card(
                          child: ListTile(
                            selected: _stopIndex == entry.key,
                            selectedTileColor: c.teal10,
                            shape: RoundedRectangleBorder(
                              borderRadius: BorderRadius.circular(16),
                            ),
                            leading: CircleAvatar(
                              backgroundColor: c.surfaceElevated,
                              child: Text(
                                '${entry.value['sequence']}',
                                style: TextStyle(color: c.text),
                              ),
                            ),
                            title: Text(
                              entry.value['type']
                                  .toString()
                                  .replaceAll('_', ' ')
                                  .toLowerCase(),
                            ),
                            subtitle: Text(
                              (entry.value['location'] as Map?)?['address']
                                      ?.toString() ??
                                  'Stop ${entry.value['sequence']}',
                            ),
                            trailing: _stopIndex == entry.key
                                ? Icon(
                                    Icons.radio_button_checked,
                                    color: c.brandTeal600,
                                  )
                                : null,
                            onTap: _busy == null && _capture == null
                                ? () => setState(() => _stopIndex = entry.key)
                                : null,
                          ),
                        ),
                      const SizedBox(height: 20),
                    ],
                    if (_handover.isNotEmpty &&
                        _handover['stop_ref'] == _stop?['stop_ref']) ...[
                      _handoverProgress(context),
                      const SizedBox(height: 16),
                    ],
                    if (activeAttempt) ...[
                      ActionNotice(
                        message: remaining > 0
                            ? 'Wait ${remaining ~/ 60}m ${remaining % 60}s and try contacting the recipient.'
                            : 'The waiting period has ended. Record the delivery outcome.',
                      ),
                      const SizedBox(height: 18),
                    ],
                    _taskActions(
                      activeAttempt: activeAttempt,
                      remaining: remaining,
                    ),
                    if (_recovery['attempt_state'] ==
                        'RECEIVER_UNAVAILABLE') ...[
                      const SizedBox(height: 16),
                      const ActionNotice(
                        message:
                            'The delivery outcome was recorded. Wait for the customer or operations team to confirm the next action.',
                      ),
                    ],
                    if (_parcels.isNotEmpty) ...[
                      const SizedBox(height: 28),
                      Text(
                        'Parcels',
                        style: Theme.of(context).textTheme.titleLarge,
                      ),
                      const SizedBox(height: 12),
                      for (final parcel in _parcels)
                        ListTile(
                          contentPadding: EdgeInsets.zero,
                          leading: Icon(
                            Icons.inventory_2_outlined,
                            color: c.brandTeal600,
                          ),
                          title: Text(parcel['reference']?.toString() ?? ''),
                          subtitle: Text(
                            AppModeConfig.instance.isMock
                                ? 'Demo scan code: ${parcel['scannable_code']}'
                                : parcel['state']
                                      .toString()
                                      .replaceAll('_', ' ')
                                      .toLowerCase(),
                          ),
                        ),
                    ],
                    if (_mission.isEmpty)
                      const ActionNotice(
                        message: 'Continue this delivery from Home.',
                      ),
                  ],
                ],
              ),
            ),
    );
  }
}
