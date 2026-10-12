import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:vendor/vendor.dart';
import 'package:vendor/core/state/vendor_scope.dart';
import 'package:vendor/core/state/vendor_feature_controller.dart';

const session = VendorSession(
  accessToken: 'old',
  refreshToken: 'refresh-old',
  expiresIn: 900,
  staffId: 'staff-1',
  vendorId: 'vendor-1',
);
const renewed = VendorSession(
  accessToken: 'new',
  refreshToken: 'refresh-new',
  expiresIn: 900,
  staffId: 'staff-1',
  vendorId: 'vendor-1',
);
http.Response ok(
  Object? data, {
  int status = 200,
  Map<String, String> headers = const {},
}) => http.Response(
  jsonEncode({'success': true, 'data': data}),
  status,
  headers: headers,
);
http.Response fail(int status, String code) => http.Response(
  jsonEncode({
    'success': false,
    'error': {'code': code, 'message': 'Controlled failure'},
  }),
  status,
);
Future<VendorApp> appWith(MockClient mock) async {
  final store = MemoryVendorSessionStore();
  await store.save(session);
  final app = VendorApp(
    config: VendorAppConfig(origin: 'https://vendor.example'),
    sessions: store,
    httpClient: mock,
  );
  await app.initialize();
  return app;
}

void main() {
  test('generated Vendor contracts exclude customer and service APIs', () {
    final manifest =
        jsonDecode(File('contracts/vendor_manifest.json').readAsStringSync())
            as Map;
    final app = VendorApp(
      config: VendorAppConfig(origin: 'https://vendor.example'),
      sessions: MemoryVendorSessionStore(),
      httpClient: MockClient((r) async => ok({})),
    );
    expect(manifest['unresolved_requests'], isEmpty);
    for (final raw in manifest['operations'] as List) {
      final op = raw as Map;
      expect(op['path'], startsWith('/api/v1/vendor/'));
      expect((op['path'] as String).contains('/internal/'), isFalse);
      expect(op['request_known'], isTrue);
    }
    expect(
      app.ordersApi.operations.any(
        (o) => o.path.contains('substitution-cases'),
      ),
      isFalse,
    );
    app.dispose();
  });

  test('exact route path encoding, query and envelope', () async {
    final app = await appWith(
      MockClient((r) async {
        expect(r.method, 'GET');
        expect(r.url.pathSegments.last, 'branch/a b');
        expect(r.url.queryParameters, isEmpty);
        expect(r.headers['authorization'], 'Bearer old');
        return ok(
          {
            'items': [],
            'pagination': {'limit': 5, 'offset': 0, 'has_more': false},
          },
          headers: {'etag': '7'},
        );
      }),
    );
    final response = await app.businessApi.request(
      'GET',
      '/api/v1/vendor/businesses/{businessId}/branches/{branchId}',
      pathParameters: {'businessId': 'business-1', 'branchId': 'branch/a b'},
    );
    expect(response.etag, '7');
    app.dispose();
  });

  test('undeclared request fields rejected before HTTP', () async {
    var calls = 0;
    final app = await appWith(
      MockClient((r) async {
        calls++;
        return ok({});
      }),
    );
    await expectLater(
      app.financeApi.request(
        'POST',
        '/api/v1/vendor/finance/withdrawals',
        body: VendorPayload({
          'bank_account_id': 'b',
          'amount_kobo': 100,
          'step_up_grant_id': 'grant',
          'balance': 999,
        }),
        command: const VendorCommand(idempotencyKey: 'same'),
      ),
      throwsA(isA<VendorContractException>()),
    );
    expect(calls, 0);
    app.dispose();
  });

  test('auth verify business choice reuses exact challenge and OTP', () async {
    final requests = <Map<String, dynamic>>[];
    final mock = MockClient((r) async {
      final body = jsonDecode(r.body) as Map<String, dynamic>;
      if (r.url.path.endsWith('/start')) {
        return ok({
          'challenge_id': 'challenge-1',
          'expires_in': 300,
          'debug_otp': 'secret',
        });
      }
      requests.add(body);
      if (requests.length == 1) {
        return ok({
          'requires_vendor_choice': true,
          'available_vendor_ids': ['vendor-1', 'vendor-2'],
        });
      }
      return ok(session.toJson());
    });
    final app = VendorApp(
      config: VendorAppConfig(origin: 'https://vendor.example'),
      sessions: MemoryVendorSessionStore(),
      httpClient: mock,
    );
    await app.auth.start(email: 'owner@example.com');
    final choice = await app.auth.verify(otp: '123456');
    expect(choice.requiresVendorChoice, isTrue);
    expect(await app.client.sessions.read(), isNull);
    await app.auth.chooseVendor('vendor-1');
    expect(requests[0]['challenge_id'], requests[1]['challenge_id']);
    expect(requests[0]['otp'], requests[1]['otp']);
    expect(requests[0].containsKey('vendor_id'), isFalse);
    expect(requests[1]['vendor_id'], 'vendor-1');
    expect(app.auth.phase, VendorAuthPhase.authenticated);
    expect(app.scope.vendorId, 'vendor-1');
    app.dispose();
  });

  test('simultaneous 401s rotate refresh token only once', () async {
    var refreshes = 0;
    final gate = Completer<void>();
    var oldReads = 0;
    final app = await appWith(
      MockClient((r) async {
        if (r.url.path.endsWith('/refresh')) {
          refreshes++;
          expect(jsonDecode(r.body)['refresh_token'], 'refresh-old');
          await gate.future;
          return ok(renewed.toJson());
        }
        if (r.headers['authorization'] == 'Bearer old') {
          oldReads++;
          if (oldReads == 2) gate.complete();
          return fail(401, 'unauthorized');
        }
        expect(r.headers['authorization'], 'Bearer new');
        return ok({'available': true});
      }),
    );
    final results = await Future.wait([
      app.financeApi.getFinanceOverview(),
      app.financeApi.getFinanceOverview(),
    ]);
    expect(results.length, 2);
    expect(refreshes, 1);
    expect((await app.client.sessions.read())!.refreshToken, 'refresh-new');
    app.dispose();
  });

  test('late refresh cannot restore a logged-out session', () async {
    final entered = Completer<void>();
    final gate = Completer<void>();
    final app = await appWith(
      MockClient((r) async {
        entered.complete();
        await gate.future;
        return ok(renewed.toJson());
      }),
    );
    final refresh = app.client.refreshSession();
    final expected = expectLater(refresh, throwsA(isA<StateError>()));
    await entered.future;
    await app.client.clearSession();
    gate.complete();
    await expected;
    expect(await app.client.sessions.read(), isNull);
    expect(app.scope.vendorId, isNull);
    app.dispose();
  });

  test('withdrawal timeout is not retried and retains session', () async {
    var calls = 0;
    final store = MemoryVendorSessionStore();
    await store.save(session);
    final client = VendorApiClient(
      config: VendorAppConfig(
        origin: 'https://vendor.example',
        timeout: const Duration(milliseconds: 10),
      ),
      sessions: store,
      client: MockClient((r) async {
        calls++;
        await Future<void>.delayed(const Duration(milliseconds: 50));
        return ok({});
      }),
    );
    final api = VendorFinanceApi(client);
    await expectLater(
      api.postFinanceWithdrawals(
        body: PostFinanceWithdrawalsRequest(
          bankAccountId: 'bank',
          amountKobo: 100,
          stepUpGrantId: 'grant',
        ),
        command: const VendorCommand(idempotencyKey: 'withdrawal-1'),
      ),
      throwsA(
        isA<ApiException>().having((e) => e.code, 'code', ApiErrorCode.network),
      ),
    );
    expect(calls, 1);
    expect(await store.read(), isNotNull);
    client.close();
  });

  test('If-Match and original idempotency key reach order command', () async {
    final app = await appWith(
      MockClient((r) async {
        expect(r.headers['if-match'], '4');
        expect(r.headers['idempotency-key'], 'order-command');
        expect(jsonDecode(r.body), {'reason_code': 'STAFF_REASON'});
        return fail(409, 'conflict');
      }),
    );
    final flow = VendorOrderWorkflow(app.ordersApi);
    await expectLater(
      flow.execute(
        action: 'reject',
        vendorOrderId: 'order-1',
        command: const VendorCommand(
          idempotencyKey: 'order-command',
          ifMatch: '4',
        ),
        reasonCode: 'STAFF_REASON',
      ),
      throwsA(isA<ApiException>().having((e) => e.statusCode, 'status', 409)),
    );
    await expectLater(
      app.ordersApi.postCommerceOrdersVendorOrderIdAccept(
        vendorOrderId: 'order-1',
        body: PostCommerceOrdersVendorOrderIdAcceptRequest(),
        command: const VendorCommand(idempotencyKey: 'key'),
      ),
      throwsA(isA<VendorContractException>()),
    );
    app.dispose();
  });

  test('picking quality accepts no-content response', () async {
    final app = await appWith(MockClient((r) async => http.Response('', 204)));
    final result = await app.ordersApi
        .postCommerceOrdersVendorOrderIdLinesLineIdQuality(
          vendorOrderId: 'order-1',
          lineId: 'line-1',
          body: PostCommerceOrdersVendorOrderIdLinesLineIdQualityRequest(
            result: 'PASS',
          ),
          command: const VendorCommand(idempotencyKey: 'quality', ifMatch: '2'),
        );
    expect(result.statusCode, 204);
    expect(result.data, isNull);
    app.dispose();
  });

  test(
    'handover unknown outcome does not retry or persist credentials',
    () async {
      var calls = 0;
      final app = await appWith(
        MockClient((r) async {
          calls++;
          expect(jsonDecode(r.body)['credential'], '123456');
          return fail(503, 'HANDOVER_OUTCOME_UNKNOWN');
        }),
      );
      final flow = VendorPickupWorkflow(app.handoverApi);
      await expectLater(
        flow.confirm(
          vendorOrderId: 'order-1',
          missionId: 'mission-1',
          handoverRef: 'handover-1',
          method: 'OTP',
          credential: '123456',
          command: const VendorCommand(idempotencyKey: 'pickup-1'),
        ),
        throwsA(
          isA<ApiException>().having(
            (e) => e.code,
            'code',
            'HANDOVER_OUTCOME_UNKNOWN',
          ),
        ),
      );
      expect(calls, 1);
      expect(
        (await app.client.sessions.read())!.encode().contains('123456'),
        isFalse,
      );
      app.dispose();
    },
  );

  test('scope change discards late feature response', () async {
    final scope = VendorScope()
      ..select(vendorId: 'vendor-1', branchId: 'branch-1');
    final controller = VendorFeatureController(scope);
    final gate = Completer<VendorResponse>();
    final pending = controller.load(() => gate.future);
    scope.select(vendorId: 'vendor-1', branchId: 'branch-2');
    gate.complete(
      const VendorResponse(
        statusCode: 200,
        headers: {},
        data: {'branch_id': 'branch-1'},
      ),
    );
    expect(await pending, isNull);
    expect(controller.response, isNull);
    expect(controller.phase, VendorLoadPhase.idle);
    controller.dispose();
    scope.dispose();
  });

  test('undeclared queries and internal headers cannot be sent', () async {
    var calls = 0;
    final app = await appWith(
      MockClient((r) async {
        calls++;
        return ok({});
      }),
    );
    await expectLater(
      app.financeApi.getFinanceOverview(query: {'vendor_id': 'another'}),
      throwsA(isA<VendorContractException>()),
    );
    await expectLater(
      app.financeApi.getFinanceOverview(
        contractHeaders: {'X-Service-Signature': 'secret'},
      ),
      throwsA(isA<VendorContractException>()),
    );
    expect(calls, 0);
    app.dispose();
  });

  test(
    'forbidden errors record denial without inferring role privileges',
    () async {
      final app = await appWith(
        MockClient((r) async => fail(403, 'forbidden')),
      );
      await expectLater(
        app.financeApi.getFinanceOverview(),
        throwsA(isA<ApiException>()),
      );
      expect(app.access.deniedActions, isNotEmpty);
      expect(await app.client.sessions.read(), isNotNull);
      app.dispose();
    },
  );

  test('refresh preserves branch selection in the same vendor', () async {
    final app = await appWith(MockClient((r) async => ok(renewed.toJson())));
    app.scope.select(vendorId: 'vendor-1', branchId: 'branch-2');
    await app.client.refreshSession();
    expect(app.scope.branchId, 'branch-2');
    app.dispose();
  });
  test('document deletion sends only expected_version in JSON', () async {
    final app = await appWith(
      MockClient((r) async {
        expect(r.method, 'DELETE');
        expect(jsonDecode(r.body), {'expected_version': 3});
        expect(r.headers['idempotency-key'], 'delete-doc');
        return ok({'deleted': true});
      }),
    );
    final response = await app.onboardingApi
        .deleteOnboardingDocumentsDocumentId(
          documentId: 'document-1',
          body: DeleteOnboardingDocumentsDocumentIdRequest(expectedVersion: 3),
          command: const VendorCommand(idempotencyKey: 'delete-doc'),
        );
    expect(response.object['deleted'], true);
    app.dispose();
  });
  test('body-level versions are not silently moved to If-Match', () async {
    var calls = 0;
    final app = await appWith(
      MockClient((r) async {
        calls++;
        return ok({});
      }),
    );
    await expectLater(
      app.onboardingApi.deleteOnboardingDocumentsDocumentId(
        documentId: 'document-1',
        body: DeleteOnboardingDocumentsDocumentIdRequest(expectedVersion: 3),
        command: const VendorCommand(
          idempotencyKey: 'delete-doc',
          ifMatch: '3',
        ),
      ),
      throwsA(isA<VendorContractException>()),
    );
    expect(calls, 0);
    app.dispose();
  });

  test(
    'refresh suspension clears session but a dependency outage does not',
    () async {
      final suspended = await appWith(
        MockClient((r) async => fail(403, 'forbidden')),
      );
      await expectLater(
        suspended.client.refreshSession(),
        throwsA(isA<ApiException>()),
      );
      expect(await suspended.client.sessions.read(), isNull);
      expect(suspended.scope.vendorId, isNull);
      suspended.dispose();
      final outage = await appWith(
        MockClient((r) async => fail(503, 'service_unavailable')),
      );
      await expectLater(
        outage.client.refreshSession(),
        throwsA(isA<ApiException>()),
      );
      expect(await outage.client.sessions.read(), isNotNull);
      outage.dispose();
    },
  );
}
