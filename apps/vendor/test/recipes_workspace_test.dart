import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/features/recipes/state/recipes_workspace_controller.dart';
import 'finance_test.dart' show FinanceTransport, financeApp;
import 'order_support_test.dart' show SupportRuntime;

void main() {
  test(
    'public recipe contracts support draft activation and immutable cost snapshots',
    () async {
      final transport = FinanceTransport();
      final app = await financeApp(transport);
      final runtime = SupportRuntime(app);
      final list = RecipesWorkspaceController(runtime, recipesBase);
      await list.load();
      expect(list.error, isNull);
      final recipe = list.rows.first;
      final path = '$recipesBase/{recipeId}/versions/{versionId}';
      final versions = list.collection(
        await list.fetch(
          '$recipesBase/{recipeId}/versions',
          params: {'recipeId': recipe['id']},
        ),
      );
      final params = <String, String>{
        'recipeId': recipe['id'],
        'versionId': versions.first['id'],
      };
      final c = RecipesWorkspaceController(runtime, path, parameters: params);
      await c.load();
      expect(c.error, isNull);
      expect(c.record!['version']['status'], 'DRAFT');
      final check = await c.save(
        method: 'POST',
        route: '$path/validate',
        body: {},
        params: params,
      );
      expect(check?['valid'], true, reason: c.error);
      final active = await c.save(
        method: 'POST',
        route: '$path/activate',
        body: {},
        params: params,
        headerVersion: 1,
      );
      expect(active?['status'], 'ACTIVE', reason: c.error);
      final cost = RecipesWorkspaceController(
        runtime,
        '/api/v1/vendor/recipe-cost/versions/{versionId}',
        parameters: {'versionId': params['versionId']!},
        branchId: recipe['branch_id'],
      );
      await cost.load();
      expect(cost.error, isNull);
      expect(cost.record!['total_cost_minor'], isNull);
      final snapshot = await cost.save(
        method: 'POST',
        route: '/api/v1/vendor/recipe-cost/versions/{versionId}/snapshots',
        body: {},
        params: {'versionId': params['versionId']!},
        query: {'branch_id': recipe['branch_id']},
      );
      expect(
        snapshot?['calculation_type'],
        'RECIPE_SNAPSHOT',
        reason: cost.error,
      );
      final retired = await c.save(
        method: 'POST',
        route: '$path/retire',
        body: {},
        params: params,
        headerVersion: 2,
      );
      expect(retired?['status'], 'RETIRED', reason: c.error);
      final archived = await list.save(
        method: 'POST',
        route: '$recipesBase/{recipeId}/archive',
        body: {},
        params: {'recipeId': recipe['id']},
        headerVersion: recipe['version'],
      );
      expect(archived?['status'], 'ARCHIVED', reason: list.error);
      list.dispose();
      c.dispose();
      cost.dispose();
      app.client.close();
    },
  );
}
