import 'dart:convert';
import 'provider_operations_mock.dart';

class CommercePickupMock {
  final Map<String, dynamic> _mission = {
    'mission_ref': 'demo-commerce-mission',
    'trip_ref': 'demo-commerce-trip',
    'state': 'ASSIGNED',
    'version': 1,
    'pickup_actions_allowed': true,
    'returns': <Map<String, dynamic>>[],
    'can_start_transfer': false,
    'exceptions': <Map<String, dynamic>>[],
    'evidence': <Map<String, dynamic>>[],
    'stops': [
      for (final i in [1, 2])
        {
          'stop_ref': 'demo-stop-$i',
          'sequence': i,
          'manifest_ref': 'demo-manifest-$i',
          'manifest_version': 1,
          'parcel_refs': ['demo-parcel-$i'],
          'status': 'AWAITING_PICKUP',
          'handover_status': 'PENDING',
          'location': {
            'address': i == 1
                ? 'Island Kitchen, Victoria Island'
                : 'Fresh Basket, Lekki',
          },
        },
    ],
    'parcels': [
      for (final i in [1, 2])
        {
          'parcel_ref': 'demo-parcel-$i',
          'pickup_stop_ref': 'demo-stop-$i',
          'manifest_ref': 'demo-manifest-$i',
          'manifest_version': 1,
          'package_count': i,
          'status': 'AWAITING_PICKUP',
          'custodian_type': 'VENDOR',
        },
    ],
  };
  final _attachments = <String, Set<String>>{};
  final _exceptionActions = <String, List<String>>{
    'VENDOR_NOT_READY': ['WAIT', 'RETRY_PICKUP', 'RESCHEDULE', 'REPLAN'],
    'PACKAGE_MISMATCH': ['RETURN_TO_VENDOR', 'SUPPORT'],
    'PROHIBITED_ITEM': ['RETURN_TO_VENDOR', 'SUPPORT'],
    'RESTRICTED_ITEM': ['RETURN_TO_VENDOR', 'SUPPORT'],
    'DAMAGED_PACKAGE': ['RETURN_TO_VENDOR', 'SUPPORT', 'REPLAN'],
    'RECEIVER_UNAVAILABLE': [
      'WAIT',
      'RETRY_DELIVERY',
      'RESCHEDULE',
      'RETURN_TO_SENDER',
      'RETURN_TO_VENDOR',
    ],
    'RECEIVER_REFUSED': ['RETURN_TO_SENDER', 'RETURN_TO_VENDOR', 'SUPPORT'],
    'WRONG_ADDRESS': ['WAIT', 'RESCHEDULE', 'REPLAN', 'SUPPORT'],
    'ADDRESS_INACCESSIBLE': ['WAIT', 'RESCHEDULE', 'REPLAN', 'SUPPORT'],
    'VEHICLE_BREAKDOWN': [
      'PROVIDER_REPLACEMENT',
      'RETURN_TO_VENDOR',
      'SUPPORT',
    ],
    'PROVIDER_ILLNESS': ['PROVIDER_REPLACEMENT', 'RETURN_TO_VENDOR', 'SUPPORT'],
    'SAFETY_INCIDENT': ['PROVIDER_REPLACEMENT', 'RETURN_TO_VENDOR', 'SUPPORT'],
    'WEATHER_ROAD': [
      'WAIT',
      'RESCHEDULE',
      'RETURN_TO_VENDOR',
      'REPLAN',
      'SUPPORT',
    ],
    'SECURITY_RISK': [
      'WAIT',
      'RESCHEDULE',
      'RETURN_TO_VENDOR',
      'REPLAN',
      'SUPPORT',
    ],
  };
  bool _incoming = false;
  final _recoveryReplays = <String, (String, Map<String, dynamic>)>{};
  Map<String, dynamic> _projection() {
    final view = Map<String, dynamic>.from(
      jsonDecode(jsonEncode(_mission)) as Map,
    );
    final parcels = (view['parcels'] as List).cast<Map<String, dynamic>>();
    final eligible = parcels
        .where(
          (p) =>
              p['custodian_type'] == 'PROVIDER' &&
              [
                'PICKED_UP',
                'IN_TRANSIT',
                'RETURN_REQUIRED',
              ].contains(p['status']),
        )
        .map((p) => p['parcel_ref'])
        .toList();
    view['transfer_parcel_refs'] = eligible;
    view['return_parcel_refs'] =
        view['can_start_transfer'] == true ||
            (view['transfer'] as Map?)?['state'] == 'CHALLENGE_ISSUED' ||
            view['state'] == 'RETURNED'
        ? <String>[]
        : eligible;
    if (view['transfer'] != null)
      (view['transfer'] as Map)['role'] = _incoming ? 'INCOMING' : 'OUTGOING';
    if (view['state'] == 'PICKUP_BLOCKED')
      view['pickup_actions_allowed'] = false;
    view['exception_options'] =
        ['COMPLETED', 'CANCELLED', 'RETURNED'].contains(view['state']) ||
            (_incoming &&
                (view['transfer'] as Map?)?['state'] == 'CHALLENGE_ISSUED')
        ? <Map<String, dynamic>>[]
        : [
            for (final entry in _exceptionActions.entries)
              if (entry.key != 'VENDOR_NOT_READY' ||
                  ['ASSIGNED', 'PICKING_UP'].contains(view['state']))
                {
                  'reason_code': entry.key,
                  'recovery_actions': entry.value,
                  'photo_required': [
                    'DAMAGED_PACKAGE',
                    'PACKAGE_MISMATCH',
                  ].contains(entry.key),
                },
          ];
    return view;
  }

  MockOperationResult? _recovery(
    String method,
    String path,
    Map<String, dynamic> body,
    String? key,
  ) {
    const base = '/api/v1/provider/commerce-missions/demo-commerce-mission';
    Map<String, dynamic> copy(Map<String, dynamic> value) =>
        Map<String, dynamic>.from(jsonDecode(jsonEncode(value)) as Map);
    if (method == 'GET' &&
        path == '/api/v1/provider/commerce-missions/by-trip/demo-commerce-trip')
      return MockOperationResult(200, _projection());
    final transfer = _mission['transfer'] as Map<String, dynamic>?;
    if (method == 'GET' &&
        path == '$base/provider-transfers/demo-transfer/credential') {
      if (_incoming || transfer?['state'] != 'CHALLENGE_ISSUED')
        return const MockOperationResult(
          404,
          null,
          'An active outgoing transfer code was not found.',
        );
      return MockOperationResult(200, {
        'challenge_code': '123456',
        'transfer_ref': 'demo-transfer',
        'challenge_expires_at': transfer!['expires_at'],
      });
    }
    if (!path.startsWith(base) || path.contains('/stops/')) return null;
    if (method != 'POST' ||
        key == null ||
        key.isEmpty ||
        body['operation_ref'] != key)
      return const MockOperationResult(
        400,
        null,
        'Invalid operation reference.',
      );
    final fingerprint = '$path:${jsonEncode(body)}';
    final prior = _recoveryReplays[key];
    if (prior != null)
      return prior.$1 == fingerprint
          ? MockOperationResult(200, copy(prior.$2))
          : const MockOperationResult(
              409,
              null,
              'Operation reference already used.',
            );
    if (path.endsWith('/evidence-content') ||
        path.endsWith('/evidence') ||
        path.endsWith('/exceptions')) {
      final refs = (body['parcel_refs'] as List? ?? [])
          .map((v) => v.toString())
          .toSet();
      final allowed = (_mission['parcels'] as List)
          .cast<Map>()
          .map((p) => p['parcel_ref'])
          .toSet();
      if (refs.isEmpty ||
          !allowed.containsAll(refs) ||
          (_projection()['exception_options'] as List).isEmpty)
        return const MockOperationResult(
          409,
          null,
          'Evidence must belong to your assigned mission and parcels.',
        );
      Map<String, dynamic> result;
      if (path.endsWith('/evidence-content')) {
        final ref = 'demo-proof-${_attachments.length + 1}';
        _attachments[ref] = refs;
        result = {'media_ref': ref};
      } else {
        final exception = path.endsWith('/exceptions');
        if (!exception &&
            ![
              'DAMAGE',
              'DISCREPANCY',
              'FAILED_ATTEMPT',
            ].contains(body['evidence_type']))
          return const MockOperationResult(
            409,
            null,
            'Use the dedicated handover workflow for this evidence type.',
          );
        final evidence = exception
            ? Map<String, dynamic>.from(body['evidence'] as Map? ?? {})
            : body;
        final reason =
            (exception ? body['reason_code'] : evidence['reason_code'])
                ?.toString();
        final options = (_projection()['exception_options'] as List)
            .cast<Map>();
        final option = options
            .where((o) => o['reason_code'] == reason)
            .firstOrNull;
        final media = (evidence['media_refs'] as List? ?? [])
            .map((v) => v.toString())
            .toList();
        if (DateTime.tryParse(evidence['captured_at']?.toString() ?? '') ==
                null ||
            option == null ||
            (exception &&
                (body['expected_version'] != _mission['version'] ||
                    !(option['recovery_actions'] as List).contains(
                      body['recovery_action'],
                    ))) ||
            (option['photo_required'] == true && media.isEmpty) ||
            media.any(
              (m) =>
                  _attachments[m] == null ||
                  _attachments[m]!.length != refs.length ||
                  !_attachments[m]!.containsAll(refs),
            ))
          return const MockOperationResult(
            409,
            null,
            'Choose valid recovery and attach photo evidence for the exact parcels.',
          );
        final record = {
          'evidence_id':
              'demo-evidence-${(_mission['evidence'] as List).length + 1}',
          'evidence_type': reason == 'DAMAGED_PACKAGE'
              ? 'DAMAGE'
              : reason == 'PACKAGE_MISMATCH'
              ? 'DISCREPANCY'
              : 'FAILED_ATTEMPT',
          'parcel_refs': refs.toList(),
          'captured_at': evidence['captured_at'],
          'attachment_count': media.length,
          'description': evidence['description'] ?? '',
        };
        (_mission['evidence'] as List).insert(0, record);
        result = record;
        if (exception) {
          result = {
            'exception_id':
                'demo-issue-${(_mission['exceptions'] as List).length + 1}',
            'reason_code': reason,
            'recovery_action': body['recovery_action'],
            'state': 'RECOVERY_SELECTED',
            'parcel_refs': refs.toList(),
            'created_at': DateTime.now().toUtc().toIso8601String(),
          };
          (_mission['exceptions'] as List).insert(0, result);
          _mission['state'] =
              [
                'VEHICLE_BREAKDOWN',
                'PROVIDER_ILLNESS',
                'SAFETY_INCIDENT',
              ].contains(reason)
              ? 'REPLACEMENT_REQUIRED'
              : [
                  'PACKAGE_MISMATCH',
                  'PROHIBITED_ITEM',
                  'RESTRICTED_ITEM',
                ].contains(reason)
              ? 'PICKUP_BLOCKED'
              : ['RECEIVER_UNAVAILABLE', 'RECEIVER_REFUSED'].contains(reason)
              ? 'DELIVERY_EXCEPTION'
              : 'RECOVERY_REQUIRED';
        }
        _mission['version'] = (_mission['version'] as int) + 1;
      }
      final snapshot = copy(result);
      _recoveryReplays[key] = (fingerprint, snapshot);
      return MockOperationResult(200, snapshot);
    }
    if (body['expected_version'] != _mission['version'])
      return const MockOperationResult(
        409,
        null,
        'Refresh this delivery before continuing.',
      );
    final parcels = (_mission['parcels'] as List).cast<Map<String, dynamic>>();
    final requested = (body['parcel_refs'] as List? ?? [])
        .map((v) => v.toString())
        .toSet();
    final eligible = (_projection()['transfer_parcel_refs'] as List)
        .map((v) => v.toString())
        .toSet();
    final returns = (_mission['returns'] as List).cast<Map<String, dynamic>>();
    Map<String, dynamic> result;
    if (path.endsWith('/mock-replacement')) {
      if (_mission['state'] != 'IN_TRANSIT')
        return const MockOperationResult(
          409,
          null,
          'Finish both pickups before requesting a replacement.',
        );
      _mission['can_start_transfer'] = true;
      result = _mission;
    } else if (path.endsWith('/mock-incoming-provider')) {
      if (transfer?['state'] != 'CHALLENGE_ISSUED')
        return const MockOperationResult(
          409,
          null,
          'Prepare the handover first.',
        );
      _incoming = true;
      result = _mission;
    } else if (path.endsWith('/provider-transfers/start')) {
      if (_mission['can_start_transfer'] != true ||
          requested.length != eligible.length ||
          !requested.containsAll(eligible) ||
          body['package_count'] !=
              parcels
                  .where((p) => eligible.contains(p['parcel_ref']))
                  .fold<int>(0, (sum, p) => sum + (p['package_count'] as int)))
        return const MockOperationResult(
          409,
          null,
          'Confirm all parcels with the assigned replacement.',
        );
      final expires = DateTime.now()
          .add(const Duration(minutes: 5))
          .toUtc()
          .toIso8601String();
      _mission['transfer'] = {
        'transfer_ref': 'demo-transfer',
        'state': 'CHALLENGE_ISSUED',
        'parcel_refs': requested.toList(),
        'package_count': body['package_count'],
        'expires_at': expires,
      };
      _mission['can_start_transfer'] = false;
      _mission['state'] = 'TRANSFER_PENDING';
      _mission['pickup_actions_allowed'] = false;
      result = {
        'transfer_ref': 'demo-transfer',
        'challenge_code': '123456',
        'challenge_expires_at': expires,
      };
    } else if (path.endsWith('/provider-transfers/demo-transfer/complete')) {
      if (!_incoming ||
          transfer?['state'] != 'CHALLENGE_ISSUED' ||
          body['code'] != '123456' ||
          requested.length != (transfer!['parcel_refs'] as List).length ||
          !requested.containsAll(
            (transfer['parcel_refs'] as List).cast<String>(),
          ) ||
          body['package_count'] != transfer['package_count'] ||
          body['latitude'] != 6.45 ||
          body['longitude'] != 3.4)
        return const MockOperationResult(
          409,
          null,
          'Only the replacement can accept the exact parcels with the correct code and location.',
        );
      transfer['state'] = 'COMPLETED';
      _mission['state'] = 'IN_TRANSIT';
      _mission['pickup_actions_allowed'] = true;
      result = _mission;
    } else if (path == '$base/returns') {
      final type = body['product_class'];
      final disposition = body['disposition'];
      final policies = type == 'PREPARED_FOOD'
          ? ['RETURN_TO_VENDOR', 'FOOD_DISPOSAL_POLICY', 'SUPPORT_REVIEW']
          : type == 'SHOP_RETAIL'
          ? ['RETURN_TO_VENDOR', 'SHOP_INSPECTION', 'SUPPORT_REVIEW']
          : <String>[];
      if (requested.isEmpty ||
          !eligible.containsAll(requested) ||
          !policies.contains(disposition) ||
          returns.any((r) => r['state'] != 'RETURNED') ||
          (_projection()['return_parcel_refs'] as List).isEmpty ||
          DateTime.tryParse(
                (body['evidence'] as Map?)?['captured_at']?.toString() ?? '',
              ) ==
              null)
        return const MockOperationResult(
          409,
          null,
          'Select parcels in your custody and the correct return policy.',
        );
      final record = {
        'return_id': 'demo-return-${returns.length + 1}',
        'state': 'IN_RETURN',
        'product_class': type,
        'disposition': disposition,
        'parcel_refs': requested.toList(),
      };
      (_mission['returns'] as List).add(record);
      for (final parcel in parcels.where(
        (p) => requested.contains(p['parcel_ref']),
      )) {
        parcel['status'] = 'RETURN_REQUIRED';
      }
      _mission['state'] = 'RETURNING';
      _mission['pickup_actions_allowed'] = false;
      result = record;
    } else if (path.contains('/returns/') && path.endsWith('/complete')) {
      final id = path.split('/').reversed.skip(1).first;
      final record = returns.where((r) => r['return_id'] == id).firstOrNull;
      if (record == null ||
          record['state'] != 'IN_RETURN' ||
          requested.length != (record['parcel_refs'] as List).length ||
          !requested.containsAll(
            (record['parcel_refs'] as List).cast<String>(),
          ))
        return const MockOperationResult(
          409,
          null,
          'Return completion requires the exact original parcels.',
        );
      record['state'] = 'RETURNED';
      for (final parcel in parcels.where(
        (p) => requested.contains(p['parcel_ref']),
      )) {
        parcel['status'] = 'RETURNED';
        parcel['custodian_type'] = 'RETURN';
      }
      _mission['state'] = 'RETURNED';
      result = record;
    } else {
      return const MockOperationResult(404, null, 'Demo action not found.');
    }
    _mission['version'] = (_mission['version'] as int) + 1;
    final snapshot = copy(result);
    _recoveryReplays[key] = (fingerprint, snapshot);
    return MockOperationResult(200, snapshot);
  }

  final _replays = <String, (String, Map<String, dynamic>)>{};
  MockOperationResult? handle(
    String method,
    String path,
    Map<String, dynamic> body,
    String? key,
  ) {
    const base = '/api/v1/provider/commerce-missions';
    if (!path.startsWith(base)) return null;
    final recovery = _recovery(method, path, body, key);
    if (recovery != null) return recovery;
    Map<String, dynamic> copy(Map<String, dynamic> value) =>
        Map<String, dynamic>.from(jsonDecode(jsonEncode(value)) as Map);
    if (method == 'GET' && path == '$base/by-trip/demo-commerce-trip')
      return MockOperationResult(200, copy(_mission));
    final match = RegExp(
      '^$base'
      r'/demo-commerce-mission/stops/([^/]+)/(arrive|complete|mock-sender-confirmation)$',
    ).firstMatch(path);
    if (method != 'POST' || match == null)
      return const MockOperationResult(404, null, 'Demo mission not found.');
    if (key == null || key.isEmpty || body['operation_ref'] != key)
      return const MockOperationResult(
        400,
        null,
        'Operation reference must match the request key.',
      );
    final fingerprint = '$path:${jsonEncode(body)}';
    final prior = _replays[key];
    if (prior != null)
      return prior.$1 == fingerprint
          ? MockOperationResult(200, copy(prior.$2))
          : const MockOperationResult(
              409,
              null,
              'Operation reference already used.',
            );
    if (_mission['state'] == 'PICKUP_BLOCKED')
      return const MockOperationResult(
        409,
        null,
        'Pickup is blocked until the reported package issue is resolved.',
      );
    final stops = (_mission['stops'] as List).cast<Map<String, dynamic>>();
    final next = stops.where((s) => s['status'] != 'PICKED_UP').firstOrNull;
    if (next == null || next['stop_ref'] != match[1])
      return const MockOperationResult(
        409,
        null,
        'Complete the current pickup first.',
      );
    if (body['expected_version'] != _mission['version'])
      return const MockOperationResult(
        409,
        null,
        'Refresh this delivery before continuing.',
      );
    Map<String, dynamic> result;
    if (match[2] == 'arrive') {
      next['status'] = 'ARRIVED';
      next['handover_status'] = 'ISSUED';
      _mission['state'] = 'PICKING_UP';
      result = {
        'stop_ref': next['stop_ref'],
        'mission_ref': _mission['mission_ref'],
        'code': '123456',
        'expires_at': DateTime.now()
            .add(const Duration(minutes: 5))
            .toUtc()
            .toIso8601String(),
      };
    } else if (match[2] == 'mock-sender-confirmation') {
      if (next['handover_status'] != 'ISSUED')
        return const MockOperationResult(
          409,
          null,
          'Request the pickup code first.',
        );
      next['status'] = 'READY_FOR_HANDOVER';
      next['handover_status'] = 'VERIFIED';
      result = _mission;
    } else {
      final parcels = (_mission['parcels'] as List)
          .cast<Map<String, dynamic>>()
          .where((p) => p['pickup_stop_ref'] == next['stop_ref'])
          .toList();
      final refs = (body['parcel_refs'] as List? ?? [])
          .map((v) => v.toString())
          .toSet();
      if (next['handover_status'] != 'VERIFIED' ||
          body['manifest_ref'] != next['manifest_ref'] ||
          body['manifest_version'] != next['manifest_version'] ||
          refs.length != parcels.length ||
          parcels.any((p) => !refs.contains(p['parcel_ref'])) ||
          body['package_count'] !=
              parcels.fold<int>(
                0,
                (sum, p) => sum + (p['package_count'] as int),
              )) {
        return const MockOperationResult(
          409,
          null,
          'Sender approval and exact parcel confirmation are required.',
        );
      }
      next['status'] = 'PICKED_UP';
      next['handover_status'] = 'CONSUMED';
      for (final p in parcels) {
        p['status'] = 'PICKED_UP';
        p['custodian_type'] = 'PROVIDER';
      }
      if (stops.every((s) => s['status'] == 'PICKED_UP'))
        _mission['state'] = 'IN_TRANSIT';
      result = _mission;
    }
    _mission['version'] = (_mission['version'] as int) + 1;
    final snapshot = copy(result);
    _replays[key] = (fingerprint, snapshot);
    return MockOperationResult(200, snapshot);
  }
}
