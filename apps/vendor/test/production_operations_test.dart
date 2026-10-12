import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:vendor/features/production/state/production_operations_controller.dart';
import 'finance_test.dart' show FinanceTransport, financeApp;
import 'order_support_test.dart' show SupportRuntime;

class ProductionTransport extends FinanceTransport {
  bool lose = false;
  bool loseRelease = false;
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    final result = await super.send(request);
    if (loseRelease &&
        request.method == 'POST' &&
        request.url.path.endsWith('/release')) {
      loseRelease = false;
      await result.stream.drain<void>();
      throw http.ClientException('Lost release response', request.url);
    }
    if (lose &&
        request.method == 'POST' &&
        request.url.path.endsWith('/QUEUED')) {
      lose = false;
      await result.stream.drain<void>();
      throw http.ClientException('Lost response', request.url);
    }
    return result;
  }
}

void main() {
  test(
    'production queue restores the original command without duplicate transitions',
    () async {
      final transport = ProductionTransport();
      final app = await financeApp(transport);
      final runtime = SupportRuntime(app);
      final queue = ProductionOperationsController(runtime, batchesBase);
      await queue.load(status: 'ACTIVE');
      expect(queue.error, isNull);
      final job = queue.rows.first;
      final params = <String, String>{'jobId': job['id']};
      final detail = ProductionOperationsController(
        runtime,
        '$batchesBase/{jobId}',
        parameters: params,
      );
      await detail.load();
      transport.lose = true;
      expect(
        await detail.save(
          method: 'POST',
          route: '$batchesBase/{jobId}/{status}',
          body: {'note': 'Begin lunch preparation'},
          params: {...params, 'status': 'QUEUED'},
          headerVersion: job['version'],
        ),
        isNull,
      );
      expect(detail.pending, isNotNull);
      final key = detail.pending!['key'];
      detail.dispose();
      final reopened = ProductionOperationsController(
        runtime,
        '$batchesBase/{jobId}',
        parameters: params,
      );
      await reopened.load();
      expect(reopened.record!['status'], 'QUEUED');
      expect(reopened.pending!['key'], key);
      expect(await reopened.save(), isNotNull, reason: reopened.error);
      await reopened.load();
      expect(reopened.record!['version'], 2);
      final history = reopened.collection(
        await reopened.fetch('$batchesBase/{jobId}/events', params: params),
      );
      expect(history.where((e) => e['to_status'] == 'QUEUED').length, 1);
      final step = reopened
          .collection(
            await reopened.fetch('$batchesBase/{jobId}/steps', params: params),
          )
          .first;
      expect(
        await reopened.save(
          method: 'POST',
          route: '$batchesBase/{jobId}/steps/{stepId}/{status}',
          params: {...params, 'stepId': step['id'], 'status': 'READY'},
          body: {'expected_version': step['version']},
          headerVersion: 2,
        ),
        isNotNull,
        reason: reopened.error,
      );
      await reopened.load();
      expect(reopened.record!['status'], 'IN_PROGRESS');
      queue.dispose();
      reopened.dispose();
      app.client.close();
    },
  );
  test(
    'capacity release recovers exactly once and ready handoff enforces required steps',
    () async {
      final transport = ProductionTransport();
      final app = await financeApp(transport);
      final runtime = SupportRuntime(app);
      final c = ProductionOperationsController(
        runtime,
        '$productionBase/pools',
      );
      await c.load();
      expect(c.error, isNull);
      final pool = c.rows.first;
      final start = DateTime.now().toUtc().add(const Duration(hours: 2)),
          end = start.add(const Duration(hours: 1));
      final request = <String, dynamic>{
        'branch_id': pool['branch_id'],
        'capability': pool['capability'],
        'source_type': 'MANUAL',
        'window_start': start.toIso8601String(),
        'window_end': end.toIso8601String(),
        'hold_for_minutes': 30,
        'priority': 'NORMAL',
        'allocations': [
          {'capacity_pool_id': pool['id'], 'quantity': 5},
        ],
      };
      final created = await c.save(
        method: 'POST',
        route: '$productionBase/reservations',
        body: request,
      );
      expect(created, isNotNull, reason: c.error);
      final id = created!['id'] as String;
      final params = {'reservationId': id};
      transport.loseRelease = true;
      expect(
        await c.save(
          method: 'POST',
          route: '$productionBase/reservations/{reservationId}/release',
          params: params,
          body: {
            'expected_version': 1,
            'partial': [
              {'capacity_pool_id': pool['id'], 'quantity': 2},
            ],
            'reason': 'Reduced preparation quantity',
          },
        ),
        isNull,
      );
      expect(c.pending, isNotNull);
      c.dispose();
      final reopened = ProductionOperationsController(
        runtime,
        '$productionBase/reservations/{reservationId}',
        parameters: params,
      );
      await reopened.load();
      expect(reopened.record!['version'], 2);
      expect(reopened.pending, isNotNull);
      expect(await reopened.save(), isNotNull, reason: reopened.error);
      await reopened.load();
      expect(reopened.record!['version'], 2);
      expect(
        (reopened.record!['allocations'] as List).first['released_quantity'],
        2,
      );
      final usage = reopened.collection(
        await reopened.fetch(
          '$productionBase/capacity/utilization',
          query: {
            'branch_id': pool['branch_id'],
            'capability': pool['capability'],
            'start': request['window_start'],
            'end': request['window_end'],
          },
        ),
      );
      expect(usage.first['held'], 3);
      final job = ProductionOperationsController(
        runtime,
        '$batchesBase/{jobId}',
        parameters: const {'jobId': 'mock-production-job'},
      );
      await job.load();
      Future<Object?> status(String target, Map<String, dynamic> body) =>
          job.save(
            method: 'POST',
            route:
                '$batchesBase/{jobId}/${switch (target) {
                  'IN_PROGRESS' => 'start',
                  'READY' => 'ready',
                  'COMPLETED' => 'complete',
                  _ => '{status}',
                }}',
            params: {'jobId': 'mock-production-job', 'status': target},
            body: body,
            headerVersion: job.record!['version'],
          );
      expect(await status('IN_PROGRESS', {}), isNotNull);
      await job.load();
      final evidence = {
        'produced_quantity': 24,
        'waste_quantity': 0,
        'packaging_profile': {'note': 'Sealed containers'},
        'packaging_evidence': {'note': 'Packaging verified'},
      };
      expect(await status('READY', evidence), isNull);
      await job.load();
      expect(job.record!['status'], 'IN_PROGRESS');
      for (final target in ['READY', 'IN_PROGRESS', 'COMPLETED']) {
        final step = job
            .collection(
              await job.fetch(
                '$batchesBase/{jobId}/steps',
                params: const {'jobId': 'mock-production-job'},
              ),
            )
            .first;
        expect(
          await job.save(
            method: 'POST',
            route: '$batchesBase/{jobId}/steps/{stepId}/{status}',
            params: {
              'jobId': 'mock-production-job',
              'stepId': step['id'],
              'status': target,
            },
            body: {'expected_version': step['version']},
            headerVersion: job.record!['version'],
          ),
          isNotNull,
          reason: job.error,
        );
        await job.load();
      }
      expect(await status('READY', evidence), isNotNull, reason: job.error);
      await job.load();
      expect(job.record!['ready_evidence_id'], isNotNull);
      expect(
        await status('COMPLETED', {
          'produced_quantity': 24,
          'waste_quantity': 0,
        }),
        isNotNull,
      );
      await job.load();
      expect(job.record!['status'], 'COMPLETED');
      expect(
        await reopened.save(
          method: 'POST',
          route: '$productionBase/reservations/{reservationId}/release',
          params: params,
          body: {},
        ),
        isNull,
      );
      expect(reopened.pending, isNull);
      expect(reopened.writable, isTrue);
      reopened.dispose();
      job.dispose();
      app.client.close();
    },
  );
}
