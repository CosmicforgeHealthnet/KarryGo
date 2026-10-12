import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/features/scheduled_commerce/state/scheduled_creation_controller.dart';
import 'package:vendor/features/scheduled_commerce/state/scheduling_workspace_controller.dart';
import 'finance_test.dart' show financeApp;
import 'order_support_test.dart' show SupportRuntime;
import 'crm_workspace_test.dart' show CrmTransport;

void main() {
  test(
    'Approved quote scheduling reserves shared capacity and recovers without duplicates',
    () async {
      final transport = CrmTransport(),
          app = await financeApp(transport),
          runtime = SupportRuntime(app);
      final available = (await app.scheduledCommerceApi.request(
        'GET',
        '$schedulingBase/scheduling-quotes',
      )).object;
      final source = jsonObject((available['items'] as List).single);
      final pools =
          (await app.productionApi.request(
                'GET',
                '/api/v1/vendor/production/pools',
              )).data
              as List;
      final pool = jsonObject(pools.first),
          start = DateTime.now().toUtc().add(const Duration(days: 1)),
          delivery = DateTime.now().toUtc().add(const Duration(days: 2));
      final capacity = {
        'required': true,
        'branch_id': source['BranchID'],
        'capability': pool['capability'],
        'window_start': start.toIso8601String(),
        'window_end': start.add(const Duration(hours: 1)).toIso8601String(),
        'allocations': [
          {'capacity_pool_id': pool['id'], 'quantity': 1},
        ],
      };
      final c = ScheduledCreationController(runtime);
      await c.restore();
      transport.lose = true;
      transport.suffix = '/scheduled-orders';
      expect(
        await c.create(
          source: source,
          changes: {
            'DesiredDeliveryAt': delivery.toIso8601String(),
            'Capacity': capacity,
          },
        ),
        isNull,
      );
      final key = c.pending!['body']['IdempotencyKey'];
      c.dispose();
      final restored = ScheduledCreationController(runtime);
      await restored.restore();
      expect(restored.pending!['body']['IdempotencyKey'], key);
      final created = await restored.create();
      expect(created, isNotNull, reason: restored.error);
      expect(created!['QuoteID'], source['QuoteID']);
      expect(created['CapacityStatus'], 'RESERVED');
      final reservation = (await app.productionApi.request(
        'GET',
        '/api/v1/vendor/production/reservations/{reservationId}',
        pathParameters: {'reservationId': created['CapacityReservationID']},
      )).object;
      expect(reservation['status'], 'CONFIRMED');
      final list = SchedulingWorkspaceController(runtime);
      await list.load();
      expect(list.rows.where((r) => r['ID'] == created['ID']).length, 1);
      final detail = SchedulingWorkspaceController(runtime, id: created['ID']);
      await detail.load();
      expect(
        await detail.changeStatus('CANCELLED'),
        isTrue,
        reason: detail.error,
      );
      expect(detail.record!['CapacityStatus'], 'RELEASED');
      final foreign = {...source, 'BusinessID': 'foreign-vendor'};
      expect(
        await restored.create(
          source: foreign,
          changes: {'DesiredDeliveryAt': delivery.toIso8601String()},
        ),
        isNull,
      );
      expect(restored.pending, isNull);
      detail.dispose();
      list.dispose();
      restored.dispose();
      app.client.close();
    },
  );
}
