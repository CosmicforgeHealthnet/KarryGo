// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/production_stock_policies_models.dart';
import 'production_stock_policies_routes.dart';

class VendorProductionStockPoliciesApi extends VendorFeatureApi {
  VendorProductionStockPoliciesApi(VendorApiClient client)
    : super(client, productionStockPoliciesOperations);

  /// Source: section16c-vendor-production-batches.openapi.json
  Future<VendorResponse> getProductionStockPoliciesBranchesBranchId({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production-stock-policies/branches/{branchId}',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/productionbatch/http.go#putPolicy
  Future<VendorResponse> putProductionStockPoliciesBranchesBranchId({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutProductionStockPoliciesBranchesBranchIdRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/production-stock-policies/branches/{branchId}',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );
}
