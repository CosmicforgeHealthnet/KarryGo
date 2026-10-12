import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/features/pastry/state/pastry_workspace_controller.dart';
import 'finance_test.dart' show financeApp;
import 'order_support_test.dart' show SupportRuntime;
import 'crm_workspace_test.dart' show CrmTransport;

void main() {
  test(
    'Quote revision, real step-up, duplicate-safe approval, acceptance and ordered milestones',
    () async {
      final transport = CrmTransport(),
          app = await financeApp(transport),
          runtime = SupportRuntime(app);
      final c = PastryWorkspaceController(
        runtime,
        '$pastryBase/custom-requests',
      );
      await c.load();
      final request = c.rows.first, params = {'requestId': '${request['id']}'};
      final spec = await c.save(
        method: 'POST',
        route: '$pastryBase/custom-requests/{requestId}/specifications',
        params: params,
        body: {
          'expected_version': 1,
          'specification': {'servings': 24, 'inscription': 'Happy birthday'},
        },
      );
      expect(spec, isNotNull, reason: c.error);
      final start = DateTime.now().toUtc().add(const Duration(days: 1)),
          end = start.add(const Duration(hours: 2));
      final created = await c.save(
        method: 'POST',
        route: '$pastryBase/custom-requests/{requestId}/quotes',
        params: params,
        body: {
          'specification_version_id': spec!['id'],
          'amount_minor': 4500000,
          'expires_at': end.add(const Duration(days: 1)).toIso8601String(),
          'production_start': start.toIso8601String(),
          'production_end': end.toIso8601String(),
          'capacity_allocations': [
            {'capacity_pool_id': 'mock-pastry-pool', 'quantity': 2},
          ],
          'hold_for_minutes': 30,
          'approval_required': true,
          'expected_request_version': 2,
        },
      );
      expect(created, isNotNull, reason: c.error);
      final old = {'quoteId': '${created!['id']}'};
      expect(
        await c.save(
          method: 'POST',
          route: '$pastryBase/quotes/{quoteId}/revise',
          params: old,
          body: {
            'specification_version_id': spec['id'],
            'amount_minor': 4800000,
            'expires_at': end.add(const Duration(days: 1)).toIso8601String(),
            'reason_code': 'PRICE_UPDATE',
            'note': 'Updated price',
            'expected_version': 1,
          },
        ),
        isNotNull,
        reason: c.error,
      );
      final list = (await app.pastryApi.request(
        'GET',
        '$pastryBase/custom-requests/{requestId}/quotes',
        pathParameters: params,
      )).list.map(jsonObject).toList();
      expect(list.first['status'], 'SUPERSEDED');
      expect(list.last['status'], 'PENDING_APPROVAL');
      final q = {'quoteId': '${list.last['id']}'};
      final oldReservation = (await app.productionApi.request(
        'GET',
        '/api/v1/vendor/production/reservations/{reservationId}',
        pathParameters: {'reservationId': '${created['reservation_id']}'},
      )).object;
      expect(oldReservation['status'], 'HELD');
      expect(oldReservation['source_ref'], q['quoteId']);
      expect(list.last['reservation_id'], created['reservation_id']);
      expect(
        await c.save(
          method: 'POST',
          route: '$pastryBase/quotes/{quoteId}/approve',
          params: q,
          body: {'expected_version': 1},
        ),
        isNull,
      );
      expect(c.pending, isNull);
      await c.load();
      final challenge = await app.auth.startStepUp(
            'pastry.high_value_quote_approval',
          ),
          grant = await app.auth.verifyStepUp(
            purpose: 'pastry.high_value_quote_approval',
            challengeId: challenge.challengeId,
            otp: '123456',
          );
      transport.lose = true;
      transport.suffix = '/approve';
      expect(
        await c.save(
          method: 'POST',
          route: '$pastryBase/quotes/{quoteId}/approve',
          params: q,
          body: {'expected_version': 1},
          headers: {'X-Step-Up-Grant': grant.grantId},
        ),
        isNull,
      );
      expect(c.pending, isNotNull);
      expect(c.pending!.containsKey('headers'), false);
      expect(
        await c.save(headers: {'X-Step-Up-Grant': grant.grantId}),
        isNotNull,
        reason: c.error,
      );
      final accepted = await c.save(
        method: 'POST',
        route: '$pastryBase/quotes/{quoteId}/accept',
        params: q,
        body: {'expected_version': 2},
      );
      expect(accepted, isNotNull, reason: c.error);
      expect(accepted!['order_authority'], 'NOT_PRESENT');
      expect(accepted['conversion_intent_id'], isNotEmpty);
      final reservation = (await app.productionApi.request(
        'GET',
        '/api/v1/vendor/production/reservations/{reservationId}',
        pathParameters: {'reservationId': '${accepted['reservation_id']}'},
      )).object;
      expect(reservation['status'], 'CONFIRMED');
      final mc = PastryWorkspaceController(
        runtime,
        '$pastryBase/custom-requests/{requestId}/milestones',
        parameters: params,
      );
      await mc.load();
      expect(mc.rows.length, 6);
      expect(
        await mc.save(
          method: 'POST',
          route:
              '$pastryBase/custom-requests/{requestId}/milestones/{milestoneType}/{state}',
          params: {
            ...params,
            'milestoneType': 'BASE_PREPARED',
            'state': 'ACTIVE',
          },
          body: {'expected_version': 1, 'note': 'Too early'},
        ),
        isNull,
      );
      await mc.load();
      for (final type in [
        'DESIGN_APPROVED',
        'BASE_PREPARED',
        'DECORATION',
        'CHILLING',
        'PACKED',
        'READY',
      ]) {
        await mc.load();
        var milestone = mc.rows.firstWhere((m) => m['type'] == type);
        if (milestone['state'] == 'PENDING') {
          expect(
            await mc.save(
              method: 'POST',
              route:
                  '$pastryBase/custom-requests/{requestId}/milestones/{milestoneType}/{state}',
              params: {...params, 'milestoneType': type, 'state': 'ACTIVE'},
              body: {'expected_version': milestone['version']},
            ),
            isNotNull,
            reason: mc.error,
          );
          await mc.load();
          milestone = mc.rows.firstWhere((m) => m['type'] == type);
        }
        expect(
          await mc.save(
            method: 'POST',
            route:
                '$pastryBase/custom-requests/{requestId}/milestones/{milestoneType}/{state}',
            params: {...params, 'milestoneType': type, 'state': 'COMPLETED'},
            body: {
              'expected_version': milestone['version'],
              'note': 'Completed',
            },
          ),
          isNotNull,
          reason: mc.error,
        );
      }
      final eligibility = (await app.pastryApi.request(
        'GET',
        '$pastryBase/custom-requests/{requestId}/change-eligibility',
        pathParameters: params,
      )).object;
      expect(eligibility['allowed'], false);
      final cancellation = (await app.pastryApi.request(
        'GET',
        '$pastryBase/custom-requests/{requestId}/cancellation-eligibility',
        pathParameters: params,
      )).object;
      expect(cancellation['state'], 'RESTRICTED');
      final history = (await app.pastryApi.request(
        'GET',
        '$pastryBase/custom-requests/{requestId}/milestones/{milestoneType}/history',
        pathParameters: {...params, 'milestoneType': 'READY'},
      )).list;
      expect(history.length, 2);
      mc.dispose();
      c.dispose();
      app.client.close();
    },
  );
  test(
    'Shared capacity refuses oversized batches and lost creation retries do not duplicate jobs',
    () async {
      final transport = CrmTransport(),
          app = await financeApp(transport),
          runtime = SupportRuntime(app),
          params = {'productId': 'mock-pastry-cake'};
      final c = PastryWorkspaceController(
        runtime,
        '$pastryBase/products/{productId}',
        parameters: params,
      );
      await c.load();
      final start = DateTime.now().toUtc().add(const Duration(days: 2)),
          body = <String, dynamic>{
            'quantity': 120,
            'window_start': start.toIso8601String(),
            'window_end': start.add(const Duration(hours: 1)).toIso8601String(),
          };
      expect(
        (await c.save(
          method: 'POST',
          route: '$pastryBase/products/{productId}/capacity-preview',
          params: params,
          body: body,
        ))?['available'],
        false,
      );
      expect(
        await c.save(
          method: 'POST',
          route: '$pastryBase/products/{productId}/production-batches',
          params: params,
          body: body,
        ),
        isNull,
      );
      await c.load();
      body['quantity'] = 12;
      expect(
        (await c.save(
          method: 'POST',
          route: '$pastryBase/products/{productId}/capacity-preview',
          params: params,
          body: body,
        ))?['available'],
        true,
      );
      transport.lose = true;
      transport.suffix = '/production-batches';
      expect(
        await c.save(
          method: 'POST',
          route: '$pastryBase/products/{productId}/production-batches',
          params: params,
          body: body,
        ),
        isNull,
      );
      expect(c.pending, isNotNull);
      final saved = await c.save();
      expect(saved, isNotNull, reason: c.error);
      final job = (await app.productionApi.request(
        'GET',
        '/api/v1/vendor/production/jobs/{jobId}',
        pathParameters: {'jobId': '${saved!['production_job_id']}'},
      )).object;
      expect(job['planned_quantity'], 12);
      c.dispose();
      app.client.close();
    },
  );

  test(
    'Design comparison, immutable decisions and unapproved specifications cannot be quoted',
    () async {
      final transport = CrmTransport(),
          app = await financeApp(transport),
          runtime = SupportRuntime(app);
      final c = PastryWorkspaceController(
        runtime,
        '$pastryBase/custom-requests',
      );
      await c.load();
      final r = c.rows.first, p = {'requestId': '${r['id']}'};
      final spec = await c.save(
        method: 'POST',
        route: '$pastryBase/custom-requests/{requestId}/specifications',
        params: p,
        body: {
          'expected_version': 1,
          'specification': {'servings': 24, 'inscription': 'Happy birthday'},
        },
      );
      expect(spec, isNotNull);
      final change = await c.save(
        method: 'POST',
        route: '$pastryBase/custom-requests/{requestId}/changes',
        params: p,
        body: {
          'expected_version': 2,
          'reason': 'Inscription update',
          'specification': {'servings': 24, 'inscription': 'Happy anniversary'},
        },
      );
      expect(change, isNotNull, reason: c.error);
      final diff = (await app.pastryApi.request(
        'GET',
        '$pastryBase/custom-requests/{requestId}/specifications/diff',
        pathParameters: p,
        query: {
          'from': '${spec!['id']}',
          'to': '${change!['specification_id']}',
        },
      )).list;
      expect(diff.length, 1);
      expect(jsonObject(diff.single)['field'], 'inscription');
      final expires = DateTime.now()
          .toUtc()
          .add(const Duration(days: 2))
          .toIso8601String();
      expect(
        await c.save(
          method: 'POST',
          route: '$pastryBase/custom-requests/{requestId}/quotes',
          params: p,
          body: {
            'expected_request_version': 3,
            'specification_version_id': change['specification_id'],
            'amount_minor': 5000000,
            'expires_at': expires,
          },
        ),
        isNull,
      );
      await c.load();
      expect(
        await c.save(
          method: 'POST',
          route: '$pastryBase/changes/{changeId}/decision',
          params: {'changeId': '${change['change_request_id']}'},
          body: {
            'expected_version': 1,
            'decision': 'APPROVED',
            'note': 'Reviewed',
          },
        ),
        isNotNull,
        reason: c.error,
      );
      expect(
        await c.save(
          method: 'POST',
          route: '$pastryBase/changes/{changeId}/decision',
          params: {'changeId': '${change['change_request_id']}'},
          body: {'expected_version': 2, 'decision': 'DECLINED'},
        ),
        isNull,
      );
      await c.load();
      expect(
        await c.save(
          method: 'POST',
          route: '$pastryBase/custom-requests/{requestId}/quotes',
          params: p,
          body: {
            'expected_request_version': 3,
            'specification_version_id': change['specification_id'],
            'amount_minor': 5000000,
            'expires_at': expires,
          },
        ),
        isNotNull,
        reason: c.error,
      );
      final history = (await app.pastryApi.request(
        'GET',
        '$pastryBase/changes/{changeId}/history',
        pathParameters: {'changeId': '${change['change_request_id']}'},
      )).list;
      expect(history.length, 1);
      expect(jsonObject(history.single)['to_status'], 'APPROVED');
      c.dispose();
      app.client.close();
    },
  );
}
