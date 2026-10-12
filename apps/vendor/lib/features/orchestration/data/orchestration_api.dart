// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/orchestration_models.dart';
import 'orchestration_routes.dart';

class VendorOrchestrationApi extends VendorFeatureApi {
  VendorOrchestrationApi(VendorApiClient client)
    : super(client, orchestrationOperations);

  /// Source: services/vendor-service/internal/features/orchestration/http.go
  Future<VendorResponse> getFulfilmentGroupsGroupIdOrchestration({
    required String groupId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/fulfilment/groups/{groupId}/orchestration',
    pathParameters: {'groupId': groupId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/orchestration/http.go#override
  Future<VendorResponse>
  postFulfilmentGroupsGroupIdOrchestrationAuthorityOverride({
    required String groupId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFulfilmentGroupsGroupIdOrchestrationAuthorityOverrideRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/fulfilment/groups/{groupId}/orchestration/authority-override',
    pathParameters: {'groupId': groupId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/orchestration/http.go
  Future<VendorResponse> getFulfilmentGroupsGroupIdOrchestrationDecisions({
    required String groupId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/fulfilment/groups/{groupId}/orchestration/decisions',
    pathParameters: {'groupId': groupId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );
}
