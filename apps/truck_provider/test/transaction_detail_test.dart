import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:truck_provider/features/earnings/models/earnings_models.dart';
import 'package:truck_provider/features/earnings/state/provider_earnings_controller.dart';
import 'package:truck_provider/features/earnings/ui/provider_transaction_detail_screen.dart';
import 'package:truck_provider/features/home/data/provider_api.dart';

ProviderEarningsController _controller({List<String>? paths}) {
  final mock = MockClient((req) async {
    paths?.add(req.url.path);
    if (req.url.path.contains('/provider/bookings/')) {
      return http.Response(
        jsonEncode({
          'success': true,
          'data': {
            'id': 'b1234567',
            'pickup_address': '24 Awawu str, Ikotun',
            'pickup_lat': 6.5, 'pickup_lng': 3.3,
            'dropoff_address': '23 Akeju Victoria Island',
            'dropoff_lat': 6.4, 'dropoff_lng': 3.4,
            'cargo_type': 'general', 'cargo_weight_kg': 100,
            'distance_km': 21, 'fare_estimate_kobo': 700,
            'receiver_name': 'Anthonia Dipson', 'status': 'completed',
            'created_at': '2026-05-22T10:00:00Z',
          },
        }),
        200,
      );
    }
    return http.Response(jsonEncode({'success': true, 'data': {}}), 200);
  });
  return ProviderEarningsController(
    api: ProviderApi(config: const ApiCoreConfig(baseUrl: 'http://test/api/v1/hauling'), client: mock),
    accessToken: () => 'token',
  );
}

EarningsTransaction _txn({required bool trip, String status = 'completed'}) => EarningsTransaction(
      id: 'txn123569876',
      bookingId: trip ? 'b1234567' : '',
      kind: 'credit',
      title: trip ? 'Anthonia Dipson' : 'Withdrawal',
      subtitle: 'Ikeja',
      amountKobo: 2400000,
      status: status,
      isTrip: trip,
      occurredAt: DateTime(2026, 5, 22, 12),
    );

Future<void> _pump(WidgetTester tester, ProviderTransactionDetailScreen s) async {
  await tester.binding.setSurfaceSize(const Size(430, 932));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(MaterialApp(home: s));
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('shows status, amount and commission breakdown', (tester) async {
    await _pump(tester, ProviderTransactionDetailScreen(txn: _txn(trip: false), controller: _controller()));

    expect(find.text('Transaction Details'), findsWidgets);
    expect(find.text('Successful'), findsOneWidget);
    expect(find.text('+₦24,000.00'), findsWidgets);
    expect(find.text('Commission'), findsOneWidget);
    expect(find.text('-10%'), findsOneWidget);
    // Net = 24,000 - 10% = 21,600.
    expect(find.text('₦21,600.00'), findsOneWidget);
    expect(find.text('Contact Support'), findsOneWidget);
  });

  testWidgets('fetches and renders the trip section for a trip transaction', (tester) async {
    final paths = <String>[];
    await _pump(tester, ProviderTransactionDetailScreen(txn: _txn(trip: true), controller: _controller(paths: paths)));

    expect(paths.any((p) => p.contains('/provider/bookings/b1234567')), isTrue);
    expect(find.text('Anthonia Dipson'), findsWidgets);
    expect(find.text('Pick-up'), findsOneWidget);
    expect(find.text('Drop off'), findsOneWidget);
    expect(find.text('Trip Fee:'), findsOneWidget);
  });

  testWidgets('maps pending and failed statuses', (tester) async {
    await _pump(tester, ProviderTransactionDetailScreen(txn: _txn(trip: false, status: 'pending'), controller: _controller()));
    expect(find.text('Pending'), findsOneWidget);
  });
}
