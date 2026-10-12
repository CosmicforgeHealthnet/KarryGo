import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:vendor/features/production_batches/state/production_batches_workspace_controller.dart';
import 'package:vendor/features/recipes/state/recipes_workspace_controller.dart';
import 'finance_test.dart' show FinanceTransport, financeApp;
import 'order_support_test.dart' show SupportRuntime;

class BatchTransport extends FinanceTransport {
  bool loseComplete = true;
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    final response = await super.send(request);
    if (loseComplete && request.url.path.endsWith('/complete')) {
      loseComplete = false;
      await response.stream.drain<void>();
      throw http.ClientException('Accepted response lost');
    }
    return response;
  }
}

void main() {
  test(
    'prepared output is posted once after lost completion response and reopening',
    () async {
      final app = await financeApp(BatchTransport());
      final runtime = SupportRuntime(app);
      final rc = RecipesWorkspaceController(runtime, recipesBase);
      await rc.load();
      final r = rc.rows.first;
      final versions = rc.collection(
        await rc.fetch(
          '$recipesBase/{recipeId}/versions',
          params: {'recipeId': r['id']},
        ),
      );
      expect(
        await rc.save(
          method: 'POST',
          route: '$recipesBase/{recipeId}/versions/{versionId}/activate',
          body: {},
          params: {'recipeId': r['id'], 'versionId': versions.first['id']},
          headerVersion: 1,
        ),
        isNotNull,
        reason: rc.error,
      );
      final policyPath =
          '/api/v1/vendor/production-stock-policies/branches/{branchId}';
      final policy = ProductionBatchesWorkspaceController(
        runtime,
        policyPath,
        parameters: {'branchId': r['branch_id']},
        branchId: r['branch_id'],
      );
      policy.readQuery = {
        for (final key in ['target_type', 'target_ref', 'target_variant_ref'])
          key: '${r[key] ?? ''}',
      };
      await policy.load();
      expect(policy.error, isNull);
      expect(policy.record!['version'], 0);
      final prepared = 'mock-prepared-output-${r['branch_id']}';
      expect(
        await policy.save(
          method: 'PUT',
          route: policyPath,
          body: {
            for (final key in [
              'target_type',
              'target_ref',
              'target_variant_ref',
            ])
              key: r[key] ?? '',
            'operating_model': 'BATCH_STOCK',
            'prepared_inventory_item_id': prepared,
            'output_inventory_numerator': 1,
            'output_inventory_denominator': 1,
          },
          params: {'branchId': r['branch_id']},
          headerVersion: 0,
        ),
        isNotNull,
        reason: policy.error,
      );
      final list = ProductionBatchesWorkspaceController(runtime, batchesBase);
      await list.load();
      expect(list.error, isNull);
      final created = await list.save(
        method: 'POST',
        route: batchesBase,
        body: {
          'branch_id': r['branch_id'],
          'recipe_id': r['id'],
          'recipe_version_id': versions.first['id'],
          'planned_output_numerator': 3,
          'planned_output_denominator': 1,
        },
      );
      expect(created, isNotNull, reason: list.error);
      final path = '$batchesBase/{batchId}',
          params = <String, String>{'batchId': created!['id']};
      final c = ProductionBatchesWorkspaceController(
        runtime,
        path,
        parameters: params,
      );
      await c.load();
      expect(
        await c.save(
          method: 'POST',
          route: '$path/plan',
          body: {},
          params: params,
          headerVersion: 1,
        ),
        isNotNull,
        reason: c.error,
      );
      expect(
        await c.save(
          method: 'POST',
          route: '$path/start',
          body: {},
          params: params,
          headerVersion: 2,
        ),
        isNotNull,
        reason: c.error,
      );
      expect(
        await c.save(
          method: 'POST',
          route: '$path/complete',
          body: {'actual_output_numerator': 3, 'actual_output_denominator': 1},
          params: params,
          headerVersion: 3,
        ),
        isNull,
      );
      expect(c.pending, isNotNull);
      c.dispose();
      final reopened = ProductionBatchesWorkspaceController(
        runtime,
        path,
        parameters: params,
      );
      await reopened.load();
      expect(reopened.pending, isNotNull);
      expect(await reopened.save(), isNotNull, reason: reopened.error);
      final output = (await app.productionBatchesApi.request(
        'GET',
        '$path/output',
        pathParameters: params,
      )).object;
      expect(output['quantity'], 3);
      final balances =
          (await app.inventoryApi.request(
                'GET',
                '/api/v1/vendor/inventory/branches/{branchId}/balances',
                pathParameters: {'branchId': r['branch_id']},
              )).object['items']
              as List;
      expect(
        balances.firstWhere(
          (b) => b['inventory_item_id'] == prepared,
        )['on_hand'],
        3,
      );
      rc.dispose();
      policy.dispose();
      list.dispose();
      reopened.dispose();
      app.client.close();
    },
  );
}
