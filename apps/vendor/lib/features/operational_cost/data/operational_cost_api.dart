// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/operational_cost_models.dart';
import 'operational_cost_routes.dart';

class VendorOperationalCostApi extends VendorFeatureApi {
  VendorOperationalCostApi(VendorApiClient client)
    : super(client, operationalCostOperations);

  /// Source: section16d-vendor-operational-cost.openapi.json
  Future<VendorResponse> getCostHighestCostSubjects({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/cost/highest-cost-subjects',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section16d-vendor-operational-cost.openapi.json
  Future<VendorResponse> getCostPolicies({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/cost/policies',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/cost/http.go#createPolicy
  Future<VendorResponse> postCostPolicies({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCostPoliciesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/cost/policies',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16d-vendor-operational-cost.openapi.json
  Future<VendorResponse> getCostPoliciesPolicyId({
    required String policyId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/cost/policies/{policyId}',
    pathParameters: {'policyId': policyId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/cost/http.go#updatePolicy
  Future<VendorResponse> putCostPoliciesPolicyId({
    required String policyId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutCostPoliciesPolicyIdRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/cost/policies/{policyId}',
    pathParameters: {'policyId': policyId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/cost/http.go#archivePolicy
  Future<VendorResponse> postCostPoliciesPolicyIdArchive({
    required String policyId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCostPoliciesPolicyIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/cost/policies/{policyId}/archive',
    pathParameters: {'policyId': policyId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section16d-vendor-operational-cost.openapi.json
  Future<VendorResponse> getCostSnapshotsSnapshotIdComponents({
    required String snapshotId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/cost/snapshots/{snapshotId}/components',
    pathParameters: {'snapshotId': snapshotId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );
}
