import 'dart:convert';

import '../../home/mock/dispatch_home_mock.dart';
import '../../trips/data/trip_model.dart';
import '../../trips/mock/trips_tab_mock.dart';

class MockOperationResult {
  const MockOperationResult(this.status, this.data, [this.error]);
  final int status;
  final Map<String, dynamic>? data;
  final String? error;
}

class ProviderOperationsMock {
  final _hubs = <String, Map<String, dynamic>>{};
  final _hubAssets = <String, String>{};
  final _trips = <String, Map<String, dynamic>>{};
  final _confirmedStops = <String, Set<String>>{};
  final _captureKinds = <String, String>{};
  final _scans = <String, Set<String>>{};
  final _replays = <String, MockOperationResult>{};

  MockOperationResult? handle(
    String method,
    String path,
    Map<String, dynamic> body,
    String? key,
  ) {
    final match = RegExp(
      r'^/api/v1/provider/trips/([^/]+)(.*)$',
    ).firstMatch(path);
    if (match == null) return null;
    final id = Uri.decodeComponent(match[1]!);
    final suffix = match[2]!;
    final candidates = [
      if (DispatchHomeMock.activeTrip != null) DispatchHomeMock.activeTrip!,
      ...TripsTabMock.seed(),
    ];
    final seed = candidates.where((trip) => trip.id == id).firstOrNull;
    if (!_trips.containsKey(id)) {
      if (seed == null)
        return const MockOperationResult(404, null, 'Delivery not found.');
      _trips[id] = seed.toJson();
      if (seed.statusCode.isActive)
        _trips[id]!['mission'] = _mission(id, seed.rawStatus);
    }
    final trip = _trips[id]!;
    if (method == 'GET' && suffix.isEmpty)
      return MockOperationResult(200, _copy(trip));
    final mission = trip['mission'] as Map<String, dynamic>?;
    if (mission == null)
      return const MockOperationResult(
        409,
        null,
        'This delivery is no longer active.',
      );
    final fingerprint = '$method:$path:${jsonEncode(body)}';
    if (key != null && _replays.containsKey('$key:$fingerprint'))
      return _replays['$key:$fingerprint'];
    final handover = mission['handover'] as Map<String, dynamic>;
    final stops = (mission['stops'] as List).cast<Map<String, dynamic>>();
    final parcels = (mission['parcels'] as List).cast<Map<String, dynamic>>();
    final stopMatch = RegExp(r'^/stops/([^/]+)/(.+)$').firstMatch(suffix);
    Map<String, dynamic> result = {};
    if (suffix == '/mock-hub-hold') {
      _hubs[id] = {
        'hub_ref': 'Demo approved hub',
        'state': 'APPROVED',
        'manifest_ref': mission['manifest']?['reference'] ?? 'demo-manifest',
        'parcel_refs': parcels.map((p) => p['reference']).toList(),
        'parcel_codes': parcels.map((p) => p['scannable_code']).toList(),
        'allowed_stage': 'ARRIVAL',
      };
      mission['recovery'] = {
        ...(mission['recovery'] as Map? ?? {}),
        'action': 'HUB_HOLD',
      };
      result = _hubs[id]!;
    } else if (method == 'GET' && suffix == '/hub-hold') {
      if (_hubs[id] == null)
        return const MockOperationResult(
          404,
          null,
          'Approved hub hold was not found.',
        );
      return MockOperationResult(200, _copy(_hubs[id]!));
    } else if (suffix == '/hub-hold/evidence-content') {
      final hub = _hubs[id];
      if (hub == null || hub['allowed_stage'] == '')
        return const MockOperationResult(
          409,
          null,
          'No hub checkpoint is available.',
        );
      if (body['manifest_ref'] != hub['manifest_ref'])
        return const MockOperationResult(
          409,
          null,
          'Hub manifest changed. Refresh this delivery.',
        );
      final ref = 'demo-hub-proof-${_hubAssets.length + 1}';
      _hubAssets[ref] = '$id:${hub['manifest_ref']}';
      result = {'media_ref': ref};
    } else if (suffix == '/hub-hold/checkpoints') {
      final hub = _hubs[id];
      final expected = (hub?['parcel_codes'] as List? ?? [])
          .map((v) => v.toString())
          .toSet();
      final scanned = (body['parcel_codes'] as List? ?? [])
          .map((v) => v.toString())
          .toSet();
      if (hub == null ||
          body['stage'] != hub['allowed_stage'] ||
          expected.length != scanned.length ||
          !scanned.containsAll(expected) ||
          _hubAssets[body['media_ref']] != '$id:${hub['manifest_ref']}')
        return const MockOperationResult(
          409,
          null,
          'Scan every parcel and attach a current hub photo before confirming.',
        );
      hub['state'] = body['stage'] == 'ARRIVAL' ? 'HELD' : 'RELEASED';
      hub['allowed_stage'] = body['stage'] == 'ARRIVAL' ? 'RELEASE' : '';
      result = {'state': hub['state'], 'stage': body['stage']};
    } else if (method == 'GET' && stopMatch?[2] == 'pickup-credential') {
      final stop = stops
          .where(
            (stop) => stop['id'] == stopMatch![1] && stop['type'] == 'PICKUP',
          )
          .firstOrNull;
      if (stop == null)
        return const MockOperationResult(404, null, 'Pickup stop not found.');
      handover['stop_ref'] = stop['stop_ref'];
      result = {
        ...handover,
        'otp': '123456',
        'qr_token': 'DEMO-PICKUP-${stop['stop_ref']}',
      };
    } else if (suffix == '/arrived') {
      trip['status'] = 'arrived_pickup';
      mission['state'] = 'AT_PICKUP';
      result = trip;
    } else if (suffix == '/mock-sender-confirmation') {
      handover['status'] = 'CONSUMED';
      result = {'verified': true};
    } else if (stopMatch?[2] == 'parcels/scan') {
      final parcel = parcels
          .where(
            (parcel) =>
                parcel['scannable_code'] == body['parcel_code'] &&
                parcel['pickup_stop_ref'] == stopMatch![1],
          )
          .firstOrNull;
      if (parcel == null)
        return const MockOperationResult(
          422,
          null,
          'This parcel is not in the expected manifest.',
        );
      parcel['state'] = 'PICKUP_VERIFIED';
      result = {'scanned': true};
    } else if (stopMatch?[2] == 'evidence-session') {
      if (handover['status'] != 'CONSUMED')
        return const MockOperationResult(
          409,
          null,
          'Verify the handover before capturing evidence.',
        );
      result = {
        'capture_session_ref': 'demo-capture-${stopMatch![1]}',
        'stage': body['stage'],
        'manifest_ref': body['manifest_ref'],
        'manifest_version': body['manifest_version'],
        'expires_at': DateTime.now()
            .add(const Duration(minutes: 10))
            .toUtc()
            .toIso8601String(),
      };
    } else if (stopMatch?[2] == 'evidence-content') {
      result = {'media_ref': 'demo-photo-${stopMatch![1]}'};
    } else if (stopMatch?[2] == 'evidence') {
      handover['accepted_evidence_count'] = 1;
      handover['accepted_evidence_types'] = <String>['PHOTO'];
      if (handover['stage'] == 'PICKUP') {
        final confirmed = _confirmedStops.putIfAbsent(id, () => <String>{});
        confirmed.add(stopMatch![1]!);
        final next = stops
            .where(
              (stop) =>
                  stop['type'] == 'PICKUP' && !confirmed.contains(stop['id']),
            )
            .firstOrNull;
        if (next != null) {
          mission['handover'] = {
            ...handover,
            'stop_ref': next['stop_ref'],
            'status': 'ACTIVE',
            'challenge_ref': 'demo-pickup-${next['stop_ref']}',
            'accepted_evidence_count': 0,
            'accepted_evidence_types': <String>[],
            'scanned_parcel_count': 0,
          };
        } else {
          mission['state'] = 'PICKED_UP';
        }
      } else {
        trip['status'] = 'proof_submitted';
        mission['state'] = 'DELIVERED';
      }
      result = {'evidence_ref': 'demo-evidence-${stopMatch![1]}'};
    } else if (suffix == '/start') {
      if (mission['state'] != 'PICKED_UP')
        return const MockOperationResult(
          409,
          null,
          'Finish pickup handover at both stops before starting delivery.',
        );
      trip['status'] = 'in_progress';
      mission['state'] = 'IN_TRANSIT';
      mission['handover'] = {
        ...handover,
        'stage': 'DELIVERY',
        'status': 'ACTIVE',
        'stop_ref': 'dropoff',
        'challenge_ref': 'demo-delivery',
        'accepted_evidence_count': 0,
        'accepted_evidence_types': <String>[],
        'scanned_parcel_count': 0,
        'expires_at': DateTime.now()
            .add(const Duration(minutes: 15))
            .toUtc()
            .toIso8601String(),
      };
      result = trip;
    } else if (suffix == '/delivery/verify' || suffix == '/return/verify') {
      if (body['credential'] != '123456')
        return const MockOperationResult(
          422,
          null,
          'Use 123456 for the demo handover.',
        );
      handover['status'] = 'CONSUMED';
      result = {'status': 'CONSUMED'};
    } else if (suffix == '/delivery-attempts') {
      final recovery = {
        'attempt_id': 'demo-attempt',
        'attempt_state': 'GRACE_PERIOD',
        'grace_expires_at': DateTime.now()
            .add(const Duration(seconds: 30))
            .toUtc()
            .toIso8601String(),
        'source_version': 1,
      };
      mission['recovery'] = recovery;
      result = {...recovery, 'state': 'GRACE_PERIOD'};
    } else if (suffix.endsWith('/contact')) {
      result = {'contact_attempt_id': 'demo-contact', ...body};
    } else if (suffix.endsWith('/fail')) {
      final recovery = mission['recovery'] as Map<String, dynamic>?;
      final until = DateTime.tryParse(
        recovery?['grace_expires_at']?.toString() ?? '',
      );
      if (until != null && until.isAfter(DateTime.now()))
        return const MockOperationResult(
          409,
          null,
          'Wait until the contact period ends.',
        );
      recovery?['attempt_state'] = 'RECEIVER_UNAVAILABLE';
      result = {'state': 'RECEIVER_UNAVAILABLE'};
    } else if (suffix == '/breakdown') {
      mission['state'] = 'REASSIGNING';
      result = {'state': 'REPLACEMENT_REQUESTED'};
    } else if (suffix == '/reroute') {
      result = {'status': 'CURRENT', 'version': 2};
      mission['route'] = result;
    } else if (suffix == '/complete') {
      if (trip['status'] != 'proof_submitted')
        return const MockOperationResult(
          409,
          null,
          'Complete receiver handover and evidence first.',
        );
      trip['status'] = 'completed';
      mission['state'] = 'COMPLETED';
      result = trip;
    } else {
      return const MockOperationResult(
        404,
        null,
        'This operation is not available.',
      );
    }
    mission['version'] = (mission['version'] as int) + 1;
    mission['allowed_actions'] = _actions(trip['status'] as String);
    if (trip['status'] == 'completed') {
      DispatchHomeMock.activeTrip = null;
    } else if (DispatchHomeMock.activeTrip?.id == id) {
      DispatchHomeMock.activeTrip = TripModel.fromJson(trip);
    }
    final response = MockOperationResult(200, _copy(result));
    if (key != null) _replays['$key:$fingerprint'] = response;
    return response;
  }

  static Map<String, dynamic> _copy(Map<String, dynamic> value) =>
      jsonDecode(jsonEncode(value)) as Map<String, dynamic>;
  static List<String> _actions(String status) => switch (status) {
    'assigned' || 'en_route_pickup' => ['ARRIVE_AT_PICKUP', 'REPORT_BREAKDOWN'],
    'arrived_pickup' => [
      'READ_PICKUP_CREDENTIAL',
      'SCAN_PARCELS',
      'START_DELIVERY',
      'REPORT_BREAKDOWN',
    ],
    'in_progress' => [
      'SCAN_PARCELS',
      'VERIFY_DELIVERY',
      'START_DELIVERY_ATTEMPT',
      'REPORT_BREAKDOWN',
    ],
    'proof_submitted' => ['COMPLETE'],
    _ => [],
  };
  static Map<String, dynamic> _mission(String id, String status) => {
    'mission_id': 'demo-mission-$id',
    'delivery_group_id': 'demo-group',
    'state': status == 'in_progress'
        ? 'IN_TRANSIT'
        : status == 'proof_submitted'
        ? 'DELIVERED'
        : 'ASSIGNED',
    'version': 1,
    'stops': [
      {
        'id': 'pickup-a',
        'stop_ref': 'pickup-a',
        'sequence': 1,
        'type': 'PICKUP',
        'location': {'address': '12 Admiralty Way, Lekki'},
      },
      {
        'id': 'pickup-b',
        'stop_ref': 'pickup-b',
        'sequence': 2,
        'type': 'PICKUP',
        'location': {'address': '8 Akin Adesola Street, Victoria Island'},
      },
      {
        'id': 'dropoff',
        'stop_ref': 'dropoff',
        'sequence': 3,
        'type': 'DELIVERY',
        'location': {'address': '22 Bourdillon Road, Ikoyi'},
      },
    ],
    'manifest': {
      'reference': 'demo-manifest',
      'version': 1,
      'status': 'ACTIVE',
    },
    'parcels': [
      for (final letter in ['A', 'B'])
        {
          'reference': 'PARCEL-$letter',
          'scannable_code': 'DEMO-$letter',
          'pickup_stop_ref': letter == 'A' ? 'pickup-a' : 'pickup-b',
          'state': 'READY_FOR_PICKUP',
        },
    ],
    'allowed_actions': _actions(status),
    'handover': {
      'challenge_ref': 'demo-pickup-a',
      'stop_ref': status == 'in_progress' || status == 'proof_submitted'
          ? 'dropoff'
          : 'pickup-a',
      'stage': status == 'in_progress' || status == 'proof_submitted'
          ? 'DELIVERY'
          : 'PICKUP',
      'status': 'ACTIVE',
      'manifest_ref': 'demo-manifest',
      'manifest_version': 1,
      'photo_required': true,
      'signature_required': false,
      'required_evidence_count': 1,
      'accepted_evidence_count': 0,
      'accepted_evidence_types': <String>[],
      'parcel_scans_required': true,
      'expected_parcel_count': 1,
      'scanned_parcel_count': 0,
      'expires_at': DateTime.now()
          .add(const Duration(minutes: 15))
          .toUtc()
          .toIso8601String(),
    },
  };
}
