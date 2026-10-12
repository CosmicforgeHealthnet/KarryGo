import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/features/crm/state/crm_workspace_controller.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'finance_test.dart' show financeApp;
import 'order_support_test.dart' show SupportRuntime;
import 'crm_workspace_test.dart' show CrmTransport;

void main() {
  test(
    'Campaign preparation, recoverable execution and scoped delivery evidence',
    () async {
      final transport = CrmTransport(),
          app = await financeApp(transport),
          runtime = SupportRuntime(app);
      final list = CrmWorkspaceController(runtime, '$crmBase/campaigns');
      await list.load();
      expect(list.error, isNull);
      expect(list.rows, isNotEmpty);
      final ref = list.rows.first['campaign_ref'] as String;
      final params = {'campaignId': ref};
      final detail = CrmWorkspaceController(
        runtime,
        '$crmBase/campaigns/{campaignId}',
        parameters: params,
      );
      await detail.load();
      expect(detail.record?['campaign']['status'], 'DRAFT');
      final preview = (await app.crmApi.request(
        'POST',
        '$crmBase/campaigns/{campaignId}/preview',
        pathParameters: params,
        body: VendorPayload({'sample_limit': 25}),
      )).object;
      expect(preview['target_count'], 2);
      expect(preview['channels'][0]['consent_eligible_count'], 0);
      final snapshot = await detail.save(
        method: 'POST',
        route: '$crmBase/campaigns/{campaignId}/prepare',
        params: params,
        body: {},
      );
      expect(snapshot?['status'], 'CURRENT', reason: detail.error);
      await detail.load();
      transport.lose = true;
      transport.suffix = '/runs';
      final body = {
        'snapshot_ref': snapshot!['snapshot_ref'],
        'mode': 'SEND_NOW',
        'expected_version': detail.record!['campaign']['version'],
        'title': 'New menu',
        'body': 'Explore our menu',
      };
      expect(
        await detail.save(
          method: 'POST',
          route: '$crmBase/campaigns/{campaignId}/runs',
          params: params,
          body: body,
        ),
        isNull,
      );
      final pendingKey = detail.pending!['key'];
      detail.dispose();
      final reopened = CrmWorkspaceController(
        runtime,
        '$crmBase/campaigns/{campaignId}',
        parameters: params,
      );
      await reopened.load();
      expect(reopened.pending!['key'], pendingKey);
      final run = await reopened.save();
      expect(run, isNotNull, reason: reopened.error);
      expect(run!['delivered_count'], 0);
      expect(run['suppressed_count'], 0);
      final runs = CrmWorkspaceController(
        runtime,
        '$crmBase/campaigns/{campaignId}/runs',
        parameters: params,
      );
      await runs.load();
      expect(runs.rows.where((r) => r['run_ref'] == run['run_ref']).length, 1);
      final runParams = {
        ...params,
        'runId':
            runs.rows
                    .where((r) => r['run_ref'] != run['run_ref'])
                    .first['run_ref']
                as String,
      };
      final deliveries = CrmWorkspaceController(
        runtime,
        '$crmBase/campaigns/{campaignId}/runs/{runId}/deliveries',
        parameters: runParams,
      );
      await deliveries.load();
      expect(deliveries.rows.length, 2);
      expect(deliveries.rows.every((r) => r['status'] == 'SUPPRESSED'), isTrue);
      final history = CrmWorkspaceController(
        runtime,
        '$crmBase/campaigns/{campaignId}/runs/{runId}/deliveries/{deliveryId}/history',
        parameters: {
          ...runParams,
          'deliveryId': deliveries.rows.first['delivery_ref'] as String,
        },
      );
      await history.load();
      expect(history.rows.length, 1);
      final wrong = (await app.crmApi.request(
        'GET',
        '$crmBase/campaigns/{campaignId}/runs/{runId}',
        pathParameters: runParams,
      )).object;
      expect(wrong['run_ref'], runParams['runId']);
      final scheduled = await reopened.save(
        method: 'POST',
        route: '$crmBase/campaigns/{campaignId}/runs',
        params: params,
        body: {
          ...body,
          'mode': 'SCHEDULED',
          'scheduled_for': DateTime.now()
              .add(const Duration(days: 1))
              .toUtc()
              .toIso8601String(),
        },
      );
      expect(scheduled?['status'], 'SCHEDULED', reason: reopened.error);
      final scheduledParams = {
        ...params,
        'runId': scheduled!['run_ref'] as String,
      };
      final controls = CrmWorkspaceController(
        runtime,
        '$crmBase/campaigns/{campaignId}/runs/{runId}',
        parameters: scheduledParams,
      );
      await controls.load();
      transport.lose = true;
      transport.suffix = '/pause';
      expect(
        await controls.save(
          method: 'POST',
          route: '$crmBase/campaigns/{campaignId}/runs/{runId}/pause',
          params: scheduledParams,
          body: {'expected_version': 1},
        ),
        isNull,
      );
      expect(controls.pending, isNotNull);
      controls.dispose();
      final recovered = CrmWorkspaceController(
        runtime,
        '$crmBase/campaigns/{campaignId}/runs/{runId}',
        parameters: scheduledParams,
      );
      await recovered.load();
      expect(recovered.record?['status'], 'PAUSED');
      expect(recovered.pending, isNull);
      expect(
        (await recovered.save(
          method: 'POST',
          route: '$crmBase/campaigns/{campaignId}/runs/{runId}/resume',
          params: scheduledParams,
          body: {'expected_version': 2},
        ))?['status'],
        'SCHEDULED',
      );
      expect(
        (await recovered.save(
          method: 'POST',
          route: '$crmBase/campaigns/{campaignId}/runs/{runId}/cancel',
          params: scheduledParams,
          body: {'expected_version': 3},
        ))?['status'],
        'CANCELLED',
      );
      for (final c in [list, reopened, runs, deliveries, history, recovered])
        c.dispose();
      app.client.close();
    },
  );
  test(
    'Campaign definition scope, exclusions and experiment holdouts',
    () async {
      final app = await financeApp(CrmTransport()),
          runtime = SupportRuntime(app);
      final c = CrmWorkspaceController(runtime, '$crmBase/campaigns');
      await c.load();
      final body = <String, dynamic>{
        'name': 'Audience split',
        'objective': 'RETENTION',
        'scope_type': 'BUSINESS',
        'channels': ['IN_APP'],
        'source_segment_ref': 'mock-campaign-segment',
        'exclusion_segment_refs': [],
        'experiment_enabled': true,
        'experiment_variants': [
          {
            'variant_key': 'CONTROL',
            'display_name': 'Control',
            'allocation_bps': 5000,
            'is_control': true,
          },
          {
            'variant_key': 'TREATMENT',
            'display_name': 'Treatment',
            'allocation_bps': 5000,
            'is_control': false,
          },
        ],
      };
      final created = await c.save(
        method: 'POST',
        route: '$crmBase/campaigns',
        body: body,
      );
      expect(created, isNotNull, reason: c.error);
      final params = {
        'campaignId': created!['campaign']['campaign_ref'] as String,
      };
      final preview = (await app.crmApi.request(
        'POST',
        '$crmBase/campaigns/{campaignId}/preview',
        pathParameters: params,
        body: VendorPayload({}),
      )).object;
      final snapshot = await c.save(
        method: 'POST',
        route: '$crmBase/campaigns/{campaignId}/prepare',
        params: params,
        body: {},
      );
      expect(snapshot?['control_count'], preview['control_count']);
      final targets =
          (await app.crmApi.request(
                'GET',
                '$crmBase/campaigns/{campaignId}/snapshots/{snapshotId}/targets',
                pathParameters: {
                  ...params,
                  'snapshotId': snapshot!['snapshot_ref'] as String,
                },
              )).object['targets']
              as List;
      expect(
        targets.where((r) => r['is_control'] == true).length,
        snapshot['control_count'],
      );
      final current = (await app.crmApi.request(
        'GET',
        '$crmBase/campaigns/{campaignId}',
        pathParameters: params,
      )).object['campaign'];
      final run = await c.save(
        method: 'POST',
        route: '$crmBase/campaigns/{campaignId}/runs',
        params: params,
        body: {
          'snapshot_ref': snapshot['snapshot_ref'],
          'expected_version': current['version'],
          'mode': 'SEND_NOW',
          'body': 'Hello',
        },
      );
      expect(run?['total_deliveries'], 0);
      final invalid = await c.save(
        method: 'PATCH',
        route: '$crmBase/campaigns/{campaignId}',
        params: params,
        body: {
          ...body,
          'scope_type': 'BRANCH',
          'branch_ref': 'mock-branch-1',
          'expected_version': current['version'],
        },
      );
      expect(invalid, isNull);
      expect(c.pending, isNull);
      final invalidVariants = await c.save(
        method: 'POST',
        route: '$crmBase/campaigns',
        body: {
          ...body,
          'experiment_variants': [
            {
              'variant_key': 'INVALID',
              'display_name': 'Invalid',
              'allocation_bps': 1,
              'is_control': false,
            },
          ],
        },
      );
      expect(invalidVariants, isNull);
      c.dispose();
      app.client.close();
    },
  );
}
