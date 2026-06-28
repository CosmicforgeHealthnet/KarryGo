import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:truck_provider/features/disputes/data/provider_support_api.dart';
import 'package:truck_provider/features/disputes/state/provider_dispute_controller.dart';
import 'package:truck_provider/features/disputes/ui/provider_select_dispute_type_screen.dart';
import 'package:truck_provider/features/earnings/models/earnings_models.dart';

EarningsTransaction _txn() => EarningsTransaction(
      id: 'txn1', bookingId: 'b1', kind: 'credit', title: 'Anthonia Dipson',
      subtitle: 'Ikeja', amountKobo: 2400000, status: 'completed', isTrip: true,
      occurredAt: DateTime(2026, 1, 2, 12),
    );

ProviderDisputeController _controller({List<String>? requests}) {
  final mock = MockClient((req) async {
    requests?.add('${req.method} ${req.url.path}');
    if (req.method == 'POST' && req.url.path.endsWith('/complaints')) {
      final body = jsonDecode(req.body) as Map<String, dynamic>;
      return http.Response(
        jsonEncode({'success': true, 'data': {
          'id': 'c-new', 'subject': body['subject'], 'description': body['description'],
          'booking_reference': body['booking_reference'], 'service_type': 'hauling',
          'status': 'open', 'created_at': '2026-01-02T12:00:00Z',
        }}),
        201,
      );
    }
    if (req.url.path.endsWith('/complaints')) {
      return http.Response(jsonEncode({'success': true, 'data': {'complaints': [
        {'id': 'c1', 'subject': 'Withdrawal', 'status': 'open', 'created_at': '2023-07-27T12:09:39Z'},
      ]}}), 200);
    }
    if (req.url.path.endsWith('/messages') && req.method == 'POST') {
      final body = jsonDecode(req.body) as Map<String, dynamic>;
      return http.Response(jsonEncode({'success': true, 'data': {
        'id': 'm-new', 'sender_type': 'hauling_provider', 'content': body['content'],
        'created_at': '2026-01-02T12:05:00Z',
      }}), 201);
    }
    return http.Response(jsonEncode({'success': true, 'data': {}}), 200);
  });
  return ProviderDisputeController(
    api: ProviderSupportApi(config: const ApiCoreConfig(baseUrl: 'http://test/api/v1/support-disputes'), client: mock),
    accessToken: () => 'token',
  );
}

void main() {
  group('controller', () {
    test('loadComplaints fetches the feedback list', () async {
      final c = _controller();
      await c.loadComplaints();
      expect(c.complaints, hasLength(1));
      expect(c.complaints.first.subject, 'Withdrawal');
      expect(c.complaints.first.statusLabel, 'Pending');
    });

    test('submitDispute posts a complaint with the selected type + booking ref', () async {
      final reqs = <String>[];
      final c = _controller(requests: reqs);
      c.selectTransaction(_txn());
      c.selectType('Cancelled trip');
      final complaint = await c.submitDispute();
      expect(complaint, isNotNull);
      expect(complaint!.subject, 'Cancelled trip');
      expect(reqs, contains('POST /api/v1/support-disputes/provider/complaints'));
      // The new complaint is prepended to the feedback list.
      expect(c.complaints.first.id, 'c-new');
    });

    test('submitDispute is blocked without a selection', () async {
      final c = _controller();
      final complaint = await c.submitDispute();
      expect(complaint, isNull);
    });

    test('sendMessage appends to the thread', () async {
      final c = _controller();
      await c.sendMessage('c1', 'Hello support');
      expect(c.messages, hasLength(1));
      expect(c.messages.first.content, 'Hello support');
      expect(c.messages.first.isMine, isTrue);
    });

    test('complaint stage maps to the status timeline', () async {
      final c = _controller();
      await c.loadComplaints();
      expect(c.complaints.first.stage, 0); // open → Submitted
    });
  });

  group('select dispute type screen', () {
    testWidgets('Confirm enables only after a type is chosen', (tester) async {
      await tester.binding.setSurfaceSize(const Size(430, 932));
      addTearDown(() => tester.binding.setSurfaceSize(null));
      final c = _controller();
      c.selectTransaction(_txn());

      await tester.pumpWidget(MaterialApp(home: ProviderSelectDisputeTypeScreen(controller: c)));
      await tester.pumpAndSettle();

      expect(find.text('Select Dispute Type'), findsOneWidget);
      expect(find.text('Cancelled trip'), findsOneWidget);

      final button = tester.widget<FilledButton>(find.widgetWithText(FilledButton, 'Confirm'));
      expect(button.onPressed, isNull); // disabled

      await tester.tap(find.text('Cancelled trip'));
      await tester.pump();

      final button2 = tester.widget<FilledButton>(find.widgetWithText(FilledButton, 'Confirm'));
      expect(button2.onPressed, isNotNull); // enabled
    });
  });
}
