import 'package:dispatch_provider/core/theme/dispatch_theme.dart';
import 'package:dispatch_provider/features/requests/data/request_model.dart';
import 'package:dispatch_provider/features/requests/ui/widgets/opportunity_card.dart';
import 'package:dispatch_provider/features/trips/data/trip_model.dart';
import 'package:dispatch_provider/features/trips/ui/customer_rating_screen.dart';
import 'dart:async';
import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:dispatch_provider/core/app_mode.dart';
import 'package:dispatch_provider/features/trips/data/trips_api.dart';
import 'package:dispatch_provider/features/trips/state/trips_controller.dart';
import 'package:dispatch_provider/features/trips/ui/activity_filters_sheet.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
    'Filter changes ignore stale pages and pagination retains server filters',
    () async {
      FlutterSecureStorage.setMockInitialValues({});
      await AppModeConfig.instance.setMode(AppMode.live);
      final requests = <http.Request>[];
      final stale = Completer<http.Response>();
      final client = MockClient((request) async {
        requests.add(request);
        if (requests.length == 1) return stale.future;
        return http.Response(
          jsonEncode({
            'success': true,
            'data': {
              'trips': [
                {
                  'id': 'filtered',
                  'status': 'completed',
                  'created_at': '2026-10-15T00:00:00Z',
                },
              ],
              'total': 2,
            },
          }),
          200,
        );
      });
      final controller = TripsController(
        api: TripsApi(
          ApiCoreConfig(baseUrl: 'https://dispatch.test'),
          client: client,
        ),
        getAccessToken: () => 'token',
      );
      final old = controller.loadTrips();
      await Future<void>.delayed(Duration.zero);
      final start = DateTime.utc(2026, 10, 1);
      await controller.loadTrips(
        applyFilters: true,
        status: 'completed',
        createdFrom: start,
      );
      stale.complete(
        http.Response(
          jsonEncode({
            'success': true,
            'data': {'trips': [], 'total': 0},
          }),
          200,
        ),
      );
      await old;
      expect(controller.trips.single.id, 'filtered');
      await controller.loadMore();
      expect(requests.last.url.queryParameters['page'], '2');
      expect(requests.last.url.queryParameters['status'], 'completed');
      expect(
        DateTime.parse(requests.last.url.queryParameters['created_from']!),
        start,
      );
      controller.dispose();
      client.close();
      await AppModeConfig.instance.setMode(AppMode.mock);
    },
  );
  testWidgets('Filter sheet preserves selection and reset returns all time', (
    tester,
  ) async {
    ActivityFilterResult? result;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Builder(
            builder: (context) => TextButton(
              onPressed: () async {
                result = await showActivityFiltersSheet(
                  context,
                  initial: const ActivityFilterResult(
                    tab: ActivityTab.completed,
                    dateRange: 'Last 30 days',
                  ),
                );
              },
              child: const Text('Open'),
            ),
          ),
        ),
      ),
    );
    await tester.tap(find.text('Open'));
    await tester.pumpAndSettle();
    expect(find.text('Last 30 days'), findsOneWidget);
    await tester.ensureVisible(find.text('Reset filters'));
    await tester.tap(find.text('Reset filters'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Apply filters'));
    await tester.pumpAndSettle();
    expect(result?.dateRange, 'All time');
    expect(result?.tab, ActivityTab.all);
  });
  testWidgets('Opportunity actions fit narrow screens with larger text', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(320, 720);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    var accepted = false;
    await tester.pumpWidget(
      MaterialApp(
        theme: DispatchTheme.light,
        home: MediaQuery(
          data: const MediaQueryData(
            size: Size(320, 720),
            textScaler: TextScaler.linear(1.3),
          ),
          child: Scaffold(
            body: SingleChildScrollView(
              child: Padding(
                padding: const EdgeInsets.all(16),
                child: OpportunityCard(
                  request: RequestModel.fromJson({
                    'inbox_id': 'request',
                    'booking_id': 'booking',
                  }),
                  onTap: () {},
                  onAccept: () => accepted = true,
                  onDecline: () {},
                ),
              ),
            ),
          ),
        ),
      ),
    );
    expect(tester.takeException(), isNull);
    expect(find.text('Estimated fare'), findsNothing);
    await tester.ensureVisible(find.text('Accept'));
    await tester.tap(find.text('Accept'));
    expect(accepted, isTrue);
  });
  testWidgets('Missing rating is not presented as a zero or verified rating', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: DispatchTheme.dark,
        home: CustomerRatingScreen(
          trip: TripModel.fromJson({'id': 'trip', 'status': 'completed'}),
        ),
      ),
    );
    expect(find.text('Not rated'), findsOneWidget);
    expect(find.text('Verified customer'), findsNothing);
    expect(tester.takeException(), isNull);
  });
}
