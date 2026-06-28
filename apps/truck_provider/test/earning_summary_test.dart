import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:truck_provider/features/earnings/state/provider_earnings_controller.dart';
import 'package:truck_provider/features/earnings/ui/provider_earning_summary_screen.dart';
import 'package:truck_provider/features/earnings/ui/widgets/earnings_chart.dart';
import 'package:truck_provider/features/home/data/provider_api.dart';

ProviderEarningsController _controller() {
  final monthly = [60, 90, 130, 100, 120, 204, 170, 240, 360, 500, 470, 430]
      .map((n) => n * 1000 * 100)
      .toList();
  final payload = {
    'success': true,
    'data': {
      'available_balance_kobo': 20400000,
      'pending_balance_kobo': 5400000,
      'today_earnings_kobo': 5400000,
      'trips_completed_today': 3,
      'hours_online': 1,
      'total_earnings_kobo': 50400000,
      'summary_year': 2026,
      'monthly_earnings_kobo': monthly,
      'transactions': [
        {
          'id': 't1', 'booking_id': 't1', 'kind': 'credit',
          'title': 'Anthonia Dipson', 'subtitle': 'Ikeja, Lagos',
          'amount_kobo': 2400000, 'status': 'completed', 'is_trip': true,
          'occurred_at': DateTime.now().toUtc().toIso8601String(),
        },
      ],
    },
  };
  final mock = MockClient((req) async => http.Response(jsonEncode(payload), 200));
  return ProviderEarningsController(
    api: ProviderApi(config: const ApiCoreConfig(baseUrl: 'http://test/api/v1/hauling'), client: mock),
    accessToken: () => 'token',
  );
}

Future<void> _pump(WidgetTester tester, ProviderEarningsController c) async {
  await tester.binding.setSurfaceSize(const Size(430, 932));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(MaterialApp(home: ProviderEarningSummaryScreen(controller: c)));
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('renders total earnings, chart and year', (tester) async {
    final c = _controller();
    await c.load();
    await _pump(tester, c);

    expect(find.text('Earning Summary'), findsOneWidget);
    expect(find.text('Total Earnings'), findsOneWidget);
    expect(find.text('₦ 504,000.00'), findsOneWidget);
    expect(find.text('Monitor and track your earnings.'), findsOneWidget);
    expect(find.text('2026'), findsOneWidget);
    expect(find.byType(EarningsChart), findsOneWidget);
    expect(find.text('Anthonia Dipson'), findsOneWidget);
  });

  testWidgets('eye toggle masks the total', (tester) async {
    final c = _controller();
    await c.load();
    await _pump(tester, c);

    expect(find.text('₦ 504,000.00'), findsOneWidget);
    await tester.tap(find.byIcon(Icons.visibility_outlined));
    await tester.pump();
    expect(find.text('₦ 504,000.00'), findsNothing);
    expect(find.text('₦ ****'), findsOneWidget);
  });
}
