// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/production_batches_models.dart';
import 'production_batches_routes.dart';

class VendorProductionBatchesApi extends VendorFeatureApi {
  VendorProductionBatchesApi(VendorApiClient client)
    : super(client, productionBatchesOperations);

  /// Source: section16c-vendor-production-batches.openapi.json
  Future<VendorResponse> getProductionBatches({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production-batches',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/productionbatch/http.go#createBatch
  Future<VendorResponse> postProductionBatches({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionBatchesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production-batches',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16c-vendor-production-batches.openapi.json
  Future<VendorResponse> getProductionBatchesBatchId({
    required String batchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production-batches/{batchId}',
    pathParameters: {'batchId': batchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/productionbatch/http.go#actual
  Future<VendorResponse> postProductionBatchesBatchIdActualConsumption({
    required String batchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionBatchesBatchIdActualConsumptionRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production-batches/{batchId}/actual-consumption',
    pathParameters: {'batchId': batchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/productionbatch/http.go#cancel
  Future<VendorResponse> postProductionBatchesBatchIdCancel({
    required String batchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionBatchesBatchIdCancelRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production-batches/{batchId}/cancel',
    pathParameters: {'batchId': batchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/productionbatch/http.go#complete
  Future<VendorResponse> postProductionBatchesBatchIdComplete({
    required String batchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionBatchesBatchIdCompleteRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production-batches/{batchId}/complete',
    pathParameters: {'batchId': batchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16c-vendor-production-batches.openapi.json
  Future<VendorResponse> getProductionBatchesBatchIdInventory({
    required String batchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production-batches/{batchId}/inventory',
    pathParameters: {'batchId': batchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/productionbatch/http.go#topUp
  Future<VendorResponse> postProductionBatchesBatchIdInventoryAdjustments({
    required String batchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionBatchesBatchIdInventoryAdjustmentsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production-batches/{batchId}/inventory-adjustments',
    pathParameters: {'batchId': batchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16c-vendor-production-batches.openapi.json
  Future<VendorResponse> getProductionBatchesBatchIdOutput({
    required String batchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production-batches/{batchId}/output',
    pathParameters: {'batchId': batchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section16c-vendor-production-batches.openapi.json
  Future<VendorResponse> postProductionBatchesBatchIdPlan({
    required String batchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/production-batches/{batchId}/plan',
    pathParameters: {'batchId': batchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16c-vendor-production-batches.openapi.json
  Future<VendorResponse> postProductionBatchesBatchIdPreview({
    required String batchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/production-batches/{batchId}/preview',
    pathParameters: {'batchId': batchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16c-vendor-production-batches.openapi.json
  Future<VendorResponse> postProductionBatchesBatchIdStart({
    required String batchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/production-batches/{batchId}/start',
    pathParameters: {'batchId': batchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/productionbatch/http.go#waste
  Future<VendorResponse> postProductionBatchesBatchIdWaste({
    required String batchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionBatchesBatchIdWasteRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production-batches/{batchId}/waste',
    pathParameters: {'batchId': batchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );
}
