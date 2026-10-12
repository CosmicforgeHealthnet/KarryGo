// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import 'recipe_cost_routes.dart';

class VendorRecipeCostApi extends VendorFeatureApi {
  VendorRecipeCostApi(VendorApiClient client)
    : super(client, recipeCostOperations);

  /// Source: section16d-vendor-operational-cost.openapi.json
  Future<VendorResponse> getRecipeCostRecipesRecipeId({
    required String recipeId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/recipe-cost/recipes/{recipeId}',
    pathParameters: {'recipeId': recipeId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section16d-vendor-operational-cost.openapi.json
  Future<VendorResponse> getRecipeCostRecipesRecipeIdHistory({
    required String recipeId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/recipe-cost/recipes/{recipeId}/history',
    pathParameters: {'recipeId': recipeId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section16d-vendor-operational-cost.openapi.json
  Future<VendorResponse> getRecipeCostVersionsVersionId({
    required String versionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/recipe-cost/versions/{versionId}',
    pathParameters: {'versionId': versionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section16d-vendor-operational-cost.openapi.json
  Future<VendorResponse> getRecipeCostVersionsVersionIdLatest({
    required String versionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/recipe-cost/versions/{versionId}/latest',
    pathParameters: {'versionId': versionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section16d-vendor-operational-cost.openapi.json
  Future<VendorResponse> postRecipeCostVersionsVersionIdSnapshots({
    required String versionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/recipe-cost/versions/{versionId}/snapshots',
    pathParameters: {'versionId': versionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );
}
