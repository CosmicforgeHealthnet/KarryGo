import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:dispatch_provider/features/trips/data/trips_api.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

const _config = ApiCoreConfig(baseUrl: 'https://dispatch.example.test');

void main() {
  test('malformed success envelope is not an empty trips list', () async {
    final api = TripsApi(_config, client: MockClient((_) async {
      return http.Response(jsonEncode({
        'success': true,
        'data': {'unexpected': 'shape'},
      }), 200);
    }));
    final response = await api.listTrips(accessToken: 'test-token');
    expect(response.isFailure, true);
  });

  test('bad trip elements cannot silently disappear', () async {
    final api = TripsApi(_config, client: MockClient((_) async {
      return http.Response(jsonEncode({
        'success': true,
        'data': [{'id': 'trip-1'}, 7],
      }), 200);
    }));
    expect((await api.listTrips(accessToken: 'test-token')).isFailure, true);
  });

  test('real empty trips envelope is still valid', () async {
    final api = TripsApi(_config, client: MockClient((_) async {
      return http.Response(jsonEncode({
        'success': true,
        'data': [],
      }), 200);
    }));
    final response = await api.listTrips(accessToken: 'test-token');
    expect(response.isSuccess, true);
    response.when(
      success: (items) => expect(items, isEmpty),
      failure: (_) => fail('Explicit empty list should succeed'),
    );
  });

  test('invalid active-trip payload is not treated as no active trip',
      () async {
    final api = TripsApi(_config, client: MockClient((_) async {
      return http.Response(jsonEncode({
        'success': true,
        'data': 'broken',
      }), 200);
    }));
    expect((await api.getActiveTrip(accessToken: 'test-token')).isFailure, true);
  });
}
