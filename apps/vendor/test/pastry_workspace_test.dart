import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/features/pastry/state/pastry_workspace_controller.dart';
import 'finance_test.dart' show financeApp;
import 'order_support_test.dart' show SupportRuntime;
import 'crm_workspace_test.dart' show CrmTransport;

void main() {
  test(
    'Pastry product CAS, attachment versions and acknowledgement-only edits',
    () async {
      final transport = CrmTransport(),
          app = await financeApp(transport),
          runtime = SupportRuntime(app);
      final c = PastryWorkspaceController(runtime, '$pastryBase/products');
      await c.load();
      expect(c.error, isNull);
      expect(c.rows.length, 2);
      final product = c.rows.first, p = {'productId': '${product['id']}'};
      expect(
        await c.save(
          method: 'POST',
          route: '$pastryBase/products/{productId}/option-groups',
          params: p,
          body: {
            'group_id': 'mock-pastry-flavours',
            'position': 0,
            'required': true,
            'expected_version': 1,
          },
        ),
        isNotNull,
        reason: c.error,
      );
      expect(c.pending, isNull);
      final detail = (await app.pastryApi.request(
        'GET',
        '$pastryBase/products/{productId}',
        pathParameters: p,
      )).object;
      expect(detail['version'], 2);
      expect(
        await c.save(
          method: 'PATCH',
          route: '$pastryBase/products/{productId}',
          params: p,
          body: {...detail, 'version': 1, 'name': 'Stale name'},
        ),
        isNull,
      );
      expect(c.pending, isNull);
      await c.load();
      expect(
        await c.save(
          method: 'PATCH',
          route: '$pastryBase/options/{optionId}',
          params: {'optionId': 'mock-pastry-vanilla'},
          body: {
            'name': 'Vanilla bean',
            'code': 'VANILLA',
            'price_delta_minor': 0,
            'workload_weight': 1,
            'position': 0,
            'capacity_class': null,
            'version': 1,
          },
        ),
        isNotNull,
        reason: c.error,
      );
      final updated = (await app.pastryApi.request(
        'GET',
        '$pastryBase/products/{productId}',
        pathParameters: p,
      )).object;
      expect(updated['version'], 3);
      expect(
        await c.save(
          method: 'PATCH',
          route: '$pastryBase/option-groups/{groupId}',
          params: {'groupId': 'mock-pastry-flavours'},
          body: {
            'name': 'Flavours',
            'min_select': 1,
            'max_select': 2,
            'version': 1,
          },
        ),
        isNotNull,
        reason: c.error,
      );
      expect(c.pending, isNull);
      final attached = (await app.pastryApi.request(
        'GET',
        '$pastryBase/products/{productId}/option-groups',
        pathParameters: p,
      )).data;
      expect((attached as List).single['group_id'], 'mock-pastry-flavours');
      final latest = (await app.pastryApi.request(
        'GET',
        '$pastryBase/products/{productId}',
        pathParameters: p,
      )).object;
      expect(
        await c.save(
          method: 'DELETE',
          route: '$pastryBase/products/{productId}/option-groups/{groupId}',
          params: {...p, 'groupId': 'mock-pastry-flavours'},
          body: {'expected_version': latest['version']},
        ),
        isNotNull,
        reason: c.error,
      );
      final cleared = (await app.pastryApi.request(
        'GET',
        '$pastryBase/products/{productId}/option-groups',
        pathParameters: p,
      )).data;
      expect(cleared, isEmpty);
      c.dispose();
      app.client.close();
    },
  );
  test(
    'Request creation recovery, specification revisions and submit/cancel acknowledgement',
    () async {
      final transport = CrmTransport(),
          app = await financeApp(transport),
          runtime = SupportRuntime(app);
      final c = PastryWorkspaceController(
        runtime,
        '$pastryBase/custom-requests',
      );
      await c.load();
      final request = c.rows.first, p = {'requestId': '${request['id']}'};
      expect(
        await c.save(
          method: 'POST',
          route: '$pastryBase/custom-requests/{requestId}/specifications',
          params: p,
          body: {
            'expected_version': 1,
            'specification': {
              'occasion': 'Birthday',
              'servings': 24,
              'theme': 'Gold',
              'notes': 'No invented fields',
            },
          },
        ),
        isNotNull,
        reason: c.error,
      );
      expect(
        await c.save(
          method: 'POST',
          route: '$pastryBase/custom-requests/{requestId}/submit',
          params: p,
          body: {'expected_version': 2},
        ),
        isNotNull,
        reason: c.error,
      );
      expect(c.pending, isNull);
      final detail = (await app.pastryApi.request(
        'GET',
        '$pastryBase/custom-requests/{requestId}',
        pathParameters: p,
      )).object;
      expect(detail['status'], 'SUBMITTED');
      expect(detail['version'], 3);
      expect(
        await c.save(
          method: 'POST',
          route: '$pastryBase/custom-requests/{requestId}/cancel',
          params: p,
          body: {
            'expected_version': 3,
            'reason': 'Customer cancelled',
            'review_approved': false,
          },
        ),
        isNotNull,
        reason: c.error,
      );
      await c.load();
      transport.lose = true;
      transport.suffix = '/custom-requests';
      expect(
        await c.save(
          method: 'POST',
          route: '$pastryBase/custom-requests',
          body: {
            'branch_id': request['branch_id'],
            'title': 'New celebration',
            'notes': 'Mock testing',
          },
        ),
        isNull,
      );
      expect(c.pending, isNotNull);
      final saved = jsonObject(c.pending!);
      c.dispose();
      final restored = PastryWorkspaceController(
        runtime,
        '$pastryBase/custom-requests',
      );
      await restored.load();
      expect(restored.pending?['key'], saved['key']);
      expect(await restored.save(), isNotNull, reason: restored.error);
      await restored.load();
      expect(
        restored.rows.where((r) => r['title'] == 'New celebration').length,
        1,
      );
      restored.dispose();
      app.client.close();
    },
  );
  test(
    'Request configuration and extended designs preserve server fields',
    () async {
      final app = await financeApp(CrmTransport());
      final runtime = SupportRuntime(app);
      final c = PastryWorkspaceController(
        runtime,
        '$pastryBase/custom-requests',
      );
      await c.load();
      final result = await c.save(
        method: 'POST',
        route: '$pastryBase/custom-requests',
        body: {
          'branch_id': c.rows.first['branch_id'],
          'title': 'Celebration design',
          'change_policy': {
            'minutes_before_required_at': 1440,
            'disallowed_after_milestone': 'DECORATION',
          },
          'cancellation_policy': {'review_after_milestone': 'BASE_PREPARED'},
          'logistics': {'temperature_class': 'CHILLED', 'fragile': true},
          'risk_flags': ['MATERIAL_DEPENDENCY'],
        },
      );
      expect(result, isNotNull, reason: c.error);
      await c.load();
      final request = c.rows.singleWhere(
        (r) => r['title'] == 'Celebration design',
      );
      expect(request['risk_flags'], ['MATERIAL_DEPENDENCY']);
      expect(request['change_policy']['minutes_before_required_at'], 1440);
      final params = {'requestId': '${request['id']}'};
      final design = {
        'servings': 24,
        'colours': ['Ivory', 'Gold'],
        'selected_options': ['mock-pastry-vanilla'],
        'reference_media': ['existing-media-reference'],
        'production_requirements': {
          'saved_requirement': 'Preserve existing server content',
        },
      };
      expect(
        await c.save(
          method: 'POST',
          route: '$pastryBase/custom-requests/{requestId}/specifications',
          params: params,
          body: {'expected_version': 1, 'specification': design},
        ),
        isNotNull,
        reason: c.error,
      );
      final versions =
          (await app.pastryApi.request(
                'GET',
                '$pastryBase/custom-requests/{requestId}/specifications',
                pathParameters: params,
              )).data
              as List;
      expect(versions.single['specification'], design);
      expect(
        await c.save(
          method: 'POST',
          route: '$pastryBase/custom-requests/{requestId}/specifications',
          params: params,
          body: {
            'expected_version': 2,
            'specification': {
              ...design,
              'reference_media': [''],
            },
          },
        ),
        isNull,
      );
      c.dispose();
      app.client.close();
    },
  );
}
