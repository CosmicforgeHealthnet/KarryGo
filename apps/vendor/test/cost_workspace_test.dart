import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:vendor/features/operational_cost/state/cost_workspace_controller.dart';
import 'package:vendor/features/recipes/state/recipes_workspace_controller.dart';
import 'package:vendor/features/production_batches/state/production_batches_workspace_controller.dart'
    show ProductionBatchesWorkspaceController;
import 'finance_test.dart' show FinanceTransport, financeApp;
import 'order_support_test.dart' show SupportRuntime;

class CostTransport extends FinanceTransport {
  bool lose = false;
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    final result = await super.send(request);
    if (lose &&
        request.method == 'POST' &&
        request.url.path.endsWith('/expected')) {
      lose = false;
      await result.stream.drain<void>();
      throw http.ClientException('Accepted cost response lost');
    }
    return result;
  }
}

void main() {
  test(
    'cost policies value frozen batch quantities and retry snapshots once',
    () async {
      final transport = CostTransport();
      final app = await financeApp(transport);
      final runtime = SupportRuntime(app);
      final recipes = RecipesWorkspaceController(runtime, recipesBase);
      await recipes.load();
      final recipe = recipes.rows.first;
      final versions = recipes.collection(
        await recipes.fetch(
          '$recipesBase/{recipeId}/versions',
          params: {'recipeId': recipe['id']},
        ),
      );
      expect(
        await recipes.save(
          method: 'POST',
          route: '$recipesBase/{recipeId}/versions/{versionId}/activate',
          body: {},
          params: {'recipeId': recipe['id'], 'versionId': versions.first['id']},
          headerVersion: 1,
        ),
        isNotNull,
        reason: recipes.error,
      );
      final batches = ProductionBatchesWorkspaceController(
        runtime,
        '/api/v1/vendor/production-batches',
      );
      await batches.load();
      final batch = await batches.save(
        method: 'POST',
        route: '/api/v1/vendor/production-batches',
        body: {
          'branch_id': recipe['branch_id'],
          'recipe_id': recipe['id'],
          'recipe_version_id': versions.first['id'],
          'planned_output_numerator': 3,
          'planned_output_denominator': 1,
        },
      );
      expect(batch, isNotNull, reason: batches.error);
      const path = '/api/v1/vendor/production-cost/batches/{batchId}';
      final params = <String, String>{'batchId': batch!['id']};
      final batchDetail = ProductionBatchesWorkspaceController(
        runtime,
        '/api/v1/vendor/production-batches/{batchId}',
        parameters: params,
      );
      await batchDetail.load();
      expect(
        await batchDetail.save(
          method: 'POST',
          route: '/api/v1/vendor/production-batches/{batchId}/plan',
          body: {},
          params: params,
          headerVersion: 1,
        ),
        isNotNull,
        reason: batchDetail.error,
      );
      final costs = CostWorkspaceController(
        runtime,
        path,
        parameters: params,
        allowMissing: true,
      );
      await costs.load();
      expect(costs.writable, true);
      final unknown = await costs.save(
        method: 'POST',
        route: '$path/expected',
        body: {},
        params: params,
      );
      expect(unknown?['completeness'], 'UNKNOWN');
      expect(unknown?.containsKey('total_cost_minor'), false);
      const policies = '/api/v1/vendor/cost/policies';
      final pc = CostWorkspaceController(
        runtime,
        policies,
        wrapper: 'policies',
      );
      await pc.load();
      final body = {
        'branch_id': '',
        'subject_type': 'INGREDIENT',
        'subject_ref': 'mock-production-rice',
        'strategy': 'MANUAL_CONFIGURED_COST',
        'currency': 'NGN',
        'weighted_average_receipt_limit': 10,
        'manual_unit_cost_minor': 2,
        'manual_unit_code': 'G',
        'manual_cost_reason': 'Mock supplier estimate',
        'reason': 'Initial valuation',
      };
      final policy = await pc.save(method: 'POST', route: policies, body: body);
      expect(policy, isNotNull, reason: pc.error);
      transport.lose = true;
      expect(
        await costs.save(
          method: 'POST',
          route: '$path/expected',
          body: {},
          params: params,
        ),
        isNull,
      );
      final key = costs.pending!['key'];
      costs.dispose();
      final reopened = CostWorkspaceController(
        runtime,
        path,
        parameters: params,
        allowMissing: true,
      );
      await reopened.load();
      expect(reopened.pending!['key'], key);
      final snapshot = await reopened.save();
      expect(snapshot?['total_cost_minor'], 1500, reason: reopened.error);
      final history =
          (await app.productionCostApi.request(
                'GET',
                '/api/v1/vendor/production-cost/history',
              )).object['snapshots']
              as List;
      expect(history.length, 2);
      expect(
        await pc.save(
          method: 'PUT',
          route: '$policies/{policyId}',
          body: {...body, 'manual_unit_cost_minor': 4},
          params: {'policyId': policy!['id']},
          headerVersion: 1,
        ),
        isNotNull,
        reason: pc.error,
      );
      final evidence =
          (await app.operationalCostApi.request(
                'GET',
                '/api/v1/vendor/cost/snapshots/{snapshotId}/components',
                pathParameters: {'snapshotId': snapshot!['id']},
              )).object['components']
              as List;
      expect(evidence.first['unit_cost_minor'], 2);
      final recipeCost = (await app.recipeCostApi.request(
        'GET',
        '/api/v1/vendor/recipe-cost/versions/{versionId}',
        pathParameters: {'versionId': versions.first['id']},
        query: {'branch_id': recipe['branch_id']},
      )).object;
      expect(recipeCost['total_cost_minor'], 1000);
      final next = await reopened.save(
        method: 'POST',
        route: '$path/expected',
        body: {},
        params: params,
      );
      expect(next?['total_cost_minor'], 3000);
      final variance = (await app.productionCostApi.request(
        'GET',
        '$path/variance',
        pathParameters: params,
      )).object;
      expect(variance['completeness'], 'INCOMPLETE');
      expect(variance.containsKey('variance_minor'), false);
      final actual = await reopened.save(
        method: 'POST',
        route: '$path/actual',
        body: {},
        params: params,
      );
      expect(actual?['total_cost_minor'], 0);
      final completeVariance = (await app.productionCostApi.request(
        'GET',
        '$path/variance',
        pathParameters: params,
      )).object;
      expect(completeVariance['completeness'], 'COMPLETE');
      expect(completeVariance['variance_minor'], -3000);
      final detail = CostWorkspaceController(
        runtime,
        '$policies/{policyId}',
        parameters: {'policyId': policy['id']},
      );
      await detail.load();
      expect(
        await detail.save(
          method: 'PUT',
          route: '$policies/{policyId}',
          body: body,
          params: {'policyId': policy['id']},
          headerVersion: 1,
        ),
        isNull,
      );
      expect(detail.pending, isNull);
      expect(detail.stale, true);
      batchDetail.dispose();
      recipes.dispose();
      batches.dispose();
      pc.dispose();
      detail.dispose();
      reopened.dispose();
      app.client.close();
    },
  );
}
