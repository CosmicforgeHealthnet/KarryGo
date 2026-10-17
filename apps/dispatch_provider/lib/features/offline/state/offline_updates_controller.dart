import 'dart:async';
import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:http/http.dart' as http;
import '../../../core/app_mode.dart';
import '../../missions/data/provider_operations_api.dart';

class OfflineUpdatesController extends ChangeNotifier {
  OfflineUpdatesController({
    required this.client,
    required this.origin,
    required this.token,
    required this.providerId,
    FlutterSecureStorage? storage,
  }) : _storage = storage ?? const FlutterSecureStorage();
  final http.Client client;
  final String origin;
  final String? Function() token, providerId;
  final FlutterSecureStorage _storage;
  final AppMode _mode = AppModeConfig.instance.current;
  List<Map<String, dynamic>> _records = [];
  String? _owner, _device;
  Future<void>? _loading;
  Future<void> _writes = Future.value();
  Timer? _timer;
  bool _disposed = false, _syncing = false;
  int _generation = 0;
  String? error;
  List<Map<String, dynamic>> get records => List.unmodifiable(
    _records.map(
      (v) => Map<String, dynamic>.from(jsonDecode(jsonEncode(v)) as Map),
    ),
  );
  bool get syncing => _syncing;
  int get pending => _records.where((r) => r['state'] == 'PENDING').length;
  int get needsReview => _records
      .where(
        (r) => [
          'REJECTED',
          'EXPIRED',
          'PENDING_EVIDENCE_SYNC',
        ].contains(r['state']),
      )
      .length;
  bool get hasUpdates => pending > 0 || needsReview > 0 || error != null;
  String get _key => 'dp_offline_${_mode.name}_${Uri.encodeComponent(_owner!)}';
  bool _valid(int generation) =>
      !_disposed &&
      generation == _generation &&
      _owner == providerId() &&
      _mode == AppModeConfig.instance.current;
  void _notify() {
    if (!_disposed) notifyListeners();
  }

  Future<T> _serial<T>(Future<T> Function() operation) {
    final result = Completer<T>();
    _writes = _writes.then((_) async {
      try {
        result.complete(await operation());
      } catch (e, s) {
        result.completeError(e, s);
      }
    });
    return result.future;
  }

  Future<void> start() async {
    if (_disposed ||
        _mode != AppModeConfig.instance.current ||
        providerId() == null ||
        providerId()!.isEmpty)
      return;
    if (_owner != providerId()) {
      _generation++;
      _owner = providerId();
      _records = [];
      _device = null;
      error = null;
      final generation = _generation;
      final key = _key;
      _loading = () async {
        try {
          final raw = await _storage.read(key: key);
          if (!_valid(generation)) return;
          final data = raw == null
              ? <String, dynamic>{}
              : Map<String, dynamic>.from(jsonDecode(raw) as Map);
          final records = (data['records'] as List? ?? [])
              .whereType<Map>()
              .map((v) => Map<String, dynamic>.from(v))
              .toList();
          _device =
              data['device_id']?.toString() ??
              ProviderOperationsApi.newOperationKey();
          _records = records;
        } catch (_) {
          if (_valid(generation)) {
            error =
                'Saved updates could not be opened. Try again before recording more.';
            _owner = null;
          }
        } finally {
          _notify();
        }
      }();
    }
    await _loading;
    if (_disposed || _owner != providerId()) return;
    _timer ??= Timer.periodic(
      const Duration(seconds: 25),
      (_) => unawaited(sync()),
    );
  }

  Future<void> _persist(
    List<Map<String, dynamic>> records,
    int generation,
  ) async {
    if (!_valid(generation)) return;
    final key = _key;
    await _storage.write(
      key: key,
      value: jsonEncode({'device_id': _device, 'records': records}),
    );
    if (_valid(generation)) {
      _records = records;
      _notify();
    }
  }

  Future<bool> _enqueue(
    String kind,
    Map<String, dynamic> payload,
    DateTime observedAt,
  ) async {
    await start();
    if (_owner == null || _device == null || _disposed) return false;
    final generation = _generation;
    return _serial(() async {
      if (!_valid(generation)) return false;
      if (_records.any(
        (r) =>
            r['kind'] == kind &&
            r['body']?['client_timestamp'] ==
                observedAt.toUtc().toIso8601String() &&
            jsonEncode(r['body']?['payload']) == jsonEncode(payload),
      ))
        return true;
      final retained = _records
          .where((r) => !['ACCEPTED', 'DISMISSED'].contains(r['state']))
          .toList();
      if (retained.length >= 500) {
        error = 'Offline storage is full. Reconnect to sync saved updates.';
        _notify();
        return false;
      }
      final id = ProviderOperationsApi.newOperationKey();
      final record = <String, dynamic>{
        'id': id,
        'kind': kind,
        'state': 'PENDING',
        'attempts': 0,
        'body': {
          'device_id': _device,
          'device_command_id': id,
          'command_type': kind,
          'payload': payload,
          'client_timestamp': observedAt.toUtc().toIso8601String(),
          'last_known_server_version': 0,
        },
      };
      final history = _records
          .where((r) => ['ACCEPTED', 'DISMISSED'].contains(r['state']))
          .toList()
          .reversed
          .take(30)
          .toList()
          .reversed;
      try {
        await _persist([...history, ...retained, record], generation);
        return true;
      } catch (_) {
        error =
            'This update could not be saved on your phone. Keep the delivery open and retry.';
        _notify();
        return false;
      }
    });
  }

  Future<bool> saveLocation({
    required double latitude,
    required double longitude,
    required double heading,
    required double speed,
    required double accuracy,
    required DateTime observedAt,
  }) async {
    if (![
          latitude,
          longitude,
          heading,
          speed,
          accuracy,
        ].every((n) => n.isFinite) ||
        latitude.abs() > 90 ||
        longitude.abs() > 180 ||
        heading < 0 ||
        heading > 360 ||
        speed < 0 ||
        accuracy < 0)
      return false;
    return _enqueue('LOCATION_FIX', {
      'latitude': latitude,
      'longitude': longitude,
      'heading': heading,
      'speed': speed,
      'accuracy': accuracy,
    }, observedAt);
  }

  Future<void> saveEvidence(List<String> refs, String sourcePath) async {
    final media = refs.where((v) => v.trim().isNotEmpty).toSet().toList()
      ..sort();
    if (media.isEmpty) return;
    await start();
    if (_records.any(
      (r) =>
          r['kind'] == 'EVIDENCE_REFERENCE' &&
          ['PENDING', 'PENDING_EVIDENCE_SYNC'].contains(r['state']) &&
          jsonEncode(r['body']?['payload']?['media_refs']) ==
              jsonEncode(media) &&
          r['body']?['payload']?['source_route'] == sourcePath,
    ))
      return;
    await _enqueue('EVIDENCE_REFERENCE', {
      'media_refs': media,
      'source_route': sourcePath,
    }, DateTime.now().toUtc());
  }

  Future<void> _update(
    String id,
    Map<String, dynamic> changes,
    int generation,
  ) => _serial(() async {
    if (!_valid(generation)) return;
    await _persist([
      for (final r in _records) r['id'] == id ? {...r, ...changes} : r,
    ], generation);
  });
  Future<void> sync({bool force = false}) async {
    if (_syncing || _disposed) return;
    _syncing = true;
    try {
      await start();
      final generation = _generation;
      if (_owner == null || token() == null || !_valid(generation)) return;
      error = null;
      _notify();
      final batch = records
          .where((r) => r['state'] == 'PENDING')
          .take(20)
          .toList();
      for (final item in batch) {
        if (!_valid(generation)) break;
        if (!['LOCATION_FIX', 'EVIDENCE_REFERENCE'].contains(item['kind']) ||
            item['body']?['command_type'] != item['kind']) {
          await _update(item['id'] as String, {
            'state': 'REJECTED',
            'reason': 'This action needs online verification in the delivery.',
          }, generation);
          continue;
        }
        final timestamp = DateTime.tryParse(
          item['body']?['client_timestamp']?.toString() ?? '',
        );
        if (timestamp == null ||
            DateTime.now().toUtc().difference(timestamp) >
                const Duration(hours: 24)) {
          await _update(item['id'] as String, {
            'state': 'EXPIRED',
            'reason': 'This update is older than the server replay window.',
          }, generation);
          continue;
        }
        final next = DateTime.tryParse(
          item['next_attempt_at']?.toString() ?? '',
        );
        if (!force && next != null && next.isAfter(DateTime.now())) continue;
        final attempts = (item['attempts'] as num? ?? 0).toInt() + 1;
        await _update(item['id'] as String, {'attempts': attempts}, generation);
        try {
          final response = await client
              .post(
                Uri.parse('$origin/api/v1/provider/offline-commands'),
                headers: {
                  'Authorization': 'Bearer ${token() ?? ''}',
                  'Content-Type': 'application/json',
                  'Idempotency-Key': item['id'] as String,
                },
                body: jsonEncode(item['body']),
              )
              .timeout(const Duration(seconds: 25));
          if (!_valid(generation)) break;
          Map<String, dynamic>? envelope;
          try {
            envelope = Map<String, dynamic>.from(
              jsonDecode(response.body) as Map,
            );
          } catch (_) {}
          if (response.statusCode >= 200 &&
              response.statusCode < 300 &&
              envelope?['success'] != false) {
            final data = envelope?['data'] as Map?;
            final state = data?['state'];
            if (![
                  'ACCEPTED',
                  'REJECTED',
                  'PENDING_EVIDENCE_SYNC',
                ].contains(state) ||
                data?['command_type'] != item['kind'])
              throw const FormatException('Unconfirmed sync response');
            await _update(item['id'] as String, {
              'state': state,
              'reason_code': data?['reason_code'],
              'server_version': data?['server_version'],
            }, generation);
          } else if ([400, 404, 409, 422].contains(response.statusCode)) {
            await _update(item['id'] as String, {
              'state': 'REJECTED',
              'reason':
                  (envelope?['error'] as Map?)?['message']?.toString() ??
                  'The server could not accept this update.',
              'reason_code': (envelope?['error'] as Map?)?['code'],
            }, generation);
          } else {
            throw const FormatException('Sync temporarily unavailable');
          }
        } catch (_) {
          if (!_valid(generation)) break;
          final seconds = 15 * (1 << attempts.clamp(0, 4));
          await _update(item['id'] as String, {
            'next_attempt_at': DateTime.now()
                .add(Duration(seconds: seconds))
                .toUtc()
                .toIso8601String(),
          }, generation);
          error =
              'Updates are saved on this phone. Sync will retry when the server is reachable.';
          _notify();
          break;
        }
      }
    } catch (_) {
      if (!_disposed) {
        error = 'Saved updates could not be synced. Try again.';
        _notify();
      }
    } finally {
      _syncing = false;
      _notify();
    }
  }

  Future<void> dismiss(String id) async {
    final generation = _generation;
    await _serial(() async {
      if (!_valid(generation)) return;
      try {
        await _persist(_records.where((r) => r['id'] != id).toList(), generation);
      } catch (_) {
        error = 'This saved update could not be removed. Try again.';
        _notify();
      }
    });
  }

  void stop() {
    _generation++;
    _timer?.cancel();
    _timer = null;
    _owner = null;
    _device = null;
    _records = [];
    _loading = null;
    error = null;
    _notify();
  }

  @override
  void dispose() {
    _disposed = true;
    stop();
    super.dispose();
  }
}
