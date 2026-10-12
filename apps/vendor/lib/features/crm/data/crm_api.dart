// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/crm_models.dart';
import 'crm_routes.dart';

class VendorCrmApi extends VendorFeatureApi {
  VendorCrmApi(VendorApiClient client) : super(client, crmOperations);

  /// Source: section8e-crm-campaign-intelligence.openapi.json
  Future<VendorResponse> getCrmCampaigns({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/campaigns',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/campaigns_http.go#CreateCampaign
  Future<VendorResponse> postCrmCampaigns({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCampaignsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/campaigns',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8e-crm-campaign-intelligence.openapi.json
  Future<VendorResponse> getCrmCampaignsCampaignId({
    required String campaignId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/campaigns/{campaignId}',
    pathParameters: {'campaignId': campaignId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/campaigns_http.go#UpdateCampaign
  Future<VendorResponse> patchCrmCampaignsCampaignId({
    required String campaignId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchCrmCampaignsCampaignIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/crm/campaigns/{campaignId}',
    pathParameters: {'campaignId': campaignId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/campaigns_http.go#ArchiveCampaign
  Future<VendorResponse> postCrmCampaignsCampaignIdArchive({
    required String campaignId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCampaignsCampaignIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/campaigns/{campaignId}/archive',
    pathParameters: {'campaignId': campaignId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8e-crm-campaign-intelligence.openapi.json
  Future<VendorResponse> postCrmCampaignsCampaignIdPrepare({
    required String campaignId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/campaigns/{campaignId}/prepare',
    pathParameters: {'campaignId': campaignId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/campaigns_http.go#PreviewCampaign
  Future<VendorResponse> postCrmCampaignsCampaignIdPreview({
    required String campaignId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCampaignsCampaignIdPreviewRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/campaigns/{campaignId}/preview',
    pathParameters: {'campaignId': campaignId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8f-campaign-execution.openapi.json
  Future<VendorResponse> getCrmCampaignsCampaignIdRuns({
    required String campaignId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/campaigns/{campaignId}/runs',
    pathParameters: {'campaignId': campaignId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/campaign_execution_http.go#CreateCampaignRun
  Future<VendorResponse> postCrmCampaignsCampaignIdRuns({
    required String campaignId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCampaignsCampaignIdRunsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/campaigns/{campaignId}/runs',
    pathParameters: {'campaignId': campaignId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8f-campaign-execution.openapi.json
  Future<VendorResponse> getCrmCampaignsCampaignIdRunsRunId({
    required String campaignId,
    required String runId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/campaigns/{campaignId}/runs/{runId}',
    pathParameters: {'campaignId': campaignId, 'runId': runId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/campaign_execution_http.go#CancelCampaignRun
  Future<VendorResponse> postCrmCampaignsCampaignIdRunsRunIdCancel({
    required String campaignId,
    required String runId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCampaignsCampaignIdRunsRunIdCancelRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/campaigns/{campaignId}/runs/{runId}/cancel',
    pathParameters: {'campaignId': campaignId, 'runId': runId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8f-campaign-execution.openapi.json
  Future<VendorResponse> getCrmCampaignsCampaignIdRunsRunIdDeliveries({
    required String campaignId,
    required String runId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/campaigns/{campaignId}/runs/{runId}/deliveries',
    pathParameters: {'campaignId': campaignId, 'runId': runId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section8f-campaign-execution.openapi.json
  Future<VendorResponse>
  getCrmCampaignsCampaignIdRunsRunIdDeliveriesDeliveryId({
    required String campaignId,
    required String runId,
    required String deliveryId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/campaigns/{campaignId}/runs/{runId}/deliveries/{deliveryId}',
    pathParameters: {
      'campaignId': campaignId,
      'runId': runId,
      'deliveryId': deliveryId,
    },
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section8f-campaign-execution.openapi.json
  Future<VendorResponse>
  getCrmCampaignsCampaignIdRunsRunIdDeliveriesDeliveryIdHistory({
    required String campaignId,
    required String runId,
    required String deliveryId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/campaigns/{campaignId}/runs/{runId}/deliveries/{deliveryId}/history',
    pathParameters: {
      'campaignId': campaignId,
      'runId': runId,
      'deliveryId': deliveryId,
    },
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/campaign_execution_http.go#PauseCampaignRun
  Future<VendorResponse> postCrmCampaignsCampaignIdRunsRunIdPause({
    required String campaignId,
    required String runId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCampaignsCampaignIdRunsRunIdPauseRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/campaigns/{campaignId}/runs/{runId}/pause',
    pathParameters: {'campaignId': campaignId, 'runId': runId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/campaign_execution_http.go#ResumeCampaignRun
  Future<VendorResponse> postCrmCampaignsCampaignIdRunsRunIdResume({
    required String campaignId,
    required String runId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCampaignsCampaignIdRunsRunIdResumeRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/campaigns/{campaignId}/runs/{runId}/resume',
    pathParameters: {'campaignId': campaignId, 'runId': runId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8e-crm-campaign-intelligence.openapi.json
  Future<VendorResponse> getCrmCampaignsCampaignIdSnapshots({
    required String campaignId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/campaigns/{campaignId}/snapshots',
    pathParameters: {'campaignId': campaignId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section8e-crm-campaign-intelligence.openapi.json
  Future<VendorResponse> getCrmCampaignsCampaignIdSnapshotsSnapshotId({
    required String campaignId,
    required String snapshotId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/campaigns/{campaignId}/snapshots/{snapshotId}',
    pathParameters: {'campaignId': campaignId, 'snapshotId': snapshotId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section8e-crm-campaign-intelligence.openapi.json
  Future<VendorResponse> getCrmCampaignsCampaignIdSnapshotsSnapshotIdTargets({
    required String campaignId,
    required String snapshotId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/campaigns/{campaignId}/snapshots/{snapshotId}/targets',
    pathParameters: {'campaignId': campaignId, 'snapshotId': snapshotId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section8g-crm-corporate-customers.openapi.json
  Future<VendorResponse> getCrmCorporateCustomers({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/corporate-customers',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#CreateCorporateCustomer
  Future<VendorResponse> postCrmCorporateCustomers({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8g-crm-corporate-customers.openapi.json
  Future<VendorResponse> getCrmCorporateCustomersCorporateId({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#UpdateCorporateCustomer
  Future<VendorResponse> patchCrmCorporateCustomersCorporateId({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchCrmCorporateCustomersCorporateIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8g-crm-corporate-customers.openapi.json
  Future<VendorResponse> postCrmCorporateCustomersCorporateIdActivate({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdActivateRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/activate',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8g-crm-corporate-customers.openapi.json
  Future<VendorResponse> postCrmCorporateCustomersCorporateIdArchive({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/archive',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8g-crm-corporate-customers.openapi.json
  Future<VendorResponse> getCrmCorporateCustomersCorporateIdBranches({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/branches',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#ReplaceCorporateBranchScopes
  Future<VendorResponse> putCrmCorporateCustomersCorporateIdBranches({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutCrmCorporateCustomersCorporateIdBranchesRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/branches',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8g-crm-corporate-customers.openapi.json
  Future<VendorResponse> getCrmCorporateCustomersCorporateIdContacts({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/contacts',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#CreateCorporateContact
  Future<VendorResponse> postCrmCorporateCustomersCorporateIdContacts({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdContactsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/contacts',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8g-crm-corporate-customers.openapi.json
  Future<VendorResponse> getCrmCorporateCustomersCorporateIdContactsContactId({
    required String corporateId,
    required String contactId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/contacts/{contactId}',
    pathParameters: {'corporateId': corporateId, 'contactId': contactId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#UpdateCorporateContact
  Future<VendorResponse>
  patchCrmCorporateCustomersCorporateIdContactsContactId({
    required String corporateId,
    required String contactId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchCrmCorporateCustomersCorporateIdContactsContactIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/contacts/{contactId}',
    pathParameters: {'corporateId': corporateId, 'contactId': contactId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#ArchiveCorporateContact
  Future<VendorResponse>
  postCrmCorporateCustomersCorporateIdContactsContactIdArchive({
    required String corporateId,
    required String contactId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdContactsContactIdArchiveRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/contacts/{contactId}/archive',
    pathParameters: {'corporateId': corporateId, 'contactId': contactId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8g-crm-corporate-customers.openapi.json
  Future<VendorResponse> getCrmCorporateCustomersCorporateIdCostCenters({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/cost-centers',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#CreateCorporateCostCenter
  Future<VendorResponse> postCrmCorporateCustomersCorporateIdCostCenters({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdCostCentersRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/cost-centers',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8g-crm-corporate-customers.openapi.json
  Future<VendorResponse>
  getCrmCorporateCustomersCorporateIdCostCentersCostCenterId({
    required String corporateId,
    required String costCenterId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/cost-centers/{costCenterId}',
    pathParameters: {'corporateId': corporateId, 'costCenterId': costCenterId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#UpdateCorporateCostCenter
  Future<VendorResponse>
  patchCrmCorporateCustomersCorporateIdCostCentersCostCenterId({
    required String corporateId,
    required String costCenterId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchCrmCorporateCustomersCorporateIdCostCentersCostCenterIdRequest
    body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/cost-centers/{costCenterId}',
    pathParameters: {'corporateId': corporateId, 'costCenterId': costCenterId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#ArchiveCorporateCostCenter
  Future<VendorResponse>
  postCrmCorporateCustomersCorporateIdCostCentersCostCenterIdArchive({
    required String corporateId,
    required String costCenterId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdCostCentersCostCenterIdArchiveRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/cost-centers/{costCenterId}/archive',
    pathParameters: {'corporateId': corporateId, 'costCenterId': costCenterId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8g-crm-corporate-customers.openapi.json
  Future<VendorResponse> postCrmCorporateCustomersCorporateIdDeactivate({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdDeactivateRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/deactivate',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8g-crm-corporate-customers.openapi.json
  Future<VendorResponse> postCrmCorporateCustomersCorporateIdHold({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdHoldRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/hold',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8g-crm-corporate-customers.openapi.json
  Future<VendorResponse> getCrmCorporateCustomersCorporateIdInteractions({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/interactions',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#CreateCorporateInteraction
  Future<VendorResponse> postCrmCorporateCustomersCorporateIdInteractions({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdInteractionsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/interactions',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8g-crm-corporate-customers.openapi.json
  Future<VendorResponse>
  getCrmCorporateCustomersCorporateIdInteractionsInteractionId({
    required String corporateId,
    required String interactionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/interactions/{interactionId}',
    pathParameters: {
      'corporateId': corporateId,
      'interactionId': interactionId,
    },
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#CorrectCorporateInteraction
  Future<VendorResponse>
  postCrmCorporateCustomersCorporateIdInteractionsInteractionIdCorrect({
    required String corporateId,
    required String interactionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdInteractionsInteractionIdCorrectRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/interactions/{interactionId}/correct',
    pathParameters: {
      'corporateId': corporateId,
      'interactionId': interactionId,
    },
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8g-crm-corporate-customers.openapi.json
  Future<VendorResponse> getCrmCorporateCustomersCorporateIdLocations({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/locations',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#CreateCorporateLocation
  Future<VendorResponse> postCrmCorporateCustomersCorporateIdLocations({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdLocationsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/locations',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8g-crm-corporate-customers.openapi.json
  Future<VendorResponse>
  getCrmCorporateCustomersCorporateIdLocationsLocationId({
    required String corporateId,
    required String locationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/locations/{locationId}',
    pathParameters: {'corporateId': corporateId, 'locationId': locationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#UpdateCorporateLocation
  Future<VendorResponse>
  patchCrmCorporateCustomersCorporateIdLocationsLocationId({
    required String corporateId,
    required String locationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchCrmCorporateCustomersCorporateIdLocationsLocationIdRequest
    body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/locations/{locationId}',
    pathParameters: {'corporateId': corporateId, 'locationId': locationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#ArchiveCorporateLocation
  Future<VendorResponse>
  postCrmCorporateCustomersCorporateIdLocationsLocationIdArchive({
    required String corporateId,
    required String locationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdLocationsLocationIdArchiveRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/locations/{locationId}/archive',
    pathParameters: {'corporateId': corporateId, 'locationId': locationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8g-crm-corporate-customers.openapi.json
  Future<VendorResponse> getCrmCorporateCustomersCorporateIdMemberships({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/memberships',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#CreateCorporateMembership
  Future<VendorResponse> postCrmCorporateCustomersCorporateIdMemberships({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdMembershipsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/memberships',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#UpdateCorporateMembership
  Future<VendorResponse>
  patchCrmCorporateCustomersCorporateIdMembershipsMembershipId({
    required String corporateId,
    required String membershipId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchCrmCorporateCustomersCorporateIdMembershipsMembershipIdRequest
    body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/memberships/{membershipId}',
    pathParameters: {'corporateId': corporateId, 'membershipId': membershipId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#ArchiveCorporateMembership
  Future<VendorResponse>
  postCrmCorporateCustomersCorporateIdMembershipsMembershipIdArchive({
    required String corporateId,
    required String membershipId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdMembershipsMembershipIdArchiveRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/memberships/{membershipId}/archive',
    pathParameters: {'corporateId': corporateId, 'membershipId': membershipId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8g-crm-corporate-customers.openapi.json
  Future<VendorResponse> getCrmCorporateCustomersCorporateIdTasks({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/tasks',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#CreateCorporateTask
  Future<VendorResponse> postCrmCorporateCustomersCorporateIdTasks({
    required String corporateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdTasksRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/tasks',
    pathParameters: {'corporateId': corporateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8g-crm-corporate-customers.openapi.json
  Future<VendorResponse> getCrmCorporateCustomersCorporateIdTasksTaskId({
    required String corporateId,
    required String taskId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/tasks/{taskId}',
    pathParameters: {'corporateId': corporateId, 'taskId': taskId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#UpdateCorporateTask
  Future<VendorResponse> patchCrmCorporateCustomersCorporateIdTasksTaskId({
    required String corporateId,
    required String taskId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchCrmCorporateCustomersCorporateIdTasksTaskIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/tasks/{taskId}',
    pathParameters: {'corporateId': corporateId, 'taskId': taskId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#CancelCorporateTask
  Future<VendorResponse> postCrmCorporateCustomersCorporateIdTasksTaskIdCancel({
    required String corporateId,
    required String taskId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdTasksTaskIdCancelRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/tasks/{taskId}/cancel',
    pathParameters: {'corporateId': corporateId, 'taskId': taskId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#CompleteCorporateTask
  Future<VendorResponse>
  postCrmCorporateCustomersCorporateIdTasksTaskIdComplete({
    required String corporateId,
    required String taskId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdTasksTaskIdCompleteRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/tasks/{taskId}/complete',
    pathParameters: {'corporateId': corporateId, 'taskId': taskId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#ReassignCorporateTask
  Future<VendorResponse>
  postCrmCorporateCustomersCorporateIdTasksTaskIdReassign({
    required String corporateId,
    required String taskId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdTasksTaskIdReassignRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/tasks/{taskId}/reassign',
    pathParameters: {'corporateId': corporateId, 'taskId': taskId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/corporate_http.go#StartCorporateTask
  Future<VendorResponse> postCrmCorporateCustomersCorporateIdTasksTaskIdStart({
    required String corporateId,
    required String taskId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCorporateCustomersCorporateIdTasksTaskIdStartRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/corporate-customers/{corporateId}/tasks/{taskId}/start',
    pathParameters: {'corporateId': corporateId, 'taskId': taskId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8a-crm-foundation.openapi.json
  Future<VendorResponse> getCrmCustomers({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/customers',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section8a-crm-foundation.openapi.json
  Future<VendorResponse> getCrmCustomersProfileId({
    required String profileId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/customers/{profileId}',
    pathParameters: {'profileId': profileId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section8a-crm-foundation.openapi.json
  Future<VendorResponse> postCrmCustomersProfileIdArchive({
    required String profileId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/customers/{profileId}/archive',
    pathParameters: {'profileId': profileId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8a-crm-foundation.openapi.json
  Future<VendorResponse> getCrmCustomersProfileIdConsent({
    required String profileId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/customers/{profileId}/consent',
    pathParameters: {'profileId': profileId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section8h-crm-intelligence-exports.openapi.json
  Future<VendorResponse> getCrmCustomersProfileIdIntelligence({
    required String profileId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/customers/{profileId}/intelligence',
    pathParameters: {'profileId': profileId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section8c-crm-interactions-tasks.openapi.json
  Future<VendorResponse> getCrmCustomersProfileIdInteractions({
    required String profileId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/customers/{profileId}/interactions',
    pathParameters: {'profileId': profileId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section8d-crm-loyalty.openapi.json
  Future<VendorResponse> getCrmCustomersProfileIdLoyalty({
    required String profileId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/customers/{profileId}/loyalty',
    pathParameters: {'profileId': profileId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/loyalty_http.go#AdjustCustomerLoyalty
  Future<VendorResponse> postCrmCustomersProfileIdLoyaltyAdjustments({
    required String profileId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmCustomersProfileIdLoyaltyAdjustmentsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/customers/{profileId}/loyalty/adjustments',
    pathParameters: {'profileId': profileId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8d-crm-loyalty.openapi.json
  Future<VendorResponse> getCrmCustomersProfileIdLoyaltyEntries({
    required String profileId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/customers/{profileId}/loyalty/entries',
    pathParameters: {'profileId': profileId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section8a-crm-foundation.openapi.json
  Future<VendorResponse> getCrmCustomersProfileIdPrivacy({
    required String profileId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/customers/{profileId}/privacy',
    pathParameters: {'profileId': profileId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section8a-crm-foundation.openapi.json
  Future<VendorResponse> postCrmCustomersProfileIdRestore({
    required String profileId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/customers/{profileId}/restore',
    pathParameters: {'profileId': profileId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8c-crm-interactions-tasks.openapi.json
  Future<VendorResponse> getCrmCustomersProfileIdTasks({
    required String profileId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/customers/{profileId}/tasks',
    pathParameters: {'profileId': profileId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section8h-crm-intelligence-exports.openapi.json
  Future<VendorResponse> getCrmExports({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/exports',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/exports_http.go#CreateExport
  Future<VendorResponse> postCrmExports({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmExportsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/exports',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8h-crm-intelligence-exports.openapi.json
  Future<VendorResponse> getCrmExportsExportId({
    required String exportId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/exports/{exportId}',
    pathParameters: {'exportId': exportId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/exports_http.go#CancelExport
  Future<VendorResponse> postCrmExportsExportIdCancel({
    required String exportId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmExportsExportIdCancelRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/exports/{exportId}/cancel',
    pathParameters: {'exportId': exportId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/exports_http.go#DownloadExport
  Future<VendorResponse> postCrmExportsExportIdDownload({
    required String exportId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmExportsExportIdDownloadRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/exports/{exportId}/download',
    pathParameters: {'exportId': exportId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8h-crm-intelligence-exports.openapi.json
  Future<VendorResponse> getCrmIntelligenceChurn({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/intelligence/churn',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section8h-crm-intelligence-exports.openapi.json
  Future<VendorResponse> getCrmIntelligenceOverview({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/intelligence/overview',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/intelligence_http.go#RebuildIntelligence
  Future<VendorResponse> postCrmIntelligenceRebuild({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmIntelligenceRebuildRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/intelligence/rebuild',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8h-crm-intelligence-exports.openapi.json
  Future<VendorResponse> getCrmIntelligenceRfm({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/intelligence/rfm',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section8c-crm-interactions-tasks.openapi.json
  Future<VendorResponse> getCrmInteractions({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/interactions',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/activity_http.go#CreateInteraction
  Future<VendorResponse> postCrmInteractions({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmInteractionsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/interactions',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8c-crm-interactions-tasks.openapi.json
  Future<VendorResponse> getCrmInteractionsInteractionId({
    required String interactionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/interactions/{interactionId}',
    pathParameters: {'interactionId': interactionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/activity_http.go#CorrectInteraction
  Future<VendorResponse> postCrmInteractionsInteractionIdCorrect({
    required String interactionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmInteractionsInteractionIdCorrectRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/interactions/{interactionId}/correct',
    pathParameters: {'interactionId': interactionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8c-crm-interactions-tasks.openapi.json
  Future<VendorResponse> getCrmInteractionsInteractionIdRevisions({
    required String interactionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/interactions/{interactionId}/revisions',
    pathParameters: {'interactionId': interactionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/loyalty_http.go#ReverseLoyaltyEntry
  Future<VendorResponse> postCrmLoyaltyEntriesEntryIdReverse({
    required String entryId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmLoyaltyEntriesEntryIdReverseRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/loyalty/entries/{entryId}/reverse',
    pathParameters: {'entryId': entryId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8d-crm-loyalty.openapi.json
  Future<VendorResponse> getCrmLoyaltyProgram({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/loyalty/program',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/loyalty_http.go#UpdateLoyaltyProgram
  Future<VendorResponse> patchCrmLoyaltyProgram({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchCrmLoyaltyProgramRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/crm/loyalty/program',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8-completion-crm.openapi.json
  Future<VendorResponse> getCrmLoyaltyTiers({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/loyalty/tiers',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/loyalty_tiers_http.go#ReplaceLoyaltyTiers
  Future<VendorResponse> putCrmLoyaltyTiers({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutCrmLoyaltyTiersRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/crm/loyalty/tiers',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8b-crm-tags-segmentation.openapi.json
  Future<VendorResponse> getCrmSegmentFields({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/segment-fields',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section8b-crm-tags-segmentation.openapi.json
  Future<VendorResponse> getCrmSegments({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/segments',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/segments_http.go#CreateSegment
  Future<VendorResponse> postCrmSegments({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmSegmentsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/segments',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/segments_http.go#PreviewSegment
  Future<VendorResponse> postCrmSegmentsPreview({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmSegmentsPreviewRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/segments/preview',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8b-crm-tags-segmentation.openapi.json
  Future<VendorResponse> getCrmSegmentsSegmentId({
    required String segmentId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/segments/{segmentId}',
    pathParameters: {'segmentId': segmentId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/segments_http.go#UpdateSegment
  Future<VendorResponse> patchCrmSegmentsSegmentId({
    required String segmentId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchCrmSegmentsSegmentIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/crm/segments/{segmentId}',
    pathParameters: {'segmentId': segmentId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/segments_http.go#ActivateSegment
  Future<VendorResponse> postCrmSegmentsSegmentIdActivate({
    required String segmentId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmSegmentsSegmentIdActivateRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/segments/{segmentId}/activate',
    pathParameters: {'segmentId': segmentId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/segments_http.go#ArchiveSegment
  Future<VendorResponse> postCrmSegmentsSegmentIdArchive({
    required String segmentId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmSegmentsSegmentIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/segments/{segmentId}/archive',
    pathParameters: {'segmentId': segmentId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/segments_http.go#BulkAddExclusions
  Future<VendorResponse> postCrmSegmentsSegmentIdExclusionsBulkAdd({
    required String segmentId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmSegmentsSegmentIdExclusionsBulkAddRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/segments/{segmentId}/exclusions/bulk-add',
    pathParameters: {'segmentId': segmentId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/segments_http.go#BulkRemoveExclusions
  Future<VendorResponse> postCrmSegmentsSegmentIdExclusionsBulkRemove({
    required String segmentId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmSegmentsSegmentIdExclusionsBulkRemoveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/segments/{segmentId}/exclusions/bulk-remove',
    pathParameters: {'segmentId': segmentId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8b-crm-tags-segmentation.openapi.json
  Future<VendorResponse> postCrmSegmentsSegmentIdMaterialize({
    required String segmentId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/segments/{segmentId}/materialize',
    pathParameters: {'segmentId': segmentId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8b-crm-tags-segmentation.openapi.json
  Future<VendorResponse> getCrmSegmentsSegmentIdMembers({
    required String segmentId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/segments/{segmentId}/members',
    pathParameters: {'segmentId': segmentId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/segments_http.go#BulkAddMembers
  Future<VendorResponse> postCrmSegmentsSegmentIdMembersBulkAdd({
    required String segmentId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmSegmentsSegmentIdMembersBulkAddRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/segments/{segmentId}/members/bulk-add',
    pathParameters: {'segmentId': segmentId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/segments_http.go#BulkRemoveMembers
  Future<VendorResponse> postCrmSegmentsSegmentIdMembersBulkRemove({
    required String segmentId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmSegmentsSegmentIdMembersBulkRemoveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/segments/{segmentId}/members/bulk-remove',
    pathParameters: {'segmentId': segmentId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8b-crm-tags-segmentation.openapi.json
  Future<VendorResponse> getCrmTags({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/tags',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/segments_http.go#CreateTag
  Future<VendorResponse> postCrmTags({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmTagsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/tags',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/segments_http.go#BulkAssignTags
  Future<VendorResponse> postCrmTagsBulkAssign({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmTagsBulkAssignRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/tags/bulk-assign',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/segments_http.go#BulkRemoveTags
  Future<VendorResponse> postCrmTagsBulkRemove({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmTagsBulkRemoveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/tags/bulk-remove',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8b-crm-tags-segmentation.openapi.json
  Future<VendorResponse> getCrmTagsTagId({
    required String tagId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/tags/{tagId}',
    pathParameters: {'tagId': tagId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/segments_http.go#UpdateTag
  Future<VendorResponse> patchCrmTagsTagId({
    required String tagId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchCrmTagsTagIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/crm/tags/{tagId}',
    pathParameters: {'tagId': tagId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/crm/segments_http.go#ArchiveTag
  Future<VendorResponse> postCrmTagsTagIdArchive({
    required String tagId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmTagsTagIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/tags/{tagId}/archive',
    pathParameters: {'tagId': tagId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8c-crm-interactions-tasks.openapi.json
  Future<VendorResponse> getCrmTasks({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/tasks',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/activity_http.go#CreateTask
  Future<VendorResponse> postCrmTasks({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmTasksRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/tasks',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8c-crm-interactions-tasks.openapi.json
  Future<VendorResponse> getCrmTasksTaskId({
    required String taskId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/tasks/{taskId}',
    pathParameters: {'taskId': taskId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/activity_http.go#UpdateTask
  Future<VendorResponse> patchCrmTasksTaskId({
    required String taskId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchCrmTasksTaskIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/crm/tasks/{taskId}',
    pathParameters: {'taskId': taskId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8c-crm-interactions-tasks.openapi.json
  Future<VendorResponse> postCrmTasksTaskIdCancel({
    required String taskId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmTasksTaskIdCancelRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/tasks/{taskId}/cancel',
    pathParameters: {'taskId': taskId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8c-crm-interactions-tasks.openapi.json
  Future<VendorResponse> postCrmTasksTaskIdComplete({
    required String taskId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmTasksTaskIdCompleteRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/tasks/{taskId}/complete',
    pathParameters: {'taskId': taskId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8c-crm-interactions-tasks.openapi.json
  Future<VendorResponse> getCrmTasksTaskIdHistory({
    required String taskId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/crm/tasks/{taskId}/history',
    pathParameters: {'taskId': taskId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/crm/activity_http.go#ReassignTask
  Future<VendorResponse> postCrmTasksTaskIdReassign({
    required String taskId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmTasksTaskIdReassignRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/tasks/{taskId}/reassign',
    pathParameters: {'taskId': taskId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section8c-crm-interactions-tasks.openapi.json
  Future<VendorResponse> postCrmTasksTaskIdStart({
    required String taskId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCrmTasksTaskIdStartRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/crm/tasks/{taskId}/start',
    pathParameters: {'taskId': taskId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );
}
