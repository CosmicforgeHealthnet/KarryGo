// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import 'production_cost_routes.dart';

class VendorProductionCostApi extends VendorFeatureApi {
  VendorProductionCostApi(VendorApiClient client)
    : super(client, productionCostOperations);

  /// Source: section16d-vendor-operational-cost.openapi.json
  Future<VendorResponse> getProductionCostBatchesBatchId({
    required String batchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production-cost/batches/{batchId}',
    pathParameters: {'batchId': batchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section16d-vendor-operational-cost.openapi.json
  Future<VendorResponse> postProductionCostBatchesBatchIdActual({
    required String batchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/production-cost/batches/{batchId}/actual',
    pathParameters: {'batchId': batchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16d-vendor-operational-cost.openapi.json
  Future<VendorResponse> postProductionCostBatchesBatchIdExpected({
    required String batchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/production-cost/batches/{batchId}/expected',
    pathParameters: {'batchId': batchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16d-vendor-operational-cost.openapi.json
  Future<VendorResponse> getProductionCostBatchesBatchIdVariance({
    required String batchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production-cost/batches/{batchId}/variance',
    pathParameters: {'batchId': batchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section16d-vendor-operational-cost.openapi.json
  Future<VendorResponse> getProductionCostHistory({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production-cost/history',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );
}
