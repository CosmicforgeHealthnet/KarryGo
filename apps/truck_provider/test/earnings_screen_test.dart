import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:truck_provider/features/earnings/state/provider_earnings_controller.dart';
import 'package:truck_provider/features/earnings/ui/provider_earnings_screen.dart';
import 'package:truck_provider/features/home/data/provider_api.dart';

String _iso(DateTime d) => d.toUtc().toIso8601String();

void main() {
  late String requestedPath;

  ProviderEarningsController makeController(Map<String, dynamic> data) {
    final mock = MockClient((req) async {
      requestedPath = req.url.path;
      return http.Response(jsonEncode({'success': true, 'data': data}), 200);
    });
    final api = ProviderApi(
      config: const ApiCoreConfig(baseUrl: 'http://test/api/v1/hauling'),
      client: mock,
    );
    return ProviderEarningsController(api: api, accessToken: () => 'token');
  }

  Map<String, dynamic> sampleData() {
    final today = DateTime.now();
    final todayAt = DateTime(today.year, today.month, today.day, 12, 0, 23);
    return {
      'available_balance_kobo': 20400000,
      'pending_balance_kobo': 5400000,
      'today_earnings_kobo': 5400000,
      'trips_completed_today': 3,
      'hours_online': 1,
      'transactions': [
        {
          'id': 't1', 'booking_id': 't1', 'kind': 'credit',
          'title': 'Anthonia Dipson', 'subtitle': '23rd avenue Ikeja, Lagos Nigeria',
          'amount_kobo': 2400000, 'status': 'completed', 'is_trip': true,
          'occurred_at': _iso(todayAt),
        },
      ],
    };
  }

  Future<void> pump(WidgetTester tester, ProviderEarningsController controller) async {
    await tester.pumpWidget(MaterialApp(
      home: ProviderEarningsScreen(controller: controller),
    ));
    await tester.pumpAndSettle();
  }

  testWidgets('loads earnings from /provider/earnings and renders balance + stats', (tester) async {
    final controller = makeController(sampleData());
    await pump(tester, controller);

    expect(requestedPath, '/api/v1/hauling/provider/earnings');

    // Header
    expect(find.text('Earnings'), findsOneWidget);
    expect(find.text('Manage your Income here.'), findsOneWidget);

    // Balance card — formatted with thousands separators.
    expect(find.text('₦ 204,000.00'), findsOneWidget);
    expect(find.text("Today's Earnings"), findsOneWidget);

    // Stats
    expect(find.text('Trips Completed Today'), findsOneWidget);
    expect(find.text('3'), findsOneWidget);
    expect(find.text('Hours Online'), findsOneWidget);

    // Transaction
    expect(find.text('Anthonia Dipson'), findsOneWidget);
    expect(find.text('+₦24,000.00'), findsOneWidget);
    expect(find.text('Go to Trips'), findsWidgets);
  });

  testWidgets('eye toggle hides and shows the balance', (tester) async {
    final controller = makeController(sampleData());
    await pump(tester, controller);

    expect(find.text('₦ 204,000.00'), findsOneWidget);

    await tester.tap(find.byIcon(Icons.visibility_outlined));
    await tester.pump();

    expect(find.text('₦ 204,000.00'), findsNothing);
    expect(find.text('₦ ****'), findsOneWidget);

    // Toggling back restores it.
    await tester.tap(find.byIcon(Icons.visibility_off_outlined));
    await tester.pump();
    expect(find.text('₦ 204,000.00'), findsOneWidget);
  });

  testWidgets('shows empty state when there are no transactions', (tester) async {
    final controller = makeController({
      'available_balance_kobo': 0,
      'pending_balance_kobo': 0,
      'today_earnings_kobo': 0,
      'trips_completed_today': 0,
      'hours_online': 0,
      'transactions': <dynamic>[],
    });
    await pump(tester, controller);

    expect(find.textContaining('No transactions yet'), findsOneWidget);
  });
}
