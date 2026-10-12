// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/recipes_models.dart';
import 'recipes_routes.dart';

class VendorRecipesApi extends VendorFeatureApi {
  VendorRecipesApi(VendorApiClient client) : super(client, recipesOperations);

  /// Source: section16b-vendor-recipes.openapi.json
  Future<VendorResponse> getRecipes({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/recipes',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/recipes/http.go#createRecipe
  Future<VendorResponse> postRecipes({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostRecipesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/recipes',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16b-vendor-recipes.openapi.json
  Future<VendorResponse> getRecipesRecipeId({
    required String recipeId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/recipes/{recipeId}',
    pathParameters: {'recipeId': recipeId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section16b-vendor-recipes.openapi.json
  Future<VendorResponse> postRecipesRecipeIdArchive({
    required String recipeId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/recipes/{recipeId}/archive',
    pathParameters: {'recipeId': recipeId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16b-vendor-recipes.openapi.json
  Future<VendorResponse> getRecipesRecipeIdVersions({
    required String recipeId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/recipes/{recipeId}/versions',
    pathParameters: {'recipeId': recipeId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/recipes/http.go#createVersion
  Future<VendorResponse> postRecipesRecipeIdVersions({
    required String recipeId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostRecipesRecipeIdVersionsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/recipes/{recipeId}/versions',
    pathParameters: {'recipeId': recipeId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16b-vendor-recipes.openapi.json
  Future<VendorResponse> getRecipesRecipeIdVersionsVersionId({
    required String recipeId,
    required String versionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/recipes/{recipeId}/versions/{versionId}',
    pathParameters: {'recipeId': recipeId, 'versionId': versionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/recipes/http.go#updateVersion
  Future<VendorResponse> putRecipesRecipeIdVersionsVersionId({
    required String recipeId,
    required String versionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutRecipesRecipeIdVersionsVersionIdRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/recipes/{recipeId}/versions/{versionId}',
    pathParameters: {'recipeId': recipeId, 'versionId': versionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16b-vendor-recipes.openapi.json
  Future<VendorResponse> postRecipesRecipeIdVersionsVersionIdActivate({
    required String recipeId,
    required String versionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/recipes/{recipeId}/versions/{versionId}/activate',
    pathParameters: {'recipeId': recipeId, 'versionId': versionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/recipes/http.go#replaceComponents
  Future<VendorResponse> putRecipesRecipeIdVersionsVersionIdComponents({
    required String recipeId,
    required String versionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutRecipesRecipeIdVersionsVersionIdComponentsRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/recipes/{recipeId}/versions/{versionId}/components',
    pathParameters: {'recipeId': recipeId, 'versionId': versionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/recipes/http.go#replacePackaging
  Future<VendorResponse> putRecipesRecipeIdVersionsVersionIdPackaging({
    required String recipeId,
    required String versionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutRecipesRecipeIdVersionsVersionIdPackagingRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/recipes/{recipeId}/versions/{versionId}/packaging',
    pathParameters: {'recipeId': recipeId, 'versionId': versionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16b-vendor-recipes.openapi.json
  Future<VendorResponse> postRecipesRecipeIdVersionsVersionIdRetire({
    required String recipeId,
    required String versionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/recipes/{recipeId}/versions/{versionId}/retire',
    pathParameters: {'recipeId': recipeId, 'versionId': versionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/recipes/http.go#scaleVersion
  Future<VendorResponse> postRecipesRecipeIdVersionsVersionIdScale({
    required String recipeId,
    required String versionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostRecipesRecipeIdVersionsVersionIdScaleRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/recipes/{recipeId}/versions/{versionId}/scale',
    pathParameters: {'recipeId': recipeId, 'versionId': versionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16b-vendor-recipes.openapi.json
  Future<VendorResponse> postRecipesRecipeIdVersionsVersionIdValidate({
    required String recipeId,
    required String versionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/recipes/{recipeId}/versions/{versionId}/validate',
    pathParameters: {'recipeId': recipeId, 'versionId': versionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );
}
