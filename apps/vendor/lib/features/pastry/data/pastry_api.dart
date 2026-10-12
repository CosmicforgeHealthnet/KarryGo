// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/pastry_models.dart';
import 'pastry_routes.dart';

class VendorPastryApi extends VendorFeatureApi {
  VendorPastryApi(VendorApiClient client) : super(client, pastryOperations);

  /// Source: services/vendor-service/internal/features/pastry/http.go#decideChange
  Future<VendorResponse> postPastryChangesChangeIdDecision({
    required String changeId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryChangesChangeIdDecisionRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/changes/{changeId}/decision',
    pathParameters: {'changeId': changeId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getPastryChangesChangeIdHistory({
    required String changeId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/pastry/changes/{changeId}/history',
    pathParameters: {'changeId': changeId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getPastryCustomRequests({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/pastry/custom-requests',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#request
  Future<VendorResponse> postPastryCustomRequests({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryCustomRequestsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/custom-requests',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getPastryCustomRequestsRequestId({
    required String requestId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/pastry/custom-requests/{requestId}',
    pathParameters: {'requestId': requestId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#updateRequest
  Future<VendorResponse> patchPastryCustomRequestsRequestId({
    required String requestId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchPastryCustomRequestsRequestIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/pastry/custom-requests/{requestId}',
    pathParameters: {'requestId': requestId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#cancelRequest
  Future<VendorResponse> postPastryCustomRequestsRequestIdCancel({
    required String requestId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryCustomRequestsRequestIdCancelRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/custom-requests/{requestId}/cancel',
    pathParameters: {'requestId': requestId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse>
  getPastryCustomRequestsRequestIdCancellationEligibility({
    required String requestId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/pastry/custom-requests/{requestId}/cancellation-eligibility',
    pathParameters: {'requestId': requestId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getPastryCustomRequestsRequestIdChangeEligibility({
    required String requestId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/pastry/custom-requests/{requestId}/change-eligibility',
    pathParameters: {'requestId': requestId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getPastryCustomRequestsRequestIdChanges({
    required String requestId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/pastry/custom-requests/{requestId}/changes',
    pathParameters: {'requestId': requestId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#change
  Future<VendorResponse> postPastryCustomRequestsRequestIdChanges({
    required String requestId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryCustomRequestsRequestIdChangesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/custom-requests/{requestId}/changes',
    pathParameters: {'requestId': requestId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getPastryCustomRequestsRequestIdMilestones({
    required String requestId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/pastry/custom-requests/{requestId}/milestones',
    pathParameters: {'requestId': requestId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse>
  getPastryCustomRequestsRequestIdMilestonesMilestoneTypeHistory({
    required String requestId,
    required String milestoneType,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/pastry/custom-requests/{requestId}/milestones/{milestoneType}/history',
    pathParameters: {'requestId': requestId, 'milestoneType': milestoneType},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#transitionMilestone
  Future<VendorResponse>
  postPastryCustomRequestsRequestIdMilestonesMilestoneTypeState({
    required String requestId,
    required String milestoneType,
    required String state,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryCustomRequestsRequestIdMilestonesMilestoneTypeStateRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/custom-requests/{requestId}/milestones/{milestoneType}/{state}',
    pathParameters: {
      'requestId': requestId,
      'milestoneType': milestoneType,
      'state': state,
    },
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getPastryCustomRequestsRequestIdQuotes({
    required String requestId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/pastry/custom-requests/{requestId}/quotes',
    pathParameters: {'requestId': requestId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#quote
  Future<VendorResponse> postPastryCustomRequestsRequestIdQuotes({
    required String requestId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryCustomRequestsRequestIdQuotesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/custom-requests/{requestId}/quotes',
    pathParameters: {'requestId': requestId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getPastryCustomRequestsRequestIdSpecifications({
    required String requestId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/pastry/custom-requests/{requestId}/specifications',
    pathParameters: {'requestId': requestId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#spec
  Future<VendorResponse> postPastryCustomRequestsRequestIdSpecifications({
    required String requestId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryCustomRequestsRequestIdSpecificationsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/custom-requests/{requestId}/specifications',
    pathParameters: {'requestId': requestId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getPastryCustomRequestsRequestIdSpecificationsDiff({
    required String requestId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/pastry/custom-requests/{requestId}/specifications/diff',
    pathParameters: {'requestId': requestId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#submitRequest
  Future<VendorResponse> postPastryCustomRequestsRequestIdSubmit({
    required String requestId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryCustomRequestsRequestIdSubmitRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/custom-requests/{requestId}/submit',
    pathParameters: {'requestId': requestId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getPastryOptionGroups({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/pastry/option-groups',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#createGroup
  Future<VendorResponse> postPastryOptionGroups({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryOptionGroupsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/option-groups',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getPastryOptionGroupsGroupId({
    required String groupId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/pastry/option-groups/{groupId}',
    pathParameters: {'groupId': groupId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#updateGroup
  Future<VendorResponse> patchPastryOptionGroupsGroupId({
    required String groupId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchPastryOptionGroupsGroupIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/pastry/option-groups/{groupId}',
    pathParameters: {'groupId': groupId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#archiveGroup
  Future<VendorResponse> postPastryOptionGroupsGroupIdArchive({
    required String groupId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryOptionGroupsGroupIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/option-groups/{groupId}/archive',
    pathParameters: {'groupId': groupId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getPastryOptionGroupsGroupIdOptions({
    required String groupId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/pastry/option-groups/{groupId}/options',
    pathParameters: {'groupId': groupId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#addOption
  Future<VendorResponse> postPastryOptionGroupsGroupIdOptions({
    required String groupId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryOptionGroupsGroupIdOptionsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/option-groups/{groupId}/options',
    pathParameters: {'groupId': groupId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#updateOption
  Future<VendorResponse> patchPastryOptionsOptionId({
    required String optionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchPastryOptionsOptionIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/pastry/options/{optionId}',
    pathParameters: {'optionId': optionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#archiveOption
  Future<VendorResponse> postPastryOptionsOptionIdArchive({
    required String optionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryOptionsOptionIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/options/{optionId}/archive',
    pathParameters: {'optionId': optionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getPastryProducts({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/pastry/products',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#createProduct
  Future<VendorResponse> postPastryProducts({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryProductsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/products',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getPastryProductsProductId({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/pastry/products/{productId}',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#updateProduct
  Future<VendorResponse> patchPastryProductsProductId({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchPastryProductsProductIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/pastry/products/{productId}',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#archiveProduct
  Future<VendorResponse> postPastryProductsProductIdArchive({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryProductsProductIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/products/{productId}/archive',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#previewStandardBatch
  Future<VendorResponse> postPastryProductsProductIdCapacityPreview({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryProductsProductIdCapacityPreviewRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/products/{productId}/capacity-preview',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#attach
  Future<VendorResponse> postPastryProductsProductIdOptionGroups({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryProductsProductIdOptionGroupsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/products/{productId}/option-groups',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#detach
  Future<VendorResponse> deletePastryProductsProductIdOptionGroupsGroupId({
    required String productId,
    required String groupId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required DeletePastryProductsProductIdOptionGroupsGroupIdRequest body,
  }) => request(
    'DELETE',
    '/api/v1/vendor/pastry/products/{productId}/option-groups/{groupId}',
    pathParameters: {'productId': productId, 'groupId': groupId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#createStandardBatch
  Future<VendorResponse> postPastryProductsProductIdProductionBatches({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryProductsProductIdProductionBatchesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/products/{productId}/production-batches',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getPastryQuotesQuoteId({
    required String quoteId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/pastry/quotes/{quoteId}',
    pathParameters: {'quoteId': quoteId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#accept
  Future<VendorResponse> postPastryQuotesQuoteIdAccept({
    required String quoteId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryQuotesQuoteIdAcceptRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/quotes/{quoteId}/accept',
    pathParameters: {'quoteId': quoteId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#approve
  Future<VendorResponse> postPastryQuotesQuoteIdApprove({
    required String quoteId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryQuotesQuoteIdApproveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/quotes/{quoteId}/approve',
    pathParameters: {'quoteId': quoteId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#decline
  Future<VendorResponse> postPastryQuotesQuoteIdDecline({
    required String quoteId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryQuotesQuoteIdDeclineRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/quotes/{quoteId}/decline',
    pathParameters: {'quoteId': quoteId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/pastry/http.go#reviseQuote
  Future<VendorResponse> postPastryQuotesQuoteIdRevise({
    required String quoteId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPastryQuotesQuoteIdReviseRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/pastry/quotes/{quoteId}/revise',
    pathParameters: {'quoteId': quoteId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );
}
