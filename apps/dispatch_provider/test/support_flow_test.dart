import 'dart:async';
import 'package:dispatch_provider/features/disputes/ui/support/case_detail_screen.dart';
import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:dispatch_provider/core/app_mode.dart';
import 'package:dispatch_provider/core/theme/dispatch_theme.dart';
import 'package:dispatch_provider/features/disputes/data/rider_support_api.dart';
import 'package:dispatch_provider/features/disputes/models/dispute_models.dart';
import 'package:dispatch_provider/features/disputes/state/rider_dispute_controller.dart';
import 'package:dispatch_provider/features/disputes/ui/support/create_case_screen.dart';
import 'package:dispatch_provider/features/disputes/ui/dispute_chat_screen.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(() async {
    FlutterSecureStorage.setMockInitialValues({});
    await AppModeConfig.instance.setMode(AppMode.live);
  });
  tearDown(() async {
    await AppModeConfig.instance.setMode(AppMode.mock);
  });
  RiderDisputeController controller(MockClient client) =>
      RiderDisputeController(
        api: RiderSupportApi(
          config: ApiCoreConfig(baseUrl: 'https://support.test'),
          client: client,
        ),
        accessToken: () => 'token',
      );
  http.Response response(Map<String, dynamic> data) =>
      http.Response(jsonEncode({'success': true, 'data': data}), 200);
  testWidgets(
    'New case sends entered subject/details without requiring a selected trip',
    (tester) async {
      Map<String, dynamic>? body;
      final c = controller(
        MockClient((request) async {
          body = Map<String, dynamic>.from(jsonDecode(request.body) as Map);
          return response({
            'id': 'case-one',
            'subject': body!['subject'],
            'description': body!['description'],
            'status': 'open',
          });
        }),
      );
      await tester.pumpWidget(
        MaterialApp(
          theme: DispatchTheme.dark,
          home: CreateCaseScreen(
            disputeController: c,
            initialCategory: 'payment',
          ),
        ),
      );
      await tester.enterText(find.byType(TextField).at(0), 'My payout');
      await tester.enterText(
        find.byType(TextField).at(1),
        'The reference is PAY-123.',
      );
      await tester.pump();
      await tester.tap(find.text('Submit case'));
      await tester.pumpAndSettle();
      expect(body?['subject'], 'My payout');
      expect(body?['description'], 'The reference is PAY-123.');
      expect(body?['category'], 'payment');
      expect(body?['booking_reference'], '');
      expect(find.text('Case submitted'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      c.dispose();
    },
  );
  testWidgets('Failed message stays editable and is never reported sent', (
    tester,
  ) async {
    final c = controller(
      MockClient((request) async {
        if (request.url.path.endsWith('/messages/read')) return response({});
        if (request.method == 'GET') return response({'messages': []});
        throw http.ClientException('Offline');
      }),
    );
    final complaint = Complaint(
      id: 'case-one',
      serviceType: 'dispatch',
      bookingReference: '',
      subject: 'Help',
      description: 'Issue',
      status: 'open',
      createdAt: DateTime.now(),
    );
    await tester.pumpWidget(
      MaterialApp(
        theme: DispatchTheme.light,
        home: DisputeChatScreen(complaint: complaint, controller: c),
      ),
    );
    await tester.pumpAndSettle();
    await tester.enterText(find.byType(TextField), 'Keep this exact message');
    await tester.tap(find.byIcon(Icons.arrow_upward_rounded));
    await tester.pumpAndSettle();
    expect(
      tester.widget<TextField>(find.byType(TextField)).controller?.text,
      'Keep this exact message',
    );
    expect(c.messages, isEmpty);
    expect(c.sendError, isNotNull);
    expect(find.textContaining('Your text is still here'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    c.dispose();
  });
  test(
    'Changing conversations ignores an older in-flight message response',
    () async {
      final old = Completer<http.Response>();
      final c = controller(
        MockClient((request) async {
          if (request.url.path.contains('/old/')) return old.future;
          return response({
            'messages': [
              {
                'id': 'new-message',
                'content': 'New case only',
                'sender_type': 'admin',
                'created_at': '2026-10-15T12:00:00Z',
              },
            ],
          });
        }),
      );
      final pending = c.loadMessages('old');
      await Future<void>.delayed(Duration.zero);
      await c.loadMessages('new');
      old.complete(
        response({
          'messages': [
            {
              'id': 'old-message',
              'content': 'Private old case',
              'sender_type': 'admin',
            },
          ],
        }),
      );
      await pending;
      expect(c.messages.map((m) => m.id), ['new-message']);
      c.dispose();
    },
  );
  testWidgets(
    'Case details refresh saved status and preserve it during a later outage',
    (tester) async {
      var outage = false;
      final c = controller(
        MockClient((request) async {
          expect(request.url.path, '/dispatch-provider/complaints/case-one');
          if (outage) return http.Response('unavailable', 503);
          return response({
            'id': 'case-one',
            'subject': 'My issue',
            'description': 'Saved details',
            'status': 'resolved',
            'created_at': '2026-10-15T12:00:00Z',
            'resolved_at': '2026-10-15T13:00:00Z',
          });
        }),
      );
      final cached = Complaint(
        id: 'case-one',
        serviceType: 'dispatch',
        bookingReference: '',
        subject: 'My issue',
        description: 'Saved details',
        status: 'open',
        createdAt: DateTime.now(),
      );
      await tester.pumpWidget(
        MaterialApp(
          theme: DispatchTheme.dark,
          home: CaseDetailScreen(complaint: cached, disputeController: c),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('This case has been resolved'), findsOneWidget);
      outage = true;
      await tester.tap(find.byTooltip('Refresh case'));
      await tester.pumpAndSettle();
      expect(find.text('This case has been resolved'), findsOneWidget);
      expect(
        find.textContaining('Showing the last saved case details'),
        findsOneWidget,
      );
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      c.dispose();
    },
  );
}
