// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/production_master_models.dart';
import 'production_master_routes.dart';

class VendorProductionMasterApi extends VendorFeatureApi {
  VendorProductionMasterApi(VendorApiClient client)
    : super(client, productionMasterOperations);

  /// Source: section16a-vendor-production-master.openapi.json
  Future<VendorResponse> getIngredients({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/ingredients',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/productionmaster/http.go#createIngredient
  Future<VendorResponse> postIngredients({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostIngredientsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/ingredients',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16a-vendor-production-master.openapi.json
  Future<VendorResponse> getIngredientsIngredientId({
    required String ingredientId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/ingredients/{ingredientId}',
    pathParameters: {'ingredientId': ingredientId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/productionmaster/http.go#updateIngredient
  Future<VendorResponse> putIngredientsIngredientId({
    required String ingredientId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutIngredientsIngredientIdRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/ingredients/{ingredientId}',
    pathParameters: {'ingredientId': ingredientId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16a-vendor-production-master.openapi.json
  Future<VendorResponse> postIngredientsIngredientIdArchive({
    required String ingredientId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/ingredients/{ingredientId}/archive',
    pathParameters: {'ingredientId': ingredientId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16a-vendor-production-master.openapi.json
  Future<VendorResponse> getIngredientsIngredientIdBranchesBranchId({
    required String ingredientId,
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/ingredients/{ingredientId}/branches/{branchId}',
    pathParameters: {'ingredientId': ingredientId, 'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/productionmaster/http.go#putIngredientBranchConfig
  Future<VendorResponse> putIngredientsIngredientIdBranchesBranchId({
    required String ingredientId,
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutIngredientsIngredientIdBranchesBranchIdRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/ingredients/{ingredientId}/branches/{branchId}',
    pathParameters: {'ingredientId': ingredientId, 'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16a-vendor-production-master.openapi.json
  Future<VendorResponse> getIngredientsIngredientIdConversions({
    required String ingredientId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/ingredients/{ingredientId}/conversions',
    pathParameters: {'ingredientId': ingredientId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/productionmaster/http.go#createIngredientConversion
  Future<VendorResponse> postIngredientsIngredientIdConversions({
    required String ingredientId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostIngredientsIngredientIdConversionsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/ingredients/{ingredientId}/conversions',
    pathParameters: {'ingredientId': ingredientId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/productionmaster/http.go#updateIngredientConversion
  Future<VendorResponse> putIngredientsIngredientIdConversionsConversionId({
    required String ingredientId,
    required String conversionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutIngredientsIngredientIdConversionsConversionIdRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/ingredients/{ingredientId}/conversions/{conversionId}',
    pathParameters: {
      'ingredientId': ingredientId,
      'conversionId': conversionId,
    },
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16a-vendor-production-master.openapi.json
  Future<VendorResponse> getPackagingMaterials({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/packaging-materials',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/productionmaster/http.go#createPackaging
  Future<VendorResponse> postPackagingMaterials({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPackagingMaterialsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/packaging-materials',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16a-vendor-production-master.openapi.json
  Future<VendorResponse> getPackagingMaterialsPackagingId({
    required String packagingId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/packaging-materials/{packagingId}',
    pathParameters: {'packagingId': packagingId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/productionmaster/http.go#updatePackaging
  Future<VendorResponse> putPackagingMaterialsPackagingId({
    required String packagingId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutPackagingMaterialsPackagingIdRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/packaging-materials/{packagingId}',
    pathParameters: {'packagingId': packagingId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16a-vendor-production-master.openapi.json
  Future<VendorResponse> postPackagingMaterialsPackagingIdArchive({
    required String packagingId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/packaging-materials/{packagingId}/archive',
    pathParameters: {'packagingId': packagingId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16a-vendor-production-master.openapi.json
  Future<VendorResponse> getPackagingMaterialsPackagingIdBranchesBranchId({
    required String packagingId,
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/packaging-materials/{packagingId}/branches/{branchId}',
    pathParameters: {'packagingId': packagingId, 'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/productionmaster/http.go#putPackagingBranchConfig
  Future<VendorResponse> putPackagingMaterialsPackagingIdBranchesBranchId({
    required String packagingId,
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutPackagingMaterialsPackagingIdBranchesBranchIdRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/packaging-materials/{packagingId}/branches/{branchId}',
    pathParameters: {'packagingId': packagingId, 'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16a-vendor-production-master.openapi.json
  Future<VendorResponse> getPackagingMaterialsPackagingIdConversions({
    required String packagingId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/packaging-materials/{packagingId}/conversions',
    pathParameters: {'packagingId': packagingId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/productionmaster/http.go#createPackagingConversion
  Future<VendorResponse> postPackagingMaterialsPackagingIdConversions({
    required String packagingId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPackagingMaterialsPackagingIdConversionsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/packaging-materials/{packagingId}/conversions',
    pathParameters: {'packagingId': packagingId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/productionmaster/http.go#updatePackagingConversion
  Future<VendorResponse>
  putPackagingMaterialsPackagingIdConversionsConversionId({
    required String packagingId,
    required String conversionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutPackagingMaterialsPackagingIdConversionsConversionIdRequest
    body,
  }) => request(
    'PUT',
    '/api/v1/vendor/packaging-materials/{packagingId}/conversions/{conversionId}',
    pathParameters: {'packagingId': packagingId, 'conversionId': conversionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );
}
