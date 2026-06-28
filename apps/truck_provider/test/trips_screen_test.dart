import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:truck_provider/features/home/data/provider_api.dart';
import 'package:truck_provider/features/trips/state/provider_trips_controller.dart';
import 'package:truck_provider/features/trips/ui/provider_trips_screen.dart';

String _iso(DateTime d) => d.toUtc().toIso8601String();

Map<String, dynamic> _booking({
  required String id,
  required String status,
  String receiver = 'Anthonia Dipson',
}) {
  return {
    'id': id,
    'pickup_address': '24 Awawu str, Abaranje Ikotun lagos.',
    'pickup_lat': 6.5,
    'pickup_lng': 3.3,
    'dropoff_address': '23 Akeju Victoria Island',
    'dropoff_lat': 6.4,
    'dropoff_lng': 3.4,
    'cargo_type': 'general',
    'cargo_weight_kg': 750,
    'distance_km': 21.0,
    'fare_estimate_kobo': 700,
    'receiver_name': receiver,
    'status': status,
    'created_at': _iso(DateTime(2026, 3, 6, 11, 59)),
  };
}

void main() {
  ProviderTripsController makeController(List<Map<String, dynamic>> bookings) {
    final mock = MockClient((req) async {
      return http.Response(jsonEncode({'success': true, 'data': bookings}), 200);
    });
    final api = ProviderApi(
      config: const ApiCoreConfig(baseUrl: 'http://test/api/v1/hauling'),
      client: mock,
    );
    return ProviderTripsController(api: api, accessToken: () => 'token');
  }

  test('buckets bookings into completed / ongoing / cancelled', () async {
    final c = makeController([
      _booking(id: 'a', status: 'completed'),
      _booking(id: 'b', status: 'delivered'),
      _booking(id: 'c', status: 'en_route_pickup'),
      _booking(id: 'd', status: 'accepted'),
      _booking(id: 'e', status: 'cancelled'),
      _booking(id: 'f', status: 'unmatched'),
    ]);

    await c.load();

    expect(c.completed.map((b) => b.id), containsAll(['a', 'b']));
    expect(c.ongoing.map((b) => b.id), containsAll(['c', 'd']));
    expect(c.cancelled.map((b) => b.id), containsAll(['e', 'f']));
    expect(c.completed.length, 2);
    expect(c.ongoing.length, 2);
    expect(c.cancelled.length, 2);
  });

  testWidgets('renders My Trips header and a completed trip card', (tester) async {
    final c = makeController([_booking(id: 'a', status: 'completed')]);

    await tester.pumpWidget(MaterialApp(home: ProviderTripsScreen(controller: c)));
    await tester.pumpAndSettle();

    expect(find.text('My Trips'), findsOneWidget);
    expect(find.text('Completed'), findsWidgets);
    expect(find.text('Anthonia Dipson'), findsOneWidget);
  });

  testWidgets('shows empty state when a tab has no trips', (tester) async {
    final c = makeController([_booking(id: 'a', status: 'completed')]);

    await tester.pumpWidget(MaterialApp(home: ProviderTripsScreen(controller: c)));
    await tester.pumpAndSettle();

    // Switch to the Ongoing tab (no ongoing trips in the data).
    await tester.tap(find.text('Ongoing'));
    await tester.pumpAndSettle();

    expect(find.text('No ongoing trips.'), findsOneWidget);
  });
}
